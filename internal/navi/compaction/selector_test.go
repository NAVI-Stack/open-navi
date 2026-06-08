package compaction

import "testing"

func TestSelector_DoesNotSplitToolCallAndResult(t *testing.T) {
	selector := SegmentSelector{DefaultProtectedRecentWindow: 2, MinimumProtectedRecentWindow: 1}
	msgs := []Message{{ID: "1", MessageKind: "reply"}, {ID: "1b", MessageKind: "reply"}, {ID: "2", MessageKind: "tool_call"}, {ID: "3", MessageKind: "tool_result"}, {ID: "4", MessageKind: "reply"}}
	result, ok := selector.Select(SelectorInput{Messages: msgs, Trigger: TriggerHard})
	if !ok {
		t.Fatal("expected selection")
	}
	if result.EndIndex >= 2 {
		t.Fatalf("selector split tool pair: end=%d", result.EndIndex)
	}
}

func TestSelector_DoesNotSplitProposalAndResolutionExchange(t *testing.T) {
	selector := SegmentSelector{DefaultProtectedRecentWindow: 2, MinimumProtectedRecentWindow: 1}
	msgs := []Message{{ID: "1", MessageKind: "reply"}, {ID: "1b", MessageKind: "reply"}, {ID: "2", MessageKind: "proposal"}, {ID: "3", MessageKind: "proposal_resolution"}, {ID: "4", MessageKind: "reply"}}
	res, ok := selector.Select(SelectorInput{Messages: msgs, Trigger: TriggerHard})
	if !ok || res.EndIndex >= 2 {
		t.Fatalf("selector split proposal boundary: ok=%v end=%d", ok, res.EndIndex)
	}
}

func TestSelector_DoesNotSplitFailureAndRecoveryExchange(t *testing.T) {
	selector := SegmentSelector{DefaultProtectedRecentWindow: 2, MinimumProtectedRecentWindow: 1}
	msgs := []Message{{ID: "1", MessageKind: "reply"}, {ID: "1b", MessageKind: "reply"}, {ID: "2", MessageKind: "failure_open"}, {ID: "3", MessageKind: "recovery_active"}, {ID: "4", MessageKind: "reply"}}
	res, ok := selector.Select(SelectorInput{Messages: msgs, Trigger: TriggerHard})
	if !ok || res.EndIndex >= 2 {
		t.Fatalf("selector split failure boundary: ok=%v end=%d", ok, res.EndIndex)
	}
}

func TestSelector_RespectsProtectedRecentWindow(t *testing.T) {
	selector := SegmentSelector{DefaultProtectedRecentWindow: 3, MinimumProtectedRecentWindow: 2}
	msgs := []Message{{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}, {ID: "5"}}
	res, ok := selector.Select(SelectorInput{Messages: msgs, Trigger: TriggerHard})
	if !ok {
		t.Fatal("expected selection")
	}
	if res.EndIndex > 1 {
		t.Fatalf("protected recent window violated: end=%d", res.EndIndex)
	}
}

func TestSelector_DoesNotSplitMessyMultiTurnBoundaries(t *testing.T) {
	selector := SegmentSelector{DefaultProtectedRecentWindow: 3, MinimumProtectedRecentWindow: 2}
	msgs := []Message{
		{ID: "m1", MessageKind: "reply"},
		{ID: "m2", MessageKind: "proposal_open"},
		{ID: "m3", MessageKind: "reply"},
		{ID: "m4", MessageKind: "recovery_active"},
		{ID: "m5", MessageKind: "reply"},
		{ID: "m6", MessageKind: "proposal_resolution"},
		{ID: "m7", MessageKind: "recovery_resolved"},
		{ID: "m8", MessageKind: "tool_call"},
		{ID: "m9", MessageKind: "reply"},
		{ID: "m10", MessageKind: "tool_result"},
		{ID: "m11", MessageKind: "reply"},
	}
	res, ok := selector.Select(SelectorInput{Messages: msgs, Trigger: TriggerHard})
	if ok {
		t.Fatalf("expected no safe selection when proposal/recovery/tool bundles are still open; got end=%d", res.EndIndex)
	}
}

func TestSelector_ProtectsLatestArtifactMutationSequence(t *testing.T) {
	selector := SegmentSelector{DefaultProtectedRecentWindow: 1, MinimumProtectedRecentWindow: 1}
	msgs := []Message{
		{ID: "a1", MessageKind: "reply"},
		{ID: "a2", MessageKind: "reply"},
		{ID: "a3", RunID: "artifact-run", MessageKind: "artifact_mutation", Content: "updated file report.md"},
		{ID: "a4", RunID: "artifact-run", MessageKind: "reply", Content: "verified report.md"},
	}
	res, ok := selector.Select(SelectorInput{Messages: msgs, Trigger: TriggerHard})
	if !ok {
		t.Fatal("expected selection")
	}
	if res.EndIndex >= 2 {
		t.Fatalf("expected latest artifact mutation sequence protected, got end=%d", res.EndIndex)
	}
}

func TestSelector_UsesContextSignalsForProposalBundleWithoutExplicitKinds(t *testing.T) {
	selector := SegmentSelector{DefaultProtectedRecentWindow: 2, MinimumProtectedRecentWindow: 1}
	msgs := []Message{
		{ID: "m1", MessageKind: "reply", Content: "older context"},
		{ID: "m1b", MessageKind: "reply", Content: "second older context"},
		{ID: "m2", RunID: "proposal-7", Content: "[proposal:proposal-7] Approval required to modify release.yml"},
		{ID: "m3", RunID: "proposal-7", Content: "I approve proposal-7"},
		{ID: "m4", MessageKind: "reply", Content: "tail"},
	}
	res, ok := selector.Select(SelectorInput{Messages: msgs, Trigger: TriggerHard})
	if !ok {
		t.Fatal("expected selection")
	}
	if res.EndIndex >= 2 {
		t.Fatalf("selector should protect proposal bundle even without normalized kinds, got end=%d", res.EndIndex)
	}
}

func TestSelector_UsesRunScopedSignalsForToolAndRecoveryBundles(t *testing.T) {
	selector := SegmentSelector{DefaultProtectedRecentWindow: 2, MinimumProtectedRecentWindow: 1}
	msgs := []Message{
		{ID: "m1", Content: "older"},
		{ID: "m1b", Content: "older again"},
		{ID: "m2", RunID: "run-42", Content: "Calling tool read_file for workspace scan"},
		{ID: "m3", RunID: "run-42", Content: "Tool result: loaded config/runtime.yaml"},
		{ID: "m4", RunID: "run-43", Content: "[failure:run-43] command failed"},
		{ID: "m5", RunID: "run-43", Content: "Recovery: retrying run-43"},
		{ID: "m6", RunID: "run-43", Content: "Recovery resolved for run-43"},
		{ID: "m7", Content: "tail"},
	}
	res, ok := selector.Select(SelectorInput{Messages: msgs, Trigger: TriggerHard})
	if !ok {
		t.Fatal("expected selection")
	}
	if res.EndIndex >= 4 {
		t.Fatalf("selector should stop before unresolved failure/recovery bundle, got end=%d", res.EndIndex)
	}
}

func TestSelector_UsesAuthoritativeProposalAndArtifactRefs(t *testing.T) {
	selector := SegmentSelector{DefaultProtectedRecentWindow: 2, MinimumProtectedRecentWindow: 1}
	msgs := []Message{
		{ID: "m1", Content: "older"},
		{ID: "m2", Content: "older 2"},
		{ID: "m3", RunID: "run-1", Content: "some execution context"},
		{ID: "m4", RunID: "run-1", Content: "more execution context"},
		{ID: "m5", Content: "tail"},
	}
	res, ok := selector.Select(SelectorInput{
		Messages: msgs,
		Trigger:  TriggerHard,
		RuntimeSnapshots: []RunSnapshot{{
			RunID:                 "run-1",
			PendingProposalID:     "proposal-7",
			PendingProposalReason: "needs approval",
			MainArtifactID:        "artifact:report",
		}},
	})
	if !ok {
		t.Fatal("expected selection")
	}
	if res.EndIndex >= 2 {
		t.Fatalf("selector should stop before authoritative proposal/artifact bundle, got end=%d", res.EndIndex)
	}
	if res.Trace.StopReason == nil || res.Trace.StopReason.Kind == "" {
		t.Fatalf("expected stop reason metadata, got %+v", res.Trace)
	}
}

func TestSelector_RemainsDeterministicWithAuthoritativeSignals(t *testing.T) {
	selector := SegmentSelector{DefaultProtectedRecentWindow: 2, MinimumProtectedRecentWindow: 1}
	input := SelectorInput{
		Messages: []Message{
			{ID: "m1", Content: "older"},
			{ID: "m2", Content: "older-2"},
			{ID: "m3", RunID: "run-1", Content: "proposal open"},
			{ID: "m4", RunID: "run-1", Content: "artifact work"},
			{ID: "m5", Content: "tail"},
		},
		Trigger: TriggerHard,
		RuntimeSnapshots: []RunSnapshot{{
			RunID:             "run-1",
			PendingProposalID: "proposal-1",
			MainArtifactID:    "artifact:a",
		}},
	}
	first, ok := selector.Select(input)
	if !ok {
		t.Fatal("expected first selection")
	}
	second, ok := selector.Select(input)
	if !ok {
		t.Fatal("expected second selection")
	}
	if first.EndIndex != second.EndIndex || first.Trace.CandidateEndID != second.Trace.CandidateEndID {
		t.Fatalf("selector not deterministic: first=%+v second=%+v", first, second)
	}
}
