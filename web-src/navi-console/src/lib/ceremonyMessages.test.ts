import { describe, expect, it } from 'vitest';

function compareChatMessageOrder(
  a: { id?: string; createdAt?: string; created_at?: string },
  b: { id?: string; createdAt?: string; created_at?: string },
): number {
  const ta = messageTimestamp(a);
  const tb = messageTimestamp(b);
  if (ta !== tb) return ta - tb;
  return (a.id ?? '').localeCompare(b.id ?? '');
}

function messageTimestamp(msg: { createdAt?: string; created_at?: string }): number {
  const raw = msg.createdAt ?? msg.created_at;
  if (!raw) return 0;
  const parsed = Date.parse(raw);
  return Number.isNaN(parsed) ? 0 : parsed;
}

describe('ceremony message ordering', () => {
  it('sorts messages chronologically with id tie-break', () => {
    const sorted = [
      { id: 'b', created_at: '2026-06-04T12:00:01Z', content: 'second' },
      { id: 'a', created_at: '2026-06-04T12:00:00Z', content: 'first' },
      { id: 'c', created_at: '2026-06-04T12:00:02Z', content: 'third' },
    ].sort(compareChatMessageOrder);

    expect(sorted.map((m) => m.content)).toEqual(['first', 'second', 'third']);
  });
});
