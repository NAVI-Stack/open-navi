package presence

import (
	"context"
	"database/sql"

	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

// DBAttentionSource derives NAVI attention state from pending proposals.
//
// This is intentionally a narrow projection. Presence does not need full proposal
// entities; it only needs to know whether owner attention is blocking or useful.
type DBAttentionSource struct {
	db *sql.DB
}

// NewDBAttentionSource creates a proposal-backed attention source.
func NewDBAttentionSource(db *sql.DB) *DBAttentionSource {
	return &DBAttentionSource{db: db}
}

// Attention reports blocking pending proposals as needs_attention and queued
// pending proposals as wants_attention.
func (s *DBAttentionSource) Attention(ctx context.Context, chatID string) PresenceAttention {
	if s == nil || s.db == nil {
		return PresenceAttention{Level: "none", Blocking: false}
	}

	proposals, err := store.ListPendingProposals(ctx, s.db, 50)
	if err != nil || len(proposals) == 0 {
		return PresenceAttention{Level: "none", Blocking: false}
	}

	var queued *schema.Proposal
	for i := range proposals {
		p := &proposals[i]
		if p.Priority == schema.ProposalPriorityBlocking {
			return PresenceAttention{
				Level:      "needs_attention",
				ReasonCode: stringPtr("blocking_proposal"),
				ProposalID: stringPtr(p.ProposalID),
				Blocking:   true,
			}
		}
		if queued == nil {
			queued = p
		}
	}

	if queued != nil {
		return PresenceAttention{
			Level:      "wants_attention",
			ReasonCode: stringPtr("queued_proposal"),
			ProposalID: stringPtr(queued.ProposalID),
			Blocking:   false,
		}
	}

	return PresenceAttention{Level: "none", Blocking: false}
}

func stringPtr(value string) *string {
	return &value
}
