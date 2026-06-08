package render

import "strings"

// CapabilityPrototype names the prototype render capability (mirrors
// CapabilityToolUsage for the data-render lane).
const CapabilityPrototype = "prototype"

// unconditionalPrototypePhrases are unambiguous "make me a mockup" cues. When any
// of these appear, the request routes to a prototype render on its own (even inside
// a question: "What would a dashboard look like? Mock it up."). Matched as substrings
// against the normalized (lowercased, punctuation-stripped, single-spaced) text.
var unconditionalPrototypePhrases = []string{
	"mock up", "mock it up", "mockup",
	"wireframe", "visual demo", "interactive demo", "show me a ui",
}

// contextualPrototypePhrases are softer cues that only trigger when ALSO paired with
// a UI-surface noun. This prevents hijacking ordinary chat such as "what would a
// normal day look like?", "turn this into a short summary", or "render this as
// markdown" — none of which name a UI surface.
var contextualPrototypePhrases = []string{
	"show me what", "what would a", "could look like",
	"render this as", "render as", "turn this into", "turn into",
}

// prototypeBuildCues are build/create verbs. NOTE: "show" is deliberately NOT a
// cue — it is far too broad and would hijack ordinary information requests
// ("show me the weather"). "show" only triggers via a prototype phrase.
var prototypeBuildCues = []string{
	"create", "build", "make", "mock", "prototype", "wireframe", "design", "generate",
}

// prototypeActionSubjects are action-on-tools terms ("approve", "review", …). When a
// build/create cue is paired with one of these, the request is a UI mockup ABOUT an
// action workflow ("build a dashboard for approving tool calls"), not a request to
// visualize real tool-usage telemetry — so the data-render classifier defers to the
// prototype lane (see Classify). Used by the data classifier, not here.
var prototypeActionSubjects = []string{
	"approve", "approving", "approval", "approvals",
	"review", "reviewing", "reviewed", "reviews",
	"manage", "managing", "moderate", "moderation",
}

// prototypeSurfaceNouns are UI-surface nouns. A request routes to a prototype only
// when one of these is paired with a build cue or a contextual phrase (or an
// unconditional phrase is present). "weather" counts only here — paired with an
// explicit build cue/phrase — so ordinary "what's the weather" questions never
// produce placeholder weather data.
var prototypeSurfaceNouns = []string{
	"dashboard", "mockup", "prototype", "wireframe", "demo", "component",
	"panel", "ui", "display", "cards", "timeline", "inspector", "visual",
	"weather", "settings", "status", "approval", "memory",
}

// ClassifyPrototype routes a user message to RenderIntentOpenUIPrototypeRender when
// it clearly asks NAVI to materialize a visual prototype/mockup, using narrow,
// deterministic heuristics. It is intentionally conservative so it does not hijack
// ordinary chat: an explanatory question ("What is connector health?", "What's the
// weather today?") carries neither a build cue nor a prototype phrase and stays plain.
//
// Three trigger paths:
//
//	(a) an unconditional prototype phrase is present (e.g. "mock up", "wireframe"); OR
//	(b) a contextual phrase (e.g. "could look like", "render this as") paired with a
//	    UI-surface noun; OR
//	(c) a build cue token AND a surface-noun token both appear.
//
// Precedence: callers check the data-render Classify FIRST (it keeps priority);
// prototype is the fallthrough. A request like "build me a component for approving
// tool calls" does not match the data classifier (no visual token) and so routes
// here correctly.
func ClassifyPrototype(raw string) Classification {
	plain := Classification{Intent: RenderIntentPlainResponse}
	norm := normalize(raw)
	if norm == "" {
		return plain
	}
	if !prototypeTriggered(norm) {
		return plain
	}
	return Classification{
		Intent:        RenderIntentOpenUIPrototypeRender,
		Capability:    CapabilityPrototype,
		PreferredView: string(prototypePurpose(norm)),
	}
}

func prototypeTriggered(norm string) bool {
	for _, phrase := range unconditionalPrototypePhrases {
		if strings.Contains(norm, phrase) {
			return true
		}
	}
	hasSurface := containsAny(norm, prototypeSurfaceNouns)
	for _, phrase := range contextualPrototypePhrases {
		if hasSurface && strings.Contains(norm, phrase) {
			return true
		}
	}
	return hasSurface && containsAny(norm, prototypeBuildCues)
}

// prototypePurpose maps the phrasing to a NaviUIPrototype purpose. Order is
// load-bearing: "dashboard" wins over "mockup" so "create a dashboard mockup" is a
// dashboard; weather/display wins over the generic demo/mockup fallback. The result
// is re-sanitized by NormalizePrototypePurpose on the construction path.
func prototypePurpose(norm string) PrototypePurpose {
	switch {
	case strings.Contains(norm, "dashboard"):
		return PurposeDashboard
	case strings.Contains(norm, "inspector"):
		return PurposeInspector
	case strings.Contains(norm, "panel") ||
		strings.Contains(norm, "settings") ||
		strings.Contains(norm, "status") ||
		strings.Contains(norm, "approval") ||
		strings.Contains(norm, "memory"):
		return PurposePanel
	case strings.Contains(norm, "weather") ||
		strings.Contains(norm, "display") ||
		strings.Contains(norm, "timeline"):
		return PurposeDisplay
	case strings.Contains(norm, "workflow"):
		return PurposeWorkflow
	case strings.Contains(norm, "wireframe") ||
		strings.Contains(norm, "mockup") ||
		strings.Contains(norm, "demo"):
		return PurposeDemo
	default:
		return PurposeComponent
	}
}
