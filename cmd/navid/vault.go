package main

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/config"
	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/vault"
)

// startVaultWorker constructs and starts the Memory Vault worker. The Vault root
// defaults to <workspace>/vault. Owner edits flow through governor.EvaluateMutation
// with autonomy resolved by cfg.Autonomy — identical authority to intake
// synthesis (the Vault is a new caller of the same seam, not a new authority).
func startVaultWorker(ctx context.Context, cfg *config.Config, db *sql.DB) (*vault.Worker, error) {
	root := strings.TrimSpace(cfg.Vault.Dir)
	if root == "" {
		root = filepath.Join(cfg.Navi.WorkspaceDir, "vault")
	}
	debounce := parseDurationOr(cfg.Vault.Debounce, 750*time.Millisecond)
	drift := parseDurationOr(cfg.Vault.DriftInterval, 24*time.Hour)

	w, err := vault.New(vault.Config{
		DB:            db,
		Root:          root,
		GovernorOpts:  governor.MutationPipelineOptions{Resolver: &cfg.Autonomy},
		PrivacyMode:   llm.PrivacyMode(strings.ToLower(strings.TrimSpace(cfg.Vault.PrivacyMode))),
		Debounce:      debounce,
		DriftInterval: drift,
		Logger:        slog.Default(),
	})
	if err != nil {
		return nil, err
	}
	if err := w.Start(ctx); err != nil {
		return nil, err
	}
	slog.Info("vault: Memory Vault started", "root", root, "debounce", debounce, "drift_interval", drift)
	return w, nil
}

func parseDurationOr(s string, def time.Duration) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
