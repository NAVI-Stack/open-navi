// Package contextread implements query_context: the single governed, read-only
// mediation surface that Python deciders (and any extension surface) use to pull
// scoped context from the kernel.
//
// It is the Go-side enforcement of Language-Layer Contract §4 ("Reads are governed
// too"): every read is purpose-bound, scope-limited, redacted, provenance-tagged,
// audit-attributed, least-context, and policy-aware. There is deliberately no
// generic world-model- or DB-shaped query here — only an enumerated set of
// (purpose, scope) pairs, each backed by a specific reuse of an existing kernel
// read path.
//
// The package is read-only by construction: it depends on a narrow read interface
// and an append-only audit sink. It holds no database handle and exposes no
// write, mutate, or schedule path.
package contextread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

// Source is the provenance marker stamped on every returned context block.
const Source = "navi.kernel.contextread"

// Purpose is the declared reason a caller needs context. It is required and must
// be one of the enumerated values; unknown purposes are rejected, never widened.
type Purpose string

const (
	// PurposeEvalScoring is used by the advisory eval scorer to read just enough
	// about a run to score it.
	PurposeEvalScoring Purpose = "eval_scoring"
)

// Scope bounds what a read may return. It is required and enumerated.
type Scope string

const (
	// ScopeCurrentRunSummary returns only the summary of a single run, never the
	// whole world model.
	ScopeCurrentRunSummary Scope = "current_run_summary"
)

// validPurposes / validScopes are the closed enumerations. A value absent here is
// rejected.
var (
	validPurposes = map[Purpose]bool{
		PurposeEvalScoring: true,
	}
	validScopes = map[Scope]bool{
		ScopeCurrentRunSummary: true,
	}
	// allowedCombos is the policy matrix: only these (purpose, scope) pairs are
	// permitted. This is the least-context allow-list — a valid purpose plus a
	// valid scope is not sufficient unless the pairing is explicitly allowed.
	allowedCombos = map[Purpose]map[Scope]bool{
		PurposeEvalScoring: {ScopeCurrentRunSummary: true},
	}
)

// Rejection errors are 400-class: the request was malformed or asked for
// something outside the governed envelope. Callers map these to a client error.
var (
	ErrMissingPurpose     = &RejectionError{reason: "purpose is required"}
	ErrMissingScope       = &RejectionError{reason: "scope is required"}
	ErrUnknownPurpose     = &RejectionError{reason: "unknown purpose"}
	ErrUnknownScope       = &RejectionError{reason: "unknown scope"}
	ErrPurposeScopeDenied = &RejectionError{reason: "purpose is not permitted to use this scope"}
	ErrMissingRunID       = &RejectionError{reason: "run_id is required for this scope"}
)

// ErrRunNotFound is returned when the requested run does not exist. It maps to a
// 404, distinct from a malformed request.
var ErrRunNotFound = errors.New("contextread: run not found")

// RejectionError marks a request that was refused by policy (bad/unknown
// purpose or scope, or a disallowed pairing). It is distinguished from internal
// errors so the transport can return a 400-class status.
type RejectionError struct {
	reason string
}

func (e *RejectionError) Error() string { return "contextread: " + e.reason }

// IsRejection reports whether err is a policy rejection (400-class).
func IsRejection(err error) bool {
	var re *RejectionError
	return errors.As(err, &re)
}

// Request is a governed read request. RunID is required only for run-scoped reads.
type Request struct {
	RunID   string
	Purpose Purpose
	Scope   Scope
	// Caller is the attributable identity (owner/API key id) recorded in the audit
	// entry. The mediation layer does not authenticate; the transport supplies this.
	Caller string
}

// Provenance is the marker attached to every returned context block so consumers
// (prompts, logs, eval artifacts) can trace where the context came from and what
// was stripped.
type Provenance struct {
	Source         string    `json:"source"`
	KernelMediated bool      `json:"kernel_mediated"`
	RunID          string    `json:"run_id,omitempty"`
	Purpose        Purpose   `json:"purpose"`
	Scope          Scope     `json:"scope"`
	Redactions     []string  `json:"redactions"`
	GeneratedAt    time.Time `json:"generated_at"`
}

// Response is the redacted, scope-bounded, least-context result of a governed read.
type Response struct {
	RunID       string         `json:"run_id,omitempty"`
	Purpose     Purpose        `json:"purpose"`
	Scope       Scope          `json:"scope"`
	Context     map[string]any `json:"context"`
	Provenance  Provenance     `json:"provenance"`
	GeneratedAt time.Time      `json:"generated_at"`
}

// reader is the read-only data dependency. It exposes only the specific scoped
// reads the enumerated scopes need — never a write or a generic query.
// *worldmodel.WorldModel satisfies it.
type reader interface {
	RunSummary(ctx context.Context, runID string) (*schema.ExecutionOutcome, bool, error)
}

// AuditFunc appends an attributable audit record. It is append-only (an event-log
// write), not a world-model or execution-history mutation.
type AuditFunc func(ctx context.Context, ev schema.Event) error

// Mediator wraps the kernel read paths with the §4 governance. It is the only
// thing the gateway hands a query_context request to.
type Mediator struct {
	reader reader
	audit  AuditFunc
}

// NewMediator builds a Mediator from a read-only data source and an append-only
// audit sink. Both are required.
func NewMediator(r reader, audit AuditFunc) *Mediator {
	return &Mediator{reader: r, audit: audit}
}

// Query executes a governed read: validate → scope-bounded fetch → redact →
// provenance-tag → audit. It never writes the world model, mutates execution
// history, schedules, or calls a connector.
func (m *Mediator) Query(ctx context.Context, req Request) (*Response, error) {
	if err := validate(req); err != nil {
		return nil, err
	}

	var (
		contextBlock map[string]any
		redactions   []string
		found        bool
		err          error
	)
	switch req.Scope {
	case ScopeCurrentRunSummary:
		contextBlock, redactions, found, err = m.currentRunSummary(ctx, req)
	default:
		// Unreachable: validate() rejects unknown scopes. Guard against future drift.
		return nil, ErrUnknownScope
	}
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	prov := Provenance{
		Source:         Source,
		KernelMediated: true,
		RunID:          req.RunID,
		Purpose:        req.Purpose,
		Scope:          req.Scope,
		Redactions:     redactions,
		GeneratedAt:    now,
	}
	resp := &Response{
		RunID:       req.RunID,
		Purpose:     req.Purpose,
		Scope:       req.Scope,
		Context:     contextBlock,
		Provenance:  prov,
		GeneratedAt: now,
	}

	// Audit attribution is mandatory and fail-closed: a read that cannot be
	// recorded is not returned.
	if err := m.recordAudit(ctx, req, redactions, found); err != nil {
		return nil, fmt.Errorf("contextread: audit: %w", err)
	}
	return resp, nil
}

func validate(req Request) error {
	if req.Purpose == "" {
		return ErrMissingPurpose
	}
	if req.Scope == "" {
		return ErrMissingScope
	}
	if !validPurposes[req.Purpose] {
		return ErrUnknownPurpose
	}
	if !validScopes[req.Scope] {
		return ErrUnknownScope
	}
	if !allowedCombos[req.Purpose][req.Scope] {
		return ErrPurposeScopeDenied
	}
	if req.Scope == ScopeCurrentRunSummary && req.RunID == "" {
		return ErrMissingRunID
	}
	return nil
}

// currentRunSummary returns the least-context, redacted summary of a single run.
// It reuses the existing execution-ledger read path via the world-model façade;
// it does not expose the raw record. Sensitive free-text and entity references
// are stripped and reported in the redactions list.
func (m *Mediator) currentRunSummary(ctx context.Context, req Request) (map[string]any, []string, bool, error) {
	eo, found, err := m.reader.RunSummary(ctx, req.RunID)
	if err != nil {
		return nil, nil, false, err
	}
	if !found {
		return nil, nil, false, ErrRunNotFound
	}

	redactions := []string{}
	block := map[string]any{
		"run_id":            firstNonEmpty(eo.RunID, eo.AttemptID),
		"command_type":      string(eo.CommandType),
		"outcome":           string(eo.Outcome),
		"failure_class":     string(eo.FailureClass),
		"retryable":         eo.Retryable,
		"started_at":        eo.StartTime.UTC().Format(time.RFC3339),
		"skill_ids":         eo.SkillIDs,
		"connector_ids":     eo.ConnectorIDs,
		"llm_provider":      eo.LLMProvider,
		"llm_model":         eo.LLMModel,
		"llm_task_class":    eo.LLMTaskClass,
		"llm_complexity":    eo.LLMComplexity,
		"workspace_id":      eo.WorkspaceID,
		"boundary_crossing": eo.BoundaryCrossing,
		"approval_required": eo.ApprovalRequired,
		"approval_outcome":  string(eo.ApprovalOutcome),
	}
	if eo.EndTime != nil {
		block["ended_at"] = eo.EndTime.UTC().Format(time.RFC3339)
		block["duration_ms"] = eo.EndTime.Sub(eo.StartTime).Milliseconds()
	}

	// Redaction: failure_reason is operator-facing free text that can leak prompt
	// or user content. The scorer learns that a run failed and its class, not the
	// raw message.
	if eo.FailureReason != "" {
		block["has_failure_reason"] = true
		redactions = append(redactions, "failure_reason")
	}
	// Redaction: affected_entities can carry world-model entity references / PII.
	// Expose only the count so the scorer knows the blast radius, not the targets.
	if count := affectedEntityCount(eo.AffectedEntities); count > 0 {
		block["affected_entity_count"] = count
		redactions = append(redactions, "affected_entities")
	}
	// Least-context: internal correlation plumbing (correlation_id, proposal_id,
	// parent_run_id, runtime_session_id, command_id) is not exposed — the purpose
	// does not need it.

	return block, redactions, true, nil
}

func (m *Mediator) recordAudit(ctx context.Context, req Request, redactions []string, found bool) error {
	correlationID := req.RunID
	if correlationID == "" {
		correlationID = "context_read"
	}
	payload := schema.ContextReadAuditPayload{
		Caller:     req.Caller,
		Purpose:    string(req.Purpose),
		Scope:      string(req.Scope),
		RunID:      req.RunID,
		Redactions: redactions,
		Found:      found,
	}
	ev := schema.NewEvent(schema.FactContextRead, schema.EventKindFact, correlationID, schema.AgentNavi, payload)
	if req.RunID != "" {
		ev.RunID = req.RunID
	}
	return m.audit(ctx, ev)
}

func affectedEntityCount(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	var list []any
	if err := json.Unmarshal([]byte(raw), &list); err == nil {
		return len(list)
	}
	// Not a JSON array — treat any non-empty value as a single reference.
	return 1
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
