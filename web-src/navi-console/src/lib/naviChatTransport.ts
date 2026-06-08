import type { ChatTransport, UIMessage, UIMessageChunk } from 'ai';
import type { LiveEvent } from '@/types/events';
import { FRONTEND_CHAT_WAIT_BUDGET_MS } from '@/lib/chatTimeouts';
import {
  extractNaviChatEvent,
  NAVI_CHAT_TERMINAL_EVENT_TYPES,
} from '@/lib/naviLiveStream';

type SubscribeFn = (handler: (event: LiveEvent) => void) => () => void;

const STREAM_TIMEOUT_MS = FRONTEND_CHAT_WAIT_BUDGET_MS;

/**
 * ChatTransport implementation that bridges NAVI's REST + WebSocket architecture
 * to the AI SDK's UIMessageChunk streaming protocol.
 *
 * Flow: sendMessage() → POST /api/navi/chats/{id}/message
 *       → subscribe to WebSocket events → stream UIMessageChunks back to useChat
 */
export class NaviChatTransport implements ChatTransport<UIMessage> {
  constructor(
    private readonly chatId: string,
    private readonly subscribe: SubscribeFn,
    private readonly getSystemContext?: () => string | undefined,
  ) {}

  async sendMessages(options: {
    trigger: 'submit-message' | 'regenerate-message';
    chatId: string;
    messageId: string | undefined;
    messages: UIMessage[];
    abortSignal: AbortSignal | undefined;
  }): Promise<ReadableStream<UIMessageChunk>> {
    const lastUserMsg = [...options.messages].reverse().find(m => m.role === 'user');
    if (!lastUserMsg) throw new Error('No user message to send');

    const textContent = lastUserMsg.parts
      .filter((p): p is { type: 'text'; text: string } => p.type === 'text')
      .map(p => p.text)
      .join('');

    if (!textContent.trim()) throw new Error('Empty message content');

    const reqBody: Record<string, unknown> = { content: textContent };
    if (this.getSystemContext) {
      const sysCtx = this.getSystemContext();
      if (sysCtx) {
        reqBody.system_context = sysCtx;
      }
    }

    const sendResp = await fetch(`/api/navi/chats/${this.chatId}/message`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(reqBody),
      credentials: 'include',
      signal: options.abortSignal,
    });

    if (!sendResp.ok) {
      const errText = await sendResp.text().catch(() => sendResp.statusText);
      throw new Error(`NAVI message send failed (${sendResp.status}): ${errText}`);
    }

    const chatId = this.chatId;
    const subscribeFn = this.subscribe;
    let lastContentLength = 0;
    let textBlockId: string | null = null;
    let unsubscribe: (() => void) | undefined;
    let timeoutId: ReturnType<typeof setTimeout> | undefined;

    return new ReadableStream<UIMessageChunk>({
      start(controller) {
        const messageId = crypto.randomUUID();

        controller.enqueue({ type: 'start', messageId });

        const emitContentSnapshot = (fullContent: string) => {
          const delta = fullContent.slice(lastContentLength);
          if (!delta) return;
          if (!textBlockId) {
            textBlockId = crypto.randomUUID();
            try { controller.enqueue({ type: 'text-start', id: textBlockId }); } catch {}
          }
          try { controller.enqueue({ type: 'text-delta', id: textBlockId, delta }); } catch {}
          lastContentLength = fullContent.length;
        };

        const done = (finishReason: 'stop' | 'error' = 'stop') => {
          unsubscribe?.();
          clearTimeout(timeoutId);
          if (textBlockId) {
            try { controller.enqueue({ type: 'text-end', id: textBlockId }); } catch {}
          }
          try { controller.enqueue({ type: 'finish-step' }); } catch {}
          try { controller.enqueue({ type: 'finish', finishReason }); } catch {}
          try { controller.close(); } catch {}
        };

        unsubscribe = subscribeFn((event: LiveEvent) => {
          const ev = extractNaviChatEvent(event);
          if (!ev || ev.chatId !== chatId) return;

          if (ev.type === 'assistant.message.partial') {
            const fullContent =
              typeof ev.payload.content === 'string' ? ev.payload.content : '';
            emitContentSnapshot(fullContent);
          } else if (NAVI_CHAT_TERMINAL_EVENT_TYPES.has(ev.type)) {
            if (ev.type === 'assistant.message.completed' && typeof ev.payload.content === 'string') {
              emitContentSnapshot(ev.payload.content);
            }
            done('stop');
          }
        });

        options.abortSignal?.addEventListener('abort', () => done('stop'));
        timeoutId = setTimeout(() => done('stop'), STREAM_TIMEOUT_MS);
      },
      cancel() {
        unsubscribe?.();
        clearTimeout(timeoutId);
      },
    });
  }

  async reconnectToStream(): Promise<ReadableStream<UIMessageChunk> | null> {
    return null;
  }
}
