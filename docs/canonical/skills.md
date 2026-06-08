> [!NOTE]
> Part of the [NAVI Systems Map](../architecture/navi-systems-map.md).

**Status:** Active  
**Last Updated:** 2026-03-10  
**Updated By:** agent

# NAVI AI — Skills (Canonical Definition)

Skills are NAVI's canonical unit of modular capability in the **Capability Layer**. This document defines what a Skill is, its role and boundaries, and the high-level schema, lifecycle, and execution model that all skills must follow.

For mutable, implementation-level details and examples, see:
- [Skills System (concepts)](../concepts/skills.md)
- [Python Skill Runtime (concepts)](../concepts/skill-python-runtime.md)
- `internal/navi/skill/spec.go` (Go schema definition)

---

## Concept Overview

Within the Capability Layer, NAVI distinguishes three primary constructs:

- **Primitive Commands** — The fixed set of 10 execution primitives (Query, Create, Update, Delete, Invoke, Send, Acquire, Schedule, Delegate, Compose) defined in `conceptual-design-overview.md`.
- **Skills** — Versioned, declarative capability modules that expose governed interfaces which may map to one or more primitive Commands during execution; they do not define new Commands or own decision logic.
- **Connectors & Plugins** — Integration surfaces to external systems and extended runtimes.

Skills sit between the Cognitive Layer and concrete execution:

- The **Cognitive Layer** (Conscious loop) decides *which* Skill interface to call during the **Decide → Execute** steps.
- The **Capability Layer** (through the Skill registry and executors) is responsible for *how* the Skill is executed, including transport, isolation, and enforcement of governance.

A Skill never introduces new primitive Commands; it binds existing Commands into governed capabilities.

A Skill is:

- **Declarative** — Described entirely in a `SKILL.md` or `SKILL.yaml` file.
- **Discoverable** — Loaded at runtime from configured skill roots; no Go code changes or recompilation are required to add or update a Skill.
- **Governed** — Carries explicit metadata about effects, security posture, and performance characteristics so the Governor and policy logic can enforce constraints.

Skills do **not** own global reasoning, scheduling, or policy; they are invoked as tools by NAVI's Cognitive processes.

---

## Definition and Identity

A **Skill** is a versioned, declarative capability module that:

- Extends what NAVI can do without modifying Go code.
- Is defined entirely in a single folder containing `SKILL.md` or `SKILL.yaml`.
- Is discovered by the Skill loader and exposed to the LLM as one or more tool-callable interfaces.
- Declares its own permission profile, effects, and resource expectations.

Canonical identity fields (as reflected in `OSS27Spec` in `internal/navi/skill/spec.go`) include:

- `skill_id` — Stable dot-notation identifier (e.g., `navi.github.pr-review`).
- `semver` — Semantic version, used for compatibility and upgrade decisions.
- `display.name` — Human-readable name surfaced in skill listings.
- `display.description` — Short description of what the Skill does, injected into the LLM context.
- `display.emoji` (optional) — Visual cue in skill listings.

Legacy `SKILL.md` skills may omit some of these fields but must still clearly identify the skill name and purpose. New skills **should** prefer the OSS-27 `SKILL.yaml` format for full metadata.

---

## Schema Overview (High Level)

The canonical schema for Skills is defined by the `OSS27Spec` type in `internal/navi/skill/spec.go`. At a high level, every OSS-27 skill must define:

- **Display & Identity**
  - `oss27_version` — Spec version.
  - `skill_id`, `semver`, `display` (name, description, optional emoji).

- **Interfaces**
  - `interfaces[]` — One or more named entry points (e.g., `review_pr`, `summarize`).
  - `interfaces[].input_schema` — JSON Schema describing expected inputs.
  - `interfaces[].output_schema` — JSON Schema describing outputs.
  - `interfaces[].transport` — Declares *how* this interface executes (see Execution Model).

- **Effects**
  - `effects.side_effects` — Declares what the Skill can change (e.g., `writes_external_system`, `reads_filesystem`).
  - `effects.risk_tier` — Qualitative risk (`low`, `medium`, `high`).
  - `effects.requires_confirmation` — Whether human confirmation is required before execution.
  - `effects.idempotency_level` — Idempotency semantics (`idempotent`, `non_idempotent`, `conditional`); used by retries and composition. When set, it is authoritative and the legacy `effects.idempotency` flag is ignored; conflicting values must be treated as invalid by the validator.
  - `effects.reversibility` — Recovery semantics (`reversible_internal`, `compensable_external`, `irreversible`); used by failure and compensation logic.

- **Security**
  - `security.auth` — Required authentication mechanisms (e.g., OAuth scopes, API keys).
  - `security.data_access` — PII and secrets exposure characterization.
  - `security.sandbox` — Whether the Skill must run inside a sandbox and permitted network egress.

- **Performance**
  - `performance.timeout_ms`, `performance.rate_limit` — Timeouts and rate limits that inform governor behavior.

- **Governance**
  - `governance.publisher` — Who published the Skill.
  - `governance.signed` — Whether the Skill spec is cryptographically signed.
  - `governance.trust_tier` — Trust classification (`builtin`, `verified`, `community`, `local`) used by installation and policy.

- **Capability Metadata**
  - `capability.tags` — Free-form tags for discovery and UX (e.g., `github`, `code-review`).
  - `capability.domains` — Higher-level domain labels (e.g., `developer`, `personal`, `productivity`).
  - `capability.provides` — Named capabilities this Skill offers (e.g., `pr_review`).
  - `capability.requires` — Named capabilities or preconditions this Skill expects from the environment.

- **Reliability**
  - `reliability.expected_failure_modes` — Known failure modes (e.g., `permission_denied`, `rate_limited`).
  - `reliability.retry_policy` — Retry semantics (`none`, `safe`, `conditional`).

This document is the conceptual source of truth; field-level details and exact types are owned by `internal/navi/skill/spec.go`.

---

## Formats

NAVI recognizes two Skill formats with **different authority levels**:

- **`SKILL.yaml` (OSS-27, canonical executable format)**
  - Structured YAML following the OSS-27 Skill standard.
  - Required for Skills that interact with external systems, require authentication, declare side effects, or participate in marketplace/governance flows.
  - Canonical representation for all new executable Skills; supports full schema, security, effects, capability, reliability, and transport metadata.

- **`SKILL.md` (Legacy / advisory format)**
  - Markdown-based description of what the Skill does, when to use it, and step-by-step instructions.
  - Appropriate only for simple, low-risk, conversational or informational behaviors that do not require structured transport or governance metadata.
  - Lacks full schema; the system treats it as a prompt-defined advisory behavior, not as a fully governed capability.

Legacy `SKILL.md` skills MUST NOT declare or perform privileged, side-effecting execution. Governed, side-effecting capabilities MUST use `SKILL.yaml`.

Implementation-level guidance for authoring each format lives in [Skills System](../concepts/skills.md).

---

## Lifecycle, Identity, and Loading

Skills are discovered from workspace/user tiers plus active plugin manifests:

1. `<workspace>/skills/` — Workspace-specific skills for the current project.
2. `~/.navi/skills/` (or `NAVI_SKILLS_DIR`) — User-global skills.
3. `plugins/<name>/skills/...` — Repo-owned plugin skills declared by valid, active `plugin.yaml` manifests.

The root `skills/` directory is reserved for skill-system support such as shared runtimes and documentation. Repo-owned built-in capability skills must live under their owning plugin package.

Plugin-owned skills are activation-gated. The skill loader must not scan `plugins/*/skills/*` directly. The plugin loader first discovers and validates `plugin.yaml`, checks enabled/disabled state and gating metadata, then passes only declared `components.skills[]` paths into the skill registry.

When the same `skill_id` is present in multiple tiers, the **highest-priority** tier wins and lower tiers are ignored. This allows workspace or user-level overrides of built-in skills.

Identity and overrides are governed by:

- `skill_id` — The stable identity key for a Skill. All override decisions are keyed on `skill_id`, not folder name or display name.
- `semver` — Drives version selection and compatibility; breaking changes must be modeled as new versions.
- Folder paths and `display.name` — Cosmetic only; they must not participate in override or selection logic.

Before a Skill is made available to the agent, the loader enforces **requirements gating** based on metadata in the spec:

- `requires_bins` — All declared binaries must be present on `PATH`.
- `requires_env` — All declared environment variables must be set.
- `os` — The current OS must be compatible.

Skills that fail gating are not exposed to the LLM. They may be logged for diagnostics but must behave as if they do not exist from the agent's perspective.

At session start, the loader constructs a **Skill snapshot** summarizing all available skills and injects it into the agent system prompt. This snapshot is the Cognitive Layer's view of the Capability Layer.

---

## Interface Naming and Tool Surface

Each Skill may expose multiple interfaces. To keep the tool surface deterministic and transport-independent, NAVI uses a single canonical naming rule:

- **Canonical tool name**: `tool_name = "<skill_id>.<interface_name>"`
  - Example: `navi.github.pr-review.review_pr`

This canonical name MUST be used consistently:

- In the LLM tool definitions injected into the agent prompt.
- In logs and telemetry referring to skill invocations.
- In governance and policy rules that match on skills or interfaces.
- In proposals, error reports, and any persisted references to skill interfaces.

Transports and UI layers must not invent alternative names; they may present friendlier labels in UX but the underlying tool identity remains `skill_id.interface_name`.

## Execution Model

From the Cognitive Layer's perspective, Skill execution follows a consistent pattern:

1. The Conscious process decides to invoke a Skill interface during the **Decide → Execute** steps.
2. The Skill registry looks up the specified `skill_id` and interface, ensuring it is available and permitted.
3. The Skill executor reads the interface's `transport` block and dispatches accordingly.
4. Results are wrapped in a structured result envelope and returned to the Cognitive Layer, which continues the loop.

Supported transport types include (non-exhaustive):

- `mcp_tool` — Dispatch to an MCP server via JSON-RPC.
- `rest` — HTTP-based APIs.
- `internal` — Registered in-process Go handlers.
- `subprocess_python` — Governed Python subprocess runtime (see [Python Skill Runtime](../concepts/skill-python-runtime.md)).
- `subprocess` — Generic JSON-RPC stdio worker runtime for Python, Node/TypeScript, shell, or external harness adapters (see [Generic Subprocess Skill Runtime](../concepts/skill-subprocess-runtime.md)).

Regardless of transport, all Skills:

- Execute **under** governance (permissions, policy, configuration, and risk checks) rather than bypassing it.
- Are invoked with explicit, structured inputs and must return bounded, structured outputs.
- Must not compromise process stability; failures are recorded as first-class outcomes.

### Result Envelope

All Skill executions return a **result envelope** with, at minimum:

- A machine-readable status (success vs. error categories such as timeout or killed).
- A structured output payload for successful calls.
- Structured error information for failures.
- Basic metrics and metadata (e.g., duration, skill/interface identifiers).

For all transports, this envelope is represented by the `SkillExecutionResult` type in `internal/navi/skill/spec.go`. Transport-specific results (including the Python `SkillResult` described in [Python Skill Runtime](../concepts/skill-python-runtime.md)) are normalized into this shape so the Cognitive Layer and governance logic do not need transport-specific special cases.

---

## Design Principles and Constraints

Skills inherit and reinforce NAVI's core design principles:

- **Zero Framework Cognition**
  - Go code owns discovery, validation, transport, guardrails, and authoritative runtime control.
  - Skills describe *what* can be done and with what constraints; the control kernel decides when execution is allowed, and prompts support the reasoning behind that decision.

- **Governed by Default**
  - Skills must declare side effects and security posture up front.
  - Network access and filesystem writes are denied by default and must be explicitly requested in metadata.
  - Risk tier and confirmation requirements are part of the spec, not ad-hoc logic.

- **Deterministic Surfaces**
  - The same Skill spec must produce the same tool surface and behavioral expectations across runs.
  - Versioning via `semver` is required; breaking changes must be modeled as new versions.

- **Single-Responsibility Capabilities**
  - Each Skill should do a small number of related things well, expressed as its interfaces.
  - Cross-cutting workflows are composed by the Cognitive Layer from multiple Skills and Commands.

---

## Invariants

The following invariants apply to all Skills:

1. **No direct World Model writes** — Skills never write directly to the World Model; they act only through primitive Commands and other governed capability surfaces.
2. **No governance bypass** — Skills always execute under the Governor and policy system; they cannot skip permission, policy, configuration, or risk checks.
3. **No hidden durable state** — Skills do not own durable truth. Long-lived state lives in NAVI entities or explicitly declared external systems; skills must not maintain private, undeclared stores that affect behavior.
4. **Legacy format constraints** — `SKILL.md` skills are advisory only and cannot declare or perform privileged, side-effecting execution; governed capabilities must use `SKILL.yaml`.
5. **Side-effect declaration** — Any interface that can produce side effects must declare `effects.side_effects`, `effects.idempotency_level`, and `effects.reversibility` so retries and compensation can be reasoned about correctly.

---

## Versioning and Compatibility

Skills use semantic versioning (`semver`) to communicate compatibility guarantees:

- **Breaking changes (require a new major version)** include:
  - Removing an interface or renaming it.
  - Tightening input schemas incompatibly (e.g., removing required fields, narrowing types without safe defaults).
  - Changing output schemas in ways that would break existing consumers.
  - Changing transports or effects/security/risk in ways that materially alter behavior or guarantees.
- **Non-breaking changes (minor/patch)** include:
  - Adding new optional fields to input or output schemas.
  - Adding new interfaces without changing existing ones.
  - Relaxing constraints or improving performance without changing behavior.

Registries and loaders should treat incompatible upgrades as separate major lines and, where possible, warn or block in-place upgrades that violate these rules.

---

## Authoring Guidelines (Canonical Level)

When creating a new Skill:

- Prefer `SKILL.yaml` (OSS-27) for any capability that:
  - Calls external services.
  - Reads or writes local or remote state.
  - Requires authentication or elevated permissions.
  - Needs explicit side-effect, trust, capability, or reliability metadata.
- At minimum, define:
  - `skill_id`, `semver`, and `display` fields.
  - At least one `interfaces[]` entry with clear `input_schema` and `output_schema`.
  - `effects.side_effects`, `effects.risk_tier`, and `effects.requires_confirmation`.
  - Appropriate `security` metadata (auth, data access, sandbox/network egress).
- Use `SKILL.md` only for lightweight, low-risk, purely informational skills where structured execution metadata is unnecessary; these are treated as advisory behaviors, not fully governed capabilities.
- Choose a Skill when:
  - You are extending NAVI's abilities in a way that can be expressed declaratively.
  - You want the Cognitive Layer to be able to decide autonomously when to use the capability.
- Prefer connectors, native Go, or plugins when:
  - You are defining a new integration surface for many skills to share.
  - You need deep, low-level access to the runtime or infrastructure.

Implementation recipes, templates, and examples live in [Skills System](../concepts/skills.md) and related runbooks.

---

## Validation and Enforcement

Skill specs are not just descriptive metadata — they are inputs to a validator that enforces hard constraints before any Skill is made available.

At minimum, a valid `SKILL.yaml` **MUST** satisfy:

- **Enum validity**
  - `effects.idempotency_level`, `effects.reversibility`, `governance.trust_tier`, and `reliability.retry_policy` must use known values only; unknown values are rejected.

- **Legacy format constraints**
  - `SKILL.md` skills are treated as advisory only. Any skill that declares non-trivial `effects.side_effects` or executable transports **must** use `SKILL.yaml`.

- **Risk and security**
  - Skills with `effects.risk_tier: "high"` must declare sufficient `security` metadata (auth, data_access, sandbox/network_egress) for their transports; missing or incomplete security is rejected.

- **Irreversibility and confirmation**
  - Skills with `effects.reversibility: "irreversible"` must set `effects.requires_confirmation: true`. Irreversible + no confirmation is rejected.

- **External transports and sandboxing**
  - Skills using external transports (`rest`, `mcp_tool`, `subprocess`, or `subprocess_python` with network access enabled) must declare appropriate `security.auth` and `security.sandbox.network_egress`. Missing declarations are rejected.

The skill loader and registry are responsible for applying these rules so that the Cognitive Layer only ever sees Skills that satisfy the canonical governance model.

### Trust Tiers

Trust tiers influence how aggressively NAVI may use a Skill, especially in autonomous or high-risk contexts:

- `builtin` — Shipped with NAVI. Eligible for autonomous execution by default within governor limits, subject to risk tier and owner configuration.
- `verified` — Signature and publisher validated against a trusted registry. Eligible for autonomous execution, but owner/tenant policy may impose additional bounds.
- `community` — Published by third parties without verification. Installable, but not eligible for high-autonomy or high-risk execution by default; typically requires explicit confirmation or stricter guardrails.
- `local` — User-authored or manually installed. Trusted only to the extent the owner configures; conservative defaults (e.g., confirmation required, tighter risk bounds) should apply.

The governor and policy engines should combine `trust_tier` with `effects.risk_tier` and `effects.requires_confirmation` when deciding whether a Skill may run autonomously or must involve the user.

## Relationships and References

Skills participate in NAVI's architecture as follows:

- They are part of the **Capability Layer**, sitting below the Cognitive Layer and above concrete transports and runtimes.
- They rely on the **Governor** and policy systems to enforce cost, risk, and autonomy limits.
- They may depend on external connectors, plugins, or runtimes but must always declare those dependencies in their specs.

For further details and evolving implementation notes, see:

- [Skills System (concepts)](../concepts/skills.md)
- [Python Skill Runtime (concepts)](../concepts/skill-python-runtime.md)
- [Generic Subprocess Skill Runtime (concepts)](../concepts/skill-subprocess-runtime.md)
- [Orchestration Loop](../concepts/orchestration-loop.md)

