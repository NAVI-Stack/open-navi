package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ceoai/navi/internal/sandbox"
	"github.com/ceoai/navi/internal/schema"
)

func TestExecuteSubprocessPython_WrapperSuccess(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	entry := makePythonSkill(t, root, `oss27_version: "1.0"
skill_id: "navi.test.echo"
semver: "1.0.0"
display:
  name: "echo-skill"
  description: "Echoes values."
interfaces:
  - name: "echo"
    transport:
      type: "subprocess_python"
      subprocess_python:
        entrypoint: "main.py"
        function: "echo"
    input_schema:
      type: "object"
    output_schema:
      type: "object"
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: []
python_runtime:
  venv_strategy: "shared"
  timeout_ms: 3000
governance:
  publisher: "navi.test"
  signed: false
`, `def echo(params):
    return {"echo": params.get("value")}
`)

	iface := entry.Spec.Interfaces[0]
	raw, err := Execute(context.Background(), entry, &iface, map[string]any{"value": "hello"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	result := decodeExecutionResult(t, raw)
	if result.Status != "success" {
		t.Fatalf("expected success, got %s (%+v)", result.Status, result.Error)
	}
	payload, ok := result.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", result.Payload)
	}
	if payload["echo"] != "hello" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestExecuteSubprocessPython_Timeout(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	entry := makePythonSkill(t, root, `oss27_version: "1.0"
skill_id: "navi.test.timeout"
semver: "1.0.0"
display:
  name: "timeout-skill"
  description: "Sleeps too long."
interfaces:
  - name: "sleep"
    transport:
      type: "subprocess_python"
      subprocess_python:
        entrypoint: "main.py"
        function: "sleepy"
    input_schema:
      type: "object"
    output_schema:
      type: "object"
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: []
python_runtime:
  venv_strategy: "shared"
  timeout_ms: 100
governance:
  publisher: "navi.test"
  signed: false
`, `import time

def sleepy(params):
    time.sleep(5)
    return {"ok": True}
`)

	iface := entry.Spec.Interfaces[0]
	raw, err := Execute(context.Background(), entry, &iface, map[string]any{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	result := decodeExecutionResult(t, raw)
	if result.Status != "timeout" {
		t.Fatalf("expected timeout, got %s (%+v)", result.Status, result.Error)
	}
}

func TestExecuteSubprocessPython_Exception(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := t.TempDir()
	entry := makePythonSkill(t, root, `oss27_version: "1.0"
skill_id: "navi.test.error"
semver: "1.0.0"
display:
  name: "error-skill"
  description: "Raises an error."
interfaces:
  - name: "boom"
    transport:
      type: "subprocess_python"
      subprocess_python:
        entrypoint: "main.py"
        function: "boom"
    input_schema:
      type: "object"
    output_schema:
      type: "object"
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: []
python_runtime:
  venv_strategy: "shared"
  timeout_ms: 3000
governance:
  publisher: "navi.test"
  signed: false
`, `def boom(params):
    raise ValueError("kaboom")
`)

	iface := entry.Spec.Interfaces[0]
	raw, err := Execute(context.Background(), entry, &iface, map[string]any{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	result := decodeExecutionResult(t, raw)
	if result.Status != "error" {
		t.Fatalf("expected error, got %s", result.Status)
	}
	if result.Error == nil || result.Error.Type != "ValueError" {
		t.Fatalf("expected ValueError, got %+v", result.Error)
	}
}

func TestExecuteSubprocessPython_HealthCheckFixture(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	skillDir := filepath.Join("testdata", "python-health-check")
	entry, err := loadSkillEntry(skillDir, filepath.Join(skillDir, "SKILL.yaml"))
	if err != nil {
		t.Fatalf("load python-health-check fixture: %v", err)
	}

	var iface *Interface
	for i := range entry.Spec.Interfaces {
		if entry.Spec.Interfaces[i].Name == "health_check" {
			iface = &entry.Spec.Interfaces[i]
			break
		}
	}
	if iface == nil {
		t.Fatal("health_check interface not found")
	}

	raw, err := Execute(context.Background(), entry, iface, map[string]any{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	result := decodeExecutionResult(t, raw)
	if result.Status != "success" {
		t.Fatalf("expected success, got %s (%+v)", result.Status, result.Error)
	}
	payload, ok := result.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", result.Payload)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected health payload: %#v", payload)
	}
}

func TestExecuteSubprocess_JSONRPCStdio(t *testing.T) {
	pythonExe, err := resolvePythonExecutable("")
	if err != nil {
		t.Skipf("python executable not available: %v", err)
	}

	root := t.TempDir()
	skillDir := filepath.Join(root, "generic-subprocess")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}

	spec := fmt.Sprintf(`oss27_version: "1.0"
skill_id: "navi.test.subprocess"
semver: "1.0.0"
display:
  name: "subprocess-skill"
  description: "Uses jsonrpc stdio."
interfaces:
  - name: "invoke_worker"
    transport:
      type: "subprocess"
      runtime: "python"
      command: ["%s", "worker.py"]
      protocol: "jsonrpc_stdio"
    input_schema:
      type: "object"
    output_schema:
      type: "object"
effects:
  side_effects: ["filesystem_write"]
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: ["package_registries_only"]
performance:
  timeout_ms: 3000
governance:
  publisher: "navi.test"
  signed: false
`, filepath.ToSlash(pythonExe))
	worker := `import json
import sys

request = json.load(sys.stdin)
params = request["params"]
print(json.dumps({"type": "log", "stream": "stdout", "data": "worker started"}))
print(json.dumps({
    "jsonrpc": "2.0",
    "id": request["id"],
    "result": {
        "status": "success",
        "output": {
            "skill_id": params["skill_id"],
            "interface": params["interface"],
            "value": params["input"]["value"],
            "policy": params["policy"],
        },
        "metrics": {"duration_ms": 9},
    },
}))
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "worker.py"), []byte(worker), 0o644); err != nil {
		t.Fatalf("write worker: %v", err)
	}

	entry, err := loadSkillEntry(skillDir, filepath.Join(skillDir, "SKILL.yaml"))
	if err != nil {
		t.Fatalf("load skill: %v", err)
	}
	iface := entry.Spec.Interfaces[0]
	raw, err := Execute(context.Background(), entry, &iface, map[string]any{"value": "hello"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	result := decodeExecutionResult(t, raw)
	if result.Status != "success" {
		t.Fatalf("expected success, got %s (%+v)", result.Status, result.Error)
	}
	if result.DurationMS != 9 {
		t.Fatalf("expected worker duration to be normalized, got %d", result.DurationMS)
	}
	if result.Metadata.Runtime != "python" || result.Metadata.Protocol != "jsonrpc_stdio" {
		t.Fatalf("unexpected metadata: %+v", result.Metadata)
	}
	payload, ok := result.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", result.Payload)
	}
	if payload["skill_id"] != "navi.test.subprocess" || payload["interface"] != "invoke_worker" || payload["value"] != "hello" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	policy, ok := payload["policy"].(map[string]any)
	if !ok {
		t.Fatalf("expected policy map, got %#v", payload["policy"])
	}
	if policy["network"] != "package_registries_only" || policy["filesystem"] != "workspace_rw" {
		t.Fatalf("unexpected policy: %#v", policy)
	}
}

func TestExecuteSubprocess_SandboxedJSONRPCStdio(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "repo")
	skillDir := filepath.Join(root, "sandboxed-subprocess")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	spec := `oss27_version: "1.0"
skill_id: "navi.test.sandboxed"
semver: "1.0.0"
display:
  name: "sandboxed-subprocess"
  description: "Uses sandboxed jsonrpc stdio."
interfaces:
  - name: "inspect"
    transport:
      type: "subprocess"
      runtime: "python"
      command: ["python", "/navi/skill/worker.py"]
      protocol: "jsonrpc_stdio"
      sandbox_profile: "test-python"
    input_schema:
      type: "object"
    output_schema:
      type: "object"
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: true
    network_egress: []
performance:
  timeout_ms: 3000
governance:
  publisher: "navi.test"
  signed: false
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	entry, err := loadSkillEntry(skillDir, filepath.Join(skillDir, "SKILL.yaml"))
	if err != nil {
		t.Fatalf("load skill: %v", err)
	}
	runner := &fakeSubprocessSandboxRunner{}
	ctx := WithExecutionContext(context.Background(), ExecutionContext{
		WorkspaceDir:  workspace,
		SandboxRunner: runner,
		SandboxProfileResolver: func(ctx context.Context, id string) (schema.SandboxProfile, error) {
			if id != "test-python" {
				t.Fatalf("profile id = %q, want test-python", id)
			}
			return schema.SandboxProfile{
				ID:                   id,
				Runtime:              schema.SandboxRuntimeDocker,
				Status:               schema.SandboxProfileStatusActive,
				Image:                "python:3.11",
				NetworkMode:          schema.SandboxNetworkNone,
				WorkspaceMountTarget: "/workspace",
				CommandAllowlist:     []string{"python"},
				CreatedBy:            "test",
			}, nil
		},
	})
	iface := entry.Spec.Interfaces[0]
	raw, err := Execute(ctx, entry, &iface, map[string]any{"value": "inside"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	result := decodeExecutionResult(t, raw)
	if result.Status != "success" {
		t.Fatalf("expected success, got %s (%+v)", result.Status, result.Error)
	}
	if runner.prepared.WorkspaceRoot != workspace {
		t.Fatalf("workspace root = %q, want %q", runner.prepared.WorkspaceRoot, workspace)
	}
	if len(runner.prepared.ExtraMounts) != 1 || runner.prepared.ExtraMounts[0].Target != sandboxSkillMountTarget || !runner.prepared.ExtraMounts[0].ReadOnly {
		t.Fatalf("unexpected extra mounts: %#v", runner.prepared.ExtraMounts)
	}
	payload, ok := result.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", result.Payload)
	}
	if payload["workspace"] != "/workspace" || payload["value"] != "inside" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

type fakeSubprocessSandboxRunner struct {
	prepared sandbox.PrepareRequest
}

func (r *fakeSubprocessSandboxRunner) Prepare(ctx context.Context, req sandbox.PrepareRequest) (sandbox.Handle, error) {
	r.prepared = req
	return sandbox.Handle{
		ID:            req.Profile.ID,
		Profile:       req.Profile,
		WorkspaceRoot: req.WorkspaceRoot,
		MountTarget:   req.Profile.WorkspaceMountTarget,
		ExtraMounts:   req.ExtraMounts,
	}, nil
}

func (r *fakeSubprocessSandboxRunner) Run(ctx context.Context, handle sandbox.Handle, cmd sandbox.Command) (sandbox.Result, error) {
	var req subprocessJSONRPCRequest
	if err := json.Unmarshal(cmd.Stdin, &req); err != nil {
		return sandbox.Result{}, err
	}
	response, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result": map[string]any{
			"status": "success",
			"output": map[string]any{
				"workspace": req.Params.Workspace,
				"value":     req.Params.Input["value"],
			},
			"metrics": map[string]any{"duration_ms": 7},
		},
	})
	return sandbox.Result{ExitCode: 0, Stdout: string(response), DurationMS: 7}, nil
}

func (r *fakeSubprocessSandboxRunner) Cleanup(ctx context.Context, handle sandbox.Handle) error {
	return nil
}

func TestCanFallbackToSystemPython(t *testing.T) {
	if !canFallbackToSystemPython(&PythonRuntimeSpec{}, "") {
		t.Fatal("expected empty runtime spec to allow system python fallback")
	}
	if canFallbackToSystemPython(&PythonRuntimeSpec{RequirementsFile: "requirements.txt"}, "requirements.txt") {
		t.Fatal("expected requirements file to block system python fallback")
	}
	if canFallbackToSystemPython(&PythonRuntimeSpec{Dependencies: []string{"requests"}}, "") {
		t.Fatal("expected dependencies to block system python fallback")
	}
}

func TestPythonExecutableReadyRejectsMissingFile(t *testing.T) {
	if pythonExecutableReady(filepath.Join(t.TempDir(), "missing-python.exe")) {
		t.Fatal("expected missing python executable to be rejected")
	}
}

func TestResolvedRequirementsFileIgnoresMissingDefault(t *testing.T) {
	if got := resolvedRequirementsFile(t.TempDir(), &PythonRuntimeSpec{RequirementsFile: "requirements.txt"}); got != "" {
		t.Fatalf("expected missing requirements file to resolve to empty path, got %q", got)
	}
}

func TestLoadSkillEntry_PythonRuntimeRequired(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "missing-runtime")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	spec := `oss27_version: "1.0"
skill_id: "navi.test.invalid"
semver: "1.0.0"
display:
  name: "invalid-skill"
  description: "Missing python runtime."
interfaces:
  - name: "run"
    transport:
      type: "subprocess_python"
      subprocess_python:
        entrypoint: "main.py"
    input_schema:
      type: "object"
    output_schema:
      type: "object"
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: []
governance:
  publisher: "navi.test"
  signed: false
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatalf("write SKILL.yaml: %v", err)
	}
	if _, err := loadSkillEntry(skillDir, filepath.Join(skillDir, "SKILL.yaml")); err == nil {
		t.Fatal("expected python runtime validation error")
	}
}

func makePythonSkill(t *testing.T, root, specYAML, mainPy string) *SkillEntry {
	t.Helper()
	skillsDir := filepath.Join(root, "skills")
	skillDir := filepath.Join(skillsDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	runtimeDir := filepath.Join(skillsDir, "_runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	runnerSrc := filepath.Join("..", "..", "..", "skills", "_runtime", "runner.py")
	runnerData, err := os.ReadFile(runnerSrc)
	if err != nil {
		t.Fatalf("read runtime runner: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "runner.py"), runnerData, 0o644); err != nil {
		t.Fatalf("write runtime runner: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), []byte(specYAML), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "main.py"), []byte(mainPy), 0o644); err != nil {
		t.Fatalf("write main.py: %v", err)
	}

	entry, err := loadSkillEntry(skillDir, filepath.Join(skillDir, "SKILL.yaml"))
	if err != nil {
		t.Fatalf("load skill: %v", err)
	}
	return entry
}

func decodeExecutionResult(t *testing.T, raw string) SkillExecutionResult {
	t.Helper()
	var result SkillExecutionResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("decode execution result: %v\nraw=%s", err, raw)
	}
	return result
}
