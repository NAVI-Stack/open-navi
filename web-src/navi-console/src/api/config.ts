import { useQuery } from '@tanstack/react-query';
import { naviFetch } from './client';
import { z } from 'zod';
import {
  AuthMeSchema,
  IdentitySchema,
  ExperienceSchema,
  ExperienceInspectSchema,
  ExperienceModuleRegistrySchema,
  PresenceSnapshotSchema,
  NaviPresenceSchema,
  LlmActiveResponseSchema,
  LlmPreferencesSchema,
  LlmProvidersSchema,
  LlmProfilesSchema,
  LlmProviderModelsSchema,
  LlmProviderHealthSchema,
} from '@/types/api';

// --- Identity ---
export function useAuthMe() {
  return useQuery({
    queryKey: ['auth', 'me'],
    queryFn: () => naviFetch('/api/auth/me', AuthMeSchema),
    staleTime: 60_000,
  });
}

export function useIdentity() {
  return useQuery({
    queryKey: ['identity'],
    queryFn: () => naviFetch('/api/identity', IdentitySchema),
    staleTime: 60_000,
  });
}

// --- Experience ---
export function useExperience() {
  return useQuery({
    queryKey: ['experience'],
    queryFn: () => naviFetch('/api/experience', ExperienceSchema),
    staleTime: 60_000,
  });
}

export function useExperienceInspect() {
  return useQuery({
    queryKey: ['experience', 'inspect'],
    queryFn: () => naviFetch('/api/experience/inspect', ExperienceInspectSchema),
    staleTime: 60_000,
  });
}

export function useExperienceModuleRegistry() {
  return useQuery({
    queryKey: ['experience', 'module-registry'],
    queryFn: () => naviFetch('/api/experience/module-registry', ExperienceModuleRegistrySchema),
    staleTime: 60_000,
  });
}

// --- Presence ---
export function usePresenceSnapshot() {
  return useQuery({
    queryKey: ['presence'],
    queryFn: () => naviFetch('/api/presence', PresenceSnapshotSchema),
    refetchInterval: 15_000,
  });
}

export function useNaviPresence() {
  return useQuery({
    queryKey: ['presence', 'navi'],
    queryFn: () => naviFetch('/api/presence/navi', NaviPresenceSchema),
    refetchInterval: 15_000,
  });
}

export async function updateUserPresence(payload: unknown): Promise<void> {
  await naviFetch('/api/presence/user', z.any(), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

// --- LLM Queries ---
export function useLlmActive() {
  return useQuery({
    queryKey: ['llm', 'active'],
    queryFn: () => naviFetch('/api/llm/active', LlmActiveResponseSchema),
    staleTime: 30_000,
  });
}

export function useLlmPreferences() {
  return useQuery({
    queryKey: ['llm', 'preferences'],
    queryFn: () => naviFetch('/api/llm/preferences', LlmPreferencesSchema),
    staleTime: 30_000,
  });
}

export function useLlmProviders() {
  return useQuery({
    queryKey: ['llm', 'providers'],
    queryFn: () => naviFetch('/api/llm/providers', LlmProvidersSchema),
    staleTime: 30_000,
  });
}

export function useLlmProfiles() {
  return useQuery({
    queryKey: ['llm', 'profiles'],
    queryFn: () => naviFetch('/api/llm/profiles', LlmProfilesSchema),
    staleTime: 30_000,
  });
}

export function useLlmProviderModels(providerKey: string | null) {
  return useQuery({
    queryKey: ['llm', 'providers', providerKey, 'models'],
    queryFn: () => naviFetch(`/api/llm/providers/${providerKey}/models`, LlmProviderModelsSchema),
    enabled: !!providerKey,
    staleTime: 60_000,
  });
}

export function useLlmProviderHealth(providerKey: string | null, enabled: boolean) {
  return useQuery({
    queryKey: ['llm', 'providers', providerKey, 'health'],
    queryFn: () => naviFetch(`/api/llm/providers/${providerKey}/health`, LlmProviderHealthSchema),
    enabled: !!providerKey && enabled,
    staleTime: 0,
    gcTime: 0,
  });
}

// --- LLM Mutations ---
export async function setLlmActive(provider: string, model: string): Promise<{ provider: string; model: string; status: string }> {
  return naviFetch('/api/llm/active', z.object({ provider: z.string(), model: z.string(), status: z.string() }), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ provider, model }),
  });
}

export async function patchLlmPreferences(patch: Record<string, unknown>): Promise<unknown> {
  return naviFetch('/api/llm/preferences', LlmPreferencesSchema, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
}

export async function configureLlmProvider(
  key: string,
  req: { api_key?: string; model?: string; endpoint?: string },
): Promise<unknown> {
  return naviFetch(`/api/llm/providers/${encodeURIComponent(key)}`, z.unknown(), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  });
}

export async function disableLlmProvider(key: string): Promise<void> {
  await naviFetch(`/api/llm/providers/${encodeURIComponent(key)}`, z.unknown(), {
    method: 'DELETE',
  });
}
