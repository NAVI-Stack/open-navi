import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { naviFetch } from './client';
import { ProposalListSchema, type Proposal } from '@/types/api';

export function useProposals() {
  return useQuery<Proposal[]>({
    queryKey: ['proposals'],
    queryFn: () => naviFetch('/api/proposals', ProposalListSchema),
    refetchInterval: 5_000,
  });
}

export function useResolveProposal() {
  const queryClient = useQueryClient();
  
  return useMutation({
    mutationFn: async ({
      id,
      action,
      resolution_type,
      resolution_note,
      switch_workspace_id,
    }: {
      id: string;
      action: 'approve' | 'decline' | 'allow_once';
      resolution_type?: string;
      resolution_note?: string;
      switch_workspace_id?: string;
    }) => {
      return naviFetch(`/api/proposals/${id}/resolve`, undefined, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          action,
          resolution_type,
          resolution_note,
          switch_workspace_id,
        }),
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['proposals'] });
      queryClient.invalidateQueries({ queryKey: ['status'] });
      queryClient.invalidateQueries({ queryKey: ['activity'] });
      queryClient.invalidateQueries({ queryKey: ['runs'] });
      // Only invalidate chats if proposal resolution actually changes transcript/runtime-visible state.
    },
  });
}
