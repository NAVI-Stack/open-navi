// Package policy implements CIP P5 per-connector sync policy (CIP §7, §7.1):
// the declarative configuration — cadence, budget, cursor strategy, dedupe rule,
// freshness window, privacy class, visibility — that governs how an intake-capable
// connector pulls data, with separate Backfill and Delta blocks.
//
// Policy is configuration, never World Model state (frozen contract): defaults
// are defined here in Go, may be overlaid from per-connector YAML under
// config/connectors/, and owner runtime overrides persist in the
// connector_sync_policy store. The Governor remains authoritative for budgets —
// this package supplies the per-connector inputs; the Governor decides.
package policy

import (
	"fmt"
	"strings"

	"github.com/ceoai/navi/internal/schema"
)

// SyncPolicy is re-exported from internal/schema so callers use policy.SyncPolicy.
type SyncPolicy = schema.SyncPolicy

// DefaultPolicy returns the conservative built-in policy for a connector id.
// Telegram (and any "telegram:*" account id) gets a webhook-triggered delta
// policy; every other connector falls back to a generic manual default. These
// are the V1 starting values per CIP §7 (illustrative, tunable), not a contract.
func DefaultPolicy(connectorID string) SyncPolicy {
	base := genericDefault(connectorID)
	switch driverOf(connectorID) {
	case "telegram":
		base.PrivacyClass = schema.PrivacyClassPersonal
		base.Delta = schema.SyncModePolicy{
			Cadence:        schema.CadenceWebhookTriggered,
			Budget:         schema.SyncBudget{MaxRecordsPerPass: 500, MaxBytesPerPass: 10 << 20, CostCeilingUSD: 0.50},
			CursorStrategy: "incremental",
			DedupeRule:     "by_source_id",
		}
		base.Backfill = schema.SyncModePolicy{
			Cadence:         schema.CadenceManual,
			Budget:          schema.SyncBudget{MaxRecordsPerPass: 2000, MaxBytesPerPass: 50 << 20, CostCeilingUSD: 5.00},
			CursorStrategy:  "incremental",
			DedupeRule:      "by_source_id",
			FreshnessWindow: "30d",
		}
	}
	return base
}

// genericDefault is the fallback policy for connectors without a tuned default.
func genericDefault(connectorID string) SyncPolicy {
	return SyncPolicy{
		ConnectorID:  connectorID,
		PrivacyClass: schema.PrivacyClassPersonal,
		Visibility:   schema.SyncVisibilityLogged,
		Delta: schema.SyncModePolicy{
			Cadence:        schema.CadenceWebhookTriggered,
			Budget:         schema.SyncBudget{MaxRecordsPerPass: 200, MaxBytesPerPass: 4 << 20, CostCeilingUSD: 0.25},
			CursorStrategy: "incremental",
			DedupeRule:     "by_source_id",
		},
		Backfill: schema.SyncModePolicy{
			Cadence:         schema.CadenceManual,
			Budget:          schema.SyncBudget{MaxRecordsPerPass: 1000, MaxBytesPerPass: 20 << 20, CostCeilingUSD: 2.00},
			CursorStrategy:  "incremental",
			DedupeRule:      "by_source_id",
			FreshnessWindow: "7d",
		},
	}
}

// driverOf returns the connector driver from a connector id like "telegram:acct-1".
func driverOf(connectorID string) string {
	if i := strings.IndexByte(connectorID, ':'); i >= 0 {
		return strings.ToLower(connectorID[:i])
	}
	return strings.ToLower(connectorID)
}

// Validate checks a policy for internal consistency. It does not enforce the
// Governor's authority (that happens at runtime); it only rejects nonsensical
// configuration the owner could otherwise save.
func Validate(p SyncPolicy) error {
	if strings.TrimSpace(p.ConnectorID) == "" {
		return fmt.Errorf("policy: missing connector_id")
	}
	if p.PrivacyClass != "" && !knownPrivacyClass(p.PrivacyClass) {
		return fmt.Errorf("policy: unknown privacy_class %q", p.PrivacyClass)
	}
	if p.Visibility != "" && p.Visibility != schema.SyncVisibilityLogged && p.Visibility != schema.SyncVisibilityHidden {
		return fmt.Errorf("policy: unknown visibility %q", p.Visibility)
	}
	if err := validateBlock("delta", p.Delta); err != nil {
		return err
	}
	if err := validateBlock("backfill", p.Backfill); err != nil {
		return err
	}
	return nil
}

func validateBlock(name string, m schema.SyncModePolicy) error {
	if m.Budget.MaxRecordsPerPass < 0 {
		return fmt.Errorf("policy: %s.max_records_per_pass must be >= 0", name)
	}
	if m.Budget.MaxBytesPerPass < 0 {
		return fmt.Errorf("policy: %s.max_bytes_per_pass must be >= 0", name)
	}
	if m.Budget.CostCeilingUSD < 0 {
		return fmt.Errorf("policy: %s.cost_ceiling_usd must be >= 0", name)
	}
	// A non-sentinel cadence must parse as a duration.
	if c := strings.TrimSpace(strings.ToLower(m.Cadence)); c != "" &&
		c != schema.CadenceManual && c != schema.CadenceWebhookTriggered {
		if _, ok := m.CadenceDuration(); !ok {
			return fmt.Errorf("policy: %s.cadence %q is not a valid interval, %q, or %q",
				name, m.Cadence, schema.CadenceManual, schema.CadenceWebhookTriggered)
		}
	}
	return nil
}

func knownPrivacyClass(c schema.PrivacyClass) bool {
	switch c {
	case schema.PrivacyClassPublic, schema.PrivacyClassPersonal,
		schema.PrivacyClassSensitive, schema.PrivacyClassSecret:
		return true
	}
	return false
}

// merge overlays the non-zero fields of override onto base. It is used to apply
// a YAML file or a runtime override on top of the Go default so an owner only
// has to specify what they want to change.
func merge(base, override SyncPolicy) SyncPolicy {
	out := base
	if override.ConnectorID != "" {
		out.ConnectorID = override.ConnectorID
	}
	if override.PrivacyClass != "" {
		out.PrivacyClass = override.PrivacyClass
	}
	if override.Visibility != "" {
		out.Visibility = override.Visibility
	}
	out.Delta = mergeBlock(base.Delta, override.Delta)
	out.Backfill = mergeBlock(base.Backfill, override.Backfill)
	return out
}

func mergeBlock(base, override schema.SyncModePolicy) schema.SyncModePolicy {
	out := base
	if override.Cadence != "" {
		out.Cadence = override.Cadence
	}
	if override.CursorStrategy != "" {
		out.CursorStrategy = override.CursorStrategy
	}
	if override.DedupeRule != "" {
		out.DedupeRule = override.DedupeRule
	}
	if override.FreshnessWindow != "" {
		out.FreshnessWindow = override.FreshnessWindow
	}
	if override.Budget.MaxRecordsPerPass != 0 {
		out.Budget.MaxRecordsPerPass = override.Budget.MaxRecordsPerPass
	}
	if override.Budget.MaxBytesPerPass != 0 {
		out.Budget.MaxBytesPerPass = override.Budget.MaxBytesPerPass
	}
	if override.Budget.CostCeilingUSD != 0 {
		out.Budget.CostCeilingUSD = override.Budget.CostCeilingUSD
	}
	return out
}
