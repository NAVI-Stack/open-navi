package llm

import (
	"fmt"
	"regexp"
	"strings"
)

// FailureReason classifies why an LLM API call failed.
type FailureReason string

const (
	ReasonAuth       FailureReason = "auth"
	ReasonRateLimit  FailureReason = "rate_limit"
	ReasonBilling    FailureReason = "billing"
	ReasonTimeout    FailureReason = "timeout"
	ReasonOverloaded FailureReason = "overloaded"
	ReasonFormat     FailureReason = "format"
	ReasonUnknown    FailureReason = "unknown"
)

// ProviderError is a typed error from an LLM provider.
type ProviderError struct {
	Reason   FailureReason
	Provider string
	Model    string
	Status   int
	Err      error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("llm %s/%s: %s (status %d): %v", e.Provider, e.Model, e.Reason, e.Status, e.Err)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// IsRetriable returns false for auth, billing, and format errors.
func (e *ProviderError) IsRetriable() bool {
	switch e.Reason {
	case ReasonAuth, ReasonBilling, ReasonFormat:
		return false
	default:
		return true
	}
}

// --- Error pattern tables (adapted from picoclaw production patterns) ---

type errorPattern struct {
	substring string
	regex     *regexp.Regexp
}

func substr(s string) errorPattern { return errorPattern{substring: s} }
func rxp(r string) errorPattern    { return errorPattern{regex: regexp.MustCompile("(?i)" + r)} }

var (
	rateLimitPatterns = []errorPattern{
		rxp(`rate[_ ]limit`),
		substr("too many requests"),
		substr("429"),
		substr("exceeded your current quota"),
		rxp(`exceeded.*quota`),
		rxp(`resource has been exhausted`),
		substr("resource_exhausted"),
		substr("quota exceeded"),
		substr("usage limit"),
		substr("anthropic-ratelimit"),
	}

	overloadedPatterns = []errorPattern{
		rxp(`overloaded_error`),
		substr("overloaded"),
	}

	timeoutPatterns = []errorPattern{
		substr("timeout"),
		substr("timed out"),
		substr("deadline exceeded"),
	}

	billingPatterns = []errorPattern{
		rxp(`\b402\b`),
		substr("payment required"),
		substr("insufficient credits"),
		substr("credit balance"),
		substr("insufficient balance"),
	}

	authPatterns = []errorPattern{
		rxp(`invalid[_ ]?api[_ ]?key`),
		substr("incorrect api key"),
		substr("invalid token"),
		substr("authentication"),
		substr("unauthorized"),
		substr("forbidden"),
		substr("access denied"),
		substr("expired"),
		rxp(`\b401\b`),
		rxp(`\b403\b`),
		substr("no api key found"),
	}

	formatPatterns = []errorPattern{
		substr("invalid request format"),
		substr("string should match pattern"),
		substr("tool_use.id"),
	}

	transientStatusCodes = map[int]bool{
		500: true, 502: true, 503: true,
		521: true, 522: true, 523: true, 524: true,
		529: true,
	}
)

// ClassifyError classifies an HTTP error into a ProviderError.
func ClassifyError(err error, provider, model string, status int) *ProviderError {
	if err == nil {
		return nil
	}

	// Try status code first.
	if status > 0 {
		if reason := classifyByStatus(status); reason != "" {
			return &ProviderError{
				Reason:   reason,
				Provider: provider,
				Model:    model,
				Status:   status,
				Err:      err,
			}
		}
	}

	// Fall back to message pattern matching.
	msg := strings.ToLower(err.Error())
	if reason := classifyByMessage(msg); reason != "" {
		return &ProviderError{
			Reason:   reason,
			Provider: provider,
			Model:    model,
			Status:   status,
			Err:      err,
		}
	}

	return &ProviderError{
		Reason:   ReasonUnknown,
		Provider: provider,
		Model:    model,
		Status:   status,
		Err:      err,
	}
}

func classifyByStatus(status int) FailureReason {
	switch {
	case status == 401 || status == 403:
		return ReasonAuth
	case status == 402:
		return ReasonBilling
	case status == 408:
		return ReasonTimeout
	case status == 429:
		return ReasonRateLimit
	case status == 400:
		return ReasonFormat
	case transientStatusCodes[status]:
		return ReasonTimeout
	}
	return ""
}

func classifyByMessage(msg string) FailureReason {
	if matchesAny(msg, rateLimitPatterns) {
		return ReasonRateLimit
	}
	if matchesAny(msg, overloadedPatterns) {
		return ReasonOverloaded
	}
	if matchesAny(msg, billingPatterns) {
		return ReasonBilling
	}
	if matchesAny(msg, timeoutPatterns) {
		return ReasonTimeout
	}
	if matchesAny(msg, authPatterns) {
		return ReasonAuth
	}
	if matchesAny(msg, formatPatterns) {
		return ReasonFormat
	}
	return ""
}

func matchesAny(msg string, patterns []errorPattern) bool {
	for _, p := range patterns {
		if p.regex != nil {
			if p.regex.MatchString(msg) {
				return true
			}
		} else if p.substring != "" {
			if strings.Contains(msg, p.substring) {
				return true
			}
		}
	}
	return false
}
