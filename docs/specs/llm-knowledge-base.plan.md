# NAVI LLM Knowledge Base — Architecture Specification

> [!NOTE]
> Part of the [NAVI Systems Map](../architecture/navi-systems-map.md).


**Status:** Draft v3.0 — Implementation-Ready
**Scope:** World Model entity design, schema definitions, controlled vocabularies, field classification, storage design, ingestion paths, router integration, governance contracts, promotion UX.

---

## 1. Overview

The LLM Knowledge Base (LLM-KB) is a governed set of World Model entities responsible for holding everything NAVI knows about language models — what they are, what they can do, whether they are reachable, how they have performed, and whether NAVI should use them.

It is **not** a passive catalog. It is an active reasoning substrate that drives model selection, fallback logic, cost-aware routing, autonomy limits, and user-facing explanations.

### Three Classes of Truth

| Class | Examples | NAVI Layer |
|---|---|---|
| **Curated facts** | context window, tool support, license | `Knowledge` entities |
| **Operational state** | installed, reachable, quota state | Derived state from connector probes |
| **Learned observations** | success rate by task, failure patterns | `History` → promoted to inferred `Knowledge` |

---

## 2. Architectural Position

### Write Path

The Capability Layer never writes directly to the World Model.

```
Connector (Capability Layer)
    → probes reality (health checks, /models introspection, runtime state)
    → returns structured results to Cognitive Layer

Cognitive Layer
    → normalizes connector results
    → determines what becomes durable state
    → writes to World Model

World Model
    → owns all persistent, queryable entity state
    → is the single source of truth for routing, evaluation, and reasoning
```

### What the Connector Owns (ephemeral)

- Transport details and auth mechanics
- Active health check execution and raw introspection responses
- Temporary circuit-breaker state
- Retry logic and connection pooling

### What the World Model Owns (persistent, queryable)

- Normalized runtime facts NAVI reasons over
- Canonical runtime availability state and last verified health snapshot
- Installed / downloaded / warm / available status
- Observed latency summaries
- Association between a logical model and its concrete runtime instances

---

## 3. Field Classification Taxonomy

Every field in every entity is classified into one of three mutation classes. This classification is enforced at the schema level.

### Observed
Direct telemetry and execution-derived facts. Autonomous updates by Subconscious consolidation permitted without governance gate.

### Inferred
Learned conclusions and ranking signals derived from Observed data. Autonomous updates permitted within bounds (confidence threshold + sample size requirements).

### Governed
Fields that affect behavior, risk, autonomy, or user expectations. **Changes require a Proposal** if they would materially alter what NAVI does autonomously or which models are eligible for consequential tasks.

If a field's classification is ambiguous, treat it as Governed by default.

---

## 4. World Model Entity Set

Six entity types.

```
LLMProvider
    └── LLMProfile (many per provider)
            └── LLMRuntimeInstance (many per profile)
RouterDecision (pre-execution; linked to LLMProfile + LLMRuntimeInstance)
    └── LLMExecutionRecord (post-execution; linked to RouterDecision)
LLMEvaluation (linked to LLMProfile; curated or reflection-derived)
```

---

## 5. Entity Schemas

### 5.1 `LLMProvider`

```go
type LLMProvider struct {
    // [Governed]
    ProviderID          string
    CanonicalName       string
    ShortName           string
    Website             string
    APIDocsURL          string
    AuthMethod          AuthMethod      // api_key | oauth | none | bearer_token | custom
    EndpointBase        string
    EndpointType        EndpointType    // openai_compat | anthropic | google | custom | ollama_local
    PricingModel        PricingModel    // per_token | per_request | subscription | free | self_hosted
    TermsURL            string
    DataRetentionPolicy string
    TrustLevel          TrustLevel      // trusted | standard | restricted | unverified
    RiskNotes           string

    Attributes  []Attribute
    Provenance  Provenance
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

---

### 5.2 `LLMProfile`

The canonical model profile. One record per logical model.

```go
type LLMProfile struct {
    // A. Identity [Governed]
    LLMID             string
    ProviderModelID   string
    CanonicalName     string
    Aliases           []string
    ProviderID        string
    Family            string
    Variant           string
    Version           string
    ReleaseChannel    ReleaseChannel      // stable | preview | experimental | deprecated | retired
    DeprecationStatus DeprecationStatus   // active | deprecated | retired | announced
    DeprecationDate   *time.Time
    OpenWeight        bool
    License           string
    SchemaVersion     int

    // B. Capability Profile [Governed — curated; Inferred — scored confidence]
    Capabilities      CapabilityProfile

    // C. Technical Features [Governed]
    Features          TechnicalFeatures

    // D. Operational State [Observed — refreshed by connector probes]
    OperationalState  OperationalState

    // E. Evaluation Profile [Observed + Inferred]
    Evaluation        EvaluationProfile

    // F. Usage Telemetry [Observed — derived from ExecutionRecords, never directly authored]
    UsageStats        UsageStats

    // G. Routing Profile [split — see RoutingProfile]
    Routing           RoutingProfile

    Attributes  []Attribute
    Provenance  Provenance
    LastVerifiedAt time.Time
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

#### 5.2.1 `CapabilityProfile`

```go
type CapabilityProfile struct {
    // [Governed — curated from provider docs and manual evaluation]
    PrimaryUseCases           []UseCase
    InteractionModes          []InteractionMode
    AgenticClass              AgenticClass
    CodingClass               CodingClass
    ReasoningClass            ReasoningClass
    InstructionFollowingClass ClassLevel
    ToolDisciplineClass       ClassLevel
    SchemaReliabilityClass    ClassLevel
    MultimodalClass           ClassLevel
    SummaryLine               string

    // [Inferred — updated autonomously by Subconscious from execution records]
    InferredTaskFit           map[TaskClass]float32   // 0.0–1.0 scored signal, NOT routing label
    InferredTaskFitSampleSize map[TaskClass]int
    InferredTaskFitUpdatedAt  map[TaskClass]time.Time
}
```

**Note:** `InferredTaskFit` is a scoring signal only. It feeds router scoring but does not directly modify `RoutingProfile.PreferredFor`. The promotion pipeline governs escalation from inferred score to canonical routing label.

#### 5.2.2 `TechnicalFeatures`

```go
// [Governed — curated from provider specs; verified via runtime introspection at Phase 2]
type TechnicalFeatures struct {
    ContextWindowTokens       int
    MaxOutputTokens           int
    SupportsTools             bool
    SupportsParallelTools     bool
    SupportsJSONSchema        bool
    SupportsStreaming          bool
    SupportsVision            bool
    SupportsAudioIn           bool
    SupportsAudioOut          bool
    SupportsSystemPrompt      bool
    SupportsFunctionCalling   bool
    SupportsReasoningControls bool
    SupportsSeedDeterminism   bool
    SupportsFineTuning        bool
    SupportsEmbeddings        bool
    KnowledgeCutoffDate       *time.Time
}
```

#### 5.2.3 `OperationalState`

Fully **Observed**. Empty or `unknown` defaults in seed records.

```go
type OperationalState struct {
    AvailableNow          bool
    AvailabilityState     AvailabilityState  // available | degraded | unavailable | unknown
    InstalledLocally       bool
    Downloaded             bool
    RuntimeBackend         RuntimeBackend
    ConnectorStatus        ConnectorStatus   // connected | auth_error | unreachable | not_configured
    AuthStatus             AuthStatus        // ok | missing | expired | invalid
    HealthStatus           HealthStatus      // healthy | degraded | unhealthy | unknown
    LastHealthCheckAt      *time.Time
    WarmState              WarmState         // warm | cold | unknown
    CurrentRateLimitState  RateLimitState    // ok | throttled | quota_exceeded | unknown
    ObservedP50LatencyMs   *int
    ObservedP95LatencyMs   *int
    CurrentCostEstimate    *CostEstimate
}
```

#### 5.2.4 `EvaluationProfile`

```go
type EvaluationProfile struct {
    // [Observed — directly from execution records]
    ObservedFailurePatterns []FailurePattern
    ObservedTaskFit         map[TaskClass]FitScore  // poor | fair | good | excellent

    // [Inferred — derived by Subconscious consolidation]
    InferredReliabilityTrend ReliabilityTrend        // improving | stable | degrading | unknown
    InferredStrengths        []TaskClass
    InferredWeaknesses       []TaskClass

    // [Governed — curated or manually authored]
    BenchmarkRefs            []BenchmarkRef
    InternalScores           []InternalEvalScore
    EvalSummary              string
    LastEvaluatedAt          *time.Time
}
```

#### 5.2.5 `UsageStats`

Fully **Observed**. Materialized by consolidation. Never directly authored.

```go
type UsageStats struct {
    FirstSeenAt             *time.Time
    LastUsedAt              *time.Time
    UseCountTotal           int
    UseCount30d             int
    SuccessRateTotal        float32
    SuccessRateByTask       map[TaskClass]float32
    AverageUserRating       *float32
    AbortRate               float32
    FallbackRate            float32
    OverrideRate            float32
    MostRecentFailureReason string
}
```

#### 5.2.6 `RoutingProfile`

Split across classification tiers. This is where the governance boundary is enforced.

```go
type RoutingProfile struct {
    // [Governed — Proposal required to change]
    // These fields directly control routing behavior.
    PreferredFor            []TaskClass
    AvoidFor                []TaskClass
    FallbackModels          []string            // ordered LLMIDs
    AutonomyCeiling         AutonomyLevel
    MaxRiskTierAllowed      RiskTier
    RequiresConfirmationFor []TaskClass
    TrustLevel              TrustLevel
    BlockedDomains          []string
    DefaultRoutingPriority  int
    CostTier                CostTier
    LatencyTier             LatencyTier
    PreferredIf             []RoutingCondition  // Phase 1: simple tags; Phase 4: CEL

    // [Inferred — autonomous updates permitted within confidence bounds]
    // Scoring signals. Feed the router but do not override Governed fields.
    InferredPreferenceScore  map[TaskClass]float32
    InferredAvoidanceScore   map[TaskClass]float32
    InferredRoutingPriority  int
    InferredCostTier         CostTier
    InferredLatencyTier      LatencyTier

    // Promotion pipeline configuration [Governed]
    // When InferredPreferenceScore[task] crosses PromotionThreshold AND
    // sample size exceeds PromotionMinSamples AND probation has elapsed,
    // Subconscious MUST emit a Proposal to add the task to PreferredFor.
    // Direct mutation of PreferredFor by Subconscious is prohibited.
    PromotionThreshold      float32             // default: 0.85
    PromotionMinSamples     int                 // default: 50
    PromotionProbationDays  int                 // default: 7
}
```

---

### 5.3 `LLMRuntimeInstance`

First-class World Model entity. One `LLMProfile` can have many runtime instances. Not connector-internal state.

```go
type LLMRuntimeInstance struct {
    // [Governed]
    InstanceID     string
    LLMID          string          // FK → LLMProfile
    ProviderID     string          // FK → LLMProvider
    RuntimeBackend RuntimeBackend
    Endpoint       string
    LocalPath      string
    OllamaTag      string
    AuthConfigKey  string          // reference to secret store; never the secret itself
    ConnectorID    string

    // [Observed — refreshed from connector probes]
    LoadedStatus     LoadedStatus
    HealthStatus     HealthStatus
    AuthStatus       AuthStatus
    LastProbeAt      *time.Time
    QuotaState       QuotaState
    StorageSizeBytes *int64

    Attributes []Attribute
    Provenance Provenance
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

---

### 5.4 `RouterDecision`

**Distinct entity type.** Not a subtype of execution outcome.

A `RouterDecision` is a **pre-execution event** — it can and does exist when execution never starts, fails immediately, or is rejected before any model is invoked. Its lifecycle is independent of any execution record.

```
RouterDecision         answers: what was selected? what was rejected? why? under what constraints?
LLMExecutionRecord     answers: what happened after attempting to execute with that decision?
```

They are linked by `router_decision_id` / `execution_record_id`, but causally distinct. Burying `RouterDecision` as an execution outcome subtype would distort this causality and break cases where routing resolves but execution never occurs (auth missing, governance rejection, user-initiated inspection before execution).

```go
type RouterDecision struct {
    // [Observed]
    DecisionID           string
    TaskID               string          // FK → originating task
    TaskClass            TaskClass
    RiskTier             RiskTier

    // Selection outcome
    SelectedLLMID        string
    SelectedInstanceID   string
    FallbackChain        []string        // ordered LLMIDs

    // Scoring trace
    CandidateSet         []string
    Scores               map[string]float32
    RejectedModels       []RejectionRecord

    // Constraints applied
    CapabilityFilters    []string
    AutonomyConstraints  []string
    CostConstraints      []string
    GovernanceConstraints []string

    Confidence           float32

    // Surfacing
    UserVisibleSummary   string          // concise, human-readable; sourced by Experience Layer
    DebugRationale       string          // full trace for audit/debug view only
    SurfacingLevel       SurfacingLevel  // silent | advisory | proactive

    // Linkage
    ExecutionRecordID    *string         // nil if execution never occurred

    Provenance Provenance
    CreatedAt  time.Time
}

type RejectionRecord struct {
    LLMID      string
    Reason     string
    ReasonCode RejectionCode   // unavailable | capability_gap | governance | cost | autonomy_ceiling
}
```

---

### 5.5 `LLMExecutionRecord`

One record per model invocation. Append-only. All fields **Observed**.

```go
type LLMExecutionRecord struct {
    RecordID             string
    LLMID                string          // FK → LLMProfile
    InstanceID           string          // FK → LLMRuntimeInstance
    RouterDecisionID     string          // FK → RouterDecision
    TaskClass            TaskClass
    TaskID               string
    ContextSizeTokens    int
    OutputSizeTokens     int
    LatencyMs            int
    CostUSD              *float32
    Outcome              ExecutionOutcome    // success | failure | timeout | user_aborted | fallback
    FailureReason        string
    ToolsInvoked         []string
    ToolSuccessRate      float32
    SchemaValidated      bool
    FallbackTriggered    bool
    FallbackModelID      *string
    UserRating           *int                // 1–5
    UserOverride         bool
    Notes                string

    Attributes []Attribute
    Provenance Provenance
    CreatedAt  time.Time
}
```

---

### 5.6 `LLMEvaluation`

Curated or reflection-derived assessment. Multiple per model, accumulate over time.

```go
type LLMEvaluation struct {
    EvalID       string
    LLMID        string
    EvalType     EvalType        // internal | benchmark_import | reflection_derived | manual
    Assessor     string
    TaskClass    TaskClass
    Score        float32
    Confidence   float32
    SampleSize   *int
    Remarks      string
    EvidenceLinks []string

    // Curated evals are [Governed]; reflection-derived are [Inferred]
    FieldClass   FieldClass

    Attributes []Attribute
    Provenance Provenance
    CreatedAt  time.Time
    ValidUntil *time.Time  // benchmark imports: 90 days; telemetry-derived: no expiry
}
```

---

## 6. Router Rationale UX Contract

The router produces a `RouterDecision` with a `SurfacingLevel`. The Experience Layer consumes this.

| Level | When | What the user sees |
|---|---|---|
| `silent` | Routine, low-stakes, preferred model selected normally | Nothing |
| `advisory` | Fallback used; preferred runtime unavailable; model excluded by governance; degraded execution | Short inline note: *"I used a fallback model because the preferred runtime was unavailable."* |
| `proactive` | User in debug/admin/advanced mode | Full explanation from `UserVisibleSummary` |
| On request | User asks "why did you use this model?" | Answer sourced from `UserVisibleSummary` in `RouterDecision` — never improvised |

`DebugRationale` (scored candidates, applied filters, rejection reasons) surfaces only in audit/debug views.

**Rationale discipline:** Answers to "why did you use this model?" must come from the structured `RouterDecision` record. The model must not reason about itself to produce this answer.

---

## 7. Promotion Notification UX Contract

When Subconscious identifies that an `InferredPreferenceScore[task]` has crossed the promotion threshold and probation has elapsed, it must emit a `RoutingProposalItem` to the Proposal Queue. It must not update `RoutingProfile.PreferredFor` directly.

### Proposal Queue Item Shape

```go
type RoutingProposalItem struct {
    ProposalID         string
    LLMID              string
    ProposedChange     string          // human-readable: "Add coding_generation to PreferredFor"
    AffectedField      string          // "routing_profile.preferred_for"
    CurrentValue       interface{}
    ProposedValue      interface{}
    InferredScore      float32
    SampleSize         int
    ProbationElapsedDays int
    RecentSuccessRate  float32
    FallbackRateDelta  float32         // change in fallback rate vs. baseline
    RoutingImpact      string          // "Would become primary model for coding_generation tasks"
    RiskAssessment     string
    EvidenceRecordIDs  []string
    Status             ProposalStatus  // pending | approved | rejected | deferred
    CreatedAt          time.Time
    ResolvedAt         *time.Time
    ResolvedBy         string
}
```

### UX Layers

**Normal user mode**
- No unsolicited chatter during chat
- Optional badge/count on a settings or status surface: *"Model routing updates available"*

**Advanced/admin mode**
- Proposal inbox or review queue
- Per proposal: what changes, why, evidence summary, routing impact, risk level, approve/reject/defer actions
- Good proposal message: *"Proposal: add Claude 3.5 Sonnet to PreferredFor=coding. Evidence: 92 successful coding tasks over probation window, 0.84 inferred preference score, lower fallback rate than current default."*

**Audit/debug mode**
- Full trace: inferred score history, probation window stats, threshold crossing event, affected routing decisions

### What not to do
- Do not silently update Governed fields
- Do not interrupt normal conversation for routing proposals
- Do not hide proposals deep enough that behavioral drift becomes invisible to the owner

---

## 8. Controlled Vocabularies

Must be defined and frozen before any records are written.

### Capability Classes

```go
type AgenticClass string
const (
    AgenticNone        AgenticClass = "none"
    AgenticConstrained AgenticClass = "constrained"
    AgenticCapable     AgenticClass = "capable"
    AgenticStrong      AgenticClass = "strong"
)

type CodingClass string
const (
    CodingNone      CodingClass = "none"
    CodingBasic     CodingClass = "basic"
    CodingCompetent CodingClass = "competent"
    CodingStrong    CodingClass = "strong"
    CodingElite     CodingClass = "elite"
)

type ReasoningClass string
const (
    ReasoningNone        ReasoningClass = "none"
    ReasoningBasic       ReasoningClass = "basic"
    ReasoningCompetent   ReasoningClass = "competent"
    ReasoningStrong      ReasoningClass = "strong"
    ReasoningExceptional ReasoningClass = "exceptional"
)

type ClassLevel string
const (
    ClassWeak        ClassLevel = "weak"
    ClassFair        ClassLevel = "fair"
    ClassGood        ClassLevel = "good"
    ClassStrong      ClassLevel = "strong"
    ClassExceptional ClassLevel = "exceptional"
)
```

### Use Cases and Task Classes

```go
type UseCase string
const (
    UseCaseChat             UseCase = "chat"
    UseCaseCoding           UseCase = "coding"
    UseCasePlanning         UseCase = "planning"
    UseCaseAgenticExecution UseCase = "agentic_execution"
    UseCaseSummarization    UseCase = "summarization"
    UseCaseExtraction       UseCase = "extraction"
    UseCaseClassification   UseCase = "classification"
    UseCaseCreativeWriting  UseCase = "creative_writing"
    UseCaseMultimodal       UseCase = "multimodal_understanding"
    UseCaseEmbeddings       UseCase = "embeddings"
)

type TaskClass string
const (
    TaskCasualChat          TaskClass = "casual_chat"
    TaskEmpatheticChat      TaskClass = "empathetic_chat"
    TaskCodingGeneration    TaskClass = "coding_generation"
    TaskCodingRefactor      TaskClass = "coding_refactor"
    TaskShellUse            TaskClass = "shell_use"
    TaskPlanning            TaskClass = "planning"
    TaskLongFormExplanation TaskClass = "long_form_explanation"
    TaskRetrievalSynthesis  TaskClass = "retrieval_synthesis"
    TaskStructuredExtraction TaskClass = "structured_extraction"
    TaskToolOrchestration   TaskClass = "tool_orchestration"
    TaskDocumentSummary     TaskClass = "document_summary"
    TaskCreativeGeneration  TaskClass = "creative_generation"
    TaskImageUnderstanding  TaskClass = "image_understanding"
)
```

### Operational Enums

```go
type AvailabilityState  string  // available | degraded | unavailable | unknown
type RuntimeBackend     string  // ollama | openai | anthropic | google | vllm | mistral | cohere | custom
type ConnectorStatus    string  // connected | auth_error | unreachable | not_configured
type AuthStatus         string  // ok | missing | expired | invalid
type HealthStatus       string  // healthy | degraded | unhealthy | unknown
type WarmState          string  // warm | cold | unknown
type RateLimitState     string  // ok | throttled | quota_exceeded | unknown
type LoadedStatus       string  // loaded | unloaded | loading | unknown
type ReleaseChannel     string  // stable | preview | experimental | deprecated | retired
type DeprecationStatus  string  // active | deprecated | retired | announced
type HostingMode        string  // cloud_api | local | self_hosted | cloud_managed
type ReliabilityTrend   string  // improving | stable | degrading | unknown
type FieldClass         string  // governed | inferred | observed
type SurfacingLevel     string  // silent | advisory | proactive
type ProposalStatus     string  // pending | approved | rejected | deferred
```

### Routing / Cost / Risk Enums

```go
type CostTier       string  // free | cheap | moderate | expensive | premium
type LatencyTier    string  // realtime | fast | moderate | slow
type RiskTier       string  // low | medium | high | critical
type TrustLevel     string  // trusted | standard | restricted | unverified
type AutonomyLevel  string  // none | supervised | assisted | delegated | autonomous
type FitScore       string  // poor | fair | good | excellent
type RejectionCode  string  // unavailable | capability_gap | governance | cost | autonomy_ceiling
```

### Failure Patterns

```go
type FailurePattern string
const (
    FailureHallucination      FailurePattern = "hallucination"
    FailureSchemaDrift        FailurePattern = "schema_drift"
    FailureVerbosityBloat     FailurePattern = "verbosity_bloat"
    FailurePrematureCertainty FailurePattern = "premature_certainty"
    FailurePoorToolSelection  FailurePattern = "poor_tool_selection"
    FailureWeakPersistence    FailurePattern = "weak_persistence"
    FailureContextDropoff     FailurePattern = "context_dropoff"
    FailureToolLooping        FailurePattern = "tool_looping"
    FailureRefusalOverreach   FailurePattern = "refusal_overreach"
    FailureLatencySpike       FailurePattern = "latency_spike"
    FailureQuotaExhaustion    FailurePattern = "quota_exhaustion"
)
```

---

## 9. Governance Boundary Summary

| Field Group | Classification | Who can update | Gate |
|---|---|---|---|
| `UsageStats.*` | Observed | Subconscious consolidation | None |
| `OperationalState.*` | Observed | Connector → Cognitive | None (automated probe) |
| `InferredTaskFit[task]` | Inferred | Subconscious | Confidence + sample size threshold |
| `InferredPreferenceScore[task]` | Inferred | Subconscious | Confidence + sample size threshold |
| `EvaluationProfile.ObservedFailurePatterns` | Observed | Subconscious | None |
| `EvaluationProfile.InferredReliabilityTrend` | Inferred | Subconscious | None |
| `RoutingProfile.PreferredFor` | **Governed** | Proposal only | Explicit owner approval |
| `RoutingProfile.AvoidFor` | **Governed** | Proposal only | Explicit owner approval |
| `RoutingProfile.AutonomyCeiling` | **Governed** | Proposal only | Explicit owner approval |
| `RoutingProfile.MaxRiskTierAllowed` | **Governed** | Proposal only | Explicit owner approval |
| `RoutingProfile.BlockedDomains` | **Governed** | Proposal only | Explicit owner approval |
| `RoutingProfile.RequiresConfirmationFor` | **Governed** | Proposal only | Explicit owner approval |
| `CapabilityProfile.AgenticClass` | **Governed** | Manual or Proposal | Explicit owner approval if promotion unlocks autonomous execution |
| `LLMProfile.Identity.*` | **Governed** | Manual update only | — |
| `TechnicalFeatures.*` | **Governed** | Manual or provider_api probe | Verified before overwrite |

**Hard rule:** Any change to a field that affects what NAVI is willing to do autonomously, which model is eligible for higher-risk tasks, or which model becomes default for consequential task classes — requires a Proposal. Subconscious cannot update these fields directly regardless of confidence level.

---

## 10. Provenance Integration

```go
type Provenance struct {
    Source          ProvenanceSource
    SourceDetail    string
    Confidence      float32
    DerivationChain []string
    AssertedAt      time.Time
    AssertedBy      string
    MutationHistory []MutationRecord
}

type ProvenanceSource string
const (
    ProvCuratedSeed     ProvenanceSource = "curated_seed"
    ProvProviderAPI     ProvenanceSource = "provider_api"
    ProvRuntimeProbe    ProvenanceSource = "runtime_probe"
    ProvExecTelemetry   ProvenanceSource = "execution_telemetry"
    ProvReflection      ProvenanceSource = "reflection"
    ProvUserInput       ProvenanceSource = "user_input"
    ProvBenchmarkImport ProvenanceSource = "benchmark_import"
)
```

---

## 11. Seed Record Format

Canonical seed YAML. Contains curated facts and routing hints only — no live state, no telemetry.

```yaml
kind: LLMProfile
schema_version: 1

llm_id: anthropic.claude-3-5-sonnet
canonical_name: Claude 3.5 Sonnet
aliases:
  - claude-3.5-sonnet
  - claude sonnet 3.5

identity:
  provider: anthropic
  publisher: Anthropic
  family: claude-3
  variant: sonnet
  version: "3.5"
  release_channel: stable
  hosting_modes_supported:
    - api
  open_weight: false
  deprecated: false

capabilities:
  modalities:
    input: [text, image]
    output: [text]
  primary_use_cases:
    - chat
    - reasoning
    - summarization
    - analysis
    - coding
  interaction_modes:
    - conversational
    - tool_using
    - structured_output
    - long_context
  agentic_class: capable
  coding_class: strong
  reasoning_class: strong
  instruction_following_class: strong
  tool_discipline_class: competent
  schema_reliability_class: competent
  multimodal_class: competent

technical_profile:
  supports_tools: true
  supports_parallel_tools: false
  supports_json_schema: true
  supports_streaming: true
  supports_system_prompt: true
  supports_vision: true
  supports_audio_in: false
  supports_audio_out: false
  supports_seed_determinism: false
  supports_fine_tuning: false
  context_window:
    input_tokens: null
    notes: "Verify via runtime/provider introspection at Phase 2"
  max_output_tokens: null

routing_profile:
  preferred_for:
    - complex_chat
    - reasoning_heavy_analysis
    - coding_generation
    - image_understanding
  avoid_for: []
  fallback_models:
    - openai.gpt-4o
    - anthropic.claude-3-haiku
  autonomy_ceiling: medium
  risk_tier_allowed: medium
  default_routing_priority: 1
  cost_tier: expensive
  latency_tier: moderate
  requires_confirmation_for:
    - high_impact_external_actions
  blocked_domains: []
  promotion_threshold: 0.85
  promotion_min_samples: 50
  promotion_probation_days: 7

provenance:
  sources:
    - type: curated_seed
      confidence: 0.9
  last_verified_at: null

# Empty in seed records — populated by runtime probes, telemetry, and reflection only
operational_state: {}
evaluation_profile:
  notes:
    - "Initial seed record."
usage_stats: {}
runtime_instances: []
```

### Must NOT be in seed records
- Current download or health state
- Current latency or quota
- Observed success rate
- Inferred preference or avoidance scores

---

## 12. Storage Design

### Primary Store

SQLite (Phase 1–3) → Postgres (Phase 4+ if scale requires).

```
llm_providers
llm_profiles
llm_profile_capabilities          (1:1)
llm_profile_features              (1:1)
llm_profile_operational_state     (1:1, highest mutation rate)
llm_profile_evaluation            (1:1, updated by reflection)
llm_profile_usage_stats           (1:1, materialized from execution records)
llm_profile_routing               (1:1)
llm_runtime_instances
llm_execution_records             (append-only, indexed on llm_id + created_at)
llm_evaluations
router_decisions                  (append-only, indexed on task_id + created_at)
routing_proposal_items
```

### Secondary (Generated Artifacts)

Outputs only, never sources of truth.

```
models/
  index.md
  {llm_id}/
    PROFILE.yaml
    NOTES.md
```

---

## 13. Ingestion Paths

| Path | Provenance | Confidence | Triggers | Updates |
|---|---|---|---|---|
| Curated seed | `curated_seed` | 0.9 | Init / admin command | Identity, Capabilities, Features, Routing (Governed fields) |
| Provider/runtime introspection | `provider_api` / `runtime_probe` | 1.0 binary; 0.7 interpreted | Startup, connector registration, health interval | OperationalState, RuntimeInstance |
| Execution telemetry | `execution_telemetry` | 1.0 | Every invocation | ExecutionRecord → UsageStats (aggregated) |
| Reflection synthesis | `reflection` | Scaled to sample size | Consolidation cycle | EvaluationProfile (Inferred); Proposals for Governed |

---

## 14. Phased Build Plan

### Phase 1 — Schema + Seed Catalog ← **Current sprint**
- Go structs, enums, field classifications
- SQLite schema with migration support
- Seed loader with validation and idempotent import
- Three seed records: frontier cloud model, cheap fallback, local/offline model
- Basic read/query API (list, get by id, list runtimes, list preferred for task class)
- Validation and unit tests
- Admin/debug inspection surface

### Phase 2 — Runtime State + Health Checks
- `LLMRuntimeInstance` and `OperationalState` refresh from connector probes
- Connectors: Anthropic, OpenAI, Ollama
- Write path enforced: connector → Cognitive normalization → World Model

### Phase 3 — Execution Telemetry
- `LLMExecutionRecord` on every invocation
- `RouterDecision` per routing event
- Consolidation job materializes `UsageStats`

### Phase 4 — Routing Integration
- Router consumes KB for model selection (five-step scored selection)
- `RouterDecision` drives surfacing level and user-visible rationale

### Phase 5 — Reflection-Driven Adaptation
- Subconscious scans records, promotes inferences into `EvaluationProfile`
- Proposal Queue receives routing promotion proposals
- Promotion pipeline: `InferredPreferenceScore` → threshold + probation → Proposal → owner approval → `PreferredFor`

---

## 15. Open Questions (Parked)

- [ ] **Seed data location:** `data/seed/models/` YAML preferred over Go-embedded for human editability
- [ ] **Aggregation cadence:** Suggested default: on-demand + nightly
- [ ] **Routing DSL:** Phase 1 simple tag matching; Phase 4 CEL for `RoutingCondition`
- [ ] **Multi-instance fallback preference:** User preference; suggested default: local-first
- [ ] **Schema versioning:** `schema_version` per profile; migrations run on load

---

*v3.0 — Incorporates RouterDecision lifecycle (distinct from execution outcome), promotion notification UX contract, and RoutingProposalItem schema. Implementation authority: Eric. Execution authority: delegated agents.*