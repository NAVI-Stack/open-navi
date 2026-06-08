package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/schema"
)

// LoopConfig configures an orchestration loop.
type LoopConfig struct {
	Bus          bus.Bus
	LLM          LLMAdapter
	Governor     *governor.Governor
	TickInterval time.Duration
}

type workAwareAdapter interface {
	HasWork(ctx context.Context) (bool, error)
}

// reads state from SQLite, invokes the LLM adapter, and emits events.
// It exits cleanly on context cancellation or governor trip.
func RunLoop(ctx context.Context, cfg LoopConfig) error {
	ticker := time.NewTicker(cfg.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			err := tick(ctx, cfg)
			if err != nil {
				// If it's a governor trip, emit the fact and exit cleanly.
				if tripErr, ok := isGovernorTrip(err); ok {
					_ = emitGovernorTripped(ctx, cfg.Bus, tripErr)
					return err
				}
				return fmt.Errorf("orchestrator: tick: %w", err)
			}
		}
	}
}

// RunOnce executes exactly one tick of the orchestration loop. Useful for testing
// and smoke tests where a full loop is not needed.
func RunOnce(ctx context.Context, cfg LoopConfig) error {
	return tick(ctx, cfg)
}

func tick(ctx context.Context, cfg LoopConfig) error {
	// Governor checks
	if err := cfg.Governor.CheckDuration(); err != nil {
		return err
	}
	if detector, ok := cfg.LLM.(workAwareAdapter); ok {
		hasWork, err := detector.HasWork(ctx)
		if err != nil {
			return fmt.Errorf("orchestrator: detect work: %w", err)
		}
		if !hasWork {
			return nil
		}
	}
	if err := cfg.Governor.RecordAction(); err != nil {
		return err
	}

	// Ask the LLM adapter what to do.
	decision, err := cfg.LLM.Decide(ctx)
	if err != nil {
		return fmt.Errorf("orchestrator: llm decide: %w", err)
	}

	// Record the LLM cost.
	if decision.Cost > 0 {
		if err := cfg.Governor.RecordCost(decision.Cost); err != nil {
			return err
		}
	}

	// Repetition check — orchestrator runs one directive at a time; no session ID needed.
	if decision.Content != "" {
		if err := cfg.Governor.RecordRepetition("", decision.Content); err != nil {
			return err
		}
	}

	// Execute the decision: emit events and update state.
	for _, ev := range decision.Events {
		if err := cfg.Bus.Publish(ctx, ev); err != nil {
			return fmt.Errorf("orchestrator: publish event: %w", err)
		}

	}

	return nil
}

func emitGovernorTripped(ctx context.Context, b bus.Bus, tripErr *governor.ErrGovernorTripped) error {
	ev := schema.NewEvent(
		schema.FactGovernorTripped,
		schema.EventKindFact,
		"governor-trip",
		schema.AgentType("orchestrator"),
		schema.GovernorTrippedPayload{
			GovernorType: string(tripErr.Type),
			Limit:        tripErr.Limit,
			Actual:       tripErr.Actual,
		},
	)
	return b.Publish(ctx, ev)
}

func isGovernorTrip(err error) (*governor.ErrGovernorTripped, bool) {
	if tripErr, ok := err.(*governor.ErrGovernorTripped); ok {
		return tripErr, true
	}
	return nil, false
}
