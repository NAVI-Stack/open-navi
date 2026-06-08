---
name: navi-streaming-runtime-gap-closure
overview: Identify and close the main gaps between the current NAVI implementation and the intended inbox-driven, event-native streaming runtime design.
todos:
  - id: inbox-store-classifier
    content: Design and implement a durable InboxItem store plus a basic queue classifier with append/merge/defer/supersede semantics wired into RunCoordinator.Submit.
    status: completed
  - id: run-coordinator-ownership
    content: Refactor the session outer loop so RunCoordinator owns foreground run lifecycle and a richer RunState while AgentLoop focuses on cognitive execution.
    status: completed
  - id: interrupt-engine-checkpoints
    content: Add an interrupt policy engine and minimal checkpoint/resume support, emitting the corresponding interrupt and run lifecycle events.
    status: completed
  - id: ws-protocol-renderers
    content: Extend the WebSocket protocol to support bidirectional control and update CLI, web, and Telegram renderers to consume richer runtime events.
    status: completed
isProject: false
---

### NAVI Streaming Runtime Gap-Closure Plan

#### 1. Clarify current coverage vs. target design

- **Map existing concepts**: Confirm that `InboxItem` (in `internal/runtime/inbox.go`), `RunState` (in `internal/runtime/run.go`), and `Event`/fact types (in `internal/schema/event.go`) are the canonical homes for the spec’s `InboxItem`, `RunState`, and `NaviEvent` models.
- **Baseline behavior**: Treat the current AgentLoop-centric outer loop and `/ws/live` event feed as the baseline implementation to extend rather than replace wholesale.

#### 2. Inbox and queue arbitration

- **Inbox persistence**: Introduce a durable inbox store (SQLite table + minimal store API) keyed by `InboxItemID`, `ChatID`, and `RuntimeSessionID`, supporting `pending`/`consumed`/`merged`/`deferred`/`superseded` statuses.
- **Classification fields**: Extend `InboxItem` to include the richer metadata from the spec (source_channel, actor_type, payload_type, correlation_id, priority, dedupe_key, classification sub-object) while keeping defaults that preserve current behavior.
- **Queue classifier**: Add a queue classifier component that computes `queue_action` (`append`/`merge`/`steer`/`interrupt`/`defer`/`supersede`) from the current run state + recent inbox items; start with deterministic, non-LLM rules and emit new message.* events (classified/merged/deferred/superseded).

#### 3. Run coordinator and lifecycle ownership

- **Outer loop handoff**: Gradually move the outer wake/select loop from `AgentLoop.Run` into a `RunCoordinator.Run` in `internal/runtime`, turning the existing `RunCoordinator` from a thin wrapper into the single foreground run owner per session.
- **RunState enrichment**: Extend `RunState` with mode (foreground/background), richer status (paused, waiting_for_tool, waiting_for_proposal, waiting_for_recovery), phase, interrupt_class, and blocked_on refs as in the design.
- **Foreground/background arbitration**: Add explicit rules in `RunCoordinator` for one foreground run per session while allowing detached background work, and wire them to inbox queue actions (defer/supersede, etc.).

#### 4. Interrupt policy engine

- **Dedicated engine**: Introduce an interrupt policy module that evaluates incoming inbox items and operator actions against the current `RunState`, returning an interrupt class (soft/hard/governance/state/system) and allowed actions.
- **Safe boundaries**: Encode the spec’s safe vs. unsafe boundaries in code (before inference, between phases, before tool execution, etc.) and ensure they gate actual cancellations/pauses.
- **Events and integration**: Emit `interrupt.raised` / `interrupt.applied` events and integrate with existing governor/proposal/failure logic so we don’t duplicate or bypass policy.

#### 5. Event model completion and streaming semantics

- **Event vocabulary**: Fill in missing event types from the spec (e.g., `run.phase.changed`, `assistant.message.partial`/`completed`, `tool.call.progress`, `governance.blocked`, `recovery.`*) on top of the existing fact/event schema.
- **Visibility & streams**: Start using `Event.Visibility` and/or subjects to distinguish user-visible vs. operator vs. audit events, even if they still share the same `/ws/live` transport initially.
- **Semantic streaming**: Layer semantic message-block events on top of token chunks, so clients can render paragraphs/blocks with stable IDs without treating partial content as final.

#### 6. WebSocket protocol and bidirectional control

- **Versioned protocol**: Add a small, versioned WS protocol wrapper that can carry `req` / `res` / `event` frames while still encoding today’s envelopes inside `event` frames for backward compatibility.
- **Control methods**: Implement server handling for core methods (`sendMessage`, `cancelRun`, `resumeRun`, `resolveProposal`, `ackNotification`, `getPresence`) and route them into the inbox + run coordinator rather than directly into AgentLoop.
- **Session binding & idempotency**: Attach authenticated session context to the WS connection and support idempotency keys + replay-from-last-seq on reconnect.

#### 7. Checkpoints, replay, and resume

- **Checkpoint store**: Add a dedicated checkpoint store for resumable `RunState` snapshots that references the event log rather than duplicating it, and define the minimal fields needed for safe resume.
- **Resume semantics**: Introduce a guarded resume path in `RunCoordinator` that checks for superseding runs, unresolved dependencies, and irreversible side effects before allowing a run to continue.
- **Operator tooling**: Extend existing CLI and HTTP endpoints with simple “replay by run/session” views built on top of the current event log plus new checkpoints (a thin `navi trace`-style experience).

#### 8. Renderer hardening across surfaces

- **CLI and Telegram**: Incrementally teach the CLI and Telegram connectors to consume richer events (run status, tool lifecycle, basic progress) in addition to final `replied` events, without breaking current UX.
- **Web/app renderer**: Add a minimal TypeScript client that connects to `/ws/live`, understands the new event vocabulary, and renders a basic yet correct streaming UI (tokens + status + tools + proposals) in the existing `web/` shell.
- **Renderer abstraction**: Optionally introduce a thin renderer utility layer that maps canonical events into per-surface patterns, to avoid duplicating logic as more connectors are added.

#### 9. Prioritization aligned with the spec

- **Foundation first**: Prioritize (1) inbox + classifier, (2) run coordinator ownership + enriched RunState, (3) interrupt policy + minimal checkpoints, before investing in UI polish.
- **Transport second**: Once the runtime is deterministic, layer in the bidirectional WS protocol and improved renderers, keeping HTTP endpoints as the canonical API during migration.
- **Hardening last**: After behavior is correct across CLI and at least one external connector, invest in semantic streaming, compaction/backpressure tuning, richer operator views, and chaos/load testing focused on the new runtime.
