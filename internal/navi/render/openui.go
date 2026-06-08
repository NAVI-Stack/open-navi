package render

import (
	"fmt"
	"strconv"
	"strings"
)

// GenerateOpenUILang deterministically emits an OpenUI Lang program that renders
// the given tool-usage NaviDataView through NAVI's OpenUI vocabulary
// (Card → MetricCard + UsageChart + DataTable). OpenUI Lang is one render target
// derived from the canonical view — it is never the source of truth.
//
// The grammar follows NAVI's vocabulary (see web-src/.../openui/library.ts):
// one statement per line, positional arguments, `root = Card(...)` first so the
// shell streams in before its children. Strings are emitted quoted; collections
// are arrays of statement identifiers.
func GenerateOpenUILang(view *NaviDataView) string {
	if view == nil {
		return ""
	}
	b := &lang{}

	rows := view.Dataset.Rows
	total := 0
	for _, r := range rows {
		total += asInt(r["count"])
	}

	// Metric card.
	metricSub := fmt.Sprintf("across %d tool%s", len(rows), plural(len(rows)))
	b.stmt("metric", fmt.Sprintf("MetricCard(%s, %s, %s)",
		q("Total tool calls"), q(strconv.Itoa(total)), q(metricSub)))

	// Usage chart: one Bar per tool.
	barIDs := make([]string, 0, len(rows))
	for i, r := range rows {
		id := fmt.Sprintf("b%d", i)
		barIDs = append(barIDs, id)
		b.stmt(id, fmt.Sprintf("Bar(%s, %s)", q(str(r["tool"])), q(strconv.Itoa(asInt(r["count"])))))
	}
	b.stmt("chart", fmt.Sprintf("UsageChart(%s, %s)", q("Calls per tool"), arr(barIDs)))

	// Data table: header row + a row per tool, cells as Cell() elements.
	cellID := 0
	nextCell := func(value string) string {
		id := fmt.Sprintf("c%d", cellID)
		cellID++
		b.stmt(id, fmt.Sprintf("Cell(%s)", q(value)))
		return id
	}
	headerCells := []string{
		nextCell("Tool"), nextCell("Calls"), nextCell("Failed"), nextCell("Last used"),
	}
	b.stmt("head", fmt.Sprintf("Row(%s)", arr(headerCells)))

	rowIDs := make([]string, 0, len(rows))
	for i, r := range rows {
		last := str(r["lastUsed"])
		if strings.TrimSpace(last) == "" {
			last = "—"
		}
		cells := []string{
			nextCell(str(r["tool"])),
			nextCell(strconv.Itoa(asInt(r["count"]))),
			nextCell(strconv.Itoa(asInt(r["failures"]))),
			nextCell(last),
		}
		rid := fmt.Sprintf("r%d", i)
		rowIDs = append(rowIDs, rid)
		b.stmt(rid, fmt.Sprintf("Row(%s)", arr(cells)))
	}
	b.stmt("table", fmt.Sprintf("DataTable(%s, head, %s)", q("Details"), arr(rowIDs)))

	// Root first (prepended) so streaming renders the shell before children.
	title := view.Title
	if strings.TrimSpace(title) == "" {
		title = "Tool usage"
	}
	root := fmt.Sprintf("root = Card(%s, %s)", q(title), arr([]string{"metric", "chart", "table"}))
	return root + "\n" + b.String()
}

// lang accumulates OpenUI Lang statements in declaration order.
type lang struct {
	lines []string
}

func (l *lang) stmt(id, expr string) {
	l.lines = append(l.lines, id+" = "+expr)
}

func (l *lang) String() string {
	return strings.Join(l.lines, "\n")
}

// q quotes a string as an OpenUI Lang string literal, escaping backslashes and
// double quotes so generated programs stay parseable.
func q(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}

// arr renders a positional array of statement identifiers: [a, b, c].
func arr(ids []string) string {
	return "[" + strings.Join(ids, ", ") + "]"
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
