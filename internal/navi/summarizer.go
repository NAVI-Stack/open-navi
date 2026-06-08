package navi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/prompts"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

const defaultChatSummaryBatchSize = 20

// ChatSummarizer compacts older chat turns into durable memory before prompt assembly.
type ChatSummarizer interface {
	MaybeSummarize(ctx context.Context, chatID string) error
	SummarizeChat(ctx context.Context, chatID string) error
}

// LLMChatSummarizer batches messages and persists structured summaries + facts.
type LLMChatSummarizer struct {
	db             *sql.DB
	llm            llm.Provider
	model          string
	routeLLM       func(ctx context.Context, req llm.RouteRequest) (llm.RouteDecision, error)
	resolveOwnerID func(ctx context.Context, chatID string) string
	prompts        *prompts.Manager
	batchSize      int
}

// ChatSummary is the JSON shape returned by the summarization LLM call.
type ChatSummary struct {
	ChatID          string                 `json:"chat_id"`
	TurnCount       int                    `json:"turn_count"`
	TopicSummary    string                 `json:"topic_summary"`
	KeyDecisions    []string               `json:"key_decisions"`
	FactsLearned    []schema.ExtractedFact `json:"facts_learned"`
	UnresolvedItems []string               `json:"unresolved_items"`
	ToolsUsed       []string               `json:"tools_used"`
}

// NewChatSummarizer wires batch summarization against the SQLite chat + memories tables.
func NewChatSummarizer(
	db *sql.DB,
	llmProv llm.Provider,
	model string,
	routeLLM func(ctx context.Context, req llm.RouteRequest) (llm.RouteDecision, error),
	resolveOwnerID func(ctx context.Context, chatID string) string,
	pm *prompts.Manager,
) *LLMChatSummarizer {
	return &LLMChatSummarizer{
		db:             db,
		llm:            llmProv,
		model:          model,
		routeLLM:       routeLLM,
		resolveOwnerID: resolveOwnerID,
		prompts:        pm,
		batchSize:      defaultChatSummaryBatchSize,
	}
}

// MaybeSummarize runs when the chat crosses the next batch threshold.
func (s *LLMChatSummarizer) MaybeSummarize(ctx context.Context, chatID string) error {
	return s.summarize(ctx, chatID, false)
}

// SummarizeChat forces summarization of any remaining chat messages.
func (s *LLMChatSummarizer) SummarizeChat(ctx context.Context, chatID string) error {
	return s.summarize(ctx, chatID, true)
}

func (s *LLMChatSummarizer) summarize(ctx context.Context, chatID string, includeRemainder bool) error {
	if s == nil || s.db == nil || s.llm == nil || strings.TrimSpace(chatID) == "" {
		return nil
	}

	messageCount, err := s.countMessages(ctx, chatID)
	if err != nil {
		return err
	}
	if !includeRemainder && messageCount < s.batchSize {
		return nil
	}

	completedBatches, err := s.countCompletedBatches(ctx, chatID)
	if err != nil {
		return err
	}
	nextBatch := completedBatches + 1
	if !includeRemainder && messageCount < nextBatch*s.batchSize {
		return nil
	}

	offset := completedBatches * s.batchSize
	limit := s.batchSize
	if includeRemainder {
		remaining := messageCount - offset
		if remaining <= 0 {
			return nil
		}
		limit = remaining
	}
	messages, err := s.loadBatch(ctx, chatID, offset, limit)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	model := s.model
	if s.routeLLM != nil {
		if decision, routeErr := s.routeLLM(ctx, llm.RouteRequest{
			UserMessage: "thanks",
			Tools:       nil,
			History:     nil,
		}); routeErr == nil && strings.TrimSpace(decision.Model) != "" {
			model = decision.Model
		}
	}

	pm := s.prompts
	if pm == nil {
		var perr error
		pm, perr = prompts.EmbeddedManager()
		if perr != nil {
			return perr
		}
	}
	sysContent, err := pm.Render(prompts.KindSummarizerSystem, struct{}{}, prompts.RenderOptions{})
	if err != nil {
		return fmt.Errorf("chat summarizer system prompt: %w", err)
	}
	userContent, err := pm.Render(prompts.KindSummarizerUser, prompts.SummarizerUserData{
		ChatID:       chatID,
		MessagesText: formatChatSummaryMessagesText(messages),
	}, prompts.RenderOptions{})
	if err != nil {
		return fmt.Errorf("chat summarizer user prompt: %w", err)
	}

	resp, err := s.llm.Chat(ctx, model, []llm.Message{
		{Role: "system", Content: sysContent},
		{Role: "user", Content: userContent},
	}, nil, llm.Options{Temperature: 0.1, MaxTokens: 1200})
	if err != nil {
		return fmt.Errorf("chat summarizer llm: %w", err)
	}

	summary, err := decodeChatSummary(resp.Content)
	if err != nil {
		return err
	}
	if strings.TrimSpace(summary.TopicSummary) == "" {
		return nil
	}
	summary.ChatID = chatID
	summary.TurnCount = len(messages)

	return s.persistSummary(ctx, chatID, nextBatch, summary)
}

func formatChatSummaryMessagesText(messages []chatSummaryMessage) string {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString("- " + msg.Role + ": " + msg.Content + "\n")
	}
	return b.String()
}

func (s *LLMChatSummarizer) persistSummary(ctx context.Context, chatID string, batch int, summary ChatSummary) error {
	detailsJSON, _ := json.Marshal(summary)
	chatMemory := schema.Memory{
		ID:           fmt.Sprintf("chat-summary:%s:%d", chatID, batch),
		Scope:        "chat",
		ScopeID:      chatID,
		Summary:      summary.TopicSummary,
		Details:      string(detailsJSON),
		Significance: "medium",
		Source:       "chat_summarizer",
	}
	if _, err := store.SaveMemory(ctx, s.db, chatMemory); err != nil {
		return err
	}

	ownerID := ""
	if s.resolveOwnerID != nil {
		ownerID = s.resolveOwnerID(ctx, chatID)
	}
	if ownerID != "" {
		ownerMemory := chatMemory
		ownerMemory.ID = fmt.Sprintf("owner-chat-summary:%s:%d", chatID, batch)
		ownerMemory.Scope = "owner"
		ownerMemory.ScopeID = ownerID
		if _, err := store.SaveMemory(ctx, s.db, ownerMemory); err != nil {
			return err
		}

		for idx, decision := range summary.KeyDecisions {
			decision = strings.TrimSpace(decision)
			if decision == "" {
				continue
			}
			if err := store.SaveFact(ctx, s.db, store.Fact{
				ID:       fmt.Sprintf("chat-decision:%s:%d:%d", chatID, batch, idx),
				Scope:    "owner",
				ScopeID:  ownerID,
				Category: "project_decision",
				Key:      fmt.Sprintf("chat_%s_batch_%d_decision_%d", compactKey(chatID), batch, idx+1),
				Value:    decision,
				Source:   "chat_summarizer",
			}); err != nil {
				return err
			}
		}

		for idx, fact := range summary.FactsLearned {
			if strings.TrimSpace(fact.Key) == "" || strings.TrimSpace(fact.Value) == "" {
				continue
			}
			category := strings.TrimSpace(fact.Category)
			if category == "" {
				category = "technical_context"
			}
			scope := strings.TrimSpace(fact.Scope)
			scopeID := ownerID
			if scope == "" || scope == "owner" {
				scope = "owner"
				scopeID = ownerID
			}
			if scope == "chat" {
				scope = "chat"
				scopeID = chatID
			}
			if err := store.SaveFact(ctx, s.db, store.Fact{
				ID:       fmt.Sprintf("chat-fact:%s:%d:%d", chatID, batch, idx),
				Scope:    scope,
				ScopeID:  scopeID,
				Category: category,
				Key:      fact.Key,
				Value:    fact.Value,
				Source:   "chat_summarizer",
			}); err != nil {
				return err
			}
		}
	}

	return nil
}

type chatSummaryMessage struct {
	Role    string
	Content string
}

func (s *LLMChatSummarizer) countMessages(ctx context.Context, chatID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM navi_chat_messages WHERE chat_id = ?`, chatID).Scan(&count)
	return count, err
}

func (s *LLMChatSummarizer) countCompletedBatches(ctx context.Context, chatID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM memories
		WHERE scope_id = ?
		  AND (
		    scope = 'chat' AND source = 'chat_summarizer'
		  )
	`, chatID).Scan(&count)
	return count, err
}

func (s *LLMChatSummarizer) loadBatch(ctx context.Context, chatID string, offset, limit int) ([]chatSummaryMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT role, content
		FROM navi_chat_messages
		WHERE chat_id = ?
		ORDER BY created_at ASC
		LIMIT ? OFFSET ?
	`, chatID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []chatSummaryMessage
	for rows.Next() {
		var msg chatSummaryMessage
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func decodeChatSummary(raw string) (ChatSummary, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var summary ChatSummary
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		return ChatSummary{}, fmt.Errorf("decode chat summary: %w", err)
	}
	return summary, nil
}

func compactKey(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if len(v) > 12 {
		v = v[:12]
	}
	v = strings.ReplaceAll(v, " ", "-")
	return strings.ReplaceAll(v, "_", "-")
}

// chatSummaryBlock injects the latest rolling summary for long chats (from batch summarizer memories).
func (l *AgentLoop) chatSummaryBlock(ctx context.Context, chatID string) string {
	if structured := strings.TrimSpace(l.chatCompactionBlock(ctx, chatID)); structured != "" {
		return "Earlier chat continuity state:\n" + structured
	}
	// Structured compaction chats must not fall back to rolling summary as a canonical continuity source.
	if l.isStructuredCompactionEnabled() {
		return ""
	}
	// Migration fallback: rolling summary bridge is non-canonical and only used when structured state is unavailable.
	if l.cfg.WorldModel == nil {
		return ""
	}
	db := l.cfg.WorldModel.DB()
	if db == nil {
		return ""
	}
	memories, err := store.ListMemories(ctx, db, "chat", chatID, 20)
	if err != nil || len(memories) == 0 {
		return ""
	}
	var latest *schema.Memory
	for i := range memories {
		if memories[i].Source != "chat_summarizer" {
			continue
		}
		if latest == nil || memories[i].UpdatedAt.After(latest.UpdatedAt) {
			latest = &memories[i]
		}
	}
	if latest == nil {
		return ""
	}
	var summary ChatSummary
	if err := json.Unmarshal([]byte(latest.Details), &summary); err != nil {
		return "\n\n## Earlier chat summary\n\n- " + latest.Summary + "\n"
	}
	var b strings.Builder
	b.WriteString("\n\n## Earlier chat summary\n\n")
	if strings.TrimSpace(summary.TopicSummary) != "" {
		b.WriteString("- " + strings.TrimSpace(summary.TopicSummary) + "\n")
	}
	for _, item := range summary.UnresolvedItems {
		item = strings.TrimSpace(item)
		if item != "" {
			b.WriteString("- Open item: " + item + "\n")
		}
	}
	return b.String()
}
