// Package anthropic provides a native Anthropic provider for NAVI's LLM layer.
//
// It implements Provider, StreamingProvider, and ManagedProvider using the
// Anthropic Messages API (/v1/messages) with support for:
//   - Extended thinking (thinking content blocks + budget_tokens)
//   - Vision: image content blocks with base64 source
//   - Tool use in Anthropic's native format
//   - SSE streaming with text, tool, and thinking delta handling
//   - Static model catalog for health and model listing
package anthropic

import (
	"net/http"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/llm"
)

// Compile-time assertions: Provider must satisfy these llm interfaces.
var (
	_ llm.Provider          = (*Provider)(nil)
	_ llm.StreamingProvider = (*Provider)(nil)
	_ llm.ManagedProvider   = (*Provider)(nil)
)

// Provider is the native Anthropic provider. It is safe for concurrent use.
type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// Config holds constructor options for New.
type Config struct {
	// APIKey is the Anthropic API key (required).
	APIKey string

	// BaseURL is the API root (e.g. "https://api.anthropic.com/v1").
	// Defaults to "https://api.anthropic.com/v1" when empty.
	BaseURL string

	// RequestTimeout controls per-request deadlines.
	// Defaults to 300s (thinking-capable models may need longer).
	RequestTimeout time.Duration
}

// New creates a native Anthropic provider from cfg.
func New(cfg Config) *Provider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	// Normalize: strip trailing slashes.
	baseURL = strings.TrimRight(baseURL, "/")

	timeout := cfg.RequestTimeout
	if timeout == 0 {
		timeout = 300 * time.Second
	}

	return &Provider{
		apiKey:  cfg.APIKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

// Name returns the provider identifier used as a registry key.
func (p *Provider) Name() string { return "anthropic" }
