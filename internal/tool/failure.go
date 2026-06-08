package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/schema"
)

// ExecutionFailureCode is the ATS execution-failure taxonomy used by tool
// selection, degradation handling, and future fallback routing.
type ExecutionFailureCode string

const (
	ExecutionFailureCodeSelectedToolUnavailable ExecutionFailureCode = "selected_tool_unavailable"
	ExecutionFailureCodeSelectedActionMissing   ExecutionFailureCode = "selected_action_missing"
	ExecutionFailureCodeConnectorUnavailable    ExecutionFailureCode = "connector_unavailable"
	ExecutionFailureCodeAuthFailure             ExecutionFailureCode = "auth_failure"
	ExecutionFailureCodeSchemaMismatch          ExecutionFailureCode = "schema_mismatch"
	ExecutionFailureCodeAPIContractDrift        ExecutionFailureCode = "api_contract_drift"
	ExecutionFailureCodeTransientTimeout        ExecutionFailureCode = "transient_timeout"
	ExecutionFailureCodeExecutionFailed         ExecutionFailureCode = "execution_failed"
	ExecutionFailureCodePartialFailure          ExecutionFailureCode = "partial_failure"
	ExecutionFailureCodeGovernanceBlocked       ExecutionFailureCode = "governance_blocked"
	ExecutionFailureCodePolicyRejected          ExecutionFailureCode = "policy_rejected"
)

// ExecutionFailure captures the normalized ATS failure taxonomy plus the
// broader execution-model class still consumed by existing runtime recovery.
type ExecutionFailure struct {
	Code            ExecutionFailureCode           `json:"code,omitempty"`
	Outcome         schema.ExecutionOutcomeOutcome `json:"outcome,omitempty"`
	FailureClass    schema.FailureClass            `json:"failure_class,omitempty"`
	Recoverable     bool                           `json:"recoverable,omitempty"`
	FallbackAllowed bool                           `json:"fallback_allowed,omitempty"`
	Retryable       bool                           `json:"retryable,omitempty"`
	Summary         string                         `json:"summary,omitempty"`
}

// IsZero reports whether the classification is empty.
func (f ExecutionFailure) IsZero() bool {
	return f.Code == ""
}

// ExecutionError wraps an underlying error with ATS execution-failure metadata.
type ExecutionError struct {
	Failure ExecutionFailure
	Err     error
}

func (e *ExecutionError) Error() string {
	summary := strings.TrimSpace(e.Failure.Summary)
	switch {
	case e == nil:
		return ""
	case summary != "":
		return summary
	case e.Err != nil:
		return e.Err.Error()
	default:
		return ""
	}
}

func (e *ExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// FailureClass exposes the schema-level class for generic command execution.
func (e *ExecutionError) FailureClass() schema.FailureClass {
	if e == nil {
		return ""
	}
	return e.Failure.FailureClass
}

// FailureCode exposes the ATS taxonomy code for runtime/tool-specific callers.
func (e *ExecutionError) FailureCode() string {
	if e == nil {
		return ""
	}
	return string(e.Failure.Code)
}

// WrapExecutionFailure applies a normalized ATS classification to an error.
func WrapExecutionFailure(err error, failure ExecutionFailure) error {
	failure = normalizeExecutionFailure(failure, err)
	return &ExecutionError{
		Failure: failure,
		Err:     err,
	}
}

// ErrorForFailureCode constructs a typed execution error from a taxonomy code.
func ErrorForFailureCode(code ExecutionFailureCode, summary string) error {
	failure := FailureForCode(code, summary)
	return &ExecutionError{
		Failure: failure,
		Err:     errors.New(failure.Summary),
	}
}

// FailureForCode returns the default classification envelope for an ATS code.
func FailureForCode(code ExecutionFailureCode, summary string) ExecutionFailure {
	failure := ExecutionFailure{
		Code:    code,
		Summary: strings.TrimSpace(summary),
	}
	switch code {
	case ExecutionFailureCodeSelectedToolUnavailable:
		failure.Outcome = schema.ExecutionOutcomeFailed
		failure.FailureClass = schema.FailureClassExecutionFailure
		failure.Recoverable = true
		failure.FallbackAllowed = true
	case ExecutionFailureCodeSelectedActionMissing:
		failure.Outcome = schema.ExecutionOutcomeFailed
		failure.FailureClass = schema.FailureClassExecutionFailure
		failure.Recoverable = true
		failure.FallbackAllowed = true
	case ExecutionFailureCodeConnectorUnavailable:
		failure.Outcome = schema.ExecutionOutcomeFailed
		failure.FailureClass = schema.FailureClassConnectorUnavailable
		failure.Recoverable = true
		failure.FallbackAllowed = true
		failure.Retryable = true
	case ExecutionFailureCodeAuthFailure:
		failure.Outcome = schema.ExecutionOutcomeFailed
		failure.FailureClass = schema.FailureClassPermissionDenial
	case ExecutionFailureCodeSchemaMismatch:
		failure.Outcome = schema.ExecutionOutcomeFailed
		failure.FailureClass = schema.FailureClassSchemaInvalid
	case ExecutionFailureCodeAPIContractDrift:
		failure.Outcome = schema.ExecutionOutcomeFailed
		failure.FailureClass = schema.FailureClassExecutionFailure
		failure.Recoverable = true
		failure.FallbackAllowed = true
	case ExecutionFailureCodeTransientTimeout:
		failure.Outcome = schema.ExecutionOutcomeTimedOut
		failure.FailureClass = schema.FailureClassTimeout
		failure.Recoverable = true
		failure.FallbackAllowed = true
		failure.Retryable = true
	case ExecutionFailureCodePartialFailure:
		failure.Outcome = schema.ExecutionOutcomePartiallySucceeded
		failure.FailureClass = schema.FailureClassPartialExecution
		failure.Recoverable = true
	case ExecutionFailureCodeGovernanceBlocked:
		failure.Outcome = schema.ExecutionOutcomeRejectedPreExecution
		failure.FailureClass = schema.FailureClassPolicyBlocked
	case ExecutionFailureCodePolicyRejected:
		failure.Outcome = schema.ExecutionOutcomeRejectedPreExecution
		failure.FailureClass = schema.FailureClassPolicyBlocked
	default:
		failure.Code = ExecutionFailureCodeExecutionFailed
		failure.Outcome = schema.ExecutionOutcomeFailed
		failure.FailureClass = schema.FailureClassExecutionFailure
	}
	return failure
}

// ClassifyExecutionFailure maps a runtime/tool error into the ATS taxonomy.
func ClassifyExecutionFailure(err error) ExecutionFailure {
	if err == nil {
		return ExecutionFailure{}
	}

	var classified *ExecutionError
	if errors.As(err, &classified) && classified != nil {
		return normalizeExecutionFailure(classified.Failure, err)
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return FailureForCode(ExecutionFailureCodeTransientTimeout, err.Error())
	case errors.Is(err, context.Canceled):
		failure := FailureForCode(ExecutionFailureCodeExecutionFailed, err.Error())
		failure.FailureClass = schema.FailureClassPermissionDenial
		return failure
	}

	if providerErr := llm.ClassifyError(err, "", "", 0); providerErr != nil {
		switch providerErr.Reason {
		case llm.ReasonAuth, llm.ReasonBilling:
			return FailureForCode(ExecutionFailureCodeAuthFailure, err.Error())
		case llm.ReasonTimeout, llm.ReasonRateLimit, llm.ReasonOverloaded:
			return FailureForCode(ExecutionFailureCodeTransientTimeout, err.Error())
		case llm.ReasonFormat:
			return FailureForCode(ExecutionFailureCodeSchemaMismatch, err.Error())
		}
	}

	lowered := strings.ToLower(err.Error())
	switch {
	case containsAny(lowered,
		"tool registry: tool",
		"tool not found",
		"selected tool unavailable",
		"unknown tool:",
		"unknown tool ",
	):
		return FailureForCode(ExecutionFailureCodeSelectedToolUnavailable, err.Error())
	case containsAny(lowered,
		"has no executor",
		"selected action missing",
		"not executable",
		"unsupported transport type",
		"unknown router tool",
		"unknown tool kind",
		"is not configured",
		"missing server or tool",
		"python runtime wrapper not found",
		"python entrypoint not found",
	):
		return FailureForCode(ExecutionFailureCodeSelectedActionMissing, err.Error())
	case containsAny(lowered,
		"connector unavailable",
		"connector not available",
		"service unavailable",
		"temporarily unavailable",
		"connection refused",
		"connection reset",
		"dial tcp",
		"no such host",
		"econnrefused",
	):
		return FailureForCode(ExecutionFailureCodeConnectorUnavailable, err.Error())
	case containsAny(lowered,
		"governance blocked",
		"blocked by governance",
		"control boundary blocked",
		"approval required",
		"approval boundary",
	):
		return FailureForCode(ExecutionFailureCodeGovernanceBlocked, err.Error())
	case containsAny(lowered,
		"policy rejected",
		"validation rejected",
		"explicitly denied",
		"outside workspace scope",
		"protected path",
		"workspace scope",
	):
		return FailureForCode(ExecutionFailureCodePolicyRejected, err.Error())
	case containsAny(lowered,
		"schema mismatch",
		"invalid schema",
		"schema invalid",
		"invalid request format",
		"missing required",
		"additionalproperties",
		"cannot unmarshal",
		"cannot decode",
		"validation failed",
	):
		return FailureForCode(ExecutionFailureCodeSchemaMismatch, err.Error())
	case containsAny(lowered,
		"api contract drift",
		"unknown field",
		"unexpected field",
		"unsupported response format",
		"response shape",
		"version mismatch",
	):
		return FailureForCode(ExecutionFailureCodeAPIContractDrift, err.Error())
	case containsAny(lowered,
		"unauthorized",
		"forbidden",
		"access denied",
		"authentication",
		"invalid api key",
		"incorrect api key",
		" 401",
		" 403",
	):
		return FailureForCode(ExecutionFailureCodeAuthFailure, err.Error())
	case containsAny(lowered,
		"timeout",
		"timed out",
		"deadline exceeded",
	):
		return FailureForCode(ExecutionFailureCodeTransientTimeout, err.Error())
	default:
		return FailureForCode(ExecutionFailureCodeExecutionFailed, err.Error())
	}
}

func normalizeExecutionFailure(failure ExecutionFailure, err error) ExecutionFailure {
	if failure.Code == "" {
		failure = FailureForCode(ExecutionFailureCodeExecutionFailed, "")
	}
	defaults := FailureForCode(failure.Code, failure.Summary)
	if failure.Outcome == "" {
		failure.Outcome = defaults.Outcome
	}
	if failure.FailureClass == "" {
		failure.FailureClass = defaults.FailureClass
	}
	if !failure.Recoverable {
		failure.Recoverable = defaults.Recoverable
	}
	if !failure.FallbackAllowed {
		failure.FallbackAllowed = defaults.FallbackAllowed
	}
	if !failure.Retryable {
		failure.Retryable = defaults.Retryable
	}
	if strings.TrimSpace(failure.Summary) == "" {
		switch {
		case err != nil:
			failure.Summary = strings.TrimSpace(err.Error())
		default:
			failure.Summary = defaults.Summary
		}
	}
	return failure
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		needle = strings.TrimSpace(strings.ToLower(needle))
		if needle == "" {
			continue
		}
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func (f ExecutionFailure) String() string {
	if f.Code == "" {
		return ""
	}
	if strings.TrimSpace(f.Summary) == "" {
		return string(f.Code)
	}
	return fmt.Sprintf("%s: %s", f.Code, strings.TrimSpace(f.Summary))
}
