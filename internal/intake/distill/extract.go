package distill

import (
	"regexp"
	"strings"

	"github.com/ceoai/navi/internal/intake/canonicalize"
)

// minQuoteRunes is the shortest span we treat as a meaningful quote.
const minQuoteRunes = 12

var (
	// Markdown blockquote line ("> quoted text").
	blockquoteLineRE = regexp.MustCompile(`(?m)^>\s?(.+)$`)
	// Double-quoted span (straight or curly quotes), non-greedy.
	doubleQuotedRE = regexp.MustCompile(`"([^"\n]+)"|“([^”\n]+)”`)
)

// extractQuotes pulls quoted material out of content with span-level offsets
// (byte offsets into content). It captures Markdown blockquotes and inline
// double-quoted spans. Offsets link each quote back to its position in the
// owning chunk, preserving provenance.
func extractQuotes(content string) []Quote {
	var quotes []Quote
	seen := map[string]struct{}{}

	for _, loc := range blockquoteLineRE.FindAllStringSubmatchIndex(content, -1) {
		text := strings.TrimSpace(content[loc[2]:loc[3]])
		if len([]rune(text)) < minQuoteRunes {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		quotes = append(quotes, Quote{Text: text, StartOffset: loc[2], EndOffset: loc[3]})
	}

	for _, loc := range doubleQuotedRE.FindAllStringSubmatchIndex(content, -1) {
		// Either capture group 1 (straight) or group 2 (curly) is populated.
		gs, ge := loc[2], loc[3]
		if gs < 0 {
			gs, ge = loc[4], loc[5]
		}
		if gs < 0 {
			continue
		}
		text := strings.TrimSpace(content[gs:ge])
		if len([]rune(text)) < minQuoteRunes {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		quotes = append(quotes, Quote{Text: text, StartOffset: gs, EndOffset: ge})
	}
	return quotes
}

var (
	urlRE     = regexp.MustCompile(`https?://[^\s<>()\[\]"']+`)
	emailRE   = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	handleRE  = regexp.MustCompile(`@[A-Za-z0-9_]{2,}`)
	hashtagRE = regexp.MustCompile(`#[A-Za-z][A-Za-z0-9_]+`)
	// Identifier-like tokens: dotted/slashed/colon paths, ALLCAPS codes, snake/kebab ids.
	identifierRE = regexp.MustCompile(`\b[A-Za-z0-9]+(?:[._/:\-][A-Za-z0-9]+)+\b`)
	// Proper-noun runs: one or more Capitalized words in sequence.
	properNounRE = regexp.MustCompile(`\b([A-Z][a-z]+)(?:\s+[A-Z][a-z]+)*\b`)
)

// extractEntities deterministically extracts entity-name spans using
// capitalization / identifier / URL heuristics — no LLM. URLs are emitted in
// normalized form (canonicalize.NormalizeURL) but offsets reference the original
// span in content so provenance stays exact. Overlapping lower-priority matches
// (e.g. a proper noun inside an email) are suppressed.
func extractEntities(content string) []EntitySpan {
	var spans []EntitySpan
	occupied := newOccupancy(len(content))

	add := func(matches [][]int, kind string, transform func(string) string) {
		for _, loc := range matches {
			s, e := loc[0], loc[1]
			if occupied.overlaps(s, e) {
				continue
			}
			raw := content[s:e]
			name := raw
			if transform != nil {
				name = transform(raw)
			}
			occupied.mark(s, e)
			spans = append(spans, EntitySpan{Name: name, Kind: kind, StartOffset: s, EndOffset: e})
		}
	}

	// Highest priority first so broad heuristics don't shadow precise ones.
	add(urlRE.FindAllStringIndex(content, -1), "url", canonicalize.NormalizeURL)
	add(emailRE.FindAllStringIndex(content, -1), "email", nil)
	add(handleRE.FindAllStringIndex(content, -1), "handle", nil)
	add(hashtagRE.FindAllStringIndex(content, -1), "hashtag", nil)
	add(identifierRE.FindAllStringIndex(content, -1), "identifier", nil)
	add(properNounRE.FindAllStringIndex(content, -1), "proper_noun", strings.TrimSpace)

	sortEntities(spans)
	return spans
}

// occupancy tracks claimed byte ranges so higher-priority entity kinds suppress
// overlapping lower-priority matches.
type occupancy struct {
	claimed []bool
}

func newOccupancy(n int) *occupancy { return &occupancy{claimed: make([]bool, n)} }

func (o *occupancy) overlaps(s, e int) bool {
	if s < 0 || e > len(o.claimed) {
		return false
	}
	for i := s; i < e; i++ {
		if o.claimed[i] {
			return true
		}
	}
	return false
}

func (o *occupancy) mark(s, e int) {
	if s < 0 || e > len(o.claimed) {
		return
	}
	for i := s; i < e; i++ {
		o.claimed[i] = true
	}
}
