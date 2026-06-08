package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/store"
)

func openTelemetryKBTestRepo(t *testing.T) *store.SQLiteLLMKBRepo {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	repo := store.NewSQLiteLLMKBRepo(db)
	seedDir := filepath.Join("..", "..", "data", "seed", "models")
	if err := repo.LoadLLMSeeds(ctx, seedDir, nil); err != nil {
		t.Fatalf("LoadLLMSeeds: %v", err)
	}
	return repo
}

func TestEnsureKBProfileExists_ResolvesSeededAlias(t *testing.T) {
	ctx := context.Background()
	repo := openTelemetryKBTestRepo(t)

	got := ensureKBProfileExists(ctx, repo, "anthropic", "claude-3.5-sonnet")
	if got != "anthropic.claude-3-5-sonnet" {
		t.Fatalf("expected seeded sonnet profile ID, got %q", got)
	}

	profiles, err := repo.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(profiles) != 3 {
		t.Fatalf("expected seed-backed resolution without extra stub profiles, got %d profiles", len(profiles))
	}
}

func TestBuildRoutingProfilesFromCatalogAndKB_UsesLiveModelIDs(t *testing.T) {
	ctx := context.Background()
	repo := openTelemetryKBTestRepo(t)
	catalog := llm.LLMCatalog{
		Providers: []llm.LLMProviderInfo{
			{
				Key:         "anthropic",
				DisplayName: "Anthropic",
				Models: []llm.LLMModelInfo{
					{Name: "claude-3.5-sonnet"},
					{Name: "claude-haiku-3"},
				},
			},
		},
	}

	profiles := buildRoutingProfilesFromCatalogAndKB(ctx, repo, catalog)
	if len(profiles) != 2 {
		t.Fatalf("expected 2 routing profiles, got %d", len(profiles))
	}

	var sonnet llm.ModelProfile
	found := false
	for _, profile := range profiles {
		if profile.ProviderKey == "anthropic" && profile.ModelID == "claude-3.5-sonnet" {
			sonnet = profile
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected live anthropic/claude-3.5-sonnet routing profile, got %+v", profiles)
	}
	if sonnet.DisplayName != "Claude 3.5 Sonnet" {
		t.Fatalf("expected KB display name overlay, got %q", sonnet.DisplayName)
	}
	if !sonnet.SupportsTools || !sonnet.SupportsStream {
		t.Fatalf("expected KB technical features overlay, got %+v", sonnet)
	}
	if sonnet.MaxContextTokens != 200000 {
		t.Fatalf("expected KB context window overlay, got %d", sonnet.MaxContextTokens)
	}
	if sonnet.CostScore != 20 || sonnet.SpeedScore != 70 {
		t.Fatalf("expected KB routing tiers to influence speed/cost, got %+v", sonnet)
	}
}
