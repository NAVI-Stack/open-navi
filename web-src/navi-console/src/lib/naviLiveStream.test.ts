import { describe, expect, it, vi } from 'vitest';
import type { LiveEvent } from '@/types/events';
import { extractNaviChatEvent, listenForNaviAssistantReply } from './naviLiveStream';

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

describe('listenForNaviAssistantReply', () => {
  it('streams partial snapshots then terminal content', () => {
    let handler: (event: LiveEvent) => void = () => {
      throw new Error('subscribe handler was not registered');
    };
    const partials: string[] = [];
    let terminal = '';

    const stop = listenForNaviAssistantReply(
      (next) => {
        handler = next;
        return vi.fn();
      },
      'chat-1',
      {
        onPartial: (content) => partials.push(content),
        onTerminal: (content) => {
          terminal = content;
        },
      },
    );

    handler(liveEvent('assistant.message.partial', { content: 'How would' }));
    handler(
      liveEvent('assistant.message.completed', {
        content: 'How would you prefer I conduct myself?',
      }),
    );
    handler(liveEvent('run.completed', { chat_id: 'chat-1' }));

    expect(partials).toEqual(['How would', 'How would you prefer I conduct myself?']);
    expect(terminal).toBe('How would you prefer I conduct myself?');
    stop();
  });
});

describe('extractNaviChatEvent', () => {
  it('resolves chat id from runtime_session_id in payload', () => {
    const ev = extractNaviChatEvent(
      liveEvent('assistant.message.partial', {
        content: 'hi',
        runtime_session_id: 'chat-99',
      }, ''),
    );
    expect(ev?.chatId).toBe('chat-99');
  });
});
