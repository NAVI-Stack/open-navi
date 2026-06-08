package render

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestClassify_TriggersOnToolUsageVisualRequests(t *testing.T) {
	triggers := []string{
		"Show me a graph of tool usage in this chat.",
		"chart the tools used in this conversation",
		"show tool usage as a table",
		"give me a dashboard of tool calls",
		"visualize tool usage",
		"Visualize tool calls in this conversation.",
		"Make a table of tools used here.",
	}
	for _, in := range triggers {
		got := Classify(in)
		if got.Intent != RenderIntentOpenUIDataRender {
			t.Errorf("Classify(%q).Intent = %q, want %q", in, got.Intent, RenderIntentOpenUIDataRender)
		}
		if got.Capability != CapabilityToolUsage {
			t.Errorf("Classify(%q).Capability = %q, want %q", in, got.Capability, CapabilityToolUsage)
		}
	}
}

func TestClassify_DoesNotHijackPlainToolQuestions(t *testing.T) {
	plain := []string{
		"What tools have you used lately?",
		"Which tools did you use in this chat?",
		"what tools do you have?",
		"can you use a tool to read the file?",
		"show me a graph of revenue",  // visual but not tool usage
		"summarize this conversation", // neither
		"",                            // empty
		"draw a chart",                // visual but no tool subject
	}
	for _, in := range plain {
		if got := Classify(in); got.Intent != RenderIntentPlainResponse {
			t.Errorf("Classify(%q).Intent = %q, want plain_response", in, got.Intent)
		}
	}
}

func TestClassify_PreferredView(t *testing.T) {
	cases := map[string]string{
		"show tool usage as a table":        "table",
		"give me a dashboard of tool calls": "dashboard",
		"timeline of tool usage":            "timeline",
		"graph of tool usage":               "chart",
	}
	for in, want := range cases {
		if got := Classify(in); got.PreferredView != want {
			t.Errorf("Classify(%q).PreferredView = %q, want %q", in, got.PreferredView, want)
		}
	}
}

func sampleEvents() []ToolEvent {
	base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	return []ToolEvent{
		{ToolName: "read_file", IsError: false, UsedAt: base},
		{ToolName: "read_file", IsError: false, UsedAt: base.Add(2 * time.Minute)},
		{ToolName: "read_file", IsError: true, UsedAt: base.Add(time.Minute)},
		{ToolName: "list_dir", IsError: false, UsedAt: base.Add(3 * time.Minute)},
	}
}

func TestBuildToolUsageDataView_Aggregation(t *testing.T) {
	view := BuildToolUsageDataView(sampleEvents(), "chart", Trace{RunID: "run-1", ChatID: "chat-1"})
	if view.Intent != "chart" {
		t.Fatalf("intent = %q, want chart", view.Intent)
	}
	if len(view.Dataset.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(view.Dataset.Rows))
	}
	// read_file should sort first (3 calls > 1).
	top := view.Dataset.Rows[0]
	if top["tool"] != "read_file" {
		t.Errorf("top tool = %v, want read_file", top["tool"])
	}
	if asInt(top["count"]) != 3 {
		t.Errorf("read_file count = %d, want 3", asInt(top["count"]))
	}
	if asInt(top["failures"]) != 1 {
		t.Errorf("read_file failures = %d, want 1", asInt(top["failures"]))
	}
	if asInt(top["successes"]) != 2 {
		t.Errorf("read_file successes = %d, want 2", asInt(top["successes"]))
	}
	last, ok := top["lastUsed"].(string)
	if !ok || !strings.HasPrefix(last, "2026-06-05T12:02:00") {
		t.Errorf("read_file lastUsed = %v, want latest timestamp", top["lastUsed"])
	}
	if view.Trace.RunID != "run-1" {
		t.Errorf("trace runID not propagated: %+v", view.Trace)
	}
}

func TestBuildToolUsageDataView_NormalizesIntent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty defaults to chart", in: "", want: "chart"},
		{name: "chart preserved", in: "chart", want: "chart"},
		{name: "table preserved", in: "table", want: "table"},
		{name: "dashboard preserved", in: "dashboard", want: "dashboard"},
		{name: "timeline preserved", in: "timeline", want: "timeline"},
		{name: "invalid becomes chart", in: "inspector", want: "chart"},
		{name: "random becomes chart", in: "run arbitrary ui", want: "chart"},
		{name: "whitespace and casing normalized", in: "  TABLE \n", want: "table"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := BuildToolUsageDataView(sampleEvents(), tc.in, Trace{})
			if view.Intent != tc.want {
				t.Fatalf("Intent = %q, want %q", view.Intent, tc.want)
			}
		})
	}
}

func TestBuildToolUsageDataView_ReadOnlyGovernance(t *testing.T) {
	view := BuildToolUsageDataView(sampleEvents(), "", Trace{})
	if len(view.Governance.AllowedActions) != 0 {
		t.Errorf("AllowedActions = %v, want empty (read-only)", view.Governance.AllowedActions)
	}
	if len(view.Actions) != 0 {
		t.Errorf("Actions = %v, want none (read-only)", view.Actions)
	}
}

func TestBuildToolUsageDataView_EmptyDegraded(t *testing.T) {
	view := BuildToolUsageDataView(nil, "chart", Trace{})
	if len(view.Dataset.Rows) != 0 {
		t.Fatalf("rows = %d, want 0 (no fabricated data)", len(view.Dataset.Rows))
	}
	if !strings.Contains(strings.ToLower(view.Fallback.Markdown), "no tool usage") {
		t.Errorf("empty fallback markdown should explain absence, got: %q", view.Fallback.Markdown)
	}
}

func TestFallbackMarkdown_ContainsTotalsAndTable(t *testing.T) {
	view := BuildToolUsageDataView(sampleEvents(), "chart", Trace{})
	md := view.Fallback.Markdown
	if !strings.Contains(md, "Total tool calls") {
		t.Errorf("fallback missing total: %q", md)
	}
	if !strings.Contains(md, "**Total tool calls:** 4") {
		t.Errorf("fallback total wrong, want 4: %q", md)
	}
	if !strings.Contains(md, "| read_file |") {
		t.Errorf("fallback missing tool row: %q", md)
	}
}

func TestGenerateOpenUILang_Structure(t *testing.T) {
	view := BuildToolUsageDataView(sampleEvents(), "chart", Trace{})
	lang := GenerateOpenUILang(view)
	if !strings.HasPrefix(lang, "root = Card(") {
		t.Errorf("OpenUI Lang must start with root Card, got: %q", firstLine(lang))
	}
	for _, want := range []string{"MetricCard(", "UsageChart(", "DataTable(", "Bar(", "Row(", "Cell("} {
		if !strings.Contains(lang, want) {
			t.Errorf("OpenUI Lang missing %q:\n%s", want, lang)
		}
	}
	// One Bar per tool (2 tools).
	if got := strings.Count(lang, "Bar("); got != 2 {
		t.Errorf("Bar count = %d, want 2", got)
	}
}

func TestBuildToolUsagePayload_OpenUIPrimaryWithFallback(t *testing.T) {
	payload := BuildToolUsagePayload(sampleEvents(), "chart", Trace{RunID: "r1"})
	if payload.Mode != RenderModeOpenUI {
		t.Errorf("mode = %q, want openui", payload.Mode)
	}
	if strings.TrimSpace(payload.OpenUILang) == "" {
		t.Error("OpenUILang empty — OpenUI is the primary lane")
	}
	if strings.TrimSpace(payload.FallbackMarkdown) == "" {
		t.Error("FallbackMarkdown empty — fallback must always be present")
	}
	if payload.DataView == nil {
		t.Fatal("DataView nil — canonical view must be carried")
	}
	// Round-trips as JSON (it is persisted into metadata_json).
	if _, err := json.Marshal(payload); err != nil {
		t.Fatalf("payload not JSON-serializable: %v", err)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
