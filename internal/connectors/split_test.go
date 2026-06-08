package connectors

import (
	"strings"
	"testing"
)

func TestSplitMessage_NoSplitNeeded(t *testing.T) {
	text := "Hello, world!"
	parts := SplitMessage(text, 100)
	if len(parts) != 1 || parts[0] != text {
		t.Fatalf("expected 1 part, got %d: %v", len(parts), parts)
	}
}

func TestSplitMessage_ZeroLimit(t *testing.T) {
	text := "Hello, world!"
	parts := SplitMessage(text, 0)
	if len(parts) != 1 || parts[0] != text {
		t.Fatalf("expected 1 part with limit=0, got %d: %v", len(parts), parts)
	}
}

func TestSplitMessage_ParagraphBreak(t *testing.T) {
	text := "First paragraph.\n\nSecond paragraph."
	parts := SplitMessage(text, 20)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d: %v", len(parts), parts)
	}
	if parts[0] != "First paragraph." {
		t.Errorf("part[0] = %q, want %q", parts[0], "First paragraph.")
	}
	if parts[1] != "Second paragraph." {
		t.Errorf("part[1] = %q, want %q", parts[1], "Second paragraph.")
	}
}

func TestSplitMessage_SentenceBreak(t *testing.T) {
	text := "First sentence. Second sentence."
	parts := SplitMessage(text, 20)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d: %v", len(parts), parts)
	}
	if parts[0] != "First sentence." {
		t.Errorf("part[0] = %q, want %q", parts[0], "First sentence.")
	}
	if parts[1] != "Second sentence." {
		t.Errorf("part[1] = %q, want %q", parts[1], "Second sentence.")
	}
}

func TestSplitMessage_WordBreak(t *testing.T) {
	text := "aaa bbb ccc ddd eee"
	parts := SplitMessage(text, 8)
	if len(parts) < 2 {
		t.Fatalf("expected >=2 parts, got %d: %v", len(parts), parts)
	}
	// Reassemble should have all words
	joined := strings.Join(parts, " ")
	for _, word := range []string{"aaa", "bbb", "ccc", "ddd", "eee"} {
		if !strings.Contains(joined, word) {
			t.Errorf("missing word %q in split result", word)
		}
	}
}

func TestSplitMessage_HardCut(t *testing.T) {
	text := "abcdefghijklmnopqrstuvwxyz"
	parts := SplitMessage(text, 10)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts for 26-char string with limit 10, got %d: %v", len(parts), parts)
	}
	// Reassemble should match original (no whitespace to trim in this case)
	reassembled := strings.Join(parts, "")
	if reassembled != text {
		t.Errorf("reassembled = %q, want %q", reassembled, text)
	}
}

func TestSplitMessage_Unicode(t *testing.T) {
	text := "こんにちは世界"
	parts := SplitMessage(text, 4)
	if len(parts) < 2 {
		t.Fatalf("expected >=2 parts for 7-rune string, got %d: %v", len(parts), parts)
	}
	// Reassemble runes
	total := 0
	for _, p := range parts {
		total += len([]rune(p))
	}
	if total != 7 {
		t.Errorf("total runes = %d, want 7", total)
	}
}

func TestSplitMessage_EmptyString(t *testing.T) {
	parts := SplitMessage("", 100)
	if len(parts) != 1 || parts[0] != "" {
		t.Fatalf("expected 1 empty part, got %d: %v", len(parts), parts)
	}
}
