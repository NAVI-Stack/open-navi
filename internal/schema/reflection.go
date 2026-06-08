package schema

import "time"

// ReflectionTier identifies the depth of reflection work to perform.
type ReflectionTier string

const (
	ReflectionTierShallow       ReflectionTier = "shallow"
	ReflectionTierConsolidation ReflectionTier = "consolidation"
	ReflectionTierDeep          ReflectionTier = "deep"
)

// ExtractedFact is a durable fact produced by post-turn extraction or
// summarization. Scope defaults to owner when omitted by the extractor.
type ExtractedFact struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Category string `json:"category"`
	Scope    string `json:"scope,omitempty"`
	ScopeID  string `json:"scope_id,omitempty"`
}

// PreferenceEvidenceClass identifies how NAVI inferred a preference signal.
type PreferenceEvidenceClass string

const (
	PreferenceEvidenceExplicitCorrection PreferenceEvidenceClass = "explicit_correction"
	PreferenceEvidenceRepeatedBehavior   PreferenceEvidenceClass = "repeated_behavior"
	PreferenceEvidenceSilentWin          PreferenceEvidenceClass = "silent_win"
	PreferenceEvidenceAnomaly            PreferenceEvidenceClass = "anomaly"
)

// PreferenceSignalStatus tracks the lifecycle of a captured adaptation signal.
type PreferenceSignalStatus string

const (
	PreferenceSignalStatusCaptured  PreferenceSignalStatus = "captured"
	PreferenceSignalStatusApplied   PreferenceSignalStatus = "applied"
	PreferenceSignalStatusPersisted PreferenceSignalStatus = "persisted"
)

// PreferenceSignal is the shallow-reflection capture for an interaction-level
// preference cue that may later inform inferred configuration.
type PreferenceSignal struct {
	SignalID       string                  `json:"signal_id,omitempty"`
	Scope          string                  `json:"scope"`
	ScopeID        string                  `json:"scope_id"`
	ChatID         string                  `json:"chat_id,omitempty"`
	Trait          string                  `json:"trait"`
	TargetValue    float64                 `json:"target_value"`
	EvidenceClass  PreferenceEvidenceClass `json:"evidence_class"`
	SignalStrength float64                 `json:"signal_strength"`
	Immediate      bool                    `json:"immediate,omitempty"`
	Summary        string                  `json:"summary,omitempty"`
	Status         PreferenceSignalStatus  `json:"status,omitempty"`
	CreatedAt      time.Time               `json:"created_at,omitempty"`
	UpdatedAt      time.Time               `json:"updated_at,omitempty"`
}

// ReflectionDetails is the structured JSON payload stored in
// ReflectionPayload.Details for shallow reflection processing.
//
// Legacy single-fact fields are kept for backward compatibility with older
// emitters while newer emitters should prefer Facts.
type ReflectionDetails struct {
	Kind              string             `json:"kind,omitempty"`
	Scope             string             `json:"scope,omitempty"`
	ScopeID           string             `json:"scope_id,omitempty"`
	Category          string             `json:"category,omitempty"`
	Key               string             `json:"key,omitempty"`
	Value             string             `json:"value,omitempty"`
	Significance      string             `json:"significance,omitempty"`
	Summary           string             `json:"summary,omitempty"`
	UserMessage       string             `json:"user_message,omitempty"`
	Facts             []ExtractedFact    `json:"facts,omitempty"`
	PreferenceSignals []PreferenceSignal `json:"preference_signals,omitempty"`
}

// InferenceReflectionDetails is the structured payload emitted from ICS
// reflection hooks on the authoritative runtime path.
type InferenceReflectionDetails struct {
	Kind                  string   `json:"kind,omitempty"`
	RunID                 string   `json:"run_id,omitempty"`
	RuntimeSessionID      string   `json:"runtime_session_id,omitempty"`
	TraceID               string   `json:"trace_id,omitempty"`
	GoalID                string   `json:"goal_id,omitempty"`
	CandidateType         string   `json:"candidate_type,omitempty"`
	RecoveryRoute         string   `json:"recovery_route,omitempty"`
	RecoveryCheckpointRef string   `json:"recovery_checkpoint_ref,omitempty"`
	WhatHappened          string   `json:"what_happened,omitempty"`
	ExpectedVsActual      string   `json:"expected_vs_actual,omitempty"`
	SuccessFailureState   string   `json:"success_failure_state,omitempty"`
	Surprises             []string `json:"surprises,omitempty"`
	LessonCandidate       string   `json:"lesson_candidate,omitempty"`
	MemoryCandidate       string   `json:"memory_candidate,omitempty"`
	ProposalCandidate     string   `json:"proposal_candidate,omitempty"`
	FollowupTrigger       string   `json:"followup_trigger,omitempty"`
}

// ReflectionPayload is emitted by the Conscious process (Reflect step) and
// consumed by background reflection workers.
// Details may be either free-form text or a small JSON object when
// EscalationReason is set. The JSON convention is:
//
//	{ "kind": "memory" | "fact",
//	  "scope": "owner" | "session" | "directive" | ...,
//	  "scope_id": "<id>",
//	  "category": "<fact-category>",
//	  "key": "<fact-key>",
//	  "value": "<fact-value>",
//	  "facts": [{ "key": "...", "value": "...", "category": "...", "scope": "owner" }],
//	  "significance": "low" | "medium" | "high" }
type ReflectionPayload struct {
	ID               string         `json:"id"`
	RuntimeSessionID string         `json:"runtime_session_id,omitempty"`
	DirectiveID      string         `json:"directive_id,omitempty"`
	Tier             ReflectionTier `json:"tier"`
	Summary          string         `json:"summary"`
	Details          string         `json:"details,omitempty"`
	EscalationReason string         `json:"escalation_reason,omitempty"` // "manual" (user said remember this), "automatic" (significance detected)
	CreatedAt        time.Time      `json:"created_at"`
}

// InterruptionMode is how the Subconscious surfaces an urgent correction to the Conscious process.
type InterruptionMode string

const (
	InterruptionAdvisory InterruptionMode = "advisory" // insight surfaced; action not halted
	InterruptionBlocking InterruptionMode = "blocking" // action paused; user confirmation required
)

// SubconsciousInterruption is sent when the Subconscious detects that the Conscious
// is using outdated or contradicted information that would cause material harm.
// Only one interruption per decision cycle (recursion guard).
type SubconsciousInterruption struct {
	ID            string           `json:"id"`
	Mode          InterruptionMode `json:"mode"`
	Summary       string           `json:"summary"`
	Details       string           `json:"details,omitempty"`
	Contradiction string           `json:"contradiction,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
}
