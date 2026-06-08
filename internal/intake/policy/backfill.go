package policy

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
	"github.com/google/uuid"
)

// CIP P5 — Backfill consent (CIP §7.1, §12; synthesis seam §12).
//
// A Backfill pass ingests a historical window — a deliberate act — so it does
// not start until the owner approves a consent Proposal raised in the ONE
// existing Proposal queue (no second queue, no second authority). Delta passes
// are pre-authorized by the standing sync policy and need no consent.
//
// source_process is "intake_consent" so the Console renders the source tag
// `intake-consent` on the Proposal.

// SourceProcessConsent is the proposal source_process for backfill consent.
const SourceProcessConsent = "intake_consent"

// consentBoundaryKey is the stable approval boundary for a connector's backfill
// consent, so a re-request reuses the open Proposal instead of duplicating it.
func consentBoundaryKey(connectorID string) string {
	return "intake-backfill-consent:" + connectorID
}

func consentAction(connectorID string) string {
	return "intake_backfill_consent:" + connectorID
}

// RequestBackfill raises (or reuses) a backfill consent Proposal for a connector
// and returns its id. resume is true when the connector already has historical
// state: the Proposal then offers a resume-from-cursor sweep rather than a full
// re-sweep (CIP §7.1). The pass does NOT start here — approval triggers it; see
// StartApprovedBackfill.
func RequestBackfill(ctx context.Context, db *sql.DB, connectorID string, p SyncPolicy) (proposalID string, resume bool, err error) {
	boundary := consentBoundaryKey(connectorID)
	if existing, err := store.GetPendingProposalByBoundaryKey(ctx, db, boundary); err == nil && existing.ProposalID != "" {
		// A consent Proposal is already open for this connector.
		return existing.ProposalID, false, nil
	}

	count, err := store.CountIntakeRecordsByConnector(ctx, db, connectorID)
	if err != nil {
		return "", false, err
	}
	resume = count > 0
	var cursor string
	if resume {
		cursor, _ = store.LatestCursorByConnector(ctx, db, connectorID)
	}

	prop := schema.Proposal{
		ProposalID:       uuid.NewString(),
		SourceProcess:    SourceProcessConsent,
		SourceTrigger:    connectorID + ":backfill",
		BoundaryKey:      boundary,
		ProposedAction:   consentAction(connectorID),
		AffectedEntities: "[]",
		Rationale:        consentRationale(connectorID, p, resume, cursor),
		Priority:         schema.ProposalPriorityQueued,
		Status:           schema.ProposalStatusPending,
	}
	if err := store.SaveProposal(ctx, db, prop); err != nil {
		return "", resume, fmt.Errorf("policy: raise backfill consent proposal: %w", err)
	}
	return prop.ProposalID, resume, nil
}

func consentRationale(connectorID string, p SyncPolicy, resume bool, cursor string) string {
	b := p.Backfill.Budget
	window := p.Backfill.FreshnessWindow
	if window == "" {
		window = "all history"
	}
	mode := "full historical sweep"
	if resume {
		mode = "resume from cursor"
		if cursor != "" {
			mode += " (" + cursor + ")"
		}
	}
	return fmt.Sprintf(
		"Backfill %s for %s — scope: %s; budget: %d records / %d bytes / $%.2f. Approve to start the historical sweep; no consent means no historical ingestion.",
		mode, connectorID, window, b.MaxRecordsPerPass, b.MaxBytesPerPass, b.CostCeilingUSD)
}

// ConsentState reports the current backfill consent disposition for a connector:
// the Proposal status ("" when none has been raised) and the Proposal id.
func ConsentState(ctx context.Context, db *sql.DB, connectorID string) (schema.ProposalStatus, string, error) {
	prop, ok, err := store.GetLatestProposalByBoundaryKey(ctx, db, consentBoundaryKey(connectorID))
	if err != nil {
		return "", "", err
	}
	if !ok {
		return "", "", nil
	}
	return prop.Status, prop.ProposalID, nil
}

// StartApprovedBackfill is the gate that decides whether a backfill pass may run.
// It returns allowed=true only when the owner has approved the consent Proposal.
// On a declined Proposal it appends an intake_sync_log row with terminal status
// consent_rejected and returns allowed=false. A pending/absent Proposal also
// yields allowed=false (status reported for the caller to surface).
func StartApprovedBackfill(ctx context.Context, db *sql.DB, connectorID string) (allowed bool, status schema.ProposalStatus, err error) {
	status, _, err = ConsentState(ctx, db, connectorID)
	if err != nil {
		return false, "", err
	}
	switch status {
	case schema.ProposalStatusApproved:
		return true, status, nil
	case schema.ProposalStatusDeclined:
		now := time.Now().UTC()
		_, _ = store.AppendIntakeSyncLog(ctx, db, schema.IntakeSyncLogEntry{
			ConnectorID:    connectorID,
			JobMode:        schema.JobModeBackfill,
			StartedAt:      now,
			EndedAt:        &now,
			TerminalStatus: schema.SyncStatusConsentRejected,
			Note:           "backfill consent declined by owner",
		})
		return false, status, nil
	default:
		return false, status, nil
	}
}
