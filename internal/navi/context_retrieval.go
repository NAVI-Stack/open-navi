package navi

import (
	"context"
	"log/slog"
	"strings"

	"github.com/open-navi/navi/internal/navi/orchestration"
	orchestrationcontext "github.com/open-navi/navi/internal/navi/orchestration/context"
)

// defaultContextRetrievalBudgetTokens bounds how much retrieved context the
// Contextualize step injects per turn. Kept modest so retrieval augments rather
// than dominates the context window; the Conscious loop can raise it via
// LoopConfig.ContextRetrievalBudgetTokens (CIP P4: budget is per-call,
// configurable per existing compaction patterns).
const defaultContextRetrievalBudgetTokens = 1500

// ContextRetriever is the seam the Conscious loop uses to pull provenance-bearing
// background context during the Contextualize step (CIP §10). The concrete
// implementation lives in internal/intake/retrieve (ContextProvider) and is wired
// in cmd/navid; defining the interface here keeps internal/navi free of an intake
// import (no cycle).
//
// RetrieveContext returns a formatted, trust-labeled, distilled context block for
// query, compressed to budgetTokens, or "" when there is nothing relevant. It
// must be read-only.
type ContextRetriever interface {
	RetrieveContext(ctx context.Context, query string, budgetTokens int) (string, error)
}

// contextRetrievalBudgetTokens resolves the per-turn retrieval distillation budget.
func (l *AgentLoop) contextRetrievalBudgetTokens() int {
	if l.cfg.ContextRetrievalBudgetTokens > 0 {
		return l.cfg.ContextRetrievalBudgetTokens
	}
	return defaultContextRetrievalBudgetTokens
}

// retrievedContextFacts runs the configured ContextRetriever for the current
// turn's user message and returns it as a Facts block for the NCOS context pack.
// It returns ok=false (no injection) when retrieval is disabled, the query is
// empty, retrieval errors, or there is nothing relevant — preserving turn-time
// behavior on every empty-retrieval path.
func (l *AgentLoop) retrievedContextFacts(ctx context.Context, req orchestration.CanonicalRunRequest) (orchestrationcontext.FactsInput, bool) {
	if l.cfg.ContextRetriever == nil {
		return orchestrationcontext.FactsInput{}, false
	}
	query := strings.TrimSpace(req.UserMessage)
	if query == "" {
		return orchestrationcontext.FactsInput{}, false
	}
	block, err := l.cfg.ContextRetriever.RetrieveContext(ctx, query, l.contextRetrievalBudgetTokens())
	if err != nil {
		slog.Warn("navi: intake retrieval failed", "err", err, "chat_id", req.Frame.ChatID)
		return orchestrationcontext.FactsInput{}, false
	}
	block = strings.TrimSpace(block)
	if block == "" {
		return orchestrationcontext.FactsInput{}, false
	}
	return orchestrationcontext.FactsInput{
		Block:      block,
		SourceRef:  "intake_retrieval:" + req.Frame.ChatID,
		Confidence: 0.9,
	}, true
}
