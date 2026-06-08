import { useQuery } from '@tanstack/react-query';
import { naviFetch } from './client';
import {
  StatusResponseSchema,
  AgentStatusResponseSchema,
  LlmActiveResponseSchema,
  PresenceSnapshotSchema,
  ErrorSummaryResponseSchema,
  OperatorOverviewSchema,
  type StatusResponse,
  type AgentStatusResponse,
  type LlmActiveResponse,
  type PresenceSnapshot,
  type ErrorSummaryResponse,
  type OperatorOverview,
} from '@/types/api';

export function useOperatorOverview() {
  return useQuery<OperatorOverview>({
    queryKey: ['operator-overview'],
    queryFn: () => naviFetch('/api/operator/overview', OperatorOverviewSchema),
    refetchInterval: 10_000,
  });
}


export function useSystemStatus() {
  return useQuery<StatusResponse>({
    queryKey: ['status'],
    queryFn: () => naviFetch('/api/status', StatusResponseSchema),
    refetchInterval: 10_000,
  });
}

export function useAgentStatus() {
  return useQuery<AgentStatusResponse>({
    queryKey: ['agent-status'],
    queryFn: () => naviFetch('/api/agent/status', AgentStatusResponseSchema),
    refetchInterval: 5_000,
  });
}

export function useLlmActive() {
  return useQuery<LlmActiveResponse>({
    queryKey: ['llm-active'],
    queryFn: () => naviFetch('/api/llm/active', LlmActiveResponseSchema),
    refetchInterval: 30_000,
    retry: 1,
  });
}

export function usePresence() {
  return useQuery<PresenceSnapshot>({
    queryKey: ['presence'],
    queryFn: () => naviFetch('/api/presence', PresenceSnapshotSchema),
    refetchInterval: 10_000,
  });
}

export function useErrorSummary() {
  return useQuery<ErrorSummaryResponse>({
    queryKey: ['error-summary'],
    queryFn: () => naviFetch('/api/errors/summary', ErrorSummaryResponseSchema),
    refetchInterval: 15_000,
  });
}

