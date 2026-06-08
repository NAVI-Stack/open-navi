package skill

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/ceoai/navi/internal/sandbox"
	"github.com/ceoai/navi/internal/schema"
)

type executionContextKey struct{}

type ExecutionContext struct {
	ChatID                 string
	RunID                  string
	WorkspaceDir           string
	SandboxRunner          sandbox.Runner
	SandboxProfileResolver func(context.Context, string) (schema.SandboxProfile, error)
}

func WithExecutionContext(ctx context.Context, exec ExecutionContext) context.Context {
	return context.WithValue(ctx, executionContextKey{}, exec)
}

func ExecutionContextFromContext(ctx context.Context) (ExecutionContext, bool) {
	exec, ok := ctx.Value(executionContextKey{}).(ExecutionContext)
	return exec, ok
}

func absoluteWorkspaceDir(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}
