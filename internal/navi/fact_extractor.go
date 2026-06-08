package navi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/experience"
	"github.com/ceoai/navi/internal/schema"
)

const factExtractorPrompt = `Extract durable facts from this conversation turn.

Return ONLY a JSON array. Each item must have:
- "key": short snake_case identifier
- "value": concise fact text
- "category": one of "user_preference", "project_decision", "technical_context", "task_outcome"
- "scope": "owner" or "session"

Rules:
- Default to scope "owner" unless the fact is useful only in this conversation.
- Return [] when there is nothing worth remembering.
- Do not include ephemeral greetings, pleasantries, or generic assistant behavior.
- Do not wrap the JSON in prose.`

func (l *AgentLoop) reflectionDetailsForTurn(ctx context.Context, chatID, summary, lastUserContent, replyContent string) string {
	ownerID := ""
	if l.cfg.ResolveOwnerID != nil {
		ownerID = strings.TrimSpace(l.cfg.ResolveOwnerID(ctx, chatID))
	}
	signals := experience.DetectPreferenceSignals(lastUserContent, ownerID, chatID)

	var facts []schema.ExtractedFact
	if shouldExtractFacts(summary, lastUserContent, replyContent) {
		extractedFacts, err := l.extractFacts(ctx, lastUserContent, replyContent)
		if err != nil {
			slog.Debug("navi: fact extraction skipped", "error", err)
		} else {
			facts = extractedFacts
		}
	}
	if len(facts) == 0 && len(signals) == 0 {
		return ""
	}
	for i := range facts {
		switch facts[i].Scope {
		case "owner":
			if ownerID != "" {
				facts[i].ScopeID = ownerID
			}
		case "session":
			facts[i].ScopeID = chatID
		}
	}
	for i := range signals {
		if signals[i].CreatedAt.IsZero() {
			signals[i].CreatedAt = nowUTC()
		}
		if signals[i].UpdatedAt.IsZero() {
			signals[i].UpdatedAt = signals[i].CreatedAt
		}
	}

	details, err := json.Marshal(schema.ReflectionDetails{
		Summary:           summary,
		UserMessage:       lastUserContent,
		Facts:             facts,
		PreferenceSignals: signals,
	})
	if err != nil {
		slog.Debug("navi: failed to marshal reflection details", "error", err)
		return ""
	}
	return string(details)
}

func shouldExtractFacts(summary, lastUserContent, replyContent string) bool {
	if strings.TrimSpace(lastUserContent) == "" || strings.TrimSpace(replyContent) == "" {
		return false
	}
	lowerSummary := strings.ToLower(summary)
	if strings.Contains(lowerSummary, "provider/model query") ||
		strings.Contains(lowerSummary, "router state") ||
		strings.Contains(lowerSummary, "llm error") {
		return false
	}
	if isGreetingOrStatusCheck(lastUserContent) && wordCount(replyContent) < 20 {
		return false
	}
	return wordCount(replyContent) >= 5
}

func (l *AgentLoop) extractFacts(ctx context.Context, userMsg, replyContent string) ([]schema.ExtractedFact, error) {
	if l.cfg.LLM == nil {
		return nil, fmt.Errorf("fact extractor: llm provider not configured")
	}
	msgs := []llm.Message{
		{Role: "system", Content: factExtractorPrompt},
		{Role: "user", Content: fmt.Sprintf("User said:\n%s\n\nNAVI replied:\n%s", userMsg, replyContent)},
	}
	resp, err := l.cfg.LLM.Chat(ctx, l.cfg.Model, msgs, nil, llm.Options{
		Temperature: 0,
		MaxTokens:   400,
	})
	if err != nil {
		return nil, fmt.Errorf("fact extractor: chat: %w", err)
	}

	payload := strings.TrimSpace(resp.Content)
	payload = trimJSONFence(payload)
	if payload == "" {
		return nil, nil
	}

	var facts []schema.ExtractedFact
	if err := json.Unmarshal([]byte(payload), &facts); err != nil {
		return nil, fmt.Errorf("fact extractor: decode json: %w", err)
	}

	out := make([]schema.ExtractedFact, 0, len(facts))
	for _, fact := range facts {
		fact.Key = strings.TrimSpace(fact.Key)
		fact.Value = strings.TrimSpace(fact.Value)
		fact.Category = strings.TrimSpace(fact.Category)
		fact.Scope = strings.TrimSpace(strings.ToLower(fact.Scope))
		if fact.Key == "" || fact.Value == "" || fact.Category == "" {
			continue
		}
		if fact.Scope == "" {
			fact.Scope = "owner"
		}
		out = append(out, fact)
	}
	return out, nil
}

var timeNowUTC = func() time.Time {
	return time.Now().UTC()
}

func trimJSONFence(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "```") {
		return raw
	}
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 {
		return raw
	}
	lines = lines[1:]
	if n := len(lines); n > 0 && strings.TrimSpace(lines[n-1]) == "```" {
		lines = lines[:n-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func isGreetingOrStatusCheck(content string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(content))
	if trimmed == "" {
		return false
	}
	if len(strings.Fields(trimmed)) <= 5 {
		for _, prefix := range []string{
			"hi", "hello", "hey", "yo",
			"how are you", "how's it going", "whats up", "what's up",
			"are you there", "status", "ping",
		} {
			if strings.HasPrefix(trimmed, prefix) {
				return true
			}
		}
	}
	return false
}

func wordCount(content string) int {
	count := 0
	inWord := false
	for _, r := range content {
		if unicode.IsSpace(r) {
			inWord = false
			continue
		}
		if !inWord {
			count++
			inWord = true
		}
	}
	return count
}

func nowUTC() time.Time {
	return timeNowUTC()
}
