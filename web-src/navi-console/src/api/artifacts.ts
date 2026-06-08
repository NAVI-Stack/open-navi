import { useQuery } from '@tanstack/react-query';
import { naviFetch } from './client';
import {
  ArtifactListResponseSchema,
  ArtifactSchema,
} from '@/types/api';
import type { Artifact } from '@/types/api';
import { z } from 'zod';

export function useArtifacts(params: Record<string, string> = {}) {
  const query = new URLSearchParams(params).toString();
  const url = `/api/artifacts${query ? `?${query}` : ''}`;
  return useQuery<Artifact[]>({
    queryKey: ['artifacts', params],
    queryFn: async () => {
      const resp = await naviFetch(url, ArtifactListResponseSchema);
      return resp.items;
    },
    refetchInterval: 30_000,
  });
}

export function useArtifact(id: string | null) {
  const url = id ? `/api/artifacts/${id}` : null;
  return useQuery<Artifact>({
    queryKey: ['artifact', id],
    queryFn: () => {
      if (!url) throw new Error('No ID');
      return naviFetch(url, ArtifactSchema);
    },
    enabled: !!id,
    refetchInterval: 30_000,
  });
}

export async function getArtifactContent(id: string, versionId?: string): Promise<string> {
  const url = versionId
    ? `/api/artifacts/${id}/versions/${versionId}/content`
    : `/api/artifacts/${id}/content`;

  const resp = await naviFetch(url, z.any());
  if (typeof resp === 'string') return resp;
  if (resp && typeof resp === 'object' && 'content' in resp && typeof resp.content === 'string') {
    return resp.content;
  }
  return JSON.stringify(resp, null, 2);
}

export async function restoreArtifactVersion(id: string, versionId: string, reason?: string): Promise<void> {
  await naviFetch(`/api/artifacts/${id}/versions/${versionId}/restore`, z.any(), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ reason }),
  });
}

export async function createArtifactExport(id: string, format: string, targetKind: string): Promise<void> {
  await naviFetch(`/api/artifacts/${id}/exports`, z.any(), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      format,
      target_kind: targetKind,
    }),
  });
}
