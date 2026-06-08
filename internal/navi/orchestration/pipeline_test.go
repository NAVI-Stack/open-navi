package orchestration

import (
	"context"
	"errors"
	"testing"

	"github.com/open-navi/navi/internal/llm"
)

type stubAssembler struct{}

func (stubAssembler) Assemble(context.Context, CanonicalRunRequest) (ContextPack, error) {
	return ContextPack{}, nil
}

type stubCompiler struct{}

func (stubCompiler) Compile(context.Context, CanonicalRunRequest, ContextPack) (InstructionStack, error) {
	return InstructionStack{}, nil
}

type stubAdapter struct{}

func (stubAdapter) CompileRequest(context.Context, CanonicalRunRequest, ContextPack, InstructionStack) (CompiledModelRequest, error) {
	return CompiledModelRequest{}, nil
}

func (stubAdapter) NormalizeResponse(context.Context, CompiledModelRequest, *llm.Response) (NormalizedModelResponse, error) {
	return NormalizedModelResponse{}, nil
}

type scriptedAssembler struct {
	pack ContextPack
	err  error
}

func (s scriptedAssembler) Assemble(context.Context, CanonicalRunRequest) (ContextPack, error) {
	return s.pack, s.err
}

type scriptedCompiler struct {
	stack InstructionStack
	err   error
}

func (s scriptedCompiler) Compile(context.Context, CanonicalRunRequest, ContextPack) (InstructionStack, error) {
	return s.stack, s.err
}

type scriptedAdapter struct {
	req  CompiledModelRequest
	resp NormalizedModelResponse
	err  error
}

func (s scriptedAdapter) CompileRequest(context.Context, CanonicalRunRequest, ContextPack, InstructionStack) (CompiledModelRequest, error) {
	return s.req, s.err
}

func (s scriptedAdapter) NormalizeResponse(context.Context, CompiledModelRequest, *llm.Response) (NormalizedModelResponse, error) {
	return s.resp, s.err
}

type recordingSink struct {
	events []TraceEvent
}

func (s *recordingSink) Record(_ context.Context, event TraceEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestInstructionStackOrdered(t *testing.T) {
	stack := InstructionStack{
		ExperienceOverlay: InstructionLayer{
			Fragments: []InstructionFragment{{Key: "experience", Content: "persona"}},
		},
		SystemCore: InstructionLayer{
			Key:       InstructionLayerCapabilitySurface,
			Fragments: []InstructionFragment{{Key: "identity", Content: "navi"}},
		},
		CapabilitySurface: InstructionLayer{
			Fragments: []InstructionFragment{{Key: "tools", Content: "runtime"}},
		},
	}

	got := stack.Ordered()
	if len(got) != 3 {
		t.Fatalf("expected 3 populated layers, got %d", len(got))
	}

	want := []InstructionLayerKey{
		InstructionLayerSystemCore,
		InstructionLayerExperienceOverlay,
		InstructionLayerCapabilitySurface,
	}
	for i, wantKey := range want {
		if got[i].Key != wantKey {
			t.Fatalf("ordered layer %d: expected %q, got %q", i, wantKey, got[i].Key)
		}
	}
}

func TestInstructionStackOrderedSkipsEmptyLayers(t *testing.T) {
	stack := InstructionStack{
		RuntimeConstraints: InstructionLayer{},
		TaskFraming: InstructionLayer{
			Fragments: []InstructionFragment{{Key: "task", Content: "complete the run"}},
		},
	}

	got := stack.Ordered()
	if len(got) != 1 {
		t.Fatalf("expected 1 populated layer, got %d", len(got))
	}
	if got[0].Key != InstructionLayerTaskFraming {
		t.Fatalf("expected task framing layer, got %q", got[0].Key)
	}
}

func TestPipelineValidate(t *testing.T) {
	t.Run("missing dependencies return sentinel errors", func(t *testing.T) {
		err := NewPipeline(nil, stubCompiler{}, nil, nil).Validate()
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !errors.Is(err, ErrMissingContextAssembler) {
			t.Fatalf("expected missing context assembler error, got %v", err)
		}
		if errors.Is(err, ErrMissingInstructionCompiler) {
			t.Fatalf("did not expect missing instruction compiler error, got %v", err)
		}
		if !errors.Is(err, ErrMissingModelAdapter) {
			t.Fatalf("expected missing model adapter error, got %v", err)
		}
	})

	t.Run("trace sink is optional", func(t *testing.T) {
		err := NewPipeline(stubAssembler{}, stubCompiler{}, stubAdapter{}, nil).Validate()
		if err != nil {
			t.Fatalf("expected validation success, got %v", err)
		}
	})
}

func TestPipelineReady(t *testing.T) {
	cases := []struct {
		name  string
		p     *Pipeline
		ready bool
	}{
		{name: "nil pipeline", p: nil, ready: false},
		{name: "missing assembler", p: NewPipeline(nil, stubCompiler{}, stubAdapter{}, nil), ready: false},
		{name: "missing compiler", p: NewPipeline(stubAssembler{}, nil, stubAdapter{}, nil), ready: false},
		{name: "missing adapter", p: NewPipeline(stubAssembler{}, stubCompiler{}, nil, nil), ready: false},
		{name: "required dependencies present", p: NewPipeline(stubAssembler{}, stubCompiler{}, stubAdapter{}, nil), ready: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.Ready(); got != tc.ready {
				t.Fatalf("expected ready=%t, got %t", tc.ready, got)
			}
		})
	}
}

func TestPipelineCompileAndNormalize(t *testing.T) {
	sink := &recordingSink{}
	pack := ContextPack{Items: []ContextItem{{ID: "ctx-1"}}}
	stack := InstructionStack{
		SystemCore: InstructionLayer{
			Fragments: []InstructionFragment{{Key: "identity", Content: "navi"}},
		},
	}
	compiledReq := CompiledModelRequest{
		Metadata: map[string]string{
			"trace_id":   "trace-123",
			"chat_id":    "sess-123",
			"run_id":     "run-123",
		},
	}
	normalizedResp := NormalizedModelResponse{
		Content:      "done",
		OutputTokens: 42,
	}
	pipeline := NewPipeline(
		scriptedAssembler{pack: pack},
		scriptedCompiler{stack: stack},
		scriptedAdapter{req: compiledReq, resp: normalizedResp},
		sink,
	)

	gotPack, gotStack, gotCompiled, err := pipeline.Compile(context.Background(), CanonicalRunRequest{
		Frame: ExecutionFrame{TraceID: "trace-123", ChatID: "sess-123", RunID: "run-123"},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(gotPack.Items) != 1 || gotPack.Items[0].ID != "ctx-1" {
		t.Fatalf("unexpected context pack: %#v", gotPack)
	}
	if len(gotStack.Ordered()) != 1 || gotStack.Ordered()[0].Key != InstructionLayerSystemCore {
		t.Fatalf("unexpected instruction stack: %#v", gotStack)
	}
	if gotCompiled.Metadata["run_id"] != "run-123" {
		t.Fatalf("unexpected compiled request metadata: %#v", gotCompiled.Metadata)
	}
	if gotCompiled.Metadata["trace_id"] != "trace-123" {
		t.Fatalf("expected trace metadata to be preserved, got %#v", gotCompiled.Metadata)
	}

	gotResp, err := pipeline.Normalize(context.Background(), gotCompiled, &llm.Response{Content: "done"})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if gotResp.Content != "done" || gotResp.OutputTokens != 42 {
		t.Fatalf("unexpected normalized response: %#v", gotResp)
	}
	if len(sink.events) != 5 {
		t.Fatalf("expected 5 trace events, got %d", len(sink.events))
	}
	wantStages := []string{
		"run_started",
		"context_assembled",
		"instructions_compiled",
		"provider_request_compiled",
		"model_response_normalized",
	}
	for i, want := range wantStages {
		if sink.events[i].Stage != want {
			t.Fatalf("trace event %d: expected stage %q, got %#v", i, want, sink.events[i])
		}
		if sink.events[i].TraceID != "trace-123" {
			t.Fatalf("trace event %d: expected trace_id %q, got %#v", i, "trace-123", sink.events[i])
		}
	}
}

func TestPipelineCompileFailureRecordsRunFailed(t *testing.T) {
	sink := &recordingSink{}
	compileErr := errors.New("boom")
	pipeline := NewPipeline(
		scriptedAssembler{pack: ContextPack{}},
		scriptedCompiler{err: compileErr},
		scriptedAdapter{},
		sink,
	)

	_, _, _, err := pipeline.Compile(context.Background(), CanonicalRunRequest{
		Frame: ExecutionFrame{TraceID: "trace-456", ChatID: "sess-456", RunID: "run-456"},
	})
	if err == nil {
		t.Fatal("expected compile error")
	}
	if len(sink.events) != 3 {
		t.Fatalf("expected 3 trace events, got %d", len(sink.events))
	}
	if sink.events[2].Stage != "run_failed" {
		t.Fatalf("expected run_failed trace event, got %#v", sink.events[2])
	}
	if sink.events[2].Metadata["failed_stage"] != "instruction_compile" {
		t.Fatalf("expected failed stage metadata, got %#v", sink.events[2].Metadata)
	}
}

func TestCoordinatorValidate(t *testing.T) {
	t.Run("nil coordinator validates missing pipeline dependencies", func(t *testing.T) {
		var coord *Coordinator
		err := coord.Validate()
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !errors.Is(err, ErrMissingContextAssembler) {
			t.Fatalf("expected missing context assembler error, got %v", err)
		}
		if !errors.Is(err, ErrMissingInstructionCompiler) {
			t.Fatalf("expected missing instruction compiler error, got %v", err)
		}
		if !errors.Is(err, ErrMissingModelAdapter) {
			t.Fatalf("expected missing model adapter error, got %v", err)
		}
	})

	t.Run("coordinator delegates to pipeline", func(t *testing.T) {
		coord := NewCoordinator(NewPipeline(stubAssembler{}, stubCompiler{}, stubAdapter{}, nil))
		if err := coord.Validate(); err != nil {
			t.Fatalf("expected validation success, got %v", err)
		}
	})
}
