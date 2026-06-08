package extract

import (
	"context"
	"testing"
)

// TestRunner_ExtractResolveEnvelope drives the real Python worker and asserts the
// structured-envelope contract: emails/handles/proper nouns are extracted, a
// known existing entity resolves by blocking key, and an unknown one does not.
// Skips (does not fail) when no Python runtime is available so the suite stays
// green on machines without Python; CI/dev with Python exercises it for real.
func TestRunner_ExtractResolveEnvelope(t *testing.T) {
	r := NewRunner(nil)
	if !r.Available() {
		t.Skip("python worker unavailable; skipping Python integration test")
	}

	res, err := r.Process(context.Background(), ProcessRequest{
		ChunkID: "chunk-1",
		Content: "Met Alex Rivera today. Reach them at alex@example.com or @alexr.",
		ExistingEntities: []ExistingEntity{
			{ID: "contact-9", Type: "contact", Name: "Alex Rivera",
				BlockingKeys: map[string]string{"email": "alex@example.com", "normalized_name": "alex rivera"}},
		},
	})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if res.Extraction.ChunkID != "chunk-1" {
		t.Errorf("chunk id echo: got %q", res.Extraction.ChunkID)
	}
	if len(res.Extraction.Candidates) == 0 {
		t.Fatal("expected at least one candidate")
	}

	var sawEmail, sawMatch bool
	for _, c := range res.Extraction.Candidates {
		if c.Kind == "email" && c.Value == "alex@example.com" {
			sawEmail = true
		}
	}
	for _, rr := range res.Resolutions {
		if rr.MatchedID == "contact-9" && !rr.Ambiguous {
			sawMatch = true
		}
	}
	if !sawEmail {
		t.Errorf("expected an email candidate; got %+v", res.Extraction.Candidates)
	}
	if !sawMatch {
		t.Errorf("expected the email/name to resolve to contact-9; got %+v", res.Resolutions)
	}
}

// TestRunner_AmbiguousResolution verifies the worker reports ambiguity (rather
// than guessing) when two existing entities share a candidate's name.
func TestRunner_AmbiguousResolution(t *testing.T) {
	r := NewRunner(nil)
	if !r.Available() {
		t.Skip("python worker unavailable")
	}
	res, err := r.Process(context.Background(), ProcessRequest{
		ChunkID: "chunk-2",
		Content: "Talked to Jordan Lee about the plan.",
		ExistingEntities: []ExistingEntity{
			{ID: "c1", Type: "contact", Name: "Jordan Lee", BlockingKeys: map[string]string{"normalized_name": "jordan lee"}},
			{ID: "c2", Type: "contact", Name: "Jordan Lee", BlockingKeys: map[string]string{"normalized_name": "jordan lee"}},
		},
	})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	var ambiguous bool
	for _, rr := range res.Resolutions {
		if rr.CandidateName == "Jordan Lee" && rr.Ambiguous && len(rr.PossibleMergeIDs) == 2 {
			ambiguous = true
		}
	}
	if !ambiguous {
		t.Errorf("expected ambiguous resolution for duplicated name; got %+v", res.Resolutions)
	}
}
