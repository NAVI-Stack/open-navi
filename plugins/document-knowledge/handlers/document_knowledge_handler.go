package handlers

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/open-navi/navi/internal/llm"
	coreskill "github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

type SkillEntry = coreskill.SkillEntry
type Skill = coreskill.Skill
type OSS27Spec = coreskill.OSS27Spec
type Interface = coreskill.Interface
type ExecutionContext = coreskill.ExecutionContext
type SkillExecutionResult = coreskill.SkillExecutionResult

const SkillResultSuccess = coreskill.SkillResultSuccess

var (
	Execute                     = coreskill.Execute
	ExecutionContextFromContext = coreskill.ExecutionContextFromContext
	RegisterInternalHandler     = coreskill.RegisterInternalHandler
	WithExecutionContext        = coreskill.WithExecutionContext
)

type documentPathChecker interface {
	CheckPath(targetPath string) error
}

type DocumentKnowledgeHandlerConfig struct {
	DB             *sql.DB
	LLM            llm.Provider
	Model          string
	WorkspaceDir   string
	PathChecker    documentPathChecker
	ResolveOwnerID func(ctx context.Context, chatID string) string
}

type DocumentExtraction struct {
	Path         string           `json:"path"`
	DocumentType string           `json:"document_type"`
	PageCount    int              `json:"page_count,omitempty"`
	Text         string           `json:"text"`
	TextPreview  string           `json:"text_preview,omitempty"`
	Entities     []DocumentEntity `json:"entities,omitempty"`
}

type DocumentEntity struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

type DocumentSummary struct {
	Summary string                 `json:"summary"`
	Facts   []schema.ExtractedFact `json:"facts,omitempty"`
}

var documentExtractor = executeDocumentExtractor

func RegisterDocumentKnowledgeHandlers(skillID string, cfg DocumentKnowledgeHandlerConfig) {
	if cfg.DB == nil || cfg.LLM == nil || strings.TrimSpace(cfg.Model) == "" {
		return
	}
	if strings.TrimSpace(skillID) == "" {
		skillID = "document-knowledge"
	}

	RegisterInternalHandler(skillID, "summarize_document", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		path, err := resolveDocumentPath(cfg.WorkspaceDir, stringArg(args, "path"), cfg.PathChecker)
		if err != nil {
			return nil, err
		}
		extracted, err := documentExtractor(ctx, entry, path)
		if err != nil {
			return nil, err
		}
		summary, err := summarizeDocument(ctx, cfg.LLM, cfg.Model, extracted)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"path":          extracted.Path,
			"document_type": extracted.DocumentType,
			"page_count":    extracted.PageCount,
			"summary":       summary.Summary,
			"facts":         summary.Facts,
			"entities":      extracted.Entities,
			"text_preview":  extracted.TextPreview,
		}, nil
	})

	RegisterInternalHandler(skillID, "ingest_document", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		path, err := resolveDocumentPath(cfg.WorkspaceDir, stringArg(args, "path"), cfg.PathChecker)
		if err != nil {
			return nil, err
		}
		extracted, err := documentExtractor(ctx, entry, path)
		if err != nil {
			return nil, err
		}
		summary, err := summarizeDocument(ctx, cfg.LLM, cfg.Model, extracted)
		if err != nil {
			return nil, err
		}
		chatID, ownerID := resolveDocumentScopes(ctx, args, cfg.ResolveOwnerID)
		if strings.TrimSpace(chatID) == "" && strings.TrimSpace(ownerID) == "" {
			return nil, fmt.Errorf("ingest_document requires chat context or explicit chat_id/owner_id")
		}
		storedFacts, memoryID, err := persistDocumentKnowledge(ctx, cfg.DB, extracted, summary, chatID, ownerID)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"path":             extracted.Path,
			"document_type":    extracted.DocumentType,
			"page_count":       extracted.PageCount,
			"summary":          summary.Summary,
			"facts":            summary.Facts,
			"stored_fact_ids":  storedFacts,
			"memory_id":        memoryID,
			"entities":         extracted.Entities,
			"text_preview":     extracted.TextPreview,
			"knowledge_scopes": buildDocumentScopes(chatID, ownerID),
		}, nil
	})
}

func executeDocumentExtractor(ctx context.Context, entry *SkillEntry, resolvedPath string) (DocumentExtraction, error) {
	if entry == nil || entry.Spec == nil {
		return DocumentExtraction{}, fmt.Errorf("document knowledge skill entry not loaded")
	}
	iface := documentExtractionInterface(entry)
	if iface == nil {
		return DocumentExtraction{}, fmt.Errorf("document knowledge extract_entities interface not found")
	}
	raw, err := Execute(ctx, entry, iface, map[string]any{"path": resolvedPath})
	if err != nil {
		return DocumentExtraction{}, err
	}
	var execResult SkillExecutionResult
	if err := json.Unmarshal([]byte(raw), &execResult); err != nil {
		return DocumentExtraction{}, fmt.Errorf("decode document extractor result: %w", err)
	}
	if execResult.Status != string(SkillResultSuccess) && execResult.Status != "success" {
		if execResult.Error != nil && strings.TrimSpace(execResult.Error.Message) != "" {
			return DocumentExtraction{}, fmt.Errorf("%s", execResult.Error.Message)
		}
		return DocumentExtraction{}, fmt.Errorf("document extractor returned status %s", execResult.Status)
	}
	var extracted DocumentExtraction
	payload, err := json.Marshal(execResult.Payload)
	if err != nil {
		return DocumentExtraction{}, fmt.Errorf("marshal document extractor payload: %w", err)
	}
	if err := json.Unmarshal(payload, &extracted); err != nil {
		return DocumentExtraction{}, fmt.Errorf("decode document extractor payload: %w", err)
	}
	if strings.TrimSpace(extracted.Path) == "" {
		extracted.Path = resolvedPath
	}
	if strings.TrimSpace(extracted.TextPreview) == "" {
		extracted.TextPreview = summarizePreview(extracted.Text, 1200)
	}
	return extracted, nil
}

func documentExtractionInterface(entry *SkillEntry) *Interface {
	if entry == nil || entry.Spec == nil {
		return nil
	}
	for i := range entry.Spec.Interfaces {
		if entry.Spec.Interfaces[i].Name == "extract_entities" {
			return &entry.Spec.Interfaces[i]
		}
	}
	return nil
}

func summarizeDocument(ctx context.Context, provider llm.Provider, model string, extracted DocumentExtraction) (DocumentSummary, error) {
	if provider == nil || strings.TrimSpace(model) == "" {
		return DocumentSummary{}, fmt.Errorf("document summarizer is not configured")
	}
	text := strings.TrimSpace(extracted.Text)
	if text == "" {
		return DocumentSummary{}, fmt.Errorf("document contains no extractable text")
	}
	if len(text) > 12000 {
		text = text[:12000]
	}
	resp, err := provider.Chat(ctx, model, []llm.Message{
		{
			Role: "system",
			Content: "You summarize documents into durable knowledge. Return strict JSON with keys " +
				"`summary` and `facts`. `facts` must be an array of objects with `key`, `value`, and `category`. " +
				"Use category `technical_context` unless a better fit like `project_decision` is obvious. Keep facts concrete and non-duplicative.",
		},
		{
			Role: "user",
			Content: fmt.Sprintf("Path: %s\nDocument type: %s\nPage count: %d\nEntities: %s\n\nDocument text:\n%s",
				extracted.Path, extracted.DocumentType, extracted.PageCount, joinDocumentEntities(extracted.Entities), text),
		},
	}, nil, llm.Options{Temperature: 0.1, MaxTokens: 900})
	if err != nil {
		return DocumentSummary{}, err
	}
	return decodeDocumentSummary(resp.Content)
}

func decodeDocumentSummary(raw string) (DocumentSummary, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var out DocumentSummary
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return DocumentSummary{}, fmt.Errorf("decode document summary: %w", err)
	}
	out.Summary = strings.TrimSpace(out.Summary)
	return out, nil
}

func resolveDocumentPath(workspaceDir, rawPath string, checker documentPathChecker) (string, error) {
	workspaceDir = strings.TrimSpace(workspaceDir)
	rawPath = strings.TrimSpace(rawPath)
	if workspaceDir == "" {
		return "", fmt.Errorf("workspace directory is not configured")
	}
	if rawPath == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(rawPath) {
		return "", fmt.Errorf("path must be relative to the workspace")
	}
	cleaned := filepath.Clean(filepath.FromSlash(rawPath))
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("path is required")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path access denied: workspace escape blocked")
	}
	if checker != nil {
		if err := checker.CheckPath(cleaned); err != nil {
			return "", err
		}
	}
	absWorkspace, err := filepath.Abs(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	target := filepath.Join(absWorkspace, cleaned)
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	rel, err := filepath.Rel(absWorkspace, absTarget)
	if err != nil {
		return "", fmt.Errorf("resolve workspace relation: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path access denied: workspace escape blocked")
	}
	return absTarget, nil
}

func resolveDocumentScopes(ctx context.Context, args map[string]any, resolveOwnerID func(context.Context, string) string) (string, string) {
	chatID := stringArg(args, "chat_id")
	ownerID := stringArg(args, "owner_id")
	if exec, ok := ExecutionContextFromContext(ctx); ok {
		if chatID == "" {
			chatID = strings.TrimSpace(exec.ChatID)
		}
	}
	if ownerID == "" && resolveOwnerID != nil && chatID != "" {
		ownerID = strings.TrimSpace(resolveOwnerID(ctx, chatID))
	}
	return chatID, ownerID
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func persistDocumentKnowledge(ctx context.Context, db *sql.DB, extracted DocumentExtraction, summary DocumentSummary, chatID, ownerID string) ([]string, string, error) {
	if db == nil {
		return nil, "", fmt.Errorf("document knowledge store is not configured")
	}
	if strings.TrimSpace(summary.Summary) == "" {
		return nil, "", fmt.Errorf("document summary is empty")
	}
	baseName := filepath.Base(extracted.Path)
	scope := "chat"
	scopeID := chatID
	if ownerID != "" {
		scope = "owner"
		scopeID = ownerID
	}
	if scopeID == "" {
		return nil, "", fmt.Errorf("missing scope for document knowledge persistence")
	}

	if len(summary.Facts) == 0 {
		summary.Facts = []schema.ExtractedFact{{
			Key:      fmt.Sprintf("document_%s_summary", compactDocumentKey(baseName)),
			Value:    summary.Summary,
			Category: "technical_context",
		}}
	}

	storedIDs := make([]string, 0, len(summary.Facts))
	for _, fact := range summary.Facts {
		if strings.TrimSpace(fact.Key) == "" || strings.TrimSpace(fact.Value) == "" {
			continue
		}
		factScope := scope
		factScopeID := scopeID
		if strings.TrimSpace(fact.Scope) == "chat" && chatID != "" {
			factScope = "chat"
			factScopeID = chatID
		}
		if strings.TrimSpace(fact.Scope) == "owner" && ownerID != "" {
			factScope = "owner"
			factScopeID = ownerID
		}
		id := documentHashID("fact", factScope, factScopeID, extracted.Path, fact.Key)
		if err := store.SaveFact(ctx, db, store.Fact{
			ID:       id,
			Scope:    factScope,
			ScopeID:  factScopeID,
			Category: defaultDocumentFactCategory(fact.Category),
			Key:      fact.Key,
			Value:    fact.Value,
			Keywords: []string{baseName, extracted.DocumentType},
			Tags:     documentEntityTags(extracted.Entities),
			Source:   "document_ingestion",
		}); err != nil {
			return nil, "", err
		}
		storedIDs = append(storedIDs, id)
	}

	memoryID := ""
	if chatID != "" {
		mem := schema.Memory{
			ID:           documentHashID("memory", "chat", chatID, extracted.Path, "summary"),
			Scope:        "chat",
			ScopeID:      chatID,
			Summary:      summary.Summary,
			Details:      extracted.TextPreview,
			Keywords:     []string{baseName, extracted.DocumentType},
			Tags:         documentEntityTags(extracted.Entities),
			Significance: "medium",
			Source:       "document_ingestion",
		}
		saved, err := store.SaveMemory(ctx, db, mem)
		if err != nil {
			return nil, "", err
		}
		memoryID = saved.ID
	}

	return storedIDs, memoryID, nil
}

func buildDocumentScopes(chatID, ownerID string) []string {
	var scopes []string
	if chatID != "" {
		scopes = append(scopes, "chat:"+chatID)
	}
	if ownerID != "" {
		scopes = append(scopes, "owner:"+ownerID)
	}
	return scopes
}

func summarizePreview(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return strings.TrimSpace(text[:limit]) + "..."
}

func joinDocumentEntities(entities []DocumentEntity) string {
	if len(entities) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(entities))
	for _, entity := range entities {
		label := strings.TrimSpace(entity.Text)
		if label == "" {
			continue
		}
		if kind := strings.TrimSpace(entity.Type); kind != "" {
			label = label + " (" + kind + ")"
		}
		parts = append(parts, label)
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func documentEntityTags(entities []DocumentEntity) []string {
	seen := make(map[string]struct{})
	tags := make([]string, 0, len(entities))
	for _, entity := range entities {
		kind := strings.TrimSpace(entity.Type)
		if kind == "" {
			continue
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		tags = append(tags, kind)
	}
	return tags
}

func defaultDocumentFactCategory(category string) string {
	category = strings.TrimSpace(category)
	if category == "" {
		return "technical_context"
	}
	return category
}

func compactDocumentKey(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.ReplaceAll(v, " ", "_")
	v = strings.ReplaceAll(v, "-", "_")
	if len(v) > 24 {
		v = v[:24]
	}
	return v
}

func documentHashID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:16])
}
