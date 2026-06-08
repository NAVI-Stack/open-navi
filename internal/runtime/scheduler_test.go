package runtime

import (
	"sync"
	"testing"
	"time"
)

func TestSchedulerRunAfterFires(t *testing.T) {
	s := NewScheduler()
	defer s.Stop()
	done := make(chan struct{})
	s.RunAfter(20*time.Millisecond, func() { close(done) })
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("callback did not fire")
	}
}

func TestSchedulerRunAfterOrder(t *testing.T) {
	s := NewScheduler()
	defer s.Stop()
	var order []int
	var mu sync.Mutex
	s.RunAfter(30*time.Millisecond, func() {
		mu.Lock()
		order = append(order, 2)
		mu.Unlock()
	})
	s.RunAfter(10*time.Millisecond, func() {
		mu.Lock()
		order = append(order, 1)
		mu.Unlock()
	})
	time.Sleep(80 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("expected order [1,2], got %v", order)
	}
}
