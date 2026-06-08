// Package openai provides a native OpenAI provider for NAVI's LLM layer.
//
// It implements Provider, StreamingProvider, and ManagedProvider using the
// OpenAI Chat Completions API (/v1/chat/completions) with support for:
//   - Reasoning models (o1, o3, o4-mini): max_completion_tokens, developer role
//   - Vision: image_url content blocks
//   - Tool use in OpenAI's native format
//   - SSE streaming with incremental tool call assembly
//   - OpenRouter compatibility (same API, different base URL)
//   - Static model catalog for health and model listing
package openai

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

// Provider is the native OpenAI provider. It is safe for concurrent use.
// It can also serve as an OpenRouter provider with a different base URL.
type Provider struct {
	name    string // "openai" or "openrouter"
	apiKey  string
	baseURL string
	client  *http.Client
}

// Config holds constructor options for New.
type Config struct {
	// Name is the provider identifier: "openai" or "openrouter".
	// Defaults to "openai" when empty.
	Name string

	// APIKey is the API key (required).
	APIKey string

	// BaseURL is the API root. Defaults based on Name:
	//   "openai"     → "https://api.openai.com/v1"
	//   "openrouter" → "https://openrouter.ai/api/v1"
	BaseURL string

	// RequestTimeout controls per-request deadlines.
	// Defaults to 300s.
	RequestTimeout time.Duration
}

// New creates a native OpenAI provider from cfg.
func New(cfg Config) *Provider {
	name := cfg.Name
	if name == "" {
		name = "openai"
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		switch name {
		case "openrouter":
			baseURL = "https://openrouter.ai/api/v1"
		default:
			baseURL = "https://api.openai.com/v1"
		}
	}
	baseURL = strings.TrimRight(baseURL, "/")

	timeout := cfg.RequestTimeout
	if timeout == 0 {
		timeout = 300 * time.Second
	}

	return &Provider{
		name:    name,
		apiKey:  cfg.APIKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

// Name returns the provider identifier used as a registry key.
func (p *Provider) Name() string { return p.name }
