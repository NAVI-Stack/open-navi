# NAVI Console Capability Management and Skill UI

**Status:** Draft implementation design  
**Last Updated:** 2026-05-06  
**Scope:** Design only. This document defines the target implementation shape for reworking the NAVI Console "Skills" page into a Capability Layer management surface. It does not authorize broad route, UI, registry, or loader changes by itself.

## Status and Scope

This is an implementation design document for a later phased rework. It records current repo integration points, target architecture, proposed API shape, migration order, and acceptance boundaries.

In scope:

- Read-only design and repo inspection.
- A target normalized capability graph.
- A V1 headless/declarative UI surface model.
- A NAVI Contacts MVP path.
- Backend and frontend implementation guidance for later tickets.

Out of scope:

- Renaming routes or changing Console behavior now.
- Adding runtime plugin UI execution.
- Changing skill, plugin, connector, or tool loading behavior now.

## Goals

- Rename the Console sidebar page from **Skills** to **Capabilities**.
- Make **Plugins** the default owner-facing management layer.
- Preserve **Skills** as canonical governed execution contracts.
- Treat **Tool Interfaces** as callable skill/interface execution surfaces.
- Treat **Connectors** as external bridge, runtime, auth, and health surfaces.
- Treat **UI Surfaces** as first-class capability components in the graph.
- Add search and filtering across every capability tab.
- Provide a headless, declarative V1 skill UI framework that can render NAVI Contacts without arbitrary plugin JavaScript or direct store writes.
- Define backend projections and APIs that let later implementation proceed in small phases.

## Non-goals

- No plugin-provided React, HTML, JavaScript, remote scripts, inline JavaScript, or arbitrary web bundles in V1.
- No marketplace install/update flow in this pass.
- No full plugin lifecycle persistence model unless the backend adds one explicitly.
- No direct Console writes to stores such as contacts, settings, skills, or plugin state.
- No PET implementation beyond keeping APIs surface-aware and reusable.
- No replacement of the existing ToolRegistry, SkillRegistry, PluginRegistry, or connector manager.
- No broad implementation in this task.

## Architecture Alignment

NAVI's Capability Layer is composed of Commands, Connectors, and Plugins.

- **Plugins** are the owner-facing package layer. A plugin may package skills, commands, connectors, permissions, interfaces, policies, constraints, config, docs, runtime, and future UI surfaces.
- **Skills** remain governed declarative execution contracts. OSS27 `SKILL.yaml` is the canonical machine-readable skill shape.
- **Tool Interfaces** are callable execution surfaces derived from skill interfaces and registered tools.
- **Connectors** are external bridges and runtime/auth surfaces.
- **UI Surfaces** are renderable presentation contracts that bind to governed skill/interface invocation.
- **Commands** remain the typed action vocabulary used by governance and runtime execution.

This design must align with the existing NCOS capability-surface contract in `docs/design/ncos-capability-surface-contract.md`: registered tools remain authoritative for callable exposure, and capability resolution must not widen beyond registry-backed policy.

The Console is a presentation and control surface. It must invoke governed runtime operations. It must not bypass `Governor`, `NAVI.ValidateAction`, proposal confirmation, the tool registry, or skill execution envelopes.

## Current Repo Integration Points

Frontend:

- `web-src/navi-console/src/components/NavSidebar.tsx` has the current sidebar item `{ path: 'skills', label: 'Skills' }`.
- `web-src/navi-console/src/app/Router.tsx` maps `#/skills` to the `Skills` page.
- `web-src/navi-console/src/pages/Skills.tsx` implements the current Skills page with four tabs: `skills`, `tools`, `plugins`, `connectors`. It defaults to `skills`.
- `web-src/navi-console/src/pages/Skills.module.css` styles the current split list/detail inspector layout.
- `web-src/navi-console/src/api/extensions.ts` defines the current React Query hooks and zod schemas for `/api/skills`, `/api/tools`, `/api/plugins`, `/api/connectors`, `/api/connectors/factories`, `/v2/api/connectors/instances`, `/api/health/connectors`, and `/api/diagnostics/connectors`.
- `web-src/navi-console/package.json` has build support only (`tsc --noEmit && vite build`). There is no project-owned frontend unit test runner today.

Gateway and API:

- `internal/gateway/server.go` registers the current routes:
  - `GET /api/plugins`
  - `GET /api/skills`
  - `GET /api/skills/{id}`
  - `POST /api/skills/reload`
  - `POST /api/skills/{id}/validate`
  - `GET /api/tools`
  - `GET /api/tools/{name}`
  - `GET /api/connectors`
  - `GET /api/connectors/factories`
  - `GET /api/connectors/setup-schema`
  - `GET /api/connectors/{name}`
  - `POST /api/connectors`
  - `DELETE /api/connectors/{name}`
  - `GET /v2/api/connectors/instances`
  - `GET /api/health/connectors`
  - `GET /api/diagnostics/connectors`
- `internal/gateway/server.go` contains `skillEntryToMap`, which currently returns a small skill projection and not the full OSS27 interface list.
- `internal/gateway/tools.go` lists and gets registered tools from `internal/tool.Registry`, with `surface` and `include_hidden` filters.
- `cmd/navid/main.go` wires `SkillRegistry`, `PluginManifests`, `ConnectorSetupDescriptors`, `ToolRegistry`, connector registry, connector manager, and the NAVI runtime into the gateway.

Skill and tool model:

- `internal/navi/skill/spec.go` defines `OSS27Spec`, `Interface`, `TransportSpec`, effects, security, governance, capability metadata, and `SkillExecutionResult`.
- `internal/navi/skill/registry.go` loads workspace and plugin skill entries, exposes `List`, `Lookup`, `FindTool`, and `Tools`.
- `internal/navi/skill/executor.go` executes `subprocess`, `subprocess_python`, `rest`, `internal`, and `mcp_tool` transports and always returns a normalized skill execution result string.
- `internal/navi/skill/internal_registry.go` registers internal skill handlers by `skill_id/interface`.
- `internal/tool/types.go` defines the first-class `Tool`, `ToolGovernance`, `ToolMetadata`, and tool status fields.
- `internal/tool/registry.go` stores registered tools, supports surface filtering, and exposes `Execute`.
- `internal/navi/tool_registry.go` derives registered skill tools from `SkillRegistry` interfaces. Current tool IDs use the shape `skill.<skill_id>.<interface>`.

Plugin model:

- `internal/navi/plugin/manifest.go` defines `Manifest`, `ManifestComponents`, component refs, permissions, runtime, lifecycle, connector manifests, and `IsActive`.
- `internal/navi/plugin/loader.go` discovers `plugin.yaml` and related manifest files across workspace, global, and built-in plugin roots.
- `internal/navi/plugin/registry.go` stores manifests, registered plugin tools, services, HTTP routes, and active plugin skill/connector/provider projections.
- `cmd/navid/plugin_bootstrap.go` imports built-in provider plugins, registers Telegram/Slack connector factories, and registers internal skill handlers such as NAVI Contacts.

Connector model:

- `connectors/connector.go` defines the shared connector interface.
- `connectors/capabilities.go` defines optional runtime capabilities discovered by type assertion.
- `internal/connectors/model.go` defines `ConnectorDriver`, `ConnectorInstance`, `CapabilityDescriptor`, `ResultEnvelope`, and setup descriptors.
- `internal/connectors/registry.go` stores factories, drivers, instances, `instancesV2`, and `InstanceMetadata`.
- `internal/connectors/manager.go` owns connector workers, dispatch, stop/start, health, diagnostics, circuit state, and `InvokeAction` for `messaging.send`.
- `internal/connectors/diag.go` defines the current diagnostic ring.

NAVI Contacts:

- `plugins/navi-contacts/plugin.yaml` defines plugin `navi.contacts`, kind `domain`, trust tier `builtin`, capabilities `contacts.read`, `contacts.write`, `contacts.vcf`, and skill component `skills/navi-contacts`.
- `plugins/navi-contacts/skills/navi-contacts/SKILL.yaml` defines the governed `navi-contacts` skill and interfaces: `create_contact`, `get_contact`, `list_contacts`, `update_contact`, `delete_contact`, `search_contacts`, `import_vcf`, `export_vcf`, and `share_contact`.
- `plugins/navi-contacts/handlers/contacts_handler.go` registers internal handlers for every NAVI Contacts skill interface.
- `plugins/navi-contacts/handlers/vcf.go` implements vCard import/export helpers.
- `internal/store/contact.go` is the backing contact store, but the Console must call skill interfaces rather than this store directly.

Tests to extend:

- `internal/gateway/server_test.go` already covers `/api/tools`, `/api/skills/reload`, and gateway route behavior.
- `internal/navi/skill/executor_test.go` and `internal/navi/skill/internal_executor_test.go` cover skill execution envelopes and internal handlers.
- `internal/tool/registry_test.go`, `internal/tool/adapters_test.go`, and related `internal/tool/*_test.go` cover registry and execution behavior.
- `internal/connectors/*_test.go` covers connector registry, worker, diagnostics, lifecycle, and v2 instance behavior.
- `cmd/navid/plugin_bootstrap_test.go` and `cmd/navid/start_connector_test.go` cover plugin/bootstrap connector wiring.
- `plugins/navi-contacts` has no dedicated handler test file today; NAVI Contacts MVP should add one.
- `web-src/navi-console` has no project-owned component/unit tests today; V1 should at least keep `npm run build` green and consider adding a frontend test runner before complex renderers land.

## Console Information Architecture

Sidebar:

- Keep the route stable at `#/skills` for the first phase unless the router adds an alias. Change the visible label to **Capabilities**.
- A later compatibility cleanup may add `#/capabilities` and redirect `#/skills`, but route renaming is not part of the first implementation ticket.

Tabs:

```txt
Plugins | Skills | Tool Interfaces | Connectors | UI Surfaces | Docs
```

Default tab: **Plugins**.

Search and filters:

- Every tab must have a search box.
- Search must match IDs, display names, descriptions, plugin owners, tags, domains, status values, paths, and component types where available.
- Filters should include status dimensions appropriate to each tab.
- The search/filter state should be tab-local in V1.
- Empty states should distinguish "none installed" from "no results for current filters".

Tab responsibilities:

- **Plugins:** primary package inventory and control layer.
- **Skills:** secondary index of governed execution contracts.
- **Tool Interfaces:** secondary index of callable interfaces and registered tools.
- **Connectors:** runtime/auth/health management for external bridges.
- **UI Surfaces:** renderable presentation contracts by owner plugin, skill, connector, or system.
- **Docs:** capability-linked local documentation, README files, runbooks, and future generated docs.

## Capability Graph Model

Backend should add a normalized projection endpoint:

```txt
CapabilityGraph {
  plugins: PluginNode[]
  skills: SkillNode[]
  toolInterfaces: ToolInterfaceNode[]
  connectors: ConnectorNode[]
  uiSurfaces: UISurfaceNode[]
  docs: DocNode[]
  edges: CapabilityEdge[]
}
```

```go
type CapabilityGraph struct {
    Plugins        []PluginNode        `json:"plugins"`
    Skills         []SkillNode         `json:"skills"`
    ToolInterfaces []ToolInterfaceNode `json:"toolInterfaces"`
    Connectors     []ConnectorNode     `json:"connectors"`
    UISurfaces     []UISurfaceNode     `json:"uiSurfaces"`
    Docs           []DocNode           `json:"docs"`
    Edges          []CapabilityEdge    `json:"edges"`
}
```

Canonical JSON shape:

```json
{
  "plugins": [],
  "skills": [],
  "toolInterfaces": [],
  "connectors": [],
  "uiSurfaces": [],
  "docs": [],
  "edges": []
}
```

Node identity:

- Plugin node ID: `plugin:<plugin_id>`
- Skill node ID: `skill:<skill_id>`
- Tool interface node ID: `tool_interface:<skill_id>:<interface_name>` for skill interfaces, or `tool_interface:<tool_id>` for non-skill registered tools.
- Connector node ID: `connector:<driver_or_instance_id>`
- UI surface node ID: `ui_surface:<owner_type>:<owner_id>:<surface_id>`
- Doc node ID: `doc:<owner_type>:<owner_id>:<doc_id_or_path_hash>`

Common node fields:

```go
type CapabilityNodeStatus struct {
    Lifecycle   string `json:"lifecycle"`
    Validation  string `json:"validation"`
    Availability string `json:"availability"`
    Runtime     string `json:"runtime"`
    Auth        string `json:"auth"`
    Health      string `json:"health"`
    UI          string `json:"ui"`
}

type CapabilityNodeBase struct {
    ID          string               `json:"id"`
    Kind        string               `json:"kind"`
    Name        string               `json:"name"`
    Description string               `json:"description,omitempty"`
    OwnerPlugin string               `json:"ownerPlugin,omitempty"`
    Source      string               `json:"source,omitempty"`
    Path        string               `json:"path,omitempty"`
    Tags        []string             `json:"tags,omitempty"`
    Domains     []string             `json:"domains,omitempty"`
    Status      CapabilityNodeStatus `json:"status"`
    RawRef      map[string]string    `json:"rawRef,omitempty"`
}
```

Suggested specialized fields:

- `PluginNode`: `version`, `kind`, `trustTier`, `capabilities`, `categories`, `components`, `permissions`, `configSchema`, `actions`.
- `SkillNode`: `skillID`, `version`, `trustTier`, `riskTier`, `sideEffects`, `requiresConfirmation`, `interfaces`, `governance`, `actions`.
- `ToolInterfaceNode`: `skillID`, `interfaceName`, `toolID`, `transport`, `inputSchema`, `outputSchema`, `riskTier`, `sideEffects`, `requiresConfirmation`, `commandType`, `actions`.
- `ConnectorNode`: `driverID`, `instanceID`, `category`, `capabilities`, `setupSchema`, `health`, `diagnostics`, `actions`.
- `UISurfaceNode`: `surfaceID`, `ownerType`, `ownerID`, `targetSurface`, `renderMode`, `schema`, `actions`.
- `DocNode`: `title`, `path`, `docType`, `ownerType`, `ownerID`.

Edges:

```go
type CapabilityEdge struct {
    From string `json:"from"`
    To   string `json:"to"`
    Type string `json:"type"`
}
```

Initial edge types:

- `plugin_declares_skill`
- `plugin_declares_connector`
- `plugin_declares_doc`
- `plugin_declares_ui_surface`
- `skill_exposes_interface`
- `interface_registered_as_tool`
- `skill_requires_capability`
- `skill_provides_capability`
- `connector_provides_capability`
- `ui_surface_invokes_interface`
- `doc_documents_node`
- `plugin_owns_runtime`

The graph must not assume UI belongs only to skills. V1 may discover UI metadata from `SKILL.yaml`, but the graph model must allow plugin-owned and connector-owned UI surfaces later.

## Status Vocabulary

Status is multi-dimensional. Do not collapse it into one badge.

`lifecycle`:

- `enabled`
- `disabled`

`validation`:

- `valid`
- `invalid`
- `unvalidated`

`availability`:

- `available`
- `gated`
- `blocked`
- `unavailable`

`runtime`:

- `running`
- `stopped`
- `degraded`
- `not_applicable`
- `unknown`

`auth`:

- `configured`
- `unconfigured`
- `expired`
- `error`
- `not_required`

`health`:

- `healthy`
- `warning`
- `failing`
- `unknown`

`ui`:

- `none`
- `available`
- `invalid`
- `unsupported_surface`
- `blocked`

Mapping guidance:

- Plugin `lifecycle` maps from manifest active/enabled state. Today `Manifest.IsActive()` is the closest existing signal.
- Plugin `validation` maps from `Manifest.Validate()`.
- Skill `availability` maps from `SkillEntry.Activatable` plus `ReasonsUnbound`.
- Skill `validation` maps from `skill.ValidateSpec`.
- Tool interface `availability` maps from registered tool status and executor presence.
- Connector `runtime` maps from registry/manager status, v2 instance metadata, and manager health.
- Connector `auth` maps from `InstanceMetadata.AuthState` where available.
- UI `ui` maps from discovered UI metadata validation and supported render mode.

## Lifecycle Controls

Plugin level:

- `enable`
- `disable`
- `validate`
- `reload`
- `configure`
- `view_logs`
- `restart_runtime` if applicable

Skill level:

- `validate`
- `inspect`
- `open_owning_plugin`
- `enable`/`disable` only when the skill is standalone or an explicit child override exists. A plugin-owned skill should normally inherit plugin lifecycle.

Connector level:

- `configure_auth`
- `test_connection`
- `restart`
- `disable`
- `view_dependents`

Tool interface level:

- Inspect only for now unless a real backend policy or governance editing surface already exists.
- Invocation may be exposed only through governed debug/demo flows, not as a general policy editor.

Existing controls:

- `POST /api/skills/reload` exists.
- `POST /api/skills/{id}/validate` exists.
- `POST /api/connectors` and `DELETE /api/connectors/{name}` exist.
- Connector start/restart currently exists inside the daemon and manager (`StartOne`, `StopOne`) but not as a clean owner-facing restart endpoint.
- Plugin enable/disable currently exists in memory via `plugin.Registry.SetPluginEnabled`, but there is no persisted API surface. Do not expose plugin toggles until persistence and reload semantics are designed.

## Plugin Inspector Design

Plugin inspector is the primary detail panel.

Sections:

- Header: name, plugin ID, kind, version, trust tier, lifecycle, validation, availability.
- Overview: description, categories, declared capabilities, source path.
- Components: skills, tool interfaces, connectors, UI surfaces, docs, policies, providers, runtime.
- Controls: show only actions supported by backend action metadata.
- Permissions and governance: `permissions`, trust tier, declared policy refs, data access hints.
- Config: `config_schema`, configured/unconfigured state, secret redaction.
- Runtime: services, connector instances, provider state, health and diagnostics.
- UI Surfaces: available surfaces, target surface support, validation errors.
- Docs: README and plugin docs links.
- Raw manifest: full manifest JSON for debugging.

The plugin inspector should link every child node to its secondary index detail panel while preserving the plugin as the owner-facing home.

## Skill Inspector Design

Skill inspector is a governed execution contract view.

Sections:

- Header: display name, skill ID, version, owning plugin, trust tier, lifecycle, validation, availability.
- Contract: interfaces, transport types, input/output schemas, error schemas.
- Governance: risk tier, side effects, idempotency, reversibility, confirmation requirement, command type, data access, sandbox.
- Dependencies: required capabilities, required env/binaries if surfaced by loader, connector dependencies when added.
- UI Surfaces: declarative surfaces attached to this skill.
- Runtime: executable/unexecutable status per interface.
- Actions: validate, inspect raw spec, open owning plugin. Enable/disable only for standalone skills or explicit child override.
- Raw definition: `SKILL.yaml` projection and loader metadata.

Current gap: `GET /api/skills/{id}` does not return the full `OSS27Spec.Interfaces` list through `skillEntryToMap`; the graph builder should use the registry directly and include interfaces.

## Tool Interface Inspector Design

Tool Interface means "a callable interface that the runtime can expose or execute." It includes skill interfaces and registered tools.

Sections:

- Header: display name, interface ID, tool ID, source type, owner skill, owner plugin.
- Callable contract: input schema, output schema, error schema, transport.
- Governance: command type, workspace action, risk tier, reversibility, confirmation requirement, authority, domain.
- Availability: registered tool status, hidden flag, surface visibility, executor presence.
- Routing: visible surfaces, exposure class, interaction modes, model/tool capability requirements where available.
- Invocation history: future execution outcomes filtered by tool ID/interface.
- Raw tool payload: `internal/tool.Tool` projection.

V1 control behavior:

- Inspect only.
- Debug invocation can be added later only through `POST /api/skills/{id}/interfaces/{name}/invoke`, with governance and confirmation. It must not call stores directly.

## Connector Inspector Design

Connector inspector is a runtime/auth/bridge view.

Sections:

- Header: driver ID, instance ID, display name, category, lifecycle, runtime, auth, health.
- Driver: setup descriptor, capability descriptors, plugin owner.
- Instance: status, health state, auth state, labels, granted scopes, policy refs.
- Runtime: connected time, queue depth, send count, error count, last send, last error, circuit/degraded state when available.
- Diagnostics: recent entries from `Manager.Diag.List()`.
- Dependents: skills, plugins, UI surfaces, or tools that require this connector/capability.
- Controls: configure auth, test connection, restart, disable, view dependents, only where backend supports or new backend endpoints are explicitly added.

Current gaps:

- `/api/diagnostics/connectors` returns `Diagnostic` with JSON field `connector`, while `web-src/navi-console/src/api/extensions.ts` expects `name`. The current Console filters diagnostics by `d.name`, so connector diagnostics may not appear correctly.
- Connector runtime status words differ across endpoints: registry uses `connected/disconnected`, manager health uses `healthy/degraded/down`, and v2 metadata uses `configured/connected` plus `health_state`. The graph projection should normalize these into the status vocabulary.

## UI Surface Framework

UI Surfaces are capability graph nodes.

V1 source:

- V1 UI metadata may come from `SKILL.yaml`.
- V1 may also allow manifest-declared surfaces later, but the initial parser should not hard-code "UI belongs to skills".
- Since `OSS27Spec` currently has no `ui` field, implementation needs a backward-compatible extension such as `ui_surfaces,omitempty` or `capability.ui_surfaces,omitempty`. Unknown YAML fields are currently ignored by Go structs, so a typed field is needed before metadata can be served.

Supported render mode:

- `headless`

V1 forbidden inputs:

- remote scripts
- inline JavaScript
- arbitrary HTML
- plugin React bundles
- iframe-sourced plugin apps
- direct database/store mutation actions

V1 allowed primitives:

- Text, headings, descriptions, badges, tables, lists, forms, field groups, tabs, detail panels, empty states, loading states, error states, confirmation prompts.
- Input controls generated from JSON Schema for bound interfaces.
- Read views backed by skill interface invocations such as `list_contacts`, `get_contact`, and `search_contacts`.
- Mutating actions bound to skill interface invocations such as `create_contact`, `update_contact`, `delete_contact`, `import_vcf`, and `export_vcf`.

Action rules:

- UI actions must bind to existing skill interfaces.
- UI must call a governed invocation endpoint.
- UI must never write directly to stores.
- Delete, high-risk, irreversible, external, or broad-scope actions inherit governance and confirmation requirements from the skill/interface/tool metadata.
- Confirmation requirements must be enforced server-side; UI confirmation is a user experience layer, not an authority boundary.

Suggested V1 UI surface shape:

```yaml
ui_surfaces:
  - id: contacts.console
    target: console
    render_mode: headless
    title: NAVI Contacts
    owner:
      type: skill
      id: navi-contacts
    data:
      initial:
        interface: list_contacts
        args:
          owner_type: all
          limit: 50
    actions:
      - id: search
        interface: search_contacts
        input_binding: query
      - id: create
        interface: create_contact
      - id: update
        interface: update_contact
      - id: delete
        interface: delete_contact
        confirmation: required
```

The renderer can begin with a small internal registry of known renderer templates keyed by declarative metadata. It should not evaluate plugin code.

## NAVI Contacts MVP

NAVI Contacts is the MVP skill UI example because it already has:

- A plugin manifest: `plugins/navi-contacts/plugin.yaml`.
- A governed skill: `plugins/navi-contacts/skills/navi-contacts/SKILL.yaml`.
- Real internal handlers: `plugins/navi-contacts/handlers/contacts_handler.go`.
- Internal store backing: `internal/store/contact.go`.

MVP Console behavior:

- Plugin inspector for `navi.contacts` shows the Contacts skill and its interfaces.
- UI Surfaces tab shows `contacts.console` once metadata exists.
- Contacts UI renders from declarative metadata in headless mode.
- Initial read calls `list_contacts`.
- Search calls `search_contacts`.
- Detail calls `get_contact`.
- Create calls `create_contact`.
- Edit calls `update_contact`.
- Delete calls `delete_contact` and must require confirmation because it is irreversible at the user experience level even though current skill-level effects are broad and currently `requires_confirmation: false`.
- Import calls `import_vcf`.
- Export/share calls `export_vcf` or `share_contact`.

MVP risk to resolve: current `plugins/navi-contacts/skills/navi-contacts/SKILL.yaml` uses one top-level effects block for all interfaces and marks `delete_contact` as not requiring confirmation. The UI framework needs interface-level risk overrides or a conservative server-side confirmation rule for destructive interface names until the spec supports per-interface effects.

## Backend API Changes

Add:

```txt
GET /api/capabilities/graph
```

Returns the normalized `CapabilityGraph`. It should aggregate:

- plugin manifests from `PluginManifests`
- skills from `SkillRegistry.List`
- tool interfaces from `ToolRegistry.List` and skill `Spec.Interfaces`
- connectors from `Registry.Drivers`, `Registry.InstancesV2`, `Registry.InstanceMetadata`, manager health, and diagnostics
- UI surfaces from typed metadata on skills/manifests when added
- docs from manifest/docs metadata or known plugin docs paths when added

Add:

```txt
GET /api/skill-ui?surface=console|pet
```

Returns all valid UI surfaces for the requested target. V1 should return only `render_mode=headless` surfaces. Unknown or unsupported render modes must be marked `unsupported_surface`, not silently executed.

Add:

```txt
GET /api/skills/{id}/ui
```

Returns UI surfaces attached to a skill. This is a convenience endpoint for skill inspectors and MVP development. It should be a filtered projection of the same underlying UI surface registry used by `/api/skill-ui`.

Add:

```txt
POST /api/skills/{id}/interfaces/{name}/invoke
```

Invokes one skill interface through the governed runtime path.

Minimum server behavior:

- Resolve `{id}` to a loaded skill by `Spec.SkillID` or current registry lookup compatibility.
- Resolve `{name}` to an interface on that skill.
- Resolve the associated registered tool ID if present (`skill.<skill_id>.<interface>` today).
- Validate request JSON against the interface input schema where feasible.
- Construct a governance action descriptor from skill/interface metadata.
- Enforce governor, proposals, and confirmation requirements.
- Execute through `ToolRegistry.Execute` when the tool is registered, otherwise through `skill.Execute` only if that route still applies the same governance checks.
- Return the normalized `SkillExecutionResult`.
- Never expose an endpoint that directly calls `store.SaveContact`, `store.DeleteContact`, or equivalent backing stores.

Lifecycle endpoints:

- Reuse existing `POST /api/skills/reload`.
- Reuse existing `POST /api/skills/{id}/validate`.
- Reuse existing connector setup/list endpoints where practical.
- Do not add plugin enable/disable endpoints until persistence, reload semantics, and child-skill behavior are defined.
- Add connector restart/test endpoints only when backed by explicit manager operations and tests. Candidate future routes:
  - `POST /api/connectors/{name}/restart`
  - `POST /api/connectors/{name}/test`

## Frontend Changes

Phase 1 frontend changes:

- In `web-src/navi-console/src/components/NavSidebar.tsx`, change visible label from `Skills` to `Capabilities`.
- Keep `#/skills` route initially.
- In `web-src/navi-console/src/pages/Skills.tsx`, rename local page language to Capabilities when the implementation ticket lands.
- Change tabs to `Plugins | Skills | Tool Interfaces | Connectors | UI Surfaces | Docs`.
- Default active tab to `plugins`.
- Add search/filter controls for every tab.
- Prefer new `useCapabilitiesGraph` hook over stitching multiple endpoints in the page once `/api/capabilities/graph` exists.

Phase 2 frontend changes:

- Split the current large `Skills.tsx` into smaller components:
  - `Capabilities.tsx`
  - `CapabilitiesPluginsTab.tsx`
  - `CapabilitiesSkillsTab.tsx`
  - `CapabilitiesToolInterfacesTab.tsx`
  - `CapabilitiesConnectorsTab.tsx`
  - `CapabilitiesUISurfacesTab.tsx`
  - `CapabilitiesDocsTab.tsx`
  - shared `CapabilitySearchBar`
  - shared `CapabilityStatusBadges`
- Add `web-src/navi-console/src/api/capabilities.ts` for graph and skill UI APIs.
- Keep `web-src/navi-console/src/api/extensions.ts` as compatibility until the page is fully graph-backed.

Phase 3 frontend changes:

- Add a headless UI renderer that consumes validated UI surface metadata.
- Add the NAVI Contacts Console surface.
- Add governed action invocation through `POST /api/skills/{id}/interfaces/{name}/invoke`.

## Testing Plan

Backend unit/gateway tests:

- Add `GET /api/capabilities/graph` tests in `internal/gateway/server_test.go`.
- Assert graph includes plugin, skill, tool interface, connector, UI surface, and doc arrays even when empty.
- Assert graph edges connect `navi.contacts` to `navi-contacts` and skill interfaces when the plugin is loaded.
- Assert status dimensions use only the documented vocabulary.
- Assert invalid plugin manifests appear as validation errors only if diagnostic projection is included; otherwise they must not crash graph generation.
- Assert `/api/skill-ui?surface=console` returns only supported `headless` surfaces.
- Assert unsupported render modes are marked `unsupported_surface`.
- Assert `/api/skills/{id}/interfaces/{name}/invoke` enforces governance and returns normalized `SkillExecutionResult`.

Skill/tool tests:

- Extend `internal/navi/skill/executor_test.go` or `internal/navi/skill/internal_executor_test.go` for UI-bound internal interface invocation.
- Extend `internal/tool/adapters_test.go` for skill tool execution path parity.
- Add tests for NAVI Contacts handlers under `plugins/navi-contacts/handlers`.

Connector tests:

- Extend `internal/connectors/registry_test.go` or related tests to verify graph-ready driver/instance metadata.
- Extend gateway connector tests for normalized connector status mapping.

Frontend verification:

- Keep `npm run build` passing in `web-src/navi-console`.
- Add a frontend test runner before or during renderer work if the headless UI renderer becomes non-trivial.
- Add fixture-based zod schema tests for `CapabilityGraph` and UI surface payloads.

Manual checks:

- Start NaviD through Docker Compose or local daemon mode.
- Open Console.
- Confirm sidebar says Capabilities.
- Confirm Plugins is the default tab.
- Confirm all six tabs render and search/filter does not overlap or resize controls.
- Confirm NAVI Contacts appears as plugin, skill, tool interfaces, and UI surface when metadata is added.

## Phased Migration Plan

Phase 0 - document and inventory:

- Complete this design document.
- Do not change runtime behavior.

Phase 1 - graph backend:

- Add graph structs and builder near gateway or a new small internal capability projection package.
- Add `GET /api/capabilities/graph`.
- Normalize status vocabulary.
- Add backend tests.

Phase 2 - Console IA:

- Rename sidebar label to Capabilities.
- Keep `#/skills` route for compatibility.
- Change default tab to Plugins.
- Add six tabs and tab-local search/filter.
- Prefer graph data for list/detail projections.

Phase 3 - inspector completeness:

- Build plugin inspector as primary control layer.
- Build skill, tool interface, connector, UI surface, and docs inspectors.
- Expose only action buttons returned by backend action metadata.

Phase 4 - UI surface metadata:

- Extend skill/plugin manifest structs with typed UI surface metadata.
- Validate headless render mode.
- Add `/api/skill-ui?surface=console|pet` and `/api/skills/{id}/ui`.
- Add tests for unsupported/invalid UI surfaces.

Phase 5 - governed invocation:

- Add `POST /api/skills/{id}/interfaces/{name}/invoke`.
- Route invocation through governance and existing execution envelopes.
- Add confirmation handling for destructive/high-risk actions.

Phase 6 - NAVI Contacts MVP:

- Add declarative Contacts UI metadata.
- Render list/search/detail/create/edit/delete/import/export from headless UI.
- Ensure delete and other high-risk actions are confirmed and governed.

Phase 7 - lifecycle controls:

- Add persisted plugin lifecycle only after semantics are explicit.
- Add connector restart/test endpoints if manager behavior and tests are ready.
- Add logs/dependents views.

## Risks and Gaps

- Existing `/api/skills` projection omits full interface details; graph builder must use registry entries directly.
- Existing `/api/plugins` returns raw manifests but no validation status, active reason, diagnostics, or runtime state.
- Existing plugin enable/disable is in-memory only and not safe as a Console control yet.
- Existing connector status terms are inconsistent across endpoints and need normalization.
- Existing connector diagnostics JSON field is `connector`, while the Console schema expects `name`.
- Existing NAVI Contacts skill has top-level effects only; per-interface risk/confirmation is needed for destructive UI actions.
- Existing `OSS27Spec` lacks typed UI surface metadata.
- Existing frontend has no unit test runner.
- Direct skill invocation over HTTP does not exist yet; adding it must be done carefully so the Console does not become a governance bypass.

## Recommended First Implementation Ticket

Implement `GET /api/capabilities/graph` as a read-only backend projection with tests. It should aggregate current plugin, skill, tool, and connector data, emit the normalized status vocabulary, and include empty `uiSurfaces` and `docs` arrays until typed metadata lands. This gives the Console one truthful inventory surface before any UI behavior or lifecycle controls change.
