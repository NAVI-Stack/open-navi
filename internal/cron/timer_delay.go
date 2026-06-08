package cron

import "time"

// clampCronTimerDelay maps next-due millis to timer delay with refire guards.
func clampCronTimerDelay(nowUnixMilli, nextDueUnixMilli int64, minGap, maxDelay time.Duration) time.Duration {
	if nextDueUnixMilli <= 0 {
		return maxDelay
	}
	delay := time.Duration(nextDueUnixMilli-nowUnixMilli) * time.Millisecond
	if delay < minGap {
		return minGap
	}
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}
