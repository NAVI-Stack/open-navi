package navi

import (
	"context"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/skill"
)

// stubProvider implements llm.Provider for testing.
type stubProvider struct{ name string }

func (s stubProvider) Chat(ctx context.Context, model string, msgs []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
	return &llm.Response{Content: "ok"}, nil
}
func (s stubProvider) Name() string { return s.name }

func newTestLoop(overrides func(*LoopConfig)) *AgentLoop {
	pe := NewExperienceManager("", ExperienceModeStandard)
	reg := skill.NewRegistry("")
	cfg := LoopConfig{
		ExperienceManager: pe,
		Skills:            reg,
		LLM:               stubProvider{name: "ollama"},
		Model:             "llama3.1:latest",
	}
	if overrides != nil {
		overrides(&cfg)
	}
	return NewAgentLoop(cfg)
}

func TestBuildIntrospectionBlock_BasicFields(t *testing.T) {
	loop := newTestLoop(nil)
	ctx := context.Background()

	block := loop.buildIntrospectionBlock(ctx, "test-session-123", 5)

	// Must contain agent identity
	if !strings.Contains(block, "Agent: NAVI") {
		t.Error("missing agent identity")
	}
	// Must contain chat info
	if !strings.Contains(block, "test-session-123") {
		t.Error("missing chat ID")
	}
	if !strings.Contains(block, "messages: 5") {
		t.Error("missing message count")
	}
	// Must contain snapshot timestamp
	if !strings.Contains(block, "Snapshot:") {
		t.Error("missing snapshot timestamp")
	}
	// Must contain grounding rules
	if !strings.Contains(block, "Grounding Rules") {
		t.Error("missing grounding rules section")
	}
	if !strings.Contains(block, "fabrication") {
		t.Error("missing anti-hallucination rule")
	}
}

func TestBuildIntrospectionBlock_WithGetActiveLLM(t *testing.T) {
	loop := newTestLoop(func(cfg *LoopConfig) {
		cfg.GetActiveLLM = func(ctx context.Context) (string, string, error) {
			return "ollama", "qwen3.5:latest", nil
		}
	})
	ctx := context.Background()

	block := loop.buildIntrospectionBlock(ctx, "s1", 0)

	if !strings.Contains(block, "ollama/qwen3.5:latest") {
		t.Errorf("expected active LLM from callback, got:\n%s", block)
	}
}

func TestBuildIntrospectionBlock_FallbackLLM(t *testing.T) {
	loop := newTestLoop(nil) // no GetActiveLLM callback
	ctx := context.Background()

	block := loop.buildIntrospectionBlock(ctx, "s1", 0)

	// Should fall back to provider.Name() / cfg.Model
	if !strings.Contains(block, "ollama/llama3.1:latest") {
		t.Errorf("expected fallback LLM, got:\n%s", block)
	}
}

func TestBuildIntrospectionBlock_WithGovernor(t *testing.T) {
	gov := governor.NewGovernor(governor.GovernorConfig{
		MaxActionBudget:    1000,
		MaxRetries:         5,
		CostCeiling:        50.0,
		AutonomousDuration: 72 * 60 * 60 * 1e9, // 72h in ns as time.Duration
	}, ".")

	loop := newTestLoop(func(cfg *LoopConfig) {
		cfg.Governor = gov
	})
	ctx := context.Background()

	block := loop.buildIntrospectionBlock(ctx, "s1", 0)

	if !strings.Contains(block, "Governor: active") {
		t.Errorf("expected active governor, got:\n%s", block)
	}
	if !strings.Contains(block, "budget 0/1000") {
		t.Errorf("expected budget 0/1000, got:\n%s", block)
	}
	if !strings.Contains(block, "cost $0.00/$50.00") {
		t.Errorf("expected cost info, got:\n%s", block)
	}
}

func TestBuildIntrospectionBlock_NilGovernor(t *testing.T) {
	loop := newTestLoop(nil) // no governor
	ctx := context.Background()

	block := loop.buildIntrospectionBlock(ctx, "s1", 0)

	if !strings.Contains(block, "Governor: not configured") {
		t.Errorf("expected 'not configured' governor, got:\n%s", block)
	}
}

func TestBuildIntrospectionBlock_WithConnectors(t *testing.T) {
	loop := newTestLoop(func(cfg *LoopConfig) {
		cfg.ConnectorHealth = func() []ConnectorHealthInfo {
			return []ConnectorHealthInfo{
				{Name: "telegram", Status: "running"},
				{Name: "slack", Status: "degraded"},
			}
		}
	})
	ctx := context.Background()

	block := loop.buildIntrospectionBlock(ctx, "s1", 0)

	if !strings.Contains(block, "telegram (running)") {
		t.Errorf("expected telegram connector, got:\n%s", block)
	}
	if !strings.Contains(block, "slack (degraded)") {
		t.Errorf("expected slack connector, got:\n%s", block)
	}
}

func TestBuildIntrospectionBlock_NoConnectors(t *testing.T) {
	loop := newTestLoop(nil) // no ConnectorHealth callback
	ctx := context.Background()

	block := loop.buildIntrospectionBlock(ctx, "s1", 0)

	// Should not contain "Connectors:" line when callback is nil
	if strings.Contains(block, "Connectors:") {
		t.Error("should not include connector line when callback is nil")
	}
}

func TestStalenessWarning_ShortConversation(t *testing.T) {
	warning := stalenessWarning(5)
	if warning != "" {
		t.Errorf("expected no warning for short conversation, got: %s", warning)
	}
}

func TestStalenessWarning_LongConversation(t *testing.T) {
	warning := stalenessWarning(15)
	if warning == "" {
		t.Error("expected warning for long conversation")
	}
	if !strings.Contains(warning, "15 messages") {
		t.Error("warning should mention message count")
	}
	if !strings.Contains(warning, "stale") {
		t.Error("warning should mention staleness")
	}
}

func TestStalenessWarning_Boundary(t *testing.T) {
	if stalenessWarning(10) != "" {
		t.Error("should not warn at exactly 10 messages")
	}
	if stalenessWarning(11) == "" {
		t.Error("should warn at 11 messages")
	}
}
