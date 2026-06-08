package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

const runtimeReadyMarker = ".navi-runtime-ready"

func executeSubprocessPython(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (string, error) {
	if entry.Spec == nil || entry.Spec.PythonRuntime == nil {
		return "", fmt.Errorf("subprocess_python transport requires python_runtime")
	}
	if iface.Transport.SubprocessPython == nil {
		return "", fmt.Errorf("subprocess_python transport missing entrypoint config")
	}

	rt := entry.Spec.PythonRuntime
	venvPath, err := resolveVenvPath(entry.Spec.SkillID, rt.VenvStrategy)
	if err != nil {
		return marshalExecutionResult(errorExecutionResult("error", entry, iface, "RuntimeError", err.Error()))
	}
	pythonExe, err := provisionVenv(venvPath, entry.Skill.BaseDir, rt)
	if err != nil {
		return marshalExecutionResult(errorExecutionResult("error", entry, iface, "ProvisionError", err.Error()))
	}

	entrypointPath := filepath.Join(entry.Skill.BaseDir, filepath.FromSlash(iface.Transport.SubprocessPython.Entrypoint))
	if !fileExists(entrypointPath) {
		return marshalExecutionResult(errorExecutionResult("error", entry, iface, "EntrypointError", "python entrypoint not found"))
	}

	invocationID := uuid.NewString()
	payload := args
	cmdArgs := []string{}
	functionName := strings.TrimSpace(iface.Transport.SubprocessPython.Function)
	if functionName == "" {
		functionName = "run"
	}
	if functionName == "run" {
		payload = map[string]any{
			"interface": iface.Name,
			"arguments": args,
		}
		cmdArgs = []string{entrypointPath}
	} else {
		wrapperPath, err := runtimeWrapperPath(entry.Skill.BaseDir)
		if err != nil {
			return marshalExecutionResult(errorExecutionResult("error", entry, iface, "RuntimeError", err.Error()))
		}
		cmdArgs = []string{wrapperPath, entrypointPath, functionName}
	}

	input, err := json.Marshal(payload)
	if err != nil {
		return marshalExecutionResult(errorExecutionResult("error", entry, iface, "MarshalError", err.Error()))
	}
	if len(input) > rt.MaxInputBytes {
		return marshalExecutionResult(errorExecutionResult("error", entry, iface, "InputTooLarge", fmt.Sprintf("input exceeds %d bytes", rt.MaxInputBytes)))
	}

	timeout := time.Duration(rt.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(defaultPythonTimeoutMS) * time.Millisecond
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, pythonExe, cmdArgs...)
	cmd.Dir = entry.Skill.BaseDir
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1", "NAVI_SKILL_INVOCATION_ID="+invocationID)
	if execMeta, ok := ExecutionContextFromContext(ctx); ok && strings.TrimSpace(execMeta.WorkspaceDir) != "" {
		cmd.Env = append(cmd.Env, "NAVI_WORKSPACE_DIR="+absoluteWorkspaceDir(execMeta.WorkspaceDir))
	}
	cmd.Stdin = bytes.NewReader(input)

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	start := time.Now()
	runErr := cmd.Run()
	durationMS := time.Since(start).Milliseconds()
	if runCtx.Err() == context.DeadlineExceeded {
		return marshalExecutionResult(timeoutExecutionResult(entry, iface, invocationID, durationMS, pythonExe))
	}

	rawStdout := bytes.TrimSpace(stdoutBuf.Bytes())
	if len(rawStdout) > rt.MaxOutputBytes {
		return marshalExecutionResult(truncatedExecutionResult(entry, iface, invocationID, durationMS, pythonExe, string(rawStdout[:rt.MaxOutputBytes])))
	}

	if len(rawStdout) == 0 {
		msg := strings.TrimSpace(stderrBuf.String())
		if msg == "" && runErr != nil {
			msg = runErr.Error()
		}
		if msg == "" {
			msg = "python skill produced no output"
		}
		return marshalExecutionResult(errorExecutionResultWithMetadata("error", entry, iface, invocationID, durationMS, pythonExe, "EmptyOutput", msg))
	}

	var result SkillResult
	if err := json.Unmarshal(rawStdout, &result); err != nil {
		msg := fmt.Sprintf("invalid skill result envelope: %v", err)
		stderr := strings.TrimSpace(stderrBuf.String())
		if stderr != "" {
			msg += ": " + stderr
		}
		return marshalExecutionResult(errorExecutionResultWithMetadata("error", entry, iface, invocationID, durationMS, pythonExe, "DecodeError", msg))
	}
	if result.Status == "" {
		if result.Error != nil {
			result.Status = SkillResultStatusError
		} else {
			result.Status = SkillResultSuccess
		}
	}
	if result.DurationMS <= 0 {
		result.DurationMS = durationMS
	}
	if result.Metadata.SkillID == "" {
		result.Metadata.SkillID = entry.Spec.SkillID
	}
	if result.Metadata.Interface == "" {
		result.Metadata.Interface = iface.Name
	}
	if result.Metadata.InvocationID == "" {
		result.Metadata.InvocationID = invocationID
	}
	if result.Metadata.PythonVersion == "" {
		result.Metadata.PythonVersion = detectPythonVersion(pythonExe)
	}

	execResult := SkillExecutionResult{
		Status:     string(result.Status),
		Payload:    result.Output,
		Error:      result.Error,
		DurationMS: result.DurationMS,
		Metadata:   result.Metadata,
	}
	if runErr != nil && execResult.Error == nil && execResult.Status == string(SkillResultSuccess) {
		execResult.Status = "error"
		execResult.Error = &SkillResultError{
			Type:    "ProcessError",
			Message: runErr.Error(),
		}
	}
	return marshalExecutionResult(execResult)
}

func resolveVenvPath(skillID, strategy string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	root := filepath.Join(home, ".navi", "venvs")
	key := "shared"
	switch strategy {
	case "", "shared":
		key = "shared"
	case "per_category":
		key = skillID
		if idx := strings.Index(skillID, "."); idx > 0 {
			key = skillID[:idx]
		}
	case "per_skill":
		key = strings.ReplaceAll(skillID, ".", "_")
	default:
		return "", fmt.Errorf("unsupported venv strategy %q", strategy)
	}
	return filepath.Join(root, key), nil
}

func provisionVenv(venvPath, skillBaseDir string, rt *PythonRuntimeSpec) (string, error) {
	if err := os.MkdirAll(filepath.Dir(venvPath), 0o755); err != nil {
		return "", fmt.Errorf("create venv parent: %w", err)
	}

	basePython, err := resolvePythonExecutable("")
	if err != nil {
		return "", err
	}
	reqFile := resolvedRequirementsFile(skillBaseDir, rt)
	if canFallbackToSystemPython(rt, reqFile) {
		return basePython, nil
	}

	pythonExe := venvPythonPath(venvPath)
	if !fileExists(pythonExe) {
		cmd := exec.Command(basePython, "-m", "venv", venvPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("create venv: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}

	if !pythonExecutableReady(pythonExe) {
		return "", fmt.Errorf("venv python is not runnable: %s", pythonExe)
	}

	markerPath := filepath.Join(venvPath, runtimeReadyMarker)
	if fileExists(markerPath) {
		return pythonExe, nil
	}

	if reqFile != "" {
		cmd := exec.Command(pythonExe, "-m", "pip", "install", "-r", reqFile)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("install requirements: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}
	if len(rt.Dependencies) > 0 {
		args := append([]string{"-m", "pip", "install"}, rt.Dependencies...)
		cmd := exec.Command(pythonExe, args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("install dependencies: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}
	if err := os.WriteFile(markerPath, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
		return "", fmt.Errorf("write runtime marker: %w", err)
	}
	return pythonExe, nil
}

func canFallbackToSystemPython(rt *PythonRuntimeSpec, reqFile string) bool {
	if rt == nil {
		return false
	}
	return reqFile == "" && len(rt.Dependencies) == 0
}

func resolvedRequirementsFile(skillBaseDir string, rt *PythonRuntimeSpec) string {
	if rt == nil || strings.TrimSpace(rt.RequirementsFile) == "" {
		return ""
	}
	reqFile := filepath.Join(skillBaseDir, filepath.FromSlash(rt.RequirementsFile))
	if !fileExists(reqFile) {
		return ""
	}
	return reqFile
}

func runtimeWrapperPath(skillBaseDir string) (string, error) {
	candidates := []string{
		filepath.Join(filepath.Dir(skillBaseDir), "_runtime", "runner.py"),
		filepath.Join("skills", "_runtime", "runner.py"),
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return "", err
			}
			return abs, nil
		}
	}
	return "", fmt.Errorf("python runtime wrapper not found")
}

func venvPythonPath(venvPath string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvPath, "Scripts", "python.exe")
	}
	return filepath.Join(venvPath, "bin", "python")
}

func detectPythonVersion(pythonExe string) string {
	out, err := exec.Command(pythonExe, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	return extractPythonVersion(string(out))
}

func pythonExecutableReady(pythonExe string) bool {
	if !fileExists(pythonExe) {
		return false
	}
	cmd := exec.Command(pythonExe, "--version")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func timeoutExecutionResult(entry *SkillEntry, iface *Interface, invocationID string, durationMS int64, pythonExe string) SkillExecutionResult {
	return errorExecutionResultWithMetadata("timeout", entry, iface, invocationID, durationMS, pythonExe, "TimeoutError", "execution timeout")
}

func truncatedExecutionResult(entry *SkillEntry, iface *Interface, invocationID string, durationMS int64, pythonExe, partial string) SkillExecutionResult {
	result := errorExecutionResultWithMetadata("truncated", entry, iface, invocationID, durationMS, pythonExe, "OutputTruncated", "output truncated")
	result.Payload = partial
	return result
}

func errorExecutionResultWithMetadata(status string, entry *SkillEntry, iface *Interface, invocationID string, durationMS int64, pythonExe, errType, msg string) SkillExecutionResult {
	result := errorExecutionResult(status, entry, iface, errType, msg)
	result.DurationMS = durationMS
	result.Metadata.InvocationID = invocationID
	result.Metadata.PythonVersion = detectPythonVersion(pythonExe)
	return result
}
