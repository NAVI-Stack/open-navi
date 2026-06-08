package inference

import "testing"

func TestGoalStackOperationsRemainExplicit(t *testing.T) {
	stack := GoalStack{
		ActiveGoalID: "goal-1",
		Ready: []GoalRef{
			{GoalID: "goal-1", Priority: 0.7},
			{GoalID: "goal-2", Priority: 0.8},
		},
		Latent: []GoalRef{
			{GoalID: "goal-3", Priority: 0.4},
		},
	}

	stack, transition, ok := stack.Promote("goal-2")
	if !ok || transition.To != GoalStatusActive || stack.ActiveGoalID != "goal-2" {
		t.Fatalf("expected goal-2 promote to active, got stack=%#v transition=%#v ok=%t", stack, transition, ok)
	}

	stack, transition, ok = stack.Suspend("goal-2")
	if !ok || transition.To != GoalStatusSuspended || stack.ActiveGoalID != "" {
		t.Fatalf("expected active goal suspension, got stack=%#v transition=%#v ok=%t", stack, transition, ok)
	}

	stack, transition, ok = stack.Resume("goal-2")
	if !ok || transition.To != GoalStatusReady {
		t.Fatalf("expected goal-2 resume into ready queue, got transition=%#v ok=%t", transition, ok)
	}

	stack, transition, ok = stack.Retire("goal-1")
	if !ok || transition.To != GoalStatusRetired {
		t.Fatalf("expected goal-1 retire, got transition=%#v ok=%t", transition, ok)
	}

	stack, transition, ok = stack.Abandon("goal-3")
	if !ok || transition.To != GoalStatusAbandoned {
		t.Fatalf("expected goal-3 abandon, got transition=%#v ok=%t", transition, ok)
	}
}

func TestGoalStackMergeRetiresInputsAndPromotesReplacement(t *testing.T) {
	stack := GoalStack{
		ActiveGoalID: "goal-1",
		Ready: []GoalRef{
			{GoalID: "goal-1", Status: GoalStatusActive, Priority: 0.9},
			{GoalID: "goal-2", Status: GoalStatusReady, Priority: 0.7},
		},
	}

	stack, transitions, ok := stack.Merge("goal-1", "goal-2", GoalRef{
		GoalID:      "goal-12",
		Summary:     "merged goal",
		Priority:    1.0,
		Preemptible: true,
	})
	if !ok {
		t.Fatal("expected merge to succeed")
	}
	if stack.ActiveGoalID != "goal-12" {
		t.Fatalf("expected merged goal to become active, got %q", stack.ActiveGoalID)
	}
	if _, found := stack.Lookup("goal-1"); !found {
		t.Fatal("expected retired source goal to remain addressable")
	}
	if _, found := stack.Lookup("goal-2"); !found {
		t.Fatal("expected retired secondary goal to remain addressable")
	}
	merged, found := stack.Lookup("goal-12")
	if !found || merged.Status != GoalStatusActive {
		t.Fatalf("expected active merged goal, got %#v found=%t", merged, found)
	}
	if len(transitions) != 3 {
		t.Fatalf("expected explicit merge transitions, got %#v", transitions)
	}
}

func TestGoalStackSplitRetiresOriginalAndCreatesExplicitSuccessors(t *testing.T) {
	stack := GoalStack{
		ActiveGoalID: "goal-1",
		Ready: []GoalRef{
			{GoalID: "goal-1", Status: GoalStatusActive, Priority: 0.9},
		},
	}

	stack, transitions, ok := stack.Split(
		"goal-1",
		[]GoalRef{{GoalID: "goal-1a", Summary: "ready child"}},
		[]GoalRef{{GoalID: "goal-1b", Summary: "latent child"}},
	)
	if !ok {
		t.Fatal("expected split to succeed")
	}
	if stack.ActiveGoalID != "goal-1a" {
		t.Fatalf("expected first ready split goal to become active, got %q", stack.ActiveGoalID)
	}
	child, found := stack.Lookup("goal-1a")
	if !found || child.Status != GoalStatusActive {
		t.Fatalf("expected active split child, got %#v found=%t", child, found)
	}
	latent, found := stack.Lookup("goal-1b")
	if !found || latent.Status != GoalStatusLatent {
		t.Fatalf("expected latent split child, got %#v found=%t", latent, found)
	}
	original, found := stack.Lookup("goal-1")
	if !found || original.Status != GoalStatusRetired {
		t.Fatalf("expected original goal retired after split, got %#v found=%t", original, found)
	}
	if len(transitions) != 3 {
		t.Fatalf("expected explicit split transitions, got %#v", transitions)
	}
}
