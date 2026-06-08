# NAVI UI Library (`@navi/ui`) — Architecture & Roadmap

> Status: **Milestone 2 (library hardening + OpenUI adapter) — implemented.**
> The NAVI Console is the first consumer and proving ground. The library now
> also ships a standalone built artifact (`dist/`) for external NAVI apps.

The **NAVI UI Library** is a persistent, shareable UI foundation for the whole
NAVI ecosystem. It gives every NAVI app one design language (tokens, theming,
primitives, interaction patterns) and a **generative-UI runtime** so NAVI can
dynamically compose interfaces and inputs — for the Console today and future
frontends tomorrow — without each app re-inventing or fragmenting the system.

---

## 1. Goals & non-goals

**Goals**

1. **Shared design primitives, layouts, and interaction patterns** that look and
   behave identically across NAVI apps.
2. **One visual + behavioural language** — a single token system and theme engine.
3. **Generative UI**: NAVI emits a compact, safe UI spec; the library validates
   and renders it with first-class primitives.
4. **A clean, importable package** (`@navi/ui`) other NAVI apps depend on.
5. **Extensibility**: apps build on top (custom node types, registry overrides,
   themes) without forking the design system.
6. **OpenUI integration where it helps** — schema-driven / generative composition
   — behind a pluggable seam, **without lock-in**.

**Non-goals (for now)**

- Replacing the Console's data layer (`@tanstack/react-query`, `useLiveEvents`).
- Publishing to an external registry (the package now *builds* a `dist/`
  artifact, but in-repo apps still consume source; registry publish is a later
  milestone).
- A full component zoo. We grow primitives as real apps demand them.

---

## 2. Architecture at a glance

```
                          ┌─────────────────────────────────────────┐
   NAVI backend (Go) ──►  │  UI spec (JSON, safe, declarative)       │
   tool result / event    └───────────────────┬─────────────────────┘
                                               │
                                   ┌───────────▼───────────┐
                                   │   GenUIEngine (iface)  │   pluggable
                                   │  ├─ NaviSpecEngine     │   (native, default)
                                   │  └─ OpenUIEngine       │   (adapter → @openuidev)
                                   └───────────┬───────────┘
                                               │ validated NaviUINode tree
                                   ┌───────────▼───────────┐
                                   │  <GenUI> + registry    │   node type → component
                                   └───────────┬───────────┘
                                               │
   ┌───────────────────────────────────────────▼──────────────────────────────┐
   │  @navi/ui primitives  (Button, Card, Stack, Text, Field, StatusBadge, …)   │
   │  styled by tokens.css  ·  themed by the theme engine                       │
   └────────────────────────────────────────────────────────────────────────────┘
```

The library has five layers, each usable on its own:

| Layer | Source | What it is |
|-------|--------|-----------|
| **Tokens** | `src/tokens/tokens.css` | ~140 `--navi-*` CSS custom properties — the design system's atoms (surfaces, text, accent, status, spacing, radius, typography, motion, z-index). |
| **Theme** | `src/theme/` | The token data model (`ThemeModel`), `applyTheme` (projects a model onto CSS vars), `ThemeProvider`/`useTheme`, light/dark groups, density + font scaling, contrast checks. |
| **Primitives** | `src/primitives/` | Accessible, token-styled building blocks (built on `react-aria-components`, CSS Modules). |
| **Patterns** | `src/patterns/` | Composed, opinionated UX: `Dialog` (generic modal), `Toast` (`ToastProvider`/`useToast`), `ConfirmDialog` (`ConfirmProvider`/`useConfirm`). |
| **GenUI** | `src/genui/` | The generative-UI runtime: spec schema, engines, component registry, `<GenUI>` renderer, streaming helpers, and an LLM prompt generator. |
| **OpenUI** | `src/openui/` | Opt-in `@navi/ui/openui` subpath: a concrete OpenUI Lang → NaviUINode adapter (depends on the optional peer `@openuidev/react-lang`). |

**Design-principle alignment** (see `CLAUDE.md`): generative behaviour is
declarative data (specs + prompts), not hard-coded Go/TS heuristics; engines are
swappable so NAVI is never locked into one generative-UI format; invalid specs
degrade visibly instead of crashing or faking success.

---

## 3. Package structure & workspace

The frontend is a **pnpm workspace** rooted at `web-src/` (`pnpm-workspace.yaml`).
pnpm is used for ecosystem-wide uniformity and install efficiency — a global
content-addressable store, the `workspace:` protocol, and a single lockfile. The
Console consumes `@navi/ui` **as source** via a Vite/TS alias, so the package
manager never affects bundling.

```
web-src/
├── pnpm-workspace.yaml     # workspace roots: packages/* + navi-console
├── package.json            # root scripts + packageManager: pnpm@…
├── pnpm-lock.yaml          # single lockfile for the whole frontend
├── packages/
│   └── navi-ui/            # @navi/ui — the shared library
│       ├── package.json    # exports ".", "./theme", "./genui", "./patterns", "./openui", "./tokens.css", "./navi-ui.css"
│       ├── vite.config.ts      # lightweight react+alias+test config (vitest + Ladle share it)
│       ├── vite.lib.config.ts  # standalone library build (lib entries + dts + token copy)
│       ├── .ladle/             # Ladle workbench config + themed provider
│       ├── scripts/            # smoke-dist.{mjs,ts} — import + typecheck the built dist
│       ├── README.md
│       ├── dist/               # BUILD OUTPUT (gitignored): index.js, index.d.ts, openui.js, navi-ui.css, tokens.css
│       └── src/
│           ├── index.ts        # public barrel
│           ├── tokens/tokens.css
│           ├── theme/          # theme.ts, ThemeProvider.tsx
│           ├── primitives/     # Button, Card, StatusBadge, EmptyState, Stack, Text, Spinner, Field
│           ├── patterns/       # Dialog, Toast, ConfirmDialog
│           ├── genui/          # schema, engine, NaviSpecEngine, OpenUIEngine, partial, registry, GenUI, prompt
│           ├── openui/         # OpenUI Lang adapter (opt-in @navi/ui/openui subpath)
│           └── **/*.stories.tsx  # Ladle stories for every primitive, pattern, and GenUI
└── navi-console/           # the Console app; depends on "@navi/ui": "workspace:*"
```

### How apps consume it

In-repo, the Console consumes `@navi/ui` **as source** via the workspace symlink
plus a Vite/TS alias — no build step, instant HMR, one type-check:

```ts
// web-src/navi-console/vite.config.ts  (alias is ordered: tokens.css before root)
resolve.alias = [
  { find: '@navi/ui/tokens.css', replacement: '.../packages/navi-ui/src/tokens/tokens.css' },
  { find: '@navi/ui',           replacement: '.../packages/navi-ui/src/index.ts' },
  { find: '@',                  replacement: '.../navi-console/src' },
];
// web-src/navi-console/tsconfig.json
"paths": { "@/*": ["./src/*"], "@navi/ui": ["../packages/navi-ui/src/index.ts"] }
```

```ts
import { Button, Card, Stack, Text } from '@navi/ui';
import { applyTheme, ThemeProvider } from '@navi/ui';      // or '@navi/ui/theme'
import { GenUI, naviSpecEngine } from '@navi/ui';          // or '@navi/ui/genui'
import '@navi/ui/tokens.css';
```

An **external** app consuming the *built* artifact instead of source only changes
the resolution: add `@navi/ui` as a dependency and import the same public API.
The package's `exports` map points `.`/`./theme`/`./genui`/`./patterns` at
`dist/index.js` (+ generated `.d.ts`), `./openui` at `dist/openui.js`, and
`./tokens.css` / `./navi-ui.css` at the emitted stylesheets, while
`peerDependencies` describe the runtime contract.

### Standalone library build (`dist/`)

`pnpm --filter @navi/ui run build` (Vite library mode + `vite-plugin-dts`, via
`vite.lib.config.ts`) emits ESM JS, type declarations, and CSS:

```
dist/index.js · dist/index.d.ts        # core entry + types
dist/openui.js · dist/openui/*.d.ts    # opt-in OpenUI subpath
dist/navi-ui.css                       # all component (CSS Module) styles
dist/tokens.css                        # design tokens, copied verbatim
```

`react`, `react-dom`, `react-aria-components`, `lucide-react`, and the opt-in
`@openuidev/react-lang` are **externalized**; `clsx` and `zod` are **bundled**
(small, internal-only) so consumers needn't install them. `scripts/smoke-dist.*`
imports the built artifact at runtime **and** typechecks against the generated
declarations, so a broken bundle fails CI loudly.

**Critically, the Console still consumes source** (its Vite/TS alias bypasses
`exports`), so there is no build-order coupling and its production bundle is
byte-identical to before the build existed. The lib build is build-only and is
kept in `vite.lib.config.ts`, separate from the `vite.config.ts` that vitest and
Ladle share.

---

## 4. Theming

The theme engine moved into `@navi/ui` and is now app-agnostic:

- `ThemeModel` — `{ mode, accent, density, font_scale, tokens: { light, dark } }`.
- `applyTheme(theme, target?)` — writes the resolved tokens onto a DOM element's
  `--navi-*` custom properties (defaults to `#root`). Safe to call repeatedly
  and in non-DOM environments.
- `ThemeProvider` / `useTheme` — a React-idiomatic wrapper for apps that prefer
  context over imperative application.
- Utilities: `createDefaultTheme`, `normalizeTheme`, `cloneTheme`, `contrastRatio`.

The Console keeps its **app-specific appearance layer** (named presets, server
persistence via `/api/console/appearance`, schema versioning) in
`navi-console/src/appearance/theme.ts`, which now re-exports the shared engine —
so existing `@/appearance/theme` imports keep working unchanged. This is the
extensibility model in miniature: **generic engine in the library, app policy on
top.**

---

## 5. Generative UI

### 5.1 The NAVI UI Spec

A spec is a JSON tree of nodes discriminated by a short `t` tag. Short tags keep
the wire form compact (a nod to OpenUI's token-efficiency); the tree stays
human-readable and **safe** — there is no embedded code or raw HTML, and
interactivity is expressed declaratively.

```jsonc
{
  "t": "card", "title": "Create project",
  "children": [
    { "t": "text", "value": "Name your project.", "tone": "secondary" },
    { "t": "form", "action": { "intent": "create_project" }, "submitLabel": "Create",
      "children": [ { "t": "field", "name": "title", "label": "Project name", "required": true } ] },
    { "t": "badge", "label": "ACT", "status": "running" }
  ]
}
```

Node types (M1): `text`, `heading`, `markdown`, `badge`, `button`, `field`,
`divider`, `stack`, `card`, `list`, `form`. Defined and validated with **Zod**
in `genui/schema.ts` (`naviUINodeSchema`).

**Safety model.** Buttons/forms carry an `action` with a semantic `intent`
(e.g. `create_project`). The library never executes anything; it surfaces the
action to the host via `onAction`, and the host decides what each intent does.
Form field values arrive in `action.params` keyed by field `name`. Specs that
fail validation render a visible error — never a crash, never a silent success.

### 5.2 Engines (the pluggable seam)

```ts
interface GenUIEngine {
  readonly name: string;
  parse(input: unknown): ParsedUI;          // validate → NaviUINode tree; never throws
  parsePartial?(input: unknown): ParsedUI;  // optional: tolerant parse for streaming
  systemPrompt(): string;                   // LLM vocabulary description
}
```

- **`naviSpecEngine`** (default) — parses NAVI UI Spec JSON/objects/envelopes.
- **`createOpenUIEngine(adapter?)`** — wraps an external generative-UI format
  (OpenUI Lang) behind the same interface (see §6).

### 5.2a Streaming / progressive rendering

Generative UI streams in token-by-token. `<GenUI streaming>` prefers an engine's
optional `parsePartial`, which renders the **valid prefix** of an incomplete
spec, and keeps the last good tree across chunks so a transient mid-stream parse
miss never flashes an error (`aria-busy` reflects the in-flight state). The core,
dependency-free helpers in `genui/partial.ts`:

- `completeTruncatedJson` — repairs a truncated JSON string into its longest
  parseable prefix (closes an open string, balances brackets).
- `coerceValidPrefix` — prunes a node tree to its leading run of valid children,
  dropping a half-streamed tail.

The native engine uses these; the OpenUI engine uses OpenUI's own incremental
parser. Both are pure and never throw — incomplete input degrades visibly, never
crashes.

`<GenUI source={...} engine={...} registry={...} onAction={...} />` parses with
the chosen engine and walks the validated tree, mapping node types to primitives
via the **registry**.

### 5.3 Registry & extensibility

`defaultRegistry` maps each node type to a `@navi/ui` primitive. Apps extend it
**without forking**:

```ts
import { extendRegistry, defaultRegistry } from '@navi/ui';
// The Console plugs in its full Markdown renderer for `markdown` nodes:
const registry = extendRegistry(defaultRegistry, {
  markdown: ({ node }) => <MarkdownRenderer source={node.value} />,
});
<GenUI source={spec} registry={registry} />;
```

This is also how apps add **app-specific node types** (e.g. a `chart` or
`run-timeline` node) on top of the shared vocabulary.

### 5.4 Backend integration (design)

NAVI's backend already streams structured events to the Console over
`/ws/live` (`useLiveEvents`) and renders tool invocations in `ChatMessage`. The
generative-UI transport reuses this spine:

- A tool/skill (or a dedicated "render" capability) returns a **NAVI UI Spec**
  as its result payload (a JSON string/object).
- The Console renders that payload with `<GenUI>` inside the message/tool surface
  and forwards `onAction` intents back to NAVI as a normal user/tool message.
- The spec vocabulary is advertised to the model via `engine.systemPrompt()`,
  so the LLM knows exactly what it can emit. This keeps decision logic in prompts
  + specs (Zero Framework Cognition), not in Go/TS.

Wiring this end-to-end into the chat surface is **M3** (below); M1 ships the
runtime and a standalone demo so the contract is proven first.

---

## 6. OpenUI integration strategy

[`thesysdev/openui`](https://github.com/thesysdev/openui) ("The Open Standard for
Generative UI", MIT) is a spec-based, streaming generative-UI framework: an LLM
emits **OpenUI Lang**, a parser builds a component tree, and a React renderer
renders it progressively. Its published packages include `@openuidev/react-lang`
(parser/renderer/prompt-gen), `@openuidev/react-headless`, and `@openuidev/react-ui`.

**Strategy: adopt the ideas, integrate behind an adapter, avoid lock-in.**

`@navi/ui` defines `GenUIEngine` as the seam. `createOpenUIEngine(adapter)` lets a
host bridge OpenUI Lang into NAVI's render pipeline by mapping it onto a
`NaviUINode` tree, which is then validated and rendered by the **same** registry
and primitives as native specs:

```ts
import { createOpenUIEngine, GenUI } from '@navi/ui';
import * as openuiLang from '@openuidev/react-lang';  // host installs this

const openui = createOpenUIEngine({
  toNaviSpec: (payload) => mapOpenUILangToNaviSpec(openuiLang.parse(payload)),
  systemPrompt: () => openuiLang.generatePrompt(/* component library */),
});

<GenUI engine={openui} source={openUILangString} />;
```

Why a seam and not a hard dependency (M1):

- **No lock-in / no build coupling.** `@navi/ui` takes *zero* dependency on
  `@openuidev/*`; the adapter is injected by the host, so the library build never
  depends on an external, still-evolving GenUI engine. Without an adapter the
  OpenUI engine is inert and reports a clear, actionable error.
- **One render target.** Whether a spec originates from NAVI or OpenUI Lang, it
  renders through NAVI primitives + tokens — consistent look, theming, and a11y.
- **Optionality.** Apps that want OpenUI install `@openuidev/react-lang` and wire
  the adapter; apps that don't pay nothing.

What we borrow from OpenUI regardless: streaming-first spec parsing, compact
type tags for token efficiency, and prompt-generation-from-vocabulary
(`systemPrompt()`).

**M2 update — the concrete adapter ships.** `@navi/ui/openui` provides
`createNaviOpenUIEngine()` (and `createNaviOpenUIAdapter()`), built on
`@openuidev/react-lang`'s real parser:

- A hand-authored `LibraryJSONSchema` (`naviOpenUISchema`) mirrors NAVI's
  vocabulary. OpenUI Lang is statement-based with **positional** arguments, so
  the schema's property order is load-bearing — it defines the positional → named
  prop mapping the parser applies.
- `mapOpenUIElement` walks the parsed `ElementNode` tree into `NaviUINode`s;
  `createParser`/`createStreamingParser` give one-shot and incremental (streaming)
  parsing. `naviOpenUISystemPrompt()` documents the grammar for the model.
- The adapter is the **only** module that imports `@openuidev/react-lang`
  (declared `optionalDependency`), and it is a **separate build entry**
  (`dist/openui.js`), so the core library and its build stay dependency-free.

The Console's `/dev/genui` demo toggles native ↔ OpenUI engines (OpenUI loaded
lazily) and replays streaming.

---

## 7. Implementation roadmap

### M1 — Foundation + proof slice ✅ (this change)

- pnpm workspace at `web-src/`; `@navi/ui` package scaffolded with `exports`,
  peer deps, typecheck + tests.
- Tokens + theme engine moved into `@navi/ui`; Console re-exports them (no
  visual regression).
- Primitives: `Button`, `Card`, `StatusBadge`, `EmptyState`, `Stack`, `Text`,
  `Spinner`, `Field`. Card/StatusBadge/EmptyState migrated out of the Console
  with re-export shims so ~30 import sites are untouched.
- GenUI runtime: spec schema, `GenUIEngine`, `NaviSpecEngine`, `createOpenUIEngine`
  (adapter seam), `defaultRegistry`/`extendRegistry`, `<GenUI>`, prompt generator.
- Console consumes `@navi/ui`; a live GenUI demo at **`/dev/genui`**.
- Docker/Make/CI updated for the workspace; build + 98 tests (15 lib + 83
  Console) green.

### M2 — Library hardening + OpenUI adapter ✅ (this change)

- **Standalone library build** (`vite --lib` + `vite-plugin-dts`, in
  `vite.lib.config.ts`) emitting `dist/` (ESM JS + `.d.ts` + `navi-ui.css` +
  `tokens.css`) so external apps consume a built, typed artifact. A
  `smoke:dist` script imports + typechecks the built output. The Console keeps
  consuming source — no build-order coupling, byte-identical bundle.
- **Concrete `@openuidev/react-lang` → `NaviUINode` adapter** behind the opt-in
  `@navi/ui/openui` subpath (core stays dependency-free); **streaming /
  progressive `parsePartial`** for partial specs in both engines.
- **Patterns migrated/added**: a generic `Dialog` primitive, plus `Toast` and
  `ConfirmDialog` moved out of the Console (re-export shims keep call sites
  unchanged). `EmptyState` no longer relies on the host's global button classes.
- **Ladle** workbench with stories for every primitive, pattern, and `<GenUI>`,
  and the built-in **axe a11y** addon. (Visual-regression tooling — Playwright/
  Chromatic — is proposed as an M3+ follow-up rather than half-done here.)

### M3 — Generative UI in the chat surface

- Backend "render" capability returns NAVI UI Specs; Console renders them inline
  in `ChatMessage`/tool surfaces and round-trips `onAction` intents to NAVI.
- Advertise `systemPrompt()` to the model; iterate the vocabulary (charts,
  tables, run timelines) as app-specific registry extensions.

> **Landed (first data-driven slice, 2026-06-05):** the data-driven render path
> for "graph of tool usage in this chat" is implemented. New NAVI-native
> components ship in the library: `NaviMetricCard`, `NaviUsageChart`
> (dependency-free CSS bars), `NaviDataTable` (`src/primitives/`), and the
> `NaviToolUsagePanel` pattern (`src/patterns/`). The GenUI schema/registry and
> the OpenUI vocabulary (`openui/library.ts`, `openui/map.ts`) gained
> `metricCard`, `usageChart`/`Bar`, and `dataTable`/`Row`/`Cell` so OpenUI Lang
> renders through these domain components. The renderer-neutral `NaviDataView` TS
> type lives in `src/genui/dataview.ts`. The Console renders the OpenUI lane in
> `ChatMessage` behind an error boundary with markdown fallback; see
> [Data-Driven UI Rendering Architecture](./data-driven-ui-rendering.md).

### M4 — Ecosystem adoption

- Second NAVI app (e.g. PET) consumes `@navi/ui` to validate cross-app
  consistency; extract the package to its own repo/registry if warranted.
- Token theming presets shared across apps; design-token docs site.

---

## 8. Decisions & rationale

| Decision | Rationale |
|----------|-----------|
| **pnpm workspaces** | Ecosystem-wide uniformity + install efficiency (global store, `workspace:` protocol, single lockfile). The Console's one phantom dependency (`ai`, previously transitive via `@ai-sdk/react`) is now declared, so pnpm's strict linker is kept — no hoisting hack. |
| **Source consumption in M1** | Fast feedback, single type-check, no build-order coupling while the API stabilizes. The package is still "real" (own `package.json`/`exports`) and extractable. |
| **CSS Modules + token CSS vars** | Keeps the Console's proven, framework-free styling; theming stays dynamic via `--navi-*`. No Tailwind/CSS-in-JS migration. |
| **`react-aria-components`** | Already the Console's a11y foundation; primitives inherit accessible behaviour. |
| **Engine seam, no OpenUI dep** | Generative-UI format independence; no build coupling to an evolving external package. |
| **Declarative actions** | Generated UI stays inside NAVI's safety envelope — no code/HTML execution from model output. |
| **Vite library mode + dts (M2)** | Native CSS-Module + asset emission and first-class `.d.ts` generation; one toolchain shared with the rest of the frontend. Kept in a separate `vite.lib.config.ts` so it never leaks into vitest/Ladle. |
| **Bundle `clsx`+`zod`, externalize peers (M2)** | clsx/zod are tiny and internal-only — bundling spares consumers an install; React/react-aria/lucide are externalized so apps share one copy. |
| **Hand-authored OpenUI `LibraryJSONSchema` (M2)** | OpenUI Lang maps positional args via a schema's property order; hand-authoring it (vs. `defineComponent`+Zod+dummy renderers) keeps the adapter lean and avoids pulling React renderers we never use. |
| **Ladle over Storybook (M2)** | Vite-native and lightweight; built-in axe a11y addon; faster install/build for a small component surface. Visual-regression tooling deferred to avoid half-doing it. |

---

## 9. Build, test, try

```bash
# from web-src/
pnpm install                          # one lockfile for the workspace
pnpm run build                        # builds the Console (tsc + vite) → web/
pnpm -r test                          # runs @navi/ui + navi-console suites
pnpm run typecheck                    # @navi/ui standalone typecheck

# @navi/ui standalone library
pnpm --filter @navi/ui run build      # standalone build → dist/ (vite.lib.config.ts)
pnpm --filter @navi/ui run smoke:dist # import + typecheck the built dist artifact
pnpm --filter @navi/ui run ladle      # component workbench (stories + axe a11y) at :61000

make build-console                    # same, via the repo Makefile (cd web-src && pnpm install --frozen-lockfile && pnpm run build)
```

**Try generative UI:** run the Console and open **`/dev/genui`** — edit a NAVI UI
Spec and watch `<GenUI>` validate + render it live with `@navi/ui` primitives;
button/form actions stream into an action log. Toggle the engine to **OpenUI
Lang** to render the same UI through the opt-in adapter, and hit **Replay
streaming** to watch an incomplete spec render its valid prefix progressively.

**Browse the library:** `pnpm --filter @navi/ui run ladle` opens the Ladle
workbench with a story for every primitive, pattern, and `<GenUI>` mode, plus an
axe accessibility tab.

See also: [`web-src/packages/navi-ui/README.md`](../../web-src/packages/navi-ui/README.md)
and [`docs/architecture/navi-console-frontend.md`](./navi-console-frontend.md).
