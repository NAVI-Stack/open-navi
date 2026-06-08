import { useState, useCallback, useEffect, useMemo } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  useLlmActive,
  useLlmProviders,
  useLlmProfiles,
  useLlmPreferences,
  setLlmActive,
  patchLlmPreferences,
} from '@/api/config';
import type { LlmModelProfile, LlmProviderDescriptor } from '@/types/api';
import type {
  BrowserModel,
  BrowserProvider,
  ModelCapability,
  ModelStatus,
} from './types';

// ─── Capability derivation ────────────────────────────────────────────────────

function deriveCapabilities(profile: LlmModelProfile): ModelCapability[] {
  const caps: ModelCapability[] = [];
  const tags = profile.tags ?? [];

  if (profile.speed_score > 75 || tags.includes('fast')) caps.push('fast');
  if (tags.includes('vision') || tags.includes('multimodal')) caps.push('vision');
  if (profile.reasoning_score > 60 || tags.includes('reasoning') || tags.includes('architect'))
    caps.push('reasoning');
  if (profile.supports_tools) caps.push('tools');
  if (tags.includes('image') || tags.includes('imagegen') || tags.includes('image-gen'))
    caps.push('image');
  if (tags.includes('pdf') || tags.includes('document') || tags.includes('docs'))
    caps.push('pdf');
  if (profile.coding_score > 50 || tags.includes('coding') || tags.includes('code') || tags.includes('coder'))
    caps.push('code');

  return caps;
}

// ─── Provider status ──────────────────────────────────────────────────────────

function deriveProviderStatus(provider: LlmProviderDescriptor): ModelStatus {
  if (!provider.enabled) return 'unavailable';
  if (provider.healthy === false) return 'offline';
  if (provider.healthy === true) return 'connected';
  return 'unknown';
}

// ─── Map profile to BrowserModel ─────────────────────────────────────────────

function profileToBrowserModel(
  profile: LlmModelProfile,
  provider: LlmProviderDescriptor | undefined,
): BrowserModel {
  const tags = profile.tags ?? [];
  const isLegacy = tags.includes('legacy');
  const isRecommended = tags.includes('recommended') || tags.includes('workhorse') || tags.includes('generalist');
  const status = provider ? deriveProviderStatus(provider) : 'unknown';
  const executionType: BrowserModel['executionType'] =
    (provider?.kind === 'local' || tags.includes('local')) ? 'local' :
    (provider?.kind === 'cloud' || tags.includes('cloud')) ? 'cloud' :
    'unknown';

  // Passthrough fields from LLM KB (present when the backend emits them)
  const raw = profile as Record<string, unknown>;
  const learnedAgenticDelta = typeof raw.learned_agentic_delta === 'number' ? raw.learned_agentic_delta : undefined;
  const learnedCodingDelta = typeof raw.learned_coding_delta === 'number' ? raw.learned_coding_delta : undefined;
  const learnedChatDelta = typeof raw.learned_chat_delta === 'number' ? raw.learned_chat_delta : undefined;
  const learnedReasoningDelta = typeof raw.learned_reasoning_delta === 'number' ? raw.learned_reasoning_delta : undefined;
  const learnedEvidenceCount = typeof raw.learned_evidence_count === 'number' ? raw.learned_evidence_count : undefined;

  return {
    id: `${profile.provider_key}/${profile.model_id}`,
    providerKey: profile.provider_key,
    providerName: provider?.display_name ?? profile.provider_key,
    modelId: profile.model_id,
    displayName: profile.display_name || profile.model_id,
    capabilities: deriveCapabilities(profile),
    isLegacy,
    isRecommended,
    isNaviAuto: false,
    executionType,
    status,
    tags,
    speedScore: profile.speed_score,
    costScore: profile.cost_score,
    reasoningScore: profile.reasoning_score,
    codingScore: profile.coding_score,
    agenticScore: profile.agentic_score,
    chatScore: profile.chat_score,
    maxContextTokens: profile.max_context_tokens,
    supportsTools: profile.supports_tools,
    toolCallReliable: profile.tool_call_reliable,
    learnedAgenticDelta,
    learnedCodingDelta,
    learnedChatDelta,
    learnedReasoningDelta,
    learnedEvidenceCount,
  };
}

// ─── NAVI Auto synthetic entry ────────────────────────────────────────────────

const NAVI_AUTO_MODEL: BrowserModel = {
  id: 'navi/auto',
  providerKey: 'navi',
  providerName: 'NAVI',
  modelId: 'auto',
  displayName: 'NAVI Auto',
  capabilities: ['fast', 'vision', 'reasoning', 'tools', 'pdf', 'code'],
  isLegacy: false,
  isRecommended: true,
  isNaviAuto: true,
  executionType: 'auto',
  status: 'connected',
  tags: ['auto', 'recommended'],
  speedScore: 90,
  costScore: 80,
  reasoningScore: 85,
  codingScore: 80,
  agenticScore: 85,
  chatScore: 85,
  maxContextTokens: 0,
  supportsTools: true,
  toolCallReliable: true,
};

// ─── Hook ─────────────────────────────────────────────────────────────────────

export interface ModelBrowserData {
  models: BrowserModel[];
  providers: BrowserProvider[];
  selectedId: string;
  favoriteIds: string[];
  toggleFavorite: (id: string) => void;
  selectModel: (id: string) => void;
  isLoading: boolean;
}

export function useModelBrowserData(): ModelBrowserData {
  const queryClient = useQueryClient();
  const { data: activeData, isLoading: activeLoading } = useLlmActive();
  const { data: providersData, isLoading: providersLoading } = useLlmProviders();
  const { data: profilesData, isLoading: profilesLoading } = useLlmProfiles();
  const { data: preferencesData, isLoading: preferencesLoading } = useLlmPreferences();

  const isLoading = activeLoading || providersLoading || profilesLoading || preferencesLoading;

  // Build BrowserModel list: NAVI Auto first, then profiles.
  // Memoized so the derived list (and its array identity) only changes when the
  // underlying query data does — not on every parent re-render (e.g. composer keystrokes).
  // A stable `models` identity also lets ModelBrowser's own filter/sort memo stay warm.
  const models: BrowserModel[] = useMemo(() => {
    const providerMap = new Map<string, LlmProviderDescriptor>(
      (providersData?.providers ?? []).map((p) => [p.key, p]),
    );
    const profileModels = (profilesData?.profiles ?? []).map((p) =>
      profileToBrowserModel(p, providerMap.get(p.provider_key)),
    );
    return [NAVI_AUTO_MODEL, ...profileModels];
  }, [providersData, profilesData]);

  // Build BrowserProvider list from real providers.
  const providers: BrowserProvider[] = useMemo(
    () =>
      (providersData?.providers ?? []).map((p) => ({
        key: p.key,
        displayName: p.display_name,
        kind: p.kind,
        enabled: p.enabled,
        healthy: p.healthy,
        status: deriveProviderStatus(p),
        modelCount: p.model_count ?? 0,
      })),
    [providersData],
  );

  // Favorites come from server-persisted preferences.
  // Fall back to NAVI Auto being favorited by default when no preferences exist yet.
  const favoriteIds: string[] = useMemo(
    () => preferencesData?.favorite_model_ids ?? [NAVI_AUTO_MODEL.id],
    [preferencesData],
  );

  // Selected model: initialize from active API response
  const [selectedId, setSelectedId] = useState<string>(() => {
    if (activeData?.provider && activeData?.model) {
      return `${activeData.provider}/${activeData.model}`;
    }
    return NAVI_AUTO_MODEL.id;
  });

  // Sync selectedId when active data loads (covers navi/auto sentinel too)
  useEffect(() => {
    if (activeData?.provider && activeData?.model) {
      setSelectedId(`${activeData.provider}/${activeData.model}`);
    }
  }, [activeData?.provider, activeData?.model]);

  const toggleFavorite = useCallback(
    async (id: string) => {
      const next = favoriteIds.includes(id)
        ? favoriteIds.filter((x) => x !== id)
        : [...favoriteIds, id];
      try {
        await patchLlmPreferences({ favorite_model_ids: next });
        void queryClient.invalidateQueries({ queryKey: ['llm', 'preferences'] });
      } catch {
        // Silently ignore — UI will revert on next preferences fetch
      }
    },
    [favoriteIds, queryClient],
  );

  const selectModel = useCallback(
    async (id: string) => {
      setSelectedId(id);
      const slashIdx = id.indexOf('/');
      if (slashIdx === -1) return;
      const provider = id.slice(0, slashIdx);
      const model = id.slice(slashIdx + 1);
      try {
        await setLlmActive(provider, model);
        void queryClient.invalidateQueries({ queryKey: ['llm', 'active'] });
        // Selecting navi/auto toggles auto_routing_enabled on the backend;
        // selecting any real model clears it — in both cases the preferences cache is stale.
        void queryClient.invalidateQueries({ queryKey: ['llm', 'preferences'] });
      } catch {
        // Revert on failure
        if (activeData?.provider && activeData?.model) {
          setSelectedId(`${activeData.provider}/${activeData.model}`);
        }
      }
    },
    [queryClient, activeData],
  );

  return { models, providers, selectedId, favoriteIds, toggleFavorite, selectModel, isLoading };
}
