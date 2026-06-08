package proposals

import (
	"context"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

func TestUpsert_ReusesPendingProposalForSameBoundary(t *testing.T) {
	ctx := context.Background()
	saved := make([]schema.Proposal, 0, 1)
	existing := &schema.Proposal{
		ProposalID:  "proposal-existing",
		BoundaryKey: "boundary:runtime_echo",
		CreatedAt:   time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC),
	}

	proposal, err := Upsert(
		ctx,
		func(ctx context.Context, proposal schema.Proposal) error {
			saved = append(saved, proposal)
			return nil
		},
		func(ctx context.Context, boundaryKey string) (*schema.Proposal, error) {
			if boundaryKey == existing.BoundaryKey {
				return existing, nil
			}
			return nil, nil
		},
		Draft{
			SourceProcess: "ics",
			BoundaryKey:   existing.BoundaryKey,
			Payload:       map[string]any{"type": "ics_governance_boundary"},
			Priority:      schema.ProposalPriorityBlocking,
			Status:        schema.ProposalStatusPending,
		},
	)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if proposal.ProposalID != existing.ProposalID {
		t.Fatalf("expected proposal id reuse for matching boundary, got %#v", proposal)
	}
	if len(saved) != 1 || saved[0].ProposalID != existing.ProposalID {
		t.Fatalf("expected saved proposal to update existing id, got %#v", saved)
	}
}

func TestUpsert_CreatesNewProposalForChangedBoundary(t *testing.T) {
	ctx := context.Background()
	saved := make([]schema.Proposal, 0, 1)

	proposal, err := Upsert(
		ctx,
		func(ctx context.Context, proposal schema.Proposal) error {
			saved = append(saved, proposal)
			return nil
		},
		func(ctx context.Context, boundaryKey string) (*schema.Proposal, error) {
			return nil, nil
		},
		Draft{
			SourceProcess: "ics",
			BoundaryKey:   "boundary:new",
			Payload:       map[string]any{"type": "ics_governance_boundary"},
			Priority:      schema.ProposalPriorityBlocking,
			Status:        schema.ProposalStatusPending,
		},
	)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if proposal.ProposalID == "" {
		t.Fatalf("expected new proposal id, got %#v", proposal)
	}
	if proposal.BoundaryKey != "boundary:new" {
		t.Fatalf("expected boundary key to persist, got %#v", proposal)
	}
	if len(saved) != 1 || saved[0].ProposalID == "" {
		t.Fatalf("expected a newly saved proposal, got %#v", saved)
	}
}
