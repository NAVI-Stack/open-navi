import { Fragment, useMemo, useRef, type CSSProperties, type ReactNode } from 'react';
import { naviSpecEngine } from './NaviSpecEngine';
import { defaultRegistry, type ComponentRegistry, type GenUIActionHandler } from './registry';
import type { GenUIEngine } from './engine';
import type { NaviUINode } from './schema';

const errorStyle: CSSProperties = {
  color: 'var(--navi-danger)',
  fontSize: 'var(--navi-font-size-sm)',
  fontFamily: 'var(--navi-font-mono)',
  padding: '8px 10px',
  border: '1px solid var(--navi-danger)',
  borderRadius: 'var(--navi-radius)',
  background: 'var(--navi-danger-dim)',
};

function GenUIError({ message }: { message: string }) {
  return (
    <div role="alert" style={errorStyle}>
      GenUI: {message}
    </div>
  );
}

/** Recursively render a validated node tree using a component registry. */
export function renderNaviNode(
  node: NaviUINode,
  registry: ComponentRegistry,
  onAction?: GenUIActionHandler,
  key?: number | string,
): ReactNode {
  const renderer = registry[node.t];
  if (!renderer) {
    return <GenUIError key={key} message={`unknown component type "${node.t}"`} />;
  }
  const rendered = renderer({
    node,
    onAction,
    render: (child, childKey) => renderNaviNode(child, registry, onAction, childKey),
  });
  return key === undefined ? rendered : <Fragment key={key}>{rendered}</Fragment>;
}

export interface GenUIProps {
  /** A NAVI UI Spec (object), an envelope, a JSON string, or an engine-specific payload. */
  source: unknown;
  /** Generative-UI engine. Defaults to the native NaviSpecEngine. */
  engine?: GenUIEngine;
  /** Node-type → component mapping. Defaults to `defaultRegistry`. */
  registry?: ComponentRegistry;
  /** Called when a generated control dispatches its declarative action. */
  onAction?: GenUIActionHandler;
  /** Rendered when the payload fails to parse/validate. */
  fallback?: ReactNode;
  /**
   * Progressive rendering for streaming sources. When set, the engine's tolerant
   * `parsePartial` is preferred so an incomplete spec renders its valid prefix,
   * and a transient mid-stream parse miss keeps the last good tree on screen
   * instead of flashing an error. Falls back to `parse` if the engine has no
   * `parsePartial`.
   */
  streaming?: boolean;
  className?: string;
}

/**
 * Render generative UI from a spec. The engine parses + validates; the registry
 * maps node types to `@navi/ui` primitives. Parse failures render `fallback`
 * (or a visible error) — never a crash, never silent.
 */
export function GenUI({
  source,
  engine = naviSpecEngine,
  registry = defaultRegistry,
  onAction,
  fallback,
  streaming,
  className,
}: GenUIProps) {
  const parsed = useMemo(
    () => (streaming && engine.parsePartial ? engine.parsePartial(source) : engine.parse(source)),
    [engine, source, streaming],
  );

  // While streaming, keep the last successfully-parsed tree so a transient miss
  // between chunks doesn't flash an error. Caching a value during render is a
  // supported React pattern.
  const lastGood = useRef<NaviUINode | null>(null);
  if (parsed.ok) lastGood.current = parsed.root;

  const root = parsed.ok ? parsed.root : streaming ? lastGood.current : null;
  if (!root) {
    const message = parsed.ok ? 'empty spec' : parsed.error;
    return <>{fallback ?? <GenUIError message={message} />}</>;
  }

  const incomplete = streaming && (!parsed.ok || parsed.partial);
  return (
    <div className={className} aria-busy={incomplete || undefined}>
      {renderNaviNode(root, registry, onAction)}
    </div>
  );
}
