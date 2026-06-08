package bus

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// StreamNames lists all NAVI JetStream stream names for purge/reset.
var StreamNames = []string{
	"NAVI", "NAVI_AGENTS", "NAVI_REFINERY", "NAVI_HITL",
	"NAVI_AUDIT", "NAVI_GOVERNOR", "NAVI_FACTS", "NAVI_COMMANDS",
}

// PurgeStreams purges all NAVI streams so no messages remain. Stream configs are unchanged.
func PurgeStreams(js nats.JetStreamContext) error {
	for _, name := range StreamNames {
		if err := js.PurgeStream(name); err != nil && err != nats.ErrStreamNotFound {
			return fmt.Errorf("bus: purge stream %s: %w", name, err)
		}
	}
	return nil
}

// EnsureStreams creates all required JetStream streams if they do not exist.
// Safe to call on every startup — idempotent.
func EnsureStreams(js nats.JetStreamContext) error {
	streams := []*nats.StreamConfig{
		{
			Name:         "NAVI",
			Subjects:     []string{"navi.core.inbox", "navi.core.outbox"},
			Retention:    nats.LimitsPolicy,
			MaxAge:       24 * time.Hour,
			MaxConsumers: -1,
		},
		{
			Name:         "NAVI_AGENTS",
			Subjects:     []string{"navi.agents.>"},
			Retention:    nats.WorkQueuePolicy,
			MaxAge:       48 * time.Hour,
			MaxConsumers: 1,
		},
		{
			Name:         "NAVI_REFINERY",
			Subjects:     []string{"navi.refinery.queue"},
			Retention:    nats.WorkQueuePolicy,
			MaxAge:       48 * time.Hour,
			MaxConsumers: 1,
		},
		{
			Name:         "NAVI_HITL",
			Subjects:     []string{"navi.hitl.requests"},
			Retention:    nats.LimitsPolicy,
			MaxAge:       7 * 24 * time.Hour,
			MaxConsumers: -1,
		},
		{
			Name:         "NAVI_AUDIT",
			Subjects:     []string{"navi.audit.>"},
			Retention:    nats.LimitsPolicy,
			MaxAge:       30 * 24 * time.Hour,
			MaxConsumers: -1,
		},
		{
			Name:         "NAVI_GOVERNOR",
			Subjects:     []string{"navi.governor.>"},
			Retention:    nats.LimitsPolicy,
			MaxAge:       7 * 24 * time.Hour,
			MaxConsumers: -1,
		},
		{
			// NAVI_FACTS carries all live fact events (navi.fact.*).
			// The Orchestrator loop and other consumers subscribe here for operational events.
			// The audit bridge independently mirrors facts into NAVI_AUDIT for replay.
			Name:         "NAVI_FACTS",
			Subjects:     []string{"navi.fact.>", "navi.navi.fact.>"},
			Retention:    nats.LimitsPolicy,
			MaxAge:       7 * 24 * time.Hour,
			MaxConsumers: -1,
		},
		{
			// NAVI_COMMANDS captures all cmd events.
			Name:         "NAVI_COMMANDS",
			Subjects:     []string{"navi.cmd.>", "navi.navi.cmd.>"},
			Retention:    nats.LimitsPolicy,
			MaxAge:       7 * 24 * time.Hour,
			MaxConsumers: -1,
		},
	}

	for _, cfg := range streams {
		_, err := js.StreamInfo(cfg.Name)
		if err == nats.ErrStreamNotFound {
			_, err = js.AddStream(cfg)
			if err != nil && err != nats.ErrStreamNameAlreadyInUse {
				return err
			}
		} else if err != nil {
			return err
		} else {
			_, err = js.UpdateStream(cfg)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
