package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/open-navi/navi/internal/config"
)

// SettingStore abstracts the persistence layer used to store the active
// provider/model selection. It is intentionally minimal so that the llm
// package does not depend directly on database/sql.
type SettingStore interface {
	GetSetting(ctx context.Context, key string) (value string, found bool, err error)
	SetSetting(ctx context.Context, key, value string) error
}

// SettingStoreFunc adapts free functions to the SettingStore interface.
type SettingStoreFunc func(ctx context.Context, key string) (value string, found bool, err error)

// GetSetting implements SettingStore.
func (f SettingStoreFunc) GetSetting(ctx context.Context, key string) (string, bool, error) {
	return f(ctx, key)
}

// SetSetting implements SettingStore. When used, the function must be a
// closure that knows how to persist the key/value pair.
type SettingStoreSetFunc func(ctx context.Context, key, value string) error

// compositeSettingStore is a small helper that wires separate get/set
// functions into a single SettingStore.
type compositeSettingStore struct {
	get SettingStoreFunc
	set SettingStoreSetFunc
}

func (c compositeSettingStore) GetSetting(ctx context.Context, key string) (string, bool, error) {
	return c.get(ctx, key)
}

func (c compositeSettingStore) SetSetting(ctx context.Context, key, value string) error {
	return c.set(ctx, key, value)
}

// SelectionState tracks the active provider/model pair and applies it to a
// DynamicProvider. It is backed by a SettingStore so selections survive
// process restarts.
type SelectionState struct {
	store SettingStore
	cfg   *config.LLMConfig
	dp    *DynamicProvider
}

// NewSelectionState constructs a SelectionState bound to the given
// DynamicProvider, settings backend, and LLMConfig.
func NewSelectionState(dp *DynamicProvider, store SettingStore, cfg *config.LLMConfig) *SelectionState {
	return &SelectionState{
		store: store,
		cfg:   cfg,
		dp:    dp,
	}
}

// NewSettingStore wires separate get/set functions into a SettingStore.
func NewSettingStore(
	get func(ctx context.Context, key string) (string, bool, error),
	set func(ctx context.Context, key, value string) error,
) SettingStore {
	return compositeSettingStore{
		get: SettingStoreFunc(get),
		set: SettingStoreSetFunc(set),
	}
}

// Active holds the currently selected provider/model values.
type Active struct {
	Provider string
	Model    string
}

// GetActive returns the currently selected provider and model, falling back
// to a sensible default derived from config and catalog when no explicit
// selection has been stored yet.
func (s *SelectionState) GetActive(ctx context.Context, catalog LLMCatalog) (Active, error) {
	if s.store == nil || s.cfg == nil {
		return Active{}, nil
	}

	// 1) Try persisted provider/model pair first.
	provider, _, err := s.store.GetSetting(ctx, "llm_provider")
	if err != nil {
		return Active{}, err
	}
	model, _, err := s.store.GetSetting(ctx, "llm_model")
	if err != nil {
		return Active{}, err
	}
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)

	if provider != "" && model != "" {
		return Active{Provider: provider, Model: model}, nil
	}

	// 2) Fall back to first catalog provider/model if available.
	if len(catalog.Providers) > 0 {
		p := catalog.Providers[0]
		if len(p.Models) > 0 {
			return Active{Provider: p.Key, Model: p.Models[0].Name}, nil
		}
	}

	// 3) Finally, fall back to config-level defaults.
	// Preference order: Ollama → Anthropic → OpenAI → OpenRouter.
	if strings.TrimSpace(s.cfg.OllamaURL) != "" && s.cfg.OllamaModel != "" {
		return Active{Provider: "ollama", Model: s.cfg.OllamaModel}, nil
	}
	if s.cfg.AnthropicKey != "" && s.cfg.AnthropicModel != "" {
		return Active{Provider: "anthropic", Model: s.cfg.AnthropicModel}, nil
	}
	if s.cfg.OpenAIKey != "" && s.cfg.OpenAIModel != "" {
		return Active{Provider: "openai", Model: s.cfg.OpenAIModel}, nil
	}
	if s.cfg.OpenRouterKey != "" && s.cfg.OpenRouterModel != "" {
		return Active{Provider: "openrouter", Model: s.cfg.OpenRouterModel}, nil
	}

	return Active{}, nil
}

// SetActive validates the requested provider/model against the catalog,
// persists the selection, and updates the DynamicProvider so subsequent
// Chat calls use the new backend.
func (s *SelectionState) SetActive(ctx context.Context, catalog LLMCatalog, providerKey, model string) (Active, error) {
	return s.apply(ctx, catalog, providerKey, model, true)
}

// ApplyTransient validates and swaps the provider/model without persisting it.
// This is used by per-turn contextual routing so the current request can switch
// models without rewriting the operator's manually selected default.
func (s *SelectionState) ApplyTransient(ctx context.Context, catalog LLMCatalog, providerKey, model string) (Active, error) {
	return s.apply(ctx, catalog, providerKey, model, false)
}

func (s *SelectionState) apply(ctx context.Context, catalog LLMCatalog, providerKey, model string, persist bool) (Active, error) {
	if s.store == nil || s.cfg == nil || s.dp == nil {
		return Active{}, fmt.Errorf("llm: selection state not fully configured")
	}

	providerKey = strings.TrimSpace(strings.ToLower(providerKey))
	model = strings.TrimSpace(model)
	if providerKey == "" || model == "" {
		return Active{}, fmt.Errorf("llm: provider and model are required")
	}

	// Validate against catalog and normalize model casing.
	valid := false
	for _, p := range catalog.Providers {
		if strings.ToLower(p.Key) != providerKey {
			continue
		}
		for _, m := range p.Models {
			if strings.EqualFold(m.Name, model) {
				model = m.Name // normalize to catalog spelling
				valid = true
				break
			}
		}
		break
	}
	if !valid {
		return Active{}, fmt.Errorf("llm: unknown provider/model combination %q / %q", providerKey, model)
	}

	// Build the concrete provider instance for this selection.
	selectedProvider, err := s.buildProviderFor(providerKey)
	if err != nil {
		return Active{}, err
	}

	// Persist and apply to DynamicProvider.
	if persist {
		if err := s.store.SetSetting(ctx, "llm_provider", providerKey); err != nil {
			return Active{}, err
		}
		if err := s.store.SetSetting(ctx, "llm_model", model); err != nil {
			return Active{}, err
		}
	}

	s.dp.SwapWithModel(selectedProvider, model)

	return Active{Provider: providerKey, Model: model}, nil
}

// buildProviderFor constructs a concrete Provider for the given logical
// provider key, using the shared LLMConfig for credentials and URLs.
func (s *SelectionState) buildProviderFor(providerKey string) (Provider, error) {
	return BuildProviderForSelection(s.cfg, providerKey)
}
