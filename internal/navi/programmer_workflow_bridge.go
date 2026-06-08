package navi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/open-navi/navi/internal/coderalias"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/skill"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	navitool "github.com/open-navi/navi/internal/tool"
)

const (
	programmerWorkflowScratchpadKey            = "programmer_workflow"
	programmerWorkflowStateScratchpadKey       = "programmer_workflow_state"
	programmerWorkflowOutcomeScratchpadKey     = "programmer_workflow_outcome"
	programmerWorkflowSummaryScratchpadKey     = "programmer_workflow_summary"
	programmerWorkflowProgressStage            = "programmer_workflow"
	programmerWorkflowFallbackRunnerPath       = "plugins/navi-programmer/workflows/bounded_mutation_runner.py"
	programmerWorkflowBoundedContractPath      = "bounded-mutation.compiled.json"
	programmerWorkflowSelfUpdateContractPath   = "self-update-candidate.compiled.json"
	programmerWorkflowTicketDrivenContractPath = "ticket-driven-coding.compiled.json"
)

type programmerWorkflowState struct {
	ProgrammerRun  map[string]any `json:"programmer_run,omitempty"`
	Evidence       map[string]any `json:"evidence,omitempty"`
	ContractPath   string         `json:"contract_path,omitempty"`
	WorkflowID     string         `json:"workflow_id,omitempty"`
	CurrentState   string         `json:"current_state,omitempty"`
	Outcome        string         `json:"outcome,omitempty"`
	BlockedReasons []string       `json:"blocked_reasons,omitempty"`
	LastResult     map[string]any `json:"last_result,omitempty"`
	LastSkillID    string         `json:"last_skill_id,omitempty"`
	LastInterface  string         `json:"last_interface,omitempty"`
}

var invokeProgrammerWorkflow = defaultInvokeProgrammerWorkflow

func defaultInvokeProgrammerWorkflow(ctx context.Context, runnerPath, iface string, arguments map[string]any) (map[string]any, error) {
	pythonExe, err := resolveProgrammerWorkflowPython()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"interface": iface,
		"arguments": arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal runner payload: %w", err)
	}
	cmd := exec.CommandContext(ctx, pythonExe, runnerPath)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run programmer workflow runner: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var result map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		return nil, fmt.Errorf("decode programmer workflow runner output: %w", err)
	}
	status := strings.TrimSpace(anyString(result["status"]))
	if status != "" && status != "success" {
		if errPayload, ok := result["error"].(map[string]any); ok {
			return nil, fmt.Errorf("%s", firstNonEmpty(strings.TrimSpace(anyString(errPayload["message"])), strings.TrimSpace(anyString(errPayload["type"]))))
		}
		return nil, fmt.Errorf("programmer workflow runner returned status %q", status)
	}
	output, ok := result["output"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("programmer workflow runner returned no object output for %s", iface)
	}
	return output, nil
}

func resolveProgrammerWorkflowPython() (string, error) {
	for _, candidate := range []string{"python", "python3", "py"} {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("python executable not found on PATH")
}

func (l *AgentLoop) captureProgrammerWorkflowEvidence(ctx context.Context, run *naviruntime.RunState, inboxItem *naviruntime.InboxItem, tc llm.ToolCall, registeredTool *navitool.Tool, skillEntry *skill.SkillEntry, iface *skill.Interface, rawResult any) error {
	skillID, ifaceName, runnerPath, ok := resolveProgrammerWorkflowSkill(registeredTool, skillEntry, iface)
	if !ok || run == nil {
		return nil
	}
	rawJSON, ok := rawResult.(string)
	if !ok || strings.TrimSpace(rawJSON) == "" {
		return nil
	}
	skillResult, err := decodeProgrammerSkillResult(rawJSON)
	if err != nil {
		return err
	}
	state, err := loadProgrammerWorkflowState(run)
	if err != nil {
		return err
	}
	if state.ProgrammerRun == nil {
		taskSource := firstNonEmpty(strings.TrimSpace(anyString(tc.Arguments["source"])), programmerWorkflowSource(inboxItem))
		contractPath := state.ContractPath
		if contractPath == "" {
			contractPath = programmerWorkflowContractPath(taskSource, mergeProgrammerWorkflowEvidence(state.Evidence, skillResult))
		}
		started, err := invokeProgrammerWorkflow(ctx, runnerPath, "start_run", map[string]any{
			"contract_path": contractPath,
			"run_id":        run.RunID,
			"raw_task":      firstNonEmpty(strings.TrimSpace(anyString(tc.Arguments["raw_task"])), strings.TrimSpace(inboxItemContent(inboxItem)), strings.TrimSpace(run.Scratchpad["current_user_message"])),
			"source":        taskSource,
			"source_id":     firstNonEmpty(strings.TrimSpace(anyString(tc.Arguments["source_id"])), programmerWorkflowSourceID(inboxItem)),
			"metadata": map[string]any{
				"runtime_session_id": run.RuntimeSessionID,
				"tool_name":          tc.Name,
			},
		})
		if err != nil {
			return err
		}
		if programRun, ok := started["programmer_run"].(map[string]any); ok {
			state.ProgrammerRun = programRun
		}
		if evidence, ok := started["evidence"].(map[string]any); ok {
			state.Evidence = evidence
		}
		state.ContractPath = firstNonEmpty(strings.TrimSpace(anyString(started["contract_path"])), contractPath)
		state.WorkflowID = strings.TrimSpace(anyString(mapValue(started, "programmer_run", "workflow_id")))
		if current := strings.TrimSpace(anyString(mapValue(started, "programmer_run", "state"))); current != "" {
			state.CurrentState = current
		}
	}
	state.ContractPath = resolveProgrammerWorkflowContractPath(firstNonEmpty(strings.TrimSpace(anyString(tc.Arguments["source"])), programmerWorkflowSource(inboxItem)), state.ContractPath, mergeProgrammerWorkflowEvidence(state.Evidence, skillResult))
	if wf := coderalias.CanonicalWorkflowID(state.WorkflowID); wf == "" || wf == "navi.programmer.bounded_mutation" || wf == "navi.programmer.ticket_driven_coding" {
		if workflowID := programmerWorkflowIDForContract(state.ContractPath); workflowID != "" {
			state.WorkflowID = workflowID
		}
	}
	if state.ProgrammerRun != nil {
		if workflowID := programmerWorkflowIDForContract(state.ContractPath); workflowID != "" {
			state.ProgrammerRun["workflow_id"] = workflowID
		}
	}

	recordArgs := map[string]any{
		"contract_path": state.ContractPath,
		"skill_id":      skillID,
		"interface":     ifaceName,
		"skill_result":  skillResult,
		"evidence":      cloneAnyMap(state.Evidence),
	}
	for key, value := range tc.Arguments {
		if _, exists := recordArgs[key]; !exists {
			recordArgs[key] = value
		}
	}
	recorded, err := invokeProgrammerWorkflow(ctx, runnerPath, "record_step", recordArgs)
	if err != nil {
		return err
	}
	if evidence, ok := recorded["evidence"].(map[string]any); ok {
		state.Evidence = evidence
	}
	blockedReasons := append([]string{}, state.BlockedReasons...)
	blockedReasons = append(blockedReasons, stringList(recorded["blocked_reasons"])...)
	blockedReasons = append(blockedReasons, stringList(mapValue(recorded, "step_evidence", "blocked_reasons"))...)
	blockedReasons = append(blockedReasons, stringList(mapValue(skillResult, "output", "blocked_reasons"))...)
	blockedReasons = uniqueStrings(blockedReasons)
	if blockedBy := strings.TrimSpace(anyString(mapValue(skillResult, "output", "blocked_by"))); blockedBy != "" {
		blockedReasons = uniqueStrings(append(blockedReasons, blockedBy))
	}
	state.BlockedReasons = blockedReasons
	stepState := strings.TrimSpace(anyString(mapValue(recorded, "step_evidence", "state")))
	if len(blockedReasons) > 0 {
		state.CurrentState = "blocked"
	} else if stepState != "" {
		state.CurrentState = stepState
	}

	synthArgs := map[string]any{
		"contract_path":   state.ContractPath,
		"current_state":   state.CurrentState,
		"blocked_reasons": blockedReasons,
		"evidence":        cloneAnyMap(state.Evidence),
	}
	if taskClass := strings.TrimSpace(anyString(mapValue(state.Evidence, "normalized_task", "task_class"))); taskClass != "" {
		synthArgs["task_class"] = taskClass
	}
	synthesized, err := invokeProgrammerWorkflow(ctx, runnerPath, "synthesize_result", synthArgs)
	if err != nil {
		return err
	}
	state.LastResult = synthesized
	state.Outcome = strings.TrimSpace(anyString(synthesized["outcome"]))
	state.CurrentState = firstNonEmpty(strings.TrimSpace(anyString(synthesized["current_state"])), state.CurrentState)
	if terminal := strings.TrimSpace(state.Outcome); state.CurrentState == "" && (terminal == "blocked" || terminal == "failed" || terminal == "partially_succeeded" || terminal == "review_ready" || terminal == "completed") {
		state.CurrentState = terminal
	}
	state.LastSkillID = skillID
	state.LastInterface = ifaceName
	if len(state.BlockedReasons) == 0 {
		state.BlockedReasons = stringList(synthesized["blocked_reasons"])
	}
	if workflowID := strings.TrimSpace(anyString(synthesized["workflow_id"])); workflowID != "" {
		state.WorkflowID = workflowID
	}

	if err := persistProgrammerWorkflowState(run, state); err != nil {
		return err
	}
	message := fmt.Sprintf("state=%s outcome=%s", state.CurrentState, firstNonEmpty(state.Outcome, state.CurrentState))
	if len(state.BlockedReasons) > 0 {
		message += " blocked_by=" + strings.Join(state.BlockedReasons, ",")
	}
	run.SetScratchpadValue(programmerWorkflowSummaryScratchpadKey, message)
	l.emitToolProgress(ctx, run, tc, firstNonEmpty(state.CurrentState, programmerWorkflowProgressStage), message, run.StartedAt)
	return nil
}

func resolveProgrammerWorkflowSkill(registeredTool *navitool.Tool, skillEntry *skill.SkillEntry, iface *skill.Interface) (skillID, ifaceName, runnerPath string, ok bool) {
	if skillEntry != nil && skillEntry.Spec != nil && coderalias.IsProgrammerScopedSkillID(strings.TrimSpace(skillEntry.Spec.SkillID)) {
		// Canonicalize Coder-facing aliases to the legacy skill id so downstream
		// literal comparisons (task-normalize / mutation interfaces) keep working
		// for both namespaces without further edits. Legacy ids are unchanged.
		skillID = coderalias.CanonicalSkillID(strings.TrimSpace(skillEntry.Spec.SkillID))
		if iface != nil {
			ifaceName = strings.TrimSpace(iface.Name)
		}
		runnerPath = filepath.Clean(filepath.Join(skillEntry.Skill.BaseDir, "..", "..", "workflows", "bounded_mutation_runner.py"))
		return skillID, ifaceName, runnerPath, true
	}
	if registeredTool == nil || registeredTool.Source != navitool.ToolSourceSkill || !coderalias.IsProgrammerScopedSkillID(strings.TrimSpace(registeredTool.SourceID)) {
		return "", "", "", false
	}
	skillID = coderalias.CanonicalSkillID(strings.TrimSpace(registeredTool.SourceID))
	ifaceName = strings.TrimSpace(registeredTool.Metadata.SkillInterface)
	if runnerPath == "" {
		runnerPath = programmerWorkflowFallbackRunnerPath
	}
	return skillID, ifaceName, runnerPath, true
}

func loadProgrammerWorkflowState(run *naviruntime.RunState) (*programmerWorkflowState, error) {
	state := &programmerWorkflowState{}
	if run == nil || strings.TrimSpace(run.Scratchpad[programmerWorkflowScratchpadKey]) == "" {
		return state, nil
	}
	if err := json.Unmarshal([]byte(run.Scratchpad[programmerWorkflowScratchpadKey]), state); err != nil {
		return nil, fmt.Errorf("decode programmer workflow scratchpad: %w", err)
	}
	if state.Evidence == nil {
		state.Evidence = map[string]any{}
	}
	return state, nil
}

func persistProgrammerWorkflowState(run *naviruntime.RunState, state *programmerWorkflowState) error {
	if run == nil || state == nil {
		return nil
	}
	if state.Evidence == nil {
		state.Evidence = map[string]any{}
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode programmer workflow scratchpad: %w", err)
	}
	run.SetScratchpadValue(programmerWorkflowScratchpadKey, string(data))
	run.SetScratchpadValue(programmerWorkflowStateScratchpadKey, state.CurrentState)
	run.SetScratchpadValue(programmerWorkflowOutcomeScratchpadKey, state.Outcome)
	return nil
}

func programmerWorkflowResultFromRun(run *naviruntime.RunState) map[string]any {
	state, err := loadProgrammerWorkflowState(run)
	if err != nil || state == nil {
		return nil
	}
	return cloneAnyMap(state.LastResult)
}

func attachProgrammerWorkflowResult(result *naviruntime.ExecuteResult, run *naviruntime.RunState) {
	if result == nil || run == nil {
		return
	}
	if structured := programmerWorkflowResultFromRun(run); len(structured) > 0 {
		result.ProgrammerResult = structured
	}
	if strings.TrimSpace(result.OutcomeSummary) == "" {
		result.OutcomeSummary = strings.TrimSpace(run.Scratchpad[programmerWorkflowSummaryScratchpadKey])
	}
}

func programmerWorkflowContractPath(taskSource string, evidence map[string]any) string {
	if isSelfUpdateProgrammerEvidence(evidence) {
		return programmerWorkflowSelfUpdateContractPath
	}
	if statePath := strings.TrimSpace(anyString(mapValue(evidence, "raw_task_input", "source"))); isTicketDrivenProgrammerSource(statePath) {
		return programmerWorkflowTicketDrivenContractPath
	}
	if statePath := strings.TrimSpace(anyString(mapValue(evidence, "normalized_task", "task_class"))); statePath == "ticket_driven_mutation" {
		return programmerWorkflowTicketDrivenContractPath
	}
	if isTicketDrivenProgrammerSource(taskSource) {
		return programmerWorkflowTicketDrivenContractPath
	}
	return programmerWorkflowBoundedContractPath
}

func resolveProgrammerWorkflowContractPath(taskSource, currentContract string, evidence map[string]any) string {
	resolved := programmerWorkflowContractPath(taskSource, evidence)
	if resolved != "" {
		return resolved
	}
	return currentContract
}

func programmerWorkflowIDForContract(contractPath string) string {
	switch strings.TrimSpace(contractPath) {
	case programmerWorkflowSelfUpdateContractPath:
		return "navi.programmer.self_update_candidate"
	case programmerWorkflowTicketDrivenContractPath:
		return "navi.programmer.ticket_driven_coding"
	case programmerWorkflowBoundedContractPath:
		return "navi.programmer.bounded_mutation"
	default:
		return ""
	}
}

func isSelfUpdateProgrammerEvidence(evidence map[string]any) bool {
	if strings.TrimSpace(anyString(mapValue(evidence, "normalized_task", "task_class"))) == "self_update_candidate" {
		return true
	}
	if value, ok := mapValue(evidence, "normalized_task", "self_update").(bool); ok && value {
		return true
	}
	if value, ok := mapValue(evidence, "workspace_binding", "is_self_update").(bool); ok && value {
		return true
	}
	return false
}

func mergeProgrammerWorkflowEvidence(existing, skillResult map[string]any) map[string]any {
	merged := cloneAnyMap(existing)
	output, _ := skillResult["output"].(map[string]any)
	for _, key := range []string{"normalized_task", "workspace_binding", "scope_binding", "raw_task_input"} {
		if value, ok := output[key]; ok {
			merged[key] = value
		}
	}
	return merged
}

func isTicketDrivenProgrammerSource(source string) bool {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "linear", "github_issue", "jira", "project_task", "ticket", "issue":
		return true
	default:
		return false
	}
}

func decodeProgrammerSkillResult(raw string) (map[string]any, error) {
	decoded := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("decode programmer skill result: %w", err)
	}
	output, ok := decoded["payload"].(map[string]any)
	if !ok {
		output, _ = decoded["output"].(map[string]any)
	}
	result := map[string]any{
		"status":   firstNonEmpty(strings.TrimSpace(anyString(decoded["status"])), "success"),
		"metadata": cloneAnyMap(decoded["metadata"]),
	}
	if output != nil {
		result["output"] = output
	} else {
		result["output"] = map[string]any{}
	}
	if errPayload, ok := decoded["error"].(map[string]any); ok {
		result["error"] = errPayload
	}
	return result, nil
}

func programmerWorkflowSource(inboxItem *naviruntime.InboxItem) string {
	if inboxItem == nil {
		return "chat"
	}
	return firstNonEmpty(strings.TrimSpace(inboxItem.SourceChannel), "chat")
}

func programmerWorkflowSourceID(inboxItem *naviruntime.InboxItem) string {
	if inboxItem == nil {
		return ""
	}
	return firstNonEmpty(strings.TrimSpace(inboxItem.SourceMessageRef), strings.TrimSpace(inboxItem.ID))
}

func inboxItemContent(inboxItem *naviruntime.InboxItem) string {
	if inboxItem == nil {
		return ""
	}
	return inboxItem.Content
}

func cloneAnyMap(value any) map[string]any {
	src, _ := value.(map[string]any)
	if len(src) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(src))
	for key, item := range src {
		out[key] = item
	}
	return out
}

type programmerWorkspaceBinding struct {
	RepoRoot         string
	AllowedScope     []string
	IsSelfUpdate     bool
	CandidateContext map[string]any
}

func prepareProgrammerToolArguments(workspaceDir string, run *naviruntime.RunState, tc llm.ToolCall, registeredTool *navitool.Tool, skillEntry *skill.SkillEntry, iface *skill.Interface) (map[string]any, error) {
	skillID, ifaceName, _, ok := resolveProgrammerWorkflowSkill(registeredTool, skillEntry, iface)
	if !ok {
		return cloneAnyMap(tc.Arguments), nil
	}
	args := cloneAnyMap(tc.Arguments)
	state, err := loadProgrammerWorkflowState(run)
	if err != nil {
		return nil, err
	}
	if skillID == "navi-programmer.task-normalize" && ifaceName == "bind_scope" {
		return prepareProgrammerBindScopeArguments(workspaceDir, state, args), nil
	}
	binding := programmerBindingFromState(state)
	if binding.RepoRoot == "" || len(binding.AllowedScope) == 0 {
		if programmerMutationInterface(skillID, ifaceName) {
			return nil, navitool.ErrorForFailureCode(
				navitool.ExecutionFailureCodeGovernanceBlocked,
				fmt.Sprintf("programmer workflow blocked: explicit repo/workspace binding is required before %s", tc.Name),
			)
		}
		return args, nil
	}
	args["root"] = binding.RepoRoot
	args["repo_root"] = binding.RepoRoot
	args["allowed_scope"] = append([]string(nil), binding.AllowedScope...)
	if len(binding.CandidateContext) > 0 {
		args["candidate_context"] = cloneAnyMap(binding.CandidateContext)
	}
	if err := validateProgrammerToolArgumentsWithinScope(args, binding); err != nil {
		return nil, err
	}
	return args, nil
}

func prepareProgrammerBindScopeArguments(workspaceDir string, state *programmerWorkflowState, args map[string]any) map[string]any {
	if strings.TrimSpace(anyString(args["explicit_repo"])) != "" || strings.TrimSpace(anyString(args["current_repo"])) != "" {
		return args
	}
	normalizedTask, _ := args["normalized_task"].(map[string]any)
	stateEvidence := map[string]any{}
	if state != nil {
		stateEvidence = state.Evidence
	}
	if strings.TrimSpace(anyString(mapValue(normalizedTask, "task_class", ""))) == "self_update_candidate" || isSelfUpdateProgrammerEvidence(mergeProgrammerWorkflowEvidence(stateEvidence, map[string]any{"output": map[string]any{"normalized_task": normalizedTask}})) {
		if strings.TrimSpace(workspaceDir) != "" {
			args["current_repo"] = strings.TrimSpace(workspaceDir)
		}
	}
	return args
}

func programmerBindingFromState(state *programmerWorkflowState) programmerWorkspaceBinding {
	if state == nil {
		return programmerWorkspaceBinding{}
	}
	binding, _ := mapValue(state.Evidence, "workspace_binding", "").(map[string]any)
	binding = cloneAnyMap(binding)
	if len(binding) == 0 {
		scopeBinding, _ := mapValue(state.Evidence, "scope_binding", "").(map[string]any)
		binding = cloneAnyMap(scopeBinding)
	}
	result := programmerWorkspaceBinding{
		RepoRoot:     strings.TrimSpace(anyString(binding["repo_root"])),
		AllowedScope: stringList(binding["allowed_scope"]),
		IsSelfUpdate: false,
	}
	if value, ok := binding["is_self_update"].(bool); ok {
		result.IsSelfUpdate = value
	}
	if candidateContext, ok := binding["candidate_context"].(map[string]any); ok {
		result.CandidateContext = cloneAnyMap(candidateContext)
	}
	return result
}

func programmerMutationInterface(skillID, ifaceName string) bool {
	switch strings.TrimSpace(skillID) {
	case "navi-programmer.file-mutate":
		return ifaceName == "create_file" || ifaceName == "write_file"
	case "navi-programmer.patch-apply":
		return ifaceName == "apply_patch"
	case "navi-programmer.git-lifecycle":
		return ifaceName == "create_branch" || ifaceName == "create_commit"
	default:
		return false
	}
}

func validateProgrammerToolArgumentsWithinScope(args map[string]any, binding programmerWorkspaceBinding) error {
	if err := validateProgrammerScopeValue(args["path"], binding); err != nil {
		return err
	}
	if err := validateProgrammerScopeValue(args["cwd"], binding); err != nil {
		return err
	}
	if values, ok := args["paths"].([]any); ok {
		for _, value := range values {
			if err := validateProgrammerScopeValue(value, binding); err != nil {
				return err
			}
		}
	}
	if values, ok := args["paths"].([]string); ok {
		for _, value := range values {
			if err := validateProgrammerScopeValue(value, binding); err != nil {
				return err
			}
		}
	}
	if operations, ok := args["operations"].([]any); ok {
		for _, raw := range operations {
			operation, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if err := validateProgrammerScopeValue(operation["path"], binding); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateProgrammerScopeValue(value any, binding programmerWorkspaceBinding) error {
	raw := strings.TrimSpace(anyString(value))
	if raw == "" {
		return nil
	}
	candidate := raw
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(binding.RepoRoot, candidate)
	}
	candidate = filepath.Clean(candidate)
	for _, scope := range binding.AllowedScope {
		scopeRoot := strings.TrimSpace(scope)
		if scopeRoot == "" {
			continue
		}
		if scopeRoot == "." {
			scopeRoot = binding.RepoRoot
		} else if !filepath.IsAbs(scopeRoot) {
			scopeRoot = filepath.Join(binding.RepoRoot, scopeRoot)
		}
		scopeRoot = filepath.Clean(scopeRoot)
		if isProgrammerPathWithinScope(candidate, scopeRoot) {
			return nil
		}
	}
	return navitool.ErrorForFailureCode(
		navitool.ExecutionFailureCodeGovernanceBlocked,
		fmt.Sprintf("programmer workflow blocked: %q is outside the bound scope", raw),
	)
}

func isProgrammerPathWithinScope(candidate, scopeRoot string) bool {
	rel, err := filepath.Rel(scopeRoot, candidate)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	rel = filepath.Clean(rel)
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func stringList(value any) []string {
	items, ok := value.([]any)
	if ok {
		out := make([]string, 0, len(items))
		for _, item := range items {
			if text := strings.TrimSpace(anyString(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	}
	if values, ok := value.([]string); ok {
		out := make([]string, 0, len(values))
		for _, item := range values {
			if text := strings.TrimSpace(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	}
	if text := strings.TrimSpace(anyString(value)); text != "" {
		return []string{text}
	}
	return nil
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
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
	return out
}

func anyString(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	default:
		return fmt.Sprint(value)
	}
}

func mapValue(root map[string]any, key string, nested string) any {
	value, ok := root[key]
	if !ok || nested == "" {
		return value
	}
	typed, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return typed[nested]
}
