# Language-Layer Phase 1 — Sub-Agent Task Prompts

Each file here is a **self-contained implementation prompt** for one Phase 1 task. Hand a single file to
a sub-agent (or paste it into a fresh session). The prompts assume no prior conversation context.

**Direction docs (read first, every task):**
- Contract: [../../architecture/language-layer-contract.md](../../architecture/language-layer-contract.md)
- Phase 1 plan: [../language-layer-phase1-contracts.plan.md](../language-layer-phase1-contracts.plan.md)

**Sequencing:** `T1 → (T2, T3) → T4 → T5 → T6`. T2 and T3 parallelize once T1 lands.

**Status:** Phase 1 is complete — T1–T5 are merged and T6 (docs & index wiring) is done. See the
[Phase 1 plan](../language-layer-phase1-contracts.plan.md) for per-deliverable status.

| Prompt | Task | Blocked by | Status |
|---|---|---|---|
| [T1-codegen-python.prompt.md](T1-codegen-python.prompt.md) | Codegen governed types: Go → Python | — | ✅ Merged |
| [T2-codegen-typescript.prompt.md](T2-codegen-typescript.prompt.md) | Codegen governed types: Go → TypeScript | T1 | ✅ Merged |
| [T3-query-context-read-surface.prompt.md](T3-query-context-read-surface.prompt.md) | Governed read surface: `query_context` | T1 | ✅ Merged |
| [T4-navi-python-sdk.prompt.md](T4-navi-python-sdk.prompt.md) | `navi` Python SDK (reads only) | T1, T3 | ✅ Merged |
| [T5-ci-conformance-guards.prompt.md](T5-ci-conformance-guards.prompt.md) | CI conformance guards | T1–T4 | ✅ Merged |
| [T6-docs-index-wiring.prompt.md](T6-docs-index-wiring.prompt.md) | Docs & index wiring | T5 | ✅ Done |

## Next: Phase 2

Phase 1 unblocks the **validation experiment** — a Python **eval scorer** (advisory, recordable, no
execution authority) built on the generated contract and the `query_context` governed read. It is **not yet
specced**; when its plan lands, link it here. See [Phase 1 plan §8](../language-layer-phase1-contracts.plan.md)
and the [Language-Layer Contract](../../architecture/language-layer-contract.md) for the phase sequence.

## House rules (apply to every task)

- **Build artifact policy (STRICTLY ENFORCED):** never run bare `go build ./cmd/...`. Compile-check with
  `go build -o /dev/null ./cmd/navid/ && go build -o /dev/null ./cmd/navi/`; build via `make build-all`
  (lands in `bin/`). Delete any stray `navid`/`navi.exe` in the repo root.
- **Tests:** `make test` before reporting done. Write failing tests first where practical.
- **No scope creep:** do only your task. If you discover a blocker or a missing canonical type, **stop and
  report it** — do not invent governed types or approximate.
- **Honor the contract's central invariant:** Python/TS may request/advise/propose; Go approves, executes,
  records, persists. Phase 1 adds **no** effect path and **no** agent-behavior change.
- **Commit format:** `<package>: <description>`. Branch off `master`; do not commit/push unless asked.
