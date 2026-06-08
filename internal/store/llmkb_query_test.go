package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/llmkb"
)

func TestSQLiteLLMKBRepo_QuerySurface_UsesTypedGovernedData(t *testing.T) {
	ctx := context.Background()
	db, err := openTestSQLiteDB(t)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}

	repo := NewSQLiteLLMKBRepo(db)
	seedDir := filepath.Join("..", "..", "data", "seed", "models")
	if err := repo.LoadLLMSeeds(ctx, seedDir, nil); err != nil {
		t.Fatalf("LoadLLMSeeds: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	for _, instance := range []llmkb.LLMRuntimeInstance{
		{
			InstanceID:        "anthropic-runtime-1",
			LLMID:             "anthropic.claude-3-5-sonnet",
			ProviderID:        "anthropic",
			RuntimeBackend:    llmkb.RuntimeBackendAnthropic,
			LoadedStatus:      llmkb.LoadedStatusLoaded,
			LastProbeAt:       &now,
		},
		{
			InstanceID:        "ollama-runtime-1",
			LLMID:             "meta.llama-3-2-3b",
			ProviderID:        "ollama",
			RuntimeBackend:    llmkb.RuntimeBackendOllama,
			LoadedStatus:      llmkb.LoadedStatusLoaded,
			LastProbeAt:       &now,
		},
		{
			InstanceID:        "anthropic-runtime-2",
			LLMID:             "anthropic.claude-haiku-3",
			ProviderID:        "anthropic",
			RuntimeBackend:    llmkb.RuntimeBackendAnthropic,
			LastProbeAt:       &now,
		},
	} {
		if err := repo.SaveRuntimeInstance(ctx, instance); err != nil {
			t.Fatalf("SaveRuntimeInstance(%s): %v", instance.InstanceID, err)
		}
	}

	byAlias, err := repo.GetProfileByAlias(ctx, "claude-3.5-sonnet")
	if err != nil {
		t.Fatalf("GetProfileByAlias: %v", err)
	}
	if byAlias == nil || byAlias.LLMID != "anthropic.claude-3-5-sonnet" {
		t.Fatalf("expected sonnet alias lookup, got %+v", byAlias)
	}

	anthropicProfiles, err := repo.ListProfilesByProviderID(ctx, "anthropic")
	if err != nil {
		t.Fatalf("ListProfilesByProviderID: %v", err)
	}
	if len(anthropicProfiles) != 2 {
		t.Fatalf("expected 2 anthropic profiles, got %d", len(anthropicProfiles))
	}

	codingProfiles, err := repo.ListPreferredForTask(ctx, llmkb.TaskClassCoding)
	if err != nil {
		t.Fatalf("ListPreferredForTask(coding): %v", err)
	}
	if len(codingProfiles) != 1 || codingProfiles[0].LLMID != "anthropic.claude-3-5-sonnet" {
		t.Fatalf("expected sonnet coding preference, got %+v", codingProfiles)
	}
	chatProfiles, err := repo.ListPreferredForTask(ctx, llmkb.TaskClassChat)
	if err != nil {
		t.Fatalf("ListPreferredForTask(chat): %v", err)
	}
	if len(chatProfiles) != 2 {
		t.Fatalf("expected 2 chat-preferred profiles, got %+v", chatProfiles)
	}

	routing, err := repo.GetRoutingProfile(ctx, "anthropic.claude-haiku-3")
	if err != nil {
		t.Fatalf("GetRoutingProfile: %v", err)
	}
	if routing == nil || routing.CostTier != llmkb.CostTierCheap || routing.AutonomyCeiling != llmkb.AutonomyLevelChat {
		t.Fatalf("unexpected haiku routing profile: %+v", routing)
	}

	sonnetRuntimes, err := repo.ListRuntimeInstances(ctx, "anthropic.claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("ListRuntimeInstances filtered: %v", err)
	}
	if len(sonnetRuntimes) != 1 || sonnetRuntimes[0].InstanceID != "anthropic-runtime-1" {
		t.Fatalf("unexpected filtered runtimes: %+v", sonnetRuntimes)
	}

	available, err := repo.ListAvailableRuntimes(ctx)
	if err != nil {
		t.Fatalf("ListAvailableRuntimes: %v", err)
	}
	if len(available) != 2 {
		t.Fatalf("expected 2 available runtimes, got %d", len(available))
	}

	description, err := repo.DescribeProfile(ctx, "meta.llama-3-2-3b")
	if err != nil {
		t.Fatalf("DescribeProfile: %v", err)
	}
	for _, fragment := range []string{"meta.llama-3-2-3b", "provider Meta", "hosting local", "cost free", "autonomy assistive"} {
		if !strings.Contains(description, fragment) {
			t.Fatalf("expected description to contain %q, got %q", fragment, description)
		}
	}
}

func TestSQLiteLLMKBRepo_RefreshRuntimeInstancesFromCatalog_ResolvesExistingProfileIDs(t *testing.T) {
	ctx := context.Background()
	db, err := openTestSQLiteDB(t)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}

	repo := NewSQLiteLLMKBRepo(db)
	seedDir := filepath.Join("..", "..", "data", "seed", "models")
	if err := repo.LoadLLMSeeds(ctx, seedDir, nil); err != nil {
		t.Fatalf("LoadLLMSeeds: %v", err)
	}

	catalog := llm.LLMCatalog{
		Providers: []llm.LLMProviderInfo{
			{
				Key:         "anthropic",
				DisplayName: "Anthropic",
				Models: []llm.LLMModelInfo{
					{Name: "claude-3.5-sonnet"},
				},
			},
			{
				Key:         "ollama",
				DisplayName: "Ollama",
				Models: []llm.LLMModelInfo{
					{Name: "llama-3.2-3b"},
				},
			},
		},
	}

	if err := repo.RefreshRuntimeInstancesFromCatalog(ctx, catalog); err != nil {
		t.Fatalf("RefreshRuntimeInstancesFromCatalog: %v", err)
	}

	sonnetInstance, err := repo.GetRuntimeInstance(ctx, "anthropic:claude-3.5-sonnet")
	if err != nil {
		t.Fatalf("GetRuntimeInstance(sonnet): %v", err)
	}
	if sonnetInstance == nil || sonnetInstance.LLMID != "anthropic.claude-3-5-sonnet" {
		t.Fatalf("expected sonnet runtime to resolve to seeded profile, got %+v", sonnetInstance)
	}

	llamaInstance, err := repo.GetRuntimeInstance(ctx, "ollama:llama-3.2-3b")
	if err != nil {
		t.Fatalf("GetRuntimeInstance(llama): %v", err)
	}
	if llamaInstance == nil || llamaInstance.LLMID != "meta.llama-3-2-3b" {
		t.Fatalf("expected llama runtime to resolve to seeded profile, got %+v", llamaInstance)
	}
}
