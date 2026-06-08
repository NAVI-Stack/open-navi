package llm

import (
	"errors"
	"testing"
)

func TestClassifyError_429_RateLimit(t *testing.T) {
	pe := ClassifyError(errors.New("rate limit exceeded"), "openai", "gpt-4", 429)
	if pe.Reason != ReasonRateLimit {
		t.Errorf("expected rate_limit, got %s", pe.Reason)
	}
	if !pe.IsRetriable() {
		t.Error("rate limit should be retriable")
	}
}

func TestClassifyError_401_Auth(t *testing.T) {
	pe := ClassifyError(errors.New("unauthorized"), "openai", "gpt-4", 401)
	if pe.Reason != ReasonAuth {
		t.Errorf("expected auth, got %s", pe.Reason)
	}
	if pe.IsRetriable() {
		t.Error("auth should not be retriable")
	}
}

func TestClassifyError_500_Timeout(t *testing.T) {
	pe := ClassifyError(errors.New("internal server error"), "openai", "gpt-4", 500)
	if pe.Reason != ReasonTimeout {
		t.Errorf("expected timeout, got %s", pe.Reason)
	}
	if !pe.IsRetriable() {
		t.Error("timeout should be retriable")
	}
}

func TestClassifyError_402_Billing(t *testing.T) {
	pe := ClassifyError(errors.New("payment required"), "openai", "gpt-4", 402)
	if pe.Reason != ReasonBilling {
		t.Errorf("expected billing, got %s", pe.Reason)
	}
	if pe.IsRetriable() {
		t.Error("billing should not be retriable")
	}
}

func TestClassifyError_Format(t *testing.T) {
	pe := ClassifyError(errors.New("invalid request format"), "anthropic", "claude", 400)
	if pe.Reason != ReasonFormat {
		t.Errorf("expected format, got %s", pe.Reason)
	}
	if pe.IsRetriable() {
		t.Error("format should not be retriable")
	}
}

func TestClassifyError_MessagePattern(t *testing.T) {
	pe := ClassifyError(errors.New("anthropic-ratelimit header triggered"), "anthropic", "claude", 0)
	if pe.Reason != ReasonRateLimit {
		t.Errorf("expected rate_limit from message pattern, got %s", pe.Reason)
	}
}

func TestClassifyError_Nil(t *testing.T) {
	pe := ClassifyError(nil, "openai", "gpt-4", 0)
	if pe != nil {
		t.Error("expected nil for nil error")
	}
}

func TestProviderError_Unwrap(t *testing.T) {
	inner := errors.New("inner error")
	pe := &ProviderError{Reason: ReasonAuth, Err: inner}
	if !errors.Is(pe, inner) {
		t.Error("Unwrap should return inner error")
	}
}
