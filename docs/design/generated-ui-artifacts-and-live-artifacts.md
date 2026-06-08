# Generated UI, Artifacts, and Live Artifacts

**Status:** Evolving design
**Last Updated:** 2026-06-05
**Updated By:** Data-driven UI / artifact planning discussion

## Purpose

This document records the product and architecture direction for generated UI, artifact persistence, and future live artifacts in NAVI.

It is intentionally scoped as a design companion to the existing artifact specification, not a replacement for it.

Primary references:

- `docs/specs/artifact-system-v1.md`
- `docs/canonical/data-driven-ui-rendering.md`
- `docs/architecture/data-driven-ui-rendering.md`
- `docs/adr/ADR-013-openui-data-driven-rendering.md`
- `docs/architecture/navi-ui-library.md`

## Core Position

NAVI should be able to materialize useful visual displays, dashboards, components, and personal utilities from natural language.

The governing principle remains:

```text
NAVI owns meaning.
OpenUI renders meaning.
```

Generated UI is a presentation and interaction lane. Artifacts are the persistence and lifecycle layer for user-meaningful work products. Live artifacts are not a separate product silo; they are an artifact archetype.

## Why This Matters

NAVI is a personal AI, not only a developer or enterprise dashboard tool. Generated UI should therefore support both professional and everyday personal use cases.

Examples:

- weather displays
- weekly plans
- task overviews
- travel prep cards
- shopping comparison views
- budget snapshots
- health/wellness summaries
- memory timelines
- project status views
- connector health dashboards
- tool usage panels
- generated UI component prototypes

The goal is not to make everything a dashboard. The goal is to let NAVI create durable, useful personal visual utilities when a visual surface is more valuable than text.

## Layered Capability Model

Generated UI and artifacts should be developed in layers.

### 1. In-chat generated UI

A user asks for a visual, dashboard, component, display, chart, table, prototype, or demo. NAVI renders it directly in the chat stream.

Examples:

```text
Show me a graph of tool usage in this chat.
Create a dashboard mockup for connector health.
Make me a weather display for this week.
Build a component for approving tool calls.
```

This layer is currently being proven through the data-driven UI branch.

### 2. Snapshot artifacts

A generated UI or structured work product can be saved as an artifact with durable identity, metadata, provenance, versioning, and reopen behavior.

Example:

```text
Save that weather display.
```

NAVI creates an artifact record and preserves the canonical meaning object, fallback representation, render target metadata, provenance, and source context.

### 3. Live artifacts

A live artifact is an artifact whose content can refresh or update from declared data sources and policies.

Example:

```text
Keep this weather display updated.
```

The artifact remains the durable user-facing object. Its rendered view may update over time through governed refresh operations.

### 4. Artifact revision and incremental modification

A user can revise an existing artifact naturally.

Examples:

```text
Add humidity to this weather display.
Also show air quality.
Make it more compact.
Change this from daily to hourly.
```

NAVI resolves the target artifact, plans an update, validates the change, creates a new version, and re-renders the artifact. It does not create a disconnected second display unless the user asks for a branch or copy.

## Unified Artifact Model

Artifacts are one umbrella concept in NAVI.

```text
Artifact
  ├─ Snapshot artifact
  ├─ Live artifact
  ├─ Editable artifact
  ├─ Exported artifact
  ├─ Synced artifact
  └─ Future archetypes
```

A live artifact is an artifact archetype, not a separate system.

This differs from product models where ordinary artifacts and live/collaborative artifacts are treated as separate modes. NAVI should keep artifact identity, provenance, versioning, governance, and Library behavior unified across archetypes.

## Relationship to Existing Artifact Spec

The existing Artifact System specification already establishes the correct foundation:

- artifacts are durable work products
- artifacts live as World Model entities
- versions are immutable saved states
- branches represent alternate lines of evolution
- references link artifacts to messages, conversations, projects, sources, exports, shares, and external systems
- renderer/editor behavior is registry-driven
- assistant/tool/Skill mutations go through governed artifact operations
- every mutation emits History records and provenance
- Library is the Experience Layer artifact manager surface

This design adds a specific forward direction: generated UI, data-driven views, prototype renders, and future live artifacts should be modeled as artifact-compatible payloads and archetypes rather than independent UI-only state.

## Generated UI vs Artifact

Not every generated UI should automatically become an artifact.

### In-chat generated UI

Temporary visual response. Useful inside the conversation, but not necessarily durable.

Examples:

- one-off chart
- temporary comparison table
- lightweight visual summary
- quick prototype preview

### Artifact-worthy generated UI

Durable work product likely to be reused, edited, refreshed, exported, or reopened.

Examples:

- weather display the user wants to keep
- dashboard the user wants to revisit
- project status view
- reusable approval component
- generated report or planning board
- live connector health panel

The Cognitive Layer decides whether generated UI crosses the artifact threshold, using the artifact qualification policy.

## Canonical Payloads

Generated UI must have a NAVI-owned source of truth before it becomes durable.

Known canonical payload families:

### NaviDataView

Use for real structured data-backed displays.

Examples:

- tool usage chart
- weather forecast display
- model usage table
- budget snapshot
- task distribution graph

### NaviUIPrototype

Sibling model for generated mockups, dashboards, UI components, and visual demos that may use placeholder or conceptual data.

> **As-built (Block 2):** `NaviUIPrototype` is now implemented as an **in-chat,
> non-persisted** canonical model in `internal/navi/render/` (Go). It backs the
> `openui_prototype_render` lane: deterministic classification → `NaviUIPrototype`
> with `dataSourceKind = "placeholder"` → derived OpenUI Lang + honest fallback
> markdown → `mode: "openui"` chat payload. It is **not** yet wrapped in an artifact
> envelope or persisted; artifact persistence remains future work (Block 4).

Illustrative shape:

```ts
type NaviUIPrototype = {
  id: string;
  title: string;
  purpose: "dashboard" | "component" | "workflow" | "demo" | "inspector" | "display";
  prompt: string;
  dataSourceKind: "real" | "placeholder" | "mixed";
  placeholderPolicy: "none" | "allowed" | "required";
  componentIntent: unknown;
  fallbackMarkdown: string;
  governance: {
    allowedActions: string[];
    auditLevel: "none" | "basic" | "sensitive";
  };
  trace?: {
    runId?: string;
    messageId?: string;
    artifactId?: string;
  };
};
```

### Artifact wrapper

When persisted, these payloads should sit inside a unified artifact envelope.

Illustrative shape:

```ts
type ArtifactPayload =
  | { kind: "data_view"; view: NaviDataView }
  | { kind: "ui_prototype"; prototype: NaviUIPrototype }
  | { kind: "document"; markdown: string }
  | { kind: "code"; files: unknown }
  | { kind: "presentation"; deck: unknown };
```

OpenUI Lang may be stored as renderer source or cached render material, but it must not be the only canonical durable representation.

## Data Source Honesty

Every generated display should make data provenance clear.

```text
dataSourceKind = real | placeholder | mixed
```

Rules:

- If real data is available, use it and cite/source it internally through provenance.
- If real data is unavailable, do not fabricate it as real.
- Prototype placeholder data must be labeled as placeholder.
- Mixed displays must distinguish real and placeholder sections.

This is especially important for personal utilities such as weather, finance, health, calendar, and task views.

## Weather Display as Motivating Example

Weather is the right personal-AI proving case because it is useful, simple to understand, and naturally exercises the future artifact stack.

### Step 1 — conversational answer

```text
User: What's the weather this week?
NAVI: Provides text summary.
```

### Step 2 — generated display in chat

```text
User: Make me a weather display for this week.
NAVI: Renders a generated weather panel in chat.
```

### Step 3 — saved snapshot artifact

```text
User: Save that.
NAVI: Creates a weather display artifact with identity, provenance, and version 1.
```

### Step 4 — live artifact

```text
User: Keep this updated.
NAVI: Adds refresh policy and source binding to the artifact.
```

### Step 5 — incremental revision

```text
User: Add humidity and air quality.
NAVI: Updates the existing artifact, creates a new version, and re-renders.
```

This should become a future proving slice for live artifacts, not part of the current data-driven UI proving slice.

## Incremental Update Semantics

Incremental generated UI updates should use artifact versioning, not invisible mutation.

When a user asks to modify an existing generated display:

1. Resolve target artifact or in-chat render.
2. Determine whether the request is a patch, replace, branch, or metadata update.
3. Validate against artifact type/subtype and renderer registry.
4. Apply the update to the NAVI-owned canonical payload.
5. Create a new version if persisted.
6. Regenerate renderer output, such as OpenUI Lang.
7. Preserve fallback markdown or raw-view fallback.
8. Emit History/provenance records.

For persisted artifacts, the prior version remains recoverable.

## Live Artifact Archetype

A live artifact is an artifact with declared refresh/update behavior.

Illustrative fields:

```ts
type LiveArtifactDescriptor = {
  refreshPolicy: {
    mode: "manual" | "scheduled" | "event_driven";
    schedule?: string;
    staleAfter?: string;
  };
  dataBindings: Array<{
    bindingId: string;
    sourceKind: "skill" | "connector" | "internal_query" | "external_api";
    sourceRef: string;
    queryShape: unknown;
    sensitivity: "public" | "local" | "private" | "sensitive";
  }>;
  updatePolicy: {
    autoCommit: boolean;
    requireProposalForExternalEffects: boolean;
    preserveManualEdits: boolean;
  };
};
```

Live artifact refresh is a governed operation. A refresh may be autonomous only if it is read-only, permissioned, and within the user's autonomy settings. External writes, publishing, sharing, and destructive updates remain governed by the Proposal Queue where required.

## Relationship to Skills and Connectors

Skills and connectors supply capabilities and data. They do not own artifact truth.

A weather display might depend on:

- a weather connector or weather skill
- location context
- user preference configuration
- a generated data view
- an OpenUI renderer target
- an artifact record for persistence

The Capability Layer returns structured data. The Cognitive Layer decides how to materialize it. The Artifact System owns durable identity and versions.

## Sequencing

Do not build the whole artifact/live-artifact system inside the current data-driven UI slice.

Recommended sequence:

### Block 1 — Data-driven chat render slice

Land current work: tool usage visualization, NaviDataView, OpenUI render payload, fallback markdown, chat integration.

### Block 2 — Prototype rendering in chat

Add prompt-to-visual/prototype generation for requests such as dashboards, displays, components, and demos. This is OpenUI-backed but not automatically persisted.

> **Status: implemented (in-chat only).** The `openui_prototype_render` lane,
> `NaviUIPrototype` canonical model, deterministic classifier, placeholder-honesty
> labeling, and OpenUI/fallback rendering are landed. Persistence (Block 4) and live
> artifacts (Block 5) remain future work.

### Block 3 — Artifact architecture refresh

Update artifact docs to explicitly include generated UI payloads, live artifact archetypes, data source honesty, and OpenUI renderer boundaries.

### Block 4 — Snapshot artifact persistence

Allow users to save generated displays/prototypes as artifacts. Start with snapshot artifacts only.

### Block 5 — Live artifact proving slice

Weather display is the recommended first live artifact proving slice.

### Block 6 — Artifact revision flow

Support natural-language updates to existing artifacts, such as adding humidity or changing a view from daily to hourly.

## Open Questions

1. What artifact subtype should generated UI use first?
   - `data_view/openui`
   - `display/openui`
   - `prototype/openui`
   - another renderer-neutral subtype

2. Should in-chat render payloads be promotable to artifacts directly, or should promotion always regenerate a canonical artifact payload?

3. How much OpenUI Lang should be persisted?
   - renderer cache only
   - source/debug text
   - not persisted unless explicitly exported

4. How should live artifacts expose stale/refresh state in the Library and chat?

5. What is the minimum renderer registry work needed before generated UI artifacts are safe to save?

6. Should weather be implemented as a connector, skill, or internal read-only capability first?

7. How should user location and weather provider preferences be governed?

8. How should live artifact refresh interact with schedules and background tasks?

## Non-Goals for the Current Branch

Do not implement these in the current data-driven UI proving slice:

- full artifact persistence for generated UI
- live artifact refresh engine
- weather connector/provider integration
- artifact revision UI
- multi-user collaboration
- CRDT/OT editing
- arbitrary external sync
- generated UI action execution beyond governed read-only seams

## Near-Term Delegation Prompt

Use this when ready to delegate the next documentation/design pass:

```text
Audit and update NAVI's artifact documentation to align with data-driven generated UI and future live artifacts.

Working branch: feat/data-driven-ui or the current data-driven UI integration branch.

Read first:
- docs/specs/artifact-system-v1.md
- docs/design/generated-ui-artifacts-and-live-artifacts.md
- docs/canonical/data-driven-ui-rendering.md
- docs/architecture/data-driven-ui-rendering.md
- docs/adr/ADR-013-openui-data-driven-rendering.md
- docs/architecture/navi-ui-library.md

Goal:
Update the artifact docs so they explicitly support generated UI payloads, OpenUI-rendered displays, snapshot artifacts, live artifact archetypes, data source honesty, and natural-language artifact revision flows.

Do not implement runtime code yet. This is a documentation/design alignment pass.

Required outcomes:
1. Preserve the unified artifact model: live artifacts are an artifact archetype, not a separate system.
2. Clarify how NaviDataView and future NaviUIPrototype payloads can be persisted as artifact payloads.
3. Clarify that OpenUI Lang is renderer output/cache/source text, not canonical artifact truth.
4. Add weather display as a motivating future proving slice for live artifacts.
5. Add dataSourceKind/provenance rules for real, placeholder, and mixed generated displays.
6. Clarify artifact promotion from in-chat generated UI to saved artifact.
7. Clarify incremental update/versioning semantics for generated UI artifacts.
8. Track open questions and non-goals so this does not derail the current data-driven UI slice.
9. Update relevant indexes if needed.

Keep the changes focused and avoid rewriting the whole artifact spec unless necessary.
```
