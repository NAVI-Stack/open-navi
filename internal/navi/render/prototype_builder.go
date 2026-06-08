package render

import (
	"fmt"
	"strings"
)

// BuildPrototype constructs the canonical NaviUIPrototype for a request. The
// purpose is re-sanitized via NormalizePrototypePurpose so the canonical model
// never trusts raw classifier/model output. Prototypes are always placeholder,
// read-only (no allowed actions), and carry an honest fallback markdown banner.
func BuildPrototype(purpose PrototypePurpose, prompt string, trace PrototypeTrace) *NaviUIPrototype {
	p := NormalizePrototypePurpose(string(purpose))
	title := prototypeTitle(p, prompt)
	proto := &NaviUIPrototype{
		ID:                fmt.Sprintf("prototype-%s", p),
		Title:             title,
		Purpose:           p,
		Prompt:            strings.TrimSpace(prompt),
		DataSourceKind:    DataSourcePlaceholder,
		PlaceholderPolicy: PlaceholderRequired,
		ComponentIntent:   "unknown",
		Governance: PrototypeGovernance{
			AllowedActions: []string{}, // read-only: generated prototype UI may not invoke actions
			AuditLevel:     "basic",
		},
	}
	if trace.RunID != "" || trace.MessageID != "" {
		t := trace
		proto.Trace = &t
	}
	proto.FallbackMarkdown = PrototypeFallbackMarkdown(proto)
	return proto
}

// prototypeTitle derives a human title from the purpose and the user's prompt. It
// special-cases the proving-slice subjects (connector health, weather) and falls
// back to a generic "<Purpose> Prototype" otherwise.
func prototypeTitle(purpose PrototypePurpose, prompt string) string {
	norm := normalize(prompt)
	switch {
	case purpose == PurposeDashboard && mentionsConnectorHealth(norm):
		return "Connector Health Dashboard"
	case mentionsWeather(norm):
		return "Weather Display Concept"
	case strings.Contains(norm, "connector"):
		return "Connector " + titleCasePurpose(purpose)
	case strings.Contains(norm, "approval") || strings.Contains(norm, "approving"):
		return "Approval " + titleCasePurpose(purpose)
	case strings.Contains(norm, "project") || strings.Contains(norm, "status"):
		return "Project Status " + titleCasePurpose(purpose)
	case strings.Contains(norm, "settings"):
		return "Settings " + titleCasePurpose(purpose)
	case strings.Contains(norm, "memory"):
		return "Memory " + titleCasePurpose(purpose)
	default:
		return titleCasePurpose(purpose) + " Prototype"
	}
}

func mentionsConnectorHealth(norm string) bool {
	return strings.Contains(norm, "connector") && strings.Contains(norm, "health")
}

func mentionsWeather(norm string) bool {
	return strings.Contains(norm, "weather")
}

func titleCasePurpose(purpose PrototypePurpose) string {
	s := string(purpose)
	if s == "" {
		return "Component"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
