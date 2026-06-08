package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/schema"
)

// SaveRelationship inserts or updates a relationship edge.
func SaveRelationship(ctx context.Context, db *sql.DB, rel schema.Relationship) error {
	return SaveRelationshipWithProvenance(ctx, db, rel, nil)
}

// SaveRelationshipWithProvenance saves the relationship and upserts entity_provenance for
// entity_type="relationship", entity_id=rel.ID so derivation chain and mutation history
// are available for reflection and audit. If prov is nil, a minimal provenance row is
// written (source=rel.Provenance, confidence=rel.Confidence, empty chain/history).
func SaveRelationshipWithProvenance(ctx context.Context, db *sql.DB, rel schema.Relationship, prov *schema.EntityProvenance) error {
	if rel.ID == "" {
		rel.ID = uuid.New().String()
	}
	recency := rel.Recency.UTC().Format(timeFormat)
	_, err := db.ExecContext(ctx, `
		INSERT INTO entity_relationships (
			id, from_entity_id, to_entity_id, relationship_type, confidence, recency, provenance
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			from_entity_id=excluded.from_entity_id,
			to_entity_id=excluded.to_entity_id,
			relationship_type=excluded.relationship_type,
			confidence=excluded.confidence,
			recency=excluded.recency,
			provenance=excluded.provenance
	`, rel.ID, rel.FromEntityID, rel.ToEntityID, rel.RelationshipType, rel.Confidence, recency, rel.Provenance)
	if err != nil {
		return fmt.Errorf("store: save relationship: %w", err)
	}
	ep := prov
	if ep == nil {
		now := time.Now().UTC()
		ep = &schema.EntityProvenance{
			Source:     rel.Provenance,
			Timestamp:  now,
			Confidence: rel.Confidence,
		}
	}
	if err := SaveEntityProvenance(ctx, db, "relationship", rel.ID, *ep); err != nil {
		return fmt.Errorf("store: save relationship provenance: %w", err)
	}
	return nil
}

// GetRelationshipByEndpoints returns a relationship for the given endpoints and type, if any.
func GetRelationshipByEndpoints(ctx context.Context, db *sql.DB, fromEntityID, toEntityID, relationshipType string) (*schema.Relationship, error) {
	var (
		rel     schema.Relationship
		recency string
	)
	err := db.QueryRowContext(ctx, `
		SELECT id, from_entity_id, to_entity_id, relationship_type, confidence, recency, provenance
		FROM entity_relationships
		WHERE from_entity_id = ? AND to_entity_id = ? AND relationship_type = ?
		LIMIT 1
	`, fromEntityID, toEntityID, relationshipType).Scan(
		&rel.ID,
		&rel.FromEntityID,
		&rel.ToEntityID,
		&rel.RelationshipType,
		&rel.Confidence,
		&recency,
		&rel.Provenance,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get relationship: %w", err)
	}
	rel.Recency, _ = parseTime(recency)
	return &rel, nil
}

// ListRelationships returns relationships originating from a given entity.
func ListRelationships(ctx context.Context, db *sql.DB, fromEntityID string, limit int) ([]schema.Relationship, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, from_entity_id, to_entity_id, relationship_type, confidence, recency, provenance
		FROM entity_relationships
		WHERE from_entity_id = ?
		ORDER BY recency DESC
		LIMIT ?
	`, fromEntityID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list relationships: %w", err)
	}
	defer rows.Close()

	var out []schema.Relationship
	for rows.Next() {
		var (
			rel      schema.Relationship
			recency  string
		)
		if err := rows.Scan(
			&rel.ID,
			&rel.FromEntityID,
			&rel.ToEntityID,
			&rel.RelationshipType,
			&rel.Confidence,
			&recency,
			&rel.Provenance,
		); err != nil {
			return nil, fmt.Errorf("store: scan relationship: %w", err)
		}
		rel.Recency, _ = parseTime(recency)
		out = append(out, rel)
	}
	return out, rows.Err()
}

// ListRelationshipsOlderThan returns relationships whose recency is before the cutoff.
func ListRelationshipsOlderThan(ctx context.Context, db *sql.DB, cutoff time.Time, limit int) ([]schema.Relationship, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, from_entity_id, to_entity_id, relationship_type, confidence, recency, provenance
		FROM entity_relationships
		WHERE recency < ?
		ORDER BY recency ASC
		LIMIT ?
	`, cutoff.UTC().Format(timeFormat), limit)
	if err != nil {
		return nil, fmt.Errorf("store: list relationships older than: %w", err)
	}
	defer rows.Close()

	var out []schema.Relationship
	for rows.Next() {
		var (
			rel     schema.Relationship
			recency string
		)
		if err := rows.Scan(
			&rel.ID,
			&rel.FromEntityID,
			&rel.ToEntityID,
			&rel.RelationshipType,
			&rel.Confidence,
			&recency,
			&rel.Provenance,
		); err != nil {
			return nil, fmt.Errorf("store: scan relationship: %w", err)
		}
		rel.Recency, _ = parseTime(recency)
		out = append(out, rel)
	}
	return out, rows.Err()
}

// ListRelationshipsBelowConfidence returns relationships whose confidence is below the threshold.
func ListRelationshipsBelowConfidence(ctx context.Context, db *sql.DB, threshold float64, limit int) ([]schema.Relationship, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, from_entity_id, to_entity_id, relationship_type, confidence, recency, provenance
		FROM entity_relationships
		WHERE confidence < ?
		ORDER BY confidence ASC
		LIMIT ?
	`, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list relationships below confidence: %w", err)
	}
	defer rows.Close()

	var out []schema.Relationship
	for rows.Next() {
		var (
			rel     schema.Relationship
			recency string
		)
		if err := rows.Scan(
			&rel.ID,
			&rel.FromEntityID,
			&rel.ToEntityID,
			&rel.RelationshipType,
			&rel.Confidence,
			&recency,
			&rel.Provenance,
		); err != nil {
			return nil, fmt.Errorf("store: scan relationship: %w", err)
		}
		rel.Recency, _ = parseTime(recency)
		out = append(out, rel)
	}
	return out, rows.Err()
}

// DeleteRelationship deletes a relationship by ID.
func DeleteRelationship(ctx context.Context, db *sql.DB, id string) error {
	if id == "" {
		return nil
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM entity_relationships WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete relationship: %w", err)
	}
	return nil
}


