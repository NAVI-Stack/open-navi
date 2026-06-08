package render

import "strings"

// Classification is the result of deterministic render-intent routing.
type Classification struct {
	Intent RenderIntent
	// Capability names the data capability to invoke (e.g. "tool_usage") when
	// Intent is a data render. Empty otherwise.
	Capability string
	// PreferredView is the data-view intent the phrasing implies
	// (chart | table | timeline | dashboard). Defaults to "chart".
	PreferredView string
}

// CapabilityToolUsage is the only data capability wired for the first slice.
const CapabilityToolUsage = "tool_usage"

// visualIntentTokens are explicit data-driven presentation cues. A request only
// routes to OpenUI when one of these is present — plain "what tools did you use"
// questions stay prose/markdown.
var visualIntentTokens = []string{
	"graph", "chart", "charts", "visualize", "visualise", "visualization",
	"visualisation", "plot", "dashboard", "timeline", "table", "diagram",
}

// toolUsageIndicators are the usage-subject cues that, combined with the word
// "tool", indicate the request is about tool usage in this conversation.
var toolUsageIndicators = []string{
	"usage", "used", "use", "call", "calls", "invocation", "invocations", "activity",
}

// Classify routes a user message to a RenderIntent using narrow, deterministic
// heuristics. It is intentionally conservative: it only returns
// RenderIntentOpenUIDataRender when the message clearly asks to *visualize* tool
// usage (a tool-usage subject AND an explicit visual/data-render cue). This is a
// clean seam for model-assisted routing later — do not broaden it here.
//
// Triggers:   "show me a graph of tool usage in this chat",
//
//	"visualize tool calls in this conversation",
//	"make a table of tools used here".
//
// Does NOT trigger (stays prose/markdown):
//
//	"what tools have you used lately?",
//	"which tools did you use in this chat?".
func Classify(raw string) Classification {
	plain := Classification{Intent: RenderIntentPlainResponse}
	norm := normalize(raw)
	if norm == "" {
		return plain
	}
	if !containsAny(norm, visualIntentTokens) {
		return plain
	}
	if !mentionsTool(norm) || !containsAny(norm, toolUsageIndicators) {
		return plain
	}
	// Guard against prototype build requests that merely mention "tool calls". A
	// request that pairs a build/create cue with an action-on-tools subject (e.g.
	// "build me a dashboard for approving tool calls", "mock up a tool-call approval
	// dashboard") is a UI mockup, not a request to visualize real tool-usage
	// telemetry — defer it to the prototype lane. Genuine "visualize tool usage in
	// this chat" requests carry no build-cue+action-subject pairing and still match.
	if containsAny(norm, prototypeBuildCues) && containsAny(norm, prototypeActionSubjects) {
		return plain
	}
	return Classification{
		Intent:        RenderIntentOpenUIDataRender,
		Capability:    CapabilityToolUsage,
		PreferredView: preferredView(norm),
	}
}

func preferredView(norm string) string {
	switch {
	case strings.Contains(norm, "timeline"):
		return "timeline"
	case strings.Contains(norm, "dashboard"):
		return "dashboard"
	case strings.Contains(norm, "table") &&
		!containsAny(norm, []string{"graph", "chart", "visualize", "visualise", "plot", "diagram"}):
		return "table"
	default:
		return "chart"
	}
}

// mentionsTool matches the standalone word "tool"/"tools" (not substrings like
// "toolkit") so unrelated requests don't trip the router.
func mentionsTool(norm string) bool {
	for _, w := range strings.Fields(norm) {
		if w == "tool" || w == "tools" {
			return true
		}
	}
	return false
}

func containsAny(norm string, tokens []string) bool {
	for _, w := range strings.Fields(norm) {
		for _, t := range tokens {
			if w == t {
				return true
			}
		}
	}
	return false
}

// normalize lowercases, replaces punctuation with spaces, and collapses runs of
// whitespace so token matching is stable.
func normalize(raw string) string {
	replacer := strings.NewReplacer(
		"\r", " ", "\n", " ", "\t", " ",
		"?", " ", "!", " ", ".", " ", ",", " ", ":", " ", ";", " ",
		"'", "", "\"", " ", "(", " ", ")", " ", "-", " ", "/", " ",
	)
	return strings.Join(strings.Fields(strings.ToLower(replacer.Replace(strings.TrimSpace(raw)))), " ")
}
