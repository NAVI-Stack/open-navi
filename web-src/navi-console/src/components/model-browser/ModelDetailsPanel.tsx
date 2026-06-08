import clsx from 'clsx';
import {
  Brain,
  Cloud,
  Code2,
  Eye,
  FileText,
  HardDrive,
  Image,
  Info,
  Sparkles,
  Wrench,
  X,
  Zap,
} from 'lucide-react';
import type { BrowserModel, ModelCapability } from './types';
import styles from './ModelDetailsPanel.module.css';

// ─── Capability pill ──────────────────────────────────────────────────────────

const CAP_ICONS: Record<ModelCapability, React.ElementType> = {
  fast: Zap,
  vision: Eye,
  reasoning: Brain,
  tools: Wrench,
  image: Image,
  pdf: FileText,
  code: Code2,
};

const CAP_LABELS: Record<ModelCapability, string> = {
  fast: 'Fast',
  vision: 'Vision',
  reasoning: 'Reasoning',
  tools: 'Tool Calling',
  image: 'Image Gen',
  pdf: 'PDF',
  code: 'Coding',
};

const CAP_PILL_CLASS: Record<ModelCapability, string> = {
  fast: styles.capPillFast,
  vision: styles.capPillVision,
  reasoning: styles.capPillReasoning,
  tools: styles.capPillTools,
  image: styles.capPillImage,
  pdf: styles.capPillPdf,
  code: styles.capPillCode,
};

function CapabilityPill({ cap }: { cap: ModelCapability }) {
  const Icon = CAP_ICONS[cap];
  return (
    <span className={clsx(styles.capPill, CAP_PILL_CLASS[cap])}>
      <Icon size={11} />
      {CAP_LABELS[cap]}
    </span>
  );
}

// ─── Score bar ────────────────────────────────────────────────────────────────

function ScoreBar({ label, value }: { label: string; value: number }) {
  return (
    <div className={styles.scoreRow}>
      <span className={styles.scoreLabel}>{label}</span>
      <div className={styles.scoreTrack}>
        <div className={styles.scoreBar} style={{ width: `${value}%` }} />
      </div>
      <span className={styles.scoreNum}>{value}</span>
    </div>
  );
}

// ─── LLM KB callout ───────────────────────────────────────────────────────────

function KbCallout({ model }: { model: BrowserModel }) {
  const hasData =
    (model.learnedEvidenceCount ?? 0) > 0 &&
    (model.learnedAgenticDelta !== undefined ||
      model.learnedCodingDelta !== undefined ||
      model.learnedChatDelta !== undefined ||
      model.learnedReasoningDelta !== undefined);

  if (!hasData) return null;

  function DeltaSpan({ value, label }: { value: number | undefined; label: string }) {
    if (value === undefined || value === 0) return null;
    const isPos = value > 0;
    return (
      <span>
        {label}:{' '}
        <span className={clsx(styles.kbDelta, isPos ? styles.kbDeltaPos : styles.kbDeltaNeg)}>
          {isPos ? '+' : ''}{value}
        </span>{' '}
      </span>
    );
  }

  return (
    <div className={styles.kbCallout}>
      <div className={styles.kbCalloutTitle}>
        NAVI has observed this model ({model.learnedEvidenceCount} run
        {(model.learnedEvidenceCount ?? 0) !== 1 ? 's' : ''})
      </div>
      <DeltaSpan value={model.learnedAgenticDelta} label="Agentic" />
      <DeltaSpan value={model.learnedCodingDelta} label="Coding" />
      <DeltaSpan value={model.learnedChatDelta} label="Chat" />
      <DeltaSpan value={model.learnedReasoningDelta} label="Reasoning" />
    </div>
  );
}

// ─── Decision guidance derivation ────────────────────────────────────────────

interface DecisionInfo {
  bestFor: string[];
  avoidFor: string[];
  costTier: string;
  privacy: string;
}

function deriveDecisionInfo(model: BrowserModel): DecisionInfo {
  const GOOD = 72, POOR = 38;
  const tasks = [
    { label: 'Agentic tasks',    score: model.agenticScore },
    { label: 'Coding',           score: model.codingScore },
    { label: 'Reasoning',        score: model.reasoningScore },
    { label: 'Conversation',     score: model.chatScore },
    { label: 'Fast responses',   score: model.speedScore },
  ];
  return {
    bestFor:  tasks.filter(t => t.score >= GOOD).map(t => t.label),
    avoidFor: tasks.filter(t => t.score <  POOR).map(t => t.label),
    costTier: model.costScore >= 80 ? 'Free'
            : model.costScore >= 60 ? 'Low'
            : model.costScore >= 40 ? 'Moderate'
            : 'High',
    privacy:  model.executionType === 'local' ? 'Private — processed locally'
            : model.executionType === 'cloud' ? 'Shared — processed via API'
            : model.executionType === 'auto'  ? 'Varies by selected model'
            : 'Unknown',
  };
}

// ─── ModelDetailsPanel ────────────────────────────────────────────────────────

interface ModelDetailsPanelProps {
  model: BrowserModel;
  onClose: () => void;
}

export function ModelDetailsPanel({ model, onClose }: ModelDetailsPanelProps) {
  function formatContext(tokens: number): string {
    if (!tokens) return 'Unknown';
    if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1)}M tokens`;
    if (tokens >= 1_000) return `${Math.round(tokens / 1_000)}K tokens`;
    return `${tokens} tokens`;
  }

  const executionLabel =
    model.executionType === 'auto' ? 'Auto-routed' :
    model.executionType === 'cloud' ? 'Cloud' :
    model.executionType === 'local' ? 'Local' :
    'Unknown';

  const ExecutionIcon =
    model.executionType === 'auto' ? Sparkles :
    model.executionType === 'cloud' ? Cloud :
    model.executionType === 'local' ? HardDrive :
    Info;

  return (
    <div className={styles.panel} role="region" aria-label={`Details for ${model.displayName}`}>
      {/* Header */}
      <div className={styles.header}>
        <div className={styles.headerInfo}>
          <h3 className={styles.name}>{model.displayName}</h3>
          {!model.isNaviAuto && (
            <div className={styles.modelId}>
              {model.providerKey} / {model.modelId}
            </div>
          )}
        </div>
        <button
          type="button"
          className={styles.closeBtn}
          onClick={onClose}
          aria-label="Close details"
        >
          <X size={14} />
        </button>
      </div>

      {/* Meta */}
      <div className={styles.section}>
        <div className={styles.sectionTitle}>Overview</div>
        <div className={styles.metaGrid}>
          <div className={styles.metaItem}>
            <div className={styles.metaLabel}>Provider</div>
            <div className={styles.metaValue}>{model.providerName}</div>
          </div>
          <div className={styles.metaItem}>
            <div className={styles.metaLabel}>Execution</div>
            <div className={styles.metaValue}>
              <ExecutionIcon size={11} style={{ verticalAlign: 'middle', marginRight: 4 }} />
              {executionLabel}
            </div>
          </div>
          {model.maxContextTokens > 0 && (
            <div className={styles.metaItem}>
              <div className={styles.metaLabel}>Context window</div>
              <div className={styles.metaValue}>{formatContext(model.maxContextTokens)}</div>
            </div>
          )}
          <div className={styles.metaItem}>
            <div className={styles.metaLabel}>Tool calling</div>
            <div className={styles.metaValue}>
              {model.supportsTools
                ? model.toolCallReliable
                  ? 'Reliable'
                  : 'Supported'
                : 'Not supported'}
            </div>
          </div>
          {model.family && (
            <div className={styles.metaItem}>
              <div className={styles.metaLabel}>Family</div>
              <div className={styles.metaValue}>{model.family}</div>
            </div>
          )}
          {model.parameterSize && (
            <div className={styles.metaItem}>
              <div className={styles.metaLabel}>Parameters</div>
              <div className={styles.metaValue}>{model.parameterSize}</div>
            </div>
          )}
        </div>
      </div>

      {/* Capabilities */}
      {model.capabilities.length > 0 && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>Capabilities</div>
          <div className={styles.capPills}>
            {model.capabilities.map((cap) => (
              <CapabilityPill key={cap} cap={cap} />
            ))}
          </div>
        </div>
      )}

      {/* Scores */}
      {!model.isNaviAuto && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>Performance scores</div>
          <div className={styles.scoreList}>
            <ScoreBar label="Speed" value={model.speedScore} />
            <ScoreBar label="Cost efficiency" value={model.costScore} />
            <ScoreBar label="Reasoning" value={model.reasoningScore} />
            <ScoreBar label="Coding" value={model.codingScore} />
            <ScoreBar label="Agentic" value={model.agenticScore} />
            <ScoreBar label="Chat" value={model.chatScore} />
          </div>
        </div>
      )}

      {/* LLM KB observed data */}
      <KbCallout model={model} />

      {/* Decision guidance — derived from existing profile scores */}
      {!model.isNaviAuto && (() => {
        const { bestFor, avoidFor, costTier, privacy } = deriveDecisionInfo(model);
        return (
          <div className={styles.section}>
            <div className={styles.sectionTitle}>Decision guidance</div>
            <div className={styles.decisionGrid}>
              {bestFor.length > 0 && (
                <div className={styles.decisionRow}>
                  <span className={styles.decisionLabel}>Best for</span>
                  <span className={styles.decisionPills}>
                    {bestFor.map(t => (
                      <span key={t} className={clsx(styles.decisionPill, styles.decisionPillGood)}>{t}</span>
                    ))}
                  </span>
                </div>
              )}
              {avoidFor.length > 0 && (
                <div className={styles.decisionRow}>
                  <span className={styles.decisionLabel}>Avoid for</span>
                  <span className={styles.decisionPills}>
                    {avoidFor.map(t => (
                      <span key={t} className={clsx(styles.decisionPill, styles.decisionPillBad)}>{t}</span>
                    ))}
                  </span>
                </div>
              )}
              <div className={styles.decisionRow}>
                <span className={styles.decisionLabel}>Relative cost</span>
                <span className={clsx(
                  styles.decisionValue,
                  costTier === 'Free' && styles.decisionValueGood,
                  costTier === 'High' && styles.decisionValueBad,
                )}>{costTier}</span>
              </div>
              <div className={styles.decisionRow}>
                <span className={styles.decisionLabel}>Privacy</span>
                <span className={styles.decisionValue}>{privacy}</span>
              </div>
            </div>
          </div>
        );
      })()}
    </div>
  );
}
