package navi

import (
	"context"
	"testing"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/orchestration"
	naviruntime "github.com/ceoai/navi/internal/runtime"
)

func TestBuildCanonicalRequestBaseSuppressesToolsForPlainReply(t *testing.T) {
	loop := &AgentLoop{}

	req, err := loop.buildCanonicalRequestBaseForSurface(
		context.Background(),
		nil,
		ExperienceModeStandard,
		"Reply with exactly: pong",
		nil,
		runtimeToolSurface,
		orchestration.ExecutionModeRunExecute,
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if req.RequiredOutput.AllowToolCalls {
		t.Fatalf("plain reply should not expose runtime tools: %+v", req.RequiredOutput)
	}
	if req.RequiredOutput.AllowScheduledReplies {
		t.Fatalf("plain reply should not expose scheduled replies: %+v", req.RequiredOutput)
	}
}

func TestBuildCanonicalRequestBaseAllowsToolsForActionIntent(t *testing.T) {
	loop := &AgentLoop{}

	req, err := loop.buildCanonicalRequestBaseForSurface(
		context.Background(),
		nil,
		ExperienceModeStandard,
		"List the files in my workspace",
		nil,
		runtimeToolSurface,
		orchestration.ExecutionModeRunExecute,
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if !req.RequiredOutput.AllowToolCalls {
		t.Fatalf("action-like request should expose runtime tools: %+v", req.RequiredOutput)
	}
	if !req.RequiredOutput.AllowScheduledReplies {
		t.Fatalf("action-like request should preserve scheduled reply support: %+v", req.RequiredOutput)
	}
}

func TestBuildCanonicalRequestBaseAllowsToolsForRenderIntent(t *testing.T) {
	loop := &AgentLoop{}

	req, err := loop.buildCanonicalRequestBaseForSurface(
		context.Background(),
		nil,
		ExperienceModeStandard,
		"Show me a graph of token usage lately",
		nil,
		runtimeToolSurface,
		orchestration.ExecutionModeRunExecute,
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if !req.RequiredOutput.AllowToolCalls {
		t.Fatalf("render-like request should expose runtime tools: %+v", req.RequiredOutput)
	}
	if !req.RequiredOutput.AllowScheduledReplies {
		t.Fatalf("render-like request should preserve scheduled reply support: %+v", req.RequiredOutput)
	}
}

func TestRuntimeRequestRequiresReliableToolsForRenderIntent(t *testing.T) {
	for _, input := range []string{
		"Show me a graph of tool usage in this chat",
		"Render the data view now",
		"Use navi.render.visualize for a chart",
		"OpenUI visualization please",
	} {
		if !runtimeRequestRequiresReliableTools(input) {
			t.Fatalf("runtimeRequestRequiresReliableTools(%q) = false, want true", input)
		}
	}
}

func TestBuildCanonicalRequestBaseAllowsToolsForCheckpointResume(t *testing.T) {
	loop := &AgentLoop{}

	req, err := loop.buildCanonicalRequestBaseForSurface(
		context.Background(),
		nil,
		ExperienceModeStandard,
		"continue",
		&naviruntime.RunState{LatestCheckpointID: "checkpoint-1"},
		runtimeToolSurface,
		orchestration.ExecutionModeRunExecute,
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if !req.RequiredOutput.AllowToolCalls {
		t.Fatalf("checkpoint resume should preserve runtime tools: %+v", req.RequiredOutput)
	}
}

func TestCompiledModelRequestTraceStatsIncludesPromptBudget(t *testing.T) {
	think := false
	stats := compiledModelRequestTraceStats(orchestration.CompiledModelRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "core"},
			{Role: "user", Content: "hello"},
		},
		Tools: []llm.ToolDefinition{{
			Name:        "read_file",
			Description: "Read a file.",
			Parameters:  map[string]any{"type": "object"},
		}},
		Options: llm.Options{
			MaxTokens:     512,
			Temperature:   0.4,
			RepeatPenalty: 1.2,
			RepeatLastN:   128,
			Think:         &think,
		},
	})

	if stats["message_chars"] != "9" {
		t.Fatalf("expected total message chars, got %+v", stats)
	}
	if stats["system_chars"] != "4" {
		t.Fatalf("expected system chars, got %+v", stats)
	}
	if stats["user_chars"] != "5" {
		t.Fatalf("expected user chars, got %+v", stats)
	}
	if stats["max_tokens"] != "512" {
		t.Fatalf("expected max token budget, got %+v", stats)
	}
	if stats["think"] != "false" {
		t.Fatalf("expected think option in trace stats, got %+v", stats)
	}
	if stats["tool_schema_chars"] == "" || stats["tool_schema_chars"] == "0" {
		t.Fatalf("expected tool schema char count, got %+v", stats)
	}
}
