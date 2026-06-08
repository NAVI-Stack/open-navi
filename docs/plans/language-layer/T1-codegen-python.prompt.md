# Task T1 — Codegen governed types: Go → Python

> Self-contained prompt for a sub-agent. No prior conversation context is assumed.

## Who/where you are

You are implementing in the NAVI codebase at
`C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi` (Go 1.24+, module `github.com/ceoai/navi`).
NAVI is a two-binary personal AI system: `navid` (server) and `navi` (CLI). Read `CLAUDE.md` first.

## Required reading before you touch code

1. `docs/architecture/language-layer-contract.md` — the boundary contract. §7 (generated vs. hand-maintained)
   is your spec; §2.1 lists the kernel packages.
2. `docs/plans/language-layer-phase1-contracts.plan.md` — §3.1 and §7 (T1) are your scope.
3. `internal/schema/command.go` — already defines `CommandType`, `FailureClass`, `ReversibilityClass`,
   `ComposeFailureMode`, `IdempotencyExpectation`, `DegradationVisibility`.
4. How Python is generated today: inspect the `generate-python` target in the `Makefile` and the existing
   output under `schema/python/`. **Extend the existing generator — do not build a parallel one.**

## The job

Make `make generate-python` additionally emit the **governed types** to `schema/python/`, sourced from the
Go canonical types, as **stdlib-only** Python models (`dataclass`/`TypedDict` — no third-party deps, matching
the stdlib-only posture of `python/intake/worker.py`).

Governed types to emit:

```text
CommandType            ValidationOutcome      FailureClass
ReversibilityClass     ExecutionOutcome       ActionDescriptor
MutationDescriptor     Proposal DTOs          Run DTOs
Capability DTOs        Telemetry/event DTOs
```

## Known wrinkle you must resolve (do not invent)

The generator's source today is `internal/schema/`, but some governed types live elsewhere:
- `ValidationOutcome`, `ActionDescriptor` live in `internal/governor/` (see `validate.go`).
- `MutationDescriptor` is described as *illustrative* in `docs/design/intake-synthesis-seam.md` §5 and **may
  not yet exist as a concrete Go type.**

For each type: locate the canonical Go definition. If it lives outside `internal/schema/`, extend the
generator's input set to cover that package (do **not** duplicate/redefine the type in `internal/schema`).
**If a governed type does not yet exist in Go (e.g. `MutationDescriptor`), STOP and report it** in your
final summary as a scoping gap — do not fabricate the shape.

## Acceptance criteria

1. `make generate-python` emits the located governed types to `schema/python/`; **regeneration is
   idempotent** (running twice produces no diff).
2. Generated files are clearly marked generated (header comment) and stdlib-only.
3. A **Go↔Python round-trip test** proves `ExecutionOutcome` and `ValidationOutcome` serialize Go→JSON→Python
   and back with no field loss (a small Go test that emits a fixture + a Python check, or a golden-file test —
   match the testing style already used around `schema/python/`).
4. `make test` passes; compile-check clean (`go build -o /dev/null ./cmd/navid/ && go build -o /dev/null ./cmd/navi/`).

## Out of scope / do not touch

- No TypeScript (that is T2). No `query_context` (T3). No Python SDK (T4).
- Do not hand-write governed models; they must be generated.
- Do not alter governance/runtime semantics.

## Definition of done / report back

Report: which types were generated and from which packages; how you handled the governor-located types; any
governed type that does not yet exist in Go (scoping gap); and the round-trip test location. This task is the
**foundation** — T2–T6 depend on it.
