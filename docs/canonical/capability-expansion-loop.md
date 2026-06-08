# NAVI — Capability Expansion Loop (Canonical)

**Status:** Canonical (immutable — changes require human approval)  
**Last Updated:** 2026-03-14

This document defines the **Capability Expansion Loop**: the formal process by which NAVI detects gaps in its capabilities, classifies them, chooses an expansion path, and produces, validates, installs, and governs new skills and connector instances. It turns NAVI from a system with a skill format into a **self-extending capability platform**.

For implementation-level states, APIs, and commands, see [docs/specs/capability-expansion-loop.md](../specs/capability-expansion-loop.md).

---

## Relationship to Other Canonical Docs

- **Capability Layer** — Defined in [conceptual-design-overview.md](conceptual-design-overview.md). Skills and Connectors live in the Capability Layer; the Expansion Loop is the process by which that layer grows.
- **Skills** — [skills.md](skills.md) defines what a Skill is and how it is governed. The Loop generates and installs skills that satisfy that definition.
- **Connectors** — [specs/connectors.md](specs/connectors.md) defines the connector driver/instance model. The Loop registers drivers and creates instances; it does not replace the connector contract.

---

## The Ten-Step Loop

The Capability Expansion Loop consists of ten ordered steps. Each step has defined inputs, outputs, and invariants. Implementation may batch or pipeline steps but must preserve the logical order and gates.

```
1. Detect gap
2. Classify gap
3. Choose expansion path
4. Generate artifact
5. Validate artifact
6. Install / register
7. Bind dependencies
8. Test execution
9. Promote trust level
10. Observe and refine
```

---

## 1. Detect gap

NAVI must explicitly notice that a capability need is not satisfied. Detection triggers include:

- **Requested capability not available** — User or task requests a behavior that no installed skill or connector provides.
- **Requested capability available but not executable** — A skill exists but its transport or runtime is not implemented or not configured (e.g. internal handler missing, Python runtime unavailable).
- **Requested capability blocked by missing connector, runtime, or auth** — A skill declares a dependency on a connector instance, auth scope, or runtime that is not present or not bound.
- **Requested capability repeatedly approximated manually** — The same workflow is performed multiple times in an ad hoc way and could be codified as a skill or workflow.

Gap detection is the entry point. Without it, capability growth remains reactive and human-driven. Evidence (user message, tool result, session context) should be recorded so classification and refinement can use it.

---

## 2. Classify gap

Every gap must be classified before NAVI builds anything. Classification determines whether the response is a new skill, a new connector, a configuration change, or something else.

### Gap classes

| Class | Description |
|-------|-------------|
| **A. Missing skill** | A reusable governed capability is needed; no skill exists that provides it. |
| **B. Missing connector** | Access to a new external platform, channel, or system is needed; no connector driver or instance exists. |
| **C. Missing skill transport/runtime** | A skill exists but its transport (e.g. internal, subprocess_python) or runtime is not available or not implemented. |
| **D. Missing auth/config** | Capability exists but credentials, scopes, or configuration are missing. No new artifact should be created; only setup is required. |
| **E. Missing policy/trust approval** | Capability exists but governance or trust tier blocks autonomous use (e.g. requires confirmation, human review). |
| **F. Missing composition/workflow** | Need is orchestration across existing skills/connectors (plugin or workflow), not a single new skill or connector. |

### Rule of thumb

- **Reusable governed capability** → Create skill (Class A).
- **New integration surface** → Create connector (Class B).
- **Orchestration across existing parts** → Create plugin/workflow (Class F).
- **Only credentials or setup** → Do not create new capability (Class D); resolve via config/auth flow.

This aligns with the conceptual layering: skills are modular capabilities, connectors are integration adapters, plugins compose both.

---

## 3. Choose expansion path

NAVI must make a deterministic decision from the classification.

### Expansion decision tree

- **Need new behavior against an existing system?** → New skill.
- **Need access to a new external platform/channel?** → New connector (driver and/or instance).
- **Need multi-step reusable business logic?** → Plugin or workflow.
- **Need only a different prompting or presentation style?** → Do not create a skill.

This prevents using "skill" as a junk drawer. Not every need produces a new skill; connectors are rarer than skills.

### Critical design rule

**Skills can be generated frequently; connectors should be generated rarely.** Skills are capability contracts. Connectors are integration surfaces. Skills depend on connectors. Plugins compose both. The Loop must not encourage connector-shaped hacks as skills.

---

## 4. Generate artifact

### For a skill

NAVI must generate:

- **Primary:** `workspace/skills/<name>/SKILL.yaml` in OSS-27 format (see [skills.md](skills.md)).
- **Optional:** Smoke test fixture (e.g. `tests/<skill_name>_smoke.json` or equivalent).
- **Optional:** Handler or runtime binding metadata where the skill uses internal or subprocess_python transport.

New executable, governed capabilities **must** use `SKILL.yaml`. Legacy `SKILL.md` is not an acceptable creation output for side-effecting or transport-backed skills; it may remain for docs-style informational behaviors only.

### For a connector

NAVI may generate a connector scaffold (e.g. driver layout, config schema, manifest). Today connector creation is code-only; the long-term direction is a driver/instance model where drivers can be registered and instances created at runtime. Repo-owned connector artifacts belong under `plugins/<name>/connectors/<name>/` with `plugins/<name>/plugin.yaml`; shared framework contracts remain in `connectors/` and `internal/connectors/`.

---

## 5. Validate artifact

This stage is **hard-gated**. Invalid artifacts must not proceed to install.

### Skill validation

- Valid `skill_id`, `semver`, `display`.
- Valid `interfaces[]` with transport, input_schema, output_schema.
- Effects declared; risk tier, reversibility, and confirmation consistent (e.g. irreversible requires confirmation).
- Security metadata present where required (auth, data_access, sandbox/network_egress for external or high-risk skills).
- Transport-specific rules (e.g. subprocess_python requires `python_runtime` and entrypoint).

The canonical skill schema and validation rules are in [skills.md](skills.md) and `internal/navi/skill/validate.go`.

### Connector validation

- Implements connector interface (Name, Start, Stop, Send, IsRunning).
- Declares auth model, action surface, event surface, config schema.
- Satisfies healthcheck contract where defined.

Connector validation may be partially code-enforced (compiler) and partially manifest/schema-enforced when a connector manifest format exists.

---

## 6. Install / register

### Skills

- **Install** — Place or copy the skill into a discoverable tier (e.g. workspace/skills) and register it in the runtime.
- **Reload** — Rescan skill roots and refresh the in-memory registry so newly added skills become visible without daemon restart.
- **List** — Enumerate installed skills with identity and status.
- **Inspect** — Return full metadata and dependency requirements for a skill.
- **Disable / Remove** — Deactivate or delete a skill from the registry and storage.

Skills are discoverable from configured roots at runtime; no recompilation is required.

### Connectors

- **Register driver** — Make a connector driver available to the runtime (today: compile-time registration; future: manifest-based or plugin).
- **Create instance** — Instantiate a configured connector instance (e.g. Telegram bot, Slack workspace).
- **Bind auth** — Attach credentials and scopes to an instance.
- **Enable / Disable** — Turn an instance on or off without removing it.
- **Healthcheck** — Verify instance connectivity and contract.
- **Remove instance** — Tear down and remove an instance.

---

## 7. Bind dependencies

An artifact is not fully usable until its dependencies are bound. This step is the difference between "artifact exists" and "artifact works."

### Dependency binding includes

- **Connector instance exists** — Any skill that requires a connector (e.g. `requires: connector: discord`) must have a live, configured instance.
- **Runtime exists** — For subprocess_python, the Python runtime and venv (if used) must be provisioned.
- **Env vars / config** — Required environment or config keys must be set.
- **Auth scopes granted** — Required OAuth or API scopes must be granted.
- **Sandbox policy declared** — Where governance requires sandbox/network rules, they must be present.
- **Required binaries available** — Skills that declare `requires_bins` must run in an environment where those binaries exist.

If any dependency is missing, the capability must be reported as **installed but not activatable**, not silently failed at execution time. The loader and registry should support this state explicitly.

---

## 8. Test execution

Every newly added capability must pass a smoke test before NAVI promotes it to normal use.

### Skill smoke test

- Invoke the skill with known-good input (fixture or minimal safe args).
- Validate the result envelope (status, payload shape, no unexpected errors).
- Optionally measure duration and record failure mode for telemetry.

### Connector smoke test

- Connect and authenticate.
- Perform a no-op or safe dry run (e.g. healthcheck, read-only call).
- Verify response normalization and error classification.

Failure is a first-class outcome; degraded capability must be surfaced explicitly, consistent with the failure model in [conceptual-design-overview.md](conceptual-design-overview.md).

---

## 9. Promote trust level

Skills (and optionally connector instances) carry a **trust tier** that affects whether they may be used autonomously. Tiers are defined in [skills.md](skills.md): builtin, verified, community, local.

### Promotion path

- **Generated locally** → Start as **local**.
- **Passes validation and smoke tests** → Remain **local** but marked usable.
- **Human reviewed and optionally signed** → **Verified**.
- **Ships with NAVI** → **Builtin**.

A generated skill must **not** receive the same autonomous treatment as a built-in skill until it has passed validation, smoke test, and optionally human review. The governor and policy engines must respect trust tier when allowing autonomous execution.

---

## 10. Observe and refine

Capabilities are not static. NAVI must observe usage and outcomes and may refine or retire capabilities.

### Observation inputs

- Invocation success rate, latency, retry rate.
- Permission or auth failures.
- Human overrides and proposal frequency.
- Degraded mode frequency.

Execution outcomes and history are the source of truth (see World Model and History in the conceptual design). Capability-level aggregation (per skill_id, per connector instance) should support refinement decisions.

### Refinement actions

- **Keep as-is** — No change.
- **Revise skill** — Update SKILL.yaml or implementation (e.g. fix schema, tighten effects).
- **Disable skill** — Deactivate due to failure rate or policy.
- **Request human review** — Escalate to owner for trust promotion or deprecation.
- **Replace transport** — Switch to a different transport (e.g. rest instead of internal) if runtime is unavailable.
- **Suggest connector upgrade** — Recommend connector or auth changes when a skill’s dependency is degraded.

---

## Summary

The Capability Expansion Loop is:

**Gap detection → Classification → Expansion path choice → Artifact generation → Validation → Install/register → Dependency binding → Smoke test → Trust promotion → Observation and refinement.**

This loop applies to both skills and connectors, with the constraint that **skills are generated often, connectors rarely**, and **skills depend on connectors**. Implementation details, state machines, APIs, and commands are specified in [docs/specs/capability-expansion-loop.md](../specs/capability-expansion-loop.md).
