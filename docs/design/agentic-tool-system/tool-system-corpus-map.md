**Status:** Evolving  
**Last Updated:** 2026-04-15  

# Tool System Corpus Map

## Purpose

Defines the full documentation structure required for the NAVI tool system.

Prevents gaps between concept, design, and implementation.

## Layers

### Conceptual

- Tool System Overview
- Discovery/Search
- Usage/Execution
- Lifecycle and State Model
- Governance and Safety

### Design

- Tool Registry
- Tool Broker
- Active Tool Set
- Tool Search and Ranking
- Execution Runtime
- Provider Protocols
- Observability

### Specification (later)

- entity schemas
- APIs
- runtime contracts
- adapter specs
- telemetry
- migration

## Principle

Incomplete specs create system drift.

All surfaces must be covered before implementation.

## Design Rule

Prefer explicit contracts over implicit behavior.

Prefer narrow surfaces over global exposure.

## Outcome

A high-fidelity spec aligned with implementation.
