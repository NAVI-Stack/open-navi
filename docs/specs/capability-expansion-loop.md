# Capability Expansion Loop — Implementation Spec

**Status:** Mutable (evolve with implementation)  
**Last Updated:** 2026-03-28

This spec defines states, APIs, commands, and data shapes for the [Capability Expansion Loop](../canonical/capability-expansion-loop.md). The canonical doc defines the ten-step process; this doc specifies how it is implemented and where it plugs into the codebase.

---

## 1. State machines

### 1.1 Gap state

| State | Description |
|-------|-------------|
| `detected` | Gap identified; evidence recorded. |
| `classified` | Gap class (A–F) and recommended path assigned. |
| `expansion_path_chosen` | Decision made: skill \| connector \| plugin \| none. |

**Package / location:** To be implemented (e.g. `internal/capability/` or under `internal/orchestrator/`). No persistent gap store exists today.

### 1.2 Artifact state (skill or connector)

| State | Description |
|-------|-------------|
| `generated` | Artifact files produced (SKILL.yaml or connector scaffold). |
| `validated` | Validation passed. |
| `validation_failed` | Validation failed; artifact not installable. |
| `installed` | Registered in runtime (skill in registry, connector instance created). |
| `install_failed` | Install/register failed. |
| `dependencies_unbound` | Installed but missing connector/runtime/auth (installed but not activatable). |
| `bound` | Dependencies satisfied; capability activatable. |
| `smoke_passed` | Smoke test succeeded. |
| `smoke_failed` | Smoke test failed. |

**Package / location:** Skill lifecycle: `internal/navi/skill/registry.go` (Load, Install). No explicit "dependencies_unbound" or "bound" state in code yet; would extend registry or store.

### 1.3 Capability runtime state (skill or connector instance)

| State | Description |
|-------|-------------|
| `active` | Enabled and usable. |
| `disabled` | Present but disabled (no invocations). |
| `pending_review` | Awaiting human review for trust promotion or deprecation. |
| `deprecated` | Marked deprecated; may still be invokable with warning. |

**Package / location:** Skills: tier and load state in `internal/navi/skill/registry.go`. Connectors: `internal/connectors/registry.go` (InstanceMetadata.Status, HealthState). Disable/deprecated flags to be added.

### 1.4 Trust level

| Level | Description | Set when |
|-------|-------------|---------|
| `untrusted` | Generated, not yet validated. | On generate. |
| `local` | Passes validation and optional smoke; user/workspace scope. | After validate + smoke. |
| `verified` | Human reviewed and/or signed. | After review. |
| `builtin` | Shipped with NAVI. | Build time. |

**Package / location:** Canonical tiers in `docs/canonical/skills.md`. Governor and policy do not yet key off trust tier in code; `internal/navi/skill/spec.go` has governance/trust fields in OSS27Spec.

---

## 2. APIs (intended surface)

APIs may be exposed via HTTP (gateway) and/or as internal Go interfaces called by the agent loop or CLI.

### 2.1 Gap detection

| API | Input | Output | Notes |
|-----|--------|--------|--------|
| Detect gap | User message, tool/skill result, session context | Candidate gap (type, evidence, suggested class) | Not implemented. Could live in `internal/orchestrator` or new `internal/capability`. |

### 2.2 Classification

| API | Input | Output | Notes |
|-----|--------|--------|--------|
| Classify gap | Gap (evidence, type) | GapClass (A–F), recommended path (skill \| connector \| plugin \| none) | Not implemented. |

### 2.3 Skill expansion

| API | Input | Output | Notes |
|-----|--------|--------|--------|
| Generate skill | Name, description, interfaces, transport, effects | Path to generated SKILL.yaml (and optional fixture) | Partially: `internal/navi/skill/synthesizer.go` (OpenClaw/Claude → SKILL.yaml). No SKILL.yaml-first agent-facing generator. |
| Validate skill | Path or skill name | Validation result (ok / errors[]) | Implemented and exposed via `POST /api/skills/{id}/validate` plus `navi skills validate <id>`. |
| Install skill | Source path, overwrite | Registry updated, Load() called | Exists: `SkillRegistry.Install()` in `internal/navi/skill/registry.go`. Not exposed as a general operator workflow yet. |
| Reload skills | — | Registry.Load() called; list of loaded skills | Implemented and exposed via `POST /api/skills/reload` plus `navi skills reload`. |
| List skills | — | Skill list (name, tier, status, activatable) | Implemented and exposed via `GET /api/skills` plus `navi skills list`. |
| Inspect skill | Name | Full SkillEntry + dependency requirements | Implemented and exposed via `GET /api/skills/{id}` plus `navi skills inspect <id>` / `show <id>`. |
| Disable / Remove skill | Name | Registry updated | Not implemented. |

### 2.4 Connector expansion

| API | Input | Output | Notes |
|-----|--------|--------|--------|
| Register driver | Driver manifest or Go factory | Driver registered | Today: compile-time `connectorRegistry.RegisterFactory()` in `cmd/navid/main.go`. No runtime manifest. |
| Create instance | Driver id, config, auth params | Instance created and started | Today: `startConnector()` in main.go; `connectorRegistry.Create()`, `connectorMgr.StartOne()`. |
| Connector health | Instance name (optional) | Health status per instance | Implemented via `GET /api/health/connectors` and `navi connectors health`. |
| Bind skill dependency | Skill id, connector instance id, scopes | Binding recorded; activatable if all deps bound | Not implemented. |

### 2.5 Dependency binding

| API | Input | Output | Notes |
|-----|--------|--------|--------|
| Resolve dependencies | Skill id (or entry) | List of required connectors, runtimes, auth; satisfied or missing | Not implemented. Requires skill spec to declare `requires: connector: X` etc. |
| Set activatable | Skill id, activatable bool | Stored for list/inspect | Not implemented. |

### 2.6 Smoke test

| API | Input | Output | Notes |
|-----|--------|--------|--------|
| Smoke skill | Skill name, optional fixture path | Result (pass/fail, duration, error) | Not implemented. Would invoke skill with fixture and validate envelope. |
| Smoke connector | Instance name | Result (connect, auth, dry run) | Not implemented. |

### 2.7 Trust promotion

| API | Input | Output | Notes |
|-----|--------|--------|--------|
| Promote trust | Skill id (or connector instance), target tier | Updated trust tier | Not implemented. Trust tier in spec; no promotion workflow in code. |

### 2.8 Observation and refinement

| API | Input | Output | Notes |
|-----|--------|--------|--------|
| Capability telemetry | Skill id or connector instance id, time range | Aggregates: success rate, latency, retries, permission failures | Partially: execution outcomes in `internal/store`; no per-capability aggregation API. |
| Refinement decision | Telemetry + policy | Suggested action: keep, revise, disable, request_review, replace_transport, suggest_connector_upgrade | Not implemented. |

---

## 3. Commands (CLI)

Intended subcommands for the `navi` CLI. Implementation status in parentheses.

### 3.1 Skills

| Command | Description | Status |
|---------|-------------|--------|
| `navi skills list` | List installed skills (name, tier, activatable). | Implemented. |
| `navi skills reload` | Trigger registry reload (so new skills on disk are visible). | Implemented. |
| `navi skills validate <path\|name>` | Run skill validation; output ok or errors. | Implemented for skill IDs through the gateway. |
| `navi skills inspect <name>` | Show full skill metadata and dependency requirements. | Implemented (`inspect` aliases `show`). |
| `navi skills disable <name>` | Disable skill (no invocations). | Not implemented. |
| `navi skills remove <name>` | Remove skill from workspace tier. | Not implemented. |
| `navi skills smoke <name>` | Run smoke test for skill. | Stub only; CLI prints not implemented. |

**Codebase:** Skill operator routes live in `internal/gateway/server.go`; CLI wrappers live in `cmd/navi/main.go`.

### 3.2 Connectors

| Command | Description | Status |
|---------|-------------|--------|
| `navi connectors list` | List connector instances and status. | Implemented. |
| `navi connectors health [name]` | Health check for one or all instances. | Implemented. |
| `navi connectors instance create ...` | Create and start an instance (params TBD). | Not implemented (setup today via POST /api/setup/connector). |

### 3.3 Expansion (optional)

| Command | Description | Status |
|---------|-------------|--------|
| `navi capability expand --gap-type=skill --name=...` | Trigger expansion for a classified gap (agent or operator). | Not implemented. |

Alternatively, expansion may be triggered only by agent tools (e.g. create_skill, validate_skill) that call the same backend logic.

---

## 4. Data shapes

### 4.1 Gap report (proposed)

```json
{
  "id": "uuid",
  "type": "A|B|C|D|E|F",
  "evidence": { "message_id": "...", "tool_result": "...", "session_id": "..." },
  "classification_result": { "path": "skill|connector|plugin|none" },
  "created_at": "ISO8601"
}
```

No persistent gap store exists today.

### 4.2 Skill dependency (proposed)

- **In skill spec:** `requires: { connector: ["discord"], auth_scopes: ["messages.write"], transport: "internal|mcp_tool" }`.
- **Resolved at runtime:** `connector_instance_ids[]`, `auth_bound` (bool), `activatable` (bool).

Current loader only checks `requires_bins`, `requires_env`, `os` in metadata (`internal/navi/skill/loader.go`). No connector or auth binding.

### 4.3 Connector manifest (future)

For connector driver registration and validation:

- `driver_id`, `config_schema`, `auth_model`, `action_surface`, `event_surface`, `healthcheck_contract`.

Today connectors are Go types only; no manifest file. See `docs/canonical/specs/connectors.md` for v2 model.

---

## 5. Implementation status summary

| Component | Status | Package / file |
|-----------|--------|----------------|
| Canonical loop (10 steps) | Documented | `docs/canonical/capability-expansion-loop.md` |
| Gap detection | Not implemented | — |
| Gap classification | Not implemented | — |
| SKILL.yaml generation | Partial (Synthesizer) | `internal/navi/skill/synthesizer.go` |
| Skill validation | Implemented | `internal/navi/skill/validate.go` |
| Skill install/reload | Implemented (reload has CLI/API; install is library-only) | `internal/navi/skill/registry.go`, `internal/gateway/server.go`, `cmd/navi/main.go` |
| Skill list/inspect | Implemented | `internal/gateway/server.go`, `cmd/navi/main.go` |
| Skill disable/remove | Not implemented | — |
| Connector register/create | Implemented (compile-time + startConnector) | `cmd/navid/main.go`, `internal/connectors/registry.go` |
| Connector manifest/driver abstraction | Partial (v2 metadata) | `internal/connectors/registry.go` |
| Dependency binding | Not implemented | — |
| Installed-but-not-activatable state | Implemented for env/OS deps (skills) | `internal/navi/skill/loader.go`, `internal/navi/skill/types.go` |
| Skill smoke harness | Not implemented | — |
| Connector smoke | Not implemented | — |
| Trust promotion workflow | Not implemented | — |
| Capability telemetry / refinement | Partial (execution outcomes) | `internal/store` |

---

## 6. Connector Phase 2 → Phase 3 integration points

Phase 2 focuses on **connector dependency binding** and making connector state visible to the rest of the system. Its outputs become direct inputs to the Phase 3 autonomous growth loop:

- **Driver/instance model and manifests** (Phase 2) → give Phase 3’s **gap detector** and **classifier** a stable way to reason about “missing connector driver” vs “missing instance” vs “instance unhealthy”. When a skill declares `requires.connectors`, the resolver can classify gaps as:
  - Class B (missing connector) when no driver/instance exists.
  - Class C/D when drivers exist but are unhealthy or unauthenticated (auth/config issues).
- **InstanceMetadata.HealthState/AuthState** (Phase 2, `internal/connectors/registry.go`) → feeds Phase 3 **telemetry and refinement**:
  - The telemetry aggregator can correlate skill failures with connector health/auth transitions.
  - The refinement engine can propose disabling skills that depend on persistently degraded connectors or suggest connector upgrades.
- **Skill dependency resolution and activatable flag** (Phase 2, `internal/navi/skill/loader.go`, `SkillEntry.Activatable`) → directly shapes Phase 3 **gap detection**:
  - When a frequently requested skill is installed but not activatable due to missing connector dependencies, the detector records a structured gap (Class B/C/D) instead of a generic execution failure.
  - Expansion requests generated in Phase 3 can target either “create connector instance” or “fix auth/config” depending on the dependency status.

These integration points should be kept in sync with any future changes to:

- `OSS27Spec.Capability.Requires` (skill-side dependency declarations).
- Connector `InstanceMetadata` fields (driver_id, health_state, auth_state).
- The gap model and classifier in Phase 3 (so connector-related gaps are first-class, not ad hoc string matches).

---

## 7. Recommended build order (from canonical doc)

- **Phase 1 — Skill expansion:** SKILL.yaml scaffolding (replace SKILL.md in skill-creator), skill validate/list/reload/inspect commands, skill smoke harness, installed-but-unavailable state.
- **Phase 2 — Connector dependency:** Connector manifests, driver vs instance split (already partially in registry), connector healthcheck + authcheck, skill dependency binding to connectors.
- **Phase 3 — Autonomous growth:** Gap detector, classifier, trust promotion workflow, capability telemetry and self-refinement, using the Phase 1–2 surfaces above as inputs.

---

[docs INDEX](../INDEX.md) · [Canonical Capability Expansion Loop](../canonical/capability-expansion-loop.md) · [canonical specs](../canonical/specs/INDEX.md)
