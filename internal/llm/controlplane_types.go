package llm

import "time"

// ProviderKind classifies the deployment model of a provider.
type ProviderKind string

const (
	ProviderKindLocal ProviderKind = "local"
	ProviderKindCloud ProviderKind = "cloud"
	ProviderKindProxy ProviderKind = "proxy"
)

// ProviderDescriptor is the operator-facing view of a configured provider.
type ProviderDescriptor struct {
	Key          string          `json:"key"`
	DisplayName  string          `json:"display_name"`
	Kind         ProviderKind    `json:"kind"`
	Enabled      bool            `json:"enabled"`
	Configured   bool            `json:"configured"`
	Healthy      *bool           `json:"healthy,omitempty"`
	Capabilities []string        `json:"capabilities"`
	ModelCount   int             `json:"model_count"`
	Policies     *ProviderPolicy `json:"policies,omitempty"`
}

// ProviderConfigRequest is the input for ConfigureProvider.
type ProviderConfigRequest struct {
	Provider string
	APIKey   string
	Model    string
	Endpoint string // used by Ollama for the base URL
}

// ProviderHealth is a point-in-time health snapshot.
type ProviderHealth struct {
	Provider   string    `json:"provider"`
	Healthy    bool      `json:"healthy"`
	Latency    int       `json:"latency_ms"`
	Message    string    `json:"message,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
	ModelCount int       `json:"model_count,omitempty"`
}

// OperationStatus tracks the lifecycle of a provider operation.
type OperationStatus string

const (
	OperationStatusPending   OperationStatus = "pending"
	OperationStatusRunning   OperationStatus = "running"
	OperationStatusCompleted OperationStatus = "completed"
	OperationStatusFailed    OperationStatus = "failed"
)

// ProviderOperation tracks an async provider action (pull, delete, warm, etc.).
type ProviderOperation struct {
	ID          string          `json:"id"`
	Provider    string          `json:"provider"`
	Action      string          `json:"action"` // "pull","delete","warm","copy","create"
	Model       string          `json:"model"`
	Status      OperationStatus `json:"status"`
	Progress    float64         `json:"progress,omitempty"` // 0.0-1.0
	Message     string          `json:"message,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Error       string          `json:"error,omitempty"`
}

// ProviderActionRequest is the input for ExecuteProviderAction.
type ProviderActionRequest struct {
	Provider string `json:"provider"`
	Action   string `json:"action"` // "pull","delete","warm","copy","create"
	Model    string `json:"model"`
	Target   string `json:"target,omitempty"` // for copy: destination name
}

// ModelDescriptor is the control-plane view of a model (richer than LLMModelInfo).
type ModelDescriptor struct {
	Name          string    `json:"name"`
	Size          int64     `json:"size,omitempty"`
	Family        string    `json:"family,omitempty"`
	ParameterSize string    `json:"parameter_size,omitempty"`
	QuantLevel    string    `json:"quantization_level,omitempty"`
	ModifiedAt    time.Time `json:"modified_at,omitempty"`
	Digest        string    `json:"digest,omitempty"`
}

// RunningModel represents a model currently loaded in memory.
type RunningModel struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size,omitempty"`
	VRAM      int64     `json:"vram_size,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// CreateModelRequest for creating derived models (Modelfile).
type CreateModelRequest struct {
	Name      string `json:"name"`
	Modelfile string `json:"modelfile"`
}

// ProviderPolicy is the operator-configured policy for a provider.
type ProviderPolicy struct {
	Enabled            bool `json:"enabled" yaml:"enabled"`
	LocalFirst         bool `json:"local_first,omitempty" yaml:"local_first"`
	AllowPull          bool `json:"allow_pull,omitempty" yaml:"allow_pull"`
	AllowDelete        bool `json:"allow_delete,omitempty" yaml:"allow_delete"`
	AutoWarm           bool `json:"auto_warm,omitempty" yaml:"auto_warm"`
	AllowPaidRemote    bool `json:"allow_paid_remote,omitempty" yaml:"allow_paid_remote"`
	AllowFallbackLocal bool `json:"allow_fallback_from_local,omitempty" yaml:"allow_fallback_from_local"`
}
