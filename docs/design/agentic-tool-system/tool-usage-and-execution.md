**Status:** Evolving  
**Last Updated:** 2026-04-15  

# Tool Usage and Execution

## Purpose

Execution governs how NAVI turns possible capability into actual action.

The model proposes. NAVI decides and executes.

## Core Principle

Tool calls are untrusted proposals.

They must pass validation and governance before execution.

## Responsibilities

- maintain active tool set
- expose provider schemas
- validate tool calls
- validate arguments
- enforce governance
- dispatch execution
- normalize results
- handle recovery

## Active Tool Set

The active tool set is the current callable surface.

A tool must be loaded before it can be called.

Loaded != executable.

## Execution Lifecycle

1. user input
2. broker loads tools
3. model proposes tool call
4. runtime validates tool + args
5. governance approves/rejects
6. execution
7. result normalization

## Failure Modes

### Unknown tool

- do not execute
- recover internally

### Unloaded tool

- reject
- optionally load via broker

### Disallowed tool

- fail closed

### Schema mismatch

- validate before execution

## Governance

All tools go through the same governance path.

No bypass for internal tools.

## Results

Tool results are data, not instructions.

They must be treated as untrusted.

## Anti-Patterns

- model-defined tools executing
- multiple execution paths
- raw errors exposed to user

## Design Position

Execution must be strict, deterministic, and observable.

Discovery can be flexible.

Execution cannot.
