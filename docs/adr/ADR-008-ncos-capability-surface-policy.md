# ADR-008: NCOS Capability-Surface Policy

## Context
NCOS Phase 2 adds runtime capability-surface resolution over the unified `ToolRegistry` foundation established by `OMN-83` and `OMN-84`. NAVI already has one registry-backed source of truth for registered tools; what Phase 2 needed was a single authoritative policy for deciding which registered tools are exposed on a given turn or run.

Before this ADR, the project had a Phase 2 contract draft in `docs/design/`, but future contributors would still have to infer final behavior from implementation details spread across resolver code, routing code, and runtime tests.

## Decision
NAVI adopts the following Phase 2 capability-surface policy as normative behavior.

### 1. ToolRegistry remains the source of truth
- Phase 2 builds on the unified `ToolRegistry`; it does not replace it.
- Capability resolution operates over already-registered tools only.
- Phase 2 does not redesign registry ownership, dispatch, governance, plugins, or skill transport.

### 2. One capability-surface contract and one resolver
- The authoritative typed contract is defined in `internal/navi/orchestration/capability_surface.go`.
- The authoritative resolver is `CapabilitySurfaceResolver`.
- Resolution happens before model invocation.
- Resolution is deterministic and preserves registry order for surfaced tools unless a future decision explicitly changes that rule.

### 3. User-facing surfaces are explicit
- The settled normal user-facing surfaces for this phase are:
  - `loop`
  - `runtime`
- `ProcessTurn` uses the `loop` surface.
- `ExecuteRun` uses the `runtime` surface.
- Hidden, internal, development, and test-only tools are excluded from normal user-facing surfaces.

### 4. Metadata assumptions are explicit
Resolver decisions use these registry-backed inputs:
- `Tool.Hidden`
- `Tool.VisibleOn`
- `Tool.Metadata.ExposureClass`
- `Tool.Metadata.InteractionModes`
- `Tool.Metadata.RequiresToolCapableModel`

These fields narrow exposure; they do not create a second tool model.

### 5. Resolution may narrow only
- Capability resolution may preserve or narrow exposure.
- It may not widen beyond the requested surface and registry-backed policy.
- Explicit requested subsets remain bounded by the same registry policy and may not bypass hidden/internal/dev/test filtering.

### 6. Fail-closed guard model is mandatory
Phase 2 uses explicit fail-closed outcomes:
- `invalid_surface`
  - malformed or policy-invalid surface request
- `empty_surface`
  - valid request that resolves to zero surfaced tools when tools were expected
- `tool_capable_model_required`
  - a tool-required request cannot safely proceed on the selected/routed model

These outcomes must not trigger silent widening fallback.

### 7. Routing interaction is narrow and non-widening
Routing consumes the already-resolved capability surface.

Routing may:
- preserve the resolved surface
- strip tools from the resolved surface
- trigger an explicit guard outcome

Routing may not:
- add tools that were not already in the resolved surface
- reintroduce hidden/internal/dev/test tools
- bypass fail-closed outcomes

### 8. Path parity is required
For equivalent scenarios, `ExecuteRun` and `ProcessTurn` must converge on the same capability-surface behavior class:
- same surfaced tool subset class
- same strip behavior when routing strips tools
- same guard behavior when routing guards execution

The surrounding runtime flow may differ, but the exposure policy may not.

### 9. Forbidden behaviors
The following patterns are explicitly forbidden:
- silent widening fallback
- local inline exposure filtering outside the resolver path
- separate loop-specific and run-specific exposure policies
- parallel tool catalogs or second tool models
- routing-specific secret tool exposure logic
- best-effort conversion of invalid requests into broader surfaces

## Rationale
- **Architectural clarity:** contributors have one place to read the settled policy instead of reconstructing it from runtime files.
- **Safety:** fail-closed behavior prevents hidden/internal/dev/test tool leakage and blocks "just this once" widening.
- **Reviewability:** explicit resolver and routing policy seams are easier to audit than scattered runtime branches.
- **Parity:** shared policy prevents `ExecuteRun` and `ProcessTurn` from drifting over time.

## Status
**Proposed: 2026-04-07**  
**Ratified: 2026-04-07**

## Consequences
- Future capability-surface work must extend the resolver, guard policy, or routing/surface policy directly rather than adding runtime-local filtering.
- New registry-backed tools that should stay out of normal user-facing surfaces must express that through the settled metadata/visibility inputs.
- Future contributors should treat `docs/design/ncos-capability-surface-contract.md` as the implementation-phase draft that led to this ADR, and treat this ADR as the authoritative Phase 2 policy record.
