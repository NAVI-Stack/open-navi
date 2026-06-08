package navi

import (
	"context"
	"database/sql"
	"time"

	"github.com/open-navi/navi/internal/bus"
	cronsvc "github.com/open-navi/navi/internal/cron"
	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/plugin"
	"github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/prompts"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/worldmodel"
)

// AutonomyPresetResolver is aliased so config can stay free of governor import when not using autonomy.
type AutonomyPresetResolver = governor.AutonomyPresetResolver

// Config defines the configuration for a NAVI instance.
type Config struct {
	LLM                   llm.Provider
	Bus                   bus.Bus
	Chats                 ChatStore
	RuntimeSessions       RuntimeSessionStore
	ConversationEndpoints ConversationEndpointStore
	ConnectorDispatcher   ConnectorDispatcher
	DB                    *sql.DB      // optional; used for gap detection
	GapDetector           *GapDetector // optional; if nil, will be initialized if DB is set
	WorldModel            *worldmodel.WorldModel
	SkillsDir             string             // path to skills directory on disk
	HubIndexURL           string             // URL of the remote skill registry index (optional)
	BraveSearchKey        string             // API key for Brave Search
	ExperienceProfilesDir string             // path to experience profile directory on disk
	WorkspaceDir          string             // workspace root for file tools (ReadFile, ListDir, WriteFile)
	ArtifactsDir          string             // root directory for artifact blob storage (default: artifacts)
	Governor              *governor.Governor // optional; when set, file tools call CheckPath before I/O
	InitialExperienceMode ExperienceMode
	Model                 string         // Default model to use for chat
	LLMCallTimeout        time.Duration  // Foreground LLM deadline; zero falls back to the default runtime timeout
	Summarizer            ChatSummarizer // optional; compacts chat history during/after runs

	// Cron runs persisted cron_jobs (internal/cron). When nil with DB set,
	// core-scheduler internal handlers may be omitted depending on bootstrap wiring.
	Cron *cronsvc.Service

	// LLM introspection and control hooks. When set, these allow skills to
	// list available providers/models, report the active selection, and
	// update the active provider/model at runtime.
	ListLLMs     func(ctx context.Context) (any, error)
	GetActiveLLM func(ctx context.Context) (provider, model string, err error)
	SetActiveLLM func(ctx context.Context, provider, model string) (providerOut, modelOut string, err error)
	RouteLLM     func(ctx context.Context, req llm.RouteRequest) (llm.RouteDecision, error)

	// IsSetupDone is called to check whether the initial setup wizard has been
	// completed. If nil, falls back to reading the session meta "setup_done" key.
	IsSetupDone func(ctx context.Context) bool

	// OnSetTimezone is called when the onboarding_set_timezone tool runs.
	OnSetTimezone func(ctx context.Context, tz string) error

	// OnStartConnector is called when the LLM requests to activate a connector
	// (e.g. telegram_setup skill tool). connType is "telegram", "slack", etc.
	OnStartConnector func(ctx context.Context, connType string, params map[string]string) error

	// FactsBlock optionally returns a memory/facts block for the given chat ID to inject into the system prompt.
	FactsBlock func(ctx context.Context, chatID string) (string, error)

	// ContextRetriever optionally supplies provenance-bearing retrieved context
	// (CIP P4) injected into the Contextualize step. Nil disables retrieval
	// augmentation, preserving pre-P4 turn-time behavior.
	ContextRetriever ContextRetriever
	// ContextRetrievalBudgetTokens overrides the per-turn retrieval distillation
	// budget. 0 uses the loop default.
	ContextRetrievalBudgetTokens int

	// Proposal Queue: when set, the loop creates proposals on Requires Confirmation
	// and resolution is available via ExecuteApprovedProposal + ResolveProposal.
	SaveProposal                     func(ctx context.Context, p schema.Proposal) error
	GetProposal                      func(ctx context.Context, proposalID string) (schema.Proposal, error)
	FindPendingProposalByBoundaryKey func(ctx context.Context, boundaryKey string) (schema.Proposal, error)
	ResolveProposal                  func(ctx context.Context, proposalID string, status schema.ProposalStatus, resolutionType schema.ResolutionType, resolvedBy, resolutionNote string) error
	ICSStateStore                    ICSStateStore

	// Prompts optionally supplies external prompt templates for non-chat workflows
	// such as heartbeat cycles.
	Prompts *prompts.Manager

	// SaveExecutionOutcome records a command attempt for observability and failure model.
	// When set, the loop records outcomes for skill executions with the skill's CommandType.
	SaveExecutionOutcome func(ctx context.Context, eo schema.ExecutionOutcome) error

	// SaveErrorRecord records structured runtime failures for diagnostics.
	SaveErrorRecord func(ctx context.Context, component, chatID, runID, errorType, message, contextJSON string) error

	// AutonomyResolver optionally applies autonomy (high preset => approve when no hard floor).
	AutonomyResolver AutonomyPresetResolver
	// DomainForSkill returns the domain for a skill for autonomy overrides (e.g. "coding", "memory").
	DomainForSkill func(skillName string) string

	// GovConfigPriorityReader supplies owner config/priorities for governance; when set, ValidateActionWithOwner is used.
	GovConfigPriorityReader governor.ConfigPriorityReader
	// ResolveOwnerID returns the owner ID for the given chat for governance scope.
	ResolveOwnerID func(ctx context.Context, chatID string) string

	// ConnectorHealth optionally returns live connector status for introspection block injection.
	ConnectorHealth func() []ConnectorHealthInfo

	// PluginRegistry optionally holds plugin manifests and tools for discovery and tool-list merging.
	PluginRegistry PluginRegistryInterface

	// RegisterPluginSkillHandlers lets the bootstrap boundary register
	// capability-specific internal transport handlers without internal/navi
	// importing concrete plugin packages.
	RegisterPluginSkillHandlers func(SkillHandlerContext)

	// Debug when true allows surfacing internal errors (e.g. LLM failures) in replies for easier debugging.
	Debug bool
}

// PluginRegistryInterface is implemented by *plugin.Registry so the loop can merge plugin tools.
type PluginRegistryInterface interface {
	ToolEntries() []plugin.ToolRegistration
	ActivePluginSkillPaths() map[string][]string
}

type SkillHandlerContext struct {
	DB             *sql.DB
	LLM            llm.Provider
	Model          string
	WorkspaceDir   string
	BraveSearchKey string
	Hub            skill.Hub
	SkillBuilder   *skill.SkillBuilder
	ResolveOwnerID func(ctx context.Context, chatID string) string
}
