package intake

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/intake/embed"
	"github.com/ceoai/navi/internal/intake/extract"
	"github.com/ceoai/navi/internal/intake/fold"
	"github.com/ceoai/navi/internal/intake/score"
	"github.com/ceoai/navi/internal/intake/synthesize"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

// SynthesisConfig carries the P3 stage dependencies. It is assembled once at
// startup (cmd/navid) and reused per record. Any field left nil degrades that
// stage honestly: a nil Extractor falls back to the deterministic entity spans
// P2 already recorded on each chunk; nil Scorer/Embedder default to the V1
// backends.
type SynthesisConfig struct {
	Scorer      score.Scorer
	Embedder    embed.Embedder
	Extractor   *extract.Runner
	Synthesizer *synthesize.Synthesizer
	Folder      *fold.Folder
	JobMode     schema.JobMode
	// MaxExistingEntities bounds the resolution candidate pool (personal-assistant
	// scale). Zero defaults to 500.
	MaxExistingEntities int
}

// NewSynthesisConfig builds a SynthesisConfig with the V1 deterministic backends
// (heuristic scorer, stub embedder) and a Synthesizer wired to the governor.
// The Extractor is located lazily; when Python is unavailable the pipeline falls
// back to P2's deterministic spans.
func NewSynthesisConfig(db *sql.DB, opts governor.MutationPipelineOptions, jobMode schema.JobMode, log *slog.Logger) *SynthesisConfig {
	if log == nil {
		log = slog.Default()
	}
	if jobMode == "" {
		jobMode = schema.JobModeDelta
	}
	return &SynthesisConfig{
		Scorer:      score.NewHeuristicScorer(),
		Embedder:    embed.NewStubEmbedder(),
		Extractor:   extract.NewRunner(log),
		Synthesizer: synthesize.New(db, opts, log),
		Folder:      fold.New(db, log),
		JobMode:     jobMode,
	}
}

// RunSynthesis drives the P3 stages over a record's persisted chunks: Score,
// Embed (persist reference), Extract + Resolve (Python, with deterministic
// fallback), Synthesize (governed World Model writes), then Fold (derived index).
// It never deletes chunks — un-promoted chunks remain retrievable (seam §14).
func RunSynthesis(ctx context.Context, db *sql.DB, rec schema.IntakeRecord, chunks []schema.IntakeChunk, cfg *SynthesisConfig, log *slog.Logger) (int, error) {
	if cfg == nil || len(chunks) == 0 {
		return 0, nil
	}
	if log == nil {
		log = slog.Default()
	}
	scorer := cfg.Scorer
	if scorer == nil {
		scorer = score.NewHeuristicScorer()
	}

	existing := buildExistingEntities(ctx, db, cfg.MaxExistingEntities, log)

	inputs := make([]synthesize.Input, 0, len(chunks))
	for _, ch := range chunks {
		sc := scorer.Score(ctx, ch)

		// Embed (stage 7): compute + persist the embedding reference. Non-fatal.
		if cfg.Embedder != nil {
			if emb, err := cfg.Embedder.Embed(ctx, ch.Content); err == nil {
				if err := store.SaveIntakeEmbedding(ctx, db, store.IntakeEmbedding{
					ChunkID: ch.ID, Model: emb.Model, Dim: emb.Dim, Vector: emb.Vector,
				}); err != nil {
					log.Warn("intake: synthesis: persist embedding failed", "err", err, "chunk_id", ch.ID)
				}
			}
		}

		extraction, resolutions := extractAndResolve(ctx, cfg.Extractor, ch, existing, log)
		inputs = append(inputs, synthesize.Input{
			Chunk: ch, Score: sc, Extraction: extraction, Resolutions: resolutions,
		})
	}

	if cfg.Synthesizer == nil {
		return 0, nil
	}
	jobMode := cfg.JobMode
	if jobMode == "" {
		jobMode = schema.JobModeDelta
	}
	result, err := cfg.Synthesizer.Synthesize(ctx, rec, inputs, jobMode)
	if err != nil {
		return 0, err
	}

	if cfg.Folder != nil {
		entries := make([]fold.Entry, 0, len(result.Outcomes))
		for _, o := range result.Outcomes {
			if o.EntityID == "" || o.Skipped {
				continue
			}
			entries = append(entries, fold.Entry{EntityType: o.EntityType, EntityID: o.EntityID})
		}
		if _, err := cfg.Folder.Fold(ctx, entries); err != nil {
			log.Warn("intake: synthesis: fold failed", "err", err)
		}
	}

	log.Info("intake: synthesis: pass complete",
		"record_id", rec.ID, "connector_id", rec.ConnectorID,
		"entities_written", result.EntitiesWritten, "proposals_raised", result.ProposalsRaised,
		"modified", result.Modified, "rejected", result.Rejected, "skipped", result.Skipped,
		"job_mode", jobMode)
	return result.EntitiesWritten, nil
}

// extractAndResolve runs the Python extraction+resolution worker for one chunk,
// falling back to the deterministic entity spans P2 recorded on the chunk when
// the worker is unavailable or errors (honest degradation, never silent success).
func extractAndResolve(ctx context.Context, runner *extract.Runner, ch schema.IntakeChunk, existing []extract.ExistingEntity, log *slog.Logger) (schema.ExtractionResult, []schema.ResolutionResult) {
	if runner != nil && runner.Available() {
		res, err := runner.Process(ctx, extract.ProcessRequest{
			ChunkID: ch.ID, Content: ch.Content, ExistingEntities: existing,
		})
		if err == nil {
			return res.Extraction, res.Resolutions
		}
		log.Warn("intake: synthesis: python extractor failed, falling back to P2 spans", "err", err, "chunk_id", ch.ID)
	}
	return fallbackExtraction(ch)
}

// fallbackExtraction reuses the deterministic entity spans P2 already extracted
// (schema.ChunkEntity) when the Python worker is unavailable. Resolutions are
// empty (every candidate is treated as a new entity / Create) — honest, not a
// guess.
func fallbackExtraction(ch schema.IntakeChunk) (schema.ExtractionResult, []schema.ResolutionResult) {
	ext := schema.ExtractionResult{ChunkID: ch.ID}
	for _, e := range ch.Provenance.Entities {
		bk := map[string]string{}
		switch e.Kind {
		case "email":
			bk["email"] = strings.ToLower(e.Name)
		case "handle":
			bk["handle"] = strings.ToLower(strings.TrimPrefix(e.Name, "@"))
		case "proper_noun", "person", "organization":
			bk["normalized_name"] = strings.ToLower(strings.TrimSpace(e.Name))
		}
		ext.Candidates = append(ext.Candidates, schema.ExtractionCandidate{
			Name: e.Name, Kind: e.Kind, Value: e.Name,
			StartOffset: e.StartOffset, EndOffset: e.EndOffset,
			Confidence: 0.5, BlockingKeys: bk,
		})
	}
	resolutions := make([]schema.ResolutionResult, len(ext.Candidates))
	for i, c := range ext.Candidates {
		resolutions[i] = schema.ResolutionResult{CandidateName: c.Name}
	}
	return ext, resolutions
}

// buildExistingEntities assembles the resolution candidate pool from the contact
// store (personal-assistant scale), deriving deterministic blocking keys from the
// contact name and metadata.
func buildExistingEntities(ctx context.Context, db *sql.DB, limit int, log *slog.Logger) []extract.ExistingEntity {
	if limit <= 0 {
		limit = 500
	}
	contacts, err := store.ListContacts(ctx, db, store.ListContactsFilter{Limit: limit})
	if err != nil {
		log.Warn("intake: synthesis: list contacts for resolution failed", "err", err)
		return nil
	}
	out := make([]extract.ExistingEntity, 0, len(contacts))
	for _, c := range contacts {
		bk := map[string]string{"normalized_name": strings.ToLower(strings.TrimSpace(c.Name))}
		applyMetadataBlockingKeys(c.Metadata, bk)
		out = append(out, extract.ExistingEntity{
			ID: c.ID, Type: "contact", Name: c.Name, BlockingKeys: bk,
		})
	}
	return out
}

// applyMetadataBlockingKeys reads email/handle identifiers from a contact's
// JSON metadata blob into the blocking-key map.
func applyMetadataBlockingKeys(metadata string, bk map[string]string) {
	if strings.TrimSpace(metadata) == "" {
		return
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(metadata), &m); err != nil {
		return
	}
	if v, ok := m["email"].(string); ok && v != "" {
		bk["email"] = strings.ToLower(v)
	}
	if v, ok := m["handle"].(string); ok && v != "" {
		bk["handle"] = strings.ToLower(strings.TrimPrefix(v, "@"))
	}
}
