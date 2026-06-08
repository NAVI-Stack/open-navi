package onboarding

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/store"
)

func TestFirstRunTransitions(t *testing.T) {
	valid := []struct {
		from FirstRunState
		to   FirstRunState
	}{
		{FirstRunUninitialized, FirstRunRecoveryCreated},
		{FirstRunRecoveryCreated, FirstRunProviderConfigured},
		{FirstRunProviderConfigured, FirstRunComplete},
		{FirstRunRecoveryCreated, FirstRunRecoveryCreated},
	}
	for _, tc := range valid {
		if !CanTransitionFirstRun(tc.from, tc.to) {
			t.Fatalf("expected transition %s -> %s to be valid", tc.from, tc.to)
		}
	}

	invalid := []struct {
		from FirstRunState
		to   FirstRunState
	}{
		{FirstRunUninitialized, FirstRunProviderConfigured},
		{FirstRunRecoveryCreated, FirstRunComplete},
		{FirstRunComplete, FirstRunRecoveryCreated},
	}
	for _, tc := range invalid {
		if CanTransitionFirstRun(tc.from, tc.to) {
			t.Fatalf("expected transition %s -> %s to be invalid", tc.from, tc.to)
		}
	}
}

func TestDeriveFirstRunStatus(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	status, err := DeriveFirstRunStatus(ctx, db, dir)
	if err != nil {
		t.Fatalf("derive fresh: %v", err)
	}
	if status.State != FirstRunUninitialized {
		t.Fatalf("fresh state = %s, want %s", status.State, FirstRunUninitialized)
	}

	if err := store.CreateOwner(ctx, db, store.Owner{
		Name:              "Owner",
		Handle:            "owner",
		InstanceID:        "navi_inst_test",
		SecretFingerprint: "fingerprint",
	}); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	status, err = DeriveFirstRunStatus(ctx, db, dir)
	if err != nil {
		t.Fatalf("derive recovery: %v", err)
	}
	if status.State != FirstRunRecoveryCreated {
		t.Fatalf("owner-only state = %s, want %s", status.State, FirstRunRecoveryCreated)
	}

	if err := store.SetSetting(ctx, db, SettingKeyProviderConfigured, "true"); err != nil {
		t.Fatalf("set provider configured: %v", err)
	}
	if err := store.SetSetting(ctx, db, SettingKeyProviderName, "ollama"); err != nil {
		t.Fatalf("set provider name: %v", err)
	}
	status, err = DeriveFirstRunStatus(ctx, db, dir)
	if err != nil {
		t.Fatalf("derive provider: %v", err)
	}
	if status.State != FirstRunProviderConfigured || status.ProviderName != "ollama" {
		t.Fatalf("provider state = %+v", status)
	}

	if err := store.SetSetting(ctx, db, SettingKeySetupComplete, "true"); err != nil {
		t.Fatalf("set setup complete: %v", err)
	}
	status, err = DeriveFirstRunStatus(ctx, db, dir)
	if err != nil {
		t.Fatalf("derive complete: %v", err)
	}
	if status.State != FirstRunComplete || !status.Complete {
		t.Fatalf("complete state = %+v", status)
	}
}

func TestWriteRecoveryPassportPermissionsAndRedaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery", "passport.txt")
	if err := WriteRecoveryPassport(path, []byte("Primary API key: navi_abcdefghijklmnopqrstuvwxyz\n")); err != nil {
		t.Fatalf("write passport: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat passport: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("passport permissions too broad: %v", info.Mode().Perm())
	}

	redacted := RedactSecrets("api_key=navi_abcdefghijklmnopqrstuvwxyz secret=owner-secret token=sk-abcdefghi")
	for _, leaked := range []string{"navi_abcdefghijklmnopqrstuvwxyz", "owner-secret", "sk-abcdefghi"} {
		if strings.Contains(redacted, leaked) {
			t.Fatalf("redacted text leaked %q: %s", leaked, redacted)
		}
	}
}
