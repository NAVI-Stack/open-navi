package artifact

import (
	"encoding/json"
	"testing"
)

func TestBuildDiffPayload_TextSubtype(t *testing.T) {
	raw, err := buildDiffPayload("markdown", "a\nb\n", "a\nc\n")
	if err != nil {
		t.Fatalf("buildDiffPayload: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got["kind"] != "line" {
		t.Fatalf("kind = %v, want line", got["kind"])
	}
	if got["changed"] != true {
		t.Fatalf("changed = %v, want true", got["changed"])
	}
}

func TestBuildDiffPayload_JSONSubtype(t *testing.T) {
	raw, err := buildDiffPayload("json", `{"a":1}`, `{"a":2}`)
	if err != nil {
		t.Fatalf("buildDiffPayload: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got["kind"] != "structured" {
		t.Fatalf("kind = %v, want structured", got["kind"])
	}
	if got["changed"] != true {
		t.Fatalf("changed = %v, want true", got["changed"])
	}
}
