import { useQuery } from '@tanstack/react-query';
import { z } from 'zod';
import { naviFetch } from './client';
import {
  ChatEntrySchema,
  ChatSendResponseSchema,
  ChatThreadSchema,
  RuntimeSummarySchema,
  type ChatEntry,
  type ChatSendResponse,
  type ChatThread,
  type ChatThreadView,
  MessageVariantsSchema,
  type MessageVariants,
  type RuntimeSummary,
} from '@/types/api';

const RawChatListSchema = z.array(ChatEntrySchema).nullable();

export function normalizeChat(chat: ChatEntry): ChatEntry {
  const chatId = chat.chat_id || chat.id || '';
  return {
    ...chat,
    id: chat.id || chatId,
    chat_id: chatId,
    project_id: chat.project_id ?? chat.projectId,
    created_at: chat.created_at ?? chat.createdAt,
    updated_at: chat.updated_at ?? chat.updatedAt,
  };
}

function assistantMessageCount(chat: ChatEntry): number {
  const raw = chat as ChatEntry & { assistantMessageCount?: number; assistant_message_count?: number };
  return raw.assistantMessageCount ?? raw.assistant_message_count ?? 0;
}

/** Chats appear in recents only after NAVI has replied at least once. */
export function hasChatMessages(chat: ChatEntry): boolean {
  return assistantMessageCount(chat) > 0;
}

function normalizeThread(thread: ChatThread): ChatThreadView {
  const chat = normalizeChat(thread.chat);
  return {
    ...chat,
    messages: thread.messages ?? [],
  };
}

export function useChats(projectId?: string) {
  const url = projectId ? `/api/navi/chats?project_id=${projectId}` : '/api/navi/chats';
  return useQuery<ChatEntry[]>({
    queryKey: ['chats', projectId],
    queryFn: async () => {
      const chats = await naviFetch<ChatEntry[] | null>(url, RawChatListSchema);
      return (chats ?? []).map(normalizeChat).filter(hasChatMessages);
    },
    refetchInterval: 15_000,
  });
}

export function useChatThread(id: string | null, options?: { refetchInterval?: number }) {
  const url = id ? `/api/navi/chats/${id}` : null;
  return useQuery<ChatThreadView>({
    queryKey: ['chat', id],
    queryFn: async () => {
      if (!url) throw new Error('No ID');
      return normalizeThread(await naviFetch(url, ChatThreadSchema));
    },
    enabled: !!id,
    refetchInterval: options?.refetchInterval ?? 15_000,
  });
}

export function useChatRuntimeSummary(id: string | null) {
  const url = id ? `/api/navi/chats/${id}/runtime_summary` : null;
  return useQuery<RuntimeSummary>({
    queryKey: ['chat-runtime-summary', id],
    queryFn: () => {
      if (!url) throw new Error('No ID');
      return naviFetch(url, RuntimeSummarySchema);
    },
    enabled: !!id,
    refetchInterval: 15_000,
  });
}

export async function archiveChat(id: string): Promise<void> {
  await naviFetch(`/api/navi/chats/${id}/archive`, z.any(), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ archive: true }),
  });
}

export async function deleteChat(id: string): Promise<void> {
  await naviFetch(`/api/navi/chats/${id}`, z.any(), {
    method: 'DELETE',
  });
}

export async function renameChat(id: string, title: string): Promise<void> {
  await naviFetch(`/api/navi/chats/${id}`, z.any(), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  });
}

export async function updateChat(id: string, updates: { title?: string; project_id?: string | null }): Promise<void> {
  await naviFetch(`/api/navi/chats/${id}`, z.any(), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(updates),
  });
}

export async function createChat(projectId?: string, initialMessage?: string): Promise<ChatEntry> {
  const body: Record<string, string> = {};
  if (projectId?.trim()) {
    body.project_id = projectId.trim();
  }
  if (initialMessage?.trim()) {
    body.initial_message = initialMessage.trim();
  }
  return normalizeChat(await naviFetch('/api/navi/chats', ChatEntrySchema, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }));
}

export async function sendChatMessage(id: string, text: string, systemContext?: string): Promise<ChatSendResponse> {
  return naviFetch(`/api/navi/chats/${id}/message`, ChatSendResponseSchema, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content: text, system_context: systemContext }),
  });
}

/** Edits a prior user message and re-runs the turn (truncates the thread from it). */
export async function editAndResendMessage(
  chatId: string,
  messageId: string,
  content: string,
): Promise<void> {
  await naviFetch(
    `/api/navi/chats/${encodeURIComponent(chatId)}/messages/${encodeURIComponent(messageId)}/edit-resend`,
    z.any(),
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content }),
    },
  );
}

/** Re-runs the most recent user turn to produce a fresh assistant reply. */
export async function regenerateLastReply(chatId: string): Promise<void> {
  await naviFetch(`/api/navi/chats/${encodeURIComponent(chatId)}/regenerate`, z.any(), {
    method: 'POST',
  });
}

/** Continues the most recent assistant reply. */
export async function continueLastReply(chatId: string): Promise<void> {
  await naviFetch(`/api/navi/chats/${encodeURIComponent(chatId)}/continue`, z.any(), {
    method: 'POST',
  });
}

export async function getMessageVariants(chatId: string, messageId: string): Promise<MessageVariants> {
  return naviFetch(
    `/api/navi/chats/${encodeURIComponent(chatId)}/messages/${encodeURIComponent(messageId)}/variants`,
    MessageVariantsSchema,
  );
}

export async function selectMessageVariant(chatId: string, messageId: string, index: number): Promise<void> {
  await naviFetch(
    `/api/navi/chats/${encodeURIComponent(chatId)}/messages/${encodeURIComponent(messageId)}/variants/select`,
    z.any(),
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ index }),
    },
  );
}

/** Records owner feedback (👍/👎) on an assistant message; rating null clears it. */
export async function sendMessageFeedback(
  chatId: string,
  messageId: string,
  rating: 'up' | 'down' | null,
): Promise<void> {
  await naviFetch(
    `/api/navi/chats/${encodeURIComponent(chatId)}/messages/${encodeURIComponent(messageId)}/feedback`,
    z.any(),
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ rating }),
    },
  );
}
