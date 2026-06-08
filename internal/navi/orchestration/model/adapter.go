package model

import (
	"context"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/orchestration"
)

type ProfileResolver interface {
	ResolveProfile(ctx context.Context, req orchestration.CanonicalRunRequest) (orchestration.ModelProfile, error)
}

type ProfileResolverFunc func(ctx context.Context, req orchestration.CanonicalRunRequest) (orchestration.ModelProfile, error)

func (f ProfileResolverFunc) ResolveProfile(ctx context.Context, req orchestration.CanonicalRunRequest) (orchestration.ModelProfile, error) {
	return f(ctx, req)
}

type DefaultAdapter struct {
	ProfileResolver ProfileResolver
	ToolResolver    ToolResolver
}

func NewDefaultAdapter(profileResolver ProfileResolver, toolResolver ToolResolver) *DefaultAdapter {
	return &DefaultAdapter{
		ProfileResolver: profileResolver,
		ToolResolver:    toolResolver,
	}
}

func (a *DefaultAdapter) CompileRequest(ctx context.Context, req orchestration.CanonicalRunRequest, pack orchestration.ContextPack, stack orchestration.InstructionStack) (orchestration.CompiledModelRequest, error) {
	profile := ApplyProviderDefaults(req.Model)
	if a != nil && a.ProfileResolver != nil {
		resolved, err := a.ProfileResolver.ResolveProfile(ctx, req)
		if err != nil {
			return orchestration.CompiledModelRequest{}, err
		}
		profile = ApplyProviderDefaults(mergeProfiles(profile, ApplyProviderDefaults(resolved)))
	}
	return compileProviderRequest(profile, req, pack, stack, toolResolverOrNil(a)), nil
}

func (a *DefaultAdapter) NormalizeResponse(_ context.Context, req orchestration.CompiledModelRequest, raw *llm.Response) (orchestration.NormalizedModelResponse, error) {
	profile := ApplyProviderDefaults(req.Profile)
	response := orchestration.NormalizedModelResponse{
		Profile:    profile,
		RawPayload: normalizeProviderResponse(llmProviderProfile{Provider: profile.Provider, Model: profile.Model}, raw),
	}
	if raw == nil {
		return response, nil
	}
	response.Content = raw.Content
	response.ToolCalls = append([]llm.ToolCall(nil), raw.ToolCalls...)
	response.FinishReason = raw.FinishReason
	response.InputTokens = raw.InputTokens
	response.OutputTokens = raw.OutputTokens
	return response, nil
}

func toolResolverOrNil(adapter *DefaultAdapter) ToolResolver {
	if adapter == nil {
		return nil
	}
	return adapter.ToolResolver
}

func mergeProfiles(base, override orchestration.ModelProfile) orchestration.ModelProfile {
	base.Provider = firstNonEmpty(override.Provider, base.Provider)
	base.Model = firstNonEmpty(override.Model, base.Model)
	base.SupportsTools = base.SupportsTools || override.SupportsTools
	base.SupportsStreaming = base.SupportsStreaming || override.SupportsStreaming
	base.SupportsSystemRole = base.SupportsSystemRole || override.SupportsSystemRole
	base.SupportsMultiSystemMessages = base.SupportsMultiSystemMessages || override.SupportsMultiSystemMessages
	if override.MaxContextTokens > 0 {
		base.MaxContextTokens = override.MaxContextTokens
	}
	base.Quirks = uniqueClean(append(base.Quirks, override.Quirks...))
	if len(override.Metadata) > 0 {
		if base.Metadata == nil {
			base.Metadata = map[string]string{}
		}
		for key, value := range override.Metadata {
			base.Metadata[key] = value
		}
	}
	return base
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
