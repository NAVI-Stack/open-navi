package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/google/uuid"
)

// entityIDForProposal returns the format used in proposal affected_entities (e.g. "memory:uuid").
func entityIDForProposal(entityType, entityID string) string {
	return entityType + ":" + entityID
}

// ArchiveEntity records an Archive lifecycle action (soft-delete) and auto-declines
// any pending proposals that reference this entity.
func ArchiveEntity(ctx context.Context, db *sql.DB, entityType, entityID string) error {
	id := uuid.New().String()
	now := time.Now().UTC().Format(timeFormat)
	_, err := db.ExecContext(ctx, `
		INSERT INTO tombstones (id, entity_type, entity_id, action, deleted_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, entityType, entityID, string(schema.LifecycleArchive), now)
	if err != nil {
		return fmt.Errorf("store: archive entity: %w", err)
	}
	_, _ = AutoDeclineProposalsForEntity(ctx, db, entityIDForProposal(entityType, entityID))
	return nil
}

// ForgetMemory applies the Forget lifecycle to a memory (deletion path),
// auto-declines pending proposals that reference it, and flags downstream
// derivatives (entities whose derivation_chain includes this memory) for
// re-evaluation at the next Consolidation or Deep Reflection.
func ForgetMemory(ctx context.Context, db *sql.DB, memoryID string) error {
	id := uuid.New().String()
	now := time.Now().UTC().Format(timeFormat)
	_, err := db.ExecContext(ctx, `
		INSERT INTO tombstones (id, entity_type, entity_id, action, deleted_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, "memory", memoryID, string(schema.LifecycleForget), now)
	if err != nil {
		return fmt.Errorf("store: forget memory: %w", err)
	}
	_, _ = AutoDeclineProposalsForEntity(ctx, db, entityIDForProposal("memory", memoryID))
	_, _ = FlagDerivativesForReEvaluation(ctx, db, "memory", memoryID, "forgotten_memory")
	return nil
}

// FlagDerivativesForReEvaluation finds all entity_provenance rows whose
// derivation_chain contains the given source entity and inserts them into
// re_evaluation_flags so Consolidation/Deep Reflection can re-evaluate them.
func FlagDerivativesForReEvaluation(ctx context.Context, db *sql.DB, sourceEntityType, sourceEntityID, reason string) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT entity_type, entity_id, derivation_chain FROM entity_provenance WHERE derivation_chain IS NOT NULL AND derivation_chain != '[]'`)
	if err != nil {
		return 0, fmt.Errorf("store: flag derivatives: %w", err)
	}
	defer rows.Close()
	var flagged int
	sourceRef := sourceEntityType + ":" + sourceEntityID
	for rows.Next() {
		var entityType, entityID string
		var chainJSON sql.NullString
		if err := rows.Scan(&entityType, &entityID, &chainJSON); err != nil {
			return flagged, fmt.Errorf("store: scan provenance: %w", err)
		}
		if !chainJSON.Valid || chainJSON.String == "" {
			continue
		}
		var chain []string
		if err := json.Unmarshal([]byte(chainJSON.String), &chain); err != nil {
			continue
		}
		for _, ref := range chain {
			if ref == sourceRef || ref == sourceEntityID {
				flagID := uuid.New().String()
				now := time.Now().UTC().Format(timeFormat)
				_, err := db.ExecContext(ctx, `
					INSERT INTO re_evaluation_flags (id, entity_type, entity_id, reason, source_entity_type, source_entity_id, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?)
				`, flagID, entityType, entityID, reason, sourceEntityType, sourceEntityID, now)
				if err != nil {
					return flagged, fmt.Errorf("store: insert re_evaluation_flag: %w", err)
				}
				flagged++
				break
			}
		}
	}
	return flagged, nil
}

// ReEvaluationFlag represents a pending re-evaluation for a derived entity.
type ReEvaluationFlag struct {
	ID               string
	EntityType       string
	EntityID         string
	Reason           string
	SourceEntityType string
	SourceEntityID   string
	CreatedAt        time.Time
}

// ListReEvaluationFlags returns up to limit pending re-evaluation flags,
// ordered by created_at ascending.
func ListReEvaluationFlags(ctx context.Context, db *sql.DB, limit int) ([]ReEvaluationFlag, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, entity_type, entity_id, reason, source_entity_type, source_entity_id, created_at
		FROM re_evaluation_flags
		ORDER BY created_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list re_evaluation_flags: %w", err)
	}
	defer rows.Close()
	var out []ReEvaluationFlag
	for rows.Next() {
		var f ReEvaluationFlag
		var createdStr string
		if err := rows.Scan(&f.ID, &f.EntityType, &f.EntityID, &f.Reason, &f.SourceEntityType, &f.SourceEntityID, &createdStr); err != nil {
			return nil, err
		}
		f.CreatedAt, _ = parseTime(createdStr)
		out = append(out, f)
	}
	return out, rows.Err()
}

// ClearReEvaluationFlag deletes a single re-evaluation flag by ID after it
// has been processed by Consolidation/Deep reflection.
func ClearReEvaluationFlag(ctx context.Context, db *sql.DB, id string) error {
	if id == "" {
		return nil
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM re_evaluation_flags WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: clear re_evaluation_flag: %w", err)
	}
	return nil
}

// SupersedeFact records that one fact supersedes another (Knowledge lifecycle).
// Pending proposals that reference the superseded fact are not auto-declined;
// design may require a proposal for owner-set knowledge supersession (caller responsibility).
func SupersedeFact(ctx context.Context, db *sql.DB, factID, supersededByFactID string) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := db.ExecContext(ctx, `
		INSERT INTO fact_supersessions (fact_id, superseded_by_fact_id, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(fact_id) DO UPDATE SET superseded_by_fact_id = excluded.superseded_by_fact_id, created_at = excluded.created_at
	`, factID, supersededByFactID, now)
	if err != nil {
		return fmt.Errorf("store: supersede fact: %w", err)
	}
	return nil
}

// TombstoneEntity records a Tombstone (hard-delete) and removes the row from the
// corresponding table. Auto-declines pending proposals that reference this entity.
// When tombstone is executed from an approved proposal, pass proposalID so the
// tombstones row records it for audit.
func TombstoneEntity(ctx context.Context, db *sql.DB, entityType, entityID, proposalID string) error {
	id := uuid.New().String()
	now := time.Now().UTC().Format(timeFormat)
	var propID any
	if proposalID != "" {
		propID = proposalID
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO tombstones (id, entity_type, entity_id, action, proposal_id, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, entityType, entityID, string(schema.LifecycleTombstone), propID, now)
	if err != nil {
		return fmt.Errorf("store: tombstone entity: %w", err)
	}
	_, _ = AutoDeclineProposalsForEntity(ctx, db, entityIDForProposal(entityType, entityID))

	switch entityType {
	case "memory":
		_, _ = db.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, entityID)
	case "fact":
		_, _ = db.ExecContext(ctx, `DELETE FROM facts WHERE id = ?`, entityID)
	case "contact":
		_, _ = db.ExecContext(ctx, `DELETE FROM contacts WHERE id = ?`, entityID)
	case "artifact":
		_, _ = db.ExecContext(ctx, `DELETE FROM artifacts WHERE id = ?`, entityID)
	case "wm_event":
		_, _ = db.ExecContext(ctx, `DELETE FROM wm_events WHERE id = ?`, entityID)
	}
	return nil
}

// IsFactSuperseded returns the superseding fact ID if the given fact was superseded, otherwise empty.
func IsFactSuperseded(ctx context.Context, db *sql.DB, factID string) (string, error) {
	var supersededBy string
	err := db.QueryRowContext(ctx, `SELECT superseded_by_fact_id FROM fact_supersessions WHERE fact_id = ?`, factID).Scan(&supersededBy)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: is fact superseded: %w", err)
	}
	return supersededBy, nil
}
