package cron

import (
	"testing"
)

func TestStableStaggerOffsetMS_deterministic(t *testing.T) {
	t.Parallel()
	a := StableStaggerOffsetMS("job-alpha", 600_000)
	b := StableStaggerOffsetMS("job-alpha", 600_000)
	if a != b {
		t.Fatalf("unstable offset %d vs %d", a, b)
	}
	if a < 0 || a >= 600_000 {
		t.Fatalf("offset out of range %d", a)
	}
}
