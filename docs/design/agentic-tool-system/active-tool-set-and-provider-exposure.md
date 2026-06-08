**Status:** Evolving  
**Last Updated:** 2026-04-15  

# Active Tool Set and Provider Exposure

## Purpose

Define how NAVI turns its internal tool architecture into the actual tool surface seen by a provider or local model.

This document exists because the model-facing surface is where most tool systems fail in practice:

- too many tools exposed
- wrong tools exposed
- stale tools remain active
- provider-specific tool protocols drift apart
- local models receive the same surface as stronger frontier models
- token overhead grows without control

The Active Tool Set is the runtime boundary that prevents those failures.

---

## Core Principle

The full registry is not the model surface.

The broker selects a small set of tools.

The Active Tool Set stores that set.

Provider exposure translates that set into provider-specific callable schemas.

Only tools in the Active Tool Set may be exposed.

Only exposed tools may be proposed by the model.

Only proposed tools that pass governance may execute.

---

## Definitions

### Active Tool Set

The Active Tool Set is the current collection of tools that are loaded and eligible for provider exposure.

It is the immediate predecessor to the provider-visible tool list.

### Provider Exposure

Provider Exposure is the act of converting the Active Tool Set into the concrete tool/function format expected by a given provider or local model runtime.

### Exposure Plan

The Exposure Plan is the provider-call-specific decision that includes:

- which tools are exposed
- which tools are suppressed
- what tool-choice mode is used
- whether parallel calls are allowed
- any provider-specific constraints

---

## Why the Active Tool Set Exists

The Active Tool Set exists to separate:

- tools NAVI knows about
- tools NAVI has loaded
- tools NAVI is willing to expose right now

Without this layer, systems collapse into one of two bad models:

1. expose the full registry
2. reconstruct the tool surface ad hoc for each provider call

The first causes confusion and overhead.

The second causes inconsistency and drift.

The Active Tool Set provides a stable runtime boundary.

---

## Active Tool Set Structure

The Active Tool Set should be an explicit runtime object.

```json
{
  "active_tool_set_id": "ats_123",
  "scope": "session",
  "session_id": "s_001",
  "workflow_id": null,
  "mode": "coder",
  "environment": "production",
  "model_profile": "local_medium",
  "tools": [
    {
      "tool_id": "repo.search",
      "schema_version": "1.0.0",
      "loaded_at": "2026-04-15T12:00:00Z",
      "expires_at": "2026-04-15T13:00:00Z",
      "load_reason": "coder_session_baseline"
    }
  ],
  "constraints": {
    "max_tools_exposed": 3,
    "max_parallel_calls": 1,
    "risk_ceiling": "medium"
  }
}
```

The Active Tool Set should be observable, versioned enough for runtime tracing, and attached to the turn/session/workflow that produced it.

---

## Active Tool Set Scopes

NAVI should support multiple scopes.

### Turn Scope

The tool set exists for one provider call or one turn only.

Use when:
- the task is narrow
- no persistence is needed
- token minimization matters

Example:
- one weather query
- one contact lookup

### Session Scope

The tool set persists across multiple turns in a coherent session.

Use when:
- the user is in a stable mode
- repeated tool reuse is likely

Example:
- coder session with repo search and file inspection tools

### Workflow Scope

The tool set persists for a workflow state machine.

Use when:
- the workflow is multi-step
- different phases need different tools

Example:
- scheduling workflow
- debugging workflow
- content drafting workflow

### Mode Baseline Scope

A minimal baseline set associated with a session mode.

Use when:
- the mode has known safe defaults

Example:
- coder mode baseline: read-only repo inspection tools
- companion mode baseline: none or discovery meta-tools only

### Provider-Call Scope

Even if a broader session tool set exists, provider exposure may narrow further for a single call.

Example:
- greeting turn in coder session still gets `tool_choice = none`

---

## Tool Set Population Rules

The broker is the only component that may populate the Active Tool Set.

Population may be triggered by:

- turn-level planning
- session-mode activation
- workflow-state transition
- user-explicit request
- model-requested load through governed meta-tools
- cache restore

Population must not happen because:

- a provider call failed and the runtime guessed more tools might help
- the model named a tool that was not active
- a tool happened to exist in the registry

---

## Tool Set Invalidation Rules

A tool or entire Active Tool Set may need to be invalidated when:

- session mode changes
- environment changes
- user authority changes
- connector becomes unhealthy
- auth expires or is revoked
- workflow completes or transitions
- policy or feature flags change
- tool repeatedly fails or is suspended
- provider/model changes to a less capable profile

Invalidation must be explicit and observable.

---

## Exposure Plan

The broker should derive an Exposure Plan from the Active Tool Set for each provider call.

```json
{
  "exposure_plan_id": "exp_001",
  "active_tool_set_id": "ats_123",
  "provider": "anthropic",
  "tool_choice_mode": "auto",
  "parallel_calls_allowed": false,
  "tools_exposed": [
    "repo.search",
    "navi.files.read"
  ],
  "tools_suppressed": [
    {
      "tool_id": "git.commit",
      "reason": "risk_tier_too_high_for_current_turn"
    }
  ]
}
```

The Exposure Plan is narrower than the Active Tool Set when needed.

---

## Exposure Filtering

Even inside the Active Tool Set, not all tools should always be exposed.

Examples:

- cached write tool suppressed during a read-only turn
- multiple similar tools narrowed to one best candidate for a weak model
- diagnostic tool loaded for the workflow but hidden during a casual explanatory turn

This means:

- loaded != exposed
- exposure is per provider call

---

## Tool Count Budgeting

The tool count exposed to the model should be tightly budgeted.

Suggested defaults:

### Local / Weak Models

- preferred: 0–2
- upper bound: 3

### Mid-tier Models

- preferred: 2–4
- upper bound: 5

### Strong Frontier Models

- preferred: 3–6
- upper bound: 8 in exceptional workflow contexts

The system should default toward fewer tools unless there is a demonstrated need to widen the set.

---

## Token Budgeting

Provider exposure must account for token cost.

Tool schemas, descriptions, and arguments all increase prompt cost.

NAVI should therefore treat tool exposure as a budgeted resource.

### Exposure cost factors

- number of tools
- schema size
- description length
- examples embedded in schema
- provider/tool protocol overhead

### Practical rules

- prefer narrow exposure
- prefer concise, precise descriptions
- suppress redundant tools
- reuse stable session sets when possible
- downgrade to no-tool for ordinary chat even in tool-heavy sessions

---

## Provider Exposure Objectives

Every provider exposure implementation must preserve these invariants:

1. only exposed tools are callable
2. provider sees canonical tool names
3. schema versions are consistent
4. provider-specific modes map cleanly from internal modes
5. result correlation is preserved across request/response loops
6. local models do not receive a sloppier or broader surface than stronger models

---

## Internal Tool Choice Modes

NAVI should use provider-neutral internal modes:

- `none`
- `auto`
- `required`
- `forced_single`
- `allowed_subset`

Provider adapters then translate these.

This prevents semantics from being baked into any one vendor's API shape.

---

## Provider Mapping

## OpenAI-style Mapping

Internal:
- `none` → no tools or explicit no-tool mode
- `auto` → normal function/tool choice
- `forced_single` → force one named tool
- `allowed_subset` → expose only selected subset

NAVI should preserve exact call IDs and correlate tool results back to the right call.

## Anthropic-style Mapping

Anthropic-style providers require careful message ordering and matching `tool_use` / `tool_result` semantics.

NAVI should:
- expose only selected tools
- preserve call/result pairing
- avoid malformed mixed-content ordering
- disable parallel tools by default for weaker models or stateful workflows

## Gemini-style Mapping

NAVI should map internal modes into Gemini-style function calling modes and allowed function subsets.

The same exposure invariants apply:
- do not widen beyond the Exposure Plan
- preserve function identity and arguments

## Local / Open-Weight Model Mapping

Many local runtimes do not support strong native tool calling.

NAVI should still use the same internal Exposure Plan and Active Tool Set.

The difference is only in adapter format.

Local adapters may:
- serialize callable tools into a smaller JSON contract
- use stricter prompting with fewer tools
- disable parallel calls
- require more explicit schemas and examples

Local models must not get separate semantics, only stricter exposure.

---

## Discovery Meta-Tools in Exposure

Some discovery tools may be part of a baseline callable surface.

Examples:
- `tool.search`
- `tool.describe`
- `tool.request_load`

These should be treated differently from ordinary action tools.

Rules:
- they are discovery-facing, not world-mutating
- they must not imply arbitrary execution
- they may be more broadly available than action tools
- they should still be governed and suppressible

---

## Exposure by Session Mode

### Companion Mode

Default exposure:
- none
- or only safe meta-tools if discovery is part of the experience

### Assistant Mode

Default exposure:
- narrow read-oriented tools
- action tools only when clearly needed

### Coder Mode

Default exposure:
- repo search
- file read
- safe diagnostics

Write tools:
- loaded/exposed only when task or workflow supports them

### Debug Mode

Default exposure:
- diagnostic tools
- still constrained by environment and authority

### Admin Mode

Default exposure:
- privileged tools possible
- but still narrow and auditable

---

## Exposure by Turn Type

Examples:

### Greeting / Smalltalk

Exposure:
- `none`

### Ambiguous Ask

Exposure:
- none or safe discovery meta-tools only

### Focused Read Task

Exposure:
- 1–3 read-only tools

### Multi-Step Workflow Turn

Exposure:
- tools only for current workflow state

### High-Risk Action Turn

Exposure:
- the specific action tool, possibly forced or confirmation-gated

---

## Suppression Reasons

Suppressed tools should record machine-readable reasons.

Examples:
- `tool_not_relevant`
- `too_many_similar_tools`
- `risk_tier_too_high`
- `requires_debug_mode`
- `environment_blocked`
- `local_model_budget_limit`
- `connector_unhealthy`
- `auth_missing`

This matters for observability and later system tuning.

---

## Stale Tool Protection

Provider exposure must guard against stale tools.

Examples:
- tool schema version changed after loading
- connector became unhealthy after load
- auth expired before execution
- workflow state changed after exposure plan creation

The runtime must revalidate before execution, but exposure should also try to avoid presenting stale tools in the first place.

---

## Parallel Tool Calls

Parallel tool calls should be treated as an explicit exposure decision, not a default.

Recommended defaults:

- local/weak models: disabled
- stateful workflows: disabled
- high-risk actions: disabled
- simple read-only independent queries: maybe allowed for stronger models

Most systems overuse parallelism before they can even trust single-step calls.

---

## Failure Handling

### Provider Requests Unknown Tool

This should not happen if exposure is correct, but if it does:
- reject
- correlate with Exposure Plan
- log as hallucinated or stale request
- perform bounded internal repair

### Provider Requests Suppressed Tool

- reject
- log mismatch
- ask broker for repair only if safe

### Exposure Plan Too Broad

Detected by:
- high selected-but-unused rate
- repeated irrelevant tool proposals
- local-model misuse spikes

Response:
- narrow future exposures
- adjust ranking and suppression policies

---

## Observability

Every provider exposure should log:

- exposure plan id
- active tool set id
- provider
- model profile
- tool count exposed
- tool count suppressed
- suppression reasons
- tool choice mode
- parallel call policy
- estimated token cost
- downstream tool usage
- selected-but-unused tools
- mismatched tool requests

---

## Quality Metrics

Track:

- average exposed tool count by mode
- average exposed tool count by provider/model
- selected-but-unused rate
- token cost of exposure
- hallucinated tool call rate after exposure
- suppressed tool request rate
- local-model error rate by exposure width
- workflow success rate by exposure width

---

## Anti-Patterns

### Anti-Pattern 1: Full Registry as Provider Surface

Wrong because it confuses the model and wastes tokens.

### Anti-Pattern 2: Provider-Specific Semantics as Internal Truth

Wrong because it locks architecture to a vendor and causes drift.

### Anti-Pattern 3: Same Exposure Width for All Models

Wrong because local models need stricter surfaces.

### Anti-Pattern 4: Session Cache Overrides Turn Needs

Wrong because a casual turn should not inherit a heavy active tool set blindly.

### Anti-Pattern 5: Loaded Tool Auto-Exposure

Wrong because loaded tools still need turn-level suppression and exposure decisions.

---

## Hard Constraints

- Only the broker may decide provider exposure.
- Only tools in the Active Tool Set may be exposed.
- Provider exposure must be narrower than or equal to the Active Tool Set.
- Internal tool choice modes must be provider-neutral.
- Local models must receive stricter or equal exposure, never broader.
- Token budget must be treated as a first-class constraint.

---

## Design Position

The Active Tool Set is the runtime boundary.

Provider exposure is the final shaping layer.

If this layer is wrong, the rest of the tool architecture does not matter because the model will still see the wrong surface.

If this layer is right, NAVI can support:

- rich discovery
- strict execution
- low token overhead
- strong local-model behavior
- clean provider abstraction
