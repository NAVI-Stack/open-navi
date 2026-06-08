# Plugin Lifecycle — Current State and Roadmap

**Status:** Active  
**Source:** [Conceptual Design Overview](../canonical/conceptual-design-overview.md)  
**Implementation:** `internal/navi/plugin/registry.go`, `internal/navi/plugin/builtin.go`, `cmd/navid/main.go`

---

## Design lifecycle (full)

Conceptually: **Discover → Install → Register → Activate → Invoke → Learn → Update → Retire.**

| Stage | Design meaning | Current implementation |
|-------|----------------|-------------------------|
| **Discover** | Capability gap → find plugin | Implemented: `Registry.Discover(ctx, capabilityGap)` filters manifests by Capabilities, Triggers, ID, Name, Description. |
| **Install** | Add to system, resolve deps, request permissions | Implemented: `Registry.Install(ctx, manifestID, opts)` registers from catalog (`SetCatalogEntry`). Idempotent if already registered. Host populates catalog (e.g. `RegisterBuiltin` calls `SetCatalogEntry` per builtin). |
| **Register** | Publish capabilities, interfaces, triggers, constraints | Implemented at startup via `RegisterBuiltin`; tools, routes, manifests merged into registry. |
| **Activate** | Make plugin available for invocation | Implemented: `Registry.SetPluginEnabled(pluginID, true/false)`; `Tools()` excludes disabled plugins. |
| **Invoke** | Tool call / execution | Implemented: loop merges plugin tools and executes built-in executors. |
| **Learn** | Refine when/why/how | Not implemented; out of scope for current release. |
| **Update** | Evolve plugin | Implemented: `Registry.Update(ctx, manifestID)` replaces installed plugin with catalog entry (e.g. new version). Preserves enabled state. Catalog updated via `SetCatalogEntry`. |
| **Retire** | Disable, replace, or remove | Implemented: `Registry.Retire(ctx, manifestID, mode, opts)`. Disable: `SetPluginEnabled(id, false)`. Replace: opts.ReplacementID required; disable old, enable new. Remove: `RemovePlugin(manifestID)` unregisters manifest and tools. |

---

## Current state: Register at startup + Invoke

- **Register:** At daemon startup, built-in plugins are registered in code (`RegisterBuiltin`). The registry holds tools, HTTP routes, services, and manifests. Gateway exposes manifests for discovery.
- **Invoke:** The agent loop receives the merged tool list; when the LLM calls a plugin tool, the loop executes it via the built-in executor (e.g. calendar time window, workflow summary, GitHub repo, critic request). No separate plugin process; execution is in-process with panic recovery (see [plugin-connector-isolation](plugin-connector-isolation.md)).

**Lifecycle API:** `internal/navi/plugin/lifecycle.go` defines `Discover`, `Install`, `Update`, and `Retire` on the Registry. **Discover** filters registered manifests by capability gap. **Install** registers from catalog (`SetCatalogEntry`); idempotent if already registered. **Update** replaces an installed plugin with the catalog entry (preserves enabled state). **Retire** supports Disable, Replace (opts.ReplacementID), and Remove (RemovePlugin). **Activate/Deactivate** is implemented via `SetPluginEnabled`. The host populates the catalog via `SetCatalogEntry` (e.g. `RegisterBuiltin` does so for each builtin).

---

## Roadmap

1. **Done:** **Activate/Deactivate** — `SetPluginEnabled(pluginID, enabled)`; builtins registered via `RegisterPlugin` so they can be toggled by manifest ID.
2. **Done:** **Discover** — capability gap → filter manifests. **Install** — from catalog (`SetCatalogEntry`); **Update** — replace from catalog. **Retire(Replace/Remove)** — Replace via opts.ReplacementID; Remove via RemovePlugin. Builtins populate catalog in `RegisterBuiltin`.
3. **Later:** Learn; load plugin from path/URL (bundle loader); full dependency resolution or marketplace.

---

## References

- `internal/navi/plugin/registry.go` — Registry, AddTool, Tools, manifests
- `internal/navi/plugin/builtin.go` — RegisterBuiltin, built-in tool definitions and executors
- `cmd/navid/main.go` — Plugin registration at startup
- `internal/gateway/server.go` — Manifest exposure
