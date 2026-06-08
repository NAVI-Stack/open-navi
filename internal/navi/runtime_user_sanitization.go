package navi

import "strings"

const runtimeGovernanceBlockedUserReply = "I couldn't safely continue with that action. The runtime blocked it under NAVI's governance rules."
const runtimeToolCallMarkupLeakReply = "I tried to express a tool call as plain text, so I stopped before sending unexecuted tool markup. This request needs a tool-capable model or a surfaced executable tool."

// sanitizeRuntimeUserFacingReply removes runtime/ICS implementation details from
// text that may be delivered to a user. Internal reasons remain available in
// persisted inference envelopes, validation results, traces, snapshots, and logs.
func sanitizeRuntimeUserFacingReply(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	if runtimeReplyContainsToolCallMarkup(trimmed) {
		return runtimeToolCallMarkupLeakReply
	}

	const blockedPrefix = "I couldn't continue because "
	if strings.HasPrefix(strings.ToLower(trimmed), strings.ToLower(blockedPrefix)) {
		reason := strings.TrimSpace(trimmed[len(blockedPrefix):])
		reason = strings.TrimSuffix(reason, ".")
		if runtimeReasonLooksInternal(reason) {
			return runtimeGovernanceBlockedUserReply
		}
	}

	if runtimeReasonLooksInternal(trimmed) {
		return runtimeGovernanceBlockedUserReply
	}
	return raw
}

func runtimeReplyContainsToolCallMarkup(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "<tool_call") || strings.Contains(lower, "</tool_call>")
}

func runtimeReasonLooksInternal(reason string) bool {
	lower := strings.ToLower(strings.TrimSpace(reason))
	if lower == "" {
		return false
	}

	internalMarkers := []string{
		"executionintent",
		"execution intent",
		"executionintent authorized capabilities",
		"model returned no executable tool call",
		"no executable tool call",
		"required authorized capability",
		"ics ",
		"ics:",
		"ics did",
		"ics could",
		"toolcontract",
		"tool contract",
		"toolpermit",
		"modeldirective",
		"executionboundary",
		"targetcapability",
		"allowedcapabilities",
		"authorized capabilities",
		"authoritative executable",
		"governance handoff",
		"resolved execution metadata",
		"runtime disposition",
		"fail-closed",
		"fail closed",
		"control boundary",
		"could not safely surface",
		"surfaced tool",
		"validationdependency",
	}
	for _, marker := range internalMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
