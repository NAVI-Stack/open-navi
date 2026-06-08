// Package inference defines the Inference Control System (ICS) contracts.
package inference

import (
	"strings"
	"time"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/navi/orchestration"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
)

// ContractVersionV1 identifies the first ICS contract version.
const ContractVersionV1 = "ics.v1"

// DecisionMode is the dominant control mode selected for one inference cycle.
type DecisionMode string

const (
	DecisionModeRespond DecisionMode = "respond"
	DecisionModeClarify DecisionMode = "clarify"
	DecisionModePlan    DecisionMode = "plan"
	DecisionModeExecute DecisionMode = "execute"
	DecisionModeMonitor DecisionMode = "monitor"
	DecisionModeDefer   DecisionMode = "defer"
	DecisionModeReject  DecisionMode = "reject"
	DecisionModePropose DecisionMode = "propose"
	DecisionModeRecover DecisionMode = "recover"
	DecisionModeReplan  DecisionMode = "replan"
)

// PlanningStyle describes how much explicit structure ICS should commit up front.
type PlanningStyle string

const (
	PlanningStyleDirect      PlanningStyle = "direct"
	PlanningStyleShallow     PlanningStyle = "shallow"
	PlanningStyleProgressive PlanningStyle = "progressive"
	PlanningStyleAdaptive    PlanningStyle = "adaptive"
)

// FocusReason explains why a focus target won the current arbitration cycle.
type FocusReason string

const (
	FocusReasonUserRequest        FocusReason = "user_request"
	FocusReasonActiveGoal         FocusReason = "active_goal"
	FocusReasonPendingRecovery    FocusReason = "pending_recovery"
	FocusReasonPendingProposal    FocusReason = "pending_proposal"
	FocusReasonScheduledTrigger   FocusReason = "scheduled_trigger"
	FocusReasonUrgentConflict     FocusReason = "urgent_contradiction"
	FocusReasonReflectionFollowup FocusReason = "reflection_followup"
	FocusReasonIdle               FocusReason = "idle"
)

// Subreasoner identifies a selective reasoning helper available to ICS.
type Subreasoner string

const (
	SubreasonerInterpreter  Subreasoner = "interpreter"
	SubreasonerPlanner      Subreasoner = "planner"
	SubreasonerClarifier    Subreasoner = "clarifier"
	SubreasonerCritic       Subreasoner = "critic"
	SubreasonerRiskAssessor Subreasoner = "risk_assessor"
	SubreasonerUncertainty  Subreasoner = "uncertainty_assessor"
	SubreasonerRecovery     Subreasoner = "recovery_reasoner"
	SubreasonerReflection   Subreasoner = "reflection_reasoner"
)

// CandidateType classifies a ranked action candidate.
type CandidateType string

const (
	CandidateTypeRespond CandidateType = "respond"
	CandidateTypeClarify CandidateType = "clarify"
	CandidateTypePlan    CandidateType = "plan"
	CandidateTypeExecute CandidateType = "execute"
	CandidateTypeMonitor CandidateType = "monitor"
	CandidateTypeDefer   CandidateType = "defer"
	CandidateTypeReject  CandidateType = "reject"
	CandidateTypePropose CandidateType = "propose"
	CandidateTypeRecover CandidateType = "recover"
	CandidateTypeReplan  CandidateType = "replan"
)

// GoalStatus captures where a goal sits in the ranked stack.
type GoalStatus string

const (
	GoalStatusActive    GoalStatus = "active"
	GoalStatusReady     GoalStatus = "ready"
	GoalStatusLatent    GoalStatus = "latent"
	GoalStatusSuspended GoalStatus = "suspended"
	GoalStatusRetired   GoalStatus = "retired"
	GoalStatusAbandoned GoalStatus = "abandoned"
)

// PlanStatus captures lifecycle state for an explicit plan graph.
type PlanStatus string

const (
	PlanStatusDraft     PlanStatus = "draft"
	PlanStatusActive    PlanStatus = "active"
	PlanStatusBlocked   PlanStatus = "blocked"
	PlanStatusFailed    PlanStatus = "failed"
	PlanStatusCompleted PlanStatus = "completed"
)

// PlanNodeStatus captures lifecycle state for an explicit plan node.
type PlanNodeStatus string

const (
	PlanNodeStatusPending   PlanNodeStatus = "pending"
	PlanNodeStatusReady     PlanNodeStatus = "ready"
	PlanNodeStatusBlocked   PlanNodeStatus = "blocked"
	PlanNodeStatusFailed    PlanNodeStatus = "failed"
	PlanNodeStatusCompleted PlanNodeStatus = "completed"
)

// ApprovalRequirement describes the governance gate that must be satisfied.
type ApprovalRequirement string

const (
	ApprovalRequirementNone                 ApprovalRequirement = "none"
	ApprovalRequirementExplicitConfirmation ApprovalRequirement = "explicit_confirmation"
	ApprovalRequirementBlockingProposal     ApprovalRequirement = "blocking_proposal"
)

// OutcomeState is the summarized reflection state for a decision/execution cycle.
type OutcomeState string

const (
	OutcomeStatePending OutcomeState = "pending"
	OutcomeStateSuccess OutcomeState = "success"
	OutcomeStateFailure OutcomeState = "failure"
	OutcomeStatePartial OutcomeState = "partial"
	OutcomeStateBlocked OutcomeState = "blocked"
)

// SubreasonerPin tracks a bounded, decaying pin for one subreasoner.
type SubreasonerPin struct {
	Reasoner       Subreasoner `json:"reasoner,omitempty"`
	Source         string      `json:"source,omitempty"`
	Reason         string      `json:"reason,omitempty"`
	DecayRemaining int         `json:"decay_remaining,omitempty"`
}

// RecoveryRouteKind identifies the explicit recovery action chosen by ICS.
type RecoveryRouteKind string

const (
	RecoveryRouteNone       RecoveryRouteKind = "none"
	RecoveryRouteRetry      RecoveryRouteKind = "retry"
	RecoveryRouteCompensate RecoveryRouteKind = "compensate"
	RecoveryRouteRecover    RecoveryRouteKind = "recover"
	RecoveryRouteReplan     RecoveryRouteKind = "replan"
	RecoveryRouteDefer      RecoveryRouteKind = "defer"
	RecoveryRoutePropose    RecoveryRouteKind = "propose"
)

// ThresholdBand classifies a candidate score into an explicit decision band.
type ThresholdBand string

const (
	ThresholdBandBlocked   ThresholdBand = "blocked"
	ThresholdBandWeak      ThresholdBand = "weak"
	ThresholdBandViable    ThresholdBand = "viable"
	ThresholdBandPreferred ThresholdBand = "preferred"
)

// RejectionReason identifies a structured rejection family.
type RejectionReason string

const (
	RejectionReasonUnsafe                     RejectionReason = "unsafe"
	RejectionReasonUnauthorized               RejectionReason = "unauthorized"
	RejectionReasonImpossibleCapability       RejectionReason = "impossible_with_current_capability"
	RejectionReasonMissingCriticalConstraints RejectionReason = "missing_critical_constraints"
	RejectionReasonPolicyBlocked              RejectionReason = "policy_blocked"
	RejectionReasonGovernanceBlocked          RejectionReason = "governance_blocked"
	RejectionReasonSelfStateCompromised       RejectionReason = "self_state_compromised"
	RejectionReasonEnvironmentUntrusted       RejectionReason = "environment_not_trustworthy_enough"
)

// InferenceInput is the canonical ICS input contract.
// NCOS remains the structured context seam; runtime references point back to
// the current run state rather than duplicating it.
type InferenceInput struct {
	Version         string                            `json:"version,omitempty"`
	NCOS            orchestration.CanonicalRunRequest `json:"ncos"`
	GoalStack       GoalStack                         `json:"goal_stack,omitempty"`
	PlanState       *PlanGraph                        `json:"plan_state,omitempty"`
	PreviousFocus   *FocusFrame                       `json:"previous_focus,omitempty"`
	Posture         PostureState                      `json:"posture,omitempty"`
	Recovery        RecoveryState                     `json:"recovery,omitempty"`
	Governance      GovernanceState                   `json:"governance,omitempty"`
	Chat            ChatContext                       `json:"chat,omitempty"`
	Runtime         RuntimeContext                    `json:"runtime,omitempty"`
	Capabilities    []CapabilityAvailability          `json:"capabilities,omitempty"`
	RelevantContext []ContextReference                `json:"relevant_context,omitempty"`
	Extensions      map[string]any                    `json:"extensions,omitempty"`
}

// GoalRef is a ranked goal reference inside the active control stack.
type GoalRef struct {
	GoalID      string         `json:"goal_id"`
	Summary     string         `json:"summary,omitempty"`
	Status      GoalStatus     `json:"status,omitempty"`
	Priority    float64        `json:"priority,omitempty"`
	Preemptible bool           `json:"preemptible,omitempty"`
	Extensions  map[string]any `json:"extensions,omitempty"`
}

// GoalStack represents the foreground/background objective stack.
type GoalStack struct {
	ActiveGoalID string    `json:"active_goal_id,omitempty"`
	Ready        []GoalRef `json:"ready,omitempty"`
	Latent       []GoalRef `json:"latent,omitempty"`
	Suspended    []GoalRef `json:"suspended,omitempty"`
	Retired      []GoalRef `json:"retired,omitempty"`
}

// PlanNode represents one explicit unit of work in a plan graph.
type PlanNode struct {
	NodeID             string                    `json:"node_id"`
	Intent             string                    `json:"intent,omitempty"`
	TargetCapability   string                    `json:"target_capability,omitempty"`
	CommandType        schema.CommandType        `json:"command_type,omitempty"`
	Dependencies       []string                  `json:"dependencies,omitempty"`
	Reversibility      schema.ReversibilityClass `json:"reversibility,omitempty"`
	RetryPolicy        string                    `json:"retry_policy,omitempty"`
	CheckpointBoundary bool                      `json:"checkpoint_boundary,omitempty"`
	SuccessCriteria    []string                  `json:"success_criteria,omitempty"`
	FailureCriteria    []string                  `json:"failure_criteria,omitempty"`
	Blockers           []string                  `json:"blockers,omitempty"`
	Status             PlanNodeStatus            `json:"status,omitempty"`
}

// PlanEdge represents an explicit dependency edge between plan nodes.
type PlanEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind,omitempty"`
}

// PlanCheckpoint ties an explicit plan boundary back to runtime checkpoint semantics.
type PlanCheckpoint struct {
	CheckpointID  string `json:"checkpoint_id,omitempty"`
	NodeID        string `json:"node_id,omitempty"`
	CheckpointRef string `json:"checkpoint_ref,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

// PlanGraph is the explicit, resumable work graph managed by ICS.
type PlanGraph struct {
	PlanID          string           `json:"plan_id"`
	GoalID          string           `json:"goal_id,omitempty"`
	PlanningStyle   PlanningStyle    `json:"planning_style,omitempty"`
	Nodes           []PlanNode       `json:"nodes,omitempty"`
	Edges           []PlanEdge       `json:"edges,omitempty"`
	Checkpoints     []PlanCheckpoint `json:"checkpoints,omitempty"`
	CurrentNodeID   string           `json:"current_node_id,omitempty"`
	SuccessCriteria []string         `json:"success_criteria,omitempty"`
	FailureCriteria []string         `json:"failure_criteria,omitempty"`
	Status          PlanStatus       `json:"status,omitempty"`
}

// ActiveGoal returns the active goal when one is available.
func (g GoalStack) ActiveGoal() (GoalRef, bool) {
	if goal, ok := g.Lookup(g.ActiveGoalID); ok {
		return goal, true
	}
	if len(g.Ready) > 0 {
		return g.Ready[0], true
	}
	return GoalRef{}, false
}

// Lookup returns the goal with the given identifier when present in the stack.
func (g GoalStack) Lookup(goalID string) (GoalRef, bool) {
	goalID = strings.TrimSpace(goalID)
	if goalID == "" {
		return GoalRef{}, false
	}
	for _, goal := range g.All() {
		if strings.TrimSpace(goal.GoalID) == goalID {
			return goal, true
		}
	}
	return GoalRef{}, false
}

// All returns all goals in stable stack order.
func (g GoalStack) All() []GoalRef {
	total := len(g.Ready) + len(g.Latent) + len(g.Suspended) + len(g.Retired)
	if total == 0 {
		return nil
	}
	out := make([]GoalRef, 0, total)
	out = append(out, g.Ready...)
	out = append(out, g.Latent...)
	out = append(out, g.Suspended...)
	out = append(out, g.Retired...)
	return out
}

// PostureState carries durable control posture that may bias but never bypass governance.
type PostureState struct {
	AutonomyLevel            string           `json:"autonomy_level,omitempty"`
	InitiativeBias           float64          `json:"initiative_bias,omitempty"`
	ClarificationStrictness  float64          `json:"clarification_strictness,omitempty"`
	MonitoringAggressiveness float64          `json:"monitoring_aggressiveness,omitempty"`
	PinnedSubreasoners       []Subreasoner    `json:"pinned_subreasoners,omitempty"`
	Pins                     []SubreasonerPin `json:"pins,omitempty"`
}

// RecoveryState carries open recovery work and failure semantics into ICS.
type RecoveryState struct {
	Status            schema.RecoveryStatus `json:"status,omitempty"`
	FailureClass      schema.FailureClass   `json:"failure_class,omitempty"`
	FailureReason     string                `json:"failure_reason,omitempty"`
	CurrentTask       string                `json:"current_task,omitempty"`
	CurrentStep       string                `json:"current_step,omitempty"`
	PendingBlockers   []string              `json:"pending_blockers,omitempty"`
	ResumeConditions  []string              `json:"resume_conditions,omitempty"`
	RollbackPoint     string                `json:"rollback_point,omitempty"`
	ExpiryOrStaleness *time.Time            `json:"expiry_or_staleness,omitempty"`
}

// GovernanceState captures the current approval/governance posture around the decision.
type GovernanceState struct {
	PendingProposalID     string                     `json:"pending_proposal_id,omitempty"`
	PendingProposalStatus schema.ProposalStatus      `json:"pending_proposal_status,omitempty"`
	ConfirmationRequired  bool                       `json:"confirmation_required,omitempty"`
	HardBlocked           bool                       `json:"hard_blocked,omitempty"`
	BlockingReason        string                     `json:"blocking_reason,omitempty"`
	RiskHint              schema.RiskLevel           `json:"risk_hint,omitempty"`
	LastValidation        *governor.ValidationResult `json:"last_validation,omitempty"`
	ConstraintMetadata    map[string]string          `json:"constraint_metadata,omitempty"`
	LastApprovalOutcome   schema.ApprovalOutcome     `json:"last_approval_outcome,omitempty"`
}

// ChatContext carries the active chat/task/user input references.
type ChatContext struct {
	ChatID           string       `json:"chat_id,omitempty"`
	RuntimeSessionID string       `json:"runtime_session_id,omitempty"`
	DirectiveID      string       `json:"directive_id,omitempty"`
	UserMessage      string       `json:"user_message,omitempty"`
	SourceChannel    string       `json:"source_channel,omitempty"`
	CurrentTask      *schema.Task `json:"current_task,omitempty"`
}

// RuntimeContext reuses the live run/checkpoint seam rather than duplicating runtime state.
type RuntimeContext struct {
	Run                 *naviruntime.RunState      `json:"-"`
	Checkpoint          *naviruntime.Checkpoint    `json:"-"`
	RunID               string                     `json:"run_id,omitempty"`
	RunStatus           schema.RunStatus           `json:"run_status,omitempty"`
	CurrentPhase        naviruntime.RunPhase       `json:"current_phase,omitempty"`
	BlockedOnProposalID string                     `json:"blocked_on_proposal_id,omitempty"`
	PauseReason         string                     `json:"pause_reason,omitempty"`
	InterruptClass      naviruntime.InterruptClass `json:"interrupt_class,omitempty"`
	InterruptReason     string                     `json:"interrupt_reason,omitempty"`
	LatestCheckpointID  string                     `json:"latest_checkpoint_id,omitempty"`
	Resume              *naviruntime.ResumeSignal  `json:"resume,omitempty"`
	LastResult          *ExecutionSnapshot         `json:"last_result,omitempty"`
}

// ExecutionSnapshot summarizes the most recent runtime execution outcome visible to ICS.
type ExecutionSnapshot struct {
	Outcome            schema.ExecutionOutcomeOutcome `json:"outcome,omitempty"`
	FailureClass       schema.FailureClass            `json:"failure_class,omitempty"`
	FailureCode        string                         `json:"failure_code,omitempty"`
	ApprovalOutcome    schema.ApprovalOutcome         `json:"approval_outcome,omitempty"`
	ProposalID         string                         `json:"proposal_id,omitempty"`
	ExecutedCapability string                         `json:"executed_capability,omitempty"`
	CommandType        schema.CommandType             `json:"command_type,omitempty"`
	CheckpointRef      string                         `json:"checkpoint_ref,omitempty"`
	Summary            string                         `json:"summary,omitempty"`
	Fallback           *FallbackExecutionTrace        `json:"fallback,omitempty"`
}

// FallbackExecutionTrace records governed fallback routing chosen after an
// execution failure so runtime and supervision can distinguish the original
// failed action from the selected recovery path.
type FallbackExecutionTrace struct {
	OriginalToolName    string   `json:"original_tool_name,omitempty"`
	OriginalAction      string   `json:"original_action,omitempty"`
	OriginalFailureCode string   `json:"original_failure_code,omitempty"`
	CandidateID         string   `json:"candidate_id,omitempty"`
	SelectedToolIDs     []string `json:"selected_tool_ids,omitempty"`
	Disposition         string   `json:"disposition,omitempty"`
}

// CapabilityAvailability describes a governed capability candidate visible to ICS.
type CapabilityAvailability struct {
	Name                 string                     `json:"name"`
	Kind                 string                     `json:"kind,omitempty"`
	SourceType           string                     `json:"source_type,omitempty"`
	Domain               string                     `json:"domain,omitempty"`
	Available            bool                       `json:"available"`
	Governed             bool                       `json:"governed,omitempty"`
	CommandType          schema.CommandType         `json:"command_type,omitempty"`
	WorkspaceAction      schema.WorkspaceActionType `json:"workspace_action,omitempty"`
	TargetPathArg        string                     `json:"target_path_arg,omitempty"`
	WorkspaceScopedPath  bool                       `json:"workspace_scoped_path,omitempty"`
	RequiresConfirmation bool                       `json:"requires_confirmation,omitempty"`
	SkillName            string                     `json:"skill_name,omitempty"`
	PluginName           string                     `json:"plugin_name,omitempty"`
	ConnectorID          string                     `json:"connector_id,omitempty"`
	RiskHint             schema.RiskLevel           `json:"risk_hint,omitempty"`
	Reversibility        schema.ReversibilityClass  `json:"reversibility,omitempty"`
	ExpectedSideEffects  []string                   `json:"expected_side_effects,omitempty"`
	Reason               string                     `json:"reason,omitempty"`
}

// ContextReference points at relevant non-durable context that informed the cycle.
type ContextReference struct {
	Kind    string `json:"kind,omitempty"`
	ID      string `json:"id,omitempty"`
	Summary string `json:"summary,omitempty"`
	Source  string `json:"source,omitempty"`
}

// FocusFrame represents the active focus selected for this cycle.
type FocusFrame struct {
	FocusID            string         `json:"focus_id"`
	ActiveGoalID       string         `json:"active_goal_id,omitempty"`
	FocusReason        FocusReason    `json:"focus_reason,omitempty"`
	DominantMode       DecisionMode   `json:"dominant_mode,omitempty"`
	SubmodeChain       []DecisionMode `json:"submode_chain,omitempty"`
	PriorityScore      float64        `json:"priority_score,omitempty"`
	StartedAt          time.Time      `json:"started_at"`
	Preemptible        bool           `json:"preemptible,omitempty"`
	PinnedSubreasoners []Subreasoner  `json:"pinned_subreasoners,omitempty"`
}

// CandidateSummary is the compact audit summary for one candidate action.
type CandidateSummary struct {
	CandidateID         string          `json:"candidate_id"`
	CandidateType       CandidateType   `json:"candidate_type"`
	Description         string          `json:"description"`
	TargetCapability    string          `json:"target_capability,omitempty"`
	AllowedCapabilities []string        `json:"allowed_capabilities,omitempty"`
	Score               float64         `json:"score,omitempty"`
	SupportingFactors   []string        `json:"supporting_factors,omitempty"`
	BlockingFactors     []string        `json:"blocking_factors,omitempty"`
	ApprovalNeeded      bool            `json:"approval_needed,omitempty"`
	SelectionReason     string          `json:"selection_reason,omitempty"`
	RejectionCode       RejectionReason `json:"rejection_code,omitempty"`
	RejectionReason     string          `json:"rejection_reason,omitempty"`
}

// ApprovalRef references a required or pending approval artifact.
type ApprovalRef struct {
	Requirement ApprovalRequirement   `json:"requirement"`
	ProposalID  string                `json:"proposal_id,omitempty"`
	Reason      string                `json:"reason,omitempty"`
	Status      schema.ProposalStatus `json:"status,omitempty"`
}

// ThresholdState makes scoring, vetoes, and gating explicit in code.
type ThresholdState struct {
	Score                  float64       `json:"score,omitempty"`
	ScoreBand              ThresholdBand `json:"score_band,omitempty"`
	SoftThresholdSatisfied bool          `json:"soft_threshold_satisfied,omitempty"`
	HardThresholdSatisfied bool          `json:"hard_threshold_satisfied,omitempty"`
	GoverningOverride      bool          `json:"governing_override,omitempty"`
	Vetoes                 []VetoState   `json:"vetoes,omitempty"`
}

// VetoState captures one soft or hard veto signal.
type VetoState struct {
	Source string `json:"source,omitempty"`
	Hard   bool   `json:"hard,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// GovernanceHandoff is the deterministic payload handed to Validate/Govern.
type GovernanceHandoff struct {
	CommandType              schema.CommandType         `json:"command_type,omitempty"`
	Domain                   string                     `json:"domain,omitempty"`
	ActorKind                string                     `json:"actor_kind,omitempty"`
	Intent                   string                     `json:"intent,omitempty"`
	Scope                    string                     `json:"scope,omitempty"`
	TargetCapability         string                     `json:"target_capability,omitempty"`
	AllowedCapabilities      []string                   `json:"allowed_capabilities,omitempty"`
	AllowedCapabilityDetails []CapabilityAvailability   `json:"allowed_capability_details,omitempty"`
	ExpectedSideEffects      []string                   `json:"expected_side_effects,omitempty"`
	Reversibility            schema.ReversibilityClass  `json:"reversibility,omitempty"`
	RiskLevel                schema.RiskLevel           `json:"risk_level,omitempty"`
	ApprovalRequirement      ApprovalRequirement        `json:"approval_requirement,omitempty"`
	ConfirmationRequired     bool                       `json:"confirmation_required,omitempty"`
	Tags                     map[string]string          `json:"tags,omitempty"`
	ValidationOrder          []string                   `json:"validation_order,omitempty"`
	ValidationResult         *governor.ValidationResult `json:"validation_result,omitempty"`
}

// ExecutionIntent is the structured handoff from control into execution.
type ExecutionIntent struct {
	ActionType             string                    `json:"action_type,omitempty"`
	Target                 string                    `json:"target,omitempty"`
	TargetCapability       string                    `json:"target_capability,omitempty"`
	AllowedCapabilities    []string                  `json:"allowed_capabilities,omitempty"`
	RequiredInputs         []string                  `json:"required_inputs,omitempty"`
	ExpectedSideEffects    []string                  `json:"expected_side_effects,omitempty"`
	Reversibility          schema.ReversibilityClass `json:"reversibility,omitempty"`
	RiskLevel              schema.RiskLevel          `json:"risk_level,omitempty"`
	ApprovalRequirement    ApprovalRequirement       `json:"approval_requirement,omitempty"`
	SuccessCondition       string                    `json:"success_condition,omitempty"`
	FailureCondition       string                    `json:"failure_condition,omitempty"`
	FallbackOrRollbackPath string                    `json:"fallback_or_rollback_path,omitempty"`
}

// AuthorizedCapabilities returns the explicit capability boundary selected by ICS.
func (e ExecutionIntent) AuthorizedCapabilities() []string {
	values := make([]string, 0, len(e.AllowedCapabilities)+1)
	if target := strings.TrimSpace(e.TargetCapability); target != "" {
		values = append(values, target)
	}
	for _, allowed := range e.AllowedCapabilities {
		if trimmed := strings.TrimSpace(allowed); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return compactStrings(values)
}

// PrimaryCapability returns the primary execution target when one exists.
func (e ExecutionIntent) PrimaryCapability() string {
	return strings.TrimSpace(e.TargetCapability)
}

// AllowsCapabilityExecution reports whether the intent authorizes executable runtime capabilities.
func (e ExecutionIntent) AllowsCapabilityExecution() bool {
	switch strings.ToLower(strings.TrimSpace(e.ActionType)) {
	case "",
		"compose",
		"respond",
		"clarify",
		"reject",
		"defer",
		"propose",
		"plan":
		return false
	}
	return len(e.AuthorizedCapabilities()) > 0
}

// RecoveryCheckpoint is the structured resume state for interruptions and failures.
type RecoveryCheckpoint struct {
	CurrentGoalID     string                  `json:"current_goal_id,omitempty"`
	CurrentPhase      naviruntime.RunPhase    `json:"current_phase,omitempty"`
	CurrentTask       string                  `json:"current_task,omitempty"`
	CurrentStep       string                  `json:"current_step,omitempty"`
	CheckpointRef     string                  `json:"checkpoint_ref,omitempty"`
	PendingBlockers   []string                `json:"pending_blockers,omitempty"`
	PendingApprovals  []ApprovalRef           `json:"pending_approvals,omitempty"`
	ResumeConditions  []string                `json:"resume_conditions,omitempty"`
	RollbackPoint     string                  `json:"rollback_point,omitempty"`
	ExpiryOrStaleness *time.Time              `json:"expiry_or_staleness,omitempty"`
	Open              bool                    `json:"open,omitempty"`
	Route             RecoveryRoute           `json:"route,omitempty"`
	RuntimeCheckpoint *naviruntime.Checkpoint `json:"-"`
}

// RecoveryRoute captures the concrete recovery action chosen for the current state.
type RecoveryRoute struct {
	Action           RecoveryRouteKind `json:"action,omitempty"`
	Reason           string            `json:"reason,omitempty"`
	ProposalID       string            `json:"proposal_id,omitempty"`
	RequiresApproval bool              `json:"requires_approval,omitempty"`
	KeepOpen         bool              `json:"keep_open,omitempty"`
}

// ReflectionHookSet is the structured reflection payload emitted after decision/execution.
type ReflectionHookSet struct {
	WhatHappened        string       `json:"what_happened,omitempty"`
	ExpectedVsActual    string       `json:"expected_vs_actual,omitempty"`
	SuccessFailureState OutcomeState `json:"success_failure_state,omitempty"`
	Surprises           []string     `json:"surprises,omitempty"`
	LessonCandidate     string       `json:"lesson_candidate,omitempty"`
	MemoryCandidate     string       `json:"memory_candidate,omitempty"`
	ProposalCandidate   string       `json:"proposal_candidate,omitempty"`
	FollowupTrigger     string       `json:"followup_trigger,omitempty"`
}

// RejectionState captures the structured reason and resume context for a rejected path.
type RejectionState struct {
	Reason           RejectionReason `json:"reason,omitempty"`
	Explanation      string          `json:"explanation,omitempty"`
	AffectedGoalID   string          `json:"affected_goal_id,omitempty"`
	Blockers         []string        `json:"blockers,omitempty"`
	ResumeConditions []string        `json:"resume_conditions,omitempty"`
	ProposalID       string          `json:"proposal_id,omitempty"`
}

// Rationale is the canonical ICS decision artifact for one cycle.
type Rationale struct {
	Version             string             `json:"version,omitempty"`
	Focus               FocusFrame         `json:"focus"`
	SelectedGoalID      string             `json:"selected_goal_id,omitempty"`
	GoalStack           GoalStack          `json:"goal_stack,omitempty"`
	Posture             PostureState       `json:"posture,omitempty"`
	DominantMode        DecisionMode       `json:"dominant_mode,omitempty"`
	SubmodeChain        []DecisionMode     `json:"submode_chain,omitempty"`
	InvokedSubreasoners []Subreasoner      `json:"invoked_subreasoners,omitempty"`
	Arbitration         ArbitrationState   `json:"arbitration,omitempty"`
	PlanningStyle       PlanningStyle      `json:"planning_style,omitempty"`
	PlanningDepth       int                `json:"planning_depth,omitempty"`
	ChosenAction        CandidateSummary   `json:"chosen_action"`
	Confidence          float64            `json:"confidence,omitempty"`
	Thresholds          ThresholdState     `json:"thresholds,omitempty"`
	RequiredApprovals   []ApprovalRef      `json:"required_approvals,omitempty"`
	PlanGraph           *PlanGraph         `json:"plan_graph,omitempty"`
	Governance          GovernanceHandoff  `json:"governance,omitempty"`
	ExecutionIntent     ExecutionIntent    `json:"execution_intent,omitempty"`
	Rejection           *RejectionState    `json:"rejection,omitempty"`
	RecoveryCheckpoint  RecoveryCheckpoint `json:"recovery_checkpoint,omitempty"`
	ReflectionHooks     ReflectionHookSet  `json:"reflection_hooks,omitempty"`
	CandidateSummaries  []CandidateSummary `json:"candidate_summaries,omitempty"`
	DecisionTrace       DecisionTrace      `json:"decision_trace,omitempty"`
	Extensions          map[string]any     `json:"extensions,omitempty"`
}

// DecisionSynthesis is the pre-governance artifact produced by Decide.
// It captures focus arbitration, mode routing, candidate evaluation, and rationale.
type DecisionSynthesis struct {
	Focus      FocusSelection   `json:"focus"`
	Mode       ModeSelection    `json:"mode"`
	Candidate  EvaluationResult `json:"candidate"`
	Rationale  Rationale        `json:"rationale"`
	OccurredAt time.Time        `json:"occurred_at,omitempty"`
}
