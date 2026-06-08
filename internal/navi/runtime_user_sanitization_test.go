package navi

import "testing"

func TestSanitizeRuntimeUserFacingReply_HidesInternalICSReason(t *testing.T) {
	raw := "I couldn't continue because ExecutionIntent authorized capabilities [GitDiffTool, GitCommitTool] but the model returned no executable tool call."
	got := sanitizeRuntimeUserFacingReply(raw)
	if got != runtimeGovernanceBlockedUserReply {
		t.Fatalf("got %q, want %q", got, runtimeGovernanceBlockedUserReply)
	}
}

func TestSanitizeRuntimeUserFacingReply_PreservesOrdinaryReplies(t *testing.T) {
	raw := "I couldn't continue because the connector is offline."
	got := sanitizeRuntimeUserFacingReply(raw)
	if got != raw {
		t.Fatalf("got %q, want unchanged %q", got, raw)
	}
}

func TestSanitizeRuntimeUserFacingReply_HidesRawToolCallMarkup(t *testing.T) {
	raw := `<tool_call>write_file(path="skills/check_disk_usage.py", content="print('disk')")</tool_call>`
	got := sanitizeRuntimeUserFacingReply(raw)
	if got == raw || runtimeReplyContainsToolCallMarkup(got) {
		t.Fatalf("expected raw tool call markup to be hidden, got %q", got)
	}
}

func TestShapeReply_SanitizesInternalRuntimeBlockBeforeDelivery(t *testing.T) {
	loop := NewAgentLoop(LoopConfig{})
	raw := "I couldn't continue because ICS did not provide an authoritative executable governance handoff for this execution intent."
	got := loop.shapeReply(raw, ExperienceModeStandard)
	if got != runtimeGovernanceBlockedUserReply {
		t.Fatalf("got %q, want %q", got, runtimeGovernanceBlockedUserReply)
	}
}
