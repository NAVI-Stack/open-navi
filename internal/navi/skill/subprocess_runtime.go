package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/sandbox"
	"github.com/open-navi/navi/internal/schema"
)

const (
	defaultCoderSandboxProfile = "local-coder-python-network-disabled"
	sandboxSkillMountTarget    = "/navi/skill"
)

type subprocessJSONRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      string                 `json:"id"`
	Method  string                 `json:"method"`
	Params  subprocessInvokeParams `json:"params"`
}

type subprocessInvokeParams struct {
	SkillID   string         `json:"skill_id"`
	Interface string         `json:"interface"`
	Workspace string         `json:"workspace,omitempty"`
	Input     map[string]any `json:"input"`
	Policy    map[string]any `json:"policy,omitempty"`
}

type subprocessJSONRPCResponse struct {
	JSONRPC string                  `json:"jsonrpc"`
	ID      any                     `json:"id"`
	Result  json.RawMessage         `json:"result,omitempty"`
	Error   *subprocessJSONRPCError `json:"error,omitempty"`
}

type subprocessJSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func executeSubprocess(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (string, error) {
	if entry.Spec == nil {
		return "", fmt.Errorf("subprocess transport requires a skill spec")
	}

	command := normalizedCommand(iface.Transport.Command)
	if len(command) == 0 {
		return marshalExecutionResult(errorExecutionResult("error", entry, iface, "TransportConfigError", "subprocess transport missing command"))
	}

	protocol := strings.TrimSpace(iface.Transport.Protocol)
	if protocol == "" {
		protocol = defaultSubprocessProto
	}
	if protocol != defaultSubprocessProto {
		return marshalExecutionResult(errorExecutionResult("error", entry, iface, "TransportConfigError", fmt.Sprintf("unsupported subprocess protocol %q", protocol)))
	}

	invocationID := uuid.NewString()
	execMeta, _ := ExecutionContextFromContext(ctx)
	workspace := absoluteWorkspaceDir(execMeta.WorkspaceDir)
	if entry.Spec.Security.Sandbox.Required {
		return executeSandboxedSubprocess(ctx, entry, iface, args, command, protocol, invocationID, execMeta, workspace)
	}
	return executeHostSubprocess(ctx, entry, iface, args, command, protocol, invocationID, workspace)
}

func executeHostSubprocess(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any, command []string, protocol string, invocationID string, workspace string) (string, error) {
	payload := subprocessJSONRPCRequest{
		JSONRPC: "2.0",
		ID:      invocationID,
		Method:  "invoke",
		Params: subprocessInvokeParams{
			SkillID:   entry.Spec.SkillID,
			Interface: iface.Name,
			Workspace: workspace,
			Input:     args,
			Policy:    subprocessPolicy(entry.Spec),
		},
	}
	input, err := marshalSubprocessInput(payload)
	if err != nil {
		return marshalExecutionResult(subprocessErrorExecutionResult("error", entry, iface, invocationID, 0, "MarshalError", err.Error()))
	}

	timeoutMS := entry.Spec.Performance.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = defaultPythonTimeoutMS
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(runCtx, command[0], command[1:]...)
	if strings.TrimSpace(entry.Skill.BaseDir) != "" {
		cmd.Dir = entry.Skill.BaseDir
	}
	cmd.Env = append(os.Environ(),
		"NAVI_SKILL_INVOCATION_ID="+invocationID,
		"NAVI_SUBPROCESS_RUNTIME="+strings.TrimSpace(iface.Transport.Runtime),
		"NAVI_SUBPROCESS_PROTOCOL="+protocol,
	)
	if workspace != "" {
		cmd.Env = append(cmd.Env, "NAVI_WORKSPACE_DIR="+workspace)
	}
	cmd.Stdin = bytes.NewReader(append(input, '\n'))

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	start := time.Now()
	runErr := cmd.Run()
	durationMS := time.Since(start).Milliseconds()
	if runCtx.Err() == context.DeadlineExceeded {
		return marshalExecutionResult(subprocessErrorExecutionResult("timeout", entry, iface, invocationID, durationMS, "TimeoutError", "execution timeout"))
	}

	rawStdout := bytes.TrimSpace(stdoutBuf.Bytes())
	if len(rawStdout) > defaultMaxOutputBytes {
		result := subprocessErrorExecutionResult("truncated", entry, iface, invocationID, durationMS, "OutputTruncated", "output truncated")
		result.Payload = string(rawStdout[:defaultMaxOutputBytes])
		return marshalExecutionResult(result)
	}

	if len(rawStdout) == 0 {
		msg := strings.TrimSpace(stderrBuf.String())
		if msg == "" && runErr != nil {
			msg = runErr.Error()
		}
		if msg == "" {
			msg = "subprocess produced no output"
		}
		return marshalExecutionResult(subprocessErrorExecutionResult("error", entry, iface, invocationID, durationMS, "EmptyOutput", msg))
	}

	response, err := decodeSubprocessJSONRPCResponse(rawStdout, invocationID)
	if err != nil {
		msg := fmt.Sprintf("invalid jsonrpc_stdio response: %v", err)
		if stderr := strings.TrimSpace(stderrBuf.String()); stderr != "" {
			msg += ": " + stderr
		}
		return marshalExecutionResult(subprocessErrorExecutionResult("error", entry, iface, invocationID, durationMS, "DecodeError", msg))
	}

	result := normalizeSubprocessJSONRPCResult(response, entry, iface, invocationID, durationMS, protocol)
	if runErr != nil && result.Error == nil && result.Status == string(SkillResultSuccess) {
		result.Status = string(SkillResultStatusError)
		result.Error = &SkillResultError{
			Type:    "ProcessError",
			Message: runErr.Error(),
		}
	}
	return marshalExecutionResult(result)
}

func executeSandboxedSubprocess(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any, command []string, protocol string, invocationID string, execMeta ExecutionContext, workspaceRoot string) (string, error) {
	workspaceRoot = strings.TrimSpace(firstNonEmptyString(workspaceRoot, stringArg(args["root"]), stringArg(args["repo_root"])))
	if workspaceRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return marshalExecutionResult(subprocessErrorExecutionResult("error", entry, iface, invocationID, 0, "WorkspaceResolveError", err.Error()))
		}
		workspaceRoot = cwd
	}
	absWorkspace, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return marshalExecutionResult(subprocessErrorExecutionResult("error", entry, iface, invocationID, 0, "WorkspaceResolveError", err.Error()))
	}

	profileID := subprocessSandboxProfileID(iface, args)
	profile, err := resolveSubprocessSandboxProfile(ctx, execMeta, profileID)
	if err != nil {
		return marshalExecutionResult(subprocessErrorExecutionResult(string(SkillResultPolicyBlock), entry, iface, invocationID, 0, "SandboxProfileError", err.Error()))
	}
	mountTarget := strings.TrimSpace(profile.WorkspaceMountTarget)
	if mountTarget == "" {
		mountTarget = "/workspace"
	}

	payload := subprocessJSONRPCRequest{
		JSONRPC: "2.0",
		ID:      invocationID,
		Method:  "invoke",
		Params: subprocessInvokeParams{
			SkillID:   entry.Spec.SkillID,
			Interface: iface.Name,
			Workspace: mountTarget,
			Input:     args,
			Policy:    subprocessPolicy(entry.Spec),
		},
	}
	input, err := marshalSubprocessInput(payload)
	if err != nil {
		return marshalExecutionResult(subprocessErrorExecutionResult("error", entry, iface, invocationID, 0, "MarshalError", err.Error()))
	}

	runner := execMeta.SandboxRunner
	if runner == nil {
		runner = sandbox.NewDockerRunner()
	}
	handle, err := runner.Prepare(ctx, sandbox.PrepareRequest{
		Profile:       profile,
		WorkspaceRoot: absWorkspace,
		RunID:         execMeta.RunID,
		ExtraMounts: []sandbox.Mount{{
			Source:   entry.Skill.BaseDir,
			Target:   sandboxSkillMountTarget,
			ReadOnly: true,
		}},
	})
	if err != nil {
		return marshalExecutionResult(subprocessErrorExecutionResult(string(SkillResultPolicyBlock), entry, iface, invocationID, 0, "SandboxPrepareError", err.Error()))
	}
	defer func() {
		_ = runner.Cleanup(ctx, handle)
	}()

	timeoutMS := entry.Spec.Performance.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = defaultPythonTimeoutMS
	}
	start := time.Now()
	ran, err := runner.Run(ctx, handle, sandbox.Command{
		Args:      command,
		Workdir:   ".",
		TimeoutMS: timeoutMS,
		Stdin:     append(input, '\n'),
	})
	durationMS := time.Since(start).Milliseconds()
	if ran.DurationMS > 0 {
		durationMS = int64(ran.DurationMS)
	}
	if ran.TimedOut {
		return marshalExecutionResult(subprocessErrorExecutionResult("timeout", entry, iface, invocationID, durationMS, "TimeoutError", "execution timeout"))
	}
	if err != nil {
		return marshalExecutionResult(subprocessErrorExecutionResult("error", entry, iface, invocationID, durationMS, "SandboxRunError", err.Error()))
	}
	rawStdout := bytes.TrimSpace([]byte(ran.Stdout))
	if len(rawStdout) > defaultMaxOutputBytes {
		result := subprocessErrorExecutionResult("truncated", entry, iface, invocationID, durationMS, "OutputTruncated", "output truncated")
		result.Payload = string(rawStdout[:defaultMaxOutputBytes])
		return marshalExecutionResult(result)
	}
	if len(rawStdout) == 0 {
		msg := strings.TrimSpace(ran.Stderr)
		if msg == "" {
			msg = "subprocess produced no output"
		}
		return marshalExecutionResult(subprocessErrorExecutionResult("error", entry, iface, invocationID, durationMS, "EmptyOutput", msg))
	}
	response, err := decodeSubprocessJSONRPCResponse(rawStdout, invocationID)
	if err != nil {
		msg := fmt.Sprintf("invalid jsonrpc_stdio response: %v", err)
		if stderr := strings.TrimSpace(ran.Stderr); stderr != "" {
			msg += ": " + stderr
		}
		return marshalExecutionResult(subprocessErrorExecutionResult("error", entry, iface, invocationID, durationMS, "DecodeError", msg))
	}
	result := normalizeSubprocessJSONRPCResult(response, entry, iface, invocationID, durationMS, protocol)
	if ran.ExitCode != 0 && result.Error == nil && result.Status == string(SkillResultSuccess) {
		result.Status = string(SkillResultStatusError)
		result.Error = &SkillResultError{
			Type:    "ProcessError",
			Message: fmt.Sprintf("subprocess exited with code %d", ran.ExitCode),
		}
	}
	return marshalExecutionResult(result)
}

func marshalSubprocessInput(payload subprocessJSONRPCRequest) ([]byte, error) {
	input, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if len(input) > defaultMaxInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", defaultMaxInputBytes)
	}
	return input, nil
}

func normalizedCommand(command []string) []string {
	out := make([]string, 0, len(command))
	for _, part := range command {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func subprocessSandboxProfileID(iface *Interface, args map[string]any) string {
	if args != nil {
		if profileID := stringArg(args["sandbox_profile"]); profileID != "" {
			return profileID
		}
		if profileID := stringArg(args["sandbox_profile_id"]); profileID != "" {
			return profileID
		}
	}
	if iface != nil {
		if profileID := strings.TrimSpace(iface.Transport.SandboxProfile); profileID != "" {
			return profileID
		}
	}
	return defaultCoderSandboxProfile
}

func resolveSubprocessSandboxProfile(ctx context.Context, execMeta ExecutionContext, profileID string) (schema.SandboxProfile, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		profileID = defaultCoderSandboxProfile
	}
	if execMeta.SandboxProfileResolver != nil {
		return execMeta.SandboxProfileResolver(ctx, profileID)
	}
	return schema.SandboxProfile{}, fmt.Errorf("sandbox profile resolver not configured: %s", profileID)
}

func stringArg(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func subprocessPolicy(spec *OSS27Spec) map[string]any {
	policy := map[string]any{
		"network":    "none",
		"filesystem": "workspace_ro",
		"secrets":    []string{},
	}
	if len(spec.Security.Sandbox.NetworkEgress) > 0 {
		network := spec.Security.Sandbox.NetworkEgress[0]
		switch network {
		case "none", "package_registries_only", "github_only", "full_unlocked_sandbox":
			policy["network"] = network
		default:
			policy["network"] = "approved_hosts"
			policy["approved_hosts"] = spec.Security.Sandbox.NetworkEgress
		}
	}
	for _, effect := range spec.Effects.SideEffects {
		switch strings.TrimSpace(effect) {
		case "filesystem_write", "file_write", "workspace_write":
			policy["filesystem"] = "workspace_rw"
		}
	}
	return policy
}

func decodeSubprocessJSONRPCResponse(raw []byte, expectedID string) (*subprocessJSONRPCResponse, error) {
	var single subprocessJSONRPCResponse
	if err := json.Unmarshal(raw, &single); err == nil && single.JSONRPC == "2.0" {
		if !subprocessIDMatches(single.ID, expectedID) {
			return nil, fmt.Errorf("response id %q does not match invocation %q", fmt.Sprint(single.ID), expectedID)
		}
		return &single, nil
	}

	var lastErr error
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var response subprocessJSONRPCResponse
		if err := json.Unmarshal(line, &response); err != nil {
			lastErr = err
			continue
		}
		if response.JSONRPC != "2.0" {
			continue
		}
		if !subprocessIDMatches(response.ID, expectedID) {
			lastErr = fmt.Errorf("response id %q does not match invocation %q", fmt.Sprint(response.ID), expectedID)
			continue
		}
		return &response, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("no JSON-RPC response line found")
}

func subprocessIDMatches(id any, expected string) bool {
	if s, ok := id.(string); ok {
		return s == expected
	}
	return fmt.Sprint(id) == expected
}

func rawJSONObjectHasAny(raw json.RawMessage, keys ...string) bool {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return false
	}
	for _, key := range keys {
		if _, ok := obj[key]; ok {
			return true
		}
	}
	return false
}

func normalizeSubprocessJSONRPCResult(response *subprocessJSONRPCResponse, entry *SkillEntry, iface *Interface, invocationID string, durationMS int64, protocol string) SkillExecutionResult {
	if response.Error != nil {
		result := subprocessErrorExecutionResult("error", entry, iface, invocationID, durationMS, "JSONRPCError", response.Error.Message)
		result.Payload = response.Error.Data
		return result
	}

	if len(bytes.TrimSpace(response.Result)) == 0 || bytes.Equal(bytes.TrimSpace(response.Result), []byte("null")) {
		result := SkillExecutionResult{
			Status:     string(SkillResultSuccess),
			DurationMS: durationMS,
		}
		fillSubprocessMetadata(&result, entry, iface, invocationID, protocol)
		return result
	}

	var executionResult SkillExecutionResult
	if !rawJSONObjectHasAny(response.Result, "output", "metrics") {
		if err := json.Unmarshal(response.Result, &executionResult); err == nil && executionResult.Status != "" {
			if executionResult.DurationMS <= 0 {
				executionResult.DurationMS = durationMS
			}
			fillSubprocessMetadata(&executionResult, entry, iface, invocationID, protocol)
			return executionResult
		}
	}

	var skillResult SkillResult
	if !rawJSONObjectHasAny(response.Result, "metrics") {
		if err := json.Unmarshal(response.Result, &skillResult); err == nil && (skillResult.Status != "" || skillResult.Output != nil || skillResult.Error != nil) {
			status := skillResult.Status
			if status == "" {
				status = SkillResultSuccess
				if skillResult.Error != nil {
					status = SkillResultStatusError
				}
			}
			result := SkillExecutionResult{
				Status:     string(status),
				Payload:    skillResult.Output,
				Error:      skillResult.Error,
				DurationMS: skillResult.DurationMS,
				Metadata:   skillResult.Metadata,
			}
			if result.DurationMS <= 0 {
				result.DurationMS = durationMS
			}
			fillSubprocessMetadata(&result, entry, iface, invocationID, protocol)
			return result
		}
	}

	var resultMap map[string]any
	if err := json.Unmarshal(response.Result, &resultMap); err != nil {
		result := SkillExecutionResult{
			Status:     string(SkillResultSuccess),
			Payload:    string(response.Result),
			DurationMS: durationMS,
		}
		fillSubprocessMetadata(&result, entry, iface, invocationID, protocol)
		return result
	}

	status := firstNonEmpty(stringValue(resultMap["status"]), string(SkillResultSuccess))
	result := SkillExecutionResult{
		Status:     status,
		Payload:    resultMap,
		DurationMS: firstPositiveDuration(durationMS, resultMap["duration_ms"], nestedValue(resultMap, "metrics", "duration_ms")),
		Error:      skillResultErrorFromAny(resultMap["error"]),
	}
	if output, ok := resultMap["output"]; ok {
		result.Payload = output
	} else if payload, ok := resultMap["payload"]; ok {
		result.Payload = payload
	}
	if result.Error == nil && status != string(SkillResultSuccess) {
		result.Error = &SkillResultError{
			Type:    "WorkerResult",
			Message: firstNonEmpty(stringValue(resultMap["summary"]), fmt.Sprintf("worker returned status %q", status)),
		}
	}
	fillSubprocessMetadata(&result, entry, iface, invocationID, protocol)
	return result
}

func firstPositiveDuration(fallback int64, values ...any) int64 {
	for _, value := range values {
		if n := int64FromAny(value); n > 0 {
			return n
		}
	}
	return fallback
}

func nestedValue(values map[string]any, key, nestedKey string) any {
	nested, ok := values[key].(map[string]any)
	if !ok {
		return nil
	}
	return nested[nestedKey]
}

func int64FromAny(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return 0
	}
}

func skillResultErrorFromAny(raw any) *SkillResultError {
	switch v := raw.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return &SkillResultError{Type: "WorkerError", Message: strings.TrimSpace(v)}
	case map[string]any:
		msg := firstNonEmpty(stringValue(v["message"]), stringValue(v["summary"]))
		if msg == "" {
			return nil
		}
		return &SkillResultError{
			Type:      firstNonEmpty(stringValue(v["type"]), "WorkerError"),
			Message:   msg,
			Traceback: stringValue(v["traceback"]),
		}
	default:
		return nil
	}
}

func subprocessErrorExecutionResult(status string, entry *SkillEntry, iface *Interface, invocationID string, durationMS int64, errType, msg string) SkillExecutionResult {
	result := errorExecutionResult(status, entry, iface, errType, msg)
	result.DurationMS = durationMS
	fillSubprocessMetadata(&result, entry, iface, invocationID, defaultSubprocessProto)
	return result
}

func fillSubprocessMetadata(result *SkillExecutionResult, entry *SkillEntry, iface *Interface, invocationID string, protocol string) {
	if entry != nil && entry.Spec != nil && result.Metadata.SkillID == "" {
		result.Metadata.SkillID = entry.Spec.SkillID
	}
	if iface != nil {
		if result.Metadata.Interface == "" {
			result.Metadata.Interface = iface.Name
		}
		if result.Metadata.Runtime == "" {
			result.Metadata.Runtime = strings.TrimSpace(iface.Transport.Runtime)
		}
	}
	if result.Metadata.InvocationID == "" {
		result.Metadata.InvocationID = invocationID
	}
	if result.Metadata.Protocol == "" {
		result.Metadata.Protocol = protocol
	}
}
