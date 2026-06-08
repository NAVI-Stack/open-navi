package store

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestSaveAndGetIntakeChunk(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	// A chunk references an intake_records row (FK).
	rec := schema.IntakeRecord{
		ID:          "rec-c1",
		ConnectorID: "telegram:acct-1",
		SourceKind:  "message",
		SourceID:    "200:1",
		Trust:       schema.ContentTrustExternalUntrusted,
		Raw:         []byte("hello"),
		RawMIME:     "text/plain",
		FetchedAt:   time.Now().UTC(),
	}
	if err := SaveIntakeRecord(ctx, db, rec); err != nil {
		t.Fatalf("SaveIntakeRecord: %v", err)
	}

	c := schema.IntakeChunk{
		ID:             "chunk-1",
		IntakeRecordID: "rec-c1",
		ConnectorID:    "telegram:acct-1",
		SourceID:       "200:1",
		ChunkIndex:     0,
		Content:        "hello world",
		StartOffset:    0,
		EndOffset:      11,
		TokenEstimate:  3,
		Trust:          schema.ContentTrustExternalUntrusted,
		PrivacyClass:   schema.PrivacyClassPersonal,
		Provenance: schema.ChunkProvenance{
			ConnectorID:    "telegram:acct-1",
			SourceChunkIDs: []string{"chunk-1"},
			Quotes:         []schema.ChunkQuote{{Text: "hello world", StartOffset: 0, EndOffset: 11}},
			Entities:       []schema.ChunkEntity{{Name: "Hello", Kind: "proper_noun", StartOffset: 0, EndOffset: 5}},
		},
	}
	inserted, err := SaveIntakeChunk(ctx, db, c)
	if err != nil {
		t.Fatalf("SaveIntakeChunk: %v", err)
	}
	if !inserted {
		t.Fatal("expected first insert to report inserted=true")
	}

	got, err := GetIntakeChunk(ctx, db, "chunk-1")
	if err != nil {
		t.Fatalf("GetIntakeChunk: %v", err)
	}
	if got.IntakeRecordID != c.IntakeRecordID || got.Content != c.Content {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
	if got.ContentMIME != "text/markdown" {
		t.Errorf("ContentMIME default: want text/markdown, got %q", got.ContentMIME)
	}
	if len(got.Provenance.SourceChunkIDs) != 1 || got.Provenance.SourceChunkIDs[0] != "chunk-1" {
		t.Errorf("provenance SourceChunkIDs not preserved: %+v", got.Provenance)
	}
	if len(got.Provenance.Quotes) != 1 || got.Provenance.Quotes[0].EndOffset != 11 {
		t.Errorf("provenance Quotes not preserved: %+v", got.Provenance.Quotes)
	}
	if len(got.Provenance.Entities) != 1 || got.Provenance.Entities[0].Kind != "proper_noun" {
		t.Errorf("provenance Entities not preserved: %+v", got.Provenance.Entities)
	}
}

func TestSaveIntakeChunk_Idempotent(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	rec := schema.IntakeRecord{
		ID: "rec-c2", ConnectorID: "c", SourceKind: "message", SourceID: "s2",
		Trust: schema.ContentTrustOwner, Raw: []byte("x"), RawMIME: "text/plain", FetchedAt: time.Now().UTC(),
	}
	if err := SaveIntakeRecord(ctx, db, rec); err != nil {
		t.Fatalf("SaveIntakeRecord: %v", err)
	}

	c := schema.IntakeChunk{
		ID: "chunk-2", IntakeRecordID: "rec-c2", ConnectorID: "c", SourceID: "s2",
		ChunkIndex: 0, Content: "abc", Trust: schema.ContentTrustOwner, PrivacyClass: schema.PrivacyClassPersonal,
	}
	first, err := SaveIntakeChunk(ctx, db, c)
	if err != nil || !first {
		t.Fatalf("first save: inserted=%v err=%v", first, err)
	}
	second, err := SaveIntakeChunk(ctx, db, c)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if second {
		t.Fatal("re-saving identical chunk should be a no-op (inserted=false)")
	}

	// A different id but same (intake_record_id, chunk_index) must also be ignored.
	c2 := c
	c2.ID = "chunk-2-alt"
	dup, err := SaveIntakeChunk(ctx, db, c2)
	if err != nil {
		t.Fatalf("dup index save: %v", err)
	}
	if dup {
		t.Fatal("duplicate (record_id, chunk_index) should be ignored")
	}

	n, err := CountIntakeChunksByRecord(ctx, db, "rec-c2")
	if err != nil {
		t.Fatalf("CountIntakeChunksByRecord: %v", err)
	}
	if n != 1 {
		t.Errorf("expected exactly 1 chunk, got %d", n)
	}
}

func TestListIntakeChunksByRecord(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	rec := schema.IntakeRecord{
		ID: "rec-c3", ConnectorID: "c", SourceKind: "message", SourceID: "s3",
		Trust: schema.ContentTrustOwner, Raw: []byte("x"), RawMIME: "text/plain", FetchedAt: time.Now().UTC(),
	}
	if err := SaveIntakeRecord(ctx, db, rec); err != nil {
		t.Fatalf("SaveIntakeRecord: %v", err)
	}
	for i := 0; i < 3; i++ {
		c := schema.IntakeChunk{
			ID: "rec-c3-" + string(rune('a'+i)), IntakeRecordID: "rec-c3", ConnectorID: "c", SourceID: "s3",
			ChunkIndex: i, Content: "chunk", Trust: schema.ContentTrustOwner, PrivacyClass: schema.PrivacyClassPersonal,
		}
		if _, err := SaveIntakeChunk(ctx, db, c); err != nil {
			t.Fatalf("SaveIntakeChunk %d: %v", i, err)
		}
	}
	got, err := ListIntakeChunksByRecord(ctx, db, "rec-c3")
	if err != nil {
		t.Fatalf("ListIntakeChunksByRecord: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(got))
	}
	for i, c := range got {
		if c.ChunkIndex != i {
			t.Errorf("chunk %d out of order: index=%d", i, c.ChunkIndex)
		}
	}
}
