package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ceoai/navi/internal/llm"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// PullProgressFunc
// ---------------------------------------------------------------------------

// PullProgressFunc is called with progress updates during a model pull.
// id is the operation ID returned from PullModel.
type PullProgressFunc func(
	id string,
	status llm.OperationStatus,
	progress float64,
	msg, errMsg string,
)

// SetPullProgressCallback registers the function that receives pull progress
// updates. Called by ControlPlane after building the provider registry.
func (p *Provider) SetPullProgressCallback(fn PullProgressFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pullProgressFn = fn
}

func (p *Provider) pullProgress(id string, status llm.OperationStatus, progress float64, msg, errMsg string) {
	p.mu.Lock()
	fn := p.pullProgressFn
	p.mu.Unlock()
	if fn != nil {
		fn(id, status, progress, msg, errMsg)
	}
}

// ---------------------------------------------------------------------------
// HealthCheck
// ---------------------------------------------------------------------------

// HealthCheck pings the Ollama root and reports availability plus model count.
func (p *Provider) HealthCheck(ctx context.Context) (llm.ProviderHealth, error) {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL, nil)
	if err != nil {
		return llm.ProviderHealth{Provider: "ollama", Healthy: false, CheckedAt: time.Now()}, err
	}

	res, err := p.client.Do(req)
	if err != nil {
		return llm.ProviderHealth{
			Provider:  "ollama",
			Healthy:   false,
			Latency:   int(time.Since(start).Milliseconds()),
			Message:   err.Error(),
			CheckedAt: time.Now(),
		}, nil
	}
	defer res.Body.Close()

	latency := int(time.Since(start).Milliseconds())

	modelCount := 0
	if models, listErr := p.ListModels(ctx); listErr == nil {
		modelCount = len(models)
	}

	return llm.ProviderHealth{
		Provider:   "ollama",
		Healthy:    res.StatusCode == http.StatusOK,
		Latency:    latency,
		Message:    fmt.Sprintf("HTTP %d", res.StatusCode),
		CheckedAt:  time.Now(),
		ModelCount: modelCount,
	}, nil
}

// ---------------------------------------------------------------------------
// ListModels
// ---------------------------------------------------------------------------

// ListModels queries /api/tags and returns all locally available models.
func (p *Provider) ListModels(ctx context.Context) ([]llm.ModelDescriptor, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	res, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: list models: %w", err)
	}
	defer res.Body.Close()

	var payload ollamaTagsResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("ollama: parse tags: %w", err)
	}

	descriptors := make([]llm.ModelDescriptor, 0, len(payload.Models))
	for _, m := range payload.Models {
		name := m.Name
		if name == "" {
			name = m.Model
		}
		descriptors = append(descriptors, llm.ModelDescriptor{
			Name:          name,
			Size:          m.Size,
			Family:        m.Details.Family,
			ParameterSize: m.Details.ParameterSize,
			QuantLevel:    m.Details.QuantizationLevel,
			ModifiedAt:    m.ModifiedAt,
			Digest:        m.Digest,
		})
	}
	return descriptors, nil
}

// ---------------------------------------------------------------------------
// GetModel
// ---------------------------------------------------------------------------

// GetModel queries /api/show for detailed model information and returns a
// ModelDescriptor enriched with capability data.
func (p *Provider) GetModel(ctx context.Context, model string) (llm.ModelDescriptor, error) {
	body, err := json.Marshal(ollamaShowRequest{Name: model})
	if err != nil {
		return llm.ModelDescriptor{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/show", bytes.NewReader(body))
	if err != nil {
		return llm.ModelDescriptor{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		return llm.ModelDescriptor{}, fmt.Errorf("ollama: show model: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return llm.ModelDescriptor{}, fmt.Errorf("ollama: show model %q: status %d", model, res.StatusCode)
	}

	var show ollamaShowResponse
	if err := json.NewDecoder(res.Body).Decode(&show); err != nil {
		return llm.ModelDescriptor{}, fmt.Errorf("ollama: parse show: %w", err)
	}

	// Update capability cache as a side effect.
	caps := parseCapabilities(show)
	p.capCache.set(model, caps)

	return llm.ModelDescriptor{
		Name:          model,
		Family:        show.Details.Family,
		ParameterSize: show.Details.ParameterSize,
		QuantLevel:    show.Details.QuantizationLevel,
		ModifiedAt:    show.ModifiedAt,
	}, nil
}

// ---------------------------------------------------------------------------
// ListRunningModels
// ---------------------------------------------------------------------------

// ListRunningModels queries /api/ps for currently loaded (in-memory) models.
func (p *Provider) ListRunningModels(ctx context.Context) ([]llm.RunningModel, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/ps", nil)
	if err != nil {
		return nil, err
	}

	res, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: list running models: %w", err)
	}
	defer res.Body.Close()

	var payload ollamaPsResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("ollama: parse ps: %w", err)
	}

	running := make([]llm.RunningModel, 0, len(payload.Models))
	for _, m := range payload.Models {
		running = append(running, llm.RunningModel{
			Name:      m.Name,
			Size:      m.Size,
			VRAM:      m.SizeVRAM,
			ExpiresAt: m.ExpiresAt,
		})
	}
	return running, nil
}

// ---------------------------------------------------------------------------
// PullModel
// ---------------------------------------------------------------------------

// PullModel initiates an async model pull. It returns immediately with a
// pending ProviderOperation; progress is streamed via the registered
// PullProgressFunc callback.
func (p *Provider) PullModel(ctx context.Context, model string) (llm.ProviderOperation, error) {
	opID := uuid.New().String()
	now := time.Now()
	op := llm.ProviderOperation{
		ID:        opID,
		Provider:  "ollama",
		Action:    "pull",
		Model:     model,
		Status:    llm.OperationStatusPending,
		StartedAt: now,
	}

	body, _ := json.Marshal(map[string]any{"name": model, "stream": true})
	// Use context.Background so the pull survives caller context cancellation.
	req, err := http.NewRequestWithContext(context.Background(), "POST", p.baseURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		op.Status = llm.OperationStatusFailed
		op.Error = err.Error()
		completed := time.Now()
		op.CompletedAt = &completed
		return op, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := p.pullClient.Do(req)
	if err != nil {
		op.Status = llm.OperationStatusFailed
		op.Error = err.Error()
		completed := time.Now()
		op.CompletedAt = &completed
		return op, err
	}

	op.Status = llm.OperationStatusRunning
	go p.consumePullStream(opID, model, res)

	return op, nil
}

// consumePullStream reads NDJSON progress from an Ollama pull response and
// forwards updates to the registered pull progress callback.
func (p *Provider) consumePullStream(opID, model string, res *http.Response) {
	defer res.Body.Close()

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	for scanner.Scan() {
		var progress ollamaPullProgress
		if err := json.Unmarshal(scanner.Bytes(), &progress); err != nil {
			continue
		}

		pct := 0.0
		if progress.Total > 0 {
			pct = float64(progress.Completed) / float64(progress.Total)
		}
		p.pullProgress(opID, llm.OperationStatusRunning, pct, progress.Status, "")
	}

	finalStatus := llm.OperationStatusCompleted
	errMsg := ""
	if err := scanner.Err(); err != nil {
		finalStatus = llm.OperationStatusFailed
		errMsg = err.Error()
	}

	// Invalidate capability cache and fallback model caches so the next Chat call re-fetches.
	p.capCache.invalidate(model)
	p.mu.Lock()
	p.fallbackModel = ""
	p.mu.Unlock()

	p.pullProgress(opID, finalStatus, 1.0, "complete", errMsg)
}

// ---------------------------------------------------------------------------
// DeleteModel
// ---------------------------------------------------------------------------

// DeleteModel removes a model from local Ollama storage.
func (p *Provider) DeleteModel(ctx context.Context, model string) (llm.ProviderOperation, error) {
	opID := uuid.New().String()
	now := time.Now()
	op := llm.ProviderOperation{
		ID:        opID,
		Provider:  "ollama",
		Action:    "delete",
		Model:     model,
		Status:    llm.OperationStatusRunning,
		StartedAt: now,
	}

	body, _ := json.Marshal(map[string]string{"name": model})
	req, err := http.NewRequestWithContext(ctx, "DELETE", p.baseURL+"/api/delete", bytes.NewReader(body))
	if err != nil {
		op.Status = llm.OperationStatusFailed
		op.Error = err.Error()
		completed := time.Now()
		op.CompletedAt = &completed
		return op, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		op.Status = llm.OperationStatusFailed
		op.Error = err.Error()
		completed := time.Now()
		op.CompletedAt = &completed
		return op, err
	}
	defer res.Body.Close()

	completed := time.Now()
	op.CompletedAt = &completed

	if res.StatusCode != http.StatusOK {
		op.Status = llm.OperationStatusFailed
		op.Error = fmt.Sprintf("HTTP %d", res.StatusCode)
		return op, fmt.Errorf("ollama: delete model %q: status %d", model, res.StatusCode)
	}

	// Invalidate capability cache and fallback model caches for the deleted model.
	p.capCache.invalidate(model)
	p.mu.Lock()
	p.fallbackModel = ""
	p.mu.Unlock()

	op.Status = llm.OperationStatusCompleted
	op.Progress = 1.0
	return op, nil
}

// ---------------------------------------------------------------------------
// CopyModel
// ---------------------------------------------------------------------------

// CopyModel creates a local copy or alias of a model.
func (p *Provider) CopyModel(ctx context.Context, source, target string) (llm.ProviderOperation, error) {
	opID := uuid.New().String()
	now := time.Now()
	op := llm.ProviderOperation{
		ID:        opID,
		Provider:  "ollama",
		Action:    "copy",
		Model:     source,
		Status:    llm.OperationStatusRunning,
		StartedAt: now,
	}

	body, _ := json.Marshal(map[string]string{"source": source, "destination": target})
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/copy", bytes.NewReader(body))
	if err != nil {
		op.Status = llm.OperationStatusFailed
		op.Error = err.Error()
		completed := time.Now()
		op.CompletedAt = &completed
		return op, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		op.Status = llm.OperationStatusFailed
		op.Error = err.Error()
		completed := time.Now()
		op.CompletedAt = &completed
		return op, err
	}
	defer res.Body.Close()

	completed := time.Now()
	op.CompletedAt = &completed

	if res.StatusCode != http.StatusOK {
		op.Status = llm.OperationStatusFailed
		op.Error = fmt.Sprintf("HTTP %d", res.StatusCode)
		return op, fmt.Errorf("ollama: copy model %q -> %q: status %d", source, target, res.StatusCode)
	}

	op.Status = llm.OperationStatusCompleted
	op.Progress = 1.0
	return op, nil
}

// ---------------------------------------------------------------------------
// CreateModel
// ---------------------------------------------------------------------------

// CreateModel creates a model from a Modelfile.
func (p *Provider) CreateModel(ctx context.Context, createReq llm.CreateModelRequest) (llm.ProviderOperation, error) {
	opID := uuid.New().String()
	now := time.Now()
	op := llm.ProviderOperation{
		ID:        opID,
		Provider:  "ollama",
		Action:    "create",
		Model:     createReq.Name,
		Status:    llm.OperationStatusRunning,
		StartedAt: now,
	}

	body, _ := json.Marshal(map[string]any{
		"name":      createReq.Name,
		"modelfile": createReq.Modelfile,
		"stream":    false,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/create", bytes.NewReader(body))
	if err != nil {
		op.Status = llm.OperationStatusFailed
		op.Error = err.Error()
		completed := time.Now()
		op.CompletedAt = &completed
		return op, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		op.Status = llm.OperationStatusFailed
		op.Error = err.Error()
		completed := time.Now()
		op.CompletedAt = &completed
		return op, err
	}
	defer res.Body.Close()

	completed := time.Now()
	op.CompletedAt = &completed

	if res.StatusCode != http.StatusOK {
		op.Status = llm.OperationStatusFailed
		op.Error = fmt.Sprintf("HTTP %d", res.StatusCode)
		return op, fmt.Errorf("ollama: create model %q: status %d", createReq.Name, res.StatusCode)
	}

	op.Status = llm.OperationStatusCompleted
	op.Progress = 1.0
	return op, nil
}

// ---------------------------------------------------------------------------
// WarmModel
// ---------------------------------------------------------------------------

// WarmModel pre-loads a model into GPU/CPU memory by sending an empty
// generate request.
func (p *Provider) WarmModel(ctx context.Context, model string) error {
	body, _ := json.Marshal(map[string]any{
		"model":  model,
		"prompt": "",
	})
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: warm model %q: %w", model, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: warm model %q: status %d", model, res.StatusCode)
	}
	return nil
}
