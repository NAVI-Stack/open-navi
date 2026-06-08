package tool

import (
	"sort"
	"strings"
	"unicode"
)

// Index projects a registry snapshot into exact and ranked discovery views.
// It is intentionally read-only and never acts as a source of truth.
type Index struct {
	snapshotID string
	byID       map[string]indexedTool
	docs       []indexedTool
}

// SearchCandidate is a ranked discovery result derived from the registry.
type SearchCandidate struct {
	SnapshotID   string
	Tool         *Tool
	Relevance    float64
	MatchKind    string
	Discoverable bool
}

// ExactLookupResult represents an exact tool identity lookup plus optional related matches.
type ExactLookupResult struct {
	SnapshotID string
	ExactMatch *SearchCandidate
	Related    []SearchCandidate
}

type indexedTool struct {
	tool            *Tool
	normalizedID    string
	normalizedName  string
	normalizedDesc  string
	normalizedAlias []string
	idTokens        map[string]struct{}
	nameTokens      map[string]struct{}
	descTokens      map[string]struct{}
	aliasTokens     map[string]struct{}
	tagTokens       map[string]struct{}
	discoverable    bool
}

// NewIndex builds a fresh discovery index from the current registry snapshot.
func NewIndex(reg *Registry) *Index {
	if reg == nil {
		return &Index{
			byID: make(map[string]indexedTool),
		}
	}
	return NewIndexFromSnapshot(reg.Snapshot())
}

// NewIndexFromSnapshot builds an immutable index projection from a registry snapshot.
func NewIndexFromSnapshot(snapshot Snapshot) *Index {
	idx := &Index{
		snapshotID: snapshot.ID,
		byID:       make(map[string]indexedTool, len(snapshot.Tools)),
		docs:       make([]indexedTool, 0, len(snapshot.Tools)),
	}
	for _, toolEntry := range snapshot.Tools {
		if toolEntry == nil {
			continue
		}
		doc := buildIndexedTool(toolEntry)
		idx.byID[doc.normalizedID] = doc
		idx.docs = append(idx.docs, doc)
	}
	return idx
}

// SnapshotID returns the registry snapshot id this index projects.
func (idx *Index) SnapshotID() string {
	if idx == nil {
		return ""
	}
	return idx.snapshotID
}

// ExactLookup resolves a canonical tool_id and returns related matches on miss.
func (idx *Index) ExactLookup(query string, relatedLimit int) ExactLookupResult {
	result := ExactLookupResult{SnapshotID: idx.SnapshotID()}
	if idx == nil {
		return result
	}
	query = normalizeSearchText(query)
	if query == "" {
		return result
	}
	if doc, ok := idx.byID[query]; ok {
		candidate := idx.candidate(doc, 1.0, "exact")
		result.ExactMatch = &candidate
		return result
	}
	result.Related = idx.relatedCandidates(query, relatedLimit)
	return result
}

// LexicalSearch ranks discoverable tools using aliases, names, ids, and descriptions.
func (idx *Index) LexicalSearch(query string, limit int) []SearchCandidate {
	if idx == nil {
		return nil
	}
	query = normalizeSearchText(query)
	if query == "" {
		return nil
	}
	queryTokens := tokenSet(query)
	ranked := make([]SearchCandidate, 0)
	for _, doc := range idx.docs {
		if !doc.discoverable {
			continue
		}
		score, matchKind := lexicalScore(doc, query, queryTokens)
		if score <= 0 {
			continue
		}
		ranked = append(ranked, idx.candidate(doc, score, matchKind))
	}
	return sortCandidates(ranked, limit)
}

// TagSearch ranks discoverable tools by capability tags and governance domain.
func (idx *Index) TagSearch(query string, limit int) []SearchCandidate {
	if idx == nil {
		return nil
	}
	query = normalizeSearchText(query)
	if query == "" {
		return nil
	}
	queryTokens := tokenSet(query)
	ranked := make([]SearchCandidate, 0)
	for _, doc := range idx.docs {
		if !doc.discoverable {
			continue
		}
		score := tagScore(doc, query, queryTokens)
		if score <= 0 {
			continue
		}
		ranked = append(ranked, idx.candidate(doc, score, "tag"))
	}
	return sortCandidates(ranked, limit)
}

// RelatedTools returns discoverable nearby matches for a query without implying authority.
func (idx *Index) RelatedTools(query string, limit int) []SearchCandidate {
	if idx == nil {
		return nil
	}
	return idx.relatedCandidates(normalizeSearchText(query), limit)
}

func (idx *Index) relatedCandidates(query string, limit int) []SearchCandidate {
	if query == "" {
		return nil
	}
	queryTokens := tokenSet(query)
	ranked := make([]SearchCandidate, 0)
	for _, doc := range idx.docs {
		if !doc.discoverable {
			continue
		}
		score, matchKind := lexicalScore(doc, query, queryTokens)
		tagMatch := tagScore(doc, query, queryTokens)
		if tagMatch > score {
			score = tagMatch
			matchKind = "related"
		}
		if score < 0.15 {
			continue
		}
		if matchKind == "" {
			matchKind = "related"
		}
		ranked = append(ranked, idx.candidate(doc, score, matchKind))
	}
	return sortCandidates(ranked, limit)
}

func (idx *Index) candidate(doc indexedTool, relevance float64, matchKind string) SearchCandidate {
	return SearchCandidate{
		SnapshotID:   idx.snapshotID,
		Tool:         cloneTool(doc.tool),
		Relevance:    clampScore(relevance),
		MatchKind:    matchKind,
		Discoverable: doc.discoverable,
	}
}

func buildIndexedTool(toolEntry *Tool) indexedTool {
	normalizedAliases := make([]string, 0, len(toolEntry.Aliases))
	for _, alias := range toolEntry.Aliases {
		alias = normalizeSearchText(alias)
		if alias != "" {
			normalizedAliases = append(normalizedAliases, alias)
		}
	}
	tagValues := append([]string(nil), toolEntry.CapabilityTags...)
	tagValues = append(tagValues, strings.TrimSpace(toolEntry.Governance.Domain))
	tagValues = append(tagValues, toolEntry.Metadata.Tags...)
	return indexedTool{
		tool:            cloneTool(toolEntry),
		normalizedID:    normalizeSearchText(toolEntry.ToolID),
		normalizedName:  normalizeSearchText(toolEntry.DisplayName),
		normalizedDesc:  normalizeSearchText(toolEntry.Description),
		normalizedAlias: normalizedAliases,
		idTokens:        tokenSet(toolEntry.ToolID),
		nameTokens:      tokenSet(toolEntry.DisplayName),
		descTokens:      tokenSet(toolEntry.Description),
		aliasTokens:     tokenSet(strings.Join(toolEntry.Aliases, " ")),
		tagTokens:       tokenSet(strings.Join(tagValues, " ")),
		discoverable:    isDiscoverableTool(toolEntry),
	}
}

func isDiscoverableTool(toolEntry *Tool) bool {
	if toolEntry == nil || toolEntry.Hidden {
		return false
	}
	switch toolEntry.Status {
	case ToolStatusActive, ToolStatusDeprecated:
		return true
	default:
		return false
	}
}

func lexicalScore(doc indexedTool, query string, queryTokens map[string]struct{}) (float64, string) {
	if query == "" {
		return 0, ""
	}
	if query == doc.normalizedID {
		return 1.0, "exact"
	}
	for _, alias := range doc.normalizedAlias {
		if query == alias {
			return 0.98, "alias"
		}
	}
	if query == doc.normalizedName {
		return 0.94, "name"
	}

	score := 0.0
	matchKind := "lexical"
	if strings.Contains(doc.normalizedID, query) {
		score += 0.35
	}
	if strings.Contains(doc.normalizedName, query) {
		score += 0.30
	}
	for _, alias := range doc.normalizedAlias {
		if strings.Contains(alias, query) {
			score += 0.38
			matchKind = "alias"
			break
		}
	}
	if strings.Contains(doc.normalizedDesc, query) {
		score += 0.10
	}
	if len(queryTokens) == 0 {
		return clampScore(score), matchKind
	}
	overlap := 0.0
	for token := range queryTokens {
		tokenScore := 0.0
		if _, ok := doc.aliasTokens[token]; ok {
			tokenScore = maxFloat(tokenScore, 0.26)
			matchKind = "alias"
		}
		if _, ok := doc.idTokens[token]; ok {
			tokenScore = maxFloat(tokenScore, 0.24)
		}
		if _, ok := doc.nameTokens[token]; ok {
			tokenScore = maxFloat(tokenScore, 0.22)
		}
		if _, ok := doc.descTokens[token]; ok {
			tokenScore = maxFloat(tokenScore, 0.12)
		}
		overlap += tokenScore
	}
	score += overlap / float64(len(queryTokens))
	return clampScore(score), matchKind
}

func tagScore(doc indexedTool, query string, queryTokens map[string]struct{}) float64 {
	if query == "" {
		return 0
	}
	score := 0.0
	if len(queryTokens) == 0 {
		return 0
	}
	for token := range queryTokens {
		if _, ok := doc.tagTokens[token]; ok {
			score += 0.5
		}
	}
	if len(queryTokens) > 0 {
		score = score / float64(len(queryTokens))
	}
	for tag := range doc.tagTokens {
		if strings.Contains(tag, query) {
			score += 0.35
			break
		}
	}
	return clampScore(score)
}

func sortCandidates(candidates []SearchCandidate, limit int) []SearchCandidate {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Relevance == candidates[j].Relevance {
			leftID, rightID := "", ""
			if candidates[i].Tool != nil {
				leftID = candidates[i].Tool.ToolID
			}
			if candidates[j].Tool != nil {
				rightID = candidates[j].Tool.ToolID
			}
			return leftID < rightID
		}
		return candidates[i].Relevance > candidates[j].Relevance
	})
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates
}

func normalizeSearchText(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	return strings.Join(fields, " ")
}

func tokenSet(raw string) map[string]struct{} {
	normalized := normalizeSearchText(raw)
	if normalized == "" {
		return nil
	}
	out := make(map[string]struct{})
	for _, token := range strings.Fields(normalized) {
		if token == "" {
			continue
		}
		out[token] = struct{}{}
	}
	return out
}

func clampScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func maxFloat(left, right float64) float64 {
	if right > left {
		return right
	}
	return left
}
