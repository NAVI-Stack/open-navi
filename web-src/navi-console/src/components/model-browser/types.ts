export type ModelCapability =
  | 'fast'
  | 'vision'
  | 'reasoning'
  | 'tools'
  | 'image'
  | 'pdf'
  | 'code';

export type ModelStatus =
  | 'connected'
  | 'missing_key'
  | 'offline'
  | 'unavailable'
  | 'unknown';

export type SortMode =
  | 'recommended'
  | 'fastest'
  | 'cheapest'
  | 'reasoning'
  | 'coding'
  | 'local'
  | 'recent';

export interface BrowserModel {
  id: string;             // "${providerKey}/${modelId}" — "navi/auto" for NAVI Auto
  providerKey: string;
  providerName: string;
  modelId: string;
  displayName: string;
  capabilities: ModelCapability[];
  isLegacy: boolean;
  isRecommended: boolean;
  isNaviAuto: boolean;
  executionType: 'cloud' | 'local' | 'auto' | 'unknown';
  status: ModelStatus;
  tags: string[];
  speedScore: number;     // 0-100
  costScore: number;      // 0-100
  reasoningScore: number; // 0-100
  codingScore: number;    // 0-100
  agenticScore: number;   // 0-100
  chatScore: number;      // 0-100
  maxContextTokens: number;
  supportsTools: boolean;
  toolCallReliable: boolean;
  parameterSize?: string;
  family?: string;
  // LLM-KB learned calibration data (from GET /api/llm/profiles passthrough fields)
  learnedAgenticDelta?: number;
  learnedCodingDelta?: number;
  learnedChatDelta?: number;
  learnedReasoningDelta?: number;
  learnedEvidenceCount?: number;
}

export interface BrowserProvider {
  key: string;
  displayName: string;
  kind: 'local' | 'cloud' | 'proxy';
  enabled: boolean;
  healthy?: boolean;
  status: ModelStatus;
  modelCount: number;
}

export const CAPABILITY_META: Record<
  ModelCapability,
  { label: string; colorClass: string }
> = {
  fast: { label: 'Fast', colorClass: 'capFast' },
  vision: { label: 'Vision', colorClass: 'capVision' },
  reasoning: { label: 'Reasoning', colorClass: 'capReasoning' },
  tools: { label: 'Tool Calling', colorClass: 'capTools' },
  image: { label: 'Image Gen', colorClass: 'capImage' },
  pdf: { label: 'PDF', colorClass: 'capPdf' },
  code: { label: 'Coding', colorClass: 'capCode' },
};

export const SORT_OPTIONS: { id: SortMode; label: string }[] = [
  { id: 'recommended', label: 'Recommended' },
  { id: 'fastest', label: 'Fastest' },
  { id: 'cheapest', label: 'Cheapest' },
  { id: 'reasoning', label: 'Best reasoning' },
  { id: 'coding', label: 'Best coding' },
  { id: 'local', label: 'Local / private' },
  { id: 'recent', label: 'Recently added' },
];
