# Artifact Alignment — Generated UI and Live Artifacts

**Status:** Proposed alignment addendum
**Last Updated:** 2026-06-05
**Updated By:** Data-driven UI / artifact alignment pass

## Purpose

This addendum aligns the existing [Artifact System V1 specification](artifact-system-v1.md) with NAVI's data-driven UI and OpenUI rendering direction.

It does not replace `artifact-system-v1.md`. It clarifies how generated UI, OpenUI-rendered displays, snapshot artifacts, and future live artifacts fit into the existing artifact model without creating a parallel artifact system.

Related documents:

- [Artifact System V1](artifact-system-v1.md)
- [Generated UI, Artifacts, and Live Artifacts](../design/generated-ui-artifacts-and-live-artifacts.md)
- [Data-Driven UI Rendering — Canonical Principle](../canonical/data-driven-ui-rendering.md)
- [Data-Driven UI Rendering Architecture](../architecture/data-driven-ui-rendering.md)
- [ADR-013: OpenUI Data-Driven Rendering](../adr/ADR-013-openui-data-driven-rendering.md)
- [NAVI UI Library Architecture](../architecture/navi-ui-library.md)

## Alignment Decision

Generated UI does not create a separate persistence model.

```text
Artifact is the umbrella.
Live artifact is an artifact archetype.
Generated UI artifact is an artifact subtype/payload family.
OpenUI is a renderer target, not artifact truth.
```

The existing Artifact System already defines the correct durable layer: artifacts have identity, versions, branches, references, snapshots, provenance, renderer/editor registry behavior, governance integration, History linkage, and Library surfaces.

This addendum narrows the next design direction: data-driven and generated UI must plug into that system rather than bypass it.

## Terminology

### In-chat generated UI

A visual/data-driven render shown inside the chat stream. It may be temporary and may never become durable.

Examples:

- tool usage graph
- generated dashboard preview
- weather display preview
- UI component prototype
- timeline or inspector panel

### Generated UI artifact

A persisted artifact whose canonical payload describes a generated visual/display/prototype. It may render through OpenUI, native NAVI UI, markdown fallback, or future renderers.

### Snapshot artifact

An artifact whose content is saved at a point in time. It does not refresh automatically. Most generated UI artifacts should start here.

### Live artifact

An artifact with declared data bindings and refresh/update policy. Live artifact status is an archetype/status/capability of the same artifact model, not a separate system.

### Renderer output

A target-specific render representation such as OpenUI Lang, rendered HTML, markdown fallback, or a native component tree. Renderer output is derived from canonical artifact payloads and may be cached, but it does not replace canonical truth.

## Artifact Type Alignment

The existing V1 artifact types remain valid:

- `document`
- `code`
- `data`
- `presentation`

Generated UI can initially fit under `data` or a deferred display/prototype subtype without changing the top-level V1 type list.

Recommended near-term subtype strategy:

| Use case | Type | Candidate subtype | Notes |
|---|---|---|---|
| Real structured display | `data` | `data_view` | Backed by `NaviDataView`. |
| OpenUI-rendered data display | `data` | `data_view.openui` | OpenUI Lang is renderer output/cache, not canonical truth. |
| Generated visual prototype | `data` or deferred `template/form` | `ui_prototype` | Backed by future `NaviUIPrototype`. |
| Weather display snapshot | `data` | `weather_display` | Starts as snapshot artifact; may become live later. |
| Live weather display | `data` | `weather_display.live` | Same artifact family plus live descriptor. |

Do not introduce a new top-level `live` type. Live is an archetype/capability attached to an artifact.

## Canonical Payload Families

Generated UI artifacts should persist a NAVI-owned canonical payload.

### NaviDataView

Use for real data-backed displays.

Examples:

- current chat tool usage
- weather forecast display
- model usage dashboard
- task distribution summary
- budget or spending snapshot

`NaviDataView` owns the semantic data shape: columns, rows, suggested views, governance metadata, fallback markdown, and trace/provenance fields.

### NaviUIPrototype

Use for generated mockups, dashboards, UI components, and demonstrations that may rely on placeholder or conceptual data.

This model is the canonical sibling to `NaviDataView` for prompt-to-visual/prototype rendering. As of Block 2 it is implemented as an **in-chat, non-persisted** canonical model in `internal/navi/render/` (Go), carried on the chat render payload (`mode: "openui"`, `dataSourceKind: "placeholder"`). It is **not yet persisted** as an artifact; when persistence lands it must sit inside the artifact/version envelope below, with OpenUI Lang treated as derived renderer output (never the sole canonical truth).

### Artifact envelope

When persisted, canonical payloads sit inside the artifact/version model.

Illustrative artifact payload union:

```ts
type ArtifactPayload =
  | { kind: "data_view"; view: NaviDataView }
  | { kind: "ui_prototype"; prototype: NaviUIPrototype }
  | { kind: "document"; markdown: string }
  | { kind: "code"; files: unknown }
  | { kind: "presentation"; deck: unknown };
```

The exact wire schema remains implementation-owned. The invariant is that the persisted artifact must have a NAVI-owned canonical payload, not only renderer-specific source.

## OpenUI Persistence Boundary

OpenUI is first-class for React/web rendering, but OpenUI Lang must not become the only durable artifact representation.

Allowed uses of OpenUI Lang in artifact storage:

- renderer source text for debugging
- cached render material
- export material when the user explicitly exports OpenUI/HTML-like output
- replay input for a renderer, as long as canonical payload also exists

Disallowed:

- persisting only raw OpenUI Lang as artifact truth
- treating OpenUI component names as the only semantic component model
- letting OpenUI actions bypass artifact operations, governance, or History records

## Data Source Honesty

Generated displays must carry data-source provenance.

Recommended field:

```ts
type DataSourceKind = "real" | "placeholder" | "mixed";
```

Rules:

1. Real data must come from a declared internal query, Skill, Connector, user input, or artifact source.
2. Placeholder data must be clearly labeled in UI and metadata.
3. Mixed displays must distinguish which sections are real and which are placeholder.
4. Personal, financial, health, weather, calendar, location, or task displays must never imply placeholder values are real.
5. If real data is unavailable, NAVI may offer a prototype, but must say it is a prototype.

This applies before and after artifact persistence.

## Promotion from Chat Render to Artifact

In-chat generated UI can be promoted to an artifact when it crosses artifact qualification thresholds or the user explicitly requests it.

Promotion flow:

1. Resolve the source chat message and render payload.
2. Extract or reconstruct the NAVI-owned canonical payload.
3. Determine artifact type/subtype.
4. Capture source message, conversation, project, policy, and data source snapshots.
5. Create artifact, branch, version, references, and History record.
6. Store renderer output only as derived/cached material.
7. Show artifact card/link in chat and Library.

If the in-chat render only contains renderer-specific output and no canonical payload, promotion should either fail safely or regenerate a canonical payload before commit.

## Incremental Update Semantics

Generated UI artifact updates follow artifact versioning, not invisible mutation.

When a user says:

```text
Add humidity to this weather display.
Also show air quality.
Make this dashboard more compact.
Change this to hourly view.
```

NAVI should:

1. Resolve the target artifact using artifact id, current open artifact, or recent message linkage.
2. Classify the operation as patch, replace, branch, or metadata update.
3. Validate against artifact type/subtype and renderer/editor registry.
4. Update the canonical payload.
5. Create a new version if the artifact is persisted.
6. Regenerate renderer output such as OpenUI Lang.
7. Preserve fallback representation.
8. Emit History/provenance records.

The previous artifact version remains recoverable.

## Live Artifact Semantics

A live artifact is an artifact with declared refresh/update behavior.

Illustrative descriptor:

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

Live refresh is a governed operation:

- read-only refresh may run autonomously if permissions and autonomy settings allow
- external writes, sync, publish, or share operations require normal artifact governance
- refresh failures must surface as degraded artifact state, not silent success
- refresh should create a version when user-visible artifact content changes
- manual edits must not be overwritten silently by refresh

## Weather Display Proving Slice

Weather is the recommended future live-artifact proving slice because it is personal, useful, simple to understand, and naturally demonstrates refreshable generated UI.

Suggested sequence:

1. User asks for weather in text.
2. User asks for a weather display.
3. NAVI renders an in-chat weather display with real or clearly-labeled placeholder data.
4. User says "save that".
5. NAVI creates a snapshot weather artifact.
6. User says "keep this updated".
7. NAVI adds live descriptor and refresh policy.
8. User says "add humidity and air quality".
9. NAVI updates the existing artifact and creates a new version.

Do not implement this in the current data-driven UI proving slice. Use it as the next major proving slice after snapshot generated UI artifacts exist.

## Governance Alignment

Generated UI artifacts inherit artifact governance:

- all assistant/tool/Skill mutations go through governed artifact operations
- all committed changes create versions and History/provenance records
- generated UI actions may request, but NAVI decides
- OpenUI cannot call privileged endpoints directly
- share/export/sync remain governed operations
- live refresh respects Autonomy settings and Proposal Queue hard floors

Skills and Connectors may supply data or execution. They do not own artifact durable truth.

## Renderer and Fallback Alignment

Generated UI artifacts must be renderable through a registry entry or a safe fallback.

Minimum fallback requirements:

- metadata view if renderer unavailable
- raw canonical payload view where safe
- markdown/table fallback for `NaviDataView`
- clear unsupported-editing state if editor unavailable
- explicit failure when renderer output is malformed

No generated UI artifact should become inaccessible merely because OpenUI or a specific renderer is unavailable.

## Documentation Consistency Rules

Use these rules when updating related docs:

1. Do not describe live artifacts as a separate system.
2. Do not describe OpenUI as artifact truth.
3. Do not describe generated UI as always automatically persisted.
4. Do not describe placeholder/prototype data as real data.
5. Do not let Skills, Connectors, or OpenUI own artifact durable state.
6. Keep artifact updates versioned and provenance-backed.
7. Keep Library as the Experience Layer artifact manager surface.
8. Keep ArtifactWorkspaceState as session/UI state, not World Model truth.

## Near-Term Implementation Non-Goals

The current data-driven UI branch should not attempt to implement:

- full generated UI artifact persistence
- live artifact refresh engine
- weather provider integration
- artifact revision UI
- artifact action execution through OpenUI
- external sync for generated displays
- collaborative editing or CRDT/OT behavior

Those should be sequenced after the current chat render slice and prototype render slice.

## Recommended Next Doc Updates

When ready for a deeper artifact spec refresh, update `artifact-system-v1.md` in place to include:

1. generated UI examples in artifact qualification and automatic creation triggers
2. `data_view` / generated display subtype guidance
3. explicit OpenUI persistence boundary
4. data-source honesty rules
5. snapshot vs live artifact archetype language
6. promotion flow from in-chat generated UI to saved artifact
7. live artifact descriptor and weather proving slice
8. fallback requirements for generated UI artifacts

Until then, this addendum is the alignment source for generated UI and live artifact interpretation.
