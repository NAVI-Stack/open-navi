package cron

import (
	"crypto/sha256"
	"encoding/binary"
	"time"
)

// StableStaggerOffsetMS returns deterministic 0 … staggerMS−1 from job id (OpenClaw-style).
func StableStaggerOffsetMS(jobID string, staggerMS int64) int64 {
	if staggerMS <= 1 {
		return 0
	}
	h := sha256.Sum256([]byte(jobID))
	v := binary.BigEndian.Uint32(h[:4])
	return int64(v) % staggerMS
}

// NextStaggeredCronRun returns the next fire instant for a cron schedule with optional stagger window.
func NextStaggeredCronRun(jobID string, sched Schedule, now time.Time, nextPlain func(from time.Time) (time.Time, bool)) (time.Time, bool) {
	if sched.Kind != ScheduleCron || sched.Stagger <= 1 {
		return nextPlain(now)
	}
	offset := StableStaggerOffsetMS(jobID, sched.Stagger)
	if offset <= 0 {
		return nextPlain(now)
	}
	off := time.Duration(offset) * time.Millisecond
	cursor := now.Add(-off)
	const maxAttempts = 4
	for attempt := 0; attempt < maxAttempts; attempt++ {
		baseNext, ok := nextPlain(cursor)
		if !ok {
			return time.Time{}, false
		}
		shifted := baseNext.Add(off)
		if shifted.After(now) {
			return shifted, true
		}
		cursor = baseNext.Add(time.Millisecond)
	}
	return time.Time{}, false
}
