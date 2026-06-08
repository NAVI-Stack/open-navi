package trace

import (
	"context"
	"errors"
	"log/slog"

	"github.com/open-navi/navi/internal/navi/orchestration"
)

type LoggerSink struct {
	logger *slog.Logger
}

func NewLoggerSink(logger *slog.Logger) *LoggerSink {
	if logger == nil {
		logger = slog.Default()
	}
	return &LoggerSink{logger: logger}
}

func (s *LoggerSink) Record(_ context.Context, event orchestration.TraceEvent) error {
	if s == nil || s.logger == nil {
		return nil
	}
	attrs := []any{
		"stage", event.Stage,
		"trace_id", event.TraceID,
		"run_id", event.RunID,
		"chat_id", event.ChatID,
	}
	for key, value := range event.Metadata {
		attrs = append(attrs, key, value)
	}
	s.logger.Info("ncos trace", attrs...)
	return nil
}

type SinkFunc func(context.Context, orchestration.TraceEvent) error

func (f SinkFunc) Record(ctx context.Context, event orchestration.TraceEvent) error {
	if f == nil {
		return nil
	}
	return f(ctx, event)
}

type MultiSink struct {
	sinks []orchestration.TraceSink
}

func NewMultiSink(sinks ...orchestration.TraceSink) orchestration.TraceSink {
	filtered := make([]orchestration.TraceSink, 0, len(sinks))
	for _, sink := range sinks {
		if sink == nil {
			continue
		}
		filtered = append(filtered, sink)
	}
	switch len(filtered) {
	case 0:
		return nil
	case 1:
		return filtered[0]
	default:
		return &MultiSink{sinks: filtered}
	}
}

func (s *MultiSink) Record(ctx context.Context, event orchestration.TraceEvent) error {
	if s == nil {
		return nil
	}
	var errs []error
	for _, sink := range s.sinks {
		if sink == nil {
			continue
		}
		if err := sink.Record(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
