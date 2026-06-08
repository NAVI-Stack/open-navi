package trace

import (
	"context"
	"strings"
	"testing"

	"github.com/ceoai/navi/internal/navi/orchestration"
)

type recordingTraceSink struct {
	events []orchestration.TraceEvent
}

func (s *recordingTraceSink) Record(_ context.Context, event orchestration.TraceEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestSurfaceResolutionMetadataCapturesRequestedResolvedAndExclusions(t *testing.T) {
	meta := SurfaceResolutionMetadata(
		orchestration.SurfaceResolutionInput{
			ExecutionMode: orchestration.ExecutionModeChatTurn,
			Model:         orchestration.ModelProfile{SupportsTools: true},
			RequiredOutput: orchestration.RequiredOutput{
				AllowToolCalls: true,
			},
		},
		orchestration.SurfaceResolutionResult{
			Requested: orchestration.CapabilitySurface{
				Surface:   orchestration.CapabilitySurfaceLoop,
				ToolNames: []string{"beta", "beta", "alpha"},
			},
			Resolved: orchestration.CapabilitySurface{
				Surface:         orchestration.CapabilitySurfaceLoop,
				ToolNames:       []string{"alpha"},
				SelectionReason: "minimal safe loop surface",
			},
			ExpectedTools:            true,
			RequiresToolCapableModel: true,
			Exclusions: []orchestration.SurfaceExclusion{
				{ToolName: "beta", Reason: orchestration.SurfaceExclusionNotRequested},
				{ToolName: "hidden_tool", Reason: orchestration.SurfaceExclusionHidden},
			},
		},
	)

	if meta["requested_surface"] != orchestration.CapabilitySurfaceLoop {
		t.Fatalf("requested_surface = %q", meta["requested_surface"])
	}
	if meta["requested_tool_names"] != "beta,alpha" {
		t.Fatalf("requested_tool_names = %q", meta["requested_tool_names"])
	}
	if meta["resolved_tool_names"] != "alpha" {
		t.Fatalf("resolved_tool_names = %q", meta["resolved_tool_names"])
	}
	if meta["surface_selection_reason"] != "minimal safe loop surface" {
		t.Fatalf("surface_selection_reason = %q", meta["surface_selection_reason"])
	}
	if meta["surface_exclusion_reason_counts"] != "hidden=1,not_requested=1" {
		t.Fatalf("surface_exclusion_reason_counts = %q", meta["surface_exclusion_reason_counts"])
	}
	if !strings.Contains(meta["surface_exclusions_json"], `"tool_name":"hidden_tool"`) {
		t.Fatalf("surface_exclusions_json missing hidden_tool: %q", meta["surface_exclusions_json"])
	}
	if meta["surface_path_parity_class"] != "loop:1" {
		t.Fatalf("surface_path_parity_class = %q", meta["surface_path_parity_class"])
	}
}

func TestSurfaceGuardTraceEventIsOnlyEmittedForGuardedResult(t *testing.T) {
	frame := orchestration.ExecutionFrame{TraceID: "trace-1", RunID: "run-1", ChatID: "sess-1"}
	input := orchestration.SurfaceResolutionInput{
		Surface: orchestration.CapabilitySurface{Surface: orchestration.CapabilitySurfaceRuntime},
	}

	if event := SurfaceGuardTraceEvent(frame, input, orchestration.SurfaceResolutionResult{}); event != nil {
		t.Fatalf("expected nil guard event for unguarded result, got %+v", event)
	}

	event := SurfaceGuardTraceEvent(frame, input, orchestration.SurfaceResolutionResult{
		Requested: orchestration.CapabilitySurface{Surface: orchestration.CapabilitySurfaceRuntime},
		Resolved:  orchestration.CapabilitySurface{Surface: orchestration.CapabilitySurfaceRuntime},
		Guard:     orchestration.SurfaceGuardEmptySurface,
	})
	if event == nil {
		t.Fatal("expected guard event")
	}
	if event.Stage != StageCapabilitySurfaceGuarded {
		t.Fatalf("guard stage = %q, want %q", event.Stage, StageCapabilitySurfaceGuarded)
	}
	if event.Metadata["guard_only"] != "true" {
		t.Fatalf("guard_only metadata = %q", event.Metadata["guard_only"])
	}
}

func TestRecordSurfaceResolutionEmitsResolvedAndGuardedStages(t *testing.T) {
	sink := &recordingTraceSink{}
	frame := orchestration.ExecutionFrame{TraceID: "trace-2", RunID: "run-2", ChatID: "sess-2"}
	input := orchestration.SurfaceResolutionInput{
		Surface:       orchestration.CapabilitySurface{Surface: orchestration.CapabilitySurfaceRuntime},
		ExecutionMode: orchestration.ExecutionModeRunExecute,
		Model:         orchestration.ModelProfile{SupportsTools: false},
		RequiredOutput: orchestration.RequiredOutput{
			AllowToolCalls: true,
		},
	}
	result := orchestration.SurfaceResolutionResult{
		Requested: orchestration.CapabilitySurface{Surface: orchestration.CapabilitySurfaceRuntime},
		Resolved: orchestration.CapabilitySurface{
			Surface:   orchestration.CapabilitySurfaceRuntime,
			ToolNames: []string{"send_reply"},
		},
		ExpectedTools:            true,
		RequiresToolCapableModel: true,
		Guard:                    orchestration.SurfaceGuardToolCapableModelRequired,
		GuardReason:              "resolved surface requires a tool-capable model",
	}

	if err := RecordSurfaceResolution(context.Background(), sink, frame, input, result); err != nil {
		t.Fatalf("RecordSurfaceResolution: %v", err)
	}
	if len(sink.events) != 2 {
		t.Fatalf("expected 2 trace events, got %d", len(sink.events))
	}
	if sink.events[0].Stage != StageCapabilitySurfaceResolved {
		t.Fatalf("resolved stage = %q", sink.events[0].Stage)
	}
	if sink.events[1].Stage != StageCapabilitySurfaceGuarded {
		t.Fatalf("guard stage = %q", sink.events[1].Stage)
	}
	if sink.events[0].Metadata["surface_guard"] != string(orchestration.SurfaceGuardToolCapableModelRequired) {
		t.Fatalf("resolved metadata missing guard classification: %+v", sink.events[0].Metadata)
	}
	if sink.events[1].Metadata["surface_path_parity_class"] != "guard:tool_capable_model_required" {
		t.Fatalf("guard parity class = %q", sink.events[1].Metadata["surface_path_parity_class"])
	}
}
