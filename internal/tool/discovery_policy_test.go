package tool

import (
	"testing"

	"github.com/open-navi/navi/internal/schema"
)

func TestToolDiscoveryPolicyHidesAdminCriticalToolsFromNormalUsers(t *testing.T) {
	reg := NewRegistry()
	adminTool := validTestTool("navi.admin.rotate_credentials")
	adminTool.DisplayName = "Rotate Credentials"
	adminTool.Category = ToolCategoryAdminCritical
	adminTool.RequiredAuthority = ToolAuthorityAdmin
	adminTool.RiskTier = "critical"
	adminTool.Metadata.ExposureClass = ToolExposureInternal
	if err := reg.Register(adminTool); err != nil {
		t.Fatalf("register admin tool: %v", err)
	}

	result := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeExact,
		Text: "navi.admin.rotate_credentials",
		Context: DiscoveryContext{
			Environment: "production",
			SessionMode: DiscoverySessionModeAssistant,
			Authority:   ToolAuthorityUser,
		},
	})

	if result.ExactMatch != nil {
		t.Fatalf("expected admin tool to stay hidden from normal user, got %+v", result.ExactMatch)
	}
	if result.AvailabilityStatus != DiscoveryAvailabilityExplicitMiss {
		t.Fatalf("expected explicit miss for hidden admin tool, got %s", result.AvailabilityStatus)
	}
}

func TestToolDiscoveryPolicyOwnerDebugCanSeeUnavailableInternalTool(t *testing.T) {
	reg := NewRegistry()
	debugTool := validTestTool("navi.debug.session_trace")
	debugTool.DisplayName = "Session Trace"
	debugTool.Category = ToolCategoryInternalDiagnostic
	debugTool.Metadata.ExposureClass = ToolExposureInternal
	debugTool.RequiredModes = []schema.DirectiveMode{schema.DirectiveModeAct}
	if err := reg.Register(debugTool); err != nil {
		t.Fatalf("register debug tool: %v", err)
	}

	companionResult := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeExact,
		Text: "navi.debug.session_trace",
		Context: DiscoveryContext{
			Environment: "development",
			SessionMode: DiscoverySessionModeCompanion,
			Authority:   ToolAuthorityOwner,
		},
	})
	if companionResult.ExactMatch != nil {
		t.Fatalf("expected companion mode to hide internal debug tool, got %+v", companionResult.ExactMatch)
	}

	debugResult := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeExact,
		Text: "navi.debug.session_trace",
		Context: DiscoveryContext{
			Environment: "development",
			SessionMode: DiscoverySessionModeDebug,
			Authority:   ToolAuthorityOwner,
		},
	})
	if debugResult.ExactMatch == nil || debugResult.ExactMatch.Tool == nil {
		t.Fatalf("expected owner debug context to see tool, got %+v", debugResult)
	}
	if debugResult.AvailabilityStatus != DiscoveryAvailabilityAvailable {
		t.Fatalf("expected available debug tool in owner debug context, got %s", debugResult.AvailabilityStatus)
	}
}

func TestToolDiscoveryPolicyProductionHidesDevelopmentAndTestTools(t *testing.T) {
	reg := NewRegistry()

	devTool := validTestTool("navi.dev.workspace_probe")
	devTool.DisplayName = "Workspace Probe"
	devTool.EnvironmentVisibility = []string{"development", "test"}
	devTool.Metadata.ExposureClass = ToolExposureDevelopment
	devTool.Category = ToolCategoryDevTest
	if err := reg.Register(devTool); err != nil {
		t.Fatalf("register dev tool: %v", err)
	}

	testTool := validTestTool("navi.test.synthetic_fixture")
	testTool.DisplayName = "Synthetic Fixture"
	testTool.EnvironmentVisibility = []string{"test"}
	testTool.Metadata.ExposureClass = ToolExposureTest
	testTool.Category = ToolCategoryDevTest
	if err := reg.Register(testTool); err != nil {
		t.Fatalf("register test tool: %v", err)
	}

	productionResult := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeLexical,
		Text: "workspace synthetic fixture",
		Context: DiscoveryContext{
			Environment: "production",
			SessionMode: DiscoverySessionModeCoder,
			Authority:   ToolAuthorityOwner,
		},
	})
	if len(productionResult.RelatedMatches) != 0 {
		t.Fatalf("expected production discovery to hide dev/test tools, got %+v", productionResult.RelatedMatches)
	}

	testResult := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeLexical,
		Text: "workspace synthetic fixture",
		Context: DiscoveryContext{
			Environment: "test",
			SessionMode: DiscoverySessionModeDebug,
			Authority:   ToolAuthorityOwner,
		},
	})
	if len(testResult.RelatedMatches) == 0 {
		t.Fatal("expected test discovery to surface dev/test tools")
	}
}

func TestToolDiscoveryPolicyCoderModeSurfacesDevelopmentTools(t *testing.T) {
	reg := NewRegistry()
	devTool := validTestTool("navi.dev.go_test")
	devTool.DisplayName = "Go Test"
	devTool.Category = ToolCategoryDevTest
	devTool.Metadata.ExposureClass = ToolExposureDevelopment
	devTool.EnvironmentVisibility = []string{"development", "test"}
	devTool.RequiredModes = []schema.DirectiveMode{schema.DirectiveModeAct}
	if err := reg.Register(devTool); err != nil {
		t.Fatalf("register coder tool: %v", err)
	}

	assistantResult := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeExact,
		Text: "navi.dev.go_test",
		Context: DiscoveryContext{
			Environment: "development",
			SessionMode: DiscoverySessionModeAssistant,
			Authority:   ToolAuthorityOwner,
		},
	})
	if assistantResult.ExactMatch != nil {
		t.Fatalf("expected assistant mode to hide development tool, got %+v", assistantResult.ExactMatch)
	}

	coderResult := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeExact,
		Text: "navi.dev.go_test",
		Context: DiscoveryContext{
			Environment: "development",
			SessionMode: DiscoverySessionModeCoder,
			Authority:   ToolAuthorityOwner,
		},
	})
	if coderResult.ExactMatch == nil || coderResult.ExactMatch.Tool == nil {
		t.Fatalf("expected coder mode to surface development tool, got %+v", coderResult)
	}
	if coderResult.AvailabilityStatus != DiscoveryAvailabilityAvailable {
		t.Fatalf("expected coder mode development tool to be available, got %s", coderResult.AvailabilityStatus)
	}
}

func TestToolDiscoveryPolicyFeatureRiskAndTrustFilters(t *testing.T) {
	reg := NewRegistry()
	toolEntry := validTestTool("navi.integration.external_draft")
	toolEntry.DisplayName = "External Draft"
	toolEntry.FeatureFlags = []string{"beta-discovery"}
	toolEntry.RiskTier = "high"
	toolEntry.Metadata.TrustTier = "trusted_local"
	if err := reg.Register(toolEntry); err != nil {
		t.Fatalf("register gated tool: %v", err)
	}

	debugResult := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeExact,
		Text: "navi.integration.external_draft",
		Context: DiscoveryContext{
			Environment:      "development",
			SessionMode:      DiscoverySessionModeDebug,
			Authority:        ToolAuthorityOwner,
			MaximumRiskTier:  "medium",
			MinimumTrustTier: "trusted_internal",
		},
	})
	if debugResult.ExactMatch == nil || debugResult.ExactMatch.Tool == nil {
		t.Fatalf("expected owner debug to see unavailable gated tool, got %+v", debugResult)
	}
	if debugResult.AvailabilityStatus != DiscoveryAvailabilityVisibleUnavailable {
		t.Fatalf("expected visible_unavailable for gated tool, got %s", debugResult.AvailabilityStatus)
	}

	enabledResult := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeExact,
		Text: "navi.integration.external_draft",
		Context: DiscoveryContext{
			Environment:      "development",
			SessionMode:      DiscoverySessionModeDebug,
			Authority:        ToolAuthorityOwner,
			FeatureFlags:     []string{"beta-discovery"},
			MaximumRiskTier:  "high",
			MinimumTrustTier: "trusted_local",
		},
	})
	if enabledResult.ExactMatch == nil || enabledResult.ExactMatch.Tool == nil {
		t.Fatalf("expected enabled tool to remain visible, got %+v", enabledResult)
	}
	if enabledResult.AvailabilityStatus != DiscoveryAvailabilityAvailable {
		t.Fatalf("expected enabled gated tool to be available, got %s", enabledResult.AvailabilityStatus)
	}
}

func TestDiscoveryContextNormalizeDefaultsToProductionAssistantUser(t *testing.T) {
	normalized := (DiscoveryContext{}).normalize()
	if normalized.Environment != "production" {
		t.Fatalf("expected missing environment to default to production, got %+v", normalized)
	}
	if normalized.SessionMode != DiscoverySessionModeAssistant {
		t.Fatalf("expected missing session mode to default to assistant, got %+v", normalized)
	}
	if normalized.Authority != ToolAuthorityUser {
		t.Fatalf("expected missing authority to default to user, got %+v", normalized)
	}
}

func TestToolDiscoveryPolicyMissingContextDoesNotRevealDevelopmentTool(t *testing.T) {
	reg := NewRegistry()
	devTool := validTestTool("navi.dev.workspace_probe")
	devTool.DisplayName = "Workspace Probe"
	devTool.EnvironmentVisibility = []string{"development", "test"}
	devTool.Metadata.ExposureClass = ToolExposureDevelopment
	devTool.Category = ToolCategoryDevTest
	if err := reg.Register(devTool); err != nil {
		t.Fatalf("register dev tool: %v", err)
	}

	result := NewIndex(reg).Discover(DiscoveryQuery{
		Mode: DiscoveryQueryModeExact,
		Text: "navi.dev.workspace_probe",
	})

	if result.ExactMatch != nil {
		t.Fatalf("expected omitted context to fail closed on development tool, got %+v", result.ExactMatch)
	}
	if result.AvailabilityStatus != DiscoveryAvailabilityExplicitMiss {
		t.Fatalf("expected explicit miss under production-safe default, got %+v", result)
	}
}
