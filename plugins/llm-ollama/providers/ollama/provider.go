// Package ollama provides a native Ollama provider for NAVI's LLM layer.
//
// It implements all four internal/llm provider interfaces using Ollama's native
// /api/chat endpoint instead of the OpenAI-compatible /v1/chat/completions path.
// Key capabilities over the OpenAI-compat approach:
//   - Per-model capability detection from /api/show (vision, tools, thinking)
//   - Native request-shape controls for local latency-sensitive chat
//   - Native tool calling format
//   - Thinking block accumulation from message.thinking field and <think> tags
//   - Image/vision support: base64 extraction from message content
//   - Full local lifecycle: pull, delete, warm, copy, create
package ollama

import (
	"net/http"
	"sync"
	"time"

	"github.com/ceoai/navi/internal/llm"
)

// Compile-time assertions: Provider must satisfy all four llm interfaces.
var (
	_ llm.Provider               = (*Provider)(nil)
	_ llm.StreamingProvider      = (*Provider)(nil)
	_ llm.ManagedProvider        = (*Provider)(nil)
	_ llm.LocalLifecycleProvider = (*Provider)(nil)
)

// Provider is the native Ollama provider. It is safe for concurrent use.
type Provider struct {
	baseURL string

	// client is used for all requests except model pulls, which may run for
	// many minutes and require no timeout.
	client *http.Client

	// pullClient has no timeout so long pulls can stream without interruption.
	pullClient *http.Client

	// capCache stores per-model capability data fetched from /api/show.
	capCache *capabilityCache

	// mu protects fallbackModel and pullProgressFn.
	mu             sync.Mutex
	fallbackModel  string           // cached after first successful /api/tags lookup
	pullProgressFn PullProgressFunc // wired by ControlPlane; may be nil
}

// Config holds constructor options for New.
type Config struct {
	// BaseURL is the root Ollama address (e.g. "http://localhost:11434").
	// Trailing slashes and a "/v1" suffix are stripped automatically.
	// Defaults to "http://localhost:11434" when empty.
	BaseURL string

	// RequestTimeout controls per-request deadlines for inference and
	// management calls. Defaults to 600s. Pull operations are exempt.
	RequestTimeout time.Duration

	// CapCacheTTL controls how long /api/show results are cached per model.
	// Defaults to 15 minutes. Invalidated automatically on pull/delete.
	CapCacheTTL time.Duration
}

// New creates a native Ollama provider from cfg.
func New(cfg Config) *Provider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	// Strip trailing slash and /v1 suffix so callers can pass either form.
	for len(baseURL) > 0 && baseURL[len(baseURL)-1] == '/' {
		baseURL = baseURL[:len(baseURL)-1]
	}
	if len(baseURL) >= 3 && baseURL[len(baseURL)-3:] == "/v1" {
		baseURL = baseURL[:len(baseURL)-3]
	}

	timeout := cfg.RequestTimeout
	if timeout == 0 {
		timeout = 600 * time.Second
	}

	capTTL := cfg.CapCacheTTL
	if capTTL == 0 {
		capTTL = 15 * time.Minute
	}

	return &Provider{
		baseURL:    baseURL,
		client:     &http.Client{Timeout: timeout},
		pullClient: &http.Client{}, // no timeout
		capCache:   newCapabilityCache(capTTL),
	}
}

// Name returns the provider identifier used as a registry key.
func (p *Provider) Name() string { return "ollama" }
