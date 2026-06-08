# Plugin and Connector Isolation

**Status:** Active  
**Source:** [Conceptual Design Overview](../canonical/conceptual-design-overview.md) (Failure Model)  
**Implementation:** `internal/navi/loop.go`, `internal/navi/plugin/`, `internal/connectors/bridge.go`

---

## Design intent

- **Plugin isolation:** Plugins execute in isolated processes; a plugin crash must not block the Conscious loop or write to the World Model.
- **Connector boundary:** Connector failures are caught at the boundary; a structured error is returned. World Model is only writable by the Cognitive layer.

---

## Current behavior

### Plugins

- Plugins run **in-process**. Built-in plugin tools (e.g. calendar time window, workflow summary, GitHub repo, critic request) are invoked in the same process as the agent loop.
- Errors from plugin code are returned as normal Go errors and surface as structured tool results (e.g. via `skill.SanitizeResult` or execution outcome).
- **Minimal isolation:** The agent loop recovers panics from tool execution (including plugin and skill paths). A panicking plugin does not crash the daemon; the turn returns a structured failure to the LLM/user.
- **Risks:** A buggy or hostile plugin can still consume CPU/memory, block the turn until timeout, or access process-wide state before a panic. Out-of-process or sandbox isolation would remove these risks.

### Connectors

- Connectors run in the same process (bridge, Send path). Failures are returned as structured errors; the gateway and loop do not rely on panics for control flow.
- Connector Send and lifecycle are wrapped so that failures are captured and returned. Panics in connector code would still propagate unless caught at the bridge boundary; adding a recover at the connector invocation boundary is recommended for parity with the loop.

---

## Roadmap (process isolation)

1. **Current (done):** In-process execution with panic recovery in the agent loop so plugin/skill panics become structured failures.
2. **Short term:** Add panic recovery at the connector invocation boundary (bridge Send / connector Start/Stop) so connector panics never reach the loop.
3. **Medium term:** Document or implement an out-of-process plugin runner (e.g. subprocess or sidecar) so plugin code runs in a separate process; only results and errors cross the boundary. Requires a clear plugin contract (stdio, gRPC, or MCP).
4. **Long term:** Sandboxing (e.g. OS-level or container) for plugins and optionally connectors, with resource limits and no direct World Model or store access.

---

## References

- `internal/navi/loop.go` — tool execution and panic recovery for plugin/skill
- `internal/navi/plugin/` — plugin registry and built-in tools
- `internal/connectors/bridge.go` — connector Send and lifecycle
