package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Candidate is a provider/model pair in the fallback chain.
type Candidate struct {
	Provider Provider
	Model    string
}

// FallbackChain tries candidates in order, respecting cooldowns and error classification.
// It satisfies the Provider interface.
type FallbackChain struct {
	candidates []Candidate
	cooldown   *CooldownTracker
}

// NewFallbackChain creates a fallback chain with the given candidates.
func NewFallbackChain(candidates []Candidate) *FallbackChain {
	return &FallbackChain{
		candidates: candidates,
		cooldown:   NewCooldownTracker(),
	}
}

// Name returns a slash-separated list of candidate provider names.
func (fc *FallbackChain) Name() string {
	names := make([]string, len(fc.candidates))
	for i, c := range fc.candidates {
		names[i] = c.Provider.Name()
	}
	return strings.Join(names, "/")
}

// Chat tries each candidate in order. Returns the first successful response.
func (fc *FallbackChain) Chat(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options) (*Response, error) {
	var lastErr error

	for _, c := range fc.candidates {
		provName := c.Provider.Name()

		// Check cooldown
		if fc.cooldown.IsInCooldown(provName) {
			continue
		}

		// Use candidate's model if caller passed empty
		m := model
		if m == "" {
			m = c.Model
		}

		resp, err := c.Provider.Chat(ctx, m, messages, tools, opts)
		if err == nil {
			// A response that omits a "required" tool call is a content-level
			// outcome, not a provider failure: the runtime executor handles it
			// via its own recovery + fallback reply. We must NOT cool down the
			// provider for it — the FallbackChain is shared and long-lived, so
			// cooling the (often only) provider here starves every other chat.
			fc.cooldown.Clear(provName)
			return resp, nil
		}

		lastErr = err

		if isToolCallingFailure(err) {
			continue
		}

		// Classify the error
		pe, ok := err.(*ProviderError)
		if !ok {
			pe = ClassifyError(err, provName, m, 0)
		}

		if pe != nil && !pe.IsRetriable() {
			// Non-retriable — set long cooldown and stop trying
			fc.cooldown.Set(provName, 300*time.Second)
			return nil, pe
		}

		// Retriable — set cooldown based on reason
		if pe != nil {
			switch pe.Reason {
			case ReasonRateLimit:
				fc.cooldown.Set(provName, 60*time.Second)
			case ReasonOverloaded:
				fc.cooldown.Set(provName, 10*time.Second)
			default:
				fc.cooldown.Set(provName, 30*time.Second)
			}
		}
	}

	if lastErr != nil {
		return nil, &FallbackExhaustedError{Err: lastErr}
	}
	return nil, &FallbackExhaustedError{Err: fmt.Errorf("all providers in cooldown")}
}

// ChatStream tries each candidate that implements StreamingProvider; otherwise falls back to Chat (no streaming).
func (fc *FallbackChain) ChatStream(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options, onChunk func(delta string)) (*Response, error) {
	var lastErr error

	for _, c := range fc.candidates {
		provName := c.Provider.Name()
		if fc.cooldown.IsInCooldown(provName) {
			continue
		}
		m := model
		if m == "" {
			m = c.Model
		}
		if sp, ok := c.Provider.(StreamingProvider); ok {
			resp, err := sp.ChatStream(ctx, m, messages, tools, opts, onChunk)
			if err == nil {
				// Missing "required" tool calls are handled by the runtime
				// executor's recovery path, never by cooling the provider.
				fc.cooldown.Clear(provName)
				return resp, nil
			}
			lastErr = err
			if isToolCallingFailure(err) {
				continue
			}
			if pe, ok := err.(*ProviderError); ok && !pe.IsRetriable() {
				fc.cooldown.Set(provName, 300*time.Second)
				return nil, pe
			}
			continue
		}
		resp, err := c.Provider.Chat(ctx, m, messages, tools, opts)
		if err == nil {
			fc.cooldown.Clear(provName)
			return resp, nil
		}
		lastErr = err
		if isToolCallingFailure(err) {
			continue
		}
		if pe, ok := err.(*ProviderError); ok && !pe.IsRetriable() {
			fc.cooldown.Set(provName, 300*time.Second)
			return nil, pe
		}
	}

	if lastErr != nil {
		return nil, &FallbackExhaustedError{Err: lastErr}
	}
	return nil, &FallbackExhaustedError{Err: fmt.Errorf("stream: no streaming provider available")}
}

// FallbackExhaustedError indicates all fallback candidates failed.
type FallbackExhaustedError struct {
	Err error
}

func (e *FallbackExhaustedError) Error() string {
	return fmt.Sprintf("llm: all fallback candidates exhausted: %v", e.Err)
}

func (e *FallbackExhaustedError) Unwrap() error {
	return e.Err
}

// CooldownTracker tracks provider cooldown periods.
type CooldownTracker struct {
	entries sync.Map // provider name → time.Time (available after)
}

// NewCooldownTracker creates a new cooldown tracker.
func NewCooldownTracker() *CooldownTracker {
	return &CooldownTracker{}
}

// IsInCooldown returns true if the provider is still in cooldown.
func (ct *CooldownTracker) IsInCooldown(provider string) bool {
	val, ok := ct.entries.Load(provider)
	if !ok {
		return false
	}
	return time.Now().Before(val.(time.Time))
}

// Set puts a provider into cooldown for the given duration.
func (ct *CooldownTracker) Set(provider string, duration time.Duration) {
	ct.entries.Store(provider, time.Now().Add(duration))
}

// Clear removes a provider from cooldown.
func (ct *CooldownTracker) Clear(provider string) {
	ct.entries.Delete(provider)
}

func isToolCallingFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "failed to call tools") ||
		(strings.Contains(msg, "tool") && (strings.Contains(msg, "not support") || strings.Contains(msg, "invalid") || strings.Contains(msg, "unsupported") || strings.Contains(msg, "no tool")))
}
