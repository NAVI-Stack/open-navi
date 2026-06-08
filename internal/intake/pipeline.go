package intake

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/open-navi/navi/internal/intake/canonicalize"
	"github.com/open-navi/navi/internal/intake/chunk"
	"github.com/open-navi/navi/internal/intake/distill"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// IngestDistillFn is the ingest-side reference to the distillation primitive
// (CIP stage 5). P4 retrieval shares the SAME function via retrieve.DistillFn;
// a test asserts both resolve to distill.Distill, proving "one implementation,
// two call sites" (the entire point of P2's dual-use interface).
var IngestDistillFn = distill.Distill

// PipelineMetrics is the per-record telemetry emitted by the downstream stages
// (Canonicalize → Chunk → Distill → persist). Surfaced at slog level for P2;
// a Console surface lands in P5.
type PipelineMetrics struct {
	RecordID       string
	CanonicalBytes int
	ChunksProduced int // distill survivors
	Persisted      int // new rows written to intake_chunks
	Synthesized    int // World Model entities written by synthesis (P3); 0 when synthesis is off
	distill.Metrics
}

// PipelineConfig tunes the downstream stages. Zero values fall back to the
// stage defaults (≈3k-token chunks, 200-token overlap, extractive-only distill).
type PipelineConfig struct {
	Chunk   chunk.Options
	Distill distill.Options
	// Synthesis, when non-nil, enables the P3 stages (Score → Embed → Extract →
	// Resolve → Synthesize → Fold) after Distill. Nil preserves the exact P2-only
	// behavior (no World Model writes), so existing P1/P2 callers are unaffected.
	Synthesis *SynthesisConfig
}

// RunStages drives a stored IntakeRecord through Canonicalize → Chunk → Distill
// and persists the surviving chunks to intake_chunks. It is idempotent: stable
// chunk ids + INSERT OR IGNORE mean re-running for the same record produces no
// duplicate rows. rec.ID must be the persisted intake_records.id (used as the
// chunk FK). No World Model tables are touched; no LLM calls are made (the
// extractive default uses no Summarizer).
func RunStages(ctx context.Context, db *sql.DB, rec schema.IntakeRecord, cfg PipelineConfig, log *slog.Logger) (PipelineMetrics, error) {
	if log == nil {
		log = slog.Default()
	}
	m := PipelineMetrics{RecordID: rec.ID}

	doc, err := canonicalize.Canonicalize(rec)
	if err != nil {
		return m, fmt.Errorf("intake: canonicalize: %w", err)
	}
	m.CanonicalBytes = len(doc.Markdown)

	namespace := rec.ConnectorID + "\x00" + rec.SourceID
	chunks := chunk.Split(doc.Markdown, namespace, cfg.Chunk)
	if len(chunks) == 0 {
		log.Debug("intake: pipeline: no chunks produced (empty canonical content)",
			"record_id", rec.ID, "connector_id", rec.ConnectorID, "source_id", rec.SourceID)
		return m, nil
	}

	prov := distill.Provenance{
		ConnectorID: rec.ConnectorID,
		SourceID:    rec.SourceID,
		AccountID:   rec.Provenance.AccountID,
		LinkBack:    rec.Provenance.LinkBack,
		Author:      rec.Author,
		SourceKind:  rec.SourceKind,
	}
	pchunks := make([]distill.ProvenancedChunk, len(chunks))
	for i, c := range chunks {
		pchunks[i] = distill.ProvenancedChunk{
			ChunkID:     c.ID,
			RecordID:    rec.ID,
			Index:       c.Index,
			Content:     c.Content,
			StartOffset: c.StartOffset,
			EndOffset:   c.EndOffset,
			Provenance:  prov,
		}
	}

	result := IngestDistillFn(ctx, distill.FromChunks(pchunks, cfg.Distill))
	m.Metrics = result.Metrics
	m.ChunksProduced = len(result.Chunks)

	now := time.Now().UTC()
	schemaChunks := make([]schema.IntakeChunk, 0, len(result.Chunks))
	for _, dc := range result.Chunks {
		ic := toSchemaChunk(rec, dc, now)
		inserted, err := store.SaveIntakeChunk(ctx, db, ic)
		if err != nil {
			return m, fmt.Errorf("intake: persist chunk %s: %w", dc.ChunkID, err)
		}
		if inserted {
			m.Persisted++
		}
		schemaChunks = append(schemaChunks, ic)
	}

	// Stages 6–9 (P3): Score → Embed → Extract → Resolve → Synthesize → Fold.
	// Disabled (and thus zero World Model writes) when cfg.Synthesis is nil.
	if cfg.Synthesis != nil {
		synthesized, err := RunSynthesis(ctx, db, rec, schemaChunks, cfg.Synthesis, log)
		if err != nil {
			return m, fmt.Errorf("intake: synthesis: %w", err)
		}
		m.Synthesized = synthesized
	}

	log.Info("intake: pipeline: distilled record",
		"record_id", rec.ID,
		"connector_id", rec.ConnectorID,
		"source_id", rec.SourceID,
		"chunks_produced", m.ChunksProduced,
		"persisted", m.Persisted,
		"duplicates", m.Duplicates,
		"dedupe_rate", m.DedupeRate,
		"input_bytes", m.InputBytes,
		"output_bytes", m.OutputBytes,
		"quotes", m.Quotes,
		"entities", m.Entities,
	)
	return m, nil
}

func toSchemaChunk(rec schema.IntakeRecord, dc distill.DistilledChunk, now time.Time) schema.IntakeChunk {
	cp := schema.ChunkProvenance{
		ConnectorID:       rec.ConnectorID,
		AccountID:         rec.Provenance.AccountID,
		LinkBack:          rec.Provenance.LinkBack,
		Author:            rec.Author,
		SourceKind:        rec.SourceKind,
		FetchedAt:         rec.FetchedAt,
		SourceChunkIDs:    dc.SourceChunkIDs,
		CollapsedChunkIDs: dc.CollapsedIDs,
	}
	for _, q := range dc.Quotes {
		cp.Quotes = append(cp.Quotes, schema.ChunkQuote{
			Text: q.Text, StartOffset: q.StartOffset, EndOffset: q.EndOffset,
		})
	}
	for _, e := range dc.Entities {
		cp.Entities = append(cp.Entities, schema.ChunkEntity{
			Name: e.Name, Kind: e.Kind, StartOffset: e.StartOffset, EndOffset: e.EndOffset,
		})
	}
	return schema.IntakeChunk{
		ID:             dc.ChunkID,
		IntakeRecordID: rec.ID,
		ConnectorID:    rec.ConnectorID,
		SourceID:       rec.SourceID,
		ChunkIndex:     dc.Index,
		Content:        dc.Content,
		ContentMIME:    "text/markdown",
		StartOffset:    dc.StartOffset,
		EndOffset:      dc.EndOffset,
		TokenEstimate:  chunk.EstimateTokens(dc.Content),
		Trust:          rec.Trust,
		PrivacyClass:   rec.PrivacyClass,
		Provenance:     cp,
		CreatedAt:      now,
	}
}
