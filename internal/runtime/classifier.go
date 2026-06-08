package runtime

import (
	"time"
)

// QueueAction describes how a new inbox item should be handled relative to the
// current runtime session state. These values are advisory; the RunCoordinator owns
// the actual dispatch semantics.
const (
	QueueActionAppend    = "append"
	QueueActionDefer     = "defer"
	QueueActionMerge     = "merge"
	QueueActionSupersede = "supersede"
	QueueActionInterrupt = "interrupt"
	QueueActionSteer     = "steer"
	QueueActionResume    = "resume"
)

// Classifier encapsulates queue classification logic so it can evolve
// independently of the RunCoordinator.
type Classifier struct {
	// burstWindow is the time window within which messages from the same
	// source channel may be merged as a single intake.
	burstWindow time.Duration
}

func NewClassifier() *Classifier {
	return &Classifier{
		burstWindow: 3 * time.Second,
	}
}

// classify applies queue semantics to an inbox item, mutating it in-place.
// It does not persist anything; callers are responsible for storing the item.
func (c *Classifier) classify(item *InboxItem, pausedRun *RunState, pending []InboxItem) {
	if item == nil {
		return
	}
	if item.QueueAction == "" {
		item.QueueAction = QueueActionAppend
	}
	if item.Confidence == 0 {
		item.Confidence = 1
	}

	// If there is a paused run waiting for a proposal resolution, defer
	// everything except explicit resume / proposal resolution signals.
	if pausedRun != nil &&
		item.PayloadType != "proposal_resolution" &&
		item.QueueAction != QueueActionResume {
		item.QueueAction = QueueActionDefer
		item.Status = InboxStatusDeferred
		item.ClassifiedReason = "session has a paused foreground run waiting for external input"
		item.Confidence = 0.95
		return
	}

	// Supersede: same idempotency key as an existing pending item.
	if item.IdempotencyKey != "" {
		for _, existing := range pending {
			if existing.IdempotencyKey == "" {
				continue
			}
			if existing.IdempotencyKey == item.IdempotencyKey {
				item.QueueAction = QueueActionSupersede
				item.Status = InboxStatusSuperseded
				item.ClassifiedReason = "duplicate idempotency key matches an existing pending inbox item"
				item.Confidence = 1
				return
			}
		}
	}

	// Merge: bursty input from same source channel within a short window.
	for _, existing := range pending {
		if existing.SourceChannel != item.SourceChannel {
			continue
		}
		if existing.ActorType != item.ActorType || existing.PayloadType != item.PayloadType {
			continue
		}
		if item.ReceivedAt.IsZero() || existing.ReceivedAt.IsZero() {
			continue
		}
		delta := item.ReceivedAt.Sub(existing.ReceivedAt)
		if delta < 0 {
			delta = -delta
		}
		if delta <= c.burstWindow {
			item.QueueAction = QueueActionMerge
			item.MergedIntoID = existing.ID
			item.Status = InboxStatusMerged
			item.ClassifiedReason = "bursty same-source message merged into the already pending runtime session turn"
			item.Confidence = 0.8
			return
		}
	}

	// Default: append as a new pending item.
	item.Status = InboxStatusPending
	item.ClassifiedReason = "default append queue behavior"
}
