package tool

import "testing"

func TestToolMetadataDefaultsPreserveCurrentExposureSemantics(t *testing.T) {
	tool := &Tool{}

	if got := tool.NormalizedExposureClass(); got != ToolExposureUserFacing {
		t.Fatalf("default exposure class = %q, want %q", got, ToolExposureUserFacing)
	}

	if tool.HasInvalidExposureClass() {
		t.Fatal("empty exposure class should not be invalid")
	}

	modes := tool.NormalizedInteractionModes()
	if len(modes) != 2 || modes[0] != ToolInteractionConversation || modes[1] != ToolInteractionAction {
		t.Fatalf("default interaction modes = %#v, want conversation+action", modes)
	}

	if !tool.SupportsInteractionMode(ToolInteractionConversation) || !tool.SupportsInteractionMode(ToolInteractionAction) {
		t.Fatal("default interaction policy should allow both conversation and action")
	}

	if got := tool.RequiresToolCapableModel(); !got {
		t.Fatal("default tool-capable-model requirement should be true")
	}
}

func TestToolMetadataExplicitOverridesArePreserved(t *testing.T) {
	requiresToolCapableModel := false
	tool := &Tool{
		Metadata: ToolMetadata{
			ExposureClass:            ToolExposureInternal,
			InteractionModes:         []ToolInteractionMode{ToolInteractionAction, ToolInteractionAction},
			RequiresToolCapableModel: &requiresToolCapableModel,
		},
	}

	if got := tool.NormalizedExposureClass(); got != ToolExposureInternal {
		t.Fatalf("normalized exposure class = %q, want %q", got, ToolExposureInternal)
	}

	if tool.HasInvalidExposureClass() {
		t.Fatal("known exposure class should not be invalid")
	}

	modes := tool.NormalizedInteractionModes()
	if len(modes) != 1 || modes[0] != ToolInteractionAction {
		t.Fatalf("normalized interaction modes = %#v, want action-only", modes)
	}

	if tool.SupportsInteractionMode(ToolInteractionConversation) {
		t.Fatal("conversation mode should be excluded by explicit action-only metadata")
	}
	if !tool.SupportsInteractionMode(ToolInteractionAction) {
		t.Fatal("action mode should be preserved by explicit metadata")
	}

	if got := tool.RequiresToolCapableModel(); got {
		t.Fatal("explicit tool-capable-model override should be preserved")
	}
}

func TestToolMetadataInvalidValuesAreDetectableWithoutSilentFallback(t *testing.T) {
	tool := &Tool{
		Metadata: ToolMetadata{
			ExposureClass:    ToolExposureClass("mystery"),
			InteractionModes: []ToolInteractionMode{ToolInteractionMode("surprise"), ToolInteractionAction},
		},
	}

	if !tool.HasInvalidExposureClass() {
		t.Fatal("invalid exposure class should be detectable")
	}

	if got := tool.NormalizedExposureClass(); got != ToolExposureClass("mystery") {
		t.Fatalf("explicit invalid exposure class should remain inspectable, got %q", got)
	}

	if !tool.HasInvalidInteractionModes() {
		t.Fatal("invalid interaction mode should be detectable")
	}

	modes := tool.NormalizedInteractionModes()
	if len(modes) != 1 || modes[0] != ToolInteractionAction {
		t.Fatalf("normalized interaction modes = %#v, want only the valid explicit mode", modes)
	}
}
