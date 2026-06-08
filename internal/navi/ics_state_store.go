package navi

import (
	"context"
	"time"

	"github.com/open-navi/navi/internal/navi/inference"
)

// ICSState is the latest authoritative persisted ICS state for a run.
type ICSState struct {
	RunID            string                     `json:"run_id"`
	Version          string                     `json:"version"`
	DecisionEnvelope inference.DecisionEnvelope `json:"decision_envelope"`
	CreatedAt        time.Time                  `json:"created_at"`
	UpdatedAt        time.Time                  `json:"updated_at"`
}

// ICSHistory is one append-only persisted ICS decision step for a run.
type ICSHistory struct {
	RunID     string                     `json:"run_id"`
	Step      int                        `json:"step"`
	Envelope  inference.DecisionEnvelope `json:"envelope"`
	Timestamp time.Time                  `json:"timestamp"`
}

// ICSStateStore is the first-class persistence boundary for authoritative ICS state.
type ICSStateStore interface {
	SaveICSState(ctx context.Context, runID string, envelope inference.DecisionEnvelope) (*ICSState, error)
	LoadICSState(ctx context.Context, runID string) (*ICSState, error)
	AppendICSHistory(ctx context.Context, runID string, envelope inference.DecisionEnvelope) (*ICSHistory, error)
	LoadICSHistory(ctx context.Context, runID string, limit int) ([]ICSHistory, error)
}
