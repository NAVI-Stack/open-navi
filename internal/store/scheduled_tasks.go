package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

// PersistScheduledTask inserts a new scheduled task.
func PersistScheduledTask(ctx context.Context, db *sql.DB, t schema.ScheduledTask) error {
	var lastRunAt sql.NullString
	if !t.LastRunAt.IsZero() {
		lastRunAt.String = t.LastRunAt.UTC().Format(timeFormat)
		lastRunAt.Valid = true
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO scheduled_tasks (id, owner_id, name, description, schedule_pattern, prompt, status, last_run_at, next_run_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.OwnerID, t.Name, t.Description, t.SchedulePattern, t.Prompt, t.Status,
		lastRunAt, t.NextRunAt.UTC().Format(timeFormat),
		t.CreatedAt.UTC().Format(timeFormat), t.UpdatedAt.UTC().Format(timeFormat))
	return err
}

// UpdateScheduledTask updates an existing scheduled task.
func UpdateScheduledTask(ctx context.Context, db *sql.DB, t schema.ScheduledTask) error {
	var lastRunAt sql.NullString
	if !t.LastRunAt.IsZero() {
		lastRunAt.String = t.LastRunAt.UTC().Format(timeFormat)
		lastRunAt.Valid = true
	}

	_, err := db.ExecContext(ctx, `
		UPDATE scheduled_tasks
		SET name = ?, description = ?, schedule_pattern = ?, prompt = ?, status = ?, last_run_at = ?, next_run_at = ?, updated_at = ?
		WHERE id = ?
	`, t.Name, t.Description, t.SchedulePattern, t.Prompt, t.Status,
		lastRunAt, t.NextRunAt.UTC().Format(timeFormat),
		t.UpdatedAt.UTC().Format(timeFormat), t.ID)
	return err
}

// GetDueScheduledTasks returns active scheduled tasks that are due to run.
func GetDueScheduledTasks(ctx context.Context, db *sql.DB, now time.Time) ([]schema.ScheduledTask, error) {
	nowStr := now.UTC().Format(timeFormat)
	rows, err := db.QueryContext(ctx, `
		SELECT id, owner_id, name, description, schedule_pattern, prompt, status, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_tasks
		WHERE status = 'active' AND next_run_at <= ?
	`, nowStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []schema.ScheduledTask
	for rows.Next() {
		var t schema.ScheduledTask
		var nextRun, created, updated string
		var lastRunNull sql.NullString
		if err := rows.Scan(&t.ID, &t.OwnerID, &t.Name, &t.Description, &t.SchedulePattern, &t.Prompt, &t.Status,
			&lastRunNull, &nextRun, &created, &updated); err != nil {
			return nil, err
		}
		if lastRunNull.Valid {
			t.LastRunAt, _ = parseTime(lastRunNull.String)
		}
		t.NextRunAt, _ = parseTime(nextRun)
		t.CreatedAt, _ = parseTime(created)
		t.UpdatedAt, _ = parseTime(updated)
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListScheduledTasks returns all scheduled tasks.
func ListScheduledTasks(ctx context.Context, db *sql.DB) ([]schema.ScheduledTask, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, owner_id, name, description, schedule_pattern, prompt, status, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_tasks
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []schema.ScheduledTask
	for rows.Next() {
		var t schema.ScheduledTask
		var nextRun, created, updated string
		var lastRunNull sql.NullString
		if err := rows.Scan(&t.ID, &t.OwnerID, &t.Name, &t.Description, &t.SchedulePattern, &t.Prompt, &t.Status,
			&lastRunNull, &nextRun, &created, &updated); err != nil {
			return nil, err
		}
		if lastRunNull.Valid {
			t.LastRunAt, _ = parseTime(lastRunNull.String)
		}
		t.NextRunAt, _ = parseTime(nextRun)
		t.CreatedAt, _ = parseTime(created)
		t.UpdatedAt, _ = parseTime(updated)
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteScheduledTask removes a scheduled task by ID.
func DeleteScheduledTask(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE id = ?`, id)
	return err
}
