package worldmodel

import (
	"context"
	"fmt"
	"strings"

	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

// UpsertSandboxProfile inserts or updates a sandbox profile through the World
// Model boundary.
func (wm *WorldModel) UpsertSandboxProfile(ctx context.Context, profile schema.SandboxProfile) (schema.SandboxProfile, error) {
	if strings.TrimSpace(profile.ID) == "" {
		return schema.SandboxProfile{}, fmt.Errorf("worldmodel: sandbox profile id required")
	}
	if err := store.SaveSandboxProfile(ctx, wm.db, profile); err != nil {
		return schema.SandboxProfile{}, err
	}
	return store.GetSandboxProfile(ctx, wm.db, profile.ID)
}

// GetSandboxProfile returns one sandbox profile.
func (wm *WorldModel) GetSandboxProfile(ctx context.Context, id string) (schema.SandboxProfile, error) {
	return store.GetSandboxProfile(ctx, wm.db, id)
}

// ListSandboxProfiles returns sandbox profiles, optionally filtered by status.
func (wm *WorldModel) ListSandboxProfiles(ctx context.Context, status schema.SandboxProfileStatus) ([]schema.SandboxProfile, error) {
	return store.ListSandboxProfiles(ctx, wm.db, status)
}
