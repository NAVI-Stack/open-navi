package gateway

// OpenAI-compatible /v1/chat/completions bridge for NAVI.
//
// Lets any OpenAI-formatted client (ai-chat, open-webui, etc.) talk to NAVI
// without modification. Point the client at the NAVI gateway URL and set the
// model to "navi". Unsupported persona-style model aliases are rejected.
//
// Routes registered in server.go:
//   POST /v1/chat/completions  → handleOpenAIChatCompletions
//   GET  /v1/models            → handleOpenAIModels

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/navi"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
	"github.com/google/uuid"
)

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// handleOpenAIChatCompletions handles POST /v1/chat/completions.
func (s *Server) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI not enabled")
		return
	}

	var req openAIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	userContent := oaiLastUserMessage(req.Messages)
	if userContent == "" {
		replyError(w, http.StatusBadRequest, "no user message found in messages array")
		return
	}

	ctx := r.Context()
	experienceMode, err := oaiModelToExperienceMode(req.Model)
	if err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}

	chatID, err := oaiResolveChat(ctx, s.cfg.Navi, experienceMode)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "chat resolution error: "+err.Error())
		return
	}

	// Snapshot current chat event seq before sending so we only watch for new runtime events.
	seqBefore, _ := oaiMaxChatSeq(ctx, s.cfg.DB, chatID)

	if _, err := s.cfg.Navi.SendMessageInput(ctx, chatID, naviruntime.MessageInput{
		Content:       userContent,
		SourceChannel: "openai",
	}); err != nil {
		replyError(w, http.StatusInternalServerError, "send message: "+err.Error())
		return
	}

	id := "chatcmpl-" + uuid.NewString()
	model := req.Model
	if model == "" {
		model = "navi"
	}

	if req.Stream {
		if err := oaiStreamReply(ctx, w, s.cfg.DB, chatID, seqBefore, id, model); err != nil {
			replyError(w, http.StatusGatewayTimeout, "timed out waiting for NAVI reply")
		}
		return
	}

	replyContent, err := oaiWaitForReply(ctx, s.cfg.DB, chatID, seqBefore)
	if err != nil {
		replyError(w, http.StatusGatewayTimeout, "timed out waiting for NAVI reply")
		return
	}
	oaiWriteJSON(w, id, model, replyContent)
}

// handleOpenAIModels handles GET /v1/models.
// Returns a single default model so clients do not show a experience-mode switcher; legacy persona choices are not exposed.
func (s *Server) handleOpenAIModels(w http.ResponseWriter, r *http.Request) {
	now := time.Now().Unix()
	models := []map[string]any{
		{"id": "navi", "object": "model", "created": now, "owned_by": "navi"},
	}
	replyJSON(w, http.StatusOK, map[string]any{"object": "list", "data": models})
}

// --- internal helpers ---

func oaiLastUserMessage(msgs []openAIMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return strings.TrimSpace(msgs[i].Content)
		}
	}
	return ""
}

func oaiModelToExperienceMode(model string) (navi.ExperienceMode, error) {
	normalized := strings.ToLower(strings.TrimSpace(model))
	switch normalized {
	case "", string(navi.ExperienceModeStandard):
		return navi.ExperienceModeStandard, nil
	default:
		return "", fmt.Errorf("unsupported model %q: only %q is supported", model, navi.ExperienceModeStandard)
	}
}

// "session" here is external protocol vocabulary; internally NAVI resolves it
// immediately to Chat + RuntimeSession and does not expose Session as a domain model.
func oaiResolveChat(ctx context.Context, n *navi.NAVI, experienceMode navi.ExperienceMode) (string, error) {
	chats, err := n.RecentChats(ctx, 10)
	if err == nil {
		for _, c := range chats {
			if chatID := string(c.ID); chatID != "" {
				return chatID, nil
			}
		}
	}
	return n.CreateChat(ctx, experienceMode, "")
}

func oaiMaxChatSeq(ctx context.Context, db *sql.DB, chatID string) (int64, error) {
	var seq int64
	err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM events WHERE correlation_id = ?`, chatID).Scan(&seq)
	return seq, err
}

// oaiWaitForReply polls chat runtime events until the assistant message is committed.
func oaiWaitForReply(ctx context.Context, db *sql.DB, chatID string, afterSeq int64) (string, error) {
	deadline := time.Now().Add(60 * time.Second)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	lastSeq := afterSeq

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return "", fmt.Errorf("deadline exceeded")
			}
			events, err := store.SessionEventsSince(ctx, db, chatID, lastSeq, []schema.EventVisibility{schema.VisibilityUser}, 50)
			if err != nil {
				continue
			}
			for _, ev := range events {
				if ev.Seq > lastSeq {
					lastSeq = ev.Seq
				}
				switch ev.Type {
				case schema.FactAssistantMessageCompleted:
					if kind, ok := oaiEventString(ev.Payload, "message_kind"); ok && kind == string(schema.AssistantMessageKindProactive) {
						continue
					}
					if content, ok := oaiEventString(ev.Payload, "content"); ok && content != "" {
						return content, nil
					}
				case schema.FactRunFailed:
					if msg, ok := oaiEventString(ev.Payload, "error"); ok && msg != "" {
						return "", fmt.Errorf("%s", msg)
					}
				}
			}
		}
	}
}

// oaiStreamReply streams chat runtime events over OpenAI SSE.
func oaiStreamReply(ctx context.Context, w http.ResponseWriter, db *sql.DB, chatID string, afterSeq int64, id, model string) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	now := time.Now().Unix()
	sendChunk := func(delta map[string]any, finishReason any) {
		chunk := map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": now, "model": model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finishReason}},
		}
		b, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", string(b))
		if flusher != nil {
			flusher.Flush()
		}
	}
	sendChunk(map[string]any{"role": "assistant"}, nil)

	deadline := time.Now().Add(60 * time.Second)
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	lastSeq := afterSeq
	streamedAny := false
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			events, err := store.SessionEventsSince(ctx, db, chatID, lastSeq, []schema.EventVisibility{schema.VisibilityUser}, 50)
			if err != nil {
				continue
			}
			for _, ev := range events {
				if ev.Seq > lastSeq {
					lastSeq = ev.Seq
				}
				switch ev.Type {
				case schema.FactAssistantTokenDelta:
					if delta, ok := oaiEventString(ev.Payload, "delta"); ok && delta != "" {
						sendChunk(map[string]any{"content": delta}, nil)
						streamedAny = true
					}
				case schema.FactAssistantMessageCompleted:
					if kind, ok := oaiEventString(ev.Payload, "message_kind"); ok && kind == string(schema.AssistantMessageKindProactive) {
						continue
					}
					if !streamedAny {
						if content, ok := oaiEventString(ev.Payload, "content"); ok && content != "" {
							sendChunk(map[string]any{"content": content}, nil)
						}
					}
					sendChunk(map[string]any{}, "stop")
					fmt.Fprintf(w, "data: [DONE]\n\n")
					if flusher != nil {
						flusher.Flush()
					}
					return nil
				case schema.FactRunFailed:
					if msg, ok := oaiEventString(ev.Payload, "error"); ok && msg != "" {
						return fmt.Errorf("%s", msg)
					}
				}
			}
		}
	}
	return fmt.Errorf("deadline exceeded")
}

func oaiEventString(payload any, key string) (string, bool) {
	if payload == nil {
		return "", false
	}
	if m, ok := payload.(map[string]any); ok {
		v, ok := m[key].(string)
		return v, ok
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", false
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return "", false
	}
	v, ok := m[key].(string)
	return v, ok
}

// oaiWriteStream sends the reply as an OpenAI SSE stream (simulated: one content chunk).
// Used when streaming was not requested; kept for compatibility.
func oaiWriteStream(w http.ResponseWriter, id, model, content string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	f, canFlush := w.(http.Flusher)
	now := time.Now().Unix()

	send := func(delta map[string]any, finishReason any) {
		chunk := map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": now,
			"model":   model,
			"choices": []map[string]any{
				{"index": 0, "delta": delta, "finish_reason": finishReason},
			},
		}
		b, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", string(b))
		if canFlush {
			f.Flush()
		}
	}

	send(map[string]any{"role": "assistant"}, nil)
	send(map[string]any{"content": content}, nil)
	send(map[string]any{}, "stop")
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if canFlush {
		f.Flush()
	}
}

// oaiWriteJSON sends a non-streaming OpenAI chat completion response.
func oaiWriteJSON(w http.ResponseWriter, id, model, content string) {
	replyJSON(w, http.StatusOK, map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       map[string]string{"role": "assistant", "content": content},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	})
}
