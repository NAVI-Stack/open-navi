package tool

import "testing"

func TestToolBrokerResolveRefreshesIndexWhenRegistryChanges(t *testing.T) {
	reg := NewRegistry()
	readTool := validTestTool("navi.files.read")
	readTool.DisplayName = "Read File"
	readTool.Description = "Read a file from the workspace."
	readTool.CapabilityTags = []string{"file", "read", "workspace"}
	if err := reg.Register(readTool); err != nil {
		t.Fatalf("register read tool: %v", err)
	}

	broker := NewToolBrokerFromRegistry(reg)
	input := BrokerInput{
		UserInput:     "create linear tickets for the bug",
		SessionMode:   DiscoverySessionModeAssistant,
		Environment:   "production",
		UserAuthority: ToolAuthorityOwner,
		ModelProfile: BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	}

	initial := broker.Resolve(input)
	if len(initial.SelectedToolIDs) != 0 {
		t.Fatalf("expected initial no-tool result before linear exists, got %+v", initial.SelectedToolIDs)
	}
	initialSnapshot := initial.SnapshotID

	linearTool := validTestTool("navi.linear.create_issue")
	linearTool.DisplayName = "Create Linear Issue"
	linearTool.Description = "Create Linear tickets and issues."
	linearTool.Aliases = []string{"linear tickets", "create linear tickets", "create issue"}
	linearTool.CapabilityTags = []string{"linear", "tickets", "issues", "project"}
	linearTool.Governance.Domain = "linear"
	linearTool.EnvironmentVisibility = []string{"development", "production"}
	if err := reg.Register(linearTool); err != nil {
		t.Fatalf("register linear tool: %v", err)
	}

	refreshed := broker.Resolve(input)
	if !broker.CachedDecisionInvalid(initialSnapshot) {
		t.Fatalf("expected initial cached no-tool decision to become stale after registry change")
	}
	if len(refreshed.SelectedToolIDs) == 0 || refreshed.SelectedToolIDs[0] != "navi.linear.create_issue" {
		t.Fatalf("expected refreshed broker to discover new linear tool, got %+v", refreshed.SelectedToolIDs)
	}
}

func TestToolBrokerRefreshCapabilityLinearStyleLateEnabledLoad(t *testing.T) {
	reg := NewRegistry()
	broker := NewToolBrokerFromRegistry(reg)
	input := BrokerInput{
		UserInput:     "create Linear tickets for onboarding bugs",
		SessionMode:   DiscoverySessionModeAssistant,
		Environment:   "production",
		UserAuthority: ToolAuthorityOwner,
		ModelProfile: BrokerModelProfile{
			Name:             "frontier_strong",
			SupportsTools:    true,
			ToolCallReliable: true,
		},
	}
	initial := broker.Resolve(input)

	linearTool := validTestTool("navi.linear.create_issue")
	linearTool.DisplayName = "Create Linear Issue"
	linearTool.Description = "Create Linear tickets and issues."
	linearTool.Aliases = []string{"linear tickets", "create linear tickets", "create issue"}
	linearTool.CapabilityTags = []string{"linear", "tickets", "issues", "project"}
	linearTool.Governance.Domain = "linear"
	linearTool.EnvironmentVisibility = []string{"development", "production"}
	if err := reg.Register(linearTool); err != nil {
		t.Fatalf("register linear tool: %v", err)
	}

	refresh := broker.RefreshCapability(CapabilityRefreshInput{
		QueryText:          "create Linear tickets for onboarding bugs",
		BrokerInput:        input,
		PreviousSnapshotID: initial.SnapshotID,
	})

	if refresh.AvailabilityState != ToolAvailabilityLoadableNotLoaded {
		t.Fatalf("expected late-enabled tool to become loadable_not_loaded, got %+v", refresh)
	}
	if !refresh.ShouldLoad {
		t.Fatalf("expected refresh to recommend loading late-enabled tool, got %+v", refresh)
	}
	if !refresh.CachedDecisionInvalidated {
		t.Fatalf("expected cached unavailable/no-tool decision invalidation, got %+v", refresh)
	}
	if refresh.Broker == nil || len(refresh.Broker.SelectedToolIDs) == 0 || refresh.Broker.SelectedToolIDs[0] != "navi.linear.create_issue" {
		t.Fatalf("expected refreshed broker to select linear tool, got %+v", refresh.Broker)
	}
}

func TestToolBrokerRefreshCapabilityClassifiesRegisteredButNotDiscoverable(t *testing.T) {
	reg := NewRegistry()
	hiddenTool := validTestTool("navi.linear.create_issue")
	hiddenTool.DisplayName = "Create Linear Issue"
	hiddenTool.Hidden = true
	hiddenTool.Aliases = []string{"create linear tickets"}
	hiddenTool.EnvironmentVisibility = []string{"development", "production"}
	if err := reg.Register(hiddenTool); err != nil {
		t.Fatalf("register hidden tool: %v", err)
	}

	refresh := NewToolBrokerFromRegistry(reg).RefreshCapability(CapabilityRefreshInput{
		QueryText: "navi.linear.create_issue",
		BrokerInput: BrokerInput{
			UserInput:     "create linear tickets",
			SessionMode:   DiscoverySessionModeAssistant,
			Environment:   "production",
			UserAuthority: ToolAuthorityOwner,
			ModelProfile: BrokerModelProfile{
				Name:             "frontier_strong",
				SupportsTools:    true,
				ToolCallReliable: true,
			},
		},
	})

	if refresh.AvailabilityState != ToolAvailabilityRegisteredNotDiscoverable {
		t.Fatalf("expected registered_not_discoverable state, got %+v", refresh)
	}
}

func TestToolBrokerRefreshCapabilityClassifiesLoadedNotExposedAndGovernanceBlocked(t *testing.T) {
	reg := NewRegistry()
	linearTool := validTestTool("navi.linear.create_issue")
	linearTool.DisplayName = "Create Linear Issue"
	linearTool.Aliases = []string{"create linear tickets"}
	linearTool.Description = "Create Linear tickets and issues."
	linearTool.CapabilityTags = []string{"linear", "tickets", "issues"}
	linearTool.EnvironmentVisibility = []string{"development", "production"}
	if err := reg.Register(linearTool); err != nil {
		t.Fatalf("register linear tool: %v", err)
	}

	broker := NewToolBrokerFromRegistry(reg)
	activeSet := broker.BuildActiveToolSet(
		BrokerInput{
			UserInput:     "create linear tickets",
			SessionMode:   DiscoverySessionModeAssistant,
			Environment:   "production",
			UserAuthority: ToolAuthorityOwner,
			ModelProfile: BrokerModelProfile{
				Name:             "frontier_strong",
				SupportsTools:    true,
				ToolCallReliable: true,
			},
		},
		BrokerResolution{
			SnapshotID:      reg.Snapshot().ID,
			SelectedToolIDs: []string{"navi.linear.create_issue"},
		},
		ActiveToolSetSpec{
			Scope:       ActiveToolSetScopeProviderCall,
			ChatID:      "sess-refresh",
			Mode:        "assistant",
			Environment: "production",
			ToolIDs:     []string{"navi.linear.create_issue"},
		},
	)

	loadedNotExposed := broker.RefreshCapability(CapabilityRefreshInput{
		QueryText:     "create linear tickets",
		BrokerInput:   BrokerInput{UserInput: "create linear tickets", SessionMode: DiscoverySessionModeAssistant, Environment: "production", UserAuthority: ToolAuthorityOwner, ModelProfile: BrokerModelProfile{Name: "frontier_strong", SupportsTools: true, ToolCallReliable: true}},
		ActiveToolSet: &activeSet,
	})
	if loadedNotExposed.AvailabilityState != ToolAvailabilityLoadedNotExposed || !loadedNotExposed.ShouldExpose {
		t.Fatalf("expected loaded_not_exposed state, got %+v", loadedNotExposed)
	}

	governanceBlocked := broker.RefreshCapability(CapabilityRefreshInput{
		QueryText:                "create linear tickets",
		BrokerInput:              BrokerInput{UserInput: "create linear tickets", SessionMode: DiscoverySessionModeAssistant, Environment: "production", UserAuthority: ToolAuthorityOwner, ModelProfile: BrokerModelProfile{Name: "frontier_strong", SupportsTools: true, ToolCallReliable: true}},
		ActiveToolSet:            &activeSet,
		ExposedToolIDs:           []string{"navi.linear.create_issue"},
		GovernanceBlockedToolIDs: []string{"navi.linear.create_issue"},
	})
	if governanceBlocked.AvailabilityState != ToolAvailabilityExposedGovernanceBlocked {
		t.Fatalf("expected exposed_governance_blocked state, got %+v", governanceBlocked)
	}
}
