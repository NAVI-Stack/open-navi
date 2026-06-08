package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	pkgconn "github.com/ceoai/navi/connectors"
	"github.com/ceoai/navi/internal/ai"
	artifactsvc "github.com/ceoai/navi/internal/artifact"
	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/coderalias"
	"github.com/ceoai/navi/internal/cognitive"
	"github.com/ceoai/navi/internal/connectors"
	"github.com/ceoai/navi/internal/contextread"
	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/identity"
	intakepolicy "github.com/ceoai/navi/internal/intake/policy"
	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi"
	"github.com/ceoai/navi/internal/navi/experience"
	"github.com/ceoai/navi/internal/navi/plugin"
	"github.com/ceoai/navi/internal/navi/skill"
	"github.com/ceoai/navi/internal/onboarding"
	"github.com/ceoai/navi/internal/presence"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
	navitool "github.com/ceoai/navi/internal/tool"
	"github.com/ceoai/navi/internal/worldmodel"
)

// InterruptionGuardClearer clears the Subconscious recursion guard so a new
// urgent interruption can be emitted. Implemented by the reflection worker.
type InterruptionGuardClearer interface {
	ClearInterruptionGuard()
}

// Config holds all dependencies for the gateway server.
type Config struct {
	DB                    *sql.DB
	DirectiveWriter       cognitive.DirectiveWriter // when set, directive/message/mode writes go through Cognitive facade
	DataDir               string                    // directory containing SQLite DB; used for onboarding checkpoint path
	WorkspaceDir          string                    // NaviD-visible workspace root; path browsing reports server-visible paths
	Bus                   bus.Bus
	Governor              *governor.Governor
	Registry              *ConnectorRegistry
	Manager               *connectors.Manager
	Navi                  *navi.NAVI
	ConversationEndpoints navi.ConversationEndpointStore
	ConnectorDispatcher   navi.ConnectorDispatcher
	ArtifactService       *artifactsvc.Service
	ToolRegistry          *navitool.Registry
	WorldModel            *worldmodel.WorldModel
	Presence              presence.Service
	RefWorker             InterruptionGuardClearer // optional; clears guard when user sends next message
	OriginPatterns        []string
	Addr                  string // default ":8080"
	StaticDir             string // default "web"
	Version               string // default "v0.1.0-open-core"
	Build                 string // optional build identifier (ldflags); included in status

	// LLM is the control plane service that owns all LLM provider infrastructure.
	// When nil, LLM-related endpoints return 503.
	LLM LLMService

	// OnSetupConnector starts a connector.
	OnSetupConnector func(ctx context.Context, connType string, params map[string]string) error

	// SharedSecret is the gateway shared secret; when set, POST /auth/token accepts it and
	// middleware accepts it as X-API-Key for connector/auth compatibility.
	SharedSecret string

	// PluginManifests returns registered plugin manifests for discovery; when nil, GET /api/plugins returns [].
	PluginManifests func() []plugin.Manifest

	// SetPluginEnabled toggles a plugin's enabled state in the registry. When nil,
	// POST /api/plugins/{id}/enable|disable return 501. NOTE: this mutates in-memory
	// registry/tool visibility and is not persisted across daemon restart.
	SetPluginEnabled func(pluginID string, enabled bool) error

	// ReloadPlugins re-runs the plugin loader into the registry. When nil,
	// POST /api/plugins/{id}/reload returns 501.
	ReloadPlugins func() error

	// IsPluginEnabled reports a plugin's runtime enabled-state so the capability
	// graph reflects enable/disable actions. When nil, manifest status is used.
	IsPluginEnabled func(pluginID string) bool

	// SkillRegistry lists skills for GET /api/skills; when nil, skills endpoints return empty/404.
	SkillRegistry *skill.SkillRegistry

	// SetupSchema is the static list of connector setup descriptors captured at startup.
	// Kept for backward compatibility; prefer ConnectorSetupDescriptors when available.
	SetupSchema []connectors.SetupDescriptor

	// ConnectorSetupDescriptors queries the connector registry live for setup
	// descriptors. When set, GET /api/connectors/setup-schema and Phase 5
	// validation use this instead of the static SetupSchema snapshot.
	ConnectorSetupDescriptors func() []connectors.SetupDescriptor

	SaveErrorRecord func(ctx context.Context, component, chatID, runID, errorType, message, contextJSON string) error

	// ExperienceBuilder renders the full derived experience-layer state. When set,
	// the owner-gated inspection endpoint (GET /api/experience/inspect) is available.
	// The function mirrors the same signature used by the reflection worker.
	ExperienceBuilder func(ctx context.Context, mode string, req experience.BuildRequest) (experience.RenderedControl, error)

	// IntakePolicies resolves per-connector sync policy (CIP P5). When nil, the
	// intake policy endpoints construct a resolver from DB on demand (defaults +
	// runtime overrides, without the config/connectors YAML overlay).
	IntakePolicies *intakepolicy.Resolver
}

type Server struct {
	cfg Config
	mux *http.ServeMux
	srv *http.Server

	watchdogStarted atomic.Bool
	watchdogStop    chan struct{}
	watchdogDone    chan struct{}

	greetingTimer atomic.Pointer[time.Timer]

	sendMessageInput func(context.Context, string, naviruntime.MessageInput) (*naviruntime.InboxItem, error)
}

func NewServer(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.StaticDir == "" {
		cfg.StaticDir = "web"
	}
	if cfg.Version == "" {
		cfg.Version = "v0.1.0-open-core"
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	if cfg.Navi != nil {
		s.sendMessageInput = cfg.Navi.SendMessageInput
	}
	s.routes()
	s.srv = &http.Server{Addr: cfg.Addr, Handler: s.requestTracingMiddleware(s.recoverMiddleware(s.mux))}
	return s
}

func (s *Server) Start() error {
	s.startDBPoolWatchdog()
	return s.srv.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	s.stopDBPoolWatchdog()
	if t := s.greetingTimer.Load(); t != nil {
		t.Stop()
	}
	return s.srv.Shutdown(ctx)
}

func (s *Server) Handler() http.Handler {
	return s.requestTracingMiddleware(s.recoverMiddleware(s.mux))
}

func (s *Server) routes() {
	// --- Always-public endpoints ---
	s.mux.Handle("GET /api/version", corsMiddleware(s.cfg.OriginPatterns, http.HandlerFunc(s.handleVersion)))
	s.mux.Handle("GET /health", corsMiddleware(s.cfg.OriginPatterns, http.HandlerFunc(s.handleHealth)))
	s.mux.Handle("GET /api/health", corsMiddleware(s.cfg.OriginPatterns, http.HandlerFunc(s.handleHealth)))
	s.mux.Handle("OPTIONS /api/version", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("OPTIONS /health", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("OPTIONS /api/health", corsPreflightHandler(s.cfg.OriginPatterns))

	// Onboarding — bypasses auth middleware (no API key required) but each
	// route is wrapped with localOnboardingOnly so only loopback/Docker-bridge
	// clients can reach first-run browser APIs.
	onboardCORS := func(h http.HandlerFunc) http.Handler {
		return corsMiddleware(s.cfg.OriginPatterns, s.localOnboardingOnly(http.HandlerFunc(h)))
	}
	s.mux.Handle("GET /api/onboarding/status", onboardCORS(s.handleOnboardingStatus))
	s.mux.Handle("POST /api/onboarding/recovery", onboardCORS(s.handleOnboardingRecovery))
	s.mux.Handle("POST /api/onboarding/provider", onboardCORS(s.handleOnboardingProvider))
	s.mux.Handle("POST /api/onboarding/connection", onboardCORS(s.handleOnboardingConnection))
	s.mux.Handle("POST /api/onboarding/complete", onboardCORS(s.handleOnboardingComplete))
	s.mux.Handle("POST /api/onboarding/reset", onboardCORS(s.handleOnboardingReset))
	s.mux.Handle("OPTIONS /api/onboarding/status", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("OPTIONS /api/onboarding/recovery", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("OPTIONS /api/onboarding/provider", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("OPTIONS /api/onboarding/connection", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("OPTIONS /api/onboarding/complete", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("OPTIONS /api/onboarding/reset", corsPreflightHandler(s.cfg.OriginPatterns))

	// Static files.
	fs := http.FileServer(http.FS(os.DirFS(s.cfg.StaticDir)))
	s.mux.Handle("GET /onboarding", s.localOnboardingOnly(http.HandlerFunc(s.handleOnboardingPage)))
	s.mux.Handle("GET /", s.handleStaticOrOnboardingRedirect(fs))

	// Auth token exchange — rate-limited; no auth required.
	s.mux.Handle("POST /auth/token", rateLimitAuthToken(s.handleTokenAuth))

	// --- Authenticated endpoints ---
	protect := func(h http.HandlerFunc) http.Handler {
		return AuthMiddleware(s.cfg.DB, s.cfg.SharedSecret, h)
	}
	s.mux.Handle("GET /api/plugins", protect(s.handlePlugins))
	s.mux.Handle("POST /api/plugins/{id}/enable", protect(s.handleSetPluginEnabled(true)))
	s.mux.Handle("POST /api/plugins/{id}/disable", protect(s.handleSetPluginEnabled(false)))
	s.mux.Handle("POST /api/plugins/{id}/validate", protect(s.handleValidatePlugin))
	s.mux.Handle("POST /api/plugins/{id}/reload", protect(s.handleReloadPlugins))
	s.mux.Handle("GET /api/capabilities/graph", protect(s.handleCapabilityGraph))
	s.mux.Handle("GET /api/skill-ui", protect(s.handleSkillUI))

	// Auth: who am I (owner check for PET setup visibility).
	s.mux.Handle("GET /api/auth/me", corsMiddleware(s.cfg.OriginPatterns, protect(s.handleAuthMe)))
	s.mux.Handle("OPTIONS /api/auth/me", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("GET /v1/api/auth/me", corsMiddleware(s.cfg.OriginPatterns, protect(s.handleAuthMe)))
	s.mux.Handle("OPTIONS /v1/api/auth/me", corsPreflightHandler(s.cfg.OriginPatterns))

	// API key management (owner secret required — enforced inside handler).
	s.mux.Handle("POST /api/keys", corsMiddleware(s.cfg.OriginPatterns, protect(s.handleCreateAPIKey)))
	s.mux.Handle("GET /api/keys", corsMiddleware(s.cfg.OriginPatterns, protect(s.handleListAPIKeys)))
	s.mux.Handle("DELETE /api/keys/{id}", corsMiddleware(s.cfg.OriginPatterns, protect(s.handleRevokeAPIKey)))
	s.mux.Handle("OPTIONS /api/keys", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("OPTIONS /api/keys/{id}", corsPreflightHandler(s.cfg.OriginPatterns))

	// Instance reset (owner secret required). OPTIONS routed to same handler so preflight gets 204.
	s.mux.Handle("POST /api/instance/reset", corsMiddleware(s.cfg.OriginPatterns, protect(s.handleInstanceReset)))
	s.mux.Handle("OPTIONS /api/instance/reset", corsMiddleware(s.cfg.OriginPatterns, http.HandlerFunc(s.handleInstanceReset)))

	// Setup (legacy CLI onboarding path — loopback bypass keeps it accessible).
	s.mux.Handle("GET /api/setup", protect(s.handleGetSetup))
	s.mux.Handle("POST /api/setup/llm", protect(s.handleSetupLLM))
	s.mux.Handle("POST /api/setup/connector", protect(s.handleSetupConnector))

	// Directives.
	s.mux.Handle("GET /api/directives", protect(s.handleListDirectives))
	s.mux.Handle("POST /api/directives", protect(s.handleCreateDirective))
	s.mux.Handle("GET /api/directives/{id}", protect(s.handleGetDirective))
	s.mux.Handle("POST /api/directives/{id}/message", protect(s.handleDirectiveMessage))
	s.mux.Handle("PUT /api/directives/{id}/mode", protect(s.handleDirectiveMode))

	// Governor.
	s.mux.Handle("GET /api/governor", protect(s.handleGovernorStatus))

	// Operator status — single aggregated snapshot (version, setup, identity, governor, proposals, connectors, llm, degraded).
	s.mux.Handle("GET /api/status", corsMiddleware(s.cfg.OriginPatterns, protect(s.handleStatus)))
	s.mux.Handle("OPTIONS /api/status", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("GET /api/operator/overview", corsMiddleware(s.cfg.OriginPatterns, protect(s.handleOperatorOverview)))
	s.mux.Handle("OPTIONS /api/operator/overview", corsPreflightHandler(s.cfg.OriginPatterns))

	// Agent activity status — lightweight, non-LLM snapshot of what the agent is doing right now.
	s.mux.Handle("GET /api/agent/status", corsMiddleware(s.cfg.OriginPatterns, protect(s.handleAgentStatus)))
	s.mux.Handle("OPTIONS /api/agent/status", corsPreflightHandler(s.cfg.OriginPatterns))

	s.mux.Handle("GET /api/presence/navi", corsMiddleware(s.cfg.OriginPatterns, protect(s.handleNaviPresence)))
	s.mux.Handle("GET /api/presence", corsMiddleware(s.cfg.OriginPatterns, protect(s.handlePresenceSnapshot)))
	s.mux.Handle("POST /api/presence/user", corsMiddleware(s.cfg.OriginPatterns, protect(s.handleUpdateUserPresence)))
	s.mux.Handle("GET /api/presence/heartbeat", corsMiddleware(s.cfg.OriginPatterns, protect(s.handlePresenceHeartbeat)))
	s.mux.Handle("OPTIONS /api/presence/navi", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("OPTIONS /api/presence", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("OPTIONS /api/presence/user", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("OPTIONS /api/presence/heartbeat", corsPreflightHandler(s.cfg.OriginPatterns))

	// LLM operator surface.
	s.mux.Handle("GET /api/llm/catalog", protect(s.handleLLMCatalog))
	s.mux.Handle("GET /api/llm/profiles", protect(s.handleLLMProfiles))
	s.mux.Handle("GET /api/llm/active", protect(s.handleLLMActive))
	s.mux.Handle("PUT /api/llm/active", protect(s.handleSetLLMActive))
	s.mux.Handle("GET /api/llm/preferences", protect(s.handleLLMPreferences))
	s.mux.Handle("PATCH /api/llm/preferences", protect(s.handlePatchLLMPreferences))
	s.mux.Handle("GET /api/llm/proposals", protect(s.handleListLLMKBRoutingProposals))
	s.mux.Handle("POST /api/llm/proposals/{id}/resolve", protect(s.handleResolveLLMKBRoutingProposal))

	// LLM provider control surface.
	s.mux.Handle("GET /api/llm/providers", protect(s.handleListProviders))
	s.mux.Handle("GET /api/llm/providers/{id}", protect(s.handleGetProvider))
	s.mux.Handle("PUT /api/llm/providers/{id}", protect(s.handleConfigureProvider))
	s.mux.Handle("DELETE /api/llm/providers/{id}", protect(s.handleDisableProvider))
	s.mux.Handle("GET /api/llm/providers/{id}/health", protect(s.handleProviderHealth))
	s.mux.Handle("GET /api/llm/providers/{id}/models", protect(s.handleProviderModels))
	s.mux.Handle("GET /api/llm/providers/{id}/running", protect(s.handleProviderRunning))
	s.mux.Handle("POST /api/llm/providers/{id}/actions/pull", protect(s.handleProviderPull))
	s.mux.Handle("POST /api/llm/providers/{id}/actions/delete", protect(s.handleProviderDelete))
	s.mux.Handle("POST /api/llm/providers/{id}/actions/warm", protect(s.handleProviderWarm))
	s.mux.Handle("GET /api/llm/operations/{id}", protect(s.handleGetOperation))

	// Identity — current agent identity (read-only).
	s.mux.Handle("GET /api/identity", protect(s.handleIdentity))

	// Console preferences — narrow owner-facing UI configuration.
	s.mux.Handle("GET /api/console/appearance", corsMiddleware(s.cfg.OriginPatterns, protect(s.handleGetConsoleAppearance)))
	s.mux.Handle("PATCH /api/console/appearance", corsMiddleware(s.cfg.OriginPatterns, protect(s.handlePatchConsoleAppearance)))
	s.mux.Handle("OPTIONS /api/console/appearance", corsPreflightHandler(s.cfg.OriginPatterns))

	// Experience configuration — owner-facing stored experience state.
	s.mux.Handle("GET /api/experience", protect(s.handleGetExperience))
	s.mux.Handle("GET /api/experience/module-registry", protect(s.handleGetExperienceModuleRegistry))
	s.mux.Handle("GET /api/experience/inspect", protect(s.handleGetExperienceInspect))
	s.mux.Handle("PUT /api/experience/core-identity", protect(s.handlePutExperienceCoreIdentity))
	s.mux.Handle("PUT /api/experience/output-preferences", protect(s.handlePutExperienceOutputPreferences))
	s.mux.Handle("PUT /api/experience/modules", protect(s.handlePutExperienceModules))

	// NAVI Ceremony — post-required-onboarding relationship preferences.
	s.mux.Handle("GET /api/ceremony", protect(s.handleGetCeremony))
	s.mux.Handle("POST /api/ceremony/start", protect(s.handleStartCeremony))
	s.mux.Handle("POST /api/ceremony/complete", protect(s.handleCompleteCeremony))
	s.mux.Handle("POST /api/ceremony/skip", protect(s.handleSkipCeremony))
	s.mux.Handle("POST /api/ceremony/init-chat", protect(s.handleInitCeremonyChat))
	s.mux.Handle("POST /api/ceremony/step", protect(s.handleCeremonyStep))

	// Connectors. Register /factories and /setup-schema before /{name} so path segments are not captured as name.
	s.mux.Handle("GET /api/connectors", protect(s.handleListConnectors))
	s.mux.Handle("GET /api/connectors/factories", protect(s.handleListConnectorFactories))
	s.mux.Handle("GET /api/connectors/setup-schema", protect(s.handleSetupSchema))
	s.mux.Handle("GET /api/connectors/{name}", protect(s.handleGetConnector))
	s.mux.Handle("POST /api/connectors", protect(s.handleRegisterConnector))
	s.mux.Handle("DELETE /api/connectors/{name}", protect(s.handleDeregisterConnector))
	s.mux.Handle("POST /api/connectors/endpoints/resolve", protect(s.handleConnectorEndpointResolve))

	// v2 connector runtime surface: instances and capabilities.
	s.mux.Handle("GET /v2/api/connectors/instances", protect(s.handleListConnectorInstancesV2))

	// NAVI chat API.
	s.mux.Handle("POST /api/navi/chats", protect(s.handleNaviCreateChat))
	s.mux.Handle("GET /api/navi/chats", protect(s.handleNaviListChats))
	s.mux.Handle("GET /api/navi/chats/{id}", protect(s.handleNaviGetChat))
	s.mux.Handle("GET /api/navi/chats/{id}/runtime_summary", protect(s.handleNaviGetChatRuntimeSummary))
	s.mux.Handle("GET /api/navi/chats/{id}/endpoints", protect(s.handleNaviListConversationEndpoints))
	s.mux.Handle("POST /api/navi/chats/{id}/endpoints", protect(s.handleNaviCreateConversationEndpoint))
	s.mux.Handle("PATCH /api/navi/chats/{id}/endpoints/{endpoint_id}", protect(s.handleNaviPatchConversationEndpoint))
	s.mux.Handle("GET /api/navi/chats/{id}/delivery-policy", protect(s.handleNaviGetDeliveryPolicy))
	s.mux.Handle("PATCH /api/navi/chats/{id}/delivery-policy", protect(s.handleNaviPatchDeliveryPolicy))
	s.mux.Handle("POST /api/navi/chats/{id}/send", protect(s.handleNaviExplicitEndpointSend))
	s.mux.Handle("PATCH /api/navi/chats/{id}", protect(s.handleNaviUpdateChat))
	s.mux.Handle("POST /api/navi/chats/{id}/message", withGatewayTimeout(func() time.Duration { return gatewaySessionMessageTimeout }, protect(s.handleNaviSendMessage)))
	s.mux.Handle("POST /api/navi/chats/{id}/archive", protect(s.handleNaviArchiveChat))
	s.mux.Handle("DELETE /api/navi/chats/{id}", protect(s.handleNaviDeleteChat))
	s.mux.Handle("POST /api/navi/chats/{id}/messages/{messageId}/feedback", protect(s.handleNaviMessageFeedback))
	s.mux.Handle("POST /api/navi/chats/{id}/messages/{messageId}/edit-resend", protect(s.handleNaviEditResend))
	s.mux.Handle("GET /api/navi/chats/{id}/messages/{messageId}/variants", protect(s.handleNaviMessageVariants))
	s.mux.Handle("POST /api/navi/chats/{id}/messages/{messageId}/variants/select", protect(s.handleNaviSelectMessageVariant))
	s.mux.Handle("POST /api/navi/chats/{id}/regenerate", protect(s.handleNaviRegenerate))
	s.mux.Handle("POST /api/navi/chats/{id}/continue", protect(s.handleNaviContinue))

	// Proposal Queue — list pending and resolve (approve/decline).
	s.mux.Handle("GET /api/proposals", protect(s.handleListProposals))
	s.mux.Handle("POST /api/proposals/{id}/resolve", protect(s.handleResolveProposal))

	// CIP P5 — intake sync policy, sync log, and Vault sync log.
	s.mux.Handle("GET /api/intake/policy", protect(s.handleGetIntakePolicy))
	s.mux.Handle("POST /api/intake/policy", protect(s.handleSetIntakePolicy))
	s.mux.Handle("GET /api/intake/sync-log", protect(s.handleIntakeSyncLog))
	s.mux.Handle("GET /api/intake/recent", protect(s.handleRecentIntake))
	s.mux.Handle("POST /api/intake/backfill", protect(s.handleRequestBackfill))
	s.mux.Handle("GET /api/vault/sync-log", protect(s.handleVaultSyncLog))

	// Capability Gaps — list open gaps and trigger scaffolding.
	s.mux.Handle("GET /api/gaps", protect(s.handleListGaps))
	s.mux.Handle("POST /api/gaps/{id}/scaffold", protect(s.handleBuildSkill))
	s.mux.Handle("POST /api/skills/build", protect(s.handleBuildSkill))

	// Runs (execution outcomes) — list and show with correlation fields.
	s.mux.Handle("GET /api/runs", protect(s.handleListRuns))
	s.mux.Handle("GET /api/runs/{id}", protect(s.handleGetRun))

	// Governed read surface — purpose-scoped, redacted, audited context (read-only).
	s.mux.Handle("POST /api/context/query", protect(s.handleContextQuery))

	s.mux.Handle("GET /api/sandbox-profiles", protect(s.handleListSandboxProfiles))
	s.mux.Handle("POST /api/sandbox-profiles", protect(s.handleCreateSandboxProfile))
	s.mux.Handle("GET /api/sandbox-profiles/{id}", protect(s.handleGetSandboxProfile))
	s.mux.Handle("GET /api/projects", protect(s.handleListProjects))
	s.mux.Handle("POST /api/projects", protect(s.handleCreateProject))
	s.mux.Handle("GET /api/projects/{id}/readiness", protect(s.handleGetProjectReadiness))
	s.mux.Handle("GET /api/projects/{id}/chats", protect(s.handleListProjectChats))
	s.mux.Handle("POST /api/projects/{id}/chats", protect(s.handleCreateProjectChat))
	s.mux.Handle("GET /api/projects/{id}/tasks", protect(s.handleListProjectTasks))
	s.mux.Handle("POST /api/projects/{id}/tasks", protect(s.handleCreateProjectTask))
	s.mux.Handle("GET /api/projects/{id}/tasks/{taskID}", protect(s.handleGetProjectTask))
	s.mux.Handle("POST /api/projects/{id}/tasks/{taskID}/cancel", protect(s.handleCancelProjectTask))
	s.mux.Handle("PUT /api/projects/{id}/workspace-binding", protect(s.handleBindProjectWorkspace))
	s.mux.Handle("DELETE /api/projects/{id}/workspace-binding", protect(s.handleUnbindProjectWorkspace))
	s.mux.Handle("GET /api/projects/{id}/workspace", protect(s.handleGetProjectWorkspace))
	s.mux.Handle("GET /api/projects/{id}", protect(s.handleGetProject))
	s.mux.Handle("PUT /api/projects/{id}", protect(s.handleUpdateProject))
	s.mux.Handle("POST /api/projects/{id}/archive", protect(s.handleArchiveProject))
	s.mux.Handle("DELETE /api/projects/{id}", protect(s.handleDeleteProject))
	s.mux.Handle("GET /api/workspace-mode", protect(s.handleGetWorkspaceMode))
	s.mux.Handle("PUT /api/workspace-mode", protect(s.handleSetWorkspaceMode))
	s.mux.Handle("GET /api/workspaces/active", protect(s.handleGetActiveWorkspace))
	s.mux.Handle("PUT /api/workspaces/active", protect(s.handleSetActiveWorkspace))
	s.mux.Handle("GET /api/workspaces/path-roots", protect(s.handleWorkspacePathRoots))
	s.mux.Handle("GET /api/workspaces/path-children", protect(s.handleWorkspacePathChildren))
	s.mux.Handle("GET /api/workspaces", protect(s.handleListWorkspaces))
	s.mux.Handle("POST /api/workspaces", protect(s.handleCreateWorkspace))
	s.mux.Handle("POST /api/workspaces/boundary/resolve", protect(s.handleResolveWorkspaceBoundary))
	s.mux.Handle("GET /api/workspaces/{id}", protect(s.handleGetWorkspace))
	s.mux.Handle("PUT /api/workspaces/{id}", protect(s.handleUpdateWorkspace))
	s.mux.Handle("DELETE /api/workspaces/{id}", protect(s.handleDeleteWorkspace))
	s.mux.Handle("POST /api/workspaces/{id}/archive", protect(s.handleArchiveWorkspace))
	s.mux.Handle("PUT /api/workspaces/{id}/project-binding", protect(s.handleBindWorkspaceProject))
	s.mux.Handle("GET /api/workspaces/{id}/whitelist-rules", protect(s.handleListWhitelistRules))
	s.mux.Handle("POST /api/workspaces/{id}/whitelist-rules", protect(s.handleCreateWhitelistRule))
	s.mux.Handle("POST /api/workspaces/{id}/whitelist-rules/{ruleID}/revoke", protect(s.handleRevokeWhitelistRule))
	s.mux.Handle("GET /api/artifacts", protect(s.handleListArtifacts))
	s.mux.Handle("GET /api/artifacts/{id}", protect(s.handleGetArtifact))
	s.mux.Handle("GET /api/artifacts/{id}/content", protect(s.handleGetArtifactContent))
	s.mux.Handle("POST /api/artifacts/{id}/content", protect(s.handleSaveArtifactContent))
	s.mux.Handle("GET /api/artifacts/{id}/versions/{versionID}/content", protect(s.handleGetArtifactContent))
	s.mux.Handle("GET /api/artifacts/{id}/versions/{versionID}/diff", protect(s.handleGetArtifactDiff))
	s.mux.Handle("POST /api/artifacts/{id}/versions/{versionID}/restore", protect(s.handleRestoreArtifactVersion))
	s.mux.Handle("POST /api/artifacts/{id}/branches", protect(s.handleCreateArtifactBranch))
	s.mux.Handle("GET /api/artifacts/{id}/exports", protect(s.handleListArtifactExports))
	s.mux.Handle("POST /api/artifacts/{id}/exports", protect(s.handleCreateArtifactExport))
	s.mux.Handle("GET /api/artifacts/{id}/shares", protect(s.handleListArtifactShares))
	s.mux.Handle("POST /api/artifacts/{id}/shares", protect(s.handleCreateArtifactShare))
	s.mux.Handle("GET /api/artifact-exports/{exportID}/content", protect(s.handleDownloadArtifactExport))

	// Activity — operator feed (mapped from events; cursor = event seq).
	s.mux.Handle("GET /api/activity", protect(s.handleActivity))

	// Skills — list and show with governance/operational metadata.
	s.mux.Handle("GET /api/skills", protect(s.handleListSkills))
	s.mux.Handle("GET /api/skills/{id}", protect(s.handleGetSkill))
	s.mux.Handle("GET /api/skills/{id}/ui", protect(s.handleSkillUISurfaces))
	s.mux.Handle("POST /api/skills/reload", protect(s.handleReloadSkills))
	s.mux.Handle("POST /api/prompts/reload", protect(s.handleReloadPrompts))
	s.mux.Handle("POST /api/skills/{id}/validate", protect(s.handleValidateSkill))
	s.mux.Handle("POST /api/skills/{id}/interfaces/{interface}/invoke", protect(s.handleInvokeSkillInterface))
	s.mux.Handle("GET /api/tools", protect(s.handleListTools))
	s.mux.Handle("GET /api/tools/{name}", protect(s.handleGetTool))
	s.mux.Handle("GET /api/knowledge", protect(s.handleKnowledge))

	// Debug — raw event log (auth-protected).
	s.mux.Handle("GET /api/debug/events", protect(s.handleDebugEvents))
	s.mux.Handle("GET /api/debug/llmkb/profiles", protect(s.handleDebugLLMKBProfiles))
	s.mux.Handle("GET /api/debug/llmkb/profiles/{id}", protect(s.handleDebugLLMKBProfile))
	s.mux.Handle("GET /api/errors", protect(s.handleErrors))
	s.mux.Handle("GET /api/errors/summary", protect(s.handleErrorSummary))

	// Debug — streaming runtime metrics snapshot.
	s.mux.Handle("GET /api/debug/runtime-metrics", protect(s.handleRuntimeMetrics))

	// WebSocket live feed.
	s.mux.Handle("GET /ws/live", protect(s.handleLive))

	// OpenAI-compatible (ai-chat / any OpenAI client).
	s.mux.Handle("POST /v1/chat/completions",
		corsMiddleware(s.cfg.OriginPatterns, protect(http.HandlerFunc(s.handleOpenAIChatCompletions))))
	s.mux.Handle("GET /v1/models",
		corsMiddleware(s.cfg.OriginPatterns, protect(http.HandlerFunc(s.handleOpenAIModels))))
	s.mux.Handle("OPTIONS /v1/chat/completions", corsPreflightHandler(s.cfg.OriginPatterns))
	s.mux.Handle("OPTIONS /v1/models", corsPreflightHandler(s.cfg.OriginPatterns))

	// Diagnostics.
	s.mux.Handle("GET /api/health/connectors", protect(s.handleConnectorHealth))
	s.mux.Handle("GET /api/diagnostics/connectors", protect(s.handleConnectorDiagnostics))
	s.mux.Handle("GET /api/webhooks", protect(s.handleListWebhookRegistrations))
	s.mux.Handle("PUT /api/webhooks/{source}", protect(s.handlePutWebhookRegistration))
	s.mux.Handle("DELETE /api/webhooks/{source}", protect(s.handleDeleteWebhookRegistration))

	// AI Utilities.
	s.mux.Handle("POST /api/ai/conversations/title", protect(s.handleGenerateConversationTitle))

	// Connector webhooks — no auth (external services POST here).
	s.mux.Handle("POST /api/webhooks/{source}", http.HandlerFunc(s.handleWebhookIngress))
	s.mux.Handle("POST /webhooks/{name}", http.HandlerFunc(s.handleConnectorWebhook))
}

func replyJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}

func replyError(w http.ResponseWriter, code int, msg string) {
	replyJSON(w, code, map[string]string{"error": msg})
}

// replyErrorAPI returns the operator-shell spec error shape (section 9): code, message, details.
func replyErrorAPI(w http.ResponseWriter, httpCode int, errCode, message string, details any) {
	replyJSON(w, httpCode, map[string]any{
		"error": map[string]any{
			"code":    errCode,
			"message": message,
			"details": details,
		},
	})
}

// ErrorPayload is the optional structured error body for programmatic clients
// (degradation_type, recovery_proposal_id per Tier-3 failure visibility).
type ErrorPayload struct {
	Error              string `json:"error"`
	DegradationType    string `json:"degradation_type,omitempty"`
	RecoveryProposalID string `json:"recovery_proposal_id,omitempty"`
}

func replyErrorStructured(w http.ResponseWriter, code int, msg, degradationType, recoveryProposalID string) {
	payload := ErrorPayload{Error: msg}
	if degradationType != "" {
		payload.DegradationType = degradationType
	}
	if recoveryProposalID != "" {
		payload.RecoveryProposalID = recoveryProposalID
	}
	replyJSON(w, code, payload)
}

func parseConnectorMessageRef(ref string) (chatID, messageID string, ok bool) {
	parts := strings.Split(strings.TrimSpace(ref), ":")
	if len(parts) != 3 {
		return "", "", false
	}
	chatID = strings.TrimSpace(parts[0])
	messageID = strings.TrimSpace(parts[2])
	if chatID == "" || messageID == "" {
		return "", "", false
	}
	return chatID, messageID, true
}

func (s *Server) triggerConnectorInbound(ctx context.Context, connectorName, sourceMessageRef string) {
	if s.cfg.Manager == nil || strings.TrimSpace(connectorName) == "" {
		return
	}
	if registry := s.cfg.Manager.Registry(); registry != nil {
		if conn := registry.Get(connectorName); conn != nil {
			if managed, ok := conn.(pkgconn.InboundOrchestrationManaged); ok && managed.ManagesInboundOrchestration() {
				return
			}
		}
	}
	chatID, messageID, ok := parseConnectorMessageRef(sourceMessageRef)
	if !ok {
		return
	}
	s.cfg.Manager.OnInbound(ctx, connectorName, chatID, messageID)
}

func (s *Server) handleIdentity(w http.ResponseWriter, r *http.Request) {
	id, found, err := store.GetActiveAgentIdentity(r.Context(), s.cfg.DB)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		replyError(w, http.StatusNotFound, "no active identity")
		return
	}
	replyJSON(w, http.StatusOK, map[string]string{
		"id":          id.ID,
		"navi_id":     identity.NaviID(id.Fingerprint),
		"key_type":    id.KeyType,
		"public_key":  id.PublicKey,
		"fingerprint": id.Fingerprint,
		"created_at":  id.CreatedAt.UTC().Format(time.RFC3339Nano),
		"status":      id.Status,
	})
}

func (s *Server) handleListDirectives(w http.ResponseWriter, r *http.Request) {
	list, err := store.GetActiveDirectives(r.Context(), s.cfg.DB)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []schema.Directive{}
	}
	replyJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateDirective(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DirectiveWriter == nil {
		replyError(w, http.StatusServiceUnavailable, "directive writer not configured (Cognitive layer required for World Model writes)")
		return
	}
	var req struct {
		Title string `json:"title"`
		Mode  string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	mode := schema.DirectiveMode(req.Mode)
	if err := mode.Validate(); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	d := schema.NewDirective(req.Title, mode, "api")
	if err := s.cfg.DirectiveWriter.SaveDirective(r.Context(), d); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusCreated, d)
}

func (s *Server) handleGetDirective(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, found, err := store.GetDirective(r.Context(), s.cfg.DB, id)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		replyError(w, http.StatusNotFound, "directive not found")
		return
	}
	msgs, err := store.GetMessages(r.Context(), s.cfg.DB, id, 100)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if msgs == nil {
		msgs = []schema.DirectiveMessage{}
	}
	replyJSON(w, http.StatusOK, map[string]any{"directive": d, "messages": msgs})
}

func (s *Server) handleDirectiveMessage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DirectiveWriter == nil {
		replyError(w, http.StatusServiceUnavailable, "directive writer not configured (Cognitive layer required for World Model writes)")
		return
	}
	id := r.PathValue("id")
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	ownerID := OwnerIDFromCtx(r.Context())
	msg := schema.DirectiveMessage{
		MessageID:   uuid.NewString(),
		DirectiveID: id,
		Role:        "owner",
		Content:     req.Content,
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.cfg.DirectiveWriter.AppendMessage(r.Context(), msg); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	payload := schema.DirectiveMessagePayload{
		DirectiveID: id,
		MessageID:   msg.MessageID,
		OwnerID:     ownerID,
	}
	ev := schema.NewEvent(schema.CmdDirectiveMessage, schema.EventKindCommand,
		uuid.NewString(), schema.AgentNavi, payload)
	if err := s.cfg.Bus.Publish(r.Context(), ev); err != nil {
		_ = s.cfg.DirectiveWriter.DeleteMessage(r.Context(), msg.MessageID)
		replyError(w, http.StatusInternalServerError, "failed to deliver message: "+err.Error())
		return
	}
	replyJSON(w, http.StatusCreated, msg)
}

func (s *Server) handleDirectiveMode(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DirectiveWriter == nil {
		replyError(w, http.StatusServiceUnavailable, "directive writer not configured (Cognitive layer required for World Model writes)")
		return
	}
	id := r.PathValue("id")
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	mode := schema.DirectiveMode(req.Mode)
	if err := mode.Validate(); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.cfg.DirectiveWriter.UpdateDirectiveMode(r.Context(), id, mode); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGovernorStatus(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Governor == nil {
		replyError(w, http.StatusInternalServerError, "governor not configured")
		return
	}
	stats := s.cfg.Governor.Stats()
	replyJSON(w, http.StatusOK, map[string]any{
		"actions":    stats.Actions,
		"total_usd":  stats.TotalUSD,
		"elapsed_ms": stats.Elapsed.Milliseconds(),
	})
}

// connectorState maps health or registry status to canonical operator state (running|stopped|degraded|error|starting).
func connectorState(status string) string {
	switch status {
	case "healthy", "connected", "configured":
		return "running"
	case "degraded":
		return "degraded"
	case "down", "disconnected", "error":
		return "error"
	case "starting":
		return "starting"
	default:
		return "stopped"
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	settings, _ := store.GetAllSettings(ctx, s.cfg.DB)
	setupComplete := settings["setup_complete"] == "true"

	agentID := ""
	ownerID := ""
	if ident, ok, _ := store.GetActiveAgentIdentity(ctx, s.cfg.DB); ok {
		if ident.Fingerprint != "" {
			agentID = ident.Fingerprint
		} else {
			agentID = ident.ID
		}
	}
	timezone := "UTC"
	if owner, ok, err := store.GetOwner(ctx, s.cfg.DB); err == nil && ok {
		ownerID = owner.ID
		timezone = owner.Timezone
	}

	govTripped := false
	govBudgetUsed := 0
	govBudgetMax := 0
	govCostUsed := 0.0
	govCostCeiling := 0.0
	govDurationRemaining := ""
	if s.cfg.Governor != nil {
		stats := s.cfg.Governor.Stats()
		limits := s.cfg.Governor.Limits()
		govBudgetUsed = stats.Actions
		govBudgetMax = limits.MaxActionBudget
		govCostUsed = stats.TotalUSD
		govCostCeiling = limits.CostCeiling
		remaining := limits.AutonomousDuration - stats.Elapsed
		if remaining < 0 {
			remaining = 0
		}
		govDurationRemaining = remaining.String()
		govTripped = stats.Actions >= limits.MaxActionBudget ||
			stats.TotalUSD >= limits.CostCeiling ||
			stats.Elapsed >= limits.AutonomousDuration ||
			stats.Retries >= limits.MaxRetries
	}

	pendingCount := 0
	if list, err := store.ListPendingProposals(ctx, s.cfg.DB, 1000); err == nil {
		pendingCount = len(list)
	}

	connItems := []map[string]any{}
	connRunning := 0
	connDegraded := 0
	connStopped := 0
	connError := 0
	if s.cfg.Manager != nil {
		for _, h := range s.cfg.Manager.Health() {
			state := connectorState(h.Status)
			connItems = append(connItems, map[string]any{
				"name":             h.Name,
				"state":            state,
				"last_activity_at": h.LastSendAt,
				"last_error_at":    h.LastErrorAt,
				"send_count":       h.SendCount,
				"error_count":      h.ErrorCount,
				"queue_depth":      h.QueueDepth,
			})
			switch state {
			case "running":
				connRunning++
			case "degraded":
				connDegraded++
			case "error":
				connError++
			default:
				connStopped++
			}
		}
	}

	llmStatus := ""
	llmConfigured := false
	if s.cfg.LLM != nil {
		llmStatus = s.cfg.LLM.Status()
		if llmStatus == "none" {
			llmStatus = ""
		}
		llmConfigured = llmStatus != ""
	}

	degraded := govTripped || connError > 0
	for _, c := range connItems {
		if c["state"] == "degraded" {
			degraded = true
			break
		}
	}

	replyJSON(w, http.StatusOK, map[string]any{
		"gateway": map[string]any{
			"reachable": true,
			"version":   s.cfg.Version,
			"build":     s.cfg.Build,
		},
		"setup": map[string]any{
			"complete": setupComplete,
		},
		"identity": map[string]any{
			"agent_id": agentID,
			"owner_id": ownerID,
			"timezone": timezone,
		},
		"governor": map[string]any{
			"tripped":            govTripped,
			"budget_used":        govBudgetUsed,
			"budget_max":         govBudgetMax,
			"cost_used":          govCostUsed,
			"cost_ceiling":       govCostCeiling,
			"duration_remaining": govDurationRemaining,
		},
		"proposals": map[string]any{
			"pending_count": pendingCount,
		},
		"connectors": map[string]any{
			"total":    len(connItems),
			"running":  connRunning,
			"degraded": connDegraded,
			"stopped":  connStopped,
			"error":    connError,
			"items":    connItems,
		},
		"llm": map[string]any{
			"configured": llmConfigured,
			"status":     llmStatus,
		},
		"degraded":   degraded,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleAgentStatus(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyJSON(w, http.StatusOK, map[string]any{
			"state":      string(schema.AgentStateOffline),
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	snap := s.cfg.Navi.Status()
	replyJSON(w, http.StatusOK, map[string]any{
		"state":                     string(snap.State),
		"since":                     snap.Since.Format(time.RFC3339),
		"last_activity_at":          snap.LastActivityAt.Format(time.RFC3339),
		"uptime_since":              snap.UptimeSince.Format(time.RFC3339),
		"active_runtime_session_id": snap.ActiveRuntimeSessionID,
		"current_detail":            snap.CurrentDetail,
		"turns_processed":           snap.TurnsProcessed,
		"updated_at":                snap.UpdatedAt.Format(time.RFC3339),
	})
}

func (s *Server) buildOperatorStatusPayload(ctx context.Context) map[string]any {
	settings, _ := store.GetAllSettings(ctx, s.cfg.DB)
	setupComplete := settings["setup_complete"] == "true"

	agentID := ""
	ownerID := ""
	if ident, ok, _ := store.GetActiveAgentIdentity(ctx, s.cfg.DB); ok {
		if ident.Fingerprint != "" {
			agentID = ident.Fingerprint
		} else {
			agentID = ident.ID
		}
	}
	timezone := "UTC"
	if owner, ok, err := store.GetOwner(ctx, s.cfg.DB); err == nil && ok {
		ownerID = owner.ID
		timezone = owner.Timezone
	}

	govTripped := false
	govBudgetUsed := 0
	govBudgetMax := 0
	govCostUsed := 0.0
	govCostCeiling := 0.0
	govDurationRemaining := ""
	if s.cfg.Governor != nil {
		stats := s.cfg.Governor.Stats()
		limits := s.cfg.Governor.Limits()
		govBudgetUsed = stats.Actions
		govBudgetMax = limits.MaxActionBudget
		govCostUsed = stats.TotalUSD
		govCostCeiling = limits.CostCeiling
		remaining := limits.AutonomousDuration - stats.Elapsed
		if remaining < 0 {
			remaining = 0
		}
		govDurationRemaining = remaining.String()
		govTripped = stats.Actions >= limits.MaxActionBudget ||
			stats.TotalUSD >= limits.CostCeiling ||
			stats.Elapsed >= limits.AutonomousDuration ||
			stats.Retries >= limits.MaxRetries
	}

	pendingCount := 0
	if list, err := store.ListPendingProposals(ctx, s.cfg.DB, 1000); err == nil {
		pendingCount = len(list)
	}

	connItems := []map[string]any{}
	connRunning := 0
	connDegraded := 0
	connStopped := 0
	connError := 0
	if s.cfg.Manager != nil {
		for _, h := range s.cfg.Manager.Health() {
			state := connectorState(h.Status)
			connItems = append(connItems, map[string]any{
				"name":             h.Name,
				"state":            state,
				"last_activity_at": h.LastSendAt,
				"last_error_at":    h.LastErrorAt,
				"send_count":       h.SendCount,
				"error_count":      h.ErrorCount,
				"queue_depth":      h.QueueDepth,
			})
			switch state {
			case "running":
				connRunning++
			case "degraded":
				connDegraded++
			case "error":
				connError++
			default:
				connStopped++
			}
		}
	}

	llmStatus := ""
	llmConfigured := false
	if s.cfg.LLM != nil {
		llmStatus = s.cfg.LLM.Status()
		if llmStatus == "none" {
			llmStatus = ""
		}
		llmConfigured = llmStatus != ""
	}

	degraded := govTripped || connError > 0
	for _, c := range connItems {
		if c["state"] == "degraded" {
			degraded = true
			break
		}
	}

	return map[string]any{
		"gateway": map[string]any{
			"reachable": true,
			"version":   s.cfg.Version,
			"build":     s.cfg.Build,
		},
		"setup": map[string]any{
			"complete": setupComplete,
		},
		"identity": map[string]any{
			"agent_id": agentID,
			"owner_id": ownerID,
			"timezone": timezone,
		},
		"governor": map[string]any{
			"tripped":            govTripped,
			"budget_used":        govBudgetUsed,
			"budget_max":         govBudgetMax,
			"cost_used":          govCostUsed,
			"cost_ceiling":       govCostCeiling,
			"duration_remaining": govDurationRemaining,
		},
		"proposals": map[string]any{
			"pending_count": pendingCount,
		},
		"connectors": map[string]any{
			"total":    len(connItems),
			"running":  connRunning,
			"degraded": connDegraded,
			"stopped":  connStopped,
			"error":    connError,
			"items":    connItems,
		},
		"llm": map[string]any{
			"configured": llmConfigured,
			"status":     llmStatus,
		},
		"degraded":   degraded,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) buildAgentStatusPayload() map[string]any {
	if s.cfg.Navi == nil {
		return map[string]any{
			"state":      string(schema.AgentStateOffline),
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		}
	}
	snap := s.cfg.Navi.Status()
	return map[string]any{
		"state":                     string(snap.State),
		"since":                     snap.Since.Format(time.RFC3339),
		"last_activity_at":          snap.LastActivityAt.Format(time.RFC3339),
		"uptime_since":              snap.UptimeSince.Format(time.RFC3339),
		"active_runtime_session_id": snap.ActiveRuntimeSessionID,
		"current_detail":            snap.CurrentDetail,
		"turns_processed":           snap.TurnsProcessed,
		"updated_at":                snap.UpdatedAt.Format(time.RFC3339),
	}
}

func (s *Server) buildSkillsSummaryPayload() map[string]any {
	if s.cfg.SkillRegistry == nil {
		return map[string]any{
			"total":       0,
			"activatable": 0,
			"items":       []any{},
		}
	}
	list := s.cfg.SkillRegistry.List()
	items := make([]map[string]any, 0, len(list))
	activatable := 0
	for _, entry := range list {
		if entry.Activatable {
			activatable++
		}
		items = append(items, skillEntryToMap(entry))
	}
	return map[string]any{
		"total":       len(items),
		"activatable": activatable,
		"items":       items,
	}
}

func (s *Server) handleOperatorOverview(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusInternalServerError, "database not configured")
		return
	}
	ctx := r.Context()
	status := s.buildOperatorStatusPayload(ctx)
	agent := s.buildAgentStatusPayload()

	pendingProposals, err := store.ListPendingProposals(ctx, s.cfg.DB, 20)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pendingProposals == nil {
		pendingProposals = []schema.Proposal{}
	}

	runs, _, err := store.ListExecutionOutcomes(ctx, s.cfg.DB, 10, "", nil)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	runItems := make([]map[string]any, 0, len(runs))
	for i := range runs {
		runItems = append(runItems, runOutcomeToMap(&runs[i]))
	}

	events, nextSeq, err := store.ListEvents(ctx, s.cfg.DB, 0, nil, 25)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	activityItems := make([]ActivityEntry, 0, len(events))
	for _, ev := range events {
		if entry, ok := MapEventToActivity(ev); ok {
			activityItems = append(activityItems, entry)
		}
	}

	replyJSON(w, http.StatusOK, map[string]any{
		"status": status,
		"agent":  agent,
		"proposals": map[string]any{
			"pending_count": len(pendingProposals),
			"items":         pendingProposals,
		},
		"runs": map[string]any{
			"items": runItems,
		},
		"activity": map[string]any{
			"items":       activityItems,
			"next_cursor": nextSeq,
		},
		"skills":     s.buildSkillsSummaryPayload(),
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleNaviPresence(w http.ResponseWriter, r *http.Request) {
	presenceSvc := s.presenceService()
	if presenceSvc == nil {
		replyError(w, http.StatusServiceUnavailable, "presence service not configured")
		return
	}
	replyJSON(w, http.StatusOK, presenceSvc.NaviPresence(r.Context()))
}

func (s *Server) handlePresenceSnapshot(w http.ResponseWriter, r *http.Request) {
	presenceSvc := s.presenceService()
	if presenceSvc == nil {
		replyError(w, http.StatusServiceUnavailable, "presence service not configured")
		return
	}
	replyJSON(w, http.StatusOK, presenceSvc.Snapshot(r.Context()))
}

func (s *Server) handleUpdateUserPresence(w http.ResponseWriter, r *http.Request) {
	presenceSvc := s.presenceService()
	if presenceSvc == nil {
		replyError(w, http.StatusServiceUnavailable, "presence service not configured")
		return
	}
	var envelope presence.PresenceEnvelope
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := presenceSvc.UpdateUserPresence(r.Context(), envelope); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePresenceHeartbeat(w http.ResponseWriter, r *http.Request) {
	presenceSvc := s.presenceService()
	if presenceSvc == nil {
		replyError(w, http.StatusServiceUnavailable, "presence service not configured")
		return
	}
	presenceSvc.Heartbeat(r.Context())
	replyJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) presenceService() presence.Service {
	if s == nil {
		return nil
	}
	if s.cfg.Navi != nil {
		return s.cfg.Navi.PresenceService()
	}
	return s.cfg.Presence
}

func runOutcomeToMap(eo *schema.ExecutionOutcome) map[string]any {
	m := map[string]any{
		"run_id":              eo.AttemptID,
		"command_id":          eo.CommandID,
		"type":                string(eo.CommandType),
		"status":              string(eo.Outcome),
		"started_at":          eo.StartTime.UTC().Format(time.RFC3339),
		"outcome":             string(eo.Outcome),
		"runtime_session_id":  eo.RuntimeSessionID,
		"proposal_id":         eo.ProposalID,
		"skill_ids":           eo.SkillIDs,
		"connector_ids":       eo.ConnectorIDs,
		"failure_class":       string(eo.FailureClass),
		"llm_provider":        eo.LLMProvider,
		"llm_model":           eo.LLMModel,
		"llm_task_class":      eo.LLMTaskClass,
		"llm_complexity":      eo.LLMComplexity,
		"parent_run_id":       eo.ParentRunID,
		"correlation_id":      eo.CorrelationID,
		"failure_reason":      eo.FailureReason,
		"compensation_status": string(eo.CompensationStatus),
		"recovery_status":     string(eo.RecoveryStatus),
		"affected_entities":   decodeFlexibleJSON(eo.AffectedEntities),
		"workspace_id":        eo.WorkspaceID,
		"boundary_crossing":   eo.BoundaryCrossing,
		"approval_required":   eo.ApprovalRequired,
		"approval_outcome":    string(eo.ApprovalOutcome),
		"artifact_id":         eo.ArtifactID,
	}
	if eo.EndTime != nil {
		m["ended_at"] = eo.EndTime.UTC().Format(time.RFC3339)
	}
	return m
}

func artifactToMap(a *schema.Artifact) map[string]any {
	if a == nil {
		return nil
	}
	reg := artifactsvc.NewRegistry()
	renderer := reg.GetRenderer(a.Subtype)
	editor := reg.GetEditor(a.Subtype)
	return decorateArtifactMap(map[string]any{
		"id":                  a.ID,
		"workspace_id":        a.WorkspaceID,
		"project_id":          a.ProjectID,
		"owner_id":            a.OwnerID,
		"title":               firstNonEmptyGateway(a.DisplayTitle, a.CanonicalTitle, a.Description, a.ID),
		"canonical_title":     a.CanonicalTitle,
		"display_title":       a.DisplayTitle,
		"type":                string(a.Type),
		"kind":                a.Kind,
		"subtype":             a.Subtype,
		"schema_version":      a.SchemaVersion,
		"content_format":      a.ContentFormat,
		"lifecycle_state":     string(a.LifecycleState),
		"current_branch_id":   a.CurrentBranchID,
		"current_version_id":  a.CurrentVersionID,
		"head_version_number": a.HeadVersionNumber,
		"location":            a.Location,
		"description":         a.Description,
		"status":              a.Status,
		"render_status":       a.RenderStatus,
		"version":             a.Version,
		"source_tool":         a.SourceTool,
		"source_execution_id": a.SourceExecutionID,
		"source_run_id":       a.SourceRunID,
		"last_error":          a.LastError,
		"recoverable_output":  decodeFlexibleJSON(a.RecoverableOutput),
		"metadata":            decodeFlexibleJSON(a.Metadata),
		"renderer":            rendererDescriptorToMap(renderer),
		"editor":              editorDescriptorToMap(editor),
		"created_at":          a.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":          a.UpdatedAt.UTC().Format(time.RFC3339),
		"archived_at":         formatOptionalTime(a.ArchivedAt),
	}, a)
}

func artifactVersionToMap(v schema.ArtifactVersion) map[string]any {
	out := map[string]any{
		"version_id":           v.VersionID,
		"artifact_id":          v.ArtifactID,
		"branch_id":            v.BranchID,
		"version":              v.Version,
		"version_number":       v.VersionNumber,
		"parent_version_id":    v.ParentVersionID,
		"base_version_id":      v.BaseVersionID,
		"change_type":          v.ChangeType,
		"change_mode":          v.ChangeMode,
		"change_summary":       v.ChangeSummary,
		"status":               v.Status,
		"location":             v.Location,
		"description":          v.Description,
		"snapshot":             decodeFlexibleJSON(v.Snapshot),
		"execution_attempt_id": v.ExecutionAttemptID,
		"validation_state":     v.ValidationState,
		"commit_state":         string(v.CommitState),
		"size_in_bytes":        v.SizeInBytes,
		"patch_strategy":       v.PatchStrategy,
		"policy_snapshot_id":   v.PolicySnapshotID,
		"content_ref":          contentRefToMap(&v.ContentRef),
		"rendered_ref":         contentRefToMap(v.RenderedRef),
		"diff_ref":             contentRefToMap(v.DiffRef),
		"created_at":           v.CreatedAt.UTC().Format(time.RFC3339),
	}
	return out
}

func rendererDescriptorToMap(desc artifactsvc.RendererDescriptor) map[string]any {
	return map[string]any{
		"type":             string(desc.Type),
		"subtype":          desc.Subtype,
		"component_id":     desc.ComponentID,
		"diff_viewer_id":   desc.DiffViewerID,
		"icon":             desc.Icon,
		"label":            desc.Label,
		"display_modes":    desc.DisplayModes,
		"export_formats":   desc.ExportFormats,
		"patch_strategies": desc.PatchStrategies,
		"supports_diff":    desc.SupportsDiff,
		"supports_history": desc.SupportsHistory,
		"supports_print":   desc.SupportsPrint,
	}
}

func editorDescriptorToMap(desc artifactsvc.EditorDescriptor) map[string]any {
	return map[string]any{
		"type":                string(desc.Type),
		"subtype":             desc.Subtype,
		"editor_id":           desc.EditorID,
		"diff_viewer_id":      desc.DiffViewerID,
		"features":            desc.Features,
		"parse_formats":       desc.ParseFormats,
		"export_formats":      desc.ExportFormats,
		"patch_strategies":    desc.PatchStrategies,
		"validate_strategy":   desc.ValidateStrategy,
		"supports_patch":      desc.SupportsPatch,
		"supports_replace":    desc.SupportsReplace,
		"supports_preview":    desc.SupportsPreview,
		"supports_structured": desc.SupportsStructured,
	}
}

func contentRefToMap(ref *schema.ContentRef) map[string]any {
	if ref == nil {
		return nil
	}
	return map[string]any{
		"uri":             ref.URI,
		"storage_key":     ref.StorageKey,
		"checksum_sha256": ref.ChecksumSHA256,
	}
}

func formatOptionalTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func firstNonEmptyGateway(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusInternalServerError, "database not configured")
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	cursor := r.URL.Query().Get("cursor")
	var since *time.Time
	if t := r.URL.Query().Get("since"); t != "" {
		parsed, err := time.Parse(time.RFC3339, t)
		if err == nil {
			since = &parsed
		}
	}
	list, nextCursor, err := store.ListExecutionOutcomes(r.Context(), s.cfg.DB, limit, cursor, since)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]any, 0, len(list))
	for i := range list {
		items = append(items, runOutcomeToMap(&list[i]))
	}
	resp := map[string]any{"items": items}
	if nextCursor != "" {
		resp["next_cursor"] = nextCursor
	}
	replyJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "run id required", nil)
		return
	}
	if s.cfg.DB == nil {
		replyError(w, http.StatusInternalServerError, "database not configured")
		return
	}
	eo, err := store.GetExecutionOutcomeByAttemptID(r.Context(), s.cfg.DB, id)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if eo == nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "Run not found", nil)
		return
	}
	replyJSON(w, http.StatusOK, runOutcomeToMap(eo))
}

// handleContextQuery is the governed, read-only context surface (query_context).
// It is the Go enforcement point for Language-Layer Contract §4: the request must
// declare a purpose and scope, both enumerated; the result is scope-bounded,
// redacted, provenance-tagged, and audit-attributed. There is no generic
// world-model/DB passthrough here and no write/schedule path.
func (s *Server) handleContextQuery(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil || s.cfg.DB == nil {
		replyError(w, http.StatusServiceUnavailable, "context query not configured")
		return
	}
	var req struct {
		RunID   string `json:"run_id"`
		Purpose string `json:"purpose"`
		Scope   string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}

	caller := OwnerIDFromCtx(r.Context())
	if caller == "" {
		if resolved, err := store.GetOwnerID(r.Context(), s.cfg.DB); err == nil {
			caller = resolved
		}
	}

	db := s.cfg.DB
	mediator := contextread.NewMediator(s.cfg.WorldModel, func(ctx context.Context, ev schema.Event) error {
		return store.AppendEvent(ctx, db, ev)
	})
	resp, err := mediator.Query(r.Context(), contextread.Request{
		RunID:   req.RunID,
		Purpose: contextread.Purpose(req.Purpose),
		Scope:   contextread.Scope(req.Scope),
		Caller:  caller,
	})
	if err != nil {
		switch {
		case contextread.IsRejection(err):
			replyError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, contextread.ErrRunNotFound):
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "run not found", nil)
		default:
			replyError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	replyJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyError(w, http.StatusInternalServerError, "world model not configured")
		return
	}
	ownerID := OwnerIDFromCtx(r.Context())
	if ownerID == "" && s.cfg.DB != nil {
		if resolved, err := store.GetOwnerID(r.Context(), s.cfg.DB); err == nil {
			ownerID = resolved
		}
	}
	if ownerID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "owner id unavailable", nil)
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	fetchLimit := limit
	if hasArtifactFilters(r) {
		if fetchLimit < 200 {
			fetchLimit = 200
		}
	}
	list, err := s.cfg.WorldModel.ListArtifactsForOwner(r.Context(), ownerID, fetchLimit)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	list = filterArtifacts(list, r, s.cfg.DB)
	if len(list) > limit {
		list = list[:limit]
	}
	items := make([]map[string]any, 0, len(list))
	for i := range list {
		items = append(items, artifactListItemMap(r.Context(), s.cfg.DB, list[i]))
	}
	replyJSON(w, http.StatusOK, map[string]any{"items": items})
}

func hasArtifactFilters(r *http.Request) bool {
	for _, key := range []string{"q", "type", "subtype", "status", "lifecycle_state", "project_id", "source_run_id", "source_conversation_id"} {
		if strings.TrimSpace(r.URL.Query().Get(key)) != "" {
			return true
		}
	}
	return false
}

func filterArtifacts(list []schema.Artifact, r *http.Request, db *sql.DB) []schema.Artifact {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	typeFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	subtypeFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("subtype")))
	statusFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	lifecycleFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lifecycle_state")))
	projectIDFilter := strings.TrimSpace(r.URL.Query().Get("project_id"))
	sourceRunFilter := strings.TrimSpace(r.URL.Query().Get("source_run_id"))
	sourceConversationFilter := strings.TrimSpace(r.URL.Query().Get("source_conversation_id"))

	if q == "" && typeFilter == "" && subtypeFilter == "" && statusFilter == "" && lifecycleFilter == "" && projectIDFilter == "" && sourceRunFilter == "" && sourceConversationFilter == "" {
		return list
	}

	allowedConversationArtifacts := map[string]struct{}(nil)
	if sourceConversationFilter != "" && db != nil {
		ids, err := store.ListArtifactIDsBySource(r.Context(), db, "conversation", sourceConversationFilter, 200)
		if err == nil {
			allowedConversationArtifacts = make(map[string]struct{}, len(ids))
			for _, id := range ids {
				allowedConversationArtifacts[id] = struct{}{}
			}
		}
	}

	out := make([]schema.Artifact, 0, len(list))
	for _, item := range list {
		if q != "" {
			haystack := strings.ToLower(strings.Join([]string{
				item.CanonicalTitle,
				item.DisplayTitle,
				item.Description,
				item.Location,
			}, "\n"))
			if !strings.Contains(haystack, q) {
				continue
			}
		}
		if typeFilter != "" && strings.ToLower(string(item.Type)) != typeFilter {
			continue
		}
		if subtypeFilter != "" && strings.ToLower(item.Subtype) != subtypeFilter {
			continue
		}
		if statusFilter != "" && strings.ToLower(item.Status) != statusFilter {
			continue
		}
		if lifecycleFilter != "" && strings.ToLower(string(item.LifecycleState)) != lifecycleFilter {
			continue
		}
		if projectIDFilter != "" && item.ProjectID != projectIDFilter {
			continue
		}
		if sourceRunFilter != "" && item.SourceRunID != sourceRunFilter {
			continue
		}
		if sourceConversationFilter != "" {
			if len(allowedConversationArtifacts) == 0 {
				continue
			}
			if _, ok := allowedConversationArtifacts[item.ID]; !ok {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func (s *Server) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now().UTC()
	if s.cfg.WorldModel == nil {
		replyError(w, http.StatusInternalServerError, "world model not configured")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "artifact id required", nil)
		return
	}
	artifact, err := s.cfg.WorldModel.GetArtifact(r.Context(), id)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if artifact == nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "Artifact not found", nil)
		return
	}
	versions, err := s.cfg.WorldModel.ListArtifactVersions(r.Context(), id, 20)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := artifactToMap(artifact)
	versionMaps := make([]map[string]any, 0, len(versions))
	for _, v := range versions {
		versionMaps = append(versionMaps, artifactVersionToMap(v))
	}
	resp["versions"] = versionMaps
	attachArtifactRecords(r.Context(), s.cfg.DB, artifact.ID, resp)
	if artifact.SourceExecutionID != "" && s.cfg.DB != nil {
		if eo, err := store.GetExecutionOutcomeByAttemptID(r.Context(), s.cfg.DB, artifact.SourceExecutionID); err == nil && eo != nil {
			resp["history"] = runOutcomeToMap(eo)
		}
	}
	s.emitArtifactWorkspaceEvent(r.Context(), schema.FactArtifactWorkspaceLoaded, artifact, nil, "loaded", "", "", startedAt)
	replyJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetArtifactContent(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now().UTC()
	if s.cfg.WorldModel == nil {
		replyError(w, http.StatusInternalServerError, "world model not configured")
		return
	}
	if s.cfg.ArtifactService == nil {
		replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "artifact content storage not configured", nil)
		return
	}
	artifact, version, ok := s.resolveArtifactVersionForRead(w, r)
	if !ok {
		return
	}
	content, err := s.cfg.ArtifactService.ReadVersionContent(r.Context(), version.ID)
	if err != nil {
		s.emitArtifactWorkspaceEvent(r.Context(), schema.FactArtifactWorkspaceLoadFailed, artifact, &version, "failed", schema.FailureClassStorageFailure, "read_version_content", startedAt)
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.emitArtifactWorkspaceEvent(r.Context(), schema.FactArtifactWorkspaceLoaded, artifact, &version, "loaded", "", "", startedAt)
	replyJSON(w, http.StatusOK, map[string]any{
		"artifact_id": artifact.ID,
		"version_id":  version.ID,
		"content":     content,
		"content_ref": contentRefToMap(&version.ContentRef),
	})
}

func (s *Server) handleGetArtifactDiff(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now().UTC()
	if s.cfg.WorldModel == nil {
		replyError(w, http.StatusInternalServerError, "world model not configured")
		return
	}
	if s.cfg.ArtifactService == nil {
		replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "artifact content storage not configured", nil)
		return
	}
	artifact, version, ok := s.resolveArtifactVersionForRead(w, r)
	if !ok {
		return
	}
	diff, err := s.cfg.ArtifactService.ReadVersionDiff(r.Context(), version.ID)
	if err != nil {
		if strings.Contains(err.Error(), "diff not available") {
			s.emitArtifactWorkspaceEvent(r.Context(), schema.FactArtifactWorkspaceLoadFailed, artifact, &version, "failed", schema.FailureClassRendererMissing, "diff_not_available", startedAt)
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "Artifact diff not found", nil)
			return
		}
		s.emitArtifactWorkspaceEvent(r.Context(), schema.FactArtifactWorkspaceLoadFailed, artifact, &version, "failed", schema.FailureClassStorageFailure, "read_version_diff", startedAt)
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.emitArtifactWorkspaceEvent(r.Context(), schema.FactArtifactWorkspaceLoaded, artifact, &version, "loaded", "", "", startedAt)
	replyJSON(w, http.StatusOK, map[string]any{
		"artifact_id": artifact.ID,
		"version_id":  version.ID,
		"diff":        decodeFlexibleJSON(diff),
		"diff_ref":    contentRefToMap(version.DiffRef),
	})
}

func (s *Server) resolveArtifactVersionForRead(w http.ResponseWriter, r *http.Request) (*schema.Artifact, schema.ArtifactVersion, bool) {
	id := r.PathValue("id")
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "artifact id required", nil)
		return nil, schema.ArtifactVersion{}, false
	}
	artifact, err := s.cfg.WorldModel.GetArtifact(r.Context(), id)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return nil, schema.ArtifactVersion{}, false
	}
	if artifact == nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "Artifact not found", nil)
		return nil, schema.ArtifactVersion{}, false
	}
	versionID := strings.TrimSpace(r.PathValue("versionID"))
	if versionID == "" {
		versionID = artifact.CurrentVersionID
	}
	if versionID == "" {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "Artifact version not found", nil)
		return nil, schema.ArtifactVersion{}, false
	}
	version, err := store.GetArtifactVersion(r.Context(), s.cfg.DB, versionID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return nil, schema.ArtifactVersion{}, false
	}
	if version.ArtifactID == "" || version.ArtifactID != artifact.ID {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "Artifact version not found", nil)
		return nil, schema.ArtifactVersion{}, false
	}
	return artifact, version, true
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusInternalServerError, "database not configured")
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	afterSeq := int64(0)
	if c := r.URL.Query().Get("cursor"); c != "" {
		if n, err := strconv.ParseInt(c, 10, 64); err == nil && n >= 0 {
			afterSeq = n
		}
	}
	var since *time.Time
	if t := r.URL.Query().Get("since"); t != "" {
		parsed, err := time.Parse(time.RFC3339, t)
		if err == nil {
			since = &parsed
		}
	}
	events, nextSeq, err := store.ListEvents(r.Context(), s.cfg.DB, afterSeq, since, limit)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]ActivityEntry, 0, len(events))
	for _, ev := range events {
		entry, ok := MapEventToActivity(ev)
		if ok {
			items = append(items, entry)
		}
	}
	resp := map[string]any{"items": items}
	if nextSeq > 0 {
		resp["next_cursor"] = nextSeq
	}
	replyJSON(w, http.StatusOK, resp)
}

func skillEntryToMap(e skill.SkillEntry) map[string]any {
	m := map[string]any{
		"id":                 e.Skill.ID,
		"name":               e.Skill.Name,
		"description":        e.Skill.Description,
		"availability_state": "available",
		"activatable":        e.Activatable,
	}
	if !e.Activatable && len(e.ReasonsUnbound) > 0 {
		m["reasons_unbound"] = e.ReasonsUnbound
		m["availability_state"] = "installed_not_activatable"
	}
	if e.Spec != nil {
		m["version"] = e.Spec.Semver
		m["approval_requirement"] = e.Spec.Effects.RequiresConfirmation
		m["risk_tier"] = e.Spec.Effects.RiskTier
		m["side_effects"] = e.Spec.Effects.SideEffects
		if len(e.Spec.Capability.Requires) > 0 {
			m["dependencies"] = e.Spec.Capability.Requires
		}
		m["trust_tier"] = e.Spec.Governance.TrustTier
	}
	if e.Skill.ID == "" {
		m["id"] = e.Skill.Name
	}
	return m
}

func decodeFlexibleJSON(raw string) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var out any
	if err := json.Unmarshal([]byte(trimmed), &out); err == nil {
		return out
	}
	return raw
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if s.cfg.SkillRegistry == nil {
		replyJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	list := s.cfg.SkillRegistry.List()
	items := make([]map[string]any, 0, len(list))
	for _, e := range list {
		items = append(items, skillEntryToMap(e))
	}
	replyJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "skill id required", nil)
		return
	}
	entry, ok := s.lookupSkillEntry(id)
	if !ok || entry == nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "skill not found", nil)
		return
	}
	replyJSON(w, http.StatusOK, skillEntryToMap(*entry))
}

func (s *Server) lookupSkillEntry(id string) (*skill.SkillEntry, bool) {
	id = strings.TrimSpace(id)
	if id == "" || s.cfg.SkillRegistry == nil {
		return nil, false
	}
	if entry, ok := s.cfg.SkillRegistry.Lookup(id); ok && entry != nil {
		return entry, true
	}
	for _, e := range s.cfg.SkillRegistry.List() {
		if e.Skill.ID == id || (e.Spec != nil && e.Spec.SkillID == id) {
			e2 := e
			return &e2, true
		}
	}
	// Coder-facing skill aliases (navi-coder.*) resolve to the legacy skill that
	// holds the implementation. Forward-compat for external/Coder callers; legacy
	// and unrelated ids (e.g. navi.coder.repo) are unchanged by CanonicalSkillID.
	if canonical := coderalias.CanonicalSkillID(id); canonical != id {
		for _, e := range s.cfg.SkillRegistry.List() {
			if e.Skill.ID == canonical || (e.Spec != nil && e.Spec.SkillID == canonical) {
				e2 := e
				return &e2, true
			}
		}
	}
	return nil, false
}

func (s *Server) handleReloadSkills(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi != nil {
		if err := s.cfg.Navi.ReloadSkills(); err != nil {
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", "failed to reload skills: "+err.Error(), nil)
			return
		}
		replyJSON(w, http.StatusOK, map[string]any{"status": "ok", "registry_refreshed": true})
		return
	}
	if s.cfg.SkillRegistry == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "skill registry not configured", nil)
		return
	}
	if err := s.cfg.SkillRegistry.Load(); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", "failed to reload skills: "+err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleReloadPrompts(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "navi not configured", nil)
		return
	}
	if err := s.cfg.Navi.ReloadPrompts(); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", "failed to reload prompts: "+err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleValidateSkill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "skill id required", nil)
		return
	}
	if s.cfg.SkillRegistry == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "skill registry not configured", nil)
		return
	}
	entry, ok := s.lookupSkillEntry(id)
	if !ok || entry == nil || entry.Spec == nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "skill not found or not OSS-27", nil)
		return
	}
	// Re-run spec validation to surface any errors after on-disk edits.
	if err := skill.ValidateSpec(entry.Spec); err != nil {
		replyJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
			"skill": skillEntryToMap(*entry),
		})
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"skill": skillEntryToMap(*entry),
	})
}

type skillInvokeRequest struct {
	Arguments map[string]any `json:"arguments"`
}

func (s *Server) handleInvokeSkillInterface(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	ifaceName := strings.TrimSpace(r.PathValue("interface"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "skill id required", nil)
		return
	}
	if ifaceName == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "skill interface required", nil)
		return
	}
	if s.cfg.SkillRegistry == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "skill registry not configured", nil)
		return
	}
	var req skillInvokeRequest
	if r.Body != nil {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "failed to read request body", nil)
			return
		}
		if len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &req); err != nil {
				replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "request body must be JSON with an arguments object", nil)
				return
			}
		}
	}
	if req.Arguments == nil {
		req.Arguments = map[string]any{}
	}
	entry, ok := s.lookupSkillEntry(id)
	if !ok || entry == nil || entry.Spec == nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "skill not found or not OSS-27", nil)
		return
	}
	var iface *skill.Interface
	for i := range entry.Spec.Interfaces {
		if entry.Spec.Interfaces[i].Name == ifaceName {
			iface = &entry.Spec.Interfaces[i]
			break
		}
	}
	if iface == nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "skill interface not found", nil)
		return
	}
	execCtx := skill.ExecutionContext{
		WorkspaceDir: s.cfg.WorkspaceDir,
		SandboxProfileResolver: func(ctx context.Context, profileID string) (schema.SandboxProfile, error) {
			if s.cfg.DB == nil {
				return schema.SandboxProfile{}, errors.New("database not configured")
			}
			return store.GetSandboxProfile(ctx, s.cfg.DB, profileID)
		},
	}
	raw, err := skill.Execute(skill.WithExecutionContext(r.Context(), execCtx), entry, iface, req.Arguments)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL"
		if errors.Is(err, skill.ErrNotExecutable) {
			status = http.StatusNotImplemented
			code = "NOT_EXECUTABLE"
		}
		replyErrorAPI(w, status, code, err.Error(), nil)
		return
	}
	var result skill.SkillExecutionResult
	if err := json.Unmarshal([]byte(raw), &result); err == nil && result.Status != "" {
		replyJSON(w, http.StatusOK, result)
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"status": "success", "payload": decodeFlexibleJSON(raw)})
}

func (s *Server) handleDebugEvents(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusInternalServerError, "database not configured")
		return
	}
	afterSeq := int64(0)
	if a := r.URL.Query().Get("after_seq"); a != "" {
		if n, err := strconv.ParseInt(a, 10, 64); err == nil && n >= 0 {
			afterSeq = n
		}
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	events, nextSeq, err := store.ListEvents(r.Context(), s.cfg.DB, afterSeq, nil, limit)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		items = append(items, map[string]any{
			"id":             ev.ID,
			"type":           string(ev.Type),
			"kind":           string(ev.Kind),
			"correlation_id": ev.CorrelationID,
			"causal_parent":  ev.CausalParent,
			"source_agent":   string(ev.SourceAgent),
			"target_agent":   string(ev.TargetAgent),
			"timestamp":      ev.Timestamp.UTC().Format(time.RFC3339),
			"payload":        ev.Payload,
			"schema_version": ev.SchemaVersion,
			"seq":            ev.Seq,
			"run_id":         ev.RunID,
			"visibility":     ev.Visibility,
		})
	}
	resp := map[string]any{"items": items}
	if nextSeq > 0 {
		resp["next_cursor"] = nextSeq
	}
	replyJSON(w, http.StatusOK, resp)
}

// handleRuntimeMetrics exposes a snapshot of the streaming runtime metrics.
func (s *Server) handleRuntimeMetrics(w http.ResponseWriter, r *http.Request) {
	snapshot := naviruntime.DefaultMetrics().Snapshot()
	replyJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleListConnectors(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Registry == nil {
		replyJSON(w, http.StatusOK, []map[string]any{})
		return
	}
	list := s.cfg.Registry.List()
	items := make([]map[string]any, 0, len(list))
	for _, c := range list {
		items = append(items, map[string]any{
			"name":         c.Name,
			"state":        connectorState(c.Status),
			"status":       c.Status,
			"connected_at": c.ConnectedAt,
			"category":     c.Category,
		})
	}
	replyJSON(w, http.StatusOK, items)
}

func (s *Server) handleListConnectorFactories(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Registry == nil {
		replyJSON(w, http.StatusOK, []string{})
		return
	}
	replyJSON(w, http.StatusOK, s.cfg.Registry.FactoryNames())
}

func (s *Server) handleSetupSchema(w http.ResponseWriter, r *http.Request) {
	// Prefer the live callback so dynamically registered connectors are visible.
	if s.cfg.ConnectorSetupDescriptors != nil {
		descriptors := s.cfg.ConnectorSetupDescriptors()
		if descriptors == nil {
			descriptors = []connectors.SetupDescriptor{}
		}
		replyJSON(w, http.StatusOK, descriptors)
		return
	}
	if s.cfg.SetupSchema == nil {
		replyJSON(w, http.StatusOK, []connectors.SetupDescriptor{})
		return
	}
	replyJSON(w, http.StatusOK, s.cfg.SetupSchema)
}

// connectorDetailResponse is the response for GET /api/connectors/{name}.
type connectorDetailResponse struct {
	Name             string    `json:"name"`
	Status           string    `json:"status"`
	ConnectedAt      time.Time `json:"connected_at"`
	WebhookPath      string    `json:"webhook_path,omitempty"`
	Capabilities     []string  `json:"capabilities,omitempty"`
	MaxMessageLength int       `json:"max_message_length,omitempty"`
}

// connectorInstanceV2Response is the v2-style connector instance view, aligned
// with the canonical connectors spec (driver/instance identity + metadata +
// declared capabilities).
type connectorInstanceV2Response struct {
	InstanceID   string                            `json:"instance_id"`
	DriverID     string                            `json:"driver_id"`
	Status       string                            `json:"status,omitempty"`
	HealthState  string                            `json:"health_state,omitempty"`
	AuthState    string                            `json:"auth_state,omitempty"`
	Labels       map[string]string                 `json:"labels,omitempty"`
	Capabilities []connectors.CapabilityDescriptor `json:"capabilities,omitempty"`
}

func (s *Server) handleGetConnector(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		replyError(w, http.StatusBadRequest, "name is required")
		return
	}
	if s.cfg.Registry == nil {
		http.NotFound(w, r)
		return
	}
	info, ok := s.cfg.Registry.GetInfo(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	resp := connectorDetailResponse{
		Name:        info.Name,
		Status:      info.Status,
		ConnectedAt: info.ConnectedAt,
	}
	conn := s.cfg.Registry.Get(name)
	if conn != nil {
		if wh, ok := conn.(pkgconn.WebhookHandler); ok {
			resp.WebhookPath = wh.WebhookPath()
		}
		var caps []string
		if _, ok := conn.(pkgconn.WebhookHandler); ok {
			caps = append(caps, "webhook")
		}
		if _, ok := conn.(pkgconn.HealthChecker); ok {
			caps = append(caps, "health_check")
		}
		if _, ok := conn.(pkgconn.MediaSender); ok {
			caps = append(caps, "media")
		}
		if _, ok := conn.(pkgconn.TypingCapable); ok {
			caps = append(caps, "typing")
		}
		if _, ok := conn.(pkgconn.MessageEditor); ok {
			caps = append(caps, "message_edit")
		}
		if _, ok := conn.(pkgconn.ReactionCapable); ok {
			caps = append(caps, "reaction")
		}
		if _, ok := conn.(pkgconn.PlaceholderCapable); ok {
			caps = append(caps, "placeholder")
		}
		if len(caps) > 0 {
			resp.Capabilities = caps
		}
		if ml, ok := conn.(pkgconn.MessageLengthProvider); ok {
			if n := ml.MaxMessageLength(); n > 0 {
				resp.MaxMessageLength = n
			}
		}
	}
	replyJSON(w, http.StatusOK, resp)
}

// handleListConnectorInstancesV2 exposes the v2 connector runtime model:
// drivers, instances, metadata, and capability declarations.
func (s *Server) handleListConnectorInstancesV2(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Registry == nil {
		replyJSON(w, http.StatusOK, []connectorInstanceV2Response{})
		return
	}

	drivers := s.cfg.Registry.Drivers()
	driverByID := make(map[string]connectors.ConnectorDriver, len(drivers))
	for _, d := range drivers {
		driverByID[d.DriverID()] = d
	}

	instances := s.cfg.Registry.InstancesV2()
	out := make([]connectorInstanceV2Response, 0, len(instances))

	for _, inst := range instances {
		id := inst.InstanceID()
		meta, _ := s.cfg.Registry.InstanceMetadata(id)
		driver := driverByID[inst.DriverID()]

		resp := connectorInstanceV2Response{
			InstanceID:  id,
			DriverID:    inst.DriverID(),
			Status:      meta.Status,
			HealthState: meta.HealthState,
			AuthState:   meta.AuthState,
			Labels:      meta.Labels,
		}
		if driver != nil {
			resp.Capabilities = driver.Capabilities()
		}
		out = append(out, resp)
	}

	replyJSON(w, http.StatusOK, out)
}

func (s *Server) handleRegisterConnector(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string `json:"name"`
		CallbackURL      string `json:"callback_url"`
		MaxMessageLength int    `json:"max_message_length"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		replyError(w, http.StatusBadRequest, "name is required")
		return
	}
	if s.cfg.Registry != nil {
		s.cfg.Registry.Register(req.Name)
	}
	if req.CallbackURL != "" && s.cfg.Manager != nil {
		bridge := connectors.NewGatewayBridgeConnector(connectors.BridgeConfig{
			Name:             req.Name,
			CallbackURL:      req.CallbackURL,
			MaxMessageLength: req.MaxMessageLength,
		})
		s.cfg.Registry.RegisterInstance(req.Name, bridge)
		s.cfg.Manager.StartOne(r.Context(), req.Name)
	}
	replyJSON(w, http.StatusOK, map[string]string{"result": "ok"})
}

func (s *Server) handleDeregisterConnector(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		replyError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !HasScope(r.Context(), "execute") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "execute scope required", nil)
		return
	}
	var conn pkgconn.Connector
	var found bool
	if s.cfg.Registry != nil {
		if _, found = s.cfg.Registry.GetInfo(name); found {
			conn = s.cfg.Registry.Get(name)
		}
	}
	if !found && conn == nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "connector not found", nil)
		return
	}

	if s.cfg.Manager != nil {
		if err := s.cfg.Manager.StopOne(r.Context(), name, 10*time.Second); err != nil && err != connectors.ErrUnknownConnector {
			replyError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else if conn != nil {
		if err := conn.Stop(r.Context()); err != nil {
			replyError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if s.cfg.Registry != nil {
		s.cfg.Registry.Remove(name)
	}
	replyJSON(w, http.StatusOK, map[string]string{"result": "ok", "name": name})
}

func (s *Server) handleConnectorWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" || s.cfg.Registry == nil {
		http.NotFound(w, r)
		return
	}
	conn := s.cfg.Registry.Get(name)
	if conn == nil {
		http.NotFound(w, r)
		return
	}
	wh, ok := conn.(pkgconn.WebhookHandler)
	if !ok || wh.WebhookPath() != r.URL.Path {
		http.NotFound(w, r)
		return
	}
	wh.HandleWebhook(w, r)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	replyJSON(w, http.StatusOK, map[string]string{
		"version":        s.cfg.Version,
		"schema_version": schema.SchemaVersion,
	})
}

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		replyError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	manifests := []plugin.Manifest{}
	if s.cfg.PluginManifests != nil {
		manifests = s.cfg.PluginManifests()
	}
	replyJSON(w, http.StatusOK, map[string]any{"plugins": manifests})
}

func (s *Server) handleTokenAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		replyError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.cfg.SharedSecret == "" {
		replyError(w, http.StatusServiceUnavailable, "token auth not configured")
		return
	}
	var req struct {
		OwnerID string `json:"owner_id"`
		Secret  string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Secret != s.cfg.SharedSecret {
		replyError(w, http.StatusUnauthorized, "invalid secret")
		return
	}
	replyJSON(w, http.StatusOK, map[string]string{"token": s.cfg.SharedSecret})
}

func (s *Server) handleConnectorHealth(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Manager == nil {
		replyJSON(w, http.StatusOK, []map[string]any{})
		return
	}
	health := s.cfg.Manager.Health()
	items := make([]map[string]any, 0, len(health))
	for _, h := range health {
		items = append(items, map[string]any{
			"name":               h.Name,
			"state":              connectorState(h.Status),
			"status":             h.Status,
			"last_send_at":       h.LastSendAt,
			"last_error_at":      h.LastErrorAt,
			"send_count":         h.SendCount,
			"error_count":        h.ErrorCount,
			"consecutive_errors": h.ConsecutiveErrs,
			"queue_depth":        h.QueueDepth,
		})
	}
	replyJSON(w, http.StatusOK, items)
}

func (s *Server) handleConnectorDiagnostics(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Manager == nil {
		replyJSON(w, http.StatusOK, []connectors.Diagnostic{})
		return
	}
	replyJSON(w, http.StatusOK, s.cfg.Manager.Diag.List())
}

func (s *Server) handleErrors(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusInternalServerError, "database not configured")
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			replyError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = n
	}
	items, err := store.ListErrorRecords(r.Context(), s.cfg.DB, store.ListErrorsFilter{
		Severity:  strings.TrimSpace(r.URL.Query().Get("severity")),
		Component: strings.TrimSpace(r.URL.Query().Get("component")),
		ChatID:    strings.TrimSpace(r.URL.Query().Get("chat_id")),
		ErrorType: strings.TrimSpace(r.URL.Query().Get("error_type")),
		Limit:     limit,
	})
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleErrorSummary(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusInternalServerError, "database not configured")
		return
	}
	window := strings.TrimSpace(r.URL.Query().Get("window"))
	if window == "" {
		window = "24h"
	}
	dur, err := time.ParseDuration(window)
	if err != nil {
		replyError(w, http.StatusBadRequest, "invalid window")
		return
	}
	rows, err := store.ErrorSummary(r.Context(), s.cfg.DB, time.Now().UTC().Add(-dur))
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"window": window,
		"items":  rows,
	})
}

// --- NAVI chat handlers ---

func (s *Server) handleNaviCreateChat(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI not enabled")
		return
	}
	dataDir := s.cfg.DataDir
	if dataDir == "" {
		dataDir = "."
	}
	status, _ := onboarding.DeriveFirstRunStatus(r.Context(), s.cfg.DB, dataDir)
	if status.State != onboarding.FirstRunComplete {
		replyJSON(w, http.StatusConflict, map[string]any{
			"error":           "complete onboarding before starting a NAVI chat",
			"first_run_state": status.State,
			"onboarding_url":  "/onboarding",
		})
		return
	}
	var projectID string
	var req struct {
		ProjectID      string `json:"project_id"`
		InitialMessage string `json:"initial_message,omitempty"`
		Content        string `json:"content,omitempty"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil && err != io.EOF {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectID = strings.TrimSpace(req.ProjectID)
	if projectID != "" {
		project, ok := s.requireProjectContext(w, r, projectID)
		if !ok {
			return
		}
		projectID = project.ID
	}
	initialMessage := firstNonEmptyGateway(req.InitialMessage, req.Content)
	title := s.generateInitialChatTitle(r.Context(), initialMessage)
	chatID, err := s.cfg.Navi.CreateChatWithTitle(r.Context(), navi.ExperienceModeStandard, projectID, title)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.setActiveProjectContext(r.Context(), projectID); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]string{"id": chatID, "chat_id": chatID, "title": title}
	if projectID != "" {
		resp["project_id"] = projectID
	}
	replyJSON(w, http.StatusCreated, resp)
}

func (s *Server) generateInitialChatTitle(ctx context.Context, initialMessage string) string {
	initialMessage = strings.TrimSpace(initialMessage)
	if initialMessage == "" {
		return "New Chat"
	}
	input := ai.TitleGeneratorInput{
		Messages: []llm.Message{{Role: "user", Content: initialMessage}},
		Mode:     ai.ModeCreate,
	}
	if s.cfg.LLM != nil {
		if providerService, ok := s.cfg.LLM.(chatProviderService); ok {
			if active, err := s.cfg.LLM.GetActive(ctx); err == nil {
				if resp, err := ai.GenerateConversationTitle(ctx, providerService.ChatProvider(), active.Model, input); err == nil {
					if title := strings.TrimSpace(resp.Title); title != "" {
						return title
					}
				}
			}
		}
	}
	resp, _ := ai.GenerateConversationTitle(ctx, nil, "", input)
	if title := strings.TrimSpace(resp.Title); title != "" {
		return title
	}
	return "New Conversation"
}

func (s *Server) handleNaviListChats(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI not enabled")
		return
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	var (
		chats []navi.Chat
		err   error
	)
	if projectID != "" {
		project, ok := s.requireProjectContext(w, r, projectID)
		if !ok {
			return
		}
		chats, err = s.cfg.Navi.RecentProjectChats(r.Context(), project.ID, 50)
	} else {
		chats, err = s.cfg.Navi.RecentChats(r.Context(), 50)
	}
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(chats) > 0 {
		publicChats := chats[:0]
		for _, chat := range chats {
			hidden, hiddenErr := chatHiddenFromPublicAPI(r.Context(), s.cfg.DB, string(chat.ID))
			if hiddenErr != nil {
				replyError(w, http.StatusInternalServerError, hiddenErr.Error())
				return
			}
			if !hidden {
				publicChats = append(publicChats, chat)
			}
		}
		chats = publicChats
	}
	if chats == nil {
		chats = []navi.Chat{}
	}
	replyJSON(w, http.StatusOK, chats)
}

func (s *Server) handleNaviGetChat(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI not enabled")
		return
	}
	id := r.PathValue("id")
	hidden, err := chatHiddenFromPublicAPI(r.Context(), s.cfg.DB, id)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hidden {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "chat not found", nil)
		return
	}
	chat, err := s.cfg.Navi.GetChat(r.Context(), id)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, chat)
}

func (s *Server) handleNaviGetChatRuntimeSummary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	hidden, err := chatHiddenFromPublicAPI(r.Context(), s.cfg.DB, id)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hidden {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "chat not found", nil)
		return
	}
	summary, err := liveChatRuntimeSummary(r.Context(), s.cfg.DB, id)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, summary)
}

func (s *Server) handleNaviSendMessage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI not enabled")
		return
	}
	id := r.PathValue("id")
	traceID := strings.TrimSpace(r.Header.Get("X-Trace-Id"))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
	if requestID == "" {
		requestID = uuid.NewString()
	}
	traceCtx := naviruntime.ProgressTrace{
		TraceID:          traceID,
		RequestID:        requestID,
		RuntimeSessionID: id,
		Route:            r.URL.Path,
		Method:           r.Method,
	}
	ctx := naviruntime.WithProgressTrace(r.Context(), traceCtx)
	r = r.WithContext(ctx)
	tracer := naviruntime.NewProgressTracer("gateway.navi_send_message", traceCtx)
	handlerDone := tracer.StageStart(ctx, "http.handler")
	defer handlerDone(nil)
	tracer.Mark("gateway.handler.enter")
	slog.Info("gateway navi send message entry",
		"request_id", requestID,
		"chat_id", id,
		"path", r.URL.Path,
		"method", r.Method,
	)
	var req struct {
		Content          string `json:"content"`
		SourceChannel    string `json:"source_channel,omitempty"`
		SourceMessageRef string `json:"source_message_ref,omitempty"`
		IdempotencyKey   string `json:"idempotency_key,omitempty"`
		SystemContext    string `json:"system_context,omitempty"`
	}
	decodeDone := tracer.StageStart(ctx, "gateway.decode_body")
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		decodeDone(err)
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	decodeDone(nil)
	slog.Info("gateway navi send message decoded body",
		"request_id", requestID,
		"chat_id", id,
		"source_channel", req.SourceChannel,
		"source_message_ref", req.SourceMessageRef,
	)

	if req.SystemContext != "" && s.cfg.DB != nil {
		_ = store.SaveFact(ctx, s.cfg.DB, store.Fact{
			ID:       "app_ctx_" + id,
			Scope:    "directive",
			ScopeID:  id,
			Category: "technical_context",
			Key:      "system_context",
			Value:    req.SystemContext,
			Source:   "app",
		})
	}

	ingressCtx, cancelIngress := newNaviMessageIngressContext(ctx)
	defer cancelIngress()

	hidden, err := chatHiddenFromPublicAPI(ingressCtx, s.cfg.DB, id)
	if err != nil {
		if replyIngressTimeout(w, err) {
			return
		}
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hidden {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "chat not found", nil)
		return
	}

	// Ceremony message intercept: handle free-text and derailment during in-progress ceremony.
	if s.tryCeremonyMessageIntercept(ingressCtx, w, id, req.Content) {
		return
	}

	act := governor.ActionDescriptor{
		CommandType: schema.CommandTypeSend,
		Domain:      governor.AutonomyDomainMessaging,
		ActorKind:   "gateway",
		ChatID:      id,
		Tags: map[string]string{
			"source_channel":     req.SourceChannel,
			"source_message_ref": req.SourceMessageRef,
		},
	}
	governanceDone := tracer.StageStart(ctx, "gateway.governance_validation")
	validation := s.cfg.Navi.ValidateAction(ingressCtx, act)
	governanceDone(nil)
	if errors.Is(ingressCtx.Err(), context.DeadlineExceeded) {
		replyIngressTimeout(w, ingressCtx.Err())
		return
	}
	switch validation.Outcome {
	case governor.ValidationRejected:
		replyErrorStructured(w, http.StatusForbidden, "governance rejected: "+validation.Reason, string(schema.DegradationBlocking), "")
		return
	case governor.ValidationRequiresConfirmation, governor.ValidationModified:
		replyErrorStructured(w, http.StatusForbidden, "messaging requires confirmation: "+validation.Reason, "requires_confirmation", "")
		return
	}
	if s.cfg.RefWorker != nil {
		clearDone := tracer.StageStart(ctx, "gateway.clear_interruption_guard")
		s.cfg.RefWorker.ClearInterruptionGuard()
		clearDone(nil)
	}
	intakeDone := tracer.StageStart(ctx, "gateway.intake")
	slog.Info("gateway runtime intake submit_message",
		"request_id", requestID,
		"chat_id", id,
		"source_channel", req.SourceChannel,
		"source_message_ref", req.SourceMessageRef,
	)
	sendMessage := s.sendMessageInput
	if sendMessage == nil {
		sendMessage = s.cfg.Navi.SendMessageInput
	}
	sendDone := tracer.StageStart(ctx, "gateway.send_message_input")
	item, err := sendMessage(ingressCtx, id, naviruntime.MessageInput{
		Content:          req.Content,
		SourceChannel:    req.SourceChannel,
		SourceMessageRef: req.SourceMessageRef,
		IdempotencyKey:   req.IdempotencyKey,
	})
	sendDone(err)
	intakeDone(err)
	if err != nil {
		if replyIngressTimeout(w, err) || replyIngressTimeout(w, ingressCtx.Err()) {
			return
		}
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.triggerConnectorInbound(r.Context(), req.SourceChannel, req.SourceMessageRef)
	resp := map[string]any{
		"status":        "queued",
		"inbox_item_id": itemID(item),
	}
	if item != nil {
		resp["queue_action"] = item.QueueAction
		resp["inbox_status"] = string(item.Status)
		if item.ClassifiedReason != "" {
			resp["classified_reason"] = item.ClassifiedReason
		}
		if item.MergedIntoID != "" {
			resp["merged_into_id"] = item.MergedIntoID
		}
	}
	if item != nil && item.Status == naviruntime.InboxStatusDeferred {
		preflightDone := tracer.StageStart(ctx, "gateway.paused_run_preflight")
		if paused, pausedErr := s.cfg.Navi.PausedRun(r.Context(), id); pausedErr == nil && paused != nil {
			if paused.RunID != "" {
				resp["run_id"] = paused.RunID
			}
			if paused.BlockedOnProposalID != "" {
				resp["blocked_on_proposal_id"] = paused.BlockedOnProposalID
			}
			if paused.PauseReason != "" {
				resp["pause_reason"] = paused.PauseReason
			}
			if paused.Status != "" {
				resp["run_status"] = string(paused.Status)
			}
		}
		preflightDone(nil)
	}
	responseDone := tracer.StageStart(ctx, "gateway.http_response_write")
	replyJSON(w, http.StatusCreated, resp)
	responseDone(nil)
}

func (s *Server) handleNaviUpdateChat(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI not enabled")
		return
	}
	id := r.PathValue("id")
	hidden, err := chatHiddenFromPublicAPI(r.Context(), s.cfg.DB, id)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hidden {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "chat not found", nil)
		return
	}

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if len(req) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	chatThread, err := s.cfg.Navi.GetChat(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "chat not found", nil)
			return
		}
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated := false

	if v, ok := req["title"]; ok {
		if title, ok := v.(string); ok {
			title = strings.TrimSpace(title)
			if title == "" {
				replyError(w, http.StatusBadRequest, "chat title cannot be empty")
				return
			}
			chatThread.Chat.Title = title
			updated = true
		}
	}

	if v, ok := req["project_id"]; ok {
		if v == nil {
			chatThread.Chat.ProjectID = nil
			updated = true
		} else if pid, ok := v.(string); ok {
			pid = strings.TrimSpace(pid)
			if pid == "" {
				chatThread.Chat.ProjectID = nil
			} else {
				pidPtr := navi.ID(pid)
				chatThread.Chat.ProjectID = &pidPtr
			}
			updated = true
		}
	}

	if updated {
		if err := s.cfg.Navi.UpdateChat(r.Context(), chatThread.Chat); err != nil {
			replyError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNaviArchiveChat(w http.ResponseWriter, r *http.Request) {
	s.handleNaviChatLifecycle(w, r, true)
}

func (s *Server) handleNaviDeleteChat(w http.ResponseWriter, r *http.Request) {
	s.handleNaviChatLifecycle(w, r, false)
}

func (s *Server) handleNaviChatLifecycle(w http.ResponseWriter, r *http.Request, archive bool) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI not enabled")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		replyError(w, http.StatusBadRequest, "chat id required")
		return
	}
	hidden, err := chatHiddenFromPublicAPI(r.Context(), s.cfg.DB, id)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hidden {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "chat not found", nil)
		return
	}

	if r.Method == http.MethodDelete {
		archive = false
	} else {
		var req struct {
			Archive bool `json:"archive"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Archive {
			archive = true
		}
	}

	if archive {
		if err := s.cfg.Navi.ArchiveChat(r.Context(), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				replyError(w, http.StatusNotFound, err.Error())
				return
			}
			replyError(w, http.StatusBadRequest, err.Error())
			return
		}
		replyJSON(w, http.StatusOK, map[string]any{
			"status":   "archived",
			"chat_id":  id,
			"archived": true,
		})
	} else {
		if err := s.cfg.Navi.DeleteChat(r.Context(), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				replyError(w, http.StatusNotFound, err.Error())
				return
			}
			replyError(w, http.StatusBadRequest, err.Error())
			return
		}
		replyJSON(w, http.StatusOK, map[string]any{
			"status":  "deleted",
			"chat_id": id,
			"deleted": true,
		})
	}
}

func (s *Server) handleListProposals(w http.ResponseWriter, r *http.Request) {
	_, _ = store.ExpireProposals(r.Context(), s.cfg.DB)
	list, err := store.ListPendingProposals(r.Context(), s.cfg.DB, 50)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []schema.Proposal{}
	}
	replyJSON(w, http.StatusOK, list)
}

func (s *Server) handleResolveProposal(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI not enabled")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		replyError(w, http.StatusBadRequest, "proposal id required")
		return
	}
	var req struct {
		Action            string `json:"action"` // "approve" or "decline"
		ResolutionType    string `json:"resolution_type"`
		ResolutionNote    string `json:"resolution_note"`
		SwitchWorkspaceID string `json:"switch_workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch req.Action {
	case "approve", "allow_once":
		if req.Action == "allow_once" {
			if err := s.cfg.Navi.ResolveBoundaryProposal(r.Context(), id, schema.ApprovalOutcomeAllowOnce, "", req.ResolutionNote); err != nil {
				replyError(w, http.StatusBadRequest, err.Error())
				return
			}
			replyJSON(w, http.StatusOK, map[string]string{"status": "approved", "approval_outcome": string(schema.ApprovalOutcomeAllowOnce)})
			return
		}
		resType := schema.ResolutionType(req.ResolutionType)
		if resType == "" {
			resType = schema.ResolutionTypeApprovedOnce
		}
		if err := s.cfg.Navi.ResolveProposal(r.Context(), id, schema.ProposalStatusApproved, resType, "owner", req.ResolutionNote); err != nil {
			replyError(w, http.StatusBadRequest, err.Error())
			return
		}
		replyJSON(w, http.StatusOK, map[string]string{"status": "approved"})
	case "decline", "deny":
		if req.Action == "deny" {
			if err := s.cfg.Navi.ResolveBoundaryProposal(r.Context(), id, schema.ApprovalOutcomeDenied, "", req.ResolutionNote); err != nil {
				replyError(w, http.StatusBadRequest, err.Error())
				return
			}
			replyJSON(w, http.StatusOK, map[string]string{"status": "declined", "approval_outcome": string(schema.ApprovalOutcomeDenied)})
			return
		}
		if err := s.cfg.Navi.ResolveProposal(r.Context(), id, schema.ProposalStatusDeclined, schema.ResolutionTypeDenied, "owner", req.ResolutionNote); err != nil {
			replyError(w, http.StatusInternalServerError, err.Error())
			return
		}
		replyJSON(w, http.StatusOK, map[string]string{"status": "declined"})
	case "always_allow":
		if err := s.cfg.Navi.ResolveBoundaryProposal(r.Context(), id, schema.ApprovalOutcomeAlwaysAllow, "", req.ResolutionNote); err != nil {
			replyError(w, http.StatusBadRequest, err.Error())
			return
		}
		replyJSON(w, http.StatusOK, map[string]string{"status": "approved", "approval_outcome": string(schema.ApprovalOutcomeAlwaysAllow)})
	case "switch_workspace":
		if err := s.cfg.Navi.ResolveBoundaryProposal(r.Context(), id, schema.ApprovalOutcomeSwitchedWorkspace, req.SwitchWorkspaceID, req.ResolutionNote); err != nil {
			replyError(w, http.StatusBadRequest, err.Error())
			return
		}
		replyJSON(w, http.StatusOK, map[string]string{"status": "approved", "approval_outcome": string(schema.ApprovalOutcomeSwitchedWorkspace)})
	default:
		replyError(w, http.StatusBadRequest, "action must be 'approve', 'decline', 'deny', 'allow_once', 'always_allow', or 'switch_workspace'")
	}
}
func (s *Server) handleListGaps(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		replyError(w, http.StatusInternalServerError, "database not configured")
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	gaps, err := store.ListOpenGaps(r.Context(), s.cfg.DB, limit)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if gaps == nil {
		gaps = []store.Gap{}
	}
	replyJSON(w, http.StatusOK, map[string]any{"items": gaps})
}

func (s *Server) handleBuildSkill(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI agent not configured")
		return
	}
	var req struct {
		GapID       string `json:"gap_id"`
		UserContext string `json:"user_context"`
		ChatID      string `json:"chat_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.GapID == "" {
		req.GapID = r.PathValue("id")
	}
	if req.GapID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "gap_id required", nil)
		return
	}

	result, err := s.cfg.Navi.BuildGap(r.Context(), req.GapID, req.UserContext, req.ChatID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusCreated, result)
}
