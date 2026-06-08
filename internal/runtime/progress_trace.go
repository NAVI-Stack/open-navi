package runtime

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

type progressTraceKey struct{}

// ProgressTrace carries request/run correlation fields across gateway/runtime boundaries.
type ProgressTrace struct {
	TraceID          string
	RequestID        string
	RuntimeSessionID string
	RunID            string
	Route            string
	Method           string
}

func WithProgressTrace(ctx context.Context, trace ProgressTrace) context.Context {
	return context.WithValue(ctx, progressTraceKey{}, trace)
}

func ProgressTraceFromContext(ctx context.Context) ProgressTrace {
	if ctx == nil {
		return ProgressTrace{}
	}
	if trace, ok := ctx.Value(progressTraceKey{}).(ProgressTrace); ok {
		return trace
	}
	return ProgressTrace{}
}

// ProgressTracer emits low-noise structured lifecycle checkpoints.
type ProgressTracer struct {
	component string
	trace     ProgressTrace
}

func NewProgressTracer(component string, trace ProgressTrace) *ProgressTracer {
	return &ProgressTracer{
		component: strings.TrimSpace(component),
		trace:     trace,
	}
}

func (t *ProgressTracer) StageStart(ctx context.Context, stage string, attrs ...any) func(error) {
	started := time.Now().UTC()
	t.log("start", stage, 0, "", attrs...)
	return func(err error) {
		ctxErr := ""
		if ctx != nil && ctx.Err() != nil {
			ctxErr = ctx.Err().Error()
		}
		if err != nil {
			stageErr := strings.TrimSpace(err.Error())
			if stageErr == "" {
				stageErr = ctxErr
			}
			t.log("end", stage, time.Since(started).Milliseconds(), stageErr, attrs...)
			return
		}
		t.log("end", stage, time.Since(started).Milliseconds(), ctxErr, attrs...)
	}
}

func (t *ProgressTracer) Mark(stage string, attrs ...any) {
	t.log("mark", stage, 0, "", attrs...)
}

func (t *ProgressTracer) log(event, stage string, elapsedMs int64, stageErr string, attrs ...any) {
	base := []any{
		"component", t.component,
		"event", strings.TrimSpace(event),
		"stage", strings.TrimSpace(stage),
		"trace_id", strings.TrimSpace(t.trace.TraceID),
		"request_id", strings.TrimSpace(t.trace.RequestID),
		"runtime_session_id", strings.TrimSpace(t.trace.RuntimeSessionID),
		"run_id", strings.TrimSpace(t.trace.RunID),
	}
	if strings.TrimSpace(t.trace.Route) != "" {
		base = append(base, "route", strings.TrimSpace(t.trace.Route))
	}
	if strings.TrimSpace(t.trace.Method) != "" {
		base = append(base, "method", strings.TrimSpace(t.trace.Method))
	}
	if elapsedMs > 0 {
		base = append(base, "elapsed_ms", elapsedMs)
	}
	if strings.TrimSpace(stageErr) != "" {
		base = append(base, "error", strings.TrimSpace(stageErr))
	}
	base = append(base, attrs...)
	slog.Info("progress_trace", base...)
}
