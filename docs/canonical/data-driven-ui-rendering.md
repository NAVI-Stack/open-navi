# Data-Driven UI Rendering — Canonical Principle

**Status:** Active
**Last Updated:** 2026-06-04
**Updated By:** OpenUI / data-driven UI decision

## Canonical Axiom

```text
NAVI owns meaning.
OpenUI renders meaning.
```

This principle is now first-class for React and web-facing NAVI surfaces.

OpenUI is NAVI's primary data-driven rendering solution for React/web environments, especially generated dashboards, visual summaries, interactive panels, chat-rendered data views, reports, timelines, tool-result views, run summaries, and inspection surfaces.

OpenUI is not NAVI's meaning layer, state layer, governance layer, action authority layer, memory layer, or only possible renderer.

## Boundary

NAVI owns:

- semantic intent
- component meaning
- data-view shape
- action identity
- skill and tool authority
- governance and validation
- audit and provenance
- persistence and artifact schemas
- renderer selection and fallback behavior

OpenUI owns:

- first-class React/web rendering of generated data-driven UI
- OpenUI Lang rendering where selected by NAVI
- streaming-friendly generated UI presentation for supported web surfaces

The correct integration shape is:

```text
NAVI meaning / UI intent / data view
        ↓
NAVI renderer adapter boundary
        ↓
OpenUI Lang or OpenUI renderer path
        ↓
React/web surface
```

The prohibited integration shape is:

```text
OpenUI Lang
        ↓
NAVI meaning, policy, or durable state
```

## User-Facing Rule

Users should not need to know OpenUI exists.

Users may ask for visual, interactive, or data-driven results using natural requests such as:

- "show me a graph"
- "make this a dashboard"
- "compare these visually"
- "show this as a timeline"
- "make this interactive"
- "let me filter this"
- "show usage over time"
- "summarize this with cards"

NAVI decides whether those requests should be answered as prose, markdown, a deterministic system UI, a NAVI UI Spec, or an OpenUI/data-driven render.

## Render Intent

NAVI should maintain a render-intent decision step in the chat/runtime path.

The render intent decides whether a response should be:

- plain text
- structured markdown
- deterministic system UI
- NAVI UI Spec
- OpenUI/data-driven render
- artifact-backed render

OpenUI should be selected when the user's request, context, or learned preference implies visual, interactive, comparative, tabular, dashboard-like, or data-inspection output.

## Data-First Rule

Data-driven rendering must start from structured data, not from decorative UI.

The preferred flow is:

```text
query data
   ↓
normalize data
   ↓
construct NAVI data view
   ↓
choose visualization
   ↓
generate/render OpenUI
   ↓
preserve fallback output
```

A graph, table, timeline, dashboard, or inspector should be backed by a structured data view with columns, rows, metadata, sensitivity labels, action bindings, and fallback rendering.

## Skills and Capabilities

Data query capabilities and render capabilities should remain separate.

Data query skills return structured datasets and summaries. They do not render UI themselves.

Render skills or render services consume structured data views and produce a renderer-specific representation such as OpenUI Lang, NAVI UI Spec, or markdown fallback.

All skills remain governed. A render path does not bypass the Skill registry, Governor, action model, permission checks, proposal flow, or audit path.

## Chat Rendering

NAVI Chat should support mixed render payloads:

- text
- markdown
- artifact
- deterministic system card
- NAVI UI Spec
- OpenUI Lang
- data view
- fallback markdown
- render errors and trace IDs

Every generated UI response must have a fallback. If OpenUI parsing or rendering fails, the user still receives a safe textual, tabular, or deterministic-card representation of the underlying data.

## Persistence

Generated UI is not canonical product state by default.

If generated UI becomes a persistent artifact, the canonical artifact must be NAVI-owned and renderer-neutral. Raw OpenUI Lang may be stored as renderer source/debug text, but must not be the only durable representation.

Preferred:

```text
Canonical artifact: NAVI UI/data-view artifact schema
Optional renderer artifact: OpenUI Lang
```

Avoid:

```text
Canonical artifact: raw OpenUI Lang only
```

## Preference Learning

NAVI may learn user presentation preferences over time.

Examples:

- user often asks for graphs → bias toward visual summaries
- user asks "just tell me" → prefer concise text
- user expands rendered panels → use more data-driven UI
- user ignores or collapses rendered panels → use less

These preferences are inferred configuration unless explicitly set by the owner. They must not override explicit owner preferences or governance constraints.

## Governance

Generated UI may request. NAVI decides.

Any OpenUI `Query`, `Mutation`, form action, button action, or generated interaction must route through the NAVI host runtime and follow the normal validation path:

```text
generated UI request
        ↓
schema validation
        ↓
permission and policy checks
        ↓
risk and sensitivity classification
        ↓
proposal/confirmation when required
        ↓
audit and trace
        ↓
skill/tool/action execution
```

OpenUI must never widen tool access, bypass governance, access secrets directly, or execute privileged actions independently of NAVI.

## First Proving Slice

The first proving scenario is:

```text
User: "Show me a graph of tool usage in this chat."
```

Expected behavior:

1. Classify the request as OpenUI/data-driven render.
2. Query current-chat tool-use events.
3. Aggregate tool frequency counts and recent usage metadata.
4. Construct a NAVI data view.
5. Render a chart, table, and summary card through the React/web OpenUI lane.
6. Preserve fallback markdown.
7. Log render success, parse failures, and fallback usage.

This scenario is read-only, low risk, useful, and exercises the full data-driven rendering pipeline.

## Prototype Rendering (Block 2)

The second render lane is **in-chat generated UI prototypes**: a user can ask NAVI to
"create a dashboard mockup", "build a component", "mock up a panel", or "make a weather
display concept" and receive a generated visual panel in chat — without knowing OpenUI
exists.

This lane uses a distinct render intent, `openui_prototype_render`, separate from
`openui_data_render`:

- **`openui_data_render`** — real data-backed display, backed by `NaviDataView`
  (e.g. a tool-usage chart from real telemetry).
- **`openui_prototype_render`** — a generated mockup/demo/component/display, backed by a
  NAVI-owned `NaviUIPrototype`, that may use **placeholder data**.

Rules for prototype rendering:

- **Placeholder data must be honestly labeled.** Prototype renders carry
  `dataSourceKind = "placeholder"` and say, in both the rendered UI and the fallback,
  that they are a prototype/mockup with placeholder data — not live data, not saved, not
  connected to real systems.
- **Never imply placeholder values are real.** Ordinary real-data requests (weather,
  finance, health, calendar, task, telemetry) must NOT be answered with placeholder
  prototype data. A prototype is produced only when the user explicitly asks for a
  mockup/prototype/display concept. For example, "what's the weather this week?" stays a
  normal response, while "create a weather display mockup" produces a clearly-labeled
  placeholder prototype.
- **NaviUIPrototype is canonical; OpenUI Lang is derived.** The prototype model is the
  NAVI-owned meaning; OpenUI Lang is generated from it and is never canonical state.
- **Read-only.** Generated prototype UI exposes no privileged actions; buttons are inert
  prototype affordances.
- **Not persisted.** This block is in-chat only — prototypes are not saved as artifacts,
  and live artifacts remain future work.

The first prototype proving scenario is:

```text
User: "Create a dashboard mockup for connector health."
```

NAVI classifies the request as `openui_prototype_render`, builds a "Connector Health
Dashboard" `NaviUIPrototype` with clearly-labeled placeholder data, renders it through the
same OpenUI chat lane, and preserves an honest fallback markdown mockup.