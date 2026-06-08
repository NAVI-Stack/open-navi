package intake

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/store"
)

// fixtureRecord is a realistic Telegram-style message containing a quoted reply,
// a signature, duplicated content (to exercise dedupe), a URL with tracking
// params, and named entities.
func fixtureRecord() IntakeRecord {
	body := strings.Join([]string{
		"Hey team, sharing the launch notes for Project Apollo.",
		"",
		"The quarterly report shows revenue grew by twelve percent this period.",
		"",
		"The quarterly report shows revenue grew by twelve percent this period.",
		"",
		`Alice Johnson said "we should ship by Friday" — see https://Example.com/launch?utm_source=tg&id=7#top`,
		"",
		"On Mon, May 5 2026, Bob wrote:",
		"> please review the earlier draft",
		"",
		"-- ",
		"Sent from my phone",
	}, "\n")
	return IntakeRecord{
		ConnectorID:  "telegram:acct-1",
		SourceKind:   "message",
		SourceID:     "555:42",
		Cursor:       "555:42",
		FetchedAt:    time.Now().UTC(),
		Trust:        TrustExternalUntrusted,
		PrivacyClass: PrivacyPersonal,
		Author:       "alice",
		Raw:          []byte(body),
		RawMIME:      "text/plain",
		Provenance: Provenance{
			ConnectorID: "telegram:acct-1",
			AccountID:   "acct-1",
			LinkBack:    "https://t.me/c/555/42",
		},
	}
}

func TestPipeline_EndToEnd_ProducesProvenancedChunks(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	w := NewWorker(nil, db, nil, nil)

	r := fixtureRecord()
	outcome, err := w.processRecord(ctx, r)
	if err != nil {
		t.Fatalf("processRecord: %v", err)
	}
	if outcome != outcomeAdmitted {
		t.Fatalf("outcome: want admitted, got %v", outcome)
	}

	rec, err := store.GetIntakeRecordBySource(ctx, db, r.ConnectorID, r.SourceID)
	if err != nil {
		t.Fatalf("GetIntakeRecordBySource: %v", err)
	}
	chunks, err := store.ListIntakeChunksByRecord(ctx, db, rec.ID)
	if err != nil {
		t.Fatalf("ListIntakeChunksByRecord: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one persisted chunk")
	}

	for i, c := range chunks {
		// Provenance: every chunk links back to its source record.
		if c.IntakeRecordID != rec.ID {
			t.Errorf("chunk %d not linked to source record", i)
		}
		if c.ConnectorID != r.ConnectorID || c.SourceID != r.SourceID {
			t.Errorf("chunk %d missing denormalized provenance", i)
		}
		if len(c.Provenance.SourceChunkIDs) == 0 {
			t.Errorf("chunk %d missing SourceChunkIDs provenance link", i)
		}
		if c.Provenance.LinkBack != "https://t.me/c/555/42" {
			t.Errorf("chunk %d missing link-back provenance: %q", i, c.Provenance.LinkBack)
		}
		if c.Trust != r.Trust || c.PrivacyClass != r.PrivacyClass {
			t.Errorf("chunk %d trust/privacy not carried", i)
		}
		if c.ContentMIME != "text/markdown" {
			t.Errorf("chunk %d content mime: %q", i, c.ContentMIME)
		}
	}

	// Boilerplate stripped: quoted reply and signature must not survive.
	all := ""
	for _, c := range chunks {
		all += c.Content + "\n"
	}
	for _, junk := range []string{"please review the earlier draft", "Sent from my phone", "Bob wrote"} {
		if strings.Contains(all, junk) {
			t.Errorf("boilerplate %q leaked into chunks:\n%s", junk, all)
		}
	}
	// URL normalized in retained content.
	if strings.Contains(all, "utm_source") || strings.Contains(all, "#top") {
		t.Errorf("URL not normalized in chunk content:\n%s", all)
	}

	// Re-processing the same record is idempotent (resumable): dedupe outcome and
	// no new chunk rows.
	o2, err := w.processRecord(ctx, r)
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if o2 != outcomeDeduped {
		t.Errorf("reprocess outcome: want deduped, got %v", o2)
	}
	after, _ := store.ListIntakeChunksByRecord(ctx, db, rec.ID)
	if len(after) != len(chunks) {
		t.Errorf("re-processing produced duplicate chunks: before=%d after=%d", len(chunks), len(after))
	}
}

func TestPipeline_DedupeCollapsesRepeatedContent(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()

	r := fixtureRecord()
	r.ID = "rec-dedupe"
	if err := store.SaveIntakeRecord(ctx, db, r); err != nil {
		t.Fatalf("SaveIntakeRecord: %v", err)
	}
	// Force one-paragraph-per-chunk so the duplicated paragraph becomes its own
	// chunk and can be collapsed.
	cfg := PipelineConfig{}
	cfg.Chunk.MaxTokens = 24
	cfg.Chunk.OverlapTokens = 0
	m, err := RunStages(ctx, db, r, cfg, nil)
	if err != nil {
		t.Fatalf("RunStages: %v", err)
	}
	if m.Duplicates == 0 {
		t.Errorf("expected the repeated paragraph to be collapsed; metrics=%+v", m)
	}
	if m.InputBytes == 0 || m.OutputBytes == 0 {
		t.Errorf("expected non-zero byte metrics; got %+v", m)
	}
}

// worldModelEntityTablesP2 mirrors the P1 isolation list: P2's downstream stages
// must not write any World Model entity table either.
var worldModelEntityTablesP2 = []string{
	"contacts", "knowledge", "memories", "memory_links", "artifacts",
	"artifact_versions", "artifact_branches", "wm_events", "proposals",
	"entity_provenance", "entity_relationships",
}

func TestPipeline_NoWorldModelWrites(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()

	snap := func() map[string]int {
		out := map[string]int{}
		for _, tbl := range worldModelEntityTablesP2 {
			var n int
			if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM "+tbl).Scan(&n); err == nil {
				out[tbl] = n
			}
		}
		return out
	}

	before := snap()
	w := NewWorker(nil, db, nil, nil)
	if _, err := w.processRecord(ctx, fixtureRecord()); err != nil {
		t.Fatalf("processRecord: %v", err)
	}
	after := snap()
	for _, tbl := range worldModelEntityTablesP2 {
		if before[tbl] != after[tbl] {
			t.Errorf("World Model table %q mutated by P2: before=%d after=%d", tbl, before[tbl], after[tbl])
		}
	}
}

// TestPipeline_NoLLMDependency proves the P2 pipeline carries no LLM coupling:
// none of its packages import internal/llm (directly or transitively). This is
// the structural guarantee behind "no LLM provider calls from P2" — the
// extractive pipeline literally cannot reach a provider.
func TestPipeline_NoLLMDependency(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	pkgs := []string{
		"github.com/open-navi/navi/internal/intake/canonicalize",
		"github.com/open-navi/navi/internal/intake/chunk",
		"github.com/open-navi/navi/internal/intake/distill",
	}
	for _, pkg := range pkgs {
		out, err := exec.Command("go", "list", "-deps", pkg).Output()
		if err != nil {
			t.Fatalf("go list -deps %s: %v", pkg, err)
		}
		for _, dep := range strings.Split(string(out), "\n") {
			if strings.TrimSpace(dep) == "github.com/open-navi/navi/internal/llm" {
				t.Errorf("%s must not depend on internal/llm (found in dep graph)", pkg)
			}
		}
	}
}
