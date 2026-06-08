package store

import (
	"encoding/json"
	"testing"

	naviruntime "github.com/open-navi/navi/internal/runtime"
)

func TestAssistantMessageMetadata_MergesToolPartsAndRenderPayload(t *testing.T) {
	run := &naviruntime.RunState{
		ToolParts: []naviruntime.ToolInvocationPart{
			{ToolInvocationID: "c1", ToolName: "read_file", State: "result"},
		},
		RenderPayload: json.RawMessage(`{"mode":"openui","openuiLang":"root = Card(\"x\", [])","fallbackMarkdown":"### Tool usage"}`),
	}

	got := assistantMessageMetadata(run)

	var meta map[string]any
	if err := json.Unmarshal([]byte(got), &meta); err != nil {
		t.Fatalf("metadata is not valid JSON: %v (%s)", err, got)
	}
	if _, ok := meta["toolParts"]; !ok {
		t.Errorf("metadata missing toolParts: %s", got)
	}
	rp, ok := meta["renderPayload"].(map[string]any)
	if !ok {
		t.Fatalf("metadata missing renderPayload object: %s", got)
	}
	if rp["mode"] != "openui" {
		t.Errorf("renderPayload.mode = %v, want openui", rp["mode"])
	}
	if rp["fallbackMarkdown"] != "### Tool usage" {
		t.Errorf("renderPayload.fallbackMarkdown = %v", rp["fallbackMarkdown"])
	}
}

func TestAssistantMessageMetadata_EmptyWhenNothingToCarry(t *testing.T) {
	if got := assistantMessageMetadata(&naviruntime.RunState{}); got != "{}" {
		t.Errorf("empty run metadata = %q, want {}", got)
	}
	if got := assistantMessageMetadata(nil); got != "{}" {
		t.Errorf("nil run metadata = %q, want {}", got)
	}
}

func TestAssistantMessageMetadata_RenderPayloadOnly(t *testing.T) {
	run := &naviruntime.RunState{
		RenderPayload: json.RawMessage(`{"mode":"openui"}`),
	}
	got := assistantMessageMetadata(run)
	var meta map[string]any
	if err := json.Unmarshal([]byte(got), &meta); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := meta["toolParts"]; ok {
		t.Errorf("should not carry toolParts when none: %s", got)
	}
	if _, ok := meta["renderPayload"]; !ok {
		t.Errorf("missing renderPayload: %s", got)
	}
}
