**Status:** Evolving  
**Last Updated:** 2026-04-15  

# Tool Discovery and Search

## Purpose

Discovery gives NAVI awareness of capabilities without granting execution authority.

It answers:

- does a tool exist?
- is it available?
- what alternatives exist?
- why is it unavailable?

## Core Principle

Discovery is knowledge. Execution is action.

## Responsibilities

- search registry
- filter by policy
- rank candidates
- explain availability
- suggest alternatives
- detect missing capabilities

## Outputs

Discovery returns structured results:

- tool id
- description
- relevance
- status
- availability reason
- suggested action

Discovery must be able to return **no results**.

## Exact vs Semantic

- exact lookup first
- semantic fallback second

This is critical for hallucination detection.

## Missing Capability

Discovery should explicitly return when no tool exists.

This feeds future skill/plugin development.

## Policy

Discovery is filtered by:

- environment
- mode
- user authority
- risk

Not all tools should be visible.

## Surfaces

- broker-driven discovery (default)
- model meta-tools (`tool.search`, etc.)
- admin/debug discovery

## Anti-Patterns

- injecting all tools into model
- relying on prompt-only tool awareness
- treating search results as executable

## Design Position

Discovery should be broad and informative.

It must never imply execution authority.
