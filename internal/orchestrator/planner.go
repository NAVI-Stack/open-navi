package orchestrator

import (
	"context"

	"github.com/ceoai/navi/internal/schema"
)

// LLMAdapter is the interface for LLM-backed directive orchestrators.
// Implementations translate the current directive state into a decision
// (a set of events to emit) on each tick.
type LLMAdapter interface {
	Decide(ctx context.Context) (Decision, error)
}

// Decision is the LLM's output for a single tick.
type Decision struct {
	// Events to emit on the bus.
	Events []schema.Event
	// Cost is the estimated USD cost of the LLM call that produced this decision.
	Cost float64
	// Content is the raw LLM response content (used for repetition detection).
	Content string
}
