**Status:** Evolving  
**Last Updated:** 2026-04-16  

# Tool Degradation and Fallback Routing Case Study

## Purpose

Capture a concrete failure mode where a selected tool/action exists and is visible, but fails because the tool implementation, connector action, API mapping, or provider integration is degraded or broken.

This case study strengthens the Agentic Tool System requirement that NAVI should classify tool failures and attempt safe fallback routing when possible, rather than treating every tool failure as task failure.

## Case Study

The user asked the assistant to update the Agentic Tool System index in GitHub.

Initial attempt:

1. GitHub connector was available.
2. The assistant attempted to use the apparent high-level `update_file` action.
3. The connector returned an action-not-found failure for `update_file`.
4. The assistant recognized this as a tool/action degradation, not an impossible task.
5. The assistant recovered by using lower-level Git operations:
   - create blob
   - create tree
   - create commit
   - update ref
6. The branch was updated successfully.

This is a tool degradation and fallback-routing problem.

It is distinct from late-enabled tool discovery:

- Late-enabled discovery: the tool is not in the current active surface or becomes newly available.
- Tool degradation: the selected tool/action is present but fails or is unusable.

## Failure Mode

### Name

Tool degradation without fallback routing.

### Definition

A tool/action is selected correctly, but the action fails due to implementation failure, connector mismatch, outage, schema/API drift, or runtime incompatibility. The agent stops even though an equivalent safe fallback path exists.

### Root Cause

The runtime treats tool failure as terminal instead of classifying the failure and searching for governed fallback actions.

### Bad Behavior

```text
User requests task X.
Tool A is selected for X.
Tool A fails due to connector/action issue.
Assistant says task cannot be done.
Safe fallback Tool B/C/D exists.
Fallback is not attempted.
```

### Desired Behavior

```text
User requests task X.
Tool A is selected for X.
Tool A fails.
Runtime classifies failure.
Broker/runtime searches for safe fallback path.
Fallback tools are governed and executed through the same execution path.
Task succeeds or fails with precise explanation.
```

## Failure Classification Requirement

Tool failures must be classified into machine-readable types.

Examples:

- selected_tool_unavailable
- selected_action_missing
- connector_unavailable
- auth_failure
- schema_mismatch
- api_contract_drift
- transient_timeout
- execution_failed
- partial_failure
- governance_blocked
- policy_rejected

These classes affect recovery behavior.

Governance/policy blocks are not degradation cases and should not trigger fallback execution unless explicitly approved by governance.

## Fallback Routing Requirement

If a tool/action fails due to degradation, NAVI may search for fallback candidates.

Fallback candidates must preserve:

- user intent
- risk tier or stricter risk tier
- side-effect profile or safer equivalent
- environment constraints
- user authority constraints
- confirmation/proposal requirements
- single governed execution path

Fallback routing must never become a bypass.

## Fallback Examples

### GitHub File Update

High-level failed action:

```text
GitHub.update_file
```

Safe lower-level fallback path:

```text
GitHub.create_blob
GitHub.create_tree
GitHub.create_commit
GitHub.update_ref
```

This fallback is acceptable only if it preserves the same intended file update, branch, and governance constraints.

### Read Tool Degradation

If a semantic search tool fails, fallback may be lexical search or exact lookup.

### Send/External Action Degradation

If a send action fails, fallback should usually not auto-send via a different channel without explicit user confirmation.

## Broker Requirement

The broker should support fallback candidate discovery after classified degradation.

Inputs should include:

- failed tool id
- failed action
- failure class
- original intent
- original arguments or normalized task intent
- active tool set
- current mode/environment/authority
- risk and side-effect ceiling

Outputs should include:

- no fallback available
- fallback candidates
- fallback requires confirmation
- fallback blocked by governance

## Runtime Requirement

The execution runtime should:

1. detect and classify the failure
2. preserve the original tool call trace
3. request fallback candidates if failure is recoverable
4. execute fallback only through the same governed execution path
5. correlate fallback result to the original user task
6. surface a clean user-facing result

## Observability Requirement

Every fallback attempt should log:

- original tool/action
- failure class
- fallback candidates considered
- selected fallback path
- governance decision
- execution result
- whether user was informed

This allows NAVI to learn which tools are degraded and which fallback paths are reliable.

## User Experience Requirement

NAVI should avoid raw tool-internal errors unless the user is in debug/developer mode.

Good user-facing behavior:

```text
The direct file-update action failed, so I used the lower-level Git write path and completed the update.
```

Bad user-facing behavior:

```text
Action GithubConnector.update_file not found on connector.
```

The raw internal error belongs in trace logs, not ordinary chat.

## Anti-Patterns

### Anti-Pattern 1: Tool Failure Equals Task Failure

Wrong:

```text
The selected tool failed, therefore the task is impossible.
```

### Anti-Pattern 2: Fallback Bypasses Governance

Wrong:

```text
High-level action failed, so call lower-level actions without policy checks.
```

### Anti-Pattern 3: Unsafe Side-Effect Substitution

Wrong:

```text
Email send failed, so send via another channel automatically.
```

### Anti-Pattern 4: Silent Fallback With Different Semantics

Wrong:

```text
Fallback succeeds, but performs a meaningfully different operation than the user requested.
```

## Acceptance Implication

A correct implementation should pass a test equivalent to:

1. High-level GitHub file update action is selected.
2. The action fails due to selected_action_missing or api_contract_drift.
3. Runtime classifies the failure as recoverable degradation.
4. Broker discovers lower-level Git write actions.
5. Governance approves the equivalent fallback path.
6. Runtime executes fallback through the normal governed execution path.
7. User receives a clean success result.
8. Trace records the degraded tool and fallback path.

## Related Linear Ticket

- OMN-259 — ATS-12: Add tool degradation detection and fallback routing

## Related Design Docs

- `tool-usage-and-execution.md`
- `tool-broker-design.md`
- `tool-governance-and-safety.md`
- `tool-lifecycle-and-state-model.md`
- `active-tool-set-and-provider-exposure.md`
