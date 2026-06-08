# ADR-002: NATS JetStream as Event Bus

**Status:** Accepted  
**Date:** 2026-02-01  
**Deciders:** NAVI AI Core Team  
**See also:** NAVI ADR-002 (identical decision, shared rationale)

---

## Context

NAVI AI requires a message bus for:
- Orchestrator loop → worker agent task dispatch
- Worker agent → Orchestrator loop completion and failure signals
- All components → append-only audit event log
- Governor trips → all active sessions
- HITL events → gateway → user connectors
- WebSocket live stream → gateway clients

Requirements:
- Durable streams (messages survive process restarts)
- Replay from sequence number (WebSocket reconnects, crash recovery)
- Subject-based routing (different streams for different event types)
- Single-binary local deployment (no external infrastructure beyond NATS itself)
- Schema version validation at bus boundaries
- At-least-once delivery with deduplication

---

## Decision

**NATS JetStream** as the primary messaging substrate for all inter-component communication.

Both `JetStreamBus` (production) and `MemBus` (test/local) implement the same `Bus` interface, enabling deterministic testing without a live NATS server.

---

## Rationale

**NATS strengths for this use case:**
- Single binary deployment — `nats-server` is a ~20MB binary with no external dependencies. Local-first philosophy matches NAVI's design.
- JetStream provides durable, replayable streams with consumer group semantics — exactly what the worker runner pool needs.
- Subject routing (`navi.cmd.worker.coder`, `navi.fact.worker.completed`) enables precise stream partitioning without custom routing logic.
- NATS's Go SDK (`nats.go`) is the reference implementation — best-supported, most performant, Go-native.
- JetStream's WorkQueue pattern supports serialized processing where required.

**Against Kafka:**
- Kafka requires ZooKeeper or KRaft plus broker infrastructure — far too heavy for local-first deployment.
- JVM startup time and operational overhead do not fit the local runtime target.

**Against gRPC/HTTP point-to-point:**
- No durable delivery — if a consumer is down, messages are lost.
- No replay capability — crash recovery requires rebuild.
- Fan-out to multiple consumers requires explicit orchestration.

---

## Dual-Write Guarantee

Every event published to NATS is also written to the SQLite `events` table before publication. This guarantees:
- Events are replayable even if NATS JetStream state is lost
- WebSocket clients can request `after_seq=N` on reconnect and receive missed events from SQLite
- The audit log is independent of NATS availability

Implementation: `bus.go` `Publish()` calls `store.AppendEvent()` first, then `js.PublishMsg()`. If SQLite write fails, the event is not published to NATS and an error is returned. If NATS publish fails after a successful SQLite write, an outbox retry mechanism delivers it on the next attempt.

---

## Stream Topology

| Stream | Purpose | Retention |
|---|---|---|
| `NAVI_Orchestrator` | Directive intake, Orchestrator conversation | 7 days |
| `NAVI_AGENTS` | Task assignments, worker completion signals | 7 days |
| `NAVI_REFINERY` | Code review and merge queue (MaxConsumers: 1) | 7 days |
| `NAVI_HITL` | Human-in-the-loop requests and resolutions | 30 days |
| `NAVI_AUDIT` | Append-only audit log for all events | 90 days |
| `NAVI_GOVERNOR` | Governor trip events, budget signals | 30 days |

All subjects are prefixed `navi.` — e.g., `navi.cmd.worker.assign`, `navi.fact.worker.completed`.

---

## Consequences

- Schema version validation is mandatory at every publish and consume boundary — mismatches are rejected, not silently ignored.
- Stream topology is a first-class architecture artifact — stream definitions live in `internal/bus/streams.go` and are version-controlled.
- An embedded NATS mode supports zero-dependency local development.
- Consumer lag on `NAVI_AGENTS` is a leading indicator of system overload — supervisor monitors it.

---

## Status History

| Date | Change |
|---|---|
| 2026-02-01 | Accepted — NATS JetStream for all inter-component messaging |
