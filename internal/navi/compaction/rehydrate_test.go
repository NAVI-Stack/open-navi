package compaction

import (
	"strings"
	"testing"
)

func TestRehydration_Order_UsesCanonicalSurfaces(t *testing.T) {
	out := (Rehydrator{}).Assemble(RehydrationInput{
		PermanentInstructions: []string{"perm"},
		ChatFrame: ChatFrame{
			PrimaryObjective:  "obj",
			ActiveTopics:      []string{"topic-a"},
			OpenQuestions:     []string{"question-a"},
			ActiveCommitments: []string{"commitment-a"},
		},
		TaskFrames:            []TaskFrame{{TaskID: "t1", Title: "Do it", Status: "active", NextStep: "step", Blockers: []string{"waiting"}}},
		ProposalRefs:          []string{"p1"},
		FailureRefs:           []string{"f1"},
		RetrievalSpans:        []RetrievalSpan{{SpanID: "s1", Kind: "decision_support", SupportClass: "decision_support", Excerpt: "supporting excerpt", Provenance: SourceSpanProvenance{StartMessageID: "m1", EndMessageID: "m2"}}},
		LiveTail:              []Message{{ID: "m1", Role: "user", Content: "recent tail"}},
		NewInput:              "new",
		ProtectedRecentWindow: 1,
		MaxSupportSpans:       2,
	})
	if len(out.Sections) < 8 {
		t.Fatalf("sections length=%d", len(out.Sections))
	}
	if out.Sections[0] != "perm" {
		t.Fatalf("expected permanent instructions first")
	}
	joined := strings.Join(out.Sections, "\n\n")
	expectedSnippets := []string{
		"## Session Continuity",
		"Objective: obj",
		"## Task Do it [active]",
		"Next step: step",
		"## Active Proposals",
		"## Active Failures",
		"## Supporting Context",
		"## Recent Context",
		"## New User Input",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(joined, snippet) {
			t.Fatalf("expected %q in rehydration output, got %q", snippet, joined)
		}
	}
	if len(out.Metadata.IncludedTaskIDs) != 1 || out.Metadata.IncludedTaskIDs[0] != "t1" {
		t.Fatalf("expected task metadata, got %+v", out.Metadata)
	}
	if len(out.Metadata.IncludedSupportSpanIDs) != 1 || out.Metadata.IncludedSupportSpanIDs[0] != "s1" {
		t.Fatalf("expected support span metadata, got %+v", out.Metadata)
	}
}

func TestEviction_Order_DropsRetrievalBeforeProtectedRecentContext(t *testing.T) {
	out := (Rehydrator{}).Assemble(RehydrationInput{
		ChatFrame: ChatFrame{
			PrimaryObjective: "keep continuity",
		},
		RetrievalSpans:        []RetrievalSpan{{SpanID: "s1", Excerpt: strings.Repeat("support ", 20)}},
		LiveTail:              []Message{{ID: "m1", Role: "user", Content: "protected recent 1"}, {ID: "m2", Role: "navi", Content: "protected recent 2"}},
		Budget:                BudgetSnapshot{UsableTokens: 35},
		ProtectedRecentWindow: 2,
		MaxSupportSpans:       1,
	})
	if len(out.Dropped) == 0 || out.Dropped[0] != "retrieval:s1" {
		t.Fatalf("expected retrieval drop first, got %+v", out.Dropped)
	}
	joined := strings.Join(out.Sections, "\n\n")
	if !strings.Contains(joined, "protected recent 1") || !strings.Contains(joined, "protected recent 2") {
		t.Fatalf("expected protected recent continuity to survive trimming, got %q", joined)
	}
	if strings.Contains(joined, "## Supporting Context") {
		t.Fatalf("expected retrieval/support context to trim before protected recent continuity, got %q", joined)
	}
	if len(out.Metadata.DroppedSections) == 0 || out.Metadata.DroppedSections[0].Reason != "trimmed_support_context_first" {
		t.Fatalf("expected structured drop reason metadata, got %+v", out.Metadata.DroppedSections)
	}
}

func TestRehydration_SelectsHighestPrioritySupportSpans(t *testing.T) {
	out := (Rehydrator{}).Assemble(RehydrationInput{
		ChatFrame: ChatFrame{PrimaryObjective: "keep task continuity"},
		RetrievalSpans: []RetrievalSpan{
			{SpanID: "low", Priority: 10, Excerpt: "low"},
			{SpanID: "high", Priority: 90, Excerpt: "high"},
			{SpanID: "mid", Priority: 50, Excerpt: "mid"},
		},
		MaxSupportSpans: 2,
	})
	joined := strings.Join(out.Sections, "\n\n")
	if !strings.Contains(joined, "high") || !strings.Contains(joined, "mid") || strings.Contains(joined, "low") {
		t.Fatalf("expected highest-priority support spans only, got %q", joined)
	}
}
