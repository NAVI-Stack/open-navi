package inference

import "time"

// GoalTransition captures one explicit model-level change to the goal stack.
type GoalTransition struct {
	Operation string     `json:"operation,omitempty"`
	GoalID    string     `json:"goal_id,omitempty"`
	From      GoalStatus `json:"from,omitempty"`
	To        GoalStatus `json:"to,omitempty"`
}

// FocusCandidate makes focus arbitration explicit and testable.
type FocusCandidate struct {
	FocusID            string        `json:"focus_id,omitempty"`
	ActiveGoalID       string        `json:"active_goal_id,omitempty"`
	FocusReason        FocusReason   `json:"focus_reason,omitempty"`
	PriorityScore      float64       `json:"priority_score,omitempty"`
	HardPreempt        bool          `json:"hard_preempt,omitempty"`
	Preemptible        bool          `json:"preemptible,omitempty"`
	SupportingFactors  []string      `json:"supporting_factors,omitempty"`
	BlockingFactors    []string      `json:"blocking_factors,omitempty"`
	PinnedSubreasoners []Subreasoner `json:"pinned_subreasoners,omitempty"`
}

// FocusSelection summarizes one focus arbitration cycle.
type FocusSelection struct {
	Focus            FocusFrame       `json:"focus"`
	Candidates       []FocusCandidate `json:"candidates,omitempty"`
	PreemptedFocusID string           `json:"preempted_focus_id,omitempty"`
	ResumeFocusID    string           `json:"resume_focus_id,omitempty"`
}

// ModeTransition makes dominant-mode changes explicit for each cycle.
type ModeTransition struct {
	Previous DecisionMode `json:"previous,omitempty"`
	Current  DecisionMode `json:"current,omitempty"`
	Changed  bool         `json:"changed,omitempty"`
	Reason   string       `json:"reason,omitempty"`
}

// ArbitrationState captures compact subreasoner routing state for one cycle.
type ArbitrationState struct {
	ActivePins             []SubreasonerPin `json:"active_pins,omitempty"`
	ReleasedPins           []SubreasonerPin `json:"released_pins,omitempty"`
	SuppressedSubreasoners []Subreasoner    `json:"suppressed_subreasoners,omitempty"`
	Notes                  []string         `json:"notes,omitempty"`
}

// ModeSelection is the explicit, testable result of routing one focus frame.
type ModeSelection struct {
	DominantMode        DecisionMode     `json:"dominant_mode,omitempty"`
	SubmodeChain        []DecisionMode   `json:"submode_chain,omitempty"`
	PlanningStyle       PlanningStyle    `json:"planning_style,omitempty"`
	PlanningDepth       int              `json:"planning_depth,omitempty"`
	InvokedSubreasoners []Subreasoner    `json:"invoked_subreasoners,omitempty"`
	ResolvedPosture     PostureState     `json:"resolved_posture,omitempty"`
	Arbitration         ArbitrationState `json:"arbitration,omitempty"`
	Transition          ModeTransition   `json:"transition,omitempty"`
}

// EvaluationResult is the explicit output of candidate generation and scoring.
type EvaluationResult struct {
	ChosenCandidate    CandidateSummary   `json:"chosen_candidate"`
	CandidateSummaries []CandidateSummary `json:"candidate_summaries,omitempty"`
	Thresholds         ThresholdState     `json:"thresholds,omitempty"`
	Rejection          *RejectionState    `json:"rejection,omitempty"`
}

// DecisionTrace is the compact audit artifact for one ICS decision cycle.
type DecisionTrace struct {
	TraceID                string            `json:"trace_id,omitempty"`
	Stage                  string            `json:"stage,omitempty"`
	OccurredAt             time.Time         `json:"occurred_at,omitempty"`
	Focus                  FocusFrame        `json:"focus,omitempty"`
	FocusCandidates        []FocusCandidate  `json:"focus_candidates,omitempty"`
	Mode                   ModeTransition    `json:"mode,omitempty"`
	SubmodeChain           []DecisionMode    `json:"submode_chain,omitempty"`
	InvokedSubreasoners    []Subreasoner     `json:"invoked_subreasoners,omitempty"`
	ActivePins             []SubreasonerPin  `json:"active_pins,omitempty"`
	SuppressedSubreasoners []Subreasoner     `json:"suppressed_subreasoners,omitempty"`
	ArbitrationNotes       []string          `json:"arbitration_notes,omitempty"`
	CandidateIDs           []string          `json:"candidate_ids,omitempty"`
	TargetCapability       string            `json:"target_capability,omitempty"`
	AllowedCapabilities    []string          `json:"allowed_capabilities,omitempty"`
	PlanID                 string            `json:"plan_id,omitempty"`
	PlanStatus             PlanStatus        `json:"plan_status,omitempty"`
	CurrentNodeID          string            `json:"current_node_id,omitempty"`
	CheckpointRefs         []string          `json:"checkpoint_refs,omitempty"`
	RecoveryRoute          RecoveryRouteKind `json:"recovery_route,omitempty"`
	Thresholds             ThresholdState    `json:"thresholds,omitempty"`
	GovernanceOutcome      string            `json:"governance_outcome,omitempty"`
	ExecutionOutcomeRef    string            `json:"execution_outcome_ref,omitempty"`
	ExecutedCapability     string            `json:"executed_capability,omitempty"`
	ExecutedCommandType    string            `json:"executed_command_type,omitempty"`
	ProposalID             string            `json:"proposal_id,omitempty"`
	RecoveryRef            string            `json:"recovery_ref,omitempty"`
	ReflectionRef          string            `json:"reflection_ref,omitempty"`
	ResumeFocusID          string            `json:"resume_focus_id,omitempty"`
}
