# NAVI Console

**Status:** Evolving  
**Last Updated:** 2026-05-02  
**Updated By:** ChatGPT

NAVI Console is the backend-hosted, agent-first control surface for the NAVI runtime. It is not PET and should not become PET 2.0. PET remains the polished user-facing shell. NAVI Console is the local control plane for runtime clarity, debugging, inspection, onboarding, and development.

Default local URL:

```txt
http://127.0.0.1:6284/
```

The console should be served by `navid` and should default to loopback-only access. Any non-loopback exposure must be explicit and gated by stronger auth, origin checks, and transport security.

---

## Product Boundary

NAVI Console answers operational questions:

- Is NAVI running?
- What is NAVI doing?
- Which session, channel, or scheduled job triggered this run?
- Which capabilities are available, gated, degraded, or blocked?
- What did NAVI try to execute?
- What failed, partially succeeded, or timed out?
- What needs owner approval?
- What configuration or appearance preference is currently active?

NAVI Console is closer to an agent runtime control UI, local admin panel, and session/debug viewer than to a companion frontend. It may be visually polished, but runtime inspection is the priority.

PET remains responsible for the high-level daily user experience, conversational shell, user-facing presence surfaces, and future mobile/desktop ergonomics.

---

## Design References

OpenClaw is a useful reference point because its control UI is backend-owned and exposes runtime-centric concepts such as chat, sessions, channels, cron jobs, skills, config, debug tools, and logs.

NAVI should borrow the control-plane ergonomics, not the product identity. NAVI Console must map those patterns into NAVI's own architecture: Experience Layer, Cognitive Layer, World Model, Capability Layer, Governance, Proposal Queue, Skills, Connectors, Plugins, and execution outcomes.

---

## Initial Information Architecture

Recommended V1 navigation:

```txt
NAVI Console
├── Overview
├── Chat
├── NAVI
├── Sessions
├── Projects
├── Workspaces
├── Channels
├── Scheduler
├── Skills / Extensions
├── Proposals
├── Usage
├── Config
├── Appearance
├── Debug
├── Logs
└── Docs
```

This is intentionally broad as a product map, but implementation should be phased. Overview, Chat, Sessions, Logs, Debug, and Config should land before lower-priority polish.

---

## Page Responsibilities

### Overview

Landing page for runtime health and operator awareness.

Show:

- daemon status
- local bind address and port
- auth mode
- active model/provider
- active session and run
- active mode/persona
- autonomy preset
- connected channels
- scheduler state
- pending proposals
- recent failures and degraded capabilities

### Chat

Developer/operator chat pane for directly interacting with NAVI through the backend runtime.

Show:

- streamed assistant output
- active run ID
- selected session
- tool/skill call cards
- proposal interruptions
- command/result summaries
- abort control
- verbose/debug toggle

The Chat pane should make every meaningful runtime action inspectable. It should not hide tool calls, failed execution, proposal gating, or degraded capability behavior.

### NAVI

NAVI-specific runtime identity and state page. This replaces generic “Agents” terminology.

Show:

- NAVI identity/name
- mode/persona
- provider/model
- cognitive loop status
- memory/world-model status
- active tasks/runs
- capability registry snapshot
- degraded capabilities
- last heartbeat
- current workspace/project context, if any

### Sessions

Runtime session inventory and inspection.

Show:

- session list
- active/inactive state
- source channel: console, PET, CLI, webhook, scheduler, external connector
- transcript preview
- active run
- token/context usage
- compaction state
- command outcomes
- archive/reset controls, if supported

Sessions are runtime evidence, not just chat history.

### Projects

Project-centered work control surface.

Show:

- project inventory
- lifecycle status and health
- workspace binding
- readiness for chat, planning, and coding
- project sessions
- project tasks
- topology from workspace to project to runtime work

Projects represent the work boundary. The console should make the active project visible without treating project state as an execution authority bypass.

### Workspaces

Workspace-centered execution boundary surface.

Show:

- global/scoped/hybrid operating mode
- active workspace selection
- local roots and repo roots
- protected paths
- allowed action deny-mask
- out-of-scope policy
- durable whitelist rules with revoke controls
- project binding

Workspaces represent the trust and resource boundary. The console may expose controls, but enforcement remains in runtime/governance code.

### Channels

Connector/channel state and routing.

Initial channel types:

- Console
- PET
- CLI
- Webhook
- existing repo-supported messaging connectors, when available

Show for each channel:

- enabled/disabled
- connection status
- auth/pairing status
- last inbound event
- last outbound event
- allowed identities
- session routing behavior
- degraded/error state

### Scheduler

Time-based and trigger-based execution surface.

Show:

- scheduled jobs
- enabled/disabled state
- next run
- last run
- run history
- target session/channel
- prompt or command payload
- failure status
- retry/deletion behavior

UI term should be `Scheduler`; implementation may use cron-like internals.

### Skills / Extensions

Capability inventory and governance surface.

Show:

- skill/plugin/extension ID
- display name
- version
- source: built-in, user, workspace, plugin
- trust tier
- risk tier
- enabled/disabled state
- requirements/gating result
- declared effects
- auth/environment requirements
- last invocation
- failure count

This page must expose why a capability is available, blocked, risky, or degraded.

### Proposals

Owner approval queue for governance-bound actions.

Show:

- pending proposals
- blocking vs queued priority
- source process
- source trigger
- proposed action
- affected entities
- rationale
- risk/confirmation reason
- approve/decline actions
- expiry
- resolution history

The Proposal Queue remains a first-class World Model entity. The console is only a presentation and action surface for it.

### Usage

Runtime and cost visibility.

Show:

- model calls
- token/context usage
- session usage
- tool/skill invocations
- scheduler runs
- failed/retried commands
- provider usage
- estimated cost, if available

Usage should help detect runaway loops, oversized contexts, and capability misuse.

### Config

Runtime configuration viewer/editor.

Show:

- structured form view
- raw config view
- source path
- validation status
- dirty state
- base revision/hash guard
- apply/restart requirement
- secret redaction
- failed validation details

Config editing must validate before applying. Secret values must never be casually exposed.

### Appearance

Small, controlled theme customization surface.

Purpose: allow the owner to define durable appearance preferences that NAVI can apply across future frontends, including PET.

Show:

- preset selector
- light/dark mode
- accent color
- semantic token editor
- preview cards
- export/import theme JSON
- reset to default
- apply-to-all-NAVI-surfaces toggle

Appearance settings should be stored as explicit/owner-set Configuration. Reflection, plugins, and inferred preferences must not silently override them.

Candidate shape:

```txt
configuration.appearance.theme
configuration.appearance.mode
configuration.appearance.accent
configuration.appearance.tokens
```

A future PET or mobile app should be able to fetch the owner's active theme from NAVI and apply it consistently.

### Debug

Developer inspection and manual runtime diagnostics.

Show:

- health snapshot
- provider/model snapshot
- event stream inspector
- last N events
- current run state
- command queue
- capability registry snapshot
- world model diagnostics
- active proposals
- failed validation events
- dev-only manual API call panel

Debug should optimize for truth over polish.

### Logs

Live operational log surface.

Show:

- live tail
- pause/resume
- filter by level
- filter by component
- search
- export
- copy run/session/command IDs
- links to related sessions/runs when possible

### Docs

Local, runtime-adjacent help surface.

Show:

- local repo docs
- API reference
- config reference
- skill authoring reference
- troubleshooting
- onboarding guide
- architecture overview
- contextual help links from console pages

Docs should help explain the running system, not become a separate product site.

---

## Candidate Backend Shape

NAVI Console should be served by `navid` from embedded static assets.

Candidate repo/runtime shape:

```txt
cmd/navid
  └── serves embedded console frontend assets

web/navi-console
  └── frontend source

internal/navi/http or internal/gateway
  ├── static asset server
  ├── REST endpoints
  ├── SSE or WebSocket event stream
  └── auth/session guard
```

Prefer Server-Sent Events first for runtime event streaming unless bidirectional streaming is clearly required. SSE is sufficient for status, logs, streamed output, proposal updates, and run events. Chat send/abort can remain normal HTTP actions.

---

## Candidate API Surface

This is a design target, not yet a ratified implementation spec.

```txt
GET  /api/v1/status
GET  /api/v1/events

GET  /api/v1/chat/history
POST /api/v1/chat/send
POST /api/v1/chat/abort

GET  /api/v1/navi
GET  /api/v1/sessions
GET  /api/v1/sessions/:id
PATCH /api/v1/sessions/:id

GET  /api/v1/channels
PATCH /api/v1/channels/:id

GET  /api/v1/scheduler/jobs
POST /api/v1/scheduler/jobs
PATCH /api/v1/scheduler/jobs/:id
POST /api/v1/scheduler/jobs/:id/run

GET  /api/v1/skills
PATCH /api/v1/skills/:id

GET  /api/v1/proposals
POST /api/v1/proposals/:id/approve
POST /api/v1/proposals/:id/decline

GET  /api/v1/usage
GET  /api/v1/config
PATCH /api/v1/config
POST /api/v1/config/apply

GET  /api/v1/appearance/theme
PATCH /api/v1/appearance/theme

GET  /api/v1/debug/snapshot
GET  /api/v1/logs/tail
GET  /api/v1/docs
```

When implementation begins, API routes that already exist in `gateway-api.md` should be reused or extended rather than duplicated.

---

## Security Baseline

Default behavior:

```txt
bind: 127.0.0.1
port: 6284
public access: disabled
auth: required after first-run
browser launch: optional
```

Non-loopback access must be explicit and should require:

- TLS or equivalent secure transport
- authenticated session/token
- origin checks
- CSRF protection for state-changing actions
- rate limits
- clear bind-address warning in the UI
- proposal gating for dangerous actions

The console must not become an authority bypass. It presents and invokes governed runtime operations; it does not skip governance.

---

## MVP Phasing

Recommended implementation order:

1. Serve NAVI Console on `127.0.0.1:6284`.
2. Add Overview.
3. Add Chat with event stream.
4. Add Sessions.
5. Add Logs.
6. Add Debug snapshot.
7. Add Config.
8. Add Skills / Extensions.
9. Add Scheduler.
10. Add Proposals.
11. Add Channels.
12. Add Appearance.
13. Add Docs.
14. Add Usage.

This order prioritizes runtime clarity before product polish.

---

## Non-Goals for V1

- replacing PET
- mobile-first UX
- rich companion/social UI
- broad multi-agent management
- public-hosted multi-tenant console
- full marketplace UX
- complex visual theme builder
- bypassing gateway/governance constraints

---

## Open Questions

- Should `6284` become the dedicated console port, or should the console share the existing gateway listener with a configurable console route?
- Should Chat use the same message/session model as PET from day one?
- Should the console frontend live under `web/navi-console`, `apps/navi-console`, or another repo convention?
- Should `/api/v1/events` use SSE first, WebSocket first, or reuse existing gateway streaming infrastructure?
- Which config values are editable in V1 versus read-only?
- What is the minimal auth flow for first-run local access?
- Should Appearance be persisted only as Configuration, or also reflected into an explicit Theme entity later?
- How should Docs ingest local Markdown without bloating the binary or exposing private paths?

---

## Acceptance Criteria for the Design Pass

Before implementation, the repo should have:

- this design document indexed under `docs/design/INDEX.md`
- a follow-up implementation spec only after current gateway/API constraints are reviewed
- an ADR if the team commits to port `6284` and backend-served embedded frontend assets as an architectural decision
- a task plan that separates static asset serving, API/event surfaces, and UI page implementation
