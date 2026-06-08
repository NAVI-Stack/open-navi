# Streaming Runtime Design

**Status:** Evolving  
**Last Updated:** 2026-03-16  
**Updated By:** Eric (Owner)  
**ADR:** [ADR-006](../adr/ADR-006-streaming-runtime.md)  
**Companion:** [Conceptual plan](../canonical/streaming-runtime-concept.md)

---

## Summary

NAVI's streaming and message queueing are a single unified runtime model: inbox-driven on the way in, run-coordinated during execution, event-native internally, and stream-rendered on the way out. This document specifies the structural decisions, data models, and phased implementation plan.

The core shift: **Messages are runtime inputs, not prompt text.** Every inbound message is normalized into a durable inbox item, arbitrated against current session state, and routed into a governed run lifecycle. Streaming is the live projection of that runtime state.

---

## Structural Decisions

All decisions are locked in [ADR-006](../adr/ADR-006-streaming-runtime.md). Key points summarized here for implementer reference:

1. **Extend `schema.Event`** with `RunID` and `Visibility` — no parallel envelope
2. **RunCoordinator replaces AgentLoop** as the outer loop (Option B) — `AgentLoop` becomes a cognitive executor
3. **Evolve `LiveHandler`** to bidirectional — HTTP stays canonical for writes
4. **Thin InboxItem first** — full classifier deferred to Phase 5
5. **Experience shapes completed messages** — token deltas stream raw
6. **Latency targets**: TTFT < 200ms, fanout < 50ms, reconnect+replay < 500ms

---

## Package Structure

```
internal/runtime/              — Runtime coordination (new package)
  coordinator.go               — RunCoordinator: owns wake channel, session dispatch, run lifecycle
  run.go                       — Run struct, state machine, lifecycle event emission
  inbox.go                     — InboxItem intake, thin wrapper (Phase 2; expanded Phase 5)
  checkpoint.go                — CheckpointStore for resumable state (Phase 4)
  classifier.go                — Queue classifier: merge/steer/interrupt/defer/supersede (Phase 5)
  ws_protocol.go               — Bidirectional WebSocket frame types and method dispatch (Phase 3)

internal/navi/                 — Cognitive execution (refactored)
  executor.go                  — CognitiveExecutor: LLM multi-turn loop, tool dispatch (renamed from loop.go)
  events.go                    — NAVI-specific event payloads
  session.go                   — SessionStore interface
  persona.go                   — ExperienceManager
  config.go                    — CognitiveConfig (split from LoopConfig)
```

### Responsibility Split

**RunCoordinator** (`internal/runtime/`) owns:
- Wake channel and session dispatch loop (moved from `AgentLoop.Run`)
- InboxItem intake and routing
- Run lifecycle: create Run, emit `run.started`, enforce one-foreground-run, emit `run.completed`/`run.failed`
- Interrupt check points (before LLM call, before tool execution) via context or callback
- Checkpoint creation and resume (Phase 4)
- Queue arbitration (Phase 5)

**CognitiveExecutor** (`internal/navi/`) owns:
- `ExecuteTurn(ctx, sessionID) error` — the exported entry point called by RunCoordinator
- LLM multi-turn loop (call LLM, process tool calls, loop until no more tool calls)
- `executeTool` — tool dispatch with governance checks, proposal creation, failure handling
- `shapeReply` — Experience Layer application on completed content
- `emitReflect` — Subconscious reflection emission
- All LLM interaction, persona selection, system prompt construction

**CognitiveExecutor does NOT own:**
- When to start a turn (RunCoordinator decides)
- Session lifecycle or wake signaling
- Interrupt detection (RunCoordinator provides `ShouldInterrupt(ctx)`)
- Run state transitions (RunCoordinator emits lifecycle events)

### Config Split

`LoopConfig` is split into two structs:

**`RuntimeConfig`** (in `internal/runtime/`):
- Bus, SessionStore, wake channel mechanics
- InboxItem persistence
- Checkpoint store (Phase 4)

**`CognitiveConfig`** (in `internal/navi/`):
- LLM provider, Model, Persona engine, Skills registry, Policy engine
- Governor, WorldModel, Experience layer
- All callback hooks (SaveProposal, SaveExecutionOutcome, AutonomyResolver, etc.)

---

## Data Models

### Event Envelope Changes (Phase 1)

```go
// Added to schema.Event
type Event struct {
    // ... existing fields ...
    RunID      string          `json:"run_id,omitempty"`
    Visibility EventVisibility `json:"visibility,omitempty"`
}

type EventVisibility string
const (
    VisibilityUserVisible     EventVisibility = "user_visible"
    VisibilityOperatorVisible EventVisibility = "operator_visible"
    VisibilityAuditOnly       EventVisibility = "audit_only"
)
```

### New Event Types (Phase 1)

```go
// Run lifecycle
EventRunStarted      EventType = "run.started"
EventRunPhaseChanged EventType = "run.phase.changed"
EventRunCompleted    EventType = "run.completed"
EventRunFailed       EventType = "run.failed"

// Tool lifecycle
EventToolCallStarted   EventType = "tool.call.started"
EventToolCallCompleted EventType = "tool.call.completed"
EventToolCallFailed    EventType = "tool.call.failed"
```

### Payload Structs (Phase 1)

```go
type RunStartedPayload struct {
    RunID     string `json:"run_id"`
    SessionID string `json:"session_id"`
    Phase     string `json:"phase"` // "intake"
}

type RunCompletedPayload struct {
    RunID     string `json:"run_id"`
    SessionID string `json:"session_id"`
    ReplyLen  int    `json:"reply_len"`
}

type RunFailedPayload struct {
    RunID     string `json:"run_id"`
    SessionID string `json:"session_id"`
    Error     string `json:"error"`
}

type ToolCallStartedPayload struct {
    RunID    string `json:"run_id"`
    CallID   string `json:"call_id"`
    ToolName string `json:"tool_name"`
}

type ToolCallCompletedPayload struct {
    RunID    string `json:"run_id"`
    CallID   string `json:"call_id"`
    ToolName string `json:"tool_name"`
    Success  bool   `json:"success"`
}
```

### RunState (Phase 1)

```go
type RunStatus string
const (
    RunStatusStarting          RunStatus = "starting"
    RunStatusActive            RunStatus = "active"
    RunStatusWaitingForTool    RunStatus = "waiting_for_tool"
    RunStatusWaitingForProposal RunStatus = "waiting_for_proposal"
    RunStatusCompleted         RunStatus = "completed"
    RunStatusFailed            RunStatus = "failed"
    RunStatusCancelled         RunStatus = "cancelled"
    // Phase 4:
    RunStatusPaused            RunStatus = "paused"
)

type RunState struct {
    RunID     string    `json:"run_id"`
    SessionID string    `json:"session_id"`
    Status    RunStatus `json:"status"`
    Mode      string    `json:"mode"`  // "foreground" | "background"
    StartedAt time.Time `json:"started_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

### InboxItem (Phase 2, thin)

```go
type InboxItem struct {
    ID            string    `json:"id"`
    SessionID     string    `json:"session_id"`
    SourceChannel string    `json:"source_channel"` // "cli"|"app"|"web"|"telegram"|"system"
    QueueAction   string    `json:"queue_action"`   // "append" (default)
    Status        string    `json:"status"`          // "pending"|"consumed"
    Content       string    `json:"content"`
    ReceivedAt    time.Time `json:"received_at"`
}
```

### Multi-message and delayed sends

A run may complete with **multiple** assistant messages; each can have an optional delay before delivery. Delays are supported via an in-process scheduler. See [delayed-and-multi-message.md](delayed-and-multi-message.md) for the full contract (store, scheduler, tool, limits).

---

## Implementation Phases

### Phase 1 — Run Lifecycle Events on Existing Loop

**No UX changes. Establishes the event vocabulary everything else depends on.**

| Action | File | Detail |
|--------|------|--------|
| NEW | `internal/runtime/run.go` | `RunState` struct, `RunStatus` enum, lifecycle event emission via Bus |
| MODIFY | `internal/schema/event.go` | Add `RunID`, `Visibility` to `Event` (as `omitempty`); new `EventType` constants; payload structs; keep `SchemaVersion "1.0.0"` during migration window |
| MODIFY | `internal/navi/loop.go` | Instrument `processTurn`: emit `run.started` at entry, `tool.call.started`/`completed` around `executeTool`, `run.completed`/`run.failed` at all exit paths |

**Schema migration window:**
1. Add `RunID`/`Visibility` as `omitempty`, keep SchemaVersion `"1.0.0"`
2. Deploy, verify existing consumers work with new fields present
3. Bump to `"2.0.0"`, update version check to accept both `"1.0.0"` and `"2.0.0"`
4. Drop `"1.0.0"` acceptance after all consumers are updated

**Exit criteria:** `run.started` → `tool.call.*` → `run.completed` events appear in SQLite event log and are visible through `LiveHandler`.

### Phase 2 — RunCoordinator Replaces AgentLoop Outer Loop

**The structural refactor. RunCoordinator becomes the primary loop; AgentLoop becomes CognitiveExecutor.**

| Action | File | Detail |
|--------|------|--------|
| NEW | `internal/runtime/coordinator.go` | RunCoordinator: owns wake channel, session dispatch, one-foreground-run enforcement. `Run()` method replaces `AgentLoop.Run()`. `StartRun()` creates a `RunState`, emits `run.started`, calls `CognitiveExecutor.ExecuteTurn()`, emits `run.completed`/`run.failed`. |
| NEW | `internal/runtime/inbox.go` | Thin `InboxItem` struct. `Accept()` normalizes input from any source, assigns ID, persists to SQLite, emits `message.received`. |
| RENAME | `internal/navi/loop.go` → `internal/navi/executor.go` | `AgentLoop` renamed to `CognitiveExecutor`. Loses wake channel and `Run()` method. Exports `ExecuteTurn(ctx, sessionID) error` as the entry point called by RunCoordinator. |
| MODIFY | `internal/navi/config.go` | Split `LoopConfig` into `CognitiveConfig` (LLM, Persona, Skills, Governor, WorldModel, Experience, callbacks) |
| NEW | `internal/runtime/config.go` | `RuntimeConfig` struct: Bus, SessionStore, CognitiveExecutor reference, inbox persistence |
| MODIFY | `internal/gateway/server.go` | `handleNaviSendMessage` creates an `InboxItem` via `RunCoordinator.Accept()` instead of calling `SendMessage` + `Wake()` directly |
| MODIFY | `cmd/navid/main.go` | Wire `RunCoordinator` as the primary loop; pass `CognitiveExecutor` as a dependency |

**Refactor detail — what moves where:**

```
FROM AgentLoop (internal/navi/)          TO RunCoordinator (internal/runtime/)
─────────────────────────────────────    ─────────────────────────────────────
wakeCh chan struct{}                  →   wakeCh chan struct{}
Run(ctx) error (select loop)         →   Run(ctx) error (select loop + inbox dispatch)
Wake()                               →   Wake() + Accept(InboxItem)

STAYS in CognitiveExecutor (internal/navi/)
─────────────────────────────────────────────
processTurn → exported as ExecuteTurn(ctx, sessionID) error
executeTool (all governance, proposal, failure handling)
shapeReply, emitReflect
LLM multi-turn for-loop
Provider/model intent classification
All LoopConfig cognitive fields → CognitiveConfig
```

**Interrupt plumbing:**

RunCoordinator provides a `ShouldInterrupt` function via context or a callback injected into CognitiveExecutor. CognitiveExecutor checks it at two natural boundaries inside its LLM loop:

1. Before each LLM call (current line ~304 in `loop.go`)
2. Before each `executeTool` call (current line ~373 in `loop.go`)

If `ShouldInterrupt()` returns true, CognitiveExecutor returns a sentinel error (`runtime.ErrInterrupted`). RunCoordinator handles the interrupt: emits `run.cancelled` or `run.paused` depending on interrupt class.

**Exit criteria:** Messages flow through `InboxItem` → `RunCoordinator` → `CognitiveExecutor.ExecuteTurn()`. Event log shows `message.received` → `run.started` → `run.completed` chain. All existing `AgentLoop` tests pass against the new structure (test helpers updated to wire through RunCoordinator).

### Phase 3 — Bidirectional WebSocket + Streaming

| Action | File | Detail |
|--------|------|--------|
| MODIFY | `internal/gateway/live.go` | Evolve `LiveHandler` to parse incoming frames as `{ "type": "req", "id": N, "method": "...", "params": {...} }`. Support methods: `sendMessage`, `cancelRun`, `resolveProposal`. Route writes through same logic as HTTP handlers. Keep 300ms polling for event fanout. Add `replay_from_seq` on reconnect. |
| NEW | `internal/runtime/ws_protocol.go` | Frame type definitions, method dispatch table, response formatting. Request frames: `{ "type": "req", "id": N, "method": "...", "params": {...} }`. Response frames: `{ "type": "res", "id": N, "ok": bool, "payload": {...} }`. Event frames: `{ "type": "event", "event": "run.started", "payload": {...} }`. |

**Exit criteria:** PET/CLI can send messages and receive streaming events over a single WebSocket connection. HTTP endpoints continue working for non-interactive clients.

### Phase 4 — Proposal + Failure Runtime Integration + Checkpoints

| Action | File | Detail |
|--------|------|--------|
| MODIFY | `internal/runtime/coordinator.go` | Run pauses when CognitiveExecutor returns `ErrProposalRequired`. Emits `run.paused` + `proposal.waiting`. Proposal resolution resumes the run via `ResumeRun()`. Emits `run.resumed`. |
| NEW | `internal/runtime/checkpoint.go` | `CheckpointStore` interface backed by SQLite. Captures: run phase, pending proposals, interruption reason, partial assistant message snapshot, pending inbox items. |
| MODIFY | `internal/schema/event.go` | Add event types: `run.paused`, `run.resumed`, `proposal.waiting`, `proposal.resolved`, `governance.blocked`, `recovery.required`, `recovery.resolved`. |

**Resume safety rules:**
- A run may resume only if: checkpoint exists, dependencies resolved, no superseding run invalidated it, external side-effect replay is safe under existing idempotency rules.
- Checkpointing matters here because runs can now be paused mid-execution waiting on proposal approval.

**Exit criteria:** Blocking proposals pause and resume runs. Failures emit appropriate degradation events. Checkpoint exists for paused runs. Never pretend rollback happened for irreversible external work.

### Phase 5 — Multi-Surface Renderers + Queue Classifier

| Action | File | Detail |
|--------|------|--------|
| NEW | `internal/runtime/classifier.go` | Full queue classifier with merge/steer/interrupt/defer/supersede actions. Built after a second input source (e.g., Telegram + Web) creates real race conditions. |
| MODIFY | `internal/runtime/inbox.go` | Expand `InboxItem` with `dedupe_key`, `classification.confidence`, burst merge windows (text: 2–5s, messaging: 5–8s, voice: transcript segments). |
| MODIFY | `internal/schema/event.go` | Add event types: `message.merged`, `message.deferred`, `message.superseded`, `interrupt.raised`, `interrupt.applied`. |

**Queue action taxonomy:**
- **append** — normal follow-up or independent next item
- **merge** — combine bursty messages into one intake bundle
- **steer** — redirect an in-progress run without abandoning it
- **interrupt** — pause/cancel/switch active work
- **defer** — hold until a safe boundary
- **supersede** — replace stale pending input with newer intent

**Hard rule:** No prompt should decide queue semantics. Queue action is runtime logic.

**Exit criteria:** Same session receives messages from two surfaces simultaneously. Queue classifier correctly merges/defers/interrupts.

### Phase 6 — Optimization + Hardening

- Stream compaction and semantic block streaming (paragraph/code block completion events)
- Background run support (detached from foreground)
- Replay tooling and `navi trace` command
- Chaos testing: broker restart, WebSocket drops, slow consumers, tool timeouts, duplicate event replay
- Latency benchmarking against D9 targets

---

## Migration Path from Current Code

| Today's Code | What Changes | When |
|---|---|---|
| `AgentLoop.Run()` owns wake channel and session dispatch | Wake channel and dispatch move to `RunCoordinator.Run()` | Phase 2 |
| `AgentLoop.processTurn()` is the implicit run coordinator | Renamed to `CognitiveExecutor.ExecuteTurn()`, called by RunCoordinator | Phase 2 |
| `handleNaviSendMessage` appends message + calls `Wake()` | Creates `InboxItem`, emits `message.received`, routes through `RunCoordinator.Accept()` | Phase 2 |
| `executeTool` handles governance inline | Emits `tool.call.started`/`completed`, `governance.blocked` events around existing logic | Phase 1 |
| `LiveHandler` polls SQLite at 300ms, unidirectional | Add bidirectional frame handling | Phase 3 |
| `shapeReply` runs on final content | Continues shaping final content; token deltas stream raw | No change (confirmed Phase 3) |
| `schema.Event` envelope | Add `RunID`, `Visibility`, new event types | Phase 1 |
| `LoopConfig` holds all dependencies | Split into `RuntimeConfig` + `CognitiveConfig` | Phase 2 |
| `cmd/navid/main.go` wires `AgentLoop` | Wire `RunCoordinator` with `CognitiveExecutor` as dependency | Phase 2 |

### Step-by-Step Migration (Phase 2)

1. Create `internal/runtime/` package with `coordinator.go`, `run.go`, `inbox.go`, `config.go`
2. Move wake channel and `Run()` loop from `AgentLoop` to `RunCoordinator`
3. Rename `AgentLoop` to `CognitiveExecutor`, export `ExecuteTurn`
4. Split `LoopConfig` into `RuntimeConfig` + `CognitiveConfig`
5. Update `cmd/navid/main.go` to wire `RunCoordinator` → `CognitiveExecutor`
6. Update `handleNaviSendMessage` to call `RunCoordinator.Accept()`
7. Run all existing tests — they must pass against the new structure
8. Add `internal/runtime/` unit tests for run lifecycle and inbox

---

## Verification Plan

### Automated Tests

```bash
# Existing loop tests must not regress (renamed executor)
go test ./internal/navi/ -run TestCognitiveExecutor -v

# New runtime package
go test ./internal/runtime/ -v

# Event schema (new types, validation)
go test ./internal/schema/ -v

# Integration: end-to-end message → run → stream
go test ./internal/gateway/ -run TestLiveHandler -v
```

### Manual Verification

1. Send a message via PET → verify `run.started`, `tool.call.*`, `run.completed` in `/api/debug/events`
2. Connect WebSocket to `/ws/live` → verify events stream in real-time
3. Trigger a proposal-requiring tool → verify `run.paused` + `proposal.waiting` events (Phase 4)
4. Send messages from two surfaces → verify queue classifier behavior (Phase 5)

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Schema version bump breaks live consumers | Migration window: add fields `omitempty` first, bump version second |
| Phase 2 refactor breaks existing tests | Run full test suite after each migration step; keep `CognitiveExecutor` API compatible |
| Building queue classifier before real multi-input workload | Deferred to Phase 5 — build it when Telegram + Web are both active |
| Checkpoint complexity creeps into early phases | Checkpoints only needed in Phase 4 when runs can actually pause |
| Connector-specific shortcuts bypass inbox | Hard rule: all input goes through `InboxItem`, no exceptions |
| Partial streamed content treated as final | `assistant.message.completed` is the only source of truth; partial states are clearly labeled |
| Policy decisions expressed in prompt instead of code | Queue action is runtime logic, never prompt logic |
