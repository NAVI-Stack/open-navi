package model

import (
	"context"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/orchestration"
	"github.com/open-navi/navi/internal/schema"
	navitool "github.com/open-navi/navi/internal/tool"
)

func TestDefaultAdapterCompileRequestBuildsMessagesAndTools(t *testing.T) {
	adapter := NewDefaultAdapter(nil, ToolResolverFunc(func(names []string) []llm.ToolDefinition {
		out := make([]llm.ToolDefinition, 0, len(names))
		for _, name := range names {
			out = append(out, llm.ToolDefinition{Name: name})
		}
		return out
	}))

	req := orchestration.CanonicalRunRequest{
		Frame: orchestration.ExecutionFrame{
			ChatID: "sess-1",
			RunID:  "run-1",
			Mode:   orchestration.ExecutionModeRunExecute,
		},
		ExperienceMode: "standard",
		UserMessage:    "Please help",
		Model: orchestration.ModelProfile{
			Provider:                    "ollama",
			Model:                       "llama3.1",
			SupportsSystemRole:          true,
			SupportsMultiSystemMessages: true,
		},
		CapabilitySurface: orchestration.SkillSurfaceRef{
			ToolNames: []string{"read_file", "send_reply"},
		},
	}
	pack := orchestration.ContextPack{
		Items: []orchestration.ContextItem{
			{
				ID:           "c1",
				Label:        "recent user message",
				Content:      "Earlier question",
				Source:       orchestration.ContextSourceHistory,
				SourceRef:    "sess-1",
				Trust:        orchestration.TrustLabelUserSupplied,
				ContextClass: orchestration.ContextClassConversation,
				Attributes:   map[string]string{"role": "user"},
			},
			{
				ID:           "f1",
				Label:        "facts block",
				Content:      "Owner prefers concise replies.",
				Source:       orchestration.ContextSourceFacts,
				SourceRef:    "facts:sess-1",
				Trust:        orchestration.TrustLabelDerived,
				ContextClass: orchestration.ContextClassFacts,
			},
		},
	}
	stack := orchestration.InstructionStack{
		SystemCore: orchestration.InstructionLayer{
			Key: orchestration.InstructionLayerSystemCore,
			Fragments: []orchestration.InstructionFragment{{
				Key:     "identity",
				Content: "You are NAVI.",
			}},
		},
		TaskFraming: orchestration.InstructionLayer{
			Key: orchestration.InstructionLayerTaskFraming,
			Fragments: []orchestration.InstructionFragment{{
				Key:     "task_frame",
				Content: "objective: Please help",
			}},
		},
	}

	compiled, err := adapter.CompileRequest(context.Background(), req, pack, stack)
	if err != nil {
		t.Fatalf("compile request: %v", err)
	}
	if len(compiled.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(compiled.Tools))
	}
	if compiled.Options.MaxTokens != defaultMaxResponseTokens {
		t.Fatalf("expected default max tokens %d, got %+v", defaultMaxResponseTokens, compiled.Options)
	}
	if compiled.Options.ToolsRequired {
		t.Fatalf("surfaced tools should be optional for a normal chat completion, got %+v", compiled.Options)
	}
	if compiled.Options.RepeatPenalty != 1.2 || compiled.Options.RepeatLastN != 128 {
		t.Fatalf("expected ollama repeat guard options, got %+v", compiled.Options)
	}
	if len(compiled.Messages) < 3 {
		t.Fatalf("expected system, context, and user messages, got %+v", compiled.Messages)
	}
	if compiled.Messages[0].Role != "system" {
		t.Fatalf("expected first message to be system, got %+v", compiled.Messages[0])
	}
	if !strings.Contains(compiled.Messages[0].Content, "System Core") {
		t.Fatalf("expected rendered layer heading, got %q", compiled.Messages[0].Content)
	}
	foundConversation := false
	for _, msg := range compiled.Messages {
		if msg.Role == "user" && msg.Content == "Earlier question" {
			foundConversation = true
		}
	}
	if !foundConversation {
		t.Fatalf("expected conversation history to compile into chat messages, got %+v", compiled.Messages)
	}
}

func TestDefaultOptionsDoesNotRequireAvailableTools(t *testing.T) {
	opts := DefaultOptionsForRequest(orchestration.ModelProfile{
		Provider: "ollama",
		Model:    "gemma3:latest",
	}, orchestration.CanonicalRunRequest{
		RequiredOutput: orchestration.RequiredOutput{
			MustReply:      true,
			AllowToolCalls: true,
		},
		CapabilitySurface: orchestration.SkillSurfaceRef{
			ToolNames: []string{"navi.messaging.send_reply"},
		},
	})

	if opts.ToolsRequired {
		t.Fatalf("ToolsRequired should remain false when tools are merely available: %+v", opts)
	}
	if opts.ToolCallingRequired {
		t.Fatalf("ToolCallingRequired should be set only by explicit runtime directives: %+v", opts)
	}
}

func TestDefaultOptionsUsesSmallerBudgetForPlainChat(t *testing.T) {
	opts := DefaultOptionsForRequest(orchestration.ModelProfile{
		Provider: "ollama",
		Model:    "gemma4:26b",
	}, orchestration.CanonicalRunRequest{
		RequiredOutput: orchestration.RequiredOutput{
			MustReply:      true,
			AllowToolCalls: false,
		},
	})

	if opts.MaxTokens != defaultPlainChatResponseTokens {
		t.Fatalf("plain no-tool chat should use latency-bounded max tokens %d, got %+v", defaultPlainChatResponseTokens, opts)
	}
}

func TestDefaultOptionsKeepsFullBudgetForToolTurns(t *testing.T) {
	opts := DefaultOptionsForRequest(orchestration.ModelProfile{
		Provider: "ollama",
		Model:    "gemma4:26b",
	}, orchestration.CanonicalRunRequest{
		RequiredOutput: orchestration.RequiredOutput{
			MustReply:      true,
			AllowToolCalls: true,
		},
		CapabilitySurface: orchestration.SkillSurfaceRef{
			ToolNames: []string{"navi.files.read"},
		},
	})

	if opts.MaxTokens != defaultMaxResponseTokens {
		t.Fatalf("tool-capable turns should keep full max tokens %d, got %+v", defaultMaxResponseTokens, opts)
	}
}

func TestDefaultAdapterCompileRequestAddsProviderQuirksFromProfileDefaults(t *testing.T) {
	adapter := NewDefaultAdapter(nil, nil)

	compiled, err := adapter.CompileRequest(context.Background(), orchestration.CanonicalRunRequest{
		ExperienceMode: "wizard",
		Model: orchestration.ModelProfile{
			Provider: "ollama",
			Model:    "llama3.1",
		},
	}, orchestration.ContextPack{}, orchestration.InstructionStack{
		SystemCore: orchestration.InstructionLayer{
			Fragments: []orchestration.InstructionFragment{{Key: "identity", Content: "You are NAVI."}},
		},
	})
	if err != nil {
		t.Fatalf("compile request: %v", err)
	}
	if !ProfileHasQuirk(compiled.Profile, QuirkSingleSystemMessage) {
		t.Fatalf("expected single-system-message quirk, got %+v", compiled.Profile)
	}
	if !ProfileHasQuirk(compiled.Profile, QuirkPlainFunctionalStyle) {
		t.Fatalf("expected plain-functional-style quirk, got %+v", compiled.Profile)
	}
	if compiled.Options.Temperature != 0.6 {
		t.Fatalf("expected wizard temperature, got %+v", compiled.Options)
	}
}

func TestDefaultAdapterCompileRequestResolvesToolsFromCapabilityLayer(t *testing.T) {
	adapter := NewDefaultAdapter(nil, ToolResolverFunc(func(names []string) []llm.ToolDefinition {
		out := make([]llm.ToolDefinition, 0, len(names))
		for _, name := range names {
			out = append(out, llm.ToolDefinition{Name: name})
		}
		return out
	}))

	compiled, err := adapter.CompileRequest(context.Background(), orchestration.CanonicalRunRequest{}, orchestration.ContextPack{}, orchestration.InstructionStack{
		CapabilitySurface: orchestration.InstructionLayer{
			Fragments: []orchestration.InstructionFragment{{
				Key:     "capability_surface",
				Content: "surface: runtime\ntools: read_file, send_reply\nselection_reason: foreground run",
			}},
		},
	})
	if err != nil {
		t.Fatalf("compile request: %v", err)
	}
	if len(compiled.Tools) != 2 {
		t.Fatalf("expected 2 tools from capability layer, got %+v", compiled.Tools)
	}
}

func TestDefaultAdapterCompileRequestSkipsCapabilityContextRendering(t *testing.T) {
	adapter := NewDefaultAdapter(nil, nil)
	compiled, err := adapter.CompileRequest(context.Background(), orchestration.CanonicalRunRequest{}, orchestration.ContextPack{
		Items: []orchestration.ContextItem{
			{
				Label:        "capability surface",
				Content:      "tools: read_file",
				Source:       orchestration.ContextSourceCapability,
				SourceRef:    "surface:runtime",
				Trust:        orchestration.TrustLabelAuthoritative,
				ContextClass: orchestration.ContextClassCapabilitySurface,
			},
			{
				Label:        "facts block",
				Content:      "Owner prefers concise replies.",
				Source:       orchestration.ContextSourceFacts,
				SourceRef:    "facts:sess-1",
				Trust:        orchestration.TrustLabelDerived,
				ContextClass: orchestration.ContextClassFacts,
			},
		},
	}, orchestration.InstructionStack{
		SystemCore: orchestration.InstructionLayer{
			Fragments: []orchestration.InstructionFragment{{Key: "identity", Content: "You are NAVI."}},
		},
	})
	if err != nil {
		t.Fatalf("compile request: %v", err)
	}
	if len(compiled.Messages) == 0 {
		t.Fatalf("expected compiled messages")
	}
	joined := make([]string, 0, len(compiled.Messages))
	for _, msg := range compiled.Messages {
		joined = append(joined, msg.Content)
	}
	text := strings.Join(joined, "\n")
	if strings.Contains(text, "capability surface") || strings.Contains(text, "tools: read_file") {
		t.Fatalf("expected capability context to be skipped from rendered system messages, got %q", text)
	}
	if !strings.Contains(text, "Owner prefers concise replies.") {
		t.Fatalf("expected non-capability context to remain rendered, got %q", text)
	}
}

func TestDefaultAdapterCompileRequestMetadataCountsResolvedTools(t *testing.T) {
	adapter := NewDefaultAdapter(nil, ToolResolverFunc(func(names []string) []llm.ToolDefinition {
		if len(names) == 0 {
			return nil
		}
		return []llm.ToolDefinition{{Name: names[0]}}
	}))

	compiled, err := adapter.CompileRequest(context.Background(), orchestration.CanonicalRunRequest{
		CapabilitySurface: orchestration.SkillSurfaceRef{
			ToolNames: []string{"read_file", "missing_tool"},
		},
	}, orchestration.ContextPack{}, orchestration.InstructionStack{})
	if err != nil {
		t.Fatalf("compile request: %v", err)
	}
	if got := compiled.Metadata["tool_count"]; got != "1" {
		t.Fatalf("expected tool_count metadata to reflect resolved tool payload, got %q with tools %+v", got, compiled.Tools)
	}
}

func TestDefaultAdapterCompileRequestAppendsRepeatedUserTurnWhenLatestTurnIsNew(t *testing.T) {
	adapter := NewDefaultAdapter(nil, nil)
	req := orchestration.CanonicalRunRequest{
		UserMessage: "retry",
	}
	pack := orchestration.ContextPack{
		Items: []orchestration.ContextItem{
			{
				ID:           "u1",
				Content:      "retry",
				Source:       orchestration.ContextSourceHistory,
				SourceRef:    "sess-1",
				Trust:        orchestration.TrustLabelUserSupplied,
				ContextClass: orchestration.ContextClassConversation,
				Attributes:   map[string]string{"role": "user"},
			},
			{
				ID:           "a1",
				Content:      "still working",
				Source:       orchestration.ContextSourceHistory,
				SourceRef:    "sess-1",
				Trust:        orchestration.TrustLabelDerived,
				ContextClass: orchestration.ContextClassConversation,
				Attributes:   map[string]string{"role": "assistant"},
			},
		},
	}

	compiled, err := adapter.CompileRequest(context.Background(), req, pack, orchestration.InstructionStack{})
	if err != nil {
		t.Fatalf("compile request: %v", err)
	}
	if got := compiled.Messages[len(compiled.Messages)-1]; got.Role != "user" || got.Content != "retry" {
		t.Fatalf("expected latest repeated user turn to be appended, got %+v", compiled.Messages)
	}
}

func TestDefaultAdapterCompileRequestHonorsSingleSystemMessageQuirk(t *testing.T) {
	adapter := NewDefaultAdapter(nil, nil)
	req := orchestration.CanonicalRunRequest{
		Model: orchestration.ModelProfile{
			Provider:                    "openai",
			Model:                       "gpt",
			SupportsSystemRole:          true,
			SupportsMultiSystemMessages: false,
			Quirks:                      []string{QuirkSingleSystemMessage},
		},
	}
	pack := orchestration.ContextPack{
		Items: []orchestration.ContextItem{{
			Label:        "facts block",
			Content:      "Remember this fact.",
			Source:       orchestration.ContextSourceFacts,
			SourceRef:    "facts",
			Trust:        orchestration.TrustLabelDerived,
			ContextClass: orchestration.ContextClassFacts,
		}},
	}
	stack := orchestration.InstructionStack{
		SystemCore: orchestration.InstructionLayer{
			Fragments: []orchestration.InstructionFragment{{Key: "identity", Content: "You are NAVI."}},
		},
		CapabilitySurface: orchestration.InstructionLayer{
			Fragments: []orchestration.InstructionFragment{{Key: "capability_surface", Content: "tools: read_file"}},
		},
	}

	compiled, err := adapter.CompileRequest(context.Background(), req, pack, stack)
	if err != nil {
		t.Fatalf("compile request: %v", err)
	}
	systemCount := 0
	for _, msg := range compiled.Messages {
		if msg.Role == "system" {
			systemCount++
		}
	}
	if systemCount != 1 {
		t.Fatalf("expected one merged system message, got %+v", compiled.Messages)
	}
	if !strings.Contains(compiled.Messages[0].Content, "Remember this fact.") {
		t.Fatalf("expected merged system message to include context item, got %q", compiled.Messages[0].Content)
	}
}

func TestDefaultAdapterNormalizeResponsePreservesRawPayload(t *testing.T) {
	adapter := NewDefaultAdapter(nil, nil)
	req := orchestration.CompiledModelRequest{
		Profile: orchestration.ModelProfile{
			Provider: "anthropic",
			Model:    "claude",
		},
	}
	raw := &llm.Response{
		Content:      "done",
		ToolCalls:    []llm.ToolCall{{ID: "t1", Name: "read_file"}},
		FinishReason: "stop",
		InputTokens:  11,
		OutputTokens: 7,
	}

	normalized, err := adapter.NormalizeResponse(context.Background(), req, raw)
	if err != nil {
		t.Fatalf("normalize response: %v", err)
	}
	if normalized.Content != "done" || normalized.FinishReason != "stop" {
		t.Fatalf("unexpected normalized response: %+v", normalized)
	}
	if len(normalized.ToolCalls) != 1 {
		t.Fatalf("expected tool calls to be preserved, got %+v", normalized.ToolCalls)
	}
	if normalized.RawPayload["provider"] != "anthropic" || normalized.RawPayload["model"] != "claude" {
		t.Fatalf("expected raw payload metadata, got %+v", normalized.RawPayload)
	}
	if normalized.RawPayload["content"] != "done" {
		t.Fatalf("expected raw payload content, got %+v", normalized.RawPayload)
	}
}

func TestDefaultAdapterCompileRequestExcludesHiddenInternalAndDevToolPayloads(t *testing.T) {
	registry := navitool.NewRegistry()
	for _, toolEntry := range []*navitool.Tool{
		modelAdapterTestTool("visible"),
		modelAdapterTestTool("hidden", func(tool *navitool.Tool) { tool.Hidden = true }),
		modelAdapterTestTool("internal", func(tool *navitool.Tool) { tool.Metadata.ExposureClass = navitool.ToolExposureInternal }),
		modelAdapterTestTool("development", func(tool *navitool.Tool) { tool.Metadata.ExposureClass = navitool.ToolExposureDevelopment }),
		modelAdapterTestTool("test", func(tool *navitool.Tool) { tool.Metadata.ExposureClass = navitool.ToolExposureTest }),
	} {
		if err := registry.Register(toolEntry); err != nil {
			t.Fatalf("register %q: %v", toolEntry.ToolID, err)
		}
	}

	surface := orchestration.NewCapabilitySurfaceResolver(registry).Resolve(orchestration.SurfaceResolutionInput{
		Surface:       orchestration.CapabilitySurface{Surface: orchestration.CapabilitySurfaceLoop},
		ExecutionMode: orchestration.ExecutionModeChatTurn,
		Model:         orchestration.ModelProfile{SupportsTools: true},
	})
	if surface.IsGuarded() {
		t.Fatalf("expected unguarded surface resolution, got %+v", surface)
	}

	adapter := NewDefaultAdapter(nil, ToolResolverFunc(func(names []string) []llm.ToolDefinition {
		out := make([]llm.ToolDefinition, 0, len(names))
		for _, name := range names {
			lookup, ok := registry.LookupExact(name)
			if !ok || lookup.Tool == nil {
				continue
			}
			out = append(out, lookup.Tool.Definition)
		}
		return out
	}))

	compiled, err := adapter.CompileRequest(context.Background(), orchestration.CanonicalRunRequest{
		Model: orchestration.ModelProfile{
			Provider:      "openai",
			Model:         "gpt-4.1",
			SupportsTools: true,
		},
		CapabilitySurface: surface.Resolved,
	}, orchestration.ContextPack{}, orchestration.InstructionStack{})
	if err != nil {
		t.Fatalf("compile request: %v", err)
	}

	if len(compiled.Tools) != 1 || compiled.Tools[0].Name != "test.model.visible" {
		t.Fatalf("expected only visible user-facing tool in compiled payload, got %+v", compiled.Tools)
	}
}

func modelAdapterTestTool(name string, opts ...func(*navitool.Tool)) *navitool.Tool {
	toolEntry := &navitool.Tool{
		ToolID:        "test.model." + name,
		DisplayName:   name,
		Description:   "model adapter test tool",
		Source:        navitool.ToolSourceBuiltin,
		SourceID:      "tests.model.adapter",
		SchemaVersion: "1.0.0",
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
		EnvironmentVisibility: []string{"production", "development"},
		RequiredModes:         []schema.DirectiveMode{},
		RequiredAuthority:     navitool.ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{},
		CapabilityTags:        []string{"adapter"},
		Status:                navitool.ToolStatusActive,
		Definition: llm.ToolDefinition{
			Name:        "test.model." + name,
			Description: "model adapter test tool",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
	for _, opt := range opts {
		opt(toolEntry)
	}
	return toolEntry
}
