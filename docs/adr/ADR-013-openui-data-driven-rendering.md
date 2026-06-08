# ADR-013: OpenUI as NAVI's First-Class React/Web Data-Driven Renderer

**Status:** Accepted
**Date:** 2026-06-04 (first slice implemented 2026-06-05)
**Supersedes:** Earlier auxiliary-renderer wording in this ADR. (File renamed from
`ADR-013-openui-auxiliary-renderer.md` to `ADR-013-openui-data-driven-rendering.md`
to match the first-class data-driven decision.)
**Scope:** NAVI UI generation, React/web rendering, data-driven chat rendering, component-library ownership, generated UI governance, and long-term renderer lock-in control.

> **As-built note (first proving slice, 2026-06-05):** the "graph of tool usage in
> this chat" slice is implemented. The render-intent router is a narrow
> deterministic heuristic (`internal/navi/render.Classify`) on the real chat path;
> the canonical `NaviDataView` + render payload + OpenUI Lang generation live in
> `internal/navi/render/`; tool-usage data is sourced from persisted message
> `toolParts` (event-log enrichment deferred); the console renders the OpenUI lane
> via the optional `@navi/ui/openui` engine with a markdown fallback and never
> double-renders. The slice is read-only (empty `allowedActions`); the governed
> action/query gateway remains a documented seam. See
> [Data-Driven UI Rendering Architecture](../architecture/data-driven-ui-rendering.md)
> for the full as-built section.

> **As-built note (second proving slice — prototype rendering, Block 2):** a second
> chat render lane materializes generated UI *prototypes* (mockups/demos/components/
> panels) from natural language ("create a dashboard mockup for connector health").
> It adds a distinct render intent `openui_prototype_render` and a canonical,
> renderer-neutral `NaviUIPrototype` model (`internal/navi/render/`). Prototypes use
> clearly-labeled PLACEHOLDER data (`dataSourceKind = "placeholder"`), are read-only
> (empty `allowedActions`, no `onAction` wired in the console lane), and are NOT
> persisted as artifacts. Generation is deterministic and composes the EXISTING
> OpenUI vocabulary — no new `@navi/ui` components, and OpenUI Lang stays derived,
> never canonical. Two decisions were intentionally deferred: a model-callable
> `navi.render.prototype` tool (the deterministic short-circuit is the only path
> shipped) and a dedicated `Timeline` component. Real-data intents (e.g. ordinary
> weather questions) are intentionally NOT answered with placeholder prototypes; a
> prototype is produced only when the user explicitly asks for a mockup/concept. See
> the architecture doc's second as-built section for details.

## Context

NAVI needs dynamic UI generation for chat-adjacent workflows, dashboards, task results, tool approvals, onboarding surfaces, agent run summaries, telemetry views, data inspection, and visual summaries. The current NAVI Console and most planned web or desktop-facing NAVI applications already use React, so React is an accepted runtime constraint for these surfaces.

OpenUI is promising because it provides a compact language for LLM-generated UI, a React renderer, streaming-oriented parsing, and a practical component-library workflow. NAVI should lean into those strengths for React/web data-driven rendering while preserving NAVI's authority over meaning, state, governance, action execution, and persistence.

NAVI owns its component library and component language. Some components will map across multiple final renderers, including React, OpenUI-backed React surfaces, markdown/card fallback, terminal UI, and future native or specialized renderers.

## Decision

NAVI will treat OpenUI as its **first-class React/web data-driven rendering solution**.

OpenUI is first-class for:

- generated dashboards
- visual summaries
- data-driven chat responses
- interactive panels
- reports and timelines
- telemetry and usage views
- tool-result views
- run and skill inspection surfaces
- React/web generated UI composition

OpenUI is not NAVI's meaning layer, canonical UI language, stored workflow-state model, action authority layer, memory layer, governance layer, or only UI-generation solution.

NAVI owns:

- semantic intent
- the NAVI component model and component language
- the domain component vocabulary
- the action model
- governance and validation
- skill/tool authority
- sensitive-workflow approval policy
- artifact schema decisions
- renderer selection and fallback behavior
- audit and provenance

OpenUI renders:

- data-driven meaning selected by NAVI for React/web surfaces
- OpenUI Lang when selected by NAVI's render pipeline
- streaming-friendly generated UI for supported web contexts

The guiding rule is:

```text
NAVI owns meaning.
OpenUI renders meaning.
```

## Architectural Boundaries

OpenUI must sit behind a NAVI-owned adapter boundary:

```text
NAVI UI Intent / Component Language / Data View
        ↓
Renderer Adapter Layer
        ├─ Native NAVI React components
        ├─ OpenUI Lang adapter
        ├─ Markdown/card fallback
        ├─ Terminal/TUI renderer later
        └─ Native/specialized renderers later
```

The safe integration path is:

```text
NAVI Component Model / NaviDataView
        ↓
OpenUI Adapter
        ↓
OpenUI Renderer
        ↓
React DOM
```

The prohibited integration path is:

```text
OpenUI Lang
        ↓
NAVI meaning, policy, or durable state
```

## Render Intent

NAVI should maintain a render-intent decision step in the chat/runtime path.

The render intent decides whether a response should be:

- plain text
- structured markdown
- deterministic system UI
- NAVI UI Spec
- OpenUI/data-driven render
- artifact-backed render

OpenUI should be selected when the user's request, context, or learned preference implies visual, interactive, comparative, tabular, dashboard-like, timeline-based, or data-inspection output.

Users should not need to know OpenUI exists. They should be able to ask for graphs, dashboards, timelines, comparisons, filters, tables, usage views, and interactive panels naturally. NAVI decides whether OpenUI is the right rendering target.

## Data-First Rendering

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

The renderer-specific output is not the source of truth. A graph, table, timeline, dashboard, or inspector should be backed by a structured data view with columns, rows, metadata, sensitivity labels, action bindings, fallback rendering, and trace references.

## Component-Library Policy

NAVI components must be defined from NAVI concepts first. OpenUI compatibility must not shape the core component vocabulary.

Prefer domain components such as:

- `NaviAgentRunPanel`
- `NaviToolApprovalCard`
- `NaviMemoryTrace`
- `NaviWorkflowStep`
- `NaviCeremonyPanel`
- `NaviConsoleCommand`
- `NaviDataInspector`
- `NaviMetricCard`
- `NaviUsageChart`
- `NaviToolUsagePanel`
- `NaviTracePanel`

Primitive components such as `Stack`, `Card`, `Button`, `Table`, `Chart`, and `Form` are useful, but they are presentation building blocks. The model should prefer NAVI-native domain components when the task has domain meaning.

## Generated UI Governance

Generated UI may request actions. NAVI decides whether those actions are allowed.

Every generated action must flow through the host runtime, not execute directly from model output:

```text
Generated UI action request
        ↓
schema validation
        ↓
permission check
        ↓
sensitivity classification
        ↓
approval requirement when needed
        ↓
audit log
        ↓
execution policy
        ↓
actual skill/tool/action call
```

Sensitive workflows must require stronger validation, explicit approval where appropriate, and audit logging. Generated UI must not widen tool access, bypass governance, access secrets directly, or perform privileged filesystem, repository, identity, or security mutations without the normal NAVI policy path.

OpenUI `Query`, `Mutation`, form action, button action, and generated interaction hooks must route through a governed NAVI host adapter.

## Invalid UI and Fallback Rules

OpenUI output and native generated specs must fail visibly and safely.

Required behavior:

- malformed UI does not crash the host surface
- parse failures render a safe fallback state
- the last valid streamed tree may be retained during partial generation
- invalid components, props, or actions are logged
- sensitive workflows use deterministic UI when needed
- fallback rendering remains available when OpenUI is unavailable or inappropriate
- every generated UI response preserves enough data for fallback markdown/table/card rendering

## Persistence Policy

Generated UI is not currently canonical product state.

If NAVI later persists generated UI as a long-term artifact, raw OpenUI Lang must not be the only stored format. The canonical artifact should be a NAVI-owned schema containing the UI intent, data view, component tree, data bindings, action bindings, governance metadata, renderer target, trace metadata, and optional renderer-specific source text.

Preferred:

```text
Canonical artifact: NAVI UI/data-view artifact schema
Optional rendering artifact: OpenUI Lang
```

Avoid:

```text
Canonical artifact: OpenUI Lang only
```

## Evaluation Criteria

Before expanding OpenUI usage beyond the proving slices, NAVI should evaluate:

| Area | Question |
|------|----------|
| Render routing | Did the router choose the right render mode? |
| Parse reliability | How often does generated UI render without repair? |
| Repair reliability | Can one correction loop fix malformed output? |
| Data accuracy | Does the visualization reflect the underlying structured data? |
| Local model quality | Can Ollama or another local model produce usable OpenUI/NAVI specs? |
| Latency | Is time-to-useful-UI better than markdown plus handwritten cards? |
| Component drift | How often does prompt/schema drift break generated UI? |
| Fallback quality | Can the user continue when OpenUI fails? |
| Governance | Can generated UI call only approved actions? |
| Artifact safety | Can persisted UI be re-rendered without depending on OpenUI as the only format? |
| Version risk | Can packages and language versions be pinned and upgraded intentionally? |

## First Proving Slice

Initial proving scenario:

```text
User: "Show me a graph of tool usage in this chat."
```

Expected behavior:

1. Classify as OpenUI/data-driven render.
2. Query current-chat tool-use events.
3. Aggregate tool frequency counts and recent usage metadata.
4. Construct a NAVI data view.
5. Render chart, table, and summary card through the React/web OpenUI lane.
6. Preserve fallback markdown.
7. Log render success, parse failures, fallback usage, and trace ID.

This scenario is read-only, low risk, useful, and exercises the data-driven rendering pipeline.

## Consequences

This decision allows NAVI to benefit from OpenUI as a real product lane without handing it core architectural authority.

Positive consequences:

- first-class React/web generative UI direction
- clear path for dashboard and data-driven chat rendering
- compatibility with NAVI's existing React/Vite direction
- better model output structure than raw markdown for visual workflows
- low lock-in risk because NAVI owns the semantic model and adapter boundary
- practical proving path for telemetry, tool usage, run summaries, and inspection panels

Costs and risks:

- NAVI must maintain its own component vocabulary and renderer adapters
- OpenUI package and language maturity must be monitored
- prompts and schemas can drift as components evolve
- invalid output requires telemetry, fallback UI, and correction paths
- sensitive workflows still need deterministic UI and strict governance
- render-intent misclassification must be measured and corrected

## Implementation Notes

Initial use should stay in low-risk read-only or low-side-effect surfaces:

- tool usage by chat
- agent run summaries
- tool result panels
- data inspection views
- onboarding and ceremony experiments
- local model evaluation dashboards
- generated reports
- task breakdown views
- run timelines
- skill invocation tables

Initial use should avoid:

- account and security settings
- secret handling
- privileged filesystem mutation
- repository mutation
- destructive actions
- workflows requiring deterministic UX

The existing `@navi/ui` generative UI engine seam remains the right integration point. OpenUI support should be implemented as the first-class React/web data-driven rendering lane behind NAVI-owned data-view, component, governance, action, fallback, and artifact boundaries.

## References

- `docs/canonical/data-driven-ui-rendering.md`
- `docs/architecture/data-driven-ui-rendering.md`
- `docs/architecture/navi-ui-library.md`
- `web-src/packages/navi-ui/README.md`
- `docs/architecture/navi-system-architecture-guide.md`
