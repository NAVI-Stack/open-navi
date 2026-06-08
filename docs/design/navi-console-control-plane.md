# NAVI Console Control Plane Direction

## Status

Draft. This document captures the durable product and UX direction for the NAVI Console redesign. It is intended to guide phased implementation after, and in tandem with, the core cleanup that retires legacy Session terminology in favor of Chat and RuntimeSession.

## Product statement

NAVI Console is the owner/admin interface and AI workspace control plane for NAVI.

It supports simple local AI chat, project-centered work, runtime inspection, plugin/capability management, and full operator control. It should feel clean enough for normal daily use, but deep enough for power users and admins to inspect, configure, and govern the whole NAVI system.

## North star

The Console is not only a chatbot UI and not only a raw admin panel. It must support three overlapping modes:

- Regular user mode: chat, projects, files/artifacts, and simple controls.
- Power user mode: runtime state, runs, approvals, context, artifacts, and project workbench controls.
- Admin/operator mode: configuration, connectors, plugins, logs, debug, governance, system health, and owner-level control.

The UI should preserve the admin/control capability layer without making the everyday interaction feel like a rough prototype dashboard.

## Canonical domain model

The Console should reinforce the core domain model produced by the chat/runtime cleanup:

```text
Project
  ├─ Chat(s)              canonical durable conversation/thread + messages
  ├─ RuntimeSession(s)    execution lifetimes tied to chat/project/context
  ├─ Run(s)               individual execution attempts
  ├─ Task(s)              durable project work units
  └─ Workspace/Sandbox    file/repo/resource boundary
```

Terminology rules:

- Chat = what was said and remembered.
- RuntimeSession = what is or was executing.
- Run = one execution attempt inside a runtime lifecycle.
- Project = why/where the work belongs.
- Workspace = what files/resources are allowed.
- Session = retired legacy term. Do not expose it in new user-facing UI.

Do not blindly rename `session_id` to `chat_id`. Classify each backend field before mapping it into UI state:

- Conversation transcript/history -> `chat_id`.
- Execution lifecycle/run coordination -> `runtime_session_id`.
- Project/work boundary -> `project_id`.
- Workspace/filesystem boundary -> `workspace_id`.

## Layout model

The preferred shell is:

```text
Left rail  = navigation + workspace organization
Main pane  = active work surface
Right pane = collapsible runtime/control/context inspector
Bottom     = optional developer/operator drawer, not always-on
```

### Left rail

The left rail should feel closer to modern AI apps while preserving admin depth through clean grouping.

Recommended long-term grouping:

```text
Top
  New Chat
  Active project / project switcher

Workspace
  Projects
  Recent Chats / History
  Tasks
  Artifacts

Control
  Runs
  Approvals
  Runtime

Extensions
  Plugins
    Skills
    Connectors
    LLM Providers
    Workflows

System
  Memory / Knowledge
  Settings
  Config
  Debug
```

MVP grouping:

```text
Projects
Chats / Recent
Runs
Plugins
Settings
Debug
```

Projects should behave like organized work scopes. Recent chats should be sorted by last activity, similar to ChatGPT, Claude, Grok, and other modern AI apps.

### Main pane

The main pane renders the active route/work surface:

```text
/chats/:id          chat surface
/projects/:id       project dashboard/workbench
/runs/:id           run inspection
/plugins            plugin manager
/settings           admin settings
/debug              operator/debug surface
```

For chat, keep the default surface simple:

```text
Header:
  chat title
  project badge
  model/provider indicator
  experience/autonomy mode
  runtime status

Body:
  message timeline

Composer:
  prompt input
  attach/context controls
  mode selector
  send/run control
```

Advanced details should expand on demand instead of cluttering the timeline by default.

### Right pane

The right pane is the contextual control plane. It must be collapsible and, ideally, resizable.

For a chat, it may show:

```text
Runtime
Context
Artifacts
Approvals
Files / Workspace
Memory
Debug details
```

For a project, it may show:

```text
Readiness
Workspace binding
Active chats
Recent runs
Tasks
Project memory
```

For a run, it may show:

```text
Phase
Tool calls
Errors
Checkpoints
Artifacts
Proposal state
```

The owner should be able to chat normally with the pane closed, then open it to inspect or steer NAVI like a power tool.

### Bottom drawer

Do not make logs/runs/events/artifacts a permanent default bottom panel. The app already has a left rail, main pane, and right inspector; a permanent bottom panel would push the UI too far toward an IDE/debugger layout for everyday use.

Preferred rule:

```text
Default mode: no bottom panel
Developer/operator mode: optional bottom drawer
Run/debug pages: bottom drawer allowed
```

The bottom drawer can later hold:

```text
Logs
Events
Run trace
Terminal-like output
Streaming tool output
```

## Visual direction

The Console should feel like:

```text
ChatGPT / Claude cleanliness
+ Cursor-style agent visibility
+ OpenClaw-style operational control
+ polished local admin panel
```

Keep OpenClaw's versatility, not its roughness or density.

Practical visual rules:

- Clean defaults.
- Progressive disclosure.
- Collapsible sections.
- Strong active states.
- No raw tables unless inspecting/debugging.
- Cards for summaries.
- Tables for admin lists.
- Resizable/collapsible inspector.
- Command palette later, not as MVP scope.

## Redesign phases

### Phase 1 — Usability shell

Goal: make the Console coherent before deep redesign.

Scope:

- New app shell.
- Clean left rail.
- Collapsible right pane.
- Route-aware main pane.
- User-facing Session terminology replaced with Chat where backend allows.
- Modern chat layout baseline.

### Phase 2 — Chat + project workbench

Goal: make the primary workflow strong.

Scope:

- Projects section.
- Recent chats/history.
- Project-scoped chats.
- Chat composer.
- Chat header controls.
- Project badge/context.
- Basic artifacts in right pane.

Primary workflow:

```text
Open project -> open chat -> send message -> see runtime status -> inspect output
```

### Phase 3 — Runtime/control inspector

Goal: expose NAVI's real power without clutter.

Scope:

- RuntimeSession inspector.
- Runs panel.
- Tool calls.
- Approvals/proposals.
- Context sources.
- Errors.
- Checkpoints.

### Phase 4 — Admin/system surfaces

Goal: full owner control.

Scope:

- Plugins.
- Skills.
- Connectors.
- LLM providers.
- Settings.
- Config.
- Memory.
- Debug.
- Logs/events.
- Governance.

### Phase 5 — Polish/customization

Goal: production-quality feel and durable user preference support.

Scope:

- Theme customization.
- Layout persistence.
- Density modes.
- Keyboard shortcuts.
- Command palette.
- Saved views.
- Operator/developer mode toggle.

## Non-goals for the first implementation pass

- Do not redesign every route at once.
- Do not surface every backend object in the primary navigation.
- Do not keep both Sessions and Chats as user-facing concepts.
- Do not make the bottom drawer permanent by default.
- Do not turn the chat pane into a debug log.
- Do not overfit the shell to current transitional backend naming.

## Open questions

- Should the default route be `/chats`, `/projects`, or the last active project/chat?
- Should Runtime be a first-class left-rail item or mainly an inspector surfaced from Runs/Chats?
- How should the Console expose owner/admin mode versus regular chat mode: explicit toggle, persisted layout preference, or progressive disclosure only?
- What is the minimum artifact preview experience needed for Phase 2?
- How much of theme customization should land before the rest of admin/system surfaces?

## Source-of-truth policy

GitHub repo docs are canonical for implementation direction.

Notion may be used as a live thinking and iteration space. Google Drive may be used for polished conceptual summaries or external reference material. Final decisions that affect implementation should be mirrored back into GitHub docs.
