package worldmodel

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

const (
	defaultKnowledgeRecallLimit = 5
	autoLinkThreshold           = 0.42
	recallThreshold             = 0.18
)

type knowledgeCandidate struct {
	EntityType string
	EntityID   string
	Summary    string
	Scope      string
	ScopeID    string
	Keywords   []string
	Tags       []string
	Embedding  []float64
	Memory     *schema.Memory
	Fact       *schema.Fact
}

// RecallKnowledge performs semantic search across facts and memories relevant
// to the provided owner/session scopes.
func (wm *WorldModel) RecallKnowledge(ctx context.Context, query, ownerID, chatID string, limit int) ([]schema.KnowledgeHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultKnowledgeRecallLimit
	}
	queryEmbedding := store.KnowledgeEmbedding(ctx, query)
	if len(queryEmbedding) == 0 {
		return nil, nil
	}

	candidates, err := wm.loadKnowledgeCandidates(ctx, ownerID, chatID)
	if err != nil {
		return nil, err
	}
	hits := make([]schema.KnowledgeHit, 0, len(candidates))
	for _, candidate := range candidates {
		candidateEmbedding := knowledgeCandidateEmbedding(ctx, candidate, len(queryEmbedding))
		score := store.CosineSimilarity(queryEmbedding, candidateEmbedding)
		score += keywordOverlapBoost(query, candidate.Keywords, candidate.Tags)
		if score < recallThreshold {
			continue
		}
		hit := schema.KnowledgeHit{
			EntityType: candidate.EntityType,
			EntityID:   candidate.EntityID,
			Score:      minFloat(score, 1),
			Summary:    candidate.Summary,
			Scope:      candidate.Scope,
			ScopeID:    candidate.ScopeID,
			Keywords:   candidate.Keywords,
			Tags:       candidate.Tags,
			Memory:     candidate.Memory,
			Fact:       candidate.Fact,
		}
		if links, err := store.ListKnowledgeLinks(ctx, wm.db, candidate.EntityType, candidate.EntityID, 4); err == nil {
			hit.Links = links
		}
		hits = append(hits, hit)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Summary < hits[j].Summary
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func (wm *WorldModel) refreshKnowledgeLinks(ctx context.Context, entityType, entityID string) error {
	candidates, err := wm.loadKnowledgeCandidates(ctx, "", "")
	if err != nil {
		return err
	}
	var current *knowledgeCandidate
	for i := range candidates {
		if candidates[i].EntityType == entityType && candidates[i].EntityID == entityID {
			current = &candidates[i]
			break
		}
	}
	if current == nil {
		return nil
	}
	if len(current.Embedding) == 0 {
		currentEmbedding := knowledgeCandidateEmbedding(ctx, *current, 0)
		if len(currentEmbedding) == 0 {
			return nil
		}
		current.Embedding = currentEmbedding
	}
	type scored struct {
		entityType string
		entityID   string
		score      float64
	}
	var scoredLinks []scored
	for _, candidate := range candidates {
		if candidate.EntityType == entityType && candidate.EntityID == entityID {
			continue
		}
		candidateEmbedding := knowledgeCandidateEmbedding(ctx, candidate, len(current.Embedding))
		score := store.CosineSimilarity(current.Embedding, candidateEmbedding)
		if score < autoLinkThreshold {
			continue
		}
		scoredLinks = append(scoredLinks, scored{
			entityType: candidate.EntityType,
			entityID:   candidate.EntityID,
			score:      score,
		})
	}
	sort.Slice(scoredLinks, func(i, j int) bool { return scoredLinks[i].score > scoredLinks[j].score })
	if len(scoredLinks) > 5 {
		scoredLinks = scoredLinks[:5]
	}
	for _, candidate := range scoredLinks {
		if err := store.SaveKnowledgeLink(ctx, wm.db, schema.KnowledgeLink{
			EntityType:      entityType,
			EntityID:        entityID,
			RelatedType:     candidate.entityType,
			RelatedID:       candidate.entityID,
			Similarity:      candidate.score,
			Source:          "semantic_similarity",
			RelationshipTag: "related",
		}); err != nil {
			return err
		}
	}
	return nil
}

func (wm *WorldModel) loadKnowledgeCandidates(ctx context.Context, ownerID, chatID string) ([]knowledgeCandidate, error) {
	if strings.TrimSpace(ownerID) == "" && strings.TrimSpace(chatID) == "" {
		return wm.loadAllKnowledgeCandidates(ctx)
	}
	var out []knowledgeCandidate

	factScopes := []struct {
		scope   string
		scopeID string
	}{
		{scope: "owner", scopeID: ownerID},
		{scope: "session", scopeID: chatID},
	}
	seenFacts := map[string]struct{}{}
	for _, entry := range factScopes {
		if entry.scopeID == "" {
			continue
		}
		facts, err := store.ListFacts(ctx, wm.db, entry.scope, entry.scopeID, false, 100, false)
		if err != nil {
			return nil, err
		}
		for _, fact := range facts {
			if _, ok := seenFacts[fact.ID]; ok {
				continue
			}
			seenFacts[fact.ID] = struct{}{}
			factCopy := schema.Fact{
				ID:         fact.ID,
				Scope:      fact.Scope,
				ScopeID:    fact.ScopeID,
				Category:   fact.Category,
				Key:        fact.Key,
				Value:      fact.Value,
				Keywords:   fact.Keywords,
				Tags:       fact.Tags,
				Embedding:  fact.Embedding,
				Source:     fact.Source,
				Deprecated: fact.Deprecated,
				CreatedAt:  fact.CreatedAt,
				UpdatedAt:  fact.UpdatedAt,
			}
			out = append(out, knowledgeCandidate{
				EntityType: "fact",
				EntityID:   fact.ID,
				Summary:    fmt.Sprintf("%s: %s", fact.Key, fact.Value),
				Scope:      fact.Scope,
				ScopeID:    fact.ScopeID,
				Keywords:   fact.Keywords,
				Tags:       fact.Tags,
				Embedding:  fact.Embedding,
				Fact:       &factCopy,
			})
		}
	}
	globalFacts, err := store.ListFacts(ctx, wm.db, "", "", true, 100, false)
	if err != nil {
		return nil, err
	}
	for _, fact := range globalFacts {
		if _, ok := seenFacts[fact.ID]; ok {
			continue
		}
		seenFacts[fact.ID] = struct{}{}
		factCopy := schema.Fact{
			ID:         fact.ID,
			Scope:      fact.Scope,
			ScopeID:    fact.ScopeID,
			Category:   fact.Category,
			Key:        fact.Key,
			Value:      fact.Value,
			Keywords:   fact.Keywords,
			Tags:       fact.Tags,
			Embedding:  fact.Embedding,
			Source:     fact.Source,
			Deprecated: fact.Deprecated,
			CreatedAt:  fact.CreatedAt,
			UpdatedAt:  fact.UpdatedAt,
		}
		out = append(out, knowledgeCandidate{
			EntityType: "fact",
			EntityID:   fact.ID,
			Summary:    fmt.Sprintf("%s: %s", fact.Key, fact.Value),
			Scope:      fact.Scope,
			ScopeID:    fact.ScopeID,
			Keywords:   fact.Keywords,
			Tags:       fact.Tags,
			Embedding:  fact.Embedding,
			Fact:       &factCopy,
		})
	}

	memoryScopes := []struct {
		scope   string
		scopeID string
	}{
		{scope: "owner", scopeID: ownerID},
		{scope: "session", scopeID: chatID},
	}
	seenMemories := map[string]struct{}{}
	for _, entry := range memoryScopes {
		if entry.scopeID == "" {
			continue
		}
		memories, err := store.ListMemories(ctx, wm.db, entry.scope, entry.scopeID, 100)
		if err != nil {
			return nil, err
		}
		for _, memory := range memories {
			if _, ok := seenMemories[memory.ID]; ok {
				continue
			}
			seenMemories[memory.ID] = struct{}{}
			memoryCopy := memory
			out = append(out, knowledgeCandidate{
				EntityType: "memory",
				EntityID:   memory.ID,
				Summary:    memory.Summary,
				Scope:      memory.Scope,
				ScopeID:    memory.ScopeID,
				Keywords:   memory.Keywords,
				Tags:       memory.Tags,
				Embedding:  memory.Embedding,
				Memory:     &memoryCopy,
			})
		}
	}
	return out, nil
}

func (wm *WorldModel) loadAllKnowledgeCandidates(ctx context.Context) ([]knowledgeCandidate, error) {
	var out []knowledgeCandidate

	factRows, err := wm.db.QueryContext(ctx, `
		SELECT id, scope, scope_id, category, key, value, COALESCE(keywords, '[]'), COALESCE(tags, '[]'), COALESCE(embedding, '[]'), source, COALESCE(deprecated, 0), created_at, updated_at
		FROM facts
		WHERE COALESCE(deprecated, 0) = 0
	`)
	if err != nil {
		return nil, err
	}
	defer factRows.Close()
	for factRows.Next() {
		var fact store.Fact
		var keywordsJSON, tagsJSON, embeddingJSON, createdStr, updatedStr string
		var deprecated int
		if err := factRows.Scan(&fact.ID, &fact.Scope, &fact.ScopeID, &fact.Category, &fact.Key, &fact.Value, &keywordsJSON, &tagsJSON, &embeddingJSON, &fact.Source, &deprecated, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		fact.Deprecated = deprecated != 0
		fact.Keywords = store.ParseJSONStringSlice(keywordsJSON)
		fact.Tags = store.ParseJSONStringSlice(tagsJSON)
		fact.Embedding = store.ParseJSONVector(embeddingJSON)
		fact.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		fact.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		factCopy := schema.Fact{
			ID:         fact.ID,
			Scope:      fact.Scope,
			ScopeID:    fact.ScopeID,
			Category:   fact.Category,
			Key:        fact.Key,
			Value:      fact.Value,
			Keywords:   fact.Keywords,
			Tags:       fact.Tags,
			Embedding:  fact.Embedding,
			Source:     fact.Source,
			Deprecated: fact.Deprecated,
			CreatedAt:  fact.CreatedAt,
			UpdatedAt:  fact.UpdatedAt,
		}
		out = append(out, knowledgeCandidate{
			EntityType: "fact",
			EntityID:   fact.ID,
			Summary:    fmt.Sprintf("%s: %s", fact.Key, fact.Value),
			Scope:      fact.Scope,
			ScopeID:    fact.ScopeID,
			Keywords:   fact.Keywords,
			Tags:       fact.Tags,
			Embedding:  fact.Embedding,
			Fact:       &factCopy,
		})
	}
	memoryRows, err := wm.db.QueryContext(ctx, `
		SELECT id, scope, scope_id, summary, details, COALESCE(keywords, '[]'), COALESCE(tags, '[]'), COALESCE(embedding, '[]'), significance, source, created_at, updated_at
		FROM memories
	`)
	if err != nil {
		return nil, err
	}
	defer memoryRows.Close()
	for memoryRows.Next() {
		var memory schema.Memory
		var keywordsJSON, tagsJSON, embeddingJSON, createdStr, updatedStr string
		if err := memoryRows.Scan(&memory.ID, &memory.Scope, &memory.ScopeID, &memory.Summary, &memory.Details, &keywordsJSON, &tagsJSON, &embeddingJSON, &memory.Significance, &memory.Source, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		memory.Keywords = store.ParseJSONStringSlice(keywordsJSON)
		memory.Tags = store.ParseJSONStringSlice(tagsJSON)
		memory.Embedding = store.ParseJSONVector(embeddingJSON)
		memory.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		memory.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		memoryCopy := memory
		out = append(out, knowledgeCandidate{
			EntityType: "memory",
			EntityID:   memory.ID,
			Summary:    memory.Summary,
			Scope:      memory.Scope,
			ScopeID:    memory.ScopeID,
			Keywords:   memory.Keywords,
			Tags:       memory.Tags,
			Embedding:  memory.Embedding,
			Memory:     &memoryCopy,
		})
	}
	return out, nil
}

func keywordOverlapBoost(query string, keywords, tags []string) float64 {
	queryTokens := make(map[string]struct{})
	for _, token := range store.NormalizeKnowledgeText(query) {
		queryTokens[token] = struct{}{}
	}
	if len(queryTokens) == 0 {
		return 0
	}
	boost := 0.0
	for _, token := range append(append([]string{}, keywords...), tags...) {
		if _, ok := queryTokens[token]; ok {
			boost += 0.08
		}
	}
	if boost > 0.24 {
		boost = 0.24
	}
	return boost
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func knowledgeCandidateEmbedding(ctx context.Context, candidate knowledgeCandidate, expectedDims int) []float64 {
	embedding := candidate.Embedding
	if len(embedding) > 0 && (expectedDims <= 0 || len(embedding) == expectedDims) {
		return embedding
	}
	switch {
	case candidate.Fact != nil:
		return store.KnowledgeEmbedding(
			ctx,
			candidate.Fact.Category,
			candidate.Fact.Key,
			candidate.Fact.Value,
			strings.Join(candidate.Fact.Keywords, " "),
			strings.Join(candidate.Fact.Tags, " "),
		)
	case candidate.Memory != nil:
		return store.KnowledgeEmbedding(
			ctx,
			candidate.Memory.Summary,
			candidate.Memory.Details,
			candidate.Memory.Significance,
			strings.Join(candidate.Memory.Keywords, " "),
			strings.Join(candidate.Memory.Tags, " "),
		)
	default:
		return store.KnowledgeEmbedding(
			ctx,
			candidate.Summary,
			strings.Join(candidate.Keywords, " "),
			strings.Join(candidate.Tags, " "),
		)
	}
}

func recentKnowledgeQuery(ctx context.Context, db *sql.DB, chatID string) string {
	if strings.TrimSpace(chatID) == "" {
		return ""
	}
	rows, err := db.QueryContext(ctx, `
		SELECT role, content
		FROM navi_chat_messages
		WHERE chat_id = ?
		ORDER BY created_at DESC
		LIMIT 6
	`, chatID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return ""
		}
		return ""
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			return ""
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		parts = append(parts, content)
	}
	if len(parts) == 0 {
		return ""
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}
