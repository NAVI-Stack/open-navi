package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ModelCapabilities holds discovered per-model capabilities from /api/show.
type ModelCapabilities struct {
	SupportsTools    bool
	SupportsVision   bool
	SupportsThinking bool
	ContextWindow    int // 0 means unknown; use Ollama default
}

// capabilityCache stores capabilities indexed by model name with a TTL.
type capabilityCache struct {
	mu      sync.RWMutex
	entries map[string]capEntry
	ttl     time.Duration
}

type capEntry struct {
	caps      ModelCapabilities
	expiresAt time.Time
}

func newCapabilityCache(ttl time.Duration) *capabilityCache {
	return &capabilityCache{
		entries: make(map[string]capEntry),
		ttl:     ttl,
	}
}

func (c *capabilityCache) get(model string) (ModelCapabilities, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[model]
	if !ok || time.Now().After(e.expiresAt) {
		return ModelCapabilities{}, false
	}
	return e.caps, true
}

func (c *capabilityCache) set(model string, caps ModelCapabilities) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[model] = capEntry{
		caps:      caps,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// invalidate removes a model entry so the next call re-fetches from /api/show.
// Should be called after a successful pull or delete.
func (c *capabilityCache) invalidate(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, model)
}

// getCapabilities returns the cached capabilities for model, fetching from
// /api/show on cache miss. On error it returns zero capabilities and the caller
// proceeds without capability gating.
func (p *Provider) getCapabilities(ctx context.Context, model string) (ModelCapabilities, error) {
	if caps, ok := p.capCache.get(model); ok {
		return caps, nil
	}

	body, err := json.Marshal(ollamaShowRequest{Name: model})
	if err != nil {
		return ModelCapabilities{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/show", bytes.NewReader(body))
	if err != nil {
		return ModelCapabilities{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		return ModelCapabilities{}, fmt.Errorf("ollama: show %q: %w", model, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// Model doesn't exist yet (will be caught by Chat's 404 fallback).
		return ModelCapabilities{}, nil
	}

	var show ollamaShowResponse
	if err := json.NewDecoder(res.Body).Decode(&show); err != nil {
		return ModelCapabilities{}, fmt.Errorf("ollama: parse show %q: %w", model, err)
	}

	caps := parseCapabilities(show)
	p.capCache.set(model, caps)
	return caps, nil
}

// parseCapabilities converts a raw /api/show response to ModelCapabilities.
func parseCapabilities(show ollamaShowResponse) ModelCapabilities {
	var caps ModelCapabilities

	// --- Ollama 0.5+ explicit capabilities array (authoritative) ---
	for _, c := range show.Capabilities {
		switch c {
		case "vision":
			caps.SupportsVision = true
		case "tools":
			caps.SupportsTools = true
		case "thinking":
			caps.SupportsThinking = true
		}
	}

	// --- Fallback: infer from model_info keys ---
	if !caps.SupportsVision {
		if _, hasClip := show.ModelInfo["clip.vision.image_size"]; hasClip {
			caps.SupportsVision = true
		}
	}

	if !caps.SupportsThinking {
		fam := strings.ToLower(show.Details.Family)
		if (strings.Contains(fam, "qwen") || strings.Contains(fam, "deepseek")) &&
			strings.Contains(show.Template, "<think>") {
			caps.SupportsThinking = true
		}
	}

	// --- Context window ---
	if v, ok := show.ModelInfo["llama.context_length"]; ok {
		switch n := v.(type) {
		case float64:
			caps.ContextWindow = int(n)
		case int:
			caps.ContextWindow = n
		}
	}
	// Fallback: scan the parameters field for "num_ctx <value>"
	if caps.ContextWindow == 0 {
		caps.ContextWindow = parseNumCtxFromParameters(show.Parameters)
	}

	return caps
}

// parseNumCtxFromParameters scans Ollama's "parameters" string for a line like
// "num_ctx 4096" and returns the value, or 0 if not found.
func parseNumCtxFromParameters(params string) int {
	for _, line := range strings.Split(params, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "num_ctx") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		n, err := strconv.Atoi(parts[1])
		if err == nil {
			return n
		}
	}
	return 0
}
