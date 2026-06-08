import { describe, expect, it } from 'vitest';
import { shouldSyncServerMessages } from './chatMessageSync';

const message = (id: string) => ({ id });

describe('shouldSyncServerMessages', () => {
  it('keeps the existing upward sync behavior when the server has more messages', () => {
    expect(
      shouldSyncServerMessages([message('a'), message('b')], [message('a')], 'ready'),
    ).toBe(true);
  });

  it('replaces stale local state when the server tail has changed', () => {
    expect(
      shouldSyncServerMessages(
        [message('server-user'), message('server-assistant')],
        [message('server-user'), message('optimistic-empty-assistant')],
        'ready',
      ),
    ).toBe(true);
  });

  it('does not replace in-flight local state while a stream is active', () => {
    expect(
      shouldSyncServerMessages(
        [message('server-user'), message('server-assistant')],
        [message('server-user'), message('optimistic-empty-assistant')],
        'streaming',
      ),
    ).toBe(false);
  });
});
