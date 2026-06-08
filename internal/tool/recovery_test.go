package tool

import (
	"testing"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/schema"
)

func TestRecoverToolCallUnknownToolRecordsRelatedMatches(t *testing.T) {
	reg := NewRegistry()
	related := validTestTool("navi.calendar.read_availability")
	related.DisplayName = "Calendar Availability"
	related.Description = "Read calendar availability windows."
	related.Aliases = []string{"calendar availability", "calendar lookup"}
	related.CapabilityTags = []string{"calendar", "availability", "schedule"}
	related.Governance.Domain = "calendar"
	related.EnvironmentVisibility = []string{"development", "production"}
	if err := reg.Register(related); err != nil {
		t.Fatalf("register related tool: %v", err)
	}

	result := RecoverToolCall(ToolCallRecoveryInput{
		Registry: reg,
		ToolCall: llm.ToolCall{
			ID:   "tc-unknown-calendar",
			Name: "navi.calendar.lookup",
		},
		ActiveToolIDs:           []string{"navi.files.read"},
		RepairAttemptsRemaining: 1,
	})

	if result.Disposition != ToolCallRecoveryDispositionUnknownTool {
		t.Fatalf("expected unknown recovery, got %+v", result)
	}
	if !result.Hallucinated {
		t.Fatalf("expected hallucinated attempt, got %+v", result)
	}
	if !result.CanRetry {
		t.Fatalf("expected bounded repair retry, got %+v", result)
	}
	if len(result.RelatedToolIDs) != 1 || result.RelatedToolIDs[0] != "navi.calendar.read_availability" {
		t.Fatalf("expected related tool ids, got %+v", result.RelatedToolIDs)
	}
	if result.Discovery == nil || len(result.Discovery.RelatedMatches) == 0 {
		t.Fatalf("expected structured discovery result, got %+v", result.Discovery)
	}
}

func TestRecoverToolCallExistingButUnloadedConsultsBroker(t *testing.T) {
	reg := NewRegistry()
	active := validTestTool("navi.files.read")
	active.DisplayName = "Read File"
	active.Description = "Read a file from the workspace."
	active.CapabilityTags = []string{"file", "workspace", "read"}
	if err := reg.Register(active); err != nil {
		t.Fatalf("register active tool: %v", err)
	}

	unloaded := validTestTool("navi.calendar.read_availability")
	unloaded.DisplayName = "Calendar Availability"
	unloaded.Description = "Read calendar availability windows."
	unloaded.CapabilityTags = []string{"calendar", "availability", "schedule"}
	unloaded.Governance.Domain = "calendar"
	unloaded.EnvironmentVisibility = []string{"development", "production"}
	if err := reg.Register(unloaded); err != nil {
		t.Fatalf("register unloaded tool: %v", err)
	}

	result := RecoverToolCall(ToolCallRecoveryInput{
		Registry:      reg,
		ActiveToolIDs: []string{"navi.files.read"},
		ToolCall: llm.ToolCall{
			ID:   "tc-unloaded-calendar",
			Name: "navi.calendar.read_availability",
		},
		BrokerInput: BrokerInput{
			UserInput:     "check calendar availability",
			SessionMode:   DiscoverySessionModeAssistant,
			Environment:   "production",
			UserAuthority: ToolAuthorityOwner,
			ModelProfile: BrokerModelProfile{
				Name:             "frontier_strong",
				SupportsTools:    true,
				ToolCallReliable: true,
			},
		},
		RepairAttemptsRemaining: 1,
	})

	if result.Disposition != ToolCallRecoveryDispositionUnloadedTool {
		t.Fatalf("expected unloaded recovery, got %+v", result)
	}
	if result.ExistingToolID != "navi.calendar.read_availability" {
		t.Fatalf("expected exact existing tool id, got %+v", result)
	}
	if !result.LoadAllowed {
		t.Fatalf("expected broker to mark tool loadable, got %+v", result)
	}
	if result.Broker == nil || len(result.Broker.SelectedToolIDs) == 0 || result.Broker.SelectedToolIDs[0] != "navi.calendar.read_availability" {
		t.Fatalf("expected broker selection for loadable tool, got %+v", result.Broker)
	}
}

func TestRecoverToolCallObservedSelfDiagnosticFailureStaysUnavailable(t *testing.T) {
	reg := NewRegistry()
	diagnostic := validTestTool("navi.debug.self_diagnostic_recent_errors")
	diagnostic.DisplayName = "Recent Errors"
	diagnostic.Description = "Inspect recent internal diagnostic errors."
	diagnostic.Category = ToolCategoryInternalDiagnostic
	diagnostic.Metadata.ExposureClass = ToolExposureInternal
	diagnostic.RequiredModes = []schema.DirectiveMode{schema.DirectiveModeAct}
	diagnostic.EnvironmentVisibility = []string{"development", "production"}
	diagnostic.CapabilityTags = []string{"diagnostic", "debug", "errors"}
	if err := reg.Register(diagnostic); err != nil {
		t.Fatalf("register diagnostic tool: %v", err)
	}

	result := RecoverToolCall(ToolCallRecoveryInput{
		Registry:      reg,
		ActiveToolIDs: []string{"navi.files.read"},
		ToolCall: llm.ToolCall{
			ID:   "tc-self-diagnostic",
			Name: "navi.debug.self_diagnostic_recent_errors",
		},
		BrokerInput: BrokerInput{
			UserInput:     "self diagnostic recent errors",
			SessionMode:   DiscoverySessionModeAssistant,
			Environment:   "production",
			UserAuthority: ToolAuthorityOwner,
			ModelProfile: BrokerModelProfile{
				Name:             "frontier_strong",
				SupportsTools:    true,
				ToolCallReliable: true,
			},
		},
		RepairAttemptsRemaining: 1,
	})

	if result.Disposition != ToolCallRecoveryDispositionUnloadedTool {
		t.Fatalf("expected unloaded recovery, got %+v", result)
	}
	if result.LoadAllowed {
		t.Fatalf("expected observed diagnostic tool to stay unavailable, got %+v", result)
	}
	if result.Broker == nil || result.Broker.ToolChoiceMode != ToolChoiceModeNone {
		t.Fatalf("expected broker to keep diagnostic tool unavailable, got %+v", result.Broker)
	}
}
