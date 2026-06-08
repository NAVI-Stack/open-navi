package navi

import (
	"context"

	"github.com/open-navi/navi/internal/presence"
)

// PresenceService returns the stable NAVI-owned presence service for this instance.
func (n *NAVI) PresenceService() presence.Service {
	if n == nil {
		return presence.NewDefaultService(nil, nil, nil, nil, nil, nil)
	}
	if n.presence == nil {
		n.presence = presence.NewDefaultService(
			n.cfg.DB,
			n.StatusTracker(),
			presence.NewDBAttentionSource(n.cfg.DB),
			&presence.DefaultClassificationSource{},
			presence.NewDefaultHealthSource(n.StatusTracker()),
			&presence.NoOpDreamingSource{},
		)
	}
	return n.presence
}

// PresenceEnvelope returns the current authoritative NAVI presence envelope.
func (n *NAVI) PresenceEnvelope(ctx context.Context) presence.PresenceEnvelope {
	return n.PresenceService().NaviPresence(ctx)
}

// PresenceSnapshot returns the current combined presence snapshot known to NAVI.
// In the current phase, this is primarily NAVI presence plus any PET user
// presence later supplied through ObserveUserPresence.
func (n *NAVI) PresenceSnapshot(ctx context.Context) presence.PresenceSnapshot {
	return n.PresenceService().Snapshot(ctx)
}

// ObserveUserPresence records the latest PET-owned user presence visible to NAVI.
func (n *NAVI) ObserveUserPresence(ctx context.Context, envelope presence.PresenceEnvelope) error {
	return n.PresenceService().UpdateUserPresence(ctx, envelope)
}
