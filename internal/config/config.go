package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the full runtime configuration for Navi.
type Config struct {
	Gateway    GatewayConfig    `yaml:"gateway"`
	NATS       NATSConfig       `yaml:"nats"`
	SQLite     SQLiteConfig     `yaml:"sqlite"`
	Connectors ConnectorsConfig `yaml:"connectors"`
	LSP        LSPConfig        `yaml:"lsp"`
	LLM        LLMConfig        `yaml:"llm"`
	Navi       NaviConfig       `yaml:"navi"`
	Governor   GovernorConfig   `yaml:"governor"`
	Cron       CronConfig       `yaml:"cron"`
	Autonomy   AutonomyConfig   `yaml:"autonomy,omitempty"`
	Vault      VaultConfig      `yaml:"vault"`
}

// VaultConfig configures the Memory Vault (memory-projection-v1.md): the
// owner-facing Markdown projection of the World Model and the editable surface
// back into it. The Vault is local on disk; cloud sync is the owner's choice and
// lives outside this config (spec §12).
type VaultConfig struct {
	Enabled       bool   `yaml:"enabled"`        // master switch (default: true)
	Dir           string `yaml:"dir"`            // vault root (default: <workspace>/vault)
	Debounce      string `yaml:"debounce"`       // owner-edit debounce, e.g. "750ms"
	DriftInterval string `yaml:"drift_interval"` // periodic drift sweep cadence, e.g. "24h"
	PrivacyMode   string `yaml:"privacy_mode"`   // local|hybrid|cloud (CIP §9; default cloud)
}

// ConnectorsConfig holds configuration for all channel connectors (Telegram, Slack, etc.).
// Single source of truth for connector config; add new connector types here.
type ConnectorsConfig struct {
	Telegram TelegramConfig `yaml:"telegram"`
	Slack    SlackConfig    `yaml:"slack"`
}

// ConnectorInstanceConfig is a normalized, instance-centric view of connector
// configuration aligned with the v2 connectors spec. It is derived from the
// provider-specific fields above (TelegramConfig, SlackConfig, etc.) so that
// existing configs remain source-of-truth while the runtime can reason about
// connector instances in a provider-agnostic way.
type ConnectorInstanceConfig struct {
	InstanceID string            `json:"instance_id"`
	DriverID   string            `json:"driver_id"`
	Labels     map[string]string `json:"labels,omitempty"`
	// SecretRefs and additional provider-specific configuration remain in the
	// provider config structs; this view is intended for registry/manager use.
}

// Instances returns a best-effort list of connector instance configs derived
// from the current ConnectorsConfig. For now this covers the built-in
// Telegram and Slack connectors and treats their configured presence as a
// single logical instance keyed by the provider name.
func (c ConnectorsConfig) Instances() []ConnectorInstanceConfig {
	instances := make([]ConnectorInstanceConfig, 0, 2)
	if c.Telegram.BotToken != "" || len(c.Telegram.Accounts) > 0 {
		instances = append(instances, ConnectorInstanceConfig{
			InstanceID: "telegram",
			DriverID:   "telegram",
			Labels:     map[string]string{"kind": "builtin", "channel": "telegram"},
		})
	}
	if c.Slack.BotToken != "" {
		instances = append(instances, ConnectorInstanceConfig{
			InstanceID: "slack",
			DriverID:   "slack",
			Labels:     map[string]string{"kind": "builtin", "channel": "slack"},
		})
	}
	return instances
}

// GovernorConfig holds hard limits for the orchestrator governor (action budget, cost, duration).
type GovernorConfig struct {
	MaxActionBudget    int     `yaml:"max_action_budget"`   // max actions per run; 0 = use default
	MaxRetries         int     `yaml:"max_retries"`         // max retries before trip
	CostCeiling        float64 `yaml:"cost_ceiling"`        // USD ceiling
	AutonomousDuration string  `yaml:"autonomous_duration"` // e.g. "72h"; parsed when building governor
	MaxRepetitions     int     `yaml:"max_repetitions"`     // trip after N identical consecutive responses; <=0 disables
	// ExecutionOutcomeRetentionDays deletes execution_outcomes older than this many days; 0 = no cleanup.
	ExecutionOutcomeRetentionDays int `yaml:"execution_outcome_retention_days"`
	// EventRetentionDays deletes events older than this many days; 0 = no cleanup. Default: 30.
	EventRetentionDays int `yaml:"event_retention_days"`
	// EventRetentionByType overrides or disables retention for specific event types. 0 = keep indefinitely.
	EventRetentionByType map[string]int `yaml:"event_retention_by_type,omitempty"`
}

// GatewayConfig holds the gateway server configuration.
type GatewayConfig struct {
	Addr           string   `yaml:"addr"`
	SharedSecret   string   `yaml:"shared_secret"` // connector API key; presented as X-API-Key by Telegram/Slack
	OriginPatterns []string `yaml:"origin_patterns"`
	StaticDir      string   `yaml:"static_dir"`
}

// NATSConfig holds the NATS connection configuration.
type NATSConfig struct {
	URL string `yaml:"url"`
}

// SQLiteConfig holds the SQLite configuration.
type SQLiteConfig struct {
	Path string `yaml:"path"`
}

// AutonomyConfig controls global and per-domain autonomy presets and optional
// per-dimension overrides. It is the user-facing configuration surface for how
// much NAVI acts without asking. Domains correspond to the conceptual autonomy
// model and are referenced from governor.DomainForCommand. Supported domain keys
// include: "messaging", "scheduling", "coding", "memory", "plugins", "workflow",
// "delegation", "configuration".
type AutonomyConfig struct {
	// GlobalPreset is one of "conservative", "balanced", or "high".
	GlobalPreset string `yaml:"global_preset"`
	// DomainOverrides maps domain name to an autonomy preset that overrides the global preset for that domain.
	DomainOverrides map[string]string `yaml:"domain_overrides,omitempty"`
	// DomainDimensionOverrides optionally sets one or more dimensions per domain. Empty dimension values fall back to the preset-derived value for that domain.
	DomainDimensionOverrides map[string]AutonomyDimensions `yaml:"domain_dimension_overrides,omitempty"`
}

// AutonomyDimensions are the four design dimensions; used for validation and reflection.
// Presets map to these; per-domain overrides select a preset or override individual dimensions.
type AutonomyDimensions struct {
	ExecutionThreshold         string `yaml:"execution_threshold" json:"execution_threshold"`                   // low | medium | high
	InsightSurfacing           string `yaml:"insight_surfacing" json:"insight_surfacing"`                       // low | medium | high
	MemoryPromotionSensitivity string `yaml:"memory_promotion_sensitivity" json:"memory_promotion_sensitivity"` // low | medium | high
	PluginInvocationAuthority  string `yaml:"plugin_invocation_authority" json:"plugin_invocation_authority"`   // low | medium | high
}

// PresetToDimensions maps a preset name to the four autonomy dimensions.
func PresetToDimensions(preset string) AutonomyDimensions {
	switch preset {
	case "high":
		return AutonomyDimensions{"high", "high", "high", "high"}
	case "conservative":
		return AutonomyDimensions{"low", "medium", "low", "low"}
	case "balanced":
		return AutonomyDimensions{"medium", "medium", "medium", "medium"}
	default:
		return AutonomyDimensions{"medium", "medium", "medium", "medium"}
	}
}

// EffectivePreset returns the autonomy preset for a domain (override or global).
func (c AutonomyConfig) EffectivePreset(domain string) string {
	if c.DomainOverrides != nil {
		if p, ok := c.DomainOverrides[domain]; ok && p != "" {
			return p
		}
	}
	if c.GlobalPreset != "" {
		return c.GlobalPreset
	}
	return "balanced"
}

// EffectiveDimensions returns the four autonomy dimensions for a domain. Per-domain
// dimension overrides (when set) are merged over the preset-derived values; empty
// override fields fall back to the preset. So all four dimensions are exposed as
// knobs via domain_dimension_overrides in config.
func (c AutonomyConfig) EffectiveDimensions(domain string) AutonomyDimensions {
	base := PresetToDimensions(c.EffectivePreset(domain))
	if c.DomainDimensionOverrides == nil {
		return base
	}
	o, ok := c.DomainDimensionOverrides[domain]
	if !ok {
		return base
	}
	out := base
	if o.ExecutionThreshold != "" {
		out.ExecutionThreshold = o.ExecutionThreshold
	}
	if o.InsightSurfacing != "" {
		out.InsightSurfacing = o.InsightSurfacing
	}
	if o.MemoryPromotionSensitivity != "" {
		out.MemoryPromotionSensitivity = o.MemoryPromotionSensitivity
	}
	if o.PluginInvocationAuthority != "" {
		out.PluginInvocationAuthority = o.PluginInvocationAuthority
	}
	return out
}

// EffectiveExecutionThreshold returns the execution-threshold dimension for the domain.
// Implement governor.ExecutionThresholdResolver so ApplyAutonomy respects domain_dimension_overrides.
func (c AutonomyConfig) EffectiveExecutionThreshold(domain string) string {
	return c.EffectiveDimensions(domain).ExecutionThreshold
}

// Telegram persisted-setting keys (used by navid and gateway when reading/writing DB settings).
const (
	TelegramSettingKeyBotToken    = "telegram_bot_token"
	TelegramSettingKeyOwnerChatID = "telegram_owner_chat_id"
	TelegramSettingKeyAccounts    = "telegram_accounts"
)

// TelegramGroupConfig holds per-group behavior. The key "*" sets defaults for all
// groups; a specific group chat ID (as a string) overrides the wildcard.
type TelegramGroupConfig struct {
	Enabled        *bool `yaml:"enabled" json:"enabled,omitempty"`
	RequireMention *bool `yaml:"require_mention" json:"require_mention,omitempty"` // default true in groups
}

// TelegramConfig holds the Telegram bot configuration.
type TelegramConfig struct {
	BotToken      string                         `yaml:"bot_token"`
	TokenFile     string                         `yaml:"token_file"` // path to file containing bot token (used when BotToken empty)
	OwnerChatID   int64                          `yaml:"owner_chat_id"`
	AllowFrom     []int64                        `yaml:"allow_from"`   // extra chat IDs allowed to interact in addition to OwnerChatID
	PairingCode   string                         `yaml:"pairing_code"` // optional /pair code to dynamically allow new chats
	GatewayURL    string                         `yaml:"gateway_url"`
	APIURL        string                         `yaml:"api_url"`
	WebhookURL    string                         `yaml:"webhook_url"`    // when set, use webhook instead of long-polling
	WebhookSecret string                         `yaml:"webhook_secret"` // optional; validated in HandleWebhook
	Groups        map[string]TelegramGroupConfig `yaml:"groups"`         // per-group behavior, keyed by group chat ID or "*"
	Accounts      []TelegramAccountConfig        `yaml:"accounts"`       // optional named accounts; when set, legacy single-account fields are ignored
}

// TelegramAccountConfig holds one Telegram account definition for multi-account setups.
type TelegramAccountConfig struct {
	Name          string                         `yaml:"name" json:"name"`
	BotToken      string                         `yaml:"bot_token" json:"bot_token,omitempty"`
	TokenFile     string                         `yaml:"token_file" json:"token_file,omitempty"`
	OwnerChatID   int64                          `yaml:"owner_chat_id" json:"owner_chat_id"`
	AllowFrom     []int64                        `yaml:"allow_from" json:"allow_from,omitempty"`
	PairingCode   string                         `yaml:"pairing_code" json:"pairing_code,omitempty"`
	GatewayURL    string                         `yaml:"gateway_url" json:"gateway_url,omitempty"`
	APIURL        string                         `yaml:"api_url" json:"api_url,omitempty"`
	WebhookURL    string                         `yaml:"webhook_url" json:"webhook_url,omitempty"`
	WebhookSecret string                         `yaml:"webhook_secret" json:"webhook_secret,omitempty"`
	Groups        map[string]TelegramGroupConfig `yaml:"groups" json:"groups,omitempty"` // per-group behavior, keyed by group chat ID or "*"
}

// SlackConfig holds the Slack bot configuration.
type SlackConfig struct {
	BotToken       string   `yaml:"bot_token"`
	AppToken       string   `yaml:"app_token"`
	GatewayURL     string   `yaml:"gateway_url"`
	AllowedUserIDs []string `yaml:"allowed_user_ids"`
}

// LSPConfig holds the LSP proxy configuration.
type LSPConfig struct {
	WorkspaceRoot string `yaml:"workspace_root"`
}

// ModelRoute defines a primary model and optional fallbacks for a logical LLM role.
// Model strings may be provider-qualified (e.g. "anthropic/claude-sonnet-4-20250514")
// or bare model ids (e.g. "gpt-4o-mini") for legacy configs.
type ModelRoute struct {
	Primary   string   `yaml:"primary"`
	Fallbacks []string `yaml:"fallbacks,omitempty"`
}

// ProviderConfig describes one logical LLM provider and the models the
// deployment owner has made available for that provider.
//
// Keys in the Providers map are provider identifiers (e.g. "ollama",
// "anthropic", "openai"). DisplayName is optional and used only for
// human-facing lists; when empty, the key should be presented instead.
//
// Models is a free-form list of model identifiers understood by the
// underlying provider, e.g.:
//   - "llama3:latest"
//   - "gpt-4o"
//   - "claude-3.5-sonnet"
type ProviderConfig struct {
	DisplayName string   `yaml:"display_name,omitempty"`
	Models      []string `yaml:"models,omitempty"`
}

// LLMConfig holds LLM provider configuration.
type LLMConfig struct {
	OllamaURL          string `yaml:"ollama_url"`           // default "http://localhost:11434/v1"
	OllamaModel        string `yaml:"ollama_model"`         // default "llama3:latest"
	OllamaDiscussModel string `yaml:"ollama_discuss_model"` // default "llama3:latest" — used by Orchestrator DISCUSS mode
	AnthropicKey       string `yaml:"anthropic_key"`        // env: NAVI_ANTHROPIC_KEY
	AnthropicModel     string `yaml:"anthropic_model"`      // default "claude-sonnet-4-20250514"
	OpenAIKey          string `yaml:"openai_key"`           // env: NAVI_OPENAI_KEY
	OpenAIModel        string `yaml:"openai_model"`         // default "gpt-4o-mini"
	OpenRouterKey      string `yaml:"openrouter_key"`       // env: NAVI_OPENROUTER_KEY
	OpenRouterModel    string `yaml:"openrouter_model"`     // default "anthropic/claude-3.5-haiku"
	BraveSearchKey     string `yaml:"brave_search_key"`     // env: NAVI_BRAVE_SEARCH_KEY
	OrchestratorModel  string `yaml:"orchestrator_model"`   // default "llama3:latest"
	CoderModel         string `yaml:"coder_model"`          // default "llama3:latest"
	ChatModel          string `yaml:"chat_model"`           // default "llama3:latest"

	// Providers is an optional catalog of providers and the models that the
	// deployment owner has explicitly enabled for each. When empty, NAVI
	// derives a catalog from the legacy *Model fields above so existing
	// configs continue to work unchanged.
	Providers map[string]ProviderConfig `yaml:"providers,omitempty"`

	// Routes defines logical model roles (e.g. "orchestrator", "coder", "chat")
	// mapped to primary and fallback model chains. When empty, sensible defaults
	// are derived from the legacy *Model fields above.
	Routes map[string]ModelRoute `yaml:"routes,omitempty"`

	// ProviderPolicies defines operator policies for individual providers.
	// When absent, sensible defaults are applied (all providers enabled,
	// allow_pull=true, allow_delete=false). All fields use *bool so zero-value
	// means "use default".
	ProviderPolicies map[string]ProviderPolicyConfig `yaml:"provider_policies,omitempty"`
}

// ProviderPolicyConfig is the config-file representation of per-provider policies.
type ProviderPolicyConfig struct {
	Enabled            *bool `yaml:"enabled,omitempty"`
	LocalFirst         *bool `yaml:"local_first,omitempty"`
	AllowPull          *bool `yaml:"allow_pull,omitempty"`
	AllowDelete        *bool `yaml:"allow_delete,omitempty"`
	AutoWarm           *bool `yaml:"auto_warm,omitempty"`
	AllowPaidRemote    *bool `yaml:"allow_paid_remote,omitempty"`
	AllowFallbackLocal *bool `yaml:"allow_fallback_from_local,omitempty"`
}

// NaviConfig holds configuration for the persistent NAVI agent.
type NaviConfig struct {
	WorkspaceDir                    string `yaml:"workspace_dir"`                     // path to workspace (default: ./workspace)
	ExperienceProfilesDir           string `yaml:"experience_profiles_dir"`           // primary override dir for standard/wizard experience profile assets
	ArtifactsDir                    string `yaml:"artifacts_dir"`                     // path to artifact blob store (default: ./artifacts)
	InitialExperienceMode           string `yaml:"initial_experience_mode"`           // onboarding may use wizard; all other values normalize to standard NAVI
	HeartbeatEnabled                bool   `yaml:"heartbeat_enabled"`                 // default: true
	HeartbeatInterval               string `yaml:"heartbeat_interval"`                // e.g. "30m", "1h" (default: 30m)
	HeartbeatQuietHoursStart        string `yaml:"heartbeat_quiet_hours_start"`       // local-time quiet start, e.g. "22:00"
	HeartbeatQuietHoursEnd          string `yaml:"heartbeat_quiet_hours_end"`         // local-time quiet end, e.g. "08:00"
	HeartbeatDNDEnabled             bool   `yaml:"heartbeat_dnd_enabled"`             // suppress proactive heartbeat surfacing entirely
	ReflectionConsolidationInterval string `yaml:"reflection_consolidation_interval"` // cadence for background consolidation
	LLMCallTimeout                  string `yaml:"llm_call_timeout"`                  // e.g. "5m"; foreground runtime deadline for a single LLM turn
	MaxResponseTokens               int    `yaml:"max_response_tokens"`               // default: 4096; hard cap for one assistant response
	Debug                           bool   `yaml:"debug"`                             // enable slog debug level (default: false; overridden by NAVI_DEBUG)
	PromptsDir                      string `yaml:"prompts_dir"`                       // writable prompt templates (default: ./prompts)
	PromptWatchEnabled              bool   `yaml:"prompt_watch_enabled"`              // watch prompts_dir for hot reload (default: true)
	// ChatSystemPromptOverride replaces the chat system prompt template when set (non-empty); env NAVI_CHAT_SYSTEM_PROMPT wins.
	ChatSystemPromptOverride string `yaml:"chat_system_prompt_override"`
	ContextRecentMessages    int    `yaml:"context_recent_messages"` // recent messages kept verbatim in direct LLM context
	ContextPinnedMessages    int    `yaml:"context_pinned_messages"` // earliest messages kept verbatim to preserve opening intent
	EnableRunScratchpad      bool   `yaml:"enable_run_scratchpad"`   // include ephemeral run scratchpad in prompt assembly
}

// CronConfig controls the internal/cron scheduling service.
type CronConfig struct {
	Enabled                 bool  `yaml:"enabled"`
	MaxConcurrentRuns       int   `yaml:"max_concurrent_runs"`
	MissedJobStaggerMs      int64 `yaml:"missed_job_stagger_ms"`
	MaxMissedJobsPerRestart int   `yaml:"max_missed_jobs_per_restart"`
	DefaultTimeoutMs        int64 `yaml:"default_timeout_ms"`
	Retry                   struct {
		MaxAttempts int      `yaml:"max_attempts"`
		BackoffMs   []int64  `yaml:"backoff_ms"`
		RetryOn     []string `yaml:"retry_on"`
	} `yaml:"retry"`
	FailureAlert CronFailureAlertGlobal `yaml:"failure_alert"`
}

// CronFailureAlertGlobal is the default failure alert policy for cron jobs without per-job overrides.
type CronFailureAlertGlobal struct {
	Enabled    bool  `yaml:"enabled"`
	After      int   `yaml:"after"`
	CooldownMs int64 `yaml:"cooldown_ms"`
}

// EffectiveCronConfig returns sanitized defaults for the cron service at runtime.
func (c Config) EffectiveCronConfig() CronConfig {
	cfg := c.Cron
	if cfg.MaxConcurrentRuns < 1 {
		cfg.MaxConcurrentRuns = 2
	}
	if cfg.MissedJobStaggerMs <= 0 {
		cfg.MissedJobStaggerMs = 5_000
	}
	if cfg.MaxMissedJobsPerRestart < 1 {
		cfg.MaxMissedJobsPerRestart = 5
	}
	if cfg.DefaultTimeoutMs <= 0 {
		cfg.DefaultTimeoutMs = int64((5 * time.Minute).Milliseconds())
	}
	if cfg.Retry.MaxAttempts < 1 {
		cfg.Retry.MaxAttempts = 3
	}
	if len(cfg.Retry.BackoffMs) == 0 {
		cfg.Retry.BackoffMs = []int64{30_000, 60_000, 5 * 60_000, 15 * 60_000, 60 * 60_000}
	}
	if cfg.FailureAlert.After < 1 && cfg.FailureAlert.Enabled {
		cfg.FailureAlert.After = 2
	}
	if cfg.FailureAlert.CooldownMs < 0 {
		cfg.FailureAlert.CooldownMs = time.Hour.Milliseconds()
	}
	if cfg.FailureAlert.CooldownMs == 0 && cfg.FailureAlert.Enabled {
		cfg.FailureAlert.CooldownMs = time.Hour.Milliseconds()
	}
	return cfg
}

// defaults returns a Config populated with safe default values.
func defaults() Config {
	cfg := Config{
		Gateway: GatewayConfig{
			Addr:      ":6284",
			StaticDir: "web",
			OriginPatterns: []string{
				"http://localhost:6284",
				"http://localhost:5173",
				"http://localhost:3000",
				"http://127.0.0.1:6284",
				"http://127.0.0.1:5173",
				"http://127.0.0.1:3000",
			},
		},
		NATS: NATSConfig{
			URL: "embedded",
		},
		SQLite: SQLiteConfig{
			Path: "navi.db",
		},
		Connectors: ConnectorsConfig{
			Telegram: TelegramConfig{
				GatewayURL: "http://localhost:6284",
				APIURL:     "https://api.telegram.org",
			},
			Slack: SlackConfig{
				GatewayURL: "http://localhost:6284",
			},
		},
		LLM: LLMConfig{
			OllamaURL:          "http://localhost:11434/v1",
			OllamaModel:        "llama3:latest",
			OllamaDiscussModel: "llama3:latest",
			OrchestratorModel:  "llama3:latest",
			CoderModel:         "qwen3-coder:30b",
			ChatModel:          "llama3:latest",
			Providers:          nil,
		},
		Navi: NaviConfig{
			WorkspaceDir:                    "workspace",
			ExperienceProfilesDir:           "config/personas",
			ArtifactsDir:                    "artifacts",
			PromptsDir:                      "prompts",
			PromptWatchEnabled:              true,
			InitialExperienceMode:           "navi",
			HeartbeatEnabled:                true,
			HeartbeatInterval:               "30m",
			HeartbeatQuietHoursStart:        "",
			HeartbeatQuietHoursEnd:          "",
			HeartbeatDNDEnabled:             false,
			ReflectionConsolidationInterval: "30m",
			LLMCallTimeout:                  "5m",
			MaxResponseTokens:               4096,
			ContextRecentMessages:           12,
			ContextPinnedMessages:           2,
			EnableRunScratchpad:             true,
		},
		Governor: GovernorConfig{
			MaxActionBudget:    1000,
			MaxRetries:         5,
			CostCeiling:        50.0,
			AutonomousDuration: "72h",
			MaxRepetitions:     3,
			EventRetentionDays: 30,
		},
		Cron: func() CronConfig {
			var c CronConfig
			c.Enabled = true
			c.MaxConcurrentRuns = 2
			c.MissedJobStaggerMs = 5_000
			c.MaxMissedJobsPerRestart = 5
			c.DefaultTimeoutMs = int64((5 * time.Minute).Milliseconds())
			c.Retry.MaxAttempts = 3
			c.Retry.BackoffMs = []int64{30_000, 60_000, 5 * 60_000, 15 * 60_000, 60 * 60_000}
			c.FailureAlert.Enabled = false
			c.FailureAlert.After = 2
			c.FailureAlert.CooldownMs = time.Hour.Milliseconds()
			return c
		}(),
		Autonomy: AutonomyConfig{
			GlobalPreset: "balanced",
			DomainOverrides: map[string]string{
				"messaging": "high",
			},
		},
		Vault: VaultConfig{
			Enabled:       true,
			Dir:           "", // resolved to <workspace>/vault at startup when empty
			Debounce:      "750ms",
			DriftInterval: "24h",
			PrivacyMode:   "cloud",
		},
	}
	normalizeNaviExperienceConfig(&cfg.Navi)
	return cfg
}

func normalizeNaviExperienceConfig(cfg *NaviConfig) {
	if cfg == nil {
		return
	}
	profilesDir := strings.TrimSpace(cfg.ExperienceProfilesDir)
	if profilesDir == "" {
		profilesDir = "config/personas"
	}
	cfg.ExperienceProfilesDir = profilesDir

	initialMode := strings.TrimSpace(cfg.InitialExperienceMode)
	if initialMode == "" {
		initialMode = "navi"
	}
	cfg.InitialExperienceMode = initialMode
}

// Load reads and validates a YAML config file. Returns an error if the file
// cannot be read or if required fields (jwt_secret) are empty.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	cfg := defaults()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	applyEnvOverrides(&cfg)
	normalizeNaviExperienceConfig(&cfg.Navi)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadOrDefault reads the config file if it exists, otherwise returns defaults.
// Still applies env overrides and validates required fields.
func LoadOrDefault(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := defaults()
		applyEnvOverrides(&cfg)
		normalizeNaviExperienceConfig(&cfg.Navi)
		if err := validate(&cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	return Load(path)
}

// getEnvWithFallback returns the value of the primary env var, or the fallback if the primary is empty.
func getEnvWithFallback(primary, fallback string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	return os.Getenv(fallback)
}

// applyEnvOverrides applies environment variable overrides for secrets.
// Env vars take precedence over YAML file values.
func applyEnvOverrides(cfg *Config) {
	if v := getEnvWithFallback("NAVI_GATEWAY_ADDR", "HELM_GATEWAY_ADDR"); v != "" {
		cfg.Gateway.Addr = v
	}
	if v := getEnvWithFallback("NAVI_GATEWAY_SHARED_SECRET", "HELM_GATEWAY_SHARED_SECRET"); v != "" {
		cfg.Gateway.SharedSecret = v
	}
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Connectors.Telegram.BotToken = v
	} else if v := getEnvWithFallback("NAVI_TELEGRAM_BOT_TOKEN", "HELM_TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Connectors.Telegram.BotToken = v
	}
	if v := getEnvWithFallback("NAVI_TELEGRAM_OWNER_CHAT_ID", "HELM_TELEGRAM_OWNER_CHAT_ID"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Connectors.Telegram.OwnerChatID = id
		}
	}
	if v := getEnvWithFallback("NAVI_TELEGRAM_ALLOW_FROM", "HELM_TELEGRAM_ALLOW_FROM"); v != "" {
		cfg.Connectors.Telegram.AllowFrom = parseTelegramChatIDList(v)
	}
	if v := getEnvWithFallback("NAVI_TELEGRAM_PAIRING_CODE", "HELM_TELEGRAM_PAIRING_CODE"); v != "" {
		cfg.Connectors.Telegram.PairingCode = v
	}
	if v := getEnvWithFallback("NAVI_TELEGRAM_API_URL", "HELM_TELEGRAM_API_URL"); v != "" {
		cfg.Connectors.Telegram.APIURL = v
	}
	if v := getEnvWithFallback("NAVI_SLACK_BOT_TOKEN", "HELM_SLACK_BOT_TOKEN"); v != "" {
		cfg.Connectors.Slack.BotToken = v
	}
	if v := getEnvWithFallback("NAVI_ORCHESTRATOR_MODEL", "HELM_CEO_MODEL"); v != "" {
		cfg.LLM.OrchestratorModel = v
	}
	if v := getEnvWithFallback("NAVI_OLLAMA_URL", "HELM_OLLAMA_URL"); v != "" {
		cfg.LLM.OllamaURL = v
	}
	if v := getEnvWithFallback("NAVI_ANTHROPIC_KEY", "HELM_ANTHROPIC_KEY"); v != "" {
		cfg.LLM.AnthropicKey = v
	}
	if v := getEnvWithFallback("NAVI_OPENAI_KEY", "HELM_OPENAI_KEY"); v != "" {
		cfg.LLM.OpenAIKey = v
	}
	if v := getEnvWithFallback("NAVI_OPENROUTER_KEY", "HELM_OPENROUTER_KEY"); v != "" {
		cfg.LLM.OpenRouterKey = v
	}
	if v := getEnvWithFallback("NAVI_BRAVE_SEARCH_KEY", ""); v != "" {
		cfg.LLM.BraveSearchKey = v
	}
	if v := getEnvWithFallback("NAVI_SQLITE_PATH", "HELM_SQLITE_PATH"); v != "" {
		cfg.SQLite.Path = v
	}
	if v := getEnvWithFallback("NAVI_NATS_URL", "HELM_NATS_URL"); v != "" {
		cfg.NATS.URL = v
	}
	if v := getEnvWithFallback("NAVI_WORKSPACE_DIR", "HELM_NAVI_WORKSPACE_DIR"); v != "" {
		cfg.Navi.WorkspaceDir = v
	}
	if v := getEnvWithFallback("NAVI_VAULT_DIR", ""); v != "" {
		cfg.Vault.Dir = v
	}
	if v := getEnvWithFallback("NAVI_VAULT_PRIVACY_MODE", ""); v != "" {
		cfg.Vault.PrivacyMode = v
	}
	if v := getEnvWithFallback("NAVI_VAULT_ENABLED", ""); v != "" {
		cfg.Vault.Enabled = v == "1" || strings.EqualFold(v, "true")
	}
	if v := getEnvWithFallback("NAVI_EXPERIENCE_PROFILES_DIR", ""); v != "" {
		cfg.Navi.ExperienceProfilesDir = v
	}
	if v := getEnvWithFallback("NAVI_ARTIFACTS_DIR", ""); v != "" {
		cfg.Navi.ArtifactsDir = v
	}
	if v := getEnvWithFallback("NAVI_PROMPTS_DIR", ""); v != "" {
		cfg.Navi.PromptsDir = v
	}
	if v := getEnvWithFallback("NAVI_CHAT_SYSTEM_PROMPT", ""); v != "" {
		cfg.Navi.ChatSystemPromptOverride = v
	}
	if v := getEnvWithFallback("NAVI_PROMPT_WATCH", ""); v != "" {
		switch v {
		case "0", "false", "no", "off":
			cfg.Navi.PromptWatchEnabled = false
		default:
			cfg.Navi.PromptWatchEnabled = true
		}
	}
	if v := getEnvWithFallback("NAVI_INITIAL_EXPERIENCE_MODE", ""); v != "" {
		cfg.Navi.InitialExperienceMode = v
	}
	if v := getEnvWithFallback("NAVI_HEARTBEAT_INTERVAL", "HELM_NAVI_HEARTBEAT_INTERVAL"); v != "" {
		cfg.Navi.HeartbeatInterval = v
	}
	if v := getEnvWithFallback("NAVI_HEARTBEAT_QUIET_HOURS_START", ""); v != "" {
		cfg.Navi.HeartbeatQuietHoursStart = v
	}
	if v := getEnvWithFallback("NAVI_HEARTBEAT_QUIET_HOURS_END", ""); v != "" {
		cfg.Navi.HeartbeatQuietHoursEnd = v
	}
	if v := getEnvWithFallback("NAVI_HEARTBEAT_DND", ""); v != "" {
		cfg.Navi.HeartbeatDNDEnabled = v == "1" || v == "true" || v == "yes"
	}
	if v := getEnvWithFallback("NAVI_REFLECTION_CONSOLIDATION_INTERVAL", ""); v != "" {
		cfg.Navi.ReflectionConsolidationInterval = v
	}
	if v := getEnvWithFallback("NAVI_LLM_CALL_TIMEOUT", "HELM_NAVI_LLM_CALL_TIMEOUT"); v != "" {
		cfg.Navi.LLMCallTimeout = v
	}
	if v := getEnvWithFallback("NAVI_MAX_RESPONSE_TOKENS", "HELM_NAVI_MAX_RESPONSE_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Navi.MaxResponseTokens = n
		}
	}
	if v := getEnvWithFallback("NAVI_DEBUG", ""); v != "" && (v == "1" || v == "true" || v == "yes") {
		cfg.Navi.Debug = true
	}
	if v := getEnvWithFallback("NAVI_GOVERNOR_MAX_ACTION_BUDGET", "HELM_GOVERNOR_MAX_ACTION_BUDGET"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Governor.MaxActionBudget = n
		}
	}
	if v := getEnvWithFallback("NAVI_GOVERNOR_MAX_RETRIES", "HELM_GOVERNOR_MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.Governor.MaxRetries = n
		}
	}
	if v := getEnvWithFallback("NAVI_GOVERNOR_COST_CEILING", "HELM_GOVERNOR_COST_CEILING"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			cfg.Governor.CostCeiling = f
		}
	}
	if v := getEnvWithFallback("NAVI_GOVERNOR_AUTONOMOUS_DURATION", "HELM_GOVERNOR_AUTONOMOUS_DURATION"); v != "" {
		cfg.Governor.AutonomousDuration = v
	}
	if v := getEnvWithFallback("NAVI_GOVERNOR_MAX_REPETITIONS", "HELM_GOVERNOR_MAX_REPETITIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Governor.MaxRepetitions = n
		}
	}
	if v := getEnvWithFallback("NAVI_GOVERNOR_EXECUTION_OUTCOME_RETENTION_DAYS", "HELM_GOVERNOR_EXECUTION_OUTCOME_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.Governor.ExecutionOutcomeRetentionDays = n
		}
	}
	if v := getEnvWithFallback("NAVI_GOVERNOR_EVENT_RETENTION_DAYS", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.Governor.EventRetentionDays = n
		}
	}
}

// validate checks required fields and paths so misconfig fails at startup.
func validate(cfg *Config) error {
	if err := validateLLM(cfg); err != nil {
		return err
	}
	if err := validateGovernance(cfg); err != nil {
		return err
	}
	if err := validatePromptsDir(cfg); err != nil {
		return err
	}
	return validateSQLiteParentDir(cfg)
}

func validateGovernance(cfg *Config) error {
	if cfg.Navi.ContextRecentMessages < 0 {
		return errors.New("config: navi.context_recent_messages must be >= 0")
	}
	if cfg.Navi.ContextPinnedMessages < 0 {
		return errors.New("config: navi.context_pinned_messages must be >= 0")
	}
	for eventType, days := range cfg.Governor.EventRetentionByType {
		if days < 0 {
			return fmt.Errorf("config: governor.event_retention_by_type[%q] must be >= 0", eventType)
		}
	}
	return nil
}

func validatePromptsDir(cfg *Config) error {
	dir := cfg.Navi.PromptsDir
	if dir == "" {
		return errors.New("config: navi.prompts_dir is empty")
	}
	return os.MkdirAll(dir, 0o755)
}

func validateLLM(cfg *Config) error {
	hasKey := cfg.LLM.AnthropicKey != "" || cfg.LLM.OpenAIKey != "" || cfg.LLM.OpenRouterKey != ""
	hasOllama := cfg.LLM.OllamaURL != ""
	if hasKey || hasOllama {
		return nil
	}
	return errors.New("config: at least one LLM provider required (set NAVI_ANTHROPIC_KEY, NAVI_OPENAI_KEY, NAVI_OPENROUTER_KEY, or NAVI_OLLAMA_URL)")
}

func validateSQLiteParentDir(cfg *Config) error {
	path := cfg.SQLite.Path
	if path == "" {
		return errors.New("config: sqlite.path is empty")
	}
	parent := filepath.Dir(path)
	if parent == "" || parent == "." {
		parent = "."
	}
	info, err := os.Stat(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("config: sqlite path parent %q does not exist", parent)
		}
		return fmt.Errorf("config: sqlite path parent %q: %w", parent, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("config: sqlite path parent %q is not a directory", parent)
	}
	if info.Mode().Perm()&0o222 == 0 {
		return fmt.Errorf("config: sqlite path parent %q is not writable", parent)
	}
	tmp, err := os.CreateTemp(parent, ".navi_check_")
	if err != nil {
		return fmt.Errorf("config: sqlite path parent %q is not writable: %w", parent, err)
	}
	_ = tmp.Close()
	_ = os.Remove(tmp.Name())
	return nil
}

// ResolveTelegramToken returns the bot token from BotToken, or by reading TokenFile when BotToken is empty.
func ResolveTelegramToken(cfg TelegramConfig) (string, error) {
	return resolveTelegramToken(cfg.BotToken, cfg.TokenFile)
}
