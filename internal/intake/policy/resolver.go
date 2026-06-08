package policy

import (
	"context"
	"database/sql"

	"github.com/open-navi/navi/internal/store"
)

// Resolver produces the effective sync policy for a connector by layering, in
// precedence order: built-in Go default → config/connectors YAML → owner runtime
// override (connector_sync_policy store). Each layer overlays only its non-zero
// fields onto the one below (CIP §7: defaults are starting values; owner tuning
// wins).
type Resolver struct {
	db    *sql.DB
	files map[string]SyncPolicy
}

// NewResolver constructs a Resolver. files (from LoadDir) may be nil. db may be
// nil to disable runtime overrides (defaults/files only).
func NewResolver(db *sql.DB, files map[string]SyncPolicy) *Resolver {
	if files == nil {
		files = map[string]SyncPolicy{}
	}
	return &Resolver{db: db, files: files}
}

// Resolve returns the effective policy for a connector id. It never fails:
// store errors degrade to the file/default layer so intake is never blocked by
// a policy-read problem.
func (r *Resolver) Resolve(ctx context.Context, connectorID string) SyncPolicy {
	p := DefaultPolicy(connectorID)
	if f, ok := fileKeyFor(r.files, connectorID); ok {
		p = merge(p, f)
	}
	if r.db != nil {
		if ov, ok, err := store.GetConnectorSyncPolicyOverride(ctx, r.db, connectorID); err == nil && ok {
			p = merge(p, ov)
		}
	}
	p.ConnectorID = connectorID
	return p
}

// SaveOverride validates and persists an owner runtime override. The override is
// stored verbatim (its non-zero fields overlay the default/file layer at resolve
// time). Changing cadence/budget is configuration, not a governed mutation.
func (r *Resolver) SaveOverride(ctx context.Context, p SyncPolicy) error {
	if err := Validate(p); err != nil {
		return err
	}
	return store.SaveConnectorSyncPolicyOverride(ctx, r.db, p)
}

// ClearOverride removes an owner override, reverting to the default/file policy.
func (r *Resolver) ClearOverride(ctx context.Context, connectorID string) error {
	return store.DeleteConnectorSyncPolicyOverride(ctx, r.db, connectorID)
}
