package experience

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/open-navi/navi/internal/store"
)

// ConfigKeyGovernanceBounds is the configuration key for owner-set governance bounds.
const ConfigKeyGovernanceBounds = "experience.governance_bounds.v1"

// StoredGovernanceBounds represents the JSON schema stored in the DB for owner-set persona bounds.
type StoredGovernanceBounds struct {
	TraitCaps   []GovernanceCap   `json:"trait_caps"`
	TraitFloors []GovernanceFloor `json:"trait_floors"`
	TraitGates  []GovernanceGate  `json:"trait_gates"`
}

// StoreGovernanceBoundsProvider implements GovernanceBoundsProvider by loading bounds from
// the central configuration store (owner-scope), and stacking them on top of the
// contextual base bounds derived from the live running context.
type StoreGovernanceBoundsProvider struct {
	db *sql.DB
}

// NewStoreGovernanceBoundsProvider creates a new StoreGovernanceBoundsProvider.
// If db is nil, it gracefully falls back to just contextual bounds.
func NewStoreGovernanceBoundsProvider(db *sql.DB) StoreGovernanceBoundsProvider {
	return StoreGovernanceBoundsProvider{
		db: db,
	}
}

// Bounds returns the merged governance bounds (contextual + owner-set).
func (p StoreGovernanceBoundsProvider) Bounds(ctx context.Context, req BoundsRequest) GovernanceBounds {
	// 1. Derive contextual bounds (the legacy stub behavior is our baseline context behavior).
	bounds := DefaultGovernanceBoundsProvider{}.Bounds(ctx, req)

	if p.db == nil {
		return bounds // Return base contextual bounds if no DB
	}

	ownerID, err := store.GetOwnerID(ctx, p.db)
	if err != nil || strings.TrimSpace(ownerID) == "" {
		return bounds
	}

	// 2. Load owner-set overrides from store
	val, ok, err := store.GetConfigurationValue(ctx, p.db, ConfigScopeOwner, ownerID, ConfigKeyGovernanceBounds)
	if err != nil || !ok || strings.TrimSpace(val) == "" {
		return bounds
	}

	var stored StoredGovernanceBounds
	if err := json.Unmarshal([]byte(val), &stored); err != nil {
		// Log error in production, but fail open to base bounds to not break experience
		return bounds
	}

	// 3. Stacking: Owner bounds over contextual bounds
	if len(stored.TraitCaps) > 0 {
		bounds.TraitCaps = append(bounds.TraitCaps, stored.TraitCaps...)
	}
	if len(stored.TraitFloors) > 0 {
		bounds.TraitFloors = append(bounds.TraitFloors, stored.TraitFloors...)
	}
	if len(stored.TraitGates) > 0 {
		bounds.TraitGates = append(bounds.TraitGates, stored.TraitGates...)
	}

	return bounds
}
