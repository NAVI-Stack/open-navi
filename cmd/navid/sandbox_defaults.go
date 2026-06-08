package main

import (
	"context"
	"database/sql"
	"strings"

	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

const (
	defaultCoderProfileID      = "local-coder-python-network-disabled"
	defaultValidationProfileID = "local-validation-network-disabled"
)

func ensureDefaultSandboxProfiles(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	defaults := []schema.SandboxProfile{
		{
			ID:                   defaultCoderProfileID,
			Name:                 "Local Coder Python Network Disabled",
			Description:          "Default network-disabled Python sandbox for navi.coder.repo workers.",
			Runtime:              schema.SandboxRuntimeDocker,
			Status:               schema.SandboxProfileStatusActive,
			Image:                "python:3.11-slim",
			NetworkMode:          schema.SandboxNetworkNone,
			WorkspaceMountTarget: "/workspace",
			CommandAllowlist:     []string{"python", "python3"},
			EnvAllowlist:         []string{},
			DefaultTimeoutMS:     30000,
			MaxTimeoutMS:         300000,
			CPULimit:             "1.0",
			MemoryLimit:          "512m",
			CreatedBy:            "navid",
			Metadata:             `{"source":"navid_default"}`,
		},
		{
			ID:                   defaultValidationProfileID,
			Name:                 "Local Validation Network Disabled",
			Description:          "Default network-disabled validation sandbox for local build, test, lint, and smoke commands.",
			Runtime:              schema.SandboxRuntimeDocker,
			Status:               schema.SandboxProfileStatusActive,
			Image:                "golang:1.24-alpine",
			NetworkMode:          schema.SandboxNetworkNone,
			WorkspaceMountTarget: "/workspace",
			CommandAllowlist:     []string{"go", "python", "python3", "node", "npm", "pnpm", "yarn", "pytest", "ruff", "tsc", "vitest", "make"},
			EnvAllowlist:         []string{"CI", "NAVI_VALIDATION", "NO_COLOR"},
			DefaultTimeoutMS:     30000,
			MaxTimeoutMS:         600000,
			CPULimit:             "2.0",
			MemoryLimit:          "1g",
			CreatedBy:            "navid",
			Metadata:             `{"source":"navid_default"}`,
		},
	}
	for _, profile := range defaults {
		if _, err := store.GetSandboxProfile(ctx, db, profile.ID); err == nil {
			continue
		} else if !strings.Contains(err.Error(), "not found") {
			return err
		}
		if err := store.SaveSandboxProfile(ctx, db, profile); err != nil {
			return err
		}
	}
	return nil
}
