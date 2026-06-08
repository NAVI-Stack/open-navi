package presence

import "context"

// DreamingSummary describes whether NAVI is currently in a dream-like background
// processing mode that should normalize to public dreaming.
type DreamingSummary struct {
	Active     bool
	StatusText string
	Subtext    string
}

// DreamingSource provides explicit dreaming classification. This must stay a
// separate seam from heartbeat and raw activity so dreaming is never inferred
// from transport/liveness alone.
type DreamingSource interface {
	Dreaming(ctx context.Context) DreamingSummary
}

// NoOpDreamingSource is the conservative default: NAVI is not dreaming unless a
// real subsystem explicitly says so.
type NoOpDreamingSource struct{}

func (n *NoOpDreamingSource) Dreaming(ctx context.Context) DreamingSummary {
	return DreamingSummary{}
}
