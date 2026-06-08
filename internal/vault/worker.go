// Package vault implements the Memory Vault (memory-projection-v1.md): the
// owner-facing Markdown projection of the World Model and the editable surface
// back into it. The Vault is the *second mouth* on the intake pipeline — owner
// edits enter as structured diffs tagged ContentTrust: owner and flow through the
// SAME governor seam (governor.EvaluateMutation) external deltas use. There is
// one acquisition pipeline; the Vault is not a parallel authority.
//
// Worker wires the pieces together:
//
//	projector  entity → Markdown (idempotent)
//	watcher    fsnotify + debounce + projector-mute
//	diff       owner-edited Markdown → MutationDescriptors (ContentTrust: owner)
//	governor   EvaluateMutation → write / Proposal / drop (the frozen P3 seam)
//	store      vault_state (file↔entity↔hash), vault_sync_log (P5 schema)
//
// Invariant: no World Model write happens on any path that bypasses
// governor.EvaluateMutation. Hard floors (Merge/Deprecate/Forget) always raise a
// Proposal in the ONE existing queue tagged source_process: "vault".
package vault

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	"github.com/open-navi/navi/internal/vault/diff"
	"github.com/open-navi/navi/internal/vault/projector"
	"github.com/open-navi/navi/internal/vault/watcher"
)

// Config configures the Vault worker.
type Config struct {
	DB            *sql.DB
	Root          string // vault root directory (default: <workspace>/vault)
	GovernorOpts  governor.MutationPipelineOptions
	PrivacyMode   llm.PrivacyMode // CIP §9 locality posture for cloud-bound steps
	Debounce      time.Duration   // owner-edit debounce (default 750ms)
	DriftInterval time.Duration   // periodic drift sweep cadence (default 24h)
	MaxEntities   int             // projection scope bound (default 1000)
	Logger        *slog.Logger

	// CloudEmbedder, when set, is the only Vault step that may send edit content
	// to a cloud model. It is invoked after an approved write *only* when the
	// entity's privacy class is cloud-eligible under PrivacyMode. In production it
	// is nil (the local heuristic embedder in store handles embedding), which makes
	// secret-in-Local trivially safe; tests inject it to assert the gate.
	CloudEmbedder func(ctx context.Context, text string) error

	// Clock override for tests.
	Now func() time.Time
}

// Worker is the Vault orchestrator.
type Worker struct {
	cfg  Config
	db   *sql.DB
	root string
	log  *slog.Logger
	now  func() time.Time
	wch  *watcher.Watcher
}

// muteWindow is how long the projector's own writes are ignored by the watcher.
const muteWindow = 1500 * time.Millisecond

// New constructs a Worker, creating the Vault directory tree if absent.
func New(cfg Config) (*Worker, error) {
	if cfg.DB == nil {
		return nil, errors.New("vault: DB required")
	}
	if cfg.Root == "" {
		return nil, errors.New("vault: Root required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Debounce <= 0 {
		cfg.Debounce = 750 * time.Millisecond
	}
	if cfg.DriftInterval <= 0 {
		cfg.DriftInterval = 24 * time.Hour
	}
	if cfg.MaxEntities <= 0 {
		cfg.MaxEntities = 1000
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, err
	}
	for _, sub := range []string{"contacts", "knowledge", "memories", "artifacts", projector.IndexDir} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return nil, err
		}
	}
	return &Worker{cfg: cfg, db: cfg.DB, root: root, log: cfg.Logger, now: now}, nil
}

// Root returns the absolute Vault root directory.
func (w *Worker) Root() string { return w.root }

func (w *Worker) absPath(rel string) string {
	return filepath.Join(w.root, filepath.FromSlash(rel))
}

func (w *Worker) relPath(abs string) string {
	rel, err := filepath.Rel(w.root, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}

// Start projects all entities, then begins watching for owner edits and runs the
// periodic drift sweep. It returns once watching has started; it stops when ctx
// is cancelled.
func (w *Worker) Start(ctx context.Context) error {
	if _, err := w.ProjectAll(ctx); err != nil {
		return err
	}
	wch, err := watcher.New(watcher.Config{
		Root:     w.root,
		Debounce: w.cfg.Debounce,
		Logger:   w.log,
	})
	if err != nil {
		return err
	}
	if err := wch.Start(ctx); err != nil {
		_ = wch.Close()
		return err
	}
	w.wch = wch

	go w.consume(ctx)
	go w.driftLoop(ctx)
	w.log.Info("vault: worker started", "root", w.root, "drift_interval", w.cfg.DriftInterval)
	return nil
}

// Close releases the watcher.
func (w *Worker) Close() error {
	if w.wch != nil {
		return w.wch.Close()
	}
	return nil
}

func (w *Worker) consume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.wch.Events():
			if !ok {
				return
			}
			switch ev.Kind {
			case watcher.Removed:
				if err := w.handleRemoved(ctx, ev.Path); err != nil {
					w.log.Warn("vault: handle delete failed", "path", ev.Path, "err", err)
				}
			default:
				if err := w.handleWritten(ctx, ev.Path); err != nil {
					w.log.Warn("vault: handle edit failed", "path", ev.Path, "err", err)
				}
			}
		}
	}
}

func (w *Worker) driftLoop(ctx context.Context) {
	t := time.NewTicker(w.cfg.DriftInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := w.DriftSweep(ctx); err != nil {
				w.log.Warn("vault: drift sweep failed", "err", err)
			}
		}
	}
}

// ProjectAll renders every projectable entity and the index pages. Idempotent:
// files whose projector-owned content is unchanged are not rewritten.
func (w *Worker) ProjectAll(ctx context.Context) (int, error) {
	entities, err := w.loadAllProjectable(ctx)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, e := range entities {
		changed, err := w.projectEntity(ctx, e)
		if err != nil {
			w.log.Warn("vault: project entity failed", "type", e.Type, "id", e.ID, "err", err)
			continue
		}
		if changed {
			written++
		}
	}
	if err := w.writeIndexes(ctx, entities); err != nil {
		w.log.Warn("vault: write indexes failed", "err", err)
	}
	return written, nil
}

// projectEntity renders one entity and writes it iff its content changed. It
// reuses an already-mapped path so a title edit does not churn filenames.
func (w *Worker) projectEntity(ctx context.Context, e projector.Entity) (bool, error) {
	rel := projector.DefaultPath(e)
	if st, ok, _ := store.GetVaultStateByEntity(ctx, w.db, e.Type, e.ID); ok && st.FilePath != "" {
		rel = st.FilePath
	}
	content := projector.Render(e)
	hash := projector.ContentHash(content)

	// Idempotency: if the file already holds identical bytes, skip the write.
	if cur, err := os.ReadFile(w.absPath(rel)); err == nil && projector.ContentHash(string(cur)) == hash {
		// Ensure mapping/hash are recorded even when no write was needed.
		_ = store.UpsertVaultState(ctx, w.db, store.VaultState{
			FilePath: rel, EntityType: e.Type, EntityID: e.ID, ProjectedHash: hash, ProjectedAt: w.now(),
		})
		return false, nil
	}
	if err := w.writeFile(rel, content); err != nil {
		return false, err
	}
	if err := store.UpsertVaultState(ctx, w.db, store.VaultState{
		FilePath: rel, EntityType: e.Type, EntityID: e.ID, ProjectedHash: hash, ProjectedAt: w.now(),
	}); err != nil {
		return false, err
	}
	return true, nil
}

// writeFile mutes the watcher for the path, then writes the file (creating
// parents). The mute prevents the resulting fsnotify event from re-entering as a
// fake owner edit (spec §9 file-watcher mute).
func (w *Worker) writeFile(rel, content string) error {
	abs := w.absPath(rel)
	if w.wch != nil {
		w.wch.Mute(abs, muteWindow)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

// writeIndexes renders the two V1 read-only index pages.
func (w *Worker) writeIndexes(ctx context.Context, entities []projector.Entity) error {
	var contacts, knowledge []projector.IndexEntry
	for _, e := range entities {
		st, ok, _ := store.GetVaultStateByEntity(ctx, w.db, e.Type, e.ID)
		rel := projector.DefaultPath(e)
		if ok && st.FilePath != "" {
			rel = st.FilePath
		}
		switch e.Type {
		case projector.TypeContact:
			contacts = append(contacts, projector.IndexEntry{Title: e.Title, RelPath: rel, When: e.UpdatedAt, Subtitle: "updated " + shortDate(e.UpdatedAt)})
		case projector.TypeKnowledge:
			topic := ""
			for _, a := range e.Attrs {
				if a.Key == "category" {
					topic = a.Value
				}
			}
			knowledge = append(knowledge, projector.IndexEntry{Title: e.Title, RelPath: rel, When: e.UpdatedAt, Subtitle: topic})
		}
	}
	projector.SortByRecency(contacts)
	if err := w.writeFile(projector.ContactsByRecencyPath(), projector.RenderIndex("All contacts by recency", contacts)); err != nil {
		return err
	}
	return w.writeFile(projector.KnowledgeByTopicPath(), projector.RenderIndex("Knowledge by topic", knowledge))
}

func shortDate(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.UTC().Format("2006-01-02")
}

// ReprojectEntity re-renders a single entity's file from canonical state. This is
// the reprojection hook for any synthesis caller (external delta, owner edit,
// Deep Reflection): after an approved entity write, call this to refresh the
// owner-facing file. Idempotent and mute-bracketed.
func (w *Worker) ReprojectEntity(ctx context.Context, entityType, entityID string) error {
	e, ok, err := w.loadEntity(ctx, entityRef{Type: entityType, ID: entityID})
	if err != nil {
		return err
	}
	if !ok {
		return nil // entity gone; nothing to reproject
	}
	_, err = w.projectEntity(ctx, e)
	return err
}

// handleWritten processes a settled owner edit to a file.
func (w *Worker) handleWritten(ctx context.Context, abs string) error {
	rel := w.relPath(abs)
	if projector.IsIndexPath(rel) {
		w.log.Debug("vault: ignoring edit to read-only index page", "path", rel)
		return nil
	}
	started := w.now()

	content, err := os.ReadFile(abs)
	if err != nil {
		// Vanished between event and read — treat as a delete.
		return w.handleRemoved(ctx, abs)
	}
	// Echo guard: identical to what the projector last wrote → our own write.
	if st, ok, _ := store.GetVaultState(ctx, w.db, rel); ok && st.ProjectedHash == projector.ContentHash(string(content)) {
		return nil
	}

	doc, perr := projector.Parse(string(content))
	if perr != nil {
		w.appendSync(ctx, rel, started, "parse error: "+perr.Error(), 0, 0, 0, 1, schema.SyncStatusError)
		return nil
	}

	ref, ok := w.resolveRef(ctx, rel, doc)
	if !ok {
		w.log.Debug("vault: unmapped file edit ignored", "path", rel)
		w.appendSync(ctx, rel, started, "unmapped file — ignored", 0, 0, 0, 0, schema.SyncStatusCompleted)
		return nil
	}

	cur, ok, err := w.buildCurrent(ctx, ref)
	if err != nil {
		return err
	}
	if !ok {
		w.log.Debug("vault: entity for file no longer exists", "path", rel, "entity", ref)
		return nil
	}

	plan := diff.Plan(doc, cur, diff.PlanOptions{RelPath: rel})
	if plan.NoOp {
		w.appendSync(ctx, rel, started, "no changes", 0, 0, 0, 0, schema.SyncStatusCompleted)
		// Reproject to normalize owner formatting back to canonical rendering.
		_ = w.ReprojectEntity(ctx, ref.Type, ref.ID)
		return nil
	}

	summary, proposed, approved, proposals, errs := w.runMutations(ctx, ref, doc, cur, plan, rel)
	status := schema.SyncStatusCompleted
	if errs > 0 {
		status = schema.SyncStatusError
	}
	if summary == "" {
		summary = plan.Summary
	}
	w.appendSync(ctx, rel, started, summary, proposed, approved, proposals, errs, status)

	// Reproject the affected entity so the file reflects canonical state and the
	// projector hash is refreshed (prevents the next sweep flagging false drift).
	if !plan.Structural {
		_ = w.ReprojectEntity(ctx, ref.Type, ref.ID)
	}
	return nil
}

// resolveRef maps an edited file to its entity: first by the vault_state mapping,
// then by the file's own navi_entity_id frontmatter (so a freshly created file
// that names an existing entity still resolves).
func (w *Worker) resolveRef(ctx context.Context, rel string, doc projector.Doc) (entityRef, bool) {
	if st, ok, _ := store.GetVaultState(ctx, w.db, rel); ok {
		return entityRef{Type: st.EntityType, ID: st.EntityID}, true
	}
	if doc.EntityID != "" && doc.EntityType != "" {
		if _, ok, _ := w.buildCurrent(ctx, entityRef{Type: doc.EntityType, ID: doc.EntityID}); ok {
			return entityRef{Type: doc.EntityType, ID: doc.EntityID}, true
		}
	}
	return entityRef{}, false
}

// runMutations runs each planned mutation through the governor seam and acts on
// the outcome. Returns sync-log counters.
func (w *Worker) runMutations(ctx context.Context, ref entityRef, doc projector.Doc, cur diff.Current, plan diff.Result, rel string) (summary string, proposed, approved, proposals, errs int) {
	var notes []string
	for _, m := range plan.Mutations {
		proposed++
		result := governor.EvaluateMutation(m, w.cfg.GovernorOpts)
		switch result.Outcome {
		case governor.ValidationApproved:
			if err := w.applyApprovedEdit(ctx, ref, doc, ownerConfidence(m.EstConfidence)); err != nil {
				w.log.Warn("vault: apply approved edit failed", "err", err)
				errs++
				continue
			}
			approved++
			notes = append(notes, plan.Summary)
			w.maybeCloudEmbed(ctx, cur.PrivacyClass, doc.Body, &notes)
		case governor.ValidationModified:
			// Softened: apply at reduced confidence (rare for owner edits).
			if err := w.applyApprovedEdit(ctx, ref, doc, ownerConfidence(m.EstConfidence)*0.6); err != nil {
				errs++
				continue
			}
			approved++
			notes = append(notes, "modified: "+plan.Summary)
		case governor.ValidationRequiresConfirmation:
			pid, err := w.raiseVaultProposal(ctx, m, plan.Summary, rel)
			if err != nil {
				errs++
				continue
			}
			proposals++
			notes = append(notes, "proposal "+pid+": "+hardFloorLabel(m.Kind))
		case governor.ValidationRejected:
			w.log.Info("vault: mutation rejected — dropped", "entity", ref, "reason", result.Reason)
			notes = append(notes, "rejected: "+result.Reason)
		}
	}
	return strings.Join(notes, "; "), proposed, approved, proposals, errs
}

// maybeCloudEmbed enforces the privacy locality rule (spec §12): a secret-class
// entity edit in Local mode never reaches a cloud model. The cloud embedder runs
// only when the privacy class is cloud-eligible under the configured mode.
func (w *Worker) maybeCloudEmbed(ctx context.Context, privacy schema.PrivacyClass, text string, notes *[]string) {
	if w.cfg.CloudEmbedder == nil {
		return
	}
	if !llm.CloudEligible(w.cfg.PrivacyMode, privacy) {
		*notes = append(*notes, "embedding withheld (local-only: "+string(privacy)+" in "+string(w.cfg.PrivacyMode)+" mode)")
		return
	}
	if err := w.cfg.CloudEmbedder(ctx, text); err != nil {
		w.log.Debug("vault: cloud embed failed", "err", err)
	}
}

// CloudReachAllowed reports whether content of the given privacy class may reach a
// cloud model under the configured mode. Exposed for tests and observability.
func (w *Worker) CloudReachAllowed(privacy schema.PrivacyClass) bool {
	return llm.CloudEligible(w.cfg.PrivacyMode, privacy)
}

// handleRemoved processes a file deletion: it raises a Forget Proposal (hard
// floor) and never deletes the entity. The file is a rendered view; the entity is
// truth (spec §10).
func (w *Worker) handleRemoved(ctx context.Context, abs string) error {
	rel := w.relPath(abs)
	if projector.IsIndexPath(rel) {
		return nil
	}
	started := w.now()
	st, ok, _ := store.GetVaultState(ctx, w.db, rel)
	if !ok {
		return nil // unmapped file; nothing to do
	}
	ref := entityRef{Type: st.EntityType, ID: st.EntityID}
	cur, ok, err := w.buildCurrent(ctx, ref)
	if err != nil {
		return err
	}
	if !ok {
		_ = store.DeleteVaultState(ctx, w.db, rel)
		return nil
	}
	plan := diff.ForgetOnDelete(cur, rel)
	proposals := 0
	for _, m := range plan.Mutations {
		result := governor.EvaluateMutation(m, w.cfg.GovernorOpts)
		// Forget is hard-floored: it must be RequiresConfirmation regardless of
		// autonomy. If a future config ever auto-approved it, we still refuse to
		// delete the entity from the Vault path — only the governed Forget flow
		// (proposal approval) removes entities.
		if result.Outcome == governor.ValidationRequiresConfirmation {
			if _, err := w.raiseVaultProposal(ctx, m, plan.Summary, rel); err != nil {
				w.appendSync(ctx, rel, started, "forget proposal failed: "+err.Error(), 1, 0, 0, 1, schema.SyncStatusError)
				return err
			}
			proposals++
		}
	}
	w.appendSync(ctx, rel, started, "file deleted → forget proposal raised (entity preserved)", len(plan.Mutations), 0, proposals, 0, schema.SyncStatusCompleted)
	return nil
}

// raiseVaultProposal raises (or reuses) a Proposal in the ONE existing queue,
// tagged source_process: "vault" (no second queue). Boundary key dedupes
// repeated edits expressing the same structural intent.
func (w *Worker) raiseVaultProposal(ctx context.Context, m governor.MutationDescriptor, rationale, rel string) (string, error) {
	boundary := "vault:" + m.Kind.String() + ":" + m.TargetEntity + ":" + strings.Join(m.CandidateRefs, ",")
	if existing, _ := store.GetPendingProposalByBoundaryKey(ctx, w.db, boundary); existing.ProposalID != "" {
		return existing.ProposalID, nil
	}
	affected := append([]string{m.TargetEntity}, m.CandidateRefs...)
	affectedJSON := jsonStrings(dedupeNonEmpty(affected))
	p := schema.Proposal{
		ProposalID:       uuid.NewString(),
		SourceProcess:    governor.MutationSourceVault,
		SourceTrigger:    rel,
		BoundaryKey:      boundary,
		ProposedAction:   "vault_" + m.Kind.String() + ":" + boundary,
		AffectedEntities: affectedJSON,
		Rationale:        "Owner edit via Vault (" + rel + "): " + rationale,
		Priority:         schema.ProposalPriorityQueued,
		Status:           schema.ProposalStatusPending,
	}
	if err := store.SaveProposal(ctx, w.db, p); err != nil {
		return "", err
	}
	return p.ProposalID, nil
}

// DriftReport summarizes a periodic drift sweep.
type DriftReport struct {
	Checked int
	Drifted int
	Errors  int
}

// DriftSweep regenerates every Vault file from canonical state and records any
// file that had drifted (content diverged from canonical, or was missing). Drift
// rows go into vault_sync_log (spec §9, §13; P5 schema). This is the slow
// backstop that catches divergence the event path missed — including external
// deltas and Deep-Reflection writes.
func (w *Worker) DriftSweep(ctx context.Context) (DriftReport, error) {
	var rep DriftReport
	started := w.now()
	entities, err := w.loadAllProjectable(ctx)
	if err != nil {
		return rep, err
	}
	for _, e := range entities {
		rep.Checked++
		rel := projector.DefaultPath(e)
		if st, ok, _ := store.GetVaultStateByEntity(ctx, w.db, e.Type, e.ID); ok && st.FilePath != "" {
			rel = st.FilePath
		}
		want := projector.Render(e)
		wantHash := projector.ContentHash(want)
		onDisk, rerr := os.ReadFile(w.absPath(rel))
		drifted := rerr != nil || projector.ContentHash(string(onDisk)) != wantHash
		if !drifted {
			continue
		}
		rep.Drifted++
		if err := w.writeFile(rel, want); err != nil {
			rep.Errors++
			continue
		}
		_ = store.UpsertVaultState(ctx, w.db, store.VaultState{
			FilePath: rel, EntityType: e.Type, EntityID: e.ID, ProjectedHash: wantHash, ProjectedAt: w.now(),
		})
		w.appendSync(ctx, rel, started, "drift: file regenerated from canonical state", 0, 0, 0, 0, schema.SyncStatusCompleted)
	}
	if err := w.writeIndexes(ctx, entities); err != nil {
		w.log.Warn("vault: drift index refresh failed", "err", err)
	}
	// Summary row for the operator/debug surface.
	status := schema.SyncStatusCompleted
	if rep.Errors > 0 {
		status = schema.SyncStatusError
	}
	w.appendSync(ctx, "_drift_sweep", started,
		summaryDrift(rep), 0, 0, 0, rep.Errors, status)
	return rep, nil
}

func summaryDrift(r DriftReport) string {
	return "drift sweep: checked " + itoa(r.Checked) + ", regenerated " + itoa(r.Drifted) + ", errors " + itoa(r.Errors)
}

// appendSync writes one vault_sync_log row (P5 schema). Best-effort: a logging
// failure must not abort edit processing.
func (w *Worker) appendSync(ctx context.Context, rel string, started time.Time, summary string, proposed, approved, proposals, errs int, status schema.SyncTerminalStatus) {
	end := w.now()
	if _, err := store.AppendVaultSyncLog(ctx, w.db, schema.VaultSyncLogEntry{
		FilePath:          rel,
		StartedAt:         started,
		EndedAt:           &end,
		DiffSummary:       summary,
		MutationsProposed: proposed,
		MutationsApproved: approved,
		ProposalsRaised:   proposals,
		Errors:            errs,
		TerminalStatus:    status,
	}); err != nil {
		w.log.Warn("vault: append sync log failed", "path", rel, "err", err)
	}
}

func ownerConfidence(est float64) float64 {
	if est <= 0 {
		return 0.95
	}
	if est > 1 {
		return 1
	}
	return est
}

func hardFloorLabel(k governor.MutationKind) string {
	switch k {
	case governor.MutationForgetMemory:
		return "forget requested"
	case governor.MutationMergeEntities:
		return "merge requested"
	case governor.MutationDeprecateEntity:
		return "deprecate requested"
	default:
		return k.String()
	}
}
