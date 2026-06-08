# NAVI Coder ↔ Programmer ID Aliasing (NEW-A)

**Status:** Active
**Ticket:** NEW-A (prerequisite for OMN-283)
**Source audit:** [`../reviews/navi-coder-programmer-reference-audit.md`](../reviews/navi-coder-programmer-reference-audit.md) §5–§6
**Canonical naming:** [`../canonical/navi-coder.md`](../canonical/navi-coder.md)

> ## ⛔ Migration invariant
> **NEW-A creates compatibility. It does not rename the product substrate.**
> **OMN-283 starts only after NEW-A merges and its tests pass.**
>
> The legacy Programmer identifiers remain canonical **at rest**. This work never
> edits `plugin.yaml`, `SKILL.yaml`, compiled workflow JSON, fixtures, or prompt
> `.md` files, and it renames no constant or package. It adds only a Go
> resolution layer. Every change is additive and reversible.

---

## Why this exists

NAVI Coder is the first-class programming product domain; `navi.programmer` /
`navi-programmer` are transitional substrate identifiers. The OMN-280 audit found
that many Programmer identifiers are **load-bearing** — renaming them blindly
breaks plugin loading, skill dispatch, the workflow bridge, prompt loading, the
CLI, and persisted/test references.

This alias layer lets forward-facing **Coder** identifiers coexist with — and
resolve to — the existing **Programmer** implementation, so future work
(OMN-283 package repositioning, NEW-D prompt-kind rename) can proceed without a
risky big-bang rename.

## The four ID classes

| Class | Legacy (canonical at rest) | Coder alias (resolves to legacy) |
|-------|----------------------------|----------------------------------|
| Plugin | `navi.programmer` | `navi.coder` |
| Skills (×7) | `navi-programmer.<s>` | `navi-coder.<s>` |
| Workflows (×3) | `navi.programmer.<w>` | `navi.coder.<w>` |
| Prompt kind | `ncos/programmer_workflow` | `ncos/coder_workflow` |

Skills: `file-mutate, git-lifecycle, patch-apply, repo-inspect, remote-review,
run-validation, task-normalize`. Workflows: `bounded_mutation,
self_update_candidate, ticket_driven_coding`.

## Single source of truth

`internal/coderalias` is a pure leaf package (imports nothing from NAVI) that
owns the plugin/skill/workflow mappings. It is **table-driven** — every function
is "lookup in a bounded map, or return the input unchanged." There is **no**
`strings.Replace` / `TrimPrefix`+concat anywhere, which is what structurally
guarantees collision safety (below).

Key functions: `CanonicalSkillID`, `CoderSkillID`, `IsProgrammerScopedSkillID`,
`CanonicalWorkflowID`, `CoderWorkflowID`, `CanonicalPluginID`,
`PluginIDsEquivalent`.

The **prompt-kind** alias lives in `internal/prompts` (which owns the `ncos/*`
namespace), as a one-line local map normalized on the read path — see below.

## Where resolution happens

**Live** (exercised by existing/external flows):

| Site | File | What changed |
|------|------|--------------|
| Workflow bridge skill scope guards | `internal/navi/programmer_workflow_bridge.go` (`resolveProgrammerWorkflowSkill`) | `strings.HasPrefix(…,"navi-programmer.")` → `coderalias.IsProgrammerScopedSkillID(…)`; returned `skillID` canonicalized via `CanonicalSkillID`, so downstream literal dispatch (`task-normalize`, mutation interfaces) works for both namespaces unchanged |
| Workflow id guard | same file (line ~144) | compares a local `coderalias.CanonicalWorkflowID(state.WorkflowID)`; persisted state is **not** mutated |
| CLI skill smoke | `cmd/navi/main.go` (`runSkillSmoke`) | `skillID = coderalias.CanonicalSkillID(skillID)` before the `switch` |

**Forward-compat seams** (no in-tree caller yet; for external/Coder clients and OMN-283):

| Site | File | What changed |
|------|------|--------------|
| Gateway skill-by-id lookup | `internal/gateway/server.go` (`lookupSkillEntry`) | on miss, one retry with `coderalias.CanonicalSkillID(id)` so `/api/skills/navi-coder.*/…` resolves to the legacy skill |
| Prompt kind | `internal/prompts/{kind.go,manager.go}` | `KindNCOSCoderWorkflow` constant (**not** in `AllKinds()`); normalized to the legacy kind in `Render`, `Snapshot`, `IsKnownKind` so it renders the existing `ncos/programmer_workflow.md` |
| Handler dispatch | `plugins/navi-programmer/handlers/run_validation_handler.go` | `RegisterProgrammerHandlers` also registers under `coderalias.CoderSkillID(skillID)`, so `GetInternalHandler("navi-coder.run-validation", …)` resolves |
| Plugin manifest predicate | `internal/navi/plugin/manifest.go` (`Manifest.ResolvesID`) | pure predicate matching own id + `coderalias.PluginIDsEquivalent`; the seam OMN-283 wires into registry/lifecycle by-id lookups |

## Collision safety

A **separate, real** plugin already exists at `plugins/navi-coder/` with plugin
id `navi-coder` (hyphen, no dot) and skill id `navi.coder.repo` (dotted). These
are unrelated to the alias and must never be canonicalized into a Programmer id.
The bounded, table-driven design guarantees this:

- Skill ids are hyphen-scoped (`navi-coder.`), which never matches the dotted
  `navi.coder.repo` or the bare `navi-coder`.
- Workflow canonicalization is an exact-set map on the three known workflow ids —
  never a prefix rewrite — so `navi.coder.repo` is returned unchanged.
- `PluginIDsEquivalent("navi-coder", "navi.programmer")` is `false`; the
  equivalence set is exactly `{navi.programmer, navi.coder}`.

`internal/navi/skill/loader_test.go` (asserts `navi.coder.repo` loads intact)
remains the regression guard.

## Reversibility

Every wiring is an additive shim that falls back to legacy. Deleting
`internal/coderalias`, the prompt-kind shim, and the call sites restores exact
prior behavior.

## Tests

`internal/coderalias/alias_test.go` (mappings, reversibility, explicit
`navi.coder.repo`/`navi-coder` non-mangling); `internal/navi/plugin/manifest_alias_test.go`
(`ResolvesID`); `internal/navi/programmer_workflow_bridge_alias_test.go`
(`navi-coder.*` routes and canonicalizes); `internal/prompts/coder_alias_test.go`
(`Render(coder) == Render(programmer)`, alias kind absent from `AllKinds()`);
`internal/gateway/skill_alias_test.go` (`lookupSkillEntry` resolves the alias);
`plugins/navi-programmer/handlers/coder_alias_test.go` (handler reachable under
both ids).

## What becomes safe after this lands

- **OMN-283** — move `plugins/navi-programmer/` to the Coder substrate; update the
  two compile-time import paths (`cmd/navid/plugin_bootstrap.go`,
  `plugins/navi-coder/handlers/coder_repo_handler.go`); rename on-disk
  `skill_id`/`workflow_id` in manifests + compiled JSON; wire `Manifest.ResolvesID`
  into registry/lifecycle by-id lookups. The alias layer keeps runtime dispatch
  working throughout the move.
- **NEW-D** — atomic prompt-kind rename (`KindNCOSProgrammerWorkflow` constant +
  value + `programmer_workflow.md` filename + prose). The `prompts` normalization
  shim is the seam that lets this happen without breaking callers.

No broad rename of `navi.programmer` and no package move are performed by NEW-A.
