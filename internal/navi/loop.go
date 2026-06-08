package navi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/artifact"
	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/command"
	cronsvc "github.com/ceoai/navi/internal/cron"
	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/experience"
	"github.com/ceoai/navi/internal/navi/inference"
	"github.com/ceoai/navi/internal/navi/orchestration"
	"github.com/ceoai/navi/internal/navi/selfmod"
	"github.com/ceoai/navi/internal/navi/skill"
	"github.com/ceoai/navi/internal/prompts"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
	navitool "github.com/ceoai/navi/internal/tool"
	"github.com/ceoai/navi/internal/worldmodel"
	"github.com/google/uuid"
)

// skillCommandType returns the schema.CommandType for the skill (from capability.command_type), default Invoke.
func skillCommandType(entry *skill.SkillEntry) schema.CommandType {
	if entry == nil || entry.Spec == nil || entry.Spec.Capability.CommandType == "" {
		return schema.CommandTypeInvoke
	}
	switch entry.Spec.Capability.CommandType {
	case "query":
		return schema.CommandTypeQuery
	case "create":
		return schema.CommandTypeCreate
	case "update":
		return schema.CommandTypeUpdate
	case "delete":
		return schema.CommandTypeDelete
	case "invoke":
		return schema.CommandTypeInvoke
	case "send":
		return schema.CommandTypeSend
	case "acquire":
		return schema.CommandTypeAcquire
	case "schedule":
		return schema.CommandTypeSchedule
	case "delegate":
		return schema.CommandTypeDelegate
	case "compose":
		return schema.CommandTypeCompose
	default:
		return schema.CommandTypeInvoke
	}
}

func toolResultString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

var ErrRuntimeCoordinatorUnavailable = errors.New("navi: runtime coordinator not configured")

// AgentLoop handles the conversational turns for NAVI.
type AgentLoop struct {
	cfg    LoopConfig
	wakeCh chan struct{}
}

type loopAwareInferenceController interface {
	SetLoop(*AgentLoop)
}

// LoopConfig defines the dependencies for the agent loop.
type LoopConfig struct {
	NAVI                *NAVI
	Orchestration       *orchestration.Pipeline
	DB                  *sql.DB
	ExperienceManager   *ExperienceManager
	Chats               ChatStore
	RuntimeSessions     RuntimeSessionStore
	RuntimeStore        naviruntime.Store
	RuntimeWake         func(runtimeSessionID string)
	Skills              *skill.SkillRegistry
	Policy              *skill.PolicyEngine
	LLM                 llm.Provider
	Bus                 bus.Bus
	Model               string // Option to override default model
	LLMCallTimeout      time.Duration
	Summarizer          ChatSummarizer
	SummaryModel        string
	WorkspaceDir        string                               // workspace root for file tools (empty disables file tools)
	Governor            interface{ CheckPath(string) error } // optional; file tools call CheckPath when set
	WorldModel          *worldmodel.WorldModel
	GapDetector         *GapDetector
	Experience          experience.Layer // applies behavioral policy to reply before delivery; nil = passthrough
	InferenceController inference.DecisionController

	// Context window governance (0 = defaults in context_governance).
	ContextRecentMessages int
	ContextPinnedMessages int
	EnableRunScratchpad   bool

	// Optional LLM routing/introspection callbacks.
	ListLLMs     func(ctx context.Context) (any, error)
	GetActiveLLM func(ctx context.Context) (provider, model string, err error)
	SetActiveLLM func(ctx context.Context, provider, model string) (providerOut, modelOut string, err error)
	// LLMService backs hidden llm-router_* tools; nil leaves those tools returning "not configured".
	LLMService navitool.RouterLLMService

	// ToolRegistry is the first-class runtime tool catalog; lazily built when nil.
	ToolRegistry *navitool.Registry

	// OnStartConnector is called when the LLM requests to activate a connector.
	OnStartConnector func(ctx context.Context, connType string, params map[string]string) error
	// OnSetTimezone is called when the onboarding_set_timezone tool runs.
	OnSetTimezone func(ctx context.Context, tz string) error
	// FactsBlock optionally returns a memory/facts block for the given chat to inject into the NCOS context pack.
	FactsBlock func(ctx context.Context, chatID string) (string, error)

	// ContextRetriever optionally supplies provenance-bearing, distilled
	// background context for the Contextualize step (CIP P4 retrieval surface).
	// When set, the loop calls it per turn with the current user message and
	// injects the returned block into the NCOS context pack. Nil — or an empty
	// return — preserves turn-time behavior exactly. Implementations must be
	// read-only (no World Model writes).
	ContextRetriever ContextRetriever
	// ContextRetrievalBudgetTokens caps the retrieved-context distillation budget
	// per turn. 0 falls back to defaultContextRetrievalBudgetTokens.
	ContextRetrievalBudgetTokens int

	// SaveProposal persists a proposal when validation requires confirmation (Validate/Govern).
	// When set, the loop creates a proposal record and includes its ID in the action-requested payload and reply.
	SaveProposal                     func(ctx context.Context, p schema.Proposal) error
	GetProposal                      func(ctx context.Context, proposalID string) (schema.Proposal, error)
	FindPendingProposalByBoundaryKey func(ctx context.Context, boundaryKey string) (schema.Proposal, error)
	ICSStateStore                    ICSStateStore

	// SkipHallucinationRetries disables reflecting governance rejections back to the LLM (useful for mock tests asserting block behavior).
	SkipHallucinationRetries bool

	// SaveExecutionOutcome records a command attempt (for failure model and observability).
	SaveExecutionOutcome func(ctx context.Context, eo schema.ExecutionOutcome) error

	// AutonomyResolver optionally applies autonomy when validation returns RequiresConfirmation
	// (no hard floor + high preset for domain => approve). Implemented by config.AutonomyConfig.
	AutonomyResolver governor.AutonomyPresetResolver
	// DomainForSkill returns the domain for a skill (e.g. "coding", "memory") for autonomy overrides.
	DomainForSkill func(skillName string) string

	// GovConfigPriorityReader supplies owner config/priorities for Configuration and Priority Alignment checks.
	// When set, the loop uses ValidateActionWithOwner so owner-scoped configuration and priorities influence outcome.
	GovConfigPriorityReader governor.ConfigPriorityReader
	// ResolveOwnerID returns the owner ID for the given chat; when set, action.OwnerID is set before validation.
	ResolveOwnerID func(ctx context.Context, chatID string) string
	// ResolveToolAuthority returns the discovery/execution authority for the
	// current chat context. Nil means derive a conservative default.
	ResolveToolAuthority func(ctx context.Context, chatID string) navitool.ToolAuthority

	// ConnectorHealth optionally returns live connector status for introspection block injection.
	// When nil, connector info is omitted from the runtime state block.
	ConnectorHealth func() []ConnectorHealthInfo

	// Tool registries.
	PluginRegistry  PluginRegistryInterface
	SelfModExecutor *selfmod.Executor

	// ArtifactService persists skill outputs and workflow artifacts; nil disables materialization.
	ArtifactService *artifact.Service

	// Debug when true surfaces internal errors (e.g. LLM failures) in the reply content.
	Debug bool

	// StatusTracker receives coarse state transitions during turn processing; nil is a no-op.
	StatusTracker *StatusTracker

	// Cron is optionally used to durably persist send_reply messages whose
	// requested delay exceeds the in-process scheduler cap. When nil, the
	// runtime keeps the original behavior (reject with redirect error).
	Cron *cronsvc.Service

	// Prompts is the prompt template manager for loading overridable rules from disk.
	Prompts *prompts.Manager
}

// NewAgentLoop initializes a new AgentLoop.
func NewAgentLoop(cfg LoopConfig) *AgentLoop {
	if cfg.ExperienceManager == nil {
		cfg.ExperienceManager = NewExperienceManager("", ExperienceModeStandard)
	}
	if cfg.ICSStateStore == nil {
		if store, ok := cfg.RuntimeStore.(ICSStateStore); ok {
			cfg.ICSStateStore = store
		}
	}
	// Governance provider is always injected. StoreGovernanceBoundsProvider
	// gracefully falls back to contextual bounds when db is nil, so there is
	// no reason to conditionally skip this — doing so creates split-brain risk
	// when the manager is reused across contexts with different DB availability.
	cfg.ExperienceManager.SetGovernanceBoundsProvider(experience.NewStoreGovernanceBoundsProvider(cfg.DB))
	loop := &AgentLoop{
		cfg:    cfg,
		wakeCh: make(chan struct{}, 1),
	}
	if aware, ok := loop.cfg.InferenceController.(loopAwareInferenceController); ok {
		aware.SetLoop(loop)
	}
	if loop.cfg.Orchestration == nil {
		loop.cfg.Orchestration = newRuntimeOrchestrationPipeline(loop)
	}
	return loop
}

func (l *AgentLoop) eventVisibility(ctx context.Context, chatID, runtimeSessionID string) schema.EventVisibility {
	if l.chatHiddenFromUserStream(ctx, chatID) {
		return schema.VisibilityOperator
	}
	type runtimeSessionKindLookup interface {
		LookupRuntimeSessionKind(ctx context.Context, runtimeSessionID string) (schema.RuntimeSessionKind, error)
	}
	if lookup, ok := l.cfg.RuntimeStore.(runtimeSessionKindLookup); ok {
		kind, err := lookup.LookupRuntimeSessionKind(ctx, runtimeSessionID)
		if err == nil {
			if kind == schema.RuntimeSessionKindInternal || runtimeSessionID == schema.HeartbeatAutoRuntimeSessionID {
				return schema.VisibilityOperator
			}
			return schema.VisibilityUser
		}
	}
	kind := schema.DefaultRuntimeSessionKindForID(runtimeSessionID)
	if kind == schema.RuntimeSessionKindInternal || runtimeSessionID == schema.HeartbeatAutoRuntimeSessionID {
		return schema.VisibilityOperator
	}
	return schema.VisibilityUser
}

func (l *AgentLoop) chatHiddenFromUserStream(ctx context.Context, chatID string) bool {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return false
	}
	if schema.IsInternalRuntimeSessionID(chatID) {
		return true
	}
	if l.cfg.Chats == nil {
		return false
	}
	chat, err := l.cfg.Chats.GetChat(ctx, chatID)
	if err != nil || chat == nil {
		return false
	}
	return chat.Status == ChatStatusDeleted || chat.DeletedAt != nil
}

// Run starts the agent loop, blocking until the context is canceled.
func (l *AgentLoop) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-l.wakeCh:
			slog.Debug("navi: loop wake signal received")
			if err := l.ProcessTurn(ctx); err != nil {
				slog.Warn("navi: loop turn failed", "error", err)
			}
		}
	}
}

// Wake signals the loop to perform an immediate turn.
func (l *AgentLoop) Wake() {
	select {
	case l.wakeCh <- struct{}{}:
	default:
		// If wakeCh is full, it's already triggered.
	}
}

// ProcessTurn is a thin delegation shell over the authoritative runtime path.
// Conversational execution must be driven by the runtime coordinator.
func (l *AgentLoop) ProcessTurn(ctx context.Context) error {
	chatID, err := l.cfg.Chats.ActiveChatID(ctx, ActiveChatScope{})
	if err != nil {
		return fmt.Errorf("read active chat: %w", err)
	}
	if strings.TrimSpace(chatID) == "" {
		return nil
	}
	if l.shouldDelegateTurnToRuntime() {
		runtimeSessionID, err := l.activeRuntimeSessionIDForChat(ctx, chatID)
		if err != nil {
			return err
		}
		l.cfg.RuntimeWake(runtimeSessionID)
		return nil
	}

	return fmt.Errorf("%w: ProcessTurn requires RuntimeStore, RuntimeSessions, RuntimeWake, and ICSStateStore for conversational execution", ErrRuntimeCoordinatorUnavailable)
}

func (l *AgentLoop) activeRuntimeSessionIDForChat(ctx context.Context, chatID string) (string, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return "", nil
	}
	if l == nil || l.cfg.RuntimeSessions == nil {
		return "", fmt.Errorf("%w: runtime session store not configured", ErrRuntimeCoordinatorUnavailable)
	}
	runtimeSession, err := l.cfg.RuntimeSessions.FindActiveRuntimeSessionForChat(ctx, chatID, "")
	if err != nil {
		return "", fmt.Errorf("resolve active runtime session for chat %s: %w", chatID, err)
	}
	if runtimeSession == nil || strings.TrimSpace(string(runtimeSession.ID)) == "" {
		return "", fmt.Errorf("%w: no active runtime session for chat %s", ErrRuntimeCoordinatorUnavailable, chatID)
	}
	return strings.TrimSpace(string(runtimeSession.ID)), nil
}

func (l *AgentLoop) recordInteractionEventAsCreate(ctx context.Context, runtimeSessionID string, metadata map[string]any) {
	doRecord := func(ctx context.Context) (any, error) {
		return nil, l.cfg.WorldModel.RecordInteractionEvent(ctx, "", runtimeSessionID, metadata)
	}
	if l.cfg.SaveExecutionOutcome != nil {
		exec := command.NewExecutor(l.cfg.SaveExecutionOutcome)
		_, _ = exec.Execute(ctx, command.Descriptor{Type: schema.CommandTypeCreate, RuntimeSessionID: runtimeSessionID}, doRecord)
		return
	}
	_, _ = doRecord(ctx)
}

func isManualEscalationHint(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	return strings.Contains(lower, "remember this") ||
		strings.Contains(lower, "this is important") ||
		strings.Contains(lower, "don't forget") ||
		strings.Contains(lower, "keep this in mind") ||
		strings.Contains(lower, "process this deeply")
}

// emitReflect runs the Reflect step: emits a ReflectionPayload for the Subconscious to consume.
// Called after each turn completes (reply persisted). lastUserContent is used for manual escalation
// (e.g. "remember this" → tier Deep, escalation_reason "manual").
func (l *AgentLoop) emitReflect(ctx context.Context, runtimeSessionID, summary, details, lastUserContent string) {
	if l.cfg.Bus == nil {
		return
	}
	tier := schema.ReflectionTierShallow
	escalationReason := ""
	if isManualEscalationHint(lastUserContent) {
		tier = schema.ReflectionTierDeep
		escalationReason = "manual"
	}
	payload := schema.ReflectionPayload{
		ID:               uuid.New().String(),
		RuntimeSessionID: runtimeSessionID,
		Tier:             tier,
		Summary:          summary,
		Details:          details,
		EscalationReason: escalationReason,
		CreatedAt:        time.Now().UTC(),
	}
	ev := schema.NewEvent(
		schema.FactReflectionQueued,
		schema.EventKindFact,
		runtimeSessionID,
		schema.AgentNavi,
		payload,
	)
	if err := l.cfg.Bus.Publish(ctx, ev); err != nil {
		slog.Debug("navi: failed to publish reflection", "error", err)
	}
}

func (l *AgentLoop) shapeReply(raw string, experienceMode ExperienceMode) string {
	raw = sanitizeRuntimeUserFacingReply(raw)
	if l.cfg.Experience != nil {
		return l.cfg.Experience.ShapeReply(raw, string(experienceMode))
	}
	if strings.TrimSpace(raw) == "" {
		return "I didn't generate a reply. Please try again or rephrase."
	}
	if isRefusalToChatInLoop(raw) {
		return "Hi! How can I help you today?"
	}
	return raw
}

func isRefusalToChatInLoop(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	return (strings.Contains(lower, "not designed for direct messaging") ||
		(strings.Contains(lower, "no response") && strings.Contains(lower, "waiting for explicit")))
}

const (
	routerToolList      = "navi.llm.router.list"
	routerToolGetActive = "navi.llm.router.get_active"
	routerToolSetActive = "navi.llm.router.set_active"
)

func isRouterStateTool(name string) bool {
	switch name {
	case routerToolList, routerToolGetActive, routerToolSetActive, "llm-router_list", "llm-router_get_active", "llm-router_set_active":
		return true
	default:
		return false
	}
}

func isRouterToolError(content string) bool {
	return strings.HasPrefix(content, "Error:") ||
		strings.Contains(content, "CRITICAL:") ||
		strings.HasPrefix(content, "Note:") ||
		strings.Contains(content, "Operator approval") ||
		strings.HasPrefix(content, "Recovery needed:")
}

// formatRouterStructuredReply builds a single user-facing reply from llm-router tool results.
// Used when all tool calls in a turn were router state tools so the reply is grounded in tool output only.
func formatRouterStructuredReply(results []struct{ Name, Content string }) string {
	var parts []string
	for _, r := range results {
		if isRouterToolError(r.Content) {
			parts = append(parts, "Could not retrieve provider/model state: "+r.Content)
			continue
		}
		switch r.Name {
		case routerToolList:
			var catalog llm.LLMCatalog
			if err := json.Unmarshal([]byte(r.Content), &catalog); err != nil {
				parts = append(parts, "Could not parse provider list: "+r.Content)
				continue
			}
			parts = append(parts, formatCatalogForReply(catalog))
		case routerToolGetActive:
			var out struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
			}
			if err := json.Unmarshal([]byte(r.Content), &out); err != nil {
				parts = append(parts, "Could not parse active provider: "+r.Content)
				continue
			}
			parts = append(parts, fmt.Sprintf("Active provider: %s. Active model: %s.", out.Provider, out.Model))
		case routerToolSetActive:
			var out struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
			}
			if err := json.Unmarshal([]byte(r.Content), &out); err != nil {
				parts = append(parts, "Could not parse switch result: "+r.Content)
				continue
			}
			parts = append(parts, fmt.Sprintf("Switched to provider %s, model %s.", out.Provider, out.Model))
		default:
			parts = append(parts, r.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

func formatCatalogForReply(catalog llm.LLMCatalog) string {
	if len(catalog.Providers) == 0 {
		return "No providers configured."
	}
	var b strings.Builder
	b.WriteString("Available providers and models:\n\n")
	for _, p := range catalog.Providers {
		b.WriteString("**" + p.DisplayName + "**\n")
		for _, m := range p.Models {
			b.WriteString("- " + m.Name + "\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n\n")
}

func (l *AgentLoop) handleFinalReply(ctx context.Context, runtimeSessionID string, experienceMode ExperienceMode, content string) error {
	slog.Debug("navi: finalizing reply", "chat_id", runtimeSessionID, "content_len", len(content))

	// 5. Persist Reply
	msg := ChatMessage{
		Role:      "assistant",
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}

	msgID, err := l.cfg.Chats.AppendChatMessage(ctx, runtimeSessionID, msg)
	if err != nil {
		return fmt.Errorf("append reply: %w", err)
	}

	// 6. Bus Publish
	ev := schema.NewEvent(
		schema.EventType(EvtNaviReplied),
		schema.EventKindFact,
		runtimeSessionID,
		schema.AgentType("navi"),
		NaviRepliedPayload{
			ChatID:         runtimeSessionID,
			MessageID:      msgID,
			ExperienceMode: experienceMode,
		},
	)
	if l.cfg.Bus != nil {
		if err := l.cfg.Bus.Publish(ctx, ev); err != nil {
			slog.Warn("navi: loop failed to publish reply event", "error", err)
		}
	}

	return nil
}

func (l *AgentLoop) shouldDelegateTurnToRuntime() bool {
	return l.cfg.RuntimeStore != nil && l.cfg.RuntimeSessions != nil && l.cfg.RuntimeWake != nil && l.cfg.ICSStateStore != nil
}
