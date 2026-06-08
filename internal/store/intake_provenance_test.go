package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

// seedRecordChunk inserts a minimal intake_records + intake_chunks pair so FK
// constraints (e.g. intake_embeddings → intake_chunks) are satisfied in tests.
func seedRecordChunk(t *testing.T, db *sql.DB, recordID, chunkID string) {
	t.Helper()
	ctx := context.Background()
	rec := schema.IntakeRecord{
		ID:           recordID,
		ConnectorID:  "telegram:acct-1",
		SourceKind:   "message",
		SourceID:     "999:1001",
		FetchedAt:    time.Now().UTC(),
		Trust:        schema.ContentTrustExternalUntrusted,
		PrivacyClass: schema.PrivacyClassPersonal,
		Raw:          []byte("hello"),
		RawMIME:      "text/plain",
	}
	if err := SaveIntakeRecord(ctx, db, rec); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	if _, err := SaveIntakeChunk(ctx, db, schema.IntakeChunk{
		ID:             chunkID,
		IntakeRecordID: recordID,
		ConnectorID:    "telegram:acct-1",
		SourceID:       "999:1001",
		Content:        "hello",
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed chunk: %v", err)
	}
}

func TestIntakeProvenance_RoundtripAndIdempotency(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	link := IntakeProvenanceLink{
		ID:             "prov-1",
		IntakeRecordID: "rec-1",
		ChunkID:        "chunk-1",
		ConnectorID:    "telegram:acct-1",
		SourceID:       "999:1001",
		Cursor:         "999:1001",
		FetchedAt:      time.Now().UTC(),
		EntityType:     "contact",
		EntityID:       "contact-1",
		MutationKind:   "create_entity",
		Outcome:        "approved",
		DerivationKey:  "deriv-key-abc",
		Confidence:     0.72,
	}

	inserted, err := SaveIntakeProvenanceLink(ctx, db, link)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !inserted {
		t.Fatal("first save should insert")
	}

	// Replay: same derivation_key must be a no-op.
	link.ID = "prov-2"
	inserted, err = SaveIntakeProvenanceLink(ctx, db, link)
	if err != nil {
		t.Fatalf("replay save: %v", err)
	}
	if inserted {
		t.Fatal("replay with same derivation_key must not insert")
	}

	exists, err := IntakeProvenanceExists(ctx, db, "deriv-key-abc")
	if err != nil || !exists {
		t.Fatalf("exists: got %v, %v", exists, err)
	}

	byRecord, err := ListIntakeProvenanceByRecord(ctx, db, "rec-1")
	if err != nil {
		t.Fatalf("by record: %v", err)
	}
	if len(byRecord) != 1 {
		t.Fatalf("want 1 link for record, got %d", len(byRecord))
	}
	if byRecord[0].Confidence != 0.72 || byRecord[0].EntityID != "contact-1" {
		t.Errorf("roundtrip mismatch: %+v", byRecord[0])
	}

	byEntity, err := ListIntakeProvenanceByEntity(ctx, db, "contact", "contact-1")
	if err != nil {
		t.Fatalf("by entity: %v", err)
	}
	if len(byEntity) != 1 {
		t.Fatalf("want 1 link for entity, got %d", len(byEntity))
	}
}

func TestIntakeEmbedding_Upsert(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	seedRecordChunk(t, db, "rec-1", "chunk-1")

	e := IntakeEmbedding{ChunkID: "chunk-1", Model: "stub-hash-v1", Dim: 3, Vector: []float64{0.1, 0.2, 0.3}}
	if err := SaveIntakeEmbedding(ctx, db, e); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Re-embed same chunk: upsert, not duplicate.
	e.Vector = []float64{0.4, 0.5, 0.6}
	if err := SaveIntakeEmbedding(ctx, db, e); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	got, ok, err := GetIntakeEmbedding(ctx, db, "chunk-1")
	if err != nil || !ok {
		t.Fatalf("get: %v %v", ok, err)
	}
	if len(got.Vector) != 3 || got.Vector[0] != 0.4 {
		t.Errorf("upsert mismatch: %+v", got)
	}
	n, err := CountIntakeEmbeddings(ctx, db)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("want 1 embedding row after upsert, got %d", n)
	}
}
