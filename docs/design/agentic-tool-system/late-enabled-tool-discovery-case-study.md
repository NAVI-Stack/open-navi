**Status:** Evolving  
**Last Updated:** 2026-04-16  

# Late-Enabled Tool Discovery Case Study

## Purpose

Capture a concrete failure mode where a capability exists or becomes enabled, but NAVI's currently active/model-visible tool surface does not include it.

This case study strengthens the Agentic Tool System requirement that NAVI must not infer global tool availability only from the current exposed callable set.

## Case Study

A user asked the assistant to create Linear tickets.

Initial behavior:

1. Linear capability existed or became available in the wider tool environment.
2. The active/model-visible tool surface did not include Linear.
3. The assistant concluded it could not write to Linear.
4. The user manually re-exposed or re-enabled Linear.
5. The assistant then successfully created the Linear project and issues.

This is a tool-surface freshness problem, not a reasoning-only problem.

## Failure Mode

### Name

Stale or incomplete active tool surface.

### Definition

A tool exists, is installed, or becomes enabled, but the current active callable surface does not reflect that capability. The agent falsely reports that the capability is unavailable because it only checks currently exposed tools.

### Root Cause

Tool availability was inferred from the active/model-visible tool surface instead of querying Tool Discovery/Registry.

### Bad Behavior

```text
User requests capability X.
X is not in current active tool set.
Assistant says X is unavailable.
User manually re-exposes X.
Assistant can now use X.
```

### Desired Behavior

```text
User requests capability X.
X is not in current active tool set.
Broker queries Tool Discovery/Registry.
Discovery reports exact state of X.
Broker loads X if allowed.
Assistant uses X or explains the precise constraint.
```

## Required Availability States

The system must distinguish:

- not registered
- registered but not discoverable
- discoverable but not loadable
- loadable but not loaded
- loaded but not exposed
- exposed but governance-blocked

These states are not interchangeable.

## Design Requirement

Before NAVI returns a hard unavailable response for a requested capability, the broker must query Tool Discovery/Registry unless policy explicitly forbids discovery.

The current Active Tool Set is not a complete statement of system capability.

## Broker Requirement

When a user request strongly implies a capability that is absent from the current Active Tool Set, the broker must perform a surface refresh flow:

1. exact registry/index lookup for the implied capability
2. related semantic discovery if exact lookup fails
3. availability-state classification
4. load request if the tool is loadable
5. controlled refusal only after discovery confirms unavailability or policy block

## Discovery Requirement

Discovery must support late-enabled and newly available capabilities.

Registry/index changes should invalidate cached unavailable/no-tool decisions where relevant.

## Cache Invalidation Requirement

Cached decisions must be invalidated when:

- a tool is registered
- a connector becomes enabled
- auth becomes available
- environment/mode/authority changes
- registry snapshot changes
- index refresh reports new matching candidates

## User Experience Requirement

The user should not need to manually reintroduce a tool if NAVI can discover and load it safely.

If NAVI cannot load it, the user-facing response should explain the exact constraint:

- not installed
- not enabled
- not authorized
- wrong mode
- blocked by environment
- blocked by governance

## Anti-Patterns

### Anti-Pattern 1: Active Surface as Global Truth

Wrong:

```text
Tool is not currently exposed, therefore tool does not exist.
```

### Anti-Pattern 2: User Prompt Surgery

Wrong:

```text
User must manually re-expose or remind the model of a tool before NAVI can use it.
```

### Anti-Pattern 3: Sticky Unavailable Cache

Wrong:

```text
A prior unavailable result survives after the connector/tool becomes enabled.
```

## Acceptance Implication

A correct implementation should pass a test equivalent to:

1. Start with Linear unavailable from active surface.
2. User requests Linear ticket creation.
3. Broker queries discovery before refusing.
4. Linear becomes available/loadable.
5. Broker refreshes and loads Linear.
6. NAVI creates tickets without requiring user-side prompt surgery.

## Related Linear Ticket

- OMN-258 — ATS-11: Add tool surface refresh and late-enabled tool discovery

## Related Design Docs

- `tool-discovery-and-search.md`
- `tool-broker-design.md`
- `active-tool-set-and-provider-exposure.md`
- `tool-lifecycle-and-state-model.md`
