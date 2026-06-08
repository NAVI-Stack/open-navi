package llm

import (
	"context"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/llmkb"
	"github.com/ceoai/navi/internal/schema"
	"github.com/google/uuid"
)

// ensureKBProfileExists checks if a profile exists in the KB and creates a stub if missing.
func ensureKBProfileExists(ctx context.Context, repo LLMKBRepo, providerID, modelID string) string {
	if repo == nil {
		return firstNonEmpty(modelID, providerID)
	}
	profile, err := repo.EnsureRuntimeProfile(ctx, providerID, modelID)
	if err == nil && profile != nil {
		return profile.LLMID
	}
	return firstNonEmpty(modelID, providerID)
}

// MapRouteDecisionToKB converts a route decision into an LLM-KB router decision record.
func MapRouteDecisionToKB(ctx context.Context, repo LLMKBRepo, classification TaskClassification, result RouteDecision) *llmkb.RouterDecision {
	if result.Provider == "" || result.Model == "" {
		return nil
	}

	llmID := ensureKBProfileExists(ctx, repo, result.Provider, result.Model)

	return &llmkb.RouterDecision{
		DecisionID:    uuid.New().String(),
		TaskID:        result.TaskID,
		TaskClass:     llmkb.TaskClass(classification.Task),
		SelectedLLMID: llmID,
		CreatedAt:     time.Now().UTC(),
	}
}

// MapExecutionToKBRecord converts an execution outcome into an LLM-KB execution record.
func MapExecutionToKBRecord(ctx context.Context, repo LLMKBRepo, chatID, taskID string, eo schema.ExecutionOutcome) *llmkb.LLMExecutionRecord {
	if eo.LLMProvider == "" || eo.LLMModel == "" {
		return nil
	}

	llmID := ensureKBProfileExists(ctx, repo, eo.LLMProvider, eo.LLMModel)

	duration := 0
	if eo.EndTime != nil {
		duration = int(eo.EndTime.Sub(eo.StartTime).Milliseconds())
	}

	taskClass := llmkb.TaskClassChat
	if tc, err := llmkb.ParseTaskClass(eo.LLMTaskClass); err == nil {
		taskClass = tc
	}

	decisionID := strings.TrimSpace(eo.CorrelationID)

	outcome := llmkb.ExecutionOutcomeFailure
	if eo.Outcome == schema.ExecutionOutcomeSucceeded {
		outcome = llmkb.ExecutionOutcomeSuccess
	}
	return &llmkb.LLMExecutionRecord{
		RecordID:         eo.AttemptID,
		RouterDecisionID: decisionID,
		TaskID:           taskID,
		LLMID:            llmID,
		TaskClass:        taskClass,
		LatencyMs:        duration,
		Outcome:          outcome,
		CreatedAt:        time.Now().UTC(),
	}
}

// BuildProfilesFromCatalogAndKB enriches seed profiles with LLM-KB data.
func BuildProfilesFromCatalogAndKB(ctx context.Context, repo LLMKBRepo, catalog LLMCatalog) []ModelProfile {
	profiles := SeedProfiles(catalog)
	if repo == nil {
		return profiles
	}
	for i := range profiles {
		kbProfile, err := repo.ResolveProfileForProviderModel(ctx, profiles[i].ProviderKey, profiles[i].ModelID)
		if err != nil || kbProfile == nil {
			continue
		}
		profiles[i] = enrichRuntimeModelProfile(profiles[i], *kbProfile)
	}
	return profiles
}

func enrichRuntimeModelProfile(base ModelProfile, profile llmkb.LLMProfile) ModelProfile {
	agentic, coding, chat, reasoning := kbBaseScores(profile)
	successBonus := int((profile.UsageStats.SuccessRateTotal - 0.5) * 40) // -20 to +20
	base.DisplayName = firstNonEmpty(profile.CanonicalName, base.DisplayName)
	base.SupportsTools = profile.Features.SupportsTools
	base.SupportsStream = profile.Features.SupportsStreaming
	base.ToolCallReliable = profile.Features.SupportsTools
	base.AgenticScore = clampScore(base.AgenticScore + agentic + successBonus)
	base.CodingScore = clampScore(base.CodingScore + coding + successBonus)
	base.ChatScore = clampScore(base.ChatScore + chat + successBonus)
	base.ReasoningScore = clampScore(base.ReasoningScore + reasoning + successBonus)
	if profile.Features.ContextWindowTokens > 0 {
		base.MaxContextTokens = profile.Features.ContextWindowTokens
	}
	if speed := kbSpeedScore(profile.Routing.LatencyTier); speed > 0 {
		base.SpeedScore = speed
	}
	if cost := kbCostScore(profile.Routing.CostTier); cost > 0 {
		base.CostScore = cost
	}
	return base
}

func kbBaseScores(profile llmkb.LLMProfile) (agentic, coding, chat, reasoning int) {
	agentic, coding, chat, reasoning = 0, 0, 0, 0
	for _, eval := range profile.Evaluation.InternalScores {
		switch strings.ToLower(eval.Name) {
		case "agentic":
			agentic = int(eval.Score)
		case "coding":
			coding = int(eval.Score)
		case "chat":
			chat = int(eval.Score)
		case "reasoning":
			reasoning = int(eval.Score)
		}
	}
	return agentic, coding, chat, reasoning
}

func kbSpeedScore(tier llmkb.LatencyTier) int {
	switch tier {
	case llmkb.LatencyTierLow:
		return 95
	case llmkb.LatencyTierMedium:
		return 70
	case llmkb.LatencyTierHigh:
		return 40
	default:
		return 0
	}
}

func kbCostScore(tier llmkb.CostTier) int {
	switch tier {
	case llmkb.CostTierFree:
		return 100
	case llmkb.CostTierCheap:
		return 85
	case llmkb.CostTierModerate:
		return 55
	case llmkb.CostTierExpensive:
		return 20
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
