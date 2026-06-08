# Task T2 — Codegen governed types: Go → TypeScript

> Self-contained prompt for a sub-agent. No prior conversation context is assumed.
> **Blocked by T1** (Go→Python codegen) — the generator input set and the list of governed types are
> established there. Do not start until T1's generator changes have landed.

## Who/where you are

NAVI codebase at `C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi`. Read `CLAUDE.md`. The browser
Console is a React 19 + Vite app in `web-src/navi-console/` that builds into `web/`.

## Required reading

1. `docs/architecture/language-layer-contract.md` §7 — generated vs. hand-maintained. **Hand-maintained types
   must never define governed behavior.**
2. `docs/plans/language-layer-phase1-contracts.plan.md` §3.1, §7 (T2).
3. T1's generator changes (the source-of-truth set of governed Go types and how they are emitted).
4. The TS client today: `web-src/navi-console/src/api/client.ts` (`naviFetch` validates with Zod) and the
   hand-written governed schemas it imports from `@/types/api` (e.g. `ProposalListSchema`, `Proposal` used by
   `web-src/navi-console/src/api/proposals.ts`). Locate the actual file behind the `@/types/api` alias.

## The job

Add a **TypeScript emitter** (wired into the existing `make` codegen target alongside `generate-python`,
sharing T1's input set) that produces, from the canonical Go types, TypeScript **types + Zod schemas** for the
same governed types as T1. Then **replace** the hand-written *governed* schemas in `@/types/api` with the
generated output.

Governed types: same list as T1 (`CommandType`, `ValidationOutcome`, `FailureClass`, `ReversibilityClass`,
`ExecutionOutcome`, `ActionDescriptor`, `MutationDescriptor`, Proposal/Run/Capability/Telemetry DTOs).

## Constraints

- **Only governed types are generated.** UI-only state, component props, view models, appearance, etc. stay
  hand-written. If a current `@/types/api` schema mixes governed + UI fields, split it: generated governed
  core + hand-written UI extension.
- Keep `naviFetch`'s Zod-validation pattern intact — the generated Zod schemas should drop into existing call
  sites (e.g. `proposals.ts`) with minimal churn.
- Do not change gateway routes or response shapes; you are mirroring existing contracts, not redefining them.

## Acceptance criteria

1. The codegen target emits governed TS types + Zod to a generated location consumed by `web-src/navi-console`.
2. The hand-written governed Zod in `@/types/api` is removed/replaced by generated output; UI-only types remain.
3. `web-src/navi-console` type-checks and builds (`npm run build` or the project's build script) against the
   generated types; existing tests pass (`npm test` / vitest).
4. Regeneration is idempotent (no diff on re-run).
5. `make test` (Go) still passes; compile-check clean.

## Out of scope / do not touch

- No `query_context`, no Python SDK, no CI guards.
- Do not extract a standalone `@navi/sdk` package — that is Phase 3.

## Definition of done / report back

Report: the generated TS output location; which `@/types/api` governed schemas were replaced vs. which UI-only
types you intentionally left hand-written; and confirmation the console builds + tests pass against generated
types.
