**Status:** Ratified By ADR  
**Last Updated:** 2026-04-07  
**Updated By:** Codex

# NCOS Phase 2 Capability-Surface Contract

This document is the implementation contract drafted for `OMN-184`.

The settled policy is now ratified in [ADR-008](../adr/ADR-008-ncos-capability-surface-policy.md).

It defines the terminology, invariants, and forbidden behaviors for Phase 2 before resolver work begins.

It does not implement the resolver, alter runtime behavior, or redesign the unified `ToolRegistry`.

## Scope

Phase 2 capability-surface resolution decides which already-registered tools are exposed to the model for a specific turn or run.

The resolver operates over the existing registry. It does not replace registry ownership, registration order, dispatch, governance, plugins, or skill transport.

## Terminology

- `surfaced`: a registered tool included in the final resolved capability surface and therefore eligible for model exposure
- `hidden`: a tool with `Tool.Hidden=true`; hidden tools are never surfaced on normal user-facing surfaces
- `internal`: a registered tool marked for internal/system use only; internal tools are excluded from normal `loop` and `runtime` surfaces
- `dev/test`: a registered tool marked as development-only or test-only; these tools are excluded from normal user-facing surfaces
- `runtime surface`: the user-facing action-capable surface used for `ExecuteRun`
- `loop surface`: the user-facing conversational surface used for `ProcessTurn`
- `empty surface`: a valid surface request that resolves to zero surfaced tools after policy is applied
- `invalid surface`: a malformed or policy-invalid surface request, including unknown surface names and explicit requests that reference unknown or disallowed tools
- `guarded surface`: a resolution outcome that fails closed and prevents normal tool exposure from proceeding

## Code Contract

The typed contract lives in [capability_surface.go](../../internal/navi/orchestration/capability_surface.go).

The settled Phase 2 contract includes:

- `CapabilitySurface`
- `SurfaceResolutionInput`
- `SurfaceResolutionResult`
- `SurfaceExclusionReason`
- `SurfaceGuardOutcome`
- `CapabilityExposureClass`
- `CapabilityInteractionMode`

## Required Metadata Inputs

Phase 2 resolver decisions use these fields as authoritative inputs:

- Existing registry fields:
  - `Tool.Hidden`
  - `Tool.VisibleOn`
  - `Tool.Source`
- Existing governance/context hints that may assist policy but do not replace exposure policy:
  - `Tool.Governance.CommandType`
  - `Tool.Governance.Domain`
  - `Tool.Metadata.Tags`
- Required metadata additions for `OMN-185`:
  - `ExposureClass`
    - values: `user_facing`, `internal`, `development`, `test`
  - `InteractionModes`
    - values: `conversation`, `action`
  - `RequiresToolCapableModel`
    - boolean capability guard hint for surfaced tools

`Hidden` remains a hard non-exposure flag and must not be re-expressed as a second metadata field.

## Invariants

- `ToolRegistry` remains the source of truth for registered tools.
- Capability resolution happens before model invocation.
- Resolution may narrow exposure only; it may not widen beyond the requested surface plus registry-backed policy.
- `loop` and `runtime` are the only settled normal user-facing surfaces in this phase.
- Empty or invalid surfaces fail closed.
- Routing may strip tools or trigger a guard, but may not widen beyond the resolved surface.
- `ExecuteRun` and `ProcessTurn` must use the same surface-resolution contract and the same guard semantics.
- Hidden, internal, development, and test-only tools must not leak into normal user-facing surfaces.
- Registration order remains the stable ordering source for surfaced tools unless a future ticket explicitly changes that rule.

## Forbidden Behaviors

- silent widening fallback when the resolved surface is empty
- local inline filtering in runtime files that bypasses the authoritative resolver
- separate loop-specific and run-specific exposure policies
- exposing hidden/internal/dev/test tools on normal user-facing surfaces
- treating registry tags or source alone as sufficient to bypass explicit exposure policy
- introducing a second exposure catalog parallel to `ToolRegistry`
- converting invalid surface requests into a broader "best effort" surface

## Fail-Closed Policy

- `invalid_surface`
  - used when the surface request itself is malformed or policy-invalid
  - runtime must not widen or substitute a broader surface
- `empty_surface`
  - used when resolution completes successfully but yields zero surfaced tools
  - runtime must not widen; later tickets define whether the turn proceeds toolless or returns an explicit guard reply
- `tool_capable_model_required`
  - used when routing/model constraints prevent safe tool exposure for a tool-required turn
  - runtime must return an explicit guard outcome, not fake success

## Routing Interaction Policy

- Surface resolution happens before model invocation.
- Routing consumes the already-resolved surface.
- Routing may:
  - keep the resolved surface unchanged
  - strip tools from the resolved surface
  - trigger a guard outcome
- Routing may not:
  - add tools that were not in the resolved surface
  - reintroduce hidden/internal/dev/test tools
  - bypass fail-closed outcomes

## Path Parity Rule

For the same logical request class and same surface inputs, `ExecuteRun` and `ProcessTurn` must produce the same exposure decision class:

- same resolved-surface name
- same surfaced tool subset class
- same guard outcome class when guarded

The two paths may differ in surrounding runtime behavior, but not in capability-surface policy.

## Implementation Notes For Downstream Tickets

- `OMN-185`
  - add the minimal metadata fields needed by this contract
  - do not redesign registry structure or dispatch
- `OMN-186`
  - implement one authoritative resolver over the unified registry
  - emit per-tool exclusions with explicit reasons
  - preserve registry order for surfaced tools
- `OMN-187`
  - enforce `invalid_surface` and `empty_surface` as fail-closed outcomes
  - no widening fallback
- `OMN-188`
  - trace requested surface, resolved surface, exclusions, and guard outcomes
- `OMN-189` and `OMN-190`
  - integrate the same resolver into both runtime paths
  - do not introduce local filtering shortcuts
- `OMN-191`
  - formalize the routing interaction rules already fixed here
- `OMN-197`
  - ratify this contract into the final Phase 2 ADR
