import { useQuery } from '@tanstack/react-query';
import { naviFetch } from './client';
import {
  DebugEventsResponseSchema,
  RuntimeMetricsSchema,
  LlmKbProfileSchema,
  LlmKbProfilesResponseSchema,
  GovernorSchema,
  type DebugEventsResponse,
  type RuntimeMetrics,
  type LlmKbProfile,
  type LlmKbProfilesResponse,
  type Governor,
} from '@/types/api';

export function useDebugEvents(limit = 50) {
  return useQuery<DebugEventsResponse>({
    queryKey: ['debug-events', limit],
    queryFn: () => naviFetch(`/api/debug/events?limit=${limit}`, DebugEventsResponseSchema),
    refetchInterval: 15_000,
    retry: 1,
  });
}

export function useRuntimeMetrics() {
  return useQuery<RuntimeMetrics>({
    queryKey: ['runtime-metrics'],
    queryFn: () => naviFetch('/api/debug/runtime-metrics', RuntimeMetricsSchema),
    refetchInterval: 30_000,
    retry: 1,
  });
}

export function useLlmKbProfiles() {
  return useQuery<LlmKbProfilesResponse>({
    queryKey: ['llmkb-profiles'],
    queryFn: () => naviFetch('/api/debug/llmkb/profiles', LlmKbProfilesResponseSchema),
    refetchInterval: 60_000,
  });
}

export function useLlmKbProfile(id: string | null) {
  return useQuery<LlmKbProfile>({
    queryKey: ['llmkb-profile', id],
    queryFn: () => naviFetch(`/api/debug/llmkb/profiles/${id}`, LlmKbProfileSchema),
    enabled: !!id,
    refetchInterval: 60_000,
  });
}

export function useGovernor() {
  return useQuery<Governor>({
    queryKey: ['governor'],
    queryFn: () => naviFetch('/api/governor', GovernorSchema),
    refetchInterval: 10_000,
  });
}
