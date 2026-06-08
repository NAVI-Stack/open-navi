**Status:** Evolving  
**Last Updated:** 2026-04-15  

# Tool Governance and Safety

## Purpose

Define the safety, authorization, and control model for the NAVI Tool System.

This document exists to ensure that tools are not merely discoverable and executable, but also governed correctly across:

- environments
- user authority tiers
- risk levels
- session modes
- workflow states
- provider/model differences

Without a governance model, the broker and lifecycle model can still expose tools correctly while the system remains unsafe in practice.

---

## Core Principle

The model never authorizes tool execution.

The model may:
- express intent
- request discovery
- propose a tool call

But only runtime governance determines whether a tool may be:
- discovered
- loaded
- exposed
- executed

---

## Governance Scope

Governance applies to the entire tool system:

- registry visibility
- discovery visibility
- loading eligibility
- active-set exposure
- execution approval
- result handling
- recovery behavior
- observability and auditability

Governance is not a single execution-time check. It is a layered control model.

---

## Governance Layers

### 1. Registry Governance

Controls whether a tool may exist in a given environment and registry at all.

Examples:
- test-only tools excluded from production registry
- experimental tools hidden behind feature flags
- deprecated tools excluded from new registration paths

### 2. Discovery Governance

Controls whether a tool may appear in discovery results.

Examples:
- admin-only tools hidden from normal users
- debug diagnostics visible only to owners
- dangerous tools hidden from conversational sessions

### 3. Loading Governance

Controls whether a tool may be loaded into the Active Tool Set.

Examples:
- requires auth to be valid
- requires connector health
- requires correct session mode
- requires explicit user request

### 4. Exposure Governance

Controls whether a loaded tool may be exposed to the model as callable.

Examples:
- read-only tools exposed in assistant mode
- write tools suppressed when intent is ambiguous
- no tools exposed in smalltalk mode

### 5. Execution Governance

Controls whether a proposed tool call may execute.

Examples:
- requires confirmation
- blocked by policy
- blocked by authority
- blocked by workflow state

### 6. Result Governance

Controls how tool outputs are treated.

Examples:
- untrusted external data not treated as instruction
- tool results redacted or summarized for logs
- sensitive outputs restricted by user authority

---

## Governance Inputs

Governance decisions should consider:

- environment
- session mode
- user authority
- tool category
- tool source
- tool risk tier
- side-effect profile
- current workflow state
- connector/auth health
- feature flags
- trust tier
- model profile
- execution history

---

## Tool Categories

All tools should be classified at least into:

- chat-safe
- read-only
- workflow/action
- orchestration/meta
- internal diagnostic
- dev/test
- admin/critical

These categories are governance-relevant, not just descriptive.

---

## Risk Tiers

Every tool must have a risk tier.

### Tier 0 — Safe/Informational

Examples:
- local read-only inspection
- internal capability descriptions
- passive metadata lookup

Properties:
- no side effects
- no external writes
- no irreversible action

### Tier 1 — Low Risk

Examples:
- read external data
- search a repo
- inspect logs

Properties:
- read-only
- bounded scope
- minimal user harm if misused

### Tier 2 — Medium Risk

Examples:
- create draft
- modify local workspace file
- create issue/ticket

Properties:
- writes state
- usually reversible
- may require mode gating

### Tier 3 — High Risk

Examples:
- send email/message
- delete/overwrite data
- change production config
- execute shell commands

Properties:
- external side effects
- potentially user-visible impact
- often requires confirmation

### Tier 4 — Critical Risk

Examples:
- privileged admin actions
- credential or permission changes
- destructive production operations
- broad arbitrary code execution

Properties:
- high blast radius
- must be heavily restricted
- often unavailable outside admin/debug environments

---

## Governance Metadata Requirements

Each tool should declare at minimum:

- category
- risk tier
- side effects
- reversibility
- environment visibility
- required mode
- required authority
- feature flag requirement
- auth requirement
- connector dependencies
- trusted/untrusted output class
- confirmation requirement

A tool without required governance metadata should not become active.

---

## Authority Model

Governance should distinguish at least:

- normal user
- owner
- admin/operator
- internal system

A model is not an authority principal.

Examples:
- owners may discover debug tools as unavailable with reasons
- normal users may never see admin tools
- operators may use production diagnostics unavailable to standard sessions

---

## Environment Partitioning

This is non-negotiable.

The system must treat environment separation as a hard boundary.

### Environments

At minimum:
- production
- staging
- development
- test

### Rules

- test-only tools must not appear in production registries
- development diagnostics must not silently leak into production discovery
- staging-only experimental tools must not appear in ordinary production sessions

This should be enforced by registry construction, not just prompt wording.

---

## Session Mode Governance

Session modes should influence tool governance.

Examples:

### Companion Mode

- default: no tools or very small safe set
- suppress most write or diagnostic tools
- prioritize textual conversation

### Assistant Mode

- allow bounded productivity tools
- prefer read-only unless intent is explicit

### Coder Mode

- allow code inspection and development tools
- allow write operations only within policy

### Debug Mode

- allow diagnostic tools
- still enforce authority and environment constraints

### Admin Mode

- allow privileged tools only with explicit authority and auditability

---

## Confirmation Model

Not all allowed actions should auto-execute.

Tool governance should support at least:

- execute immediately
- require clarification
- require user confirmation
- require proposal/approval artifact
- reject outright

### Clarification

Use when:
- intent is ambiguous
- arguments are incomplete
- multiple risky interpretations exist

### Confirmation

Use when:
- action is high-risk but straightforward
- user intent is likely but execution has consequences

### Proposal

Use when:
- action is high-risk, multi-step, or partially irreversible
- a durable record is desirable

---

## Side-Effect Model

Every tool must declare its side-effect profile.

Examples:
- none
- local_read
- external_read
- local_write
- external_write
- network_send
- delete
- execute_code
- schedule_future_action

Governance should never infer side effects from name alone.

---

## Reversibility Model

Tools should declare whether actions are:

- reversible
- compensable
- irreversible

Examples:
- draft creation: reversible
- issue creation: compensable or reversible
- sent email: often irreversible
- deleted production data: potentially irreversible

This matters for confirmation, recovery, and user-facing language.

---

## Discovery Governance Rules

A tool may be:

- hidden entirely
- visible as available
- visible as unavailable with reason

Examples:

### Hidden Entirely

- test-only tool in production
- critical admin tool for normal user

### Visible as Unavailable

- owner in companion mode can discover debug diagnostic tool with `requires_debug_mode`

### Visible as Available

- safe read-only tools in coder mode

---

## Exposure Governance Rules

A loaded tool may still be suppressed from provider exposure.

Examples:
- cached file-edit tool suppressed during a casual chat turn
- send tool suppressed because intent is not explicit
- multiple overlapping tools suppressed to reduce local-model confusion

This means:

- loaded != exposed
- exposed != executable

---

## Execution Governance Decision Types

Execution governance should return structured decisions.

```json
{
  "decision": "approve | reject | clarify | confirm | propose",
  "reason_code": "requires_confirmation",
  "message": "Sending email is allowed but requires confirmation.",
  "recoverable": true
}
```

Do not collapse all failures into generic rejection.

---

## Handling Hallucinated and Out-of-Bounds Calls

### Unknown Tool

If the model requests a tool that does not exist:

- reject fail-closed
- log hallucinated tool attempt
- run bounded internal repair
- do not surface raw internal error to user

### Existing but Unexposed Tool

If the model requests a tool that exists but is not active:

- reject direct execution
- ask broker whether loading is allowed
- if not allowed, provide controlled recovery

### Existing, Exposed, but Disallowed Invocation

If the tool exists and is active but the arguments or action violate policy:

- reject execution
- return structured governance decision
- ask for clarification or confirmation if appropriate

---

## Result Safety

Tool results must be governed too.

### Output Trust Classes

- trusted_internal
- trusted_local
- untrusted_external
- untrusted_user_generated
- instruction_capable_internal

Default assumption should be that tool outputs are data, not instructions.

External logs, files, web results, and API payloads should not gain authority simply because they passed through a tool.

---

## Recovery Model

Governance should support bounded recovery.

Examples:

- tool not available → explain constraints and continue textually
- schema mismatch on low-risk read tool → repair once
- missing confirmation on send action → create confirmation prompt

Recovery should never silently override safety controls.

---

## Observability and Audit

Every governance decision should log:

- tool id
- category
- risk tier
- side effects
- reversibility
- environment
- mode
- user authority
- broker context
- decision type
- reason code
- whether confirmation/proposal was required
- whether recovery occurred
- whether user saw the constraint

High-risk and critical actions should be fully auditable.

---

## Anti-Patterns

### Anti-Pattern 1: Execution-Only Governance

Bad:

Only checking safety at the moment of execution.

Why wrong:

Dangerous tools may already have leaked into discovery or exposure.

### Anti-Pattern 2: Prompt-Only Safety

Bad:

Telling the model not to use certain tools without runtime enforcement.

Why wrong:

The model is not an authority boundary.

### Anti-Pattern 3: Category-Free Tools

Bad:

Tools have no risk tier or side-effect declaration.

Why wrong:

The system cannot reason about governance correctly.

### Anti-Pattern 4: Dev/Test Hidden Only by Convention

Bad:

Assuming developers simply won't register test tools in production.

Why wrong:

This eventually fails.

### Anti-Pattern 5: Raw Internal Rejection Surfaced to User

Bad:

`out-of-bound tool call(s): self_diagnostic_recent_errors`

Why wrong:

This is a runtime/model mismatch, not a user-facing error contract.

---

## Hard Constraints

- No tool may execute without governance.
- No dev/test tool may appear in production through prompt shaping alone.
- No tool may become active without governance metadata.
- No high-risk action may auto-execute purely because the model seemed confident.
- No tool result may be treated as trusted instruction by default.

---

## Design Position

The tool system is only production-grade if governance is layered, explicit, and enforced before and during execution.

Discovery without governance leaks capability.

Execution without governance leaks authority.

The model proposes.

The broker narrows.

Governance decides.

The runtime executes.
