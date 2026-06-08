package navi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/inference"
	"github.com/ceoai/navi/internal/navi/proposals"
	"github.com/ceoai/navi/internal/navi/skill"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
	navitool "github.com/ceoai/navi/internal/tool"
	"github.com/ceoai/navi/internal/worldmodel"
	"github.com/google/uuid"
)

const (
	runtimeToolSurface = "runtime"
	loopToolSurface    = "loop"
)

type scheduledMessagesContextKey struct{}

func withScheduledMessages(ctx context.Context, scheduled *[]naviruntime.ScheduledMessage) context.Context {
	return context.WithValue(ctx, scheduledMessagesContextKey{}, scheduled)
}

func scheduledMessagesFromContext(ctx context.Context) *[]naviruntime.ScheduledMessage {
	scheduled, _ := ctx.Value(scheduledMessagesContextKey{}).(*[]naviruntime.ScheduledMessage)
	return scheduled
}

func newSendReplyToolExecutor() navitool.ToolExecutor {
	return navitool.ExecutorFunc(func(ctx context.Context, args map[string]any) (navitool.ToolResult, error) {
		scheduled := scheduledMessagesFromContext(ctx)
		if scheduled == nil {
			return navitool.ToolResult{}, fmt.Errorf("send_reply is only available in runtime sessions")
		}
		content, _ := args["content"].(string)
		delaySec := 0
		if n, ok := args["delay_seconds"]; ok {
			switch v := n.(type) {
			case float64:
				delaySec = int(v)
			case int:
				delaySec = v
			}
		}
		if delaySec < 0 {
			delaySec = 0
		}
		if delaySec > maxScheduledDelaySec {
			return navitool.ToolResult{}, fmt.Errorf("delay_seconds cannot exceed %d (use core-scheduler schedule_task with kind=at or kind=every for longer horizons)", maxScheduledDelaySec)
		}
		if len(*scheduled) >= maxScheduledMessages {
			return navitool.ToolResult{}, fmt.Errorf("maximum %d scheduled messages per run", maxScheduledMessages)
		}
		*scheduled = append(*scheduled, naviruntime.ScheduledMessage{
			Content: content,
			Delay:   time.Duration(delaySec) * time.Second,
		})
		return navitool.ToolResult{Content: "Queued."}, nil
	})
}

func (l *AgentLoop) validateToolGovernanceAction(ctx context.Context, action governor.ActionDescriptor, skillEntry *skill.SkillEntry) governor.ValidationResult {
	// Enrich with owner identity if session matches.
	if l.cfg.NAVI != nil {
		l.cfg.NAVI.EnrichActionWithIdentity(ctx, &action)
	}

	var validation governor.ValidationResult
	if l.cfg.GovConfigPriorityReader != nil {
		validation = governor.ValidateActionWithOwner(ctx, l.cfg.Policy, action, l.cfg.GovConfigPriorityReader)
	} else {
		validation = governor.ValidateAction(l.cfg.Policy, action)
	}
	if l.cfg.AutonomyResolver != nil {
		validation = governor.ApplyAutonomyForAction(validation, l.cfg.AutonomyResolver, action.CommandType, action.Domain, skillEntry)
	}
	return validation
}

func (l *AgentLoop) telegramRefMatchesChatID(ref, chatID string) bool {
	ref = strings.TrimSpace(ref)
	chatID = strings.TrimSpace(chatID)
	if ref == "" || chatID == "" {
		return false
	}
	if ref == chatID {
		return true
	}
	if idx := strings.IndexByte(ref, ':'); idx > 0 {
		return ref[:idx] == chatID
	}
	return false
}

func inferenceGovernanceActionDescriptor(run *naviruntime.RunState, toolName string) (governor.ActionDescriptor, bool) {
	handoff, ok := authoritativeInferenceGovernanceHandoff(run)
	if !ok || !inferenceToolWithinBoundary(handoff, toolName) {
		return governor.ActionDescriptor{}, false
	}
	action := handoff.ActionDescriptor(inference.ChatContext{ChatID: run.RuntimeSessionID})
	if detail, ok := inferenceCapabilityDetail(handoff, toolName); ok {
		action.CommandType = firstNonEmptyCommandType(action.CommandType, detail.CommandType)
		action.Domain = firstNonEmpty(action.Domain, detail.Kind)
	}
	if action.Tags == nil {
		action.Tags = make(map[string]string, 2)
	}
	action.Tags["target_capability"] = strings.TrimSpace(toolName)
	action.Tags["allowed_capabilities"] = strings.Join(inferenceAllowedCapabilities(handoff), ",")
	return action, action.CommandType != "" || action.Domain != "" || len(action.Tags) > 0
}

func inferenceGovernanceHandoff(run *naviruntime.RunState) (inference.GovernanceHandoff, bool) {
	envelope := runtimeInferenceEnvelope(run, nil)
	if envelope == nil {
		return inference.GovernanceHandoff{}, false
	}
	handoff := envelope.Rationale.Governance
	if len(handoff.Tags) == 0 {
		handoff.Tags = inferenceGovernanceTags(run)
	}
	if strings.TrimSpace(handoff.TargetCapability) == "" && len(handoff.AllowedCapabilities) == 0 {
		return inference.GovernanceHandoff{}, false
	}
	return handoff, true
}

func authoritativeInferenceGovernanceHandoff(run *naviruntime.RunState) (inference.GovernanceHandoff, bool) {
	handoff, ok := inferenceGovernanceHandoff(run)
	if !ok {
		return inference.GovernanceHandoff{}, false
	}
	if strings.TrimSpace(handoff.TargetCapability) == "" && len(inferenceAllowedCapabilities(handoff)) == 0 {
		return inference.GovernanceHandoff{}, false
	}
	return handoff, true
}

func inferenceGovernanceTags(run *naviruntime.RunState) map[string]string {
	envelope := runtimeInferenceEnvelope(run, nil)
	if envelope == nil || len(envelope.Rationale.Governance.Tags) == 0 {
		return nil
	}
	tags := map[string]string{}
	for key, value := range envelope.Rationale.Governance.Tags {
		tags[key] = value
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func inferenceToolWithinBoundary(handoff inference.GovernanceHandoff, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	if target := strings.TrimSpace(handoff.TargetCapability); target != "" {
		return target == toolName
	}
	for _, allowed := range handoff.AllowedCapabilities {
		if strings.TrimSpace(allowed) == toolName {
			return true
		}
	}
	return false
}

func inferenceCapabilityDetail(handoff inference.GovernanceHandoff, toolName string) (inference.CapabilityAvailability, bool) {
	toolName = strings.TrimSpace(toolName)
	for _, detail := range handoff.AllowedCapabilityDetails {
		if strings.TrimSpace(detail.Name) == toolName {
			return detail, true
		}
	}
	return inference.CapabilityAvailability{}, false
}

func inferenceAllowedCapabilities(handoff inference.GovernanceHandoff) []string {
	if target := strings.TrimSpace(handoff.TargetCapability); target != "" {
		return []string{target}
	}
	values := make([]string, 0, len(handoff.AllowedCapabilities))
	for _, allowed := range handoff.AllowedCapabilities {
		if trimmed := strings.TrimSpace(allowed); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func (l *AgentLoop) getWorkspaceEnforcer(ctx context.Context, chatID string) *worldmodel.WorkspaceEnforcer {
	if l.cfg.WorldModel == nil {
		return worldmodel.NewWorkspaceEnforcer(nil, string(schema.WorkspaceOperatingModeGlobal))
	}

	mode := string(l.cfg.WorldModel.GetOperatingMode(ctx))
	projectID := ""
	if chatID != "" && l.cfg.Chats != nil {
		chat, err := l.cfg.Chats.GetChat(ctx, chatID)
		if err == nil && chat != nil && chat.ProjectID != nil {
			projectID = string(*chat.ProjectID)
		}
	}
	ws, _, err := l.cfg.WorldModel.ResolveActiveWorkspace(ctx, projectID)
	if err != nil {
		slog.Warn("navi: resolve active workspace failed", "chat_id", chatID, "project_id", projectID, "error", err)
		ws = nil
	}

	var globalProtected []string
	if ownerID, _ := store.GetOwnerID(ctx, l.cfg.WorldModel.DB()); ownerID != "" {
		configs, _ := l.cfg.WorldModel.ListConfigurationByScope(ctx, "owner", ownerID, 100)
		for _, c := range configs {
			if c.Key == "protected_paths" {
				globalProtected = append(globalProtected, parseProtectedPathConfigurationValue(c.Value)...)
			}
		}
	}

	enforcer := worldmodel.NewWorkspaceEnforcer(ws, mode)
	enforcer.GlobalProtectedPaths = globalProtected
	return enforcer
}

// postIssuanceTargetPath resolves the authoritative target path for an
// already-issued permit. Post-issuance helpers must consume permit.Contract and
// instance-bound permit data only; they must not inspect registry metadata.
func (l *AgentLoop) postIssuanceTargetPath(permit *inference.ToolPermit, args map[string]any) (string, bool) {
	if permit == nil || permit.Contract == nil {
		return "", false
	}
	if targetPath := strings.TrimSpace(permit.TargetPath); targetPath != "" {
		return targetPath, true
	}

	contract := permit.Contract
	pathArgName := strings.TrimSpace(contract.TargetPathArg)
	if pathArgName == "" {
		return "", false
	}

	rawPath := stringArgFromArgs(args, pathArgName)
	if rawPath == "" {
		return "", false
	}
	if contract.WorkspaceScopedPath {
		return l.resolveWorkspaceScopedPath(rawPath), true
	}
	return rawPath, true
}

func (l *AgentLoop) resolveWorkspaceScopedPath(rawPath string) string {
	workspaceRoot := strings.TrimSpace(l.cfg.WorkspaceDir)
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return workspaceRoot
	}
	if workspaceRoot == "" || filepath.IsAbs(rawPath) {
		return rawPath
	}
	return filepath.Join(workspaceRoot, rawPath)
}

func stringArgFromArgs(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func parseProtectedPathConfigurationValue(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var parsed []string
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			return normalizeProtectedPathList(parsed)
		}
	}

	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	raw = strings.ReplaceAll(raw, "\n", ",")
	parts := strings.Split(raw, ",")
	return normalizeProtectedPathList(parts)
}

func normalizeProtectedPathList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
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
		normalized = append(normalized, value)
	}
	return normalized
}

// postIssuanceWorkspaceCheck validates workspace scope for an already-issued
// permit using the authoritative ToolContract only.
func (l *AgentLoop) postIssuanceWorkspaceCheck(ctx context.Context, chatID string, permit *inference.ToolPermit, args map[string]any) (worldmodel.WorkspaceValidationResult, bool) {
	enforcer := l.getWorkspaceEnforcer(ctx, chatID)
	targetPath, ok := l.postIssuanceTargetPath(permit, args)
	if !ok {
		return worldmodel.WorkspaceValidationResult{}, false
	}
	if permit == nil || permit.Contract == nil || !permit.Contract.WorkspaceAction.IsValid() {
		return worldmodel.WorkspaceValidationResult{}, false
	}
	return enforcer.CheckAction(ctx, targetPath, permit.Contract.WorkspaceAction), true
}

func (l *AgentLoop) workspaceAuditFields(ctx context.Context, chatID string, permit *inference.ToolPermit, args map[string]any) (string, bool) {
	check, ok := l.postIssuanceWorkspaceCheck(ctx, chatID, permit, args)
	if !ok {
		enforcer := l.getWorkspaceEnforcer(ctx, chatID)
		if enforcer.ActiveWorkspace != nil {
			return enforcer.ActiveWorkspace.ID, false
		}
		return "", false
	}
	return check.WorkspaceID, check.BoundaryCross
}

// recordBlockedToolExecutionOutcome records an already-authorized tool attempt
// that was blocked before execution. It must consume permit.Contract rather
// than re-deriving authority from registry metadata.
func (l *AgentLoop) recordBlockedToolExecutionOutcome(ctx context.Context, chatID, runID string, permit *inference.ToolPermit, tc llm.ToolCall, validation governor.ValidationResult, proposalID string) {
	if l.cfg.SaveExecutionOutcome == nil {
		return
	}
	if permit == nil || permit.Contract == nil {
		return
	}

	now := time.Now().UTC()
	commandID := uuid.New().String()
	workspaceID, boundaryCrossing := l.workspaceAuditFields(ctx, chatID, permit, tc.Arguments)

	failureClass := schema.FailureClassValidationRejection
	if validation.Outcome == governor.ValidationRequiresConfirmation || validation.Outcome == governor.ValidationModified {
		failureClass = schema.FailureClassPolicyBlocked
	}

	skillIDs := []string{}
	if strings.TrimSpace(permit.Contract.SkillName) != "" {
		skillIDs = []string{permit.Contract.SkillName}
	}

	affectedEntities := "[]"
	if chatID != "" {
		if payload, err := json.Marshal([]string{"session:" + chatID}); err == nil {
			affectedEntities = string(payload)
		}
	}

	_ = l.cfg.SaveExecutionOutcome(ctx, schema.ExecutionOutcome{
		AttemptID:            commandID + ":1",
		CommandID:            commandID,
		AttemptNumber:        1,
		CommandType:          permit.Contract.CommandType,
		StartTime:            now,
		EndTime:              &now,
		Outcome:              schema.ExecutionOutcomeRejectedPreExecution,
		FailureClass:         failureClass,
		FailureReason:        validation.Reason,
		AffectedEntities:     affectedEntities,
		Retryable:            false,
		CompensationRequired: false,
		CompensationStatus:   schema.CompensationStatusNotRequired,
		RecoveryStatus:       schema.RecoveryStatusNotRequired,
		ProposalID:           strings.TrimSpace(permit.ApprovedProposalID),
		RunID:                runID,
		RuntimeSessionID:     chatID,
		CorrelationID:        firstNonEmpty(runID, chatID),
		ParentRunID:          runID,
		SkillIDs:             skillIDs,
		WorkspaceID:          workspaceID,
		BoundaryCrossing:     boundaryCrossing,
		ApprovalRequired:     validation.Outcome == governor.ValidationRequiresConfirmation || validation.Outcome == governor.ValidationModified,
		ApprovalOutcome:      schema.ApprovalOutcomeNA,
	})
}

func (l *AgentLoop) resolveApprovalOutcome(proposalID string, outcome governor.ValidationOutcome) schema.ApprovalOutcome {
	if proposalID != "" {
		// In V1, we assume all approved proposals for boundary crossings are 'allow_once'
		// unless the proposal itself specified 'always_allow'. For simplicity, we favor 'allow_once'.
		return schema.ApprovalOutcomeAllowOnce
	}
	if outcome == governor.ValidationRequiresConfirmation {
		// Still in confirmation state
		return schema.ApprovalOutcomeNA
	}
	if outcome == governor.ValidationApproved {
		return schema.ApprovalOutcomeNA
	}
	return schema.ApprovalOutcomeDenied
}

// toolCommandType and toolWorkspaceAction belong to the contract-compilation
// boundary only. Once a ToolContract has been issued, post-issuance helpers
// must read execution semantics from the contract instead of consulting tool
// registry metadata again.
func toolCommandType(toolEntry *navitool.Tool, fallback schema.CommandType) schema.CommandType {
	if toolEntry != nil && toolEntry.Governance.CommandType != "" {
		return toolEntry.Governance.CommandType
	}
	if fallback != "" {
		return fallback
	}
	return schema.CommandTypeInvoke
}

func toolWorkspaceAction(toolEntry *navitool.Tool, fallback schema.CommandType) schema.WorkspaceActionType {
	if toolEntry != nil && toolEntry.Governance.WorkspaceAction.IsValid() {
		return toolEntry.Governance.WorkspaceAction
	}
	return schema.WorkspaceActionForCommandType(toolCommandType(toolEntry, fallback))
}

func (l *AgentLoop) governanceProposalPayload(ctx context.Context, run *naviruntime.RunState, contract *inference.ToolContract, tc llm.ToolCall, validation governor.ValidationResult, resolvedTargetPath string) any {
	chatID, runID := "", ""
	if run != nil {
		chatID = run.RuntimeSessionID
		runID = run.RunID
	}
	targetCapability := strings.TrimSpace(tc.Name)
	handoff, hasHandoff := authoritativeInferenceGovernanceHandoff(run)
	if hasHandoff && targetCapability != "" {
		handoff = governanceHandoffForCapability(handoff, targetCapability)
	}
	executionIntent, hasIntent := inferenceExecutionIntent(run)
	if hasIntent && targetCapability != "" {
		executionIntent = executionIntentForCapability(executionIntent, targetCapability)
	}
	payload := map[string]any{
		"tool_name": tc.Name,
		"arguments": tc.Arguments,
		"chat_id":   chatID,
	}
	if runID != "" {
		payload["run_id"] = runID
	}
	if hasHandoff {
		payload["ics_governance_handoff"] = handoff
	}
	if hasIntent {
		payload["ics_execution_intent"] = executionIntent
	}
	if contract != nil {
		allowedCapabilities := inferenceAllowedCapabilitiesFromRun(run)
		if targetCapability != "" {
			allowedCapabilities = []string{targetCapability}
		}
		payload["governance"] = map[string]any{
			"command_type":         contract.CommandType,
			"domain":               contract.Domain,
			"actor_kind":           contract.ActorKind,
			"target_capability":    firstNonEmpty(targetCapability, strings.TrimSpace(contract.ToolName)),
			"allowed_capabilities": allowedCapabilities,
		}
	} else if hasHandoff {
		payload["governance"] = map[string]any{
			"command_type":         handoff.CommandType,
			"domain":               handoff.Domain,
			"actor_kind":           handoff.ActorKind,
			"target_capability":    strings.TrimSpace(handoff.TargetCapability),
			"allowed_capabilities": inferenceAllowedCapabilities(handoff),
			"tags":                 handoff.Tags,
		}
	}
	if contract == nil {
		return payload
	}

	if resolvedTargetPath == "" {
		return payload
	}
	// Post-issuance proposal shaping must use the issued contract rather than
	// re-deriving workspace authority from registry metadata.
	enforcer := l.getWorkspaceEnforcer(ctx, chatID)
	check := enforcer.CheckAction(ctx, resolvedTargetPath, contract.WorkspaceAction)
	if !check.BoundaryCross {
		return payload
	}

	return proposals.WorkspaceBoundaryAction{
		Type:            "workspace_boundary_crossing",
		ToolName:        tc.Name,
		Arguments:       tc.Arguments,
		ChatID:          chatID,
		RunID:           runID,
		ICSGovernance:   pointerIfGovernanceHandoff(hasHandoff, handoff),
		ICSIntent:       pointerIfExecutionIntentForCapability(run, targetCapability),
		WorkspaceID:     check.WorkspaceID,
		TargetPath:      resolvedTargetPath,
		WorkspaceAction: contract.WorkspaceAction,
		ActionTypes:     []string{string(contract.WorkspaceAction)},
		RuleScope:       resolvedTargetPath,
	}
}

func inferenceAllowedCapabilitiesFromRun(run *naviruntime.RunState) []string {
	intent, ok := inferenceExecutionIntent(run)
	if ok {
		return intent.AuthorizedCapabilities()
	}
	handoff, ok := authoritativeInferenceGovernanceHandoff(run)
	if ok {
		return inferenceAllowedCapabilities(handoff)
	}
	return nil
}

func inferenceExecutionIntent(run *naviruntime.RunState) (inference.ExecutionIntent, bool) {
	envelope := runtimeInferenceEnvelope(run, nil)
	if envelope == nil {
		return inference.ExecutionIntent{}, false
	}
	intent := envelope.Rationale.ExecutionIntent
	if strings.TrimSpace(intent.TargetCapability) == "" && len(intent.AllowedCapabilities) == 0 {
		return inference.ExecutionIntent{}, false
	}
	return intent, true
}

func pointerIfGovernanceHandoff(ok bool, handoff inference.GovernanceHandoff) *inference.GovernanceHandoff {
	if !ok {
		return nil
	}
	copy := handoff
	return &copy
}

func pointerIfExecutionIntent(run *naviruntime.RunState) *inference.ExecutionIntent {
	intent, ok := inferenceExecutionIntent(run)
	if !ok {
		return nil
	}
	copy := intent
	return &copy
}

func pointerIfExecutionIntentForCapability(run *naviruntime.RunState, capability string) *inference.ExecutionIntent {
	intent, ok := inferenceExecutionIntent(run)
	if !ok {
		return nil
	}
	copy := executionIntentForCapability(intent, capability)
	return &copy
}

func executionIntentForCapability(intent inference.ExecutionIntent, capability string) inference.ExecutionIntent {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return intent
	}
	intent.TargetCapability = capability
	intent.AllowedCapabilities = []string{capability}
	return intent
}

func governanceHandoffForCapability(handoff inference.GovernanceHandoff, capability string) inference.GovernanceHandoff {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return handoff
	}
	handoff.TargetCapability = capability
	handoff.AllowedCapabilities = []string{capability}
	if len(handoff.AllowedCapabilityDetails) > 0 {
		filtered := make([]inference.CapabilityAvailability, 0, 1)
		for _, detail := range handoff.AllowedCapabilityDetails {
			if strings.TrimSpace(detail.Name) == capability {
				filtered = append(filtered, detail)
			}
		}
		handoff.AllowedCapabilityDetails = filtered
	}
	return handoff
}

func (l *AgentLoop) ensureToolRegistry() (*navitool.Registry, error) {
	if l.cfg.ToolRegistry != nil {
		return l.cfg.ToolRegistry, nil
	}
	reg, err := buildRuntimeToolRegistry(l.cfg)
	if err != nil {
		return nil, err
	}
	l.cfg.ToolRegistry = reg
	return reg, nil
}

// EnsureToolRegistry returns the lazily-built runtime tool registry (same as internal ensure).
func (l *AgentLoop) EnsureToolRegistry() (*navitool.Registry, error) {
	return l.ensureToolRegistry()
}

func (l *AgentLoop) toolDefinitionsForSurfaceWithError(surface string) ([]llm.ToolDefinition, error) {
	reg, err := l.ensureToolRegistry()
	if err != nil {
		return nil, fmt.Errorf("build tool registry for %s surface: %w", surface, err)
	}
	return reg.DefinitionsFor(surface), nil
}

func (l *AgentLoop) toolDefinitionsForSurface(surface string) []llm.ToolDefinition {
	defs, err := l.toolDefinitionsForSurfaceWithError(surface)
	if err != nil {
		slog.Warn("navi: build tool registry failed", "surface", surface, "error", err)
		return nil
	}
	return defs
}
