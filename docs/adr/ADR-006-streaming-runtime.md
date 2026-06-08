# ADR-006: Streaming UX & Message Runtime

**Status:** Accepted  
**Date:** 2026-03-16  
**Deciders:** Eric (Owner)  
**See also:** [ADR-002 NATS JetStream](ADR-002-nats-jetstream-bus.md), [Design: streaming-runtime](../design/streaming-runtime.md)

---

## Context

NAVI's architecture is strong in governance, proposals, failure handling, autonomy, and skill structure. The weak point is the interaction runtime. The current model is turn-based: an HTTP POST appends a message to a session, pokes a wake channel, `processTurn` calls the LLM, emits a reply, and returns. The WebSocket (`LiveHandler`) polls SQLite at 300ms intervals and is unidirectional — clients cannot send messages, cancel runs, or approve proposals mid-stream.

This creates several gaps:

- One-turn-at-a-time feel under bursty input
- No canonical session inbox or queue arbitration model
- No standard event envelope for runtime states beyond token streaming
- No mid-run steering, interruption, or pause/resume behavior
- No unified multi-surface rendering contract
- Token streaming without enough runtime semantics (users need run state, not just deltas)

If unaddressed, NAVI risks becoming a strong internal architecture wrapped in a clunky outer loop.

---

## Decisions

### D1. Core Runtime Posture

**Inbox-driven, event-native, run-coordinated, resumable by default.**

Messages are runtime inputs, not prompt text. Every inbound message is normalized into a durable inbox item, classified, arbitrated against current session state, and routed into a governed run lifecycle. Streaming is the live projection of that runtime state to all surfaces.

### D2. Extend `schema.Event`, Don't Fork

Add `RunID string` and `Visibility EventVisibility` to the existing `Event` envelope. Extend correlation via typed payload structs per event type, not envelope-level fields. Bump `SchemaVersion` to `"2.0.0"` after a migration window (see Consequences).

This preserves the existing `Bus.Publish` → `store.AppendEvent` → `LiveHandler` pipeline without dual-envelope confusion.

### D3. RunCoordinator Replaces AgentLoop as the Outer Loop

The `RunCoordinator` in `internal/runtime/` becomes the new outer loop. The wake channel, run lifecycle, and session coordination move out of `internal/navi/`. The existing `AgentLoop` is refactored into a thin cognitive executor — it retains the LLM multi-turn loop and tool execution, but is *called by* the RunCoordinator rather than owning the session lifecycle.

**Rationale:** Option A (RunCoordinator wraps AgentLoop, calling an exported `ProcessTurn`) preserves the current structure but couples runtime coordination to cognitive execution through a leaky abstraction. Option B (RunCoordinator owns the loop, AgentLoop becomes a cognitive executor) cleanly separates runtime concerns (inbox, run lifecycle, interrupts, checkpointing) from cognitive concerns (LLM calls, tool dispatch, persona, reflection). This is the cleaner long-term boundary and avoids a second refactor when pause/resume and multi-run support arrive in Phase 4.

**Package structure:**

```
internal/runtime/          — Run lifecycle, inbox, coordination, interrupt policy
  coordinator.go           — RunCoordinator: wake channel, session dispatch, one-foreground-run enforcement
  run.go                   — Run struct, state machine, lifecycle event emission
  inbox.go                 — InboxItem, thin intake wrapper (expanded in Phase 5)
  checkpoint.go            — CheckpointStore (Phase 4)
  classifier.go            — Queue classifier: merge/steer/interrupt/defer/supersede (Phase 5)
  ws_protocol.go           — Bidirectional WebSocket frame types and dispatch (Phase 3)

internal/navi/             — Cognitive execution (LLM calls, tool dispatch, persona, reflection)
  loop.go                  — CognitiveExecutor: LLM multi-turn loop, tool execution, shapeReply
  events.go                — NAVI-specific event payloads (unchanged)
  session.go               — SessionStore interface (unchanged)
```

### D4. WebSocket: Evolve LiveHandler, Keep HTTP Canonical

`LiveHandler` evolves from unidirectional polling to bidirectional session transport. HTTP endpoints remain the canonical write path for non-interactive clients, CLI scripts, and testing.

- **Phase 1:** LiveHandler as-is (already works for token streaming via event log polling)
- **Phase 3:** Add bidirectional frame handling — client can send `sendMessage`, `cancelRun`, `resolveProposal` frames
- HTTP POST to `/api/navi/sessions/{id}/message` continues working alongside WebSocket

### D5. Thin InboxItem First, Full Classifier Deferred

Start with `source_channel`, `queue_action` (defaulting to `"append"`), and `status`. Defer `dedupe_key`, `confidence`, burst merge windows, and the full queue classifier until Phase 5 when a second input source (e.g., Telegram + Web on the same session) creates real race conditions.

### D6. Experience Layer: Post-Stream Shaping

`shapeReply` continues to shape the final assembled content. During streaming, token deltas stream raw to surfaces. The `assistant.message.completed` event carries the final shaped content. Experience applies to the completed message, not the stream.

### D7. Event Bus: NATS + JetStream (reaffirms ADR-002)

No change. Design the event abstraction so Kafka can be used later for higher-retention or analytics-heavy pipelines. The existing `Bus` interface with `JetStreamBus` and `MemBus` implementations remains canonical.

### D8. Session Concurrency: One Foreground Run Per Session

A session supports multiple pending inputs and detached background work, but exactly one foreground run coordinator at a time. Background run support is deferred to Phase 6.

### D9. Performance Targets: Latency, Not Connections

- Time-to-first-token: < 200ms from message receipt
- Event fanout to all connected surfaces: < 50ms
- WebSocket reconnect + replay: < 500ms

### D10. Governance Remains Upstream of Execution

Streaming never bypasses policy. Tools, proposals, interrupts, and retries still flow through deterministic governance and failure rules. The Proposal Queue remains canonical — no second approval system in the streaming layer.

---

## Consequences

### Schema Version Migration

The existing bus consumers (`JetStreamBus.Subscribe`, `MemBus.Publish`) hard-reject schema version mismatches. Adding `RunID` and `Visibility` requires a migration window:

1. Add fields as `omitempty`, keep `SchemaVersion` at `"1.0.0"`
2. Deploy, verify old consumers still work with new fields present
3. Bump to `"2.0.0"` and update the version check to accept both `"1.0.0"` and `"2.0.0"` during transition
4. Drop `"1.0.0"` acceptance after confirming all consumers are updated

### AgentLoop Refactor Scope

`AgentLoop` loses its wake channel, `Run()` method, and session lifecycle ownership. It retains:

- `processTurn(ctx, sessionID)` — exported as `ExecuteTurn` for the RunCoordinator to call
- `executeTool` — tool dispatch with governance checks
- `shapeReply` — Experience Layer application
- `emitReflect` — Subconscious reflection emission
- All LLM interaction logic (multi-turn loop, tool result handling)

The `LoopConfig` struct is split: runtime dependencies (Bus, Session store, wake channel) move to `RuntimeConfig` in `internal/runtime/`; cognitive dependencies (LLM, Persona, Skills, Governor, WorldModel, Experience) stay in a `CognitiveConfig` in `internal/navi/`.

### New Event Types

Phase 1 introduces: `run.started`, `run.phase.changed`, `run.completed`, `run.failed`, `tool.call.started`, `tool.call.completed`, `tool.call.failed`.

Phase 2 introduces: `message.received`, `message.classified`.

Phase 4 introduces: `run.paused`, `run.resumed`, `proposal.waiting`, `proposal.resolved`, `governance.blocked`, `recovery.required`, `recovery.resolved`.

Phase 5 introduces: `message.merged`, `message.deferred`, `message.superseded`, `interrupt.raised`, `interrupt.applied`.

### Implementation Phases

| Phase | Focus | Key Deliverable |
|-------|-------|-----------------|
| 1 | Run lifecycle events on existing loop | Event vocabulary emitted through existing Bus |
| 2 | RunCoordinator replaces AgentLoop outer loop, thin InboxItem | Messages flow through InboxItem → RunCoordinator → CognitiveExecutor |
| 3 | Bidirectional WebSocket + streaming | Single WebSocket connection for send + receive |
| 4 | Proposal/failure runtime integration + checkpoints | Blocking proposals pause/resume runs |
| 5 | Multi-surface renderers + queue classifier | Two surfaces hitting same session with correct arbitration |
| 6 | Optimization + hardening | Stream compaction, chaos testing, replay tooling |

See [design/streaming-runtime.md](../design/streaming-runtime.md) for the full implementation plan, data models, and migration path.

---

## Status History

| Date | Change |
|---|---|
| 2026-03-16 | Accepted — Streaming UX & Message Runtime with RunCoordinator as primary loop (Option B) |
