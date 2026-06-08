package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	naviproposals "github.com/open-navi/navi/internal/navi/proposals"
	"github.com/open-navi/navi/internal/schema"
)

// PrepareProposalFromReflection builds a proposal for Subconscious-originated changes
// (consolidation or deep_reflection) that touch explicit/owner-set state. Caller must
// call SaveProposal to persist. sourceProcess must be "consolidation" or "deep_reflection".
func PrepareProposalFromReflection(sourceProcess, sourceTrigger, proposedAction, affectedEntities, rationale string) (schema.Proposal, error) {
	return naviproposals.BuildReflection(sourceProcess, sourceTrigger, proposedAction, affectedEntities, rationale)
}

// SaveProposal inserts or updates a proposal record.
func SaveProposal(ctx context.Context, db *sql.DB, p schema.Proposal) error {
	if p.ProposalID == "" {
		p.ProposalID = uuid.New().String()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	// ResolvedAt / ExpiresAt are controlled by callers; nil is allowed.
	var expiresAt, resolvedAt *string
	if p.ExpiresAt != nil {
		s := p.ExpiresAt.UTC().Format(timeFormat)
		expiresAt = &s
	}
	if p.ResolvedAt != nil {
		s := p.ResolvedAt.UTC().Format(timeFormat)
		resolvedAt = &s
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO proposals (
			proposal_id, source_process, source_trigger, boundary_key, proposed_action,
			affected_entities, rationale, priority, status,
			created_at, expires_at, resolved_at, resolved_by, resolution_type, resolution_note
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(proposal_id) DO UPDATE SET
			source_process=excluded.source_process,
			source_trigger=excluded.source_trigger,
			boundary_key=excluded.boundary_key,
			proposed_action=excluded.proposed_action,
			affected_entities=excluded.affected_entities,
			rationale=excluded.rationale,
			priority=excluded.priority,
			status=excluded.status,
			expires_at=excluded.expires_at,
			resolved_at=excluded.resolved_at,
			resolved_by=excluded.resolved_by,
			resolution_type=excluded.resolution_type,
			resolution_note=excluded.resolution_note
	`, p.ProposalID, p.SourceProcess, p.SourceTrigger, proposalNullIfEmpty(p.BoundaryKey), p.ProposedAction,
		p.AffectedEntities, p.Rationale, string(p.Priority), string(p.Status),
		p.CreatedAt.UTC().Format(timeFormat),
		expiresAtOrNil(expiresAt), resolvedAtOrNil(resolvedAt),
		p.ResolvedBy, string(p.ResolutionType), p.ResolutionNote)
	if err != nil {
		return fmt.Errorf("store: save proposal: %w", err)
	}

	if p.Status == schema.ProposalStatusPending {
		_, _ = SupersedeOldProposals(ctx, db, p.ProposedAction, p.ProposalID)
	}

	return nil
}

func expiresAtOrNil(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func resolvedAtOrNil(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// GetProposal returns a single proposal by id.
func GetProposal(ctx context.Context, db *sql.DB, proposalID string) (schema.Proposal, error) {
	row := db.QueryRowContext(ctx, `
		SELECT proposal_id, source_process, source_trigger, boundary_key, proposed_action,
		       affected_entities, rationale, priority, status,
		       created_at, expires_at, resolved_at, resolved_by, resolution_type, resolution_note
		FROM proposals
		WHERE proposal_id = ?
	`, proposalID)

	var (
		p              schema.Proposal
		createdAt      string
		expiresAt      sql.NullString
		resolvedAt     sql.NullString
		resolvedBy     sql.NullString
		resolutionType sql.NullString
		resolution     sql.NullString
		boundaryKey    sql.NullString
		priority       string
		status         string
	)

	if err := row.Scan(
		&p.ProposalID, &p.SourceProcess, &p.SourceTrigger, &boundaryKey, &p.ProposedAction,
		&p.AffectedEntities, &p.Rationale, &priority, &status,
		&createdAt, &expiresAt, &resolvedAt, &resolvedBy, &resolutionType, &resolution,
	); err != nil {
		if err == sql.ErrNoRows {
			return schema.Proposal{}, fmt.Errorf("store: proposal not found: %w", err)
		}
		return schema.Proposal{}, fmt.Errorf("store: get proposal: %w", err)
	}
	if boundaryKey.Valid {
		p.BoundaryKey = boundaryKey.String
	}

	p.Priority = schema.ProposalPriority(priority)
	p.Status = schema.ProposalStatus(status)
	p.CreatedAt, _ = parseTime(createdAt)
	if expiresAt.Valid {
		t, _ := parseTime(expiresAt.String)
		p.ExpiresAt = &t
	}
	if resolvedAt.Valid {
		t, _ := parseTime(resolvedAt.String)
		p.ResolvedAt = &t
	}
	if resolvedBy.Valid {
		p.ResolvedBy = resolvedBy.String
	}
	if resolutionType.Valid {
		p.ResolutionType = schema.ResolutionType(resolutionType.String)
	}
	if resolution.Valid {
		p.ResolutionNote = resolution.String
	}

	return p, nil
}

// ListPendingProposals returns pending proposals ordered by priority and created_at.
func ListPendingProposals(ctx context.Context, db *sql.DB, limit int) ([]schema.Proposal, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT proposal_id, source_process, source_trigger, boundary_key, proposed_action,
		       affected_entities, rationale, priority, status,
		       created_at, expires_at, resolved_at, resolved_by, resolution_type, resolution_note
		FROM proposals
		WHERE status = 'pending'
		ORDER BY
			CASE priority WHEN 'blocking' THEN 0 ELSE 1 END,
			created_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list pending proposals: %w", err)
	}
	defer rows.Close()

	var out []schema.Proposal
	for rows.Next() {
		var (
			p              schema.Proposal
			createdAt      string
			expiresAt      sql.NullString
			resolvedAt     sql.NullString
			resolvedBy     sql.NullString
			resolutionType sql.NullString
			resolution     sql.NullString
			boundaryKey    sql.NullString
			priority       string
			status         string
		)
		if err := rows.Scan(
			&p.ProposalID, &p.SourceProcess, &p.SourceTrigger, &boundaryKey, &p.ProposedAction,
			&p.AffectedEntities, &p.Rationale, &priority, &status,
			&createdAt, &expiresAt, &resolvedAt, &resolvedBy, &resolutionType, &resolution,
		); err != nil {
			return nil, fmt.Errorf("store: scan proposal: %w", err)
		}
		p.Priority = schema.ProposalPriority(priority)
		p.Status = schema.ProposalStatus(status)
		p.CreatedAt, _ = parseTime(createdAt)
		if expiresAt.Valid {
			t, _ := parseTime(expiresAt.String)
			p.ExpiresAt = &t
		}
		if resolvedAt.Valid {
			t, _ := parseTime(resolvedAt.String)
			p.ResolvedAt = &t
		}
		if resolvedBy.Valid {
			p.ResolvedBy = resolvedBy.String
		}
		if resolutionType.Valid {
			p.ResolutionType = schema.ResolutionType(resolutionType.String)
		}
		if resolution.Valid {
			p.ResolutionNote = resolution.String
		}
		if boundaryKey.Valid {
			p.BoundaryKey = boundaryKey.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPendingProposalByBoundaryKey returns the open proposal for a stable
// approval boundary, if one exists.
func GetPendingProposalByBoundaryKey(ctx context.Context, db *sql.DB, boundaryKey string) (schema.Proposal, error) {
	boundaryKey = strings.TrimSpace(boundaryKey)
	if boundaryKey == "" {
		return schema.Proposal{}, nil
	}
	row := db.QueryRowContext(ctx, `
		SELECT proposal_id
		FROM proposals
		WHERE boundary_key = ? AND status = 'pending'
		ORDER BY created_at DESC
		LIMIT 1
	`, boundaryKey)
	var proposalID string
	if err := row.Scan(&proposalID); err != nil {
		if err == sql.ErrNoRows {
			return schema.Proposal{}, nil
		}
		return schema.Proposal{}, fmt.Errorf("store: get pending proposal by boundary key: %w", err)
	}
	return GetProposal(ctx, db, proposalID)
}

// GetLatestProposalByBoundaryKey returns the most recent proposal for a stable
// boundary key regardless of status (pending, approved, declined, …). Used by
// backfill consent (CIP §7.1) to read whether the owner has approved or rejected
// a connector's historical sweep. Returns ok=false when none exists.
func GetLatestProposalByBoundaryKey(ctx context.Context, db *sql.DB, boundaryKey string) (schema.Proposal, bool, error) {
	boundaryKey = strings.TrimSpace(boundaryKey)
	if boundaryKey == "" {
		return schema.Proposal{}, false, nil
	}
	row := db.QueryRowContext(ctx, `
		SELECT proposal_id
		FROM proposals
		WHERE boundary_key = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, boundaryKey)
	var proposalID string
	if err := row.Scan(&proposalID); err != nil {
		if err == sql.ErrNoRows {
			return schema.Proposal{}, false, nil
		}
		return schema.Proposal{}, false, fmt.Errorf("store: get latest proposal by boundary key: %w", err)
	}
	p, err := GetProposal(ctx, db, proposalID)
	if err != nil {
		return schema.Proposal{}, false, err
	}
	return p, true, nil
}

func proposalNullIfEmpty(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

// ResolveProposal updates a proposal's status to approved, declined, expired, or superseded,
// and sets resolved_at, resolved_by, resolution_type, and resolution_note.
func ResolveProposal(ctx context.Context, db *sql.DB, proposalID string, status schema.ProposalStatus, resolutionType schema.ResolutionType, resolvedBy, resolutionNote string) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE proposals
		SET status = ?, resolved_at = ?, resolved_by = ?, resolution_type = ?, resolution_note = ?
		WHERE proposal_id = ? AND status = 'pending'
	`, string(status), now.Format(timeFormat), resolvedBy, string(resolutionType), resolutionNote, proposalID)
	if err != nil {
		return fmt.Errorf("store: resolve proposal: %w", err)
	}
	return nil
}

// AutoDeclineProposalsForEntity marks all pending proposals whose affected_entities
// list contains the given entityID as declined with a system resolution note.
// Call this when an entity is archived or tombstoned (proposal cascade).
func AutoDeclineProposalsForEntity(ctx context.Context, db *sql.DB, entityID string) (int, error) {
	list, err := ListPendingProposals(ctx, db, 500)
	if err != nil {
		return 0, err
	}
	var count int
	for _, p := range list {
		if !affectedEntitiesContains(p.AffectedEntities, entityID) {
			continue
		}
		if err := ResolveProposal(ctx, db, p.ProposalID, schema.ProposalStatusDeclined, schema.ResolutionTypeDenied, "system", "Target entity archived or tombstoned"); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// affectedEntitiesContains parses affected_entities (JSON array string) and returns true if it contains id.
func affectedEntitiesContains(affectedEntities, id string) bool {
	if id == "" {
		return false
	}
	var ids []string
	if err := json.Unmarshal([]byte(affectedEntities), &ids); err != nil {
		return strings.Contains(affectedEntities, id)
	}
	for _, e := range ids {
		if e == id {
			return true
		}
	}
	return false
}

// ExpireProposals marks all pending proposals whose expires_at is in the past as expired.
func ExpireProposals(ctx context.Context, db *sql.DB) (int, error) {
	nowT := time.Now().UTC()
	now := nowT.Format(timeFormat)
	res, err := db.ExecContext(ctx, `
		UPDATE proposals
		SET status = 'expired', resolved_at = ?, resolved_by = 'system', resolution_type = 'na', resolution_note = 'Proposal expired'
		WHERE status = 'pending' AND expires_at IS NOT NULL AND expires_at < ?
	`, now, now)
	if err != nil {
		return 0, fmt.Errorf("store: expire proposals: %w", err)
	}
	rows, _ := res.RowsAffected()
	return int(rows), nil
}

// SupersedeOldProposals marks all pending proposals for the same proposed_action as superseded.
// Call this when a new proposal is created to ensure only the latest version of an action is pending.
func SupersedeOldProposals(ctx context.Context, db *sql.DB, proposedAction, excludeProposalID string) (int, error) {
	nowT := time.Now().UTC()
	now := nowT.Format(timeFormat)
	res, err := db.ExecContext(ctx, `
		UPDATE proposals
		SET status = 'superseded', resolved_at = ?, resolved_by = 'system', resolution_type = 'na', resolution_note = 'Superseded by newer proposal'
		WHERE status = 'pending' AND proposed_action = ? AND proposal_id != ?
	`, now, proposedAction, excludeProposalID)
	if err != nil {
		return 0, fmt.Errorf("store: supersede old proposals: %w", err)
	}
	rows, _ := res.RowsAffected()
	return int(rows), nil
}
