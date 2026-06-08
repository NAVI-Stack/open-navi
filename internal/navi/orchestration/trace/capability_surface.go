package trace

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/navi/orchestration"
)

// SurfaceResolutionTraceEvent builds the authoritative trace event for a
// completed capability-surface resolution. Metadata is derived directly from
// the resolver output and guard policy state instead of being reconstructed by
// runtime branches.
func SurfaceResolutionTraceEvent(frame orchestration.ExecutionFrame, input orchestration.SurfaceResolutionInput, result orchestration.SurfaceResolutionResult) orchestration.TraceEvent {
	return orchestration.TraceEvent{
		Stage:      StageCapabilitySurfaceResolved,
		TraceID:    frame.TraceID,
		RunID:      frame.RunID,
		ChatID:     frame.ChatID,
		OccurredAt: time.Now().UTC(),
		Metadata:   SurfaceResolutionMetadata(input, result),
	}
}

// SurfaceGuardTraceEvent returns a guard-specific trace event when the
// capability surface has failed closed.
func SurfaceGuardTraceEvent(frame orchestration.ExecutionFrame, input orchestration.SurfaceResolutionInput, result orchestration.SurfaceResolutionResult) *orchestration.TraceEvent {
	if !result.IsGuarded() {
		return nil
	}

	meta := SurfaceResolutionMetadata(input, result)
	meta["guard_only"] = "true"

	return &orchestration.TraceEvent{
		Stage:      StageCapabilitySurfaceGuarded,
		TraceID:    frame.TraceID,
		RunID:      frame.RunID,
		ChatID:     frame.ChatID,
		OccurredAt: time.Now().UTC(),
		Metadata:   meta,
	}
}

// RecordSurfaceResolution writes the capability-surface resolution trace events
// to the provided sink. When the result is guarded, both the resolved and
// guarded stages are emitted.
func RecordSurfaceResolution(ctx context.Context, sink orchestration.TraceSink, frame orchestration.ExecutionFrame, input orchestration.SurfaceResolutionInput, result orchestration.SurfaceResolutionResult) error {
	if sink == nil {
		return nil
	}
	if err := sink.Record(ctx, SurfaceResolutionTraceEvent(frame, input, result)); err != nil {
		return err
	}
	if guardEvent := SurfaceGuardTraceEvent(frame, input, result); guardEvent != nil {
		return sink.Record(ctx, *guardEvent)
	}
	return nil
}

// SurfaceResolutionMetadata flattens capability-surface diagnostics into a
// deterministic trace payload suitable for logs and persisted interaction
// events.
func SurfaceResolutionMetadata(input orchestration.SurfaceResolutionInput, result orchestration.SurfaceResolutionResult) map[string]string {
	meta := map[string]string{
		"requested_surface":               strings.TrimSpace(result.Requested.Surface),
		"requested_tool_names":            joinSortedPreserveUnique(result.Requested.ToolNames),
		"resolved_surface":                strings.TrimSpace(result.Resolved.Surface),
		"resolved_tool_names":             strings.Join(result.Resolved.ToolNames, ","),
		"resolved_tool_count":             strconv.Itoa(len(result.Resolved.ToolNames)),
		"surface_selection_reason":        strings.TrimSpace(result.Resolved.SelectionReason),
		"surface_expected_tools":          strconv.FormatBool(result.ExpectedTools),
		"surface_requires_tool_capable":   strconv.FormatBool(result.RequiresToolCapableModel),
		"surface_guard":                   string(result.Guard),
		"surface_guard_reason":            strings.TrimSpace(result.GuardReason),
		"surface_exclusion_count":         strconv.Itoa(len(result.Exclusions)),
		"surface_execution_mode":          string(input.ExecutionMode),
		"surface_model_supports_tools":    strconv.FormatBool(input.Model.SupportsTools),
		"surface_required_output_tools":   strconv.FormatBool(input.RequiredOutput.AllowToolCalls),
		"surface_exclusions_json":         marshalSurfaceExclusions(result.Exclusions),
		"surface_exclusion_reason_counts": marshalExclusionReasonCounts(result.Exclusions),
		"surface_path_parity_class":       parityClass(input, result),
	}
	return trimEmptyMetadata(meta)
}

func marshalSurfaceExclusions(exclusions []orchestration.SurfaceExclusion) string {
	if len(exclusions) == 0 {
		return ""
	}
	normalized := make([]orchestration.SurfaceExclusion, 0, len(exclusions))
	for _, exclusion := range exclusions {
		normalized = append(normalized, orchestration.SurfaceExclusion{
			ToolName: strings.TrimSpace(exclusion.ToolName),
			Reason:   exclusion.Reason,
			Detail:   strings.TrimSpace(exclusion.Detail),
		})
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(data)
}

func marshalExclusionReasonCounts(exclusions []orchestration.SurfaceExclusion) string {
	if len(exclusions) == 0 {
		return ""
	}
	counts := make(map[string]int)
	for _, exclusion := range exclusions {
		counts[string(exclusion.Reason)]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+strconv.Itoa(counts[key]))
	}
	return strings.Join(parts, ",")
}

func parityClass(input orchestration.SurfaceResolutionInput, result orchestration.SurfaceResolutionResult) string {
	if result.IsGuarded() {
		return "guard:" + string(result.Guard)
	}
	return strings.TrimSpace(result.Resolved.Surface) + ":" + strconv.Itoa(len(result.Resolved.ToolNames))
}

func joinSortedPreserveUnique(values []string) string {
	if len(values) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(values))
	ordered := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		ordered = append(ordered, name)
	}
	return strings.Join(ordered, ",")
}

func trimEmptyMetadata(meta map[string]string) map[string]string {
	out := make(map[string]string, len(meta))
	for key, value := range meta {
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}
