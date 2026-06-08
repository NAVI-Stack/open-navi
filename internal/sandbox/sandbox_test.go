package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestDockerRunnerPrepareRejectsMissingDocker(t *testing.T) {
	runner := NewDockerRunner()
	runner.lookPath = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}
	_, err := runner.Prepare(context.Background(), PrepareRequest{
		Profile:       testSandboxProfile(),
		WorkspaceRoot: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "docker executable is unavailable") {
		t.Fatalf("expected docker unavailable error, got %v", err)
	}
}

func TestDockerRunnerRunBuildsNetworkOffDockerCommand(t *testing.T) {
	runner := NewDockerRunner()
	runner.lookPath = func(file string) (string, error) { return file, nil }
	var capturedExecutable string
	var capturedArgs []string
	var capturedStdin []byte
	runner.runCommand = func(ctx context.Context, executable string, args []string, stdin []byte, timeout time.Duration) (commandResult, error) {
		capturedExecutable = executable
		capturedArgs = append([]string{}, args...)
		capturedStdin = append([]byte{}, stdin...)
		return commandResult{stdout: "ok\n", stderr: "", exitCode: 0}, nil
	}
	skillDir := t.TempDir()
	handle, err := runner.Prepare(context.Background(), PrepareRequest{
		Profile:       testSandboxProfile(),
		WorkspaceRoot: t.TempDir(),
		RunID:         "run-1",
		ExtraMounts: []Mount{{
			Source:   skillDir,
			Target:   "/navi/skill",
			ReadOnly: true,
		}},
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	result, err := runner.Run(context.Background(), handle, Command{
		Args:      []string{"go", "test", "./..."},
		Workdir:   ".",
		Env:       map[string]string{"CI": "1"},
		TimeoutMS: 1000,
		Stdin:     []byte("payload\n"),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedExecutable != "docker" {
		t.Fatalf("executable = %q, want docker", capturedExecutable)
	}
	joined := strings.Join(capturedArgs, " ")
	for _, want := range []string{"run", "--rm", "-i", "--network none", "type=bind,source=" + handle.WorkspaceRoot + ",target=/workspace", "type=bind,source=" + filepath.Clean(skillDir) + ",target=/navi/skill,readonly", "-w /workspace", "-e CI=1", "golang:1.22", "go test ./..."} {
		if !strings.Contains(joined, want) {
			t.Fatalf("docker args missing %q: %s", want, joined)
		}
	}
	if string(capturedStdin) != "payload\n" {
		t.Fatalf("stdin = %q", capturedStdin)
	}
	if result.Verdict != "passed" || result.Stdout != "ok\n" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestDockerRunnerRunUsesDockerExecWhenContainerConfigured(t *testing.T) {
	t.Setenv("NAVI_SANDBOX_CONTAINER_NAME", "navid")

	runner := NewDockerRunner()
	runner.lookPath = func(file string) (string, error) { return file, nil }
	var capturedExecutable string
	var capturedArgs []string
	root := t.TempDir()
	workdir := filepath.Join(root, "pkg")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}
	runner.runCommand = func(ctx context.Context, executable string, args []string, stdin []byte, timeout time.Duration) (commandResult, error) {
		capturedExecutable = executable
		capturedArgs = append([]string{}, args...)
		return commandResult{stdout: "ok\n", stderr: "", exitCode: 0}, nil
	}

	handle, err := runner.Prepare(context.Background(), PrepareRequest{
		Profile:       testSandboxProfile(),
		WorkspaceRoot: root,
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if handle.ExecContainer != "navid" {
		t.Fatalf("ExecContainer = %q, want navid", handle.ExecContainer)
	}

	result, err := runner.Run(context.Background(), handle, Command{
		Args:    []string{"go", "test", "./..."},
		Workdir: "pkg",
		Env:     map[string]string{"CI": "1"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedExecutable != "docker" {
		t.Fatalf("executable = %q, want docker", capturedExecutable)
	}
	joined := strings.Join(capturedArgs, " ")
	for _, want := range []string{"exec", "-w " + filepath.Clean(workdir), "-e CI=1", "navid", "go test ./..."} {
		if !strings.Contains(joined, want) {
			t.Fatalf("docker exec args missing %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "--mount") {
		t.Fatalf("docker exec path should not mount workspace: %s", joined)
	}
	if result.Verdict != "passed" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestDockerRunnerRunRejectsNonAllowlistedCommand(t *testing.T) {
	runner := NewDockerRunner()
	profile := testSandboxProfile()
	handle := Handle{Profile: profile, WorkspaceRoot: filepath.Clean(t.TempDir()), MountTarget: "/workspace"}
	_, err := runner.Run(context.Background(), handle, Command{Args: []string{"git", "status"}})
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("expected allowlist error, got %v", err)
	}
}

func TestDockerRunnerRunRejectsUnapprovedEnv(t *testing.T) {
	runner := NewDockerRunner()
	profile := testSandboxProfile()
	handle := Handle{Profile: profile, WorkspaceRoot: filepath.Clean(t.TempDir()), MountTarget: "/workspace"}
	_, err := runner.Run(context.Background(), handle, Command{
		Args: []string{"go", "test", "./..."},
		Env:  map[string]string{"SECRET_TOKEN": "nope"},
	})
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("expected env allowlist error, got %v", err)
	}
}

func TestDockerRunnerRunReturnsTimedOutVerdict(t *testing.T) {
	runner := NewDockerRunner()
	runner.runCommand = func(ctx context.Context, executable string, args []string, stdin []byte, timeout time.Duration) (commandResult, error) {
		return commandResult{stdout: "partial", exitCode: -1, timedOut: true}, context.DeadlineExceeded
	}
	handle := Handle{Profile: testSandboxProfile(), WorkspaceRoot: filepath.Clean(t.TempDir()), MountTarget: "/workspace"}
	result, err := runner.Run(context.Background(), handle, Command{Args: []string{"go", "test", "./..."}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.TimedOut || result.Verdict != "timed_out" {
		t.Fatalf("expected timed_out result, got %#v", result)
	}
}

func testSandboxProfile() schema.SandboxProfile {
	return schema.SandboxProfile{
		ID:                   "local-docker-default",
		Name:                 "Local Docker Default",
		Runtime:              schema.SandboxRuntimeDocker,
		Status:               schema.SandboxProfileStatusActive,
		Image:                "golang:1.22",
		NetworkMode:          schema.SandboxNetworkNone,
		WorkspaceMountTarget: "/workspace",
		CommandAllowlist:     []string{"go", "npm"},
		EnvAllowlist:         []string{"CI"},
		DefaultTimeoutMS:     30000,
		MaxTimeoutMS:         120000,
		CreatedBy:            "owner-1",
	}
}
