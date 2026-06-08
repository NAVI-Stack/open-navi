package store

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ceoai/navi/internal/schema"
)

const knowledgeEmbeddingDimensions = 96

type KnowledgeEmbedder func(ctx context.Context, text string) ([]float64, error)

var (
	knowledgeEmbedderMu sync.RWMutex
	knowledgeEmbedder   KnowledgeEmbedder
)

var knowledgeTokenAliases = map[string][]string{
	"go":          {"golang", "language"},
	"golang":      {"go", "language"},
	"sqlite":      {"database", "sql", "storage"},
	"postgres":    {"database", "postgresql", "sql", "storage"},
	"postgresql":  {"database", "postgres", "sql", "storage"},
	"db":          {"database", "sql", "storage"},
	"database":    {"db", "sql", "storage"},
	"memory":      {"knowledge", "recall", "context"},
	"preference":  {"preference", "setting"},
	"preferences": {"preference", "setting"},
	"task":        {"work", "todo"},
	"bug":         {"issue", "problem"},
	"error":       {"failure", "problem"},
}

func NormalizeKnowledgeText(text string) []string {
	lowered := strings.ToLower(text)
	var b strings.Builder
	b.Grow(len(lowered))
	for _, r := range lowered {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	raw := strings.Fields(b.String())
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw)*2)
	for _, token := range raw {
		if token == "" {
			continue
		}
		if _, ok := seen[token]; !ok {
			seen[token] = struct{}{}
			out = append(out, token)
		}
		for _, alias := range knowledgeTokenAliases[token] {
			if _, ok := seen[alias]; ok {
				continue
			}
			seen[alias] = struct{}{}
			out = append(out, alias)
		}
	}
	return out
}

func topKeywordsFromText(limit int, parts ...string) []string {
	if limit <= 0 {
		limit = 6
	}
	counts := map[string]int{}
	for _, part := range parts {
		for _, token := range NormalizeKnowledgeText(part) {
			if len(token) <= 2 {
				continue
			}
			counts[token]++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	type pair struct {
		token string
		count int
	}
	items := make([]pair, 0, len(counts))
	for token, count := range counts {
		items = append(items, pair{token: token, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].token < items[j].token
		}
		return items[i].count > items[j].count
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.token)
	}
	return out
}

func canonicalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func BuildKnowledgeEmbedding(parts ...string) []float64 {
	vector := make([]float64, knowledgeEmbeddingDimensions)
	var total float64
	for _, part := range parts {
		for _, token := range NormalizeKnowledgeText(part) {
			if token == "" {
				continue
			}
			h := fnv.New64a()
			_, _ = h.Write([]byte(token))
			idx := int(h.Sum64() % uint64(len(vector)))
			vector[idx] += 1
			total += 1
			if len(token) >= 4 {
				for i := 0; i <= len(token)-3; i++ {
					h := fnv.New64a()
					_, _ = h.Write([]byte(token[i : i+3]))
					idx := int(h.Sum64() % uint64(len(vector)))
					vector[idx] += 0.35
					total += 0.35
				}
			}
		}
	}
	if total == 0 {
		return nil
	}
	var norm float64
	for _, v := range vector {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return nil
	}
	for i := range vector {
		vector[i] /= norm
	}
	return vector
}

func SetKnowledgeEmbedder(embedder KnowledgeEmbedder) {
	knowledgeEmbedderMu.Lock()
	defer knowledgeEmbedderMu.Unlock()
	knowledgeEmbedder = embedder
}

func currentKnowledgeEmbedder() KnowledgeEmbedder {
	knowledgeEmbedderMu.RLock()
	defer knowledgeEmbedderMu.RUnlock()
	return knowledgeEmbedder
}

// KnowledgeEmbedding generates the semantic vector for knowledge text using the
// configured embedder when available, falling back to the local heuristic model.
func KnowledgeEmbedding(ctx context.Context, parts ...string) []float64 {
	text := strings.TrimSpace(strings.Join(parts, "\n"))
	if embedder := currentKnowledgeEmbedder(); embedder != nil && text != "" {
		if vector, err := embedder(ctx, text); err == nil && len(vector) > 0 {
			return vector
		}
	}
	return BuildKnowledgeEmbedding(parts...)
}

func CosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot float64
	for i := 0; i < n; i++ {
		dot += a[i] * b[i]
	}
	if dot < 0 {
		return 0
	}
	if dot > 1 {
		return 1
	}
	return dot
}

func marshalJSONStringSlice(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(values)
	return string(b)
}

func marshalJSONVector(values []float64) string {
	if len(values) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(values)
	return string(b)
}

func ParseJSONStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return canonicalizeTags(out)
}

func ParseJSONVector(raw string) []float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []float64
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func inferFactKnowledge(ctx context.Context, f *Fact) {
	if len(f.Keywords) == 0 {
		f.Keywords = topKeywordsFromText(6, f.Category, f.Key, f.Value)
	}
	if len(f.Tags) == 0 {
		f.Tags = canonicalizeTags([]string{f.Scope, f.Category, f.Source})
	} else {
		f.Tags = canonicalizeTags(f.Tags)
	}
	f.Embedding = KnowledgeEmbedding(ctx, f.Category, f.Key, f.Value, strings.Join(f.Keywords, " "), strings.Join(f.Tags, " "))
}

func inferMemoryKnowledge(ctx context.Context, m *schema.Memory) {
	if len(m.Keywords) == 0 {
		m.Keywords = topKeywordsFromText(6, m.Summary, m.Details, m.Significance)
	}
	if len(m.Tags) == 0 {
		m.Tags = canonicalizeTags([]string{m.Scope, m.Significance, m.Source})
	} else {
		m.Tags = canonicalizeTags(m.Tags)
	}
	m.Embedding = KnowledgeEmbedding(ctx, m.Summary, m.Details, m.Significance, strings.Join(m.Keywords, " "), strings.Join(m.Tags, " "))
}

func canonicalKnowledgePair(leftType, leftID, rightType, rightID string) (string, string, string, string) {
	left := fmt.Sprintf("%s:%s", leftType, leftID)
	right := fmt.Sprintf("%s:%s", rightType, rightID)
	if left <= right {
		return leftType, leftID, rightType, rightID
	}
	return rightType, rightID, leftType, leftID
}

func timeNowString() string {
	return time.Now().UTC().Format(timeFormat)
}
