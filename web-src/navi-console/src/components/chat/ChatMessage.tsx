import { Component, useEffect, useState, type ReactNode } from 'react';
import { GenUI, defaultRegistry, extendRegistry, parseNaviSpec, parseNaviSpecPartial, NaviToolUsagePanel, type GenUIEngine } from '@navi/ui';
import { Bot, Check, ChevronLeft, ChevronRight, Copy, Loader2, Pencil, Play, RefreshCw, RotateCcw, ThumbsDown, ThumbsUp, Wrench } from 'lucide-react';
import { MarkdownRenderer } from './MarkdownRenderer';
import { CeremonyControls, type CeremonyControlOption, type CeremonyControlAction } from './CeremonyControls';
import type { ChatRenderPayload } from './renderPayload';
import styles from './ChatMessage.module.css';

export type FeedbackRating = 'up' | 'down' | null;

export interface ToolPart {
  toolInvocationId: string;
  toolName: string;
  /** "call"/"partial-call" while running, "result" once resolved. */
  state: string;
  result?: unknown;
  isError?: boolean;
}

const consoleGenUIRegistry = extendRegistry(defaultRegistry, {
  markdown: ({ node }) => <MarkdownRenderer content={(node as { t: 'markdown'; value: string }).value} />,
});

interface DetectedNaviUISpecPayload {
  source: unknown;
  streaming?: boolean;
  pending?: boolean;
}

function looksLikeStreamingNaviUISpec(trimmed: string): boolean {
  return (
    trimmed === '{' ||
    /^\{\s*"t"?$/.test(trimmed) ||
    /^\{\s*"root"?$/.test(trimmed) ||
    /^\{\s*"t"\s*:/.test(trimmed) ||
    /^\{\s*"root"\s*:/.test(trimmed) ||
    /"t"\s*:/.test(trimmed) ||
    /"root"\s*:/.test(trimmed)
  );
}

export function detectNaviUISpecPayload(payload: unknown, options: { streaming?: boolean } = {}): DetectedNaviUISpecPayload | null {
  if (typeof payload === 'string') {
    const trimmed = payload.trim();
    if (!trimmed.startsWith('{')) return null;
    if (options.streaming && looksLikeStreamingNaviUISpec(trimmed)) {
      const parsed = parseNaviSpecPartial(trimmed);
      return {
        source: trimmed,
        streaming: true,
        pending: !parsed.ok,
      };
    }
    const parsed = parseNaviSpec(trimmed);
    return parsed.ok ? { source: parsed.root } : null;
  }
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return null;
  const parsed = parseNaviSpec(payload);
  return parsed.ok ? { source: parsed.root } : null;
}

function NaviUISpecBlock({ spec, className }: { spec: DetectedNaviUISpecPayload; className?: string }) {
  return (
    <div
      className={`${styles.genUISurface}${className ? ` ${className}` : ''}`}
      data-testid="navi-ui-spec"
      aria-label="NAVI UI spec"
    >
      <GenUI
        source={spec.source}
        streaming={spec.streaming}
        registry={consoleGenUIRegistry}
        fallback={spec.pending ? <span className={styles.genUIPending}>Rendering UI...</span> : null}
      />
    </div>
  );
}

/**
 * Error boundary so a render-time throw inside the OpenUI/GenUI lane never crashes
 * the chat surface — it degrades to the provided fallback (markdown) instead.
 */
class GenUIErrorBoundary extends Component<{ fallback: ReactNode; children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  componentDidCatch(error: unknown) {
    console.error('Data-driven render failed; showing fallback:', error);
  }
  render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}

function RenderFallback({ markdown }: { markdown?: string }) {
  if (!markdown || !markdown.trim()) {
    return <div className={styles.genUIPending}>This data view could not be rendered.</div>;
  }
  return <MarkdownRenderer content={markdown} />;
}

/**
 * Honest "prototype / placeholder data" chip shown above a generated-UI prototype
 * render. Prototypes use placeholder data (not real telemetry), so this label must
 * be visible. It renders only when the payload is a prototype / placeholder render.
 * The OpenUI program itself also carries an embedded PROTOTYPE badge, so the label
 * survives even if this chrome is absent.
 */
function PrototypeBadge({ payload }: { payload: ChatRenderPayload }) {
  if (payload.dataSourceKind !== 'placeholder' && !payload.prototype) return null;
  return (
    <div className={styles.prototypeBadge} aria-label="Prototype with placeholder data">
      Prototype · placeholder data
    </div>
  );
}

/**
 * OpenUI render lane. OpenUI is an OPTIONAL renderer (`@navi/ui/openui` depends on
 * an optional peer), so the engine is loaded lazily; if it is unavailable or the
 * program fails to parse/render, we fall back to the payload's markdown. On
 * success only the OpenUI surface is shown — the caller suppresses message.content.
 *
 * Read-only by construction: no `onAction` handler is passed to <GenUI>, so any
 * generated button/form is inert — generated prototype UI cannot invoke actions.
 */
function OpenUILane({ payload }: { payload: ChatRenderPayload }) {
  const [engine, setEngine] = useState<GenUIEngine | null>(null);
  const [unavailable, setUnavailable] = useState(false);

  useEffect(() => {
    let alive = true;
    import('@navi/ui/openui')
      .then((mod) => {
        if (alive) setEngine(mod.createNaviOpenUIEngine());
      })
      .catch(() => {
        if (alive) setUnavailable(true);
      });
    return () => {
      alive = false;
    };
  }, []);

  const badge = <PrototypeBadge payload={payload} />;
  const fallbackBody = <RenderFallback markdown={payload.fallbackMarkdown} />;

  // OpenUI unavailable, or nothing to render → fallback markdown (with the chip).
  if (unavailable || !payload.openuiLang)
    return (
      <>
        {badge}
        {fallbackBody}
      </>
    );
  // Engine still loading — show a light pending hint, not a flash of fallback.
  if (!engine) return <span className={styles.genUIPending}>Rendering UI…</span>;

  // The chip is rendered once at the top of the surface; the GenUI/error-boundary
  // fallback is the markdown body only (no second chip).
  return (
    <div
      className={styles.genUISurface}
      data-testid="openui-render"
      aria-label={payload.prototype ? 'Generated UI prototype' : 'Data view'}
    >
      {badge}
      <GenUIErrorBoundary fallback={fallbackBody}>
        {/* The payload carries a complete program (not token-streamed), so parse
            non-streaming: a malformed program then renders the markdown fallback
            rather than an empty shell. */}
        <GenUI
          engine={engine}
          source={payload.openuiLang}
          registry={consoleGenUIRegistry}
          fallback={fallbackBody}
        />
      </GenUIErrorBoundary>
    </div>
  );
}

/**
 * Dispatch a render payload to its lane. "openui" → OpenUI lane (markdown
 * fallback); "navi-ui" with a data view → the NAVI-native panel; anything else →
 * markdown fallback. Always self-contained: the caller does not also render the
 * message text when a payload is present.
 */
function DataDrivenRender({ payload }: { payload: ChatRenderPayload }) {
  if (payload.mode === 'navi-ui' && payload.dataView) {
    return (
      <div className={styles.genUISurface} data-testid="navi-data-view">
        <GenUIErrorBoundary fallback={<RenderFallback markdown={payload.fallbackMarkdown} />}>
          <NaviToolUsagePanel view={payload.dataView} />
        </GenUIErrorBoundary>
      </div>
    );
  }
  if (payload.mode === 'openui') return <OpenUILane payload={payload} />;
  return <RenderFallback markdown={payload.fallbackMarkdown} />;
}

function toolResultText(result?: unknown): string | undefined {
  return typeof result === 'string' && result.trim() ? result : undefined;
}

export interface ChatMessageView {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  createdAt?: string;
  pending?: boolean;
  failed?: boolean;
  streaming?: boolean;
  toolParts?: ToolPart[];
  /** Data-driven render payload (OpenUI / NaviDataView). When present on an
   * assistant message, the data view is rendered instead of the text content. */
  renderPayload?: ChatRenderPayload;
  variant?: {
    selectedIndex: number;
    total: number;
  };
  ceremonyStep?: string;
  ceremonyType?: string;
  ceremonyControls?: CeremonyControlOption[];
  ceremonyActions?: CeremonyControlAction[];
  ceremonyDefaults?: Record<string, boolean>;
}

function ToolChips({ parts }: { parts: ToolPart[] }) {
  if (!parts.length) return null;
  return (
    <div className={styles.toolChips} aria-label="Tool activity">
      {parts.map((part) => {
        const running = part.state !== 'result';
        const resultText = toolResultText(part.result);
        const resultUISpec = !running && !part.isError ? detectNaviUISpecPayload(part.result) : null;
        return (
          <div key={part.toolInvocationId} className={styles.toolPart}>
            <div
              className={styles.toolChip}
              data-state={running ? 'running' : part.isError ? 'error' : 'done'}
            >
              {running ? (
                <Loader2 size={13} className={styles.toolChipSpinner} aria-hidden="true" />
              ) : (
                <Wrench size={13} aria-hidden="true" />
              )}
              <span className={styles.toolChipName}>{part.toolName || 'tool'}</span>
              <span className={styles.toolChipState}>
                {running ? 'Running…' : part.isError ? 'Failed' : resultUISpec ? 'UI result' : resultText || 'Done'}
              </span>
            </div>
            {resultUISpec ? <NaviUISpecBlock spec={resultUISpec} className={styles.toolResultUISurface} /> : null}
          </div>
        );
      })}
    </div>
  );
}

interface Props {
  message: ChatMessageView;
  isNaviActive?: boolean;
  onRetry?: () => void;
  feedback?: FeedbackRating;
  onFeedback?: (rating: FeedbackRating) => void;
  onEditResend?: (newContent: string) => void;
  onRegenerate?: () => void;
  onContinue?: () => void;
  onSelectVariant?: (index: number) => void;
  onCeremonySelect?: (step: string, value: string | Record<string, boolean>) => void;
}

function formatTime(value?: string): string {
  if (!value) return '';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
}

export function ChatMessage({ message, isNaviActive, onRetry, feedback, onFeedback, onEditResend, onRegenerate, onContinue, onSelectVariant, onCeremonySelect }: Props) {
  const isUser = message.role === 'user';
  const [copied, setCopied] = useState(false);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(message.content);
  const canFeedback = !isUser && !!onFeedback && !!message.content && !message.streaming && !message.pending;
  const canEdit = isUser && !!onEditResend && !!message.content && !message.pending;
  const canRegenerate = !isUser && !!onRegenerate && !message.streaming && !message.pending;
  const canContinue = !isUser && !!onContinue && !message.streaming && !message.pending;
  const variantTotal = message.variant?.total ?? 0;
  const selectedVariant = message.variant?.selectedIndex ?? 0;
  const canSwitchVariant = !isUser && !!onSelectVariant && variantTotal > 1 && !message.streaming && !message.pending;
  const dataDrivenPayload = !isUser ? message.renderPayload : undefined;
  const assistantUISpec = !isUser && !dataDrivenPayload ? detectNaviUISpecPayload(message.content, { streaming: message.streaming }) : null;

  const startEdit = () => {
    setDraft(message.content);
    setEditing(true);
  };
  const submitEdit = () => {
    const trimmed = draft.trim();
    if (!trimmed) return;
    onEditResend?.(trimmed);
    setEditing(false);
  };

  const handleCopy = () => {
    if (!message.content) return;
    navigator.clipboard.writeText(message.content).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };

  const showEmptyPulse = !isUser && message.streaming && !message.content;

  return (
    <div
      className={isUser ? styles.rowUser : styles.rowAssistant}
      data-pending={message.pending ? 'true' : undefined}
      data-failed={message.failed ? 'true' : undefined}
      data-streaming={message.streaming ? 'true' : undefined}
    >
      {!isUser && (
        <div
          className={styles.avatar}
          data-active={isNaviActive ? 'true' : undefined}
          aria-hidden="true"
        >
          <Bot size={16} />
        </div>
      )}

      <div className={isUser ? styles.colUser : styles.colAssistant}>
        {!isUser && <span className={styles.roleLabel}>NAVI</span>}

        {!isUser && message.toolParts?.length ? <ToolChips parts={message.toolParts} /> : null}

        {showEmptyPulse && message.toolParts?.length ? null : showEmptyPulse ? (
          <div className={styles.emptyBubble}>
            <span className={styles.pulseDot} />
          </div>
        ) : isUser && editing ? (
          <div className={styles.editBox}>
            <textarea
              className={styles.editInput}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault();
                  submitEdit();
                } else if (e.key === 'Escape') {
                  setEditing(false);
                }
              }}
              rows={Math.min(8, Math.max(2, draft.split('\n').length))}
              autoFocus
              aria-label="Edit message"
            />
            <div className={styles.editActions}>
              <button className={styles.editCancel} onClick={() => setEditing(false)} type="button">
                Cancel
              </button>
              <button className={styles.editSave} onClick={submitEdit} disabled={!draft.trim()} type="button">
                Save & resend
              </button>
            </div>
          </div>
        ) : isUser ? (
          <div className={styles.userBubble}>
            <MarkdownRenderer content={message.content} />
          </div>
        ) : (
          <div className={styles.assistantBubble}>
            {dataDrivenPayload ? (
              <DataDrivenRender payload={dataDrivenPayload} />
            ) : assistantUISpec ? (
              <NaviUISpecBlock spec={assistantUISpec} />
            ) : (
              <MarkdownRenderer content={message.content} />
            )}
            {message.ceremonyStep && onCeremonySelect && (
              <CeremonyControls
                step={message.ceremonyStep}
                type={message.ceremonyType as 'single_select' | 'multi_select' | 'actions' | undefined}
                options={message.ceremonyControls}
                actions={message.ceremonyActions}
                defaults={message.ceremonyDefaults}
                onSelect={onCeremonySelect}
                disabled={!!message.pending || !!message.streaming}
              />
            )}
          </div>
        )}

        <div className={styles.toolbar}>
          {message.failed ? (
            <span className={styles.stateError}>Not sent</span>
          ) : message.pending ? (
            <span className={styles.state}>Sending…</span>
          ) : message.streaming ? (
            <span className={styles.state}>Writing…</span>
          ) : (
            message.createdAt && <span className={styles.time}>{formatTime(message.createdAt)}</span>
          )}

          {message.content && !message.pending && !message.streaming && (
            <button className={styles.iconBtn} onClick={handleCopy} type="button" title="Copy" aria-label="Copy message">
              {copied ? <Check size={14} /> : <Copy size={14} />}
            </button>
          )}

          {message.failed && onRetry && (
            <button className={styles.iconBtn} onClick={onRetry} type="button" title="Retry" aria-label="Retry sending">
              <RotateCcw size={14} />
            </button>
          )}

          {canEdit && !editing && (
            <button className={styles.iconBtn} onClick={startEdit} type="button" title="Edit & resend" aria-label="Edit message">
              <Pencil size={14} />
            </button>
          )}

          {canRegenerate && (
            <button className={styles.iconBtn} onClick={onRegenerate} type="button" title="Regenerate" aria-label="Regenerate response">
              <RefreshCw size={14} />
            </button>
          )}

          {canContinue && (
            <button className={styles.iconBtn} onClick={onContinue} type="button" title="Continue" aria-label="Continue response">
              <Play size={14} />
            </button>
          )}

          {canSwitchVariant && (
            <span className={styles.variantSwitcher} aria-label="Response variants">
              <button
                className={styles.iconBtn}
                onClick={() => onSelectVariant?.((selectedVariant - 1 + variantTotal) % variantTotal)}
                type="button"
                title="Previous variant"
                aria-label="Previous variant"
              >
                <ChevronLeft size={14} />
              </button>
              <span className={styles.variantCount}>{selectedVariant + 1}/{variantTotal}</span>
              <button
                className={styles.iconBtn}
                onClick={() => onSelectVariant?.((selectedVariant + 1) % variantTotal)}
                type="button"
                title="Next variant"
                aria-label="Next variant"
              >
                <ChevronRight size={14} />
              </button>
            </span>
          )}

          {canFeedback && (
            <>
              <button
                className={feedback === 'up' ? `${styles.iconBtn} ${styles.iconBtnActive}` : styles.iconBtn}
                onClick={() => onFeedback?.(feedback === 'up' ? null : 'up')}
                type="button"
                title="Good response"
                aria-label="Good response"
                aria-pressed={feedback === 'up'}
              >
                <ThumbsUp size={14} />
              </button>
              <button
                className={feedback === 'down' ? `${styles.iconBtn} ${styles.iconBtnActive}` : styles.iconBtn}
                onClick={() => onFeedback?.(feedback === 'down' ? null : 'down')}
                type="button"
                title="Bad response"
                aria-label="Bad response"
                aria-pressed={feedback === 'down'}
              >
                <ThumbsDown size={14} />
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
