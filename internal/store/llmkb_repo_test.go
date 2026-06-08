package store

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/llmkb"
)

func seedTestLLMProfile(t *testing.T, ctx context.Context, repo *SQLiteLLMKBRepo, llmID string, tech llmkb.TechnicalFeatures) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	prov := llmkb.Provenance{
		Source:       llmkb.ProvenanceSourceCuratedSeed,
		AssertedBy:   "unit-test",
		SourceDetail: "unit-test",
		Confidence:   1,
		AssertedAt:   now,
		FieldClass:   llmkb.FieldClassObserved,
	}
	provider := llmkb.LLMProvider{
		ProviderID:    "test",
		CanonicalName: "Test Provider",
		EndpointBase:  "https://example.com",
		Provenance:    prov,
		CreatedAt:     now,
	}
	if err := repo.SaveProvider(ctx, provider); err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	profile := llmkb.LLMProfile{
		SchemaVersion: 1,
		LLMID:         llmID,
		CanonicalName: "test/" + llmID,
		ProviderID:    "test",
		Capabilities: llmkb.CapabilityProfile{
			AgenticClass:              llmkb.AgenticClassCapable,
			CodingClass:               llmkb.CodingClassStrong,
			ReasoningClass:            llmkb.ReasoningClassStrong,
			InstructionFollowingClass: llmkb.ClassLevelStrong,
			ToolDisciplineClass:       llmkb.ClassLevelGood,
			SchemaReliabilityClass:    llmkb.ClassLevelStrong,
			MultimodalClass:           llmkb.ClassLevelFair,
			PrimaryUseCases:           []llmkb.UseCase{llmkb.UseCaseCodingAssistant},
			InteractionModes:          []llmkb.InteractionMode{llmkb.InteractionModeToolUsing},
		},
		Features: tech,
		OperationalState: llmkb.OperationalState{
			AvailabilityState: llmkb.AvailabilityStateAvailable,
			RuntimeBackend:    llmkb.RuntimeBackendOpenAI,
			HealthStatus:      llmkb.HealthStatusHealthy,
		},
		Routing: llmkb.RoutingProfile{
			PreferredFor:       []llmkb.TaskClass{llmkb.TaskClassChat},
			CostTier:           llmkb.CostTierCheap,
			LatencyTier:        llmkb.LatencyTierMedium,
			MaxRiskTierAllowed: llmkb.RiskTierLow,
			AutonomyCeiling:    llmkb.AutonomyLevelAssistive,
		},
		Provenance: prov,
		CreatedAt:  now,
	}
	if err := repo.SaveProfile(ctx, profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
}

func TestSQLiteLLMKBRepo_RoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}

	repo := NewSQLiteLLMKBRepo(db)
	now := time.Now().UTC().Truncate(time.Second)
	provenance := llmkb.Provenance{
		Source:       llmkb.ProvenanceSourceCuratedSeed,
		AssertedBy:   "seed://anthropic/claude-sonnet",
		SourceDetail: "data/seed/models/anthropic.claude-sonnet.yaml",
		Confidence:   0.9,
		AssertedAt:   now,
		FieldClass:   llmkb.FieldClassObserved,
	}
	provider := llmkb.LLMProvider{
		ProviderID:    "anthropic",
		CanonicalName: "Anthropic",
		EndpointBase:  "https://api.anthropic.com",
		Provenance:    provenance,
		CreatedAt:     now,
	}
	if err := repo.SaveProvider(ctx, provider); err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}

	profile := llmkb.LLMProfile{
		SchemaVersion: 1,
		LLMID:         "anthropic.claude-sonnet",
		CanonicalName: "anthropic/claude-sonnet",
		Aliases:       []string{"claude-sonnet", "sonnet"},
		ProviderID:    provider.ProviderID,
		Capabilities: llmkb.CapabilityProfile{
			AgenticClass:              llmkb.AgenticClassCapable,
			CodingClass:               llmkb.CodingClassStrong,
			ReasoningClass:            llmkb.ReasoningClassStrong,
			InstructionFollowingClass: llmkb.ClassLevelStrong,
			ToolDisciplineClass:       llmkb.ClassLevelGood,
			SchemaReliabilityClass:    llmkb.ClassLevelStrong,
			MultimodalClass:           llmkb.ClassLevelFair,
			PrimaryUseCases:           []llmkb.UseCase{llmkb.UseCaseCodingAssistant},
			InteractionModes:          []llmkb.InteractionMode{llmkb.InteractionModeToolUsing},
		},
		Features: llmkb.TechnicalFeatures{SupportsTools: true, SupportsStreaming: true, ContextWindowTokens: 200000},
		OperationalState: llmkb.OperationalState{
			AvailabilityState:   llmkb.AvailabilityStateAvailable,
			RuntimeBackend:      llmkb.RuntimeBackendAnthropic,
			HealthStatus:        llmkb.HealthStatusHealthy,
			CurrentCostEstimate: &llmkb.CostEstimate{InputPer1M: 3, OutputPer1M: 15, Currency: "USD", Tier: llmkb.CostTierExpensive},
		},
		Evaluation: llmkb.EvaluationProfile{ObservedFailurePatterns: []llmkb.FailurePattern{llmkb.FailurePatternTimeout}},
		UsageStats: llmkb.UsageStats{TotalExecutions: 3, SuccessRate: 0.66},
		Routing: llmkb.RoutingProfile{
			PreferredFor:       []llmkb.TaskClass{llmkb.TaskClassCoding},
			CostTier:           llmkb.CostTierExpensive,
			LatencyTier:        llmkb.LatencyTierMedium,
			MaxRiskTierAllowed: llmkb.RiskTierMedium,
			AutonomyCeiling:    llmkb.AutonomyLevelAssistive,
		},
		Provenance: provenance,
		CreatedAt:  now,
	}
	profile.Provenance.MutationHistory = []llmkb.MutationRecord{{FieldName: "display_name", NewValue: "Claude Sonnet", ChangedAt: now, Source: llmkb.ProvenanceSourceCuratedSeed}}
	if err := repo.SaveProfile(ctx, profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	gotProfile, err := repo.GetProfile(ctx, profile.LLMID)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if gotProfile == nil || gotProfile.CanonicalName != profile.CanonicalName {
		t.Fatalf("expected profile round-trip, got %+v", gotProfile)
	}
	if gotProfile.SchemaVersion != 1 || len(gotProfile.Aliases) != 2 || gotProfile.Capabilities.AgenticClass != llmkb.AgenticClassCapable {
		t.Fatalf("expected aliases/capability to round-trip, got %+v", gotProfile)
	}

	instance := llmkb.LLMRuntimeInstance{
		InstanceID:     "runtime-1",
		LLMID:          profile.LLMID,
		ProviderID:     provider.ProviderID,
		RuntimeBackend: llmkb.RuntimeBackendAnthropic,
		AuthStatus:     llmkb.AuthStatusAuthenticated,
		HealthStatus:   llmkb.HealthStatusHealthy,
		LoadedStatus:   llmkb.LoadedStatusLoaded,
		Endpoint:       provider.EndpointBase,
		LastProbeAt:    &now,
	}
	if err := repo.SaveRuntimeInstance(ctx, instance); err != nil {
		t.Fatalf("SaveRuntimeInstance: %v", err)
	}
	if gotInstance, err := repo.GetRuntimeInstance(ctx, instance.InstanceID); err != nil || gotInstance == nil || gotInstance.LLMID != profile.LLMID {
		t.Fatalf("GetRuntimeInstance: %v %+v", err, gotInstance)
	}
	instances, err := repo.ListRuntimeInstances(ctx, profile.LLMID)
	if err != nil {
		t.Fatalf("ListRuntimeInstances: %v", err)
	}
	if len(instances) != 1 || instances[0].InstanceID != instance.InstanceID {
		t.Fatalf("expected runtime instance list round-trip, got %+v", instances)
	}

	decision := llmkb.RouterDecision{
		DecisionID:     "decision-1",
		TaskID:         "task-1",
		TaskClass:      llmkb.TaskClassCoding,
		SelectedLLMID:  profile.LLMID,
		RiskTier:       llmkb.RiskTierMedium,
		CandidateSet:   []string{profile.LLMID, "openai.gpt-4o"},
		Scores:         map[string]float64{profile.LLMID: 0.91},
		RejectedModels: []llmkb.RejectionRecord{{ReasonCode: llmkb.RejectionCodeOverBudget, Reason: "too expensive"}},
		CreatedAt:      now,
	}
	if err := repo.AppendRouterDecision(ctx, decision); err != nil {
		t.Fatalf("AppendRouterDecision: %v", err)
	}
	decisions, err := repo.ListRouterDecisions(ctx, "task-1", 10)
	if err != nil {
		t.Fatalf("ListRouterDecisions: %v", err)
	}
	if len(decisions) != 1 || decisions[0].TaskID != "task-1" || len(decisions[0].CandidateSet) != 2 {
		t.Fatalf("expected router decision round-trip, got %+v", decisions)
	}

	cost := 0.15
	record := llmkb.LLMExecutionRecord{
		RecordID:          "exec-1",
		RouterDecisionID:  decision.DecisionID,
		LLMID:             profile.LLMID,
		TaskClass:         llmkb.TaskClassCoding,
		ContextSizeTokens: 123,
		OutputSizeTokens:  45,
		LatencyMs:         2345,
		CostUSD:           &cost,
		Outcome:           llmkb.ExecutionOutcomeSuccess,
		CreatedAt:         now,
	}
	if err := repo.AppendExecutionRecord(ctx, record); err != nil {
		t.Fatalf("AppendExecutionRecord: %v", err)
	}
	records, err := repo.ListExecutionRecords(ctx, profile.LLMID, 10)
	if err != nil {
		t.Fatalf("ListExecutionRecords: %v", err)
	}
	if len(records) != 1 || records[0].RecordID != "exec-1" || records[0].Outcome != llmkb.ExecutionOutcomeSuccess {
		t.Fatalf("expected execution record round-trip, got %+v", records)
	}
}

func TestSQLiteLLMKBRepo_PruneExecutionRecords(t *testing.T) {
	ctx := context.Background()
	db, err := Open("file:llmkb_prune?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}

	repo := NewSQLiteLLMKBRepo(db)
	seedTestLLMProfile(t, ctx, repo, "test.model", llmkb.TechnicalFeatures{SupportsTools: true})
	now := time.Now().UTC()

	// Create two records: one old (100 days), one new (now)
	oldRecord := llmkb.LLMExecutionRecord{
		RecordID:  "old-1",
		LLMID:     "test.model",
		TaskClass: llmkb.TaskClassChat,
		Outcome:   llmkb.ExecutionOutcomeSuccess,
		CreatedAt: now.Add(-100 * 24 * time.Hour),
	}
	newRecord := llmkb.LLMExecutionRecord{
		RecordID:  "new-1",
		LLMID:     "test.model",
		TaskClass: llmkb.TaskClassChat,
		Outcome:   llmkb.ExecutionOutcomeSuccess,
		CreatedAt: now,
	}

	if err := repo.AppendExecutionRecord(ctx, oldRecord); err != nil {
		t.Fatalf("Append old record: %v", err)
	}
	if err := repo.AppendExecutionRecord(ctx, newRecord); err != nil {
		t.Fatalf("Append new record: %v", err)
	}

	// Prune records older than 90 days
	if err := repo.PruneExecutionRecords(ctx, 90*24*time.Hour); err != nil {
		t.Fatalf("PruneExecutionRecords: %v", err)
	}

	records, err := repo.ListExecutionRecords(ctx, "test.model", 10)
	if err != nil {
		t.Fatalf("ListExecutionRecords: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record after pruning, got %d", len(records))
	}
	if records[0].RecordID != "new-1" {
		t.Fatalf("expected new-1 to remain, got %s", records[0].RecordID)
	}
}

func TestSQLiteLLMKBRepo_MaterializeUsageStats_Cost(t *testing.T) {
	ctx := context.Background()
	db, err := Open("file:llmkb_mat_cost?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}

	repo := NewSQLiteLLMKBRepo(db)

	seedTestLLMProfile(t, ctx, repo, "test.cost.model", llmkb.TechnicalFeatures{
		SupportsTools: true,
	})
	// Add context for OperationalState for pricing testing
	opProf, _ := repo.GetProfile(ctx, "test.cost.model")
	opProf.OperationalState.CurrentCostEstimate = &llmkb.CostEstimate{
		InputPer1M:  50.0,  // 0.05 per 1k = 50 per 1M
		OutputPer1M: 100.0, // 0.10 per 1k = 100 per 1M
	}
	repo.SaveProfile(ctx, *opProf)
	// Add execution record: 10,000 input tokens, 5,000 output tokens
	// Expected cost: (10 * 0.05) + (5 * 0.10) = 0.50 + 0.50 = 1.00 USD
	record := llmkb.LLMExecutionRecord{
		RecordID:          "exec-cost-1",
		LLMID:             "test.cost.model",
		TaskClass:         llmkb.TaskClassChat,
		ContextSizeTokens: 10000,
		OutputSizeTokens:  5000,
		Outcome:           llmkb.ExecutionOutcomeSuccess,
		CreatedAt:         time.Now().UTC(),
	}
	if err := repo.AppendExecutionRecord(ctx, record); err != nil {
		t.Fatalf("AppendExecutionRecord: %v", err)
	}

	// Run materialization
	if err := repo.MaterializeUsageStats(ctx); err != nil {
		t.Fatalf("MaterializeUsageStats: %v", err)
	}

	// Verify stats
	updated, err := repo.GetProfile(ctx, "test.cost.model")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}

	if updated.UsageStats.AccumulatedCost != 1.00 {
		t.Fatalf("expected accumulated cost 1.00, got %.4f", updated.UsageStats.AccumulatedCost)
	}
	if updated.UsageStats.AverageCost != 1.00 {
		t.Fatalf("expected average cost 1.00, got %.4f", updated.UsageStats.AverageCost)
	}
}
