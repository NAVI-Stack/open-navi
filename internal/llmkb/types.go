package llmkb

import "time"

type BenchmarkRef struct {
	// [Governed]
	BenchmarkName string  `json:"benchmark_name" yaml:"benchmark_name"`
	URL           string  `json:"url,omitempty" yaml:"url,omitempty"`
	Score         float64 `json:"score,omitempty" yaml:"score,omitempty"`
	Unit          string  `json:"unit,omitempty" yaml:"unit,omitempty"`
}

type InternalEvalScore struct {
	// [Governed]
	Name       string    `json:"name" yaml:"name"`
	Score      float64   `json:"score" yaml:"score"`
	SampleSize int       `json:"sample_size,omitempty" yaml:"sample_size,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

type CostEstimate struct {
	// [Observed]
	InputPer1M  float64 `json:"input_per_1m,omitempty" yaml:"input_per_1m,omitempty"`
	OutputPer1M float64 `json:"output_per_1m,omitempty" yaml:"output_per_1m,omitempty"`
	Currency    string  `json:"currency,omitempty" yaml:"currency,omitempty"`
	// [Inferred]
	Tier        CostTier `json:"tier,omitempty" yaml:"tier,omitempty"`
}

type QuotaState struct {
	// [Observed]
	RateLimitState    RateLimitState `json:"rate_limit_state,omitempty" yaml:"rate_limit_state,omitempty"`
	RequestsRemaining int            `json:"requests_remaining,omitempty" yaml:"requests_remaining,omitempty"`
	TokensRemaining   int            `json:"tokens_remaining,omitempty" yaml:"tokens_remaining,omitempty"`
	ResetAt           *time.Time     `json:"reset_at,omitempty" yaml:"reset_at,omitempty"`
}

type Provenance struct {
	// [Observed]
	Source          ProvenanceSource `json:"source" yaml:"source"`
	SourceDetail    string           `json:"source_detail,omitempty" yaml:"source_detail,omitempty"`
	Confidence      float64          `json:"confidence,omitempty" yaml:"confidence,omitempty"`
	DerivationChain []string         `json:"derivation_chain,omitempty" yaml:"derivation_chain,omitempty"`
	AssertedAt      time.Time        `json:"asserted_at" yaml:"asserted_at"`
	AssertedBy      string           `json:"asserted_by,omitempty" yaml:"asserted_by,omitempty"`
	MutationHistory []MutationRecord `json:"mutation_history,omitempty" yaml:"mutation_history,omitempty"`
	// [Governed]
	FieldClass      FieldClass       `json:"field_class,omitempty" yaml:"field_class,omitempty"`
}

type MutationRecord struct {
	// [Observed]
	FieldName string           `json:"field_name" yaml:"field_name"`
	OldValue  string           `json:"old_value,omitempty" yaml:"old_value,omitempty"`
	NewValue  string           `json:"new_value,omitempty" yaml:"new_value,omitempty"`
	ChangedAt time.Time        `json:"changed_at" yaml:"changed_at"`
	Source    ProvenanceSource `json:"source" yaml:"source"`
}

type CapabilityProfile struct {
	// [Governed - curated from provider docs and manual evaluation]
	PrimaryUseCases           []UseCase         `json:"primary_use_cases,omitempty" yaml:"primary_use_cases,omitempty"`
	InteractionModes          []InteractionMode `json:"interaction_modes,omitempty" yaml:"interaction_modes,omitempty"`
	AgenticClass              AgenticClass      `json:"agentic_class" yaml:"agentic_class"`
	CodingClass               CodingClass       `json:"coding_class" yaml:"coding_class"`
	ReasoningClass            ReasoningClass    `json:"reasoning_class" yaml:"reasoning_class"`
	InstructionFollowingClass ClassLevel        `json:"instruction_following_class" yaml:"instruction_following_class"`
	ToolDisciplineClass       ClassLevel        `json:"tool_discipline_class" yaml:"tool_discipline_class"`
	SchemaReliabilityClass    ClassLevel        `json:"schema_reliability_class" yaml:"schema_reliability_class"`
	MultimodalClass           ClassLevel        `json:"multimodal_class" yaml:"multimodal_class"`
	SummaryLine               string            `json:"summary_line,omitempty" yaml:"summary_line,omitempty"`

	// [Inferred - updated autonomously by Subconscious from execution records]
	InferredTaskFit           map[TaskClass]float64   `json:"inferred_task_fit,omitempty" yaml:"inferred_task_fit,omitempty"`
	InferredTaskFitSampleSize map[TaskClass]int       `json:"inferred_task_fit_sample_size,omitempty" yaml:"inferred_task_fit_sample_size,omitempty"`
	InferredTaskFitUpdatedAt  map[TaskClass]time.Time `json:"inferred_task_fit_updated_at,omitempty" yaml:"inferred_task_fit_updated_at,omitempty"`
}

type TechnicalFeatures struct {
	// [Governed - curated from provider specs; verified via runtime introspection at Phase 2]
	ContextWindowTokens       int        `json:"context_window_tokens,omitempty" yaml:"context_window_tokens,omitempty"`
	MaxOutputTokens           int        `json:"max_output_tokens,omitempty" yaml:"max_output_tokens,omitempty"`
	SupportsTools             bool       `json:"supports_tools,omitempty" yaml:"supports_tools,omitempty"`
	SupportsParallelTools     bool       `json:"supports_parallel_tools,omitempty" yaml:"supports_parallel_tools,omitempty"`
	SupportsJSONSchema        bool       `json:"supports_json_schema,omitempty" yaml:"supports_json_schema,omitempty"`
	SupportsStreaming         bool       `json:"supports_streaming,omitempty" yaml:"supports_streaming,omitempty"`
	SupportsVision            bool       `json:"supports_vision,omitempty" yaml:"supports_vision,omitempty"`
	SupportsAudioIn           bool       `json:"supports_audio_in,omitempty" yaml:"supports_audio_in,omitempty"`
	SupportsAudioOut          bool       `json:"supports_audio_out,omitempty" yaml:"supports_audio_out,omitempty"`
	SupportsSystemPrompt      bool       `json:"supports_system_prompt,omitempty" yaml:"supports_system_prompt,omitempty"`
	SupportsFunctionCalling   bool       `json:"supports_function_calling,omitempty" yaml:"supports_function_calling,omitempty"`
	SupportsReasoningControls bool       `json:"supports_reasoning_controls,omitempty" yaml:"supports_reasoning_controls,omitempty"`
	SupportsSeedDeterminism   bool       `json:"supports_seed_determinism,omitempty" yaml:"supports_seed_determinism,omitempty"`
	SupportsFineTuning        bool       `json:"supports_fine_tuning,omitempty" yaml:"supports_fine_tuning,omitempty"`
	SupportsEmbeddings        bool       `json:"supports_embeddings,omitempty" yaml:"supports_embeddings,omitempty"`
	KnowledgeCutoffDate       *time.Time `json:"knowledge_cutoff_date,omitempty" yaml:"knowledge_cutoff_date,omitempty"`
}

type OperationalState struct {
	// [Observed - refreshed by connector probes]
	AvailableNow          bool              `json:"available_now,omitempty" yaml:"available_now,omitempty"`
	AvailabilityState     AvailabilityState `json:"availability_state" yaml:"availability_state"`
	InstalledLocally      bool              `json:"installed_locally,omitempty" yaml:"installed_locally,omitempty"`
	Downloaded            bool              `json:"downloaded,omitempty" yaml:"downloaded,omitempty"`
	RuntimeBackend        RuntimeBackend    `json:"runtime_backend" yaml:"runtime_backend"`
	ConnectorStatus       ConnectorStatus   `json:"connector_status,omitempty" yaml:"connector_status,omitempty"`
	AuthStatus            AuthStatus        `json:"auth_status,omitempty" yaml:"auth_status,omitempty"`
	HealthStatus          HealthStatus      `json:"health_status,omitempty" yaml:"health_status,omitempty"`
	LastHealthCheckAt     *time.Time        `json:"last_health_check_at,omitempty" yaml:"last_health_check_at,omitempty"`
	WarmState             WarmState         `json:"warm_state,omitempty" yaml:"warm_state,omitempty"`
	CurrentRateLimitState RateLimitState    `json:"current_rate_limit_state,omitempty" yaml:"current_rate_limit_state,omitempty"`
	ObservedP50LatencyMs  *int              `json:"observed_p50_latency_ms,omitempty" yaml:"observed_p50_latency_ms,omitempty"`
	ObservedP95LatencyMs  *int              `json:"observed_p95_latency_ms,omitempty" yaml:"observed_p95_latency_ms,omitempty"`
	CurrentCostEstimate   *CostEstimate     `json:"current_cost_estimate,omitempty" yaml:"current_cost_estimate,omitempty"`
}

type EvaluationProfile struct {
	// [Observed]
	ObservedFailurePatterns []FailurePattern        `json:"observed_failure_patterns,omitempty" yaml:"observed_failure_patterns,omitempty"`
	ObservedTaskFit         map[TaskClass]FitScore  `json:"observed_task_fit,omitempty" yaml:"observed_task_fit,omitempty"`

	// [Inferred]
	InferredReliabilityTrend ReliabilityTrend `json:"inferred_reliability_trend,omitempty" yaml:"inferred_reliability_trend,omitempty"`
	InferredStrengths        []TaskClass      `json:"inferred_strengths,omitempty" yaml:"inferred_strengths,omitempty"`
	InferredWeaknesses       []TaskClass      `json:"inferred_weaknesses,omitempty" yaml:"inferred_weaknesses,omitempty"`

	// [Governed]
	BenchmarkRefs   []BenchmarkRef      `json:"benchmark_refs,omitempty" yaml:"benchmark_refs,omitempty"`
	InternalScores  []InternalEvalScore `json:"internal_scores,omitempty" yaml:"internal_scores,omitempty"`
	EvalSummary     string              `json:"eval_summary,omitempty" yaml:"eval_summary,omitempty"`
	LastEvaluatedAt *time.Time          `json:"last_evaluated_at,omitempty" yaml:"last_evaluated_at,omitempty"`
}

type UsageStats struct {
	// [Observed] TotalExecutions stores the number of executions.
	TotalExecutions int `json:"total_executions,omitempty" yaml:"total_executions,omitempty"`
	// [Observed] SuccessRate stores normalized success ratio.
	SuccessRate float64 `json:"success_rate,omitempty" yaml:"success_rate,omitempty"`
	// [Observed] AverageLatencyMS stores mean latency.
	AverageLatencyMS float64 `json:"average_latency_ms,omitempty" yaml:"average_latency_ms,omitempty"`
	// [Observed] AverageInputTokens stores mean input tokens.
	AverageInputTokens float64 `json:"average_input_tokens,omitempty" yaml:"average_input_tokens,omitempty"`
	// [Observed] AverageOutputTokens stores mean output tokens.
	AverageOutputTokens float64 `json:"average_output_tokens,omitempty" yaml:"average_output_tokens,omitempty"`
	// [Observed] AverageCost stores mean cost per run.
	AverageCost float64 `json:"average_cost,omitempty" yaml:"average_cost,omitempty"`
	// [Observed] AccumulatedCost stores total USD cost.
	AccumulatedCost float64 `json:"accumulated_cost,omitempty" yaml:"accumulated_cost,omitempty"`

	// [Observed]
	FirstSeenAt             *time.Time              `json:"first_seen_at,omitempty" yaml:"first_seen_at,omitempty"`
	LastUsedAt              *time.Time              `json:"last_used_at,omitempty" yaml:"last_used_at,omitempty"`
	UseCountTotal           int                     `json:"use_count_total,omitempty" yaml:"use_count_total,omitempty"`
	UseCount30d             int                     `json:"use_count_30d,omitempty" yaml:"use_count_30d,omitempty"`
	SuccessRateTotal        float64                 `json:"success_rate_total,omitempty" yaml:"success_rate_total,omitempty"`
	SuccessRateByTask       map[TaskClass]float64   `json:"success_rate_by_task,omitempty" yaml:"success_rate_by_task,omitempty"`
	AverageUserRating       *float64                `json:"average_user_rating,omitempty" yaml:"average_user_rating,omitempty"`
	AbortRate               float64                 `json:"abort_rate,omitempty" yaml:"abort_rate,omitempty"`
	FallbackRate            float64                 `json:"fallback_rate,omitempty" yaml:"fallback_rate,omitempty"`
	OverrideRate            float64                 `json:"override_rate,omitempty" yaml:"override_rate,omitempty"`
	MostRecentFailureReason string                  `json:"most_recent_failure_reason,omitempty" yaml:"most_recent_failure_reason,omitempty"`
}

type RoutingCondition struct {
	// [Governed] TaskClass scopes the routing condition.
	TaskClass TaskClass `json:"task_class,omitempty" yaml:"task_class,omitempty"`
	// [Governed] UseCase scopes the routing condition by use case.
	UseCase UseCase `json:"use_case,omitempty" yaml:"use_case,omitempty"`
	// [Governed] InteractionMode scopes the routing condition by interaction mode.
	InteractionMode InteractionMode `json:"interaction_mode,omitempty" yaml:"interaction_mode,omitempty"`
	// [Governed] ToolsRequired requires tool support.
	ToolsRequired bool `json:"tools_required,omitempty" yaml:"tools_required,omitempty"`
	// [Governed] MaxCostTier caps cost.
	MaxCostTier CostTier `json:"max_cost_tier,omitempty" yaml:"max_cost_tier,omitempty"`
	// [Governed] MinAutonomyLevel requires a minimum autonomy.
	MinAutonomyLevel AutonomyLevel `json:"min_autonomy_level,omitempty" yaml:"min_autonomy_level,omitempty"`
	// [Governed]
	Property string `json:"property" yaml:"property"`
	Operator string `json:"operator" yaml:"operator"`
	Value    string `json:"value" yaml:"value"`
}

type RoutingProfile struct {
	// [Governed - Proposal required to change]
	PreferredFor            []TaskClass        `json:"preferred_for,omitempty" yaml:"preferred_for,omitempty"`
	AvoidFor                []TaskClass        `json:"avoid_for,omitempty" yaml:"avoid_for,omitempty"`
	FallbackModels          []string           `json:"fallback_models,omitempty" yaml:"fallback_models,omitempty"`
	AutonomyCeiling         AutonomyLevel      `json:"autonomy_ceiling,omitempty" yaml:"autonomy_ceiling,omitempty"`
	MaxRiskTierAllowed      RiskTier           `json:"max_risk_tier_allowed,omitempty" yaml:"max_risk_tier_allowed,omitempty"`
	RequiresConfirmationFor []TaskClass        `json:"requires_confirmation_for,omitempty" yaml:"requires_confirmation_for,omitempty"`
	TrustLevel              TrustLevel         `json:"trust_level,omitempty" yaml:"trust_level,omitempty"`
	BlockedDomains          []string           `json:"blocked_domains,omitempty" yaml:"blocked_domains,omitempty"`
	DefaultRoutingPriority  int                `json:"default_routing_priority,omitempty" yaml:"default_routing_priority,omitempty"`
	CostTier                CostTier           `json:"cost_tier,omitempty" yaml:"cost_tier,omitempty"`
	LatencyTier             LatencyTier        `json:"latency_tier,omitempty" yaml:"latency_tier,omitempty"`
	PreferredIf             []RoutingCondition `json:"preferred_if,omitempty" yaml:"preferred_if,omitempty"`

	// [Inferred - autonomous updates permitted within confidence bounds]
	InferredPreferenceScore map[TaskClass]float64 `json:"inferred_preference_score,omitempty" yaml:"inferred_preference_score,omitempty"`
	InferredAvoidanceScore  map[TaskClass]float64 `json:"inferred_avoidance_score,omitempty" yaml:"inferred_avoidance_score,omitempty"`
	InferredRoutingPriority int                   `json:"inferred_routing_priority,omitempty" yaml:"inferred_routing_priority,omitempty"`
	InferredCostTier        CostTier              `json:"inferred_cost_tier,omitempty" yaml:"inferred_cost_tier,omitempty"`
	InferredLatencyTier     LatencyTier           `json:"inferred_latency_tier,omitempty" yaml:"inferred_latency_tier,omitempty"`

	// [Governed] Promotion pipeline configuration
	PromotionThreshold     float64 `json:"promotion_threshold,omitempty" yaml:"promotion_threshold,omitempty"`
	PromotionMinSamples    int     `json:"promotion_min_samples,omitempty" yaml:"promotion_min_samples,omitempty"`
	PromotionProbationDays int     `json:"promotion_probation_days,omitempty" yaml:"promotion_probation_days,omitempty"`
}

type LLMProvider struct {
	// [Governed]
	ProviderID          string       `json:"provider_id" yaml:"provider_id"`
	CanonicalName       string       `json:"canonical_name" yaml:"canonical_name"`
	ShortName           string       `json:"short_name,omitempty" yaml:"short_name,omitempty"`
	Website             string       `json:"website,omitempty" yaml:"website,omitempty"`
	APIDocsURL          string       `json:"api_docs_url,omitempty" yaml:"api_docs_url,omitempty"`
	AuthMethod          AuthMethod   `json:"auth_method,omitempty" yaml:"auth_method,omitempty"`
	EndpointBase        string       `json:"endpoint_base,omitempty" yaml:"endpoint_base,omitempty"`
	EndpointType        EndpointType `json:"endpoint_type,omitempty" yaml:"endpoint_type,omitempty"`
	PricingModel        PricingModel `json:"pricing_model,omitempty" yaml:"pricing_model,omitempty"`
	TermsURL            string       `json:"terms_url,omitempty" yaml:"terms_url,omitempty"`
	DataRetentionPolicy string       `json:"data_retention_policy,omitempty" yaml:"data_retention_policy,omitempty"`
	TrustLevel          TrustLevel   `json:"trust_level,omitempty" yaml:"trust_level,omitempty"`
	RiskNotes           string       `json:"risk_notes,omitempty" yaml:"risk_notes,omitempty"`

	Attributes []Attribute `json:"attributes,omitempty" yaml:"attributes,omitempty"`
	Provenance Provenance  `json:"provenance" yaml:"provenance"`
	CreatedAt  time.Time   `json:"created_at" yaml:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at" yaml:"updated_at"`
}

type LLMProfile struct {
	// [Governed]
	LLMID             string            `json:"llm_id" yaml:"llm_id"`
	ProviderModelID   string            `json:"provider_model_id" yaml:"provider_model_id"`
	CanonicalName     string            `json:"canonical_name" yaml:"canonical_name"`
	Aliases           []string          `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	ProviderID        string            `json:"provider_id" yaml:"provider_id"`
	Family            string            `json:"family,omitempty" yaml:"family,omitempty"`
	Variant           string            `json:"variant,omitempty" yaml:"variant,omitempty"`
	Version           string            `json:"version,omitempty" yaml:"version,omitempty"`
	ReleaseChannel    ReleaseChannel    `json:"release_channel,omitempty" yaml:"release_channel,omitempty"`
	DeprecationStatus DeprecationStatus `json:"deprecation_status,omitempty" yaml:"deprecation_status,omitempty"`
	DeprecationDate   *time.Time        `json:"deprecation_date,omitempty" yaml:"deprecation_date,omitempty"`
	OpenWeight        bool              `json:"open_weight,omitempty" yaml:"open_weight,omitempty"`
	License           string            `json:"license,omitempty" yaml:"license,omitempty"`
	SchemaVersion     int               `json:"schema_version" yaml:"schema_version"`

	// [Governed, Inferred]
	Capabilities CapabilityProfile `json:"capabilities" yaml:"capabilities"`

	// [Governed]
	Features TechnicalFeatures `json:"features" yaml:"features"`

	// [Observed]
	OperationalState OperationalState `json:"operational_state" yaml:"operational_state"`

	// [Observed, Inferred, Governed]
	Evaluation EvaluationProfile `json:"evaluation" yaml:"evaluation"`

	// [Observed]
	UsageStats UsageStats `json:"usage_stats" yaml:"usage_stats"`

	// [Governed, Inferred]
	Routing RoutingProfile `json:"routing" yaml:"routing"`

	Attributes     []Attribute `json:"attributes,omitempty" yaml:"attributes,omitempty"`
	Provenance     Provenance  `json:"provenance" yaml:"provenance"`
	LastVerifiedAt time.Time   `json:"last_verified_at" yaml:"last_verified_at"`
	CreatedAt      time.Time   `json:"created_at" yaml:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at" yaml:"updated_at"`
}

type LLMRuntimeInstance struct {
	// [Governed]
	InstanceID     string         `json:"instance_id" yaml:"instance_id"`
	LLMID          string         `json:"llm_id" yaml:"llm_id"`
	ProviderID     string         `json:"provider_id" yaml:"provider_id"`
	RuntimeBackend RuntimeBackend `json:"runtime_backend" yaml:"runtime_backend"`
	Endpoint       string         `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	LocalPath      string         `json:"local_path,omitempty" yaml:"local_path,omitempty"`
	OllamaTag      string         `json:"ollama_tag,omitempty" yaml:"ollama_tag,omitempty"`
	AuthConfigKey  string         `json:"auth_config_key,omitempty" yaml:"auth_config_key,omitempty"`
	ConnectorID    string         `json:"connector_id,omitempty" yaml:"connector_id,omitempty"`

	// [Observed - refreshed from connector probes]
	LoadedStatus     LoadedStatus `json:"loaded_status,omitempty" yaml:"loaded_status,omitempty"`
	HealthStatus     HealthStatus `json:"health_status,omitempty" yaml:"health_status,omitempty"`
	AuthStatus       AuthStatus   `json:"auth_status,omitempty" yaml:"auth_status,omitempty"`
	LastProbeAt      *time.Time   `json:"last_probe_at,omitempty" yaml:"last_probe_at,omitempty"`
	QuotaState       QuotaState   `json:"quota_state,omitempty" yaml:"quota_state,omitempty"`
	StorageSizeBytes *int64       `json:"storage_size_bytes,omitempty" yaml:"storage_size_bytes,omitempty"`

	Attributes []Attribute `json:"attributes,omitempty" yaml:"attributes,omitempty"`
	Provenance Provenance  `json:"provenance" yaml:"provenance"`
	CreatedAt  time.Time   `json:"created_at" yaml:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at" yaml:"updated_at"`
}

type RejectionRecord struct {
	// [Observed]
	LLMID      string        `json:"llm_id" yaml:"llm_id"`
	Reason     string        `json:"reason" yaml:"reason"`
	ReasonCode RejectionCode `json:"reason_code" yaml:"reason_code"`
}

type RouterDecision struct {
	// [Observed]
	DecisionID string    `json:"decision_id" yaml:"decision_id"`
	TaskID     string    `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	TaskClass  TaskClass `json:"task_class" yaml:"task_class"`
	RiskTier   RiskTier  `json:"risk_tier,omitempty" yaml:"risk_tier,omitempty"`

	// Selection outcome
	SelectedLLMID      string   `json:"selected_llm_id,omitempty" yaml:"selected_llm_id,omitempty"`
	SelectedInstanceID string   `json:"selected_instance_id,omitempty" yaml:"selected_instance_id,omitempty"`
	FallbackChain      []string `json:"fallback_chain,omitempty" yaml:"fallback_chain,omitempty"`

	// Scoring trace
	CandidateSet   []string           `json:"candidate_set,omitempty" yaml:"candidate_set,omitempty"`
	Scores         map[string]float64 `json:"scores,omitempty" yaml:"scores,omitempty"`
	RejectedModels []RejectionRecord  `json:"rejected_models,omitempty" yaml:"rejected_models,omitempty"`

	// Constraints applied
	CapabilityFilters     []string `json:"capability_filters,omitempty" yaml:"capability_filters,omitempty"`
	AutonomyConstraints   []string `json:"autonomy_constraints,omitempty" yaml:"autonomy_constraints,omitempty"`
	CostConstraints       []string `json:"cost_constraints,omitempty" yaml:"cost_constraints,omitempty"`
	GovernanceConstraints []string `json:"governance_constraints,omitempty" yaml:"governance_constraints,omitempty"`

	Confidence float64 `json:"confidence,omitempty" yaml:"confidence,omitempty"`

	// Surfacing
	UserVisibleSummary string         `json:"user_visible_summary,omitempty" yaml:"user_visible_summary,omitempty"`
	DebugRationale     string         `json:"debug_rationale,omitempty" yaml:"debug_rationale,omitempty"`
	SurfacingLevel     SurfacingLevel `json:"surfacing_level" yaml:"surfacing_level"`

	// Linkage
	ExecutionRecordID *string `json:"execution_record_id,omitempty" yaml:"execution_record_id,omitempty"`

	Provenance Provenance `json:"provenance" yaml:"provenance"`
	CreatedAt  time.Time  `json:"created_at" yaml:"created_at"`
}

type ExecutionOutcome string

const (
	ExecutionOutcomeSuccess     ExecutionOutcome = "success"
	ExecutionOutcomeFailure     ExecutionOutcome = "failure"
	ExecutionOutcomeTimeout     ExecutionOutcome = "timeout"
	ExecutionOutcomeUserAborted ExecutionOutcome = "user_aborted"
	ExecutionOutcomeFallback    ExecutionOutcome = "fallback"
)

type LLMExecutionRecord struct {
	// [Observed]
	RecordID          string           `json:"record_id" yaml:"record_id"`
	LLMID             string           `json:"llm_id" yaml:"llm_id"`
	InstanceID        string           `json:"instance_id" yaml:"instance_id"`
	RouterDecisionID  string           `json:"router_decision_id" yaml:"router_decision_id"`
	TaskClass         TaskClass        `json:"task_class" yaml:"task_class"`
	TaskID            string           `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	ContextSizeTokens int              `json:"context_size_tokens,omitempty" yaml:"context_size_tokens,omitempty"`
	OutputSizeTokens  int              `json:"output_size_tokens,omitempty" yaml:"output_size_tokens,omitempty"`
	LatencyMs         int              `json:"latency_ms,omitempty" yaml:"latency_ms,omitempty"`
	CostUSD           *float64         `json:"cost_usd,omitempty" yaml:"cost_usd,omitempty"`
	Outcome           ExecutionOutcome `json:"outcome" yaml:"outcome"`
	FailureReason     string           `json:"failure_reason,omitempty" yaml:"failure_reason,omitempty"`
	ToolsInvoked      []string         `json:"tools_invoked,omitempty" yaml:"tools_invoked,omitempty"`
	ToolSuccessRate   float64          `json:"tool_success_rate,omitempty" yaml:"tool_success_rate,omitempty"`
	SchemaValidated   bool             `json:"schema_validated,omitempty" yaml:"schema_validated,omitempty"`
	FallbackTriggered bool             `json:"fallback_triggered,omitempty" yaml:"fallback_triggered,omitempty"`
	FallbackModelID   *string          `json:"fallback_model_id,omitempty" yaml:"fallback_model_id,omitempty"`
	UserRating        *int             `json:"user_rating,omitempty" yaml:"user_rating,omitempty"`
	UserOverride      bool             `json:"user_override,omitempty" yaml:"user_override,omitempty"`
	Notes             string           `json:"notes,omitempty" yaml:"notes,omitempty"`

	Attributes []Attribute `json:"attributes,omitempty" yaml:"attributes,omitempty"`
	Provenance Provenance  `json:"provenance" yaml:"provenance"`
	CreatedAt  time.Time   `json:"created_at" yaml:"created_at"`
}

type EvalType string

const (
	EvalTypeInternal          EvalType = "internal"
	EvalTypeBenchmarkImport   EvalType = "benchmark_import"
	EvalTypeReflectionDerived EvalType = "reflection_derived"
	EvalTypeManual            EvalType = "manual"
)

type LLMEvaluation struct {
	// [Observed, Inferred, Governed]
	EvalID        string       `json:"eval_id" yaml:"eval_id"`
	LLMID         string       `json:"llm_id" yaml:"llm_id"`
	EvalType      EvalType     `json:"eval_type" yaml:"eval_type"`
	Assessor      string       `json:"assessor,omitempty" yaml:"assessor,omitempty"`
	TaskClass     TaskClass    `json:"task_class" yaml:"task_class"`
	Score         float64      `json:"score" yaml:"score"`
	Confidence    float64      `json:"confidence" yaml:"confidence"`
	SampleSize    *int         `json:"sample_size,omitempty" yaml:"sample_size,omitempty"`
	Remarks       string       `json:"remarks,omitempty" yaml:"remarks,omitempty"`
	EvidenceLinks []string     `json:"evidence_links,omitempty" yaml:"evidence_links,omitempty"`

	FieldClass FieldClass  `json:"field_class" yaml:"field_class"`
	Attributes []Attribute `json:"attributes,omitempty" yaml:"attributes,omitempty"`
	Provenance Provenance  `json:"provenance" yaml:"provenance"`
	CreatedAt  time.Time   `json:"created_at" yaml:"created_at"`
	ValidUntil *time.Time  `json:"valid_until,omitempty" yaml:"valid_until,omitempty"`
}

type RoutingProposalItem struct {
	// [Governed, Observed]
	ProposalID           string         `json:"proposal_id" yaml:"proposal_id"`
	LLMID                string         `json:"llm_id" yaml:"llm_id"`
	ProposedChange       string         `json:"proposed_change" yaml:"proposed_change"`
	AffectedField        string         `json:"affected_field" yaml:"affected_field"`
	CurrentValue         any            `json:"current_value,omitempty" yaml:"current_value,omitempty"`
	ProposedValue        any            `json:"proposed_value,omitempty" yaml:"proposed_value,omitempty"`
	InferredScore        float64        `json:"inferred_score,omitempty" yaml:"inferred_score,omitempty"`
	SampleSize           int            `json:"sample_size,omitempty" yaml:"sample_size,omitempty"`
	ProbationElapsedDays int            `json:"probation_elapsed_days,omitempty" yaml:"probation_elapsed_days,omitempty"`
	RecentSuccessRate    float64        `json:"recent_success_rate,omitempty" yaml:"recent_success_rate,omitempty"`
	FallbackRateDelta    float64        `json:"fallback_rate_delta,omitempty" yaml:"fallback_rate_delta,omitempty"`
	RoutingImpact        string         `json:"routing_impact,omitempty" yaml:"routing_impact,omitempty"`
	RiskAssessment       string         `json:"risk_assessment,omitempty" yaml:"risk_assessment,omitempty"`
	EvidenceRecordIDs    []string       `json:"evidence_record_ids,omitempty" yaml:"evidence_record_ids,omitempty"`
	Status               ProposalStatus `json:"status" yaml:"status"`
	CreatedAt            time.Time      `json:"created_at" yaml:"created_at"`
	ResolvedAt           *time.Time     `json:"resolved_at,omitempty" yaml:"resolved_at,omitempty"`
	ResolvedBy           string         `json:"resolved_by,omitempty" yaml:"resolved_by,omitempty"`
}
