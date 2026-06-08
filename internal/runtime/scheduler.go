package runtime

import (
	"container/heap"
	"sync"
	"time"
)

// scheduledJob is a callback to run at a specific time.
type scheduledJob struct {
	at time.Time
	fn func()
}

type jobQueue []*scheduledJob

func (h jobQueue) Len() int           { return len(h) }
func (h jobQueue) Less(i, j int) bool { return h[i].at.Before(h[j].at) }
func (h jobQueue) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *jobQueue) Push(x any) { *h = append(*h, x.(*scheduledJob)) }
func (h *jobQueue) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[0 : n-1]
	return item
}

// Scheduler runs callbacks at or after a given time. Safe for concurrent use.
type Scheduler struct {
	mu     sync.Mutex
	jobs   jobQueue
	wakeCh chan struct{}
	stopCh chan struct{}
}

// NewScheduler creates a scheduler and starts its loop. Call Stop to release.
func NewScheduler() *Scheduler {
	s := &Scheduler{
		jobs:   make(jobQueue, 0),
		wakeCh: make(chan struct{}, 1),
		stopCh: make(chan struct{}),
	}
	heap.Init(&s.jobs)
	go s.loop()
	return s
}

// Schedule runs fn at or after t. If t is in the past, fn runs soon.
func (s *Scheduler) Schedule(t time.Time, fn func()) {
	s.mu.Lock()
	heap.Push(&s.jobs, &scheduledJob{at: t, fn: fn})
	s.mu.Unlock()
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}

// Stop stops the scheduler loop. Pending jobs are not run.
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

func (s *Scheduler) loop() {
	var timer *time.Timer
	for {
		s.mu.Lock()
		if s.jobs.Len() == 0 {
			s.mu.Unlock()
			select {
			case <-s.stopCh:
				return
			case <-s.wakeCh:
				continue
			}
		}

		next := heap.Pop(&s.jobs).(*scheduledJob)
		s.mu.Unlock()

		delay := time.Until(next.at)
		if delay <= 0 {
			next.fn()
			continue
		}

		if timer == nil {
			timer = time.NewTimer(delay)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(delay)
		}

		select {
		case <-s.stopCh:
			if timer != nil {
				timer.Stop()
			}
			return
		case <-timer.C:
			next.fn()
		case <-s.wakeCh:
			s.mu.Lock()
			heap.Push(&s.jobs, next)
			s.mu.Unlock()
			continue
		}
	}
}

// RunAfter runs fn after d from now. It is a convenience for Schedule(time.Now().Add(d), fn).
func (s *Scheduler) RunAfter(d time.Duration, fn func()) {
	s.Schedule(time.Now().Add(d), fn)
}
