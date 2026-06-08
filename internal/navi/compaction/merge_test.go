package compaction

import "testing"

func TestMerge_TaskFrame_UsesStableTaskIDs(t *testing.T) {
	existing := []TaskFrame{{TaskID: "t1", Title: "A", Status: "active", NextStep: "n1"}}
	incoming := []TaskFrame{{TaskID: "t1", Title: "B", Status: "blocked", NextStep: "n2"}}
	merged := MergeTaskFrames(existing, incoming)
	if len(merged) != 1 || merged[0].Status != "blocked" || merged[0].NextStep != "n2" {
		t.Fatalf("unexpected merge: %+v", merged)
	}
}

func TestMerge_TaskFrame_DoesNotDropBlockersSilently(t *testing.T) {
	existing := []TaskFrame{{TaskID: "t1", Blockers: []string{"b1"}}}
	incoming := []TaskFrame{{TaskID: "t1", Blockers: []string{"b2"}}}
	merged := MergeTaskFrames(existing, incoming)
	if len(merged[0].Blockers) != 2 {
		t.Fatalf("expected blocker union got %+v", merged[0].Blockers)
	}
}

func TestMerge_TaskFrame_PreservesGroundedDependenciesAndLinkage(t *testing.T) {
	existing := []TaskFrame{{
		TaskID:       "t1",
		RunID:        "run-1",
		Dependencies: []string{"dep-a"},
		ProposalRefs: []string{"proposal-1"},
		FailureRefs:  []string{"failure-1"},
		NextStep:     "wait",
	}}
	incoming := []TaskFrame{{
		TaskID:       "t1",
		RunID:        "run-1",
		CheckpointID: "cp-2",
		Dependencies: []string{"dep-b"},
		ProposalRefs: []string{"proposal-1"},
		FailureRefs:  []string{"failure-2"},
		NextStep:     "resume",
	}}
	merged := MergeTaskFrames(existing, incoming)
	if got := merged[0]; len(got.Dependencies) != 2 || got.NextStep != "resume" || len(got.FailureRefs) != 2 {
		t.Fatalf("unexpected grounded merge: %+v", got)
	}
}

func TestMerge_RetrievalSpans_PreservesProvenance(t *testing.T) {
	merged := MergeRetrievalSpans(
		[]RetrievalSpan{{SpanID: "s1", Excerpt: "short", Provenance: SourceSpanProvenance{StartMessageID: "m1", EndMessageID: "m2"}}},
		[]RetrievalSpan{{SpanID: "s1", Excerpt: "longer excerpt", Provenance: SourceSpanProvenance{SourceMessageIDs: []string{"m1", "m2", "m3"}}}},
	)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged retrieval span, got %d", len(merged))
	}
	if merged[0].Excerpt != "longer excerpt" || len(merged[0].Provenance.SourceMessageIDs) != 3 {
		t.Fatalf("unexpected retrieval merge: %+v", merged[0])
	}
}
