# Plugin Console Detail Architecture

How the NAVI Console renders a plugin's contents, and how to extend it without
regressing visibility.

## Data source: the capability graph

Everything the detail view shows comes from **one** read-only endpoint:

```
GET /api/capabilities/graph   →  internal/gateway/capabilities.go
```

It returns `{ plugins, skills, toolInterfaces, connectors, uiSurfaces, docs, edges }`.
Each non-plugin node carries `parentPluginId`, and the graph also contains directed
`edges` such as `plugin_owns_skill`, `skill_exposes_interface`,
`component_provides_ui`, `plugin_declares_doc`, and `plugin_uses_connector`.

The frontend client + zod schemas live in
`web-src/navi-console/src/api/capabilities.ts` (`useCapabilitiesGraph()`).

## Grouping resources under a plugin

`web-src/navi-console/src/pages/plugin-detail/resources.ts`:

- `findPluginNode(graph, pluginId)` — locate the plugin node by `pluginId` or node id.
- `collectPluginResources(graph, plugin)` — returns `{ plugin, skills, toolInterfaces,
  connectors, uiSurfaces, docs }`. A resource is attributed to the plugin if **either**
  its `parentPluginId` matches **or** a graph edge links the plugin node to it. Using
  both means the view does not depend on a single linking convention.

## Section registry (extensibility core)

`web-src/navi-console/src/pages/plugin-detail/sectionRegistry.tsx` exports
`pluginSectionRegistry: PluginSection[]`. Each entry:

```ts
interface PluginSection {
  id: string;
  label: string;
  icon: ComponentType;
  count(resources): number;     // 0 ⇒ section hidden, unless alwaysShow
  alwaysShow?: boolean;          // e.g. the Validation & Status section
  render(resources): ReactNode;
}
```

`PluginDetailPage` iterates the registry, skipping empty sections. **To add a new
plugin resource type, append one entry here — no other file changes are required.**
Built-in sections: Custom UI, Skills, Tool Calls, Connectors, Configuration, Docs,
Validation & Status.

## Custom UI: schema-driven entity manager

A plugin or skill declares custom UI via `ui_surfaces` in its manifest / SKILL.yaml.
The graph surfaces these as `uiSurfaces` nodes with `renderMode`, `schema`, and
`actionBindings`.

`web-src/navi-console/src/components/plugin-ui/EntityManagerPanel.tsx` renders any
surface whose `schema.type === 'entity_manager'`. It reads the schema's
`list` / `detail` / `search` / `forms` / `import_export` bindings plus the
`actionBindings` array, and drives every operation through:

```
POST /api/skills/{id}/interfaces/{interface}/invoke   →  invokeSkillInterface()
```

Create/update forms are generated from the matching tool interface's `inputSchema`.
Because it is fully schema-driven, the **Contacts** plugin's Contact Manager works
with zero Contacts-specific code; any future plugin exposing an `entity_manager`
surface gets a manager for free. Unknown `schema.type` values fall back to a JSON
preview rather than crashing.

To expose a custom UI from a new plugin: add a `ui_surfaces` entry with
`target_surfaces: ["console"]`, `render_mode: "headless"`, an `entity_manager`
`schema`, and an `actions` list binding ids (`list`, `search`, `create`, `update`,
`delete`, `import`, `export`) to skill interfaces.

## Plugin lifecycle endpoints

`internal/gateway/plugins.go` (registered in `server.go` `routes()`):

| Route | Effect |
|-------|--------|
| `POST /api/plugins/{id}/enable`  | `Config.SetPluginEnabled(id, true)` |
| `POST /api/plugins/{id}/disable` | `Config.SetPluginEnabled(id, false)` |
| `POST /api/plugins/{id}/validate`| recompute `manifest.Validate()` → `{valid, reasons}` |
| `POST /api/plugins/{id}/reload`  | `Config.ReloadPlugins()` |

Hooks are wired in `cmd/navid/main.go` from the `*plugin.Registry`. Unknown id ⇒ 404;
a nil hook ⇒ 501.

**Caveat:** enable/disable mutates the in-memory registry/tool visibility and is **not**
persisted across a daemon restart. Persisting plugin enabled-state is a follow-up.

## Routing & regression safeguards

`/plugins` → list (`PluginsPage`); `/plugins/:id` → `PluginDetailPage`
(`components/shell/PageRouter.tsx`). The reserved sub-path `/plugins/skills` still
renders the list (legacy hash redirect compatibility).

The original drill-down (`pages/Skills.tsx`) was deleted during the console cleanup
(commit `8503e480`), which is the regression this work restores. To prevent recurrence:

- The detail view is driven entirely by the capability graph + section registry, so a
  plugin's contents stay visible as long as the graph endpoint and registry exist.
- **Tests** run via `vitest` (`npm test` in `web-src/navi-console`; config in
  `vite.config.ts`, setup in `src/test/setup.ts`):
  - `src/app/router.test.tsx` — `/plugins/:section` is parsed into a plugins route with a
    `section` param (guards the detail route).
  - `src/components/shell/PageRouter.test.tsx` — `/plugins/:id` maps to `PluginDetailPage`,
    bare `/plugins` and the reserved `/plugins/skills` map to the list.
  - `src/pages/PluginsPage.test.tsx` — a plugin card click navigates to `/plugins/:id`.
  - `src/pages/plugin-detail/resources.test.ts` — `collectPluginResources` groups by
    `parentPluginId` and edges; `findPluginNode` lookup.
  - `src/pages/plugin-detail/PluginDetailPage.test.tsx` — header + grouped sections render,
    empty-plugin shows "No inspectable resources", loading/not-found states, the Disable
    action calls the endpoint, and the Contact Manager drives the skill-invoke endpoint.
  - Backend endpoints are covered by `internal/gateway/plugins_test.go`.
