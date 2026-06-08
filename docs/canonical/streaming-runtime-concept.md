# Streaming Runtime Conceptual Plan

**Status:** Reference  
**Last Updated:** 2026-03-16  
**Updated By:** Eric (Owner)  
**ADR:** [ADR-006](../adr/ADR-006-streaming-runtime.md)  
**Implementation:** [design/streaming-runtime](../design/streaming-runtime.md)

---

## Purpose

This document is the original conceptual plan for NAVI's streaming UX and messaging runtime. It captures the full problem analysis, design principles, target architecture, data models, protocol design, infrastructure recommendations, and phased rollout plan.

Structural decisions from this plan have been locked in [ADR-006](../adr/ADR-006-streaming-runtime.md). The phased implementation plan with code-level detail is in [design/streaming-runtime](../design/streaming-runtime.md).

This document is retained as a reference for the full rationale behind those decisions.

---

## Document Location Note

The full conceptual plan is extensive (20 sections). Key sections and their disposition:

| Section | Disposition |
|---------|-------------|
| §1 Executive Summary | Captured in ADR-006 context |
| §2 Problem Statement | Captured in ADR-006 context |
| §3 Design Principles | Active reference — connector-agnostic, governance-respecting, replayable |
| §4 Target Architecture | Active reference — runtime flow diagram |
| §5 Core Components | Active reference — Inbox, Classifier, Coordinator, Interrupt Policy, Event Bus, Renderers, Trace/Checkpoint |
| §6 Data Models | Superseded by design/streaming-runtime.md §Data Models (simplified for phased build) |
| §7 Streaming UX Model | Active reference — three stream classes, granularity, partial message semantics |
| §8 Protocol Design | Active reference — WebSocket contract, frame types |
| §9 Multi-Channel Model | Active reference — channel bridges, deterministic ordering, per-channel constraints |
| §10 Broker Recommendation | Decided: NATS + JetStream (reaffirms ADR-002) |
| §11 Integration Rules | Active reference — proposal, failure, skill, governance integration rules |
| §12 Persistence/Replay/Resume | Active reference — resume safety rules |
| §13 Observability | Active reference — required metrics and traces |
| §14 Developer Ergonomics | Backlogged |
| §15 Priority Ordering | Captured in design/streaming-runtime.md §Phases |
| §16 Implementation Phases | Superseded by design/streaming-runtime.md (code-level detail with Option B) |
| §17 Migration Plan | Superseded by design/streaming-runtime.md §Migration Path |
| §18 Recommended Decisions | All decided in ADR-006 |
| §19 Open Risks | Captured in design/streaming-runtime.md §Risks |
| §20 Final Recommendation | Captured in ADR-006 |
