# Autonomy — Four Dimensions and Preset Mapping

**Status:** Active  
**Source:** [Conceptual Design Overview](../canonical/conceptual-design-overview.md) (Autonomy Model)  
**Implementation:** `internal/config/config.go` (`AutonomyConfig`, `PresetToDimensions`), `internal/governor/autonomy.go`

---

## Four dimensions

The design defines four independently configurable autonomy dimensions. Each can be set per dimension in the conceptual model; the current implementation exposes a **preset per domain** that bundles all four. Presets are defined below so that future per-dimension overrides can be added without redesign.

| Dimension | What it controls |
|-----------|------------------|
| **Execution Threshold** | Risk level below which NAVI executes without asking. High = act broadly unless hard floor or risk flags; low = most external actions generate a Proposal. |
| **Insight Surfacing** | How readily Subconscious findings surface (corrections-only vs proactive advisory). Does not change urgent-interruption criteria. |
| **Memory Promotion Sensitivity** | How aggressively observations advance toward Knowledge/Memory promotion. High = fast learning, more inferred state; low = high confidence, slower promotion. |
| **Plugin Invocation Authority** | Whether plugins run autonomously when triggers are met or require per-invocation confirmation. Subject to governance hard floors. |

---

## Preset → dimensions mapping (implementation)

`PresetToDimensions(preset)` in `internal/config/config.go` maps each preset name to the four dimensions. Effective behavior today is driven by **Execution Threshold** (via `ApplyAutonomy` in the governor); the other three are represented in the struct for consistency and future use.

| Preset | Execution | Insight | Memory | Plugin |
|--------|-----------|---------|--------|--------|
| **conservative** | low | medium | low | low |
| **balanced** | medium | medium | medium | medium |
| **high** | high | high | high | high |

- **Execution Threshold** is the only dimension currently used in code: when validation returns RequiresConfirmation, no hard floor applies, and effective preset for the domain is `"high"`, the governor may return Approved instead (`ApplyAutonomy`).
- **Insight Surfacing**, **Memory Promotion Sensitivity**, and **Plugin Invocation Authority** are set by the preset but not yet wired as separate knobs; reflection and plugin paths can be extended to read them when needed.

---

## Configuration surface

- **Global preset:** `autonomy.global_preset` (`conservative` | `balanced` | `high`).
- **Per-domain overrides:** `autonomy.domain_overrides` (e.g. `coding: high`, `memory: conservative`). Domain keys match `governor` (e.g. `messaging`, `scheduling`, `coding`, `memory`, `plugins`, `workflow`, `delegation`, `configuration`).
- **Per-dimension overrides:** `autonomy.domain_dimension_overrides` (optional). Keys are domain names; values are YAML objects with optional `execution_threshold`, `insight_surfacing`, `memory_promotion_sensitivity`, `plugin_invocation_authority`. Empty values fall back to the preset-derived dimension for that domain. So all four dimensions are exposed as knobs per domain.

---

## Governor vs Autonomy

- **Governor** defines what is permitted: permissions, policy, configuration, priority alignment, risk (validation pipeline), plus action/cost/duration budgets and hard floors. The governor never expands what is allowed; it only rejects or requires confirmation.
- **Autonomy** defines how much NAVI does without asking *within* those bounds. It only downgrades `RequiresConfirmation` → `Approved` when the effective execution threshold for the domain is `high` and no hard floor applies (system policy, owner-set, proposal-required, irreversible, risk override). See `ApplyAutonomy` in `internal/governor/autonomy.go` and hard floor constants there.
- Validation path: run the pipeline (Governor) first; then, when the result is `RequiresConfirmation`, optionally apply autonomy (Autonomy) to produce `Approved` when the preset/threshold and hard floors allow.
