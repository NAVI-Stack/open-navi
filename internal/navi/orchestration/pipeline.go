package orchestration

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/open-navi/navi/internal/llm"
)

type Pipeline struct {
	ContextAssembler    ContextAssembler
	InstructionCompiler InstructionCompiler
	ModelAdapter        ModelAdapter
	TraceSink           TraceSink
}

func NewPipeline(assembler ContextAssembler, compiler InstructionCompiler, adapter ModelAdapter, sink TraceSink) *Pipeline {
	return &Pipeline{
		ContextAssembler:    assembler,
		InstructionCompiler: compiler,
		ModelAdapter:        adapter,
		TraceSink:           sink,
	}
}

func (p *Pipeline) Ready() bool {
	return p != nil && p.ContextAssembler != nil && p.InstructionCompiler != nil && p.ModelAdapter != nil
}

func (p *Pipeline) Validate() error {
	var errs []error
	if p == nil || p.ContextAssembler == nil {
		errs = append(errs, ErrMissingContextAssembler)
	}
	if p == nil || p.InstructionCompiler == nil {
		errs = append(errs, ErrMissingInstructionCompiler)
	}
	if p == nil || p.ModelAdapter == nil {
		errs = append(errs, ErrMissingModelAdapter)
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

func (p *Pipeline) Compile(ctx context.Context, req CanonicalRunRequest) (ContextPack, InstructionStack, CompiledModelRequest, error) {
	if err := p.Validate(); err != nil {
		return ContextPack{}, InstructionStack{}, CompiledModelRequest{}, err
	}

	p.recordTrace(ctx, "run_started", req, map[string]string{
		"execution_mode":  string(req.Frame.Mode),
		"experience_mode": req.ExperienceMode,
	})
	pack, err := p.ContextAssembler.Assemble(ctx, req)
	if err != nil {
		p.recordFailure(ctx, "context_assemble", req, err)
		return ContextPack{}, InstructionStack{}, CompiledModelRequest{}, &StageError{Stage: "context_assemble", Err: err}
	}
	p.recordTrace(ctx, "context_assembled", req, map[string]string{
		"context_items":    itoa(len(pack.Items)),
		"context_warnings": itoa(len(pack.Warnings)),
	})

	stack, err := p.InstructionCompiler.Compile(ctx, req, pack)
	if err != nil {
		p.recordFailure(ctx, "instruction_compile", req, err)
		return ContextPack{}, InstructionStack{}, CompiledModelRequest{}, &StageError{Stage: "instruction_compile", Err: err}
	}
	p.recordTrace(ctx, "instructions_compiled", req, map[string]string{
		"context_items": itoa(len(pack.Items)),
		"layer_count":   itoa(len(stack.Ordered())),
	})

	compiled, err := p.ModelAdapter.CompileRequest(ctx, req, pack, stack)
	if err != nil {
		p.recordFailure(ctx, "model_request_compile", req, err)
		return ContextPack{}, InstructionStack{}, CompiledModelRequest{}, &StageError{Stage: "model_request_compile", Err: err}
	}
	p.recordTrace(ctx, "provider_request_compiled", req, map[string]string{
		"layer_count":    itoa(len(stack.Ordered())),
		"message_count":  itoa(len(compiled.Messages)),
		"resolved_tools": itoa(len(compiled.Tools)),
	})
	return pack, stack, compiled, nil
}

func (p *Pipeline) Normalize(ctx context.Context, req CompiledModelRequest, raw *llm.Response) (NormalizedModelResponse, error) {
	if err := p.Validate(); err != nil {
		return NormalizedModelResponse{}, err
	}
	resp, err := p.ModelAdapter.NormalizeResponse(ctx, req, raw)
	if err != nil {
		p.recordCompiledFailure(ctx, "model_response_normalize", req, err)
		return NormalizedModelResponse{}, &StageError{Stage: "model_response_normalize", Err: err}
	}
	p.recordTrace(ctx, "model_response_normalized", CanonicalRunRequest{
		Frame: ExecutionFrame{
			TraceID: req.Metadata["trace_id"],
			ChatID:  req.Metadata["chat_id"],
			RunID:   req.Metadata["run_id"],
		},
	}, map[string]string{
		"tool_calls":    itoa(len(resp.ToolCalls)),
		"output_tokens": itoa(resp.OutputTokens),
	})
	return resp, nil
}

func (p *Pipeline) recordTrace(ctx context.Context, stage string, req CanonicalRunRequest, meta map[string]string) {
	if p == nil || p.TraceSink == nil {
		return
	}
	_ = p.TraceSink.Record(ctx, TraceEvent{
		Stage:      stage,
		TraceID:    req.Frame.TraceID,
		RunID:      req.Frame.RunID,
		ChatID:     req.Frame.ChatID,
		OccurredAt: time.Now().UTC(),
		Metadata:   meta,
	})
}

func (p *Pipeline) recordFailure(ctx context.Context, failedStage string, req CanonicalRunRequest, err error) {
	p.recordTrace(ctx, "run_failed", req, map[string]string{
		"failed_stage": failedStage,
		"error":        err.Error(),
	})
}

func (p *Pipeline) recordCompiledFailure(ctx context.Context, failedStage string, req CompiledModelRequest, err error) {
	p.recordTrace(ctx, "run_failed", CanonicalRunRequest{
		Frame: ExecutionFrame{
			TraceID: req.Metadata["trace_id"],
			ChatID:  req.Metadata["chat_id"],
			RunID:   req.Metadata["run_id"],
		},
	}, map[string]string{
		"failed_stage": failedStage,
		"error":        err.Error(),
	})
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
