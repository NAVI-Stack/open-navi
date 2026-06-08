package connectors

import (
	"fmt"
	"sync"
	"time"
)

// DiagLevel indicates the severity of a diagnostic entry.
type DiagLevel string

const (
	DiagInfo    DiagLevel = "info"
	DiagWarning DiagLevel = "warning"
	DiagError   DiagLevel = "error"
)

// Diagnostic is a single diagnostic entry recorded by the Manager.
type Diagnostic struct {
	Level     DiagLevel `json:"level"`
	Connector string    `json:"connector"`
	Message   string    `json:"message"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

const diagRingSize = 100

// DiagCollector is a thread-safe ring buffer of recent diagnostics.
type DiagCollector struct {
	mu    sync.Mutex
	items []Diagnostic
	pos   int
	full  bool
}

// NewDiagCollector creates a new DiagCollector with a fixed ring size.
func NewDiagCollector() *DiagCollector {
	return &DiagCollector{
		items: make([]Diagnostic, diagRingSize),
	}
}

// Push adds a diagnostic entry. If the ring is full, the oldest is overwritten.
func (d *DiagCollector) Push(level DiagLevel, connector, message string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	entry := Diagnostic{
		Level:     level,
		Connector: connector,
		Message:   message,
		Timestamp: time.Now().UTC(),
	}
	if err != nil {
		entry.Error = fmt.Sprintf("%v", err)
	}
	d.items[d.pos] = entry
	d.pos = (d.pos + 1) % diagRingSize
	if d.pos == 0 {
		d.full = true
	}
}

// List returns a copy of all diagnostics in chronological order (oldest first).
func (d *DiagCollector) List() []Diagnostic {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.full {
		out := make([]Diagnostic, d.pos)
		copy(out, d.items[:d.pos])
		return out
	}

	// Ring is full: [pos..end] are older, [0..pos) are newer
	out := make([]Diagnostic, diagRingSize)
	copy(out, d.items[d.pos:])
	copy(out[diagRingSize-d.pos:], d.items[:d.pos])
	return out
}
