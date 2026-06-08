# NAVI Streaming UX + Messaging Runtime

## Comprehensive Design Synthesis and Implementation Plan

## 1. Executive Summary

NAVI should not implement streaming and message queueing as two separate features. They should become one unified runtime model:

* **Inbox-driven** on the way in
* **Run-coordinated** during execution
* **Event-native** internally
* **Stream-rendered** on the way out

That is the correct shape for an always-on, multi-surface agent.

The core shift is this:

**Messages are not prompt text. They are runtime inputs.**

Every inbound message should be normalized into a durable inbox item, classified, arbitrated against current session state, and then routed into a governed run lifecycle. Streaming then becomes the live projection of that runtime state to CLI, app, web, messaging connectors, and eventually voice.

This preserves NAVI’s architecture instead of fighting it:

* Experience remains presentation and delivery
* Cognitive remains reasoning and decision-making
* Capability remains execution
* Governance remains deterministic between Decide and Execute
* Skills remain governed capability contracts
* Proposal semantics remain first-class

The result is a runtime that feels alive without degrading into connector-specific hacks or prompt-driven queue logic.

---

## 2. What Problem This Solves

NAVI’s architecture is already stronger than many agent systems in governance, proposals, failure handling, autonomy, and skill structure. The weak point is the interaction runtime.

Current gap areas:

* one-turn-at-a-time feel under bursty input
* no canonical session inbox
* no explicit queue arbitration model
* no standard event envelope for all runtime states
* token streaming without enough runtime semantics
* limited mid-run steering and interruption behavior
* no fully specified pause/resume contract
* no unified multi-surface rendering contract

If this is not fixed, NAVI risks becoming a strong internal architecture wrapped in a clunky outer loop.

---

## 3. Design Principles

### 3.1 Messages are runtime inputs

Inbound messages must be durable runtime objects with metadata, not raw text shoved directly into the agent loop.

### 3.2 Streaming is a state projection, not just token output

Users need more than token deltas. They need visibility into:

* run started
* run paused
* tool running
* proposal waiting
* recovery required
* failure state
* final completion

### 3.3 One foreground run per session

A session can support multiple pending inputs and detached background work, but it should have exactly one foreground run coordinator at a time.

### 3.4 Governance remains upstream of execution

Streaming must never bypass policy. Tools, proposals, interrupts, and retries still flow through deterministic governance and failure rules.

### 3.5 Proposal queue remains canonical

Do not build a second approval system into the streaming layer. The runtime only operationalizes proposal behavior during live interaction.

### 3.6 Connector-agnostic semantics

CLI, app, Telegram, SMS, and future voice surfaces should all render the same runtime semantics. Only density and affordances change.

### 3.7 Replayability is mandatory

If an event cannot be traced, replayed, and audited, it is not part of a production-grade always-on runtime.

---

## 4. Target Runtime Architecture

```text
Connectors / Surfaces
  -> Session Inbox Service
  -> Queue Classifier
  -> Run Coordinator
  -> Governance / Proposal / Failure hooks
  -> Skill + Tool Execution
  -> Event Bus
  -> Stream Renderers
  -> Trace Store / Checkpoints / Replay
```

### Runtime flow

1. A user or system event arrives from any surface.
2. It is normalized into an `InboxItem`.
3. The queue classifier determines how it interacts with current session state.
4. The run coordinator starts, steers, interrupts, defers, or supersedes work.
5. The active run emits canonical `NaviEvent` records.
6. Renderers convert those events into surface-specific UX.
7. Trace and checkpoint layers preserve replay and resume.

---

## 5. Core Runtime Components

## 5.1 Session Inbox Service

The canonical intake for all user-originated and system-originated conversational stimuli.

### Responsibilities

* accept normalized input from all connectors
* assign durable session-scoped IDs
* deduplicate retries and replays
* maintain pending queue state
* preserve audit history
* hand off classified items to the run coordinator

### Why it matters

Without a real inbox, NAVI stays turn-based even if streaming exists.

---

## 5.2 Queue Classifier

Determines what an inbound message should do relative to the active run.

### Queue actions

* **append** — normal follow-up or independent next item
* **merge** — combine bursty messages into one intake bundle
* **steer** — redirect an in-progress run without abandoning it
* **interrupt** — pause/cancel/switch active work
* **defer** — hold until a safe boundary
* **supersede** — replace stale pending input with newer intent

### Default burst merge window

* text surfaces: 2–5 seconds
* messaging connectors: 5–8 seconds
* voice: finalized transcript segments, not the same timer

### Hard rule

No prompt should decide queue semantics. Queue action is runtime logic.

---

## 5.3 Run Coordinator

The authoritative runtime controller for a session.

### Responsibilities

* manage run lifecycle
* enforce one foreground run per session
* arbitrate between foreground and detached background runs
* coordinate interrupts, pause/resume, checkpointing, and supersession
* bridge between inbox and cognitive execution

### Run lifecycle

```text
idle -> starting -> active -> paused -> resumed -> active -> completed | failed | cancelled
```

Additional substates:

* waiting_for_tool
* waiting_for_proposal
* waiting_for_recovery

---

## 5.4 Interrupt Policy Engine

Evaluates whether active execution can be altered.

### Interrupt classes

* **soft interrupt** — steer current run if safe
* **hard interrupt** — stop/cancel/switch task
* **governance interrupt** — proposal/policy/autonomy threshold boundary
* **state interrupt** — contradiction or stale state invalidation
* **system interrupt** — connector outage, crash, timeout, backpressure breach

### Safe interrupt boundaries

Allowed:

* before inference
* between cognitive phases
* before tool execution
* after normalized tool results
* during token streaming
* while waiting on proposal or connector

Not arbitrarily allowed:

* during irreversible side-effect commits
* during compensation execution
* during connector operations with no checkpoint support

### Critical rule

Never pretend rollback happened for irreversible external work.

---

## 5.5 Event Bus

The canonical internal state surface.

### Responsibilities

* publish all runtime events
* preserve event order per run
* fan out to renderers, operator tooling, trace, and metrics
* support replay and resume

### Principle

User-visible behavior should be derived from events, not ad hoc strings.

---

## 5.6 Stream Renderers

Convert canonical events into surface-specific experiences.

### Surface classes

**Rich interactive**

* app
* web
* desktop

**Semi-rich**

* CLI
* terminal UI

**Constrained messaging**

* Telegram
* SMS
* chat connectors

### Rule

All surfaces share the same underlying semantics. Only formatting and affordances change.

---

## 5.7 Trace Store and Checkpoint Store

### Trace Store

Append-only, immutable runtime history for:

* replay
* debugging
* audit
* incident analysis

### Checkpoint Store

Captures resumable state:

* run phase
* plan pointer
* partial assistant message snapshot
* pending proposals/recovery refs
* active command refs
* interruption reason
* pending inbox items

---

## 6. Canonical Runtime Data Models

## 6.1 InboxItem

```yaml
InboxItem:
  inbox_item_id: string
  session_id: string
  source_channel: enum[cli, app, web, telegram, sms, voice, system, schedule, proposal, recovery]
  source_message_ref: string|null
  received_at: timestamp
  actor_type: enum[user, owner, system, connector, proposal_system]
  payload_type: enum[text, transcript, command, signal, proposal_resolution, recovery_resolution]
  payload:
    text: string|null
    structured: object|null
  correlation_id: string|null
  priority: enum[foreground, normal, low, urgent]
  dedupe_key: string|null
  classification:
    queue_action: enum[append, merge, steer, interrupt, defer, supersede]
    confidence: float
  status: enum[pending, consumed, merged, deferred, superseded, expired]
```

## 6.2 RunState

```yaml
RunState:
  run_id: string
  session_id: string
  mode: enum[foreground, background]
  status: enum[idle, starting, active, paused, waiting_for_tool, waiting_for_proposal, waiting_for_recovery, completed, failed, cancelled]
  initiated_by_inbox_item_id: string
  current_phase: enum[intake, perceive, interpret, contextualize, decide, validate_govern, execute, reflect, stream_finalize]
  current_step_ref: string|null
  pause_reason: string|null
  interrupt_class: enum[none, soft, hard, governance, state, system]
  started_at: timestamp
  updated_at: timestamp
  latest_checkpoint_ref: string|null
  blocked_on:
    proposal_id: string|null
    command_id: string|null
    connector_id: string|null
  superseded_by_run_id: string|null
```

## 6.3 NaviEvent

```yaml
NaviEvent:
  event_id: string
  session_id: string
  run_id: string|null
  timestamp: timestamp
  event_type: string
  actor: enum[user, system, assistant, tool, governor, connector, renderer, subconscious]
  visibility: enum[user_visible, operator_visible, audit_only]
  correlation:
    message_id: string|null
    proposal_id: string|null
    command_id: string|null
    attempt_id: string|null
    skill_tool_name: string|null
  payload: object
```

### Required event types

* message.received
* message.classified
* message.merged
* message.deferred
* message.superseded
* run.started
* run.phase.changed
* run.paused
* run.resumed
* run.cancelled
* run.completed
* run.failed
* assistant.token.delta
* assistant.message.partial
* assistant.message.completed
* tool.call.started
* tool.call.progress
* tool.call.completed
* tool.call.failed
* governance.blocked
* proposal.created
* proposal.waiting
* proposal.resolved
* interrupt.raised
* interrupt.applied
* degradation.noted
* recovery.required
* recovery.resolved

---

## 7. Streaming UX Model

## 7.1 Three stream classes

### User Stream

Carries user-facing live state:

* token deltas
* partial assistant messages
* high-level tool progress
* proposal waiting state
* degradation notices
* recovery prompts
* run paused/resumed/completed state

### Operator Stream

Carries debugging and observability state:

* routing decisions
* queue classifications
* interrupt decisions
* governance decisions
* tool metadata
* retry activity
* backpressure and circuit breaker events

### Audit Stream

Carries canonical compliance-grade history:

* identifiers
* state transitions
* proposal refs
* command refs
* attempt refs

## 7.2 Streaming granularity

Required user-visible granularity:

* token delta stream
* status changes
* tool start/progress/complete
* proposal created/waiting/resolved
* run paused/resumed/completed/failed

**Token streaming alone is not enough.**

## 7.3 Partial message semantics

Allowed states:

* drafting
* streaming
* partial
* complete
* aborted

Never treat partially streamed content as final truth until the completion event lands.

## 7.4 Token and semantic block streaming

NAVI should stream at two levels:

* **token/chunk deltas** for responsiveness
* **semantic block events** for stable rendering of paragraphs, code blocks, or completed sections

Suggested defaults:

* flush every 50–200 ms or 5–10 tokens
* emit semantic block completions for longer outputs

## 7.5 Tool and skill streaming

Tools should emit:

* started
* progress
* completed
* failed

Each tool call must have a stable call ID and use canonical skill tool names.

---

## 8. Protocol and Transport Design

## 8.1 Primary transport

**WebSocket-first** for live sessions.

### Why

* bidirectional
* low-latency
* persistent session presence
* supports cancel/approve/steer mid-stream
* maps naturally to event fanout and multi-device sync

## 8.2 Fallbacks

* HTTP + SSE for simpler read-only clients
* long polling only as last resort
* signed webhooks for outbound integration notifications

## 8.3 WebSocket contract

The socket should support:

### Request frames

```json
{ "type": "req", "id": 1, "method": "sendMessage", "params": { ... } }
```

### Response frames

```json
{ "type": "res", "id": 1, "ok": true, "payload": { ... } }
```

### Event frames

```json
{ "type": "event", "event": "assistant.token.delta", "payload": { ... } }
```

### Required methods

* connect
* sendMessage
* cancelRun
* resolveProposal
* ackNotification
* resumeRun
* getPresence

### Required semantics

* auth on connect
* session binding
* idempotency keys for side-effecting requests
* replay from last seen event if reconnecting

---

## 9. Multi-Channel and Multi-Surface Model

NAVI should behave as one logical session across multiple active surfaces.

## 9.1 Channel bridges

Each bridge:

* translates native channel payloads into `InboxItem`
* subscribes to relevant `NaviEvent` streams
* renders or batches events appropriately

## 9.2 Deterministic session ordering

Use a single foreground session writer/coordinator to avoid channel interleaving chaos.

## 9.3 Per-channel constraints

Examples:

* Telegram: partial text okay, but rate-limited
* SMS: aggregate and batch
* voice: token stream to TTS, interrupt priority may be higher
* CLI/web/app: richest streaming surface

## 9.4 Multi-device sync

All active clients for the same session receive the same events.

## 9.5 Offline and batched channels

Some connectors should consume from durable queues and send summarized output later.

---

## 10. Broker and Infrastructure Recommendation

## 10.1 Recommended broker

**NATS + JetStream**

### Why this is the best fit now

* low operational overhead relative to Kafka
* excellent latency for interactive systems
* persistence via JetStream
* good fit for pub/sub plus worker queue hybrid
* good fit for fanout across many clients and adapters

### Keep optionality

Design the event abstraction so Kafka can be used later for higher retention or analytics-heavy pipelines.

## 10.2 Suggested deployment components

* WebSocket gateway / session service
* session inbox service
* run coordinator
* event bus abstraction backed by NATS/JetStream
* worker pool for tools and async jobs
* checkpoint store
* trace store
* metrics and dashboards
* scheduler / heartbeat service

## 10.3 Suggested topology

```text
Client/Connector
  -> WebSocket/API Gateway
  -> Session Inbox
  -> Run Coordinator
  -> Event Bus (NATS/JetStream)
     -> Renderers / adapters
     -> Trace / audit
     -> Workers
     -> Metrics / ops
```

---

## 11. Integration with Proposals, Failure, Skills, and Governance

## 11.1 Proposal integration

Proposal behavior must remain canonical.

Runtime rules:

* proposal creation emits `proposal.created`
* blocking proposal pauses foreground run
* async recovery proposal becomes detached recovery work
* approval resumes original process and revalidates before execution

## 11.2 Failure integration

Map runtime failures directly into existing failure semantics.

Required runtime behaviors:

* silent retry only for truly retry-safe cases
* advisory degradation as user-visible non-blocking state
* blocking failure pauses run and offers recovery path
* deferred recovery creates durable queued recovery state

Never:

* report full success on partial execution
* lose recovery state
* silently swallow runtime failures
* retry irreversible effects without explicit approval

## 11.3 Skill integration

Rules:

* use canonical tool names: `<skill_id>.<interface_name>`
* normalize all skill result envelopes before streaming upward
* stream tool activity through events, not connector-specific strings
* never allow skills to bypass governance or write directly to the World Model through runtime shortcuts

## 11.4 Governance integration

Streaming must not weaken policy.

Enforce:

* pre-execution policy checks
* proposal thresholds
* autonomy floors
* rate limits and quotas
* per-channel ACL and privacy policy
* multi-tenant isolation where relevant

---

## 12. Persistence, Replay, and Resume

## 12.1 Event persistence

Persist all canonical events before they are considered committed runtime state.

## 12.2 Replay

Support replay by:

* session
* run
* proposal
* command/attempt
* connector incident

## 12.3 Resume safety

A run may resume only if:

* checkpoint exists
* dependencies are resolved
* no superseding run invalidated it
* external side-effect replay is safe under existing idempotency and reversibility rules

## 12.4 World-model interaction

Streaming events should not directly mutate the World Model.

Instead:

* runtime events are recorded durably
* cognitive or post-processing flows decide what updates become knowledge, memory, history, or artifacts
* trace and audit remain first-class sources for reconstruction and analysis

---

## 13. Observability and Reliability

## 13.1 Required metrics

* inbox depth by session
* merge rate
* steer rate
* defer rate
* interrupt rate by class
* time to first token
* time to first useful event
* run completion rate
* proposal pause rate
* resume success rate
* tool failure rate by skill/connector
* event fanout latency
* backpressure incidents

## 13.2 Required traces

Filter by:

* session
* run
* proposal
* command/attempt
* skill tool name
* connector
* failure class

## 13.3 Test strategy

### Unit

* queue classification
* interrupt policy
* checkpoint serialization
* event rendering logic

### Integration

* end-to-end message to run to stream
* proposal pause/resume
* tool progress fanout
* reconnect and replay

### Load

* 10k concurrent connections target
* token latency and fanout pressure
* consumer lag thresholds

### Chaos

* broker restart
* WebSocket drops
* slow consumers
* tool timeouts
* duplicate event replay

### Security

* frame validation
* auth failures
* denied tool policy checks
* multi-tenant isolation tests

---

## 14. Developer Ergonomics

Needed from day one:

* typed Go and TypeScript event models
* protocol versioning
* local replay tool
* `navi trace` style inspection command
* debug UI for run/event history
* simulator for burst input and interrupt cases
* golden test fixtures for event sequences

---

## 15. Hard Product Priorities

This is the blunt ordering that matters.

### Build first

1. canonical event model
2. inbox model
3. run coordinator
4. queue arbitration
5. interrupt policy
6. checkpoint and replay

### Build after foundation

7. WebSocket gateway
8. token streaming
9. tool/proposal/failure rendering
10. CLI renderer
11. app/web renderer
12. messaging adapters

### Optimize last

13. semantic block streaming
14. stream compaction
15. background run tuning
16. queue heuristics tuning
17. rich operator tooling

If you start with “pretty token streaming,” you will build a nicer failure.

---

## 16. Recommended Implementation Phases

## Phase 1 — Contracts and Persistence

Build:

* `InboxItem`
* `RunState`
* `NaviEvent`
* trace store
* checkpoint store

**Exit criteria**

* events are durably persisted
* replay works at least by run ID

## Phase 2 — Arbitration and Run Control

Build:

* session inbox
* queue classifier
* interrupt policy engine
* foreground run lifecycle manager

**Exit criteria**

* merge/steer/interrupt/defer/supersede all behave deterministically

## Phase 3 — Event Streaming Core

Build:

* event bus
* user stream
* operator stream
* audit stream
* WebSocket gateway

**Exit criteria**

* time-to-first-token and status event streaming work end to end

## Phase 4 — Proposal and Failure Runtime Integration

Build:

* run pause for proposal creation
* proposal resolution resume path
* degradation and recovery event mapping
* deferred recovery handling

**Exit criteria**

* blocking and queued proposals both work live
* failures cannot disappear silently

## Phase 5 — Surface Renderers

Build:

* CLI/TUI renderer
* app/web renderer
* Telegram bridge
* condensed messaging renderer rules

**Exit criteria**

* same event semantics are visible on all target surfaces

## Phase 6 — Optimization and Hardening

Build:

* stream compaction
* chunking/backpressure tuning
* detached background run tuning
* replay tooling
* dashboards
* chaos coverage

**Exit criteria**

* system holds under stress and reconnect/replay scenarios

---

## 17. Migration Plan from Current Turn-Based Mode

## Step 1

Add canonical events behind the current runtime without changing user UX.

## Step 2

Introduce WebSocket session transport and token streaming in a dev-only path.

## Step 3

Move CLI to consume live events.

## Step 4

Add queue arbitration and mid-run interruption.

## Step 5

Integrate proposal/failure runtime semantics.

## Step 6

Enable one external connector for live streaming, likely Telegram after CLI/web prove stable.

## Step 7

Enable multi-device session sync.

## Step 8

Retire legacy turn endpoints gradually.

---

## 18. Recommended Decisions

These are the calls I would make now.

### Choose now

* **Event bus:** NATS + JetStream
* **Live session transport:** WebSocket-first
* **Core runtime posture:** inbox-driven, event-native, resumable by default
* **Session concurrency rule:** one foreground run per session
* **Interrupt rule:** never fake rollback for irreversible work
* **Proposal handling:** reuse existing Proposal Queue, do not fork semantics
* **Skill handling:** canonical tool names only, all tool activity streams as events

### Defer but track

* exact merge windows by connector
* voice-specific arbitration rules
* detached background run limits
* token chunk compaction policy under renderer backpressure
* connector classes allowed to approve which proposal tiers

---

## 19. Open Risks

* building transport before arbitration
* connector-specific shortcuts that bypass inbox or event semantics
* treating partial content as final content
* missing checkpoints around interrupts
* weak replay guarantees causing corrupted resume behavior
* policy decisions expressed in prompt instead of code
* over-optimizing token smoothness before run correctness

---

## 20. Final Recommendation

NAVI should adopt this runtime posture:

* **Event-native**
* **Inbox-driven**
* **Run-coordinated**
* **Governance-respecting**
* **Proposal-integrated**
* **Connector-agnostic**
* **Resumable by default**
* **Streaming more than text**

That is the version of NAVI that will actually feel like an always-on agent instead of a turn-based system with prettier output.

The key insight is simple:

**The real product is not token streaming. The real product is session arbitration plus visible state.**

Streaming is how that state becomes usable.
