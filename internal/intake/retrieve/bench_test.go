package retrieve

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/intake/embed"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

// BenchmarkRetrieve10kChunks measures end-to-end hybrid retrieval latency at the
// V1 target volume (10k chunks) so the handoff can cite a real number. The cost
// is dominated by the brute-force cosine scan plus the entity-graph lookups.
func BenchmarkRetrieve10kChunks(b *testing.B) {
	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := store.CreateTables(ctx, db); err != nil {
		b.Fatalf("create tables: %v", err)
	}
	e := embed.NewStubEmbedder()
	now := time.Now().UTC()

	const n = 10000
	words := []string{"budget", "review", "meeting", "launch", "plan", "schedule", "contact", "report", "design", "summary"}
	recordID := "bench-record"
	if err := store.SaveIntakeRecord(ctx, db, schema.IntakeRecord{
		ID: recordID, ConnectorID: "bench:1", SourceKind: "message", SourceID: "bench",
		Cursor: "bench", FetchedAt: now, Trust: schema.ContentTrustOwner,
		PrivacyClass: schema.PrivacyClassPersonal, Raw: []byte("x"), RawMIME: "text/plain", CreatedAt: now,
	}); err != nil {
		b.Fatalf("save record: %v", err)
	}
	for i := 0; i < n; i++ {
		content := fmt.Sprintf("%s %s notes %d for the %s and %s",
			words[i%len(words)], words[(i*3)%len(words)], i, words[(i*7)%len(words)], words[(i*5)%len(words)])
		chunkID := fmt.Sprintf("chunk-%d", i)
		if _, err := store.SaveIntakeChunk(ctx, db, schema.IntakeChunk{
			ID: chunkID, IntakeRecordID: recordID, ConnectorID: "bench:1", SourceID: "bench",
			ChunkIndex: i, Content: content, ContentMIME: "text/markdown",
			Trust: schema.ContentTrustOwner, PrivacyClass: schema.PrivacyClassPersonal,
			Provenance: schema.ChunkProvenance{ConnectorID: "bench:1", FetchedAt: now}, CreatedAt: now,
		}); err != nil {
			b.Fatalf("save chunk: %v", err)
		}
		emb, _ := e.Embed(ctx, content)
		if err := store.SaveIntakeEmbedding(ctx, db, store.IntakeEmbedding{
			ChunkID: chunkID, Model: emb.Model, Dim: emb.Dim, Vector: emb.Vector,
		}); err != nil {
			b.Fatalf("save embedding: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Retrieve(ctx, db, e, "budget review schedule", Options{Limit: 10, MaxChunkScan: n}); err != nil {
			b.Fatalf("retrieve: %v", err)
		}
	}
}
