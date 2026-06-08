package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/open-navi/navi/internal/schema"
)

// SaveTaskOutcomeFact records a directive-scoped fact summarizing the outcome
// of a worker task so later orchestrator turns can learn from it.
func SaveTaskOutcomeFact(ctx context.Context, db *sql.DB, task schema.Task, outcome schema.TaskStatus, summary, failureReason, recoveryHint, source string) error {
	if db == nil {
		return fmt.Errorf("store: db required for task outcome fact")
	}
	if task.DirectiveID == "" {
		return nil
	}
	if source == "" {
		source = "task_worker"
	}

	value := formatTaskOutcomeValue(task, outcome, summary, failureReason, recoveryHint)
	fact := Fact{
		Scope:    "directive",
		ScopeID:  task.DirectiveID,
		Category: "task_outcome",
		Key:      fmt.Sprintf("task_%s_outcome", task.ID),
		Value:    value,
		Source:   source,
	}
	return SaveFact(ctx, db, fact)
}

func formatTaskOutcomeValue(task schema.Task, outcome schema.TaskStatus, summary, failureReason, recoveryHint string) string {
	title := compactText(task.Title, 120)
	summary = compactText(summary, 400)
	failureReason = compactText(failureReason, 300)
	recoveryHint = compactText(recoveryHint, 180)

	switch outcome {
	case schema.TaskStatusFailed:
		if failureReason == "" {
			failureReason = "Task execution failed."
		}
		value := fmt.Sprintf("Task '%s' FAILED: %s", title, failureReason)
		if recoveryHint != "" {
			value += ". Recovery: " + recoveryHint
		}
		return value
	case schema.TaskStatusCompleted:
		if summary == "" {
			summary = "Task completed successfully."
		}
		return fmt.Sprintf("Task '%s' SUCCEEDED: %s", title, summary)
	default:
		if summary == "" {
			summary = fmt.Sprintf("Task ended with status %s.", outcome)
		}
		return fmt.Sprintf("Task '%s' %s: %s", title, strings.ToUpper(string(outcome)), summary)
	}
}

func compactText(raw string, max int) string {
	raw = strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if max > 0 && len(raw) > max {
		return strings.TrimSpace(raw[:max-3]) + "..."
	}
	return raw
}
