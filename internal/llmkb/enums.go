package llmkb

import (
	"fmt"
	"strings"
)

func normalizeEnumValue(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func inSet[T ~string](value T, allowed map[T]struct{}) bool {
	_, ok := allowed[value]
	return ok
}

func parseEnum[T ~string](raw string, allowed map[T]struct{}, name string) (T, error) {
	normalized := T(normalizeEnumValue(raw))
	if _, ok := allowed[normalized]; !ok {
		return "", fmt.Errorf("llmkb: invalid %s %q", name, raw)
	}
	return normalized, nil
}

type FieldClass string

const (
	FieldClassObserved FieldClass = "observed"
	FieldClassInferred FieldClass = "inferred"
	FieldClassGoverned FieldClass = "governed"
)

var fieldClassValues = map[FieldClass]struct{}{
	FieldClassObserved: {},
	FieldClassInferred: {},
	FieldClassGoverned: {},
}

func (v FieldClass) IsValid() bool { return inSet(v, fieldClassValues) }
func ParseFieldClass(raw string) (FieldClass, error) {
	return parseEnum(raw, fieldClassValues, "field_class")
}

type AgenticClass string

const (
	AgenticClassNone        AgenticClass = "none"
	AgenticClassConstrained AgenticClass = "constrained"
	AgenticClassCapable     AgenticClass = "capable"
	AgenticClassStrong      AgenticClass = "strong"
)

var agenticClassValues = map[AgenticClass]struct{}{
	AgenticClassNone:        {},
	AgenticClassConstrained: {},
	AgenticClassCapable:     {},
	AgenticClassStrong:      {},
}

func (v AgenticClass) IsValid() bool { return inSet(v, agenticClassValues) }
func ParseAgenticClass(raw string) (AgenticClass, error) {
	return parseEnum(raw, agenticClassValues, "agentic_class")
}

type CodingClass string

const (
	CodingClassNone      CodingClass = "none"
	CodingClassBasic     CodingClass = "basic"
	CodingClassCompetent CodingClass = "competent"
	CodingClassStrong    CodingClass = "strong"
	CodingClassElite     CodingClass = "elite"
)

var codingClassValues = map[CodingClass]struct{}{
	CodingClassNone:      {},
	CodingClassBasic:     {},
	CodingClassCompetent: {},
	CodingClassStrong:    {},
	CodingClassElite:     {},
}

func (v CodingClass) IsValid() bool { return inSet(v, codingClassValues) }
func ParseCodingClass(raw string) (CodingClass, error) {
	return parseEnum(raw, codingClassValues, "coding_class")
}

type ReasoningClass string

const (
	ReasoningClassNone        ReasoningClass = "none"
	ReasoningClassBasic       ReasoningClass = "basic"
	ReasoningClassCompetent   ReasoningClass = "competent"
	ReasoningClassStrong      ReasoningClass = "strong"
	ReasoningClassExceptional ReasoningClass = "exceptional"
)

var reasoningClassValues = map[ReasoningClass]struct{}{
	ReasoningClassNone:        {},
	ReasoningClassBasic:       {},
	ReasoningClassCompetent:   {},
	ReasoningClassStrong:      {},
	ReasoningClassExceptional: {},
}

func (v ReasoningClass) IsValid() bool { return inSet(v, reasoningClassValues) }
func ParseReasoningClass(raw string) (ReasoningClass, error) {
	return parseEnum(raw, reasoningClassValues, "reasoning_class")
}

type ClassLevel string

const (
	ClassLevelWeak        ClassLevel = "weak"
	ClassLevelFair        ClassLevel = "fair"
	ClassLevelCompetent   ClassLevel = "competent"
	ClassLevelGood        ClassLevel = "good"
	ClassLevelStrong      ClassLevel = "strong"
	ClassLevelExceptional ClassLevel = "exceptional"
)

var classLevelValues = map[ClassLevel]struct{}{
	ClassLevelWeak:        {},
	ClassLevelFair:        {},
	ClassLevelCompetent:   {},
	ClassLevelGood:        {},
	ClassLevelStrong:      {},
	ClassLevelExceptional: {},
}

func (v ClassLevel) IsValid() bool { return inSet(v, classLevelValues) }
func ParseClassLevel(raw string) (ClassLevel, error) {
	return parseEnum(raw, classLevelValues, "class_level")
}

type UseCase string

const (
	UseCaseGeneralChat      UseCase = "general_chat"
	UseCaseCodingAssistant  UseCase = "coding_assistant"
	UseCaseResearch         UseCase = "research"
	UseCasePlanning         UseCase = "planning"
	UseCaseAutomation       UseCase = "automation"
	UseCaseStructuredOutput UseCase = "structured_output"
	UseCaseLongContext      UseCase = "long_context"
	UseCaseMultimodal       UseCase = "multimodal"
	UseCaseEvaluation       UseCase = "evaluation"
)

var useCaseValues = map[UseCase]struct{}{
	UseCaseGeneralChat:      {},
	UseCaseCodingAssistant:  {},
	UseCaseResearch:         {},
	UseCasePlanning:         {},
	UseCaseAutomation:       {},
	UseCaseStructuredOutput: {},
	UseCaseLongContext:      {},
	UseCaseMultimodal:       {},
	UseCaseEvaluation:       {},
}

func (v UseCase) IsValid() bool { return inSet(v, useCaseValues) }
func ParseUseCase(raw string) (UseCase, error) {
	return parseEnum(raw, useCaseValues, "use_case")
}

type TaskClass string

const (
	TaskClassChat             TaskClass = "chat"
	TaskClassLightweight      TaskClass = "lightweight"
	TaskClassCoding           TaskClass = "coding"
	TaskClassReasoning        TaskClass = "reasoning"
	TaskClassAgentic          TaskClass = "agentic"
	TaskClassResearch         TaskClass = "research"
	TaskClassPlanning         TaskClass = "planning"
	TaskClassSummarization    TaskClass = "summarization"
	TaskClassClassification   TaskClass = "classification"
	TaskClassExtraction       TaskClass = "extraction"
	TaskClassStructuredOutput TaskClass = "structured_output"
	TaskClassLongContext      TaskClass = "long_context"
	TaskClassEvaluation       TaskClass = "evaluation"
)

var taskClassValues = map[TaskClass]struct{}{
	TaskClassChat:             {},
	TaskClassLightweight:      {},
	TaskClassCoding:           {},
	TaskClassReasoning:        {},
	TaskClassAgentic:          {},
	TaskClassResearch:         {},
	TaskClassPlanning:         {},
	TaskClassSummarization:    {},
	TaskClassClassification:   {},
	TaskClassExtraction:       {},
	TaskClassStructuredOutput: {},
	TaskClassLongContext:      {},
	TaskClassEvaluation:       {},
}

func (v TaskClass) IsValid() bool { return inSet(v, taskClassValues) }
func ParseTaskClass(raw string) (TaskClass, error) {
	return parseEnum(raw, taskClassValues, "task_class")
}

type InteractionMode string

const (
	InteractionModeConversational   InteractionMode = "conversational"
	InteractionModeToolUsing        InteractionMode = "tool_using"
	InteractionModeStructuredOutput InteractionMode = "structured_output"
	InteractionModeLongContext      InteractionMode = "long_context"
	InteractionModeStreaming        InteractionMode = "streaming"
	InteractionModeAsync            InteractionMode = "async"
)

var interactionModeValues = map[InteractionMode]struct{}{
	InteractionModeConversational:   {},
	InteractionModeToolUsing:        {},
	InteractionModeStructuredOutput: {},
	InteractionModeLongContext:      {},
	InteractionModeStreaming:        {},
	InteractionModeAsync:            {},
}

func (v InteractionMode) IsValid() bool { return inSet(v, interactionModeValues) }
func ParseInteractionMode(raw string) (InteractionMode, error) {
	return parseEnum(raw, interactionModeValues, "interaction_mode")
}

type AuthMethod string
type EndpointType string
type PricingModel string
type AvailabilityState string
type RuntimeBackend string
type ConnectorStatus string
type AuthStatus string
type HealthStatus string
type WarmState string
type RateLimitState string
type LoadedStatus string
type ReleaseChannel string
type DeprecationStatus string
type HostingMode string
type ReliabilityTrend string
type SurfacingLevel string
type ProposalStatus string
type CostTier string
type LatencyTier string
type RiskTier string
type TrustLevel string
type AutonomyLevel string
type FitScore string
type RejectionCode string
type FailurePattern string
type ProvenanceSource string

const (
	AuthMethodAPIKey      AuthMethod = "api_key"
	AuthMethodOAuth       AuthMethod = "oauth"
	AuthMethodNone        AuthMethod = "none"
	AuthMethodBearerToken AuthMethod = "bearer_token"
	AuthMethodCustom      AuthMethod = "custom"

	EndpointTypeOpenAICompat EndpointType = "openai_compat"
	EndpointTypeAnthropic    EndpointType = "anthropic"
	EndpointTypeGoogle       EndpointType = "google"
	EndpointTypeCustom       EndpointType = "custom"
	EndpointTypeOllamaLocal  EndpointType = "ollama_local"

	PricingModelPerToken     PricingModel = "per_token"
	PricingModelPerRequest   PricingModel = "per_request"
	PricingModelSubscription PricingModel = "subscription"
	PricingModelFree         PricingModel = "free"
	PricingModelSelfHosted   PricingModel = "self_hosted"

	AvailabilityStateUnknown     AvailabilityState = "unknown"
	AvailabilityStateAvailable   AvailabilityState = "available"
	AvailabilityStateUnavailable AvailabilityState = "unavailable"
	AvailabilityStateDegraded    AvailabilityState = "degraded"

	RuntimeBackendAPI        RuntimeBackend = "api"
	RuntimeBackendOllama     RuntimeBackend = "ollama"
	RuntimeBackendOpenAI     RuntimeBackend = "openai"
	RuntimeBackendAnthropic  RuntimeBackend = "anthropic"
	RuntimeBackendOpenRouter RuntimeBackend = "openrouter"
	RuntimeBackendCustom     RuntimeBackend = "custom"

	ConnectorStatusUnknown    ConnectorStatus = "unknown"
	ConnectorStatusConfigured ConnectorStatus = "configured"
	ConnectorStatusConnected  ConnectorStatus = "connected"
	ConnectorStatusFailed     ConnectorStatus = "failed"

	AuthStatusUnknown         AuthStatus = "unknown"
	AuthStatusUnauthenticated AuthStatus = "unauthenticated"
	AuthStatusAuthenticated   AuthStatus = "authenticated"
	AuthStatusExpired         AuthStatus = "expired"

	HealthStatusUnknown  HealthStatus = "unknown"
	HealthStatusHealthy  HealthStatus = "healthy"
	HealthStatusDegraded HealthStatus = "degraded"
	HealthStatusFailing  HealthStatus = "failing"

	WarmStateCold    WarmState = "cold"
	WarmStateWarming WarmState = "warming"
	WarmStateWarm    WarmState = "warm"

	RateLimitStateUnknown RateLimitState = "unknown"
	RateLimitStateOpen    RateLimitState = "open"
	RateLimitStateNear    RateLimitState = "near"
	RateLimitStateLimited RateLimitState = "limited"

	LoadedStatusUnknown   LoadedStatus = "unknown"
	LoadedStatusNotLoaded LoadedStatus = "not_loaded"
	LoadedStatusLoaded    LoadedStatus = "loaded"
	LoadedStatusEvicted   LoadedStatus = "evicted"

	ReleaseChannelStable       ReleaseChannel = "stable"
	ReleaseChannelPreview      ReleaseChannel = "preview"
	ReleaseChannelExperimental ReleaseChannel = "experimental"

	DeprecationStatusActive       DeprecationStatus = "active"
	DeprecationStatusDeprecated   DeprecationStatus = "deprecated"
	DeprecationStatusSunsetting   DeprecationStatus = "sunsetting"
	DeprecationStatusDiscontinued DeprecationStatus = "discontinued"

	HostingModeCloudAPI     HostingMode = "cloud_api"
	HostingModeLocal        HostingMode = "local"
	HostingModeSelfHosted   HostingMode = "self_hosted"
	HostingModeCloudManaged HostingMode = "cloud_managed"

	ReliabilityTrendUnknown   ReliabilityTrend = "unknown"
	ReliabilityTrendImproving ReliabilityTrend = "improving"
	ReliabilityTrendStable    ReliabilityTrend = "stable"
	ReliabilityTrendDeclining ReliabilityTrend = "declining"

	SurfacingLevelHidden   SurfacingLevel = "hidden"
	SurfacingLevelInternal SurfacingLevel = "internal"
	SurfacingLevelOwner    SurfacingLevel = "owner"
	SurfacingLevelDefault  SurfacingLevel = "default"

	ProposalStatusPending  ProposalStatus = "pending"
	ProposalStatusApproved ProposalStatus = "approved"
	ProposalStatusRejected ProposalStatus = "rejected"
	ProposalStatusApplied  ProposalStatus = "applied"

	CostTierFree      CostTier = "free"
	CostTierCheap     CostTier = "cheap"
	CostTierModerate  CostTier = "moderate"
	CostTierExpensive CostTier = "expensive"

	LatencyTierLow    LatencyTier = "low"
	LatencyTierMedium LatencyTier = "medium"
	LatencyTierHigh   LatencyTier = "high"

	RiskTierLow      RiskTier = "low"
	RiskTierMedium   RiskTier = "medium"
	RiskTierHigh     RiskTier = "high"
	RiskTierCritical RiskTier = "critical"

	TrustLevelUnknown TrustLevel = "unknown"
	TrustLevelLow     TrustLevel = "low"
	TrustLevelMedium  TrustLevel = "medium"
	TrustLevelHigh    TrustLevel = "high"

	AutonomyLevelChat       AutonomyLevel = "chat"
	AutonomyLevelAssistive  AutonomyLevel = "assistive"
	AutonomyLevelAutonomous AutonomyLevel = "autonomous"

	FitScorePoor   FitScore = "poor"
	FitScoreFair   FitScore = "fair"
	FitScoreGood   FitScore = "good"
	FitScoreStrong FitScore = "strong"
	FitScoreIdeal  FitScore = "ideal"

	RejectionCodeUnsupportedTask  RejectionCode = "unsupported_task"
	RejectionCodeToolsUnavailable RejectionCode = "tools_unavailable"
	RejectionCodeOverBudget       RejectionCode = "over_budget"
	RejectionCodePolicyBlocked    RejectionCode = "policy_blocked"
	RejectionCodeUnavailable      RejectionCode = "unavailable"
	RejectionCodeUnhealthy        RejectionCode = "unhealthy"
	RejectionCodeContextOverflow  RejectionCode = "context_overflow"
	RejectionCodeAuthentication   RejectionCode = "authentication"
	RejectionCodeRateLimited      RejectionCode = "rate_limited"
	RejectionCodeDeprecation      RejectionCode = "deprecation"

	FailurePatternHallucination    FailurePattern = "hallucination"
	FailurePatternToolAvoidance    FailurePattern = "tool_avoidance"
	FailurePatternSchemaViolation  FailurePattern = "schema_violation"
	FailurePatternTimeout          FailurePattern = "timeout"
	FailurePatternRateLimit        FailurePattern = "rate_limit"
	FailurePatternAuthFailure      FailurePattern = "auth_failure"
	FailurePatternFormattingDrift  FailurePattern = "formatting_drift"
	FailurePatternPromptInjection  FailurePattern = "prompt_injection"
	FailurePatternInstructionDrift FailurePattern = "instruction_drift"
	FailurePatternLowRecall        FailurePattern = "low_recall"
	FailurePatternLowPrecision     FailurePattern = "low_precision"

	ProvenanceSourceVendorDoc   ProvenanceSource = "vendor_doc"
	ProvenanceSourceCatalog     ProvenanceSource = "catalog"
	ProvenanceSourceCuratedSeed ProvenanceSource = "curated_seed"
	ProvenanceSourceProbe       ProvenanceSource = "probe"
	ProvenanceSourceExecution   ProvenanceSource = "execution"
	ProvenanceSourceReflection  ProvenanceSource = "reflection"
	ProvenanceSourceManual      ProvenanceSource = "manual"
	ProvenanceSourceImported    ProvenanceSource = "imported"
)
