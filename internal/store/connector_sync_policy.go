package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

// CIP P5 — connector_sync_policy store (raw SQL, no ORM).
//
// This is the runtime-override surface for per-connector sync policy (CIP §7).
// Defaults live in code / config/connectors YAML; this table holds only the
// owner's runtime edits (cadence, budget, privacy class, …). The stored value is
// the full SyncPolicy as JSON, keyed by connector id. Policy is configuration,
// not World Model state — it deliberately does not live in the entity tables
// (frozen contract).

// SaveConnectorSyncPolicyOverride upserts the owner's override for a connector.
func SaveConnectorSyncPolicyOverride(ctx context.Context, db *sql.DB, p schema.SyncPolicy) error {
	if p.ConnectorID == "" {
		return fmt.Errorf("store: connector sync policy: missing connector_id")
	}
	blob, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("store: connector sync policy: marshal: %w", err)
	}
	now := time.Now().UTC().Format(timeFormat)
	_, err = db.ExecContext(ctx, `
		INSERT INTO connector_sync_policy (connector_id, policy, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(connector_id) DO UPDATE SET
			policy=excluded.policy,
			updated_at=excluded.updated_at
	`, p.ConnectorID, string(blob), now, now)
	if err != nil {
		return fmt.Errorf("store: save connector sync policy: %w", err)
	}
	return nil
}

// GetConnectorSyncPolicyOverride returns the owner's override for a connector.
// ok is false when no override has been saved (the caller should fall back to
// the default / file policy).
func GetConnectorSyncPolicyOverride(ctx context.Context, db *sql.DB, connectorID string) (schema.SyncPolicy, bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT policy FROM connector_sync_policy WHERE connector_id = ?
	`, connectorID)
	var blob string
	if err := row.Scan(&blob); err != nil {
		if err == sql.ErrNoRows {
			return schema.SyncPolicy{}, false, nil
		}
		return schema.SyncPolicy{}, false, fmt.Errorf("store: get connector sync policy: %w", err)
	}
	var p schema.SyncPolicy
	if err := json.Unmarshal([]byte(blob), &p); err != nil {
		return schema.SyncPolicy{}, false, fmt.Errorf("store: unmarshal connector sync policy: %w", err)
	}
	return p, true, nil
}

// ListConnectorSyncPolicyOverrides returns all stored overrides keyed by connector id.
func ListConnectorSyncPolicyOverrides(ctx context.Context, db *sql.DB) (map[string]schema.SyncPolicy, error) {
	rows, err := db.QueryContext(ctx, `SELECT connector_id, policy FROM connector_sync_policy`)
	if err != nil {
		return nil, fmt.Errorf("store: list connector sync policies: %w", err)
	}
	defer rows.Close()
	out := map[string]schema.SyncPolicy{}
	for rows.Next() {
		var id, blob string
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, fmt.Errorf("store: scan connector sync policy: %w", err)
		}
		var p schema.SyncPolicy
		if err := json.Unmarshal([]byte(blob), &p); err != nil {
			return nil, fmt.Errorf("store: unmarshal connector sync policy %s: %w", id, err)
		}
		out[id] = p
	}
	return out, rows.Err()
}

// DeleteConnectorSyncPolicyOverride removes an override, reverting the connector
// to its default / file policy.
func DeleteConnectorSyncPolicyOverride(ctx context.Context, db *sql.DB, connectorID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM connector_sync_policy WHERE connector_id = ?`, connectorID)
	if err != nil {
		return fmt.Errorf("store: delete connector sync policy: %w", err)
	}
	return nil
}
