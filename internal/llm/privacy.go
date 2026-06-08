package llm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ceoai/navi/internal/schema"
)

// PrivacyMode is the CIP §9 locality posture. It is the privacy-tier dimension
// P4 adds to the existing contextual router (it does not replace TaskClass-based
// selection — privacy is an additional filter applied before the final route
// choice).
type PrivacyMode string

const (
	// PrivacyModeLocal refuses cloud routes for any privacy class (no source
	// payload leaves the device).
	PrivacyModeLocal PrivacyMode = "local"
	// PrivacyModeHybrid allows cloud routes for low-sensitivity classes (public,
	// personal) and keeps sensitive/secret local.
	PrivacyModeHybrid PrivacyMode = "hybrid"
	// PrivacyModeCloud permits cloud routes regardless of locality. This is the
	// permissive default (empty mode resolves to Cloud) so pre-P4 behavior is
	// preserved for callers that never set a mode.
	PrivacyModeCloud PrivacyMode = "cloud"
)

// ErrNoRouteAvailable is returned when no profile can satisfy a request even
// before privacy filtering (e.g. no tool-capable model when tools are required).
var ErrNoRouteAvailable = errors.New("llm: no route available for request")

// RouteRefusedError is the observable refusal raised when privacy policy bars a
// payload from any available cloud model and no local model can serve it. It is
// a typed error (not a silent fallback) so the refusal is inspectable and
// testable — a secret-class payload in Local mode must never silently route to a
// cloud model.
type RouteRefusedError struct {
	Mode         PrivacyMode
	PrivacyClass schema.PrivacyClass
	Reason       string
}

func (e *RouteRefusedError) Error() string {
	return fmt.Sprintf(
		"llm: route refused — %q-class content may not use a cloud model in %q privacy mode and no local model is available: %s",
		string(e.PrivacyClass), string(e.Mode), e.Reason,
	)
}

// CloudEligible reports whether a payload of the given privacy class may route to
// a cloud model under mode (CIP §9 policy table).
func CloudEligible(mode PrivacyMode, class schema.PrivacyClass) bool {
	switch resolvePrivacyMode(mode) {
	case PrivacyModeLocal:
		return false
	case PrivacyModeHybrid:
		switch class {
		case schema.PrivacyClassSensitive, schema.PrivacyClassSecret:
			return false
		default: // public, personal, unset
			return true
		}
	default: // Cloud
		return true
	}
}

func resolvePrivacyMode(mode PrivacyMode) PrivacyMode {
	switch PrivacyMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case PrivacyModeLocal:
		return PrivacyModeLocal
	case PrivacyModeHybrid:
		return PrivacyModeHybrid
	default:
		return PrivacyModeCloud
	}
}

// SelectWithPrivacy is the privacy-aware route choice (P4). It applies the
// privacy tier as a filter before the existing TaskClass-based selection:
//
//   - If the class is cloud-eligible under the selector's PrivacyMode, it
//     delegates to Select unchanged.
//   - Otherwise it restricts candidates to local models and re-runs selection.
//     If a local model can serve the request, it is chosen (an observable,
//     logged fallback — Reason = "privacy_local_only"). If none can, it returns
//     a *RouteRefusedError rather than leaking to cloud.
//
// The privacyClass argument is the effective class of the payload being routed
// (for CIP it is the most restrictive class in the retrieved context set).
func (s ModelSelector) SelectWithPrivacy(classification TaskClassification, toolsNeeded bool, privacyClass schema.PrivacyClass) (RouteSelection, error) {
	if CloudEligible(s.PrivacyMode, privacyClass) {
		sel, ok := s.Select(classification, toolsNeeded)
		if !ok {
			return RouteSelection{Classification: classification}, ErrNoRouteAvailable
		}
		return sel, nil
	}

	local := s.localOnly()
	if len(local.Profiles) == 0 {
		return RouteSelection{Classification: classification}, &RouteRefusedError{
			Mode:         resolvePrivacyMode(s.PrivacyMode),
			PrivacyClass: privacyClass,
			Reason:       "no local model is configured",
		}
	}
	sel, ok := local.Select(classification, toolsNeeded)
	if !ok {
		reason := "no local model satisfies the request"
		if toolsNeeded {
			reason = "no tool-capable local model is available"
		}
		return RouteSelection{Classification: classification}, &RouteRefusedError{
			Mode:         resolvePrivacyMode(s.PrivacyMode),
			PrivacyClass: privacyClass,
			Reason:       reason,
		}
	}
	sel.Reason = "privacy_local_only"
	return sel, nil
}

// localOnly returns a copy of the selector whose candidate profiles are
// restricted to local (on-device) models, with cloud-targeting preferences
// neutralized so a configured cloud default cannot re-introduce a cloud route.
func (s ModelSelector) localOnly() ModelSelector {
	out := s
	filtered := make([]ModelProfile, 0, len(s.Profiles))
	for _, p := range s.Profiles {
		if isLocalProfile(p) {
			filtered = append(filtered, p)
		}
	}
	out.Profiles = filtered
	// PreferLocal nudges scoring toward local; harmless and consistent here.
	out.Preferences.PreferLocal = true
	return out
}

func isLocalProfile(p ModelProfile) bool {
	if strings.EqualFold(strings.TrimSpace(p.ProviderKey), "ollama") {
		return true
	}
	return hasTag(p.Tags, "local")
}

// PrivacyClassPolicy is one row of the privacy policy table surfaced via the API.
type PrivacyClassPolicy struct {
	Class         string `json:"class"`
	CloudEligible bool   `json:"cloud_eligible"`
}

// PrivacyPolicyView is the operator-inspectable privacy-tier configuration
// surfaced through GET /api/llm/profiles (so the policy can be inspected without
// code changes).
type PrivacyPolicyView struct {
	Mode    string               `json:"mode"`
	Classes []PrivacyClassPolicy `json:"classes"`
	Note    string               `json:"note"`
}

// BuildPrivacyPolicyView renders the effective privacy policy for a mode.
func BuildPrivacyPolicyView(mode PrivacyMode) PrivacyPolicyView {
	resolved := resolvePrivacyMode(mode)
	classes := []schema.PrivacyClass{
		schema.PrivacyClassPublic,
		schema.PrivacyClassPersonal,
		schema.PrivacyClassSensitive,
		schema.PrivacyClassSecret,
	}
	rows := make([]PrivacyClassPolicy, 0, len(classes))
	for _, c := range classes {
		rows = append(rows, PrivacyClassPolicy{
			Class:         string(c),
			CloudEligible: CloudEligible(resolved, c),
		})
	}
	note := "Cloud routes permitted for all privacy classes."
	switch resolved {
	case PrivacyModeLocal:
		note = "Local mode: cloud routes refused for every privacy class; secret-class payloads route to a local model or are refused."
	case PrivacyModeHybrid:
		note = "Hybrid mode: public and personal classes may use cloud models; sensitive and secret stay local."
	}
	return PrivacyPolicyView{Mode: string(resolved), Classes: rows, Note: note}
}
