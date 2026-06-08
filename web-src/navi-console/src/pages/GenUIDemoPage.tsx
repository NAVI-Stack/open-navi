import { useEffect, useMemo, useRef, useState } from 'react';
import { GenUI, naviSpecEngine, type GenUIEngine, type NaviUIAction } from '@navi/ui';
import styles from './GenUIDemoPage.module.css';

type EngineKind = 'native' | 'openui';

const NATIVE_SAMPLE = `{
  "t": "card",
  "title": "Create project",
  "children": [
    { "t": "text", "value": "NAVI generated this entire panel from a JSON spec.", "tone": "secondary" },
    { "t": "divider" },
    {
      "t": "form",
      "action": { "intent": "create_project" },
      "submitLabel": "Create project",
      "children": [
        { "t": "field", "name": "title", "label": "Project name", "placeholder": "e.g. Apollo", "required": true },
        { "t": "field", "name": "goal", "label": "Goal", "description": "What should NAVI accomplish?" }
      ]
    },
    {
      "t": "stack", "dir": "row", "gap": "sm", "wrap": true,
      "children": [
        { "t": "badge", "label": "ACT", "status": "running" },
        { "t": "badge", "label": "draft", "status": "idle" },
        { "t": "badge", "label": "verified", "status": "completed" }
      ]
    },
    {
      "t": "stack", "dir": "row", "gap": "sm",
      "children": [
        { "t": "button", "label": "Preview", "variant": "secondary", "action": { "intent": "preview" } },
        { "t": "button", "label": "Discard", "variant": "ghost", "action": { "intent": "discard" } }
      ]
    }
  ]
}`;

// Same UI, expressed in OpenUI Lang — parsed by @openuidev/react-lang and mapped
// onto the SAME NAVI primitives via the opt-in @navi/ui/openui adapter.
const OPENUI_SAMPLE = `root = Card("Create project", [intro, form, tags, actions])
intro = Text("NAVI rendered this from OpenUI Lang — same primitives, different source.", "secondary")
form = Form("create_project", [titleField, goalField], "Create project")
titleField = Field("title", "Project name", "e.g. Apollo")
goalField = Field("goal", "Goal")
tags = Stack([act, draft, verified], "row", "sm")
act = Badge("ACT", "running")
draft = Badge("draft", "idle")
verified = Badge("verified", "completed")
actions = Stack([preview, discard], "row", "sm")
preview = Button("Preview", "secondary", "preview")
discard = Button("Discard", "ghost", "discard")`;

export function GenUIDemoPage() {
  const [engineKind, setEngineKind] = useState<EngineKind>('native');
  const [nativeSpec, setNativeSpec] = useState(NATIVE_SAMPLE);
  const [openuiSpec, setOpenuiSpec] = useState(OPENUI_SAMPLE);
  const [log, setLog] = useState<string[]>([]);

  // The OpenUI engine pulls in @openuidev/react-lang, so load it lazily only
  // when the OpenUI tab is selected — the native engine stays zero-cost.
  const [openuiEngine, setOpenuiEngine] = useState<GenUIEngine | null>(null);
  useEffect(() => {
    if (engineKind === 'openui' && !openuiEngine) {
      void import('@navi/ui/openui').then((m) => setOpenuiEngine(() => m.createNaviOpenUIEngine()));
    }
  }, [engineKind, openuiEngine]);

  // Streaming demo: replay the current source character-by-character so <GenUI
  // streaming> renders the progressively-revealed valid prefix.
  const [streamPos, setStreamPos] = useState<number | null>(null);
  const timer = useRef<ReturnType<typeof setInterval> | null>(null);

  const source = engineKind === 'native' ? nativeSpec : openuiSpec;
  const engine = engineKind === 'native' ? naviSpecEngine : openuiEngine;
  const streaming = streamPos !== null;
  const shownSource = streaming ? source.slice(0, streamPos!) : source;

  useEffect(() => () => { if (timer.current) clearInterval(timer.current); }, []);

  const replayStream = () => {
    if (timer.current) clearInterval(timer.current);
    setStreamPos(0);
    const total = source.length;
    const step = Math.max(2, Math.round(total / 90)); // ~90 frames
    timer.current = setInterval(() => {
      setStreamPos((prev) => {
        const next = (prev ?? 0) + step;
        if (next >= total) {
          if (timer.current) clearInterval(timer.current);
          return null; // settle on the complete spec
        }
        return next;
      });
    }, 40);
  };

  const handleAction = (action: NaviUIAction) => {
    const params = action.params && Object.keys(action.params).length ? ` ${JSON.stringify(action.params)}` : '';
    const entry = `${new Date().toLocaleTimeString()} — ${action.kind}:${action.intent}${params}`;
    setLog((prev) => [entry, ...prev].slice(0, 20));
  };

  const promptText = useMemo(() => (engine ? engine.systemPrompt() : 'Loading OpenUI engine…'), [engine]);

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div>
          <h1 className={styles.heading}>Generative UI</h1>
          <p className={styles.subheading}>
            Edit a spec and watch <code>&lt;GenUI&gt;</code> validate and render it with <code>@navi/ui</code>{' '}
            primitives. Switch the engine to render <strong>OpenUI Lang</strong> through the very same primitives, or
            replay <strong>streaming</strong> to see incomplete specs render their valid prefix. Invalid specs surface
            an inline error instead of crashing.
          </p>
        </div>
      </header>

      <div className={styles.toolbar}>
        <div className={styles.segmented} role="group" aria-label="Engine">
          <button
            type="button"
            className={styles.segment}
            aria-pressed={engineKind === 'native'}
            onClick={() => setEngineKind('native')}
          >
            NAVI Spec (JSON)
          </button>
          <button
            type="button"
            className={styles.segment}
            aria-pressed={engineKind === 'openui'}
            onClick={() => setEngineKind('openui')}
          >
            OpenUI Lang
          </button>
        </div>
        <button type="button" className={styles.segment} onClick={replayStream} style={{ border: '1px solid var(--navi-border)', borderRadius: 'var(--navi-radius)' }}>
          ▶ Replay streaming
        </button>
        <span className={styles.toolbarHint}>
          {engineKind === 'openui' && !openuiEngine
            ? 'Loading @navi/ui/openui…'
            : streaming
              ? 'Streaming…'
              : `Engine: ${engine?.name ?? '—'}`}
        </span>
      </div>

      <div className={styles.grid}>
        <section className={styles.panel}>
          <div className={styles.panelHeader}>{engineKind === 'native' ? 'NAVI UI Spec (JSON)' : 'OpenUI Lang'}</div>
          <textarea
            className={styles.editor}
            value={engineKind === 'native' ? nativeSpec : openuiSpec}
            spellCheck={false}
            onChange={(e) => (engineKind === 'native' ? setNativeSpec(e.target.value) : setOpenuiSpec(e.target.value))}
            aria-label="Generative UI source editor"
          />
        </section>

        <section className={styles.panel}>
          <div className={styles.panelHeader}>Rendered</div>
          <div className={styles.preview}>
            {engine ? (
              <GenUI source={shownSource} engine={engine} streaming={streaming} onAction={handleAction} />
            ) : (
              <span className={styles.toolbarHint}>Loading OpenUI engine…</span>
            )}
          </div>
        </section>
      </div>

      <section className={styles.panel}>
        <div className={styles.panelHeader}>Action log</div>
        <div className={styles.log}>
          {log.length === 0 ? (
            <span className={styles.logEmpty}>Submit the form or press a button — declarative actions land here.</span>
          ) : (
            log.map((line, i) => (
              <div key={i} className={styles.logLine}>
                {line}
              </div>
            ))
          )}
        </div>
      </section>

      <details className={styles.prompt}>
        <summary>LLM system prompt (engine vocabulary)</summary>
        <pre>{promptText}</pre>
      </details>
    </div>
  );
}
