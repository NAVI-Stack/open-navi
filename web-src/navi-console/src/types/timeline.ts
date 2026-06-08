import type { ActivityItem, DebugEventItem, ErrorRecord } from '@/types/api';
import type { LiveEvent } from '@/types/events';

export type TimelineSource = 'live' | 'activity' | 'debug' | 'error';
export type TimelineVariant = 'muted' | 'accent' | 'warning' | 'danger' | 'success';

export interface TimelineItem {
  id: string;
  source: TimelineSource;
  timestamp: string;
  type: string;
  summary: string;
  variant: TimelineVariant;
  raw: unknown;
}

/** Keys that look like secrets — values will be redacted in JSON display. */
const SECRET_KEYS = new Set([
  'token', 'secret', 'password', 'api_key', 'apikey',
  'authorization', 'credential', 'credentials',
  'access_token', 'refresh_token', 'private_key',
]);

/** Recursively redact values for keys that look like secrets. */
export function redactSecrets(obj: unknown): unknown {
  if (obj === null || obj === undefined) return obj;
  if (Array.isArray(obj)) return obj.map(redactSecrets);
  if (typeof obj === 'object') {
    const out: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(obj as Record<string, unknown>)) {
      if (SECRET_KEYS.has(key.toLowerCase())) {
        out[key] = '[REDACTED]';
      } else {
        out[key] = redactSecrets(value);
      }
    }
    return out;
  }
  return obj;
}

let idCounter = 0;
function nextId(prefix: string): string {
  return `${prefix}-${++idCounter}`;
}

const activityVariants: Record<string, TimelineVariant> = {
  run_started: 'accent',
  run_completed: 'success',
  run_failed: 'danger',
  proposal_created: 'warning',
  proposal_resolved: 'success',
  connector_error: 'danger',
  connector_recovered: 'success',
  governor_tripped: 'danger',
  governor_recovered: 'success',
  message_sent: 'accent',
  message_received: 'accent',
  directive_executed: 'accent',
};

export function activityToTimelineItem(item: ActivityItem): TimelineItem {
  return {
    id: item.id ?? nextId('act'),
    source: 'activity',
    timestamp: item.at ?? new Date(0).toISOString(),
    type: item.type ?? 'unknown',
    summary: item.summary ?? 'Activity event',
    variant: activityVariants[item.type] ?? 'muted',
    raw: item,
  };
}

export function debugEventToTimelineItem(item: DebugEventItem): TimelineItem {
  return {
    id: item.id ?? nextId('dbg'),
    source: 'debug',
    timestamp: item.timestamp ?? new Date(0).toISOString(),
    type: item.type ?? item.kind ?? 'unknown',
    summary: [item.type, item.kind, item.source_agent].filter(Boolean).join(' · ') || 'Debug event',
    variant: 'muted',
    raw: item,
  };
}

export function errorToTimelineItem(item: ErrorRecord): TimelineItem {
  const severity = (item.severity ?? '').toLowerCase();
  let variant: TimelineVariant = 'danger';
  if (severity === 'warning' || severity === 'warn') variant = 'warning';

  return {
    id: item.id ?? nextId('err'),
    source: 'error',
    timestamp: item.timestamp ?? new Date(0).toISOString(),
    type: item.error_type ?? item.type ?? 'error',
    summary: item.message ?? `${item.component ?? 'unknown'} error`,
    variant,
    raw: item,
  };
}

export function liveEventToTimelineItem(event: LiveEvent): TimelineItem {
  const frame = event.frame as Record<string, unknown>;
  const frameType = String(frame.type ?? 'unknown');

  // Try to extract useful summary from event frames.
  let summary = frameType;
  let type = frameType;

  if (frameType === 'event' && frame.event && typeof frame.event === 'object') {
    const ev = frame.event as Record<string, unknown>;
    type = String(ev.type ?? 'event');
    summary = [ev.type, ev.subject].filter(Boolean).join(': ') || 'Live event';
  } else if (frameType.startsWith('presence.')) {
    summary = frameType;
  }

  return {
    id: event.id,
    source: 'live',
    timestamp: new Date(event.receivedAt).toISOString(),
    type,
    summary,
    variant: 'accent',
    raw: event.frame,
  };
}

/**
 * Merge and sort timeline items by timestamp descending (newest first).
 * De-dupes by id.
 */
export function mergeTimeline(...arrays: TimelineItem[][]): TimelineItem[] {
  const seen = new Set<string>();
  const result: TimelineItem[] = [];
  for (const arr of arrays) {
    for (const item of arr) {
      if (!seen.has(item.id)) {
        seen.add(item.id);
        result.push(item);
      }
    }
  }
  result.sort((a, b) => {
    const ta = new Date(a.timestamp).getTime();
    const tb = new Date(b.timestamp).getTime();
    return tb - ta; // newest first
  });
  return result;
}
