// Renderer-neutral NaviDataView TS types — the frontend mirror of the Go
// internal/navi/render types. NAVI owns this semantic model; OpenUI Lang is one
// render target derived from it. These types are shared by NAVI-native render
// components and the console.

export type NaviDataColumnType = 'string' | 'number' | 'datetime' | 'boolean' | 'duration';

export interface NaviDataColumn {
  key: string;
  label: string;
  type: NaviDataColumnType;
}

export interface NaviDataset {
  columns: NaviDataColumn[];
  rows: Array<Record<string, unknown>>;
}

export interface NaviSuggestedView {
  type: string;
  rationale: string;
}

export interface NaviDataViewGovernance {
  dataSensitivity: 'public' | 'local' | 'private' | 'sensitive';
  allowedActions: string[];
  auditLevel: 'none' | 'basic' | 'sensitive';
}

export interface NaviDataView {
  id: string;
  title: string;
  intent: 'chart' | 'table' | 'dashboard' | 'timeline' | 'inspector' | 'form';
  dataset: NaviDataset;
  suggestedViews?: NaviSuggestedView[];
  actions?: unknown[];
  governance?: NaviDataViewGovernance;
  fallback?: { markdown?: string; summary?: string };
  trace?: { runId?: string; messageId?: string; chatId?: string; sourceSkillIds?: string[] };
}

/** First column of the given semantic type, for deriving chart axes from a view. */
export function firstColumnOfType(view: NaviDataView, type: NaviDataColumnType): NaviDataColumn | undefined {
  return view.dataset.columns.find((c) => c.type === type);
}
