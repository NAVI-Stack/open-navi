package inference

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/orchestration"
	"github.com/ceoai/navi/internal/schema"
	navitool "github.com/ceoai/navi/internal/tool"
)

func TestDefaultPrepareModelCall_EmitsContractsForAllExecutableTools(t *testing.T) {
	controller := NewController()
	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"runtime_echo", "runtime_patch"}))

	envelope, err := controller.PrepareModelCall(context.Background(), prior, ModelCallPreparationInput{
		Compiled: orchestration.CompiledModelRequest{
			Tools: []llm.ToolDefinition{
				{Name: "runtime_echo"},
				{Name: "runtime_patch"},
			},
		},
		ExecutableToolNames: []string{"runtime_echo", "runtime_patch"},
		ToolContracts: map[string]ToolContract{
			"runtime_echo":  testToolContract("runtime_echo", schema.CommandTypeInvoke, "testing", schema.WorkspaceActionExecute),
			"runtime_patch": testToolContract("runtime_patch", schema.CommandTypeUpdate, "coding", schema.WorkspaceActionModify),
		},
	})
	if err != nil {
		t.Fatalf("PrepareModelCall: %v", err)
	}
	if envelope.RuntimeDisposition == RuntimeDispositionBlockWithReply {
		t.Fatalf("expected default prepare path to authorize executable tools, got block %q", envelope.ReplyMessage)
	}
	if envelope.ModelDirective == nil {
		t.Fatal("expected model directive")
	}
	if !envelope.ModelDirective.AllowToolCalls {
		t.Fatal("expected tool calls to remain authorized")
	}
	if len(envelope.ModelDirective.ToolNames) != 2 {
		t.Fatalf("expected two surfaced tools, got %#v", envelope.ModelDirective.ToolNames)
	}
	if len(envelope.ModelDirective.ToolContracts) != 2 {
		t.Fatalf("expected contract map for every surfaced tool, got %#v", envelope.ModelDirective.ToolContracts)
	}
	for _, name := range envelope.ModelDirective.ToolNames {
		contract := ModelDirectiveToolContract(envelope.ModelDirective, name)
		if contract == nil {
			t.Fatalf("expected authoritative contract for %q", name)
		}
		if contract.ToolName != name {
			t.Fatalf("contract tool_name = %q, want %q", contract.ToolName, name)
		}
	}
}

func TestDefaultPrepareModelCall_FailsClosedWhenExecutableContractMissing(t *testing.T) {
	controller := NewController()
	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"runtime_echo", "runtime_patch"}))

	envelope, err := controller.PrepareModelCall(context.Background(), prior, ModelCallPreparationInput{
		Compiled: orchestration.CompiledModelRequest{
			Tools: []llm.ToolDefinition{
				{Name: "runtime_echo"},
				{Name: "runtime_patch"},
			},
		},
		ExecutableToolNames: []string{"runtime_echo", "runtime_patch"},
		ToolContracts: map[string]ToolContract{
			"runtime_echo": testToolContract("runtime_echo", schema.CommandTypeInvoke, "testing", schema.WorkspaceActionExecute),
		},
	})
	if err != nil {
		t.Fatalf("PrepareModelCall: %v", err)
	}
	if envelope.RuntimeDisposition != RuntimeDispositionBlockWithReply {
		t.Fatalf("expected fail-closed block, got %s", envelope.RuntimeDisposition)
	}
	if envelope.ModelDirective != nil {
		t.Fatalf("expected no partial directive on contract loss, got %#v", envelope.ModelDirective)
	}
	if got := strings.TrimSpace(envelope.ReplyMessage); got != preparedModelDirectiveUserReply() {
		t.Fatalf("expected safe user reply %q, got %q", preparedModelDirectiveUserReply(), got)
	}
	if !strings.Contains(envelope.ExecutionBoundary.FailClosedReason, "runtime_patch") || !strings.Contains(envelope.ExecutionBoundary.FailClosedReason, "ToolContract") {
		t.Fatalf("expected fail-closed reason to retain contract detail, got %q", envelope.ExecutionBoundary.FailClosedReason)
	}
	if envelope.GovernanceResult == nil || !strings.Contains(envelope.GovernanceResult.Reason, "runtime_patch") || !strings.Contains(envelope.GovernanceResult.Reason, "ToolContract") {
		t.Fatalf("expected internal governance reason to preserve contract detail, got %#v", envelope.GovernanceResult)
	}
}

func TestPrepareModelCall_UsesSafeUserReplyWhenAuthorizedCapabilityCannotBeSurfaced(t *testing.T) {
	controller := NewController()
	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"runtime_echo"}))

	envelope, err := controller.PrepareModelCall(context.Background(), prior, ModelCallPreparationInput{
		Compiled: orchestration.CompiledModelRequest{
			Tools: []llm.ToolDefinition{
				{Name: "runtime_patch"},
			},
		},
		ExecutableToolNames: []string{"runtime_patch"},
		ToolContracts: map[string]ToolContract{
			"runtime_patch": testToolContract("runtime_patch", schema.CommandTypeUpdate, "coding", schema.WorkspaceActionModify),
		},
	})
	if err != nil {
		t.Fatalf("PrepareModelCall: %v", err)
	}
	if envelope.RuntimeDisposition != RuntimeDispositionBlockWithReply {
		t.Fatalf("expected fail-closed block, got %s", envelope.RuntimeDisposition)
	}
	if envelope.ModelDirective != nil {
		t.Fatalf("expected no model directive when authorized capability cannot be surfaced, got %#v", envelope.ModelDirective)
	}
	if got := strings.TrimSpace(envelope.ReplyMessage); got != preparedModelDirectiveUserReply() {
		t.Fatalf("expected safe user reply %q, got %q", preparedModelDirectiveUserReply(), got)
	}
	if strings.Contains(strings.ToLower(envelope.ReplyMessage), "executionintent") || strings.Contains(strings.ToLower(envelope.ReplyMessage), "could not safely surface") {
		t.Fatalf("expected user reply to avoid internal control-plane detail, got %q", envelope.ReplyMessage)
	}
	if envelope.GovernanceResult == nil || !strings.Contains(envelope.GovernanceResult.Reason, "could not safely surface") {
		t.Fatalf("expected internal governance reason to preserve surfacing detail, got %#v", envelope.GovernanceResult)
	}
	if !strings.Contains(envelope.ExecutionBoundary.FailClosedReason, "could not safely surface") {
		t.Fatalf("expected fail-closed reason to retain operator detail, got %q", envelope.ExecutionBoundary.FailClosedReason)
	}
}

func TestDefaultPrepareModelCall_LoadedToolsAreNotAutomaticallyExposed(t *testing.T) {
	controller := NewController()
	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"navi.runtime.echo"}))
	reg := testAuthorizationRegistry(t, "navi.runtime.echo", "navi.runtime.patch")
	activeToolSet := navitool.NewToolBrokerFromRegistry(reg).BuildActiveToolSet(
		navitool.BrokerInput{
			UserInput: "patch the file",
			ModelProfile: navitool.BrokerModelProfile{
				Name:             "frontier_strong",
				SupportsTools:    true,
				ToolCallReliable: true,
			},
		},
		navitool.BrokerResolution{
			SnapshotID:      reg.Snapshot().ID,
			SelectedToolIDs: []string{"navi.runtime.echo", "navi.runtime.patch"},
			BrokerReason:    "loaded runtime set",
		},
		navitool.ActiveToolSetSpec{
			Scope:       navitool.ActiveToolSetScopeProviderCall,
			ChatID:      "sess-prepare",
			Mode:        "navi",
			Environment: "development",
			ToolIDs:     []string{"navi.runtime.echo", "navi.runtime.patch"},
			LoadReason:  "test_loaded_tools",
		},
	)

	envelope, err := controller.PrepareModelCall(context.Background(), prior, ModelCallPreparationInput{
		Compiled: orchestration.CompiledModelRequest{
			Tools: []llm.ToolDefinition{
				{Name: "navi.runtime.echo"},
			},
		},
		ExecutableToolNames: []string{"navi.runtime.echo"},
		ToolContracts: map[string]ToolContract{
			"navi.runtime.echo": testToolContract("navi.runtime.echo", schema.CommandTypeInvoke, "testing", schema.WorkspaceActionExecute),
		},
		ActiveToolSet: &activeToolSet,
		ToolRegistry:  reg,
	})
	if err != nil {
		t.Fatalf("PrepareModelCall: %v", err)
	}
	if envelope.ModelDirective == nil {
		t.Fatal("expected model directive")
	}
	if len(envelope.ModelDirective.ToolNames) != 1 || envelope.ModelDirective.ToolNames[0] != "navi.runtime.echo" {
		t.Fatalf("expected only explicitly compiled tool to be surfaced, got %#v", envelope.ModelDirective.ToolNames)
	}
	if containsAuthTool(envelope.ModelDirective.ToolNames, "navi.runtime.patch") {
		t.Fatalf("expected loaded-only tool to stay unexposed, got %#v", envelope.ModelDirective.ToolNames)
	}
}

func TestAuthorizeModelResponse_LoadedToolsAreNotAutomaticallyExecutable(t *testing.T) {
	controller := NewController()
	reg := testAuthorizationRegistry(t, "navi.runtime.echo", "navi.runtime.patch")
	activeToolSet := navitool.NewToolBrokerFromRegistry(reg).BuildActiveToolSet(
		navitool.BrokerInput{
			UserInput: "patch the file",
			ModelProfile: navitool.BrokerModelProfile{
				Name:             "frontier_strong",
				SupportsTools:    true,
				ToolCallReliable: true,
			},
		},
		navitool.BrokerResolution{
			SnapshotID:      reg.Snapshot().ID,
			SelectedToolIDs: []string{"navi.runtime.echo", "navi.runtime.patch"},
			BrokerReason:    "loaded runtime set",
		},
		navitool.ActiveToolSetSpec{
			Scope:       navitool.ActiveToolSetScopeProviderCall,
			ChatID:      "sess-auth",
			Mode:        "navi",
			Environment: "development",
			ToolIDs:     []string{"navi.runtime.echo", "navi.runtime.patch"},
			LoadReason:  "test_loaded_tools",
		},
	)

	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"navi.runtime.echo"}))
	prior.ModelDirective = &ModelDirective{
		AllowToolCalls: true,
		ToolNames:      []string{"navi.runtime.echo"},
		ToolContracts: map[string]ToolContract{
			"navi.runtime.echo": testToolContract("navi.runtime.echo", schema.CommandTypeInvoke, "testing", schema.WorkspaceActionExecute),
		},
	}

	decision, err := controller.AuthorizeModelResponse(context.Background(), prior, ModelResponseAuthorizationInput{
		Compiled: orchestration.CompiledModelRequest{
			Tools: []llm.ToolDefinition{
				{Name: "navi.runtime.echo"},
			},
		},
		Response: orchestration.NormalizedModelResponse{
			ToolCalls: []llm.ToolCall{{
				ID:   "tc_patch",
				Name: "navi.runtime.patch",
			}},
		},
		ToolRegistry:  reg,
		ActiveToolSet: &activeToolSet,
	})
	if err != nil {
		t.Fatalf("AuthorizeModelResponse: %v", err)
	}
	if decision.RuntimeDisposition != RuntimeDispositionBlockWithReply {
		t.Fatalf("expected out-of-exposure loaded tool call to be blocked, got %s", decision.RuntimeDisposition)
	}
	if len(decision.AuthorizedToolCalls) != 0 {
		t.Fatalf("expected no authorized tool calls, got %#v", decision.AuthorizedToolCalls)
	}
}

func TestDefaultPrepareModelCall_ExpiredActiveToolSetFailsClosed(t *testing.T) {
	controller := NewController()
	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"navi.runtime.echo"}))
	reg := testAuthorizationRegistry(t, "navi.runtime.echo")
	expiredAt := time.Now().UTC().Add(-time.Minute)
	activeToolSet := navitool.NewToolBrokerFromRegistry(reg).BuildActiveToolSet(
		navitool.BrokerInput{
			UserInput: "echo the runtime state",
			ModelProfile: navitool.BrokerModelProfile{
				Name:             "frontier_strong",
				SupportsTools:    true,
				ToolCallReliable: true,
			},
		},
		navitool.BrokerResolution{
			SnapshotID:      reg.Snapshot().ID,
			SelectedToolIDs: []string{"navi.runtime.echo"},
			BrokerReason:    "expired runtime set",
		},
		navitool.ActiveToolSetSpec{
			Scope:       navitool.ActiveToolSetScopeProviderCall,
			ChatID:      "sess-expired",
			Mode:        "navi",
			Environment: "development",
			ToolIDs:     []string{"navi.runtime.echo"},
			LoadedAt:    expiredAt.Add(-time.Minute),
			ExpiresAt:   &expiredAt,
			LoadReason:  "expired_test_set",
		},
	)

	envelope, err := controller.PrepareModelCall(context.Background(), prior, ModelCallPreparationInput{
		Compiled: orchestration.CompiledModelRequest{
			Tools: []llm.ToolDefinition{{Name: "navi.runtime.echo"}},
		},
		ExecutableToolNames: []string{"navi.runtime.echo"},
		ToolContracts: map[string]ToolContract{
			"navi.runtime.echo": testToolContract("navi.runtime.echo", schema.CommandTypeInvoke, "testing", schema.WorkspaceActionExecute),
		},
		ActiveToolSet: &activeToolSet,
		ToolRegistry:  reg,
	})
	if err != nil {
		t.Fatalf("PrepareModelCall: %v", err)
	}
	if envelope.RuntimeDisposition != RuntimeDispositionBlockWithReply {
		t.Fatalf("expected expired active set to fail closed, got %#v", envelope)
	}
	if envelope.ModelDirective != nil {
		t.Fatalf("expected no directive when active set is stale, got %#v", envelope.ModelDirective)
	}
	if got := strings.TrimSpace(envelope.ReplyMessage); got != preparedModelDirectiveUserReply() {
		t.Fatalf("expected safe user reply %q, got %q", preparedModelDirectiveUserReply(), got)
	}
	if !strings.Contains(envelope.ExecutionBoundary.FailClosedReason, "could not safely surface") {
		t.Fatalf("expected fail-closed stale-set reason to stay in operator detail, got %q", envelope.ExecutionBoundary.FailClosedReason)
	}
}

func TestModelAuthorization_StaleActiveSetSchemaVersionFailsClosed(t *testing.T) {
	controller := NewController()
	reg := testAuthorizationRegistry(t, "navi.runtime.echo")
	activeToolSet := navitool.NewToolBrokerFromRegistry(reg).BuildActiveToolSet(
		navitool.BrokerInput{
			UserInput: "echo the runtime state",
			ModelProfile: navitool.BrokerModelProfile{
				Name:             "frontier_strong",
				SupportsTools:    true,
				ToolCallReliable: true,
			},
		},
		navitool.BrokerResolution{
			SnapshotID:      reg.Snapshot().ID,
			SelectedToolIDs: []string{"navi.runtime.echo"},
			BrokerReason:    "schema-v1 active set",
		},
		navitool.ActiveToolSetSpec{
			Scope:       navitool.ActiveToolSetScopeProviderCall,
			ChatID:      "sess-stale-schema",
			Mode:        "navi",
			Environment: "development",
			ToolIDs:     []string{"navi.runtime.echo"},
			LoadReason:  "schema_v1_test",
		},
	)

	reg.Unregister("navi.runtime.echo")
	registerAuthorizationTool(t, reg, "navi.runtime.echo", "2.0.0")

	prior := defaultDecisionEnvelope(testExecutableRationale([]string{"navi.runtime.echo"}))
	envelope, err := controller.PrepareModelCall(context.Background(), prior, ModelCallPreparationInput{
		Compiled: orchestration.CompiledModelRequest{
			Tools: []llm.ToolDefinition{{Name: "navi.runtime.echo"}},
		},
		ExecutableToolNames: []string{"navi.runtime.echo"},
		ToolContracts: map[string]ToolContract{
			"navi.runtime.echo": testToolContract("navi.runtime.echo", schema.CommandTypeInvoke, "testing", schema.WorkspaceActionExecute),
		},
		ActiveToolSet: &activeToolSet,
		ToolRegistry:  reg,
	})
	if err != nil {
		t.Fatalf("PrepareModelCall: %v", err)
	}
	if envelope.RuntimeDisposition != RuntimeDispositionBlockWithReply {
		t.Fatalf("expected stale schema version to fail closed at prepare time, got %#v", envelope)
	}
	if envelope.ModelDirective != nil {
		t.Fatalf("expected no model directive for stale active set schema, got %#v", envelope.ModelDirective)
	}

	prior.ModelDirective = &ModelDirective{
		AllowToolCalls: true,
		ToolNames:      []string{"navi.runtime.echo"},
		ToolContracts: map[string]ToolContract{
			"navi.runtime.echo": testToolContract("navi.runtime.echo", schema.CommandTypeInvoke, "testing", schema.WorkspaceActionExecute),
		},
	}
	decision, err := controller.AuthorizeModelResponse(context.Background(), prior, ModelResponseAuthorizationInput{
		Compiled: orchestration.CompiledModelRequest{
			Tools: []llm.ToolDefinition{{Name: "navi.runtime.echo"}},
		},
		Response: orchestration.NormalizedModelResponse{
			ToolCalls: []llm.ToolCall{{
				ID:   "tc_stale_schema",
				Name: "navi.runtime.echo",
			}},
		},
		ToolRegistry:  reg,
		ActiveToolSet: &activeToolSet,
	})
	if err != nil {
		t.Fatalf("AuthorizeModelResponse: %v", err)
	}
	if decision.RuntimeDisposition != RuntimeDispositionBlockWithReply {
		t.Fatalf("expected stale schema version to fail closed at authorization time, got %#v", decision)
	}
	if len(decision.AuthorizedToolCalls) != 0 {
		t.Fatalf("expected no authorized tool calls for stale schema version, got %#v", decision.AuthorizedToolCalls)
	}
}

func testExecutableRationale(allowed []string) Rationale {
	details := make([]CapabilityAvailability, 0, len(allowed))
	for _, name := range allowed {
		commandType := schema.CommandTypeInvoke
		if strings.Contains(name, "patch") {
			commandType = schema.CommandTypeUpdate
		}
		details = append(details, CapabilityAvailability{
			Name:        name,
			Kind:        "runtime",
			Available:   true,
			Governed:    true,
			CommandType: commandType,
		})
	}
	return Rationale{
		Governance: GovernanceHandoff{
			TargetCapability:         allowed[0],
			AllowedCapabilities:      append([]string(nil), allowed...),
			AllowedCapabilityDetails: details,
		},
		ExecutionIntent: ExecutionIntent{
			ActionType:          string(schema.CommandTypeInvoke),
			TargetCapability:    allowed[0],
			AllowedCapabilities: append([]string(nil), allowed...),
		},
	}
}

func testToolContract(name string, commandType schema.CommandType, domain string, action schema.WorkspaceActionType) ToolContract {
	return ToolContract{
		ID:              "contract:" + name,
		ToolName:        name,
		ExecutionKind:   ToolExecutionKindRegistry,
		CommandType:     commandType,
		Domain:          domain,
		ActorKind:       "navi",
		WorkspaceAction: action,
	}
}

func testAuthorizationRegistry(t *testing.T, toolIDs ...string) *navitool.Registry {
	t.Helper()
	reg := navitool.NewRegistry()
	for _, toolID := range toolIDs {
		registerAuthorizationTool(t, reg, toolID, "1.0.0")
	}
	return reg
}

func registerAuthorizationTool(t *testing.T, reg *navitool.Registry, toolID string, schemaVersion string) {
	t.Helper()
	tool := &navitool.Tool{
		ToolID:        toolID,
		DisplayName:   toolID,
		Description:   "authorization test tool",
		Source:        navitool.ToolSourceBuiltin,
		SourceID:      "tests.authorization",
		SchemaVersion: schemaVersion,
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		OutputSchema: map[string]any{
			"type": "string",
		},
		Category:              navitool.ToolCategoryReadOnly,
		RiskTier:              "low",
		SideEffects:           []string{},
		Reversibility:         "reversible",
		EnvironmentVisibility: []string{"development"},
		RequiredModes:         []schema.DirectiveMode{schema.DirectiveModeAct},
		RequiredAuthority:     navitool.ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{},
		CapabilityTags:        []string{"tests"},
		Status:                navitool.ToolStatusActive,
		Definition: llm.ToolDefinition{
			Name:        toolID,
			Description: "authorization test tool",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
	if err := reg.Register(tool); err != nil {
		t.Fatalf("register %q: %v", toolID, err)
	}
}

func containsAuthTool(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}
