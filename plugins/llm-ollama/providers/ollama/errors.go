package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// isModelNotFound returns true when the error indicates the requested model is
// not present on the local Ollama instance.
func isModelNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status 404") ||
		(strings.Contains(msg, "model") && strings.Contains(msg, "not found")) ||
		strings.Contains(msg, "pull model") ||
		strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "invalid_request_error")
}

// isToolSupportError returns true when Ollama reports that the model does not
// support tool / function calling.
func isToolSupportError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not support tools") ||
		(strings.Contains(msg, "tool_use") && strings.Contains(msg, "not supported"))
}

// firstAvailableModel queries /api/tags and returns the name of the first
// locally-pulled model. The result is cached for the lifetime of the provider.
func (p *Provider) firstAvailableModel(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.fallbackModel != "" {
		return p.fallbackModel, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return "", err
	}

	res, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: list models: %w", err)
	}
	defer res.Body.Close()

	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("ollama: parse tags: %w", err)
	}
	if len(payload.Models) == 0 {
		return "", fmt.Errorf("ollama: no models found locally")
	}

	p.fallbackModel = payload.Models[0].Name
	return p.fallbackModel, nil
}

// firstAvailableModelWithTools queries /api/tags, checks capabilities for each model,
// and returns the first model that supports tool calling, excluding excludeModel.
func (p *Provider) firstAvailableModelWithTools(ctx context.Context, excludeModel string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return "", err
	}

	res, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: list models: %w", err)
	}
	defer res.Body.Close()

	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("ollama: parse tags: %w", err)
	}

	for _, m := range payload.Models {
		if m.Name == excludeModel {
			continue
		}
		caps, err := p.getCapabilities(ctx, m.Name)
		if err == nil && caps.SupportsTools {
			return m.Name, nil
		}
	}

	return "", fmt.Errorf("ollama: no models found locally that support tool calling")
}
