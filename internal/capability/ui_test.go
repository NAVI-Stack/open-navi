package capability

import "testing"

func TestValidateUISurfaceHeadlessConsole(t *testing.T) {
	result := ValidateUISurface(UISurfaceSpec{
		ID:             "contacts.console",
		TargetSurfaces: []string{"console"},
		RenderMode:     "headless",
		Actions: []UIActionBinding{{
			ID:        "list",
			Interface: "list_contacts",
		}},
	}, "console")
	if !result.Valid || !result.Supported || !result.MatchesTarget {
		t.Fatalf("expected valid supported console surface, got %#v", result)
	}
}

func TestValidateUISurfaceUnsupportedRenderMode(t *testing.T) {
	result := ValidateUISurface(UISurfaceSpec{
		ID:             "custom.react",
		TargetSurfaces: []string{"console"},
		RenderMode:     "react",
	}, "console")
	if !result.Valid {
		t.Fatalf("unsupported render mode should not invalidate metadata: %#v", result)
	}
	if result.Supported {
		t.Fatalf("expected unsupported surface, got %#v", result)
	}
}

func TestValidateUISurfaceRejectsInvalidMetadata(t *testing.T) {
	result := ValidateUISurface(UISurfaceSpec{
		ID:             "",
		TargetSurfaces: []string{"console"},
		RenderMode:     "headless",
		Actions: []UIActionBinding{{
			ID: "broken",
		}},
	}, "console")
	if result.Valid {
		t.Fatalf("expected invalid metadata, got %#v", result)
	}
}
