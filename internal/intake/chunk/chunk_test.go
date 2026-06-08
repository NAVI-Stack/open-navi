package chunk

import (
	"strings"
	"testing"
)

func TestSplit_Empty(t *testing.T) {
	if got := Split("", "ns", Options{}); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
	if got := Split("   \n\n ", "ns", Options{}); got != nil {
		t.Errorf("expected nil for whitespace-only input, got %v", got)
	}
}

func TestSplit_StableIDsIdempotent(t *testing.T) {
	text := strings.Repeat("Paragraph one has some words.\n\nParagraph two follows here.\n\n", 5)
	a := Split(text, "telegram:acct-1\x00100:1", Options{MaxTokens: 20, OverlapTokens: 4})
	b := Split(text, "telegram:acct-1\x00100:1", Options{MaxTokens: 20, OverlapTokens: 4})
	if len(a) == 0 {
		t.Fatal("expected chunks")
	}
	if len(a) != len(b) {
		t.Fatalf("non-deterministic chunk count: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Errorf("chunk %d id not stable: %q vs %q", i, a[i].ID, b[i].ID)
		}
		if a[i].Content != b[i].Content {
			t.Errorf("chunk %d content not stable", i)
		}
	}
}

func TestSplit_NamespaceChangesIDs(t *testing.T) {
	text := "Some content here that we will chunk."
	a := Split(text, "ns-a", Options{})
	b := Split(text, "ns-b", Options{})
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("expected single chunk each, got %d/%d", len(a), len(b))
	}
	if a[0].ID == b[0].ID {
		t.Error("expected different namespaces to yield different chunk ids")
	}
}

func TestSplit_TokenBounded(t *testing.T) {
	// Build many small paragraphs.
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		sb.WriteString("This is sentence number with several words in it.\n\n")
	}
	maxTokens := 30
	chunks := Split(sb.String(), "ns", Options{MaxTokens: maxTokens, OverlapTokens: 0})
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		// Allow a small slack for paragraph-boundary packing.
		if c.TokenEstimate > maxTokens*2 {
			t.Errorf("chunk %d exceeds bound badly: %d tokens", i, c.TokenEstimate)
		}
	}
}

func TestSplit_HardSplitOversizedParagraph(t *testing.T) {
	// One giant paragraph with no blank lines must still be split.
	giant := strings.Repeat("word ", 2000) // ~10k chars
	chunks := Split(giant, "ns", Options{MaxTokens: 100, OverlapTokens: 0})
	if len(chunks) < 2 {
		t.Fatalf("expected oversized paragraph to be hard-split, got %d chunks", len(chunks))
	}
}

func TestSplit_Overlap(t *testing.T) {
	text := "Alpha paragraph distinct words here.\n\nBravo paragraph other content entirely.\n\nCharlie paragraph more unique text.\n\n"
	noOverlap := Split(text, "ns", Options{MaxTokens: 12, OverlapTokens: 0})
	withOverlap := Split(text, "ns", Options{MaxTokens: 12, OverlapTokens: 6})
	if len(noOverlap) < 2 || len(withOverlap) < 2 {
		t.Skip("not enough chunks to compare overlap")
	}
	// With overlap, later chunks should start at an earlier offset than without.
	if withOverlap[1].StartOffset >= noOverlap[1].StartOffset {
		t.Errorf("overlap did not extend chunk backwards: with=%d no=%d",
			withOverlap[1].StartOffset, noOverlap[1].StartOffset)
	}
}

func TestSplit_OffsetsMapToSource(t *testing.T) {
	text := "First paragraph.\n\nSecond paragraph with more text.\n\nThird one."
	chunks := Split(text, "ns", Options{MaxTokens: 8, OverlapTokens: 0})
	runes := []rune(text)
	for i, c := range chunks {
		if c.StartOffset < 0 || c.EndOffset > len(runes) || c.StartOffset > c.EndOffset {
			t.Fatalf("chunk %d invalid offsets [%d,%d] len=%d", i, c.StartOffset, c.EndOffset, len(runes))
		}
		sub := strings.TrimSpace(string(runes[c.StartOffset:c.EndOffset]))
		if !strings.Contains(sub, strings.TrimSpace(c.Content)) && !strings.Contains(strings.TrimSpace(c.Content), sub) {
			t.Errorf("chunk %d content does not match its source span", i)
		}
	}
}
