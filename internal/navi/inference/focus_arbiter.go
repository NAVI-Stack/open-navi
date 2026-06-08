package inference

import (
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

const defaultFocusHysteresis = 0.08

// DefaultFocusArbiter performs deterministic focus arbitration for one cycle.
type DefaultFocusArbiter struct {
	Hysteresis float64
}

// Select evaluates the approved focus signal families and returns one focus.
func (a DefaultFocusArbiter) Select(input InferenceInput, now time.Time) FocusSelection {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	stack := input.GoalStack.Normalize()
	candidates := a.buildCandidates(input, stack)
	if len(candidates) == 0 {
		focus := FocusFrame{
			FocusID:     firstNonEmpty(input.Chat.ChatID, "idle"),
			FocusReason: FocusReasonIdle,
			StartedAt:   now,
			Preemptible: true,
		}
		return FocusSelection{Focus: focus}
	}

	selected := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.PriorityScore > selected.PriorityScore {
			selected = candidate
		}
	}

	selection := FocusSelection{
		Focus:      a.toFocusFrame(selected, now),
		Candidates: candidates,
	}
	if previous := input.PreviousFocus; previous != nil && strings.TrimSpace(previous.FocusID) != "" && previous.FocusID != selection.Focus.FocusID {
		if retained, ok := a.shouldRetainPrevious(*previous, candidates); ok {
			selection.Focus = retained
			return selection
		}
		selection.PreemptedFocusID = previous.FocusID
		if !selected.HardPreempt && strings.TrimSpace(previous.ActiveGoalID) != "" {
			selection.ResumeFocusID = previous.ActiveGoalID
		}
	}
	return selection
}

func (a DefaultFocusArbiter) buildCandidates(input InferenceInput, stack GoalStack) []FocusCandidate {
	candidates := make([]FocusCandidate, 0, len(stack.Ready)+len(stack.Latent)+6)
	add := func(candidate FocusCandidate) {
		if strings.TrimSpace(candidate.FocusID) == "" {
			return
		}
		candidate.SupportingFactors = compactStrings(candidate.SupportingFactors)
		candidate.BlockingFactors = compactStrings(candidate.BlockingFactors)
		candidate.PinnedSubreasoners = append([]Subreasoner(nil), candidate.PinnedSubreasoners...)
		candidates = append(candidates, candidate)
	}

	if input.Recovery.Status == schema.RecoveryStatusOpen || strings.TrimSpace(input.Recovery.CurrentTask) != "" || strings.TrimSpace(input.Recovery.FailureReason) != "" {
		add(FocusCandidate{
			FocusID:            firstNonEmpty(runtimeRunID(input.Runtime.Run), checkpointID(input.Runtime.Checkpoint), stack.ActiveGoalID, input.Chat.ChatID, "recovery"),
			ActiveGoalID:       stack.ActiveGoalID,
			FocusReason:        FocusReasonPendingRecovery,
			PriorityScore:      0.97,
			HardPreempt:        true,
			Preemptible:        false,
			SupportingFactors:  []string{"open_recovery_state"},
			BlockingFactors:    compactStrings([]string{strings.TrimSpace(input.Recovery.FailureReason)}),
			PinnedSubreasoners: input.Posture.PinnedSubreasoners,
		})
	}
	if hasRelevantContext(input.RelevantContext, string(FocusReasonUrgentConflict)) {
		add(FocusCandidate{
			FocusID:            firstNonEmpty(stack.ActiveGoalID, input.Chat.ChatID, "urgent_conflict"),
			ActiveGoalID:       stack.ActiveGoalID,
			FocusReason:        FocusReasonUrgentConflict,
			PriorityScore:      0.95,
			HardPreempt:        true,
			Preemptible:        false,
			SupportingFactors:  []string{"urgent_contradiction_detected"},
			PinnedSubreasoners: input.Posture.PinnedSubreasoners,
		})
	}
	if strings.TrimSpace(input.Governance.PendingProposalID) != "" || input.Governance.ConfirmationRequired {
		add(FocusCandidate{
			FocusID:            firstNonEmpty(strings.TrimSpace(input.Governance.PendingProposalID), stack.ActiveGoalID, input.Chat.ChatID, "proposal"),
			ActiveGoalID:       stack.ActiveGoalID,
			FocusReason:        FocusReasonPendingProposal,
			PriorityScore:      0.90,
			HardPreempt:        false,
			Preemptible:        false,
			SupportingFactors:  []string{"approval_boundary_present"},
			BlockingFactors:    compactStrings([]string{strings.TrimSpace(input.Governance.BlockingReason)}),
			PinnedSubreasoners: input.Posture.PinnedSubreasoners,
		})
	}
	if strings.TrimSpace(input.Chat.UserMessage) != "" {
		score := 0.82
		if stack.ActiveGoalID != "" {
			score += 0.04
		}
		add(FocusCandidate{
			FocusID:            firstNonEmpty(input.Chat.ChatID, stack.ActiveGoalID, "user"),
			ActiveGoalID:       stack.ActiveGoalID,
			FocusReason:        FocusReasonUserRequest,
			PriorityScore:      score,
			HardPreempt:        false,
			Preemptible:        true,
			SupportingFactors:  []string{"user_message_present"},
			PinnedSubreasoners: input.Posture.PinnedSubreasoners,
		})
	}
	for idx, goal := range stack.Ready {
		score := 0.32 + clamp(goal.Priority, 0, 1)*0.35
		if goal.GoalID == stack.ActiveGoalID {
			score += 0.08
		}
		if idx == 0 {
			score += 0.02
		}
		add(FocusCandidate{
			FocusID:            goal.GoalID,
			ActiveGoalID:       goal.GoalID,
			FocusReason:        FocusReasonActiveGoal,
			PriorityScore:      score,
			HardPreempt:        false,
			Preemptible:        goal.Preemptible,
			SupportingFactors:  compactStrings([]string{"goal_ready", strings.TrimSpace(goal.Summary)}),
			PinnedSubreasoners: input.Posture.PinnedSubreasoners,
		})
	}
	for _, goal := range stack.Latent {
		score := 0.15 + clamp(goal.Priority, 0, 1)*0.15
		add(FocusCandidate{
			FocusID:            goal.GoalID,
			ActiveGoalID:       goal.GoalID,
			FocusReason:        FocusReasonActiveGoal,
			PriorityScore:      score,
			HardPreempt:        false,
			Preemptible:        true,
			SupportingFactors:  compactStrings([]string{"goal_latent", strings.TrimSpace(goal.Summary)}),
			PinnedSubreasoners: input.Posture.PinnedSubreasoners,
		})
	}
	if hasRelevantContext(input.RelevantContext, string(FocusReasonScheduledTrigger)) {
		add(FocusCandidate{
			FocusID:            firstNonEmpty(stack.ActiveGoalID, input.Chat.ChatID, "scheduled"),
			ActiveGoalID:       stack.ActiveGoalID,
			FocusReason:        FocusReasonScheduledTrigger,
			PriorityScore:      0.56 + clamp(input.Posture.MonitoringAggressiveness, 0, 1)*0.08,
			HardPreempt:        false,
			Preemptible:        true,
			SupportingFactors:  []string{"scheduled_trigger"},
			PinnedSubreasoners: input.Posture.PinnedSubreasoners,
		})
	}
	if hasRelevantContext(input.RelevantContext, string(FocusReasonReflectionFollowup)) {
		add(FocusCandidate{
			FocusID:            firstNonEmpty(stack.ActiveGoalID, input.Chat.ChatID, "reflection"),
			ActiveGoalID:       stack.ActiveGoalID,
			FocusReason:        FocusReasonReflectionFollowup,
			PriorityScore:      0.48 + clamp(input.Posture.InitiativeBias, 0, 1)*0.05,
			HardPreempt:        false,
			Preemptible:        true,
			SupportingFactors:  []string{"reflection_followup"},
			PinnedSubreasoners: input.Posture.PinnedSubreasoners,
		})
	}

	if previous := input.PreviousFocus; previous != nil {
		for i := range candidates {
			if candidates[i].FocusID == strings.TrimSpace(previous.FocusID) {
				candidates[i].PriorityScore += 0.08
				candidates[i].SupportingFactors = compactStrings(append(candidates[i].SupportingFactors, "continuity_bonus"))
			}
		}
	}

	return candidates
}

func (a DefaultFocusArbiter) shouldRetainPrevious(previous FocusFrame, candidates []FocusCandidate) (FocusFrame, bool) {
	hysteresis := a.Hysteresis
	if hysteresis <= 0 {
		hysteresis = defaultFocusHysteresis
	}
	var previousCandidate *FocusCandidate
	var bestCandidate *FocusCandidate
	for i := range candidates {
		candidate := &candidates[i]
		if bestCandidate == nil || candidate.PriorityScore > bestCandidate.PriorityScore {
			bestCandidate = candidate
		}
		if candidate.FocusID == strings.TrimSpace(previous.FocusID) {
			previousCandidate = candidate
		}
	}
	if previousCandidate == nil || bestCandidate == nil || bestCandidate.FocusID == previousCandidate.FocusID {
		return FocusFrame{}, false
	}
	if bestCandidate.HardPreempt {
		return FocusFrame{}, false
	}
	if !previous.Preemptible {
		return previous, true
	}
	if previousCandidate.PriorityScore+hysteresis >= bestCandidate.PriorityScore {
		return previous, true
	}
	return FocusFrame{}, false
}

func (a DefaultFocusArbiter) toFocusFrame(candidate FocusCandidate, now time.Time) FocusFrame {
	return FocusFrame{
		FocusID:            candidate.FocusID,
		ActiveGoalID:       candidate.ActiveGoalID,
		FocusReason:        candidate.FocusReason,
		PriorityScore:      candidate.PriorityScore,
		StartedAt:          now,
		Preemptible:        candidate.Preemptible,
		PinnedSubreasoners: append([]Subreasoner(nil), candidate.PinnedSubreasoners...),
	}
}

func hasRelevantContext(items []ContextReference, kind string) bool {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Kind), kind) {
			return true
		}
	}
	return false
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
