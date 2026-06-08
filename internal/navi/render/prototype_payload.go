package render

// BuildPrototypePayload assembles the full chat render payload for a generated-UI
// prototype request: the canonical NaviUIPrototype, a generated OpenUI Lang program
// (primary render lane), and honest fallback markdown (always present). Mode is
// "openui" so the console renders it through the SAME OpenUILane as the data-render
// slice and falls back to FallbackMarkdown if OpenUI parsing/rendering fails or is
// unavailable. DataSourceKind is "placeholder" so clients can show an honest label.
//
// No real data is read; the prototype is always placeholder and read-only. The
// payload carries no privileged actions.
func BuildPrototypePayload(purpose PrototypePurpose, prompt string, trace PrototypeTrace) *RenderPayload {
	proto := BuildPrototype(purpose, prompt, trace)
	return &RenderPayload{
		Mode:             RenderModeOpenUI,
		OpenUILang:       GeneratePrototypeOpenUILang(proto),
		Prototype:        proto,
		DataSourceKind:   DataSourcePlaceholder,
		FallbackMarkdown: proto.FallbackMarkdown,
		TraceID:          trace.RunID,
		// DataView is intentionally nil — a prototype has no real telemetry.
	}
}
