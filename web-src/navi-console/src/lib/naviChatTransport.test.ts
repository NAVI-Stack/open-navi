import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import type { UIMessage, UIMessageChunk } from 'ai';
import { NaviChatTransport } from './naviChatTransport';
import type { LiveEvent } from '@/types/events';

function liveEvent(type: string, payload: Record<string, unknown>, chatId = 'chat-1'): LiveEvent {
  return {
    id: `${type}-${Math.random()}`,
    receivedAt: Date.now(),
    frame: {
      type: 'event',
      event: {
        type,
        correlation_id: chatId,
        payload,
      },
    },
  };
}

async function readChunks(stream: ReadableStream<UIMessageChunk>): Promise<UIMessageChunk[]> {
  const reader = stream.getReader();
  const chunks: UIMessageChunk[] = [];
  for (;;) {
    const { done, value } = await reader.read();
    if (done) return chunks;
    chunks.push(value);
  }
}

function userMessage(text: string): UIMessage {
  return {
    id: 'u1',
    role: 'user',
    parts: [{ type: 'text', text }],
  } as UIMessage;
}

describe('NaviChatTransport', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('streams assistant.message.completed content before closing', async () => {
    let handler: (event: LiveEvent) => void = () => {
      throw new Error('subscribe handler was not registered');
    };
    const unsubscribe = vi.fn();
    const transport = new NaviChatTransport('chat-1', (next) => {
      handler = next;
      return unsubscribe;
    });

    const stream = await transport.sendMessages({
      trigger: 'submit-message',
      chatId: 'chat-1',
      messageId: 'assistant-1',
      messages: [userMessage('render a panel')],
      abortSignal: undefined,
    });

    handler(
      liveEvent('assistant.message.completed', {
        content: '{"t":"text","value":"Rendered without refresh"}',
      }),
    );

    const chunks = await readChunks(stream);
    expect(unsubscribe).toHaveBeenCalledTimes(1);
    expect(chunks).toContainEqual(
      expect.objectContaining({
        type: 'text-delta',
        delta: '{"t":"text","value":"Rendered without refresh"}',
      }),
    );
    expect(chunks.at(-1)).toEqual(expect.objectContaining({ type: 'finish', finishReason: 'stop' }));
  });

  it('closes the stream on run.completed when no assistant content was streamed', async () => {
    let handler: (event: LiveEvent) => void = () => {
      throw new Error('subscribe handler was not registered');
    };
    const transport = new NaviChatTransport('chat-1', (next) => {
      handler = next;
      return vi.fn();
    });

    const stream = await transport.sendMessages({
      trigger: 'submit-message',
      chatId: 'chat-1',
      messageId: 'assistant-1',
      messages: [userMessage('ej')],
      abortSignal: undefined,
    });

    handler(liveEvent('run.completed', { chat_id: 'chat-1' }));

    const chunks = await readChunks(stream);
    expect(chunks.at(-1)).toEqual(expect.objectContaining({ type: 'finish', finishReason: 'stop' }));
  });

  it('only streams the missing suffix when completion follows partial snapshots', async () => {
    let handler: (event: LiveEvent) => void = () => {
      throw new Error('subscribe handler was not registered');
    };
    const transport = new NaviChatTransport('chat-1', (next) => {
      handler = next;
      return vi.fn();
    });

    const stream = await transport.sendMessages({
      trigger: 'submit-message',
      chatId: 'chat-1',
      messageId: 'assistant-1',
      messages: [userMessage('render a panel')],
      abortSignal: undefined,
    });

    handler(liveEvent('assistant.message.partial', { content: 'hello' }));
    handler(liveEvent('assistant.message.completed', { content: 'hello world' }));

    const chunks = await readChunks(stream);
    const deltas = chunks
      .filter((chunk): chunk is Extract<UIMessageChunk, { type: 'text-delta' }> => chunk.type === 'text-delta')
      .map((chunk) => chunk.delta);

    expect(deltas).toEqual(['hello', ' world']);
  });
});
