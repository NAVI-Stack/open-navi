package capability

import "strings"

const (
	UITargetConsole = "console"
	UITargetPET     = "pet"

	UIRenderModeHeadless = "headless"
)

// UISurfaceSpec is the declarative V1 UI surface contract shared by skills and
// plugin manifests. It carries metadata only; it never authorizes executable UI
// code such as HTML, JavaScript, React bundles, inline scripts, or remote assets.
type UISurfaceSpec struct {
	ID             string                 `json:"id" yaml:"id"`
	Title          string                 `json:"title,omitempty" yaml:"title,omitempty"`
	Description    string                 `json:"description,omitempty" yaml:"description,omitempty"`
	TargetSurfaces []string               `json:"targetSurfaces,omitempty" yaml:"target_surfaces,omitempty"`
	RenderMode     string                 `json:"renderMode" yaml:"render_mode"`
	Schema         map[string]interface{} `json:"schema,omitempty" yaml:"schema,omitempty"`
	Actions        []UIActionBinding      `json:"actions,omitempty" yaml:"actions,omitempty"`
}

type UIActionBinding struct {
	ID          string                 `json:"id" yaml:"id"`
	Label       string                 `json:"label,omitempty" yaml:"label,omitempty"`
	Interface   string                 `json:"interface" yaml:"interface"`
	Confirm     bool                   `json:"confirm,omitempty" yaml:"confirm,omitempty"`
	Description string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Defaults    map[string]interface{} `json:"defaults,omitempty" yaml:"defaults,omitempty"`
}

type UISurfaceValidation struct {
	Valid         bool
	Supported     bool
	Reasons       []string
	RenderMode    string
	Targets       []string
	MatchesTarget bool
}

func ValidateUISurface(surface UISurfaceSpec, requestedTarget string) UISurfaceValidation {
	result := UISurfaceValidation{
		Valid:      true,
		Supported:  true,
		RenderMode: strings.TrimSpace(surface.RenderMode),
		Targets:    normalizeTargets(surface.TargetSurfaces),
	}
	if strings.TrimSpace(surface.ID) == "" {
		result.Valid = false
		result.Reasons = append(result.Reasons, "ui surface id is required")
	}
	if result.RenderMode == "" {
		result.RenderMode = UIRenderModeHeadless
	}
	if result.RenderMode != UIRenderModeHeadless {
		result.Supported = false
		result.Reasons = append(result.Reasons, "only headless render_mode is supported in V1")
	}
	if len(result.Targets) == 0 {
		result.Targets = []string{UITargetConsole}
	}
	for _, target := range result.Targets {
		if !IsKnownUITarget(target) {
			result.Valid = false
			result.Reasons = append(result.Reasons, "unsupported target_surface "+target)
		}
	}
	if requestedTarget == "" {
		result.MatchesTarget = true
	} else {
		target := strings.TrimSpace(strings.ToLower(requestedTarget))
		for _, candidate := range result.Targets {
			if candidate == target {
				result.MatchesTarget = true
				break
			}
		}
	}
	for _, action := range surface.Actions {
		if strings.TrimSpace(action.ID) == "" {
			result.Valid = false
			result.Reasons = append(result.Reasons, "ui action id is required")
		}
		if strings.TrimSpace(action.Interface) == "" {
			result.Valid = false
			result.Reasons = append(result.Reasons, "ui action interface binding is required")
		}
	}
	return result
}

func IsKnownUITarget(target string) bool {
	switch strings.TrimSpace(strings.ToLower(target)) {
	case UITargetConsole, UITargetPET:
		return true
	default:
		return false
	}
}

func normalizeTargets(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		target := strings.TrimSpace(strings.ToLower(value))
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		out = append(out, target)
	}
	return out
}
