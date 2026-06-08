package instructions

import (
	"context"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/navi/orchestration"
)

func TestCompileLayersPreservesCanonicalOrderAndOwnership(t *testing.T) {
	stack := CompileLayers(CompileRequest{
		SystemCore: SystemCoreInput{
			IdentityRules: []string{"You are NAVI."},
		},
		RuntimeConstraints: RuntimeConstraintsInput{
			GovernanceNotes: []string{"runtime state is authoritative"},
		},
		ExperienceOverlay: ExperienceOverlayInput{
			Fragment: "## Experience Control\nbe helpful",
		},
		TaskFrame: TaskFrameInput{
			Objective:    "answer the user",
			Instructions: []string{"stay grounded"},
		},
		OutputContract: OutputContractInput{
			Present:   true,
			MustReply: true,
		},
		CapabilitySurface: CapabilitySurfaceInput{
			Surface:         "runtime",
			ToolNames:       []string{"read_file", "write_file"},
			SelectionReason: "active surface",
		},
		ExecutionFrame: ExecutionFrameInput{
			Frame: orchestration.ExecutionFrame{
				ChatID: "sess-1",
				RunID:  "run-1",
				Mode:   orchestration.ExecutionModeRunExecute,
			},
		},
	})

	ordered := stack.Ordered()
	if len(ordered) != 5 {
		t.Fatalf("expected 5 layers, got %d", len(ordered))
	}
	wantOrder := []orchestration.InstructionLayerKey{
		orchestration.InstructionLayerSystemCore,
		orchestration.InstructionLayerRuntimeConstraints,
		orchestration.InstructionLayerExperienceOverlay,
		orchestration.InstructionLayerTaskFraming,
		orchestration.InstructionLayerCapabilitySurface,
	}
	for i, want := range wantOrder {
		if ordered[i].Key != want {
			t.Fatalf("layer %d: expected %q, got %q", i, want, ordered[i].Key)
		}
	}

	assertFragmentKeys(t, stack.RuntimeConstraints, "runtime_constraints", "execution_frame")
	assertFragmentKeys(t, stack.TaskFraming, "task_frame", "output_contract")
}

func TestCompilerDoesNotPullContextPackIntoSystemOrRuntimeLayers(t *testing.T) {
	compiler := NewCompiler(nil)
	req := orchestration.CanonicalRunRequest{
		UserMessage: "help me",
		RequiredOutput: orchestration.RequiredOutput{
			MustReply: true,
		},
	}
	pack := orchestration.ContextPack{
		Items: []orchestration.ContextItem{
			{ID: "facts-1", Content: "EXTERNAL FACT PAYLOAD"},
			{ID: "summary-1", Content: "SESSION SUMMARY PAYLOAD"},
		},
	}

	stack, err := compiler.Compile(context.Background(), req, pack)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	for _, fragment := range stack.SystemCore.Fragments {
		if strings.Contains(fragment.Content, "EXTERNAL FACT PAYLOAD") || strings.Contains(fragment.Content, "SESSION SUMMARY PAYLOAD") {
			t.Fatalf("system core leaked context-pack content: %q", fragment.Content)
		}
	}
	for _, fragment := range stack.RuntimeConstraints.Fragments {
		if strings.Contains(fragment.Content, "EXTERNAL FACT PAYLOAD") || strings.Contains(fragment.Content, "SESSION SUMMARY PAYLOAD") {
			t.Fatalf("runtime constraints leaked context-pack content: %q", fragment.Content)
		}
	}
}

func TestCompilerMapsExecutionFrameAndOutputContractIntoExistingRootFields(t *testing.T) {
	compiler := NewCompiler(ResolverFunc(func(context.Context, orchestration.CanonicalRunRequest, orchestration.ContextPack) (CompileRequest, error) {
		return CompileRequest{
			OutputContract: OutputContractInput{
				Present:               true,
				MustReply:             true,
				AllowToolCalls:        true,
				AllowScheduledReplies: true,
				ExpectStreaming:       true,
			},
			ExecutionFrame: ExecutionFrameInput{
				Frame: orchestration.ExecutionFrame{
					ChatID: "sess-42",
					RunID:  "run-42",
					Phase:  "contextualize",
					Mode:   orchestration.ExecutionModeResume,
					Scratchpad: map[string]string{
						"current_user_message": "hello",
					},
				},
			},
		}, nil
	}))

	stack, err := compiler.Compile(context.Background(), orchestration.CanonicalRunRequest{}, orchestration.ContextPack{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	assertFragmentKeys(t, stack.TaskFraming, "output_contract")
	assertFragmentKeys(t, stack.RuntimeConstraints, "execution_frame")

	if !strings.Contains(stack.RuntimeConstraints.Fragments[0].Content, "chat_id: sess-42") &&
		!strings.Contains(stack.RuntimeConstraints.Fragments[len(stack.RuntimeConstraints.Fragments)-1].Content, "chat_id: sess-42") {
		t.Fatalf("expected execution frame content in runtime constraints, got %+v", stack.RuntimeConstraints.Fragments)
	}
}

func TestCapabilitySurfacePreservesToolNamesAndSelectionReason(t *testing.T) {
	stack := CompileLayers(CompileRequest{
		CapabilitySurface: CapabilitySurfaceInput{
			Surface:         "runtime",
			ToolNames:       []string{"read_file", "send_reply"},
			SelectionReason: "foreground run",
		},
	})

	if len(stack.CapabilitySurface.Fragments) != 1 {
		t.Fatalf("expected one capability fragment, got %d", len(stack.CapabilitySurface.Fragments))
	}
	content := stack.CapabilitySurface.Fragments[0].Content
	for _, want := range []string{"surface: runtime", "tools: read_file, send_reply", "selection_reason: foreground run"} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected capability surface to contain %q, got %q", want, content)
		}
	}
}

func TestCompileLayersSkipsEmptyInputs(t *testing.T) {
	stack := CompileLayers(CompileRequest{})
	if got := stack.Ordered(); len(got) != 0 {
		t.Fatalf("expected no populated layers, got %d", len(got))
	}
}

func TestCompilerDefaultRequestMapsCanonicalRequestWithoutResolver(t *testing.T) {
	compiler := NewCompiler(nil)
	req := orchestration.CanonicalRunRequest{
		Frame: orchestration.ExecutionFrame{
			ChatID: "sess-default",
			RunID:  "run-default",
			Mode:   orchestration.ExecutionModeRunExecute,
		},
		UserMessage:    "inspect the repo",
		ExperienceMode: "navi",
		RequiredOutput: orchestration.RequiredOutput{
			MustReply:      true,
			AllowToolCalls: true,
		},
		CapabilitySurface: orchestration.SkillSurfaceRef{
			Surface:         "runtime",
			ToolNames:       []string{"read_file"},
			SelectionReason: "runtime execution surface",
		},
	}

	stack, err := compiler.Compile(context.Background(), req, orchestration.ContextPack{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(stack.TaskFraming.Fragments) == 0 || !strings.Contains(stack.TaskFraming.Fragments[0].Content, "objective: inspect the repo") {
		t.Fatalf("expected default task frame objective, got %+v", stack.TaskFraming.Fragments)
	}
	if len(stack.TaskFraming.Fragments) < 2 || !strings.Contains(stack.TaskFraming.Fragments[1].Content, "allow_tool_calls: true") {
		t.Fatalf("expected default output contract, got %+v", stack.TaskFraming.Fragments)
	}
	if len(stack.CapabilitySurface.Fragments) == 0 || !strings.Contains(stack.CapabilitySurface.Fragments[0].Content, "tools: read_file") {
		t.Fatalf("expected default capability surface, got %+v", stack.CapabilitySurface.Fragments)
	}
	if len(stack.RuntimeConstraints.Fragments) == 0 || !strings.Contains(stack.RuntimeConstraints.Fragments[len(stack.RuntimeConstraints.Fragments)-1].Content, "run_id: run-default") {
		t.Fatalf("expected default execution frame, got %+v", stack.RuntimeConstraints.Fragments)
	}
}

func TestCompilerResolverMergesWithoutDroppingBaseDefaults(t *testing.T) {
	compiler := NewCompiler(ResolverFunc(func(context.Context, orchestration.CanonicalRunRequest, orchestration.ContextPack) (CompileRequest, error) {
		return CompileRequest{
			SystemCore: SystemCoreInput{
				IdentityRules: []string{"You are NAVI."},
			},
			TaskFrame: TaskFrameInput{
				Instructions: []string{"be concise"},
			},
			OutputContract: OutputContractInput{
				ExpectStreaming: true,
			},
		}, nil
	}))
	req := orchestration.CanonicalRunRequest{
		UserMessage: "summarize this",
		RequiredOutput: orchestration.RequiredOutput{
			MustReply: true,
		},
	}

	stack, err := compiler.Compile(context.Background(), req, orchestration.ContextPack{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(stack.SystemCore.Fragments) == 0 || !strings.Contains(stack.SystemCore.Fragments[0].Content, "You are NAVI.") {
		t.Fatalf("expected resolver-provided system core, got %+v", stack.SystemCore.Fragments)
	}
	taskFrame := stack.TaskFraming.Fragments[0].Content
	if !strings.Contains(taskFrame, "objective: summarize this") || !strings.Contains(taskFrame, "be concise") {
		t.Fatalf("expected merged task framing, got %q", taskFrame)
	}
	outputContract := stack.TaskFraming.Fragments[1].Content
	if !strings.Contains(outputContract, "must_reply: true") || !strings.Contains(outputContract, "expect_streaming: true") {
		t.Fatalf("expected merged output contract, got %q", outputContract)
	}
}

func TestOutputContractCanBeExplicitlyAllFalse(t *testing.T) {
	stack := CompileLayers(CompileRequest{
		OutputContract: OutputContractInput{
			Present:               true,
			MustReply:             false,
			AllowToolCalls:        false,
			AllowScheduledReplies: false,
			ExpectStreaming:       false,
		},
	})

	assertFragmentKeys(t, stack.TaskFraming, "output_contract")
	content := stack.TaskFraming.Fragments[0].Content
	for _, want := range []string{
		"must_reply: false",
		"allow_tool_calls: false",
		"allow_scheduled_replies: false",
		"expect_streaming: false",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected output contract to contain %q, got %q", want, content)
		}
	}
}

func assertFragmentKeys(t *testing.T, layer orchestration.InstructionLayer, want ...string) {
	t.Helper()
	if len(layer.Fragments) != len(want) {
		t.Fatalf("expected %d fragments, got %d (%+v)", len(want), len(layer.Fragments), layer.Fragments)
	}
	for i, wantKey := range want {
		if layer.Fragments[i].Key != wantKey {
			t.Fatalf("fragment %d: expected %q, got %q", i, wantKey, layer.Fragments[i].Key)
		}
	}
}
