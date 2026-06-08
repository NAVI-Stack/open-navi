package vault

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
	"github.com/ceoai/navi/internal/vault/projector"
)

func newWorker(t *testing.T, opts governor.MutationPipelineOptions) *Worker {
	t.Helper()
	db := store.InitTestDB(t)
	w, err := New(Config{
		DB:           db,
		Root:         t.TempDir(),
		GovernorOpts: opts,
		Now:          func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return w
}

func seedContact(t *testing.T, w *Worker, id, name string) {
	t.Helper()
	if err := store.SaveContact(context.Background(), w.db, schema.Contact{
		ID: id, Name: name, Kind: schema.ContactKindPerson,
		OwnerType: schema.ContactOwnerTypeNavi, TrustLevel: schema.ContactTrustLevelMedium,
		Metadata: `{"notes":"Initial notes."}`,
	}); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
}

func contactFile(t *testing.T, w *Worker, id string) string {
	t.Helper()
	st, ok, err := store.GetVaultStateByEntity(context.Background(), w.db, projector.TypeContact, id)
	if err != nil || !ok {
		t.Fatalf("no vault_state for contact %s (ok=%v err=%v)", id, ok, err)
	}
	return w.absPath(st.FilePath)
}

func vaultProposals(t *testing.T, w *Worker) []schema.Proposal {
	t.Helper()
	all, err := store.ListPendingProposals(context.Background(), w.db, 100)
	if err != nil {
		t.Fatalf("list proposals: %v", err)
	}
	var out []schema.Proposal
	for _, p := range all {
		if p.SourceProcess == governor.MutationSourceVault {
			out = append(out, p)
		}
	}
	return out
}

func TestProjectAll_RendersContact(t *testing.T) {
	w := newWorker(t, governor.MutationPipelineOptions{})
	seedContact(t, w, "c1", "Alex Rivera")
	ctx := context.Background()
	if _, err := w.ProjectAll(ctx); err != nil {
		t.Fatalf("ProjectAll: %v", err)
	}
	path := contactFile(t, w, "c1")
	if !strings.HasPrefix(w.relPath(path), "contacts/") {
		t.Errorf("contact not under contacts/: %s", w.relPath(path))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read projected file: %v", err)
	}
	if !strings.Contains(string(b), "# Alex Rivera") {
		t.Errorf("projected file missing name heading:\n%s", b)
	}
	// Index page exists and is read-only.
	idx, err := os.ReadFile(w.absPath(projector.ContactsByRecencyPath()))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !strings.Contains(string(idx), "navi_readonly: true") {
		t.Error("contacts index not marked read-only")
	}
}

func TestProjectAll_Idempotent(t *testing.T) {
	w := newWorker(t, governor.MutationPipelineOptions{})
	seedContact(t, w, "c1", "Alex Rivera")
	ctx := context.Background()
	if _, err := w.ProjectAll(ctx); err != nil {
		t.Fatalf("ProjectAll: %v", err)
	}
	path := contactFile(t, w, "c1")
	first, _ := os.ReadFile(path)
	info1, _ := os.Stat(path)

	// Second projection of unchanged entity must not rewrite the file.
	n, err := w.ProjectAll(ctx)
	if err != nil {
		t.Fatalf("ProjectAll#2: %v", err)
	}
	if n != 0 {
		t.Errorf("second ProjectAll rewrote %d files; want 0 (idempotent)", n)
	}
	second, _ := os.ReadFile(path)
	info2, _ := os.Stat(path)
	if string(first) != string(second) {
		t.Error("reprojection produced different bytes")
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Error("idempotent reprojection rewrote the file (mtime changed)")
	}
}

func TestOwnerEdit_ApprovedRoundTrip(t *testing.T) {
	w := newWorker(t, governor.MutationPipelineOptions{})
	seedContact(t, w, "c1", "Alex Rivera")
	ctx := context.Background()
	_, _ = w.ProjectAll(ctx)
	path := contactFile(t, w, "c1")

	content, _ := os.ReadFile(path)
	edited := strings.Replace(string(content), "# Alex Rivera", "# Alexandra Rivera", 1)
	edited = strings.Replace(edited, "Initial notes.", "Works on RF systems now.", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := w.handleWritten(ctx, path); err != nil {
		t.Fatalf("handleWritten: %v", err)
	}

	c, err := store.GetContact(ctx, w.db, "c1")
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}
	if c.Name != "Alexandra Rivera" {
		t.Errorf("owner name edit did not round-trip: %q", c.Name)
	}
	if !strings.Contains(c.Metadata, "RF systems") {
		t.Errorf("owner body edit did not round-trip: %q", c.Metadata)
	}

	// Reprojection is idempotent: a re-render now matches the file on disk.
	after, _ := os.ReadFile(path)
	e, _, _ := w.loadEntity(ctx, entityRef{Type: projector.TypeContact, ID: "c1"})
	if projector.ContentHash(string(after)) != projector.ContentHash(projector.Render(e)) {
		t.Error("post-edit file is not the idempotent projection of canonical state")
	}

	// A sync-log row was written.
	rows, _ := store.ListVaultSyncLog(ctx, w.db, "", 10)
	if len(rows) == 0 {
		t.Error("no vault_sync_log row recorded for the edit")
	}
}

func TestOwnerEdit_NoBypassWhenWriteClassDisabled(t *testing.T) {
	// Hard invariant: every Vault write goes through the governor seam. With the
	// world_model_mutation effect disabled, an owner edit is Rejected and the
	// entity is unchanged — there is no bypass path.
	w := newWorker(t, governor.MutationPipelineOptions{WriteClassDisabled: true})
	seedContact(t, w, "c1", "Alex Rivera")
	ctx := context.Background()
	_, _ = w.ProjectAll(ctx)
	path := contactFile(t, w, "c1")
	content, _ := os.ReadFile(path)
	edited := strings.Replace(string(content), "# Alex Rivera", "# Mallory", 1)
	_ = os.WriteFile(path, []byte(edited), 0o644)

	if err := w.handleWritten(ctx, path); err != nil {
		t.Fatalf("handleWritten: %v", err)
	}
	c, _ := store.GetContact(ctx, w.db, "c1")
	if c.Name != "Alex Rivera" {
		t.Errorf("disabled write-class still mutated entity: %q", c.Name)
	}
}

func TestFileDelete_RaisesForgetProposal(t *testing.T) {
	w := newWorker(t, governor.MutationPipelineOptions{})
	seedContact(t, w, "c1", "Alex Rivera")
	ctx := context.Background()
	_, _ = w.ProjectAll(ctx)
	path := contactFile(t, w, "c1")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := w.handleRemoved(ctx, path); err != nil {
		t.Fatalf("handleRemoved: %v", err)
	}
	// Entity must NOT be deleted.
	if _, err := store.GetContact(ctx, w.db, "c1"); err != nil {
		t.Errorf("file delete deleted the entity (must only request forget): %v", err)
	}
	props := vaultProposals(t, w)
	if len(props) != 1 {
		t.Fatalf("expected 1 vault forget proposal, got %d", len(props))
	}
	if !strings.Contains(props[0].ProposedAction, "forget") {
		t.Errorf("proposal is not a forget: %q", props[0].ProposedAction)
	}
}

func TestForgetFrontmatter_RaisesProposal(t *testing.T) {
	w := newWorker(t, governor.MutationPipelineOptions{})
	seedContact(t, w, "c1", "Alex Rivera")
	ctx := context.Background()
	_, _ = w.ProjectAll(ctx)
	path := contactFile(t, w, "c1")
	content, _ := os.ReadFile(path)
	edited := strings.Replace(string(content), "navi_forget: false", "navi_forget: true", 1)
	_ = os.WriteFile(path, []byte(edited), 0o644)

	if err := w.handleWritten(ctx, path); err != nil {
		t.Fatalf("handleWritten: %v", err)
	}
	if _, err := store.GetContact(ctx, w.db, "c1"); err != nil {
		t.Errorf("navi_forget deleted the entity (must only request forget): %v", err)
	}
	if len(vaultProposals(t, w)) != 1 {
		t.Fatalf("navi_forget did not raise a proposal")
	}
}

func TestEntityIDSwap_RaisesMergeProposal(t *testing.T) {
	w := newWorker(t, governor.MutationPipelineOptions{})
	seedContact(t, w, "c1", "Alex Rivera")
	seedContact(t, w, "c2", "Alexandra Rivera")
	ctx := context.Background()
	_, _ = w.ProjectAll(ctx)
	path := contactFile(t, w, "c1")
	content, _ := os.ReadFile(path)
	edited := strings.Replace(string(content), `navi_entity_id: "c1"`, `navi_entity_id: "c2"`, 1)
	_ = os.WriteFile(path, []byte(edited), 0o644)

	if err := w.handleWritten(ctx, path); err != nil {
		t.Fatalf("handleWritten: %v", err)
	}
	props := vaultProposals(t, w)
	if len(props) != 1 || !strings.Contains(props[0].ProposedAction, "merge") {
		t.Fatalf("entity_id swap did not raise a merge proposal: %+v", props)
	}
}

func TestHardFloor_ForgetNeverAutoApproved(t *testing.T) {
	// Even with a maximally permissive autonomy resolver, Forget from the Vault
	// stays a Proposal — hard floors are unbreakable from the Vault path.
	w := newWorker(t, governor.MutationPipelineOptions{Resolver: highAutonomyResolver{}})
	seedContact(t, w, "c1", "Alex Rivera")
	ctx := context.Background()
	_, _ = w.ProjectAll(ctx)
	path := contactFile(t, w, "c1")
	content, _ := os.ReadFile(path)
	edited := strings.Replace(string(content), "navi_forget: false", "navi_forget: true", 1)
	_ = os.WriteFile(path, []byte(edited), 0o644)

	if err := w.handleWritten(ctx, path); err != nil {
		t.Fatalf("handleWritten: %v", err)
	}
	if _, err := store.GetContact(ctx, w.db, "c1"); err != nil {
		t.Error("high autonomy auto-approved a Vault forget — hard floor breached")
	}
	if len(vaultProposals(t, w)) != 1 {
		t.Error("forget did not raise a proposal under high autonomy")
	}
}

func TestConflict_OwnerOverridesExternalDelta(t *testing.T) {
	w := newWorker(t, governor.MutationPipelineOptions{})
	ctx := context.Background()
	seedContact(t, w, "c1", "Alex R.")
	// Simulate an earlier external delta: provenance from a connector, lower conf.
	if err := store.SaveEntityProvenance(ctx, w.db, projector.TypeContact, "c1", schema.EntityProvenance{
		Source: "intake:telegram:acct-1", Timestamp: time.Now().UTC().Add(-time.Minute), Confidence: 0.5,
	}); err != nil {
		t.Fatalf("seed provenance: %v", err)
	}
	_, _ = w.ProjectAll(ctx)
	path := contactFile(t, w, "c1")
	content, _ := os.ReadFile(path)
	edited := strings.Replace(string(content), "# Alex R.", "# Alex Rivera", 1)
	_ = os.WriteFile(path, []byte(edited), 0o644)

	if err := w.handleWritten(ctx, path); err != nil {
		t.Fatalf("handleWritten: %v", err)
	}
	c, _ := store.GetContact(ctx, w.db, "c1")
	if c.Name != "Alex Rivera" {
		t.Errorf("owner edit did not win routine-attribute conflict: %q", c.Name)
	}
	ep, _ := store.GetEntityProvenance(ctx, w.db, projector.TypeContact, "c1")
	if ep == nil {
		t.Fatal("provenance gone after owner override")
	}
	if ep.Confidence < 0.5 {
		t.Errorf("owner override lowered confidence: %v", ep.Confidence)
	}
	found := false
	for _, h := range ep.MutationHistory {
		if strings.Contains(h, "owner edit via vault") {
			found = true
		}
	}
	if !found {
		t.Errorf("no overridden-by-owner provenance note: %+v", ep.MutationHistory)
	}
}

func TestDriftSweep_RegeneratesDivergedFile(t *testing.T) {
	w := newWorker(t, governor.MutationPipelineOptions{})
	ctx := context.Background()
	seedContact(t, w, "c1", "Alex Rivera")
	_, _ = w.ProjectAll(ctx)
	path := contactFile(t, w, "c1")

	// External delta writes the entity directly (any synthesis caller).
	c, _ := store.GetContact(ctx, w.db, "c1")
	c.Name = "Alex Rivera (updated externally)"
	_ = store.SaveContact(ctx, w.db, c)

	rep, err := w.DriftSweep(ctx)
	if err != nil {
		t.Fatalf("DriftSweep: %v", err)
	}
	if rep.Drifted == 0 {
		t.Error("drift sweep did not detect the diverged file")
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "updated externally") {
		t.Errorf("drift sweep did not regenerate file from canonical state:\n%s", b)
	}
	// Drift rows landed in vault_sync_log (P5 schema).
	rows, _ := store.ListVaultSyncLog(ctx, w.db, "_drift_sweep", 5)
	if len(rows) == 0 {
		t.Error("no drift summary row in vault_sync_log")
	}
}

func TestPrivacy_SecretInLocalNeverReachesCloud(t *testing.T) {
	called := false
	w := newWorker(t, governor.MutationPipelineOptions{})
	w.cfg.PrivacyMode = llm.PrivacyModeLocal
	w.cfg.CloudEmbedder = func(_ context.Context, _ string) error { called = true; return nil }

	if w.CloudReachAllowed(schema.PrivacyClassSecret) {
		t.Error("secret content reported cloud-eligible in Local mode")
	}
	var notes []string
	w.maybeCloudEmbed(context.Background(), schema.PrivacyClassSecret, "secret body", &notes)
	if called {
		t.Fatal("cloud embedder invoked for secret-class edit in Local mode")
	}
	if len(notes) == 0 || !strings.Contains(notes[0], "withheld") {
		t.Errorf("no local-only withholding note recorded: %+v", notes)
	}

	// Sanity: a personal edit in Cloud mode is allowed to reach the cloud.
	called = false
	w.cfg.PrivacyMode = llm.PrivacyModeCloud
	notes = nil
	w.maybeCloudEmbed(context.Background(), schema.PrivacyClassPersonal, "personal body", &notes)
	if !called {
		t.Error("cloud embedder not invoked for personal-class edit in Cloud mode")
	}
}

func TestIndexEdits_Ignored(t *testing.T) {
	w := newWorker(t, governor.MutationPipelineOptions{})
	ctx := context.Background()
	seedContact(t, w, "c1", "Alex Rivera")
	_, _ = w.ProjectAll(ctx)
	idxPath := w.absPath(projector.ContactsByRecencyPath())
	_ = os.WriteFile(idxPath, []byte("---\nnavi_index: true\n---\n# tampered\n"), 0o644)
	if err := w.handleWritten(ctx, idxPath); err != nil {
		t.Fatalf("handleWritten(index): %v", err)
	}
	// No proposals, no entity changes from an index edit.
	if len(vaultProposals(t, w)) != 0 {
		t.Error("index edit raised a proposal")
	}
}

func TestStart_FullRoundTripWithMute(t *testing.T) {
	w := newWorker(t, governor.MutationPipelineOptions{})
	w.cfg.Debounce = 60 * time.Millisecond
	w.cfg.DriftInterval = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seedContact(t, w, "c1", "Alex Rivera")
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	path := contactFile(t, w, "c1")
	content, _ := os.ReadFile(path)
	edited := strings.Replace(string(content), "# Alex Rivera", "# Alexandra Rivera", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for the watcher → handleWritten → applyApprovedEdit pipeline.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := store.GetContact(ctx, w.db, "c1"); err == nil && c.Name == "Alexandra Rivera" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	c, _ := store.GetContact(ctx, w.db, "c1")
	if c.Name != "Alexandra Rivera" {
		t.Fatalf("full round-trip did not apply owner edit: %q", c.Name)
	}

	// The reprojection write (muted) must not generate spurious proposals.
	time.Sleep(400 * time.Millisecond)
	if n := len(vaultProposals(t, w)); n != 0 {
		t.Errorf("reprojection re-entered as owner edits (got %d proposals)", n)
	}
}

// highAutonomyResolver always reports the highest execution threshold so the
// hard-floor test proves autonomy cannot override the floor.
type highAutonomyResolver struct{}

func (highAutonomyResolver) EffectivePreset(domain string) string             { return "high" }
func (highAutonomyResolver) EffectiveExecutionThreshold(domain string) string { return "high" }

var _ = filepath.Join
