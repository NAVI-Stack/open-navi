# Unified Tool Registry

OMN-83 establishes `Tool` as a first-class NAVI entity instead of leaving it as an emergent concept spread across file tools, OSS-27 skills, plugins, selfmod helpers, and inline built-ins.

## Problem

Before this work, NAVI assembled and dispatched tools through multiple independent paths:

- file tools from `internal/navi/filetools/`
- skill interfaces from `internal/navi/skill/`
- plugin tools from `internal/navi/plugin/`
- selfmod tools from `internal/navi/selfmod/`
- inline built-ins such as `send_reply`

That fragmentation creates real drift:

- tool visibility depends on which code path assembled the current tool list
- dispatch is a growing branch cascade rather than a single registry lookup
- governance is inconsistent across sources
- naming collisions are not centrally detected

## Shipped Scope

OMN-83 is now implemented as the runtime-authoritative tool path.

What shipped:

- `internal/tool/` core types
- `internal/tool/Registry` with ordered registration, collision detection, lookup, listing, surface filtering, and execution
- startup construction of a first-class registry that includes file tools, OSS-27 skill tools, plugin tools, selfmod tools, `send_reply`, and hidden built-ins
- in-place registry refresh on skill reload so `POST /api/skills/reload` keeps the tool catalog and skill catalog synchronized without swapping registry pointers out from under active runtime code
- adapter executors for file tools, skill tools, router built-ins, onboarding, selfmod, `send_reply`, and the current runtime-executable plugin builtin subset
- registry-backed tool definition assembly for both runtime surfaces:
  - `runtime` surface for session-scoped runs
  - `loop` surface for the older conversational loop path
- registry-backed tool execution in both `runtime_executor.go` and `loop.go`
- unified cross-source governance validation for registry-backed tools in both execution paths
- operator introspection endpoints:
  - `GET /api/tools`
  - `GET /api/tools/{name}`
- hidden built-ins stay dispatchable but are not exposed as LLM-callable definitions
- loop/runtime execution bootstrap the registry if it is missing and keep definition assembly and execution on the registry path

## Core Entity

```go
type Tool struct {
    Name       string
    Source     ToolSource
    Hidden     bool
    VisibleOn  []string
    Definition llm.ToolDefinition
    Executor   ToolExecutor
    Governance ToolGovernance
    Metadata   ToolMetadata
}
```

Supporting concepts:

- `ToolSource`: `file_tools`, `skill`, `plugin`, `selfmod`, `builtin`
- `Hidden`: registry entry exists for lookup/dispatch but is excluded from the LLM-visible definition set
- `VisibleOn`: optional surface filter such as `runtime` or `loop`
- `ToolExecutor`: future unified execution adapter interface
- `ToolGovernance`: command type, risk tier, confirmation, domain, path restriction
- `ToolMetadata`: provenance and grouping hints such as skill/interface/plugin identity

## Registry Contract

The first-class registry provides:

- `Register(tool)` with duplicate-name rejection
- `Unregister(name)`
- `Lookup(name)`
- `List()` in registration order, including hidden tools
- `Definitions()` in registration order
- `DefinitionsFor(surface)` for surface-aware LLM exposure
- `Execute(name, args)` for future unified dispatch
- `ListBySource(...)`
- `ListByDomain(...)`

The important property is ordered determinism: the registry preserves tool registration order so LLM-facing definitions stay stable across surfaces and reloads.

## Current Behavior

The startup-built registry now acts as the authoritative first-class catalog for runtime execution. It includes:

1. file tools when a workspace is configured
2. loaded OSS-27 skill tools
3. plugin registry tools
4. selfmod tools
5. runtime-only `send_reply`
6. hidden built-ins for router and onboarding dispatch

The visible definition set comes directly from the registry and is the only LLM-facing tool assembly path used by NAVI.

## Dispatch Model

Every registry entry carries its execution adapter and governance metadata. The current dispatch model is:

- file tools wrap `filetools.Execute(...)`
- skill tools wrap `skill.Execute(...)`
- plugin tools wrap the currently runtime-executable builtin plugin subset
- selfmod tools wrap `selfmod.Execute(...)`
- `send_reply` wraps the runtime scheduled-message queue through context-bound execution state
- router and onboarding built-ins are registered as hidden built-in entries so dispatch can stay unified without expanding the LLM-visible tool list

Both runtime execution paths now:

- assemble tool definitions from `ToolRegistry.DefinitionsFor(surface)`
- classify registry entries by first-class `ToolSource`
- validate registry-backed tools through the same governance helper
- execute registry-backed tools through `ToolRegistry.Execute(...)`
- keep the existing command-outcome recording and error-envelope wrappers around that execution

## Operator Surface

The gateway now exposes the same first-class catalog used by NAVI:

- `GET /api/tools` returns the full registered catalog, including hidden tools by default
- `GET /api/tools?surface=runtime|loop` filters by surface visibility
- `GET /api/tools?include_hidden=false` filters hidden tools out of the response
- `GET /api/tools/{name}` returns a single first-class tool record
- `POST /api/skills/reload` now refreshes both the skill registry and the first-class tool registry together when NAVI is configured on the server

## Remaining Follow-On Ideas

OMN-83 is now the runtime baseline. Follow-on work can build on the first-class entity instead of inventing new parallel paths:

- richer metadata and backreferences for skills/plugins
- deeper operator UX on top of `/api/tools`
