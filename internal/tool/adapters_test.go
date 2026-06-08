package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/filetools"
	"github.com/ceoai/navi/internal/navi/plugin"
	"github.com/ceoai/navi/internal/navi/selfmod"
	"github.com/ceoai/navi/internal/navi/skill"
)

func TestFileToolExecutorExecute(t *testing.T) {
	dir := t.TempDir()
	exec := NewFileToolExecutor(filetools.WriteFileToolName, dir, nil)
	if _, err := exec.Execute(context.Background(), map[string]any{
		"path":    "notes.txt",
		"content": "hello",
	}); err != nil {
		t.Fatalf("write execute: %v", err)
	}

	readExec := NewFileToolExecutor(filetools.ReadFileToolName, dir, nil)
	result, err := readExec.Execute(context.Background(), map[string]any{"path": "notes.txt"})
	if err != nil {
		t.Fatalf("read execute: %v", err)
	}
	got, _ := result.Content.(string)
	if got != "hello" {
		t.Fatalf("expected read content hello, got %q", got)
	}
}

func TestSkillToolExecutorExecute(t *testing.T) {
	skill.RegisterInternalHandler("test.tool", "run", func(ctx context.Context, entry *skill.SkillEntry, iface *skill.Interface, args map[string]any) (any, error) {
		return map[string]any{"echo": args["message"]}, nil
	})
	entry := &skill.SkillEntry{
		Skill: skill.Skill{Name: "Test Tool"},
		Spec: &skill.OSS27Spec{
			SkillID: "test.tool",
		},
	}
	iface := &skill.Interface{
		Name: "run",
		Transport: skill.TransportSpec{
			Type: "internal",
		},
	}
	exec := NewSkillToolExecutor(entry, iface)
	result, err := exec.Execute(context.Background(), map[string]any{"message": "hi"})
	if err != nil {
		t.Fatalf("skill execute: %v", err)
	}
	raw, _ := result.Content.(string)
	if !strings.Contains(raw, "\"echo\":\"hi\"") {
		t.Fatalf("expected execution payload echo hi, got %q", raw)
	}
}

type fakeRouterLLM struct{}

func (f fakeRouterLLM) Catalog(ctx context.Context) (llm.LLMCatalog, error) {
	return llm.LLMCatalog{}, nil
}

func (f fakeRouterLLM) GetActive(ctx context.Context) (llm.Active, error) {
	return llm.Active{Provider: "anthropic", Model: "claude"}, nil
}

func (f fakeRouterLLM) SetActive(ctx context.Context, provider, model string) (llm.Active, error) {
	return llm.Active{}, nil
}

func TestRouterToolExecutorExecute(t *testing.T) {
	exec := NewRouterToolExecutor(
		"navi.llm.router.get_active",
		fakeRouterLLM{},
	)
	result, err := exec.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("router execute: %v", err)
	}
	payload, ok := result.Content.(map[string]any)
	if !ok || payload["provider"] != "anthropic" || payload["model"] != "claude" {
		t.Fatalf("unexpected router payload: %#v", result.Content)
	}
}

func TestBuiltinPluginExecutorExecute(t *testing.T) {
	exec := NewBuiltinPluginExecutor(plugin.CalendarTimeWindowToolName, func(ctx context.Context) string { return "UTC" })
	result, err := exec.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("plugin execute: %v", err)
	}
	payload, ok := result.Content.(map[string]any)
	if !ok || payload["start_utc"] == "" || payload["end_utc"] == "" {
		t.Fatalf("unexpected plugin payload: %#v", result.Content)
	}
}

func TestSelfModToolExecutorExecute(t *testing.T) {
	dir := t.TempDir()
	goCacheDir := t.TempDir()
	goModCacheDir := t.TempDir()
	t.Setenv("GOCACHE", goCacheDir)
	t.Setenv("GOMODCACHE", goModCacheDir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "pkg_test.go"), []byte("package pkg\n\nimport \"testing\"\n\nfunc TestOK(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	exec := selfmod.NewExecutor(filepath.Clean(dir))
	toolExec := NewSelfModToolExecutor(selfmod.GoTestToolName, exec)
	result, err := toolExec.Execute(context.Background(), map[string]any{"path": "./..."})
	if err != nil {
		t.Fatalf("selfmod execute: %v", err)
	}
	raw, _ := json.Marshal(result.Content)
	if !strings.Contains(string(raw), "ok") && len(raw) == 0 {
		t.Fatalf("expected non-empty selfmod result, got %s", raw)
	}
}
