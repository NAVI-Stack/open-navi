import { useQuery } from '@tanstack/react-query';
import { naviFetch } from './client';
import {
  ActivityResponseSchema,
  ErrorsResponseSchema,
  type ActivityResponse,
  type ErrorsResponse,
} from '@/types/api';

export function useActivity(limit = 50) {
  return useQuery<ActivityResponse>({
    queryKey: ['activity', limit],
    queryFn: () => naviFetch(`/api/activity?limit=${limit}`, ActivityResponseSchema),
    refetchInterval: 10_000,
    retry: 1,
  });
}

export function useErrors(limit = 100) {
  return useQuery<ErrorsResponse>({
    queryKey: ['errors', limit],
    queryFn: () => naviFetch(`/api/errors?limit=${limit}`, ErrorsResponseSchema),
    refetchInterval: 15_000,
    retry: 1,
  });
}
