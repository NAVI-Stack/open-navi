package skill

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/cron"
	"github.com/open-navi/navi/internal/store"
)

func argString(args map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := args[k].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func argInt64(args map[string]any, keys ...string) (int64, bool) {
	for _, k := range keys {
		v, ok := args[k]
		if !ok || v == nil {
			continue
		}
		switch x := v.(type) {
		case int:
			return int64(x), true
		case int64:
			return x, true
		case float64:
			return int64(x), true
		case json.Number:
			if i, err := x.Int64(); err == nil {
				return i, true
			}
			if i, err := strconv.ParseInt(string(x), 10, 64); err == nil {
				return i, true
			}
		case string:
			x = strings.TrimSpace(x)
			if x == "" {
				continue
			}
			if i, err := strconv.ParseInt(x, 10, 64); err == nil {
				return i, true
			}
		}
	}
	return 0, false
}

func argBool(args map[string]any, keys ...string) (bool, bool) {
	for _, k := range keys {
		v, ok := args[k]
		if !ok || v == nil {
			continue
		}
		switch x := v.(type) {
		case bool:
			return x, true
		case string:
			s := strings.ToLower(strings.TrimSpace(x))
			switch s {
			case "true", "1", "yes", "y", "on":
				return true, true
			case "false", "0", "no", "n", "off":
				return false, true
			}
		}
	}
	return false, false
}

func applyFailureAlertArgs(job *cron.Job, args map[string]any) error {
	after, hasAfter := argInt64(args, "failure_alert_after", "alert_after")
	cooldown, hasCooldown := argInt64(args, "failure_alert_cooldown_ms", "alert_cooldown_ms")
	if !hasAfter && !hasCooldown {
		return nil
	}
	if hasAfter && after < 1 {
		return fmt.Errorf("failure_alert_after must be >= 1")
	}
	if hasCooldown && cooldown < 0 {
		return fmt.Errorf("failure_alert_cooldown_ms must be >= 0")
	}
	alert := cron.FailureAlert{}
	if job.FailureAlert != nil {
		alert = *job.FailureAlert
	}
	if hasAfter {
		alert.After = int(after)
	}
	if hasCooldown {
		alert.CooldownMS = cooldown
	}
	job.FailureAlert = &alert
	return nil
}

// RegisterScheduleHandler registers internal handlers for the core-scheduler skill.
func RegisterScheduleHandler(db *sql.DB, svc *cron.Service) {
	if db == nil || svc == nil {
		return
	}

	RegisterInternalHandler("core-scheduler", "schedule_task", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		name := argString(args, "name")
		desc := argString(args, "description")
		prompt := argString(args, "prompt")

		kindStr := strings.ToLower(argString(args, "kind", "schedule_kind"))

		schedulePattern := argString(args, "schedule_pattern", "cron_expr")
		atStr := argString(args, "at")
		timezone := argString(args, "timezone", "tz", "time_zone")
		wakeRaw := strings.ToLower(argString(args, "wake_mode", "wake"))

		ownerTZ := timezone
		if ownerTZ == "" {
			if o, ok, err := store.GetOwner(ctx, db); err == nil && ok && strings.TrimSpace(o.Timezone) != "" {
				ownerTZ = strings.TrimSpace(o.Timezone)
			}
		}

		wake := cron.WakeNextHeartbeat
		switch wakeRaw {
		case "", "next-heartbeat", "nextheartbeat", "defer", "delayed":
			wake = cron.WakeNextHeartbeat
		case "now", "immediate", "wake":
			wake = cron.WakeNow
		default:
			if wakeRaw != "" {
				return nil, fmt.Errorf("invalid wake_mode %q", wakeRaw)
			}
		}

		job := cron.Job{
			ID:             uuid.New().String(),
			Name:           name,
			Description:    desc,
			PayloadKind:    cron.PayloadSystemEvent,
			PayloadText:    prompt,
			SessionTarget:  cron.SessionMain,
			WakeMode:       wake,
			DeleteAfterRun: false,
		}

		everyMS, hasEveryMS := argInt64(args, "every_ms", "everyms", "every")
		atTrim := strings.TrimSpace(atStr)

		switch {
		case hasEveryMS:
			if kindStr != "" && kindStr != string(cron.ScheduleEvery) {
				return nil, fmt.Errorf("every_ms conflicts with schedule_kind %q", kindStr)
			}
			if everyMS < 1 {
				return nil, fmt.Errorf("invalid every_ms")
			}
			job.Schedule.Kind = cron.ScheduleEvery
			job.Schedule.EveryMS = everyMS
			if anchor, ok := argInt64(args, "schedule_anchor_ms", "anchor_ms", "anchor"); ok {
				job.Schedule.AnchorMS = anchor
			}
			if ownerTZ != "" {
				job.Schedule.TZ = ownerTZ
			}
		case atTrim != "":
			if kindStr != "" && kindStr != string(cron.ScheduleAt) {
				return nil, fmt.Errorf("at conflicts with schedule_kind %q", kindStr)
			}
			job.Schedule.Kind = cron.ScheduleAt
			job.Schedule.AtRFC = atTrim
			if ownerTZ != "" {
				job.Schedule.TZ = ownerTZ
			}
		default:
			if kindStr == string(cron.ScheduleAt) {
				return nil, fmt.Errorf("missing at for schedule_kind %q", kindStr)
			}
			if kindStr == string(cron.ScheduleEvery) {
				return nil, fmt.Errorf("missing every_ms for schedule_kind %q", kindStr)
			}
			job.Schedule.Kind = cron.ScheduleCron
			job.Schedule.CronExpr = schedulePattern
			if ownerTZ != "" {
				job.Schedule.TZ = ownerTZ
			}
			if stagger, ok := argInt64(args, "schedule_stagger_ms", "stagger_ms", "stagger"); ok && stagger > 0 {
				job.Schedule.Stagger = stagger
			}
		}

		switch {
		case name == "", prompt == "":
			return nil, fmt.Errorf("missing required arguments for schedule_task: name, prompt")
		case job.Schedule.Kind == cron.ScheduleCron && strings.TrimSpace(job.Schedule.CronExpr) == "":
			return nil, fmt.Errorf("missing schedule_pattern for kind=cron schedule_task")
		case job.Schedule.Kind == cron.ScheduleAt && strings.TrimSpace(job.Schedule.AtRFC) == "":
			return nil, fmt.Errorf("missing at RFC3339 for kind=at schedule_task")
		case job.Schedule.Kind == cron.ScheduleEvery && job.Schedule.EveryMS < 1:
			return nil, fmt.Errorf("invalid every_ms for kind=every schedule_task")
		}

		if tm, ok := argInt64(args, "timeout_ms", "timeout"); ok && tm > 0 {
			job.TimeoutMS = tm
		}
		if del, ok := argBool(args, "delete_after_run"); ok {
			job.DeleteAfterRun = del
		}
		if err := applyFailureAlertArgs(&job, args); err != nil {
			return nil, err
		}

		created, err := svc.Create(ctx, job)
		if err != nil {
			return nil, fmt.Errorf("failed to save cron job: %w", err)
		}

		nextRFC := ""
		if created.State.NextRunAtMS > 0 {
			nextRFC = time.UnixMilli(created.State.NextRunAtMS).UTC().Format(time.RFC3339Nano)
		}

		scheduleSummary := summarizeSchedule(job.Schedule)

		return map[string]any{
			"id":               created.ID,
			"schedule_kind":    string(created.Schedule.Kind),
			"schedule":         scheduleSummary,
			"wake_mode":        string(_wakeOrDefaultForResponse(created.WakeMode)),
			"delete_after_run": created.DeleteAfterRun,
			"next_run_at":      nextRFC,
			"schedule_pattern": schedulePattern,
			"message":          "Task scheduled successfully.",
		}, nil
	})

	RegisterInternalHandler("core-scheduler", "list_tasks", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		jobs, err := svc.LoadJobs(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list cron jobs: %w", err)
		}
		list := make([]map[string]any, 0, len(jobs))
		for _, j := range jobs {
			entry := map[string]any{
				"id":            j.ID,
				"name":          j.Name,
				"description":   j.Description,
				"enabled":       j.Enabled,
				"schedule_kind": string(j.Schedule.Kind),
				"wake_mode":     string(_wakeOrDefaultForResponse(j.WakeMode)),
				"schedule":      summarizeSchedule(j.Schedule),
				"payload_kind":  string(j.PayloadKind),
			}
			if j.DeleteAfterRun {
				entry["delete_after_run"] = true
			}
			if j.FailureAlert != nil {
				entry["failure_alert"] = map[string]any{
					"after":       j.FailureAlert.After,
					"cooldown_ms": j.FailureAlert.CooldownMS,
				}
			}
			if j.State.NextRunAtMS > 0 {
				entry["next_run_at"] = time.UnixMilli(j.State.NextRunAtMS).UTC().Format(time.RFC3339Nano)
			}
			if j.Schedule.Kind == cron.ScheduleCron {
				entry["schedule_pattern"] = j.Schedule.CronExpr
			}
			list = append(list, entry)
		}
		return map[string]any{"tasks": list}, nil
	})

	RegisterInternalHandler("core-scheduler", "delete_task", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		id := argString(args, "id")
		if id == "" {
			return nil, fmt.Errorf("missing required argument for delete_task: id")
		}
		if err := svc.Delete(ctx, id); err != nil {
			return nil, fmt.Errorf("failed to delete cron job %s: %w", id, err)
		}
		return map[string]any{"success": true, "message": "Scheduled task deleted."}, nil
	})

	RegisterInternalHandler("core-scheduler", "update_task", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		id := argString(args, "id")
		if id == "" {
			return nil, fmt.Errorf("missing required argument for update_task: id")
		}
		job, ok, err := svc.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to load cron job %s: %w", id, err)
		}
		if !ok {
			return nil, fmt.Errorf("cron job %s not found", id)
		}

		if name := argString(args, "name"); name != "" {
			job.Name = name
		}
		if _, exists := args["description"]; exists {
			job.Description = strings.TrimSpace(fmt.Sprint(args["description"]))
		}
		if prompt := argString(args, "prompt"); prompt != "" {
			job.PayloadText = prompt
		}
		if enabled, ok := argBool(args, "enabled"); ok {
			job.Enabled = enabled
		}
		if del, ok := argBool(args, "delete_after_run"); ok {
			job.DeleteAfterRun = del
		}
		if tm, ok := argInt64(args, "timeout_ms", "timeout"); ok {
			if tm < 0 {
				return nil, fmt.Errorf("timeout_ms must be >= 0")
			}
			job.TimeoutMS = tm
		}
		if err := applyFailureAlertArgs(&job, args); err != nil {
			return nil, err
		}

		kindStr := strings.ToLower(argString(args, "kind", "schedule_kind"))
		schedulePattern := argString(args, "schedule_pattern", "cron_expr")
		atStr := strings.TrimSpace(argString(args, "at"))
		everyMS, hasEveryMS := argInt64(args, "every_ms", "everyms", "every")
		timezone := argString(args, "timezone", "tz", "time_zone")
		switch {
		case hasEveryMS:
			if everyMS < 1 {
				return nil, fmt.Errorf("invalid every_ms")
			}
			job.Schedule.Kind = cron.ScheduleEvery
			job.Schedule.EveryMS = everyMS
			job.Schedule.CronExpr = ""
			job.Schedule.AtRFC = ""
			if anchor, ok := argInt64(args, "schedule_anchor_ms", "anchor_ms", "anchor"); ok {
				job.Schedule.AnchorMS = anchor
			}
		case atStr != "":
			job.Schedule.Kind = cron.ScheduleAt
			job.Schedule.AtRFC = atStr
			job.Schedule.CronExpr = ""
			job.Schedule.EveryMS = 0
		case schedulePattern != "":
			job.Schedule.Kind = cron.ScheduleCron
			job.Schedule.CronExpr = schedulePattern
			job.Schedule.AtRFC = ""
			job.Schedule.EveryMS = 0
		case kindStr != "":
			switch kindStr {
			case string(cron.ScheduleAt):
				return nil, fmt.Errorf("missing at for schedule_kind %q", kindStr)
			case string(cron.ScheduleEvery):
				return nil, fmt.Errorf("missing every_ms for schedule_kind %q", kindStr)
			case string(cron.ScheduleCron):
				return nil, fmt.Errorf("missing schedule_pattern for schedule_kind %q", kindStr)
			default:
				return nil, fmt.Errorf("invalid schedule_kind %q", kindStr)
			}
		}
		if timezone != "" {
			job.Schedule.TZ = timezone
		}
		if stagger, ok := argInt64(args, "schedule_stagger_ms", "stagger_ms", "stagger"); ok {
			if stagger < 0 {
				return nil, fmt.Errorf("schedule_stagger_ms must be >= 0")
			}
			job.Schedule.Stagger = stagger
		}
		wakeRaw := strings.ToLower(argString(args, "wake_mode", "wake"))
		switch wakeRaw {
		case "":
		case "next-heartbeat", "nextheartbeat", "defer", "delayed":
			job.WakeMode = cron.WakeNextHeartbeat
		case "now", "immediate", "wake":
			job.WakeMode = cron.WakeNow
		default:
			return nil, fmt.Errorf("invalid wake_mode %q", wakeRaw)
		}

		updated, err := svc.Update(ctx, job)
		if err != nil {
			return nil, fmt.Errorf("failed to update cron job %s: %w", id, err)
		}
		nextRFC := ""
		if updated.State.NextRunAtMS > 0 {
			nextRFC = time.UnixMilli(updated.State.NextRunAtMS).UTC().Format(time.RFC3339Nano)
		}
		return map[string]any{
			"id":               updated.ID,
			"schedule_kind":    string(updated.Schedule.Kind),
			"schedule":         summarizeSchedule(updated.Schedule),
			"wake_mode":        string(_wakeOrDefaultForResponse(updated.WakeMode)),
			"delete_after_run": updated.DeleteAfterRun,
			"enabled":          updated.Enabled,
			"next_run_at":      nextRFC,
			"message":          "Scheduled task updated.",
		}, nil
	})

	RegisterInternalHandler("core-scheduler", "run_task_now", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		id := argString(args, "id")
		if id == "" {
			return nil, fmt.Errorf("missing required argument for run_task_now: id")
		}
		if err := svc.RunNow(ctx, id); err != nil {
			return nil, fmt.Errorf("failed to run cron job %s: %w", id, err)
		}
		return map[string]any{"success": true, "message": "Scheduled task run requested."}, nil
	})
}

func _wakeOrDefaultForResponse(w cron.WakeMode) cron.WakeMode {
	if w == "" {
		return cron.WakeNextHeartbeat
	}
	return w
}

func summarizeSchedule(s cron.Schedule) string {
	switch s.Kind {
	case cron.ScheduleAt:
		return s.AtRFC
	case cron.ScheduleEvery:
		anchor := ""
		if s.AnchorMS != 0 {
			anchor = fmt.Sprintf("; anchor_ms=%d", s.AnchorMS)
		}
		tz := ""
		if s.TZ != "" {
			tz = fmt.Sprintf("; tz=%s", s.TZ)
		}
		return fmt.Sprintf("every %d ms%s%s", s.EveryMS, anchor, tz)
	case cron.ScheduleCron:
		tz := ""
		if s.TZ != "" {
			tz = fmt.Sprintf("; tz=%s", s.TZ)
		}
		st := ""
		if s.Stagger > 0 {
			st = fmt.Sprintf("; stagger_ms=%d", s.Stagger)
		}
		return fmt.Sprintf("%s%s%s", s.CronExpr, tz, st)
	default:
		return ""
	}
}
