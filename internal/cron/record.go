package cron

import (
	"database/sql"
	"encoding/json"

	"github.com/open-navi/navi/internal/store"
)

func jobFromRecord(r store.CronJobRecord) (Job, error) {
	sk := ScheduleKind(r.ScheduleKind)
	sched := Schedule{Kind: sk}
	if r.ScheduleTZ.Valid && r.ScheduleTZ.String != "" {
		sched.TZ = r.ScheduleTZ.String
	}
	switch sk {
	case ScheduleAt:
		if r.ScheduleExpr.Valid {
			sched.AtRFC = r.ScheduleExpr.String
		}
	case ScheduleEvery:
		if r.ScheduleEveryMS.Valid {
			sched.EveryMS = r.ScheduleEveryMS.Int64
		}
		if r.ScheduleAnchorMS.Valid {
			sched.AnchorMS = r.ScheduleAnchorMS.Int64
		}
	case ScheduleCron:
		if r.ScheduleExpr.Valid {
			sched.CronExpr = r.ScheduleExpr.String
		}
		if r.ScheduleStaggerMS.Valid {
			sched.Stagger = r.ScheduleStaggerMS.Int64
		}
	}
	session := SessionTarget(r.SessionTarget)
	wake := WakeMode(r.WakeMode)
	pk := PayloadKind(r.PayloadKind)
	state := JobState{
		ConsecutiveErrors:  r.ConsecutiveErrors,
		ScheduleErrorCount: r.ScheduleErrorCount,
	}
	if r.NextRunAtMS.Valid {
		state.NextRunAtMS = r.NextRunAtMS.Int64
	}
	if r.RunningAtMS.Valid {
		state.RunningAtMS = r.RunningAtMS.Int64
	}
	if r.LastRunAtMS.Valid {
		state.LastRunAtMS = r.LastRunAtMS.Int64
	}
	if r.LastRunStatus.Valid && r.LastRunStatus.String != "" {
		state.LastRunStatus = RunStatus(r.LastRunStatus.String)
	}
	if r.LastError.Valid {
		state.LastError = r.LastError.String
	}
	if r.LastDurationMS.Valid {
		state.LastDurationMS = r.LastDurationMS.Int64
	}
	if r.LastFailureAlertMS.Valid {
		state.LastFailureAlertMS = r.LastFailureAlertMS.Int64
	}
	del := Delivery{}
	if r.DeliveryJSON.Valid && r.DeliveryJSON.String != "" {
		del.RawJSON = r.DeliveryJSON.String
	}
	var fail *FailureAlert
	if r.FailureAlertJSON.Valid && r.FailureAlertJSON.String != "" && r.FailureAlertJSON.String != "null" {
		var alert failureAlertPersist
		if err := json.Unmarshal([]byte(r.FailureAlertJSON.String), &alert); err == nil {
			fail = &FailureAlert{
				After:      alert.After,
				CooldownMS: alert.CooldownMs,
			}
		}
	}
	job := Job{
		ID:             r.ID,
		OwnerID:        r.OwnerID,
		Name:           r.Name,
		Description:    r.Description,
		Enabled:        r.Enabled,
		Schedule:       sched,
		SessionTarget:  session,
		WakeMode:       wake,
		PayloadKind:    pk,
		PayloadText:    r.PayloadText,
		Delivery:       del,
		FailureAlert:   fail,
		DeleteAfterRun: r.DeleteAfterRun,
		State:          state,
		CreatedAtMS:    r.CreatedAtMS,
		UpdatedAtMS:    r.UpdatedAtMS,
	}
	if r.TimeoutMS.Valid {
		job.TimeoutMS = r.TimeoutMS.Int64
	}
	return job, ValidateSchedule(sched)
}

type failureAlertPersist struct {
	After      int   `json:"after"`
	CooldownMs int64 `json:"cooldown_ms"`
}

func recordFromJob(j Job, nowMS int64) (store.CronJobRecord, error) {
	if err := ValidateSchedule(j.Schedule); err != nil {
		return store.CronJobRecord{}, err
	}
	rec := store.CronJobRecord{
		ID:                 j.ID,
		OwnerID:            j.OwnerID,
		Name:               j.Name,
		Description:        j.Description,
		Enabled:            j.Enabled,
		ScheduleKind:       string(j.Schedule.Kind),
		SessionTarget:      string(j.SessionTarget),
		WakeMode:           string(j.WakeMode),
		PayloadKind:        string(j.PayloadKind),
		PayloadText:        j.PayloadText,
		DeleteAfterRun:     j.DeleteAfterRun,
		ConsecutiveErrors:  j.State.ConsecutiveErrors,
		ScheduleErrorCount: j.State.ScheduleErrorCount,
		CreatedAtMS:        j.CreatedAtMS,
		UpdatedAtMS:        nowMS,
	}
	if j.CreatedAtMS == 0 {
		rec.CreatedAtMS = nowMS
		j.CreatedAtMS = nowMS
	}
	switch j.Schedule.Kind {
	case ScheduleAt:
		rec.ScheduleExpr = sql.NullString{String: j.Schedule.AtRFC, Valid: true}
		if j.Schedule.TZ != "" {
			rec.ScheduleTZ = sql.NullString{String: j.Schedule.TZ, Valid: true}
		}
	case ScheduleCron:
		rec.ScheduleExpr = sql.NullString{String: j.Schedule.CronExpr, Valid: true}
		if j.Schedule.TZ != "" {
			rec.ScheduleTZ = sql.NullString{String: j.Schedule.TZ, Valid: true}
		}
		if j.Schedule.Stagger > 0 {
			rec.ScheduleStaggerMS = sql.NullInt64{Int64: j.Schedule.Stagger, Valid: true}
		}
	case ScheduleEvery:
		rec.ScheduleEveryMS = sql.NullInt64{Int64: j.Schedule.EveryMS, Valid: true}
		if j.Schedule.AnchorMS != 0 {
			rec.ScheduleAnchorMS = sql.NullInt64{Int64: j.Schedule.AnchorMS, Valid: true}
		}
		if j.Schedule.TZ != "" {
			rec.ScheduleTZ = sql.NullString{String: j.Schedule.TZ, Valid: true}
		}
	}
	if j.TimeoutMS > 0 {
		rec.TimeoutMS = sql.NullInt64{Int64: j.TimeoutMS, Valid: true}
	}
	next := j.State.NextRunAtMS
	if next > 0 {
		rec.NextRunAtMS = sql.NullInt64{Int64: next, Valid: true}
	}
	if j.State.RunningAtMS > 0 {
		rec.RunningAtMS = sql.NullInt64{Int64: j.State.RunningAtMS, Valid: true}
	}
	if j.State.LastRunAtMS > 0 {
		rec.LastRunAtMS = sql.NullInt64{Int64: j.State.LastRunAtMS, Valid: true}
	}
	if j.State.LastRunStatus != "" {
		rec.LastRunStatus = sql.NullString{String: string(j.State.LastRunStatus), Valid: true}
	}
	if j.State.LastError != "" {
		rec.LastError = sql.NullString{String: j.State.LastError, Valid: true}
	}
	if j.State.LastDurationMS > 0 {
		rec.LastDurationMS = sql.NullInt64{Int64: j.State.LastDurationMS, Valid: true}
	}
	if j.State.LastFailureAlertMS > 0 {
		rec.LastFailureAlertMS = sql.NullInt64{Int64: j.State.LastFailureAlertMS, Valid: true}
	}
	if j.Delivery.RawJSON != "" {
		rec.DeliveryJSON = sql.NullString{String: j.Delivery.RawJSON, Valid: true}
	}
	if j.FailureAlert != nil {
		payload, err := json.Marshal(j.FailureAlert)
		if err == nil {
			rec.FailureAlertJSON = sql.NullString{String: string(payload), Valid: true}
		}
	}
	return rec, nil
}
