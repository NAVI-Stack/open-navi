package context

import (
	stdcontext "context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/navi/orchestration"
)

type Config struct {
	MaxItems           int
	MaxChars           int
	MaxTokens          int
	RecentMessageLimit int
}

type RecentMessagesInput struct {
	Messages  []orchestration.ConversationTurn
	SourceRef string
}

type FactsInput struct {
	Block      string
	SourceRef  string
	Confidence float64
}

type RuntimeStateInput struct {
	Block     string
	SourceRef string
}

type ProposalInput struct {
	ProposalID     string
	Status         string
	ProposedAction string
	Summary        string
	SourceRef      string
	Confidence     float64
}

type CapabilitySnapshotInput struct {
	Surface         string
	ToolNames       []string
	SelectionReason string
	SourceRef       string
}

type AssembleInput struct {
	RecentMessages []RecentMessagesInput
	Summaries      []SummaryInput
	Facts          []FactsInput
	RuntimeStates  []RuntimeStateInput
	Proposals      []ProposalInput
	Capabilities   []CapabilitySnapshotInput
	Config         Config
}

type Resolver interface {
	Resolve(ctx stdcontext.Context, req orchestration.CanonicalRunRequest) (AssembleInput, error)
}

type ResolverFunc func(ctx stdcontext.Context, req orchestration.CanonicalRunRequest) (AssembleInput, error)

func (f ResolverFunc) Resolve(ctx stdcontext.Context, req orchestration.CanonicalRunRequest) (AssembleInput, error) {
	return f(ctx, req)
}

type Assembler struct {
	Resolver Resolver
	Config   Config
}

func NewAssembler(resolver Resolver, cfg Config) *Assembler {
	return &Assembler{Resolver: resolver, Config: cfg}
}

func (a *Assembler) Assemble(ctx stdcontext.Context, req orchestration.CanonicalRunRequest) (orchestration.ContextPack, error) {
	input := defaultAssembleInput(req, a.Config)
	if a != nil && a.Resolver != nil {
		resolved, err := a.Resolver.Resolve(ctx, req)
		if err != nil {
			return orchestration.ContextPack{}, fmt.Errorf("context resolver: %w", err)
		}
		input = mergeAssembleInput(input, resolved)
	}

	items := assembleItems(input)
	items = rankItems(items)
	included, budget, warnings := applyBudget(items, effectiveConfig(input.Config))

	return orchestration.ContextPack{
		Items:       included,
		Budget:      budget,
		Warnings:    warnings,
		AssembledAt: time.Now().UTC(),
	}, nil
}

func defaultAssembleInput(req orchestration.CanonicalRunRequest, cfg Config) AssembleInput {
	input := AssembleInput{
		Config: effectiveConfig(cfg),
	}
	if len(req.Conversation) > 0 {
		input.RecentMessages = []RecentMessagesInput{{
			Messages:  append([]orchestration.ConversationTurn(nil), req.Conversation...),
			SourceRef: req.Frame.ChatID,
		}}
	}
	if strings.TrimSpace(req.Frame.ResumeProposalID) != "" {
		summary := ""
		if reason := strings.TrimSpace(req.Frame.ResumeReason); reason != "" {
			summary = "resume_reason: " + reason
		}
		input.Proposals = []ProposalInput{{
			ProposalID: req.Frame.ResumeProposalID,
			Status:     "pending",
			Summary:    summary,
			SourceRef:  req.Frame.RunID,
		}}
	} else if reason := strings.TrimSpace(req.Frame.ResumeReason); reason != "" {
		input.RuntimeStates = append(input.RuntimeStates, RuntimeStateInput{
			Block:     "resume_reason: " + reason,
			SourceRef: req.Frame.RunID,
		})
	}
	if sanitized := sanitizeScratchpad(req.Frame.Scratchpad); len(sanitized) > 0 {
		input.RuntimeStates = []RuntimeStateInput{{
			Block:     formatScratchpadState(sanitized),
			SourceRef: req.Frame.RunID,
		}}
	}
	return input
}

func mergeAssembleInput(base, override AssembleInput) AssembleInput {
	base.Config = mergeConfig(base.Config, override.Config)
	base.RecentMessages = append(base.RecentMessages, override.RecentMessages...)
	base.Summaries = append(base.Summaries, override.Summaries...)
	base.Facts = append(base.Facts, override.Facts...)
	base.RuntimeStates = append(base.RuntimeStates, override.RuntimeStates...)
	base.Proposals = append(base.Proposals, override.Proposals...)
	base.Capabilities = append(base.Capabilities, override.Capabilities...)
	return base
}

func mergeConfig(base, override Config) Config {
	if override.MaxItems > 0 {
		base.MaxItems = override.MaxItems
	}
	if override.MaxChars > 0 {
		base.MaxChars = override.MaxChars
	}
	if override.MaxTokens > 0 {
		base.MaxTokens = override.MaxTokens
	}
	if override.RecentMessageLimit > 0 {
		base.RecentMessageLimit = override.RecentMessageLimit
	}
	return effectiveConfig(base)
}

func effectiveConfig(cfg Config) Config {
	if cfg.MaxItems <= 0 {
		cfg.MaxItems = 24
	}
	if cfg.MaxChars <= 0 {
		cfg.MaxChars = 12000
	}
	if cfg.MaxTokens < 0 {
		cfg.MaxTokens = 0
	}
	if cfg.RecentMessageLimit <= 0 {
		cfg.RecentMessageLimit = 12
	}
	return cfg
}

func assembleItems(input AssembleInput) []orchestration.ContextItem {
	cfg := effectiveConfig(input.Config)
	items := make([]orchestration.ContextItem, 0)
	for _, batch := range input.RecentMessages {
		items = append(items, assembleRecentMessages(batch, cfg.RecentMessageLimit)...)
	}
	for _, summary := range input.Summaries {
		if item, ok := summaryContextItem(summary); ok {
			items = append(items, item)
		}
	}
	for _, facts := range input.Facts {
		if item, ok := factsContextItem(facts); ok {
			items = append(items, item)
		}
	}
	for _, runtime := range input.RuntimeStates {
		if item, ok := runtimeStateContextItem(runtime); ok {
			items = append(items, item)
		}
	}
	for _, proposal := range input.Proposals {
		if item, ok := proposalContextItem(proposal); ok {
			items = append(items, item)
		}
	}
	for _, capability := range input.Capabilities {
		if item, ok := capabilityContextItem(capability); ok {
			items = append(items, item)
		}
	}
	return items
}

func assembleRecentMessages(input RecentMessagesInput, limit int) []orchestration.ContextItem {
	selected := input.Messages
	if len(selected) > limit {
		selected = selected[len(selected)-limit:]
	}
	items := make([]orchestration.ContextItem, 0, len(selected))
	for _, msg := range selected {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		trust := orchestration.TrustLabelDerived
		label := "recent assistant message"
		if strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
			trust = orchestration.TrustLabelUserSupplied
			label = "recent user message"
		}
		items = append(items, newContextItem(
			msg.ID,
			"conversation_turn",
			label,
			content,
			orchestration.ContextSourceHistory,
			sourceRefWithDefault(input.SourceRef, msg.ID),
			trust,
			orchestration.ContextClassConversation,
			1.0,
			map[string]string{
				"role":         msg.Role,
				"message_kind": msg.MessageKind,
				"created_at":   msg.CreatedAt.UTC().Format(time.RFC3339),
			},
		))
	}
	return items
}

func formatScratchpadState(scratchpad map[string]string) string {
	if len(scratchpad) == 0 {
		return ""
	}
	keys := make([]string, 0, len(scratchpad))
	for key, value := range scratchpad {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys)+1)
	lines = append(lines, "scratchpad:")
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s: %s", key, strings.TrimSpace(scratchpad[key])))
	}
	return strings.Join(lines, "\n")
}

func sanitizeScratchpad(scratchpad map[string]string) map[string]string {
	if len(scratchpad) == 0 {
		return nil
	}
	sanitized := make(map[string]string, len(scratchpad))
	for key, value := range scratchpad {
		key = strings.TrimSpace(key)
		if key == "" || strings.HasPrefix(key, "ics.") {
			continue
		}
		sanitized[key] = value
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

func sourceRefWithDefault(sourceRef, fallback string) string {
	if strings.TrimSpace(sourceRef) != "" {
		return strings.TrimSpace(sourceRef)
	}
	return strings.TrimSpace(fallback)
}
