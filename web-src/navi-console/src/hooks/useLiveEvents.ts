import { useState, useEffect, useRef, useCallback } from 'react';
import type { ConnectionState, LiveFrame, LiveEvent, LiveEventFrame } from '@/types/events';

interface UseLiveEventsConfig {
  chatId?: string;
  enabled?: boolean;
  bufferSize?: number;
}

type LiveEventSubscriber = (event: LiveEvent) => void;

interface UseLiveEventsResult {
  events: LiveEvent[];
  connectionState: ConnectionState;
  lastError: string | null;
  send: (method: string, params?: Record<string, unknown>) => void;
  subscribe: (handler: LiveEventSubscriber) => () => void;
}

const BASE_DELAY = 1000;
const MAX_DELAY = 30_000;
const MULTIPLIER = 2;

function jitter(ms: number): number {
  return ms * (0.9 + Math.random() * 0.2);
}

export function useLiveEvents(config: UseLiveEventsConfig): UseLiveEventsResult {
  const { chatId, enabled = true, bufferSize = 200 } = config;
  const [connectionState, setConnectionState] = useState<ConnectionState>('disconnected');
  const [events, setEvents] = useState<LiveEvent[]>([]);
  const [lastError, setLastError] = useState<string | null>(null);

  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const reconnectDelay = useRef(BASE_DELAY);
  const connectIdRef = useRef<string>('');
  const eventsRef = useRef<LiveEvent[]>([]);
  const lastSeqRef = useRef<number>(0);
  const seenSeqsRef = useRef<Set<number>>(new Set());
  const chatIdForSeqRef = useRef<string | undefined>(undefined);
  const subscribersRef = useRef<Set<LiveEventSubscriber>>(new Set());

  const subscribe = useCallback((handler: LiveEventSubscriber): (() => void) => {
    subscribersRef.current.add(handler);
    return () => subscribersRef.current.delete(handler);
  }, []);

  const pushEvent = useCallback((frame: LiveFrame) => {
    const entry: LiveEvent = {
      id: crypto.randomUUID(),
      receivedAt: Date.now(),
      frame,
    };
    for (const sub of subscribersRef.current) {
      sub(entry);
    }
    eventsRef.current = [...eventsRef.current.slice(-(bufferSize - 1)), entry];
    setEvents(eventsRef.current);
  }, [bufferSize]);

  const send = useCallback((method: string, params: Record<string, unknown> = {}) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    const id = `${method}-${crypto.randomUUID()}`;
    ws.send(JSON.stringify({ type: 'req', id, method, params }));
  }, []);

  useEffect(() => {
    // Not enabled at all — truly disconnected.
    if (!enabled) {
      setConnectionState('disconnected');
      return;
    }

    // Enabled but no chat — waiting (not an error).
    if (!chatId) {
      setConnectionState('waiting');
      return;
    }

    if (chatId !== chatIdForSeqRef.current) {
      lastSeqRef.current = 0;
      seenSeqsRef.current.clear();
      chatIdForSeqRef.current = chatId;
    }

    let unmounted = false;

    function connect() {
      if (unmounted) return;

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const ws = new WebSocket(`${protocol}//${window.location.host}/ws/live`);
      wsRef.current = ws;
      setConnectionState('connecting');

      ws.onopen = () => {
        const id = `connect-${crypto.randomUUID()}`;
        connectIdRef.current = id;
        ws.send(JSON.stringify({
          type: 'req',
          id,
          method: 'connect',
          params: { chat_id: chatId, after_seq: lastSeqRef.current },
        }));
      };

      ws.onmessage = (evt) => {
        let parsed: LiveFrame;
        try {
          parsed = JSON.parse(evt.data);
        } catch {
          // Never crash on unparseable frames — log and skip.
          console.warn('useLiveEvents: unparseable frame', evt.data);
          return;
        }

        // Guard against null/undefined parsed result.
        if (!parsed || typeof parsed !== 'object') {
          console.warn('useLiveEvents: non-object frame', parsed);
          return;
        }

        const frameType = (parsed as Record<string, unknown>).type;

        switch (frameType) {
          case 'res': {
            const res = parsed as { type: string; id: string; ok: boolean; error?: string };
            if (res.id === connectIdRef.current) {
              if (res.ok) {
                setConnectionState('connected');
                setLastError(null);
                reconnectDelay.current = BASE_DELAY;
              } else {
                setLastError(res.error ?? 'Connect rejected');
                setConnectionState('error');
                ws.close();
              }
            }
            break;
          }
          case 'event': {
            const eventFrame = parsed as LiveEventFrame;
            const seq = typeof eventFrame.event?.seq === 'number' ? eventFrame.event.seq : 0;
            if (seq > 0) {
              if (seenSeqsRef.current.has(seq)) break;
              seenSeqsRef.current.add(seq);
              if (seq > lastSeqRef.current) lastSeqRef.current = seq;
              if (seenSeqsRef.current.size > 500) {
                const first = seenSeqsRef.current.values().next().value;
                if (first !== undefined) seenSeqsRef.current.delete(first);
              }
            }
            pushEvent(parsed);
            break;
          }
          default:
            // Accept any frame type — presence, unknown, etc.
            // Push everything into the buffer so the UI can display it.
            if (typeof frameType === 'string' && frameType.length > 0) {
              pushEvent(parsed);
            } else {
              console.warn('useLiveEvents: frame without type', parsed);
              pushEvent(parsed);
            }
            break;
        }
      };

      ws.onclose = () => {
        if (unmounted) return;
        setConnectionState('reconnecting');
        const delay = jitter(reconnectDelay.current);
        reconnectDelay.current = Math.min(reconnectDelay.current * MULTIPLIER, MAX_DELAY);
        reconnectTimer.current = setTimeout(connect, delay);
      };

      ws.onerror = () => {
        setLastError('WebSocket error');
      };
    }

    connect();

    return () => {
      unmounted = true;
      clearTimeout(reconnectTimer.current);
      if (wsRef.current) {
        wsRef.current.onclose = null;
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [chatId, enabled, pushEvent]);

  return { events, connectionState, lastError, send, subscribe };
}
