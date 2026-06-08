import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { naviFetch } from './client';
import {
  ConnectorSyncViewSchema,
  ConnectorSyncViewListSchema,
  IntakeSyncLogEntrySchema,
  RecentIntakeListSchema,
  VaultSyncLogListSchema,
  type ConnectorSyncView,
  type IntakeSyncLogEntry,
  type RecentIntakeRecord,
  type SyncPolicy,
  type VaultSyncLogEntry,
} from '@/types/api';
import { z } from 'zod';

// CIP P5 — intake sync policy + sync log Console hooks. These ride inside the
// existing connector, chat/project, Proposal, and operator surfaces (no new
// top-level nav).

// useConnectorSyncList returns the sync view for every connector that has a
// policy or has produced sync-log rows.
export function useConnectorSyncList() {
  return useQuery<ConnectorSyncView[]>({
    queryKey: ['intake', 'policy', 'list'],
    queryFn: () => naviFetch('/api/intake/policy', ConnectorSyncViewListSchema),
    refetchInterval: 15_000,
  });
}

// useConnectorSync returns the sync view (policy + last pass + recent passes)
// for a single connector.
export function useConnectorSync(connectorId: string | null) {
  return useQuery<ConnectorSyncView>({
    queryKey: ['intake', 'policy', connectorId],
    enabled: !!connectorId,
    queryFn: () =>
      naviFetch(`/api/intake/policy?connector=${encodeURIComponent(connectorId!)}`, ConnectorSyncViewSchema),
    refetchInterval: 15_000,
  });
}

// useUpdateSyncPolicy persists an owner override (configuration, not a governed
// mutation). The non-zero fields overlay the default/file policy.
export function useUpdateSyncPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (policy: SyncPolicy) =>
      naviFetch(`/api/intake/policy`, ConnectorSyncViewSchema, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(policy),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['intake', 'policy'] });
    },
  });
}

// useIntakeSyncLog lists sync-log rows, optionally scoped to one connector.
export function useIntakeSyncLog(connectorId?: string, limit = 50) {
  const q = connectorId
    ? `?connector=${encodeURIComponent(connectorId)}&limit=${limit}`
    : `?limit=${limit}`;
  return useQuery<IntakeSyncLogEntry[]>({
    queryKey: ['intake', 'sync-log', connectorId ?? 'all'],
    queryFn: () => naviFetch(`/api/intake/sync-log${q}`, z.array(IntakeSyncLogEntrySchema)),
    refetchInterval: 15_000,
  });
}

// useRecentIntake lists recent provenance-bearing intake records.
export function useRecentIntake(connectorId?: string, limit = 20) {
  const q = connectorId
    ? `?connector=${encodeURIComponent(connectorId)}&limit=${limit}`
    : `?limit=${limit}`;
  return useQuery<RecentIntakeRecord[]>({
    queryKey: ['intake', 'recent', connectorId ?? 'all'],
    queryFn: () => naviFetch(`/api/intake/recent${q}`, RecentIntakeListSchema),
    refetchInterval: 15_000,
  });
}

// useVaultSyncLog lists Vault per-file diff passes (empty until the Vault ships).
export function useVaultSyncLog(path?: string, limit = 50) {
  const q = path ? `?path=${encodeURIComponent(path)}&limit=${limit}` : `?limit=${limit}`;
  return useQuery<VaultSyncLogEntry[]>({
    queryKey: ['vault', 'sync-log', path ?? 'all'],
    queryFn: () => naviFetch(`/api/vault/sync-log${q}`, VaultSyncLogListSchema),
    refetchInterval: 30_000,
  });
}

// useRequestBackfill raises a backfill consent Proposal for a connector.
export function useRequestBackfill() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (connector: string) =>
      naviFetch(`/api/intake/backfill`, undefined, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ connector }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['proposals'] });
      qc.invalidateQueries({ queryKey: ['intake', 'policy'] });
    },
  });
}
