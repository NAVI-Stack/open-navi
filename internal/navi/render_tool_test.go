package navi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/inference"
	"github.com/open-navi/navi/internal/navi/render"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
	navitool "github.com/open-navi/navi/internal/tool"
)

func renderTestThread() *ChatThread {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	return &ChatThread{
		Messages: []ChatMessage{
			{Role: "user", Content: "read the file", CreatedAt: now},
			{
				Role:      "assistant",
				Content:   "done",
				CreatedAt: now.Add(time.Minute),
				Metadata: map[string]any{
					"toolParts": []any{
						map[string]any{"toolName": "read_file", "state": "result", "isError": false},
						map[string]any{"toolName": "read_file", "state": "result", "isError": false},
						map[string]any{"toolName": "list_dir", "state": "result", "isError": true},
					},
				},
			},
		},
	}
}

func TestRenderToolExecutor_AttachesPayloadToSink(t *testing.T) {
	cfg := LoopConfig{Chats: &mockChatStore{thread: renderTestThread()}}
	exec := newRenderToolExecutor(cfg)

	var sinkPayload json.RawMessage
	ctx := withRenderToolSink(context.Background(), &renderToolSink{
		ChatID:  "chat-1",
		RunID:   "run-1",
		Payload: &sinkPayload,
	})

	res, err := exec.Execute(ctx, map[string]any{"view": "chart"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	content, _ := res.Content.(string)
	if strings.TrimSpace(content) == "" {
		t.Error("expected a confirmation message for the model")
	}
	if len(sinkPayload) == 0 {
		t.Fatal("render payload was not attached to the sink")
	}

	var payload render.RenderPayload
	if err := json.Unmarshal(sinkPayload, &payload); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if payload.Mode != render.RenderModeOpenUI {
		t.Errorf("mode = %q, want openui", payload.Mode)
	}
	if payload.DataView == nil || len(payload.DataView.Dataset.Rows) != 2 {
		t.Fatalf("expected a data view with 2 tool rows, got %+v", payload.DataView)
	}
	if strings.TrimSpace(payload.FallbackMarkdown) == "" {
		t.Error("fallback markdown must always be present")
	}
}

func TestExecuteToolForRun_RenderVisualizeAttachesPayload(t *testing.T) {
	loop := &AgentLoop{cfg: LoopConfig{Chats: &mockChatStore{thread: renderTestThread()}}}
	run := &naviruntime.RunState{
		RunID:            "run-1",
		RuntimeSessionID: "chat-1",
		ChatID:           "chat-1",
	}
	contract := &inference.ToolContract{
		ID:              "contract:" + renderVisualizeToolName,
		ToolName:        renderVisualizeToolName,
		SourceType:      "builtin",
		ExecutionKind:   inference.ToolExecutionKindRegistry,
		CommandType:     schema.CommandTypeQuery,
		Domain:          "render",
		ActorKind:       "navi",
		WorkspaceAction: schema.WorkspaceActionRead,
		RiskLevel:       schema.RiskLow,
		Reversibility:   schema.ReversibilityInternal,
	}
	decision := inference.DecisionEnvelope{
		AuthorizedTools: []inference.AuthorizedToolInvocation{
			{
				ToolCall: llm.ToolCall{ID: "call-1", Name: renderVisualizeToolName},
				Permit: inference.ToolPermit{
					ContractID: contract.ID,
					Contract:   contract,
					ToolCallID: "call-1",
					ToolName:   renderVisualizeToolName,
				},
			},
		},
	}

	var scheduled []naviruntime.ScheduledMessage
	envelope, proposal, snapshot, err := loop.executeToolForRun(context.Background(), run, decision, llm.ToolCall{
		ID:        "call-1",
		Name:      renderVisualizeToolName,
		Arguments: map[string]any{"view": "table"},
	}, nil, "", &scheduled)
	if err != nil {
		t.Fatalf("executeToolForRun: %v", err)
	}
	if proposal != nil {
		t.Fatalf("unexpected proposal: %+v", proposal)
	}
	if snapshot == nil || snapshot.Outcome != schema.ExecutionOutcomeSucceeded {
		t.Fatalf("expected successful execution snapshot, got %+v", snapshot)
	}
	if strings.TrimSpace(envelope.Content) == "" {
		t.Fatal("expected concise tool confirmation content")
	}
	if len(run.RenderPayload) == 0 {
		t.Fatal("render payload was not attached to the active run")
	}

	var payload render.RenderPayload
	if err := json.Unmarshal(run.RenderPayload, &payload); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if payload.Mode != render.RenderModeOpenUI {
		t.Errorf("mode = %q, want openui", payload.Mode)
	}
	if payload.DataView == nil || len(payload.DataView.Dataset.Rows) == 0 {
		t.Fatalf("expected data view with real tool-usage rows, got %+v", payload.DataView)
	}
	if payload.DataView.Intent != "table" {
		t.Errorf("data view intent = %q, want table", payload.DataView.Intent)
	}
	if strings.TrimSpace(payload.FallbackMarkdown) == "" {
		t.Error("fallback markdown must always be present")
	}
}

func TestRenderToolExecutor_RequiresRuntimeSink(t *testing.T) {
	cfg := LoopConfig{Chats: &mockChatStore{thread: renderTestThread()}}
	exec := newRenderToolExecutor(cfg)
	// No sink in context → tool is not available (e.g. called outside a run).
	if _, err := exec.Execute(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected error when render sink is absent")
	}
}

func TestRenderVisualizeTool_RegisteredReadOnlyOnRuntimeSurface(t *testing.T) {
	cfg := LoopConfig{Chats: &mockChatStore{thread: renderTestThread()}}
	reg, err := buildRuntimeToolRegistry(cfg)
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}

	entry, ok := reg.Lookup(renderVisualizeToolName)
	if !ok {
		t.Fatalf("render tool %q not registered", renderVisualizeToolName)
	}
	// Read-only governance: a query, no privileged/mutating workspace action.
	if entry.Governance.CommandType != "query" {
		t.Errorf("CommandType = %q, want query (read-only)", entry.Governance.CommandType)
	}
	if len(entry.SideEffects) != 0 {
		t.Errorf("render tool must declare no side effects, got %v", entry.SideEffects)
	}

	// Exposed to the model on the runtime surface so NAVI can choose to render.
	defs := reg.DefinitionsFor(runtimeToolSurface)
	found := false
	for _, d := range defs {
		if d.Name == renderVisualizeToolName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("render tool not exposed on the %q surface", runtimeToolSurface)
	}
}

// Guard the no-privileged-bypass boundary: a read-only render must never be
// registered with a mutating command type.
func TestRenderVisualizeTool_NotMutating(t *testing.T) {
	cfg := LoopConfig{Chats: &mockChatStore{thread: renderTestThread()}}
	reg, err := buildRuntimeToolRegistry(cfg)
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}
	entry, ok := reg.Lookup(renderVisualizeToolName)
	if !ok {
		t.Fatalf("render tool not registered")
	}
	switch entry.Governance.CommandType {
	case "update", "create", "delete", "send", "invoke":
		t.Errorf("render tool must not use mutating command type %q", entry.Governance.CommandType)
	}
	_ = navitool.ToolStatusActive
}
