// Package chunk implements CIP stage 4 (Chunk): token-bounded segmentation of
// canonical Markdown into chunks with stable IDs and a configurable overlap
// policy. Chunking is a pure function of the input bytes — re-chunking the same
// text with the same options yields byte-identical chunk IDs, which makes the
// downstream persist step idempotent (CIP P2 resumability requirement).
package chunk

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Default segmentation parameters. Per CIP §6 these are starting values to be
// tuned, not a contract.
const (
	DefaultMaxTokens     = 3000
	DefaultOverlapTokens = 200
)

// approxCharsPerToken is the rough char→token ratio used for bounding. It mirrors
// the estimator used elsewhere in the codebase (≈4 chars/token).
const approxCharsPerToken = 4

// Options configures the chunker.
type Options struct {
	MaxTokens     int // max tokens per chunk (default DefaultMaxTokens)
	OverlapTokens int // tokens of overlap carried between adjacent chunks (default DefaultOverlapTokens)
}

func (o Options) withDefaults() Options {
	if o.MaxTokens <= 0 {
		o.MaxTokens = DefaultMaxTokens
	}
	if o.OverlapTokens < 0 {
		o.OverlapTokens = 0
	}
	if o.OverlapTokens >= o.MaxTokens {
		o.OverlapTokens = o.MaxTokens / 4
	}
	return o
}

// Chunk is a token-bounded segment of the source text with a stable id and the
// char offsets it occupies in that source.
type Chunk struct {
	ID            string
	Index         int
	Content       string
	StartOffset   int // inclusive char offset into source text
	EndOffset     int // exclusive char offset into source text
	TokenEstimate int
}

// EstimateTokens returns the approximate token count for a string.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len([]rune(s)) + approxCharsPerToken - 1) / approxCharsPerToken
}

// Split segments text into token-bounded chunks. namespace is a stable string
// (e.g. "connectorID\x00sourceID") folded into each chunk id so ids are unique
// across records yet deterministic for identical (namespace, text, options).
//
// Segmentation prefers paragraph boundaries (blank lines); a paragraph larger
// than the budget on its own is hard-split on a token boundary. Adjacent chunks
// overlap by OverlapTokens to preserve cross-boundary context.
func Split(text, namespace string, opts Options) []Chunk {
	opts = opts.withDefaults()
	if strings.TrimSpace(text) == "" {
		return nil
	}

	maxChars := opts.MaxTokens * approxCharsPerToken
	overlapChars := opts.OverlapTokens * approxCharsPerToken

	runes := []rune(text)
	spans := paragraphSpans(runes)

	type window struct{ start, end int }
	var windows []window
	curStart := -1
	curEnd := -1
	for _, sp := range spans {
		segLen := sp.end - sp.start
		if segLen > maxChars {
			// Flush any accumulated window before hard-splitting the big paragraph.
			if curStart >= 0 {
				windows = append(windows, window{curStart, curEnd})
				curStart, curEnd = -1, -1
			}
			for off := sp.start; off < sp.end; off += maxChars {
				end := off + maxChars
				if end > sp.end {
					end = sp.end
				}
				windows = append(windows, window{off, end})
			}
			continue
		}
		if curStart < 0 {
			curStart, curEnd = sp.start, sp.end
			continue
		}
		if sp.end-curStart <= maxChars {
			curEnd = sp.end
			continue
		}
		windows = append(windows, window{curStart, curEnd})
		curStart, curEnd = sp.start, sp.end
	}
	if curStart >= 0 {
		windows = append(windows, window{curStart, curEnd})
	}

	out := make([]Chunk, 0, len(windows))
	for i, w := range windows {
		start := w.start
		// Apply leading overlap from the previous window (except the first chunk).
		if i > 0 && overlapChars > 0 {
			start = w.start - overlapChars
			if start < 0 {
				start = 0
			}
		}
		content := strings.TrimRight(string(runes[start:w.end]), "\n")
		// Recompute end offset after trimming trailing newlines.
		end := start + len([]rune(content))
		if strings.TrimSpace(content) == "" {
			continue
		}
		out = append(out, Chunk{
			ID:            chunkID(namespace, len(out), content),
			Index:         len(out),
			Content:       content,
			StartOffset:   start,
			EndOffset:     end,
			TokenEstimate: EstimateTokens(content),
		})
	}
	return out
}

type span struct{ start, end int }

// paragraphSpans splits runes into spans on blank-line boundaries, keeping the
// trailing newlines with each span so offsets remain exact.
func paragraphSpans(runes []rune) []span {
	var spans []span
	start := 0
	i := 0
	for i < len(runes) {
		if runes[i] == '\n' {
			// Count consecutive newlines.
			j := i
			for j < len(runes) && runes[j] == '\n' {
				j++
			}
			if j-i >= 2 { // blank line → paragraph boundary
				spans = append(spans, span{start, j})
				start = j
				i = j
				continue
			}
			i = j
			continue
		}
		i++
	}
	if start < len(runes) {
		spans = append(spans, span{start, len(runes)})
	}
	return spans
}

func chunkID(namespace string, index int, content string) string {
	h := sha256.New()
	h.Write([]byte(namespace))
	h.Write([]byte{0})
	// index keeps ids distinct when identical content recurs within a record.
	h.Write([]byte{byte(index), byte(index >> 8), byte(index >> 16), byte(index >> 24)})
	h.Write([]byte{0})
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))[:32]
}
