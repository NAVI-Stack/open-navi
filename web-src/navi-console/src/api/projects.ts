import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';
import { naviFetch } from './client';
import { hasChatMessages, normalizeChat } from './chats';
import {
  ProjectListResponseSchema,
  ProjectReadinessSchema,
  ProjectSchema,
  ProjectChatsResponseSchema,
  ProjectTasksResponseSchema,
  ChatEntrySchema,
  type ChatEntry,
  type ProjectChatsResponse,
  type Project,
  type ProjectListResponse,
  type ProjectReadiness,
  type ProjectTask,
  type ProjectTasksResponse,
} from '@/types/api';

export type ProjectUpsertPayload = {
  project_id?: string;
  title: string;
  slug?: string;
  description?: string;
  project_kind?: string;
  status?: string;
  health?: string;
  workspace_id?: string;
  icon?: string;
  color?: string;
  memory_scope?: string;
  attributes?: Record<string, unknown>;
};

export type ProjectTaskPayload = {
  title: string;
  raw_input: string;
  description?: string;
  task_class?: string;
  assigned_to?: string;
  risk?: string;
  acceptance_target?: string;
};

export function useProjects(includeArchived = false) {
  const url = includeArchived ? '/api/projects?include_archived=true' : '/api/projects';
  return useQuery<ProjectListResponse>({
    queryKey: ['projects', includeArchived],
    queryFn: () => naviFetch(url, ProjectListResponseSchema),
    refetchInterval: 15_000,
  });
}

export function useProject(projectId: string | null) {
  return useQuery<Project>({
    queryKey: ['project', projectId],
    queryFn: () => {
      if (!projectId) throw new Error('No project ID');
      return naviFetch(`/api/projects/${projectId}`, ProjectSchema);
    },
    enabled: !!projectId,
    refetchInterval: 15_000,
  });
}

export function useProjectReadiness(projectId: string | null) {
  return useQuery<ProjectReadiness>({
    queryKey: ['project-readiness', projectId],
    queryFn: () => {
      if (!projectId) throw new Error('No project ID');
      return naviFetch(`/api/projects/${projectId}/readiness`, ProjectReadinessSchema);
    },
    enabled: !!projectId,
    refetchInterval: 15_000,
  });
}

export function useProjectTasks(projectId: string | null) {
  return useQuery<ProjectTasksResponse>({
    queryKey: ['project-tasks', projectId],
    queryFn: () => {
      if (!projectId) throw new Error('No project ID');
      return naviFetch(`/api/projects/${projectId}/tasks`, ProjectTasksResponseSchema);
    },
    enabled: !!projectId,
    refetchInterval: 15_000,
  });
}

export function useProjectChats(projectId: string | null) {
  return useQuery<ProjectChatsResponse>({
    queryKey: ['project-chats', projectId],
    queryFn: async () => {
      if (!projectId) throw new Error('No project ID');
      const res = await naviFetch(`/api/projects/${projectId}/chats`, ProjectChatsResponseSchema);
      return {
        ...res,
        items: (res.items ?? []).map(normalizeChat).filter(hasChatMessages),
      };
    },
    enabled: !!projectId,
    refetchInterval: 15_000,
  });
}

export function useCreateProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: ProjectUpsertPayload) => naviFetch('/api/projects', ProjectSchema, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }),
    onSuccess: (project) => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      queryClient.setQueryData(['project', project.project_id], project);
    },
  });
}

export function useUpdateProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ projectId, payload }: { projectId: string; payload: ProjectUpsertPayload }) => (
      naviFetch(`/api/projects/${projectId}`, ProjectSchema, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
    ),
    onSuccess: (project) => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      queryClient.invalidateQueries({ queryKey: ['project-readiness', project.project_id] });
      queryClient.setQueryData(['project', project.project_id], project);
    },
  });
}

export function useArchiveProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (projectId: string) => naviFetch(`/api/projects/${projectId}/archive`, ProjectSchema, {
      method: 'POST',
    }),
    onSuccess: (project) => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      queryClient.setQueryData(['project', project.project_id], project);
    },
  });
}

export function useDeleteProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (projectId: string) => naviFetch(`/api/projects/${projectId}`, z.any(), {
      method: 'DELETE',
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      queryClient.invalidateQueries({ queryKey: ['active-workspace'] });
    },
  });
}

export function useBindProjectWorkspace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ projectId, workspaceId }: { projectId: string; workspaceId: string }) => (
      naviFetch(`/api/projects/${projectId}/workspace-binding`, ProjectSchema, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ workspace_id: workspaceId }),
      })
    ),
    onSuccess: (project) => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      queryClient.invalidateQueries({ queryKey: ['active-workspace'] });
      queryClient.invalidateQueries({ queryKey: ['project-readiness', project.project_id] });
      queryClient.setQueryData(['project', project.project_id], project);
    },
  });
}

export function useUnbindProjectWorkspace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (projectId: string) => naviFetch(`/api/projects/${projectId}/workspace-binding`, ProjectSchema, {
      method: 'DELETE',
    }),
    onSuccess: (project) => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      queryClient.invalidateQueries({ queryKey: ['workspaces'] });
      queryClient.invalidateQueries({ queryKey: ['active-workspace'] });
      queryClient.invalidateQueries({ queryKey: ['project-readiness', project.project_id] });
      queryClient.setQueryData(['project', project.project_id], project);
    },
  });
}

export function useCreateProjectChat() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (projectId: string) => {
      const res = await naviFetch(`/api/projects/${projectId}/chats`, ChatEntrySchema, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      });
      return normalizeChat(res);
    },
    onSuccess: (_, projectId) => {
      queryClient.invalidateQueries({ queryKey: ['chats'] });
      queryClient.invalidateQueries({ queryKey: ['project-chats', projectId] });
      queryClient.invalidateQueries({ queryKey: ['active-workspace'] });
    },
  });
}

export function useCreateProjectTask() {
  const queryClient = useQueryClient();
  return useMutation<ProjectTask, Error, { projectId: string; payload: ProjectTaskPayload }>({
    mutationFn: ({ projectId, payload }) => naviFetch(`/api/projects/${projectId}/tasks`, z.any(), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: ['project-tasks', variables.projectId] });
      queryClient.invalidateQueries({ queryKey: ['activity'] });
    },
  });
}

export function chatId(chat: ChatEntry): string {
  return chat.chat_id;
}
