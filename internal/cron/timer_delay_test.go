package cron

import (
	"testing"
	"time"
)

func TestClampCronTimerDelay(t *testing.T) {
	t.Parallel()
	minG := 2 * time.Second
	maxD := 60 * time.Second
	now := int64(1_700_000_000_000)
	t.Run("fires_at_least_min_gap_when_past_due", func(t *testing.T) {
		d := clampCronTimerDelay(now, now-60_000, minG, maxD)
		if d != minG {
			t.Fatalf("got %v want %v", d, minG)
		}
	})
	t.Run("fires_at_max_when_far_future_unknown", func(t *testing.T) {
		d := clampCronTimerDelay(now, -1, minG, maxD)
		if d != maxD {
			t.Fatalf("got %v want %v", d, maxD)
		}
	})
	t.Run("clamps_to_max_when_delay_huge", func(t *testing.T) {
		future := now + int64(maxD/time.Millisecond) + 5_000
		d := clampCronTimerDelay(now, future, minG, maxD)
		if d != maxD {
			t.Fatalf("got %v want %v", d, maxD)
		}
	})
}
