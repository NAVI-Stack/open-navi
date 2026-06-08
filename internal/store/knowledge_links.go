package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/open-navi/navi/internal/schema"
)

// SaveKnowledgeLink stores a canonical relationship between two knowledge nodes.
func SaveKnowledgeLink(ctx context.Context, db *sql.DB, link schema.KnowledgeLink) error {
	if link.EntityType == "" || link.EntityID == "" || link.RelatedType == "" || link.RelatedID == "" {
		return fmt.Errorf("store: knowledge link endpoints required")
	}
	leftType, leftID, rightType, rightID := canonicalKnowledgePair(link.EntityType, link.EntityID, link.RelatedType, link.RelatedID)
	now := timeNowString()
	if link.Source == "" {
		link.Source = "semantic_similarity"
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO memory_links (left_type, left_id, right_type, right_id, similarity, source, relationship_tag, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(left_type, left_id, right_type, right_id) DO UPDATE SET
			similarity=excluded.similarity,
			source=excluded.source,
			relationship_tag=excluded.relationship_tag,
			updated_at=excluded.updated_at
	`, leftType, leftID, rightType, rightID, link.Similarity, link.Source, link.RelationshipTag, now, now)
	if err != nil {
		return fmt.Errorf("store: save knowledge link: %w", err)
	}
	return nil
}

// ListKnowledgeLinks returns all knowledge links adjacent to the given node.
func ListKnowledgeLinks(ctx context.Context, db *sql.DB, entityType, entityID string, limit int) ([]schema.KnowledgeLink, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := db.QueryContext(ctx, `
		SELECT left_type, left_id, right_type, right_id, similarity, source, COALESCE(relationship_tag, ''), created_at, updated_at
		FROM memory_links
		WHERE (left_type = ? AND left_id = ?) OR (right_type = ? AND right_id = ?)
		ORDER BY similarity DESC, updated_at DESC
		LIMIT ?
	`, entityType, entityID, entityType, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list knowledge links: %w", err)
	}
	defer rows.Close()
	var out []schema.KnowledgeLink
	for rows.Next() {
		var leftType, leftID, rightType, rightID, source, tag, createdStr, updatedStr string
		var similarity float64
		if err := rows.Scan(&leftType, &leftID, &rightType, &rightID, &similarity, &source, &tag, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("store: scan knowledge link: %w", err)
		}
		link := schema.KnowledgeLink{
			Similarity:      similarity,
			Source:          source,
			RelationshipTag: tag,
		}
		if leftType == entityType && leftID == entityID {
			link.EntityType = leftType
			link.EntityID = leftID
			link.RelatedType = rightType
			link.RelatedID = rightID
		} else {
			link.EntityType = rightType
			link.EntityID = rightID
			link.RelatedType = leftType
			link.RelatedID = leftID
		}
		link.CreatedAt, _ = parseTime(createdStr)
		link.UpdatedAt, _ = parseTime(updatedStr)
		out = append(out, link)
	}
	return out, rows.Err()
}
