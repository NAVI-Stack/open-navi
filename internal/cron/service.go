package cron

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/open-navi/navi/internal/config"
	"github.com/open-navi/navi/internal/store"
)

const (
	minRefireGap      = 2 * time.Second
	maxTimerDelay     = 60 * time.Second
	stuckRunningAfter = 2 * time.Hour
	scheduleErrorTrip = 5
)

// Deps wires cron against store and dispatch surfaces.
type Deps struct {
	DB              *sql.DB
	DirectiveWriter DirectiveWriter
	// ChatAppender is optional. When set, cron jobs whose SessionTarget has
	// the prefix "session:<id>" and PayloadKind=assistantMessage will deliver
	// their payload as an assistant message into that session instead of
	// creating a new directive. This is the durable backing for promoted
	// send_reply messages whose delay exceeds the in-process scheduler cap.
	ChatAppender ChatAppender
	Log             *slog.Logger
	Cron            config.CronConfig
	Now             func() time.Time
	OnFailureAlert  func(ctx context.Context, job Job, text string)
}

// Service persists and executes cron_jobs on a dynamic timer loop.
type Service struct {
	mu        sync.Mutex
	deps      Deps
	hb        HeartbeatWaker
	hbMu      sync.RWMutex
	timer     *time.Timer
	recheck   *time.Timer
	ticking   bool
	stopped   bool
	rootCtx   context.Context
	cancelRun context.CancelFunc
}

// Status summarizes the persisted cron service state.
type Status struct {
	Enabled     bool
	Total       int
	Active      int
	Running     int
	NextRunAtMS int64
}

func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{deps: d, rootCtx: ctx, cancelRun: cancel}
}

// SetHeartbeat updates the heartbeat waker while subsystems finish wiring after construction.
func (s *Service) SetHeartbeat(h HeartbeatWaker) {
	s.hbMu.Lock()
	s.hb = h
	s.hbMu.Unlock()
}

func (s *Service) heartbeat() HeartbeatWaker {
	s.hbMu.RLock()
	defer s.hbMu.RUnlock()
	return s.hb
}

// Start migrates legacy rows, executes startup catch-up, and arms timers.
func (s *Service) Start(ctx context.Context) error {
	if _, err := store.MigrateLegacyScheduledTasksToCronJobs(ctx, s.deps.DB); err != nil {
		s.deps.Log.Warn("cron: legacy migrate", "error", err)
	}
	if !s.deps.Cron.Enabled {
		s.deps.Log.Debug("cron: disabled — not starting timer")
		return nil
	}

	if err := s.runStartupCatchup(ctx); err != nil {
		s.deps.Log.Warn("cron: startup catch-up", "error", err)
	}
	s.armTimer()
	return nil
}

// Stop tears down timers only.
func (s *Service) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.stopTimersLocked()
	s.mu.Unlock()
	s.cancelRun()
}

func (s *Service) stopTimersLocked() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.recheck != nil {
		s.recheck.Stop()
		s.recheck = nil
	}
}

// LoadJobs parses all persisted rows into Job values (skips invalid rows).
func (s *Service) LoadJobs(ctx context.Context) ([]Job, error) {
	rs, err := store.ListCronJobs(ctx, s.deps.DB)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, row := range rs {
		j, err := jobFromRecord(row)
		if err != nil {
			s.deps.Log.Debug("cron: skip row", "id", row.ID, "error", err)
			continue
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (s *Service) nowUnixMs() int64 {
	return s.deps.Now().UnixMilli()
}

func (s *Service) prepareJobForWrite(ctx context.Context, j Job, ms int64) (Job, error) {
	if j.ID == "" {
		return Job{}, fmt.Errorf("cron: missing job id")
	}
	if j.OwnerID == "" {
		id, err := store.GetOwnerID(ctx, s.deps.DB)
		if err == nil && id != "" {
			j.OwnerID = id
		} else {
			j.OwnerID = "navi"
		}
	}
	if j.WakeMode == "" {
		j.WakeMode = WakeNextHeartbeat
	}
	if j.SessionTarget == "" {
		j.SessionTarget = SessionMain
	}
	if j.PayloadKind == "" {
		j.PayloadKind = PayloadSystemEvent
	}
	if err := ValidateSchedule(j.Schedule); err != nil {
		return Job{}, err
	}
	return j, nil
}

// Add persists a new validated job row. It is an alias for Create.
func (s *Service) Add(ctx context.Context, j Job) (Job, error) {
	return s.Create(ctx, j)
}

// Create persists a validated job row.
func (s *Service) Create(ctx context.Context, j Job) (Job, error) {
	ms := s.nowUnixMs()
	var err error
	j.Enabled = true
	j, err = s.prepareJobForWrite(ctx, j, ms)
	if err != nil {
		return Job{}, err
	}
	if j.CreatedAtMS == 0 {
		j.CreatedAtMS = ms
	}
	j.UpdatedAtMS = ms
	next, ok := ComputeJobNextInstant(j, s.deps.Now().UTC())
	if !ok {
		return Job{}, errors.New("cron: could not derive next schedule trigger")
	}
	j.State.NextRunAtMS = next.UnixMilli()
	rec, err := recordFromJob(j, ms)
	if err != nil {
		return Job{}, err
	}
	if err := store.InsertCronJob(ctx, s.deps.DB, rec); err != nil {
		return Job{}, err
	}
	s.armTimer()
	return j, nil
}

// Update replaces a persisted job's schedule/configuration and computes its next run.
func (s *Service) Update(ctx context.Context, j Job) (Job, error) {
	ms := s.nowUnixMs()
	existing, found, err := s.Get(ctx, j.ID)
	if err != nil {
		return Job{}, err
	}
	if !found {
		return Job{}, sql.ErrNoRows
	}
	if existing.State.RunningAtMS > 0 && ms-existing.State.RunningAtMS < stuckRunningAfter.Milliseconds() {
		return Job{}, fmt.Errorf("cron: job %s is currently running", j.ID)
	}
	j, err = s.prepareJobForWrite(ctx, j, ms)
	if err != nil {
		return Job{}, err
	}
	j.CreatedAtMS = existing.CreatedAtMS
	j.UpdatedAtMS = ms
	j.State.LastRunAtMS = existing.State.LastRunAtMS
	j.State.LastRunStatus = existing.State.LastRunStatus
	j.State.LastError = existing.State.LastError
	j.State.LastDurationMS = existing.State.LastDurationMS
	j.State.ConsecutiveErrors = existing.State.ConsecutiveErrors
	j.State.ScheduleErrorCount = existing.State.ScheduleErrorCount
	j.State.LastFailureAlertMS = existing.State.LastFailureAlertMS
	next, ok := ComputeJobNextInstant(j, s.deps.Now().UTC())
	if !ok {
		return Job{}, errors.New("cron: could not derive next schedule trigger")
	}
	j.State.NextRunAtMS = next.UnixMilli()
	rec, err := recordFromJob(j, ms)
	if err != nil {
		return Job{}, err
	}
	if err := store.UpdateCronJob(ctx, s.deps.DB, rec); err != nil {
		return Job{}, err
	}
	s.armTimer()
	return j, nil
}

// RunNow executes a persisted job immediately without changing its configured schedule.
func (s *Service) RunNow(ctx context.Context, id string) error {
	j, found, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return sql.ErrNoRows
	}
	ms := s.nowUnixMs()
	if j.State.RunningAtMS > 0 && ms-j.State.RunningAtMS < stuckRunningAfter.Milliseconds() {
		return fmt.Errorf("cron: job %s is currently running", id)
	}
	j.State.RunningAtMS = ms
	rec, err := recordFromJob(j, ms)
	if err != nil {
		return err
	}
	if err := store.UpdateCronJob(ctx, s.deps.DB, rec); err != nil {
		return err
	}
	s.runOneJob(ctx, j, ms)
	s.armTimer()
	return nil
}

// Delete removes a job by id.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := store.DeleteCronJob(ctx, s.deps.DB, id); err != nil {
		return err
	}
	s.armTimer()
	return nil
}

// Get returns a job by id.
func (s *Service) Get(ctx context.Context, id string) (Job, bool, error) {
	r, err := store.GetCronJob(ctx, s.deps.DB, id)
	if err == sql.ErrNoRows {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	j, err := jobFromRecord(*r)
	if err != nil {
		return Job{}, false, err
	}
	return j, true, nil
}

// Status returns a lightweight snapshot of persisted cron jobs.
func (s *Service) Status(ctx context.Context) (Status, error) {
	jobs, err := s.LoadJobs(ctx)
	if err != nil {
		return Status{}, err
	}
	st := Status{Enabled: s.deps.Cron.Enabled, Total: len(jobs)}
	for _, j := range jobs {
		if j.Enabled {
			st.Active++
		}
		if j.State.RunningAtMS > 0 {
			st.Running++
		}
		if j.Enabled && j.State.NextRunAtMS > 0 && (st.NextRunAtMS == 0 || j.State.NextRunAtMS < st.NextRunAtMS) {
			st.NextRunAtMS = j.State.NextRunAtMS
		}
	}
	return st, nil
}

func (s *Service) armTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	if s.ticking {
		s.scheduleRecheckLocked()
		return
	}
	s.stopTimersLocked()
	nextDue, has := s.nextWakeMsLocked()
	n := s.deps.Now().UnixMilli()
	var delay time.Duration
	if !has {
		delay = maxTimerDelay
	} else {
		delay = clampCronTimerDelay(n, nextDue, minRefireGap, maxTimerDelay)
	}
	s.timer = time.AfterFunc(delay, func() { s.onTimer() })
}

func (s *Service) scheduleRecheckLocked() {
	if s.recheck != nil {
		s.recheck.Stop()
	}
	s.recheck = time.AfterFunc(maxTimerDelay, func() { s.onTimer() })
}

func (s *Service) nextWakeMsLocked() (int64, bool) {
	ctx := s.rootCtx
	// Use the SQL-filtered + ASC-sorted helper instead of LoadJobs() so we avoid
	// scanning + decoding every cron_jobs row on every timer arm.
	recs, err := store.ListEnabledCronJobsWithNext(ctx, s.deps.DB)
	if err != nil {
		return 0, false
	}
	nowMs := s.deps.Now().UnixMilli()
	stuckMs := stuckRunningAfter.Milliseconds()
	for _, r := range recs {
		if !r.NextRunAtMS.Valid || r.NextRunAtMS.Int64 <= 0 {
			continue
		}
		// Skip jobs currently running (not yet considered stuck). Rows are
		// already sorted ASC by next_run_at_ms in SQL, so the first non-running
		// row wins.
		if r.RunningAtMS.Valid && r.RunningAtMS.Int64 > 0 && nowMs-r.RunningAtMS.Int64 < stuckMs {
			continue
		}
		return r.NextRunAtMS.Int64, true
	}
	return 0, false
}

func (s *Service) onTimer() {
	s.mu.Lock()
	if s.stopped || s.ticking {
		s.scheduleRecheckLocked()
		s.mu.Unlock()
		return
	}
	s.ticking = true
	s.scheduleRecheckLocked()
	s.mu.Unlock()

	now := s.deps.Now()
	nowMs := now.UnixMilli()
	ctx := s.rootCtx
	rows, err := store.GetRunnableCronJobs(ctx, s.deps.DB, nowMs)
	if err != nil {
		s.deps.Log.Warn("cron: timer load", "error", err)
		s.finishTick()
		s.armTimer()
		return
	}
	type due struct{ j Job }
	var dueJobs []due
	for _, row := range rows {
		j, err := jobFromRecord(row)
		if err != nil {
			s.deps.Log.Debug("cron: skip runnable row", "id", row.ID, "error", err)
			continue
		}
		if j.State.RunningAtMS > 0 && nowMs-j.State.RunningAtMS < stuckRunningAfter.Milliseconds() {
			continue
		}
		dueJobs = append(dueJobs, due{j: j})
	}
	sort.Slice(dueJobs, func(i, j int) bool { return dueJobs[i].j.State.NextRunAtMS < dueJobs[j].j.State.NextRunAtMS })

	pool := s.deps.Cron.MaxConcurrentRuns
	if pool < 1 {
		pool = 1
	}
	sem := make(chan struct{}, pool)
	var wg sync.WaitGroup
	for _, d := range dueJobs {
		dj := d.j
		dj.State.RunningAtMS = nowMs
		rec, er := recordFromJob(dj, nowMs)
		if er != nil {
			s.deps.Log.Error("cron: mark-running encode failed; skipping job this tick",
				"job_id", dj.ID, "error", er)
			continue
		}
		if err := store.UpdateCronJob(ctx, s.deps.DB, rec); err != nil {
			s.deps.Log.Warn("cron: mark running", "id", dj.ID, "error", err)
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(job Job) {
			defer wg.Done()
			defer func() { <-sem }()
			s.runOneJob(ctx, job, nowMs)
		}(dj)
	}
	wg.Wait()
	s.finishTick()
	s.armTimer()
}

func (s *Service) finishTick() {
	s.mu.Lock()
	s.ticking = false
	if s.recheck != nil {
		s.recheck.Stop()
		s.recheck = nil
	}
	s.mu.Unlock()
}

func timeoutForJob(j Job, defMS int64) time.Duration {
	t := j.TimeoutMS
	if t <= 0 {
		t = defMS
	}
	return time.Duration(t) * time.Millisecond
}

func (s *Service) runOneJob(ctx context.Context, j Job, markRunningMS int64) {
	_ = markRunningMS
	start := s.deps.Now()
	to := timeoutForJob(j, s.deps.Cron.DefaultTimeoutMs)
	jobCtx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	res := executeDispatch(jobCtx, s.deps.DirectiveWriter, s.deps.ChatAppender, s.heartbeat(), j)
	if errors.Is(jobCtx.Err(), context.DeadlineExceeded) {
		res = RunResult{Status: RunError, Error: "cron: job execution timed out"}
	}
	end := s.deps.Now()

	// Each early-return path between "marked running" and "outcome persisted"
	// risks leaving running_at_ms set in the database for up to stuckRunningAfter
	// (2h). Best-effort reset on each failure so the job is picked up on the
	// next tick instead of being silently hung.
	j2, found, err := s.Get(context.Background(), j.ID)
	if !found || err != nil {
		s.deps.Log.Error("cron: outcome refresh failed; job may be stuck running",
			"job_id", j.ID, "found", found, "error", err)
		s.clearRunningBestEffort(ctx, j.ID, "outcome_refresh")
		return
	}
	s.applyOutcome(&j2, start, end, res)
	if j2.Schedule.Kind == ScheduleAt && j2.DeleteAfterRun && res.Status == RunOK {
		if derr := store.DeleteCronJob(ctx, s.deps.DB, j.ID); derr != nil {
			s.deps.Log.Warn("cron: delete-after-run failed",
				"job_id", j.ID, "error", derr)
		}
		return
	}
	// Targeted UPDATE of only outcome-owned columns. Avoids the read-modify-write
	// race where a concurrent configuration edit (name, payload, schedule) could
	// be clobbered by the full-row write at the end of runOneJob.
	outcome := store.CronJobOutcome{
		Enabled:            j2.Enabled,
		NextRunAtMS:        nullableInt64(j2.State.NextRunAtMS),
		RunningAtMS:        nullableInt64(j2.State.RunningAtMS),
		LastRunAtMS:        nullableInt64(j2.State.LastRunAtMS),
		LastRunStatus:      nullableString(string(j2.State.LastRunStatus)),
		LastError:          nullableString(j2.State.LastError),
		LastDurationMS:     nullableInt64(j2.State.LastDurationMS),
		ConsecutiveErrors:  j2.State.ConsecutiveErrors,
		ScheduleErrorCount: j2.State.ScheduleErrorCount,
		LastFailureAlertMS: nullableInt64(j2.State.LastFailureAlertMS),
		UpdatedAtMS:        s.deps.Now().UnixMilli(),
	}
	if err := store.UpdateCronJobOutcome(ctx, s.deps.DB, j.ID, outcome); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Row gone (concurrent Delete) - no recovery needed.
			s.deps.Log.Info("cron: outcome row missing (deleted concurrently)",
				"job_id", j.ID)
			return
		}
		s.deps.Log.Error("cron: outcome update failed; job may be stuck running",
			"job_id", j.ID, "error", err)
		s.clearRunningBestEffort(ctx, j.ID, "outcome_update")
	}
}

// clearRunningBestEffort attempts to reset running_at_ms=0 for a job whose
// post-execution outcome could not be persisted. Targeted UPDATE - touches
// only running_at_ms + updated_at_ms, so it cannot clobber concurrent edits.
func (s *Service) clearRunningBestEffort(ctx context.Context, jobID, phase string) {
	err := store.ClearCronJobRunning(ctx, s.deps.DB, jobID, s.deps.Now().UnixMilli())
	if err == nil {
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		// Row deleted concurrently - nothing to clear.
		return
	}
	s.deps.Log.Error("cron: clear-running persist failed",
		"job_id", jobID, "phase", phase, "error", err)
}

func nullableInt64(v int64) sql.NullInt64 {
	if v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Valid: true, Int64: v}
}

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{Valid: true, String: s}
}

func (s *Service) failureAlertCooldownMS(j Job) int64 {
	cfg := j.FailureAlert
	if cfg != nil && cfg.CooldownMS >= 0 {
		return cfg.CooldownMS
	}
	if s.deps.Cron.FailureAlert.Enabled && s.deps.Cron.FailureAlert.CooldownMs > 0 {
		return s.deps.Cron.FailureAlert.CooldownMs
	}
	return time.Hour.Milliseconds()
}

func (s *Service) failureAlertAfter(j Job) int {
	cfg := j.FailureAlert
	if cfg != nil && cfg.After > 0 {
		return cfg.After
	}
	a := s.deps.Cron.FailureAlert.After
	if a <= 0 {
		return 2
	}
	return a
}

func (s *Service) emitFailureAlert(j *Job, errMsg string) {
	if !(s.deps.Cron.FailureAlert.Enabled || j.FailureAlert != nil) {
		return
	}
	th := s.failureAlertAfter(*j)
	cd := s.failureAlertCooldownMS(*j)
	nowMs := s.deps.Now().UnixMilli()
	if j.State.ConsecutiveErrors >= int64(th) && (j.State.LastFailureAlertMS == 0 || nowMs-j.State.LastFailureAlertMS >= cd) {
		txt := `Cron job "` + j.Name + `" failed ` + strconv.Itoa(int(j.State.ConsecutiveErrors)) + ` times. Last error: ` + truncateFail(errMsg, 200)
		j.State.LastFailureAlertMS = nowMs
		if s.deps.OnFailureAlert != nil {
			s.deps.OnFailureAlert(context.Background(), *j, txt)
		} else {
			s.deps.Log.Warn(txt, "job_id", j.ID)
		}
	}
}

func truncateFail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func (s *Service) applyOutcome(j *Job, started, ended time.Time, rr RunResult) {
	cfg := s.deps.Cron
	j.State.RunningAtMS = 0
	j.State.LastRunAtMS = started.UnixMilli()
	j.State.LastDurationMS = ended.Sub(started).Milliseconds()
	j.State.LastRunStatus = rr.Status
	j.State.LastError = rr.Error
	j.UpdatedAtMS = ended.UnixMilli()

	if rr.Status == RunError {
		j.State.ConsecutiveErrors++
		s.emitFailureAlert(j, rr.Error)
	} else {
		j.State.ConsecutiveErrors = 0
		j.State.LastFailureAlertMS = 0
	}

	if j.Schedule.Kind == ScheduleAt {
		if rr.Status == RunOK || rr.Status == RunSkipped {
			j.Enabled = false
			j.State.NextRunAtMS = 0
			j.State.ScheduleErrorCount = 0
			return
		}
		retryCfg := cfg.Retry
		trans := IsTransientError(rr.Error, retryCfg.RetryOn)
		if trans && int(j.State.ConsecutiveErrors) <= retryCfg.MaxAttempts {
			off := ErrorBackoffMs(int(j.State.ConsecutiveErrors), retryCfg.BackoffMs)
			j.State.NextRunAtMS = ended.UnixMilli() + off
		} else {
			j.Enabled = false
			j.State.NextRunAtMS = 0
		}
		j.State.ScheduleErrorCount = 0
		return
	}

	nt, ntOK := ComputeJobNextInstant(*j, ended.UTC())
	if !ntOK {
		j.State.ScheduleErrorCount++
		if j.State.ScheduleErrorCount >= scheduleErrorTrip {
			j.Enabled = false
			j.State.NextRunAtMS = 0
			return
		}
		j.State.NextRunAtMS = ended.UnixMilli() + ErrorBackoffMs(int(max64(1, j.State.ConsecutiveErrors)), cfg.Retry.BackoffMs)
		return
	}
	j.State.ScheduleErrorCount = 0
	next := nt.UnixMilli()

	if rr.Status == RunError || rr.Status == RunSkipped {
		conv := max64(1, j.State.ConsecutiveErrors)
		backstop := ended.UnixMilli() + ErrorBackoffMs(int(conv), cfg.Retry.BackoffMs)
		next = max64(next, backstop)
	}
	if j.Schedule.Kind == ScheduleCron {
		next = max64(next, ended.UnixMilli()+minRefireGap.Milliseconds())
	}
	j.State.NextRunAtMS = next
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (s *Service) runStartupCatchup(ctx context.Context) error {
	ms := s.nowUnixMs()
	jobs, err := s.LoadJobs(ctx)
	if err != nil {
		return err
	}
	var overdue []Job
	for _, j := range jobs {
		if j.Enabled && j.State.NextRunAtMS > 0 && j.State.NextRunAtMS <= ms {
			overdue = append(overdue, j)
		}
	}
	if len(overdue) == 0 {
		return nil
	}
	sort.Slice(overdue, func(i, j int) bool { return overdue[i].State.NextRunAtMS < overdue[j].State.NextRunAtMS })

	maxImmediate := s.deps.Cron.MaxMissedJobsPerRestart
	if maxImmediate < 1 {
		maxImmediate = 5
	}
	immediate := overdue
	deferredIDs := []string(nil)
	if len(immediate) > maxImmediate {
		for _, j := range immediate[maxImmediate:] {
			deferredIDs = append(deferredIDs, j.ID)
		}
		immediate = immediate[:maxImmediate]
	}

	for _, j := range immediate {
		rec, er := recordFromJob(j, ms)
		if er != nil {
			continue
		}
		j.State.RunningAtMS = ms
		rec.RunningAtMS = sql.NullInt64{Valid: true, Int64: ms}
		if err := store.UpdateCronJob(ctx, s.deps.DB, rec); err != nil {
			s.deps.Log.Warn("cron catchup mark running", "id", j.ID, "error", err)
			continue
		}
		s.runOneJob(ctx, j, ms)
	}

	stagger := s.deps.Cron.MissedJobStaggerMs
	if stagger < 500 {
		stagger = 5_000
	}
	offset := int64(0)
	base := ms
	for _, did := range deferredIDs {
		offset += stagger
		jj, ok, err := s.Get(ctx, did)
		if !ok || err != nil {
			continue
		}
		jj.State.NextRunAtMS = base + offset
		rec, er := recordFromJob(jj, s.nowUnixMs())
		if er != nil {
			continue
		}
		_ = store.UpdateCronJob(ctx, s.deps.DB, rec)
	}
	return nil
}
