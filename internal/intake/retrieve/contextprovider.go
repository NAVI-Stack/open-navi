package retrieve

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/ceoai/navi/internal/intake/embed"
	"github.com/ceoai/navi/internal/schema"
)

// ContextProvider adapts the hybrid retrieval surface to the Conscious loop's
// Contextualize step. It is wired into the agent loop (cmd/navid) and invoked
// per turn with the current user message as the query. It satisfies the
// navi.ContextRetriever interface structurally (no import cycle: the interface
// lives in internal/navi, this concrete type lives here, wiring happens in
// cmd/navid).
//
// It is READ-ONLY end to end: Retrieve performs only SELECTs and distillation is
// extractive, so turn-time context assembly never writes the World Model.
type ContextProvider struct {
	DB       *sql.DB
	Embedder embed.Embedder
	// Limit caps retrieved chunks before distillation. 0 defaults to 8.
	Limit int
	// MaxPrivacyClass is the privacy ceiling applied to retrieval. Empty means
	// owner context (no ceiling).
	MaxPrivacyClass schema.PrivacyClass
	// Weights overrides the ranking weights. Zero uses DefaultWeights.
	Weights Weights
	Log     *slog.Logger
}

// RetrieveContext runs retrieve → retrieval-side distill → format and returns a
// provenance- and trust-labeled context block for the given query, compressed to
// budgetTokens. Returns "" when nothing relevant is found, so the loop's
// turn-time behavior is preserved on the empty-retrieval path.
func (p *ContextProvider) RetrieveContext(ctx context.Context, query string, budgetTokens int) (string, error) {
	if p == nil || p.DB == nil {
		return "", nil
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 8
	}
	results, err := Retrieve(ctx, p.DB, p.Embedder, query, Options{
		Limit:           limit,
		Weights:         p.Weights,
		MaxPrivacyClass: p.MaxPrivacyClass,
	})
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", nil
	}
	distilled := DistillResults(ctx, results, budgetTokens)
	return FormatContextBlock(distilled, results), nil
}
