# NAVI Console Shell and Layout Design

## Status

Draft. This document defines the high-level shell and layout direction for NAVI Console V2. It is intentionally frontend/UX-focused and avoids detailed component tickets until the backend core cleanup stabilizes.

## Purpose

The shell must support three ways of using the Console:

- normal local AI chat,
- project-centered AI workspace control,
- admin/operator control.

The layout should make simple chat feel clean while keeping runtime and admin power one layer away.

## Layout principle

The Console shell should be:

```text
Left rail  +  Main pane  +  Collapsible right inspector  +  Optional bottom drawer
```

Each region has a clear job:

| Region | Job |
| --- | --- |
| Left rail | Navigation, project/chat organization, primary mode switching |
| Main pane | Active work surface: chat, project, run, plugin, settings, debug |
| Right inspector | Contextual runtime/control plane details |
| Bottom drawer | Optional developer/operator trace/log/event output |

Do not make all regions equally loud. The default experience should emphasize the left rail and main pane, with the right inspector available when useful.

## Region behavior

### Left rail

The left rail should be stable across the app.

Responsibilities:

- New Chat action.
- Active project selector or context summary.
- Projects navigation.
- Recent chats/history navigation.
- Runs navigation.
- Plugins navigation.
- Settings and Debug navigation.

The rail may contain collapsible groups. It should not become a long list of every backend object.

Preferred MVP groups:

```text
Workspace
  Projects
  Chats / Recent

Control
  Runs
  Plugins

System
  Settings
  Debug
```

Future groups can expand, but only when there is enough UX justification.

### Main pane

The main pane is route-specific and should feel like the user's primary workspace.

Examples:

```text
Chat route     -> message timeline and composer
Project route  -> project workbench
Run route      -> execution inspection
Plugins route  -> capability management
Settings route -> admin settings
Debug route    -> diagnostics
```

The main pane should not be forced into a chat layout for every route. It is the active work surface, not always the chat surface.

### Right inspector

The right inspector is the main control-plane affordance.

Requirements:

- Collapsible.
- Route-aware.
- Does not block normal chat use when closed.
- Preferably resizable later.
- Should persist open/closed state locally.
- Should show human-readable summaries before raw payloads.

For chat routes, likely sections:

```text
Runtime
Context
Artifacts
Approvals
Workspace
Memory
Debug
```

For project routes, likely sections:

```text
Readiness
Workspace Binding
Active Chats
Recent Runs
Tasks
Project Memory
```

For run routes, likely sections:

```text
Phase
Tool Calls
Errors
Checkpoints
Artifacts
Proposal State
```

Raw JSON should be an explicit debug expansion, never the default body of the inspector.

### Bottom drawer

The bottom drawer is optional and should not be present by default.

Use cases:

- logs,
- events,
- run trace,
- terminal-like output,
- streaming tool output,
- development/operator diagnostics.

The drawer should come later after the core shell and right inspector are stable.

Rule:

```text
Default user mode: no bottom drawer
Power/admin mode: bottom drawer available by toggle
Debug/run routes: bottom drawer may be route-enabled
```

## Layout states

### Clean chat state

```text
Left rail open
Main chat pane active
Right inspector collapsed
No bottom drawer
```

Purpose: regular local AI chat.

### Control-plane state

```text
Left rail open
Main pane active
Right inspector open
No bottom drawer
```

Purpose: chat/project/run with runtime/context visibility.

### Operator/debug state

```text
Left rail open or compact
Main pane active
Right inspector open
Bottom drawer open
```

Purpose: debug, logs, traces, failures, tool output.

### Focus state

```text
Left rail compact or hidden
Main pane emphasized
Right inspector collapsed
No bottom drawer
```

Purpose: long chat, writing, review, or focused work.

Focus state can be deferred.

## Responsive behavior

Desktop first, but avoid hard-coding the shell into an unusable narrow layout.

High-level rules:

- On wide screens, left rail + main pane + optional right inspector is preferred.
- On medium screens, right inspector should overlay or collapse by default.
- On small screens, left rail and right inspector should be drawer-style overlays.
- The composer should remain usable on narrow widths.
- Main pane should never be squeezed below practical reading width by optional panels.

## Persistence

Initial persistence should be minimal:

```text
right inspector open/closed
left rail collapsed/expanded later
bottom drawer open/closed later
preferred density later
preferred theme later
```

Use local preference storage first unless/until NAVI owner preferences are formalized in backend/user settings.

## Visual density

The Console should support progressive density:

```text
Default: clean, readable, modern AI app
Power: more context visible, inspector open
Operator: dense tables/logs/debug available
```

Do not make the default state dense just because the system has many admin capabilities.

## Chat layout baseline

Chat route baseline:

```text
Header
  title
  project badge
  runtime status
  model/provider indicator when available
  mode/autonomy indicator when available

Timeline
  user messages
  assistant messages
  tool/run summary chips where useful

Composer
  input
  send
  context/project affordances later

Right inspector
  runtime/context/artifacts/approvals/debug
```

The chat timeline should stay clean. Tool calls, raw payloads, and run internals should be summarized or moved into the inspector.

## Project layout baseline

Project route baseline:

```text
Header
  project title
  status/health
  workspace binding summary

Overview
  readiness cards
  recent chats
  tasks
  recent runs
  artifacts later

Right inspector
  readiness details
  workspace
  active chats
  project memory
```

Project is the workbench, not just a database record page.

## Run layout baseline

Run route baseline:

```text
Header
  run id/status
  linked chat
  linked project
  runtime session

Main detail
  phase
  timeline/trace summary
  tool calls
  proposals
  errors
  artifacts

Right inspector
  checkpoints
  raw debug expansion
  related context
```

Runs are execution attempts, not chats.

## Plugins/admin layout baseline

Plugins route baseline:

```text
Plugins overview
  enabled/disabled state
  capability groups

Subsections
  Skills
  Connectors
  Providers
  Workflows
```

Settings/debug route baseline:

```text
Settings
  user/owner configuration
  model/provider preferences
  appearance/theme later
  governance/config later

Debug
  events
  logs
  runtime metrics
  connector state
  LLM routing/context traces later
```

## Phase mapping

### Phase 1 shell

- Stabilize left rail labels.
- Rename Sessions-facing UI to Chats/History.
- Add collapsible right inspector behavior.
- Move raw JSON behind debug expansion.
- Preserve existing chat/project functionality.

### Phase 2 workbench

- Improve chat route layout.
- Improve project workbench layout.
- Add project-scoped chat flow.
- Add basic runtime status and artifact/run discovery.

### Phase 3 inspector

- Flesh out runtime/control inspector sections.
- Improve run detail pages.
- Add Chat/Project/Run cross-links.

### Phase 4 admin organization

- Reorganize Plugins.
- Improve Settings/Config/Debug hierarchy.
- Add governance/approval/admin surfaces where appropriate.

### Phase 5 polish

- Layout persistence.
- Density modes.
- Theme customization.
- Command palette.
- Optional bottom drawer.

## Non-goals for shell work

- Do not build the bottom drawer before the right inspector works.
- Do not rebuild every route at once.
- Do not make raw JSON the main representation.
- Do not expose legacy Session terminology.
- Do not make RuntimeSession top-level unless later evidence requires it.
- Do not overfit the shell to transitional backend compatibility fields.

## Open questions

- Should inspector state persist globally or per route?
- Should chat and project routes share the same inspector component or only the shell region?
- Should the project switcher be always visible or only in Workspace/Project sections?
- Should Focus state be part of V2 MVP or deferred to polish?
- Should visual density be explicit in settings or implicit through inspector/debug choices?
