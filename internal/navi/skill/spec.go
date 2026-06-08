package skill

import "github.com/ceoai/navi/internal/capability"

// OSS27Spec follows the "Structured Capability Mechanisms in AI Systems and a Proposed 2027 Skill Standard".
// It separates the LLM's understanding from the execution runtime's requirements.
type OSS27Spec struct {
	OSS27Version  string                     `json:"oss27_version" yaml:"oss27_version"`
	SkillID       string                     `json:"skill_id" yaml:"skill_id"`
	Semver        string                     `json:"semver" yaml:"semver"`
	Display       DisplayMetadata            `json:"display" yaml:"display"`
	Interfaces    []Interface                `json:"interfaces" yaml:"interfaces"`
	Effects       EffectMetadata             `json:"effects" yaml:"effects"`
	Security      SecuritySpec               `json:"security" yaml:"security"`
	Performance   PerformanceSpec            `json:"performance" yaml:"performance"`
	Observability ObservabilitySpec          `json:"observability" yaml:"observability"`
	Governance    GovernanceSpec             `json:"governance" yaml:"governance"`
	Capability    CapabilityMetadata         `json:"capability,omitempty" yaml:"capability,omitempty"`
	Reliability   ReliabilitySpec            `json:"reliability,omitempty" yaml:"reliability,omitempty"`
	UISurfaces    []capability.UISurfaceSpec `json:"uiSurfaces,omitempty" yaml:"ui_surfaces,omitempty"`
	// PythonRuntime is populated for skills that use transport.type=subprocess_python.
	// It controls environment provisioning, lifecycle, I/O limits, and isolation.
	PythonRuntime *PythonRuntimeSpec `json:"python_runtime,omitempty" yaml:"python_runtime,omitempty"`
}

type DisplayMetadata struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Emoji       string `json:"emoji,omitempty" yaml:"emoji,omitempty"`
}

type Interface struct {
	Name         string                 `json:"name" yaml:"name"`
	Transport    TransportSpec          `json:"transport" yaml:"transport"`
	InputSchema  map[string]interface{} `json:"input_schema" yaml:"input_schema"`
	OutputSchema map[string]interface{} `json:"output_schema" yaml:"output_schema"`
	ErrorSchema  map[string]interface{} `json:"error_schema,omitempty" yaml:"error_schema,omitempty"`
}

type TransportSpec struct {
	// Type is the execution backend: "mcp_tool" | "rest" | "internal" | "subprocess_python" | "subprocess"
	Type             string                     `json:"type" yaml:"type"`
	MCP              *MCPTransport              `json:"mcp,omitempty" yaml:"mcp,omitempty"`
	REST             *RESTTransport             `json:"rest,omitempty" yaml:"rest,omitempty"`
	SubprocessPython *SubprocessPythonTransport `json:"subprocess_python,omitempty" yaml:"subprocess_python,omitempty"`
	// Runtime is advisory metadata for generic subprocess transports, for example "python" or "node".
	Runtime string `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	// Command is the process argv used by generic subprocess transports.
	Command []string `json:"command,omitempty" yaml:"command,omitempty"`
	// Protocol declares the subprocess wire protocol. The first supported value is "jsonrpc_stdio".
	Protocol string `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	// SandboxProfile selects the NAVI sandbox profile for generic subprocess transports when sandboxing is required.
	SandboxProfile string `json:"sandbox_profile,omitempty" yaml:"sandbox_profile,omitempty"`
}

type MCPTransport struct {
	Server string `json:"server" yaml:"server"`
	Tool   string `json:"tool" yaml:"tool"`
}

type RESTTransport struct {
	URL    string            `json:"url" yaml:"url"`
	Method string            `json:"method" yaml:"method"`
	Auth   map[string]string `json:"auth,omitempty" yaml:"auth,omitempty"`
}

// SubprocessPythonTransport configures a single interface's Python entry point.
// The Go runner calls function(params) inside entrypoint and reads one SkillResult from stdout.
type SubprocessPythonTransport struct {
	// Entrypoint is the Python file to invoke, relative to the skill's base directory.
	Entrypoint string `json:"entrypoint" yaml:"entrypoint"`
	// Function is the callable inside the entrypoint module. Defaults to "run".
	Function string `json:"function,omitempty" yaml:"function,omitempty"`
}

// PythonRuntimeSpec is the top-level python_runtime block in SKILL.yaml.
// It applies to all subprocess_python interfaces in the skill.
type PythonRuntimeSpec struct {
	// Environment
	PythonVersion    string   `json:"python_version,omitempty" yaml:"python_version,omitempty"`       // min required; default "3.11"
	VenvStrategy     string   `json:"venv_strategy,omitempty" yaml:"venv_strategy,omitempty"`         // per_skill | per_category | shared
	RequirementsFile string   `json:"requirements_file,omitempty" yaml:"requirements_file,omitempty"` // relative path; default "requirements.txt"
	Dependencies     []string `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`           // inline pip specifiers; merged with requirements_file

	// Lifecycle
	Lifecycle      string `json:"lifecycle,omitempty" yaml:"lifecycle,omitempty"`               // cold | warm; default "cold"
	WarmPoolSize   int    `json:"warm_pool_size,omitempty" yaml:"warm_pool_size,omitempty"`     // max idle workers; only used when lifecycle=warm
	SpawnTimeoutMS int    `json:"spawn_timeout_ms,omitempty" yaml:"spawn_timeout_ms,omitempty"` // process start deadline; default 5000

	// I/O
	IOFormat       string `json:"io_format,omitempty" yaml:"io_format,omitempty"`               // json_stdio (only supported value)
	MaxInputBytes  int    `json:"max_input_bytes,omitempty" yaml:"max_input_bytes,omitempty"`   // default 1 MiB
	MaxOutputBytes int    `json:"max_output_bytes,omitempty" yaml:"max_output_bytes,omitempty"` // default 5 MiB

	// Resource Limits
	TimeoutMS     int `json:"timeout_ms,omitempty" yaml:"timeout_ms,omitempty"`           // wall-clock kill; overrides performance.timeout_ms
	MaxMemoryMB   int `json:"max_memory_mb,omitempty" yaml:"max_memory_mb,omitempty"`     // RSS cap; default 256
	MaxCPUPercent int `json:"max_cpu_percent,omitempty" yaml:"max_cpu_percent,omitempty"` // advisory throttle hint

	// Isolation
	NetworkAccess bool `json:"network_access,omitempty" yaml:"network_access,omitempty"` // default false
	AllowFSWrite  bool `json:"allow_fs_write,omitempty" yaml:"allow_fs_write,omitempty"` // default false
	TmpdirMB      int  `json:"tmpdir_mb,omitempty" yaml:"tmpdir_mb,omitempty"`           // scratch space per invocation; default 64

	// Maturity
	Maturity string `json:"maturity,omitempty" yaml:"maturity,omitempty"` // prototype | stable | hardened
}

// SkillResultStatus is the status field of every SkillResult envelope.
type SkillResultStatus string

const (
	SkillResultSuccess     SkillResultStatus = "success"
	SkillResultFailed      SkillResultStatus = "failed"
	SkillResultPartial     SkillResultStatus = "partial"
	SkillResultStatusError SkillResultStatus = "error"
	SkillResultTimeout     SkillResultStatus = "timeout"
	SkillResultKilled      SkillResultStatus = "killed"
	SkillResultPolicyBlock SkillResultStatus = "policy_blocked"
	SkillResultTruncated   SkillResultStatus = "truncated"
)

// SkillResult is the mandatory JSON envelope returned by every Python skill invocation.
// The Go runner reads exactly one SkillResult from the subprocess stdout after execution.
type SkillResult struct {
	Status     SkillResultStatus   `json:"status"`
	Output     interface{}         `json:"output,omitempty"`
	Error      *SkillResultError   `json:"error,omitempty"`
	DurationMS int64               `json:"duration_ms"`
	Metadata   SkillResultMetadata `json:"metadata,omitempty"`
}

// SkillResultError carries structured error information on non-success results.
type SkillResultError struct {
	Type      string `json:"type"` // exception class name or runner error category
	Message   string `json:"message"`
	Traceback string `json:"traceback,omitempty"` // populated in debug builds only
}

// SkillResultMetadata carries invocation context attached to every result.
type SkillResultMetadata struct {
	PythonVersion string `json:"python_version,omitempty"`
	Runtime       string `json:"runtime,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
	SkillID       string `json:"skill_id,omitempty"`
	Interface     string `json:"interface,omitempty"`
	InvocationID  string `json:"invocation_id,omitempty"` // UUID
}

// SkillExecutionResult is the transport-agnostic result envelope that all skill
// executions should map into before returning to the Cognitive layer.
// Individual transports (MCP, REST, internal, subprocess_python) normalize their
// native responses into this shape so callers do not need transport-specific handling.
type SkillExecutionResult struct {
	Status               string              `json:"status"`                          // e.g., "success", "error", "timeout", "killed", "truncated"
	Payload              interface{}         `json:"payload,omitempty"`               // normalized successful output
	Error                *SkillResultError   `json:"error,omitempty"`                 // structured error on failure
	DurationMS           int64               `json:"duration_ms,omitempty"`           // total wall-clock duration
	EffectsObserved      []string            `json:"effects_observed,omitempty"`      // side effects actually observed
	CompensationRequired bool                `json:"compensation_required,omitempty"` // whether follow-up compensation is recommended
	Metadata             SkillResultMetadata `json:"metadata,omitempty"`              // invocation context
}

type EffectMetadata struct {
	SideEffects []string `json:"side_effects" yaml:"side_effects"` // e.g., ["writes_external_system", "filesystem_write"]
	RiskTier    string   `json:"risk_tier" yaml:"risk_tier"`       // "low", "medium", "high"
	// Idempotency is retained for backward compatibility; new specs should prefer IdempotencyLevel.
	// When IdempotencyLevel is non-empty, it is authoritative and Idempotency is ignored.
	// The validator should hard-fail if Idempotency and IdempotencyLevel conflict semantically.
	Idempotency          bool   `json:"idempotency" yaml:"idempotency"`
	IdempotencyLevel     string `json:"idempotency_level,omitempty" yaml:"idempotency_level,omitempty"` // "idempotent", "non_idempotent", "conditional"
	Reversibility        string `json:"reversibility,omitempty" yaml:"reversibility,omitempty"`         // "reversible_internal", "compensable_external", "irreversible"
	RequiresConfirmation bool   `json:"requires_confirmation" yaml:"requires_confirmation"`
}

type SecuritySpec struct {
	Auth       []AuthMethod `json:"auth" yaml:"auth"`
	DataAccess DataAccess   `json:"data_access" yaml:"data_access"`
	Sandbox    SandboxSpec  `json:"sandbox" yaml:"sandbox"`
}

type AuthMethod struct {
	Type   string   `json:"type" yaml:"type"` // e.g., "oauth2", "api_key"
	Scopes []string `json:"scopes,omitempty" yaml:"scopes,omitempty"`
}

type DataAccess struct {
	PII     string `json:"pii" yaml:"pii"`         // "possible", "unlikely", "none"
	Secrets string `json:"secrets" yaml:"secrets"` // "read", "write", "forbidden"
}

type SandboxSpec struct {
	Required      bool     `json:"required" yaml:"required"`
	NetworkEgress []string `json:"network_egress" yaml:"network_egress"`
}

type PerformanceSpec struct {
	ExpectedP50MS int `json:"expected_p50_ms" yaml:"expected_p50_ms"`
	TimeoutMS     int `json:"timeout_ms" yaml:"timeout_ms"`
	RateLimit     struct {
		QPS   int `json:"qps" yaml:"qps"`
		Burst int `json:"burst" yaml:"burst"`
	} `json:"rate_limit" yaml:"rate_limit"`
}

type ObservabilitySpec struct {
	LogRedaction []string `json:"log_redaction" yaml:"log_redaction"`
	EmitMetrics  []string `json:"emit_metrics" yaml:"emit_metrics"`
}

type GovernanceSpec struct {
	Publisher  string   `json:"publisher" yaml:"publisher"`
	Signed     bool     `json:"signed" yaml:"signed"`
	Deprecates []string `json:"deprecates" yaml:"deprecates"`
	Replaces   []string `json:"replaces" yaml:"replaces"`
	TrustTier  string   `json:"trust_tier,omitempty" yaml:"trust_tier,omitempty"` // "builtin", "verified", "community", "local"
}

// CapabilityMetadata describes what a skill provides and depends on at the capability level.
type CapabilityMetadata struct {
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Domains     []string `json:"domains,omitempty" yaml:"domains,omitempty"`
	Provides    []string `json:"provides,omitempty" yaml:"provides,omitempty"`
	Requires    []string `json:"requires,omitempty" yaml:"requires,omitempty"`
	CommandType string   `json:"command_type,omitempty" yaml:"command_type,omitempty"` // one of query, create, update, delete, invoke, send, acquire, schedule, delegate, compose; default invoke
}

// ReliabilitySpec captures expected failure modes and retry semantics.
type ReliabilitySpec struct {
	ExpectedFailureModes []string `json:"expected_failure_modes,omitempty" yaml:"expected_failure_modes,omitempty"`
	RetryPolicy          string   `json:"retry_policy,omitempty" yaml:"retry_policy,omitempty"` // "none", "safe", "conditional"
}
