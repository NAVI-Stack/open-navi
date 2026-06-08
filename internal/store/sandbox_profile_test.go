package store

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestSandboxProfileCRUD(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)

	profile := schema.SandboxProfile{
		ID:                   "local-docker-default",
		Name:                 "Local Docker Default",
		Runtime:              schema.SandboxRuntimeDocker,
		Status:               schema.SandboxProfileStatusActive,
		Image:                "golang:1.22",
		NetworkMode:          schema.SandboxNetworkNone,
		WorkspaceMountTarget: "/workspace",
		CommandAllowlist:     []string{"go", "go", "npm"},
		EnvAllowlist:         []string{"CI"},
		DefaultTimeoutMS:     30000,
		MaxTimeoutMS:         120000,
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            "owner-1",
		Metadata:             "{}",
	}
	if err := SaveSandboxProfile(ctx, db, profile); err != nil {
		t.Fatalf("SaveSandboxProfile: %v", err)
	}
	got, err := GetSandboxProfile(ctx, db, profile.ID)
	if err != nil {
		t.Fatalf("GetSandboxProfile: %v", err)
	}
	if got.ID != profile.ID || got.Status != schema.SandboxProfileStatusActive || got.NetworkMode != schema.SandboxNetworkNone {
		t.Fatalf("unexpected profile: %#v", got)
	}
	if len(got.CommandAllowlist) != 2 {
		t.Fatalf("expected deduplicated command allowlist, got %#v", got.CommandAllowlist)
	}

	got.Status = schema.SandboxProfileStatusInactive
	got.UpdatedAt = now.Add(time.Minute)
	if err := SaveSandboxProfile(ctx, db, got); err != nil {
		t.Fatalf("SaveSandboxProfile(update): %v", err)
	}
	inactive, err := ListSandboxProfiles(ctx, db, schema.SandboxProfileStatusInactive)
	if err != nil {
		t.Fatalf("ListSandboxProfiles(inactive): %v", err)
	}
	if len(inactive) != 1 || inactive[0].ID != profile.ID {
		t.Fatalf("expected inactive profile in list, got %#v", inactive)
	}
}

func TestSandboxProfileValidation(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	err := SaveSandboxProfile(ctx, db, schema.SandboxProfile{
		ID:        "bad",
		Name:      "Bad",
		Runtime:   schema.SandboxRuntimeDocker,
		Status:    schema.SandboxProfileStatusActive,
		Image:     "",
		CreatedBy: "owner-1",
	})
	if err == nil || !IsValidationError(err) {
		t.Fatalf("expected validation error for missing image, got %v", err)
	}
}
