package schema

import (
	"strings"
	"time"
)

// CIP P5 — Per-connector sync policy (CIP §7, §7.1).
//
// A SyncPolicy is *configuration*, not World Model state (it is never stored in
// the entity tables). It declares how an intake-capable connector pulls data,
// with separate Backfill and Delta blocks per CIP §7.1. Defaults live in
// internal/intake/policy; owner overrides persist in the connector_sync_policy
// store. The Governor remains authoritative for budgets — the policy only
// supplies the per-connector inputs (CIP §7, §12).

// SyncVisibility controls whether a connector's passes appear in the
// owner-visible sync log.
type SyncVisibility string

const (
	// SyncVisibilityLogged surfaces passes in the Console sync log (default).
	SyncVisibilityLogged SyncVisibility = "logged"
	// SyncVisibilityHidden suppresses a connector's passes from the sync log.
	SyncVisibilityHidden SyncVisibility = "hidden"
)

// Cadence sentinel values (CIP §7): a connector may pull on an interval, only
// on explicit owner action, or only when a webhook fires.
const (
	CadenceManual           = "manual"
	CadenceWebhookTriggered = "webhook-triggered"
)

// SyncBudget bounds a single pass and the per-connector cost ceiling the
// Governor enforces (CIP §7). MaxBytesPerPass is in bytes.
type SyncBudget struct {
	MaxRecordsPerPass int     `yaml:"max_records_per_pass" json:"max_records_per_pass"`
	MaxBytesPerPass   int64   `yaml:"max_bytes_per_pass" json:"max_bytes_per_pass"`
	CostCeilingUSD    float64 `yaml:"cost_ceiling_usd" json:"cost_ceiling_usd"`
}

// SyncModePolicy is one of the two job-mode blocks (CIP §7.1). Backfill and
// Delta share the same shape but carry independent budgets and cadence.
type SyncModePolicy struct {
	// Cadence is an interval ("20m"), CadenceManual, or CadenceWebhookTriggered.
	Cadence string `yaml:"cadence" json:"cadence"`
	// Budget bounds records/bytes/cost for this mode.
	Budget SyncBudget `yaml:"budget" json:"budget"`
	// CursorStrategy describes how resume position is tracked (e.g. "incremental").
	CursorStrategy string `yaml:"cursor" json:"cursor"`
	// DedupeRule names the dedupe key (e.g. "by_source_id").
	DedupeRule string `yaml:"dedupe" json:"dedupe"`
	// FreshnessWindow is how far back a backfill reaches ("7d"); ignored for delta.
	FreshnessWindow string `yaml:"freshness" json:"freshness"`
}

// SyncPolicy is the per-connector intake policy (CIP §7). Delta governs
// steady-state upkeep; Backfill governs cold-start / explicit re-sync passes and
// requires owner consent before it runs (CIP §7.1).
type SyncPolicy struct {
	ConnectorID  string         `yaml:"connector_id" json:"connector_id"`
	PrivacyClass PrivacyClass   `yaml:"privacy_class" json:"privacy_class"`
	Visibility   SyncVisibility `yaml:"visibility" json:"visibility"`
	Delta        SyncModePolicy `yaml:"delta" json:"delta"`
	Backfill     SyncModePolicy `yaml:"backfill" json:"backfill"`
}

// Block returns the mode block for the given job mode (Backfill for
// JobModeBackfill, Delta otherwise).
func (p SyncPolicy) Block(mode JobMode) SyncModePolicy {
	if mode == JobModeBackfill {
		return p.Backfill
	}
	return p.Delta
}

// CadenceDuration parses a mode's cadence into a duration. ok is false when the
// cadence is manual, webhook-triggered, empty, or unparseable — i.e. there is no
// scheduled interval and the worker should not gate on elapsed time.
func (m SyncModePolicy) CadenceDuration() (d time.Duration, ok bool) {
	c := strings.TrimSpace(strings.ToLower(m.Cadence))
	if c == "" || c == CadenceManual || c == CadenceWebhookTriggered {
		return 0, false
	}
	parsed, err := time.ParseDuration(c)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

// FreshnessDuration parses a backfill freshness window ("7d", "720h"). The "d"
// suffix is expanded to hours since time.ParseDuration has no day unit.
func (m SyncModePolicy) FreshnessDuration() (time.Duration, bool) {
	f := strings.TrimSpace(strings.ToLower(m.FreshnessWindow))
	if f == "" {
		return 0, false
	}
	if strings.HasSuffix(f, "d") {
		f = strings.TrimSuffix(f, "d")
		// reuse hours: 1d = 24h
		if hours, err := time.ParseDuration(f + "h"); err == nil {
			return hours * 24, true
		}
		return 0, false
	}
	d, err := time.ParseDuration(f)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}
