package render

import (
	"fmt"
	"strings"
)

// prototypeBanner is the honest header shown on every prototype fallback. It must
// make clear the render is a prototype with placeholder data, is not live, is not
// saved, and is not connected to real systems.
const prototypeBanner = "> **Prototype with placeholder data.** This is a mockup, not live data. " +
	"Nothing here is saved or connected to real systems."

// PrototypeFallbackMarkdown renders an honest markdown representation of a
// prototype so a failed or unavailable OpenUI render still gives the user a
// useful, clearly-labeled mockup. It always opens with prototypeBanner.
func PrototypeFallbackMarkdown(p *NaviUIPrototype) string {
	var b strings.Builder
	title := "UI Prototype"
	if p != nil && strings.TrimSpace(p.Title) != "" {
		title = p.Title
	}
	fmt.Fprintf(&b, "### %s (prototype)\n\n", title)
	b.WriteString(prototypeBanner)
	b.WriteString("\n\n")

	if p != nil && p.Purpose == PurposeDashboard && mentionsConnectorHealth(normalize(p.Prompt)) {
		writeConnectorHealthFallback(&b)
	} else {
		writeGenericPrototypeFallback(&b, p)
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeConnectorHealthFallback(b *strings.Builder) {
	b.WriteString("A connector health dashboard would summarize each connector's status. " +
		"Example layout (placeholder values):\n\n")
	b.WriteString("| Connector | Status | Last check | Latency |\n")
	b.WriteString("| --- | --- | --- | ---: |\n")
	for _, r := range placeholderConnectorRows() {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", r.name, r.status, r.lastCheck, r.latency)
	}
	b.WriteString("\n_Placeholder data — no live connector health is wired yet._")
}

func writeGenericPrototypeFallback(b *strings.Builder, p *NaviUIPrototype) {
	purpose := PurposeComponent
	if p != nil {
		purpose = p.Purpose
	}
	fmt.Fprintf(b, "This is a %s mockup. It illustrates structure and layout only — "+
		"every value below is placeholder data.\n\n", string(purpose))
	for _, line := range placeholderOutline(purpose) {
		fmt.Fprintf(b, "- %s\n", line)
	}
	b.WriteString("\n_Placeholder data — this prototype is not wired to any real source._")
}

// placeholderOutline returns a small, purpose-shaped outline of what a real
// component would contain. Values are illustrative placeholders only.
func placeholderOutline(purpose PrototypePurpose) []string {
	switch purpose {
	case PurposeDashboard:
		return []string{
			"Headline metrics (e.g. Total: 6, Healthy: 4, Attention: 2) — placeholder",
			"A status table of items with state badges — placeholder",
			"A short summary line — placeholder",
		}
	case PurposePanel:
		return []string{
			"A titled panel with a few labeled fields — placeholder",
			"Status badges for each row — placeholder",
			"Inert prototype controls (no actions wired)",
		}
	case PurposeDisplay:
		return []string{
			"A headline value with supporting metrics — placeholder",
			"A few summary cards — placeholder",
		}
	case PurposeInspector:
		return []string{
			"A key/value detail list — placeholder",
			"A status badge for the inspected item — placeholder",
		}
	default:
		return []string{
			"A titled card with sample content — placeholder",
			"One or two metric/status elements — placeholder",
			"Inert prototype controls (no actions wired)",
		}
	}
}

// placeholderConnectorRow is one fake connector row for the proving-slice dashboard.
type placeholderConnectorRow struct {
	name      string
	status    string
	lastCheck string
	latency   string
}

func placeholderConnectorRows() []placeholderConnectorRow {
	return []placeholderConnectorRow{
		{"Slack", "Healthy", "2m ago", "120ms"},
		{"GitHub", "Healthy", "1m ago", "90ms"},
		{"Gmail", "Degraded", "5m ago", "640ms"},
		{"Telegram", "Healthy", "3m ago", "150ms"},
	}
}
