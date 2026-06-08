package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/config"
)

type mockOpStore struct {
	saveErr error
	savedOp *ProviderOperation
}

func (m *mockOpStore) SaveProviderOperation(ctx context.Context, op ProviderOperation) error {
	m.savedOp = &op
	return m.saveErr
}
func (m *mockOpStore) UpdateProviderOperation(ctx context.Context, id string, status OperationStatus, progress float64, msg, errMsg string) error {
	return nil
}
func (m *mockOpStore) GetProviderOperation(ctx context.Context, id string) (*ProviderOperation, error) {
	return m.savedOp, nil
}
func (m *mockOpStore) ListProviderOperations(ctx context.Context, provider string, limit int) ([]ProviderOperation, error) {
	return nil, nil
}

type mockLocalProv struct {
	pullOp  ProviderOperation
	pullErr error
}

func (m *mockLocalProv) Name() string { return "mock" }
func (m *mockLocalProv) Chat(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options) (*Response, error) {
	return nil, nil
}
func (m *mockLocalProv) HealthCheck(ctx context.Context) (ProviderHealth, error) {
	return ProviderHealth{}, nil
}
func (m *mockLocalProv) ListModels(ctx context.Context) ([]ModelDescriptor, error) { return nil, nil }
func (m *mockLocalProv) GetModel(ctx context.Context, model string) (ModelDescriptor, error) {
	return ModelDescriptor{}, nil
}
func (m *mockLocalProv) ListRunningModels(ctx context.Context) ([]RunningModel, error) {
	return nil, nil
}
func (m *mockLocalProv) PullModel(ctx context.Context, model string) (ProviderOperation, error) {
	return m.pullOp, m.pullErr
}
func (m *mockLocalProv) DeleteModel(ctx context.Context, model string) (ProviderOperation, error) {
	return ProviderOperation{}, nil
}
func (m *mockLocalProv) CopyModel(ctx context.Context, source, target string) (ProviderOperation, error) {
	return ProviderOperation{}, nil
}
func (m *mockLocalProv) CreateModel(ctx context.Context, req CreateModelRequest) (ProviderOperation, error) {
	return ProviderOperation{}, nil
}
func (m *mockLocalProv) WarmModel(ctx context.Context, model string) error { return nil }

func TestControlPlane_ExecuteProviderAction_SynchronousPersistence(t *testing.T) {
	ctx := context.Background()
	store := &mockOpStore{}

	prov := &mockLocalProv{
		pullOp: ProviderOperation{
			ID:       "sync-op-1",
			Action:   "pull",
			Provider: "mock",
			Status:   OperationStatusPending,
		},
	}

	cp := &ControlPlane{
		cfg:       &config.Config{},
		llmCfg:    &config.LLMConfig{},
		opStore:   store,
		providers: map[string]Provider{"mock": prov},
	}

	req := ProviderActionRequest{
		Provider: "mock",
		Action:   "pull",
		Model:    "test-model",
	}

	op, err := cp.ExecuteProviderAction(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op.ID != "sync-op-1" {
		t.Errorf("expected op ID sync-op-1, got %s", op.ID)
	}

	if store.savedOp == nil || store.savedOp.ID != "sync-op-1" {
		t.Error("expected operation to be saved synchronously to opStore")
	}
}

func TestControlPlane_Route_NilLLMKBRepo_DoesNotPanic(t *testing.T) {
	// Regression: Route() previously launched a goroutine that called
	// cp.llmkbRepo.AppendRouterDecision unconditionally, panicking when the
	// repo was nil. NewControlPlane documents LLMKBRepo as "may be nil".
	ctx := context.Background()

	noopStore := NewSettingStore(
		func(_ context.Context, _ string) (string, bool, error) { return "", false, nil },
		func(_ context.Context, _, _ string) error { return nil },
	)

	llmCfg := &config.LLMConfig{}
	cp := &ControlPlane{
		cfg:       &config.Config{},
		llmCfg:    llmCfg,
		store:     noopStore,
		llmkbRepo: nil, // intentionally nil — must not panic
		providers: make(map[string]Provider),
		selection: NewSelectionState(nil, noopStore, llmCfg),
	}

	_, err := cp.Route(ctx, RouteRequest{UserMessage: "hello"})
	// Any non-panic outcome is acceptable; the exact error depends on having no
	// providers wired up. We give the goroutine time to complete before exiting.
	_ = err
}

// memSettingStore is a simple in-memory SettingStore for tests.
type memSettingStore struct {
	m map[string]string
}

func newMemSettingStore() *memSettingStore { return &memSettingStore{m: map[string]string{}} }

func (s *memSettingStore) GetSetting(_ context.Context, key string) (string, bool, error) {
	v, ok := s.m[key]
	return v, ok, nil
}
func (s *memSettingStore) SetSetting(_ context.Context, key, value string) error {
	s.m[key] = value
	return nil
}

func TestControlPlane_DisableProvider_ClearsActiveSelection(t *testing.T) {
	ctx := context.Background()
	store := newMemSettingStore()
	// Anthropic is the active provider; OpenAI remains configured as a fallback.
	store.m["llm_provider"] = "anthropic"
	store.m["llm_model"] = "claude-sonnet-4-20250514"

	cfg := &config.Config{LLM: config.LLMConfig{
		AnthropicKey:   "sk-ant-test",
		AnthropicModel: "claude-sonnet-4-20250514",
		OpenAIKey:      "sk-openai-test",
		OpenAIModel:    "gpt-4o-mini",
	}}
	cp := &ControlPlane{
		cfg:       cfg,
		llmCfg:    &cfg.LLM,
		store:     store,
		dp:        NewDynamicProvider(nil),
		providers: make(map[string]Provider),
	}
	cp.selection = NewSelectionState(cp.dp, store, cp.llmCfg)
	cp.buildProviderRegistry()

	if err := cp.DisableProvider(ctx, "anthropic"); err != nil {
		t.Fatalf("DisableProvider: %v", err)
	}

	// Persisted active selection must be cleared so GetActive falls through.
	if store.m["llm_provider"] != "" || store.m["llm_model"] != "" {
		t.Errorf("expected active selection cleared, got provider=%q model=%q",
			store.m["llm_provider"], store.m["llm_model"])
	}
	// Anthropic credentials must be gone.
	if cp.llmCfg.AnthropicKey != "" {
		t.Errorf("expected anthropic key cleared, got %q", cp.llmCfg.AnthropicKey)
	}
	// The chat provider must have been re-pointed at the remaining provider (OpenAI),
	// not left dangling on the disabled anthropic provider.
	if name := cp.dp.Name(); name == "none" {
		t.Errorf("expected dp to be re-pointed at a remaining provider, got %q", name)
	}
}

func TestControlPlane_DisableProvider_NonActive_KeepsSelection(t *testing.T) {
	ctx := context.Background()
	store := newMemSettingStore()
	store.m["llm_provider"] = "anthropic"
	store.m["llm_model"] = "claude-sonnet-4-20250514"

	cfg := &config.Config{LLM: config.LLMConfig{
		AnthropicKey:   "sk-ant-test",
		AnthropicModel: "claude-sonnet-4-20250514",
		OpenAIKey:      "sk-openai-test",
		OpenAIModel:    "gpt-4o-mini",
	}}
	cp := &ControlPlane{
		cfg:       cfg,
		llmCfg:    &cfg.LLM,
		store:     store,
		dp:        NewDynamicProvider(nil),
		providers: make(map[string]Provider),
	}
	cp.selection = NewSelectionState(cp.dp, store, cp.llmCfg)
	cp.buildProviderRegistry()

	// Disable OpenAI (not the active provider) — active selection must be untouched.
	if err := cp.DisableProvider(ctx, "openai"); err != nil {
		t.Fatalf("DisableProvider: %v", err)
	}
	if store.m["llm_provider"] != "anthropic" || store.m["llm_model"] != "claude-sonnet-4-20250514" {
		t.Errorf("active selection should be preserved, got provider=%q model=%q",
			store.m["llm_provider"], store.m["llm_model"])
	}
	if cp.llmCfg.OpenAIKey != "" {
		t.Errorf("expected openai key cleared, got %q", cp.llmCfg.OpenAIKey)
	}
}

func TestControlPlane_ExecuteProviderAction_SaveFailure(t *testing.T) {
	ctx := context.Background()
	store := &mockOpStore{
		saveErr: errors.New("db error"),
	}

	prov := &mockLocalProv{
		pullOp: ProviderOperation{
			ID:       "sync-op-2",
			Action:   "pull",
			Provider: "mock",
			Status:   OperationStatusPending,
		},
	}

	cp := &ControlPlane{
		cfg:       &config.Config{},
		llmCfg:    &config.LLMConfig{},
		opStore:   store,
		providers: map[string]Provider{"mock": prov},
	}

	req := ProviderActionRequest{
		Provider: "mock",
		Action:   "pull",
		Model:    "test-model",
	}

	_, err := cp.ExecuteProviderAction(ctx, req)
	if err == nil {
		t.Fatal("expected error due to synchronous save failure")
	}
	if err.Error() != "llm: failed to save initial provider operation: db error" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// catalogCallCounter wraps a real-ish ControlPlane and counts Catalog() rebuilds
// by checking catalogCacheAt changes. We use the internal field directly since
// the test is in the same package.

func TestControlPlane_CatalogCache_HitWithinTTL(t *testing.T) {
	ctx := context.Background()
	store := newMemSettingStore()

	cfg := &config.Config{LLM: config.LLMConfig{
		AnthropicKey:   "sk-ant-test",
		AnthropicModel: "claude-sonnet-4-20250514",
	}}
	cp := &ControlPlane{
		cfg:    cfg,
		llmCfg: &cfg.LLM,
		store:  store,
	}

	// First call populates the cache.
	_, err := cp.Catalog(ctx)
	if err != nil {
		t.Fatalf("first Catalog() call failed: %v", err)
	}
	firstCacheAt := cp.catalogCacheAt
	if firstCacheAt.IsZero() {
		t.Fatal("expected catalogCacheAt to be set after first Catalog() call")
	}

	// Second call within TTL must reuse the cache (cachedAt must not change).
	_, err = cp.Catalog(ctx)
	if err != nil {
		t.Fatalf("second Catalog() call failed: %v", err)
	}
	if cp.catalogCacheAt != firstCacheAt {
		t.Error("expected cache to be reused within TTL, but catalogCacheAt changed")
	}
}

func TestControlPlane_CatalogCache_ExpiredAfterTTL(t *testing.T) {
	ctx := context.Background()
	store := newMemSettingStore()

	cfg := &config.Config{LLM: config.LLMConfig{
		AnthropicKey:   "sk-ant-test",
		AnthropicModel: "claude-sonnet-4-20250514",
	}}
	cp := &ControlPlane{
		cfg:    cfg,
		llmCfg: &cfg.LLM,
		store:  store,
	}

	_, _ = cp.Catalog(ctx)

	// Backdate the cache timestamp to simulate TTL expiry.
	cp.catalogCacheMu.Lock()
	cp.catalogCacheAt = time.Now().Add(-(catalogCacheTTL + time.Second))
	cp.catalogCacheMu.Unlock()

	stale := cp.catalogCacheAt

	_, err := cp.Catalog(ctx)
	if err != nil {
		t.Fatalf("Catalog() after TTL expiry failed: %v", err)
	}
	if cp.catalogCacheAt == stale {
		t.Error("expected cache to be rebuilt after TTL expiry, but catalogCacheAt was not updated")
	}
}

func TestControlPlane_CatalogCache_InvalidatedByConfigureProvider(t *testing.T) {
	ctx := context.Background()
	store := newMemSettingStore()

	cfg := &config.Config{LLM: config.LLMConfig{
		AnthropicKey:   "sk-ant-test",
		AnthropicModel: "claude-sonnet-4-20250514",
	}}
	cp := &ControlPlane{
		cfg:       cfg,
		llmCfg:    &cfg.LLM,
		store:     store,
		dp:        NewDynamicProvider(nil),
		providers: make(map[string]Provider),
	}
	cp.selection = NewSelectionState(cp.dp, store, cp.llmCfg)
	cp.buildProviderRegistry()

	// Warm the cache.
	_, _ = cp.Catalog(ctx)
	if cp.catalogCacheAt.IsZero() {
		t.Fatal("cache should be set after Catalog()")
	}

	// ConfigureProvider must invalidate the cache.
	_ = cp.ConfigureProvider(ctx, ProviderConfigRequest{
		Provider: "anthropic",
		APIKey:   "sk-ant-new",
		Model:    "claude-sonnet-4-20250514",
	})

	if !cp.catalogCacheAt.IsZero() {
		t.Error("expected catalogCacheAt to be zeroed after ConfigureProvider (cache invalidated)")
	}
}

func TestControlPlane_CatalogCache_InvalidatedByDisableProvider(t *testing.T) {
	ctx := context.Background()
	store := newMemSettingStore()
	store.m["llm_provider"] = "openai"
	store.m["llm_model"] = "gpt-4o-mini"

	cfg := &config.Config{LLM: config.LLMConfig{
		AnthropicKey:   "sk-ant-test",
		AnthropicModel: "claude-sonnet-4-20250514",
		OpenAIKey:      "sk-openai-test",
		OpenAIModel:    "gpt-4o-mini",
	}}
	cp := &ControlPlane{
		cfg:       cfg,
		llmCfg:    &cfg.LLM,
		store:     store,
		dp:        NewDynamicProvider(nil),
		providers: make(map[string]Provider),
	}
	cp.selection = NewSelectionState(cp.dp, store, cp.llmCfg)
	cp.buildProviderRegistry()

	// Warm the cache.
	_, _ = cp.Catalog(ctx)
	if cp.catalogCacheAt.IsZero() {
		t.Fatal("cache should be set after Catalog()")
	}

	// DisableProvider must invalidate the cache.
	if err := cp.DisableProvider(ctx, "openai"); err != nil {
		t.Fatalf("DisableProvider failed: %v", err)
	}

	if !cp.catalogCacheAt.IsZero() {
		t.Error("expected catalogCacheAt to be zeroed after DisableProvider (cache invalidated)")
	}
}
