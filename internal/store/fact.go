package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Fact is a single memory fact (user_preference, project_decision, technical_context, task_outcome).
// Deprecated is soft-delete: fact is hidden from normal reads; no replacement (unlike supersession).
type Fact struct {
	ID         string
	Scope      string // "directive", "chat", "global"
	ScopeID    string
	Category   string // "user_preference", "project_decision", "technical_context", "task_outcome"
	Key        string
	Value      string
	Keywords   []string
	Tags       []string
	Embedding  []float64
	Source     string // "explicit", "inferred", "observed"
	Deprecated bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SaveFact inserts or updates a fact by (scope, scope_id, category, key). Upserts by replacing value/source/deprecated/updated_at.
func SaveFact(ctx context.Context, db *sql.DB, f Fact) error {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	inferFactKnowledge(ctx, &f)
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now().UTC()
	}
	f.UpdatedAt = time.Now().UTC()
	deprecated := 0
	if f.Deprecated {
		deprecated = 1
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO facts (id, scope, scope_id, category, key, value, keywords, tags, embedding, source, deprecated, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			scope=excluded.scope,
			scope_id=excluded.scope_id,
			category=excluded.category,
			key=excluded.key,
			value=excluded.value,
			keywords=excluded.keywords,
			tags=excluded.tags,
			embedding=excluded.embedding,
			source=excluded.source,
			deprecated=excluded.deprecated,
			updated_at=excluded.updated_at
	`, f.ID, f.Scope, f.ScopeID, f.Category, f.Key, f.Value,
		marshalJSONStringSlice(f.Keywords), marshalJSONStringSlice(f.Tags), marshalJSONVector(f.Embedding),
		f.Source, deprecated, f.CreatedAt.Format(timeFormat), f.UpdatedAt.Format(timeFormat))
	if err != nil {
		return fmt.Errorf("store: save fact: %w", err)
	}
	return nil
}

// ListFacts returns facts for the given scope and scope_id, and optionally for global (scope="global", scope_id="").
// When includeDeprecated is false, deprecated facts are excluded. limit is the max number of facts per scope; order is by updated_at desc.
func ListFacts(ctx context.Context, db *sql.DB, scope, scopeID string, includeGlobal bool, limit int, includeDeprecated bool) ([]Fact, error) {
	if limit <= 0 {
		limit = 50
	}
	var clauses []string
	var args []any
	if scope != "" && scopeID != "" {
		clauses = append(clauses, "(scope = ? AND scope_id = ?)")
		args = append(args, scope, scopeID)
	}
	if includeGlobal {
		clauses = append(clauses, "(scope = 'global' AND scope_id = '')")
	}
	if len(clauses) == 0 {
		return nil, nil
	}
	where := "(" + strings.Join(clauses, " OR ") + ")"
	if !includeDeprecated {
		where += " AND (COALESCE(deprecated, 0) = 0)"
	}
	query := `SELECT id, scope, scope_id, category, key, value, COALESCE(keywords, '[]'), COALESCE(tags, '[]'), COALESCE(embedding, '[]'), source, COALESCE(deprecated, 0), created_at, updated_at FROM facts WHERE ` + where + ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list facts: %w", err)
	}
	defer rows.Close()
	var out []Fact
	for rows.Next() {
		var f Fact
		var createdAt, updatedAt, keywordsJSON, tagsJSON, embeddingJSON string
		var deprecated int
		err := rows.Scan(&f.ID, &f.Scope, &f.ScopeID, &f.Category, &f.Key, &f.Value, &keywordsJSON, &tagsJSON, &embeddingJSON, &f.Source, &deprecated, &createdAt, &updatedAt)
		if err != nil {
			return nil, fmt.Errorf("store: scan fact: %w", err)
		}
		f.Deprecated = deprecated != 0
		f.Keywords = ParseJSONStringSlice(keywordsJSON)
		f.Tags = ParseJSONStringSlice(tagsJSON)
		f.Embedding = ParseJSONVector(embeddingJSON)
		f.CreatedAt, _ = parseTime(createdAt)
		f.UpdatedAt, _ = parseTime(updatedAt)
		out = append(out, f)
	}
	return out, rows.Err()
}

// GetFact returns a single fact by ID.
func GetFact(ctx context.Context, db *sql.DB, id string) (Fact, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, scope, scope_id, category, key, value, COALESCE(keywords, '[]'), COALESCE(tags, '[]'), COALESCE(embedding, '[]'), source, COALESCE(deprecated, 0), created_at, updated_at
		FROM facts
		WHERE id = ?
	`, id)
	var f Fact
	var createdAt, updatedAt, keywordsJSON, tagsJSON, embeddingJSON string
	var deprecated int
	if err := row.Scan(&f.ID, &f.Scope, &f.ScopeID, &f.Category, &f.Key, &f.Value, &keywordsJSON, &tagsJSON, &embeddingJSON, &f.Source, &deprecated, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return Fact{}, fmt.Errorf("store: fact not found: %w", err)
		}
		return Fact{}, fmt.Errorf("store: get fact: %w", err)
	}
	f.Deprecated = deprecated != 0
	f.Keywords = ParseJSONStringSlice(keywordsJSON)
	f.Tags = ParseJSONStringSlice(tagsJSON)
	f.Embedding = ParseJSONVector(embeddingJSON)
	f.CreatedAt, _ = parseTime(createdAt)
	f.UpdatedAt, _ = parseTime(updatedAt)
	return f, nil
}

// DeprecateFact marks a fact as deprecated (soft-delete without replacement). Idempotent.
func DeprecateFact(ctx context.Context, db *sql.DB, factID string) error {
	if factID == "" {
		return fmt.Errorf("store: fact id required to deprecate")
	}
	res, err := db.ExecContext(ctx, `UPDATE facts SET deprecated = 1, updated_at = ? WHERE id = ?`, time.Now().UTC().Format(timeFormat), factID)
	if err != nil {
		return fmt.Errorf("store: deprecate fact: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("store: fact not found: %s", factID)
	}
	return nil
}

// FormatFactsForPrompt returns a markdown block suitable for injection into a system prompt.
func FormatFactsForPrompt(facts []Fact) string {
	if len(facts) == 0 {
		return ""
	}
	byCategory := make(map[string][]Fact)
	for _, f := range facts {
		byCategory[f.Category] = append(byCategory[f.Category], f)
	}
	var sb strings.Builder
	sb.WriteString("\n\n## Memory\n\n")
	for _, cat := range []string{"user_preference", "project_decision", "technical_context", "task_outcome"} {
		list := byCategory[cat]
		if len(list) == 0 {
			continue
		}
		label := strings.ReplaceAll(cat, "_", " ")
		words := strings.Fields(label)
		for i, w := range words {
			if len(w) > 0 {
				words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
			}
		}
		sb.WriteString("### " + strings.Join(words, " ") + "\n\n")
		for _, f := range list {
			sb.WriteString("- " + f.Key + ": " + f.Value + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
