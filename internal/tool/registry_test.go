package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/schema"
)

type stubExecutor struct {
	result any
}

func (s stubExecutor) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	return ToolResult{Content: s.result}, nil
}

func validTestTool(id string) *Tool {
	return &Tool{
		ToolID:        id,
		DisplayName:   "Test Tool",
		Description:   "Test tool description.",
		Source:        ToolSourceBuiltin,
		SourceID:      "tests.registry",
		SchemaVersion: "1.0.0",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		OutputSchema: map[string]any{
			"type": "string",
		},
		Category:              ToolCategoryReadOnly,
		RiskTier:              "low",
		SideEffects:           []string{},
		Reversibility:         "reversible",
		EnvironmentVisibility: []string{"development"},
		RequiredModes:         []schema.DirectiveMode{},
		RequiredAuthority:     ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{},
		CapabilityTags:        []string{"tests"},
		Status:                ToolStatusActive,
		Definition: llm.ToolDefinition{
			Name:        id,
			Description: "Test tool description.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func TestRegistryRegisterPreservesOrder(t *testing.T) {
	reg := NewRegistry()
	first := validTestTool("test.registry.first")
	first.DisplayName = "First"
	if err := reg.Register(first); err != nil {
		t.Fatalf("register first: %v", err)
	}
	second := validTestTool("test.registry.second")
	second.DisplayName = "Second"
	second.Source = ToolSourceSkill
	second.SourceID = "tests.skill"
	if err := reg.Register(second); err != nil {
		t.Fatalf("register second: %v", err)
	}

	defs := reg.Definitions()
	if len(defs) != 2 || defs[0].Name != "test.registry.first" || defs[1].Name != "test.registry.second" {
		t.Fatalf("unexpected definition order: %+v", defs)
	}
}

func TestRegistryRegisterRejectsDuplicates(t *testing.T) {
	reg := NewRegistry()
	tool := validTestTool("test.registry.dup")
	if err := reg.Register(tool); err != nil {
		t.Fatalf("register dup: %v", err)
	}
	if err := reg.Register(tool); err == nil {
		t.Fatal("expected duplicate registration error")
	}
}

func TestRegistryExecuteUsesExecutor(t *testing.T) {
	reg := NewRegistry()
	tool := validTestTool("test.registry.exec")
	tool.Executor = stubExecutor{result: "ok"}
	tool.Governance = ToolGovernance{CommandType: schema.CommandTypeQuery}
	if err := reg.Register(tool); err != nil {
		t.Fatalf("register exec: %v", err)
	}

	result, err := reg.Execute(context.Background(), "test.registry.exec", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, ok := result.Content.(string); !ok || got != "ok" {
		t.Fatalf("unexpected result: %#v", result.Content)
	}
}

func TestRegistryDefinitionsSkipsHiddenTools(t *testing.T) {
	reg := NewRegistry()
	visible := validTestTool("test.registry.visible")
	if err := reg.Register(visible); err != nil {
		t.Fatalf("register visible: %v", err)
	}
	hidden := validTestTool("test.registry.hidden")
	hidden.Hidden = true
	if err := reg.Register(hidden); err != nil {
		t.Fatalf("register hidden: %v", err)
	}

	defs := reg.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected one visible definition, got %d", len(defs))
	}
	if defs[0].Name != "test.registry.visible" {
		t.Fatalf("expected visible tool definition, got %+v", defs)
	}
	if _, ok := reg.Lookup("test.registry.hidden"); !ok {
		t.Fatal("expected hidden tool to remain registered for lookup")
	}
}

func TestRegistryDefinitionsForSurface(t *testing.T) {
	reg := NewRegistry()
	all := validTestTool("test.registry.all")
	if err := reg.Register(all); err != nil {
		t.Fatalf("register all: %v", err)
	}
	runtimeOnly := validTestTool("test.registry.runtime_only")
	runtimeOnly.VisibleOn = []string{"runtime"}
	if err := reg.Register(runtimeOnly); err != nil {
		t.Fatalf("register runtime-only: %v", err)
	}

	allDefs := reg.Definitions()
	if len(allDefs) != 2 {
		t.Fatalf("expected all visible tools in Definitions, got %+v", allDefs)
	}
	loopDefs := reg.DefinitionsFor("loop")
	if len(loopDefs) != 1 || loopDefs[0].Name != "test.registry.all" {
		t.Fatalf("expected only shared tool on loop surface, got %+v", loopDefs)
	}
	runtimeDefs := reg.DefinitionsFor("runtime")
	if len(runtimeDefs) != 2 {
		t.Fatalf("expected both tools on runtime surface, got %+v", runtimeDefs)
	}
}

func TestRegistryReplacePreservesPointerAndUpdatesContents(t *testing.T) {
	left := NewRegistry()
	oldTool := validTestTool("test.registry.old")
	if err := left.Register(oldTool); err != nil {
		t.Fatalf("register old: %v", err)
	}
	right := NewRegistry()
	newTool := validTestTool("test.registry.new")
	if err := right.Register(newTool); err != nil {
		t.Fatalf("register new: %v", err)
	}

	left.Replace(right)

	if _, ok := left.Lookup("test.registry.old"); ok {
		t.Fatal("expected old tool to be replaced")
	}
	if tool, ok := left.Lookup("test.registry.new"); !ok || tool.ToolID != "test.registry.new" {
		t.Fatalf("expected new tool after replace, got %#v", tool)
	}
}

func TestRegistryRegisterRejectsMissingCanonicalMetadata(t *testing.T) {
	reg := NewRegistry()
	tool := validTestTool("test.registry.invalid")
	tool.DisplayName = ""

	err := reg.Register(tool)
	if err == nil {
		t.Fatal("expected missing metadata to fail registration")
	}
	if !strings.Contains(err.Error(), "display_name") {
		t.Fatalf("expected display_name validation error, got %v", err)
	}
}

func TestRegistryLookupUsesToolIDNotDisplayName(t *testing.T) {
	reg := NewRegistry()
	tool := validTestTool("test.registry.lookup")
	tool.DisplayName = "Lookup Tool"
	if err := reg.Register(tool); err != nil {
		t.Fatalf("register lookup tool: %v", err)
	}

	if _, ok := reg.Lookup("Lookup Tool"); ok {
		t.Fatal("display name should not resolve registry identity")
	}
	if _, ok := reg.Lookup("test.registry.lookup"); !ok {
		t.Fatal("tool_id should resolve registry identity")
	}
}

func TestRegistryLookupExactRejectsAliasesAndCarriesSnapshotID(t *testing.T) {
	reg := NewRegistry()
	tool := validTestTool("test.registry.exact")
	tool.Aliases = []string{"exact tool"}
	if err := reg.Register(tool); err != nil {
		t.Fatalf("register exact tool: %v", err)
	}

	lookup, ok := reg.LookupExact("test.registry.exact")
	if !ok {
		t.Fatal("expected exact tool_id lookup to resolve")
	}
	if lookup.SnapshotID == "" {
		t.Fatal("expected exact lookup to carry a snapshot id")
	}
	if lookup.Tool == nil || lookup.Tool.ToolID != "test.registry.exact" {
		t.Fatalf("unexpected exact lookup result: %#v", lookup.Tool)
	}

	if _, ok := reg.LookupExact("exact tool"); ok {
		t.Fatal("exact lookup must not resolve aliases")
	}
	if aliased, ok := reg.LookupWithSnapshot("exact tool"); !ok || aliased.Tool == nil || aliased.Tool.ToolID != "test.registry.exact" {
		t.Fatalf("expected snapshot lookup to resolve alias, got %#v", aliased.Tool)
	}
}

func TestRegistrySnapshotCreationTracksMutations(t *testing.T) {
	reg := NewRegistry()

	initial := reg.Snapshot()
	if initial.ID == "" {
		t.Fatal("expected initial snapshot id")
	}
	if len(initial.Tools) != 0 {
		t.Fatalf("expected empty initial snapshot, got %d tools", len(initial.Tools))
	}

	tool := validTestTool("test.registry.snapshot")
	if err := reg.Register(tool); err != nil {
		t.Fatalf("register snapshot tool: %v", err)
	}

	list := reg.ListWithSnapshot()
	if list.SnapshotID == "" {
		t.Fatal("expected list snapshot id")
	}
	if list.SnapshotID == initial.ID {
		t.Fatalf("expected snapshot id to change after mutation, still %q", list.SnapshotID)
	}
	if len(list.Tools) != 1 || list.Tools[0].ToolID != "test.registry.snapshot" {
		t.Fatalf("unexpected snapshot list contents: %#v", list.Tools)
	}

	list.Tools[0].DisplayName = "mutated copy"
	stored, ok := reg.Lookup("test.registry.snapshot")
	if !ok {
		t.Fatal("expected stored tool to remain available")
	}
	if stored.DisplayName != "Test Tool" {
		t.Fatalf("expected snapshot tool copy to be isolated from registry, got %q", stored.DisplayName)
	}
}
