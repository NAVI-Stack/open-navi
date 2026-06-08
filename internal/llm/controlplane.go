package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ceoai/navi/internal/config"
)

// ControlPlane is the runtime service that owns all LLM provider infrastructure:
// provider registry, selection, catalog, profiles, routing, health, and actions.
// It replaces the ad-hoc closures previously scattered through cmd/navid/main.go.
type ControlPlane struct {
	cfg    *config.Config
	llmCfg *config.LLMConfig

	dp        *DynamicProvider
	selection *SelectionState
	store     SettingStore

	llmkbRepo    LLMKBRepo
	outcomeStore ExecutionOutcomeStore
	opStore      ProviderOperationStore
	healthStore  HealthSnapshotStore

	// providers maps provider key → concrete Provider instance.
	providers map[string]Provider

	// providerOverrides allows callers to inject pre-built provider instances
	// without creating a dependency from internal/llm to concrete plugin packages.
	providerOverrides map[string]Provider

	onOpenAIKeySet func(string)

	mu sync.RWMutex

	// Catalog cache — amortises repeated EnrichOllamaFromAPI round-trips.
	// Invalidated explicitly whenever provider config changes.
	catalogCacheMu sync.RWMutex
	catalogCache   LLMCatalog
	catalogCacheAt time.Time
}

// ControlPlaneOptions holds the dependencies for constructing a ControlPlane.
type ControlPlaneOptions struct {
	Config         *config.Config
	SettingStore   SettingStore
	LLMKBRepo      LLMKBRepo              // may be nil
	OutcomeStore   ExecutionOutcomeStore  // may be nil
	OpStore        ProviderOperationStore // may be nil
	HealthStore    HealthSnapshotStore    // may be nil
	BaseProvider   Provider               // from FromConfig(); may be nil
	OnOpenAIKeySet func(string)           // callback for knowledge embedder sync
	// ProviderOverrides allows callers to inject pre-built provider instances
	// without creating a dependency from internal/llm to concrete plugin packages.
	// Keyed by provider name (e.g. "ollama").
	ProviderOverrides map[string]Provider // may be nil
}

// NewControlPlane constructs a ControlPlane and restores persisted selection.
// This absorbs the LLM boot wiring that was previously inline in main.go.
func NewControlPlane(opts ControlPlaneOptions) (*ControlPlane, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("llm: config is required")
	}

	dp := NewDynamicProvider(opts.BaseProvider)

	cp := &ControlPlane{
		cfg:               opts.Config,
		llmCfg:            &opts.Config.LLM,
		dp:                dp,
		store:             opts.SettingStore,
		llmkbRepo:         opts.LLMKBRepo,
		outcomeStore:      opts.OutcomeStore,
		opStore:           opts.OpStore,
		healthStore:       opts.HealthStore,
		providers:         make(map[string]Provider),
		providerOverrides: opts.ProviderOverrides,
		onOpenAIKeySet:    opts.OnOpenAIKeySet,
	}

	// Create the selection state.
	cp.selection = NewSelectionState(dp, opts.SettingStore, cp.llmCfg)

	// Build the provider registry from config.
	cp.buildProviderRegistry()

	// Restore persisted selection from settings.
	if opts.BaseProvider != nil {
		ctx := context.Background()
		catalog, _ := cp.Catalog(ctx)
		if active, err := cp.selection.GetActive(ctx, catalog); err == nil && active.Provider != "" && active.Model != "" {
			if _, err := cp.selection.SetActive(ctx, catalog, active.Provider, active.Model); err != nil {
				slog.Warn("llm: failed to restore selection, using base provider", "error", err)
				dp.Swap(opts.BaseProvider)
			} else {
				slog.Info("LLM selection restored", "provider", active.Provider, "model", active.Model)
			}
		} else {
			dp.Swap(opts.BaseProvider)
		}
	}

	return cp, nil
}

// buildProviderRegistry populates the providers map from config.
func (cp *ControlPlane) buildProviderRegistry() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	cp.providers = make(map[string]Provider) // Clear stale providers before rebuild

	if override, ok := cp.providerOverrides["ollama"]; ok {
		cp.providers["ollama"] = override
		// Wire pull progress callback via duck-typing so we don't import plugins.
		type pullCallbackSetter interface {
			SetPullProgressCallback(func(string, OperationStatus, float64, string, string))
		}
		if setter, ok := override.(pullCallbackSetter); ok && cp.opStore != nil {
			setter.SetPullProgressCallback(func(id string, status OperationStatus, progress float64, msg, errMsg string) {
				if err := cp.opStore.UpdateProviderOperation(context.Background(), id, status, progress, msg, errMsg); err != nil {
					slog.Debug("llm: failed to update pull progress", "op_id", id, "error", err)
				}
			})
		}
	} else if strings.TrimSpace(cp.llmCfg.OllamaURL) != "" {
		if p, err := BuildProviderForSelection(cp.llmCfg, "ollama"); err == nil {
			cp.providers["ollama"] = p
		} else {
			slog.Debug("llm: build ollama provider failed", "error", err)
		}
	}
	if cp.llmCfg.AnthropicKey != "" {
		if p, err := BuildProviderForSelection(cp.llmCfg, "anthropic"); err == nil {
			cp.providers["anthropic"] = p
		} else {
			slog.Debug("llm: build anthropic provider failed", "error", err)
		}
	}
	if cp.llmCfg.OpenAIKey != "" {
		if p, err := BuildProviderForSelection(cp.llmCfg, "openai"); err == nil {
			cp.providers["openai"] = p
		} else {
			slog.Debug("llm: build openai provider failed", "error", err)
		}
	}
	if cp.llmCfg.OpenRouterKey != "" {
		if p, err := BuildProviderForSelection(cp.llmCfg, "openrouter"); err == nil {
			cp.providers["openrouter"] = p
		} else {
			slog.Debug("llm: build openrouter provider failed", "error", err)
		}
	}
}

// DynamicProvider returns the underlying DynamicProvider for direct chat use.
func (cp *ControlPlane) DynamicProvider() *DynamicProvider {
	return cp.dp
}

// Status returns a human-readable provider/model status string.
func (cp *ControlPlane) Status() string {
	return cp.dp.NameWithModel()
}

// ChatProvider returns the Provider interface for chat inference.
func (cp *ControlPlane) ChatProvider() Provider {
	return cp.dp
}

// catalogCacheTTL is how long a built catalog is considered fresh.
// Mutating operations (ConfigureProvider, DisableProvider) explicitly invalidate
// the cache so the next call always sees a current catalog.
const catalogCacheTTL = 30 * time.Second

// --- Catalog & Profiles ---

// Catalog returns the current provider/model catalog, enriched with live Ollama data.
// Results are cached for catalogCacheTTL to avoid repeated network round-trips to Ollama
// when multiple callers (ListProviders, GetActive, Profiles, Route) hit it in the same
// request burst.
func (cp *ControlPlane) Catalog(ctx context.Context) (LLMCatalog, error) {
	// Fast path: serve from cache when still fresh.
	cp.catalogCacheMu.RLock()
	if !cp.catalogCacheAt.IsZero() && time.Since(cp.catalogCacheAt) < catalogCacheTTL {
		cached := cp.catalogCache
		cp.catalogCacheMu.RUnlock()
		return cached, nil
	}
	cp.catalogCacheMu.RUnlock()

	// Slow path: rebuild (potentially involves an Ollama network call).
	catalog := BuildCatalog(cp.llmCfg)
	if strings.TrimSpace(cp.llmCfg.OllamaURL) != "" {
		if err := EnrichOllamaFromAPI(ctx, &catalog, cp.llmCfg.OllamaURL); err != nil {
			slog.Debug("llm: could not enrich ollama catalog", "error", err)
		}
	}

	cp.catalogCacheMu.Lock()
	cp.catalogCache = catalog
	cp.catalogCacheAt = time.Now()
	cp.catalogCacheMu.Unlock()

	return catalog, nil
}

// invalidateCatalogCache forces the next Catalog() call to rebuild from scratch.
func (cp *ControlPlane) invalidateCatalogCache() {
	cp.catalogCacheMu.Lock()
	cp.catalogCacheAt = time.Time{}
	cp.catalogCacheMu.Unlock()
}

// Profiles returns task-aware model profiles with KB enrichment and calibration.
func (cp *ControlPlane) Profiles(ctx context.Context) ([]ModelProfile, error) {
	catalog, _ := cp.Catalog(ctx)
	profiles := BuildProfilesFromCatalogAndKB(ctx, cp.llmkbRepo, catalog)

	if cp.outcomeStore != nil {
		outcomes, err := cp.outcomeStore.ListCalibratedLLMExecutionOutcomes(ctx, 300)
		if err != nil {
			slog.Debug("llm: could not load calibration outcomes", "error", err)
		} else {
			profiles = ApplyCalibration(profiles, outcomes)
		}
	}

	return profiles, nil
}

// --- Active Selection ---

// GetActive returns the currently selected provider/model.
func (cp *ControlPlane) GetActive(ctx context.Context) (Active, error) {
	catalog, _ := cp.Catalog(ctx)
	return cp.selection.GetActive(ctx, catalog)
}

// SetActive validates, persists, and applies a new active provider/model selection.
// The special pair provider="navi", model="auto" enables auto-routing mode: the per-turn
// routing engine selects the best model for each message instead of using a pinned model.
func (cp *ControlPlane) SetActive(ctx context.Context, provider, model string) (Active, error) {
	p := strings.TrimSpace(strings.ToLower(provider))
	m := strings.TrimSpace(strings.ToLower(model))
	if p == "navi" && m == "auto" {
		// Store sentinel directly — bypass catalog validation.
		if cp.store != nil {
			if err := cp.store.SetSetting(ctx, "llm_provider", "navi"); err != nil {
				return Active{}, err
			}
			if err := cp.store.SetSetting(ctx, "llm_model", "auto"); err != nil {
				return Active{}, err
			}
		}
		// Flip the auto-routing preference flag so Route() knows to use ModelSelector.
		if _, err := cp.PatchPreferences(ctx, map[string]any{"auto_routing_enabled": true}); err != nil {
			slog.Warn("llm: could not persist auto_routing_enabled", "error", err)
		}
		return Active{Provider: "navi", Model: "auto"}, nil
	}

	catalog, _ := cp.Catalog(ctx)
	resolvedModel := ResolveModelAlias(catalog, provider, model)
	active, err := cp.selection.SetActive(ctx, catalog, provider, resolvedModel)
	if err != nil {
		slog.Warn("llm: set active failed",
			"provider", p,
			"model", strings.TrimSpace(model),
			"resolved_model", strings.TrimSpace(resolvedModel),
			"error", err,
		)
		return Active{}, err
	}
	// Clear auto-routing flag when user explicitly picks a real model.
	if _, err := cp.PatchPreferences(ctx, map[string]any{"auto_routing_enabled": false}); err != nil {
		slog.Warn("llm: could not clear auto_routing_enabled", "error", err)
	}
	return active, nil
}

// --- Routing Preferences ---

// GetPreferences returns persisted model routing preferences.
func (cp *ControlPlane) GetPreferences(ctx context.Context) (ModelPreferences, error) {
	return LoadModelPreferences(ctx, cp.store)
}

// PatchPreferences applies a partial update to routing preferences.
func (cp *ControlPlane) PatchPreferences(ctx context.Context, raw map[string]any) (ModelPreferences, error) {
	payload, err := json.Marshal(raw)
	if err != nil {
		return ModelPreferences{}, err
	}
	var patch PreferencePatch
	if err := json.Unmarshal(payload, &patch); err != nil {
		return ModelPreferences{}, err
	}
	prefs, err := cp.GetPreferences(ctx)
	if err != nil {
		return ModelPreferences{}, err
	}
	prefs = prefs.Apply(patch)
	if err := SaveModelPreferences(ctx, cp.store, prefs); err != nil {
		return ModelPreferences{}, err
	}
	return prefs, nil
}

// --- Per-Turn Routing ---

// Route makes a per-turn routing decision. This is the method that replaces
// the huge routeTurnLLM closure from main.go.
func (cp *ControlPlane) Route(ctx context.Context, req RouteRequest) (RouteDecision, error) {
	catalog, _ := cp.Catalog(ctx)
	classification := ClassifyTask(req.UserMessage, req.Tools, req.History)

	prefs, err := cp.GetPreferences(ctx)
	if err != nil {
		return RouteDecision{}, err
	}
	if classification.PreferencePatch != nil {
		prefs = prefs.Apply(*classification.PreferencePatch)
		if err := SaveModelPreferences(ctx, cp.store, prefs); err != nil {
			return RouteDecision{}, err
		}
	}

	// Resolve current persisted selection as the default.
	currentProvider := req.CurrentProvider
	currentModel := req.CurrentModel
	if currentProvider == "" || currentModel == "" {
		if active, err := cp.selection.GetActive(ctx, catalog); err == nil {
			currentProvider = active.Provider
			currentModel = active.Model
		}
	}

	// OMN-86 Phase 1: Predictable-first routing.
	// Only switch models when the user explicitly requested it via per-turn override,
	// or when NAVI Auto mode is active (auto_routing_enabled preference or navi/auto sentinel).
	userRequestedSwitch := false
	for _, sig := range classification.Signals {
		if sig == "per_turn_override" {
			userRequestedSwitch = true
			break
		}
	}
	autoMode := prefs.AutoRoutingEnabled ||
		(strings.ToLower(currentProvider) == "navi" && strings.ToLower(currentModel) == "auto")

	if !userRequestedSwitch && !autoMode {
		// No explicit switch request — use persisted model, log classification only.
		slog.Debug("llm routing: no explicit switch requested, using persisted model",
			"task", classification.Task, "signals", classification.Signals,
			"provider", currentProvider, "model", currentModel)

		// Determine if tools need to be stripped for the current model.
		profiles, _ := cp.Profiles(ctx)
		stripTools := false
		for _, p := range profiles {
			if strings.EqualFold(p.ProviderKey, currentProvider) && strings.EqualFold(p.ModelID, currentModel) {
				if !p.SupportsTools || !p.ToolCallReliable {
					stripTools = true
				}
				break
			}
		}

		decision := RouteDecision{
			Provider:       currentProvider,
			Model:          currentModel,
			Classification: classification,
			ChatID:         req.ChatID,
			TaskID:         req.TaskID,
			Switched:       false,
			Reason:         "persisted_default",
			StripTools:     stripTools,
		}

		// Telemetry: Record decision in LLM-KB
		if cp.llmkbRepo != nil {
			go func() {
				if mapped := MapRouteDecisionToKB(context.Background(), cp.llmkbRepo, classification, decision); mapped != nil {
					_ = cp.llmkbRepo.AppendRouterDecision(context.Background(), *mapped)
				}
			}()
		}

		return decision, nil
	}

	// User explicitly requested a switch — honour it.
	profiles, _ := cp.Profiles(ctx)
	selector := ModelSelector{Profiles: profiles, Preferences: prefs}
	selectionResult, ok := selector.Select(classification, len(req.Tools) > 0)
	stripTools := false
	if (!ok || selectionResult.Provider == "" || selectionResult.Model == "") && len(req.Tools) > 0 {
		selectionResult, ok = selector.Select(classification, false)
		stripTools = ok && selectionResult.Provider != "" && selectionResult.Model != ""
	}
	if !ok || selectionResult.Provider == "" || selectionResult.Model == "" {
		return RouteDecision{Classification: classification, Reason: "no_matching_model"}, nil
	}

	resolvedModel := ResolveModelAlias(catalog, selectionResult.Provider, selectionResult.Model)

	// No-op guard: if resolved target is already the active model, skip swap.
	if strings.EqualFold(strings.TrimSpace(currentProvider), selectionResult.Provider) &&
		strings.EqualFold(strings.TrimSpace(currentModel), resolvedModel) {
		return RouteDecision{
			Provider:       currentProvider,
			Model:          currentModel,
			Classification: classification,
			Switched:       false,
			Reason:         "noop_already_active",
			Profile:        selectionResult.Profile,
			StripTools:     stripTools,
		}, nil
	}

	active, err := cp.selection.ApplyTransient(ctx, catalog, selectionResult.Provider, resolvedModel)
	if err != nil {
		return RouteDecision{}, err
	}

	// User-initiated switch → AnnouncementUser (always visible).
	announcement := fmt.Sprintf("Switching to %s/%s for this %s task.", active.Provider, active.Model, classification.Task)
	decision := RouteDecision{
		Provider:               active.Provider,
		Model:                  active.Model,
		Classification:         classification,
		Switched:               true,
		Announcement:           announcement,
		AnnouncementVisibility: RoutingVisibilityUser,
		Reason:                 selectionResult.Reason,
		Profile:                selectionResult.Profile,
		StripTools:             stripTools,
		ChatID:                 req.ChatID,
		TaskID:                 req.TaskID,
	}

	// Telemetry: Record decision in LLM-KB
	if cp.llmkbRepo != nil {
		go func() {
			if mapped := MapRouteDecisionToKB(context.Background(), cp.llmkbRepo, classification, decision); mapped != nil {
				_ = cp.llmkbRepo.AppendRouterDecision(context.Background(), *mapped)
			}
		}()
	}

	return decision, nil
}

// --- Provider Inspection ---

// knownProviders is the fixed set of provider keys the system knows about,
// returned by ListProviders even when not yet configured.
var knownProviders = []string{"anthropic", "openai", "openrouter", "ollama"}

// ListProviders returns descriptors for all known providers, including unconfigured ones.
func (cp *ControlPlane) ListProviders(ctx context.Context) ([]ProviderDescriptor, error) {
	catalog, _ := cp.Catalog(ctx)

	cp.mu.RLock()
	defer cp.mu.RUnlock()

	descriptors := make([]ProviderDescriptor, 0, len(knownProviders))
	for _, key := range knownProviders {
		prov := cp.providers[key] // may be nil (unconfigured)
		desc := cp.buildProviderDescriptorFull(ctx, key, prov)
		for _, p := range catalog.Providers {
			if strings.EqualFold(p.Key, key) {
				desc.ModelCount = len(p.Models)
				break
			}
		}
		descriptors = append(descriptors, desc)
	}
	return descriptors, nil
}

// GetProvider returns a descriptor for a single provider.
func (cp *ControlPlane) GetProvider(ctx context.Context, providerKey string) (ProviderDescriptor, error) {
	catalog, _ := cp.Catalog(ctx)

	cp.mu.RLock()
	prov := cp.providers[providerKey] // nil if unconfigured
	cp.mu.RUnlock()

	// Only reject completely unknown keys (not in our known set).
	isKnown := false
	for _, k := range knownProviders {
		if k == providerKey {
			isKnown = true
			break
		}
	}
	if !isKnown {
		return ProviderDescriptor{}, fmt.Errorf("llm: unknown provider %q", providerKey)
	}

	desc := cp.buildProviderDescriptorFull(ctx, providerKey, prov)
	for _, p := range catalog.Providers {
		if strings.EqualFold(p.Key, providerKey) {
			desc.ModelCount = len(p.Models)
			break
		}
	}
	return desc, nil
}

// buildProviderDescriptorFull builds a descriptor for any known provider, even unconfigured ones.
func (cp *ControlPlane) buildProviderDescriptorFull(ctx context.Context, key string, prov Provider) ProviderDescriptor {
	kind := classifyProviderKind(key)
	configured := prov != nil
	capabilities := []string{"chat"}

	if configured {
		if _, ok := prov.(StreamingProvider); ok {
			capabilities = append(capabilities, "stream")
		}
		if _, ok := prov.(ManagedProvider); ok {
			capabilities = append(capabilities, "managed")
		}
		if _, ok := prov.(LocalLifecycleProvider); ok {
			capabilities = append(capabilities, "lifecycle")
		}
	}

	policy := cp.policyFor(key)

	return ProviderDescriptor{
		Key:          key,
		DisplayName:  providerDisplayName(key),
		Kind:         kind,
		Enabled:      policy.Enabled && configured,
		Configured:   configured,
		Capabilities: capabilities,
		Policies:     &policy,
	}
}

// buildProviderDescriptor builds a descriptor for a configured provider (kept for internal callers).
func (cp *ControlPlane) buildProviderDescriptor(ctx context.Context, key string, prov Provider) ProviderDescriptor {
	return cp.buildProviderDescriptorFull(ctx, key, prov)
}

// Health performs a live health check on a provider.
func (cp *ControlPlane) Health(ctx context.Context, providerKey string) (ProviderHealth, error) {
	cp.mu.RLock()
	prov, ok := cp.providers[providerKey]
	cp.mu.RUnlock()

	if !ok {
		return ProviderHealth{}, fmt.Errorf("llm: unknown provider %q", providerKey)
	}

	mp, ok := prov.(ManagedProvider)
	if !ok {
		// Non-managed providers: best-effort health via name check.
		return ProviderHealth{
			Provider:  providerKey,
			Healthy:   true,
			Message:   "provider does not support health checks",
			CheckedAt: now(),
		}, nil
	}

	health, err := mp.HealthCheck(ctx)
	if err != nil {
		health = ProviderHealth{
			Provider:  providerKey,
			Healthy:   false,
			Message:   err.Error(),
			CheckedAt: now(),
		}
	}

	// Persist snapshot if store is available.
	if cp.healthStore != nil {
		go func() {
			if err := cp.healthStore.SaveHealthSnapshot(context.Background(), health); err != nil {
				slog.Debug("llm: failed to save health snapshot", "error", err)
			}
		}()
	}

	return health, nil
}

// --- Provider Actions ---

// ExecuteProviderAction runs an action (pull, delete, warm, copy, create) on a provider.
func (cp *ControlPlane) ExecuteProviderAction(ctx context.Context, req ProviderActionRequest) (ProviderOperation, error) {
	// Policy check.
	policy := cp.policyFor(req.Provider)
	if !policy.Enabled {
		return ProviderOperation{}, fmt.Errorf("llm: provider %q is disabled by policy", req.Provider)
	}

	switch req.Action {
	case "pull":
		if !policy.AllowPull {
			return ProviderOperation{}, fmt.Errorf("llm: pulling models is disabled by policy for %q", req.Provider)
		}
	case "delete":
		if !policy.AllowDelete {
			return ProviderOperation{}, fmt.Errorf("llm: deleting models is disabled by policy for %q", req.Provider)
		}
	}

	cp.mu.RLock()
	prov, ok := cp.providers[req.Provider]
	cp.mu.RUnlock()

	if !ok {
		return ProviderOperation{}, fmt.Errorf("llm: unknown provider %q", req.Provider)
	}

	lp, ok := prov.(LocalLifecycleProvider)
	if !ok {
		return ProviderOperation{}, fmt.Errorf("llm: provider %q does not support lifecycle actions", req.Provider)
	}

	var op ProviderOperation
	var err error

	switch req.Action {
	case "pull":
		op, err = lp.PullModel(ctx, req.Model)
	case "delete":
		op, err = lp.DeleteModel(ctx, req.Model)
	case "warm":
		warmErr := lp.WarmModel(ctx, req.Model)
		op = ProviderOperation{
			ID:       fmt.Sprintf("warm-%s-%d", req.Model, now().UnixMilli()),
			Provider: req.Provider,
			Action:   "warm",
			Model:    req.Model,
			Status:   OperationStatusCompleted,
			Progress: 1.0,
		}
		if warmErr != nil {
			op.Status = OperationStatusFailed
			op.Error = warmErr.Error()
			err = warmErr
		}
		completed := now()
		op.CompletedAt = &completed
		op.StartedAt = completed
	case "copy":
		if req.Target == "" {
			return ProviderOperation{}, fmt.Errorf("llm: copy action requires a target name")
		}
		op, err = lp.CopyModel(ctx, req.Model, req.Target)
	case "create":
		return ProviderOperation{}, fmt.Errorf("llm: create action requires a CreateModelRequest, use the provider directly")
	default:
		return ProviderOperation{}, fmt.Errorf("llm: unknown action %q", req.Action)
	}

	// Persist initial operation if store is available.
	if cp.opStore != nil && op.ID != "" {
		if saveErr := cp.opStore.SaveProviderOperation(ctx, op); saveErr != nil {
			return ProviderOperation{}, fmt.Errorf("llm: failed to save initial provider operation: %w", saveErr)
		}
	}

	return op, err
}

// GetOperation retrieves a provider operation by ID.
func (cp *ControlPlane) GetOperation(ctx context.Context, id string) (*ProviderOperation, error) {
	if cp.opStore == nil {
		return nil, fmt.Errorf("llm: operation store not configured")
	}
	return cp.opStore.GetProviderOperation(ctx, id)
}

// ListOperations lists recent provider operations.
func (cp *ControlPlane) ListOperations(ctx context.Context, provider string, limit int) ([]ProviderOperation, error) {
	if cp.opStore == nil {
		return nil, fmt.Errorf("llm: operation store not configured")
	}
	return cp.opStore.ListProviderOperations(ctx, provider, limit)
}

// --- Provider Models (via control surface) ---

// ProviderModels lists models for a specific provider via its management interface.
func (cp *ControlPlane) ProviderModels(ctx context.Context, providerKey string) ([]ModelDescriptor, error) {
	cp.mu.RLock()
	prov, ok := cp.providers[providerKey]
	cp.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("llm: unknown provider %q", providerKey)
	}

	mp, ok := prov.(ManagedProvider)
	if !ok {
		return nil, fmt.Errorf("llm: provider %q does not support model listing", providerKey)
	}

	return mp.ListModels(ctx)
}

// ProviderRunningModels lists currently running models for a local provider.
func (cp *ControlPlane) ProviderRunningModels(ctx context.Context, providerKey string) ([]RunningModel, error) {
	cp.mu.RLock()
	prov, ok := cp.providers[providerKey]
	cp.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("llm: unknown provider %q", providerKey)
	}

	lp, ok := prov.(LocalLifecycleProvider)
	if !ok {
		return nil, fmt.Errorf("llm: provider %q does not support running model listing", providerKey)
	}

	return lp.ListRunningModels(ctx)
}

// SearchModels queries remote provider registries (like Ollama) for available models.
func (cp *ControlPlane) SearchModels(ctx context.Context, query string) ([]ModelDescriptor, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	// For now, only Ollama models support remote lookup in this way.
	// We'll search for exact matches in the Ollama registry.
	exists, err := VerifyOllamaModel(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search remote registry: %w", err)
	}
	if exists {
		return []ModelDescriptor{
			{Name: query}, // Minimal viable descriptor for a remote hit
		}, nil
	}
	return nil, nil
}

// --- Setup (replaces OnSetupLLM from gateway) ---

// ConfigureProvider configures and hot-swaps an LLM provider, supporting the
// full set of per-provider parameters including Ollama endpoint URL.
func (cp *ControlPlane) ConfigureProvider(ctx context.Context, req ProviderConfigRequest) error {
	provider := strings.ToLower(req.Provider)
	model := req.Model

	switch provider {
	case "anthropic":
		cp.llmCfg.AnthropicKey = req.APIKey
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}
		cp.llmCfg.AnthropicModel = model
		if cp.store != nil {
			if err := cp.store.SetSetting(ctx, "llm_anthropic_key", req.APIKey); err != nil {
				return err
			}
			_ = cp.store.SetSetting(ctx, "llm_anthropic_model", model)
		}
	case "openai":
		cp.llmCfg.OpenAIKey = req.APIKey
		if model == "" {
			model = "gpt-4o-mini"
		}
		cp.llmCfg.OpenAIModel = model
		if cp.onOpenAIKeySet != nil {
			cp.onOpenAIKeySet(req.APIKey)
		}
		if cp.store != nil {
			if err := cp.store.SetSetting(ctx, "llm_openai_key", req.APIKey); err != nil {
				return err
			}
			_ = cp.store.SetSetting(ctx, "llm_openai_model", model)
		}
	case "openrouter":
		cp.llmCfg.OpenRouterKey = req.APIKey
		if model != "" {
			cp.llmCfg.OpenRouterModel = model
		}
		if cp.store != nil {
			if err := cp.store.SetSetting(ctx, "llm_openrouter_key", req.APIKey); err != nil {
				return err
			}
			if model != "" {
				_ = cp.store.SetSetting(ctx, "llm_openrouter_model", model)
			}
		}
	case "ollama":
		if req.Endpoint != "" {
			cp.llmCfg.OllamaURL = req.Endpoint
			if cp.store != nil {
				_ = cp.store.SetSetting(ctx, "llm_ollama_url", req.Endpoint)
			}
		}
		if model == "" {
			model = "llama3:latest"
		}
		cp.llmCfg.OllamaModel = model
		if cp.store != nil {
			_ = cp.store.SetSetting(ctx, "llm_ollama_model", model)
		}
	default:
		return fmt.Errorf("unknown LLM provider: %s", provider)
	}

	if cp.store != nil {
		_ = cp.store.SetSetting(ctx, "llm_provider", provider)
	}

	// Rebuild provider chain and swap.
	newProvider, err := FromConfig(cp.cfg)
	if err != nil {
		return fmt.Errorf("rebuild LLM provider: %w", err)
	}
	cp.dp.SwapWithModel(newProvider, model)

	// Rebuild provider registry.
	cp.buildProviderRegistry()

	// Catalog data has changed — invalidate so the next call reflects new config.
	cp.invalidateCatalogCache()

	slog.Info("LLM provider updated", "provider", cp.dp.Name(), "model", model)
	return nil
}

// SetupProvider configures and hot-swaps an LLM provider. This replaces
// the OnSetupLLM closure that was previously inlined in main.go's gateway config.
func (cp *ControlPlane) SetupProvider(ctx context.Context, provider, apiKey, model string) error {
	return cp.ConfigureProvider(ctx, ProviderConfigRequest{
		Provider: provider,
		APIKey:   apiKey,
		Model:    model,
	})
}

// DisableProvider removes an LLM provider's credentials and rebuilds the registry.
// If the disabled provider was the active selection, the stale selection is cleared
// and the chat provider is re-pointed at whatever remains (or disabled entirely),
// so chat never keeps using a provider the user just disconnected.
func (cp *ControlPlane) DisableProvider(ctx context.Context, providerKey string) error {
	key := strings.ToLower(providerKey)

	// Capture whether we're disabling the currently active provider before mutating.
	// (The navi/auto sentinel reports provider "navi", so it is naturally excluded.)
	wasActive := false
	if active, err := cp.GetActive(ctx); err == nil {
		wasActive = strings.EqualFold(active.Provider, key)
	}

	switch key {
	case "anthropic":
		cp.llmCfg.AnthropicKey = ""
		cp.llmCfg.AnthropicModel = ""
		if cp.store != nil {
			_ = cp.store.SetSetting(ctx, "llm_anthropic_key", "")
			_ = cp.store.SetSetting(ctx, "llm_anthropic_model", "")
		}
	case "openai":
		cp.llmCfg.OpenAIKey = ""
		cp.llmCfg.OpenAIModel = ""
		if cp.onOpenAIKeySet != nil {
			cp.onOpenAIKeySet("")
		}
		if cp.store != nil {
			_ = cp.store.SetSetting(ctx, "llm_openai_key", "")
			_ = cp.store.SetSetting(ctx, "llm_openai_model", "")
		}
	case "openrouter":
		cp.llmCfg.OpenRouterKey = ""
		cp.llmCfg.OpenRouterModel = ""
		if cp.store != nil {
			_ = cp.store.SetSetting(ctx, "llm_openrouter_key", "")
			_ = cp.store.SetSetting(ctx, "llm_openrouter_model", "")
		}
	case "ollama":
		// For Ollama, preserve the URL (so re-enabling doesn't require re-entering),
		// but clear the model so it's no longer in the active registry.
		cp.llmCfg.OllamaURL = ""
		if cp.store != nil {
			_ = cp.store.SetSetting(ctx, "llm_ollama_url", "")
		}
	default:
		return fmt.Errorf("llm: unknown provider %q", providerKey)
	}

	cp.buildProviderRegistry()

	// If the disabled provider was the active selection, both the persisted
	// pointer and the DynamicProvider still reference it. Clear the stale
	// selection and re-point chat at the remaining providers (or nil if none).
	if wasActive {
		if cp.store != nil {
			_ = cp.store.SetSetting(ctx, "llm_provider", "")
			_ = cp.store.SetSetting(ctx, "llm_model", "")
		}
		if newProvider, err := FromConfig(cp.cfg); err == nil {
			cp.dp.SwapWithModel(newProvider, "")
			slog.Info("LLM active provider disabled; switched chat provider", "to", cp.dp.Name())
		} else {
			cp.dp.Swap(nil)
			slog.Warn("LLM active provider disabled; no providers remain", "error", err)
		}
	}

	// Catalog data has changed — invalidate so the next call reflects current config.
	cp.invalidateCatalogCache()

	slog.Info("LLM provider disabled", "provider", providerKey)
	return nil
}

// --- Helpers ---

func (cp *ControlPlane) policyFor(providerKey string) ProviderPolicy {
	// Default policy: enabled, allow pull, deny delete.
	policy := ProviderPolicy{
		Enabled:   true,
		AllowPull: true,
	}

	if cp.llmCfg.ProviderPolicies == nil {
		return policy
	}

	pc, ok := cp.llmCfg.ProviderPolicies[providerKey]
	if !ok {
		return policy
	}

	if pc.Enabled != nil {
		policy.Enabled = *pc.Enabled
	}
	if pc.LocalFirst != nil {
		policy.LocalFirst = *pc.LocalFirst
	}
	if pc.AllowPull != nil {
		policy.AllowPull = *pc.AllowPull
	}
	if pc.AllowDelete != nil {
		policy.AllowDelete = *pc.AllowDelete
	}
	if pc.AutoWarm != nil {
		policy.AutoWarm = *pc.AutoWarm
	}
	if pc.AllowPaidRemote != nil {
		policy.AllowPaidRemote = *pc.AllowPaidRemote
	}
	if pc.AllowFallbackLocal != nil {
		policy.AllowFallbackLocal = *pc.AllowFallbackLocal
	}
	return policy
}

func classifyProviderKind(key string) ProviderKind {
	switch key {
	case "ollama":
		return ProviderKindLocal
	case "openrouter":
		return ProviderKindProxy
	default:
		return ProviderKindCloud
	}
}

func providerDisplayName(key string) string {
	switch key {
	case "ollama":
		return "Ollama"
	case "anthropic":
		return "Anthropic"
	case "openai":
		return "OpenAI"
	case "openrouter":
		return "OpenRouter"
	default:
		return key
	}
}

// now is a helper to avoid repeating time.Now() time.Time; also useful for testing.
func now() time.Time {
	return time.Now()
}
