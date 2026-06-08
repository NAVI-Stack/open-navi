package cron

import "testing"

func TestIsTransientError(t *testing.T) {
	t.Parallel()
	if !IsTransientError("server returned 529 overloaded", []string{}) {
		t.Fatal("expected transient")
	}
	if IsTransientError("permanent fatal business rule violated", []string{}) {
		t.Fatal("expected non-transient")
	}
	if !IsTransientError("ECONNRESET during connect", []string{"network"}) {
		t.Fatal("expected network transient")
	}
}
