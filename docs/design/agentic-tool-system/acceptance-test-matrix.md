**Status:** Active
**Last Updated:** 2026-04-17

# ATS Acceptance Test Matrix

This matrix tracks the implemented ATS acceptance surface for OMN-248 through OMN-272.

Status meanings:

- `covered` means implementation and automated tests exist in the current repo slice.
- `partial` means the core behavior exists, but an explicitly named hardening seam or runtime integration is still not fully wired.
- `deferred` means the design intent is documented, but the isolated acceptance seam is intentionally left to a later test suite.

## ATS Requirements

| ATS / OMN | Scope | Implementation Files | Test Files | Status | Notes / Gaps |
|----------|-------|----------------------|------------|--------|--------------|
| ATS-1 / OMN-248 | Canonical Tool entity and validation contract | `internal/tool/types.go`, `internal/tool/validation.go` | `internal/tool/registry_test.go`, `internal/tool/validation_test.go`, `internal/navi/ats_acceptance_test.go` | covered | Canonical `tool_id`, validation, duplicate rejection, and fail-closed metadata checks are enforced in registry registration. |
| ATS-2 / OMN-249 | Unified Tool Registry source of truth | `internal/tool/registry.go` | `internal/tool/registry_test.go`, `internal/navi/ats_acceptance_test.go` | covered | Registry snapshots, exact lookup, collision rejection, and hidden-tool non-exposure are covered. |
| ATS-3 / OMN-250 | Source adapters register current tool sources | `internal/navi/tool_registry.go`, `internal/navi/tool_registry_adapters.go`, `internal/tool/adapters.go` | `internal/navi/tool_registry_test.go`, `internal/tool/adapters_test.go` | covered | Builtins, file tools, skills, plugins, and selfmod register through the unified runtime registry path. |
| ATS-4 / OMN-251 | Tool Index exact, lexical, and tag search | `internal/tool/index.go` | `internal/tool/index_test.go`, `internal/navi/ats_acceptance_test.go` | covered | Exact miss versus related match remains explicit and index authority stays derived from the registry snapshot. |
| ATS-5 / OMN-252 | Discovery result contract | `internal/tool/index.go`, `internal/tool/recovery.go` | `internal/tool/index_test.go`, `internal/tool/recovery_test.go` | covered | Exact hit, exact miss, related match, hidden/unavailable signaling, and missing-capability output are exercised. |
| ATS-6 / OMN-253 | Discovery policy filtering | `internal/tool/discovery_policy.go`, `internal/tool/metadata.go` | `internal/tool/discovery_policy_test.go`, `internal/tool/metadata_test.go`, `internal/tool/broker_test.go` | covered | Production/user filtering remains fail-closed; debug/admin visibility can surface unavailable tools with explicit reason codes. |
| ATS-7 / OMN-254 | `ToolBroker.Resolve` no-tool default | `internal/tool/broker.go` | `internal/tool/broker_test.go`, `internal/navi/ats_acceptance_test.go` | covered | Smalltalk and ambiguous turns default to `none`; broker remains selection-only and never executes tools. |
| ATS-8 / OMN-255 | Ranking and top-K broker selection | `internal/tool/broker.go` | `internal/tool/broker_test.go` | covered | Local/weak versus strong model limits, overlap suppression, workflow relevance, and trace output are covered. |
| ATS-9 / OMN-256 | Unknown and unloaded tool-call recovery | `internal/tool/recovery.go`, `internal/navi/inference/model_authorization.go` | `internal/tool/recovery_test.go`, `internal/navi/inference/model_authorization_test.go` | covered | Out-of-surface tool calls fail closed and record bounded recovery context instead of surfacing raw internal runtime errors. |
| ATS-10 / OMN-257 | Active Tool Set runtime object | `internal/tool/active_set.go`, `internal/navi/inference/model_authorization.go` | `internal/tool/active_set_test.go`, `internal/navi/inference/model_authorization_test.go` | covered | Loaded membership, schema-version matching, expiry, dropped selections, and loaded-not-exposed / loaded-not-executable invariants are covered. |
| ATS-11 / OMN-258 | Late-enabled tool discovery and surface refresh | `internal/tool/broker_refresh.go`, `internal/tool/broker.go`, `internal/navi/tool_registry_adapters.go` | `internal/tool/broker_refresh_test.go`, `internal/tool/broker_test.go`, `internal/navi/ats_acceptance_test.go`, `internal/navi/tool_registry_test.go` | partial | Broker refresh and adapter-backed connector dependency selection are covered. Live connector-health propagation into `AvailableConnectors` is still an upstream runtime wiring concern, so this stays partial instead of over-claiming completion. |
| ATS-12 / OMN-259 | Tool degradation detection and fallback routing | `internal/tool/fallback.go`, `internal/navi/runtime_executor.go` | `internal/navi/runtime_executor_test.go`, `internal/navi/ats_acceptance_test.go` | covered | Degraded-tool classification, fallback discovery, governed fallback execution, and degradation observability are covered through the runtime path. |
| ATS-12a / OMN-260 | Failure taxonomy | `internal/tool/errors.go` | `internal/tool/errors_test.go`, `internal/navi/runtime_executor_test.go` | covered | Recoverable versus non-recoverable classes remain explicit and feed fallback eligibility. |
| ATS-12b / OMN-261 | Fallback candidate discovery | `internal/tool/fallback.go` | `internal/tool/fallback_test.go`, `internal/navi/runtime_executor_test.go` | covered | Candidate discovery preserves intent and does not widen past risk/governance boundaries. |
| ATS-12c / OMN-262 | Governed fallback execution path | `internal/navi/runtime_executor.go` | `internal/navi/runtime_executor_test.go` | covered | Fallback runs through the same governed execution path and does not create a bypass. |
| ATS-12d / OMN-263 | Degradation/fallback regressions | `internal/navi/runtime_executor.go`, `internal/tool/fallback.go` | `internal/navi/runtime_executor_test.go`, `internal/navi/ats_acceptance_test.go` | covered | Regression coverage exists for successful fallback, blocked fallback, and provenance retention. |
| ATS-13 / OMN-264 | Runtime fallback context | `internal/navi/runtime_executor.go` | `internal/navi/runtime_executor_test.go`, `internal/navi/ats_acceptance_test.go` | covered | Fallback discovery now derives runtime environment, mode, and authority instead of widening to owner/development defaults. |
| ATS-14 / OMN-272 | ATS observability and sanitized failure-output regressions | `internal/navi/runtime_executor.go`, `internal/tool/failure.go` | `internal/navi/runtime_executor_failure_test.go`, `internal/tool/failure_test.go` | covered | Failure taxonomy and observability remain preserved in runtime snapshots, and focused regressions now prove non-debug user-facing summaries stay sanitized across connector, auth, schema, contract-drift, timeout, action-missing, execution-failed, and partial-failure paths. |

## Hard Invariants

| Invariant | Implementation Files | Test Files | Status | Notes / Gaps |
|-----------|----------------------|------------|--------|--------------|
| Registry is not exposure | `internal/tool/registry.go`, `internal/navi/orchestration/capability_surface_resolver.go` | `internal/tool/registry_test.go`, `internal/navi/orchestration/capability_surface_resolver_test.go` | covered | Hidden/internal/dev/test tools can stay registered while remaining absent from the provider surface. |
| Discovery is not execution | `internal/tool/index.go`, `internal/tool/broker.go`, `internal/navi/runtime_executor.go` | `internal/tool/index_test.go`, `internal/tool/broker_test.go`, `internal/navi/ats_acceptance_test.go` | covered | Index and broker return structured selection state only; execution stays in the governed runtime path. |
| Loaded is not exposed | `internal/tool/active_set.go`, `internal/navi/inference/model_authorization.go` | `internal/navi/inference/model_authorization_test.go` | covered | Loaded active-set membership does not auto-surface additional provider-call tools. |
| Exposed is not executable | `internal/navi/inference/model_authorization.go`, `internal/navi/runtime_executor.go` | `internal/navi/inference/model_authorization_test.go`, `internal/navi/runtime_executor_test.go` | covered | Provider-visible tool calls still require authoritative contracts plus governance before execution. |
| Current active surface is not global capability truth | `internal/tool/broker_refresh.go`, `internal/tool/recovery.go` | `internal/tool/broker_refresh_test.go`, `internal/tool/recovery_test.go`, `internal/navi/ats_acceptance_test.go` | covered | Out-of-surface requests still consult registry/discovery before concluding a capability is unavailable. |
| Fallback is governed | `internal/tool/fallback.go`, `internal/navi/runtime_executor.go` | `internal/navi/runtime_executor_test.go`, `internal/navi/ats_acceptance_test.go` | covered | Fallback discovery and execution preserve the single governed path and fail closed on policy/governance blocks. |
| Production defaults fail closed | `internal/tool/discovery_policy.go`, `internal/tool/broker.go`, `internal/navi/inference/model_authorization.go` | `internal/tool/discovery_policy_test.go`, `internal/tool/broker_test.go`, `internal/navi/inference/model_authorization_test.go`, `internal/navi/runtime_executor_test.go` | covered | Omitted context, stale active sets, and unauthorized tool calls all block instead of widening to owner/development behavior. |

## Provider Exposure / Protocol Coverage

| Area | Implementation Files | Test Files | Status | Notes / Gaps |
|------|----------------------|------------|--------|--------------|
| Provider-facing compiled tool surface | `internal/navi/orchestration/model/request.go`, `internal/navi/orchestration/capability_surface_resolver.go` | `internal/navi/orchestration/model/adapter_test.go`, `internal/navi/orchestration/capability_surface_resolver_test.go` | covered | Compiled requests only include the requested/approved subset, and hidden/internal/dev/test tools are excluded before provider payload construction. |
| Provider-specific wire serialization | `plugins/llm-openai/providers/openai`, `plugins/llm-anthropic/providers/anthropic`, `plugins/llm-ollama/providers/ollama` | provider package tests under `plugins/llm-*` | partial | Request-body tests cover empty-tool omission and tool-call serialization. Forced/subset tool protocol coverage is extended in this slice for OpenAI; a full provider-by-provider serialization matrix remains a follow-on hardening area. |
