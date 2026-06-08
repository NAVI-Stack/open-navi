package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ceoai/navi/internal/schema"
)

// AssembleUserModel builds the User Model composite at query time from Contacts (owner),
// Knowledge (facts), Memories, Configuration, and Priorities. Never stored.
func AssembleUserModel(ctx context.Context, db *sql.DB, ownerID string, factsLimit, memoriesLimit, configLimit, prioritiesLimit int) (schema.UserModel, error) {
	if ownerID == "" {
		return schema.UserModel{}, fmt.Errorf("store: ownerID required to assemble user model")
	}
	if factsLimit <= 0 {
		factsLimit = 30
	}
	if memoriesLimit <= 0 {
		memoriesLimit = 20
	}
	if configLimit <= 0 {
		configLimit = 50
	}
	if prioritiesLimit <= 0 {
		prioritiesLimit = 20
	}

	out := schema.UserModel{}

	if c, err := GetContact(ctx, db, ownerID); err == nil {
		out.OwnerContact = &c
	}

	facts, err := ListFacts(ctx, db, "owner", ownerID, true, factsLimit, false)
	if err != nil {
		return out, fmt.Errorf("store: list facts: %w", err)
	}
	for _, f := range facts {
		out.Facts = append(out.Facts, schema.Fact{
			ID:         f.ID,
			Scope:      f.Scope,
			ScopeID:    f.ScopeID,
			Category:   f.Category,
			Key:        f.Key,
			Value:      f.Value,
			Source:     f.Source,
			Deprecated: f.Deprecated,
			CreatedAt:  f.CreatedAt,
			UpdatedAt:  f.UpdatedAt,
		})
	}

	memories, err := ListMemories(ctx, db, "owner", ownerID, memoriesLimit)
	if err != nil {
		return out, fmt.Errorf("store: list memories: %w", err)
	}
	out.Memories = memories

	configRows, err := db.QueryContext(ctx, `
		SELECT id, scope, scope_id, key, value, source, created_at, updated_at
		FROM configuration
		WHERE scope = 'owner' AND scope_id = ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, ownerID, configLimit)
	if err != nil {
		return out, fmt.Errorf("store: list configuration: %w", err)
	}
	defer configRows.Close()
	for configRows.Next() {
		var e schema.ConfigurationEntry
		var createdStr, updatedStr string
		if err := configRows.Scan(&e.ID, &e.Scope, &e.ScopeID, &e.Key, &e.Value, &e.Source, &createdStr, &updatedStr); err != nil {
			return out, err
		}
		e.CreatedAt, _ = parseTime(createdStr)
		e.UpdatedAt, _ = parseTime(updatedStr)
		out.Configuration = append(out.Configuration, e)
	}
	if err := configRows.Err(); err != nil {
		return out, err
	}

	priRows, err := db.QueryContext(ctx, `
		SELECT id, scope, scope_id, name, description, source, created_at, updated_at
		FROM priorities
		WHERE scope = 'owner' AND scope_id = ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, ownerID, prioritiesLimit)
	if err != nil {
		return out, fmt.Errorf("store: list priorities: %w", err)
	}
	defer priRows.Close()
	for priRows.Next() {
		var p schema.Priority
		var createdStr, updatedStr string
		if err := priRows.Scan(&p.ID, &p.Scope, &p.ScopeID, &p.Name, &p.Description, &p.Source, &createdStr, &updatedStr); err != nil {
			return out, err
		}
		p.CreatedAt, _ = parseTime(createdStr)
		p.UpdatedAt, _ = parseTime(updatedStr)
		out.Priorities = append(out.Priorities, p)
	}
	if err := priRows.Err(); err != nil {
		return out, err
	}

	return out, nil
}
