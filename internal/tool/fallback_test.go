package tool

import (
	"testing"

	"github.com/ceoai/navi/internal/schema"
)

func TestDiscoverFallbackCandidatesBuildsDecomposedGitWritePath(t *testing.T) {
	reg := NewRegistry()

	highLevel := validTestTool("navi.files.update_content")
	highLevel.DisplayName = "Update File Content"
	highLevel.Description = "Update file contents in the workspace."
	highLevel.Category = ToolCategoryWorkflowAction
	highLevel.RiskTier = "medium"
	highLevel.SideEffects = []string{"workspace_write"}
	highLevel.CapabilityTags = []string{"files", "workspace", "write", "update"}
	highLevel.Governance = ToolGovernance{
		CommandType:     schema.CommandTypeUpdate,
		WorkspaceAction: schema.WorkspaceActionWrite,
		Domain:          "files",
	}
	if err := reg.Register(highLevel); err != nil {
		t.Fatalf("register high-level tool: %v", err)
	}

	stages := []struct {
		id     string
		stage  string
		risk   string
		effect string
	}{
		{"navi.git.write_blob", "blob", "medium", "git_blob_write"},
		{"navi.git.write_tree", "tree", "medium", "git_tree_write"},
		{"navi.git.create_commit", "commit", "high", "git_commit"},
		{"navi.git.update_ref", "ref", "medium", "git_ref_update"},
	}
	for _, stage := range stages {
		toolEntry := validTestTool(stage.id)
		toolEntry.DisplayName = stage.stage
		toolEntry.Description = "Git write fallback stage for file updates."
		toolEntry.Category = ToolCategoryWorkflowAction
		toolEntry.RiskTier = stage.risk
		toolEntry.SideEffects = []string{stage.effect}
		toolEntry.CapabilityTags = []string{"git", "files", "write", "update", stage.stage}
		toolEntry.Governance = ToolGovernance{
			CommandType:     schema.CommandTypeUpdate,
			WorkspaceAction: schema.WorkspaceActionExecute,
			Domain:          "git",
		}
		toolEntry.Metadata.Notes = map[string]string{
			"fallback_stage": stage.stage,
		}
		if err := reg.Register(toolEntry); err != nil {
			t.Fatalf("register %s: %v", stage.id, err)
		}
	}

	active := ActiveToolSet{
		Constraints: ActiveToolSetConstraints{
			RiskCeiling: "high",
		},
	}

	result := DiscoverFallbackCandidates(FallbackDiscoveryInput{
		Registry:      reg,
		FailedToolID:  "navi.files.update_content",
		FailedAction:  "update file content",
		Failure:       FailureForCode(ExecutionFailureCodeSelectedActionMissing, "file update executor missing"),
		Intent:        "update the file contents safely",
		ActiveToolSet: &active,
		Context: DiscoveryContext{
			Environment: "development",
			SessionMode: DiscoverySessionModeCoder,
			Authority:   ToolAuthorityOwner,
		},
	})

	if result.Outcome != FallbackDiscoveryOutcomeRequiresConfirmation {
		t.Fatalf("expected requires_confirmation outcome, got %+v", result)
	}
	if len(result.Candidates) == 0 {
		t.Fatalf("expected fallback candidates, got %+v", result)
	}
	candidate := result.Candidates[0]
	if candidate.MatchKind != "decomposed_path" {
		t.Fatalf("expected decomposed path candidate, got %+v", candidate)
	}
	if len(candidate.ToolIDs) != 4 {
		t.Fatalf("expected 4-stage git fallback path, got %+v", candidate.ToolIDs)
	}
	if candidate.RiskComparison != "higher" {
		t.Fatalf("expected higher risk comparison due to commit step, got %+v", candidate)
	}
	if candidate.SideEffectComparison != "broader" {
		t.Fatalf("expected broader side effects for git path, got %+v", candidate)
	}
	if !candidate.RequiresConfirmation {
		t.Fatalf("expected decomposed git path to require confirmation, got %+v", candidate)
	}
	if len(candidate.Rationale) == 0 {
		t.Fatalf("expected candidate rationale, got %+v", candidate)
	}
}

func TestDiscoverFallbackCandidatesBlocksGovernanceRejections(t *testing.T) {
	reg := NewRegistry()
	result := DiscoverFallbackCandidates(FallbackDiscoveryInput{
		Registry:     reg,
		FailedToolID: "navi.files.write",
		Failure:      FailureForCode(ExecutionFailureCodeGovernanceBlocked, "blocked by governance"),
		Intent:       "write file",
	})

	if result.Outcome != FallbackDiscoveryOutcomeBlockedByGovernance {
		t.Fatalf("expected blocked_by_governance, got %+v", result)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("expected no candidates for governance block, got %+v", result)
	}
}

func TestDiscoverFallbackCandidatesBlocksPolicyRejections(t *testing.T) {
	reg := NewRegistry()
	result := DiscoverFallbackCandidates(FallbackDiscoveryInput{
		Registry:     reg,
		FailedToolID: "navi.files.write",
		Failure:      FailureForCode(ExecutionFailureCodePolicyRejected, "blocked by policy"),
		Intent:       "write file",
	})

	if result.Outcome != FallbackDiscoveryOutcomeBlockedByGovernance {
		t.Fatalf("expected blocked_by_governance, got %+v", result)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("expected no candidates for policy rejection, got %+v", result)
	}
}

func TestDiscoverFallbackCandidatesReportsGovernanceBlockedFallbackPath(t *testing.T) {
	reg := NewRegistry()

	highLevel := validTestTool("navi.files.update_content")
	highLevel.DisplayName = "Update File Content"
	highLevel.Category = ToolCategoryWorkflowAction
	highLevel.RiskTier = "medium"
	highLevel.SideEffects = []string{"workspace_write"}
	highLevel.CapabilityTags = []string{"files", "workspace", "write", "update"}
	highLevel.Governance = ToolGovernance{
		CommandType:     schema.CommandTypeUpdate,
		WorkspaceAction: schema.WorkspaceActionWrite,
		Domain:          "files",
	}
	if err := reg.Register(highLevel); err != nil {
		t.Fatalf("register high-level tool: %v", err)
	}

	blocked := validTestTool("navi.git.update_ref")
	blocked.DisplayName = "Update Git Ref"
	blocked.Description = "Git write fallback stage for file updates."
	blocked.Category = ToolCategoryWorkflowAction
	blocked.RiskTier = "medium"
	blocked.SideEffects = []string{"git_ref_update"}
	blocked.CapabilityTags = []string{"git", "files", "write", "update", "ref"}
	blocked.Governance = ToolGovernance{
		CommandType:     schema.CommandTypeUpdate,
		WorkspaceAction: schema.WorkspaceActionExecute,
		Domain:          "git",
	}
	blocked.RequiredAuthority = ToolAuthorityAdmin
	blocked.Metadata.Notes = map[string]string{
		"fallback_stage": "ref",
	}
	if err := reg.Register(blocked); err != nil {
		t.Fatalf("register blocked fallback tool: %v", err)
	}

	result := DiscoverFallbackCandidates(FallbackDiscoveryInput{
		Registry:     reg,
		FailedToolID: "navi.files.update_content",
		FailedAction: "update file content",
		Failure:      FailureForCode(ExecutionFailureCodeSelectedActionMissing, "file update executor missing"),
		Intent:       "update the file contents safely",
		Context: DiscoveryContext{
			Environment: "development",
			SessionMode: DiscoverySessionModeCoder,
			Authority:   ToolAuthorityUser,
		},
	})

	if result.Outcome != FallbackDiscoveryOutcomeBlockedByGovernance {
		t.Fatalf("expected blocked fallback outcome, got %+v", result)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("expected no available candidates when fallback is governance-blocked, got %+v", result.Candidates)
	}
	if result.BlockedReason == "" {
		t.Fatalf("expected blocked reason, got %+v", result)
	}
}

func TestDiscoverFallbackCandidatesAnnotatesAPIContractDriftRecovery(t *testing.T) {
	reg := NewRegistry()

	primary := validTestTool("navi.connector.high_level")
	primary.DisplayName = "High Level Connector Action"
	primary.Description = "Performs a high level connector operation."
	primary.Category = ToolCategoryWorkflowAction
	primary.RiskTier = "medium"
	primary.SideEffects = []string{"connector_call"}
	primary.CapabilityTags = []string{"connector", "calendar", "sync"}
	primary.Governance = ToolGovernance{
		CommandType:     schema.CommandTypeQuery,
		WorkspaceAction: schema.WorkspaceActionRead,
		Domain:          "calendar",
	}
	if err := reg.Register(primary); err != nil {
		t.Fatalf("register primary tool: %v", err)
	}

	fallback := validTestTool("navi.connector.read_only")
	fallback.DisplayName = "Connector Read Only"
	fallback.Description = "Read-only connector fallback."
	fallback.Category = ToolCategoryWorkflowAction
	fallback.RiskTier = "medium"
	fallback.SideEffects = []string{"connector_call"}
	fallback.CapabilityTags = []string{"connector", "calendar", "sync", "read"}
	fallback.Governance = ToolGovernance{
		CommandType:     schema.CommandTypeQuery,
		WorkspaceAction: schema.WorkspaceActionRead,
		Domain:          "calendar",
	}
	fallback.Metadata.Notes = map[string]string{
		"fallback_for": primary.ToolID,
	}
	if err := reg.Register(fallback); err != nil {
		t.Fatalf("register fallback tool: %v", err)
	}

	result := DiscoverFallbackCandidates(FallbackDiscoveryInput{
		Registry:     reg,
		FailedToolID: primary.ToolID,
		FailedAction: "sync calendar data",
		Failure:      FailureForCode(ExecutionFailureCodeAPIContractDrift, "unsupported response format"),
		Intent:       "read calendar data safely",
		Context: DiscoveryContext{
			Environment: "development",
			SessionMode: DiscoverySessionModeAssistant,
			Authority:   ToolAuthorityOwner,
		},
	})

	if result.Outcome != FallbackDiscoveryOutcomeFallbackCandidates {
		t.Fatalf("expected fallback candidates, got %+v", result)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected one explicit fallback candidate, got %+v", result.Candidates)
	}
	candidate := result.Candidates[0]
	if candidate.ToolIDs[0] != fallback.ToolID {
		t.Fatalf("expected explicit fallback tool %q, got %+v", fallback.ToolID, candidate)
	}
	if !containsString(candidate.Rationale, "api_contract_drift_recovery") {
		t.Fatalf("expected API contract drift rationale, got %+v", candidate.Rationale)
	}
}

func TestDiscoverFallbackCandidatesMissingContextDoesNotWidenOwnerOnlyFallback(t *testing.T) {
	reg := NewRegistry()

	primary := validTestTool("navi.files.update_content")
	primary.DisplayName = "Update File Content"
	primary.Category = ToolCategoryWorkflowAction
	primary.RiskTier = "medium"
	primary.SideEffects = []string{"workspace_write"}
	primary.CapabilityTags = []string{"files", "workspace", "write", "update"}
	primary.Governance = ToolGovernance{
		CommandType:     schema.CommandTypeUpdate,
		WorkspaceAction: schema.WorkspaceActionWrite,
		Domain:          "files",
	}
	if err := reg.Register(primary); err != nil {
		t.Fatalf("register primary tool: %v", err)
	}

	restricted := validTestTool("navi.git.update_ref")
	restricted.DisplayName = "Update Git Ref"
	restricted.Category = ToolCategoryWorkflowAction
	restricted.RiskTier = "medium"
	restricted.SideEffects = []string{"git_ref_update"}
	restricted.CapabilityTags = []string{"git", "files", "write", "update", "ref"}
	restricted.Governance = ToolGovernance{
		CommandType:     schema.CommandTypeUpdate,
		WorkspaceAction: schema.WorkspaceActionExecute,
		Domain:          "git",
	}
	restricted.RequiredAuthority = ToolAuthorityOwner
	restricted.EnvironmentVisibility = []string{"development"}
	restricted.Metadata.Notes = map[string]string{
		"fallback_for": primary.ToolID,
	}
	if err := reg.Register(restricted); err != nil {
		t.Fatalf("register restricted fallback tool: %v", err)
	}

	result := DiscoverFallbackCandidates(FallbackDiscoveryInput{
		Registry:     reg,
		FailedToolID: primary.ToolID,
		FailedAction: "update file content",
		Failure:      FailureForCode(ExecutionFailureCodeSelectedActionMissing, "file update executor missing"),
		Intent:       "update the file contents safely",
	})

	if result.Outcome != FallbackDiscoveryOutcomeBlockedByGovernance {
		t.Fatalf("expected production-safe omitted context to suppress owner/dev-only fallback, got %+v", result)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("expected no fallback candidates under omitted context, got %+v", result.Candidates)
	}
}
