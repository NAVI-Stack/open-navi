**Status:** Evolving  
**Last Updated:** 2026-04-15  

# Tool Lifecycle and State Model

## Purpose

Define the **exact lifecycle and state transitions** of a tool.

Without this, systems drift into:
- "tool exists so it must be callable"
- inconsistent availability across turns
- race conditions in loading/execution
- broken assumptions between broker and runtime

This document enforces a **single, explicit state machine**.

---

## Core Principle

A tool is not a binary (exists / does not exist).

It moves through **strict states**.

Each state has:
- explicit meaning
- explicit entry conditions
- explicit exit conditions
- explicit allowed transitions

---

## State Model

```text
REGISTERED
   ↓
DISCOVERABLE
   ↓
LOADABLE
   ↓
LOADED
   ↓
CALLABLE
   ↓
EXECUTING
   ↓
COMPLETED
```

Failure and side paths:

```text
LOADED → UNHEALTHY
CALLABLE → BLOCKED
EXECUTING → FAILED
```

---

## State Definitions

### 1. REGISTERED

Tool exists in the registry.

- known by system
- has metadata
- may not be usable

**Does NOT imply:**
- discoverable
- loadable
- callable

---

### 2. DISCOVERABLE

Tool is visible to discovery.

Conditions:
- passes environment filters
- passes visibility rules
- not hidden/deprecated

**Used by:**
- Tool Index
- Tool Broker (search phase)

---

### 3. LOADABLE

Tool can be prepared for runtime.

Conditions:
- dependencies available (connectors, config)
- auth valid
- feature flags enabled

**Failure examples:**
- missing API key
- connector down
- disabled feature

---

### 4. LOADED

Tool is initialized and bound to runtime.

Includes:
- resolved dependencies
- validated config
- bound context (session/workflow)

**Important:**
Loaded ≠ callable

---

### 5. CALLABLE

Tool is exposed to the model.

Conditions:
- broker selected it
- schema validated
- policy allows exposure

Only CALLABLE tools:
- appear in provider tool list
- can be proposed by model

---

### 6. EXECUTING

Tool is actively running.

- arguments validated
- governance approved
- dispatched to executor

---

### 7. COMPLETED

Execution finished.

- result normalized
- returned to model/runtime

---

## Side States

### UNHEALTHY

Tool cannot be loaded or executed.

Causes:
- connector failure
- repeated runtime errors
- timeout patterns

---

### BLOCKED

Tool is intentionally disallowed.

Causes:
- policy
- environment
- user authority
- risk constraints

---

### FAILED

Execution failed.

- runtime error
- partial execution
- timeout

---

## State Transitions

### Valid Transitions

```text
REGISTERED → DISCOVERABLE
DISCOVERABLE → LOADABLE
LOADABLE → LOADED
LOADED → CALLABLE
CALLABLE → EXECUTING
EXECUTING → COMPLETED
```

### Conditional Transitions

```text
LOADABLE → UNHEALTHY
CALLABLE → BLOCKED
EXECUTING → FAILED
```

### Invalid Transitions (must never happen)

```text
REGISTERED → CALLABLE
DISCOVERABLE → EXECUTING
LOADED → EXECUTING (without CALLABLE)
EXECUTING → REGISTERED
```

---

## Ownership of Transitions

| Transition | Owner |
|----------|------|
| REGISTERED → DISCOVERABLE | Registry / Policy |
| DISCOVERABLE → LOADABLE | Broker (pre-check) |
| LOADABLE → LOADED | Broker (load phase) |
| LOADED → CALLABLE | Broker (selection phase) |
| CALLABLE → EXECUTING | Execution Runtime |
| EXECUTING → COMPLETED | Executor |

---

## Broker Interaction

The broker operates primarily on:

- DISCOVERABLE
- LOADABLE
- LOADED
- CALLABLE

Broker responsibilities:
- never skip states
- never mark tool callable without loading
- never expose non-loadable tools

---

## Execution Runtime Interaction

Runtime responsibilities:

- validate tool is CALLABLE
- validate arguments
- transition to EXECUTING
- produce COMPLETED or FAILED

Runtime must NOT:
- load tools
- expose tools
- modify broker decisions

---

## Caching Interaction

Cached tools must:
- still be validated each turn
- re-check LOADABLE conditions
- re-check policy

Cached ≠ safe

---

## Session vs Turn Scope

### Turn Scope

- tools loaded only for one turn
- safer, higher overhead

### Session Scope

- tools persist across turns
- requires revalidation each turn

### Workflow Scope

- tools tied to workflow state machine
- deterministic exposure

---

## Failure Modes Prevented

### 1. Hallucinated Tool Calls

Tool never reaches CALLABLE → runtime rejects cleanly

---

### 2. Unloaded Tool Execution

Cannot transition LOADED → EXECUTING without CALLABLE

---

### 3. Dev Tool Leakage

Blocked at DISCOVERABLE or LOADABLE

---

### 4. Race Conditions

Explicit state transitions prevent parallel invalid execution

---

## Observability

Track per tool:

- current state
- last transition
- transition timestamp
- failure counts
- health status
- load latency
- execution latency

---

## Hard Constraints

- Every tool must follow this state machine
- No implicit transitions
- No direct jumps across states
- Broker and runtime must enforce transitions
- State must be observable

---

## Design Position

The lifecycle model is what makes the broker enforceable.

Without it:
- "available" becomes ambiguous
- execution becomes inconsistent
- bugs become non-deterministic

With it:
- tool behavior becomes predictable
- failures become explainable
- system becomes debuggable
