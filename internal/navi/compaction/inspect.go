package compaction

import "fmt"

type TaskSurvivalSummary struct {
	TaskID       string   `json:"task_id"`
	RunID        string   `json:"run_id,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	SourceRange  string   `json:"source_range,omitempty"`
	ProposalRefs []string `json:"proposal_refs,omitempty"`
	FailureRefs  []string `json:"failure_refs,omitempty"`
}

type SupportSpanSummary struct {
	SpanID       string   `json:"span_id"`
	Kind         string   `json:"kind,omitempty"`
	SupportClass string   `json:"support_class,omitempty"`
	Priority     int      `json:"priority,omitempty"`
	RelatedTasks []string `json:"related_tasks,omitempty"`
	SourceRange  string   `json:"source_range,omitempty"`
}

type CheckpointInspectionView struct {
	CheckpointID     string                `json:"checkpoint_id,omitempty"`
	TriggerClass     TriggerClass          `json:"trigger_class,omitempty"`
	TriggerReason    string                `json:"trigger_reason,omitempty"`
	CompactedRange   string                `json:"compacted_range,omitempty"`
	SelectedCount    int                   `json:"selected_count,omitempty"`
	StopReason       string                `json:"stop_reason,omitempty"`
	SourceRange      string                `json:"source_range,omitempty"`
	SourceMessageIDs []string              `json:"source_message_ids,omitempty"`
	TaskSurvival     []TaskSurvivalSummary `json:"task_survival,omitempty"`
	RetrievalSpanIDs []string              `json:"retrieval_span_ids,omitempty"`
}

type RehydrationInspectionView struct {
	IncludedTaskIDs        []string         `json:"included_task_ids,omitempty"`
	IncludedSupportSpanIDs []string         `json:"included_support_span_ids,omitempty"`
	ProtectedLabels        []string         `json:"protected_labels,omitempty"`
	DroppedSections        []DroppedSection `json:"dropped_sections,omitempty"`
	DroppedReasons         []string         `json:"dropped_reasons,omitempty"`
}

type CompactionInspectionView struct {
	TriggerClass  TriggerClass              `json:"trigger_class,omitempty"`
	TriggerReason string                    `json:"trigger_reason,omitempty"`
	SelectedRange string                    `json:"selected_range,omitempty"`
	StopReason    string                    `json:"stop_reason,omitempty"`
	SelectedCount int                       `json:"selected_count,omitempty"`
	TaskSurvival  []TaskSurvivalSummary     `json:"task_survival,omitempty"`
	SupportSpans  []SupportSpanSummary      `json:"support_spans,omitempty"`
	Rehydration   RehydrationInspectionView `json:"rehydration,omitempty"`
}

func InspectCheckpoint(checkpoint CompactionCheckpoint) CheckpointInspectionView {
	view := CheckpointInspectionView{
		CheckpointID:     checkpoint.CheckpointID,
		TriggerClass:     checkpoint.TriggerClass,
		TriggerReason:    checkpoint.TriggerReason,
		CompactedRange:   compactedRange(checkpoint.CompactedMessageStartID, checkpoint.CompactedMessageEndID),
		SelectedCount:    checkpoint.SelectionTrace.SelectedCount,
		StopReason:       describeBoundaryReason(checkpoint.SelectionTrace.StopReason),
		SourceRange:      compactedRange(checkpoint.SelectedSpan.StartMessageID, checkpoint.SelectedSpan.EndMessageID),
		SourceMessageIDs: append([]string(nil), checkpoint.SourceMessageIDs...),
		RetrievalSpanIDs: append([]string(nil), checkpoint.RetrievalSpanIDs...),
	}
	for _, task := range checkpoint.TaskSurvival {
		view.TaskSurvival = append(view.TaskSurvival, summarizeTaskSurvival(task))
	}
	return view
}

func InspectRunOutput(output RunOutput) CompactionInspectionView {
	view := CompactionInspectionView{
		TriggerClass:  output.Triggered,
		TriggerReason: output.Inspection.TriggerReason,
		SelectedRange: compactedRange(output.Inspection.SelectedSpan.StartMessageID, output.Inspection.SelectedSpan.EndMessageID),
		SelectedCount: output.Inspection.Selection.SelectedCount,
		StopReason:    describeBoundaryReason(output.Inspection.Selection.StopReason),
		Rehydration:   InspectRehydration(output.Rehydration),
	}
	for _, task := range output.Inspection.TaskSurvival {
		view.TaskSurvival = append(view.TaskSurvival, summarizeTaskSurvival(task))
	}
	for _, span := range output.Inspection.SupportSpans {
		view.SupportSpans = append(view.SupportSpans, summarizeSupportSpan(span))
	}
	return view
}

func InspectRehydration(output RehydrationOutput) RehydrationInspectionView {
	view := RehydrationInspectionView{
		IncludedTaskIDs:        append([]string(nil), output.Metadata.IncludedTaskIDs...),
		IncludedSupportSpanIDs: append([]string(nil), output.Metadata.IncludedSupportSpanIDs...),
		ProtectedLabels:        append([]string(nil), output.Metadata.ProtectedLabels...),
		DroppedSections:        append([]DroppedSection(nil), output.Metadata.DroppedSections...),
	}
	for _, dropped := range output.Metadata.DroppedSections {
		view.DroppedReasons = append(view.DroppedReasons, fmt.Sprintf("%s (%s)", dropped.Label, dropped.Reason))
	}
	return view
}

func describeBoundaryReason(reason *BoundaryReason) string {
	if reason == nil {
		return ""
	}
	detail := reason.Detail
	if detail == "" {
		detail = reason.Kind
	}
	ref := firstNonEmpty(reason.RefID, reason.RunID, reason.MessageID)
	if ref == "" {
		return detail
	}
	return fmt.Sprintf("%s [%s]", detail, ref)
}

func summarizeTaskSurvival(task TaskSurvivalRecord) TaskSurvivalSummary {
	return TaskSurvivalSummary{
		TaskID:       task.TaskID,
		RunID:        task.RunID,
		Reason:       task.Reason,
		SourceRange:  compactedRange(task.SourceMessageStartID, task.SourceMessageEndID),
		ProposalRefs: append([]string(nil), task.ProposalRefs...),
		FailureRefs:  append([]string(nil), task.FailureRefs...),
	}
}

func summarizeSupportSpan(span RetrievalSpan) SupportSpanSummary {
	return SupportSpanSummary{
		SpanID:       span.SpanID,
		Kind:         span.Kind,
		SupportClass: span.SupportClass,
		Priority:     span.Priority,
		RelatedTasks: append([]string(nil), span.RelatedTaskIDs...),
		SourceRange:  compactedRange(span.Provenance.StartMessageID, span.Provenance.EndMessageID),
	}
}

func compactedRange(startID, endID string) string {
	startID = firstNonEmpty(startID)
	endID = firstNonEmpty(endID)
	switch {
	case startID == "" && endID == "":
		return ""
	case endID == "" || startID == endID:
		return startID
	default:
		return startID + " -> " + endID
	}
}
