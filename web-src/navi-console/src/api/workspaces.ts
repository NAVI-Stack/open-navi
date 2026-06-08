import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { naviFetch } from './client';
import {
  ActiveWorkspaceResponseSchema,
  WhitelistRuleSchema,
  WhitelistRulesResponseSchema,
  WorkspaceListResponseSchema,
  WorkspaceModeResponseSchema,
  WorkspacePathChildrenResponseSchema,
  WorkspacePathRootsResponseSchema,
  WorkspaceSchema,
  type ActiveWorkspaceResponse,
  type AllowedActions,
  type WhitelistRule,
  type WhitelistRulesResponse,
  type Workspace,
  type WorkspaceListResponse,
  type WorkspaceModeResponse,
  type WorkspacePathChildrenResponse,
  type WorkspacePathRootsResponse,
} from '@/types/api';

export type WorkspaceUpsertPayload = {
  workspace_id?: string;
  name: string;
  description?: string;
  workspace_kind?: string;
  status?: string;
  local_roots?: string[];
  repo_roots?: string[];
  protected_paths?: string[];
  allowed_actions?: AllowedActions;
  boundary_policy?: { out_of_scope_default: string };
  audit_enabled?: boolean;
  tags?: string[];
  related_project_id?: string;
  notes?: string;
  metadata?: Record<string, unknown>;
};

export type BoundaryResolutionPayload = {
  workspace_id?: string;
  decision: 'denied' | 'allow_once' | 'always_allow' | 'switched_workspace';
  target_path?: string;
  action?: string;
  action_types?: string[];
  switch_workspace_id?: string;
  expires_at?: string;
  rule_scope?: string;
};

export const defaultAllowedActions: AllowedActions = {
  read: true,
  write: true,
  create: true,
  modify: true,
  rename_move: true,
  delete: false,
  execute: false,
};

export function useWorkspaceMode(projectId?: string) {
  const suffix = projectId ? `?project_id=${encodeURIComponent(projectId)}` : '';
  return useQuery<WorkspaceModeResponse>({
    queryKey: ['workspace-mode', projectId ?? 'global'],
    queryFn: () => naviFetch(`/api/workspace-mode${suffix}`, WorkspaceModeResponseSchema),
    refetchInterval: 15_000,
  });
}

export function useWorkspaces(status?: string) {
  const suffix = status ? `?status=${encodeURIComponent(status)}` : '';
  return useQuery<WorkspaceListResponse>({
    queryKey: ['workspaces', status ?? 'all'],
    queryFn: () => naviFetch(`/api/workspaces${suffix}`, WorkspaceListResponseSchema),
    refetchInterval: 15_000,
  });
}

export function useWorkspace(workspaceId: string | null) {
  return useQuery<Workspace>({
    queryKey: ['workspace', workspaceId],
    queryFn: () => {
      if (!workspaceId) throw new Error('No workspace ID');
      return naviFetch(`/api/workspaces/${workspaceId}`, WorkspaceSchema);
    },
    enabled: !!workspaceId,
    refetchInterval: 15_000,
  });
}

export function useActiveWorkspace(projectId?: string) {
  const suffix = projectId ? `?project_id=${encodeURIComponent(projectId)}` : '';
  return useQuery<ActiveWorkspaceResponse>({
    queryKey: ['active-workspace', projectId ?? 'global'],
    queryFn: () => naviFetch(`/api/workspaces/active${suffix}`, ActiveWorkspaceResponseSchema),
    refetchInterval: 15_000,
  });
}

export function useWhitelistRules(workspaceId: string | null) {
  return useQuery<WhitelistRulesResponse>({
    queryKey: ['workspace-whitelist-rules', workspaceId],
    queryFn: () => {
      if (!workspaceId) throw new Error('No workspace ID');
      return naviFetch(`/api/workspaces/${workspaceId}/whitelist-rules`, WhitelistRulesResponseSchema);
    },
    enabled: !!workspaceId,
    refetchInterval: 15_000,
  });
}

export function useWorkspacePathRoots(enabled: boolean) {
  return useQuery<WorkspacePathRootsResponse>({
    queryKey: ['workspace-path-roots'],
    queryFn: () => naviFetch('/api/workspaces/path-roots', WorkspacePathRootsResponseSchema),
    enabled,
    staleTime: 30_000,
  });
}

export function useWorkspacePathChildren(path: string, enabled: boolean) {
  return useQuery<WorkspacePathChildrenResponse>({
    queryKey: ['workspace-path-children', path],
    queryFn: () => naviFetch(`/api/workspaces/path-children?path=${encodeURIComponent(path)}`, WorkspacePathChildrenResponseSchema),
    enabled: enabled && Boolean(path.trim()),
    staleTime: 10_000,
  });
}

export function useSetWorkspaceMode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (mode: 'global' | 'scoped' | 'hybrid') => naviFetch('/api/workspace-mode', WorkspaceModeResponseSchema, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode }),
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['workspace-mode'] });
      queryClient.invalidateQueries({ queryKey: ['active-workspace'] });
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
    },
  });
}

export function useSetActiveWorkspace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (workspaceId: string) => naviFetch('/api/workspaces/active', ActiveWorkspaceResponseSchema, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ workspace_id: workspaceId }),
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['active-workspace'] });
      queryClient.invalidateQueries({ queryKey: ['workspace-mode'] });
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      queryClient.invalidateQueries({ queryKey: ['activity'] });
    },
  });
}

export function useCreateWorkspace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: WorkspaceUpsertPayload) => naviFetch('/api/workspaces', WorkspaceSchema, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }),
    onSuccess: (workspace) => {
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      queryClient.setQueryData(['workspace', workspace.workspace_id], workspace);
    },
  });
}

export function useUpdateWorkspace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ workspaceId, payload }: { workspaceId: string; payload: WorkspaceUpsertPayload }) => (
      naviFetch(`/api/workspaces/${workspaceId}`, WorkspaceSchema, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
    ),
    onSuccess: (workspace) => {
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      queryClient.invalidateQueries({ queryKey: ['active-workspace'] });
      queryClient.setQueryData(['workspace', workspace.workspace_id], workspace);
    },
  });
}

export function useArchiveWorkspace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (workspaceId: string) => naviFetch(`/api/workspaces/${workspaceId}/archive`, WorkspaceSchema, {
      method: 'POST',
    }),
    onSuccess: (workspace) => {
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      queryClient.invalidateQueries({ queryKey: ['active-workspace'] });
      queryClient.setQueryData(['workspace', workspace.workspace_id], workspace);
    },
  });
}

export function useDeleteWorkspace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (workspaceId: string) => naviFetch(`/api/workspaces/${workspaceId}`, undefined, {
      method: 'DELETE',
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      queryClient.invalidateQueries({ queryKey: ['active-workspace'] });
      queryClient.invalidateQueries({ queryKey: ['workspace-mode'] });
    },
  });
}

export function useCreateWhitelistRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ workspaceId, scope, actionTypes }: { workspaceId: string; scope: string; actionTypes: string[] }) => (
      naviFetch(`/api/workspaces/${workspaceId}/whitelist-rules`, WhitelistRuleSchema, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ scope, action_types: actionTypes }),
      })
    ),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: ['workspace-whitelist-rules', variables.workspaceId] });
      queryClient.invalidateQueries({ queryKey: ['workspace', variables.workspaceId] });
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
    },
  });
}

export function useRevokeWhitelistRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ workspaceId, ruleId }: { workspaceId: string; ruleId: string }) => (
      naviFetch(`/api/workspaces/${workspaceId}/whitelist-rules/${ruleId}/revoke`, WhitelistRuleSchema, {
        method: 'POST',
      })
    ),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: ['workspace-whitelist-rules', variables.workspaceId] });
      queryClient.invalidateQueries({ queryKey: ['workspace', variables.workspaceId] });
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
    },
  });
}

export function useResolveWorkspaceBoundary() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: BoundaryResolutionPayload) => naviFetch('/api/workspaces/boundary/resolve', undefined, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: ['active-workspace'] });
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      if (variables.workspace_id) {
        queryClient.invalidateQueries({ queryKey: ['workspace-whitelist-rules', variables.workspace_id] });
        queryClient.invalidateQueries({ queryKey: ['workspace', variables.workspace_id] });
      }
    },
  });
}

export function activeRules(rules: WhitelistRule[] | undefined): WhitelistRule[] {
  return (rules ?? []).filter((rule) => rule.status === 'active');
}
