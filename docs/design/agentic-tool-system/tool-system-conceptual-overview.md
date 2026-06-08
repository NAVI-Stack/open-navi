**Status:** Evolving  
**Last Updated:** 2026-04-15  

# Tool System — Conceptual Overview

## Purpose

The NAVI Tool System is the capability mediation layer that allows NAVI to discover, load, govern, and execute capabilities without exposing the entire capability universe directly to the model.

NAVI is not a simple function-calling chatbot. It is a persistent agent operating across workflows, environments, and capabilities. The Tool System must therefore be structured, stateful, and governed.

## Core Problem

Naive tool systems fail by:

- exposing too many tools
- confusing weaker models
- encouraging hallucinated tool names
- leaking dev/test tools
- bypassing governance

These are architecture failures, not model failures.

## Core Principle

The full tool universe must be known to NAVI, but only a small, governed subset may be callable at any moment.

## Capability States

A tool may exist in different states:

- registered
- discoverable
- loadable
- loaded
- callable
- executable

These are distinct states and must not be conflated.

## Architecture Placement

The Tool System sits in the Capability Layer and interfaces with:

- Cognitive Layer (intent, reasoning)
- Governance (authorization)
- Execution Runtime (actual side effects)

The model is not the source of truth for tools.

## Core Entities

- Tool — LLM-facing callable interface
- Skill — versioned capability package exposing tools
- Plugin — extension bundle
- Connector — integration surface
- Tool Registry — authoritative catalog
- Tool Index — searchable projection
- Tool Broker — mediator between registry and active set
- Active Tool Set — current callable surface
- Tool Executor — runtime execution

## Two Subsystems

### Discovery/Search

Answers: what tools exist and are relevant?

### Usage/Execution

Answers: can this tool be called and executed now?

## Invariants

- registry != exposure
- discovery != execution
- loaded != executed
- execution is single-path
- governance is authoritative
- unknown tool calls are recoverable

## Design Goals

- reduce hallucinated tool calls
- reduce token overhead
- support local models
- maintain fail-closed execution
- support dynamic discovery

## Design Position

NAVI should adopt:

- broad discovery
- narrow execution

The model proposes.

The system decides.
