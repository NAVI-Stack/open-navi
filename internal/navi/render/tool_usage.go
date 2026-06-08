package render

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// toolUsageRow is the internal aggregation accumulator for one tool.
type toolUsageRow struct {
	name      string
	count     int
	successes int
	failures  int
	lastUsed  time.Time
}

// BuildToolUsageDataView aggregates tool invocations for the current chat into a
// renderer-neutral NaviDataView (intent "chart" by default). The data is REAL —
// it comes from persisted assistant-message toolParts. When there are no events
// it returns an honest empty/degraded view (no fabricated rows) whose fallback
// explains that no tool usage has been recorded yet.
//
// preferredView selects the data-view intent ("chart" | "table" | "timeline" |
// "dashboard"); pass "" for the default chart.
func BuildToolUsageDataView(events []ToolEvent, preferredView string, trace Trace) *NaviDataView {
	intent := NormalizePreferredView(preferredView)

	agg := map[string]*toolUsageRow{}
	order := []string{}
	total := 0
	for _, ev := range events {
		name := strings.TrimSpace(ev.ToolName)
		if name == "" {
			name = "(unknown)"
		}
		row, ok := agg[name]
		if !ok {
			row = &toolUsageRow{name: name}
			agg[name] = row
			order = append(order, name)
		}
		row.count++
		total++
		if ev.IsError {
			row.failures++
		} else {
			row.successes++
		}
		if ev.UsedAt.After(row.lastUsed) {
			row.lastUsed = ev.UsedAt
		}
	}

	// Sort by invocation count desc, then name asc for stable presentation.
	sort.SliceStable(order, func(i, j int) bool {
		a, b := agg[order[i]], agg[order[j]]
		if a.count != b.count {
			return a.count > b.count
		}
		return a.name < b.name
	})

	columns := []DataColumn{
		{Key: "tool", Label: "Tool", Type: ColumnString},
		{Key: "count", Label: "Calls", Type: ColumnNumber},
		{Key: "successes", Label: "Succeeded", Type: ColumnNumber},
		{Key: "failures", Label: "Failed", Type: ColumnNumber},
		{Key: "lastUsed", Label: "Last used", Type: ColumnDatetime},
	}
	rows := make([]map[string]any, 0, len(order))
	for _, name := range order {
		r := agg[name]
		row := map[string]any{
			"tool":      r.name,
			"count":     r.count,
			"successes": r.successes,
			"failures":  r.failures,
		}
		if !r.lastUsed.IsZero() {
			row["lastUsed"] = r.lastUsed.UTC().Format(time.RFC3339)
		} else {
			row["lastUsed"] = nil
		}
		rows = append(rows, row)
	}

	view := &NaviDataView{
		ID:      "tool-usage",
		Title:   "Tool usage in this chat",
		Intent:  intent,
		Dataset: Dataset{Columns: columns, Rows: rows},
		SuggestedViews: []SuggestedView{
			{Type: "bar", Rationale: "Compare invocation counts across tools."},
			{Type: "table", Rationale: "Inspect exact counts and last-used times."},
		},
		Governance: Governance{
			DataSensitivity: "local",
			AllowedActions:  []string{}, // read-only: generated UI may not invoke actions
			AuditLevel:      "basic",
		},
		Trace: trace,
	}
	view.Fallback = Fallback{
		Markdown: FallbackMarkdown(view, total),
		Summary:  fmt.Sprintf("%d tool call%s across %d tool%s.", total, plural(total), len(order), plural(len(order))),
	}
	return view
}

// NormalizePreferredView constrains model-provided view hints to the canonical
// NaviDataView intents supported by the render slice.
func NormalizePreferredView(preferredView string) string {
	switch strings.ToLower(strings.TrimSpace(preferredView)) {
	case "chart", "table", "dashboard", "timeline":
		return strings.ToLower(strings.TrimSpace(preferredView))
	default:
		return "chart"
	}
}

// FallbackMarkdown renders a useful markdown summary/table of the data view so a
// failed or unavailable OpenUI render still gives the user the data. total is the
// total number of tool calls (sum of row counts).
func FallbackMarkdown(view *NaviDataView, total int) string {
	var b strings.Builder
	b.WriteString("### Tool usage in this chat\n\n")
	if view == nil || len(view.Dataset.Rows) == 0 {
		b.WriteString("No tool usage has been recorded in this chat yet.")
		return b.String()
	}
	fmt.Fprintf(&b, "**Total tool calls:** %d\n\n", total)
	b.WriteString("| Tool | Calls | Failed | Last used |\n")
	b.WriteString("| --- | ---: | ---: | --- |\n")
	for _, row := range view.Dataset.Rows {
		name, _ := row["tool"].(string)
		count := asInt(row["count"])
		failed := asInt(row["failures"])
		last := "—"
		if s, ok := row["lastUsed"].(string); ok && strings.TrimSpace(s) != "" {
			last = s
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %s |\n", name, count, failed, last)
	}
	return strings.TrimRight(b.String(), "\n")
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
