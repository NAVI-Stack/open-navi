package inference

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/orchestration"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
	navitool "github.com/open-navi/navi/internal/tool"
)

func TestControllerDecideAssemblesMinimalRespondRationale(t *testing.T) {
	controller := NewController()
	fixed := time.Date(2026, 4, 8, 18, 0, 0, 0, time.UTC)
	controller.now = func() time.Time { return fixed }

	decision, err := controller.Decide(context.Background(), InferenceInput{
		Chat: ChatContext{
			ChatID:      "sess-1",
			UserMessage: "help me outline the next step",
		},
		Posture: PostureState{
			Pins: []SubreasonerPin{{
				Reasoner:       SubreasonerInterpreter,
				Source:         "test",
				Reason:         "unit test pin",
				DecayRemaining: 2,
			}},
		},
		GoalStack: GoalStack{
			ActiveGoalID: "goal-1",
			Ready: []GoalRef{{
				GoalID:      "goal-1",
				Status:      GoalStatusActive,
				Priority:    0.62,
				Preemptible: true,
			}},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	rationale := decision.Rationale

	if rationale.Version != ContractVersionV1 {
		t.Fatalf("expected version %q, got %q", ContractVersionV1, rationale.Version)
	}
	if rationale.DominantMode != DecisionModeRespond {
		t.Fatalf("expected dominant mode respond, got %q", rationale.DominantMode)
	}
	if rationale.Focus.FocusReason != FocusReasonUserRequest {
		t.Fatalf("expected user_request focus, got %q", rationale.Focus.FocusReason)
	}
	if rationale.Focus.StartedAt != fixed {
		t.Fatalf("expected fixed focus timestamp, got %s", rationale.Focus.StartedAt)
	}
	if rationale.Focus.PriorityScore <= 0.62 {
		t.Fatalf("expected arbitration score above raw goal priority, got %v", rationale.Focus.PriorityScore)
	}
	if !containsSubreasoner(rationale.Focus.PinnedSubreasoners, SubreasonerInterpreter) {
		t.Fatalf("expected pinned subreasoner passthrough, got %#v", rationale.Focus.PinnedSubreasoners)
	}
	if !containsSubreasoner(rationale.InvokedSubreasoners, SubreasonerInterpreter) {
		t.Fatalf("expected explicit invoked interpreter subreasoner, got %#v", rationale.InvokedSubreasoners)
	}
	if len(rationale.Arbitration.ActivePins) == 0 || len(rationale.Posture.Pins) == 0 {
		t.Fatalf("expected explicit posture/arbitration state, got posture=%#v arbitration=%#v", rationale.Posture, rationale.Arbitration)
	}
	if len(rationale.DecisionTrace.ActivePins) == 0 {
		t.Fatalf("expected decision trace active pins, got %#v", rationale.DecisionTrace)
	}
	if rationale.PlanningStyle != PlanningStyleDirect || rationale.PlanningDepth != 0 {
		t.Fatalf("expected direct respond planning defaults, got %q depth %d", rationale.PlanningStyle, rationale.PlanningDepth)
	}
	if rationale.PlanGraph == nil || rationale.PlanGraph.PlanID == "" {
		t.Fatalf("expected explicit direct plan graph, got %#v", rationale.PlanGraph)
	}
	if len(rationale.PlanGraph.Nodes) != 1 {
		t.Fatalf("expected single-node direct plan, got %#v", rationale.PlanGraph.Nodes)
	}
	if rationale.Confidence <= 0 {
		t.Fatalf("expected explicit positive confidence score, got %v", rationale.Confidence)
	}
	if rationale.DecisionTrace.TraceID != "sess-1" {
		t.Fatalf("expected decision trace id sess-1, got %q", rationale.DecisionTrace.TraceID)
	}
	if len(rationale.DecisionTrace.FocusCandidates) == 0 {
		t.Fatalf("expected explicit focus candidates in trace")
	}
	if rationale.DecisionTrace.PlanID == "" {
		t.Fatalf("expected decision trace to reference plan graph, got %#v", rationale.DecisionTrace)
	}
	if len(rationale.CandidateSummaries) < 2 {
		t.Fatalf("expected explicit candidate set, got %#v", rationale.CandidateSummaries)
	}
}

func TestControllerDecideAssemblesProposalBoundaryFromExplicitState(t *testing.T) {
	controller := NewController()

	decision, err := controller.Decide(context.Background(), InferenceInput{
		Chat: ChatContext{ChatID: "sess-2"},
		Governance: GovernanceState{
			PendingProposalID:     "proposal-1",
			PendingProposalStatus: schema.ProposalStatusPending,
			ConfirmationRequired:  true,
			BlockingReason:        "approval required",
			LastValidation: &governor.ValidationResult{
				Outcome: governor.ValidationRequiresConfirmation,
				Reason:  "approval required",
			},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	rationale := decision.Rationale

	if rationale.DominantMode != DecisionModePropose {
		t.Fatalf("expected dominant mode propose, got %q", rationale.DominantMode)
	}
	if rationale.Focus.FocusReason != FocusReasonPendingProposal {
		t.Fatalf("expected pending proposal focus, got %q", rationale.Focus.FocusReason)
	}
	if len(rationale.RequiredApprovals) != 1 {
		t.Fatalf("expected one approval reference, got %#v", rationale.RequiredApprovals)
	}
	if got := rationale.RequiredApprovals[0].ProposalID; got != "proposal-1" {
		t.Fatalf("expected proposal id proposal-1, got %q", got)
	}
	if rationale.Governance.ApprovalRequirement != ApprovalRequirementBlockingProposal {
		t.Fatalf("expected blocking proposal requirement, got %q", rationale.Governance.ApprovalRequirement)
	}

	if !rationale.Thresholds.GoverningOverride {
		t.Fatalf("expected governing override when approval is required")
	}
	if rationale.Thresholds.ScoreBand != ThresholdBandViable {
		t.Fatalf("expected viable threshold band for proposal path, got %q", rationale.Thresholds.ScoreBand)
	}
	if rationale.PlanGraph == nil || rationale.PlanGraph.Status != PlanStatusBlocked {
		t.Fatalf("expected blocked plan graph for proposal path, got %#v", rationale.PlanGraph)
	}
	if rationale.Governance.ActionDescriptor(ChatContext{ChatID: "sess-2"}).CommandType != schema.CommandTypeAcquire {
		t.Fatalf("expected governance handoff to map to acquire action descriptor")
	}
}

func TestControllerDecideReusesRuntimeCheckpointForRecovery(t *testing.T) {
	controller := NewController()
	run := naviruntime.NewRun("sess-3", "navi")
	run.CurrentPhase = naviruntime.RunPhaseExecute
	checkpoint := naviruntime.NewCheckpoint(run)
	checkpoint.CheckpointID = "cp-1"

	decision, err := controller.Decide(context.Background(), InferenceInput{
		Chat: ChatContext{ChatID: "sess-3"},
		Runtime: RuntimeContext{
			Run:        run,
			Checkpoint: checkpoint,
		},
		Recovery: RecoveryState{
			Status:           schema.RecoveryStatusOpen,
			FailureClass:     schema.FailureClassTimeout,
			FailureReason:    "tool timed out",
			CurrentTask:      "resume execution",
			CurrentStep:      "retry tool",
			PendingBlockers:  []string{"await connector health"},
			ResumeConditions: []string{"connector healthy"},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	rationale := decision.Rationale

	if rationale.DominantMode != DecisionModeRecover {
		t.Fatalf("expected dominant mode recover, got %q", rationale.DominantMode)
	}
	if rationale.RecoveryCheckpoint.CheckpointRef != "cp-1" {
		t.Fatalf("expected checkpoint ref cp-1, got %q", rationale.RecoveryCheckpoint.CheckpointRef)
	}
	if rationale.RecoveryCheckpoint.CurrentPhase != naviruntime.RunPhaseExecute {
		t.Fatalf("expected execute phase, got %q", rationale.RecoveryCheckpoint.CurrentPhase)
	}
	if rationale.RecoveryCheckpoint.RuntimeCheckpoint != checkpoint {
		t.Fatalf("expected runtime checkpoint pointer to be preserved")
	}
	if rationale.DecisionTrace.GovernanceOutcome != "clear" {
		t.Fatalf("expected clear governance trace outcome, got %q", rationale.DecisionTrace.GovernanceOutcome)
	}
	if rationale.Rejection != nil {
		t.Fatalf("did not expect rejection state for recovery path, got %#v", rationale.Rejection)
	}
	if rationale.PlanGraph == nil || rationale.PlanGraph.PlanningStyle != PlanningStyleAdaptive {
		t.Fatalf("expected adaptive recovery plan graph, got %#v", rationale.PlanGraph)
	}
	if rationale.RecoveryCheckpoint.Route.Action != RecoveryRouteRetry {
		t.Fatalf("expected retry route for timeout recovery, got %#v", rationale.RecoveryCheckpoint.Route)
	}
	if rationale.ReflectionHooks.LessonCandidate == "" || rationale.ReflectionHooks.FollowupTrigger == "" {
		t.Fatalf("expected structured reflection hooks for recovery, got %#v", rationale.ReflectionHooks)
	}
}

func TestControllerDecideRejectsUnsupportedVersion(t *testing.T) {
	controller := NewController()

	_, err := controller.Decide(context.Background(), InferenceInput{Version: "ics.v0"})
	if err == nil {
		t.Fatal("expected version error")
	}
}

func TestControllerDecide_DoesNotRunGovernanceValidation(t *testing.T) {
	dep := &mockValidationDependency{
		handoffResult: governor.ValidationResult{
			Outcome: governor.ValidationRequiresConfirmation,
			Reason:  "approval required",
		},
		savedProposal: &schema.Proposal{ProposalID: "proposal-from-validation"},
	}
	controller := NewControllerWithDeps(ControllerDeps{ValidationDependency: dep})

	synthesis, err := controller.Decide(context.Background(), InferenceInput{
		Chat: ChatContext{
			ChatID:      "sess-governance-free",
			UserMessage: "execute runtime echo",
		},
		NCOS: orchestration.CanonicalRunRequest{
			RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
		},
		Capabilities: []CapabilityAvailability{{
			Name:        "runtime_echo",
			Available:   true,
			Governed:    true,
			CommandType: schema.CommandTypeInvoke,
		}},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if dep.lastHandoff.CommandType != "" {
		t.Fatalf("expected Decide to avoid calling ValidateCandidate/ValidateHandoff, got %+v", dep.lastHandoff)
	}
	if synthesis.Rationale.DecisionTrace.ProposalID != "" {
		t.Fatalf("expected Decide synthesis to avoid proposal outcomes, got %#v", synthesis.Rationale.DecisionTrace)
	}
}

func TestControllerDecide_DoesNotResolvePendingProposalState(t *testing.T) {
	getProposalCalls := 0
	controller := NewControllerWithDeps(ControllerDeps{
		GetProposal: func(ctx context.Context, proposalID string) (schema.Proposal, error) {
			getProposalCalls++
			return schema.Proposal{ProposalID: proposalID}, nil
		},
	})

	synthesis, err := controller.Decide(context.Background(), InferenceInput{
		Chat: ChatContext{ChatID: "sess-proposal"},
		Governance: GovernanceState{
			PendingProposalID:     "proposal-1",
			PendingProposalStatus: schema.ProposalStatusPending,
			ConfirmationRequired:  true,
			BlockingReason:        "approval required",
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if getProposalCalls != 0 {
		t.Fatalf("expected Decide to avoid proposal resolution side effects, got %d calls", getProposalCalls)
	}
	if synthesis.Rationale.Focus.FocusReason != FocusReasonPendingProposal {
		t.Fatalf("expected pending proposal focus in synthesis, got %q", synthesis.Rationale.Focus.FocusReason)
	}
	if synthesis.Rationale.RequiredApprovals[0].ProposalID != "proposal-1" {
		t.Fatalf("expected Decide to preserve pending proposal as synthesis input state, got %#v", synthesis.Rationale.RequiredApprovals)
	}
}

func TestValidateGovernedDecision_ResolvesProposalBoundaryAfterDecide(t *testing.T) {
	getProposalCalls := 0
	controller := NewControllerWithDeps(ControllerDeps{
		GetProposal: func(ctx context.Context, proposalID string) (schema.Proposal, error) {
			getProposalCalls++
			return schema.Proposal{
				ProposalID: proposalID,
				Status:     schema.ProposalStatusPending,
			}, nil
		},
	})
	input := InferenceInput{
		Chat: ChatContext{ChatID: "sess-validate"},
		Governance: GovernanceState{
			PendingProposalID:     "proposal-validate-1",
			PendingProposalStatus: schema.ProposalStatusPending,
			ConfirmationRequired:  true,
			BlockingReason:        "approval required",
		},
	}

	synthesis, err := controller.Decide(context.Background(), input)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if getProposalCalls != 0 {
		t.Fatalf("expected Decide to remain side-effect free, got %d proposal lookups", getProposalCalls)
	}

	envelope, err := controller.ValidateGovernedDecision(context.Background(), input, synthesis)
	if err != nil {
		t.Fatalf("ValidateGovernedDecision: %v", err)
	}
	if envelope.RuntimeDisposition != RuntimeDispositionPauseForProposal {
		t.Fatalf("expected proposal pause disposition, got %q", envelope.RuntimeDisposition)
	}
	if envelope.Proposal == nil || envelope.Proposal.ProposalID != "proposal-validate-1" {
		t.Fatalf("expected resolved proposal in governed envelope, got %#v", envelope.Proposal)
	}
	if getProposalCalls != 1 {
		t.Fatalf("expected governed entrypoint to resolve proposal once, got %d", getProposalCalls)
	}
}

func TestControllerDecideAppliesModeRouterAndTraceForExecute(t *testing.T) {
	controller := NewController()

	decision, err := controller.Decide(context.Background(), InferenceInput{
		Chat: ChatContext{
			ChatID:      "sess-4",
			UserMessage: "apply the change",
		},
		GoalStack: GoalStack{
			ActiveGoalID: "goal-4",
			Ready: []GoalRef{
				{GoalID: "goal-4", Priority: 0.9, Preemptible: true},
			},
		},
		NCOS: orchestration.CanonicalRunRequest{
			RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
		},
		Capabilities: []CapabilityAvailability{
			{Name: "write_file", Available: true, Governed: true},
		},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	rationale := decision.Rationale

	if rationale.DominantMode != DecisionModeExecute {
		t.Fatalf("expected execute dominant mode, got %q", rationale.DominantMode)
	}
	if len(rationale.SubmodeChain) != 1 || rationale.SubmodeChain[0] != DecisionModePlan {
		t.Fatalf("expected bounded plan submode, got %#v", rationale.SubmodeChain)
	}
	if rationale.PlanningStyle != PlanningStyleShallow || rationale.PlanningDepth != 1 {
		t.Fatalf("expected shallow execute planning, got %q depth %d", rationale.PlanningStyle, rationale.PlanningDepth)
	}
	if rationale.PlanGraph == nil || len(rationale.PlanGraph.Nodes) < 2 {
		t.Fatalf("expected shallow execute plan graph, got %#v", rationale.PlanGraph)
	}
	if len(rationale.PlanGraph.Checkpoints) == 0 {
		t.Fatalf("expected checkpoint-aware execute plan, got %#v", rationale.PlanGraph)
	}
	if !rationale.DecisionTrace.Mode.Changed && rationale.DecisionTrace.Mode.Previous != "" {
		t.Fatalf("expected explicit mode transition state, got %#v", rationale.DecisionTrace.Mode)
	}
	if len(rationale.DecisionTrace.CandidateIDs) != len(rationale.CandidateSummaries) {
		t.Fatalf("expected decision trace candidate ids to match candidate set, got trace=%#v summaries=%#v", rationale.DecisionTrace.CandidateIDs, rationale.CandidateSummaries)
	}
	if len(rationale.DecisionTrace.CandidateIDs) < 2 {
		t.Fatalf("expected decision trace to retain considered candidates, got %#v", rationale.DecisionTrace.CandidateIDs)
	}
	if rationale.DecisionTrace.PlanID == "" || rationale.DecisionTrace.CurrentNodeID == "" {
		t.Fatalf("expected decision trace plan references, got %#v", rationale.DecisionTrace)
	}
	if rationale.Governance.CommandType != schema.CommandTypeInvoke {
		t.Fatalf("expected invoke command type for execute path, got %q", rationale.Governance.CommandType)
	}
}

func TestControllerObserveOutcomeCompletesPlanAndClosesRecoveryOnSuccess(t *testing.T) {
	controller := NewController()
	prior := Rationale{
		PlanGraph: &PlanGraph{
			PlanID:        "plan-1",
			CurrentNodeID: "node-act",
			Status:        PlanStatusActive,
			Nodes: []PlanNode{
				{NodeID: "node-prepare", Status: PlanNodeStatusCompleted},
				{NodeID: "node-act", Status: PlanNodeStatusReady},
			},
		},
		RecoveryCheckpoint: RecoveryCheckpoint{
			CheckpointRef: "cp-1",
			Open:          true,
			Route:         RecoveryRoute{Action: RecoveryRouteRecover, KeepOpen: true},
		},
		ReflectionHooks: ReflectionHookSet{
			SuccessFailureState: OutcomeStatePending,
			FollowupTrigger:     "continue_structured_recovery",
		},
		DecisionTrace: DecisionTrace{
			TraceID:       "trace-1",
			PlanID:        "plan-1",
			PlanStatus:    PlanStatusActive,
			CurrentNodeID: "node-act",
			RecoveryRoute: RecoveryRouteRecover,
		},
	}

	updatedDecision, err := controller.ObserveOutcome(context.Background(), defaultDecisionEnvelope(prior), ExecutionSnapshot{
		Outcome: schema.ExecutionOutcomeSucceeded,
		Summary: "runtime action completed successfully",
	})
	if err != nil {
		t.Fatalf("ObserveOutcome: %v", err)
	}
	updated := updatedDecision.Rationale

	if updated.PlanGraph == nil || updated.PlanGraph.Status != PlanStatusCompleted {
		t.Fatalf("expected completed supervised plan graph, got %#v", updated.PlanGraph)
	}
	if updated.PlanGraph.CurrentNodeID != "" {
		t.Fatalf("expected no current node after completion, got %q", updated.PlanGraph.CurrentNodeID)
	}
	if updated.RecoveryCheckpoint.Open {
		t.Fatalf("expected recovery checkpoint to close on success, got %#v", updated.RecoveryCheckpoint)
	}
	if updated.RecoveryCheckpoint.Route.Action != RecoveryRouteNone {
		t.Fatalf("expected no recovery route after success, got %#v", updated.RecoveryCheckpoint.Route)
	}
	if updated.ReflectionHooks.SuccessFailureState != OutcomeStateSuccess {
		t.Fatalf("expected success reflection state, got %#v", updated.ReflectionHooks)
	}
	if updated.ReflectionHooks.FollowupTrigger != "" {
		t.Fatalf("expected no follow-up trigger after success, got %#v", updated.ReflectionHooks)
	}
}

func TestControllerObserveOutcomeBlocksPlanAndOpensProposalRecovery(t *testing.T) {
	controller := NewController()
	prior := Rationale{
		PlanGraph: &PlanGraph{
			PlanID:        "plan-2",
			CurrentNodeID: "node-act",
			Status:        PlanStatusActive,
			Nodes: []PlanNode{
				{NodeID: "node-act", Status: PlanNodeStatusReady},
			},
		},
		RecoveryCheckpoint: RecoveryCheckpoint{
			CheckpointRef: "cp-2",
		},
		ReflectionHooks: ReflectionHookSet{
			SuccessFailureState: OutcomeStatePending,
		},
	}

	updatedDecision, err := controller.ObserveOutcome(context.Background(), defaultDecisionEnvelope(prior), ExecutionSnapshot{
		Outcome:    schema.ExecutionOutcomeRejectedPreExecution,
		ProposalID: "proposal-2",
		Summary:    "approval required before execution can continue",
	})
	if err != nil {
		t.Fatalf("ObserveOutcome: %v", err)
	}
	updated := updatedDecision.Rationale

	if updated.PlanGraph == nil || updated.PlanGraph.Status != PlanStatusBlocked {
		t.Fatalf("expected blocked supervised plan graph, got %#v", updated.PlanGraph)
	}
	if len(updated.PlanGraph.Nodes) == 0 || updated.PlanGraph.Nodes[0].Status != PlanNodeStatusBlocked {
		t.Fatalf("expected blocked current plan node, got %#v", updated.PlanGraph.Nodes)
	}
	if !updated.RecoveryCheckpoint.Open || updated.RecoveryCheckpoint.Route.Action != RecoveryRoutePropose {
		t.Fatalf("expected open proposal recovery route, got %#v", updated.RecoveryCheckpoint)
	}
	if updated.RecoveryCheckpoint.Route.ProposalID != "proposal-2" {
		t.Fatalf("expected proposal id to propagate into recovery route, got %#v", updated.RecoveryCheckpoint.Route)
	}
	if updated.ReflectionHooks.SuccessFailureState != OutcomeStateBlocked {
		t.Fatalf("expected blocked reflection state, got %#v", updated.ReflectionHooks)
	}
	if updated.ReflectionHooks.FollowupTrigger != "await_proposal_resolution" {
		t.Fatalf("expected proposal follow-up trigger, got %#v", updated.ReflectionHooks)
	}
	if updated.DecisionTrace.PlanStatus != PlanStatusBlocked || updated.DecisionTrace.RecoveryRoute != RecoveryRoutePropose {
		t.Fatalf("expected supervised trace to reflect blocked proposal state, got %#v", updated.DecisionTrace)
	}
}

func TestControllerObserveOutcomeMarksFailedNodeAndRoutesRecoveryFromActualFailure(t *testing.T) {
	controller := NewController()
	prior := Rationale{
		SelectedGoalID: "goal-failure",
		Focus: FocusFrame{
			FocusID:      "focus-failure",
			ActiveGoalID: "goal-failure",
			DominantMode: DecisionModeExecute,
		},
		PlanGraph: &PlanGraph{
			PlanID:        "plan-failure",
			GoalID:        "goal-failure",
			CurrentNodeID: "node-act",
			Status:        PlanStatusActive,
			Nodes: []PlanNode{
				{NodeID: "node-prepare", Status: PlanNodeStatusCompleted},
				{NodeID: "node-act", Status: PlanNodeStatusReady, TargetCapability: "runtime_echo", CheckpointBoundary: true},
			},
		},
		Governance: GovernanceHandoff{
			TargetCapability:    "runtime_echo",
			AllowedCapabilities: []string{"runtime_echo"},
			AllowedCapabilityDetails: []CapabilityAvailability{{
				Name:        "runtime_echo",
				Available:   true,
				Governed:    true,
				CommandType: schema.CommandTypeInvoke,
			}},
		},
		ExecutionIntent: ExecutionIntent{
			ActionType:          string(schema.CommandTypeInvoke),
			TargetCapability:    "runtime_echo",
			AllowedCapabilities: []string{"runtime_echo"},
		},
		RecoveryCheckpoint: RecoveryCheckpoint{
			CheckpointRef: "cp-failure",
			Open:          true,
			Route:         RecoveryRoute{Action: RecoveryRouteRecover, KeepOpen: true},
		},
		ReflectionHooks: ReflectionHookSet{
			SuccessFailureState: OutcomeStatePending,
		},
		DecisionTrace: DecisionTrace{
			TraceID:       "trace-failure",
			PlanID:        "plan-failure",
			PlanStatus:    PlanStatusActive,
			CurrentNodeID: "node-act",
			RecoveryRoute: RecoveryRouteRecover,
		},
	}

	updatedDecision, err := controller.ObserveOutcome(context.Background(), defaultDecisionEnvelope(prior), ExecutionSnapshot{
		Outcome:            schema.ExecutionOutcomeFailed,
		ExecutedCapability: "runtime_echo",
		CommandType:        schema.CommandTypeInvoke,
		FailureClass:       schema.FailureClassExecutionFailure,
		Summary:            "runtime execution failed",
	})
	if err != nil {
		t.Fatalf("ObserveOutcome: %v", err)
	}
	updated := updatedDecision.Rationale

	if updated.PlanGraph == nil || updated.PlanGraph.Status != PlanStatusFailed {
		t.Fatalf("expected failed supervised plan graph, got %#v", updated.PlanGraph)
	}
	if len(updated.PlanGraph.Nodes) < 2 || updated.PlanGraph.Nodes[1].Status != PlanNodeStatusFailed {
		t.Fatalf("expected failed current plan node, got %#v", updated.PlanGraph.Nodes)
	}
	if !updated.RecoveryCheckpoint.Open || updated.RecoveryCheckpoint.Route.Action != RecoveryRouteRecover {
		t.Fatalf("expected failed execution to keep an explicit recover route, got %#v", updated.RecoveryCheckpoint)
	}
	if updated.DominantMode != DecisionModeRecover || updated.ChosenAction.CandidateType != CandidateTypeRecover {
		t.Fatalf("expected post-failure control state to move into recover mode, got mode=%q candidate=%q", updated.DominantMode, updated.ChosenAction.CandidateType)
	}
	if updated.Governance.TargetCapability != "runtime_echo" || len(updated.Governance.AllowedCapabilities) != 1 {
		t.Fatalf("expected failure supervision to keep the governed boundary bounded to runtime_echo, got %#v", updated.Governance)
	}
	if updated.DecisionTrace.PlanStatus != PlanStatusFailed || updated.DecisionTrace.RecoveryRoute != RecoveryRouteRecover {
		t.Fatalf("expected failed execution trace to reflect recovery routing, got %#v", updated.DecisionTrace)
	}
}

func TestControllerObserveOutcomeKeepsPartialExecutionRecoveryAndReflectionAuthoritative(t *testing.T) {
	controller := NewController()
	prior := Rationale{
		SelectedGoalID: "goal-partial",
		Focus: FocusFrame{
			FocusID:      "focus-partial",
			ActiveGoalID: "goal-partial",
			DominantMode: DecisionModeExecute,
		},
		PlanGraph: &PlanGraph{
			PlanID:        "plan-partial",
			GoalID:        "goal-partial",
			CurrentNodeID: "node-act",
			Status:        PlanStatusActive,
			Nodes: []PlanNode{
				{NodeID: "node-act", Status: PlanNodeStatusReady, TargetCapability: "runtime_echo", CheckpointBoundary: true},
			},
		},
		RecoveryCheckpoint: RecoveryCheckpoint{
			CheckpointRef: "cp-partial",
			Open:          true,
			Route:         RecoveryRoute{Action: RecoveryRouteRecover, KeepOpen: true},
		},
		ReflectionHooks: ReflectionHookSet{},
		DecisionTrace: DecisionTrace{
			TraceID:       "trace-partial",
			PlanID:        "plan-partial",
			PlanStatus:    PlanStatusActive,
			CurrentNodeID: "node-act",
			RecoveryRoute: RecoveryRouteRecover,
		},
	}

	updatedDecision, err := controller.ObserveOutcome(context.Background(), defaultDecisionEnvelope(prior), ExecutionSnapshot{
		Outcome:            schema.ExecutionOutcomePartiallySucceeded,
		ExecutedCapability: "runtime_echo",
		CommandType:        schema.CommandTypeInvoke,
		FailureClass:       schema.FailureClassPartialExecution,
		CheckpointRef:      "cp-partial-2",
		Summary:            "partial execution still requires compensation",
	})
	if err != nil {
		t.Fatalf("ObserveOutcome: %v", err)
	}
	updated := updatedDecision.Rationale

	if updated.PlanGraph == nil || updated.PlanGraph.Status != PlanStatusBlocked {
		t.Fatalf("expected partial execution to block the plan graph, got %#v", updated.PlanGraph)
	}
	if updated.RecoveryCheckpoint.Route.Action != RecoveryRouteCompensate || !updated.RecoveryCheckpoint.Open {
		t.Fatalf("expected partial execution to stay in explicit compensation recovery, got %#v", updated.RecoveryCheckpoint)
	}
	if updated.RecoveryCheckpoint.CheckpointRef != "cp-partial-2" {
		t.Fatalf("expected recovery checkpoint ref to update from actual outcome, got %#v", updated.RecoveryCheckpoint)
	}
	if updated.ReflectionHooks.ExpectedVsActual != "partial execution still requires compensation" {
		t.Fatalf("expected reflection expected-vs-actual to come from the actual outcome, got %#v", updated.ReflectionHooks)
	}
	if updated.ReflectionHooks.FollowupTrigger != "compensate_partial_execution" {
		t.Fatalf("expected partial execution reflection follow-up to stay on compensation, got %#v", updated.ReflectionHooks)
	}
	if updated.DecisionTrace.Stage != "post_execution" || updated.DecisionTrace.RecoveryRef != "cp-partial-2" {
		t.Fatalf("expected post-execution trace to include actual checkpoint refs, got %#v", updated.DecisionTrace)
	}
}

func TestAuthorizeModelResponse_BlocksWhenToolAttemptMissing(t *testing.T) {
	controller := NewController()
	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"runtime_echo"}))
	prior.ModelDirective = &ModelDirective{
		AllowToolCalls: true,
		ToolNames:      []string{"runtime_echo"},
		ToolChoice:     "runtime_echo",
		ToolContracts: map[string]ToolContract{
			"runtime_echo": testToolContract("runtime_echo", schema.CommandTypeInvoke, "testing", schema.WorkspaceActionExecute),
		},
	}

	decision, err := controller.AuthorizeModelResponse(context.Background(), prior, ModelResponseAuthorizationInput{
		Response: orchestration.NormalizedModelResponse{
			ToolCalls: []llm.ToolCall{{
				ID:   "tc-missing-attempt",
				Name: "runtime_echo",
			}},
		},
	})
	if err != nil {
		t.Fatalf("AuthorizeModelResponse: %v", err)
	}
	if decision.RuntimeDisposition != RuntimeDispositionBlockWithReply {
		t.Fatalf("expected fail-closed block, got %s", decision.RuntimeDisposition)
	}
	if len(decision.AuthorizedTools) != 0 {
		t.Fatalf("expected no authorized executable permits, got %#v", decision.AuthorizedTools)
	}
}

func TestAuthorizeModelResponse_BlocksWhenAttemptContractMissing(t *testing.T) {
	controller := NewController()
	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"runtime_echo"}))
	prior.ModelDirective = &ModelDirective{
		AllowToolCalls: true,
		ToolNames:      []string{"runtime_echo"},
		ToolChoice:     "runtime_echo",
		ToolContracts: map[string]ToolContract{
			"runtime_echo": testToolContract("runtime_echo", schema.CommandTypeInvoke, "testing", schema.WorkspaceActionExecute),
		},
	}

	decision, err := controller.AuthorizeModelResponse(context.Background(), prior, ModelResponseAuthorizationInput{
		Response: orchestration.NormalizedModelResponse{
			ToolCalls: []llm.ToolCall{{
				ID:   "tc-missing-contract",
				Name: "runtime_echo",
			}},
		},
		ToolAttempts: []ToolAuthorizationInput{{
			ToolCallID: "tc-missing-contract",
			ToolName:   "runtime_echo",
		}},
	})
	if err != nil {
		t.Fatalf("AuthorizeModelResponse: %v", err)
	}
	if decision.RuntimeDisposition != RuntimeDispositionBlockWithReply {
		t.Fatalf("expected fail-closed block, got %s", decision.RuntimeDisposition)
	}
	if len(decision.AuthorizedTools) != 0 {
		t.Fatalf("expected no authorized executable permits, got %#v", decision.AuthorizedTools)
	}
}

func TestAuthorizeModelResponse_NeverEmitsBareExecutablePermit(t *testing.T) {
	controller := NewControllerWithDeps(ControllerDeps{
		ValidationDependency: &mockValidationDependency{
			handoffResult: governor.ValidationResult{Outcome: governor.ValidationApproved},
		},
	})
	contract := testToolContract("runtime_echo", schema.CommandTypeInvoke, "testing", schema.WorkspaceActionExecute)
	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"runtime_echo"}))
	prior.ModelDirective = &ModelDirective{
		AllowToolCalls: true,
		ToolNames:      []string{"runtime_echo"},
		ToolChoice:     "runtime_echo",
		ToolContracts: map[string]ToolContract{
			"runtime_echo": contract,
		},
	}

	decision, err := controller.AuthorizeModelResponse(context.Background(), prior, ModelResponseAuthorizationInput{
		Response: orchestration.NormalizedModelResponse{
			ToolCalls: []llm.ToolCall{{
				ID:   "tc-valid-contract",
				Name: "runtime_echo",
			}},
		},
		ToolAttempts: []ToolAuthorizationInput{{
			ToolCallID: "tc-valid-contract",
			ToolName:   "runtime_echo",
			Contract:   &contract,
		}},
	})
	if err != nil {
		t.Fatalf("AuthorizeModelResponse: %v", err)
	}
	if decision.RuntimeDisposition == RuntimeDispositionBlockWithReply {
		t.Fatalf("expected successful authorization, got block %q", decision.ReplyMessage)
	}
	if len(decision.AuthorizedTools) != 1 {
		t.Fatalf("expected one authorized executable permit, got %#v", decision.AuthorizedTools)
	}
	permit := decision.AuthorizedTools[0].Permit
	if permit.Contract == nil || permit.ContractID == "" {
		t.Fatalf("expected authoritative contract-backed permit, got %#v", permit)
	}
}

func TestAuthorizeModelResponse_RecordsUnknownToolRecoveryWithoutRawLeak(t *testing.T) {
	controller := NewControllerWithDeps(ControllerDeps{
		ValidationDependency: &mockValidationDependency{
			handoffResult: governor.ValidationResult{Outcome: governor.ValidationApproved},
		},
	})
	reg := navitool.NewRegistry()
	related := navitool.Tool{
		ToolID:        "navi.calendar.read_availability",
		DisplayName:   "Calendar Availability",
		Description:   "Read calendar availability windows.",
		Source:        navitool.ToolSourceBuiltin,
		SourceID:      "tests.registry",
		SchemaVersion: "1.0.0",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		OutputSchema: map[string]any{
			"type": "string",
		},
		Category:              navitool.ToolCategoryReadOnly,
		RiskTier:              "low",
		SideEffects:           []string{},
		Reversibility:         "reversible",
		EnvironmentVisibility: []string{"development", "production"},
		RequiredModes:         []schema.DirectiveMode{},
		RequiredAuthority:     navitool.ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{"calendar lookup"},
		CapabilityTags:        []string{"calendar", "availability", "schedule"},
		Status:                navitool.ToolStatusActive,
		Definition: llm.ToolDefinition{
			Name:        "navi.calendar.read_availability",
			Description: "Read calendar availability windows.",
			Parameters:  map[string]any{"type": "object"},
		},
	}
	if err := reg.Register(&related); err != nil {
		t.Fatalf("register related tool: %v", err)
	}

	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"runtime_echo"}))
	prior.ModelDirective = &ModelDirective{
		AllowToolCalls: true,
		ToolNames:      []string{"runtime_echo"},
		ToolChoice:     "runtime_echo",
		ToolContracts: map[string]ToolContract{
			"runtime_echo": testToolContract("runtime_echo", schema.CommandTypeInvoke, "testing", schema.WorkspaceActionExecute),
		},
	}

	decision, err := controller.AuthorizeModelResponse(context.Background(), prior, ModelResponseAuthorizationInput{
		Compiled: orchestration.CompiledModelRequest{
			Profile: orchestration.ModelProfile{
				Model:         "frontier_strong",
				SupportsTools: true,
			},
		},
		Response: orchestration.NormalizedModelResponse{
			ToolCalls: []llm.ToolCall{{
				ID:   "tc-unknown-calendar",
				Name: "navi.calendar.lookup",
			}},
		},
		ToolRegistry:            reg,
		RepairAttemptsRemaining: 1,
	})
	if err != nil {
		t.Fatalf("AuthorizeModelResponse: %v", err)
	}
	if decision.RuntimeDisposition != RuntimeDispositionBlockWithReply {
		t.Fatalf("expected fail-closed block, got %s", decision.RuntimeDisposition)
	}
	if strings.Contains(decision.ReplyMessage, "out-of-bound tool call") {
		t.Fatalf("expected sanitized recovery message, got %q", decision.ReplyMessage)
	}
	rawRecoveries, ok := decision.Rationale.Extensions["tool_call_recovery"]
	if !ok {
		t.Fatalf("expected structured tool recovery extension, got %#v", decision.Rationale.Extensions)
	}
	recoveries, ok := rawRecoveries.([]navitool.ToolCallRecoveryResult)
	if !ok || len(recoveries) != 1 {
		t.Fatalf("expected typed recovery results, got %#v", rawRecoveries)
	}
	recovery := recoveries[0]
	if !recovery.Hallucinated || recovery.Disposition != navitool.ToolCallRecoveryDispositionUnknownTool {
		t.Fatalf("expected hallucinated unknown-tool recovery, got %+v", recovery)
	}
	if !containsString(recovery.RelatedToolIDs, "navi.calendar.read_availability") {
		t.Fatalf("expected related discovery match, got %+v", recovery.RelatedToolIDs)
	}
}

func TestToolPermit_DoesNotCarrySecondAuthoritySurface(t *testing.T) {
	tp := reflect.TypeOf(ToolPermit{})
	for _, field := range []string{"CommandType", "Domain", "ActorKind", "WorkspaceAction"} {
		if _, ok := tp.FieldByName(field); ok {
			t.Fatalf("expected ToolPermit to stop carrying mirrored authority field %q", field)
		}
	}
}

func TestToolAuthorizationInput_DoesNotCarryLegacyAuthoritySurface(t *testing.T) {
	attemptType := reflect.TypeOf(ToolAuthorizationInput{})
	for _, field := range []string{"CommandType", "Domain", "ActorKind", "WorkspaceAction", "TargetPath", "SkillEntry"} {
		if _, ok := attemptType.FieldByName(field); ok {
			t.Fatalf("expected ToolAuthorizationInput to stop carrying mirrored authority field %q", field)
		}
	}
}

func TestControllerDecide_ExecutableTargetUsesCapabilityInsteadOfRunOrSessionFallback(t *testing.T) {
	controller := NewController()

	decision, err := controller.Decide(context.Background(), InferenceInput{
		Chat: ChatContext{
			ChatID:      "sess-target",
			UserMessage: "execute runtime echo",
		},
		Runtime: RuntimeContext{
			RunID: "run-target",
		},
		GoalStack: GoalStack{
			Ready: []GoalRef{{GoalID: "goal-target", Priority: 0.9, Preemptible: true}},
		},
		NCOS: orchestration.CanonicalRunRequest{
			RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
		},
		Capabilities: []CapabilityAvailability{{
			Name:        "runtime_echo",
			Available:   true,
			Governed:    true,
			CommandType: schema.CommandTypeInvoke,
		}},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if got := decision.Rationale.ExecutionIntent.Target; got != "runtime_echo" {
		t.Fatalf("expected executable target to resolve to capability, got %q", got)
	}
}

func TestControllerDecide_ExecutableScopeAvoidsSessionFallback(t *testing.T) {
	controller := NewController()

	decision, err := controller.Decide(context.Background(), InferenceInput{
		Chat: ChatContext{
			ChatID:      "sess-scope",
			UserMessage: "execute runtime echo",
		},
		NCOS: orchestration.CanonicalRunRequest{
			RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
		},
		Capabilities: []CapabilityAvailability{{
			Name:        "runtime_echo",
			Kind:        "testing",
			Available:   true,
			Governed:    true,
			CommandType: schema.CommandTypeInvoke,
		}},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if got := decision.Rationale.Governance.Scope; got == "sess-scope" || got == "" {
		t.Fatalf("expected executable governance scope to avoid session fallback, got %q", got)
	}
}

func TestControllerArchitecture_NoLegacyAuthorityAdapterProductionReferences(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	productionRoot := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	for _, forbidden := range []string{"AuthorityAdapter", "NewControllerWithAdapter"} {
		err := filepath.WalkDir(productionRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("expected production code to stop referencing %q, found in %s", forbidden, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk production files for %q: %v", forbidden, err)
		}
	}
}

func TestNormalizePostureState_PinsAreAuthoritative(t *testing.T) {
	normalized := normalizePostureState(PostureState{
		PinnedSubreasoners: []Subreasoner{SubreasonerInterpreter},
	})
	if len(normalized.Pins) != 0 || len(normalized.PinnedSubreasoners) != 0 {
		t.Fatalf("expected legacy pinned subreasoners to stop acting as authoritative input, got %#v", normalized)
	}

	normalized = normalizePostureState(PostureState{
		Pins: []SubreasonerPin{{
			Reasoner:       SubreasonerInterpreter,
			Source:         "test",
			Reason:         "authoritative pin",
			DecayRemaining: 2,
		}},
	})
	if !containsSubreasoner(normalized.PinnedSubreasoners, SubreasonerInterpreter) {
		t.Fatalf("expected derived pinned subreasoners from authoritative pins, got %#v", normalized)
	}
}
