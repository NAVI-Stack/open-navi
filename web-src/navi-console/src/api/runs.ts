import { useQuery } from '@tanstack/react-query';
import { naviFetch } from './client';
import { RunsResponseSchema, RunItemSchema, type RunsResponse, type RunItem } from '@/types/api';

export function useRuns(limit?: number, cursor?: string) {
  return useQuery<RunsResponse>({
    queryKey: ['runs', limit, cursor],
    queryFn: () => {
      const query = new URLSearchParams();
      if (limit) query.set('limit', limit.toString());
      if (cursor) query.set('cursor', cursor);

      const qs = query.toString();
      const url = qs ? `/api/runs?${qs}` : '/api/runs';
      return naviFetch(url, RunsResponseSchema);
    },
    refetchInterval: 15_000,
  });
}

export function useRun(id: string | null) {
  const url = id ? `/api/runs/${id}` : null;
  return useQuery<RunItem>({
    queryKey: ['run', id],
    queryFn: () => {
      if (!url) throw new Error('No ID');
      return naviFetch(url, RunItemSchema);
    },
    enabled: !!id,
    refetchInterval: 15_000,
  });
}
