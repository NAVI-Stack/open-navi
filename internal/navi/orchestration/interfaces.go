package orchestration

import (
	"context"

	"github.com/open-navi/navi/internal/llm"
)

type ContextAssembler interface {
	Assemble(ctx context.Context, req CanonicalRunRequest) (ContextPack, error)
}

type InstructionCompiler interface {
	Compile(ctx context.Context, req CanonicalRunRequest, pack ContextPack) (InstructionStack, error)
}

type ModelAdapter interface {
	CompileRequest(ctx context.Context, req CanonicalRunRequest, pack ContextPack, stack InstructionStack) (CompiledModelRequest, error)
	NormalizeResponse(ctx context.Context, req CompiledModelRequest, raw *llm.Response) (NormalizedModelResponse, error)
}

type TraceSink interface {
	Record(ctx context.Context, event TraceEvent) error
}
