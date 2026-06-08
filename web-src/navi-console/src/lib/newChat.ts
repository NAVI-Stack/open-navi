export const DRAFT_CHAT_ID = 'new';

export function isDraftChatId(id: string | null | undefined): boolean {
  return id === DRAFT_CHAT_ID;
}

export function newChatPath(projectId?: string): string {
  if (projectId) {
    return `/projects/${projectId}/chats/${DRAFT_CHAT_ID}`;
  }
  return `/chats/${DRAFT_CHAT_ID}`;
}

export type NavigateFn = (path: string, replace?: boolean) => void;

export function openNewChat(
  navigate: NavigateFn,
  options?: { projectId?: string; replace?: boolean; compose?: string }
): void {
  const params = new URLSearchParams();
  const compose = options?.compose?.trim();
  if (compose) {
    params.set('compose', compose);
  }
  // Bust in-place draft state when reopening New Conversation on the same route.
  params.set('fresh', String(Date.now()));
  const path = `${newChatPath(options?.projectId)}?${params.toString()}`;
  navigate(path, options?.replace ?? false);
}

export function isOnDraftPath(path: string, projectId?: string): boolean {
  const base = newChatPath(projectId);
  return path === base || path.startsWith(`${base}?`);
}

// Re-exported for backward compatibility; the canonical value lives in
// chatTimeouts.ts so the transport stream timeout and the draft wait budget
// can never drift apart again.
import { FRONTEND_CHAT_WAIT_BUDGET_MS } from '@/lib/chatTimeouts';

const DRAFT_RESPONSE_TIMEOUT_MS = FRONTEND_CHAT_WAIT_BUDGET_MS;

export { DRAFT_RESPONSE_TIMEOUT_MS };
