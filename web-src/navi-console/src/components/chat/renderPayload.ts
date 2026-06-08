// Chat render payload — the console mirror of the Go internal/navi/render
// RenderPayload. It is carried in an assistant message's metadata under
// "renderPayload" and tells the console which render lane to use.
//
// NAVI owns meaning (dataView); OpenUI Lang is one render target. fallbackMarkdown
// is always present so a failed/unavailable renderer still shows useful data.

import type { NaviDataView } from '@navi/ui';

export type ChatRenderMode =
  | 'text'
  | 'markdown'
  | 'artifact'
  | 'navi-ui'
  | 'openui'
  | 'system-card'
  | 'proposal';

export interface ChatRenderError {
  stage: string;
  message: string;
}

/** Provenance of the rendered data. "placeholder" for prototype/mockup renders. */
export type DataSourceKind = 'real' | 'placeholder' | 'mixed';

/**
 * Lightweight mirror of the Go render.NaviUIPrototype. Present on prototype
 * renders so the console can show an honest "prototype / placeholder" label. The
 * canonical meaning lives in NAVI; OpenUI Lang is one derived render target.
 */
export interface NaviUIPrototypeRef {
  id: string;
  title: string;
  purpose: string;
  dataSourceKind?: DataSourceKind;
  fallbackMarkdown?: string;
}

export interface ChatRenderPayload {
  mode: ChatRenderMode;
  sourceText?: string;
  naviUiSpec?: unknown;
  openuiLang?: string;
  dataView?: NaviDataView;
  /** Set on generated-UI prototype renders (mockups with placeholder data). */
  prototype?: NaviUIPrototypeRef;
  /** Provenance label; "placeholder" drives the prototype chip in the UI. */
  dataSourceKind?: DataSourceKind;
  fallbackMarkdown?: string;
  traceId?: string;
  errors?: ChatRenderError[];
}

/**
 * Parse a render payload off a message's metadata. Tolerant: returns null for
 * anything that isn't a recognizable payload so the message renders normally.
 */
export function parseRenderPayload(meta: unknown): ChatRenderPayload | null {
  if (!meta || typeof meta !== 'object') return null;
  const raw = (meta as Record<string, unknown>).renderPayload;
  if (!raw || typeof raw !== 'object') return null;
  const payload = raw as Record<string, unknown>;
  const mode = payload.mode;
  if (typeof mode !== 'string') return null;
  return payload as unknown as ChatRenderPayload;
}
