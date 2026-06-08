package filetools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stubChecker struct {
	lastPath string
	err      error
}

func (s *stubChecker) CheckPath(targetPath string) error {
	s.lastPath = targetPath
	return s.err
}

func TestToolsExposeExpectedNames(t *testing.T) {
	tools := Tools()
	if len(tools) != 3 {
		t.Fatalf("expected 3 file tools, got %d", len(tools))
	}
	if tools[0].Name != ReadFileToolName {
		t.Fatalf("unexpected read tool name: %s", tools[0].Name)
	}
	if tools[1].Name != ListDirToolName {
		t.Fatalf("unexpected list tool name: %s", tools[1].Name)
	}
	if tools[2].Name != WriteFileToolName {
		t.Fatalf("unexpected write tool name: %s", tools[2].Name)
	}
}

func TestExecuteWriteThenReadFile(t *testing.T) {
	workspace := t.TempDir()

	result, err := Execute(context.Background(), WriteFileToolName, map[string]any{
		"path":    "notes/todo.txt",
		"content": "ship it",
	}, workspace, nil)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if !strings.Contains(result, "created notes/todo.txt") {
		t.Fatalf("unexpected write result: %s", result)
	}

	readResult, err := Execute(context.Background(), ReadFileToolName, map[string]any{
		"path": "notes/todo.txt",
	}, workspace, nil)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if readResult != "ship it" {
		t.Fatalf("unexpected read content: %q", readResult)
	}
}

func TestExecuteListDirOrdersDirectoriesFirst(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "b-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "z.txt"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Execute(context.Background(), ListDirToolName, map[string]any{}, workspace, nil)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	lines := strings.Split(result, "\n")
	if len(lines) < 4 {
		t.Fatalf("unexpected list output: %q", result)
	}
	if lines[0] != "." {
		t.Fatalf("expected root header, got %q", lines[0])
	}
	if lines[1] != "b-dir/" {
		t.Fatalf("expected directory first, got %q", lines[1])
	}
	if lines[2] != "a.txt" || lines[3] != "z.txt" {
		t.Fatalf("unexpected file ordering: %q", result)
	}
}

func TestExecuteBlocksWorkspaceEscape(t *testing.T) {
	workspace := t.TempDir()

	_, err := Execute(context.Background(), ReadFileToolName, map[string]any{
		"path": "../secret.txt",
	}, workspace, nil)
	if err == nil {
		t.Fatal("expected workspace escape to be rejected")
	}
	if !strings.Contains(err.Error(), "workspace escape") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteUsesPathChecker(t *testing.T) {
	workspace := t.TempDir()
	checker := &stubChecker{}
	if err := os.WriteFile(filepath.Join(workspace, "allowed.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Execute(context.Background(), ReadFileToolName, map[string]any{
		"path": "allowed.txt",
	}, workspace, checker)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if checker.lastPath != "allowed.txt" {
		t.Fatalf("checker saw %q, want allowed.txt", checker.lastPath)
	}
}

func TestExecuteRejectsUnknownTool(t *testing.T) {
	workspace := t.TempDir()
	_, err := Execute(context.Background(), "NopeTool", nil, workspace, nil)
	if err == nil {
		t.Fatal("expected unknown tool error")
	}
}

func TestExecuteWriteFileRequiresContentField(t *testing.T) {
	workspace := t.TempDir()
	_, err := Execute(context.Background(), WriteFileToolName, map[string]any{
		"path": "notes/empty.txt",
	}, workspace, nil)
	if err == nil {
		t.Fatal("expected missing content to fail")
	}
	if !strings.Contains(err.Error(), "content is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
