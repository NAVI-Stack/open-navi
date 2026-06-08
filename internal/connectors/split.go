package connectors

import (
	"strings"
	"unicode/utf8"
)

// SplitMessage splits text into chunks of at most maxLen runes. It attempts to
// break on paragraph boundaries (double newline), then sentence boundaries
// (. ! ? followed by space/newline), then word boundaries (space), and finally
// does a hard cut when no boundary is found within the chunk.
func SplitMessage(text string, maxLen int) []string {
	if maxLen <= 0 || utf8.RuneCountInString(text) <= maxLen {
		return []string{text}
	}

	runes := []rune(text)
	var parts []string

	for len(runes) > 0 {
		if len(runes) <= maxLen {
			parts = append(parts, string(runes))
			break
		}

		chunk := runes[:maxLen]
		breakAt := -1

		// 1. Try paragraph break (double newline).
		if idx := lastIndexDoubleNewline(chunk); idx > 0 {
			breakAt = idx
		}

		// 2. Try sentence break (. ! ? followed by space or newline).
		if breakAt < 0 {
			breakAt = lastSentenceBoundary(chunk)
		}

		// 3. Try word break (last space).
		if breakAt < 0 {
			breakAt = lastIndexRune(chunk, ' ')
		}

		// 4. Try newline break.
		if breakAt < 0 {
			breakAt = lastIndexRune(chunk, '\n')
		}

		// 5. Hard cut.
		if breakAt < 0 {
			breakAt = maxLen
		}

		part := strings.TrimSpace(string(runes[:breakAt]))
		if part != "" {
			parts = append(parts, part)
		}
		runes = runes[breakAt:]
		// Skip leading whitespace on next chunk.
		for len(runes) > 0 && (runes[0] == ' ' || runes[0] == '\n' || runes[0] == '\r') {
			runes = runes[1:]
		}
	}

	return parts
}

// lastIndexDoubleNewline finds the last "\n\n" in runes and returns the index
// just after it (i.e., the split point).
func lastIndexDoubleNewline(runes []rune) int {
	for i := len(runes) - 2; i >= 1; i-- {
		if runes[i] == '\n' && runes[i-1] == '\n' {
			return i + 1
		}
	}
	return -1
}

// lastSentenceBoundary finds the last ". " or "! " or "? " or ".\n" etc.
func lastSentenceBoundary(runes []rune) int {
	for i := len(runes) - 1; i >= 1; i-- {
		prev := runes[i-1]
		cur := runes[i]
		if (prev == '.' || prev == '!' || prev == '?') && (cur == ' ' || cur == '\n') {
			return i
		}
	}
	return -1
}

// lastIndexRune finds the last occurrence of r in runes.
func lastIndexRune(runes []rune, r rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == r {
			return i + 1
		}
	}
	return -1
}
