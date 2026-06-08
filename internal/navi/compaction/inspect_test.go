package compaction

import "testing"

func TestInspectCheckpoint_ExplainsCheckpointDecision(t *testing.T) {
	view := InspectCheckpoint(CompactionCheckpoint{
		CheckpointID:            "cp-1",
		TriggerClass:            TriggerHard,
		TriggerReason:           "estimated prompt tokens 900 exceeded hard threshold 700",
		CompactedMessageStartID: "m1",
		CompactedMessageEndID:   "m4",
		SelectedSpan:            SourceSpanProvenance{StartMessageID: "m1", EndMessageID: "m4"},
		SourceMessageIDs:        []string{"m1", "m2", "m3", "m4"},
		SelectionTrace: CompactionSelection{
			SelectedCount: 4,
			StopReason:    &BoundaryReason{Kind: "proposal_bundle", RefID: "proposal-7", Detail: "proposal bundle remains open"},
		},
		TaskSurvival:     []TaskSurvivalRecord{{TaskID: "run-a", Reason: "merged_with_existing", SourceMessageStartID: "m2", SourceMessageEndID: "m3"}},
		RetrievalSpanIDs: []string{"cp-1:support:1"},
	})

	if view.CompactedRange != "m1 -> m4" {
		t.Fatalf("unexpected compacted range: %+v", view)
	}
	if view.StopReason == "" || view.TaskSurvival[0].SourceRange == "" {
		t.Fatalf("expected explainable checkpoint inspection view, got %+v", view)
	}
}

func TestInspectRunOutput_ExplainsTrimmedSectionsAndSupportSpans(t *testing.T) {
	view := InspectRunOutput(RunOutput{
		Triggered: TriggerHard,
		Inspection: CompactionInspection{
			TriggerReason: "estimated prompt tokens 900 exceeded hard threshold 700",
			Selection: CompactionSelection{
				SelectedCount: 3,
				StopReason:    &BoundaryReason{Kind: "recovery_bundle", RunID: "run-9", Detail: "failure/recovery bundle remains open"},
			},
			SelectedSpan: SourceSpanProvenance{StartMessageID: "m1", EndMessageID: "m3"},
			TaskSurvival: []TaskSurvivalRecord{{TaskID: "run-9", Reason: "new_or_rehydrated", SourceMessageStartID: "m2", SourceMessageEndID: "m3"}},
			SupportSpans: []RetrievalSpan{{SpanID: "s1", Kind: "tool_output", SupportClass: "execution_support", Priority: 70, RelatedTaskIDs: []string{"run-9"}, Provenance: SourceSpanProvenance{StartMessageID: "m1", EndMessageID: "m2"}}},
		},
		Rehydration: RehydrationOutput{
			Metadata: RehydrationMetadata{
				IncludedTaskIDs:        []string{"run-9"},
				IncludedSupportSpanIDs: []string{"s1"},
				DroppedSections:        []DroppedSection{{Label: "retrieval:s2", Kind: "retrieval", Reason: "trimmed_support_context_first"}},
			},
		},
	})

	if view.SelectedRange != "m1 -> m3" {
		t.Fatalf("unexpected selected range: %+v", view)
	}
	if len(view.SupportSpans) != 1 || len(view.Rehydration.DroppedReasons) != 1 {
		t.Fatalf("expected explainable run inspection view, got %+v", view)
	}
}
