//go:build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/gateway"
	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/store"
	"github.com/stretchr/testify/require"
)

func integrationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	require.NoError(t, err)
	require.NoError(t, store.CreateTables(context.Background(), db))
	return db
}

func createScopedKey(t *testing.T, db *sql.DB, id string, scopes []string) string {
	t.Helper()
	raw, hash, err := store.GenerateAPIKey()
	require.NoError(t, err)
	require.NoError(t, store.CreateAPIKey(context.Background(), db, store.APIKey{
		ID:        id,
		OwnerID:   "integration-owner",
		KeyHash:   hash,
		Name:      id,
		Scopes:    scopes,
		CreatedAt: time.Now().UTC(),
	}))
	return raw
}

func doIntegrationReq(t *testing.T, srv *gateway.Server, method, path, apiKey string, body any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = "192.168.1.10:9999"
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec.Result()
}

func TestIntegrationLLMEndpoints_RespectScopesAndState(t *testing.T) {
	db := integrationDB(t)

	var (
		mu       sync.Mutex
		provider = "ollama"
		model    = "llama3:latest"
	)

	srv := gateway.NewServer(gateway.Config{
		DB: db,
		ListLLMs: func(context.Context) (llm.LLMCatalog, error) {
			return llm.LLMCatalog{
				Providers: []llm.LLMProviderInfo{
					{
						Key:         "ollama",
						DisplayName: "Ollama",
						Models: []llm.LLMModelInfo{
							{Name: "llama3:latest"},
							{Name: "mistral:latest"},
						},
					},
				},
			}, nil
		},
		ListLLMProfiles: func(context.Context) ([]llm.ModelProfile, error) {
			return []llm.ModelProfile{
				{ProviderKey: "ollama", ModelID: "llama3:latest", DisplayName: "llama3:latest", ChatScore: 72},
			}, nil
		},
		GetLLMProfiles: func(context.Context) (any, error) {
			return []llm.ModelProfile{
				{ProviderKey: "ollama", ModelID: "llama3:latest", ChatScore: 70},
				{ProviderKey: "ollama", ModelID: "mistral:latest", ChatScore: 72},
			}, nil
		},
		GetLLMRoutingPreferences: func(context.Context) (any, error) {
			return llm.ModelPreferences{PreferLocal: true}, nil
		},
		PatchLLMRoutingPreferences: func(_ context.Context, patch map[string]any) (any, error) {
			return patch, nil
		},
		GetActiveLLM: func(context.Context) (string, string, error) {
			mu.Lock()
			defer mu.Unlock()
			return provider, model, nil
		},
		SetActiveLLM: func(_ context.Context, nextProvider, nextModel string) (string, string, error) {
			mu.Lock()
			defer mu.Unlock()
			provider = nextProvider
			model = nextModel
			return provider, model, nil
		},
	})

	readKey := createScopedKey(t, db, "read-key", []string{"read"})
	execKey := createScopedKey(t, db, "exec-key", []string{"execute"})

	res := doIntegrationReq(t, srv, http.MethodGet, "/api/llm/catalog", readKey, nil)
	require.Equal(t, http.StatusOK, res.StatusCode)
	var catalog llm.LLMCatalog
	require.NoError(t, json.NewDecoder(res.Body).Decode(&catalog))
	require.Len(t, catalog.Providers, 1)

	res = doIntegrationReq(t, srv, http.MethodGet, "/api/llm/profiles", readKey, nil)
	require.Equal(t, http.StatusOK, res.StatusCode)
	var profiles map[string][]llm.ModelProfile
	require.NoError(t, json.NewDecoder(res.Body).Decode(&profiles))
	require.Len(t, profiles["profiles"], 1)

	res = doIntegrationReq(t, srv, http.MethodGet, "/api/llm/active", readKey, nil)
	require.Equal(t, http.StatusOK, res.StatusCode)
	var active map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&active))
	require.Equal(t, "ollama", active["provider"])
	require.Equal(t, "llama3:latest", active["model"])

	res = doIntegrationReq(t, srv, http.MethodPut, "/api/llm/active", readKey, map[string]string{
		"provider": "ollama",
		"model":    "mistral:latest",
	})
	require.Equal(t, http.StatusForbidden, res.StatusCode)

	res = doIntegrationReq(t, srv, http.MethodPut, "/api/llm/active", execKey, map[string]string{
		"provider": "ollama",
		"model":    "mistral:latest",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	res = doIntegrationReq(t, srv, http.MethodGet, "/api/llm/active", readKey, nil)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.NoError(t, json.NewDecoder(res.Body).Decode(&active))
	require.Equal(t, "mistral:latest", active["model"])

	res = doIntegrationReq(t, srv, http.MethodGet, "/api/llm/profiles", readKey, nil)
	require.Equal(t, http.StatusOK, res.StatusCode)

	res = doIntegrationReq(t, srv, http.MethodGet, "/api/llm/preferences", readKey, nil)
	require.Equal(t, http.StatusOK, res.StatusCode)

	res = doIntegrationReq(t, srv, http.MethodPatch, "/api/llm/preferences", execKey, map[string]any{
		"prefer_local": false,
	})
	require.Equal(t, http.StatusOK, res.StatusCode)
}
