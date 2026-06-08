package intake

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

const RefinerySubject = "navi.refinery.queue"

// Emitter publishes IntakeRecords to the NAVI_REFINERY stream.
// Connectors call EmitBytes (pre-serialised) or Emit (struct pointer).
type Emitter struct {
	js nats.JetStreamContext
}

// NewEmitter creates an Emitter backed by the given JetStream context.
func NewEmitter(js nats.JetStreamContext) *Emitter {
	return &Emitter{js: js}
}

// Emit serialises r and publishes it to navi.refinery.queue.
func (e *Emitter) Emit(_ context.Context, r *IntakeRecord) error {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("intake: emit: marshal: %w", err)
	}
	return e.EmitBytes(data)
}

// EmitBytes publishes pre-serialised record bytes to navi.refinery.queue.
// Used by connectors that serialise the record themselves.
func (e *Emitter) EmitBytes(data []byte) error {
	if _, err := e.js.Publish(RefinerySubject, data); err != nil {
		return fmt.Errorf("intake: emit: publish: %w", err)
	}
	return nil
}
