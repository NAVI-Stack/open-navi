package runtime

import (
	"sync"
	"time"
)

// RuntimeMetrics captures high-level streaming runtime metrics for operators.
// It is intentionally simple and in-memory; callers snapshot via Snapshot().
type RuntimeMetrics struct {
	mu sync.Mutex

	TotalRunsStarted   int `json:"total_runs_started"`
	TotalRunsCompleted int `json:"total_runs_completed"`
	TotalRunsFailed    int `json:"total_runs_failed"`
	TotalRunsCancelled int `json:"total_runs_cancelled"`

	TotalToolCalls int `json:"total_tool_calls"`

	// Latency aggregates in milliseconds.
	TotalRunDurationMs   int64 `json:"total_run_duration_ms"`
	MaxRunDurationMs     int64 `json:"max_run_duration_ms"`
	TotalTTFTMs          int64 `json:"total_ttft_ms"`
	MaxTTFTMs            int64 `json:"max_ttft_ms"`
	ObservedTTFTSamples  int   `json:"observed_ttft_samples"`
	ObservedRunDurations int   `json:"observed_run_durations"`
}

// RuntimeMetricsSnapshot is a lock-free copy of RuntimeMetrics for transport.
type RuntimeMetricsSnapshot struct {
	TotalRunsStarted     int   `json:"total_runs_started"`
	TotalRunsCompleted   int   `json:"total_runs_completed"`
	TotalRunsFailed      int   `json:"total_runs_failed"`
	TotalRunsCancelled   int   `json:"total_runs_cancelled"`
	TotalToolCalls       int   `json:"total_tool_calls"`
	TotalRunDurationMs   int64 `json:"total_run_duration_ms"`
	MaxRunDurationMs     int64 `json:"max_run_duration_ms"`
	TotalTTFTMs          int64 `json:"total_ttft_ms"`
	MaxTTFTMs            int64 `json:"max_ttft_ms"`
	ObservedTTFTSamples  int   `json:"observed_ttft_samples"`
	ObservedRunDurations int   `json:"observed_run_durations"`
}

var defaultRuntimeMetrics = &RuntimeMetrics{}

// DefaultMetrics exposes the process-wide runtime metrics collector.
func DefaultMetrics() *RuntimeMetrics {
	return defaultRuntimeMetrics
}

func (m *RuntimeMetrics) Snapshot() RuntimeMetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return RuntimeMetricsSnapshot{
		TotalRunsStarted:     m.TotalRunsStarted,
		TotalRunsCompleted:   m.TotalRunsCompleted,
		TotalRunsFailed:      m.TotalRunsFailed,
		TotalRunsCancelled:   m.TotalRunsCancelled,
		TotalToolCalls:       m.TotalToolCalls,
		TotalRunDurationMs:   m.TotalRunDurationMs,
		MaxRunDurationMs:     m.MaxRunDurationMs,
		TotalTTFTMs:          m.TotalTTFTMs,
		MaxTTFTMs:            m.MaxTTFTMs,
		ObservedTTFTSamples:  m.ObservedTTFTSamples,
		ObservedRunDurations: m.ObservedRunDurations,
	}
}

func (m *RuntimeMetrics) RecordRunStarted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalRunsStarted++
}

func (m *RuntimeMetrics) RecordRunCompleted(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalRunsCompleted++
	ms := duration.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	m.TotalRunDurationMs += ms
	m.ObservedRunDurations++
	if ms > m.MaxRunDurationMs {
		m.MaxRunDurationMs = ms
	}
}

func (m *RuntimeMetrics) RecordRunFailed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalRunsFailed++
}

func (m *RuntimeMetrics) RecordRunCancelled() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalRunsCancelled++
}

func (m *RuntimeMetrics) RecordToolCall() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalToolCalls++
}

// recordTTFT records time-to-first-token for a run. Callers should only record
// when a streaming provider is active and a first delta was observed.
func (m *RuntimeMetrics) RecordTTFT(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ms := d.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	m.TotalTTFTMs += ms
	m.ObservedTTFTSamples++
	if ms > m.MaxTTFTMs {
		m.MaxTTFTMs = ms
	}
}
