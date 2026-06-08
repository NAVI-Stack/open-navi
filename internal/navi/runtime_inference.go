package navi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/navi/inference"
	"github.com/ceoai/navi/internal/navi/orchestration"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
	navitool "github.com/ceoai/navi/internal/tool"
)

func (l *AgentLoop) decideRunInference(
	ctx context.Context,
	input naviruntime.ExecuteInput,
	session *ChatRuntimeView,
	req orchestration.CanonicalRunRequest,
	compiledReq orchestration.CompiledModelRequest,
	surfaceResult orchestration.SurfaceResolutionResult,
) (inference.DecisionEnvelope, error) {
	controller := l.inferenceController()
	prior, err := l.loadPersistedInferenceEnvelope(ctx, input.Run, input.Checkpoint)
	if err != nil {
		return inference.DecisionEnvelope{}, err
	}
	inferenceInput := l.buildRunInferenceInput(input, session, req, compiledReq, surfaceResult, prior)
	synthesis, err := controller.Decide(ctx, inferenceInput)
	if err != nil {
		return inference.DecisionEnvelope{}, err
	}
	envelope, err := controller.ValidateGovernedDecision(ctx, inferenceInput, synthesis)
	if err != nil {
		return inference.DecisionEnvelope{}, err
	}
	if err := l.applyInferenceEnvelope(ctx, input.Run, &req, envelope); err != nil {
		return inference.DecisionEnvelope{}, err
	}
	return envelope, nil
}

func (l *AgentLoop) buildRunInferenceInput(input naviruntime.ExecuteInput, session *ChatRuntimeView, req orchestration.CanonicalRunRequest, compiledReq orchestration.CompiledModelRequest, surfaceResult orchestration.SurfaceResolutionResult, prior *inference.DecisionEnvelope) inference.InferenceInput {
	previous := previousFocusFrame(prior)
	planState := planStateFromEnvelope(prior)
	lastResult := lastResultFromEnvelope(prior)
	goalStack := buildRuntimeGoalStack(input, req, previous, planState, lastResult, prior)
	activeGoalID := firstNonEmpty(goalStack.ActiveGoalID, planGraphGoalID(planState), previousGoalID(previous))
	priorGovernance := governanceStateFromEnvelope(prior)

	governance := inference.GovernanceState{
		PendingProposalID:     firstNonEmpty(input.Run.BlockedOnProposalID, pendingProposalID(input.Checkpoint)),
		PendingProposalStatus: pendingProposalStatus(input),
		ConfirmationRequired:  strings.TrimSpace(firstNonEmpty(input.Run.BlockedOnProposalID, pendingProposalID(input.Checkpoint))) != "",
		HardBlocked:           priorGovernance.HardBlocked,
		BlockingReason:        firstNonEmpty(strings.TrimSpace(input.Run.PauseReason), pendingProposalReason(input.Checkpoint), strings.TrimSpace(priorGovernance.BlockingReason)),
		LastApprovalOutcome:   approvalOutcomeForResume(input.Resume),
		LastValidation:        cloneValidationResult(priorGovernance.LastValidation),
		ConstraintMetadata:    governanceConstraintMetadata(prior),
	}
	if governance.PendingProposalID != "" {
		governance.ConfirmationRequired = true
	}
	if governance.LastValidation != nil && governance.LastValidation.Outcome == governor.ValidationRejected {
		governance.HardBlocked = true
	}

	recovery := inference.RecoveryState{
		CurrentTask:     activeGoalID,
		CurrentStep:     firstNonEmpty(planCurrentNode(planState), string(firstNonEmptyRunPhase(input.Run, input.Checkpoint))),
		RollbackPoint:   firstNonEmpty(recoveryCheckpointRef(prior), checkpointRefID(input.Checkpoint), input.Run.LatestCheckpointID),
		PendingBlockers: append([]string(nil), planCurrentNodeBlockers(planState)...),
	}
	if input.Run.Status == schema.RunStatusWaitingForRecovery || strings.TrimSpace(input.Run.InterruptReason) != "" {
		recovery.Status = schema.RecoveryStatusOpen
		recovery.FailureReason = strings.TrimSpace(input.Run.InterruptReason)
	}
	if lastResult != nil {
		recovery.FailureClass = lastResult.FailureClass
		recovery.FailureReason = firstNonEmpty(strings.TrimSpace(recovery.FailureReason), strings.TrimSpace(lastResult.Summary))
		switch lastResult.Outcome {
		case schema.ExecutionOutcomeRejectedPreExecution, schema.ExecutionOutcomeFailed, schema.ExecutionOutcomePartiallySucceeded, schema.ExecutionOutcomeTimedOut, schema.ExecutionOutcomeCancelled:
			recovery.Status = schema.RecoveryStatusOpen
		}
	}
	// Map interrupt class to failure class when no execution result is available yet.
	if recovery.FailureClass == "" && input.Run.InterruptClass != "" {
		switch input.Run.InterruptClass {
		case naviruntime.InterruptClassGovernance:
			recovery.FailureClass = schema.FailureClassPolicyBlocked
		case naviruntime.InterruptClassUserCancel:
			recovery.FailureClass = schema.FailureClassPermissionDenial
		case naviruntime.InterruptClassSystem:
			recovery.FailureClass = schema.FailureClassExecutionFailure
		}
	}
	if recovery.FailureClass == "" && governance.LastValidation != nil && governance.LastValidation.Outcome == governor.ValidationRejected {
		recovery.FailureClass = schema.FailureClassPolicyBlocked
		recovery.FailureReason = firstNonEmpty(strings.TrimSpace(recovery.FailureReason), strings.TrimSpace(governance.BlockingReason), governanceValidationReason(governance.LastValidation))
		recovery.ResumeConditions = compactRuntimeStrings(append(recovery.ResumeConditions, "governance constraints must change before execution can resume"))
	}
	if governance.PendingProposalID != "" {
		recovery.PendingBlockers = compactRuntimeStrings(append(recovery.PendingBlockers, "approval boundary remains open"))
		recovery.ResumeConditions = compactRuntimeStrings(append(recovery.ResumeConditions, "proposal resolution required"))
	}
	// Add phase- and checkpoint-based resume conditions when recovery is open.
	if recovery.Status == schema.RecoveryStatusOpen {
		if input.Run.Status == schema.RunStatusWaitingForRecovery {
			recovery.ResumeConditions = compactRuntimeStrings(append(recovery.ResumeConditions,
				"recovery state must clear before resuming"))
		}
		if input.Checkpoint != nil {
			recovery.ResumeConditions = compactRuntimeStrings(append(recovery.ResumeConditions,
				"checkpoint must be resolved"))
		}
		if reason := strings.TrimSpace(input.Run.InterruptReason); reason != "" {
			recovery.ResumeConditions = compactRuntimeStrings(append(recovery.ResumeConditions,
				"interrupt condition must be cleared: "+reason))
		}
	}

	extensions := map[string]any{}
	if surfaceResult.IsGuarded() {
		extensions["surface_guard"] = string(surfaceResult.Guard)
		extensions["surface_guard_reason"] = strings.TrimSpace(surfaceResult.GuardReason)
	}
	if surfaceResult.ActiveToolSet != nil {
		extensions["active_tool_set_id"] = strings.TrimSpace(surfaceResult.ActiveToolSet.ActiveToolSetID)
		extensions["active_tool_set_scope"] = strings.TrimSpace(string(surfaceResult.ActiveToolSet.Scope))
		extensions["active_tool_set_snapshot_id"] = strings.TrimSpace(surfaceResult.ActiveToolSet.Provenance.RegistrySnapshotID)
	}
	if value := strings.TrimSpace(req.Model.Metadata["routing_guard_reason"]); value != "" {
		extensions["routing_guard_reason"] = value
	}

	return inference.InferenceInput{
		Version:       inference.ContractVersionV1,
		NCOS:          req,
		GoalStack:     goalStack,
		PlanState:     planState,
		PreviousFocus: previous,
		Posture:       postureStateFromEnvelope(prior),
		Recovery:      recovery,
		Governance:    governance,
		Chat: inference.ChatContext{
			ChatID: input.Run.RuntimeSessionID,
		},
		Runtime: inference.RuntimeContext{
			Run:                 input.Run,
			Checkpoint:          input.Checkpoint,
			RunID:               input.Run.RunID,
			RunStatus:           input.Run.Status,
			CurrentPhase:        firstNonEmptyRunPhase(input.Run, input.Checkpoint),
			BlockedOnProposalID: firstNonEmpty(input.Run.BlockedOnProposalID, pendingProposalID(input.Checkpoint)),
			PauseReason:         firstNonEmpty(strings.TrimSpace(input.Run.PauseReason), pendingProposalReason(input.Checkpoint)),
			InterruptClass:      input.Run.InterruptClass,
			InterruptReason:     strings.TrimSpace(input.Run.InterruptReason),
			LatestCheckpointID:  firstNonEmpty(input.Run.LatestCheckpointID, checkpointRefID(input.Checkpoint)),
			Resume:              input.Resume,
			LastResult:          lastResult,
		},
		Capabilities:    l.buildInferenceCapabilities(req),
		RelevantContext: buildInferenceContextRefs(input, req, compiledReq, goalStack, planState, lastResult, prior),
		Extensions:      extensions,
	}
}

func buildRuntimeGoalStack(
	input naviruntime.ExecuteInput,
	req orchestration.CanonicalRunRequest,
	previous *inference.FocusFrame,
	planState *inference.PlanGraph,
	lastResult *inference.ExecutionSnapshot,
	prior *inference.DecisionEnvelope,
) inference.GoalStack {
	stack := goalStackFromEnvelope(prior)
	activeGoalID := firstNonEmpty(
		stack.ActiveGoalID,
		planGraphGoalID(planState),
		previousGoalID(previous),
	)
	if activeGoalID == "" {
		return stack.Normalize()
	}

	goalSummary := firstNonEmpty(
		executionIntentTarget(prior),
		currentPlanNodeIntent(planState),
		summaryForSnapshot(lastResult),
		strings.TrimSpace(req.UserMessage),
		inboxContent(input.InboxItem),
	)
	if _, ok := stack.Lookup(activeGoalID); !ok {
		stack.Ready = append([]inference.GoalRef{{
			GoalID:      activeGoalID,
			Summary:     goalSummary,
			Status:      inference.GoalStatusActive,
			Priority:    previousPriority(previous),
			Preemptible: previousPreemptible(previous),
		}}, stack.Ready...)
	}
	stack.ActiveGoalID = activeGoalID

	if planGoalID := planGraphGoalID(planState); planGoalID != "" && planGoalID != activeGoalID {
		if _, ok := stack.Lookup(planGoalID); !ok {
			stack.Latent = append(stack.Latent, inference.GoalRef{
				GoalID:      planGoalID,
				Summary:     firstNonEmpty(goalSummary, "persisted plan goal"),
				Status:      inference.GoalStatusLatent,
				Priority:    0.5,
				Preemptible: true,
			})
		}
	}

	return stack.Normalize()
}

func (l *AgentLoop) applyInferenceEnvelope(ctx context.Context, run *naviruntime.RunState, req *orchestration.CanonicalRunRequest, envelope inference.DecisionEnvelope) error {
	if run == nil {
		return nil
	}
	envelope = attachPersistedGovernanceState(envelope)
	if envelope.LastResult == nil {
		if prior := runtimeInferenceEnvelope(run, nil); prior != nil && prior.LastResult != nil {
			copy := *prior.LastResult
			envelope.LastResult = &copy
		}
	}
	if l.cfg.ICSStateStore == nil {
		return fmt.Errorf("navi: persist ICS state: store not configured")
	}
	state, err := l.cfg.ICSStateStore.SaveICSState(ctx, run.RunID, envelope)
	if err != nil {
		return fmt.Errorf("navi: persist ICS state: %w", err)
	}
	if _, err := l.cfg.ICSStateStore.AppendICSHistory(ctx, run.RunID, envelope); err != nil {
		return fmt.Errorf("navi: append ICS history: %w", err)
	}
	run.ICSStateVersion = state.Version
	if payload, err := json.Marshal(state.DecisionEnvelope); err == nil {
		run.ICSDecisionEnvelope = payload
	}
	if req != nil {
		req.Frame.Scratchpad = promptScratchpad(run)
		req.Frame.TraceID = firstNonEmpty(strings.TrimSpace(envelope.Rationale.DecisionTrace.TraceID), req.Frame.TraceID)
	}
	return nil
}

func (l *AgentLoop) emitInferenceDecisionTrace(ctx context.Context, run *naviruntime.RunState, rationale inference.Rationale) {
	if run == nil {
		return
	}
	ev := schema.NewRunEvent(
		schema.FactInferenceDecisionTrace,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		schema.VisibilityOperator,
		schema.InferenceDecisionTracePayload{
			RunID:                  run.RunID,
			RuntimeSessionID:       run.RuntimeSessionID,
			TraceID:                rationale.DecisionTrace.TraceID,
			ContractVersion:        rationale.Version,
			FocusID:                rationale.Focus.FocusID,
			SelectedGoalID:         rationale.SelectedGoalID,
			FocusReason:            string(rationale.Focus.FocusReason),
			DominantMode:           string(rationale.DominantMode),
			SubmodeChain:           decisionModesToStrings(rationale.SubmodeChain),
			InvokedSubreasoners:    subreasonersToStrings(rationale.InvokedSubreasoners),
			PinnedSubreasoners:     subreasonersToStrings(rationale.Posture.PinnedSubreasoners),
			SuppressedSubreasoners: subreasonersToStrings(rationale.Arbitration.SuppressedSubreasoners),
			ArbitrationNotes:       append([]string(nil), rationale.Arbitration.Notes...),
			CandidateIDs:           append([]string(nil), rationale.DecisionTrace.CandidateIDs...),
			CandidateID:            rationale.ChosenAction.CandidateID,
			CandidateType:          string(rationale.ChosenAction.CandidateType),
			CandidateScore:         rationale.ChosenAction.Score,
			ThresholdBand:          string(rationale.Thresholds.ScoreBand),
			GovernanceOutcome:      rationale.DecisionTrace.GovernanceOutcome,
			ExecutionActionType:    rationale.ExecutionIntent.ActionType,
			ExecutionTarget:        rationale.ExecutionIntent.Target,
			ApprovalRequirement:    string(rationale.ExecutionIntent.ApprovalRequirement),
			TraceStage:             firstNonEmpty(strings.TrimSpace(rationale.DecisionTrace.Stage), "decision"),
			TargetCapability:       rationale.Governance.TargetCapability,
			AllowedCapabilities:    append([]string(nil), rationale.Governance.AllowedCapabilities...),
			PlanID:                 planGraphID(rationale.PlanGraph),
			CurrentNodeID:          planGraphCurrentNode(rationale.PlanGraph),
			PlanStatus:             planGraphStatus(rationale.PlanGraph),
			CheckpointRefs:         planGraphCheckpointRefs(rationale.PlanGraph),
			RecoveryRoute:          string(rationale.RecoveryCheckpoint.Route.Action),
			RecoveryCheckpointRef:  rationale.RecoveryCheckpoint.CheckpointRef,
			ExecutionOutcomeRef:    rationale.DecisionTrace.ExecutionOutcomeRef,
			ExecutedCapability:     rationale.DecisionTrace.ExecutedCapability,
			ExecutedCommandType:    rationale.DecisionTrace.ExecutedCommandType,
			ProposalID:             rationale.DecisionTrace.ProposalID,
			RecoveryRef:            rationale.DecisionTrace.RecoveryRef,
			ResumeFocusID:          rationale.DecisionTrace.ResumeFocusID,
			ReflectionTrigger:      rationale.ReflectionHooks.FollowupTrigger,
			ReflectionRef:          rationale.DecisionTrace.ReflectionRef,
			Tags:                   cloneStringMap(rationale.Governance.Tags),
		},
	)
	l.appendRuntimeEvent(ctx, ev)
}

func (l *AgentLoop) emitInferenceRecoveryState(ctx context.Context, run *naviruntime.RunState, rationale inference.Rationale, snapshot *inference.ExecutionSnapshot) {
	if run == nil || snapshot == nil {
		return
	}
	if !rationale.RecoveryCheckpoint.Open &&
		rationale.RecoveryCheckpoint.Route.Action == inference.RecoveryRouteNone &&
		snapshot.Outcome != schema.ExecutionOutcomeFailed &&
		snapshot.Outcome != schema.ExecutionOutcomePartiallySucceeded &&
		snapshot.Outcome != schema.ExecutionOutcomeTimedOut {
		return
	}
	ev := schema.NewRunEvent(
		schema.FactRecoveryRequired,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		l.eventVisibility(ctx, run.ChatID, run.RuntimeSessionID),
		schema.RecoveryRequiredPayload{
			RunID:            run.RunID,
			RuntimeSessionID: run.RuntimeSessionID,
			ProposalID:       firstNonEmpty(rationale.RecoveryCheckpoint.Route.ProposalID, rationale.ReflectionHooks.ProposalCandidate),
			RecoveryRoute:    string(rationale.RecoveryCheckpoint.Route.Action),
			CheckpointRef:    rationale.RecoveryCheckpoint.CheckpointRef,
			CurrentTask:      rationale.RecoveryCheckpoint.CurrentTask,
			CurrentStep:      rationale.RecoveryCheckpoint.CurrentStep,
			PendingBlockers:  append([]string(nil), rationale.RecoveryCheckpoint.PendingBlockers...),
			ResumeConditions: append([]string(nil), rationale.RecoveryCheckpoint.ResumeConditions...),
			RollbackPoint:    rationale.RecoveryCheckpoint.RollbackPoint,
			FailureReason:    firstNonEmpty(rationale.RecoveryCheckpoint.Route.Reason, rationale.ReflectionHooks.ExpectedVsActual, summaryForSnapshot(snapshot)),
		},
	)
	l.appendRuntimeEvent(ctx, ev)
}

func (l *AgentLoop) emitInferenceReflectionHook(ctx context.Context, run *naviruntime.RunState, rationale inference.Rationale, snapshot *inference.ExecutionSnapshot) {
	if run == nil || !shouldEmitReflectionHook(rationale, snapshot) {
		return
	}
	details := schema.InferenceReflectionDetails{
		RuntimeSessionID:      run.RuntimeSessionID,
		RunID:                 run.RunID,
		TraceID:               rationale.DecisionTrace.TraceID,
		GoalID:                rationale.SelectedGoalID,
		CandidateType:         string(rationale.ChosenAction.CandidateType),
		RecoveryRoute:         string(rationale.RecoveryCheckpoint.Route.Action),
		RecoveryCheckpointRef: rationale.RecoveryCheckpoint.CheckpointRef,
		WhatHappened:          rationale.ReflectionHooks.WhatHappened,
		ExpectedVsActual:      rationale.ReflectionHooks.ExpectedVsActual,
		SuccessFailureState:   string(rationale.ReflectionHooks.SuccessFailureState),
		Surprises:             append([]string(nil), rationale.ReflectionHooks.Surprises...),
		LessonCandidate:       rationale.ReflectionHooks.LessonCandidate,
		MemoryCandidate:       rationale.ReflectionHooks.MemoryCandidate,
		ProposalCandidate:     rationale.ReflectionHooks.ProposalCandidate,
		FollowupTrigger:       rationale.ReflectionHooks.FollowupTrigger,
	}
	ev := schema.NewRunEvent(
		schema.FactReflectionQueued,
		schema.EventKindFact,
		run.RuntimeSessionID,
		schema.AgentNavi,
		run.RunID,
		schema.VisibilityOperator,
		schema.ReflectionPayload{
			ID:               firstNonEmpty(rationale.DecisionTrace.TraceID, run.RunID),
			RuntimeSessionID: run.RuntimeSessionID,
			Tier:             schema.ReflectionTierShallow,
			Summary:          firstNonEmpty(rationale.ReflectionHooks.WhatHappened, summaryForSnapshot(snapshot)),
			Details:          marshalInferenceReflectionDetails(details),
			EscalationReason: "automatic",
			CreatedAt:        time.Now().UTC(),
		},
	)
	l.appendRuntimeEvent(ctx, ev)
}

func (l *AgentLoop) buildInferenceCapabilities(req orchestration.CanonicalRunRequest) []inference.CapabilityAvailability {
	if !req.RequiredOutput.AllowToolCalls || len(req.CapabilitySurface.ToolNames) == 0 {
		return []inference.CapabilityAvailability{{
			Name:        "assistant_reply",
			Kind:        "reply",
			Available:   true,
			Governed:    true,
			CommandType: schema.CommandTypeCompose,
		}}
	}

	var reg *navitool.Registry
	if l != nil {
		reg, _ = l.ensureToolRegistry()
	}
	out := make([]inference.CapabilityAvailability, 0, len(req.CapabilitySurface.ToolNames))
	for _, name := range req.CapabilitySurface.ToolNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		capability := inference.CapabilityAvailability{
			Name:        name,
			Kind:        firstNonEmpty(req.CapabilitySurface.Surface, "runtime_surface"),
			SourceType:  "runtime_surface",
			Available:   true,
			Governed:    true,
			CommandType: schema.CommandTypeInvoke,
		}
		if reg != nil {
			if toolEntry, ok := reg.Lookup(name); ok && toolEntry != nil {
				if toolEntry.Executor == nil {
					continue
				}
				if contract, ok := buildRuntimeToolContract(toolEntry); ok {
					capability.SourceType = firstNonEmpty(contract.SourceType, capability.SourceType)
					capability.Domain = contract.Domain
					capability.CommandType = firstNonEmptyCommandType(contract.CommandType, capability.CommandType)
					capability.WorkspaceAction = contract.WorkspaceAction
					capability.TargetPathArg = contract.TargetPathArg
					capability.WorkspaceScopedPath = contract.WorkspaceScopedPath
					capability.RequiresConfirmation = contract.RequiresConfirmation
					capability.RiskHint = contract.RiskLevel
					capability.Reversibility = contract.Reversibility
					capability.ExpectedSideEffects = append([]string(nil), contract.ExpectedSideEffects...)
					capability.SkillName = contract.SkillName
					capability.PluginName = contract.PluginName
					capability.ConnectorID = contract.ConnectorID
				}
			}
		}
		out = append(out, capability)
	}
	if len(out) == 0 {
		return []inference.CapabilityAvailability{{
			Name:        "assistant_reply",
			Kind:        "reply",
			Available:   true,
			Governed:    true,
			CommandType: schema.CommandTypeCompose,
		}}
	}
	return out
}

func compiledToolNames(compiledReq orchestration.CompiledModelRequest) []string {
	if len(compiledReq.Tools) == 0 {
		return nil
	}
	out := make([]string, 0, len(compiledReq.Tools))
	for _, tool := range compiledReq.Tools {
		if name := strings.TrimSpace(tool.Name); name != "" {
			out = append(out, name)
		}
	}
	return compactRuntimeStrings(out)
}

func buildInferenceContextRefs(input naviruntime.ExecuteInput, req orchestration.CanonicalRunRequest, compiledReq orchestration.CompiledModelRequest, goalStack inference.GoalStack, planState *inference.PlanGraph, lastResult *inference.ExecutionSnapshot, prior *inference.DecisionEnvelope) []inference.ContextReference {
	out := make([]inference.ContextReference, 0, 14)
	if strings.TrimSpace(req.Frame.TraceID) != "" {
		out = append(out, inference.ContextReference{
			Kind:    "trace",
			ID:      strings.TrimSpace(req.Frame.TraceID),
			Summary: "ncos execution trace",
			Source:  "ncos",
		})
	}
	if strings.TrimSpace(req.Frame.CheckpointID) != "" {
		out = append(out, inference.ContextReference{
			Kind:    "checkpoint",
			ID:      strings.TrimSpace(req.Frame.CheckpointID),
			Summary: "runtime checkpoint",
			Source:  "runtime",
		})
	}
	if goalID := strings.TrimSpace(goalStack.ActiveGoalID); goalID != "" {
		out = append(out, inference.ContextReference{
			Kind:    "goal",
			ID:      goalID,
			Summary: "active goal stack entry",
			Source:  "ics",
		})
	}
	if planState != nil {
		out = append(out, inference.ContextReference{
			Kind:    "plan",
			ID:      strings.TrimSpace(planState.PlanID),
			Summary: firstNonEmpty(planCurrentNode(planState), "active plan graph"),
			Source:  "ics",
		})
	}
	if proposalID := strings.TrimSpace(firstNonEmpty(input.Run.BlockedOnProposalID, pendingProposalID(input.Checkpoint))); proposalID != "" {
		out = append(out, inference.ContextReference{
			Kind:    "proposal",
			ID:      proposalID,
			Summary: firstNonEmpty(strings.TrimSpace(input.Run.PauseReason), pendingProposalReason(input.Checkpoint), "pending approval boundary"),
			Source:  "runtime",
		})
	}
	if input.Resume != nil {
		out = append(out, inference.ContextReference{
			Kind:    "resume",
			ID:      strings.TrimSpace(input.Resume.ProposalID),
			Summary: firstNonEmpty(strings.TrimSpace(string(input.Resume.Resolution)), "runtime resume signal"),
			Source:  "runtime",
		})
	}
	if route := strings.TrimSpace(recoveryRouteAction(prior)); route != "" {
		out = append(out, inference.ContextReference{
			Kind:    "recovery",
			ID:      route,
			Summary: firstNonEmpty(strings.TrimSpace(input.Run.InterruptReason), "open recovery route"),
			Source:  "ics",
		})
	}
	if lastResult != nil {
		out = append(out, inference.ContextReference{
			Kind:    "last_result",
			ID:      firstNonEmpty(strings.TrimSpace(lastResult.ExecutedCapability), strings.TrimSpace(lastResult.ProposalID), string(lastResult.Outcome)),
			Summary: firstNonEmpty(strings.TrimSpace(lastResult.Summary), string(lastResult.Outcome)),
			Source:  "runtime",
		})
	}
	if len(compiledReq.Tools) > 0 {
		out = append(out, inference.ContextReference{
			Kind:    "capability_surface",
			ID:      strings.Join(compiledToolNames(compiledReq), ","),
			Summary: firstNonEmpty(req.CapabilitySurface.SelectionReason, "full surfaced runtime capability set"),
			Source:  "runtime",
		})
	}
	if surfaceResult := req.Model.Metadata["active_tool_set_id"]; strings.TrimSpace(surfaceResult) != "" {
		out = append(out, inference.ContextReference{
			Kind:    "active_tool_set",
			ID:      strings.TrimSpace(surfaceResult),
			Summary: firstNonEmpty(strings.TrimSpace(req.Model.Metadata["active_tool_set_scope"]), "provider_call"),
			Source:  "runtime",
		})
	}
	if profile := strings.TrimSpace(compiledReq.Profile.Model); profile != "" {
		out = append(out, inference.ContextReference{
			Kind:    "model",
			ID:      profile,
			Summary: firstNonEmpty(strings.TrimSpace(compiledReq.Profile.Provider), "compiled model profile"),
			Source:  "orchestration",
		})
	}
	if status := strings.TrimSpace(string(input.Run.Status)); status != "" {
		out = append(out, inference.ContextReference{
			Kind:    "run_status",
			ID:      status,
			Summary: firstNonEmpty(strings.TrimSpace(input.Run.PauseReason), strings.TrimSpace(input.Run.InterruptReason), "current runtime run status"),
			Source:  "runtime",
		})
	}
	if interrupt := strings.TrimSpace(string(input.Run.InterruptClass)); interrupt != "" {
		out = append(out, inference.ContextReference{
			Kind:    "interrupt",
			ID:      interrupt,
			Summary: firstNonEmpty(strings.TrimSpace(input.Run.InterruptReason), "runtime interrupt state"),
			Source:  "runtime",
		})
	}
	if reason := strings.TrimSpace(req.CapabilitySurface.SelectionReason); reason != "" {
		out = append(out, inference.ContextReference{
			Kind:    "surface_policy",
			ID:      firstNonEmpty(req.CapabilitySurface.Surface, "runtime"),
			Summary: reason,
			Source:  "orchestration",
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func previousFocusFrame(prior *inference.DecisionEnvelope) *inference.FocusFrame {
	if prior == nil || strings.TrimSpace(prior.Rationale.Focus.FocusID) == "" {
		return nil
	}
	frame := prior.Rationale.Focus
	frame.ActiveGoalID = firstNonEmpty(frame.ActiveGoalID, prior.Rationale.SelectedGoalID)
	if frame.StartedAt.IsZero() {
		frame.StartedAt = time.Now().UTC()
	}
	return &frame
}

func planStateFromEnvelope(prior *inference.DecisionEnvelope) *inference.PlanGraph {
	if prior == nil || prior.Rationale.PlanGraph == nil || strings.TrimSpace(prior.Rationale.PlanGraph.PlanID) == "" {
		return nil
	}
	copy := *prior.Rationale.PlanGraph
	return &copy
}

func goalStackFromEnvelope(prior *inference.DecisionEnvelope) inference.GoalStack {
	if prior == nil {
		return inference.GoalStack{}
	}
	return prior.Rationale.GoalStack.Normalize()
}

func postureStateFromEnvelope(prior *inference.DecisionEnvelope) inference.PostureState {
	if prior == nil {
		return inference.PostureState{}
	}
	return prior.Rationale.Posture
}

func governanceStateFromEnvelope(prior *inference.DecisionEnvelope) inference.GovernanceState {
	if prior == nil {
		return inference.GovernanceState{}
	}
	state := restoreGovernanceStateFromExtensions(prior)
	if state.LastValidation == nil && prior.GovernanceResult != nil {
		copy := *prior.GovernanceResult
		state.LastValidation = &copy
	}
	if strings.TrimSpace(state.BlockingReason) == "" {
		state.BlockingReason = firstNonEmpty(strings.TrimSpace(prior.ReplyMessage), strings.TrimSpace(governanceValidationReason(state.LastValidation)))
	}
	if len(state.ConstraintMetadata) == 0 {
		state.ConstraintMetadata = governanceConstraintMetadata(prior)
	}
	return state
}

func lastResultFromEnvelope(prior *inference.DecisionEnvelope) *inference.ExecutionSnapshot {
	if prior == nil || prior.LastResult == nil {
		return nil
	}
	copy := *prior.LastResult
	return &copy
}

func pendingProposalID(checkpoint *naviruntime.Checkpoint) string {
	if checkpoint == nil {
		return ""
	}
	return strings.TrimSpace(checkpoint.PendingProposalID)
}

func pendingProposalReason(checkpoint *naviruntime.Checkpoint) string {
	if checkpoint == nil {
		return ""
	}
	return strings.TrimSpace(checkpoint.PendingProposalReason)
}

func checkpointRefID(checkpoint *naviruntime.Checkpoint) string {
	if checkpoint == nil {
		return ""
	}
	return strings.TrimSpace(checkpoint.CheckpointID)
}

func pendingProposalStatus(input naviruntime.ExecuteInput) schema.ProposalStatus {
	if input.Resume != nil {
		switch input.Resume.Resolution {
		case naviruntime.ProposalResolutionApprove:
			return schema.ProposalStatusApproved
		case naviruntime.ProposalResolutionDecline:
			return schema.ProposalStatusDeclined
		}
	}
	if strings.TrimSpace(firstNonEmpty(input.Run.BlockedOnProposalID, pendingProposalID(input.Checkpoint))) != "" {
		return schema.ProposalStatusPending
	}
	return ""
}

func approvalOutcomeForResume(resume *naviruntime.ResumeSignal) schema.ApprovalOutcome {
	if resume == nil {
		return ""
	}
	switch resume.Resolution {
	case naviruntime.ProposalResolutionApprove:
		return schema.ApprovalOutcomeAllowOnce
	case naviruntime.ProposalResolutionDecline:
		return schema.ApprovalOutcomeDenied
	default:
		return ""
	}
}

func firstNonEmptyRunPhase(run *naviruntime.RunState, checkpoint *naviruntime.Checkpoint) naviruntime.RunPhase {
	if checkpoint != nil && checkpoint.Phase != "" {
		return checkpoint.Phase
	}
	if run != nil {
		return run.CurrentPhase
	}
	return ""
}

func sessionUserMessage(session *ChatRuntimeView) string {
	if session == nil {
		return ""
	}
	return lastSessionUserContent(session)
}

func inboxContent(item *naviruntime.InboxItem) string {
	if item == nil {
		return ""
	}
	return strings.TrimSpace(item.Content)
}

func inboxSourceChannel(item *naviruntime.InboxItem) string {
	if item == nil {
		return ""
	}
	return strings.TrimSpace(item.SourceChannel)
}

func parseRFC3339(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func stringSliceToDecisionModes(values []string) []inference.DecisionMode {
	out := make([]inference.DecisionMode, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, inference.DecisionMode(value))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func decisionModesToStrings(values []inference.DecisionMode) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(string(value)); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func subreasonersToStrings(values []inference.Subreasoner) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(string(value)); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func previousGoalID(frame *inference.FocusFrame) string {
	if frame == nil {
		return ""
	}
	return strings.TrimSpace(frame.ActiveGoalID)
}

func previousPriority(frame *inference.FocusFrame) float64 {
	if frame == nil || frame.PriorityScore == 0 {
		return 1
	}
	return frame.PriorityScore
}

func previousPreemptible(frame *inference.FocusFrame) bool {
	if frame == nil {
		return true
	}
	return frame.Preemptible
}

func planGraphJSON(plan *inference.PlanGraph) string {
	if plan == nil {
		return ""
	}
	payload, err := json.Marshal(plan)
	if err != nil {
		return ""
	}
	return string(payload)
}

func goalStackJSON(stack inference.GoalStack) string {
	payload, err := json.Marshal(stack.Normalize())
	if err != nil {
		return ""
	}
	return string(payload)
}

func planGraphID(plan *inference.PlanGraph) string {
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(plan.PlanID)
}

func planGraphGoalID(plan *inference.PlanGraph) string {
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(plan.GoalID)
}

func planGraphCurrentNode(plan *inference.PlanGraph) string {
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(plan.CurrentNodeID)
}

func planGraphStatus(plan *inference.PlanGraph) string {
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(string(plan.Status))
}

func planGraphCheckpointRefs(plan *inference.PlanGraph) []string {
	if plan == nil || len(plan.Checkpoints) == 0 {
		return nil
	}
	refs := make([]string, 0, len(plan.Checkpoints))
	for _, checkpoint := range plan.Checkpoints {
		refs = append(refs, firstNonEmpty(checkpoint.CheckpointRef, checkpoint.CheckpointID, checkpoint.NodeID))
	}
	return compactRuntimeStrings(refs)
}

func planCurrentNode(plan *inference.PlanGraph) string {
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(plan.CurrentNodeID)
}

func currentPlanNodeIntent(plan *inference.PlanGraph) string {
	if plan == nil {
		return ""
	}
	currentNodeID := planCurrentNode(plan)
	if currentNodeID == "" {
		return ""
	}
	for _, node := range plan.Nodes {
		if strings.TrimSpace(node.NodeID) == currentNodeID {
			return strings.TrimSpace(node.Intent)
		}
	}
	return ""
}

func planCurrentNodeBlockers(plan *inference.PlanGraph) []string {
	if plan == nil {
		return nil
	}
	currentNodeID := planCurrentNode(plan)
	if currentNodeID == "" {
		return nil
	}
	for _, node := range plan.Nodes {
		if strings.TrimSpace(node.NodeID) == currentNodeID {
			return compactRuntimeStrings(append([]string(nil), node.Blockers...))
		}
	}
	return nil
}

func compactRuntimeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func attachPersistedGovernanceState(envelope inference.DecisionEnvelope) inference.DecisionEnvelope {
	state := inference.GovernanceState{
		HardBlocked:        envelope.GovernanceResult != nil && envelope.GovernanceResult.Outcome == governor.ValidationRejected,
		BlockingReason:     firstNonEmpty(strings.TrimSpace(envelope.ReplyMessage), strings.TrimSpace(governanceValidationReason(envelope.GovernanceResult))),
		LastValidation:     cloneValidationResult(envelope.GovernanceResult),
		ConstraintMetadata: governanceConstraintMetadata(&envelope),
	}
	if len(state.ConstraintMetadata) == 0 {
		state.ConstraintMetadata = nil
	}
	if envelope.Rationale.Extensions == nil {
		envelope.Rationale.Extensions = map[string]any{}
	}
	envelope.Rationale.Extensions["governance_state"] = governanceStateToExtensionMap(state)
	return envelope
}

func restoreGovernanceStateFromExtensions(prior *inference.DecisionEnvelope) inference.GovernanceState {
	if prior == nil || len(prior.Rationale.Extensions) == 0 {
		return inference.GovernanceState{}
	}
	raw, ok := prior.Rationale.Extensions["governance_state"]
	if !ok || raw == nil {
		return inference.GovernanceState{}
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return inference.GovernanceState{}
	}
	var state inference.GovernanceState
	if err := json.Unmarshal(payload, &state); err != nil {
		return inference.GovernanceState{}
	}
	if len(state.ConstraintMetadata) == 0 {
		state.ConstraintMetadata = nil
	}
	return state
}

func governanceStateToExtensionMap(state inference.GovernanceState) map[string]any {
	out := map[string]any{
		"hard_blocked":    state.HardBlocked,
		"blocking_reason": strings.TrimSpace(state.BlockingReason),
	}
	if state.LastValidation != nil {
		out["last_validation"] = map[string]any{
			"outcome":         int(state.LastValidation.Outcome),
			"reason":          strings.TrimSpace(state.LastValidation.Reason),
			"modified_action": strings.TrimSpace(state.LastValidation.ModifiedAction),
			"tier":            string(state.LastValidation.Tier),
		}
	}
	if len(state.ConstraintMetadata) > 0 {
		out["constraint_metadata"] = cloneStringMap(state.ConstraintMetadata)
	}
	return out
}

func governanceConstraintMetadata(prior *inference.DecisionEnvelope) map[string]string {
	if prior == nil {
		return nil
	}
	metadata := map[string]string{}
	if gv := prior.Rationale.Governance; gv.CommandType != "" {
		metadata["command_type"] = string(gv.CommandType)
	}
	if gv := prior.Rationale.Governance; gv.TargetCapability != "" {
		metadata["target_capability"] = strings.TrimSpace(gv.TargetCapability)
	} else if intent := prior.Rationale.ExecutionIntent; intent.TargetCapability != "" {
		metadata["target_capability"] = strings.TrimSpace(intent.TargetCapability)
	}
	if allowed := compactRuntimeStrings(append([]string(nil), prior.Rationale.Governance.AllowedCapabilities...)); len(allowed) > 0 {
		metadata["allowed_capabilities"] = strings.Join(allowed, ",")
	}
	if gv := prior.Rationale.Governance; gv.ApprovalRequirement != "" {
		metadata["approval_requirement"] = string(gv.ApprovalRequirement)
	}
	if prior.GovernanceResult != nil {
		metadata["validation_outcome"] = fmt.Sprintf("%d", prior.GovernanceResult.Outcome)
		if prior.GovernanceResult.Tier != "" {
			metadata["validation_tier"] = string(prior.GovernanceResult.Tier)
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func cloneValidationResult(v *governor.ValidationResult) *governor.ValidationResult {
	if v == nil {
		return nil
	}
	copy := *v
	return &copy
}

func governanceValidationReason(result *governor.ValidationResult) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.Reason)
}

func postureStateJSON(posture inference.PostureState) string {
	payload, err := json.Marshal(posture)
	if err != nil {
		return ""
	}
	return string(payload)
}

func arbitrationStateJSON(state inference.ArbitrationState) string {
	payload, err := json.Marshal(state)
	if err != nil {
		return ""
	}
	return string(payload)
}

func stringMapJSON(value map[string]string) string {
	if len(value) == 0 {
		return ""
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(payload)
}

func stringSliceJSON(values []string) string {
	values = compactRuntimeStrings(values)
	if len(values) == 0 {
		return ""
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(payload)
}

func governanceHandoffJSON(value inference.GovernanceHandoff) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(payload)
}

func executionIntentJSON(value inference.ExecutionIntent) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(payload)
}

func executionSnapshotJSON(snapshot inference.ExecutionSnapshot) string {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	return string(payload)
}

func shouldEmitReflectionHook(rationale inference.Rationale, snapshot *inference.ExecutionSnapshot) bool {
	if snapshot != nil && snapshot.Outcome != "" {
		return true
	}
	if rationale.ReflectionHooks.SuccessFailureState != inference.OutcomeStatePending {
		return true
	}
	return strings.TrimSpace(rationale.ReflectionHooks.FollowupTrigger) != "" ||
		strings.TrimSpace(rationale.ReflectionHooks.LessonCandidate) != "" ||
		strings.TrimSpace(rationale.ReflectionHooks.MemoryCandidate) != ""
}

func summaryForSnapshot(snapshot *inference.ExecutionSnapshot) string {
	if snapshot == nil {
		return ""
	}
	return firstNonEmpty(strings.TrimSpace(snapshot.Summary), string(snapshot.Outcome))
}

func (l *AgentLoop) emitObservedInferenceOutcome(ctx context.Context, run *naviruntime.RunState, envelope inference.DecisionEnvelope, snapshot *inference.ExecutionSnapshot) {
	if run == nil || snapshot == nil {
		return
	}
	l.emitInferenceDecisionTrace(ctx, run, envelope.Rationale)
	l.emitInferenceRecoveryState(ctx, run, envelope.Rationale, snapshot)
	l.emitInferenceReflectionHook(ctx, run, envelope.Rationale, snapshot)
}

func (l *AgentLoop) superviseInferenceOutcome(ctx context.Context, run *naviruntime.RunState, envelope inference.DecisionEnvelope, snapshot *inference.ExecutionSnapshot) (inference.DecisionEnvelope, error) {
	if run == nil || snapshot == nil {
		return envelope, nil
	}
	snapshot = bindExecutionSnapshotToRuntime(run, snapshot)
	snapshot = mergeExecutionSnapshot(lastResultFromEnvelope(runtimeInferenceEnvelope(run, nil)), snapshot)
	controller := l.inferenceController()
	supervisionCtx := context.WithoutCancel(ctx)
	reconciled, err := controller.ObserveOutcome(supervisionCtx, envelope, *snapshot)
	if err != nil {
		return envelope, err
	}
	reconciled.LastResult = snapshot
	if err := l.applyInferenceEnvelope(ctx, run, nil, reconciled); err != nil {
		return envelope, err
	}
	return reconciled, nil
}

func (l *AgentLoop) loadPersistedInferenceEnvelope(ctx context.Context, run *naviruntime.RunState, checkpoint *naviruntime.Checkpoint) (*inference.DecisionEnvelope, error) {
	if l == nil || l.cfg.ICSStateStore == nil {
		return nil, fmt.Errorf("navi: load ICS state: store not configured")
	}
	runID := ""
	if checkpoint != nil {
		runID = strings.TrimSpace(checkpoint.RunID)
	}
	if runID == "" && run != nil {
		runID = strings.TrimSpace(run.RunID)
	}
	if runID == "" {
		return nil, nil
	}
	state, err := l.cfg.ICSStateStore.LoadICSState(ctx, runID)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, nil
	}
	if run != nil {
		run.ICSStateVersion = state.Version
	}
	if payload, err := json.Marshal(state.DecisionEnvelope); err == nil {
		if run != nil {
			run.ICSDecisionEnvelope = payload
		}
		if checkpoint != nil {
			checkpoint.ICSDecisionEnvelope = append([]byte(nil), payload...)
			checkpoint.ICSStateVersion = state.Version
		}
	}
	copy := state.DecisionEnvelope
	return &copy, nil
}

func restorePendingApprovals(run *naviruntime.RunState, checkpoint *naviruntime.Checkpoint) []inference.ApprovalRef {
	proposalID := strings.TrimSpace(firstNonEmpty(run.BlockedOnProposalID, pendingProposalID(checkpoint)))
	if proposalID == "" {
		return nil
	}
	return []inference.ApprovalRef{{
		Requirement: inference.ApprovalRequirementBlockingProposal,
		ProposalID:  proposalID,
		Reason:      firstNonEmpty(strings.TrimSpace(run.PauseReason), pendingProposalReason(checkpoint), "proposal resolution required"),
		Status:      schema.ProposalStatusPending,
	}}
}

func restoreRecoveryResumeConditions(run *naviruntime.RunState, checkpoint *naviruntime.Checkpoint) []string {
	conditions := make([]string, 0, 2)
	if strings.TrimSpace(firstNonEmpty(run.BlockedOnProposalID, pendingProposalID(checkpoint))) != "" {
		conditions = append(conditions, "proposal resolution required")
	}
	if reason := strings.TrimSpace(run.InterruptReason); reason != "" {
		conditions = append(conditions, reason)
	}
	return compactRuntimeStrings(conditions)
}

func executedCapabilityFromSnapshot(snapshot *inference.ExecutionSnapshot) string {
	if snapshot == nil {
		return ""
	}
	return strings.TrimSpace(snapshot.ExecutedCapability)
}

func executedCommandTypeFromSnapshot(snapshot *inference.ExecutionSnapshot) string {
	if snapshot == nil {
		return ""
	}
	return strings.TrimSpace(string(snapshot.CommandType))
}

func proposalIDFromSnapshot(snapshot *inference.ExecutionSnapshot) string {
	if snapshot == nil {
		return ""
	}
	return strings.TrimSpace(snapshot.ProposalID)
}

func bindExecutionSnapshotToRuntime(run *naviruntime.RunState, snapshot *inference.ExecutionSnapshot) *inference.ExecutionSnapshot {
	if run == nil || snapshot == nil {
		return snapshot
	}
	bound := *snapshot
	bound.CheckpointRef = firstNonEmpty(strings.TrimSpace(bound.CheckpointRef), strings.TrimSpace(run.LatestCheckpointID))
	return &bound
}

func mergeExecutionSnapshot(prior *inference.ExecutionSnapshot, snapshot *inference.ExecutionSnapshot) *inference.ExecutionSnapshot {
	if snapshot == nil {
		return nil
	}
	merged := *snapshot
	if prior == nil {
		return &merged
	}
	if strings.TrimSpace(merged.ExecutedCapability) == "" &&
		merged.CommandType == "" &&
		(strings.TrimSpace(prior.ExecutedCapability) != "" || prior.CommandType != "") {
		merged = *prior
	}
	merged.ExecutedCapability = firstNonEmpty(strings.TrimSpace(merged.ExecutedCapability), strings.TrimSpace(prior.ExecutedCapability))
	merged.CommandType = firstNonEmptyCommandType(merged.CommandType, prior.CommandType)
	merged.CheckpointRef = firstNonEmpty(strings.TrimSpace(merged.CheckpointRef), strings.TrimSpace(prior.CheckpointRef))
	merged.ProposalID = firstNonEmpty(strings.TrimSpace(merged.ProposalID), strings.TrimSpace(prior.ProposalID))
	if merged.FailureClass == "" {
		merged.FailureClass = prior.FailureClass
	}
	return &merged
}

func runtimeInferenceEnvelope(run *naviruntime.RunState, checkpoint *naviruntime.Checkpoint) *inference.DecisionEnvelope {
	if checkpoint != nil && len(checkpoint.ICSDecisionEnvelope) > 0 {
		var envelope inference.DecisionEnvelope
		if err := json.Unmarshal(checkpoint.ICSDecisionEnvelope, &envelope); err == nil && envelope.Rationale.Version != "" {
			return &envelope
		}
	}
	if run != nil && len(run.ICSDecisionEnvelope) > 0 {
		var envelope inference.DecisionEnvelope
		if err := json.Unmarshal(run.ICSDecisionEnvelope, &envelope); err == nil && envelope.Rationale.Version != "" {
			return &envelope
		}
	}
	return nil
}

func recoveryRouteAction(prior *inference.DecisionEnvelope) string {
	if prior == nil {
		return ""
	}
	return strings.TrimSpace(string(prior.Rationale.RecoveryCheckpoint.Route.Action))
}

func recoveryCheckpointRef(prior *inference.DecisionEnvelope) string {
	if prior == nil {
		return ""
	}
	return strings.TrimSpace(prior.Rationale.RecoveryCheckpoint.CheckpointRef)
}

func executionIntentTarget(prior *inference.DecisionEnvelope) string {
	if prior == nil {
		return ""
	}
	return strings.TrimSpace(prior.Rationale.ExecutionIntent.Target)
}

func promptScratchpad(run *naviruntime.RunState) map[string]string {
	if run == nil || len(run.Scratchpad) == 0 {
		return nil
	}
	dst := make(map[string]string)
	for key, value := range run.Scratchpad {
		if strings.HasPrefix(strings.TrimSpace(key), "ics.") {
			continue
		}
		dst[key] = value
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

func applyInferenceModelDirective(req orchestration.CanonicalRunRequest, directive *inference.ModelDirective) orchestration.CanonicalRunRequest {
	req.CapabilitySurface.ToolNames = append([]string(nil), req.CapabilitySurface.ToolNames...)
	if directive == nil {
		return req
	}
	if !directive.AllowToolCalls {
		req.RequiredOutput.AllowToolCalls = false
		req.CapabilitySurface.ToolNames = nil
		req.CapabilitySurface.SelectionReason = firstNonEmpty(
			strings.TrimSpace(req.CapabilitySurface.SelectionReason)+"; ics model directive disabled executable tools for this cycle",
			"ics model directive disabled executable tools for this cycle",
		)
		return req
	}
	if len(compactStrings(append([]string(nil), directive.ToolNames...))) == 0 {
		req.RequiredOutput.AllowToolCalls = len(req.CapabilitySurface.ToolNames) > 0
		req.CapabilitySurface.SelectionReason = firstNonEmpty(
			strings.TrimSpace(req.CapabilitySurface.SelectionReason)+"; ics model directive preserved resolved tools for this cycle",
			"ics model directive preserved resolved tools for this cycle",
		)
		return req
	}
	allowed := resolvedDirectiveToolNames(req.CapabilitySurface.ToolNames, directive.ToolNames)
	if len(allowed) == 0 {
		req.RequiredOutput.AllowToolCalls = false
		req.CapabilitySurface.ToolNames = nil
		req.CapabilitySurface.SelectionReason = firstNonEmpty(
			strings.TrimSpace(req.CapabilitySurface.SelectionReason)+"; ics model directive requested no tools in the resolved capability surface",
			"ics model directive requested no tools in the resolved capability surface",
		)
		return req
	}
	req.RequiredOutput.AllowToolCalls = true
	req.CapabilitySurface.ToolNames = allowed
	req.CapabilitySurface.SelectionReason = firstNonEmpty(
		strings.TrimSpace(req.CapabilitySurface.SelectionReason)+"; ics model directive narrowed tools to "+strings.Join(allowed, ","),
		"ics model directive narrowed tools to "+strings.Join(allowed, ","),
	)
	return req
}

func resolvedDirectiveToolNames(resolved, requested []string) []string {
	if len(resolved) == 0 || len(requested) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(resolved))
	for _, name := range resolved {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		allowed[name] = struct{}{}
	}
	out := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, name := range requested {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := allowed[name]; !ok {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func proposalIDFromDecision(decision inference.DecisionEnvelope) string {
	if decision.Proposal != nil {
		return strings.TrimSpace(decision.Proposal.ProposalID)
	}
	return strings.TrimSpace(decision.Rationale.DecisionTrace.ProposalID)
}

func exactTargetedCapability(toolNames []string, rationale inference.Rationale) string {
	targetCapability := rationaleTargetCapability(rationale)
	if targetCapability == "" {
		return ""
	}
	if !rationale.ExecutionIntent.AllowsCapabilityExecution() {
		return ""
	}
	for _, toolName := range toolNames {
		if strings.TrimSpace(toolName) == targetCapability {
			return targetCapability
		}
	}
	return ""
}

func rationaleTargetCapability(rationale inference.Rationale) string {
	return rationale.ExecutionIntent.PrimaryCapability()
}

func rationaleAllowedCapabilities(rationale inference.Rationale) []string {
	return compactRuntimeStrings(append([]string(nil), rationale.ExecutionIntent.AuthorizedCapabilities()...))
}

func hasAuthoritativeRationaleGovernanceHandoff(rationale inference.Rationale) bool {
	if strings.TrimSpace(rationale.Governance.TargetCapability) != "" {
		return true
	}
	return len(compactRuntimeStrings(append([]string(nil), rationale.Governance.AllowedCapabilities...))) > 0
}

func (l *AgentLoop) applyPreflightValidation(rationale inference.Rationale, validation governor.ValidationResult, allowed []inference.CapabilityAvailability) inference.Rationale {
	rationale.Governance.ValidationResult = &validation
	rationale.DecisionTrace.GovernanceOutcome = validationOutcomeName(validation.Outcome)
	if validation.Outcome == governor.ValidationRejected {
		rationale.Thresholds.ScoreBand = inference.ThresholdBandBlocked
	}
	if allowed == nil {
		rationale.Governance.TargetCapability = ""
		rationale.Governance.AllowedCapabilities = nil
		rationale.Governance.AllowedCapabilityDetails = nil
		rationale.ExecutionIntent.TargetCapability = ""
		rationale.ExecutionIntent.AllowedCapabilities = nil
		rationale.ChosenAction.TargetCapability = ""
		rationale.ChosenAction.AllowedCapabilities = nil
		rationale.DecisionTrace.TargetCapability = ""
		rationale.DecisionTrace.AllowedCapabilities = nil
		return rationale
	}

	allowed = append([]inference.CapabilityAvailability(nil), allowed...)
	allowedNames := make([]string, 0, len(allowed))
	for _, capability := range allowed {
		allowedNames = append(allowedNames, capability.Name)
	}
	allowedNames = compactRuntimeStrings(allowedNames)
	targetCapability := strings.TrimSpace(rationale.Governance.TargetCapability)
	if targetCapability != "" && !slicesContainsString(allowedNames, targetCapability) {
		targetCapability = ""
	}
	if targetCapability == "" && len(allowedNames) == 1 {
		targetCapability = allowedNames[0]
	}
	rationale.Governance.TargetCapability = targetCapability
	rationale.Governance.AllowedCapabilities = allowedNames
	rationale.Governance.AllowedCapabilityDetails = allowed
	rationale.ExecutionIntent.TargetCapability = targetCapability
	rationale.ExecutionIntent.AllowedCapabilities = append([]string(nil), allowedNames...)
	rationale.ChosenAction.TargetCapability = targetCapability
	rationale.ChosenAction.AllowedCapabilities = append([]string(nil), allowedNames...)
	rationale.DecisionTrace.TargetCapability = targetCapability
	rationale.DecisionTrace.AllowedCapabilities = append([]string(nil), allowedNames...)
	return rationale
}

func mergeInferenceValidation(current *governor.ValidationResult, next governor.ValidationResult) *governor.ValidationResult {
	if current == nil {
		copy := next
		return &copy
	}
	if validationSeverity(next.Outcome) > validationSeverity(current.Outcome) {
		copy := next
		return &copy
	}
	return current
}

func validationSeverity(outcome governor.ValidationOutcome) int {
	switch outcome {
	case governor.ValidationRejected:
		return 4
	case governor.ValidationRequiresConfirmation:
		return 3
	case governor.ValidationModified:
		return 2
	case governor.ValidationApproved:
		return 1
	default:
		return 0
	}
}

func slicesContainsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func snapshotForRunCompletion(outcome schema.ExecutionOutcomeOutcome, summary string, approvalOutcome schema.ApprovalOutcome, proposalID string) *inference.ExecutionSnapshot {
	failureClass := schema.FailureClass("")
	switch outcome {
	case schema.ExecutionOutcomeTimedOut:
		failureClass = schema.FailureClassTimeout
	case schema.ExecutionOutcomeRejectedPreExecution:
		failureClass = schema.FailureClassPolicyBlocked
	case schema.ExecutionOutcomeFailed, schema.ExecutionOutcomeCancelled:
		failureClass = schema.FailureClassExecutionFailure
	case schema.ExecutionOutcomePartiallySucceeded:
		failureClass = schema.FailureClassPartialExecution
	}
	return &inference.ExecutionSnapshot{
		Outcome:         outcome,
		FailureClass:    failureClass,
		ApprovalOutcome: approvalOutcome,
		ProposalID:      strings.TrimSpace(proposalID),
		Summary:         strings.TrimSpace(summary),
	}
}

func snapshotForInterruptedRun(run *naviruntime.RunState, checkpoint *naviruntime.Checkpoint) *inference.ExecutionSnapshot {
	failureClass := schema.FailureClassExecutionFailure
	switch {
	case run != nil && run.InterruptClass == naviruntime.InterruptClassUserCancel:
		failureClass = schema.FailureClassPermissionDenial
	case run != nil && run.InterruptClass == naviruntime.InterruptClassSystem:
		failureClass = schema.FailureClassTimeout
	}
	return &inference.ExecutionSnapshot{
		Outcome:         schema.ExecutionOutcomeCancelled,
		FailureClass:    failureClass,
		ProposalID:      firstNonEmpty(blockedProposalID(run), pendingProposalID(checkpoint)),
		CheckpointRef:   firstNonEmpty(checkpointRefIDForRuntime(checkpoint), latestRunCheckpointRef(run)),
		Summary:         firstNonEmpty(interruptSummary(run), "run interrupted before completion"),
		ApprovalOutcome: approvalOutcomeForInterruptedRun(run),
	}
}

func blockedProposalID(run *naviruntime.RunState) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(run.BlockedOnProposalID)
}

func latestRunCheckpointRef(run *naviruntime.RunState) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(run.LatestCheckpointID)
}

func checkpointRefIDForRuntime(checkpoint *naviruntime.Checkpoint) string {
	if checkpoint == nil {
		return ""
	}
	return strings.TrimSpace(checkpoint.CheckpointID)
}

func interruptSummary(run *naviruntime.RunState) string {
	if run == nil {
		return ""
	}
	return firstNonEmpty(strings.TrimSpace(run.InterruptReason), strings.TrimSpace(run.PauseReason))
}

func approvalOutcomeForInterruptedRun(run *naviruntime.RunState) schema.ApprovalOutcome {
	if run == nil {
		return schema.ApprovalOutcomeNA
	}
	return schema.ApprovalOutcomeNA
}

func firstNonEmptyCommandType(values ...schema.CommandType) schema.CommandType {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func riskLevelFromToolGovernance(raw string) schema.RiskLevel {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(schema.RiskCritical):
		return schema.RiskCritical
	case string(schema.RiskHigh):
		return schema.RiskHigh
	case string(schema.RiskMedium):
		return schema.RiskMedium
	default:
		return schema.RiskLow
	}
}

func reversibilityFromToolGovernance(raw string) schema.ReversibilityClass {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "irreversible":
		return schema.ReversibilityIrreversible
	case "compensable_external", "compensable":
		return schema.ReversibilityCompensable
	default:
		return schema.ReversibilityInternal
	}
}

func marshalInferenceReflectionDetails(details schema.InferenceReflectionDetails) string {
	payload, err := json.Marshal(details)
	if err != nil {
		return ""
	}
	return string(payload)
}
