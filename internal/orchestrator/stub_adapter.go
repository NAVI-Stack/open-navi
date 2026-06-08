package orchestrator

import "context"

// StubAdapter is a deterministic LLM adapter for testing. It always returns
// an empty decision with a fixed cost, without calling a real LLM.
type StubAdapter struct {
	// CostPerCall is the simulated USD cost returned with each decision.
	CostPerCall float64
}

// NewStubAdapter returns a StubAdapter with a default simulated cost.
func NewStubAdapter() *StubAdapter {
	return &StubAdapter{CostPerCall: 0.001}
}

// Decide implements LLMAdapter by returning a no-op decision. Tests can
// assert governor accounting and loop behaviour without depending on an LLM.
func (s *StubAdapter) Decide(_ context.Context) (Decision, error) {
	return Decision{
		Events: nil,
		Cost:   s.CostPerCall,
	}, nil
}
