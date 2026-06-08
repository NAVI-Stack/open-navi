package artifact

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestSerializeForSubtype_JSONAcceptsStructuredValues(t *testing.T) {
	got, err := serializeForSubtype("json", map[string]any{
		"name": "navi",
		"ok":   true,
	})
	if err != nil {
		t.Fatalf("serialize json: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("unmarshal serialized json: %v", err)
	}
	if decoded["name"] != "navi" || decoded["ok"] != true {
		t.Fatalf("unexpected decoded value: %#v", decoded)
	}
}

func TestSerializeForSubtype_JSONRejectsInvalidText(t *testing.T) {
	if _, err := serializeForSubtype("json", "{not-json"); err == nil {
		t.Fatalf("expected invalid json error")
	}
}

func TestSerializeForSubtype_TableAcceptsStructuredRows(t *testing.T) {
	got, err := serializeForSubtype("table", [][]string{
		{"name", "value"},
		{"navi", "1"},
	})
	if err != nil {
		t.Fatalf("serialize table: %v", err)
	}
	if got != "name,value\nnavi,1\n" {
		t.Fatalf("unexpected csv output %q", got)
	}

	parsed, err := codecForSubtype("table").Parse(got)
	if err != nil {
		t.Fatalf("parse table: %v", err)
	}
	rows, ok := parsed.([][]string)
	if !ok {
		t.Fatalf("parsed table type = %T, want [][]string", parsed)
	}
	if !slices.Equal(rows[0], []string{"name", "value"}) {
		t.Fatalf("unexpected header row: %v", rows[0])
	}
}

func TestSerializeForSubtype_TextRequiresString(t *testing.T) {
	if _, err := serializeForSubtype("markdown", map[string]string{"x": "y"}); err == nil {
		t.Fatalf("expected text subtype validation error")
	}
}
