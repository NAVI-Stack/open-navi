package artifact

import (
	"slices"
	"testing"

	"github.com/open-navi/navi/internal/schema"
)

func TestRegistry_DefaultContractsCoverV1CoreTypes(t *testing.T) {
	reg := NewRegistry()

	markdown := reg.GetRenderer("markdown")
	if markdown.Type != schema.ArtifactTypeDocument {
		t.Fatalf("markdown renderer type = %q, want %q", markdown.Type, schema.ArtifactTypeDocument)
	}
	if !markdown.SupportsDiff {
		t.Fatalf("markdown renderer should support diff")
	}
	if !slices.Equal(markdown.PatchStrategies, []string{PatchStrategyBlock, PatchStrategyTextRange, PatchStrategyFull}) {
		t.Fatalf("markdown patch strategies = %v", markdown.PatchStrategies)
	}

	code := reg.GetEditor("code")
	if code.Type != schema.ArtifactTypeCode {
		t.Fatalf("code editor type = %q, want %q", code.Type, schema.ArtifactTypeCode)
	}
	if !slices.Contains(code.Features, "syntax_highlighting") {
		t.Fatalf("code editor missing syntax_highlighting: %v", code.Features)
	}
	if !slices.Equal(code.PatchStrategies, []string{PatchStrategyAST, PatchStrategyFunction, PatchStrategyText, PatchStrategyFull}) {
		t.Fatalf("code patch strategies = %v", code.PatchStrategies)
	}

	jsonRenderer := reg.GetRenderer("json")
	if jsonRenderer.Type != schema.ArtifactTypeData {
		t.Fatalf("json renderer type = %q, want %q", jsonRenderer.Type, schema.ArtifactTypeData)
	}
	if !slices.Contains(jsonRenderer.ExportFormats, "json") {
		t.Fatalf("json renderer missing json export: %v", jsonRenderer.ExportFormats)
	}

	table := reg.GetEditor("table")
	if table.ValidateStrategy != "tabular" {
		t.Fatalf("table validate strategy = %q, want tabular", table.ValidateStrategy)
	}
	if !slices.Contains(table.ParseFormats, "csv") {
		t.Fatalf("table parse formats = %v, want csv support", table.ParseFormats)
	}
}

func TestRegistry_AliasesReuseRegisteredContracts(t *testing.T) {
	reg := NewRegistry()

	if got := reg.GetRenderer("workflow_draft"); got.ComponentID != "MarkdownRenderer" {
		t.Fatalf("workflow_draft renderer = %q, want MarkdownRenderer", got.ComponentID)
	}
	if got := reg.GetEditor("csv"); got.EditorID != "DataTableEditor" {
		t.Fatalf("csv editor = %q, want DataTableEditor", got.EditorID)
	}
}

func TestRegistry_UnknownSubtypeFallsBackToRaw(t *testing.T) {
	reg := NewRegistry()

	renderer := reg.GetRenderer("custom-binary")
	if renderer.ComponentID != "RawRenderer" {
		t.Fatalf("renderer fallback = %q, want RawRenderer", renderer.ComponentID)
	}
	if !slices.Equal(renderer.DisplayModes, []string{"read", "edit"}) {
		t.Fatalf("renderer fallback modes = %v", renderer.DisplayModes)
	}

	editor := reg.GetEditor("custom-binary")
	if editor.EditorID != "RawEditor" {
		t.Fatalf("editor fallback = %q, want RawEditor", editor.EditorID)
	}
	if !slices.Equal(editor.PatchStrategies, []string{PatchStrategyFull}) {
		t.Fatalf("editor fallback patch strategies = %v", editor.PatchStrategies)
	}
}

func TestRegistry_ListMethodsReturnSortedResults(t *testing.T) {
	reg := NewRegistry()

	renderers := reg.ListRenderers()
	if len(renderers) == 0 {
		t.Fatalf("expected default renderers")
	}
	for i := 1; i < len(renderers); i++ {
		if renderers[i-1].Subtype > renderers[i].Subtype {
			t.Fatalf("renderers not sorted: %q before %q", renderers[i-1].Subtype, renderers[i].Subtype)
		}
	}

	editors := reg.ListEditors()
	if len(editors) == 0 {
		t.Fatalf("expected default editors")
	}
	for i := 1; i < len(editors); i++ {
		if editors[i-1].Subtype > editors[i].Subtype {
			t.Fatalf("editors not sorted: %q before %q", editors[i-1].Subtype, editors[i].Subtype)
		}
	}
}
