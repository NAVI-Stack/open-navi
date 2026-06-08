# ADR-001: Go for Orchestration Core, Python for AI Layer

**Status:** Accepted  
**Date:** 2026-02-01  
**Deciders:** NAVI AI Core Team  
**Supersedes:** N/A

---

## Context

NAVI AI requires a high-concurrency orchestration runtime for:
- A 2-second tick loop managing hundreds of concurrent tasks
- NATS JetStream event publishing and consuming across 6 streams
- SQLite WAL writes from multiple goroutines (serialized via single connection pool)
- Worker agent runners each managing their own goroutine lifecycle

The system also requires an AI layer that calls LLM APIs, parses tool call responses, and manages multi-turn conversation loops. This layer benefits from Python's AI ecosystem (Pydantic, type generation tooling, future ML library access).

The question was: one language or two, and if two, where is the boundary?

---

## Decision

**Go** for the orchestration core: Orchestrator loop, state engine, event bus, governor, gateway API, supervisor, worker base.

**Python** for AI model interaction: LLM provider adapters (when Python workers are active), Pydantic schema models (generated from Go types), future ML/embedding workloads.

**Rust** reserved for future hot-path optimization only — not a foundation dependency.

---

## Rationale

**For Go orchestration:**
- NATS's strongest SDK and most active community is Go-native
- Goroutines and channels map directly to the orchestration concurrency model (one goroutine per worker runner, one goroutine per supervisor sweep, etc.)
- SQLite's `modernc.org/sqlite` is pure Go with WAL mode — no CGO required in test builds
- Faster iteration loop for early platform phases (compile times vs. Rust)
- Go's strict error handling at compile time prevents entire classes of nil-pointer and type errors that would surface at runtime in Python

**For Python AI layer:**
- LLM provider SDKs (Anthropic, OpenAI) have first-class Python clients
- `datamodel-code-generator` generates Pydantic V2 models from JSON Schema — clean cross-language contract enforcement
- Future embedding and ML workloads (vector stores, model fine-tuning utilities) assume Python
- Agent prompt engineering benefits from Python's string manipulation ergonomics

**Against Rust as foundation:**
- NATS Go SDK is more mature than the Rust equivalents at decision time
- Rust compile times and borrow checker overhead would significantly slow Phase 1-4 iteration
- The hot paths (LLM calls, SQLite writes) are I/O-bound, not CPU-bound — Rust's performance advantage doesn't apply here

---

## Boundary Rule

The Go/Python boundary is HTTP on loopback `:7700` (internal only, never exposed externally):

```
POST /internal/validate/orchestrator-output
POST /internal/governor/check-action
POST /internal/governor/add-cost
GET  /internal/governor/status
POST /internal/state/read-snapshot
```

Go starts this server inside the main control plane binary. Python workers call it via standard HTTP. No gRPC — loopback latency is <1ms and gRPC adds protobuf compilation overhead with no benefit at this scale.

---

## Consequences

- NATS stream topology and schema contracts are defined in Go and generated into Python — Go is always the source of truth
- All governor enforcement is Go-side — Python cannot bypass limits
- Rust optimization is preserved as a future option for identified hot paths without architectural disruption
- Two language runtimes must be kept in sync during development — managed via `make generate-python` and schema versioning

---

## Status History

| Date | Change |
|---|---|
| 2026-02-01 | Accepted — Go orchestration core, Python AI layer |
