package intake

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/intake/embed"
	"github.com/open-navi/navi/internal/intake/extract"
	"github.com/open-navi/navi/internal/intake/policy"
	"github.com/open-navi/navi/internal/intake/retrieve"
	"github.com/open-navi/navi/internal/intake/score"
	"github.com/open-navi/navi/internal/intake/synthesize"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// worldModelEntityTables enumerates every table that constitutes the World
// Model entity store. A P1 Admit pass must not insert any rows into these.
var worldModelEntityTables = []string{
	"contacts",
	"knowledge",
	"memories",
	"memory_links",
	"artifacts",
	"artifact_versions",
	"artifact_branches",
	"wm_events",
	"proposals",
	"entity_provenance",
	"entity_relationships",
}

func tableRowCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	// Not all tables may exist in the test schema; return 0 if the table is missing.
	err := db.QueryRowContext(context.Background(),
		"SELECT COUNT(1) FROM "+table).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

func snapshotCounts(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	out := make(map[string]int, len(worldModelEntityTables))
	for _, tbl := range worldModelEntityTables {
		out[tbl] = tableRowCount(t, db, tbl)
	}
	return out
}

// TestNoWorldModelWritesOnAdmit verifies that processing an IntakeRecord via
// the Admit stage and persisting it to intake_records does NOT write any rows
// to World Model entity tables. P1 explicitly must not touch the World Model.
func TestNoWorldModelWritesOnAdmit(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()

	// Snapshot World Model entity table row counts before intake.
	before := snapshotCounts(t, db)

	// Process a realistic Telegram message record through the full P1 path.
	w := NewWorker(nil, db, nil, nil)
	r := IntakeRecord{
		ConnectorID: "telegram:acct-1",
		SourceKind:  "message",
		SourceID:    "999:1001",
		Cursor:      "999:1001",
		FetchedAt:   time.Now().UTC(),
		Trust:       TrustExternalUntrusted,
		Raw:         []byte("Hello from the outside world"),
		RawMIME:     "text/plain",
		Provenance: Provenance{
			ConnectorID: "telegram:acct-1",
			AccountID:   "acct-1",
		},
	}

	outcome, err := w.processRecord(ctx, r)
	if err != nil {
		t.Fatalf("processRecord: %v", err)
	}
	if outcome != outcomeAdmitted {
		t.Errorf("outcome: want outcomeAdmitted, got %v", outcome)
	}

	// Also run a duplicate to exercise the dedupe path.
	o2, _ := w.processRecord(ctx, r)
	if o2 != outcomeDeduped {
		t.Errorf("duplicate outcome: want outcomeDeduped, got %v", o2)
	}

	// Verify the record landed in intake_records.
	exists, err := store.IntakeRecordExists(ctx, db, "telegram:acct-1", "999:1001")
	if err != nil {
		t.Fatalf("IntakeRecordExists: %v", err)
	}
	if !exists {
		t.Fatal("intake record not found after processing")
	}

	// Snapshot World Model entity table row counts after intake.
	after := snapshotCounts(t, db)

	// Assert no World Model table changed.
	for _, tbl := range worldModelEntityTables {
		if before[tbl] != after[tbl] {
			t.Errorf("World Model table %q mutated: before=%d after=%d — P1 must not write the World Model",
				tbl, before[tbl], after[tbl])
		}
	}
}

// TestOnlySynthesizeWritesWorldModel extends the isolation invariant to P3: it
// verifies that Admit / Canonicalize / Chunk / Distill / Score / Embed / Extract /
// Resolve / Fold cause ZERO World Model entity writes, and that only Synthesize
// writes. The upstream stages are exercised explicitly so the assertion pins the
// boundary precisely (acceptance: hard invariant — Synthesize is the only writer).
func TestOnlySynthesizeWritesWorldModel(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()

	baseline := snapshotCounts(t, db) // empty DB — all zero

	rec := IntakeRecord{
		ConnectorID: "telegram:acct-1",
		SourceKind:  "message",
		SourceID:    "999:7001",
		Cursor:      "999:7001",
		FetchedAt:   time.Now().UTC(),
		Trust:       TrustExternalUntrusted,
		Raw:         []byte("Met Alex Rivera at the summit; email alex@example.com."),
		RawMIME:     "text/plain",
		Provenance:  Provenance{ConnectorID: "telegram:acct-1", AccountID: "acct-1"},
	}

	// Phase 1: Admit + Canonicalize + Chunk + Distill (P1+P2, no synthesis).
	w := NewWorker(nil, db, nil, slog.Default())
	if _, err := w.processRecord(ctx, rec); err != nil {
		t.Fatalf("processRecord: %v", err)
	}
	afterP2 := snapshotCounts(t, db)
	for _, tbl := range worldModelEntityTables {
		if afterP2[tbl] != baseline[tbl] {
			t.Errorf("table %q mutated by Admit/Canonicalize/Chunk/Distill: %d -> %d", tbl, baseline[tbl], afterP2[tbl])
		}
	}

	stored, err := store.GetIntakeRecordBySource(ctx, db, "telegram:acct-1", "999:7001")
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	chunks, _ := store.ListIntakeChunksByRecord(ctx, db, stored.ID)
	if len(chunks) == 0 {
		t.Fatal("expected chunks from P2")
	}

	// Phase 2: Score + Embed + Extract + Resolve + Fold (the non-writing P3 stages).
	scorer := score.NewHeuristicScorer()
	embedder := embed.NewStubEmbedder()
	runner := extract.NewRunner(slog.Default())
	existing := buildExistingEntities(ctx, db, 100, slog.Default())
	for _, ch := range chunks {
		_ = scorer.Score(ctx, ch)
		if emb, err := embedder.Embed(ctx, ch.Content); err == nil {
			_ = store.SaveIntakeEmbedding(ctx, db, store.IntakeEmbedding{ChunkID: ch.ID, Model: emb.Model, Dim: emb.Dim, Vector: emb.Vector})
		}
		_, _ = extractAndResolve(ctx, runner, ch, existing, slog.Default())
	}
	afterUpstreamP3 := snapshotCounts(t, db)
	for _, tbl := range worldModelEntityTables {
		if afterUpstreamP3[tbl] != baseline[tbl] {
			t.Errorf("table %q mutated by Score/Embed/Extract/Resolve: %d -> %d — only Synthesize may write",
				tbl, baseline[tbl], afterUpstreamP3[tbl])
		}
	}

	// Phase 3: Synthesize — now World Model entity tables MUST change.
	synth := synthesize.New(db, governor.MutationPipelineOptions{}, slog.Default())
	inputs := make([]synthesize.Input, 0, len(chunks))
	for _, ch := range chunks {
		ext, resn := extractAndResolve(ctx, runner, ch, existing, slog.Default())
		inputs = append(inputs, synthesize.Input{
			Chunk:       ch,
			Score:       score.Result{Score: 0.7, PromoteCandidate: true, RetentionTier: score.RetentionDurable},
			Extraction:  ext,
			Resolutions: resn,
		})
	}
	res, err := synth.Synthesize(ctx, stored, inputs, schema.JobModeDelta)
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if res.EntitiesWritten == 0 {
		t.Fatal("synthesize should have written at least one entity for this fixture")
	}
	afterSynth := snapshotCounts(t, db)

	if afterSynth["contacts"] <= baseline["contacts"] && afterSynth["memories"] <= baseline["memories"] {
		t.Errorf("Synthesize wrote no entities: contacts %d, memories %d", afterSynth["contacts"], afterSynth["memories"])
	}
	if afterSynth["entity_provenance"] <= baseline["entity_provenance"] {
		t.Errorf("Synthesize wrote no provenance: %d", afterSynth["entity_provenance"])
	}
}

// TestNoWorldModelWritesOnPolicyChange extends the isolation invariant to P5:
// reading and writing per-connector sync policy, and writing sync-log rows, must
// NOT mutate any World Model entity table. Policy is configuration, not an
// entity (frozen contract). (Backfill consent deliberately raises a Proposal —
// that path is governed and is exercised by the consent tests, not here.)
func TestNoWorldModelWritesOnPolicyChange(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	before := snapshotCounts(t, db)

	resolver := policy.NewResolver(db, nil)

	// Reads.
	_ = resolver.Resolve(ctx, "telegram:acct-1")

	// Writes: save, re-resolve, clear.
	if err := resolver.SaveOverride(ctx, schema.SyncPolicy{
		ConnectorID:  "telegram:acct-1",
		PrivacyClass: schema.PrivacyClassSensitive,
		Delta:        schema.SyncModePolicy{Cadence: "45m"},
	}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}
	_ = resolver.Resolve(ctx, "telegram:acct-1")
	if err := resolver.ClearOverride(ctx, "telegram:acct-1"); err != nil {
		t.Fatalf("ClearOverride: %v", err)
	}

	// Sync-log writes/reads.
	end := time.Now().UTC()
	if _, err := store.AppendIntakeSyncLog(ctx, db, schema.IntakeSyncLogEntry{
		ConnectorID: "telegram:acct-1", JobMode: schema.JobModeDelta,
		EndedAt: &end, RecordsAdmitted: 3, TerminalStatus: schema.SyncStatusCompleted,
	}); err != nil {
		t.Fatalf("AppendIntakeSyncLog: %v", err)
	}
	if _, err := store.ListIntakeSyncLog(ctx, db, "telegram:acct-1", 10); err != nil {
		t.Fatalf("ListIntakeSyncLog: %v", err)
	}

	after := snapshotCounts(t, db)
	for _, tbl := range worldModelEntityTables {
		if before[tbl] != after[tbl] {
			t.Errorf("World Model table %q mutated by a policy/sync-log interaction: before=%d after=%d — policy is configuration, not state",
				tbl, before[tbl], after[tbl])
		}
	}
}

// TestRetrievalIsReadOnly extends the isolation invariant to P4: hybrid retrieval
// (vector + entity-graph + recency, with trust/privacy filtering and
// retrieval-side distillation) must cause ZERO World Model writes. Synthesis
// remains the only writer.
func TestRetrievalIsReadOnly(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()

	// Drive a fixture through P1+P2+P3 so there is something to retrieve, including
	// synthesized entities and embeddings.
	driveFullPipeline(t, db, schema.JobModeDelta, e2eRecord("999:9001",
		"Met Dana Lopez about the Q3 budget review; email dana@example.com."))

	before := snapshotCounts(t, db)

	// Run retrieval several ways: plain, with a privacy ceiling, and through the
	// ContextProvider (retrieve → distill → format). None may write.
	e := embed.NewStubEmbedder()
	if _, err := retrieve.Retrieve(ctx, db, e, "Dana budget review", retrieve.Options{Limit: 5}); err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if _, err := retrieve.Retrieve(ctx, db, e, "Dana budget review", retrieve.Options{Limit: 5, MaxPrivacyClass: schema.PrivacyClassPersonal}); err != nil {
		t.Fatalf("retrieve (filtered): %v", err)
	}
	provider := &retrieve.ContextProvider{DB: db, Embedder: e}
	if _, err := provider.RetrieveContext(ctx, "Dana budget review", 800); err != nil {
		t.Fatalf("context provider: %v", err)
	}

	after := snapshotCounts(t, db)
	for _, tbl := range worldModelEntityTables {
		if before[tbl] != after[tbl] {
			t.Errorf("World Model table %q mutated by retrieval: before=%d after=%d — P4 retrieval must be read-only",
				tbl, before[tbl], after[tbl])
		}
	}
}
