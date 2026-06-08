package instructions

import (
	"context"

	"github.com/ceoai/navi/internal/navi/orchestration"
)

type CompileRequest struct {
	SystemCore         SystemCoreInput
	RuntimeConstraints RuntimeConstraintsInput
	ExperienceOverlay  ExperienceOverlayInput
	TaskFrame          TaskFrameInput
	OutputContract     OutputContractInput
	CapabilitySurface  CapabilitySurfaceInput
	ExecutionFrame     ExecutionFrameInput
}

type Resolver interface {
	Resolve(ctx context.Context, req orchestration.CanonicalRunRequest, pack orchestration.ContextPack) (CompileRequest, error)
}

type ResolverFunc func(ctx context.Context, req orchestration.CanonicalRunRequest, pack orchestration.ContextPack) (CompileRequest, error)

func (f ResolverFunc) Resolve(ctx context.Context, req orchestration.CanonicalRunRequest, pack orchestration.ContextPack) (CompileRequest, error) {
	return f(ctx, req, pack)
}

type Compiler struct {
	Resolver Resolver
}

func NewCompiler(resolver Resolver) *Compiler {
	return &Compiler{Resolver: resolver}
}
