# Data-Driven UI Rendering Architecture

**Status:** Active design
**Last Updated:** 2026-06-05
**Updated By:** OpenUI / data-driven UI decision

> **Implementation status — first proving slice (as-built, 2026-06-05)**
>
> The "graph of tool usage in this chat" slice is implemented end-to-end. As-built
> details, where they refine the design below:
>
> - **Render-intent router** is a deterministic, narrow heuristic
>   (`internal/navi/render.Classify`) wired into the real chat path as a
>   short-circuit in `AgentLoop.ExecuteRun` (`internal/navi/runtime_executor.go`),
>   alongside the existing bounded-diagnostic short-circuits. It only routes
>   tool-usage requests that carry an explicit visual/data cue (graph, chart,
>   table, dashboard, timeline, visualize); plain "what tools did you use"
>   questions stay prose/markdown. Model-assisted routing is a documented seam, not
>   yet built.
> - **NaviDataView**, **RenderPayload (ChatMessageRenderPayload)**, OpenUI Lang
>   generation, and fallback markdown live in the renderer-neutral Go package
>   `internal/navi/render/` (decoupled from `internal/navi` and `internal/runtime`
>   to avoid import cycles).
> - **Tool-usage data source** is the chat's persisted assistant-message
>   `toolParts` metadata (chat-scoped, already persisted). This is the FIRST
>   source, not the final one: it lacks per-call durations/precise timestamps. The
>   richer future source is the event log / runtime traces (`tool.call.*` events).
>   No data is fabricated — an empty chat yields an honest degraded view.
> - **Transport:** the payload is carried as opaque JSON on `RunState.RenderPayload`
>   and merged into the assistant message's `metadata_json` under `renderPayload`
>   (no schema migration). The reply's text content is the fallback markdown so
>   non-UI clients (connectors/API) still get the data.
> - **Console:** `ChatMessage.tsx` renders the OpenUI lane via the optional
>   `@navi/ui/openui` engine (lazy-loaded; if the optional peer is unavailable it
>   degrades to fallback markdown), wrapped in a `GenUIErrorBoundary`. On a
>   successful OpenUI render the message text is suppressed (no duplicate render);
>   on parse/render failure or unavailability the markdown fallback is shown.
> - **NAVI-native components** added to `@navi/ui`: `NaviMetricCard`,
>   `NaviUsageChart` (dependency-free CSS bars), `NaviDataTable`, and the
>   `NaviToolUsagePanel` pattern. The OpenUI vocabulary (`openui/library.ts`,
>   `openui/map.ts`) and GenUI schema/registry were extended with `metricCard`,
>   `usageChart`/`Bar`, and `dataTable`/`Row`/`Cell` so OpenUI Lang renders through
>   these NAVI domain components.
> - **Governance:** the slice is read-only. Tool-usage views declare empty
>   `allowedActions`; no OpenUI interaction is wired to a privileged endpoint. The
>   governed action/query gateway is a documented seam
>   (`internal/navi/render/gateway_seam.go`), not yet built.
>
> **Phase 2 (capability awareness + test suite, as-built):**
> - **Model awareness.** Beyond the deterministic short-circuit, NAVI now exposes a
>   model-callable read-only tool `navi.render.visualize` (registered in
>   `internal/navi/tool_registry_adapters.go`, executor in `render_intake.go`). It
>   rides the same ICS/registry path as file tools and attaches the render payload
>   to the run via a context sink (mirroring `send_reply`). The chat system prompt
>   (`internal/prompts/defaults/chat/system.md`, `ncos/chat_behavior.md`) advertises
>   the capability so NAVI stops denying it can graph. The deterministic
>   short-circuit remains the zero-latency fast-path.
> - **Token/model usage** is a delegated follow-up
>   (`docs/tasks/token-usage-capability.md`); only `tool_usage` is implemented.
> - **Tests.** Go: render unit tests + render-tool registration/executor tests
>   (`internal/navi/render_tool_test.go`); Go e2e (`test/e2e/scenarios/render_test.go`,
>   `make test-e2e`, docker — not CI-gated). Frontend: component/map/render-payload
>   tests + accessibility (`vitest-axe`) for the NAVI data components and the chat
>   render surface. A manual AI-on-AI browser script lives at
>   `docs/testing/data-driven-ui-ai-test.md`.
> - **CI regression gate.** A blocking `frontend` job in `.github/workflows/ci.yml`
>   runs typecheck + vitest (incl. a11y) for `@navi/ui` and the Console
>   (`make test-frontend`). The Go e2e render scenario is runnable but not a
>   required gate.

> **Implementation status — second proving slice: prototype rendering (Block 2, as-built)**
>
> The "create a dashboard mockup for connector health" slice adds a SECOND chat
> render lane for generated UI *prototypes* (mockups/demos/components/panels), built
> on the same renderer-neutral package and the same `mode: "openui"` console lane.
>
> - **New render intent** `openui_prototype_render` (`internal/navi/render/types.go`),
>   distinct from `openui_data_render`. Data render = real telemetry backed by
>   `NaviDataView`; prototype render = generated mockup backed by `NaviUIPrototype`
>   with clearly-labeled PLACEHOLDER data.
> - **New canonical model** `NaviUIPrototype` (`internal/navi/render/prototype_types.go`)
>   — renderer-neutral, NOT durable, NOT persisted as an artifact in this slice.
>   `NormalizePrototypePurpose` sanitizes the purpose on every construction path
>   (unknown/empty/garbage → `component`).
> - **Deterministic classifier** `render.ClassifyPrototype`
>   (`prototype_classify.go`) is conservative, with three trigger paths: (a) an
>   *unconditional* phrase (`mock up`, `mock it up`, `mockup`, `wireframe`,
>   `visual demo`, `interactive demo`, `show me a ui`); (b) a *contextual* phrase
>   (`show me what`, `what would a`, `could look like`, `render this as`,
>   `turn this into`, …) **paired with a UI-surface noun**; or (c) a build/create cue
>   (`create/build/make/mock/prototype/wireframe/design/generate`) paired with a
>   surface noun. `show` is deliberately NOT a generic cue. Ordinary chat such as
>   "what would a normal day look like?", "render this as markdown", or "show me the
>   weather this week" therefore does not trigger; weather renders as a prototype only
>   with explicit mockup language ("create a weather display mockup").
> - **Data-vs-prototype precedence.** The data-render classifier (`Classify`) keeps
>   priority and is checked first in `AgentLoop.ExecuteRun`; prototype is the
>   fallthrough. To stop the data lane from claiming UI-mockup requests that merely
>   mention "tool calls", `Classify` defers (returns plain) when a build cue is paired
>   with an action-on-tools subject (`approve/approving/approval`, `review/reviewing`,
>   …) — e.g. "build me a dashboard for approving tool calls" → prototype, while
>   "create a dashboard of tool usage in this chat" → data.
> - **Deterministic generation** (`prototype_builder.go`, `prototype_openui.go`,
>   `prototype_fallback.go`, `prototype_payload.go`) composes the EXISTING OpenUI
>   vocabulary (Card/Badge/Text/MetricCard/DataTable/Row/Cell) — no new `@navi/ui`
>   components were added. The connector-health dashboard is concrete; other purposes
>   get a small purpose-shaped placeholder scaffold.
> - **Honesty in all four places:** `DataSourceKind = "placeholder"` on both the
>   `NaviUIPrototype` and the top-level `RenderPayload` (two new optional fields:
>   `prototype`, `dataSourceKind`); an honest banner in the fallback markdown; and an
>   embedded `Badge("PROTOTYPE")` + warning `Text` node inside the OpenUI program. The
>   console also shows a "Prototype · placeholder data" chip (`PrototypeBadge` in
>   `ChatMessage.tsx`).
> - **Read-only:** `Governance.AllowedActions` is empty and the console's OpenUI lane
>   wires no `onAction`, so generated buttons are inert by construction.
> - **Deferred:** a model-callable `navi.render.prototype` tool (the deterministic
>   short-circuit is the only path this slice ships); a `Timeline` component; and all
>   artifact persistence / live-artifact / weather-provider work.
> - **Tests.** Go: `internal/navi/render/prototype_test.go` (classifier triggers /
>   non-triggers / precedence / purpose normalization / honesty / payload shape);
>   Go e2e: a prototype scenario in `test/e2e/scenarios/render_test.go`. Frontend:
>   prototype render/fallback/label/read-only cases in `ChatMessage.test.tsx`, the
>   prototype a11y case in `ChatMessage.a11y.test.tsx`, and payload-parsing in
>   `renderPayload.test.ts`.

## Purpose

This document defines the architecture for NAVI's first-class data-driven rendering path in chat and React/web surfaces.

The governing principle is:

```text
NAVI owns meaning.
OpenUI renders meaning.
```

OpenUI is first-class for React/web data-driven rendering. It remains bounded by NAVI-owned meaning, data-view shape, governance, skills, action authority, fallback behavior, and persistence policy.

## Architecture at a Glance

```text
User request
   ↓
Runtime / chat intake
   ↓
Render Intent Router
   ├─ plain text
   ├─ structured markdown
   ├─ deterministic system UI
   ├─ NAVI UI Spec
   └─ OpenUI data-driven render
          ↓
      data capability / query skill
          ↓
      NaviDataView
          ↓
      render planner
          ↓
      OpenUI adapter / renderer
          ↓
      React/web chat surface
```

OpenUI is selected for visual, interactive, comparative, charted, tabular, dashboard-like, or inspection-oriented responses.

## Render Intent Router

The Render Intent Router decides how a response should be presented.

Initial output enum:

```ts
type RenderIntent =
  | "plain_response"
  | "markdown_structured"
  | "deterministic_system_ui"
  | "navi_ui_spec"
  | "openui_data_render"
  | "artifact_render";
```

Signals that should bias toward `openui_data_render` include:

- graph, chart, dashboard, visual, timeline, map
- compare, trend, frequency, breakdown, usage over time
- filter, sort, inspect, interactive, drill down
- data-heavy tool results
- repeated follow-up analysis over a dataset
- learned user preference for visual/data panels

The router should not be keyword-only. It may use heuristics, model-assisted classification, available surface capabilities, conversation context, and learned user preference.

## NaviDataView

NAVI should introduce a renderer-neutral data-view object before producing OpenUI Lang.

Illustrative shape:

```ts
type NaviDataView = {
  id: string;
  title: string;
  intent: "chart" | "table" | "dashboard" | "timeline" | "inspector" | "form";
  dataset: {
    columns: Array<{
      key: string;
      label: string;
      type: "string" | "number" | "datetime" | "boolean" | "duration";
    }>;
    rows: Record<string, unknown>[];
  };
  suggestedViews?: Array<{
    type: "bar" | "line" | "pie" | "table" | "cards" | "timeline";
    rationale: string;
  }>;
  actions?: Array<NaviActionBinding>;
  governance: {
    dataSensitivity: "public" | "local" | "private" | "sensitive";
    allowedActions: string[];
    auditLevel: "none" | "basic" | "sensitive";
  };
  fallback?: {
    markdown?: string;
    summary?: string;
  };
  trace?: {
    runId?: string;
    messageId?: string;
    sourceSkillIds?: string[];
  };
};
```

`NaviDataView` is the semantic object. OpenUI Lang is one rendering target.

## Chat Render Payload

Chat messages should support renderer-specific and fallback payloads.

Illustrative shape:

```ts
type ChatRenderMode =
  | "text"
  | "markdown"
  | "artifact"
  | "navi-ui"
  | "openui"
  | "system-card"
  | "proposal";

type ChatMessageRenderPayload = {
  mode: ChatRenderMode;
  sourceText?: string;
  naviUiSpec?: unknown;
  openuiLang?: string;
  dataView?: NaviDataView;        // real data render (canonical telemetry)
  prototype?: NaviUIPrototype;    // prototype render (canonical mockup); dataView is nil
  dataSourceKind?: "real" | "placeholder" | "mixed"; // "placeholder" ⇒ prototype label
  fallbackMarkdown?: string;
  traceId?: string;
  errors?: RenderError[];
};
```

The payload must preserve enough data for fallback rendering and render-error diagnostics.

## Data Capability Separation

Data query capabilities should return structured data, not UI.

Examples:

- `navi.telemetry.query_tool_usage`
- `navi.telemetry.query_model_usage`
- `navi.runs.query_run_history`
- `navi.skills.query_skill_invocations`
- `navi.chat.query_conversation_events`
- `navi.errors.query_failure_counts`

Render capabilities should transform data views into renderer-specific formats.

Examples:

- `navi.render.suggest_visualization`
- `navi.render.generate_openui`
- `navi.render.generate_navi_ui_spec`
- `navi.render.repair_openui`
- `navi.render.explain_render_failure`

This keeps data acquisition, visualization choice, and rendering output decoupled.

## OpenUI Chat Renderer

The React/Vite Console should support OpenUI as a chat message renderer.

Required host behavior:

- render OpenUI inside the normal chat stream
- support streaming/progressive rendering where available
- wrap renderer in an error boundary
- preserve fallback markdown/table output
- emit parse/render telemetry
- include trace IDs for debugging
- prevent OpenUI-generated actions from bypassing NAVI governance

## Governed Tool Provider

OpenUI `Query`, `Mutation`, form action, and generated interaction hooks must route through a governed host adapter:

```text
OpenUI toolProvider/action
        ↓
NAVI render/action gateway
        ↓
schema validation
        ↓
Governor / policy checks
        ↓
sensitivity classification
        ↓
Proposal Queue when required
        ↓
audit and execution trace
        ↓
skill/tool/action execution
```

OpenUI must never call privileged backend endpoints directly unless those endpoints are explicitly designed as safe read-only render data endpoints.

## NAVI-Native Component Vocabulary

Prefer NAVI domain components over generic primitives when domain meaning exists.

Initial components:

- `NaviMetricCard`
- `NaviUsageChart`
- `NaviToolUsagePanel`
- `NaviDataTable`
- `NaviTracePanel`
- `NaviRunTimeline`
- `NaviSkillInvocationTable`
- `NaviFailureSummary`
- `NaviProposalCard`
- `NaviDataInspector`

Generic primitives such as `Stack`, `Card`, `Button`, `Table`, `Chart`, and `Form` remain useful, but they should not replace domain semantics.

## Preference Adaptation

NAVI should learn rendering preferences through lightweight signals:

- user asks for a graph, dashboard, table, timeline, or interactive view
- user says "just tell me" or asks for less UI
- user expands/collapses rendered panels
- user regenerates a visual as text or text as visual
- user asks follow-up questions based on rendered data

Preference updates are inferred configuration unless explicitly set by the owner. They must not override explicit owner-set preferences or governance constraints.

## Fallback and Failure Handling

Every generated UI response needs fallback behavior.

Failure cases:

- render-intent misclassification
- data query failure
- empty dataset
- malformed OpenUI Lang
- unknown component
- invalid action binding
- missing tool provider
- renderer crash

Required fallback outcomes:

- safe markdown/table output when possible
- explicit degraded message when data is unavailable
- traceable error record
- no silent success
- no crash of the chat surface

## First Proving Slice

Scenario:

```text
User: "Show me a graph of tool usage in this chat."
```

Expected pipeline:

1. Classify as `openui_data_render`.
2. Query current chat tool-use events.
3. Aggregate tool counts and last-used metadata.
4. Create `NaviDataView`.
5. Generate/render OpenUI chart + table + summary card.
6. Preserve fallback markdown table.
7. Log render success/failure and trace ID.

Expected visual result:

- bar chart of tool frequency
- table of tool name, count, and last used
- metric card for total tool calls
- optional filter by message range or time range

This slice is low-risk because it is read-only, local to conversation telemetry, and does not require side-effecting actions.

## Implementation Phases

### Phase 1 — Documentation and decision alignment

- Update ADR-013 from auxiliary OpenUI renderer to first-class React/web data-driven rendering lane.
- Add canonical data-driven UI rendering principle.
- Add this architecture document.

### Phase 2 — Render intent and data model

- Add Render Intent Router.
- Add `NaviDataView` and chat render payload types.
- Add fallback markdown contract.

### Phase 3 — Read-only telemetry data capability

- Add tool-usage query for current chat.
- Aggregate counts, last-used timestamps, and message/run linkage.

### Phase 4 — OpenUI chat render path

- Add OpenUI renderer in chat message surface.
- Add error boundary, fallback, parse telemetry, and trace handling.

### Phase 5 — Governed toolProvider

- Route OpenUI interactions through NAVI validation/governance/audit.
- Keep initial scope read-only unless explicitly approved.

### Phase 6 — Component vocabulary

- Add NAVI-native data components.
- Prefer domain components where meaning exists.

### Phase 7 — Preference learning

- Track lightweight presentation preference signals.
- Feed inferred preferences into render intent selection.
