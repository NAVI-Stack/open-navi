package orchestration

import "testing"

func TestCapabilitySurfaceIsKnownSurface(t *testing.T) {
	tests := []struct {
		name    string
		surface CapabilitySurface
		want    bool
	}{
		{name: "loop", surface: CapabilitySurface{Surface: CapabilitySurfaceLoop}, want: true},
		{name: "runtime", surface: CapabilitySurface{Surface: CapabilitySurfaceRuntime}, want: true},
		{name: "empty", surface: CapabilitySurface{}, want: false},
		{name: "unknown", surface: CapabilitySurface{Surface: "devtools"}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.surface.IsKnownSurface(); got != tc.want {
				t.Fatalf("IsKnownSurface() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestSurfaceResolutionResultIsGuarded(t *testing.T) {
	if (SurfaceResolutionResult{}).IsGuarded() {
		t.Fatal("expected zero-value result to be unguarded")
	}
	if !(SurfaceResolutionResult{Guard: SurfaceGuardEmptySurface}).IsGuarded() {
		t.Fatal("expected guarded result when guard outcome is set")
	}
}
