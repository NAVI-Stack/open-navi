**Status:** Evolving  
**Last Updated:** 2026-04-15  

# Tool Broker Design

## Purpose

The Tool Broker is the control plane of the tool system.

It mediates between:

- full tool registry and index
- policy and environment constraints
- session and workflow state
- provider-specific callable tool exposure
- execution runtime

Its job is not to execute tools. Its job is to decide what tools are relevant, allowed, loaded, and exposed.

## Core Principle

The model must never see the full tool universe directly.

The broker determines the active callable surface for the current turn, session, workflow, and provider call.

## Why the Broker Exists

Without a broker, naive systems fail in predictable ways:

- too many tools are exposed
- the model picks irrelevant tools
- the model hallucinates plausible tool names
- unloaded or unavailable tools are requested directly
- dev/test tools leak into production contexts
- execution surfaces drift by code path

The broker exists to prevent those failures before execution is even considered.

## Responsibilities

The Tool Broker is responsible for:

1. Interpreting tool need from intent, mode, and state.
2. Querying the Discovery/Search subsystem.
3. Filtering discovery results through policy and environment constraints.
4. Ranking candidate tools.
5. Selecting a small candidate set.
6. Loading or reusing tools in the Active Tool Set.
7. Producing provider-facing callable tool schemas.
8. Choosing provider-specific tool modes (`none`, `auto`, forced single tool, allowed subset).
9. Rejecting or recovering from out-of-set tool requests.
10. Emitting traces for all of the above.

## Non-Responsibilities

The broker does not execute tools.

The broker does not bypass governance.

The broker does not synthesize or create new tools at runtime just because a model asked for one.

The broker does not let discovery results become execution authority automatically.

## Position in the Architecture

The broker sits between Discovery and Execution.

```text
Registry -> Index/Discovery -> Broker -> Active Tool Set -> Provider Adapter -> Model
                                                    -> Execution Runtime -> Governor -> Executor
```

The broker is therefore the only component allowed to transform broad capability awareness into a narrow callable surface.

## Inputs

The broker should operate on structured input.

```json
{
  "user_input": "Can you check why the last Telegram message duplicated thinking bubbles?",
  "intent": "diagnostic_request",
  "session_mode": "companion",
  "environment": "production",
  "user_authority": "owner",
  "workflow_state": null,
  "model_profile": "local_weak",
  "session_id": "s_123",
  "active_tool_set_id": "ats_001",
  "allow_cached_tools": true
}
```

### Required Inputs

- user input
- interpreted intent (or null if unresolved)
- session mode
- environment
- user authority
- model profile

### Optional Inputs

- active workflow state
- prior discovery results
- prior successful tool use
- session cache
- cost or budget constraints
- feature flags

## Outputs

The broker must return an explicit plan.

```json
{
  "active_tool_set_id": "ats_002",
  "tool_choice_mode": "none",
  "selected_tools": [],
  "suppressed_tools": [
    {
      "tool_id": "navi.diagnostics.session_errors",
      "reason": "requires_debug_mode"
    }
  ],
  "broker_reason": "diagnostic tools unavailable in current companion/production context"
}
```

For tool-using turns:

```json
{
  "active_tool_set_id": "ats_003",
  "tool_choice_mode": "auto",
  "selected_tools": [
    "navi.files.read",
    "repo.search",
    "git.diff"
  ],
  "broker_reason": "coder diagnostics baseline"
}
```

## Broker Pipeline

## Step 1: Tool Need Assessment

The broker first decides whether tools are needed at all.

Example:

- input: `"yo"`
- intent: `smalltalk`
- result: no tools

This is the single cheapest and most important filter.

If the request is ordinary conversation, acknowledgment, reflection, or emotional support, the broker should strongly prefer `tool_choice = none`.

## Step 2: Discovery Query

If tools may be needed, the broker queries Discovery.

Discovery query should include:

- normalized user request
- interpreted intent
- domains
- mode
- environment
- user authority
- active workflow
- model profile

The broker may run one broad query or several narrow ones depending on the turn.

## Step 3: Policy Filtering

The broker filters out tools that are:

- not discoverable in the current context
- not loadable in the current context
- blocked by environment partitioning
- above the allowed risk ceiling
- unavailable because of auth, connector, or health state

Policy filtering is not optional.

## Step 4: Ranking

Remaining tools are ranked by signals such as:

### Positive signals

- exact capability match
- semantic similarity
- domain match
- workflow-state relevance
- session history
- low-risk fit
- connector availability

### Negative signals

- risk mismatch
- wrong mode
- wrong environment
- stale or deprecated tool
- prior false-positive history
- schema complexity for weak models

### Hard exclusions

- test-only in production
- authority mismatch
- untrusted source
- removed tool

## Step 5: Selection

The broker selects a small top-K set.

Suggested defaults:

- no-tool turns: 0
- casual assistant turns: 0–2
- local/weak models: 1–3
- stronger models: 3–6
- workflow state machines: only the tools needed for the current state

The broker should prefer a smaller active set unless there is a strong reason to widen it.

## Step 6: Loading

The broker ensures selected tools are loaded into the Active Tool Set.

Loading may involve:

- validating tool health
- resolving connector dependencies
- binding runtime context
- restoring cached active tools
- ensuring schema version consistency

Loading is still not execution.

## Step 7: Exposure Plan

The broker translates the Active Tool Set into provider-facing exposure.

This includes:

- exact tool schemas
- allowed subset
- whether tools are disabled entirely
- whether only one forced tool is allowed
- whether parallel calls are disabled

## Tool Choice Modes

The broker should support at least these internal modes:

- `none`
- `auto`
- `required`
- `forced_single`
- `allowed_subset`

These are then mapped into provider-specific mechanics.

## Active Tool Set Integration

The broker owns loading and unloading policy.

A tool must not become callable merely because it exists in the registry.

A tool must not remain callable forever just because it was useful once.

The broker should support:

- turn-scoped loading
- session-scoped loading
- workflow-scoped loading
- mode baseline loading
- cache reuse with revalidation

## Caching

The broker should support session and workflow caches to avoid re-searching and re-loading tools on every turn.

However:

- cached tools must still be revalidated each turn
- cached tools must not survive mode/environment/authority changes blindly
- cached tools must not override no-tool decisions for casual chat turns

## Broker and Local Models

Local or weaker models require stricter broker behavior.

The broker should:

- expose fewer tools
- prefer no-tool mode more often
- prefer read-only tools when intent is ambiguous
- suppress similar overlapping tools
- disable parallel tool calls by default
- favor broker-driven discovery over model-driven discovery

## Broker and Frontier Models

Stronger models can handle more nuance, but the broker should still not expose the full registry.

Strong models benefit from the broker too:

- reduced ambiguity
- reduced token overhead
- stronger invariants
- more consistent execution traces

## Hallucinated Tool Recovery

The broker should participate in recovery for unknown or out-of-set tool requests.

### Case 1: Tool name does not exist

1. Detect exact tool miss.
2. Run discovery for related matches.
3. Return internal repair context.
4. Allow one bounded retry.
5. If retry fails, respond gracefully.

### Case 2: Tool exists but is not active

1. Reject direct call.
2. Ask broker whether load is allowed.
3. If loadable and safe, load and retry.
4. If not, provide constraint context.

### Case 3: Tool exists and is active but governance rejects execution

This is no longer a broker problem. That passes to the execution runtime and governor.

## Example Flows

## Example A: Basic Chat

Input:

`yo`

Broker outcome:

- intent = smalltalk
- discovery skipped
- active tool set = empty
- tool choice = none

Result: model replies normally without tools.

## Example B: Diagnostics in Companion Mode

Input:

`Can you check why the last Telegram message duplicated thinking bubbles?`

Broker outcome:

- intent = diagnostic_request
- discovery returns diagnostic and coder-mode tools
- policy filters out debug-only tools in production companion mode
- selected tools = none or safe meta-tools only
- tool choice = none

Result: model explains constraints or asks to switch mode rather than inventing a diagnostic tool call.

## Example C: Coder Session

Input:

`Find where the Telegram thinking placeholder is duplicated.`

Broker outcome:

- intent = code_diagnostics
- mode = coder
- discovery returns repo.search, files.read, git.diff, tests.read
- selected tools = top 3–5
- active tool set loaded for session scope
- tool choice = auto

Result: model can inspect code without seeing unrelated tools.

## Anti-Patterns

### Anti-Pattern 1: Global Tool Exposure

The broker is bypassed and the model sees the full registry.

### Anti-Pattern 2: Search Result == Callable

Discovery says a tool exists, and the broker automatically exposes it without load/policy checks.

### Anti-Pattern 3: Model-Driven Surface Expansion

The model names a tool and the runtime widens the active set to satisfy it.

### Anti-Pattern 4: Cache Wins Over Context

A cached tool remains exposed even though the current turn is casual chat.

### Anti-Pattern 5: Multiple Exposure Paths

Different runtime paths construct different callable surfaces without going through the broker.

## Observability

Every broker decision should log:

- broker invocation id
- turn id
- session id
- workflow id
- intent
- mode
- environment
- discovery query
- candidate count
- suppressed tools and reasons
- selected tools
- loaded tools
- cache hits/misses
- tool choice mode
- model profile
- downstream execution outcome linkage

## Quality Metrics

Track:

- no-tool decision rate
- tool exposure count by turn type
- selected-but-unused rate
- hallucinated unknown-tool call rate
- unloaded-tool call rate
- irrelevant-tool exposure rate
- local-model misuse rate
- cache reuse rate
- recovery success rate

## Hard Constraints

- The broker is mandatory for all provider/model tool exposure.
- The broker cannot be bypassed by built-ins, skills, plugins, or diagnostics.
- The broker cannot grant execution authority by itself.
- The broker cannot expose dev/test tools in production contexts.
- The broker cannot allow the model to widen its own callable surface by naming tools.

## Design Position

The broker is the linchpin of the agentic tool system.

Discovery without a broker is unsafe.

Execution without a broker is noisy and brittle.

The broker is what makes rich discovery and sparse execution coexist cleanly.
