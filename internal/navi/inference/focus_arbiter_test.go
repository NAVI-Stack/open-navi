package inference

import (
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestFocusArbiterSelectsRecoveryAsHardPreempt(t *testing.T) {
	arbiter := DefaultFocusArbiter{}
	now := time.Date(2026, 4, 8, 18, 30, 0, 0, time.UTC)

	selection := arbiter.Select(InferenceInput{
		Chat: ChatContext{
			ChatID:      "sess-1",
			UserMessage: "keep going",
		},
		GoalStack: GoalStack{
			ActiveGoalID: "goal-1",
			Ready:        []GoalRef{{GoalID: "goal-1", Priority: 0.9, Preemptible: true}},
		},
		Recovery: RecoveryState{
			Status:        schema.RecoveryStatusOpen,
			FailureReason: "tool timed out",
		},
	}, now)

	if selection.Focus.FocusReason != FocusReasonPendingRecovery {
		t.Fatalf("expected recovery focus, got %q", selection.Focus.FocusReason)
	}
	if selection.Focus.Preemptible {
		t.Fatalf("expected recovery focus to be non-preemptible")
	}
	if len(selection.Candidates) == 0 {
		t.Fatalf("expected explicit arbitration candidates")
	}
}

func TestFocusArbiterAppliesAntiThrashHysteresis(t *testing.T) {
	arbiter := DefaultFocusArbiter{}
	now := time.Date(2026, 4, 8, 18, 35, 0, 0, time.UTC)

	selection := arbiter.Select(InferenceInput{
		Chat: ChatContext{
			ChatID:      "sess-2",
			UserMessage: "quick update",
		},
		GoalStack: GoalStack{
			ActiveGoalID: "goal-1",
			Ready: []GoalRef{
				{GoalID: "goal-1", Priority: 1.0, Preemptible: true},
			},
		},
		PreviousFocus: &FocusFrame{
			FocusID:      "goal-1",
			ActiveGoalID: "goal-1",
			FocusReason:  FocusReasonActiveGoal,
			DominantMode: DecisionModePlan,
			StartedAt:    now.Add(-2 * time.Minute),
			Preemptible:  true,
		},
	}, now)

	if selection.Focus.FocusID != "goal-1" {
		t.Fatalf("expected previous focus to be retained, got %#v", selection.Focus)
	}
	if selection.PreemptedFocusID != "" {
		t.Fatalf("expected no preempted focus when hysteresis retains current focus, got %q", selection.PreemptedFocusID)
	}
}
