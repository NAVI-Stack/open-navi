package policy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestDefaultPolicy_Telegram(t *testing.T) {
	p := DefaultPolicy("telegram:acct-1")
	if p.PrivacyClass != schema.PrivacyClassPersonal {
		t.Errorf("privacy_class: want personal, got %q", p.PrivacyClass)
	}
	if p.Delta.Cadence != schema.CadenceWebhookTriggered {
		t.Errorf("delta cadence: want webhook-triggered, got %q", p.Delta.Cadence)
	}
	if p.Backfill.Cadence != schema.CadenceManual {
		t.Errorf("backfill cadence: want manual, got %q", p.Backfill.Cadence)
	}
	if p.Delta.Budget.MaxRecordsPerPass == 0 || p.Backfill.Budget.MaxRecordsPerPass == 0 {
		t.Error("expected non-zero per-pass record budgets")
	}
	if p.Backfill.FreshnessWindow == "" {
		t.Error("expected a backfill freshness window")
	}
}

func TestDefaultPolicy_Generic(t *testing.T) {
	p := DefaultPolicy("slack:team-1")
	if p.ConnectorID != "slack:team-1" {
		t.Errorf("connector_id: %q", p.ConnectorID)
	}
	if p.Visibility != schema.SyncVisibilityLogged {
		t.Errorf("visibility: want logged, got %q", p.Visibility)
	}
}

func TestBlock(t *testing.T) {
	p := DefaultPolicy("telegram")
	if got := p.Block(schema.JobModeBackfill); got.Cadence != p.Backfill.Cadence {
		t.Error("Block(backfill) should return backfill block")
	}
	if got := p.Block(schema.JobModeDelta); got.Cadence != p.Delta.Cadence {
		t.Error("Block(delta) should return delta block")
	}
}

func TestCadenceDuration(t *testing.T) {
	cases := []struct {
		cadence string
		wantOK  bool
		wantD   time.Duration
	}{
		{"20m", true, 20 * time.Minute},
		{"1h", true, time.Hour},
		{"webhook-triggered", false, 0},
		{"manual", false, 0},
		{"", false, 0},
		{"garbage", false, 0},
	}
	for _, c := range cases {
		m := schema.SyncModePolicy{Cadence: c.cadence}
		d, ok := m.CadenceDuration()
		if ok != c.wantOK || (ok && d != c.wantD) {
			t.Errorf("cadence %q: got (%v,%v), want (%v,%v)", c.cadence, d, ok, c.wantD, c.wantOK)
		}
	}
}

func TestFreshnessDuration(t *testing.T) {
	m := schema.SyncModePolicy{FreshnessWindow: "7d"}
	d, ok := m.FreshnessDuration()
	if !ok || d != 7*24*time.Hour {
		t.Errorf("7d: got (%v,%v)", d, ok)
	}
	m2 := schema.SyncModePolicy{FreshnessWindow: "12h"}
	if d, ok := m2.FreshnessDuration(); !ok || d != 12*time.Hour {
		t.Errorf("12h: got (%v,%v)", d, ok)
	}
}

func TestValidate(t *testing.T) {
	ok := DefaultPolicy("telegram")
	if err := Validate(ok); err != nil {
		t.Fatalf("default policy should validate: %v", err)
	}

	bad := DefaultPolicy("telegram")
	bad.ConnectorID = ""
	if err := Validate(bad); err == nil {
		t.Error("expected error for missing connector_id")
	}

	badPriv := DefaultPolicy("telegram")
	badPriv.PrivacyClass = "nonsense"
	if err := Validate(badPriv); err == nil {
		t.Error("expected error for unknown privacy_class")
	}

	badCadence := DefaultPolicy("telegram")
	badCadence.Delta.Cadence = "not-a-duration"
	if err := Validate(badCadence); err == nil {
		t.Error("expected error for invalid cadence")
	}

	negBudget := DefaultPolicy("telegram")
	negBudget.Delta.Budget.MaxRecordsPerPass = -1
	if err := Validate(negBudget); err == nil {
		t.Error("expected error for negative budget")
	}
}

func TestMerge_OverlaysNonZeroOnly(t *testing.T) {
	base := DefaultPolicy("telegram")
	override := schema.SyncPolicy{
		PrivacyClass: schema.PrivacyClassSensitive,
		Delta:        schema.SyncModePolicy{Cadence: "30m"},
	}
	got := merge(base, override)
	if got.PrivacyClass != schema.PrivacyClassSensitive {
		t.Errorf("privacy not overlaid: %q", got.PrivacyClass)
	}
	if got.Delta.Cadence != "30m" {
		t.Errorf("cadence not overlaid: %q", got.Delta.Cadence)
	}
	// Untouched fields preserved from base.
	if got.Delta.Budget.MaxRecordsPerPass != base.Delta.Budget.MaxRecordsPerPass {
		t.Error("budget should be preserved from base when override is zero")
	}
	if got.Backfill.Cadence != base.Backfill.Cadence {
		t.Error("backfill block should be preserved when override is zero")
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	yaml := `connector_id: telegram
privacy_class: sensitive
delta:
  cadence: 45m
  budget:
    max_records_per_pass: 123
`
	if err := os.WriteFile(filepath.Join(dir, "telegram.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	p, ok := files["telegram"]
	if !ok {
		t.Fatal("telegram policy not loaded")
	}
	if p.PrivacyClass != schema.PrivacyClassSensitive {
		t.Errorf("privacy: %q", p.PrivacyClass)
	}
	if p.Delta.Cadence != "45m" {
		t.Errorf("cadence: %q", p.Delta.Cadence)
	}
	if p.Delta.Budget.MaxRecordsPerPass != 123 {
		t.Errorf("records: %d", p.Delta.Budget.MaxRecordsPerPass)
	}
	// File overlays the default, so untouched backfill block survives.
	if p.Backfill.Cadence != schema.CadenceManual {
		t.Errorf("backfill cadence should come from default: %q", p.Backfill.Cadence)
	}
}

func TestLoadDir_MissingDirIsNotError(t *testing.T) {
	files, err := LoadDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected empty map, got %d", len(files))
	}
}

func TestEstimateRecordCostUSD(t *testing.T) {
	if EstimateRecordCostUSD(0) != 0 {
		t.Error("zero bytes should cost 0")
	}
	if EstimateRecordCostUSD(1024) <= 0 {
		t.Error("non-zero bytes should have positive cost")
	}
	// Larger payload costs more.
	if EstimateRecordCostUSD(2048) <= EstimateRecordCostUSD(1024) {
		t.Error("cost should scale with bytes")
	}
}
