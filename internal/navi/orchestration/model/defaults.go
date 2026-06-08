package model

import (
	"strings"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/orchestration"
)

const (
	defaultMaxResponseTokens       = 2048
	defaultPlainChatResponseTokens = 512
)

func ApplyProviderDefaults(profile orchestration.ModelProfile) orchestration.ModelProfile {
	profile = NormalizeProfile(profile)
	switch strings.ToLower(strings.TrimSpace(profile.Provider)) {
	case "ollama":
		quirks := []string{
			QuirkSingleSystemMessage,
			QuirkPlainFunctionalStyle,
			QuirkRepeatGuardOptions,
		}
		if !profile.SupportsTools {
			quirks = append(quirks, QuirkSuppressToolsForChat)
		}
		profile.Quirks = uniqueClean(append(profile.Quirks, quirks...))
	}
	return profile
}

func DefaultOptionsForRequest(profile orchestration.ModelProfile, req orchestration.CanonicalRunRequest) llm.Options {
	profile = ApplyProviderDefaults(profile)

	temp := 0.5
	if strings.EqualFold(strings.TrimSpace(req.ExperienceMode), "wizard") {
		temp = 0.6
	}

	opts := llm.Options{
		Temperature: temp,
		MaxTokens:   defaultMaxTokensForRequest(req),
	}
	if ProfileHasQuirk(profile, QuirkRepeatGuardOptions) {
		opts.RepeatPenalty = 1.2
		opts.RepeatLastN = 128
	}
	return opts
}

func defaultMaxTokensForRequest(req orchestration.CanonicalRunRequest) int {
	if req.Frame.Mode == orchestration.ExecutionModeResume {
		return defaultMaxResponseTokens
	}
	if req.RequiredOutput.AllowToolCalls || req.RequiredOutput.AllowScheduledReplies {
		return defaultMaxResponseTokens
	}
	if len(req.CapabilitySurface.ToolNames) > 0 {
		return defaultMaxResponseTokens
	}
	return defaultPlainChatResponseTokens
}
