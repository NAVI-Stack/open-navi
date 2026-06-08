package inference

import (
	"testing"

	naviruntime "github.com/open-navi/navi/internal/runtime"
)

func TestPlanGraphManagerBuildsDirectSingleNodePlan(t *testing.T) {
	manager := DefaultPlanGraphManager{}

	plan := manager.Build(
		InferenceInput{
			Chat: ChatContext{ChatID: "sess-plan-direct"},
		},
		FocusFrame{FocusID: "focus-1", ActiveGoalID: "goal-1"},
		ModeSelection{PlanningStyle: PlanningStyleDirect},
		CandidateSummary{
			CandidateID:   "candidate:respond",
			CandidateType: CandidateTypeRespond,
			Description:   "respond to the user",
		},
	)

	if plan == nil {
		t.Fatal("expected direct plan graph")
	}
	if plan.PlanningStyle != PlanningStyleDirect {
		t.Fatalf("expected direct planning style, got %q", plan.PlanningStyle)
	}
	if len(plan.Nodes) != 1 {
		t.Fatalf("expected single direct node, got %#v", plan.Nodes)
	}
	if plan.CurrentNodeID == "" || plan.CurrentNodeID != plan.Nodes[0].NodeID {
		t.Fatalf("expected current node to point at direct node, got %#v", plan)
	}
}

func TestPlanGraphManagerBuildsAdaptiveDecompositionWithCheckpointBoundary(t *testing.T) {
	manager := DefaultPlanGraphManager{}
	run := naviruntime.NewRun("sess-plan-adaptive", "navi")
	checkpoint := naviruntime.NewCheckpoint(run)
	checkpoint.CheckpointID = "cp-adaptive"

	plan := manager.Build(
		InferenceInput{
			Runtime: RuntimeContext{
				Run:        run,
				Checkpoint: checkpoint,
			},
			Recovery: RecoveryState{
				PendingBlockers: []string{"await connector recovery"},
			},
		},
		FocusFrame{FocusID: "focus-2", ActiveGoalID: "goal-2"},
		ModeSelection{PlanningStyle: PlanningStyleAdaptive},
		CandidateSummary{
			CandidateID:   "candidate:recover",
			CandidateType: CandidateTypeRecover,
			Description:   "recover the interrupted task",
		},
	)

	if plan == nil {
		t.Fatal("expected adaptive plan graph")
	}
	if plan.PlanningStyle != PlanningStyleAdaptive {
		t.Fatalf("expected adaptive planning style, got %q", plan.PlanningStyle)
	}
	if len(plan.Nodes) < 3 {
		t.Fatalf("expected adaptive upfront decomposition, got %#v", plan.Nodes)
	}
	if len(plan.Checkpoints) == 0 {
		t.Fatalf("expected checkpoint-aware boundaries, got %#v", plan.Checkpoints)
	}
	if plan.Checkpoints[0].CheckpointRef != "cp-adaptive" {
		t.Fatalf("expected runtime checkpoint ref reuse, got %#v", plan.Checkpoints)
	}
}

func TestPlanGraphManagerUsesPlanningDepthForDeeperProgressiveDecomposition(t *testing.T) {
	manager := DefaultPlanGraphManager{}

	plan := manager.Build(
		InferenceInput{
			Chat: ChatContext{ChatID: "sess-plan-deep"},
		},
		FocusFrame{FocusID: "focus-3", ActiveGoalID: "goal-3"},
		ModeSelection{PlanningStyle: PlanningStyleProgressive, PlanningDepth: 2},
		CandidateSummary{
			CandidateID:   "candidate:plan",
			CandidateType: CandidateTypePlan,
			Description:   "plan the release work",
		},
	)

	if plan == nil {
		t.Fatal("expected deep progressive plan graph")
	}
	if len(plan.Nodes) < 4 {
		t.Fatalf("expected deeper upfront decomposition, got %#v", plan.Nodes)
	}
	if plan.Nodes[1].NodeID != "candidate:plan:decompose" {
		t.Fatalf("expected explicit decomposition node, got %#v", plan.Nodes)
	}
}

func TestPlanGraphManagerResumesExistingPlanState(t *testing.T) {
	manager := DefaultPlanGraphManager{}

	plan := manager.Build(
		InferenceInput{
			PlanState: &PlanGraph{
				PlanID:        "plan:goal-3",
				GoalID:        "goal-3",
				PlanningStyle: PlanningStyleProgressive,
				Nodes: []PlanNode{
					{NodeID: "candidate:plan:stabilize", Status: PlanNodeStatusCompleted},
					{NodeID: "candidate:plan:next_step", Status: PlanNodeStatusReady},
				},
				CurrentNodeID: "candidate:plan:next_step",
				Status:        PlanStatusActive,
			},
		},
		FocusFrame{FocusID: "focus-4", ActiveGoalID: "goal-3"},
		ModeSelection{PlanningStyle: PlanningStyleProgressive},
		CandidateSummary{
			CandidateID:   "candidate:plan",
			CandidateType: CandidateTypePlan,
			Description:   "plan the release work",
		},
	)

	if plan == nil {
		t.Fatal("expected resumed plan graph")
	}
	if plan.PlanID != "plan:goal-3" {
		t.Fatalf("expected plan id reuse, got %#v", plan)
	}
	if plan.CurrentNodeID != "candidate:plan:next_step" {
		t.Fatalf("expected current node reuse, got %#v", plan)
	}
	if len(plan.Nodes) != 2 {
		t.Fatalf("expected existing nodes to be preserved, got %#v", plan.Nodes)
	}
}
