package navi

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"unicode"

	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

type chatArtifactCandidate struct {
	Content       string
	Title         string
	ArtifactType  schema.ArtifactType
	Subtype       string
	ContentFormat string
	SourceMessage ChatMessage
}

func (l *AgentLoop) executeExplicitArtifactPromotion(ctx context.Context, run *naviruntime.RunState, thread *ChatThread, latestUserMessage string, experienceMode ExperienceMode) (*naviruntime.ExecuteResult, bool, error) {
	if !looksLikeExplicitArtifactPromotion(latestUserMessage) {
		return nil, false, nil
	}
	if run == nil {
		return nil, true, fmt.Errorf("navi: promote artifact: nil run")
	}
	if l.cfg.ArtifactService == nil {
		reply := "I can save that as an artifact once artifact storage is configured."
		final := l.shapeReply(reply, experienceMode)
		return &naviruntime.ExecuteResult{Run: run, Completed: true, FinalContent: final, ReplyLen: len(final), ExperienceMode: string(experienceMode)}, true, nil
	}
	candidate, ok := latestAssistantArtifactCandidate(thread)
	if !ok {
		reply := "I don't see a prior generated work product in this chat to save as an artifact. Please tell me what to save or paste the content."
		final := l.shapeReply(reply, experienceMode)
		return &naviruntime.ExecuteResult{Run: run, Completed: true, FinalContent: final, ReplyLen: len(final), ExperienceMode: string(experienceMode)}, true, nil
	}
	artifactID, title, err := l.createChatArtifact(ctx, run, thread, candidate, string(candidate.SourceMessage.ID), "Explicit promotion from chat follow-up request.")
	if err != nil {
		return nil, true, err
	}
	reply := fmt.Sprintf("Saved the previous reply as artifact `%s` (`%s`).", title, artifactID)
	final := l.shapeReply(reply, experienceMode)
	return &naviruntime.ExecuteResult{Run: run, Completed: true, FinalContent: final, ReplyLen: len(final), ExperienceMode: string(experienceMode)}, true, nil
}

func (l *AgentLoop) maybeAutoMaterializeChatReply(ctx context.Context, run *naviruntime.RunState, thread *ChatThread, content string) (string, error) {
	if l.cfg.ArtifactService == nil || run == nil {
		return "", nil
	}
	candidate, ok := artifactCandidateFromContent(content)
	if !ok || !candidateIsAutoMaterializable(candidate) {
		return "", nil
	}
	artifactID, title, err := l.createChatArtifact(ctx, run, thread, candidate, run.RunID, "Automatic materialization from substantial chat reply.")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Saved as artifact `%s` (`%s`).", title, artifactID), nil
}

func (l *AgentLoop) createChatArtifact(ctx context.Context, run *naviruntime.RunState, thread *ChatThread, candidate chatArtifactCandidate, sourceMessageID, reason string) (string, string, error) {
	workspaceID := ""
	ownerID := ""
	if thread != nil {
		if thread.Chat.WorkspaceID != nil {
			workspaceID = string(*thread.Chat.WorkspaceID)
		}
		ownerID = string(thread.Chat.OwnerID)
	}
	if workspaceID == "" && l.cfg.DB != nil {
		workspaceID, _ = store.GetWorkspaceID(ctx, l.cfg.DB)
	}
	if ownerID == "" && l.cfg.ResolveOwnerID != nil && run != nil {
		ownerID = l.cfg.ResolveOwnerID(ctx, run.ChatID)
	}
	if ownerID == "" && l.cfg.DB != nil {
		ownerID, _ = store.GetOwnerID(ctx, l.cfg.DB)
	}
	title := firstNonEmpty(candidate.Title, chatArtifactTitle(thread), "Chat Artifact")
	projectID := ""
	if thread != nil && thread.Chat.ProjectID != nil {
		projectID = string(*thread.Chat.ProjectID)
	}
	chatID := ""
	runID := ""
	if run != nil {
		chatID = run.ChatID
		runID = run.RunID
	}
	op := schema.ArtifactOperationEnvelope{
		Operation:            schema.ArtifactOpCreate,
		WorkspaceID:          workspaceID,
		ProjectID:            projectID,
		OwnerID:              ownerID,
		Title:                title,
		ArtifactType:         candidate.ArtifactType,
		ArtifactSubtype:      candidate.Subtype,
		ContentFormat:        candidate.ContentFormat,
		Payload:              candidate.Content,
		Reason:               reason,
		ActorType:            schema.ActorAgent,
		ActorID:              "navi",
		SourceConversationID: chatID,
		SourceMessageID:      firstNonEmpty(sourceMessageID, runID),
		Attributes: map[string]any{
			"origin": "chat",
			"run_id": runID,
		},
	}
	a, err := l.cfg.ArtifactService.CreateArtifact(ctx, op)
	if err != nil {
		return "", "", fmt.Errorf("navi: create chat artifact: %w", err)
	}
	attachArtifactToRun(run, a.ID)
	l.emitArtifactCreated(ctx, chatID, runID, &a)
	l.emitArtifactMaterialized(ctx, chatID, runID, a.ID, "chat.reply")
	return a.ID, firstNonEmpty(a.DisplayTitle, a.CanonicalTitle, title), nil
}

func attachArtifactToRun(run *naviruntime.RunState, artifactID string) {
	if run == nil || strings.TrimSpace(artifactID) == "" {
		return
	}
	artifactID = strings.TrimSpace(artifactID)
	if strings.TrimSpace(run.MainArtifactID) == "" {
		run.MainArtifactID = artifactID
	}
	for _, existing := range run.ArtifactIDs {
		if existing == artifactID {
			return
		}
	}
	run.ArtifactIDs = append(run.ArtifactIDs, artifactID)
}

func looksLikeExplicitArtifactPromotion(content string) bool {
	normalized := normalizeIntentText(content)
	if normalized == "" || !strings.Contains(normalized, "artifact") {
		return false
	}
	hasSaveVerb := containsAnyWord(normalized, "save", "saved", "promote", "promoted", "turn", "convert", "make", "create")
	hasReference := containsAnyWord(normalized, "it", "that", "this", "previous", "prior", "last", "above", "reply", "response", "message")
	return hasSaveVerb && hasReference
}

func latestAssistantArtifactCandidate(thread *ChatThread) (chatArtifactCandidate, bool) {
	if thread == nil {
		return chatArtifactCandidate{}, false
	}
	for i := len(thread.Messages) - 1; i >= 0; i-- {
		msg := thread.Messages[i]
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "assistant" && role != "navi" {
			continue
		}
		candidate, ok := artifactCandidateFromContent(msg.Content)
		if !ok {
			continue
		}
		candidate.SourceMessage = msg
		return candidate, true
	}
	return chatArtifactCandidate{}, false
}

func artifactCandidateFromContent(content string) (chatArtifactCandidate, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return chatArtifactCandidate{}, false
	}
	if block, ok := bestFencedCodeBlock(content); ok {
		return candidateForPayload(block.content, block.lang, content), true
	}
	if looksLikeStandaloneHTML(content) {
		return candidateForPayload(content, "html", content), true
	}
	if looksLikeMarkdownTable(content) {
		return chatArtifactCandidate{Content: content, Title: "Generated Table", ArtifactType: schema.ArtifactTypeDocument, Subtype: "table", ContentFormat: "text/markdown"}, true
	}
	if looksLikeMermaidDiagram(content) {
		return chatArtifactCandidate{Content: content, Title: "Generated Diagram", ArtifactType: schema.ArtifactTypeDocument, Subtype: "diagram", ContentFormat: "text/markdown"}, true
	}
	if len(content) > 2048 && looksLikeDurableMarkdown(content) {
		return chatArtifactCandidate{Content: content, Title: "Generated Document", ArtifactType: schema.ArtifactTypeDocument, Subtype: "markdown", ContentFormat: "text/markdown"}, true
	}
	return chatArtifactCandidate{}, false
}

func candidateIsAutoMaterializable(candidate chatArtifactCandidate) bool {
	content := strings.TrimSpace(candidate.Content)
	if content == "" {
		return false
	}
	if candidate.Subtype == "html" || candidate.Subtype == "table" || candidate.Subtype == "diagram" {
		return true
	}
	if candidate.ArtifactType == schema.ArtifactTypeCode && len(content) >= 240 {
		return true
	}
	if candidate.ArtifactType == schema.ArtifactTypeDocument && len(content) > 2048 {
		return true
	}
	return false
}

type fencedCodeBlock struct {
	lang    string
	content string
}

func bestFencedCodeBlock(content string) (fencedCodeBlock, bool) {
	lines := strings.Split(content, "\n")
	var best fencedCodeBlock
	bestScore := 0
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "```") {
			continue
		}
		lang := strings.TrimSpace(strings.TrimPrefix(line, "```"))
		var blockLines []string
		closed := false
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "```") {
				i = j
				closed = true
				break
			}
			blockLines = append(blockLines, lines[j])
		}
		if !closed {
			continue
		}
		body := strings.TrimSpace(strings.Join(blockLines, "\n"))
		if body == "" {
			continue
		}
		score := len(body)
		if langPriority(lang) > 0 {
			score += langPriority(lang) * 100000
		}
		if looksLikeStandaloneHTML(body) {
			score += 500000
			if strings.TrimSpace(lang) == "" {
				lang = "html"
			}
		}
		if score > bestScore {
			best = fencedCodeBlock{lang: lang, content: body}
			bestScore = score
		}
	}
	return best, bestScore > 0
}

func candidateForPayload(payload, lang, original string) chatArtifactCandidate {
	lang = normalizeCodeLanguage(lang)
	title := ""
	if lang == "html" {
		title = htmlTitle(payload)
	}
	if title == "" && strings.Contains(strings.ToLower(original), "html") {
		title = "Generated HTML"
	}
	switch lang {
	case "html":
		return chatArtifactCandidate{Content: payload, Title: firstNonEmpty(title, "Generated HTML"), ArtifactType: schema.ArtifactTypeCode, Subtype: "html", ContentFormat: "text/html"}
	case "markdown", "md":
		return chatArtifactCandidate{Content: payload, Title: "Generated Markdown", ArtifactType: schema.ArtifactTypeDocument, Subtype: "markdown", ContentFormat: "text/markdown"}
	case "json":
		return chatArtifactCandidate{Content: payload, Title: "Generated Data", ArtifactType: schema.ArtifactTypeData, Subtype: "json", ContentFormat: "application/json"}
	case "csv":
		return chatArtifactCandidate{Content: payload, Title: "Generated Data", ArtifactType: schema.ArtifactTypeData, Subtype: "csv", ContentFormat: "text/csv"}
	default:
		if lang == "" {
			lang = "code"
		}
		return chatArtifactCandidate{Content: payload, Title: "Generated Code", ArtifactType: schema.ArtifactTypeCode, Subtype: lang, ContentFormat: "text/plain"}
	}
}

func normalizeCodeLanguage(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(lang, " \t"); i >= 0 {
		lang = lang[:i]
	}
	switch lang {
	case "htm":
		return "html"
	case "md", "markdown":
		return "markdown"
	case "js", "javascript", "jsx", "ts", "typescript", "tsx", "css", "go", "python", "py", "yaml", "yml", "sql", "sh", "bash", "html", "json", "csv":
		return lang
	default:
		return lang
	}
}

func langPriority(lang string) int {
	switch normalizeCodeLanguage(lang) {
	case "html":
		return 6
	case "tsx", "jsx", "javascript", "js", "typescript", "ts", "css":
		return 5
	case "go", "python", "py", "sql", "bash", "sh", "yaml", "yml":
		return 4
	case "markdown", "md", "json", "csv":
		return 3
	default:
		return 1
	}
}

func looksLikeStandaloneHTML(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	return strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") || (strings.Contains(lower, "<canvas") && strings.Contains(lower, "<script"))
}

func looksLikeMarkdownTable(content string) bool {
	return strings.Contains(content, "| ---") || strings.Contains(content, "|---")
}

func looksLikeMermaidDiagram(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "sequencediagram") || strings.Contains(lower, "graph td") || strings.Contains(lower, "```mermaid")
}

func looksLikeDurableMarkdown(content string) bool {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "#") || strings.Contains(trimmed, "\n## ") {
		return true
	}
	return looksLikeMarkdownTable(content) || looksLikeMermaidDiagram(content)
}

func htmlTitle(content string) string {
	lower := strings.ToLower(content)
	start := strings.Index(lower, "<title>")
	end := strings.Index(lower, "</title>")
	if start < 0 || end <= start {
		return ""
	}
	value := strings.TrimSpace(content[start+len("<title>") : end])
	if len(value) > 80 {
		value = strings.TrimSpace(value[:80])
	}
	return html.UnescapeString(value)
}

func chatArtifactTitle(thread *ChatThread) string {
	if thread == nil {
		return ""
	}
	title := strings.TrimSpace(thread.Chat.Title)
	if title == "" || strings.EqualFold(title, "New Chat") {
		return ""
	}
	if len(title) > 80 {
		title = strings.TrimSpace(title[:80])
	}
	return title
}

func normalizeIntentText(content string) string {
	content = strings.ToLower(content)
	var b strings.Builder
	lastSpace := true
	for _, r := range content {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func containsAnyWord(content string, words ...string) bool {
	padded := " " + content + " "
	for _, word := range words {
		if strings.Contains(padded, " "+word+" ") {
			return true
		}
	}
	return false
}

func logAutoArtifactFailure(err error) {
	if err != nil {
		slog.Warn("navi: auto artifact materialization failed", "error", err)
	}
}
