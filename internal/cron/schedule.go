package cron

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

var cronExprParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

func computeNextInstantPlain(sched Schedule, now time.Time) (next time.Time, ok bool) {
	switch sched.Kind {
	case ScheduleAt:
		if sched.AtRFC == "" {
			return time.Time{}, false
		}
		t, err := time.Parse(time.RFC3339Nano, sched.AtRFC)
		if err != nil {
			t, err = time.Parse(time.RFC3339, sched.AtRFC)
		}
		if err != nil {
			return time.Time{}, false
		}
		if !t.After(now) {
			return time.Time{}, false
		}
		return t.UTC(), true
	case ScheduleEvery:
		e := sched.EveryMS
		if e < 1 {
			return time.Time{}, false
		}
		nowMs := now.UnixMilli()
		anchor := sched.AnchorMS
		if anchor < 0 {
			anchor = 0
		}
		ev := maxInt64(e, 1)
		if nowMs < anchor {
			t := time.UnixMilli(anchor).UTC()
			return t, true
		}
		elapsed := nowMs - anchor
		steps := maxInt64((elapsed+ev-1)/ev, 1)
		return time.UnixMilli(anchor + steps*ev).UTC(), true
	case ScheduleCron:
		if sched.CronExpr == "" {
			return time.Time{}, false
		}
		parserSched, err := cronExprParser.Parse(sched.CronExpr)
		if err != nil {
			return time.Time{}, false
		}
		loc := loadTZ(sched.TZ)
		from := now.In(loc)
		n := parserSched.Next(from)
		if !n.After(from) {
			nextSecond := from.Add(time.Second)
			n = parserSched.Next(nextSecond)
			if !n.After(from) {
				tmr := tomorrowUTC(now)
				n = parserSched.Next(tmr.In(loc))
				if !n.After(from) {
					return time.Time{}, false
				}
			}
		}
		return n.UTC(), true
	default:
		return time.Time{}, false
	}
}

func tomorrowUTC(t time.Time) time.Time {
	base := time.Date(t.UTC().Year(), t.UTC().Month(), t.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return base.Add(24 * time.Hour)
}

func loadTZ(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// ComputeJobNextInstant returns next UTC scheduled instant for j (OpenClaw-style stagger for cron kind).
func ComputeJobNextInstant(j Job, now time.Time) (time.Time, bool) {
	if j.Schedule.Kind == ScheduleCron && j.Schedule.Stagger > 1 {
		nextPlain := func(from time.Time) (time.Time, bool) {
			js := j
			s := js.Schedule
			s.Stagger = 0 // compute base without stagger recursion
			js.Schedule = s
			return computeNextInstantPlain(js.Schedule, from)
		}
		return NextStaggeredCronRun(j.ID, j.Schedule, now, nextPlain)
	}
	return computeNextInstantPlain(j.Schedule, now)
}

// ValidateSchedule returns an error when the schedule definition is malformed.
func ValidateSchedule(sched Schedule) error {
	switch sched.Kind {
	case ScheduleAt:
		if _, err := time.Parse(time.RFC3339Nano, sched.AtRFC); err != nil {
			if _, err2 := time.Parse(time.RFC3339, sched.AtRFC); err2 != nil {
				return fmt.Errorf("invalid at time: %w", err)
			}
		}
		return nil
	case ScheduleEvery:
		if sched.EveryMS < 1 {
			return fmt.Errorf("everyMs must be >= 1")
		}
		return nil
	case ScheduleCron:
		if _, err := cronExprParser.Parse(sched.CronExpr); err != nil {
			return fmt.Errorf("invalid cron expr: %w", err)
		}
		if sched.TZ != "" {
			if _, err := time.LoadLocation(sched.TZ); err != nil {
				return fmt.Errorf("invalid timezone %q: %w", sched.TZ, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown schedule kind")
	}
}
