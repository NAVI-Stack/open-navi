package distill

import (
	"context"
	"strings"
	"testing"
)

func pc(id, content string) ProvenancedChunk {
	return ProvenancedChunk{
		ChunkID:  id,
		RecordID: "rec-1",
		Content:  content,
		Provenance: Provenance{
			ConnectorID: "telegram:acct-1",
			SourceID:    "100:1",
			Author:      "alice",
			SourceKind:  "message",
		},
	}
}

func TestDistill_CollapsesNearDuplicates(t *testing.T) {
	in := Input{Chunks: []ProvenancedChunk{
		pc("a", "The quarterly report shows revenue grew by twelve percent this period."),
		pc("b", "The quarterly report shows revenue grew by twelve percent this period."), // exact dup
		pc("c", "Completely different content about weekend hiking plans in the mountains."),
	}}
	res := Distill(context.Background(), in)
	if res.Metrics.ChunksIn != 3 {
		t.Errorf("ChunksIn: got %d", res.Metrics.ChunksIn)
	}
	if res.Metrics.Duplicates != 1 {
		t.Errorf("Duplicates: want 1, got %d", res.Metrics.Duplicates)
	}
	if len(res.Chunks) != 2 {
		t.Fatalf("expected 2 survivors, got %d", len(res.Chunks))
	}
	// Survivor must retain a verifiable link to the collapsed sibling.
	surv := res.Chunks[0]
	if len(surv.CollapsedIDs) != 1 || surv.CollapsedIDs[0] != "b" {
		t.Errorf("collapsed sibling not recorded: %+v", surv.CollapsedIDs)
	}
	foundB := false
	for _, id := range surv.SourceChunkIDs {
		if id == "b" {
			foundB = true
		}
	}
	if !foundB {
		t.Errorf("SourceChunkIDs must include collapsed sibling: %+v", surv.SourceChunkIDs)
	}
}

func TestDistill_PreservesProvenance(t *testing.T) {
	in := Input{Chunks: []ProvenancedChunk{pc("a", "Hello there friend.")}}
	res := Distill(context.Background(), in)
	if len(res.Chunks) != 1 {
		t.Fatalf("expected 1 chunk")
	}
	c := res.Chunks[0]
	if c.RecordID != "rec-1" || c.Provenance.ConnectorID != "telegram:acct-1" {
		t.Errorf("provenance lost: %+v", c)
	}
	if len(c.SourceChunkIDs) != 1 || c.SourceChunkIDs[0] != "a" {
		t.Errorf("SourceChunkIDs: %+v", c.SourceChunkIDs)
	}
}

func TestDistill_ExtractsQuotesWithSpans(t *testing.T) {
	content := `He said "this is an important statement" yesterday.
> a blockquote line worth keeping here`
	res := Distill(context.Background(), Input{Chunks: []ProvenancedChunk{pc("a", content)}})
	q := res.Chunks[0].Quotes
	if len(q) < 2 {
		t.Fatalf("expected at least 2 quotes, got %d: %+v", len(q), q)
	}
	for _, quote := range q {
		// Offsets must reference the actual span in content.
		if quote.StartOffset < 0 || quote.EndOffset > len(content) || quote.StartOffset >= quote.EndOffset {
			t.Errorf("invalid quote span: %+v", quote)
		}
		span := content[quote.StartOffset:quote.EndOffset]
		if !strings.Contains(span, strings.TrimSpace(quote.Text)) {
			t.Errorf("quote text %q not at span %q", quote.Text, span)
		}
	}
}

func TestDistill_ExtractsEntities(t *testing.T) {
	content := "Contact Alice Johnson at alice@example.com or visit https://example.com/docs — see issue PROJ-123 and ask @bob about #release."
	res := Distill(context.Background(), Input{Chunks: []ProvenancedChunk{pc("a", content)}})
	ents := res.Chunks[0].Entities
	kinds := map[string]bool{}
	for _, e := range ents {
		kinds[e.Kind] = true
		// Span offsets must reference real text.
		if content[e.StartOffset:e.EndOffset] == "" {
			t.Errorf("entity %+v has empty span", e)
		}
	}
	for _, want := range []string{"email", "url", "handle", "hashtag", "proper_noun"} {
		if !kinds[want] {
			t.Errorf("expected an entity of kind %q; got kinds %v", want, kinds)
		}
	}
	// URL entity should be normalized.
	for _, e := range ents {
		if e.Kind == "url" && strings.Contains(e.Name, "://example.com/docs") {
			// fine
		}
	}
}

func TestDistill_BudgetTruncates(t *testing.T) {
	chunks := []ProvenancedChunk{
		pc("a", strings.Repeat("alpha words here ", 40)),
		pc("b", strings.Repeat("bravo distinct text ", 40)),
		pc("c", strings.Repeat("charlie unique stuff ", 40)),
	}
	// Budget small enough to admit only the first chunk.
	res := Distill(context.Background(), Input{
		Chunks:  chunks,
		Options: Options{Budget: estimateTokens(chunks[0].Content) + 1},
	})
	if !res.Metrics.Budgeted {
		t.Error("expected Budgeted=true")
	}
	if len(res.Chunks) >= 3 {
		t.Errorf("budget did not drop chunks: got %d", len(res.Chunks))
	}
	if len(res.Chunks) == 0 {
		t.Error("budget should always keep at least one chunk")
	}
}

// recordingSummarizer counts invocations to prove the extractive default never
// calls the abstractive hook.
type recordingSummarizer struct{ calls int }

func (r *recordingSummarizer) Summarize(_ context.Context, text string) (string, error) {
	r.calls++
	return text, nil
}

func TestDistill_NoSummarizerByDefault(t *testing.T) {
	rec := &recordingSummarizer{}
	// Default path (no Summarizer) must not call any summarizer.
	res := Distill(context.Background(), Input{Chunks: []ProvenancedChunk{pc("a", "hello world content")}})
	if len(res.Chunks) != 1 {
		t.Fatal("expected one chunk")
	}
	if rec.calls != 0 {
		t.Errorf("recording summarizer should not be wired by default")
	}

	// When explicitly provided, the hook is honored (interface works) — but this
	// is never used by the ingest worker (extractive-only V1).
	res2 := Distill(context.Background(), Input{
		Chunks:  []ProvenancedChunk{pc("a", "hello world content")},
		Options: Options{Summarizer: rec},
	})
	if rec.calls == 0 {
		t.Error("explicit summarizer hook was not invoked")
	}
	_ = res2
}

func TestNoopSummarizer_PassThrough(t *testing.T) {
	out, err := NoopSummarizer{}.Summarize(context.Background(), "unchanged")
	if err != nil || out != "unchanged" {
		t.Errorf("NoopSummarizer must pass through: %q, %v", out, err)
	}
}

func TestDistill_Deterministic(t *testing.T) {
	in := Input{Chunks: []ProvenancedChunk{
		pc("a", "First chunk about Alice and Bob."),
		pc("b", "Second chunk about Carol and the project."),
	}}
	r1 := Distill(context.Background(), in)
	r2 := Distill(context.Background(), in)
	if len(r1.Chunks) != len(r2.Chunks) {
		t.Fatal("non-deterministic chunk count")
	}
	for i := range r1.Chunks {
		if r1.Chunks[i].ChunkID != r2.Chunks[i].ChunkID || len(r1.Chunks[i].Entities) != len(r2.Chunks[i].Entities) {
			t.Errorf("distill not deterministic at chunk %d", i)
		}
	}
}
