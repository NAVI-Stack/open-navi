package render

// BuildToolUsagePayload assembles the full chat render payload for a tool-usage
// request: the canonical NaviDataView, a generated OpenUI Lang program (primary
// render lane), and fallback markdown (always present). Mode is "openui"; the
// console renders the OpenUI lane and falls back to FallbackMarkdown if OpenUI
// parsing/rendering fails or is unavailable.
//
// events is the real, chat-scoped data (may be empty → honest degraded view).
func BuildToolUsagePayload(events []ToolEvent, preferredView string, trace Trace) *RenderPayload {
	view := BuildToolUsageDataView(events, preferredView, trace)
	return &RenderPayload{
		Mode:             RenderModeOpenUI,
		OpenUILang:       GenerateOpenUILang(view),
		DataView:         view,
		FallbackMarkdown: view.Fallback.Markdown,
		TraceID:          trace.RunID,
	}
}
