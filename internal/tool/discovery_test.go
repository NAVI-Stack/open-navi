package tool

import "testing"

func TestToolDiscoveryExactHitIsExplicitAndUsable(t *testing.T) {
	reg := NewRegistry()
	readTool := validTestTool("navi.files.read")
	readTool.DisplayName = "Read File"
	readTool.Aliases = []string{"read file", "open file"}
	if err := reg.Register(readTool); err != nil {
		t.Fatalf("register read tool: %v", err)
	}

	result := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeExact,
		Text: "navi.files.read",
		Context: DiscoveryContext{
			Environment: "development",
		},
	})

	if result.QueryID == "" {
		t.Fatal("expected discovery query id")
	}
	if result.ExactMatch == nil || result.ExactMatch.Tool == nil {
		t.Fatalf("expected exact match, got %+v", result)
	}
	if result.ExactMatch.Tool.ToolID != "navi.files.read" {
		t.Fatalf("unexpected exact match tool: %#v", result.ExactMatch.Tool)
	}
	if result.AvailabilityStatus != DiscoveryAvailabilityAvailable {
		t.Fatalf("expected available exact hit, got %s", result.AvailabilityStatus)
	}
	if result.SuggestedNextAction != DiscoveryNextActionUseExactMatch {
		t.Fatalf("expected use_exact_match suggestion, got %s", result.SuggestedNextAction)
	}
	if result.ExplicitMiss {
		t.Fatal("exact hit should not be marked as explicit miss")
	}
}

func TestToolDiscoveryExactMissReturnsStructuredRelatedMatch(t *testing.T) {
	reg := NewRegistry()
	readTool := validTestTool("navi.files.read")
	readTool.DisplayName = "Read File"
	readTool.Description = "Read a file from the workspace."
	readTool.Aliases = []string{"read file", "open file"}
	if err := reg.Register(readTool); err != nil {
		t.Fatalf("register read tool: %v", err)
	}

	result := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeExact,
		Text: "navi.files.open",
		Context: DiscoveryContext{
			Environment: "development",
		},
	})

	if result.ExactMatch != nil {
		t.Fatalf("expected explicit miss, got exact match %#v", result.ExactMatch)
	}
	if !result.ExplicitMiss {
		t.Fatal("expected explicit miss on unknown tool id")
	}
	if result.AvailabilityStatus != DiscoveryAvailabilityRelatedOnly {
		t.Fatalf("expected related_only status, got %s", result.AvailabilityStatus)
	}
	if result.UnavailableReasonCode != DiscoveryReasonExactMiss {
		t.Fatalf("expected exact_miss reason, got %s", result.UnavailableReasonCode)
	}
	if result.SuggestedNextAction != DiscoveryNextActionInspectRelated {
		t.Fatalf("expected inspect_related_matches suggestion, got %s", result.SuggestedNextAction)
	}
	if len(result.RelatedMatches) == 0 || result.RelatedMatches[0].Tool == nil || result.RelatedMatches[0].Tool.ToolID != "navi.files.read" {
		t.Fatalf("expected read tool as related candidate, got %+v", result.RelatedMatches)
	}
	if result.MissingCapability != nil {
		t.Fatalf("did not expect missing capability envelope when related matches exist, got %+v", result.MissingCapability)
	}
}

func TestToolDiscoveryLexicalSearchProducesRelatedMatches(t *testing.T) {
	reg := NewRegistry()
	replyTool := validTestTool("navi.messaging.send_reply")
	replyTool.DisplayName = "Send Reply"
	replyTool.Description = "Send a reply to the owner."
	replyTool.Aliases = []string{"send reply"}
	replyTool.Category = ToolCategoryWorkflowAction
	if err := reg.Register(replyTool); err != nil {
		t.Fatalf("register reply tool: %v", err)
	}

	result := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeLexical,
		Text: "send reply",
		Context: DiscoveryContext{
			Environment: "development",
		},
	})

	if result.ExactMatch != nil {
		t.Fatalf("lexical search should not emit exact match, got %#v", result.ExactMatch)
	}
	if result.AvailabilityStatus != DiscoveryAvailabilityRelatedOnly {
		t.Fatalf("expected related_only lexical result, got %s", result.AvailabilityStatus)
	}
	if len(result.RelatedMatches) == 0 || result.RelatedMatches[0].Tool == nil || result.RelatedMatches[0].Tool.ToolID != "navi.messaging.send_reply" {
		t.Fatalf("expected send_reply lexical candidate, got %+v", result.RelatedMatches)
	}
}

func TestToolDiscoveryHiddenToolReturnsKnownUnavailableReason(t *testing.T) {
	reg := NewRegistry()
	hiddenTool := validTestTool("navi.internal.hidden")
	hiddenTool.DisplayName = "Hidden Tool"
	hiddenTool.Hidden = true
	if err := reg.Register(hiddenTool); err != nil {
		t.Fatalf("register hidden tool: %v", err)
	}

	result := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeExact,
		Text: "navi.internal.hidden",
		Context: DiscoveryContext{
			Environment: "development",
			SessionMode: DiscoverySessionModeDebug,
			Authority:   ToolAuthorityOwner,
		},
	})

	if result.ExactMatch == nil || result.ExactMatch.Tool == nil {
		t.Fatalf("expected known hidden tool, got %+v", result)
	}
	if result.AvailabilityStatus != DiscoveryAvailabilityHidden {
		t.Fatalf("expected hidden status, got %s", result.AvailabilityStatus)
	}
	if result.UnavailableReasonCode != DiscoveryReasonHidden {
		t.Fatalf("expected hidden reason code, got %s", result.UnavailableReasonCode)
	}
	if result.SuggestedNextAction != DiscoveryNextActionReviewUnavailable {
		t.Fatalf("expected review_unavailable_tool suggestion, got %s", result.SuggestedNextAction)
	}
}

func TestToolDiscoveryMissingCapabilityEnvelopeOnStructuredMiss(t *testing.T) {
	reg := NewRegistry()

	result := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeExact,
		Text: "navi.diagnostics.session_errors",
	})

	if result.ExactMatch != nil {
		t.Fatalf("expected no exact match, got %#v", result.ExactMatch)
	}
	if len(result.RelatedMatches) != 0 {
		t.Fatalf("expected no related matches, got %+v", result.RelatedMatches)
	}
	if result.AvailabilityStatus != DiscoveryAvailabilityExplicitMiss {
		t.Fatalf("expected explicit_miss status, got %s", result.AvailabilityStatus)
	}
	if result.UnavailableReasonCode != DiscoveryReasonNoMatch {
		t.Fatalf("expected no_match reason, got %s", result.UnavailableReasonCode)
	}
	if result.SuggestedNextAction != DiscoveryNextActionRequestCapability {
		t.Fatalf("expected request_missing_capability suggestion, got %s", result.SuggestedNextAction)
	}
	if result.MissingCapability == nil {
		t.Fatal("expected missing capability envelope")
	}
	if result.MissingCapability.QueryText != "navi.diagnostics.session_errors" {
		t.Fatalf("unexpected missing capability query text: %+v", result.MissingCapability)
	}
}
