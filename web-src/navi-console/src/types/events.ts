export type ConnectionState =
  | 'disconnected'
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'error'
  | 'waiting';

export interface LiveRequestFrame {
  type: 'req';
  id: string;
  method: string;
  params: Record<string, unknown>;
}

export interface LiveResponseFrame {
  type: 'res';
  id: string;
  ok: boolean;
  error?: string;
  result?: unknown;
}

export interface LiveEventFrame {
  type: 'event';
  event: {
    id?: string;
    type?: string;
    seq?: number;
    subject?: string;
    data?: unknown;
    timestamp?: string;
    [key: string]: unknown;
  };
}

export interface LivePresenceFrame {
  type: string; // e.g. "presence.snapshot", "presence.navi.updated"
  sent_at?: string;
  data?: unknown;
}

export type LiveFrame =
  | LiveResponseFrame
  | LiveEventFrame
  | LivePresenceFrame
  | { type: string; [key: string]: unknown };

export interface LiveEvent {
  id: string;
  receivedAt: number;
  frame: LiveFrame;
}
