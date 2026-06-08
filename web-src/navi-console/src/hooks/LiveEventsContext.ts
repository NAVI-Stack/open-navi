import { createContext, useContext } from 'react';
import type { ConnectionState, LiveEvent } from '@/types/events';

export type LiveEventSubscriber = (event: LiveEvent) => void;

export interface LiveEventsContextValue {
  events: LiveEvent[];
  connectionState: ConnectionState;
  lastError: string | null;
  send: (method: string, params?: Record<string, unknown>) => void;
  subscribe: (handler: LiveEventSubscriber) => () => void;
}

const defaultValue: LiveEventsContextValue = {
  events: [],
  connectionState: 'disconnected',
  lastError: null,
  send: () => {},
  subscribe: () => () => {},
};

export const LiveEventsContext = createContext<LiveEventsContextValue>(defaultValue);

export function useLiveEventsContext(): LiveEventsContextValue {
  return useContext(LiveEventsContext);
}
