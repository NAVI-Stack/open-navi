# NAVI Console Information Architecture

## Status

Draft. This document defines the high-level information architecture for the NAVI Console V2 redesign. It intentionally avoids detailed implementation tickets while the backend `Session` -> `Chat` + `RuntimeSession` cleanup is still in progress.

## Purpose

The Console must remain the owner/admin panel for NAVI while also becoming a modern AI workspace control plane.

This document defines how the major product areas should be organized so the UI does not drift into either extreme:

- not a plain chatbot,
- not a noisy admin dashboard,
- not a kitchen-sink collection of debug pages.

## Product posture

NAVI Console supports three overlapping user postures:

```text
Regular user    -> chat, projects, basic context, artifacts
Power user      -> runs, runtime state, approvals, project workbench, plugins
Admin/operator  -> config, connectors, providers, debug, governance, logs
````

The same Console should serve all three without forcing all controls into the default view.

## Primary objects

The UI should organize around these objects:

| Object          | User-facing role                     | Navigation priority        |
| --------------- | ------------------------------------ | -------------------------- |
| Project         | Work scope and context boundary      | Primary                    |
| Chat            | Durable conversation/history surface | Primary                    |
| Run             | Inspectable execution attempt        | Primary/secondary          |
| Plugin          | Capability grouping                  | Secondary                  |
| Task            | Project-local work item              | Secondary/project-local    |
| Artifact        | Output/result, usually contextual    | Secondary/contextual       |
| Workspace       | File/repo/sandbox boundary           | Project/admin contextual   |
| RuntimeSession  | Execution lifecycle                  | Inspector/debug contextual |
| Settings/Config | Owner/admin control                  | Secondary/admin            |
| Debug/Event/Log | Operator diagnostics                 | Secondary/admin            |

RuntimeSession should not be presented like a Chat. It is an execution lifecycle that should normally appear through Chat, Run, Project, or Debug surfaces.

## Top-level navigation model

MVP top-level navigation:

```text
Projects
Chats / Recent
Runs
Plugins
Settings
Debug
```

## Expanded future navigation

```text
Top
  New Chat
  Active Project / Project Switcher

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

The expanded model should be implemented with collapsible groups, not a permanently long flat list.

## Left rail structure

The left rail should combine modern AI-app history behavior with admin/workspace navigation.

Recommended structure:

```text
Primary action
  + New Chat

Context
  Active Project selector

Workspace
  Projects
  Recent Chats / History

Control
  Runs
  Approvals

Extensions
  Plugins

System
  Settings
  Debug
```

Notes:

* Recent chats should sort by last activity.
* Projects should feel like organized work scopes/folders.
* A project can contain chats, tasks, artifacts, workspace binding, readiness, and runs.
* Plugins should group Skills, Connectors, Providers, and Workflows rather than exposing each as a top-level item.
* Debug should be available but not visually dominant.

## Route hierarchy

Recommended route model:

```text
/chats
/chats/:chatId
/projects
/projects/:projectId
/projects/:projectId/chats/:chatId
/runs
/runs/:runId
/plugins
/plugins/skills
/plugins/connectors
/plugins/providers
/settings
/debug
```

Optional future routes:

```text
/projects/:projectId/tasks
/projects/:projectId/artifacts
/artifacts
/approvals
/memory
/config
/debug/events
/debug/logs
/debug/runtime
```

Route rules:

* New work should use Chat, not Session.
* Project-scoped chat routes should preserve project context in the UI.
* Runs should link back to their Chat and Project when available.
* RuntimeSession should be visible through inspectors/details, not as a normal conversation destination.

## Primary workflows

### Local chat workflow

```text
Open Console -> New Chat or Recent Chat -> Send message -> Inspect runtime only if needed
```

Default user experience should be clean and familiar.

### Project work workflow

```text
Open Project -> Review readiness/workspace -> Open/create project chat -> Send message -> Inspect run/artifacts/tasks as needed
```

Project should be the workbench for long-running or scoped work.

### Runtime inspection workflow

```text
Open Chat or Run -> Open right inspector -> Review Runtime, Run, Tools, Approvals, Artifacts, Errors
```

Runtime inspection should be one layer deeper than normal chat.

### Admin/operator workflow

```text
Open Plugins/Settings/Debug -> Configure, inspect, repair, or govern NAVI
```

Admin surfaces should be strong and complete, but visually separated from everyday chat/project use.

## What should not be top-level by default

Avoid making every backend object a permanent top-level navigation item.

Do not make these top-level in the MVP unless later evidence demands it:

```text
RuntimeSession
Individual connector instances
Individual skills
Individual providers
Raw events
Raw logs
Artifacts as a mandatory permanent nav item
Memory internals
Config subcategories
```

These can appear as nested pages, inspector tabs, debug pages, or later advanced/admin navigation.

## Sidebar terminology rules

Use:

* Chats
* Recent
* Projects
* Runs
* Plugins
* Settings
* Debug

Avoid:

* Sessions
* Runtime Sessions as chat-like items
* Raw table names
* Backend package names
* Abbreviations that are unclear to non-developers

## Progressive disclosure model

Default surfaces should answer:

```text
Where am I?
What am I working on?
What is NAVI doing?
What can I do next?
```

Advanced surfaces should answer:

```text
What run is active?
What tools were called?
What context was used?
What failed?
What can I configure or govern?
```

The right inspector and debug pages should carry most of the advanced details.

## Phase mapping

### Phase 1

Focus:

* Rename Sessions-facing UI to Chats/History.
* Stabilize left rail.
* Add/correct collapsible right pane behavior.
* Make chat/project/run terminology coherent.

### Phase 2

Focus:

* Project workbench.
* Project-scoped chats.
* Chat header with project/runtime context.
* Recent chats sorted by activity.

### Phase 3

Focus:

* Runtime/run inspector.
* Tool calls, approvals, context, artifacts, errors.
* Chat <-> Run <-> Project linking.

### Phase 4

Focus:

* Admin/system organization.
* Plugins as grouped capability management.
* Settings/config/debug polish.

### Phase 5

Focus:

* Customization and power-user affordances.
* Layout persistence.
* Density modes.
* Command palette.
* Optional bottom drawer.

## Open questions

* Should `/chats` or `/projects` be the default route?
* Should Recent Chats appear directly in the left rail or inside a Chats/History page first?
* Should Project switcher live at the top of the rail permanently?
* Should Approvals become top-level before or after Runtime inspector work?
* Should Debug split into Events, Logs, Runtime, and LLM Routing pages later?

```