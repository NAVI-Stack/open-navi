package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClassifyPrototype_Triggers(t *testing.T) {
	cases := map[string]PrototypePurpose{
		"Create a dashboard mockup for connector health.":                PurposeDashboard,
		"Build me a component for approving tool calls.":                 PurposeComponent,
		"Mock up a project status panel.":                                PurposePanel,
		"Create a weather display mockup.":                               PurposeDisplay,
		"Show me what a weather display could look like.":                PurposeDisplay,
		"What would a connector health dashboard look like? Mock it up.": PurposeDashboard,
		"How could an approval panel look? Build one.":                   PurposePanel,
	}
	for in, wantPurpose := range cases {
		got := ClassifyPrototype(in)
		if got.Intent != RenderIntentOpenUIPrototypeRender {
			t.Errorf("ClassifyPrototype(%q).Intent = %q, want %q", in, got.Intent, RenderIntentOpenUIPrototypeRender)
			continue
		}
		if got.Capability != CapabilityPrototype {
			t.Errorf("ClassifyPrototype(%q).Capability = %q, want %q", in, got.Capability, CapabilityPrototype)
		}
		if PrototypePurpose(got.PreferredView) != wantPurpose {
			t.Errorf("ClassifyPrototype(%q).PreferredView = %q, want %q", in, got.PreferredView, wantPurpose)
		}
	}
}

func TestClassifyPrototype_DoesNotHijack(t *testing.T) {
	plain := []string{
		"What is connector health?",
		"How should approval work?",
		"What's the weather today?",
		"What's the weather this week?",
		"Show me the weather for this week.", // real-data request, NOT a prototype
		"Is it going to rain tomorrow?",
		"Explain memory timelines.",
		"summarize this conversation",
		"can you read the file for me?",
		"", // empty
	}
	for _, in := range plain {
		if got := ClassifyPrototype(in); got.Intent != RenderIntentPlainResponse {
			t.Errorf("ClassifyPrototype(%q).Intent = %q, want plain_response", in, got.Intent)
		}
	}
}

// TestClassifyPrototype_ContextualPhrasesNeedSurfaceNoun proves the softer
// "contextual" phrases do NOT trigger a prototype render unless a UI-surface noun is
// also present — so ordinary chat is not hijacked.
func TestClassifyPrototype_ContextualPhrasesNeedSurfaceNoun(t *testing.T) {
	plain := []string{
		"What would a normal day look like?",
		"Show me what happened yesterday.",
		"Turn this into a short summary.",
		"Render this as markdown.",
		"Could this look better if rewritten?",
	}
	for _, in := range plain {
		if got := ClassifyPrototype(in); got.Intent != RenderIntentPlainResponse {
			t.Errorf("ClassifyPrototype(%q).Intent = %q, want plain (contextual phrase, no surface noun)", in, got.Intent)
		}
	}
}

// TestClassify_ToolCallPrototypeVsData covers the precedence boundary for requests
// that mention tool calls: a UI mockup ABOUT an action workflow must route to the
// prototype lane (data classifier defers), while a request to visualize REAL tool
// usage must still route to the data lane.
func TestClassify_ToolCallPrototypeVsData(t *testing.T) {
	prototypeReqs := []string{
		"Build me a dashboard for approving tool calls.",
		"Create an approval panel for tool calls.",
		"Mock up a tool-call approval dashboard.",
		"Design a UI for reviewing tool invocations.",
	}
	for _, in := range prototypeReqs {
		if got := Classify(in); got.Intent != RenderIntentPlainResponse {
			t.Errorf("Classify(%q).Intent = %q, want plain (defer to prototype lane)", in, got.Intent)
		}
		if got := ClassifyPrototype(in); got.Intent != RenderIntentOpenUIPrototypeRender {
			t.Errorf("ClassifyPrototype(%q).Intent = %q, want openui_prototype_render", in, got.Intent)
		}
	}

	dataReqs := []string{
		"Show me a graph of tool calls in this chat.",
		"Make a table of tool usage in this conversation.",
		"Visualize tool invocation activity here.",
		"Create a dashboard of tool usage in this chat.",
	}
	for _, in := range dataReqs {
		if got := Classify(in); got.Intent != RenderIntentOpenUIDataRender {
			t.Errorf("Classify(%q).Intent = %q, want openui_data_render (real tool usage)", in, got.Intent)
		}
	}
}

func TestClassifyPrototype_PurposeMapping(t *testing.T) {
	cases := map[string]PrototypePurpose{
		"create a dashboard":              PurposeDashboard,
		"build me a component":            PurposeComponent,
		"design a settings panel":         PurposePanel,
		"make a weather display":          PurposeDisplay,
		"prototype an inspector for this": PurposeInspector,
		"mock up a demo":                  PurposeDemo,
		"design a workflow component":     PurposeWorkflow,
	}
	for in, want := range cases {
		if got := ClassifyPrototype(in); PrototypePurpose(got.PreferredView) != want {
			t.Errorf("ClassifyPrototype(%q).PreferredView = %q, want %q", in, got.PreferredView, want)
		}
	}
}

func TestNormalizePrototypePurpose(t *testing.T) {
	cases := []struct {
		in   string
		want PrototypePurpose
	}{
		{"dashboard", PurposeDashboard},
		{"component", PurposeComponent},
		{"panel", PurposePanel},
		{"display", PurposeDisplay},
		{"demo", PurposeDemo},
		{"inspector", PurposeInspector},
		{"workflow", PurposeWorkflow},
		{"", PurposeComponent},                // empty defaults safely
		{"nonsense", PurposeComponent},        // unknown defaults safely
		{"  DASHBOARD ", PurposeDashboard},    // casing/whitespace normalized
		{"Panel\n", PurposePanel},             // trailing whitespace
		{"{model:garbage}", PurposeComponent}, // model garbage defaults safely
	}
	for _, tc := range cases {
		if got := NormalizePrototypePurpose(tc.in); got != tc.want {
			t.Errorf("NormalizePrototypePurpose(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestClassify_DataVsPrototypePrecedence documents the load-bearing invariant:
// the data-render classifier keeps priority and a tool-usage VISUALIZE request is
// never claimed by the prototype classifier, while a "build a component" request
// is not claimed by the data classifier.
func TestClassify_DataVsPrototypePrecedence(t *testing.T) {
	dataReq := "show me a graph of tool usage in this chat"
	if got := Classify(dataReq); got.Intent != RenderIntentOpenUIDataRender {
		t.Errorf("Classify(%q).Intent = %q, want openui_data_render", dataReq, got.Intent)
	}
	if got := ClassifyPrototype(dataReq); got.Intent != RenderIntentPlainResponse {
		t.Errorf("ClassifyPrototype(%q).Intent = %q, want plain (data classifier owns it)", dataReq, got.Intent)
	}

	protoReq := "Build me a component for approving tool calls."
	if got := Classify(protoReq); got.Intent != RenderIntentPlainResponse {
		t.Errorf("Classify(%q).Intent = %q, want plain (no visual token)", protoReq, got.Intent)
	}
	if got := ClassifyPrototype(protoReq); got.Intent != RenderIntentOpenUIPrototypeRender {
		t.Errorf("ClassifyPrototype(%q).Intent = %q, want openui_prototype_render", protoReq, got.Intent)
	}
}

func TestBuildPrototype_PlaceholderHonesty(t *testing.T) {
	p := BuildPrototype(PurposeDashboard, "Create a dashboard mockup for connector health.", PrototypeTrace{RunID: "run-1"})
	if p.DataSourceKind != DataSourcePlaceholder {
		t.Errorf("DataSourceKind = %q, want placeholder", p.DataSourceKind)
	}
	if p.PlaceholderPolicy != PlaceholderRequired {
		t.Errorf("PlaceholderPolicy = %q, want required", p.PlaceholderPolicy)
	}
	if len(p.Governance.AllowedActions) != 0 {
		t.Errorf("AllowedActions = %v, want empty (read-only)", p.Governance.AllowedActions)
	}
	if p.ComponentIntent != "unknown" {
		t.Errorf("ComponentIntent = %q, want unknown", p.ComponentIntent)
	}
	if p.Title != "Connector Health Dashboard" {
		t.Errorf("Title = %q, want Connector Health Dashboard", p.Title)
	}
	if p.Trace == nil || p.Trace.RunID != "run-1" {
		t.Errorf("Trace not propagated: %+v", p.Trace)
	}

	// Garbage purpose is sanitized to component on the construction path.
	g := BuildPrototype(PrototypePurpose("not-a-real-purpose"), "make me something", PrototypeTrace{})
	if g.Purpose != PurposeComponent {
		t.Errorf("garbage purpose = %q, want component", g.Purpose)
	}
}

func TestGeneratePrototypeOpenUILang_Structure(t *testing.T) {
	p := BuildPrototype(PurposeDashboard, "Create a dashboard mockup for connector health.", PrototypeTrace{})
	prog := GeneratePrototypeOpenUILang(p)
	if !strings.HasPrefix(prog, "root = Card(") {
		t.Errorf("program must start with root Card, got: %q", firstLine(prog))
	}
	for _, want := range []string{`Badge("PROTOTYPE"`, "MetricCard(", "DataTable(", "Row(", "Cell("} {
		if !strings.Contains(prog, want) {
			t.Errorf("program missing %q:\n%s", want, prog)
		}
	}
	// The honesty label is embedded in the program itself.
	if !strings.Contains(strings.ToLower(prog), "placeholder") {
		t.Errorf("program missing embedded placeholder label:\n%s", prog)
	}
}

func TestGeneratePrototypeOpenUILang_GenericScaffold(t *testing.T) {
	p := BuildPrototype(PurposeComponent, "build a component for approving tool calls", PrototypeTrace{})
	prog := GeneratePrototypeOpenUILang(p)
	if !strings.HasPrefix(prog, "root = Card(") {
		t.Errorf("program must start with root Card, got: %q", firstLine(prog))
	}
	if !strings.Contains(prog, `Badge("PROTOTYPE"`) {
		t.Errorf("generic scaffold missing PROTOTYPE badge:\n%s", prog)
	}
	if !strings.Contains(strings.ToLower(prog), "placeholder") {
		t.Errorf("generic scaffold missing placeholder label:\n%s", prog)
	}
}

func TestPrototypeFallbackMarkdown_Honesty(t *testing.T) {
	p := BuildPrototype(PurposeDashboard, "Create a dashboard mockup for connector health.", PrototypeTrace{})
	md := strings.ToLower(p.FallbackMarkdown)
	for _, want := range []string{"prototype", "placeholder", "mockup", "not live data"} {
		if !strings.Contains(md, want) {
			t.Errorf("fallback markdown missing %q:\n%s", want, p.FallbackMarkdown)
		}
	}
	if !strings.Contains(p.FallbackMarkdown, "Connector Health Dashboard") {
		t.Errorf("fallback markdown missing title:\n%s", p.FallbackMarkdown)
	}
}

func TestBuildPrototypePayload_Shape(t *testing.T) {
	payload := BuildPrototypePayload(PurposeDashboard, "Create a dashboard mockup for connector health.", PrototypeTrace{RunID: "r1"})
	if payload.Mode != RenderModeOpenUI {
		t.Errorf("mode = %q, want openui", payload.Mode)
	}
	if strings.TrimSpace(payload.OpenUILang) == "" {
		t.Error("OpenUILang empty — OpenUI is the primary lane")
	}
	if strings.TrimSpace(payload.FallbackMarkdown) == "" {
		t.Error("FallbackMarkdown empty — fallback must always be present")
	}
	if payload.Prototype == nil {
		t.Fatal("Prototype nil — canonical prototype must be carried")
	}
	if payload.DataSourceKind != DataSourcePlaceholder {
		t.Errorf("DataSourceKind = %q, want placeholder", payload.DataSourceKind)
	}
	if payload.DataView != nil {
		t.Error("DataView should be nil for a prototype (no real telemetry)")
	}
	// Round-trips as JSON (it is persisted into metadata_json).
	if _, err := json.Marshal(payload); err != nil {
		t.Fatalf("payload not JSON-serializable: %v", err)
	}
}
