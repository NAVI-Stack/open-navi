package intake

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/intake/extract"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

func e2eRecord(sourceID, body string) IntakeRecord {
	return IntakeRecord{
		ConnectorID:  "telegram:acct-1",
		SourceKind:   "message",
		SourceID:     sourceID,
		Cursor:       sourceID,
		FetchedAt:    time.Now().UTC(),
		Trust:        TrustExternalUntrusted,
		Raw:          []byte(body),
		RawMIME:      "text/plain",
		Provenance:   Provenance{ConnectorID: "telegram:acct-1", AccountID: "acct-1"},
	}
}

// driveFullPipeline runs P1 (admit) + P2 (canon/chunk/distill) + P3 (score/embed/
// extract/resolve/synthesize/fold) for one record via the worker.
func driveFullPipeline(t *testing.T, db *sql.DB, jobMode schema.JobMode, rec IntakeRecord) {
	t.Helper()
	w := NewWorker(nil, db, nil, slog.Default())
	w.SetPipeline(PipelineConfig{
		Synthesis: NewSynthesisConfig(db, governor.MutationPipelineOptions{}, jobMode, slog.Default()),
	})
	if _, err := w.processRecord(context.Background(), rec); err != nil {
		t.Fatalf("processRecord: %v", err)
	}
}

// TestE2E_EntitiesWithProvenanceAndConfidence drives a fixture record through the
// full P1+P2+P3 pipeline and asserts entities appear with provenance + confidence.
func TestE2E_EntitiesWithProvenanceAndConfidence(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()

	driveFullPipeline(t, db, schema.JobModeDelta, e2eRecord("999:2001",
		"Had a great chat with Alex Rivera. Email alex@example.com about the proposal."))

	contacts, _ := store.ListContacts(ctx, db, store.ListContactsFilter{Limit: 50})
	if len(contacts) == 0 {
		t.Fatal("expected synthesized contacts from the fixture")
	}
	// Every synthesized contact must carry provenance with a confidence in [0,1]
	// and a link back to the source record.
	for _, c := range contacts {
		ep, err := store.GetEntityProvenance(ctx, db, "contact", c.ID)
		if err != nil || ep == nil {
			t.Fatalf("contact %s has no provenance: %v", c.Name, err)
		}
		if ep.Confidence <= 0 || ep.Confidence > 1 {
			t.Errorf("contact %s confidence out of range: %v", c.Name, ep.Confidence)
		}
		links, _ := store.ListIntakeProvenanceByEntity(ctx, db, "contact", c.ID)
		if len(links) == 0 {
			t.Errorf("contact %s has no intake provenance link", c.Name)
			continue
		}
		if links[0].SourceID != "999:2001" {
			t.Errorf("contact %s provenance missing source id: %+v", c.Name, links[0])
		}
	}
	// An embedding reference should have been persisted for the chunk(s).
	if n, _ := store.CountIntakeEmbeddings(ctx, db); n == 0 {
		t.Error("expected at least one persisted embedding reference")
	}
}

// TestE2E_MergeCandidateRaisesProposal seeds duplicate contacts sharing an email,
// then ingests a record referencing that email so resolution yields a confident
// merge, which must raise a Proposal rather than writing.
func TestE2E_MergeCandidateRaisesProposal(t *testing.T) {
	r := extract.NewRunner(nil)
	if !r.Available() {
		t.Skip("python worker unavailable; merge path relies on resolver")
	}
	db := store.InitTestDB(t)
	ctx := context.Background()

	// Two duplicate contacts sharing the same deterministic blocking key (email).
	_ = store.SaveContact(ctx, db, schema.Contact{ID: "dup-1", Name: "Dup Person", Kind: "person",
		Metadata: `{"email":"dup@example.com"}`})
	_ = store.SaveContact(ctx, db, schema.Contact{ID: "dup-2", Name: "Duplicate Person", Kind: "person",
		Metadata: `{"email":"dup@example.com"}`})

	driveFullPipeline(t, db, schema.JobModeDelta, e2eRecord("999:3001",
		"Reminder: reach dup@example.com tomorrow."))

	pending, _ := store.ListPendingProposals(ctx, db, 50)
	var merge bool
	for _, p := range pending {
		if p.SourceProcess == "intake_synthesis" {
			merge = true
		}
	}
	if !merge {
		t.Fatalf("expected a merge proposal from the duplicate-email resolution; pending=%+v", pending)
	}
}

// TestE2E_ReplayIdempotent runs the same record through the full pipeline twice
// and asserts zero duplicate entities and zero duplicate proposals.
func TestE2E_ReplayIdempotent(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	rec := e2eRecord("999:4001", "Spoke with Jordan Lee about logistics. Ping @jordanl later.")

	driveFullPipeline(t, db, schema.JobModeDelta, rec)
	contacts1, _ := store.ListContacts(ctx, db, store.ListContactsFilter{Limit: 50})
	mems1, _ := store.ListMemories(ctx, db, "intake", "telegram:acct-1", 50)
	props1, _ := store.ListPendingProposals(ctx, db, 50)

	// Replay the identical record (cursor rewind / restart).
	driveFullPipeline(t, db, schema.JobModeDelta, rec)
	contacts2, _ := store.ListContacts(ctx, db, store.ListContactsFilter{Limit: 50})
	mems2, _ := store.ListMemories(ctx, db, "intake", "telegram:acct-1", 50)
	props2, _ := store.ListPendingProposals(ctx, db, 50)

	if len(contacts2) != len(contacts1) {
		t.Errorf("replay produced duplicate contacts: %d -> %d", len(contacts1), len(contacts2))
	}
	if len(mems2) != len(mems1) {
		t.Errorf("replay produced duplicate memories: %d -> %d", len(mems1), len(mems2))
	}
	if len(props2) != len(props1) {
		t.Errorf("replay produced duplicate proposals: %d -> %d", len(props1), len(props2))
	}
}

// TestE2E_BackfillGroupsProposals drives a backfill batch whose record references
// two emails, each shared by a duplicate-contact pair, and asserts the two
// confident merges are grouped into a single Proposal (synthesis seam §12), not
// one per merge.
func TestE2E_BackfillGroupsProposals(t *testing.T) {
	runner := extract.NewRunner(nil)
	if !runner.Available() {
		t.Skip("python worker unavailable; backfill grouping relies on resolver")
	}
	db := store.InitTestDB(t)
	ctx := context.Background()

	// Two duplicate pairs, each pair sharing a deterministic email key.
	_ = store.SaveContact(ctx, db, schema.Contact{ID: "a1", Name: "Alex Rivera", Kind: "person", Metadata: `{"email":"alex@example.com"}`})
	_ = store.SaveContact(ctx, db, schema.Contact{ID: "a2", Name: "A. Rivera", Kind: "person", Metadata: `{"email":"alex@example.com"}`})
	_ = store.SaveContact(ctx, db, schema.Contact{ID: "b1", Name: "Sam Doe", Kind: "person", Metadata: `{"email":"sam@example.com"}`})
	_ = store.SaveContact(ctx, db, schema.Contact{ID: "b2", Name: "Samuel Doe", Kind: "person", Metadata: `{"email":"sam@example.com"}`})

	driveFullPipeline(t, db, schema.JobModeBackfill, e2eRecord("999:6001",
		"Backfill: contacts alex@example.com and sam@example.com both need follow-up."))

	pending, _ := store.ListPendingProposals(ctx, db, 50)
	var grouped int
	for _, p := range pending {
		if p.SourceProcess == "intake_synthesis" {
			grouped++
			if !containsStr(p.Rationale, "review as a set") {
				t.Errorf("backfill proposal not grouped: %q", p.Rationale)
			}
		}
	}
	if grouped != 1 {
		t.Fatalf("backfill should group the two merges into exactly one proposal, got %d", grouped)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestPythonNeverWritesWorldModel monitors all intake/entity table counts before
// and after running ONLY the Python extraction+resolution stage. Python produces
// envelopes over stdio and holds no DB handle, so nothing in SQLite may change.
func TestPythonNeverWritesWorldModel(t *testing.T) {
	runner := extract.NewRunner(nil)
	if !runner.Available() {
		t.Skip("python worker unavailable")
	}
	db := store.InitTestDB(t)
	ctx := context.Background()

	// Seed an existing contact so resolution has a pool to match against.
	_ = store.SaveContact(ctx, db, schema.Contact{ID: "c-seed", Name: "Alex Rivera", Kind: "person",
		Metadata: `{"email":"alex@example.com"}`})

	// Run P1+P2 (no synthesis) to land chunks.
	w := NewWorker(nil, db, nil, slog.Default())
	if _, err := w.processRecord(ctx, e2eRecord("999:5001",
		"Met Alex Rivera (alex@example.com) and @alexr today.")); err != nil {
		t.Fatalf("processRecord: %v", err)
	}
	recID, err := store.GetIntakeRecordBySource(ctx, db, "telegram:acct-1", "999:5001")
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	chunks, _ := store.ListIntakeChunksByRecord(ctx, db, recID.ID)
	if len(chunks) == 0 {
		t.Fatal("expected chunks from P2")
	}

	existing := buildExistingEntities(ctx, db, 100, slog.Default())

	before := snapshotAllTables(t, db)
	for _, ch := range chunks {
		res, err := runner.Process(ctx, extract.ProcessRequest{
			ChunkID: ch.ID, Content: ch.Content, ExistingEntities: existing,
		})
		if err != nil {
			t.Fatalf("python process: %v", err)
		}
		if len(res.Extraction.Candidates) == 0 {
			t.Fatalf("extraction produced no candidates for chunk %s", ch.ID)
		}
	}
	after := snapshotAllTables(t, db)

	for table, n := range before {
		if after[table] != n {
			t.Errorf("Python stage mutated table %q: %d -> %d — Python must never write", table, n, after[table])
		}
	}
}

// --- helpers ---------------------------------------------------------------

// snapshotAllTables returns row counts for every table in the schema (a superset
// of the World Model entity tables), so a stray write anywhere is detected.
func snapshotAllTables(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, name)
	}
	out := make(map[string]int, len(tables))
	for _, tbl := range tables {
		out[tbl] = tableRowCount(t, db, tbl)
	}
	return out
}
