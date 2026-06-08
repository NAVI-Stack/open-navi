# Capability Layer Backlog

**Scope:** Skills and connectors — creation, CLI, internal transport, driver/instance model, runbooks.  
**Purpose:** Ordered list of capability-layer tasks; each item is plan-ready for a dedicated implementation plan.  
**Related:** [blockers.md](blockers.md) (BLOCKER-5), [outstanding-work.md](outstanding-work.md). Execution work here aligns with **NAVI-AUTO-003**, **NAVI-AUTO-004**, **NAVI-AUTO-005**.

---

## Priority 1 — Skill creation (OSS-27)

| ID | Task | Scope | Outcome |
|----|------|--------|---------|
| **CAP-1.1** | Update skill-creator to scaffold **OSS-27 SKILL.yaml** instead of (or in addition to) SKILL.md. | [skills/skill-creator/SKILL.md](../../skills/skill-creator/SKILL.md) (instructions and any template the agent uses); optionally a small scaffold helper or [internal/navi/skill/synthesizer.go](../../internal/navi/skill/synthesizer.go) if a programmatic scaffold is desired. | New skills created by the agent are governed, transport-capable, and executable where transports exist. |
| **CAP-1.2** | Treat SKILL.md as docs-only in creation path: skill-creator must not present SKILL.md as the primary creation format; document that governed/side-effecting skills MUST use SKILL.yaml. | skill-creator content; [docs/canonical/skills.md](../canonical/skills.md) already states the policy. | Clear single path for creating executable skills. |

---

## Priority 2 — Skill CLI

| ID | Task | Scope | Outcome |
|----|------|--------|---------|
| **CAP-2.1** | Implement `navi skills list`: list loaded skills (name, tier, source dir). | [cmd/navi](../../cmd/navi) (new subcommand or flag); may require gateway API `GET /api/navi/skills` that returns `SkillRegistry.Snapshot()` or equivalent. | Users and agents can see what skills are active. |
| **CAP-2.2** | Implement `navi skills reload`: trigger reload of skill registry so agent-created skills appear without daemon restart. | CLI call + gateway API (e.g. `POST /api/navi/skills/reload`) that calls `SkillRegistry.Load()` on the navid side. | Autonomous skill acquisition visible mid-session. |
| **CAP-2.3** | Implement `navi skills validate`: validate a skill directory (SKILL.yaml schema, required fields, transport config). | CLI; use existing [internal/navi/skill/validate.go](../../internal/navi/skill/validate.go) and loader validation. | Catch invalid skills before load. |
| **CAP-2.4** | Implement `navi skills inspect <name>`: show one skill's metadata, interfaces, and transport types. | CLI (+ optional gateway API) using `SkillRegistry.Lookup` / loader. | Debug and audit individual skills. |

---

## Priority 3 — Internal transport

Aligns with **NAVI-AUTO-004** (internal transport dispatch) in [blockers.md](blockers.md). See also **NAVI-AUTO-003** (subprocess_python), **NAVI-AUTO-005** (MCP).

| ID | Task | Scope | Outcome |
|----|------|--------|---------|
| **CAP-3.1** | Expand `executeInternal` in [internal/navi/skill/executor.go](../../internal/navi/skill/executor.go) with a small internal handler registry (skill_id + interface name → handler func). | executor.go; registry type and registration at init or startup. | Single extension point for internal capabilities. |
| **CAP-3.2** | Add internal handlers for **filesystem** (read_file, list_dir, write_file): delegate to existing [internal/navi/filetools](../../internal/navi/filetools) so they are invokable as internal skill interfaces. | Internal registry + skill specs or built-in skill definitions. | File tools exposed as internal skills. |
| **CAP-3.3** | Add internal handlers for **git** (clone, status, commit, push) as first-class internal skills (new handlers or thin wrappers). | New handlers; governor and path constraints. | Git operations as governed internal skills. |
| **CAP-3.4** | Add internal handlers for **process/shell** (exec bounded commands) with governor and sandbox constraints. | New handlers; timeout, allowlist, workspace bounds. | Safe shell/process execution as internal skill. |
| **CAP-3.5** | Document internal transport contract and how to add new internal handlers; optionally add stubs for ssh, http if out of scope for first iteration. | docs (e.g. [docs/concepts/skills.md](../concepts/skills.md) or new internal-transport doc). | Clear contract for future internal skills. |

---

## Priority 4 — Connector driver/instance model

Reference: [docs/plans/connectors/connectors_gap_analysis.plan.md](../plans/connectors/connectors_gap_analysis.plan.md), [docs/canonical/specs/connectors.md](../canonical/specs/connectors.md).

| ID | Task | Scope | Outcome |
|----|------|--------|---------|
| **CAP-4.1** | Document the target driver vs instance model and how it maps to [internal/connectors/registry.go](../../internal/connectors/registry.go) (drivers, instancesV2, metadata). | Design doc or ADR; registry code as reference. | Shared understanding for refactor. |
| **CAP-4.2** | Refactor [cmd/navid/main.go](../../cmd/navid/main.go) so connector "drivers" are registered by name and "instances" are created from config/setup (preserve existing Telegram/Slack behavior; structure only). | main.go registration and startConnector flow. | Clear driver/instance separation without breaking current behavior. |
| **CAP-4.3** | Introduce a connector instance config store (e.g. DB or config) so multiple instances per driver (e.g. telegram-personal, telegram-work) can be created and started from persisted config. | Store API + config shape; startup and setup flows. | Multi-instance connectors without recompile. |
| **CAP-4.4** | (Optional / follow-up) Define how skills declare a dependency on a connector driver or instance so the runtime can resolve "skill requires discord" before execution. | Skill spec schema; runtime resolution in executor or governor. | Skill–connector dependency resolution. |

---

## Priority 5 — Connector runbook

| ID | Task | Scope | Outcome |
|----|------|--------|---------|
| **CAP-5.1** | Add runbook **"Add a new connector"** under [docs/runbooks](../runbooks): steps to implement [connectors.Connector](../../connectors/connector.go), add config in [internal/config/config.go](../../internal/config/config.go), register factory and `startConnector` in [cmd/navid/main.go](../../cmd/navid/main.go), and expose setup (e.g. POST /api/setup/connector). | New runbook doc. | Single place to add a connector. |
| **CAP-5.2** | Add runbook **"Add a new connector instance"** (after CAP-4.3): how to configure and start a new instance of an existing driver (e.g. second Telegram bot). | New runbook doc. | Clear path for multi-instance. |
| **CAP-5.3** | Link runbooks from [docs/runbooks/INDEX.md](../runbooks/INDEX.md) and from [docs/plans/connectors/connectors_gap_analysis.plan.md](../plans/connectors/connectors_gap_analysis.plan.md) (or connector index). | INDEX and plan doc. | Discoverable runbooks. |

---

## Task flow for planning

Each CAP-x.y is a single backlog item. When building plans per task:

- **CAP-1.1 / 1.2** — Plan: change skill-creator content and any scaffold logic; no new binary required.
- **CAP-2.x** — Plan: CLI subcommand structure (`navi skills` with list/reload/validate/inspect), gateway routes if needed, and how navid exposes reload/snapshot.
- **CAP-3.x** — Plan: executor internal registry, filetools wiring, then git/process handlers and docs.
- **CAP-4.x** — Plan: refactor steps for registry/main.go and instance persistence without breaking current Telegram/Slack.
- **CAP-5.x** — Plan: runbook outline and where to add "Add new connector" and "Add new instance" in docs/runbooks.

---

[Tasks INDEX](INDEX.md) · [blockers](blockers.md) · [outstanding-work](outstanding-work.md)
