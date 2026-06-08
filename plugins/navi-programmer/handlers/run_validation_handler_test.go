package handlers

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coresandbox "github.com/ceoai/navi/internal/sandbox"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

func TestRunValidationHandlerUsesCoreSandboxRunner(t *testing.T) {
	ctx := context.Background()
	db := openRunValidationTestDB(t)
	profile := runValidationTestProfile("local-validation-network-disabled")
	if err := store.SaveSandboxProfile(ctx, db, profile); err != nil {
		t.Fatalf("SaveSandboxProfile: %v", err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}

	fake := &fakeValidationSandboxRunner{
		runResult: coresandbox.Result{
			SandboxProfileID: profile.ID,
			Runtime:          string(profile.Runtime),
			NetworkMode:      string(profile.NetworkMode),
			Command:          []string{"go", "test", "./..."},
			CommandDisplay:   "go test ./...",
			ExitCode:         0,
			Stdout:           "ok\n",
			DurationMS:       42,
		},
	}
	handler := &runValidationHandler{cfg: ProgrammerHandlerConfig{
		DB:     db,
		Runner: fake,
	}}

	payload, err := handler.runCommand(ctx, nil, nil, map[string]any{
		"root":            root,
		"cwd":             "pkg",
		"command":         []any{"go", "test", "./..."},
		"validation_kind": "test",
		"env":             map[string]any{"CI": "1"},
	})
	if err != nil {
		t.Fatalf("runCommand: %v", err)
	}
	result := payload.(map[string]any)
	if result["verdict"] != "passed" || result["exit_code"] != 0 {
		t.Fatalf("unexpected validation result: %#v", result)
	}
	if result["stdout"] != "ok\n" || result["sandbox_profile_id"] != profile.ID {
		t.Fatalf("missing sandbox evidence: %#v", result)
	}
	if fake.prepareReq.WorkspaceRoot != root {
		t.Fatalf("workspace root = %q, want %q", fake.prepareReq.WorkspaceRoot, root)
	}
	if strings.Join(fake.runCommand.Args, " ") != "go test ./..." {
		t.Fatalf("command = %#v", fake.runCommand.Args)
	}
	if fake.runCommand.Workdir != "pkg" {
		t.Fatalf("workdir = %q, want pkg", fake.runCommand.Workdir)
	}
	if fake.runCommand.Env["CI"] != "1" {
		t.Fatalf("env not passed to sandbox runner: %#v", fake.runCommand.Env)
	}
}

func TestRunValidationHandlerReturnsNotRunWhenSandboxIsUnavailable(t *testing.T) {
	ctx := context.Background()
	db := openRunValidationTestDB(t)
	profile := runValidationTestProfile("local-validation-network-disabled")
	if err := store.SaveSandboxProfile(ctx, db, profile); err != nil {
		t.Fatalf("SaveSandboxProfile: %v", err)
	}
	fake := &fakeValidationSandboxRunner{
		prepareErr: &coresandbox.Error{Code: "docker_unavailable", Message: "sandbox: docker executable is unavailable"},
	}
	handler := &runValidationHandler{cfg: ProgrammerHandlerConfig{
		DB:     db,
		Runner: fake,
	}}

	payload, err := handler.runCommand(ctx, nil, nil, map[string]any{
		"root":    t.TempDir(),
		"command": []any{"go", "test", "./..."},
	})
	if err != nil {
		t.Fatalf("runCommand: %v", err)
	}
	result := payload.(map[string]any)
	if result["verdict"] != "not_run" || result["blocked_by"] != "docker_unavailable" {
		t.Fatalf("expected sandbox not_run evidence, got %#v", result)
	}
}

func TestRunValidationHandlerRejectsNetworkishPackageCommand(t *testing.T) {
	handler := &runValidationHandler{cfg: ProgrammerHandlerConfig{
		DefaultProfile: runValidationTestProfile("local-validation-network-disabled"),
		Runner:         &fakeValidationSandboxRunner{},
	}}
	_, err := handler.runCommand(context.Background(), nil, nil, map[string]any{
		"root":    t.TempDir(),
		"command": []any{"npm", "install"},
	})
	if err == nil || !strings.Contains(err.Error(), "not a validation command") {
		t.Fatalf("expected validation policy error, got %v", err)
	}
}

func TestRunValidationHandlerRecordNotRun(t *testing.T) {
	handler := &runValidationHandler{}
	payload, err := handler.recordNotRun(context.Background(), nil, nil, map[string]any{
		"root":                t.TempDir(),
		"reason":              "no relevant validation target",
		"blocked_by":          "docs_only",
		"recommended_command": []any{"go", "test", "./..."},
	})
	if err != nil {
		t.Fatalf("recordNotRun: %v", err)
	}
	result := payload.(map[string]any)
	if result["verdict"] != "not_run" || result["blocked_by"] != "docs_only" {
		t.Fatalf("unexpected record_not_run output: %#v", result)
	}
}

func openRunValidationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "navi.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.CreateTables(context.Background(), db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	return db
}

func runValidationTestProfile(id string) schema.SandboxProfile {
	return schema.SandboxProfile{
		ID:                   id,
		Name:                 "Local Validation",
		Runtime:              schema.SandboxRuntimeDocker,
		Status:               schema.SandboxProfileStatusActive,
		Image:                "golang:1.22",
		NetworkMode:          schema.SandboxNetworkNone,
		WorkspaceMountTarget: "/workspace",
		CommandAllowlist:     []string{"go", "npm"},
		EnvAllowlist:         []string{"CI", "NAVI_VALIDATION", "NO_COLOR"},
		DefaultTimeoutMS:     30000,
		MaxTimeoutMS:         120000,
		CreatedBy:            "owner-1",
	}
}

type fakeValidationSandboxRunner struct {
	prepareReq coresandbox.PrepareRequest
	runCommand coresandbox.Command
	prepareErr error
	runErr     error
	runResult  coresandbox.Result
}

func (f *fakeValidationSandboxRunner) Prepare(_ context.Context, req coresandbox.PrepareRequest) (coresandbox.Handle, error) {
	f.prepareReq = req
	if f.prepareErr != nil {
		return coresandbox.Handle{}, f.prepareErr
	}
	return coresandbox.Handle{
		ID:            req.RunID,
		Profile:       req.Profile,
		WorkspaceRoot: req.WorkspaceRoot,
		MountTarget:   req.Profile.WorkspaceMountTarget,
	}, nil
}

func (f *fakeValidationSandboxRunner) Run(_ context.Context, _ coresandbox.Handle, cmd coresandbox.Command) (coresandbox.Result, error) {
	f.runCommand = cmd
	if f.runErr != nil {
		return coresandbox.Result{}, f.runErr
	}
	if f.runResult.SandboxProfileID == "" {
		f.runResult.SandboxProfileID = "local-validation-network-disabled"
	}
	return f.runResult, nil
}

func (f *fakeValidationSandboxRunner) Cleanup(context.Context, coresandbox.Handle) error {
	return nil
}
