# NAVI Coder Migration — Programmer Reference Audit

**Status:** Active
**Last Updated:** 2026-06-06
**Linear:** OMN-280 (Audit and classify stale Programmer references) · parent OMN-279
**Pass type:** Audit only — **no code, label, runtime-ID, or package changes were
made in this pass.** All renames below are *recommendations* for staged follow-up.

**Grounded in the canonical direction:**

- `docs/canonical/navi-coder.md`
- `docs/architecture/navi-coder-product-architecture.md`
- `docs/plans/navi-coder-migration.plan.md`

**Supersedes (in scope):** `docs/plans/navi-coder-migration-inventory.md` — the
initial precursor inventory, which searched 6 of the 8 required terms and was
written before the canonical docs landed. That file is retained; this audit is the
authoritative OMN-280 deliverable.

---

## 1. Executive summary

NAVI Coder is now NAVI's **first-class programming product domain**. The canonical
reference is explicit:

> *"NAVI Coder is NAVI's first-class programming product domain."*
> (`docs/canonical/navi-coder.md:9`)

> *"The product name is **NAVI Coder**. The term **NAVI Programmer** is deprecated
> except when referring to legacy package names or migration history."*
> (`docs/canonical/navi-coder.md:13`)

> *"Any new document that presents 'NAVI Programmer' as the first-class product is
> stale."* (`docs/canonical/navi-coder.md:277`)

The existing `navi.programmer` / `navi-programmer` plugin is a **useful prototype
substrate**, explicitly transitional:

> *"The current `navi.programmer` package is a transitional implementation
> substrate. It should be repositioned, renamed, or migrated into a Coder-owned
> package…"* (`docs/canonical/navi-coder.md:222`)

This audit classifies every meaningful `Programmer` reference so later migration
work (OMN-281/282/283/284/285) can proceed **without breaking runtime plugin
loading or NATS/workflow routing.** The central distinction:

| Category | Treatment |
| --- | --- |
| **Stale product language** (prose, display labels, doc framing) | Safe to reframe to "NAVI Coder" — but verify no test asserts the literal first |
| **Runtime identifiers** (plugin ID, skill IDs, workflow IDs, import paths, prompt-kind keys, fixtures) | **Do NOT rename blindly.** Require staged migration behind compatibility aliases |

**The single most important finding for safety:** the load-bearing identifiers
(`id: navi.programmer`, `navi-programmer.*` skill IDs, `navi.programmer.*` workflow
IDs) are referenced by **hardcoded string guards** in Go
(`internal/navi/programmer_workflow_bridge.go`), the CLI (`cmd/navi/main.go`), the
plugin bootstrap import (`cmd/navid/plugin_bootstrap.go`), the new Coder
compatibility bridge (`plugins/navi-coder/handlers/coder_repo_handler.go`), the
workflow runner, the compiled workflow JSON, and dozens of test fixtures. Renaming
any of these without an alias mechanism will silently break plugin discovery and
workflow dispatch.

**Two clean-slate findings that de-risk the next tickets:**

1. **The console has zero references.** A case-insensitive search of `web-src/**`
   for any `programmer` token returns **no matches** — so OMN-281 can add a
   top-level `/coder` route and workspace shell with no rename entanglement.
2. **A nascent `plugins/navi-coder/` already exists** (`plugin.yaml`,
   `handlers/coder_repo_handler.go`, `skills/repo/`), and it *already* bridges to
   the legacy package by importing `navi-programmer` handlers — a working model for
   the staged-alias approach OMN-283 should generalize.

---

## 2. Count by classification

Raw term occurrences across the tracked tree (overlapping — e.g. every
`NAVI Programmer` also counts under `Programmer`):

| Search term | Occurrences |
| --- | --- |
| `NAVI Programmer` | 279 |
| `navi.programmer` | 62 |
| `navi-programmer` | 604 |
| `Programmer V1` | 26 |
| `programmer plugin` | 9 |
| `first-class programming capability plugin` | 7 |
| `Programmer` (whole word, case-sensitive) | 322 |
| `programmer` (whole word, case-insensitive) | 1103 |

Distribution by top-level area (files containing any `programmer` token):

| Area | Files |
| --- | --- |
| `plugins/` | 90 |
| `docs/` | 22 |
| `internal/` | 7 |
| `cmd/` | 2 |
| `Dockerfile` | 1 |
| `web-src/` | **0** |

Approximate classification of *meaningful* references (judgment-based grouping;
sums exceed unique lines because terms overlap and one location can carry both a
runtime ID and a display label):

| Classification | Approx. instances | Where it concentrates |
| --- | --- | --- |
| `migration_history` | ~200+ | docs snapshots, canonical deprecation notes, plugin `PROJECT_STATE.md`/`TASK_QUEUE.md`, forwarding stubs |
| `stale_product_language` | ~180+ | skill `README.md`s, plugin docs (concepts/specs/plans/runbooks), product-framing prose |
| `code_identifier_requiring_staged_rename` | ~150+ | workflow/skill IDs in YAML + `.compiled.json` + runner + Go bridge guards + CLI + `kind.go` |
| `test_fixture_compatibility` | ~80+ | `plugins/navi-programmer/tests/**`, fixtures `*.json`, `programmer_workflow_bridge_test.go` |
| `user_facing_label_requires_immediate_rename` | ~80+ | plugin `name`/description, 7 skill display names, prompt prose, README/INDEX titles |
| `safe_to_leave_temporarily` | ~30+ | comments, `.gitignore`, `Dockerfile` |
| `legacy_package_reference` | ~15+ | Go imports, Go module path, plugin directory name |

> The high-risk surface is the small `code_identifier` + `legacy_package` +
> runtime-fixture subset, **not** the large docs/prose bulk. Most of the 1103
> occurrences are prose or history that can be reframed gradually.

---

## 3. File-by-file inventory

### 3.1 `plugins/navi-programmer/` — plugin assets (90 files)

**Manifest — `plugins/navi-programmer/plugin.yaml`**

| Line | Reference | Classification |
| --- | --- | --- |
| 1 | `id: navi.programmer` | `code_identifier_requiring_staged_rename` (plugin discovery/registration key) |
| 2 | `name: NAVI Programmer` | `user_facing_label_requires_immediate_rename` (display only) |
| 3 | `description: First-party programming capability plugin…` | `user_facing_label_requires_immediate_rename` |
| 59–71 | `skill_id: navi-programmer.{file-mutate,git-lifecycle,patch-apply,repo-inspect,remote-review,run-validation,task-normalize}` | `code_identifier_requiring_staged_rename` (runtime skill IDs) |
| 74, 79, 84 | `workflow_id: navi.programmer.{bounded_mutation,self_update_candidate,ticket_driven_coding}` | `code_identifier_requiring_staged_rename` |
| 82, 87 | `extends: navi.programmer.bounded_mutation` | `code_identifier_requiring_staged_rename` |

**Skills — `plugins/navi-programmer/skills/*/SKILL.yaml`** (repeating pattern, ×7)

- `SKILL.yaml:2` → `skill_id: "navi-programmer.<folder>"` → `code_identifier_requiring_staged_rename` (runtime contract; mirrored by `main.py` `SKILL_ID` constants).
- `SKILL.yaml:6` → `name: "NAVI Programmer <Operation>"` → `user_facing_label_requires_immediate_rename` (display).
- `skills/*/README.md`, `skills/*/main.py` docstrings → `stale_product_language`.
- Representative paths: `skills/repo-inspect/SKILL.yaml:2`, `skills/file-mutate/SKILL.yaml:2`, `skills/git-lifecycle/SKILL.yaml:2`, … (all seven follow the same shape).

**Workflows — `plugins/navi-programmer/workflows/`**

- `bounded-mutation.yaml:1` `workflow_id: navi.programmer.bounded_mutation` — `code_identifier_requiring_staged_rename`.
- `self-update-candidate.yaml:1` `workflow_id: navi.programmer.self_update_candidate`, `:11` `extends:` — `code_identifier_requiring_staged_rename`.
- `ticket-driven-coding.yaml:1` `workflow_id: navi.programmer.ticket_driven_coding`, `:6` `extends:` — `code_identifier_requiring_staged_rename`.
- `*.compiled.json` (3 files) — generated mirrors of the YAML; contain the same `navi.programmer.*` / `navi-programmer.*` strings → `code_identifier_requiring_staged_rename` (keep in sync with source).
- `bounded_mutation_runner.py` — `RUNNER_ID = "navi-programmer.bounded-mutation-runner"` plus skill-ID prefix/equality checks → `code_identifier_requiring_staged_rename`.

**Docs corpus — `plugins/navi-programmer/docs/**`** (concepts, design, specs, plans, runbooks, decisions): predominantly `stale_product_language` (product framing) and `migration_history` (decisions, historical context). Notable: `specs/navi-programmer-plugin-v1.md` (entire `Programmer V1` spec), `concepts/navi-programmer.md`.

**Plugin meta — `plugins/navi-programmer/`**: `CLAUDE.md`, `README.md`, `AGENTS.md` → `stale_product_language` (product framing portions); `PROJECT_STATE.md`, `TASK_QUEUE.md`, `NEXT_ACTION.md`, `docs/decisions/*.md` → `migration_history` (preserve historical record).

**Tests & fixtures — `plugins/navi-programmer/tests/**`**: `test_manifest_and_runner_contracts.py`, `test_programmer_v1_end_to_end.py`, per-skill tests, `fixtures/*.json`, eval suites → `test_fixture_compatibility` (assert the runtime IDs; preserve until aliases exist, then assert old + new deliberately). Special case: `tests/test_coder_migration_inventory.py` is the **guardrail test** that pins the precursor inventory's content (see §5.6).

### 3.2 `internal/**` (7 files)

| File:line | Reference | Classification |
| --- | --- | --- |
| `internal/navi/programmer_workflow_bridge.go:144` | guard `state.WorkflowID == "navi.programmer.bounded_mutation" \|\| … "navi.programmer.ticket_driven_coding"` | `code_identifier_requiring_staged_rename` |
| `…bridge.go:231` | `strings.HasPrefix(skillEntry.Spec.SkillID, "navi-programmer.")` | `code_identifier_requiring_staged_rename` |
| `…bridge.go:239` | `strings.HasPrefix(registeredTool.SourceID, "navi-programmer.")` | `code_identifier_requiring_staged_rename` |
| `…bridge.go:328/330/332` | returns `"navi.programmer.self_update_candidate"` / `…ticket_driven_coding"` / `…bounded_mutation"` | `code_identifier_requiring_staged_rename` |
| `…bridge.go:445` | `skillID == "navi-programmer.task-normalize"` dispatch | `code_identifier_requiring_staged_rename` |
| `…bridge.go:513/515/517` | `case "navi-programmer.file-mutate" / patch-apply / git-lifecycle` | `code_identifier_requiring_staged_rename` |
| `…bridge.go:19–23` | scratchpad/progress keys `"programmer_workflow*"` | `code_identifier_requiring_staged_rename` (persisted string keys) |
| `…bridge.go` (filename + `programmer*` symbols) | bridge file & internal identifiers | `code_identifier_requiring_staged_rename` |
| `internal/prompts/kind.go:27` | `KindNCOSProgrammerWorkflow Kind = "ncos/programmer_workflow"` | `code_identifier_requiring_staged_rename` (maps to embedded prompt path) |
| `internal/prompts/defaults/ncos/programmer_workflow.md:1` | prose: "you are acting as NAVI's programmer … using the navi-programmer skills" | `user_facing_label_requires_immediate_rename` (prose) + skill IDs are compatibility |
| `internal/navi/programmer_workflow_bridge_test.go` (many) | fixture/test data incl. the literal file path `internal/navi/programmer_workflow_bridge.go` and `navi.programmer.*` / `navi-programmer.*` IDs | `test_fixture_compatibility` |
| `internal/navi/plugin/loader_test.go:572,578` | `filepath.Join(repoRoot,"plugins","navi-programmer")`, `Normalize("navi-programmer", …)` | `code_identifier_requiring_staged_rename` |
| `internal/navi/runtime_executor.go:2071` | comment mentioning navi-programmer bounded-mutation | `safe_to_leave_temporarily` |

### 3.3 `cmd/**` (2 files)

| File:line | Reference | Classification |
| --- | --- | --- |
| `cmd/navid/plugin_bootstrap.go:18` | `naviprogrammer "github.com/ceoai/navi/plugins/navi-programmer/handlers"` | `legacy_package_reference` (import path tied to directory name) |
| `cmd/navi/main.go:2118` | `case "navi-programmer.repo-inspect":` | `code_identifier_requiring_staged_rename` |
| `cmd/navi/main.go:2120` | `case "navi-programmer.run-validation":` | `code_identifier_requiring_staged_rename` |

### 3.4 `plugins/navi-coder/**` — existing Coder bridge

| File:line | Reference | Classification |
| --- | --- | --- |
| `plugins/navi-coder/handlers/coder_repo_handler.go:9` | `naviprogrammer "github.com/ceoai/navi/plugins/navi-programmer/handlers"` | `legacy_package_reference` (intentional compatibility bridge) |
| `plugins/navi-coder/handlers/coder_repo_handler.go:41` | `coreskill.GetInternalHandler(naviprogrammer.RunValidationSkillID, "run_command")` | `legacy_package_reference` (reuses legacy skill ID by symbol) |

### 3.5 `docs/**` (22 files)

- `docs/canonical/navi-coder.md` (L13, 222, 274–275, 277, 321, 326) — **authoritative** statements *about* the deprecation/migration → `migration_history` (do not "fix"; this is the source of truth).
- `docs/plans/navi-coder-migration.plan.md`, `docs/plans/navi-coder-migration-inventory.md` — `migration_history` (the plan and precursor inventory).
- `docs/architecture/navi-coder-product-architecture.md` (L67, 261, 318, 322, 336, 376) — references to the prototype substrate → `migration_history` / `stale_product_language` (framing).
- `docs/reviews/navi-status-2026-05-*.md` — `migration_history` (status snapshots; append, don't rewrite).
- `docs/concepts/INDEX.md`, `docs/concepts/navi-programmer.md`, and forwarding stubs under `docs/design/`, `docs/specs/`, `docs/plans/` — `migration_history` + `stale_product_language` (legacy pointers; reframe to Coder-first).
- `docs/concepts/navi-coder-capability.md` — design note acknowledging the legacy namespace → `migration_history`.

### 3.6 Config / root (1 file + incidental)

- `Dockerfile:44` — comment referencing navi-programmer git-lifecycle → `safe_to_leave_temporarily`.
- `.gitignore` — ignored sample path under `plugins/navi-programmer/` → `safe_to_leave_temporarily`.

---

## 4. Immediate fixes recommended (NOT applied in this pass)

These are **display/prose only** — they change no runtime identifier. Per the
kickoff scope they are *recommended, not executed* here. **Caveat for each:**
confirm no test asserts the literal display string before editing — in particular
`plugins/navi-programmer/tests/test_manifest_and_runner_contracts.py` and the
per-skill tests may assert `name:` values.

1. `plugins/navi-programmer/plugin.yaml:2` — `name: NAVI Programmer` → **`NAVI Coder (legacy package: navi.programmer)`** or similar. Keep `id: navi.programmer` (L1) unchanged.
2. `plugins/navi-programmer/plugin.yaml:3` — reword description away from "programming capability plugin" toward NAVI Coder's built-in governed software-work capability.
3. `plugins/navi-programmer/skills/*/SKILL.yaml:6` — display `name:` fields "NAVI Programmer …" → "NAVI Coder …". Keep `skill_id` (L2) unchanged.
4. `internal/prompts/defaults/ncos/programmer_workflow.md:1` — prose "you are acting as NAVI's programmer" → "you are acting as **NAVI Coder**". Keep the `navi-programmer.*` skill names it references (they are runtime IDs) and keep the prompt `Kind` value unchanged (see §5).
5. Plugin product-framing prose — `plugins/navi-programmer/CLAUDE.md`, `README.md`, `docs/README.md`, `docs/INDEX.md` — reframe "NAVI Programmer is the … plugin" as "NAVI Coder core capability substrate (legacy package `navi.programmer`)".
6. Root forwarding pointers under `docs/concepts/`, `docs/design/`, `docs/specs/`, `docs/plans/` — mark as legacy/transitional and point to Coder-first docs; do not let any root doc state "NAVI Programmer" is the current product.

> Recommended grouping for execution: items 1–3 in OMN-283; item 4 alongside the
> prompt-kind rename (a staged code change); items 5–6 as a docs-reframing pass.

---

## 5. References that MUST NOT be renamed blindly (runtime identifiers & compatibility paths)

Renaming any of the following without first introducing a **compatibility alias /
dual-resolution mechanism** will break the running system. Listed with the failure
mode and the safe mitigation.

### 5.1 Plugin ID — `plugin.yaml:1` `id: navi.programmer`
Used as the plugin discovery/registration key. *Breaks:* plugin loading and any
lookup keyed on the ID. *Mitigation:* keep transitional, or add an `aliases:`
field resolved by the loader before changing.

### 5.2 Skill IDs — `navi-programmer.*` (×7)
`plugin.yaml:59–71`, `skills/*/SKILL.yaml:2`, `main.py` `SKILL_ID` constants, and
**Go prefix guards** at `programmer_workflow_bridge.go:231` and `:239`. *Breaks:*
skill registration filtering, tool synthesis, and the bridge's skill selection.
*Mitigation:* introduce skill-ID alias resolution (`navi-coder.* ⇆ navi-programmer.*`)
**before** renaming; update the prefix guards to accept both namespaces.

### 5.3 Workflow IDs — `navi.programmer.{bounded_mutation,self_update_candidate,ticket_driven_coding}`
`plugin.yaml:74/79/84` (+`extends` 82/87), the workflow YAML headers, the
`.compiled.json` mirrors, and **Go guards/returns** at
`programmer_workflow_bridge.go:144,328,330,332`. *Breaks:* workflow dispatch and
the self-update/ticket-driven flows. *Mitigation:* alias workflow IDs; update the
guards and the compiled JSON together.

### 5.4 Skill-specific Go dispatch — `programmer_workflow_bridge.go:445,513,515,517`
Hardcoded equality/`case` on `navi-programmer.task-normalize`, `…file-mutate`,
`…patch-apply`, `…git-lifecycle`. *Breaks:* per-skill bridging behavior.
*Mitigation:* drive these from the alias map, not literals.

### 5.5 CLI special-cases — `cmd/navi/main.go:2118,2120`
`case "navi-programmer.repo-inspect"` / `"navi-programmer.run-validation"`.
*Breaks:* CLI skill routing for those two skills. *Mitigation:* update in lockstep
with skill-ID aliases.

### 5.6 Import paths & directory name
`cmd/navid/plugin_bootstrap.go:18` and
`plugins/navi-coder/handlers/coder_repo_handler.go:9` import
`github.com/ceoai/navi/plugins/navi-programmer/handlers`; the package symbol
`naviprogrammer.RunValidationSkillID` is reused. *Breaks:* compilation if the
`plugins/navi-programmer/` directory is moved/renamed. *Mitigation:* OMN-283 must
move the package and update both imports atomically (or keep a thin re-export
package at the old path).

### 5.7 Prompt-kind key — `internal/prompts/kind.go:27` `"ncos/programmer_workflow"`
Maps the `Kind` constant to the embedded prompt file
`internal/prompts/defaults/ncos/programmer_workflow.md`. *Breaks:* prompt loading
if the value or filename changes independently. *Mitigation:* rename the constant,
value, and file together as one staged change.

### 5.8 Test fixtures & the guardrail test
`plugins/navi-programmer/tests/**` and `programmer_workflow_bridge_test.go` assert
the literal runtime IDs and (in the Go test) the literal file path. *Breaks:* the
test suite. *Mitigation:* update fixtures alongside each aliasing step; assert both
old and new IDs.
**Special note:** `plugins/navi-programmer/tests/test_coder_migration_inventory.py`
pins the precursor inventory's content — including the now-stale literal
`"not present in this checkout"`. This test must be updated **before** the
precursor inventory's stale claim can be corrected (see §6, net-new ticket).

---

## 6. Suggested follow-up tickets

**Existing children (recommended order after this audit):**

- **OMN-282** — Introduce NAVI Coder backend domain skeleton in `navid`
  (`internal/coder/`). No rename dependency; can start immediately.
- **OMN-281** — Add top-level `/coder` console route + workspace shell. **Clean:**
  `web-src/**` has zero `programmer` references — pure additive scaffold.
- **OMN-283** — Reposition the legacy `navi.programmer` package as Coder core
  capability substrate. **Blocked on the alias mechanism below.**
- **OMN-284** — Define/scaffold the isolated Coder worker execution boundary.
- **OMN-285** — Represent NAVI self-update as a Coder task with candidate runtime
  validation (Coder applied to NAVI; not a separate self-coding subsystem).

**Net-new tickets this audit recommends creating under OMN-279:**

- **(NEW-A) Skill/workflow ID alias resolution** — add `navi-coder.* ⇆
  navi-programmer.*` (and `navi.coder ⇆ navi.programmer`) dual-resolution in the
  loader, bridge prefix guards (`bridge.go:231,239`), workflow guards
  (`:144,328-332`), CLI (`main.go:2118,2120`), and the runner. **Prerequisite for
  OMN-283.** Generalizes the bridge already shown by
  `plugins/navi-coder/handlers/coder_repo_handler.go`.
- **(NEW-B) Reconcile the precursor inventory + its guardrail test** — update
  `plugins/navi-programmer/tests/test_coder_migration_inventory.py` to stop pinning
  `"not present in this checkout"` (the canonical docs have since landed), then
  correct `docs/plans/navi-coder-migration-inventory.md`.
- **(NEW-C) Staged display-label rename** — items §4.1–§4.3, guarded by first
  updating `test_manifest_and_runner_contracts.py` display-name assertions.
- **(NEW-D) Prompt-kind staged rename** — `kind.go` `KindNCOSProgrammerWorkflow`
  + value + prompt filename + §4.4 prose, as one atomic change.
- **(NEW-E) Docs reframing pass** — items §4.5–§4.6; mark legacy forwarding
  pointers transitional and point to Coder-first docs.

---

## Appendix A — Search methodology & scope

Searched the tracked tree (gitignored paths such as `node_modules/`, build output,
and `.pytest_cache/` excluded) for all eight required terms: `NAVI Programmer`,
`navi.programmer`, `navi-programmer`, `Programmer V1`, `programmer plugin`,
`first-class programming capability plugin`, `Programmer` (whole word,
case-sensitive), and `programmer` (whole word, case-insensitive). Counts in §2 are
occurrence-based and overlapping. Runtime-identifier line numbers were verified
directly against the source files at the time of writing.

## Appendix B — Relationship to the precursor inventory

`docs/plans/navi-coder-migration-inventory.md` was the first OMN-280 pass. This
audit supersedes it in scope by: (1) grounding in the now-present canonical docs;
(2) covering the two additional standalone terms (`Programmer`, `programmer`);
(3) adding the mandated six-section structure (executive summary, classification
counts, file-by-file inventory, immediate fixes, must-not-rename list, follow-up
tickets). The precursor is retained as historical record and carries a pointer to
this audit. Its stale "canonical docs not present" claim is tracked for correction
under NEW-B (gated by its guardrail test).
