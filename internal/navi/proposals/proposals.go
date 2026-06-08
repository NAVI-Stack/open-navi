package proposals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/google/uuid"
)

var ErrPersistenceNotConfigured = errors.New("proposal persistence is not configured")

// Draft is the canonical proposal creation request for ICS, reflection, and
// other runtime producers that need to enqueue a proposal entity.
type Draft struct {
	SourceProcess        string
	SourceTrigger        string
	BoundaryKey          string
	Rationale            string
	Priority             schema.ProposalPriority
	Status               schema.ProposalStatus
	TTL                  time.Duration
	ProposedAction       string
	Payload              any
	AffectedEntities     []string
	AffectedEntitiesJSON string
}

// Build constructs a proposal entity from the canonical draft shape.
func Build(draft Draft) (schema.Proposal, error) {
	action := strings.TrimSpace(draft.ProposedAction)
	if action == "" {
		if draft.Payload == nil {
			return schema.Proposal{}, fmt.Errorf("proposal action payload is required")
		}
		encoded, err := json.Marshal(draft.Payload)
		if err != nil {
			return schema.Proposal{}, fmt.Errorf("marshal proposal payload: %w", err)
		}
		action = string(encoded)
	}

	affectedEntities := strings.TrimSpace(draft.AffectedEntitiesJSON)
	if affectedEntities == "" {
		encoded, err := json.Marshal(draft.AffectedEntities)
		if err != nil {
			return schema.Proposal{}, fmt.Errorf("marshal affected entities: %w", err)
		}
		affectedEntities = string(encoded)
	}

	now := time.Now().UTC()
	proposal := schema.Proposal{
		ProposalID:       uuid.New().String(),
		SourceProcess:    strings.TrimSpace(draft.SourceProcess),
		SourceTrigger:    strings.TrimSpace(draft.SourceTrigger),
		BoundaryKey:      strings.TrimSpace(draft.BoundaryKey),
		ProposedAction:   action,
		AffectedEntities: affectedEntities,
		Rationale:        strings.TrimSpace(draft.Rationale),
		Priority:         draft.Priority,
		Status:           draft.Status,
		CreatedAt:        now,
	}
	if proposal.Priority == "" {
		proposal.Priority = schema.ProposalPriorityBlocking
	}
	if proposal.Status == "" {
		proposal.Status = schema.ProposalStatusPending
	}
	if draft.TTL > 0 {
		expiresAt := now.Add(draft.TTL)
		proposal.ExpiresAt = &expiresAt
	}
	return proposal, nil
}

// Save persists a proposal built from the canonical draft through the provided
// persistence callback.
func Save(ctx context.Context, save func(context.Context, schema.Proposal) error, draft Draft) (*schema.Proposal, error) {
	if save == nil {
		return nil, ErrPersistenceNotConfigured
	}
	proposal, err := Build(draft)
	if err != nil {
		return nil, err
	}
	if err := save(ctx, proposal); err != nil {
		return nil, err
	}
	return &proposal, nil
}

// Upsert reuses an existing pending proposal for the same approval boundary
// when possible; otherwise it persists a new proposal.
func Upsert(
	ctx context.Context,
	save func(context.Context, schema.Proposal) error,
	findPending func(context.Context, string) (*schema.Proposal, error),
	draft Draft,
) (*schema.Proposal, error) {
	if save == nil {
		return nil, ErrPersistenceNotConfigured
	}
	proposal, err := Build(draft)
	if err != nil {
		return nil, err
	}
	if boundaryKey := strings.TrimSpace(proposal.BoundaryKey); boundaryKey != "" && findPending != nil {
		pending, err := findPending(ctx, boundaryKey)
		if err != nil {
			return nil, err
		}
		if pending != nil && pending.ProposalID != "" {
			proposal.ProposalID = pending.ProposalID
			proposal.CreatedAt = pending.CreatedAt
			if pending.ExpiresAt != nil && draft.TTL <= 0 {
				proposal.ExpiresAt = pending.ExpiresAt
			}
		}
	}
	if err := save(ctx, proposal); err != nil {
		return nil, err
	}
	return &proposal, nil
}

// BuildReflection constructs a reflection-originated proposal with the default
// queued priority and seven-day TTL.
func BuildReflection(sourceProcess, sourceTrigger, proposedAction, affectedEntities, rationale string) (schema.Proposal, error) {
	return Build(Draft{
		SourceProcess:        sourceProcess,
		SourceTrigger:        sourceTrigger,
		ProposedAction:       proposedAction,
		AffectedEntitiesJSON: affectedEntities,
		Rationale:            rationale,
		Priority:             schema.ProposalPriorityQueued,
		Status:               schema.ProposalStatusPending,
		TTL:                  7 * 24 * time.Hour,
	})
}
