import type { LiveEvent } from '@/types/events';

export const NAVI_CHAT_TERMINAL_EVENT_TYPES = new Set([
  'assistant.message.completed',
  'run.completed',
  'run.failed',
  'run.cancelled',
]);

export interface NaviChatEvent {
  type: string;
  payload: Record<string, unknown>;
  chatId?: string;
}

export function extractNaviChatEvent(event: LiveEvent): NaviChatEvent | null {
  const frame = event.frame;
  if (!frame || typeof frame !== 'object') return null;
  if (!('event' in frame)) return null;
  const ev = (frame as { event?: unknown }).event;
  if (!ev || typeof ev !== 'object') return null;
  const evObj = ev as Record<string, unknown>;
  const type = typeof evObj.type === 'string' ? evObj.type : '';
  if (!type) return null;
  const payload =
    evObj.payload && typeof evObj.payload === 'object'
      ? (evObj.payload as Record<string, unknown>)
      : {};
  const chatId =
    typeof evObj.chat_id === 'string' && evObj.chat_id
      ? evObj.chat_id
      : typeof evObj.correlation_id === 'string' && evObj.correlation_id
        ? evObj.correlation_id
        : typeof payload.chat_id === 'string' && payload.chat_id
          ? payload.chat_id
          : typeof payload.runtime_session_id === 'string' && payload.runtime_session_id
            ? payload.runtime_session_id
            : undefined;
  return { type, payload, chatId };
}

export interface ListenForNaviAssistantReplyOptions {
  onPartial: (content: string) => void;
  onTerminal: (content: string) => void;
  onError?: (message: string) => void;
}

type SubscribeFn = (handler: (event: LiveEvent) => void) => () => void;

/**
 * Subscribes to NAVI live assistant reply events for a chat until a terminal event.
 * Used by ceremony step actions (non-transport) and mirrors NaviChatTransport semantics.
 */
export function listenForNaviAssistantReply(
  subscribe: SubscribeFn,
  chatId: string,
  options: ListenForNaviAssistantReplyOptions,
): () => void {
  let finished = false;
  let lastContent = '';

  const finish = (content: string) => {
    if (finished) return;
    finished = true;
    unsubscribe();
    if (content) options.onTerminal(content);
    else options.onTerminal(lastContent);
  };

  const unsubscribe = subscribe((event: LiveEvent) => {
    if (finished) return;
    const ev = extractNaviChatEvent(event);
    if (!ev || ev.chatId !== chatId) return;

    if (ev.type === 'assistant.message.partial') {
      const content = typeof ev.payload.content === 'string' ? ev.payload.content : '';
      if (content) {
        lastContent = content;
        options.onPartial(content);
      }
      return;
    }

    if (ev.type === 'assistant.message.completed') {
      const content =
        typeof ev.payload.content === 'string' ? ev.payload.content : lastContent;
      if (content) {
        lastContent = content;
        options.onPartial(content);
      }
      finish(content);
      return;
    }

    if (ev.type === 'run.failed' || ev.type === 'run.cancelled') {
      const message =
        typeof ev.payload.error === 'string'
          ? ev.payload.error
          : `NAVI run ${ev.type}`;
      options.onError?.(message);
      finish(lastContent);
      return;
    }

    if (ev.type === 'run.completed') {
      finish(lastContent);
    }
  });

  return () => {
    finished = true;
    unsubscribe();
  };
}
