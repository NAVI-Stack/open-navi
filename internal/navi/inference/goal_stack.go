package inference

import "strings"

// Normalize returns a canonical goal stack with de-duplicated membership and
// status values aligned to the containing buckets.
func (g GoalStack) Normalize() GoalStack {
	var out GoalStack
	seen := make(map[string]struct{})

	appendGoals := func(dst *[]GoalRef, goals []GoalRef, status GoalStatus) {
		for _, goal := range goals {
			id := strings.TrimSpace(goal.GoalID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			goal.GoalID = id
			goal.Status = status
			*dst = append(*dst, goal)
		}
	}

	appendGoals(&out.Ready, g.Ready, GoalStatusReady)
	appendGoals(&out.Latent, g.Latent, GoalStatusLatent)
	appendGoals(&out.Suspended, g.Suspended, GoalStatusSuspended)
	appendGoals(&out.Retired, g.Retired, GoalStatusRetired)

	if activeID := strings.TrimSpace(g.ActiveGoalID); activeID != "" {
		if goal, ok := out.Lookup(activeID); ok {
			out.ActiveGoalID = activeID
			out = out.remove(activeID)
			goal.Status = GoalStatusActive
			out.Ready = append([]GoalRef{goal}, out.Ready...)
		}
	}
	return out
}

// Promote moves a goal into the active slot while preserving the old active
// goal as a ready goal.
func (g GoalStack) Promote(goalID string) (GoalStack, GoalTransition, bool) {
	g = g.Normalize()
	goal, ok := g.Lookup(goalID)
	if !ok {
		return g, GoalTransition{}, false
	}
	from := goal.Status
	if strings.TrimSpace(goal.GoalID) == strings.TrimSpace(g.ActiveGoalID) {
		return g, GoalTransition{Operation: "promote", GoalID: goal.GoalID, From: GoalStatusActive, To: GoalStatusActive}, true
	}

	if current, ok := g.ActiveGoal(); ok && current.GoalID != "" && current.GoalID != goal.GoalID {
		g = g.remove(current.GoalID)
		current.Status = GoalStatusReady
		g.Ready = append(g.Ready, current)
	}
	g = g.remove(goal.GoalID)
	goal.Status = GoalStatusActive
	g.ActiveGoalID = goal.GoalID
	g.Ready = append([]GoalRef{goal}, g.Ready...)
	return g, GoalTransition{Operation: "promote", GoalID: goal.GoalID, From: from, To: GoalStatusActive}, true
}

// Demote moves a goal from active/ready to the ready or latent queue.
func (g GoalStack) Demote(goalID string) (GoalStack, GoalTransition, bool) {
	g = g.Normalize()
	goal, ok := g.Lookup(goalID)
	if !ok {
		return g, GoalTransition{}, false
	}
	from := goal.Status
	g = g.remove(goal.GoalID)
	goal.Status = GoalStatusReady
	g.Ready = append(g.Ready, goal)
	if strings.TrimSpace(g.ActiveGoalID) == goal.GoalID {
		g.ActiveGoalID = ""
	}
	return g, GoalTransition{Operation: "demote", GoalID: goal.GoalID, From: from, To: GoalStatusReady}, true
}

// Suspend moves a goal out of the ready path without discarding it.
func (g GoalStack) Suspend(goalID string) (GoalStack, GoalTransition, bool) {
	g = g.Normalize()
	goal, ok := g.Lookup(goalID)
	if !ok {
		return g, GoalTransition{}, false
	}
	from := goal.Status
	g = g.remove(goal.GoalID)
	goal.Status = GoalStatusSuspended
	g.Suspended = append(g.Suspended, goal)
	if strings.TrimSpace(g.ActiveGoalID) == goal.GoalID {
		g.ActiveGoalID = ""
	}
	return g, GoalTransition{Operation: "suspend", GoalID: goal.GoalID, From: from, To: GoalStatusSuspended}, true
}

// Resume moves a suspended or latent goal back into the ready queue.
func (g GoalStack) Resume(goalID string) (GoalStack, GoalTransition, bool) {
	g = g.Normalize()
	goal, ok := g.Lookup(goalID)
	if !ok {
		return g, GoalTransition{}, false
	}
	from := goal.Status
	g = g.remove(goal.GoalID)
	goal.Status = GoalStatusReady
	g.Ready = append([]GoalRef{goal}, g.Ready...)
	return g, GoalTransition{Operation: "resume", GoalID: goal.GoalID, From: from, To: GoalStatusReady}, true
}

// Retire marks a goal complete/inactive while keeping a record in the stack.
func (g GoalStack) Retire(goalID string) (GoalStack, GoalTransition, bool) {
	g = g.Normalize()
	goal, ok := g.Lookup(goalID)
	if !ok {
		return g, GoalTransition{}, false
	}
	from := goal.Status
	g = g.remove(goal.GoalID)
	goal.Status = GoalStatusRetired
	g.Retired = append(g.Retired, goal)
	if strings.TrimSpace(g.ActiveGoalID) == goal.GoalID {
		g.ActiveGoalID = ""
	}
	return g, GoalTransition{Operation: "retire", GoalID: goal.GoalID, From: from, To: GoalStatusRetired}, true
}

// Abandon marks a goal as dropped while preserving that state explicitly.
func (g GoalStack) Abandon(goalID string) (GoalStack, GoalTransition, bool) {
	g = g.Normalize()
	goal, ok := g.Lookup(goalID)
	if !ok {
		return g, GoalTransition{}, false
	}
	from := goal.Status
	g = g.remove(goal.GoalID)
	goal.Status = GoalStatusAbandoned
	g.Retired = append(g.Retired, goal)
	if strings.TrimSpace(g.ActiveGoalID) == goal.GoalID {
		g.ActiveGoalID = ""
	}
	return g, GoalTransition{Operation: "abandon", GoalID: goal.GoalID, From: from, To: GoalStatusAbandoned}, true
}

// Merge combines two existing goals into one explicit replacement goal while
// retiring the original goal records.
func (g GoalStack) Merge(primaryGoalID, secondaryGoalID string, merged GoalRef) (GoalStack, []GoalTransition, bool) {
	g = g.Normalize()
	primary, ok := g.Lookup(primaryGoalID)
	if !ok {
		return g, nil, false
	}
	secondary, ok := g.Lookup(secondaryGoalID)
	if !ok {
		return g, nil, false
	}
	merged.GoalID = strings.TrimSpace(merged.GoalID)
	if merged.GoalID == "" {
		return g, nil, false
	}
	if merged.GoalID == strings.TrimSpace(primary.GoalID) || merged.GoalID == strings.TrimSpace(secondary.GoalID) {
		return g, nil, false
	}
	primaryFrom := primary.Status
	secondaryFrom := secondary.Status

	g = g.remove(primary.GoalID)
	g = g.remove(secondary.GoalID)
	primary.Status = GoalStatusRetired
	secondary.Status = GoalStatusRetired
	g.Retired = append(g.Retired, primary, secondary)

	merged.Status = GoalStatusReady
	g.Ready = append([]GoalRef{merged}, g.Ready...)
	if strings.TrimSpace(g.ActiveGoalID) == primary.GoalID || strings.TrimSpace(g.ActiveGoalID) == secondary.GoalID {
		g.ActiveGoalID = merged.GoalID
		merged.Status = GoalStatusActive
		g.Ready[0] = merged
	}

	return g, []GoalTransition{
		{Operation: "merge", GoalID: primary.GoalID, From: primaryFrom, To: GoalStatusRetired},
		{Operation: "merge", GoalID: secondary.GoalID, From: secondaryFrom, To: GoalStatusRetired},
		{Operation: "merge", GoalID: merged.GoalID, From: "", To: merged.Status},
	}, true
}

// Split replaces one goal with explicitly tracked successor goals.
func (g GoalStack) Split(goalID string, readyGoals, latentGoals []GoalRef) (GoalStack, []GoalTransition, bool) {
	g = g.Normalize()
	goal, ok := g.Lookup(goalID)
	if !ok {
		return g, nil, false
	}
	validSuccessor := false
	for _, next := range append(append([]GoalRef(nil), readyGoals...), latentGoals...) {
		if strings.TrimSpace(next.GoalID) != "" {
			validSuccessor = true
			break
		}
	}
	if !validSuccessor {
		return g, nil, false
	}
	transitions := make([]GoalTransition, 0, 1+len(readyGoals)+len(latentGoals))
	g = g.remove(goal.GoalID)
	transitions = append(transitions, GoalTransition{Operation: "split", GoalID: goal.GoalID, From: goal.Status, To: GoalStatusRetired})
	goal.Status = GoalStatusRetired
	g.Retired = append(g.Retired, goal)
	if strings.TrimSpace(g.ActiveGoalID) == goal.GoalID {
		g.ActiveGoalID = ""
	}

	appendSplitGoals := func(dst *[]GoalRef, goals []GoalRef, status GoalStatus) {
		for _, next := range goals {
			next.GoalID = strings.TrimSpace(next.GoalID)
			if next.GoalID == "" {
				continue
			}
			next.Status = status
			*dst = append(*dst, next)
			transitions = append(transitions, GoalTransition{Operation: "split", GoalID: next.GoalID, From: "", To: status})
		}
	}

	appendSplitGoals(&g.Ready, readyGoals, GoalStatusReady)
	appendSplitGoals(&g.Latent, latentGoals, GoalStatusLatent)
	if g.ActiveGoalID == "" && len(readyGoals) > 0 {
		g.ActiveGoalID = strings.TrimSpace(readyGoals[0].GoalID)
		if g.ActiveGoalID != "" {
			for i := range g.Ready {
				if strings.TrimSpace(g.Ready[i].GoalID) == g.ActiveGoalID {
					g.Ready[i].Status = GoalStatusActive
					break
				}
			}
		}
	}
	return g.Normalize(), transitions, len(transitions) > 1
}

func (g GoalStack) remove(goalID string) GoalStack {
	goalID = strings.TrimSpace(goalID)
	filter := func(src []GoalRef) []GoalRef {
		if len(src) == 0 {
			return nil
		}
		out := make([]GoalRef, 0, len(src))
		for _, goal := range src {
			if strings.TrimSpace(goal.GoalID) == goalID {
				continue
			}
			out = append(out, goal)
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}

	g.Ready = filter(g.Ready)
	g.Latent = filter(g.Latent)
	g.Suspended = filter(g.Suspended)
	g.Retired = filter(g.Retired)
	return g
}
