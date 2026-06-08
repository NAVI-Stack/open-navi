package store

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/llmkb"
)

func TestSQLiteLLMKBRepo_LoadLLMSeeds_IdempotentAndPreservesLiveState(t *testing.T) {
	ctx := context.Background()
	db, err := openTestSQLiteDB(t)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	dir := t.TempDir()
	writeSeedFile(t, dir, "anthropic.claude-sonnet.yaml", `
kind: LLMProfile
schema_version: 1
llm_id: anthropic.claude-sonnet
canonical_name: Claude Sonnet
aliases: [claude-sonnet]
identity:
  provider: anthropic
  publisher: Anthropic
  release_channel: stable
  hosting_modes_supported: [api]
capabilities:
  primary_use_cases: [chat, coding]
  interaction_modes: [conversational, tool_using]
  agentic_class: capable
  coding_class: strong
  reasoning_class: strong
  instruction_following_class: competent
  tool_discipline_class: competent
  schema_reliability_class: strong
  multimodal_class: fair
technical_profile:
  supports_tools: true
  supports_parallel_tools: false
  supports_json_schema: true
  supports_streaming: true
  supports_vision: true
  supports_audio_in: false
  supports_audio_out: false
  max_output_tokens: 4096
  context_window:
    input_tokens: 200000
routing_profile:
  preferred_for: [coding, reasoning]
  cost_tier: expensive
  latency_tier: medium
  max_risk_tier_allowed: medium
  autonomy_ceiling: assistive
`)

	repo := NewSQLiteLLMKBRepo(db)
	if err := repo.LoadLLMSeeds(ctx, dir, slog.Default()); err != nil {
		t.Fatalf("LoadLLMSeeds first load: %v", err)
	}
	profile, err := repo.GetProfile(ctx, "anthropic.claude-sonnet")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if profile == nil || profile.SchemaVersion != 1 {
		t.Fatalf("expected schema_version=1 profile, got %+v", profile)
	}
	if profile.Provenance.Source != llmkb.ProvenanceSourceCuratedSeed || profile.Provenance.Confidence != 0.9 {
		t.Fatalf("expected curated seed provenance, got %+v", profile.Provenance)
	}
	if !strings.HasSuffix(profile.Provenance.SourceDetail, "anthropic.claude-sonnet.yaml") {
		t.Fatalf("expected source detail path, got %q", profile.Provenance.SourceDetail)
	}

	profile.OperationalState = llmkb.OperationalState{
		AvailabilityState: llmkb.AvailabilityStateDegraded,
		RuntimeBackend:    llmkb.RuntimeBackendAnthropic,
		HealthStatus:      llmkb.HealthStatusDegraded,
	}
	profile.UsageStats = llmkb.UsageStats{TotalExecutions: 11, SuccessRate: 0.75}
	if err := repo.SaveProfile(ctx, *profile); err != nil {
		t.Fatalf("SaveProfile live state: %v", err)
	}

	if err := repo.LoadLLMSeeds(ctx, dir, slog.Default()); err != nil {
		t.Fatalf("LoadLLMSeeds second load: %v", err)
	}
	got, err := repo.GetProfile(ctx, "anthropic.claude-sonnet")
	if err != nil {
		t.Fatalf("GetProfile second load: %v", err)
	}
	if got.OperationalState.HealthStatus != llmkb.HealthStatusDegraded {
		t.Fatalf("expected operational state to survive seed reload, got %+v", got.OperationalState)
	}
	if got.UsageStats.TotalExecutions != 11 {
		t.Fatalf("expected usage stats to survive seed reload, got %+v", got.UsageStats)
	}
	if got.Provenance.Source != llmkb.ProvenanceSourceCuratedSeed || got.Provenance.Confidence != 0.9 {
		t.Fatalf("expected curated provenance after reload, got %+v", got.Provenance)
	}
	profiles, err := repo.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected idempotent profile count, got %d", len(profiles))
	}
	if len(got.Provenance.MutationHistory) != 1 {
		t.Fatalf("expected 1 mutation history entry (creation), got %+v", got.Provenance.MutationHistory)
	}
}

func TestSQLiteLLMKBRepo_LoadLLMSeeds_SkipsGovernedFieldsFromOlderSeed(t *testing.T) {
	ctx := context.Background()
	db, err := openTestSQLiteDB(t)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	dir := t.TempDir()
	writeSeedFile(t, dir, "openai.gpt-4o.yaml", `
kind: LLMProfile
schema_version: 1
llm_id: openai.gpt-4o
canonical_name: GPT-4o
identity:
  provider: openai
capabilities:
  primary_use_cases: [chat]
  interaction_modes: [conversational]
  agentic_class: capable
  coding_class: competent
  reasoning_class: strong
  instruction_following_class: strong
  tool_discipline_class: competent
  schema_reliability_class: strong
  multimodal_class: strong
technical_profile:
  supports_tools: true
routing_profile:
  preferred_for: [chat]
  cost_tier: moderate
  latency_tier: low
  max_risk_tier_allowed: low
  autonomy_ceiling: chat
`)

	repo := NewSQLiteLLMKBRepo(db)
	existing := llmkb.LLMProfile{
		SchemaVersion: 2,
		LLMID:         "openai.gpt-4o",
		CanonicalName: "Old GPT-4o Name",
		ProviderID:    "openai",
		Capabilities: llmkb.CapabilityProfile{
			AgenticClass:              llmkb.AgenticClassStrong,
			CodingClass:               llmkb.CodingClassElite,
			ReasoningClass:            llmkb.ReasoningClassExceptional,
			InstructionFollowingClass: llmkb.ClassLevelStrong,
			ToolDisciplineClass:       llmkb.ClassLevelStrong,
			SchemaReliabilityClass:    llmkb.ClassLevelStrong,
			MultimodalClass:           llmkb.ClassLevelStrong,
			PrimaryUseCases:           []llmkb.UseCase{llmkb.UseCaseResearch},
			InteractionModes:          []llmkb.InteractionMode{llmkb.InteractionModeToolUsing},
		},
		Features: llmkb.TechnicalFeatures{SupportsTools: true},
		Routing: llmkb.RoutingProfile{
			PreferredFor:       []llmkb.TaskClass{llmkb.TaskClassReasoning},
			CostTier:           llmkb.CostTierExpensive,
			LatencyTier:        llmkb.LatencyTierHigh,
			MaxRiskTierAllowed: llmkb.RiskTierHigh,
			AutonomyCeiling:    llmkb.AutonomyLevelAutonomous,
		},
		Provenance: llmkb.Provenance{
			Source:       llmkb.ProvenanceSourceCuratedSeed,
			SourceDetail: "existing",
			Confidence:   0.9,
			AssertedAt:   parseTestTime(t, "2026-03-23T00:00:00Z"),
			FieldClass:   llmkb.FieldClassGoverned,
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.SaveProvider(ctx, llmkb.LLMProvider{
		ProviderID:    "openai",
		CanonicalName: "OpenAI",
		Provenance:    existing.Provenance,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveProvider existing: %v", err)
	}
	if err := repo.SaveProfile(ctx, existing); err != nil {
		t.Fatalf("SaveProfile existing: %v", err)
	}

	if err := repo.LoadLLMSeeds(ctx, dir, slog.Default()); err != nil {
		t.Fatalf("LoadLLMSeeds: %v", err)
	}
	got, err := repo.GetProfile(ctx, "openai.gpt-4o")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.SchemaVersion != 2 {
		t.Fatalf("expected existing schema version to remain, got %d", got.SchemaVersion)
	}
	if got.Routing.CostTier != llmkb.CostTierExpensive || got.Routing.AutonomyCeiling != llmkb.AutonomyLevelAutonomous {
		t.Fatalf("expected governed routing profile to be preserved, got %+v", got.Routing)
	}
	if got.CanonicalName != "GPT-4o" {
		t.Fatalf("expected observed canonical_name to update from seed, got %q", got.CanonicalName)
	}
}

func TestSQLiteLLMKBRepo_LoadLLMSeeds_ValidationErrorIncludesFieldAndValue(t *testing.T) {
	ctx := context.Background()
	db, err := openTestSQLiteDB(t)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	dir := t.TempDir()
	writeSeedFile(t, dir, "bad.yaml", `
kind: LLMProfile
schema_version: 1
llm_id: bad.model
canonical_name: Bad Model
identity:
  provider: anthropic
capabilities:
  primary_use_cases: [chat]
  interaction_modes: [conversational]
  agentic_class: impossible
  coding_class: strong
  reasoning_class: strong
  instruction_following_class: strong
  tool_discipline_class: strong
  schema_reliability_class: strong
  multimodal_class: fair
`)

	repo := NewSQLiteLLMKBRepo(db)
	err = repo.LoadLLMSeeds(ctx, dir, slog.Default())
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bad.yaml") || !strings.Contains(msg, "capabilities.agentic_class") || !strings.Contains(msg, "impossible") {
		t.Fatalf("expected detailed validation error, got %q", msg)
	}
}

func TestSQLiteLLMKBRepo_LoadLLMSeeds_RejectsUnknownSchemaVersion(t *testing.T) {
	ctx := context.Background()
	db, err := openTestSQLiteDB(t)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	dir := t.TempDir()
	writeSeedFile(t, dir, "bad-version.yaml", `
kind: LLMProfile
schema_version: 99
llm_id: bad.model
canonical_name: Bad Model
identity:
  provider: anthropic
capabilities:
  primary_use_cases: [chat]
  interaction_modes: [conversational]
  agentic_class: capable
  coding_class: strong
  reasoning_class: strong
  instruction_following_class: strong
  tool_discipline_class: strong
  schema_reliability_class: strong
  multimodal_class: fair
`)

	repo := NewSQLiteLLMKBRepo(db)
	err = repo.LoadLLMSeeds(ctx, dir, slog.Default())
	if err == nil || !strings.Contains(err.Error(), "schema_version") || !strings.Contains(err.Error(), "99") {
		t.Fatalf("expected schema_version validation error, got %v", err)
	}
}

func TestSQLiteLLMKBRepo_LoadLLMSeeds_RejectsUnexpectedFields(t *testing.T) {
	ctx := context.Background()
	db, err := openTestSQLiteDB(t)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	dir := t.TempDir()
	writeSeedFile(t, dir, "extra-field.yaml", `
kind: LLMProfile
schema_version: 1
llm_id: bad.model
canonical_name: Bad Model
extra_field: true
identity:
  provider: anthropic
capabilities:
  primary_use_cases: [chat]
  interaction_modes: [conversational]
  agentic_class: capable
  coding_class: strong
  reasoning_class: strong
  instruction_following_class: strong
  tool_discipline_class: strong
  schema_reliability_class: strong
  multimodal_class: fair
`)

	repo := NewSQLiteLLMKBRepo(db)
	err = repo.LoadLLMSeeds(ctx, dir, slog.Default())
	if err == nil || !strings.Contains(err.Error(), "extra_field") {
		t.Fatalf("expected unknown-field validation error, got %v", err)
	}
}

func TestSQLiteLLMKBRepo_LoadLLMSeeds_UpgradesGovernedFieldsOnVersionBump(t *testing.T) {
	ctx := context.Background()
	db, err := openTestSQLiteDB(t)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	dir := t.TempDir()
	writeSeedFile(t, dir, "anthropic.claude-sonnet.yaml", `
kind: LLMProfile
schema_version: 1
llm_id: anthropic.claude-sonnet
canonical_name: Claude Sonnet V2
aliases: [claude-sonnet-v2]
identity:
  provider: anthropic
capabilities:
  primary_use_cases: [chat, coding]
  interaction_modes: [conversational, tool_using]
  agentic_class: strong
  coding_class: elite
  reasoning_class: exceptional
  instruction_following_class: strong
  tool_discipline_class: strong
  schema_reliability_class: strong
  multimodal_class: good
technical_profile:
  supports_tools: true
routing_profile:
  preferred_for: [agentic]
  cost_tier: expensive
  latency_tier: medium
  max_risk_tier_allowed: high
  autonomy_ceiling: autonomous
`)

	repo := NewSQLiteLLMKBRepo(db)
	existing := llmkb.LLMProfile{
		SchemaVersion: 0,
		LLMID:         "anthropic.claude-sonnet",
		CanonicalName: "Claude Sonnet",
		ProviderID:    "anthropic",
		Capabilities: llmkb.CapabilityProfile{
			AgenticClass:              llmkb.AgenticClassCapable,
			CodingClass:               llmkb.CodingClassStrong,
			ReasoningClass:            llmkb.ReasoningClassStrong,
			InstructionFollowingClass: llmkb.ClassLevelGood,
			ToolDisciplineClass:       llmkb.ClassLevelGood,
			SchemaReliabilityClass:    llmkb.ClassLevelGood,
			MultimodalClass:           llmkb.ClassLevelFair,
			PrimaryUseCases:           []llmkb.UseCase{llmkb.UseCaseGeneralChat},
			InteractionModes:          []llmkb.InteractionMode{llmkb.InteractionModeConversational},
		},
		Routing: llmkb.RoutingProfile{
			PreferredFor:       []llmkb.TaskClass{llmkb.TaskClassChat},
			CostTier:           llmkb.CostTierCheap,
			LatencyTier:        llmkb.LatencyTierLow,
			MaxRiskTierAllowed: llmkb.RiskTierLow,
			AutonomyCeiling:    llmkb.AutonomyLevelChat,
		},
		Provenance: llmkb.Provenance{
			Source:          llmkb.ProvenanceSourceCuratedSeed,
			SourceDetail:    "existing",
			Confidence:      0.9,
			AssertedAt:      parseTestTime(t, "2026-03-23T00:00:00Z"),
			FieldClass:      llmkb.FieldClassGoverned,
			MutationHistory: []llmkb.MutationRecord{{FieldName: "seed_load", NewValue: "schema_version=0", ChangedAt: parseTestTime(t, "2026-03-23T00:00:00Z"), Source: llmkb.ProvenanceSourceCuratedSeed}},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.SaveProvider(ctx, llmkb.LLMProvider{
		ProviderID:    "anthropic",
		CanonicalName: "Anthropic",
		Provenance:    existing.Provenance,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveProvider existing: %v", err)
	}
	if err := repo.SaveProfile(ctx, existing); err != nil {
		t.Fatalf("SaveProfile existing: %v", err)
	}

	if err := repo.LoadLLMSeeds(ctx, dir, slog.Default()); err != nil {
		t.Fatalf("LoadLLMSeeds: %v", err)
	}
	got, err := repo.GetProfile(ctx, "anthropic.claude-sonnet")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.SchemaVersion != 1 || got.Routing.AutonomyCeiling != llmkb.AutonomyLevelAutonomous || got.Routing.MaxRiskTierAllowed != llmkb.RiskTierHigh {
		t.Fatalf("expected governed fields to upgrade on version bump, got %+v", got.Routing)
	}
	if got.Capabilities.AgenticClass != llmkb.AgenticClassStrong || len(got.Provenance.MutationHistory) < 1 {
		t.Fatalf("expected upgraded capability and at least one history entry, got %+v", got)
	}
}

func TestSQLiteLLMKBRepo_LoadLLMSeeds_ActualSeedFixtures(t *testing.T) {
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
	if err := repo.LoadLLMSeeds(ctx, seedDir, slog.Default()); err != nil {
		t.Fatalf("LoadLLMSeeds first fixture load: %v", err)
	}
	if err := repo.LoadLLMSeeds(ctx, seedDir, slog.Default()); err != nil {
		t.Fatalf("LoadLLMSeeds second fixture load: %v", err)
	}

	profiles, err := repo.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(profiles) != 3 {
		t.Fatalf("expected 3 seeded profiles, got %d", len(profiles))
	}
	index := map[string]llmkb.LLMProfile{}
	for _, profile := range profiles {
		index[profile.LLMID] = profile
	}

	sonnet := index["anthropic.claude-3-5-sonnet"]
	if sonnet.Routing.CostTier != llmkb.CostTierExpensive || sonnet.Routing.AutonomyCeiling != llmkb.AutonomyLevelAutonomous || !sonnet.Features.SupportsVision {
		t.Fatalf("unexpected sonnet profile: %+v", sonnet)
	}
	haiku := index["anthropic.claude-haiku-3"]
	if haiku.Routing.CostTier != llmkb.CostTierCheap || haiku.Routing.AutonomyCeiling != llmkb.AutonomyLevelChat || haiku.Capabilities.AgenticClass != llmkb.AgenticClassConstrained {
		t.Fatalf("unexpected haiku profile: %+v", haiku)
	}
	llama := index["meta.llama-3-2-3b"]
	if llama.Routing.CostTier != llmkb.CostTierFree || llama.Routing.AutonomyCeiling != llmkb.AutonomyLevelAssistive || llama.Capabilities.AgenticClass != llmkb.AgenticClassNone {
		t.Fatalf("unexpected llama profile: %+v", llama)
	}

	ollamaProvider, err := repo.GetProvider(ctx, "ollama")
	if err != nil {
		t.Fatalf("GetProvider ollama: %v", err)
	}
	if ollamaProvider == nil {
		t.Fatalf("unexpected ollama provider missing")
	}
}

func writeSeedFile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)), 0o644); err != nil {
		t.Fatalf("write seed file %s: %v", path, err)
	}
}

func parseTestTime(t *testing.T, raw string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("parse test time: %v", err)
	}
	return parsed
}

func openTestSQLiteDB(t *testing.T) (*sql.DB, error) {
	t.Helper()
	return Open(filepath.Join(t.TempDir(), "llmkb-seed-loader.db"))
}
