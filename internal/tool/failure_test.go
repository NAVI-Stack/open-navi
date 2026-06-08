package tool

import (
	"context"
	"errors"
	"testing"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/schema"
)

func TestClassifyExecutionFailureRecoverableTimeout(t *testing.T) {
	failure := ClassifyExecutionFailure(context.DeadlineExceeded)
	if failure.Code != ExecutionFailureCodeTransientTimeout {
		t.Fatalf("expected transient_timeout, got %+v", failure)
	}
	if !failure.Recoverable || !failure.FallbackAllowed || !failure.Retryable {
		t.Fatalf("expected timeout to be recoverable, fallback-eligible, and retryable, got %+v", failure)
	}
	if failure.FailureClass != schema.FailureClassTimeout {
		t.Fatalf("expected timeout failure class, got %+v", failure)
	}
}

func TestClassifyExecutionFailureNonRecoverableAuth(t *testing.T) {
	err := &llm.ProviderError{
		Reason: llm.ReasonAuth,
		Status: 401,
		Err:    errors.New("unauthorized"),
	}
	failure := ClassifyExecutionFailure(err)
	if failure.Code != ExecutionFailureCodeAuthFailure {
		t.Fatalf("expected auth_failure, got %+v", failure)
	}
	if failure.Recoverable || failure.FallbackAllowed || failure.Retryable {
		t.Fatalf("expected auth failure to be non-recoverable and non-fallback, got %+v", failure)
	}
	if failure.FailureClass != schema.FailureClassPermissionDenial {
		t.Fatalf("expected permission denial class, got %+v", failure)
	}
}

func TestClassifyExecutionFailureGovernanceBlockIsNonFallback(t *testing.T) {
	failure := ClassifyExecutionFailure(errors.New("ICS control boundary blocked this tool call"))
	if failure.Code != ExecutionFailureCodeGovernanceBlocked {
		t.Fatalf("expected governance_blocked, got %+v", failure)
	}
	if failure.FallbackAllowed || failure.Recoverable {
		t.Fatalf("expected governance block to be non-fallback and non-recoverable, got %+v", failure)
	}
	if failure.Outcome != schema.ExecutionOutcomeRejectedPreExecution {
		t.Fatalf("expected rejected_pre_execution outcome, got %+v", failure)
	}
}

func TestClassifyExecutionFailurePolicyRejectedIsNonFallback(t *testing.T) {
	failure := ClassifyExecutionFailure(errors.New("workspace action explicitly denied by policy"))
	if failure.Code != ExecutionFailureCodePolicyRejected {
		t.Fatalf("expected policy_rejected, got %+v", failure)
	}
	if failure.FallbackAllowed || failure.Recoverable {
		t.Fatalf("expected policy rejection to be non-fallback and non-recoverable, got %+v", failure)
	}
	if failure.FailureClass != schema.FailureClassPolicyBlocked {
		t.Fatalf("expected policy-blocked failure class, got %+v", failure)
	}
}

func TestClassifyExecutionFailureSelectedToolUnavailableEnablesFallback(t *testing.T) {
	failure := ClassifyExecutionFailure(errors.New(`tool registry: tool "navi.calendar.lookup" not found`))
	if failure.Code != ExecutionFailureCodeSelectedToolUnavailable {
		t.Fatalf("expected selected_tool_unavailable, got %+v", failure)
	}
	if !failure.Recoverable || !failure.FallbackAllowed || failure.Retryable {
		t.Fatalf("expected unavailable tool to be recoverable via fallback, got %+v", failure)
	}
}

func TestClassifyExecutionFailureSelectedActionMissingIsRecoverableDegradation(t *testing.T) {
	failure := ClassifyExecutionFailure(errors.New(`plugin tool "navi.files.update_content" has no executor`))
	if failure.Code != ExecutionFailureCodeSelectedActionMissing {
		t.Fatalf("expected selected_action_missing, got %+v", failure)
	}
	if !failure.Recoverable || !failure.FallbackAllowed || failure.Retryable {
		t.Fatalf("expected selected action missing to be recoverable via fallback, got %+v", failure)
	}
	if failure.FailureClass != schema.FailureClassExecutionFailure {
		t.Fatalf("expected execution failure class, got %+v", failure)
	}
}

func TestClassifyExecutionFailureAPIContractDriftIsRecoverableDegradation(t *testing.T) {
	failure := ClassifyExecutionFailure(errors.New("provider returned an unexpected field in the response shape"))
	if failure.Code != ExecutionFailureCodeAPIContractDrift {
		t.Fatalf("expected api_contract_drift, got %+v", failure)
	}
	if !failure.Recoverable || !failure.FallbackAllowed || failure.Retryable {
		t.Fatalf("expected api contract drift to be recoverable via fallback, got %+v", failure)
	}
	if failure.FailureClass != schema.FailureClassExecutionFailure {
		t.Fatalf("expected execution failure class, got %+v", failure)
	}
}

func TestRegistryExecuteWrapsLookupFailuresWithTaxonomy(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Execute(context.Background(), "missing.tool", nil)
	if err == nil {
		t.Fatal("expected execute error for missing tool")
	}
	failure := ClassifyExecutionFailure(err)
	if failure.Code != ExecutionFailureCodeSelectedToolUnavailable {
		t.Fatalf("expected selected_tool_unavailable, got %+v", failure)
	}
}
