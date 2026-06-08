package navi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/command"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/inference"
	"github.com/open-navi/navi/internal/navi/orchestration"
	"github.com/open-navi/navi/internal/navi/render"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
	navitool "github.com/open-navi/navi/internal/tool"
)

func TestSnapshotForToolExecutionFailure_ClassifiesRecoverableTimeout(t *testing.T) {
	snapshot := snapshotForToolExecutionFailure("navi.test.timeout", schema.CommandTypeQuery, nil, context.DeadlineExceeded, context.DeadlineExceeded.Error())
	if snapshot.FailureClass != schema.FailureClassTimeout {
		t.Fatalf("expected timeout failure class, got %+v", snapshot)
	}
	if snapshot.FailureCode != string(navitool.ExecutionFailureCodeTransientTimeout) {
		t.Fatalf("expected transient_timeout failure code, got %+v", snapshot)
	}
	if snapshot.Outcome != schema.ExecutionOutcomeTimedOut {
		t.Fatalf("expected timed_out outcome, got %+v", snapshot)
	}
}

func TestSnapshotForToolExecutionFailure_ClassifiesGovernanceBlockAsNonFallback(t *testing.T) {
	err := errors.New("ICS control boundary blocked this tool call")
	snapshot := snapshotForToolExecutionFailure("navi.test.blocked", schema.CommandTypeInvoke, nil, err, err.Error())
	if snapshot.FailureClass != schema.FailureClassPolicyBlocked {
		t.Fatalf("expected policy_blocked failure class, got %+v", snapshot)
	}
	if snapshot.FailureCode != string(navitool.ExecutionFailureCodeGovernanceBlocked) {
		t.Fatalf("expected governance_blocked failure code, got %+v", snapshot)
	}
	if snapshot.Outcome != schema.ExecutionOutcomeRejectedPreExecution {
		t.Fatalf("expected rejected_pre_execution outcome, got %+v", snapshot)
	}
}

func TestSnapshotForToolExecutionFailure_CoversStructuredFailureTaxonomy(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantCode  string
		wantClass schema.FailureClass
	}{
		{
			name:      "auth_failure",
			err:       navitool.ErrorForFailureCode(navitool.ExecutionFailureCodeAuthFailure, "401 unauthorized against provider"),
			wantCode:  string(navitool.ExecutionFailureCodeAuthFailure),
			wantClass: schema.FailureClassPermissionDenial,
		},
		{
			name:      "schema_mismatch",
			err:       navitool.ErrorForFailureCode(navitool.ExecutionFailureCodeSchemaMismatch, "response missing required field"),
			wantCode:  string(navitool.ExecutionFailureCodeSchemaMismatch),
			wantClass: schema.FailureClassSchemaInvalid,
		},
		{
			name:      "api_contract_drift",
			err:       navitool.ErrorForFailureCode(navitool.ExecutionFailureCodeAPIContractDrift, "unexpected response shape"),
			wantCode:  string(navitool.ExecutionFailureCodeAPIContractDrift),
			wantClass: schema.FailureClassExecutionFailure,
		},
		{
			name:      "connector_unavailable",
			err:       navitool.ErrorForFailureCode(navitool.ExecutionFailureCodeConnectorUnavailable, "dial tcp 10.0.0.8:443"),
			wantCode:  string(navitool.ExecutionFailureCodeConnectorUnavailable),
			wantClass: schema.FailureClassConnectorUnavailable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := snapshotForToolExecutionFailure("navi.test.failure", schema.CommandTypeQuery, nil, tc.err, tc.err.Error())
			if snapshot.FailureCode != tc.wantCode {
				t.Fatalf("expected failure code %q, got %+v", tc.wantCode, snapshot)
			}
			if snapshot.FailureClass != tc.wantClass {
				t.Fatalf("expected failure class %q, got %+v", tc.wantClass, snapshot)
			}
			if snapshot.Summary == "" {
				t.Fatalf("expected raw summary to be preserved for observability, got %+v", snapshot)
			}
		})
	}
}

func TestSnapshotForToolExecutionFailure_ClassifiesPartialFailureComposeResult(t *testing.T) {
	composed := command.ComposeResult{
		Outcome:       schema.ExecutionOutcomePartiallySucceeded,
		FailureReason: "2 of 5 records failed validation",
	}
	err := navitool.ErrorForFailureCode(navitool.ExecutionFailureCodeExecutionFailed, "partial execution failure")

	snapshot := snapshotForToolExecutionFailure("navi.test.partial", schema.CommandTypeUpdate, composed, err, err.Error())
	if snapshot.Outcome != schema.ExecutionOutcomePartiallySucceeded {
		t.Fatalf("expected partial outcome, got %+v", snapshot)
	}
	if snapshot.FailureClass != schema.FailureClassPartialExecution {
		t.Fatalf("expected partial execution failure class, got %+v", snapshot)
	}
	if snapshot.FailureCode != string(navitool.ExecutionFailureCodePartialFailure) {
		t.Fatalf("expected partial_failure code, got %+v", snapshot)
	}
	if snapshot.Summary != "2 of 5 records failed validation" {
		t.Fatalf("expected compose failure reason in summary, got %+v", snapshot)
	}
}

func TestUserFacingToolFailureSummary_SanitizesNonDebugFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		code      navitool.ExecutionFailureCode
		raw       string
		want      string
		forbidden []string
	}{
		{
			name:      "connector_unavailable",
			code:      navitool.ExecutionFailureCodeConnectorUnavailable,
			raw:       "dial tcp 10.0.0.8:443: connect: connection refused",
			want:      "The connector or upstream service is currently unavailable.",
			forbidden: []string{"dial tcp", "10.0.0.8", ":443", "connection refused"},
		},
		{
			name:      "auth_failure",
			code:      navitool.ExecutionFailureCodeAuthFailure,
			raw:       "401 unauthorized: bearer sk-test-secret",
			want:      "Authentication failed for the selected tool.",
			forbidden: []string{"401", "unauthorized", "bearer", "sk-test-secret"},
		},
		{
			name:      "schema_mismatch",
			code:      navitool.ExecutionFailureCodeSchemaMismatch,
			raw:       "response missing required field $.data.items[0].id",
			want:      "The tool request or response did not match the expected schema.",
			forbidden: []string{"$.data.items[0].id", "missing required field", "items[0]"},
		},
		{
			name:      "api_contract_drift",
			code:      navitool.ExecutionFailureCodeAPIContractDrift,
			raw:       "response shape drift: unknown field commitSha in connector payload",
			want:      "The integration returned an unexpected response format.",
			forbidden: []string{"commitSha", "unknown field", "connector payload", "response shape drift"},
		},
		{
			name:      "transient_timeout",
			code:      navitool.ExecutionFailureCodeTransientTimeout,
			raw:       "context deadline exceeded",
			want:      "The selected tool timed out.",
			forbidden: []string{"context deadline exceeded"},
		},
		{
			name:      "selected_action_missing",
			code:      navitool.ExecutionFailureCodeSelectedActionMissing,
			raw:       "GitHub update_file action not found on connector",
			want:      "The selected action is currently unavailable.",
			forbidden: []string{"GitHub", "update_file", "action not found", "connector"},
		},
		{
			name:      "execution_failed",
			code:      navitool.ExecutionFailureCodeExecutionFailed,
			raw:       "panic: nil pointer dereference at internal/navi/runtime_executor.go:123",
			want:      "The selected tool failed during execution.",
			forbidden: []string{"panic:", "nil pointer dereference", "internal/navi/runtime_executor.go:123", "runtime_executor.go"},
		},
		{
			name:      "partial_failure",
			code:      navitool.ExecutionFailureCodePartialFailure,
			raw:       "2 of 5 records failed validation",
			want:      "The selected tool completed with partial failures.",
			forbidden: []string{"2 of 5", "records failed validation", "validation"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := userFacingToolFailureSummary(navitool.FailureForCode(tc.code, tc.raw), tc.raw, false)
			if got != tc.want {
				t.Fatalf("expected sanitized summary %q, got %q", tc.want, got)
			}
			if got == tc.raw {
				t.Fatalf("expected non-debug summary to avoid raw failure detail, got %q", got)
			}
			for _, forbidden := range tc.forbidden {
				if strings.Contains(got, forbidden) {
					t.Fatalf("expected non-debug summary %q to avoid leaking %q", got, forbidden)
				}
			}
		})
	}
}

func TestUserFacingToolFailureSummary_DebugIncludesRawDetailsWhenDifferent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		code navitool.ExecutionFailureCode
		raw  string
		want string
	}{
		{
			name: "selected_action_missing",
			code: navitool.ExecutionFailureCodeSelectedActionMissing,
			raw:  "GitHub update_file action not found on connector",
			want: "The selected action is currently unavailable.",
		},
		{
			name: "execution_failed",
			code: navitool.ExecutionFailureCodeExecutionFailed,
			raw:  "panic: nil pointer dereference at internal/navi/runtime_executor.go:123",
			want: "The selected tool failed during execution.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := userFacingToolFailureSummary(navitool.FailureForCode(tc.code, tc.raw), tc.raw, true)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected debug summary %q to include sanitized summary %q", got, tc.want)
			}
			if !strings.Contains(got, tc.raw) {
				t.Fatalf("expected debug summary %q to include raw detail %q", got, tc.raw)
			}
		})
	}
}

func TestOllamaToolRecoveryFallback(t *testing.T) {
	// Test case 1: Ollama profile, allows capability execution, target tool is surfaced
	// shouldRecoverMissingRequiredToolCall should return true
	compiled := orchestration.CompiledModelRequest{
		Profile: orchestration.ModelProfile{
			Provider: "ollama",
		},
		Tools: []llm.ToolDefinition{
			{Name: "read_file"},
		},
	}
	decision := inference.DecisionEnvelope{
		Rationale: inference.Rationale{
			DominantMode: inference.DecisionModeExecute,
			ExecutionIntent: inference.ExecutionIntent{
				TargetCapability: "read_file",
			},
		},
		ExecutionBoundary: inference.ExecutionBoundary{
			TargetCapability: "read_file",
		},
	}

	if !shouldRecoverMissingRequiredToolCall(compiled, decision) {
		t.Errorf("expected shouldRecoverMissingRequiredToolCall to be true for ollama with target capability")
	}

	target := missingRequiredToolCallTarget(compiled, decision)
	if target != "read_file" {
		t.Errorf("expected target tool to be 'read_file', got %q", target)
	}

	// Test case 2: Ollama profile, but target capability is not surfaced
	compiledNoTool := compiled
	compiledNoTool.Tools = []llm.ToolDefinition{}
	if shouldRecoverMissingRequiredToolCall(compiledNoTool, decision) {
		t.Errorf("expected shouldRecoverMissingRequiredToolCall to be false when tool is not surfaced")
	}

	// Test case 3: Non-ollama profile (e.g. anthropic), should not recover unless options/directive force it
	compiledAnthropic := compiled
	compiledAnthropic.Profile.Provider = "anthropic"
	if shouldRecoverMissingRequiredToolCall(compiledAnthropic, decision) {
		t.Errorf("expected shouldRecoverMissingRequiredToolCall to be false for anthropic without explicit tool choice")
	}

	// Test case 4: missingRequiredToolCallFallbackReply formatting for different surfaces
	webMsg := missingRequiredToolCallFallbackReply("read_file", "web")
	if !strings.Contains(webMsg, "Please switch to a tool-capable model") {
		t.Errorf("expected default fallback msg to mention model switching, got %q", webMsg)
	}

	telegramMsg := missingRequiredToolCallFallbackReply("read_file", "telegram")
	if strings.Contains(telegramMsg, "Please switch to a tool-capable model") {
		t.Errorf("expected telegram fallback msg NOT to mention model switching, got %q", telegramMsg)
	}
	if !strings.Contains(telegramMsg, "Please try again") {
		t.Errorf("expected telegram fallback msg to tell user to try again, got %q", telegramMsg)
	}

	// Test case 5: missingRequiredToolCallRepairPrompt formatting
	repairMsg := missingRequiredToolCallRepairPrompt("read_file")
	if !strings.Contains(repairMsg, "Runtime repair:") {
		t.Errorf("expected repair message to start with prefix, got %q", repairMsg)
	}
	if !strings.Contains(repairMsg, `"read_file"`) {
		t.Errorf("expected repair message to name target tool, got %q", repairMsg)
	}

	// Test case 6: missingRequiredToolCallRepairPrompt with empty tool name
	emptyRepairMsg := missingRequiredToolCallRepairPrompt("")
	if !strings.Contains(emptyRepairMsg, `"the required tool"`) {
		t.Errorf("expected empty repair message to fall back to 'the required tool', got %q", emptyRepairMsg)
	}
}

func TestApplyRenderToolRequirementForIntent(t *testing.T) {
	compiled := orchestration.CompiledModelRequest{
		Profile: orchestration.ModelProfile{Provider: "ollama"},
		Tools: []llm.ToolDefinition{
			{Name: renderVisualizeToolName},
			{Name: "navi.files.read"},
		},
		Options: llm.Options{MaxTokens: 512},
	}

	got := applyRenderToolRequirementForIntent(compiled, "Render a chart for this chat")
	if got.Options.ToolChoice != renderVisualizeToolName {
		t.Fatalf("ToolChoice = %q, want %q", got.Options.ToolChoice, renderVisualizeToolName)
	}
	if got.Options.MaxTokens != 512 {
		t.Fatalf("MaxTokens = %d, want preserved default 512", got.Options.MaxTokens)
	}
	if got.Metadata["runtime_required_tool_reason"] != "render_intent" {
		t.Fatalf("expected render-intent metadata, got %#v", got.Metadata)
	}
	if target := missingRequiredToolCallTarget(got, inference.DecisionEnvelope{}); target != renderVisualizeToolName {
		t.Fatalf("missingRequiredToolCallTarget = %q, want %q", target, renderVisualizeToolName)
	}
	if !shouldRecoverMissingRequiredToolCall(got, inference.DecisionEnvelope{}) {
		t.Fatal("expected Ollama render requirement to use missing-tool-call recovery")
	}

	plain := applyRenderToolRequirementForIntent(compiled, "Reply with exactly pong")
	if plain.Options.ToolChoice != "" {
		t.Fatalf("plain turn should not require render tool, got %q", plain.Options.ToolChoice)
	}

	withoutRenderTool := compiled
	withoutRenderTool.Tools = []llm.ToolDefinition{{Name: "navi.files.read"}}
	withoutRenderTool = applyRenderToolRequirementForIntent(withoutRenderTool, "Render a chart for this chat")
	if withoutRenderTool.Options.ToolChoice != "" {
		t.Fatalf("unavailable render tool should not be required, got %q", withoutRenderTool.Options.ToolChoice)
	}
}

func TestExecuteRunRenderIntentMissingToolCallAttachesPayload(t *testing.T) {
	chats := &mockChatStore{
		thread: &ChatThread{
			Chat: Chat{
				ID: "chat-render",
				AIConfig: ChatAIConfig{
					Model: "ollama",
				},
			},
			Messages: []ChatMessage{
				{
					ID:      "msg-render",
					ChatID:  "chat-render",
					Role:    "user",
					Content: "Render a chart for this chat now",
				},
			},
		},
	}
	llmProv := &mockLLM{
		responses: []*llm.Response{{Content: ""}},
	}
	ctrl := &mockController{
		envelopeHistory: []inference.DecisionEnvelope{{
			RuntimeDisposition: inference.RuntimeDispositionCallModel,
			ModelDirective: &inference.ModelDirective{
				AllowToolCalls: true,
			},
		}},
	}
	loop := NewAgentLoop(LoopConfig{
		Chats:               chats,
		LLM:                 llmProv,
		InferenceController: ctrl,
		ICSStateStore:       &mockICSStateStore{},
		Model:               "ollama",
	})
	run := naviruntime.NewRun("runtime-render", string(ExperienceModeStandard))
	run.ChatID = "chat-render"

	res, err := loop.ExecuteRun(context.Background(), naviruntime.ExecuteInput{Run: run})
	if err != nil {
		t.Fatalf("ExecuteRun failed: %v", err)
	}
	if !res.Completed {
		t.Fatal("expected render fallback run to complete")
	}
	if len(run.RenderPayload) == 0 {
		t.Fatal("expected render fallback to attach a render payload")
	}
	var payload render.RenderPayload
	if err := json.Unmarshal(run.RenderPayload, &payload); err != nil {
		t.Fatalf("unmarshal render payload: %v", err)
	}
	if payload.Mode != render.RenderModeOpenUI {
		t.Fatalf("render payload mode = %q, want openui", payload.Mode)
	}
	if payload.OpenUILang == "" || payload.FallbackMarkdown == "" || payload.DataView == nil {
		t.Fatalf("expected complete render payload, got %+v", payload)
	}
	if !strings.Contains(res.FinalContent, "Tool usage in this chat") {
		t.Fatalf("expected fallback markdown final content, got %q", res.FinalContent)
	}
}

func TestExecuteRunRenderIntentVisualizeAliasAttachesPayload(t *testing.T) {
	chats := &mockChatStore{
		thread: &ChatThread{
			Chat: Chat{
				ID: "chat-render-alias",
				AIConfig: ChatAIConfig{
					Model: "ollama",
				},
			},
			Messages: []ChatMessage{
				{
					ID:      "msg-render-alias",
					ChatID:  "chat-render-alias",
					Role:    "user",
					Content: "Render a chart for this chat now",
				},
			},
		},
	}
	llmProv := &mockLLM{
		responses: []*llm.Response{{
			ToolCalls: []llm.ToolCall{{
				ID:        "call-render",
				Name:      "visualize",
				Arguments: map[string]any{"view": "chart"},
			}},
		}},
	}
	loop := NewAgentLoop(LoopConfig{
		Chats:         chats,
		LLM:           llmProv,
		ICSStateStore: &mockICSStateStore{},
		Model:         "ollama",
	})
	run := naviruntime.NewRun("runtime-render-alias", string(ExperienceModeStandard))
	run.ChatID = "chat-render-alias"

	res, err := loop.ExecuteRun(context.Background(), naviruntime.ExecuteInput{Run: run})
	if err != nil {
		t.Fatalf("ExecuteRun failed: %v", err)
	}
	if !res.Completed {
		t.Fatal("expected render alias run to complete")
	}
	if len(run.RenderPayload) == 0 {
		t.Fatalf("expected render alias to attach a render payload, final content: %q", res.FinalContent)
	}
	var payload render.RenderPayload
	if err := json.Unmarshal(run.RenderPayload, &payload); err != nil {
		t.Fatalf("unmarshal render payload: %v", err)
	}
	if payload.Mode != render.RenderModeOpenUI {
		t.Fatalf("render payload mode = %q, want openui", payload.Mode)
	}
}

func TestCanonicalizeRenderToolCallsNormalizesOnlySurfacedRenderIntent(t *testing.T) {
	compiled := orchestration.CompiledModelRequest{
		Tools: []llm.ToolDefinition{{Name: renderVisualizeToolName}},
	}
	aliasResponse := func() orchestration.NormalizedModelResponse {
		return orchestration.NormalizedModelResponse{
			ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "visualize"}},
		}
	}

	got := canonicalizeRenderToolCalls(aliasResponse(), compiled, "Render a chart for this chat")
	if got.ToolCalls[0].Name != renderVisualizeToolName {
		t.Fatalf("tool call name = %q, want %q", got.ToolCalls[0].Name, renderVisualizeToolName)
	}

	plain := canonicalizeRenderToolCalls(aliasResponse(), compiled, "Reply with exactly pong")
	if plain.ToolCalls[0].Name != "visualize" {
		t.Fatalf("plain turn should not rewrite tool call, got %q", plain.ToolCalls[0].Name)
	}

	noSurface := canonicalizeRenderToolCalls(aliasResponse(), orchestration.CompiledModelRequest{}, "Render a chart")
	if noSurface.ToolCalls[0].Name != "visualize" {
		t.Fatalf("unavailable render tool should not rewrite tool call, got %q", noSurface.ToolCalls[0].Name)
	}
}

func TestCanonicalizeRenderToolCallsPreservesUnknownToolNames(t *testing.T) {
	compiled := orchestration.CompiledModelRequest{
		Tools: []llm.ToolDefinition{{Name: renderVisualizeToolName}},
	}
	normalized := orchestration.NormalizedModelResponse{
		ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "unknown.visualize_data"}},
	}

	got := canonicalizeRenderToolCalls(normalized, compiled, "Render a chart")
	if got.ToolCalls[0].Name != "unknown.visualize_data" {
		t.Fatalf("unrelated tool name should not be canonicalized, got %q", got.ToolCalls[0].Name)
	}
}

type mockChatStore struct {
	thread *ChatThread
}

func (s *mockChatStore) CreateChat(context.Context, CreateChatInput) (*Chat, error) {
	return nil, errors.New("unexpected CreateChat")
}

func (s *mockChatStore) RenameChat(context.Context, string, string) error {
	return errors.New("unexpected RenameChat")
}

func (s *mockChatStore) GetChat(context.Context, string) (*Chat, error) {
	return &s.thread.Chat, nil
}

func (s *mockChatStore) GetChatWithMessages(ctx context.Context, _ string) (*ChatThread, error) {
	return s.thread, nil
}

func (s *mockChatStore) ListChats(context.Context, int) ([]Chat, error) {
	return nil, errors.New("unexpected ListChats")
}

func (s *mockChatStore) ListChatsByProject(context.Context, string, int) ([]Chat, error) {
	return nil, errors.New("unexpected ListChatsByProject")
}

func (s *mockChatStore) UpdateChat(context.Context, Chat) error {
	return nil
}

func (s *mockChatStore) ArchiveChat(context.Context, string) error {
	return errors.New("unexpected ArchiveChat")
}

func (s *mockChatStore) DeleteChat(context.Context, string) error {
	return errors.New("unexpected DeleteChat")
}

func (s *mockChatStore) AppendChatMessage(context.Context, string, ChatMessage) (string, error) {
	return "msg-append", nil
}

func (s *mockChatStore) AppendSystemAssistantMessage(context.Context, string, string, string, string, string) (string, error) {
	return "msg-sys-assist", nil
}

func (s *mockChatStore) ListChatMessages(context.Context, string, int, int) ([]ChatMessage, error) {
	return s.thread.Messages, nil
}

func (s *mockChatStore) MessageCount(context.Context, string) (int, error) {
	return len(s.thread.Messages), nil
}

func (s *mockChatStore) ActiveChatID(context.Context, ActiveChatScope) (string, error) {
	return "", nil
}

func (s *mockChatStore) SetActiveChat(context.Context, ActiveChatScope, string) error {
	return nil
}

type mockICSStateStore struct {
	state *ICSState
}

func (m *mockICSStateStore) SaveICSState(ctx context.Context, runID string, envelope inference.DecisionEnvelope) (*ICSState, error) {
	m.state = &ICSState{
		RunID:            runID,
		Version:          "1",
		DecisionEnvelope: envelope,
	}
	return m.state, nil
}

func (m *mockICSStateStore) LoadICSState(ctx context.Context, runID string) (*ICSState, error) {
	return m.state, nil
}

func (m *mockICSStateStore) AppendICSHistory(ctx context.Context, runID string, envelope inference.DecisionEnvelope) (*ICSHistory, error) {
	return nil, nil
}

func (m *mockICSStateStore) LoadICSHistory(ctx context.Context, runID string, limit int) ([]ICSHistory, error) {
	return nil, nil
}

type mockLLM struct {
	responses []*llm.Response
	errs      []error
	callCount int
	lastOpts  llm.Options
}

func (m *mockLLM) Chat(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
	m.lastOpts = opts
	var err error
	if m.callCount < len(m.errs) {
		err = m.errs[m.callCount]
	}
	var resp *llm.Response
	if m.callCount < len(m.responses) {
		resp = m.responses[m.callCount]
	} else if err == nil {
		resp = &llm.Response{Content: "Default reply"}
	}
	m.callCount++
	return resp, err
}

func (m *mockLLM) Name() string {
	return "ollama"
}

type mockController struct {
	decideSynthesis      inference.DecisionSynthesis
	envelopeHistory      []inference.DecisionEnvelope
	callIndex            int
	successAfterAttempts int
}

func (m *mockController) Decide(ctx context.Context, input inference.InferenceInput) (inference.DecisionSynthesis, error) {
	return m.decideSynthesis, nil
}

func (m *mockController) ValidateGovernedDecision(ctx context.Context, input inference.InferenceInput, synthesis inference.DecisionSynthesis) (inference.DecisionEnvelope, error) {
	if m.callIndex < len(m.envelopeHistory) {
		env := m.envelopeHistory[m.callIndex]
		m.callIndex++
		return env, nil
	}
	return inference.DecisionEnvelope{
		RuntimeDisposition: inference.RuntimeDispositionProceedDirect,
	}, nil
}

func (m *mockController) ObserveOutcome(ctx context.Context, prior inference.DecisionEnvelope, snapshot inference.ExecutionSnapshot) (inference.DecisionEnvelope, error) {
	return prior, nil
}

func (m *mockController) PrepareModelCall(ctx context.Context, prior inference.DecisionEnvelope, input inference.ModelCallPreparationInput) (inference.DecisionEnvelope, error) {
	return prior, nil
}

func (m *mockController) AuthorizeModelResponse(ctx context.Context, prior inference.DecisionEnvelope, input inference.ModelResponseAuthorizationInput) (inference.DecisionEnvelope, error) {
	env := prior
	if len(input.Response.ToolCalls) == 0 {
		if m.successAfterAttempts > 0 && input.RepairAttemptsRemaining == m.successAfterAttempts {
			env.RuntimeDisposition = inference.RuntimeDispositionProceedDirect
			env.ExecutionBoundary.TargetCapability = ""
		} else {
			env.RuntimeDisposition = inference.RuntimeDispositionCallModel
			env.ExecutionBoundary.TargetCapability = "navi.test.read_file"
		}
	} else {
		env.RuntimeDisposition = inference.RuntimeDispositionProceedDirect
		env.ExecutionBoundary.TargetCapability = ""
	}
	return env, nil
}

func (m *mockController) AuthorizeToolCall(ctx context.Context, prior inference.DecisionEnvelope, input inference.ToolAuthorizationInput) (inference.DecisionEnvelope, error) {
	return prior, nil
}

func TestExecuteRunOllamaMissingToolRecovery_Exhausted(t *testing.T) {
	reg := navitool.NewRegistry()
	if err := reg.Register(&navitool.Tool{
		ToolID:                "navi.test.read_file",
		DisplayName:           "read_file",
		Description:           "Read a file from disk",
		Source:                navitool.ToolSourceFileTools,
		SourceID:              "tests",
		SchemaVersion:         "1.0.0",
		InputSchema:           map[string]any{"type": "object", "properties": map[string]any{}},
		OutputSchema:          map[string]any{"type": "string"},
		Category:              navitool.ToolCategoryWorkflowAction,
		RiskTier:              "high",
		SideEffects:           []string{"workspace_write"},
		Reversibility:         "irreversible",
		EnvironmentVisibility: []string{"development"},
		RequiredModes:         []schema.DirectiveMode{},
		RequiredAuthority:     navitool.ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{},
		CapabilityTags:        []string{},
		Status:                navitool.ToolStatusActive,
		Definition: llm.ToolDefinition{
			Name: "navi.test.read_file",
		},
	}); err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	chats := &mockChatStore{
		thread: &ChatThread{
			Chat: Chat{
				ID: "chat-1",
				AIConfig: ChatAIConfig{
					Model: "ollama",
				},
			},
			Messages: []ChatMessage{
				{
					ID:      "msg-1",
					ChatID:  "chat-1",
					Role:    "user",
					Content: "Read a file",
				},
			},
		},
	}

	llmProv := &mockLLM{
		responses: []*llm.Response{
			{Content: "I want to read the file (Attempt 1)"},
			{Content: "I want to read the file (Attempt 2)"},
			{Content: "I want to read the file (Attempt 3)"},
		},
	}

	ctrl := &mockController{
		decideSynthesis: inference.DecisionSynthesis{
			Rationale: inference.Rationale{
				ExecutionIntent: inference.ExecutionIntent{
					ActionType:       "act",
					TargetCapability: "navi.test.read_file",
				},
			},
		},
		envelopeHistory: []inference.DecisionEnvelope{
			{
				RuntimeDisposition: inference.RuntimeDispositionCallModel,
				ExecutionBoundary: inference.ExecutionBoundary{
					TargetCapability: "navi.test.read_file",
				},
				ModelDirective: &inference.ModelDirective{
					AllowToolCalls: true,
					ToolNames:      []string{"navi.test.read_file"},
				},
				Rationale: inference.Rationale{
					DominantMode: inference.DecisionModeExecute,
					ExecutionIntent: inference.ExecutionIntent{
						ActionType:       "act",
						TargetCapability: "navi.test.read_file",
					},
				},
			},
			{
				RuntimeDisposition: inference.RuntimeDispositionCallModel,
				ExecutionBoundary: inference.ExecutionBoundary{
					TargetCapability: "navi.test.read_file",
				},
				ModelDirective: &inference.ModelDirective{
					AllowToolCalls: true,
					ToolNames:      []string{"navi.test.read_file"},
				},
				Rationale: inference.Rationale{
					DominantMode: inference.DecisionModeExecute,
					ExecutionIntent: inference.ExecutionIntent{
						ActionType:       "act",
						TargetCapability: "navi.test.read_file",
					},
				},
			},
			{
				RuntimeDisposition: inference.RuntimeDispositionCallModel,
				ExecutionBoundary: inference.ExecutionBoundary{
					TargetCapability: "navi.test.read_file",
				},
				ModelDirective: &inference.ModelDirective{
					AllowToolCalls: true,
					ToolNames:      []string{"navi.test.read_file"},
				},
				Rationale: inference.Rationale{
					DominantMode: inference.DecisionModeExecute,
					ExecutionIntent: inference.ExecutionIntent{
						ActionType:       "act",
						TargetCapability: "navi.test.read_file",
					},
				},
			},
		},
		successAfterAttempts: 0,
	}

	stateStore := &mockICSStateStore{}

	loop := NewAgentLoop(LoopConfig{
		Chats:               chats,
		LLM:                 llmProv,
		InferenceController: ctrl,
		ICSStateStore:       stateStore,
		ToolRegistry:        reg,
		Model:               "ollama",
	})

	run := naviruntime.NewRun("runtime-1", string(ExperienceModeStandard))
	run.ChatID = "chat-1"

	res, err := loop.ExecuteRun(context.Background(), naviruntime.ExecuteInput{Run: run})
	if err != nil {
		t.Fatalf("ExecuteRun failed: %v", err)
	}

	if !res.Completed {
		t.Errorf("expected run to be completed")
	}
	if res.Outcome != schema.ExecutionOutcomeFailed {
		t.Errorf("expected execution outcome failed, got %s", res.Outcome)
	}
	if !strings.Contains(res.FinalContent, "Please switch to a tool-capable model") {
		t.Errorf("expected fallback message about switching to tool-capable model, got %q", res.FinalContent)
	}

	if llmProv.callCount != 3 {
		t.Errorf("expected LLM call count to be 3, got %d", llmProv.callCount)
	}
}

func TestExecuteRunOllamaMissingToolRecovery_Success(t *testing.T) {
	reg := navitool.NewRegistry()
	if err := reg.Register(&navitool.Tool{
		ToolID:                "navi.test.read_file",
		DisplayName:           "read_file",
		Description:           "Read a file from disk",
		Source:                navitool.ToolSourceFileTools,
		SourceID:              "tests",
		SchemaVersion:         "1.0.0",
		InputSchema:           map[string]any{"type": "object", "properties": map[string]any{}},
		OutputSchema:          map[string]any{"type": "string"},
		Category:              navitool.ToolCategoryWorkflowAction,
		RiskTier:              "high",
		SideEffects:           []string{"workspace_write"},
		Reversibility:         "irreversible",
		EnvironmentVisibility: []string{"development"},
		RequiredModes:         []schema.DirectiveMode{},
		RequiredAuthority:     navitool.ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{},
		CapabilityTags:        []string{},
		Status:                navitool.ToolStatusActive,
		Definition: llm.ToolDefinition{
			Name: "navi.test.read_file",
		},
	}); err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	chats := &mockChatStore{
		thread: &ChatThread{
			Chat: Chat{
				ID: "chat-1",
				AIConfig: ChatAIConfig{
					Model: "ollama",
				},
			},
			Messages: []ChatMessage{
				{
					ID:      "msg-1",
					ChatID:  "chat-1",
					Role:    "user",
					Content: "Read a file",
				},
			},
		},
	}

	llmProv := &mockLLM{
		responses: []*llm.Response{
			{Content: "I want to read the file (Attempt 1)"},
			{Content: "Successfully answered without tool or tool not needed."},
		},
	}

	ctrl := &mockController{
		decideSynthesis: inference.DecisionSynthesis{
			Rationale: inference.Rationale{
				ExecutionIntent: inference.ExecutionIntent{
					ActionType:       "act",
					TargetCapability: "navi.test.read_file",
				},
			},
		},
		envelopeHistory: []inference.DecisionEnvelope{
			{
				RuntimeDisposition: inference.RuntimeDispositionCallModel,
				ExecutionBoundary: inference.ExecutionBoundary{
					TargetCapability: "navi.test.read_file",
				},
				ModelDirective: &inference.ModelDirective{
					AllowToolCalls: true,
					ToolNames:      []string{"navi.test.read_file"},
				},
				Rationale: inference.Rationale{
					DominantMode: inference.DecisionModeExecute,
					ExecutionIntent: inference.ExecutionIntent{
						ActionType:       "act",
						TargetCapability: "navi.test.read_file",
					},
				},
			},
		},
		successAfterAttempts: 1,
	}

	stateStore := &mockICSStateStore{}

	loop := NewAgentLoop(LoopConfig{
		Chats:               chats,
		LLM:                 llmProv,
		InferenceController: ctrl,
		ICSStateStore:       stateStore,
		ToolRegistry:        reg,
		Model:               "ollama",
	})

	run := naviruntime.NewRun("runtime-1", string(ExperienceModeStandard))
	run.ChatID = "chat-1"

	res, err := loop.ExecuteRun(context.Background(), naviruntime.ExecuteInput{Run: run})
	if err != nil {
		t.Fatalf("ExecuteRun failed: %v", err)
	}

	if !res.Completed {
		t.Errorf("expected run to be completed")
	}
	if res.Outcome != schema.ExecutionOutcomeSucceeded {
		t.Errorf("expected execution outcome succeeded, got %s", res.Outcome)
	}
	if !strings.Contains(res.FinalContent, "Successfully answered") {
		t.Errorf("expected final content to contain success message, got %q", res.FinalContent)
	}

	if llmProv.callCount != 2 {
		t.Errorf("expected LLM call count to be 2, got %d", llmProv.callCount)
	}
}

func TestExecuteRunOllamaMissingToolRecovery_ProviderError_Exhausted(t *testing.T) {
	reg := navitool.NewRegistry()
	if err := reg.Register(&navitool.Tool{
		ToolID:                "navi.test.read_file",
		DisplayName:           "read_file",
		Description:           "Read a file from disk",
		Source:                navitool.ToolSourceFileTools,
		SourceID:              "tests",
		SchemaVersion:         "1.0.0",
		InputSchema:           map[string]any{"type": "object", "properties": map[string]any{}},
		OutputSchema:          map[string]any{"type": "string"},
		Category:              navitool.ToolCategoryWorkflowAction,
		RiskTier:              "high",
		SideEffects:           []string{"workspace_write"},
		Reversibility:         "irreversible",
		EnvironmentVisibility: []string{"development"},
		RequiredModes:         []schema.DirectiveMode{},
		RequiredAuthority:     navitool.ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{},
		CapabilityTags:        []string{},
		Status:                navitool.ToolStatusActive,
		Definition: llm.ToolDefinition{
			Name: "navi.test.read_file",
		},
	}); err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	chats := &mockChatStore{
		thread: &ChatThread{
			Chat: Chat{
				ID: "chat-1",
				AIConfig: ChatAIConfig{
					Model: "ollama",
				},
			},
			Messages: []ChatMessage{
				{
					ID:      "msg-1",
					ChatID:  "chat-1",
					Role:    "user",
					Content: "Read a file",
				},
			},
		},
	}

	llmProv := &mockLLM{
		errs: []error{
			fmt.Errorf("ollama: model \"llama3\" failed to call tools for tool-requiring intent"),
			fmt.Errorf("ollama: model \"llama3\" failed to call tools for tool-requiring intent"),
			fmt.Errorf("ollama: model \"llama3\" failed to call tools for tool-requiring intent"),
		},
	}

	ctrl := &mockController{
		decideSynthesis: inference.DecisionSynthesis{
			Rationale: inference.Rationale{
				ExecutionIntent: inference.ExecutionIntent{
					ActionType:       "act",
					TargetCapability: "navi.test.read_file",
				},
			},
		},
		envelopeHistory: []inference.DecisionEnvelope{
			{
				RuntimeDisposition: inference.RuntimeDispositionCallModel,
				ExecutionBoundary: inference.ExecutionBoundary{
					TargetCapability: "navi.test.read_file",
				},
				ModelDirective: &inference.ModelDirective{
					AllowToolCalls: true,
					ToolNames:      []string{"navi.test.read_file"},
				},
				Rationale: inference.Rationale{
					DominantMode: inference.DecisionModeExecute,
					ExecutionIntent: inference.ExecutionIntent{
						ActionType:       "act",
						TargetCapability: "navi.test.read_file",
					},
				},
			},
			{
				RuntimeDisposition: inference.RuntimeDispositionCallModel,
				ExecutionBoundary: inference.ExecutionBoundary{
					TargetCapability: "navi.test.read_file",
				},
				ModelDirective: &inference.ModelDirective{
					AllowToolCalls: true,
					ToolNames:      []string{"navi.test.read_file"},
				},
				Rationale: inference.Rationale{
					DominantMode: inference.DecisionModeExecute,
					ExecutionIntent: inference.ExecutionIntent{
						ActionType:       "act",
						TargetCapability: "navi.test.read_file",
					},
				},
			},
			{
				RuntimeDisposition: inference.RuntimeDispositionCallModel,
				ExecutionBoundary: inference.ExecutionBoundary{
					TargetCapability: "navi.test.read_file",
				},
				ModelDirective: &inference.ModelDirective{
					AllowToolCalls: true,
					ToolNames:      []string{"navi.test.read_file"},
				},
				Rationale: inference.Rationale{
					DominantMode: inference.DecisionModeExecute,
					ExecutionIntent: inference.ExecutionIntent{
						ActionType:       "act",
						TargetCapability: "navi.test.read_file",
					},
				},
			},
		},
		successAfterAttempts: 0,
	}

	stateStore := &mockICSStateStore{}

	loop := NewAgentLoop(LoopConfig{
		Chats:               chats,
		LLM:                 llmProv,
		InferenceController: ctrl,
		ICSStateStore:       stateStore,
		ToolRegistry:        reg,
		Model:               "ollama",
	})

	run := naviruntime.NewRun("runtime-1", string(ExperienceModeStandard))
	run.ChatID = "chat-1"
	run.SetScratchpadValue("source_channel", "telegram")

	res, err := loop.ExecuteRun(context.Background(), naviruntime.ExecuteInput{
		Run: run,
		InboxItem: &naviruntime.InboxItem{
			SourceChannel: "telegram",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRun failed: %v", err)
	}

	if !res.Completed {
		t.Errorf("expected run to be completed")
	}
	if res.Outcome != schema.ExecutionOutcomeFailed {
		t.Errorf("expected execution outcome failed, got %s", res.Outcome)
	}
	if !strings.Contains(res.FinalContent, "I couldn't complete that with the current model because it did not emit the required tool call") {
		t.Errorf("expected Telegram fallback message, got %q", res.FinalContent)
	}
	if strings.Contains(res.FinalContent, "Please switch to a tool-capable model") {
		t.Errorf("Telegram fallback message should not mention switching to tool-capable model, got %q", res.FinalContent)
	}

	if llmProv.callCount != 3 {
		t.Errorf("expected LLM call count to be 3, got %d", llmProv.callCount)
	}
}

func TestExecuteRunOllamaMissingToolRecovery_ProviderError_Success(t *testing.T) {
	reg := navitool.NewRegistry()
	if err := reg.Register(&navitool.Tool{
		ToolID:                "navi.test.read_file",
		DisplayName:           "read_file",
		Description:           "Read a file from disk",
		Source:                navitool.ToolSourceFileTools,
		SourceID:              "tests",
		SchemaVersion:         "1.0.0",
		InputSchema:           map[string]any{"type": "object", "properties": map[string]any{}},
		OutputSchema:          map[string]any{"type": "string"},
		Category:              navitool.ToolCategoryWorkflowAction,
		RiskTier:              "high",
		SideEffects:           []string{"workspace_write"},
		Reversibility:         "irreversible",
		EnvironmentVisibility: []string{"development"},
		RequiredModes:         []schema.DirectiveMode{},
		RequiredAuthority:     navitool.ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{},
		CapabilityTags:        []string{},
		Status:                navitool.ToolStatusActive,
		Definition: llm.ToolDefinition{
			Name: "navi.test.read_file",
		},
	}); err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	chats := &mockChatStore{
		thread: &ChatThread{
			Chat: Chat{
				ID: "chat-1",
				AIConfig: ChatAIConfig{
					Model: "ollama",
				},
			},
			Messages: []ChatMessage{
				{
					ID:      "msg-1",
					ChatID:  "chat-1",
					Role:    "user",
					Content: "Read a file",
				},
			},
		},
	}

	llmProv := &mockLLM{
		errs: []error{
			fmt.Errorf("ollama: model \"llama3\" failed to call tools for tool-requiring intent"),
		},
		responses: []*llm.Response{
			nil, // for call 0, which gets the error
			{Content: "Successfully answered without tool or tool not needed."},
		},
	}

	ctrl := &mockController{
		decideSynthesis: inference.DecisionSynthesis{
			Rationale: inference.Rationale{
				ExecutionIntent: inference.ExecutionIntent{
					ActionType:       "act",
					TargetCapability: "navi.test.read_file",
				},
			},
		},
		envelopeHistory: []inference.DecisionEnvelope{
			{
				RuntimeDisposition: inference.RuntimeDispositionCallModel,
				ExecutionBoundary: inference.ExecutionBoundary{
					TargetCapability: "navi.test.read_file",
				},
				ModelDirective: &inference.ModelDirective{
					AllowToolCalls: true,
					ToolNames:      []string{"navi.test.read_file"},
				},
				Rationale: inference.Rationale{
					DominantMode: inference.DecisionModeExecute,
					ExecutionIntent: inference.ExecutionIntent{
						ActionType:       "act",
						TargetCapability: "navi.test.read_file",
					},
				},
			},
			{
				RuntimeDisposition: inference.RuntimeDispositionCallModel,
				ExecutionBoundary: inference.ExecutionBoundary{
					TargetCapability: "navi.test.read_file",
				},
				ModelDirective: &inference.ModelDirective{
					AllowToolCalls: true,
					ToolNames:      []string{"navi.test.read_file"},
				},
				Rationale: inference.Rationale{
					DominantMode: inference.DecisionModeExecute,
					ExecutionIntent: inference.ExecutionIntent{
						ActionType:       "act",
						TargetCapability: "navi.test.read_file",
					},
				},
			},
		},
		successAfterAttempts: 1,
	}

	stateStore := &mockICSStateStore{}

	loop := NewAgentLoop(LoopConfig{
		Chats:               chats,
		LLM:                 llmProv,
		InferenceController: ctrl,
		ICSStateStore:       stateStore,
		ToolRegistry:        reg,
		Model:               "ollama",
	})

	run := naviruntime.NewRun("runtime-1", string(ExperienceModeStandard))
	run.ChatID = "chat-1"

	res, err := loop.ExecuteRun(context.Background(), naviruntime.ExecuteInput{Run: run})
	if err != nil {
		t.Fatalf("ExecuteRun failed: %v", err)
	}

	if !res.Completed {
		t.Errorf("expected run to be completed")
	}
	if res.Outcome != schema.ExecutionOutcomeSucceeded {
		t.Errorf("expected execution outcome succeeded, got %s", res.Outcome)
	}
	if !strings.Contains(res.FinalContent, "Successfully answered") {
		t.Errorf("expected final content to contain success message, got %q", res.FinalContent)
	}

	if llmProv.callCount != 2 {
		t.Errorf("expected LLM call count to be 2, got %d", llmProv.callCount)
	}
}

func TestExecuteRunOllama_Telegram_TextParsingSuccess(t *testing.T) {
	reg := navitool.NewRegistry()
	if err := reg.Register(&navitool.Tool{
		ToolID:                "navi.test.read_file",
		DisplayName:           "read_file",
		Description:           "Read a file from disk",
		Source:                navitool.ToolSourceFileTools,
		SourceID:              "tests",
		SchemaVersion:         "1.0.0",
		InputSchema:           map[string]any{"type": "object", "properties": map[string]any{}},
		OutputSchema:          map[string]any{"type": "string"},
		Category:              navitool.ToolCategoryWorkflowAction,
		RiskTier:              "high",
		SideEffects:           []string{"workspace_write"},
		Reversibility:         "irreversible",
		EnvironmentVisibility: []string{"development"},
		RequiredModes:         []schema.DirectiveMode{},
		RequiredAuthority:     navitool.ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{},
		CapabilityTags:        []string{},
		Status:                navitool.ToolStatusActive,
		Definition: llm.ToolDefinition{
			Name: "navi.test.read_file",
		},
	}); err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	chats := &mockChatStore{
		thread: &ChatThread{
			Chat: Chat{
				ID: "chat-1",
				AIConfig: ChatAIConfig{
					Model: "ollama",
				},
			},
			Messages: []ChatMessage{
				{
					ID:      "msg-1",
					ChatID:  "chat-1",
					Role:    "user",
					Content: "Read a file",
				},
			},
		},
	}

	llmProv := &mockLLM{
		responses: []*llm.Response{
			{
				Content: "Let me call the tool:\n```json\n{\n  \"name\": \"navi.test.read_file\",\n  \"arguments\": {}\n}\n```",
				ToolCalls: []llm.ToolCall{
					{
						ID:        "call_parsed",
						Name:      "navi.test.read_file",
						Arguments: map[string]any{},
					},
				},
			},
		},
	}

	ctrl := &mockController{
		decideSynthesis: inference.DecisionSynthesis{
			Rationale: inference.Rationale{
				ExecutionIntent: inference.ExecutionIntent{
					ActionType:       "act",
					TargetCapability: "navi.test.read_file",
				},
			},
		},
		envelopeHistory: []inference.DecisionEnvelope{
			{
				RuntimeDisposition: inference.RuntimeDispositionCallModel,
				ExecutionBoundary: inference.ExecutionBoundary{
					TargetCapability: "navi.test.read_file",
				},
				ModelDirective: &inference.ModelDirective{
					AllowToolCalls: true,
					ToolNames:      []string{"navi.test.read_file"},
				},
				Rationale: inference.Rationale{
					DominantMode: inference.DecisionModeExecute,
					ExecutionIntent: inference.ExecutionIntent{
						ActionType:       "act",
						TargetCapability: "navi.test.read_file",
					},
				},
			},
		},
		successAfterAttempts: 0,
	}

	stateStore := &mockICSStateStore{}

	loop := NewAgentLoop(LoopConfig{
		Chats:               chats,
		LLM:                 llmProv,
		InferenceController: ctrl,
		ICSStateStore:       stateStore,
		ToolRegistry:        reg,
		Model:               "ollama",
	})

	run := naviruntime.NewRun("runtime-1", string(ExperienceModeStandard))
	run.ChatID = "chat-1"
	run.SetScratchpadValue("source_channel", "telegram")

	res, err := loop.ExecuteRun(context.Background(), naviruntime.ExecuteInput{
		Run: run,
		InboxItem: &naviruntime.InboxItem{
			SourceChannel: "telegram",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRun failed: %v", err)
	}

	if !res.Completed {
		t.Errorf("expected run to be completed")
	}
	if res.Outcome != schema.ExecutionOutcomeSucceeded {
		t.Errorf("expected execution outcome succeeded, got %s", res.Outcome)
	}

	if llmProv.callCount != 2 {
		t.Errorf("expected LLM call count to be 2, got %d", llmProv.callCount)
	}
}
