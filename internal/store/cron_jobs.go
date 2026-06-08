package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// CronJobRecord is the SQLite-backed row for cron_jobs (see migrations in db.go).
type CronJobRecord struct {
	ID                 string
	OwnerID            string
	Name               string
	Description        string
	Enabled            bool
	ScheduleKind       string
	ScheduleExpr       sql.NullString
	ScheduleEveryMS    sql.NullInt64
	ScheduleAnchorMS   sql.NullInt64
	ScheduleTZ         sql.NullString
	ScheduleStaggerMS  sql.NullInt64
	SessionTarget      string
	WakeMode           string
	PayloadKind        string
	PayloadText        string
	DeliveryJSON       sql.NullString
	FailureAlertJSON   sql.NullString
	TimeoutMS          sql.NullInt64
	DeleteAfterRun     bool
	NextRunAtMS        sql.NullInt64
	RunningAtMS        sql.NullInt64
	LastRunAtMS        sql.NullInt64
	LastRunStatus      sql.NullString
	LastError          sql.NullString
	LastDurationMS     sql.NullInt64
	ConsecutiveErrors  int64
	ScheduleErrorCount int64
	LastFailureAlertMS sql.NullInt64
	CreatedAtMS        int64
	UpdatedAtMS        int64
}

func cronJobColumns() string {
	return `id, owner_id, name, description, enabled, schedule_kind,
schedule_expr, schedule_every_ms, schedule_anchor_ms, schedule_tz, schedule_stagger_ms,
session_target, wake_mode, payload_kind, payload_text,
delivery_json, failure_alert_json, timeout_ms, delete_after_run,
next_run_at_ms, running_at_ms, last_run_at_ms, last_run_status, last_error, last_duration_ms,
consecutive_errors, schedule_error_count, last_failure_alert_at_ms,
created_at_ms, updated_at_ms`
}

func scanCronJobRow(rows *sql.Rows) (*CronJobRecord, error) {
	var r CronJobRecord
	var enabled, delAfter sql.NullInt64
	err := rows.Scan(
		&r.ID, &r.OwnerID, &r.Name, &r.Description,
		&enabled,
		&r.ScheduleKind, &r.ScheduleExpr, &r.ScheduleEveryMS, &r.ScheduleAnchorMS, &r.ScheduleTZ, &r.ScheduleStaggerMS,
		&r.SessionTarget, &r.WakeMode, &r.PayloadKind, &r.PayloadText,
		&r.DeliveryJSON, &r.FailureAlertJSON, &r.TimeoutMS, &delAfter,
		&r.NextRunAtMS, &r.RunningAtMS, &r.LastRunAtMS,
		&r.LastRunStatus, &r.LastError, &r.LastDurationMS,
		&r.ConsecutiveErrors, &r.ScheduleErrorCount, &r.LastFailureAlertMS,
		&r.CreatedAtMS, &r.UpdatedAtMS,
	)
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled.Valid && enabled.Int64 != 0
	r.DeleteAfterRun = delAfter.Valid && delAfter.Int64 != 0
	return &r, nil
}

// InsertCronJob inserts a cron job row.
func InsertCronJob(ctx context.Context, db *sql.DB, r CronJobRecord) error {
	enabled := int64(0)
	if r.Enabled {
		enabled = 1
	}
	del := int64(0)
	if r.DeleteAfterRun {
		del = 1
	}
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO cron_jobs (%s)
		VALUES (
		 ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
		)`, cronJobColumns()),
		r.ID, r.OwnerID, r.Name, r.Description, enabled,
		r.ScheduleKind, nullStr(r.ScheduleExpr), nullInt64(r.ScheduleEveryMS), nullInt64(r.ScheduleAnchorMS),
		nullStr(r.ScheduleTZ), nullInt64(r.ScheduleStaggerMS),
		r.SessionTarget, r.WakeMode, r.PayloadKind, r.PayloadText,
		nullStr(r.DeliveryJSON), nullStr(r.FailureAlertJSON), nullInt64(r.TimeoutMS), del,
		nullInt64(r.NextRunAtMS), nullInt64(r.RunningAtMS), nullInt64(r.LastRunAtMS),
		nullStr(r.LastRunStatus), nullStr(r.LastError), nullInt64(r.LastDurationMS),
		r.ConsecutiveErrors, r.ScheduleErrorCount, nullInt64(r.LastFailureAlertMS),
		r.CreatedAtMS, r.UpdatedAtMS,
	)
	return err
}

func nullStr(ns sql.NullString) any {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

func nullInt64(ni sql.NullInt64) any {
	if !ni.Valid {
		return nil
	}
	return ni.Int64
}

// UpdateCronJob persists the full cron job row by id (replace semantics).
func UpdateCronJob(ctx context.Context, db *sql.DB, r CronJobRecord) error {
	enabled := int64(0)
	if r.Enabled {
		enabled = 1
	}
	del := int64(0)
	if r.DeleteAfterRun {
		del = 1
	}
	res, err := db.ExecContext(ctx, `
		UPDATE cron_jobs SET
			owner_id = ?, name = ?, description = ?, enabled = ?, schedule_kind = ?,
			schedule_expr = ?, schedule_every_ms = ?, schedule_anchor_ms = ?, schedule_tz = ?, schedule_stagger_ms = ?,
			session_target = ?, wake_mode = ?, payload_kind = ?, payload_text = ?,
			delivery_json = ?, failure_alert_json = ?, timeout_ms = ?, delete_after_run = ?,
			next_run_at_ms = ?, running_at_ms = ?, last_run_at_ms = ?, last_run_status = ?, last_error = ?, last_duration_ms = ?,
			consecutive_errors = ?, schedule_error_count = ?, last_failure_alert_at_ms = ?,
			updated_at_ms = ?
		WHERE id = ?`,
		r.OwnerID, r.Name, r.Description, enabled, r.ScheduleKind,
		nullStr(r.ScheduleExpr), nullInt64(r.ScheduleEveryMS), nullInt64(r.ScheduleAnchorMS), nullStr(r.ScheduleTZ),
		nullInt64(r.ScheduleStaggerMS),
		r.SessionTarget, r.WakeMode, r.PayloadKind, r.PayloadText,
		nullStr(r.DeliveryJSON), nullStr(r.FailureAlertJSON), nullInt64(r.TimeoutMS), del,
		nullInt64(r.NextRunAtMS), nullInt64(r.RunningAtMS), nullInt64(r.LastRunAtMS),
		nullStr(r.LastRunStatus), nullStr(r.LastError), nullInt64(r.LastDurationMS),
		r.ConsecutiveErrors, r.ScheduleErrorCount, nullInt64(r.LastFailureAlertMS),
		r.UpdatedAtMS, r.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteCronJob removes a cron job by id.
func DeleteCronJob(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM cron_jobs WHERE id = ?`, id)
	return err
}

// CronJobOutcome carries the subset of fields that the cron run-loop owns
// after a job executes (or fails to). Using a targeted UPDATE keeps the
// outcome write from clobbering concurrent configuration edits to the same
// row (name/payload/schedule/etc.).
type CronJobOutcome struct {
	Enabled            bool
	NextRunAtMS        sql.NullInt64
	RunningAtMS        sql.NullInt64
	LastRunAtMS        sql.NullInt64
	LastRunStatus      sql.NullString
	LastError          sql.NullString
	LastDurationMS    sql.NullInt64
	ConsecutiveErrors  int64
	ScheduleErrorCount int64
	LastFailureAlertMS sql.NullInt64
	UpdatedAtMS        int64
}

// UpdateCronJobOutcome writes only the outcome-owned columns for the given id.
// Returns sql.ErrNoRows if the job was deleted concurrently.
func UpdateCronJobOutcome(ctx context.Context, db *sql.DB, id string, o CronJobOutcome) error {
	enabled := int64(0)
	if o.Enabled {
		enabled = 1
	}
	res, err := db.ExecContext(ctx, `
		UPDATE cron_jobs SET
			enabled = ?,
			next_run_at_ms = ?, running_at_ms = ?, last_run_at_ms = ?,
			last_run_status = ?, last_error = ?, last_duration_ms = ?,
			consecutive_errors = ?, schedule_error_count = ?, last_failure_alert_at_ms = ?,
			updated_at_ms = ?
		WHERE id = ?`,
		enabled,
		nullInt64(o.NextRunAtMS), nullInt64(o.RunningAtMS), nullInt64(o.LastRunAtMS),
		nullStr(o.LastRunStatus), nullStr(o.LastError), nullInt64(o.LastDurationMS),
		o.ConsecutiveErrors, o.ScheduleErrorCount, nullInt64(o.LastFailureAlertMS),
		o.UpdatedAtMS, id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ClearCronJobRunning resets running_at_ms to NULL for a stuck job without
// touching any other columns. Used by the dispatch loop's best-effort
// recovery path when a post-execution write fails.
func ClearCronJobRunning(ctx context.Context, db *sql.DB, id string, updatedAtMS int64) error {
	res, err := db.ExecContext(ctx, `
		UPDATE cron_jobs SET running_at_ms = NULL, updated_at_ms = ?
		WHERE id = ?`, updatedAtMS, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetCronJob returns one job by id.
func GetCronJob(ctx context.Context, db *sql.DB, id string) (*CronJobRecord, error) {
	row := db.QueryRowContext(ctx, `SELECT `+cronJobColumns()+` FROM cron_jobs WHERE id = ?`, id)
	rec, err := scanCronJobSingle(row)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

func scanCronJobSingle(row *sql.Row) (*CronJobRecord, error) {
	var r CronJobRecord
	var enabled, delAfter sql.NullInt64
	err := row.Scan(
		&r.ID, &r.OwnerID, &r.Name, &r.Description,
		&enabled,
		&r.ScheduleKind, &r.ScheduleExpr, &r.ScheduleEveryMS, &r.ScheduleAnchorMS, &r.ScheduleTZ, &r.ScheduleStaggerMS,
		&r.SessionTarget, &r.WakeMode, &r.PayloadKind, &r.PayloadText,
		&r.DeliveryJSON, &r.FailureAlertJSON, &r.TimeoutMS, &delAfter,
		&r.NextRunAtMS, &r.RunningAtMS, &r.LastRunAtMS,
		&r.LastRunStatus, &r.LastError, &r.LastDurationMS,
		&r.ConsecutiveErrors, &r.ScheduleErrorCount, &r.LastFailureAlertMS,
		&r.CreatedAtMS, &r.UpdatedAtMS,
	)
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled.Valid && enabled.Int64 != 0
	r.DeleteAfterRun = delAfter.Valid && delAfter.Int64 != 0
	return &r, nil
}

// ListCronJobs returns enabled and disabled cron jobs sorted by creation.
func ListCronJobs(ctx context.Context, db *sql.DB) ([]CronJobRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+cronJobColumns()+` FROM cron_jobs ORDER BY created_at_ms DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CronJobRecord
	for rows.Next() {
		rec, err := scanCronJobRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

// ListEnabledCronJobsWithNext returns enabled jobs that have a scheduled next_run_at_ms.
func ListEnabledCronJobsWithNext(ctx context.Context, db *sql.DB) ([]CronJobRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+cronJobColumns()+`
		FROM cron_jobs
		WHERE enabled != 0 AND next_run_at_ms IS NOT NULL AND next_run_at_ms > 0
		ORDER BY next_run_at_ms ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CronJobRecord
	for rows.Next() {
		rec, err := scanCronJobRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

// GetRunnableCronJobs returns enabled jobs due at or before nowMS.
func GetRunnableCronJobs(ctx context.Context, db *sql.DB, nowMS int64) ([]CronJobRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+cronJobColumns()+`
		FROM cron_jobs
		WHERE enabled != 0
		  AND next_run_at_ms IS NOT NULL
		  AND next_run_at_ms > 0
		  AND next_run_at_ms <= ?
		ORDER BY next_run_at_ms ASC`, nowMS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CronJobRecord
	for rows.Next() {
		rec, err := scanCronJobRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

// MigrateLegacyScheduledTasksToCron inserts active scheduled_tasks into cron_jobs once per row (idempotent by id).
func MigrateLegacyScheduledTasksToCronJobs(ctx context.Context, db *sql.DB) (int64, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, owner_id, name, description, schedule_pattern, prompt, status, next_run_at, created_at, updated_at
		FROM scheduled_tasks
		WHERE status = 'active'`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var inserted int64
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	ownerTZ := ""
	if o, ok, oe := GetOwner(ctx, db); oe == nil && ok {
		ownerTZ = strings.TrimSpace(o.Timezone)
	}
	ms := func(t time.Time) int64 { return t.UTC().UnixMilli() }
	for rows.Next() {
		var id, ownerID, name, desc, pattern, prompt, status, nextRunStr, createdStr, updatedStr string
		if err := rows.Scan(&id, &ownerID, &name, &desc, &pattern, &prompt, &status, &nextRunStr, &createdStr, &updatedStr); err != nil {
			return inserted, err
		}

		var dupCount int64
		if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM cron_jobs WHERE id = ?`, id).Scan(&dupCount); err != nil {
			return inserted, err
		}
		if dupCount > 0 {
			continue
		}

		nextRun, err := parseTime(nextRunStr)
		if err != nil {
			continue
		}
		created, _ := parseTime(createdStr)
		updated, _ := parseTime(updatedStr)

		if _, parseErr := parser.Parse(pattern); parseErr != nil {
			continue
		}

		nextMS := nextRun.UTC().UnixMilli()

		rec := CronJobRecord{
			ID:                 id,
			OwnerID:            ownerID,
			Name:               name,
			Description:        desc,
			Enabled:            true,
			ScheduleKind:       "cron",
			ScheduleExpr:       sql.NullString{String: pattern, Valid: true},
			SessionTarget:      "main",
			WakeMode:           "next-heartbeat",
			PayloadKind:        "systemEvent",
			PayloadText:        prompt,
			TimeoutMS:          sql.NullInt64{},
			DeleteAfterRun:     false,
			NextRunAtMS:        sql.NullInt64{Int64: nextMS, Valid: true},
			ConsecutiveErrors:  0,
			ScheduleErrorCount: 0,
			CreatedAtMS:        ms(created),
			UpdatedAtMS:        ms(updated),
		}
		if !created.IsZero() {
			rec.CreatedAtMS = ms(created.UTC())
		} else {
			rec.CreatedAtMS = ms(time.Now().UTC())
		}
		if !updated.IsZero() {
			rec.UpdatedAtMS = ms(updated.UTC())
		} else {
			rec.UpdatedAtMS = rec.CreatedAtMS
		}
		if ownerTZ != "" && ownerTZ != "UTC" {
			rec.ScheduleTZ = sql.NullString{String: ownerTZ, Valid: true}
		}

		if err := InsertCronJob(ctx, db, rec); err != nil {
			if isUniqueConstraint(err) {
				continue
			}
			return inserted, err
		}
		inserted++
	}
	return inserted, rows.Err()
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "PRIMARY KEY constraint")
}
