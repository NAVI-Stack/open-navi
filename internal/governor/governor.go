package governor

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// GovernorType identifies which governor limit was tripped.
type GovernorType string

const (
	GovernorActionBudget       GovernorType = "action_budget"
	GovernorRetryLimit         GovernorType = "retry_limit"
	GovernorCostCeiling        GovernorType = "cost_ceiling"
	GovernorAutonomousDuration GovernorType = "autonomous_duration"
	GovernorRepetition         GovernorType = "repetition_limit"
)

// ErrGovernorTripped is returned when any governor limit is exceeded.
// The LLM cannot catch or reason past this error — it is Go code.
type ErrGovernorTripped struct {
	Type   GovernorType
	Limit  string
	Actual string
}

func (e *ErrGovernorTripped) Error() string {
	return fmt.Sprintf("governor tripped: %s — limit %s, actual %s", e.Type, e.Limit, e.Actual)
}

// GovernorConfig holds the hard limits for a single governor instance.
type GovernorConfig struct {
	MaxActionBudget     int
	MaxRetries          int
	CostCeiling         float64
	AutonomousDuration  time.Duration
	MaxRepetitions      int
	RestrictToWorkspace bool // Strict constraint to prevent path escapes
}

// DefaultGovernorConfig returns conservative limits suitable for a single
// autonomous agent session.
func DefaultGovernorConfig() GovernorConfig {
	return GovernorConfig{
		MaxActionBudget:     10,
		MaxRetries:          3,
		CostCeiling:         1.0,
		AutonomousDuration:  100 * time.Millisecond,
		MaxRepetitions:      3,    // Default: trip after 3 identical consecutive responses
		RestrictToWorkspace: true, // Default to secure workspace sandboxing
	}
}

// GovernorStats is a read-only snapshot of a governor's counters.
type GovernorStats struct {
	Actions  int
	Retries  int
	TotalUSD float64
	Elapsed  time.Duration
}

// repetitionState tracks consecutive identical content for a single session.
type repetitionState struct {
	last  string
	count int
}

// Governor enforces hard limits on agent behaviour. It is not a prompt.
type Governor struct {
	config        GovernorConfig
	mu            sync.Mutex
	actions       int
	retries       int
	totalUSD      float64
	startTime     time.Time
	workspaceRoot string // Set during initialization for CheckPath

	// Per-runtimeSession tracking maps
	runtimeSessionRepetitions map[string]*repetitionState // runtime_session_id → repetition state
	runtimeSessionActions     map[string]int              // runtime_session_id → action count

	// Per-connector cumulative intake cost (CIP P5). The per-connector ceiling is
	// supplied by sync policy; the Governor remains authoritative — it decides.
	connectorCosts map[string]float64 // connector_id → cumulative USD this lifetime
}

// NewGovernor creates a Governor with the given config and records the start time.
func NewGovernor(config GovernorConfig, workspaceRoot string) *Governor {
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	return &Governor{
		config:                    config,
		startTime:                 time.Now(),
		workspaceRoot:             workspaceRoot,
		runtimeSessionRepetitions: make(map[string]*repetitionState),
		runtimeSessionActions:     make(map[string]int),
		connectorCosts:            make(map[string]float64),
	}
}

// RecordAction increments the global action counter and trips if the budget is exceeded.
func (g *Governor) RecordAction() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.actions++
	if g.actions > g.config.MaxActionBudget {
		return &ErrGovernorTripped{
			Type:   GovernorActionBudget,
			Limit:  fmt.Sprintf("%d", g.config.MaxActionBudget),
			Actual: fmt.Sprintf("%d", g.actions),
		}
	}
	return nil
}

// RecordRepetition tracks consecutive identical responses for a runtime session and trips
// if MaxRepetitions is exceeded. Use runtimeSessionID="" for global contexts.
func (g *Governor) RecordRepetition(runtimeSessionID, content string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// A non-positive limit disables repetition detection. Without this guard a
	// zero limit would trip on the very first response (count 1 >= 0), wedging
	// the loop on a misconfiguration rather than turning the check off.
	if g.config.MaxRepetitions <= 0 {
		return nil
	}

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}

	st := g.runtimeSessionRepetitions[runtimeSessionID]
	if st == nil {
		st = &repetitionState{}
		g.runtimeSessionRepetitions[runtimeSessionID] = st
	}

	if trimmed == st.last {
		st.count++
	} else {
		st.last = trimmed
		st.count = 1
	}

	if st.count >= g.config.MaxRepetitions {
		return &ErrGovernorTripped{
			Type:   GovernorRepetition,
			Limit:  fmt.Sprintf("%d", g.config.MaxRepetitions),
			Actual: fmt.Sprintf("%d", st.count),
		}
	}
	return nil
}

// ResetRuntimeSessionRepetition clears the repetition state for a completed/failed runtime session.
func (g *Governor) ResetRuntimeSessionRepetition(runtimeSessionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.runtimeSessionRepetitions, runtimeSessionID)
}

// CheckPath verifies if a given file/cmd path is allowed under current constraints.
// If RestrictToWorkspace is true, it strictly enforces that the path resolves within workspaceRoot,
// including resolving symlinks to prevent workspace-escape via symlink chains.
func (g *Governor) CheckPath(targetPath string) error {
	if !g.config.RestrictToWorkspace {
		return nil
	}

	// Absolute paths are never permitted — they bypass workspace containment entirely.
	if filepath.IsAbs(targetPath) {
		return fmt.Errorf("path access denied: absolute paths blocked when restrictToWorkspace is enabled (target: %s)", targetPath)
	}

	absRoot, err := filepath.Abs(g.workspaceRoot)
	if err != nil {
		return fmt.Errorf("path access denied: cannot resolve workspace root: %w", err)
	}

	joined := filepath.Join(absRoot, targetPath)

	// Attempt to resolve symlinks. If the path doesn't exist yet (e.g. a write
	// target), fall back to the lexically-cleaned join, which still catches
	// traversal attempts via "..".
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		resolved = filepath.Clean(joined)
	}

	rel, err := filepath.Rel(absRoot, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("path access denied: %s escapes workspace (resolved: %s)", targetPath, resolved)
	}
	return nil
}

// RecordRuntimeSessionAction increments the per-runtimeSession action counter and returns an error
// if the runtimeSession exceeds the given budget limit.
func (g *Governor) RecordRuntimeSessionAction(runtimeSessionID string, limit int) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.runtimeSessionActions[runtimeSessionID]++
	count := g.runtimeSessionActions[runtimeSessionID]
	if count > limit {
		return count, fmt.Errorf("runtime session %s exceeded action budget of %d (actual: %d)", runtimeSessionID, limit, count)
	}
	return count, nil
}

// ResetRuntimeSessionBudget clears the action counter for a completed/failed runtime session.
func (g *Governor) ResetRuntimeSessionBudget(runtimeSessionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.runtimeSessionActions, runtimeSessionID)
}

// RecordRetry increments the retry counter and trips if the limit is exceeded.
func (g *Governor) RecordRetry() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.retries++
	if g.retries > g.config.MaxRetries {
		return &ErrGovernorTripped{
			Type:   GovernorRetryLimit,
			Limit:  fmt.Sprintf("%d", g.config.MaxRetries),
			Actual: fmt.Sprintf("%d", g.retries),
		}
	}
	return nil
}

// RecordCost accumulates USD cost and trips if the ceiling is exceeded.
func (g *Governor) RecordCost(usd float64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.totalUSD += usd
	if g.totalUSD > g.config.CostCeiling {
		return &ErrGovernorTripped{
			Type:   GovernorCostCeiling,
			Limit:  fmt.Sprintf("$%.4f", g.config.CostCeiling),
			Actual: fmt.Sprintf("$%.4f", g.totalUSD),
		}
	}
	return nil
}

// RecordConnectorCost accumulates per-connector intake cost (CIP P5) and trips
// when the cumulative cost for that connector exceeds the supplied ceiling. The
// ceiling is a per-connector input from sync policy; the Governor is the
// authority that decides — a policy edit cannot bypass this check. A ceiling
// <= 0 disables the per-connector limit (the global RecordCost ceiling still
// applies elsewhere). Returns the cumulative cost and a tripped error when the
// ceiling is exceeded.
func (g *Governor) RecordConnectorCost(connectorID string, usd, ceiling float64) (float64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.connectorCosts == nil {
		g.connectorCosts = make(map[string]float64)
	}
	g.connectorCosts[connectorID] += usd
	total := g.connectorCosts[connectorID]
	if ceiling > 0 && total > ceiling {
		return total, &ErrGovernorTripped{
			Type:   GovernorCostCeiling,
			Limit:  fmt.Sprintf("$%.4f (connector %s)", ceiling, connectorID),
			Actual: fmt.Sprintf("$%.4f", total),
		}
	}
	return total, nil
}

// ConnectorCost returns the cumulative recorded intake cost for a connector.
func (g *Governor) ConnectorCost(connectorID string) float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.connectorCosts[connectorID]
}

// ResetConnectorCost clears the cumulative cost for a connector (e.g. at the
// start of a fresh backfill that carries its own budget).
func (g *Governor) ResetConnectorCost(connectorID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.connectorCosts, connectorID)
}

// CheckDuration trips if the elapsed time since construction exceeds the limit.
func (g *Governor) CheckDuration() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	elapsed := time.Since(g.startTime)
	if elapsed > g.config.AutonomousDuration {
		return &ErrGovernorTripped{
			Type:   GovernorAutonomousDuration,
			Limit:  g.config.AutonomousDuration.String(),
			Actual: elapsed.String(),
		}
	}
	return nil
}

// Stats returns a read-only snapshot of the governor's current counters.
func (g *Governor) Stats() GovernorStats {
	g.mu.Lock()
	defer g.mu.Unlock()
	return GovernorStats{
		Actions:  g.actions,
		Retries:  g.retries,
		TotalUSD: g.totalUSD,
		Elapsed:  time.Since(g.startTime),
	}
}

// Limits returns a copy of the governor's configured limits (for status/operator APIs).
func (g *Governor) Limits() GovernorConfig {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.config
}
