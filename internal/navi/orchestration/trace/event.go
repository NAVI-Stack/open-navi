package trace

import "github.com/open-navi/navi/internal/navi/orchestration"

const (
	StageRunStarted                = "run_started"
	StageContextAssembled          = "context_assembled"
	StageInstructionsCompiled      = "instructions_compiled"
	StageCapabilitySurfaceResolved = "capability_surface_resolved"
	StageCapabilitySurfaceGuarded  = "capability_surface_guarded"
	StageProviderRequestCompiled   = "provider_request_compiled"
	StageModelCalled               = "model_called"
	StageModelResponseNormalized   = "model_response_normalized"
	StageRunCompleted              = "run_completed"
	StageRunFailed                 = "run_failed"
)

// InteractionMetadata converts a trace event into a flat metadata payload suitable
// for existing interaction-event persistence.
func InteractionMetadata(event orchestration.TraceEvent) map[string]any {
	meta := map[string]any{
		"trace_stage": event.Stage,
	}
	if event.TraceID != "" {
		meta["trace_id"] = event.TraceID
	}
	if event.RunID != "" {
		meta["run_id"] = event.RunID
	}
	if event.ChatID != "" {
		meta["chat_id"] = event.ChatID
	}
	for key, value := range event.Metadata {
		if key == "" || value == "" {
			continue
		}
		meta[key] = value
	}
	return meta
}
