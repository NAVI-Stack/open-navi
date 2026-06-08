package render

import (
	"fmt"
	"strconv"
	"strings"
)

// GeneratePrototypeOpenUILang deterministically emits an OpenUI Lang program that
// renders a NaviUIPrototype through NAVI's existing OpenUI vocabulary (Card,
// Badge, Text, MetricCard, DataTable/Row/Cell). OpenUI Lang is one render target
// derived from the canonical prototype — it is never the source of truth.
//
// The honesty label is embedded in the PROGRAM ITSELF: the first two children are
// always a Badge("PROTOTYPE") and a warning Text, so the "prototype / placeholder
// data" label survives even if a future renderer drops other components. It mirrors
// GenerateOpenUILang's conventions (one statement per line, positional args,
// `root = Card(...)` first so the shell streams in before its children).
func GeneratePrototypeOpenUILang(p *NaviUIPrototype) string {
	if p == nil {
		return ""
	}
	b := &lang{}

	// Always-present honesty chrome.
	b.stmt("proto", fmt.Sprintf("Badge(%s, %s)", q("PROTOTYPE"), q("warning")))
	b.stmt("banner", fmt.Sprintf("Text(%s, %s)",
		q("Prototype with placeholder data — not live data, not saved."), q("warning")))

	var childIDs []string
	if p.Purpose == PurposeDashboard && mentionsConnectorHealth(normalize(p.Prompt)) {
		childIDs = generateConnectorHealthBody(b)
	} else {
		childIDs = generateGenericPrototypeBody(b, p)
	}

	children := append([]string{"proto", "banner"}, childIDs...)
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = "UI Prototype"
	}
	root := fmt.Sprintf("root = Card(%s, %s)", q(title), arr(children))
	return root + "\n" + b.String()
}

// generateConnectorHealthBody emits the proving-slice dashboard: metric cards +
// a status table, all from clearly-labeled placeholder data.
func generateConnectorHealthBody(b *lang) []string {
	rows := placeholderConnectorRows()
	healthy := 0
	for _, r := range rows {
		if strings.EqualFold(r.status, "Healthy") {
			healthy++
		}
	}
	degraded := len(rows) - healthy

	b.stmt("m1", fmt.Sprintf("MetricCard(%s, %s, %s)",
		q("Connectors"), q(strconv.Itoa(len(rows))), q("placeholder")))
	b.stmt("m2", fmt.Sprintf("MetricCard(%s, %s, %s)",
		q("Healthy"), q(strconv.Itoa(healthy)), q("placeholder")))
	b.stmt("m3", fmt.Sprintf("MetricCard(%s, %s, %s)",
		q("Degraded"), q(strconv.Itoa(degraded)), q("placeholder")))

	cellID := 0
	nextCell := func(value string) string {
		id := fmt.Sprintf("c%d", cellID)
		cellID++
		b.stmt(id, fmt.Sprintf("Cell(%s)", q(value)))
		return id
	}
	header := []string{nextCell("Connector"), nextCell("Status"), nextCell("Last check"), nextCell("Latency")}
	b.stmt("head", fmt.Sprintf("Row(%s)", arr(header)))

	rowIDs := make([]string, 0, len(rows))
	for i, r := range rows {
		cells := []string{nextCell(r.name), nextCell(r.status), nextCell(r.lastCheck), nextCell(r.latency)}
		rid := fmt.Sprintf("r%d", i)
		rowIDs = append(rowIDs, rid)
		b.stmt(rid, fmt.Sprintf("Row(%s)", arr(cells)))
	}
	b.stmt("table", fmt.Sprintf("DataTable(%s, head, %s)", q("Connector status (placeholder)"), arr(rowIDs)))

	return []string{"m1", "m2", "m3", "table"}
}

// generateGenericPrototypeBody emits a small, purpose-shaped placeholder scaffold
// for non-dashboard prototypes (component/panel/display/demo/etc.).
func generateGenericPrototypeBody(b *lang, p *NaviUIPrototype) []string {
	purpose := string(p.Purpose)
	if purpose == "" {
		purpose = string(PurposeComponent)
	}
	b.stmt("desc", fmt.Sprintf("Text(%s, %s)",
		q(fmt.Sprintf("This %s mockup shows structure and layout with placeholder data.", purpose)),
		q("secondary")))
	b.stmt("g1", fmt.Sprintf("MetricCard(%s, %s, %s)", q("Example metric"), q("42"), q("placeholder")))
	b.stmt("g2", fmt.Sprintf("MetricCard(%s, %s, %s)", q("Another metric"), q("7"), q("placeholder")))
	b.stmt("note", fmt.Sprintf("Text(%s, %s)",
		q("Controls shown in a real version would appear here; this prototype is inert."), q("tertiary")))
	return []string{"desc", "g1", "g2", "note"}
}
