import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { naviFetch } from './client';
import {
  type ConsoleAppearanceResponse,
  type ConsoleAppearanceState,
  normalizeAppearance,
} from '@/appearance/theme';

export function useConsoleAppearance() {
  return useQuery<ConsoleAppearanceResponse>({
    queryKey: ['console', 'appearance'],
    queryFn: async () => normalizeAppearanceResponse(await naviFetch<unknown>('/api/console/appearance')),
    staleTime: 60_000,
  });
}

export function useSaveConsoleAppearance() {
  const queryClient = useQueryClient();
  return useMutation<ConsoleAppearanceResponse, Error, ConsoleAppearanceState>({
    mutationFn: async (appearance: ConsoleAppearanceState) => normalizeAppearanceResponse(await naviFetch<unknown>(
      '/api/console/appearance',
      undefined,
      {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(appearance),
      },
    )),
    onSuccess: (data) => {
      queryClient.setQueryData(['console', 'appearance'], data);
    },
  });
}

function normalizeAppearanceResponse(value: unknown): ConsoleAppearanceResponse {
  const raw = value && typeof value === 'object' && !Array.isArray(value)
    ? value as { appearance?: unknown; persisted?: unknown }
    : null;

  return {
    appearance: normalizeAppearance(raw?.appearance ?? value),
    persisted: Boolean(raw?.persisted),
  };
}
