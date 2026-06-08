package cognitive

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// DirectiveWriter is the Cognitive-layer facade for directive and message writes.
// Only this interface (or its implementation) should perform SaveDirective, AppendMessage, UpdateDirectiveMode, DeleteMessage.
// Gateway and backlog call it instead of store directly; workers use it for AppendMessage when adding results.
type DirectiveWriter interface {
	SaveDirective(ctx context.Context, d schema.Directive) error
	AppendMessage(ctx context.Context, msg schema.DirectiveMessage) error
	UpdateDirectiveMode(ctx context.Context, directiveID string, mode schema.DirectiveMode) error
	DeleteMessage(ctx context.Context, messageID string) error
}

// ExecutionRecorder is the Cognitive-layer facade for task and execution-outcome writes.
// Workers call it instead of store directly so all World Model writes flow through Cognitive.
type ExecutionRecorder interface {
	UpdateTask(ctx context.Context, task schema.Task) error
	SaveExecutionOutcome(ctx context.Context, eo schema.ExecutionOutcome) error
	SaveFact(ctx context.Context, fact schema.Fact, prov *schema.EntityProvenance) error
}

// StoreDirectiveWriter implements DirectiveWriter using the store.
func StoreDirectiveWriter(db *sql.DB) DirectiveWriter {
	return &storeDirectiveWriter{db: db}
}

type storeDirectiveWriter struct {
	db *sql.DB
}

func (w *storeDirectiveWriter) SaveDirective(ctx context.Context, d schema.Directive) error {
	return store.SaveDirective(ctx, w.db, d)
}

func (w *storeDirectiveWriter) AppendMessage(ctx context.Context, msg schema.DirectiveMessage) error {
	return store.AppendMessage(ctx, w.db, msg)
}

func (w *storeDirectiveWriter) UpdateDirectiveMode(ctx context.Context, directiveID string, mode schema.DirectiveMode) error {
	return store.UpdateDirectiveMode(ctx, w.db, directiveID, mode)
}

func (w *storeDirectiveWriter) DeleteMessage(ctx context.Context, messageID string) error {
	return store.DeleteDirectiveMessage(ctx, w.db, messageID)
}

// StoreExecutionRecorder implements ExecutionRecorder using the store.
// SaveExecutionOutcome runs compensation when eo.CompensationRequired is true using the given compensator.
func StoreExecutionRecorder(db *sql.DB, compensatorFor func(schema.ExecutionOutcome) store.Compensator) ExecutionRecorder {
	return &storeExecutionRecorder{db: db, compensatorFor: compensatorFor}
}

type storeExecutionRecorder struct {
	db             *sql.DB
	compensatorFor func(schema.ExecutionOutcome) store.Compensator
}

func (r *storeExecutionRecorder) UpdateTask(ctx context.Context, task schema.Task) error {
	if err := store.UpdateTask(ctx, r.db, task); err != nil {
		return err
	}
	affected, _ := json.Marshal([]struct{ Kind, ID string }{{"task", task.ID}})
	now := time.Now().UTC()
	_ = r.SaveExecutionOutcome(ctx, schema.ExecutionOutcome{
		AttemptID:            task.ID + ":update",
		CommandID:            task.ID + ":update",
		AttemptNumber:        1,
		CommandType:          schema.CommandTypeUpdate,
		StartTime:            now,
		EndTime:              &now,
		Outcome:              schema.ExecutionOutcomeSucceeded,
		AffectedEntities:     string(affected),
		CompensationRequired: false,
		CompensationStatus:   schema.CompensationStatusNotRequired,
		RecoveryStatus:       schema.RecoveryStatusNotRequired,
	})
	return nil
}

func (r *storeExecutionRecorder) SaveExecutionOutcome(ctx context.Context, eo schema.ExecutionOutcome) error {
	if err := store.SaveExecutionOutcome(ctx, r.db, eo); err != nil {
		return err
	}
	if eo.CompensationRequired {
		comp := store.DefaultCompensator(eo)
		if r.compensatorFor != nil {
			comp = r.compensatorFor(eo)
		}
		_ = store.RunCompensation(ctx, r.db, eo, comp)
	}
	return nil
}

func (r *storeExecutionRecorder) SaveFact(ctx context.Context, fact schema.Fact, prov *schema.EntityProvenance) error {
	if err := store.SaveFact(ctx, r.db, store.Fact{
		ID:         fact.ID,
		Scope:      fact.Scope,
		ScopeID:    fact.ScopeID,
		Category:   fact.Category,
		Key:        fact.Key,
		Value:      fact.Value,
		Source:     fact.Source,
		Deprecated: fact.Deprecated,
	}); err != nil {
		return err
	}
	if prov == nil {
		return nil
	}
	return store.SaveEntityProvenance(ctx, r.db, "fact", fact.ID, *prov)
}
