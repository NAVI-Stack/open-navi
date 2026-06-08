package backlog

import (
	"context"
	"database/sql"
	"log"
	"os"

	"github.com/open-navi/navi/internal/cognitive"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// CreateDirectiveFromBacklog reads the backlog file, finds the first PENDING task,
// and creates an ACT directive with one owner message so the orchestrator can pick it up.
// Does nothing if there are already active directives, no PENDING task, or writer is nil.
// Directive writes go only through the Cognitive facade (writer); no direct store writes.
func CreateDirectiveFromBacklog(ctx context.Context, db *sql.DB, writer cognitive.DirectiveWriter, backlogPath string) error {
	if writer == nil {
		return nil
	}
	active, err := store.GetActiveDirectives(ctx, db)
	if err != nil {
		return err
	}
	if len(active) > 0 {
		return nil
	}
	if _, err := os.Stat(backlogPath); os.IsNotExist(err) {
		return nil
	}
	tasks, err := ParseBacklogFile(backlogPath)
	if err != nil {
		return err
	}
	t := FirstPending(tasks)
	if t == nil {
		return nil
	}
	d := schema.NewDirective("Complete task "+t.ID+": "+t.Name, schema.DirectiveModeAct, "backlog-poller")
	if err := writer.SaveDirective(ctx, d); err != nil {
		return err
	}
	msg := schema.NewDirectiveMessage(d.DirectiveID, "owner",
		"Please complete this task from the backlog: "+t.Name+". Task ID: "+t.ID+".")
	if err := writer.AppendMessage(ctx, msg); err != nil {
		return err
	}
	log.Printf("backlog: created directive %s for task %s: %s", d.DirectiveID, t.ID, t.Name)
	return nil
}
