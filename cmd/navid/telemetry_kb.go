package main

import (
	"context"

	"github.com/ceoai/navi/internal/llm"
)

func firstNonEmpty(values ...string) string {
	for _, s := range values {
		if s != "" {
			return s
		}
	}
	return ""
}

func ensureKBProfileExists(ctx context.Context, repo llm.LLMKBRepo, providerID, modelID string) string {
	if repo == nil {
		return firstNonEmpty(modelID, providerID)
	}
	profile, err := repo.EnsureRuntimeProfile(ctx, providerID, modelID)
	if err == nil && profile != nil {
		return profile.LLMID
	}
	return firstNonEmpty(modelID, providerID)
}

func buildRoutingProfilesFromCatalogAndKB(ctx context.Context, repo llm.LLMKBRepo, catalog llm.LLMCatalog) []llm.ModelProfile {
	return llm.BuildProfilesFromCatalogAndKB(ctx, repo, catalog)
}
