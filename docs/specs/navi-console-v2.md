# NAVI Console V2 Spec

## Status

Draft. This spec turns the NAVI Console control-plane direction into implementation-facing requirements. It assumes the core cleanup is moving the backend away from legacy Session terminology and toward Chat + RuntimeSession as the canonical split.

## Related documents

- [NAVI Console Control Plane Direction](../design/navi-console-control-plane.md)
- [NAVI Console V1 Spec](navi-console-v1.md)
- [Project System V1](project-system-v1.md)
- [Workspace V1](workspace-v1.md)
- [Streaming Messaging Architecture](streaming-messaging-architecture.md)
- [Context Intake Pipeline V1](context-intake-pipeline-v1.md)
- [Memory Projection (Vault) V1](memory-projection-v1.md)
- [Intake Synthesis Seam](../design/intake-synthesis-seam.md)

## Product role

NAVI Console V2 is the owner/admin interface and AI workspace control plane for NAVI.

It must support:

- Local AI chat.
- Project-centered work.
- Runtime and run inspection.
- Plugin/capability management.
- Connector and provider control.
- Admin/operator configuration.
- Debug and observability workflows.

The UI should be clean for everyday use and deep enough for power users/admins.

## Canonical terminology

User-facing V2 terminology must be:

| Concept | Meaning | UI status |
| --- | --- | --- |
| Project | Work/context boundary | Primary |
| Chat | Durable conversation/thread | Primary |
| RuntimeSession | Execution lifecycle/lifetime | Inspector/control detail |
| Run | One execution attempt | Primary in Runs area; contextual elsewhere |
| Task | Durable project work unit | Project/workbench surface |
| Artifact | Durable output | Project/chat/run surface |
| Workspace | File/repo/sandbox boundary | Project/admin surface |
| Session | Retired legacy term | Do not expose in new V2 UI |

Implementation rule: do not blindly rename `session_id` to `chat_id`. Classify the field first.

- Conversation history/transcript -> `chat_id`.
- Execution lifecycle -> `runtime_session_id`.
- One execution attempt -> `run_id`.
- Work boundary -> `project_id`.
- Files/resource boundary -> `workspace_id`.

## Current backend transition constraints

The `chore/core-cleanup` branch already introduces chat endpoints and frontend chat API bindings:

```http
POST /api/navi/chats
GET  /api/navi/chats
GET  /api/navi/chats/{id}
GET  /api/navi/chats/{id}/runtime_summary
PATCH /api/navi/chats/{id}
POST /api/navi/chats/{id}/message
POST /api/navi/chats/{id}/archive
```

Project chat routes are also present:

```http
GET  /api/projects/{id}/chats
POST /api/projects/{id}/chats
```

Legacy session routes should be treated as removed or transitional compatibility only. The frontend must not build new V2 UX around `/api/navi/sessions` or `/api/projects/{id}/sessions`.

The current frontend branch still contains compatibility fields such as `session_id` inside `ChatEntrySchema`, `RuntimeSummarySchema`, and `RunItemSchema`. V2 UI code may tolerate these fields while the branch is transitional, but new components should use canonical names first:

- Prefer `chat_id` over `session_id` for conversation identity.
- Prefer `runtime_session_id` over `session_id` for runtime lifecycle identity.
- Prefer `run_id` for execution attempts.

## API contract requirements

### Chat API

Required frontend operations:

```ts
useChats(projectId?: string)
useChat(chatId: string | null)
useChatRuntimeSummary(chatId: string | null)
createChat(source?: string, sourceChannel?: string)
sendChatMessage(chatId: string, text: string)
renameChat(chatId: string, title: string)
archiveChat(chatId: string)
```

Required data shape, normalized for UI:

```ts
type ChatEntry = {
  id?: string;
  chat_id: string;
  title?: string;
  status?: string;
  project_id?: string;
  source_channel?: string;
  created_at?: string;
  updated_at?: string;
};

type ChatThread = {
  chat: ChatEntry;
  messages: ChatMessage[];
};
```

Compatibility tolerance:

```ts
session_id?: string; // temporary compatibility only
createdAt?: string;  // normalize to created_at
updatedAt?: string;  // normalize to updated_at
projectId?: string;  // normalize to project_id
```

Acceptance rule: V2 UI must not display the word "Session" for Chat objects.

### Runtime summary API

Required route:

```http
GET /api/navi/chats/{id}/runtime_summary
```

Required UI-normalized fields:

```ts
type RuntimeSummary = {
  chat_id: string;
  runtime_session_id?: string;
  status?: string;
  active_run_id?: string;
  current_run_id?: string;
  recent_run_ids?: string[];
  pending_count?: number;
  deferred_count?: number;
  run?: {
    runtime_session_id?: string;
    run_id?: string;
    status?: string;
    phase?: string;
    blocked_on_proposal_id?: string;
    interrupt_class?: string;
    interrupt_reason?: string;
    main_artifact_id?: string;
    artifact_ids?: string[];
  };
};
```

The right inspector should consume this summary without requiring users to understand raw JSON.

### Project API

Required frontend operations:

```ts
useProjects(includeArchived?: boolean)
useProject(projectId: string | null)
useProjectReadiness(projectId: string | null)
useProjectChats(projectId: string | null)
useProjectTasks(projectId: string | null)
useCreateProjectChat()
```

Required routes:

```http
GET  /api/projects
GET  /api/projects/{id}
GET  /api/projects/{id}/readiness
GET  /api/projects/{id}/chats
POST /api/projects/{id}/chats
GET  /api/projects/{id}/tasks
POST /api/projects/{id}/tasks
GET  /api/projects/{id}/workspace
PUT  /api/projects/{id}/workspace-binding
DELETE /api/projects/{id}/workspace-binding
```

Project surfaces should show readiness and workspace binding as first-class workbench state.

### Runs API

Runs remain execution-attempt objects. The UI should not treat a Run as a Chat.

Required fields:

```ts
type RunItem = {
  id?: string;
  run_id: string;
  chat_id?: string;
  runtime_session_id?: string;
  project_id?: string;
  status?: string;
  created_at?: string;
  updated_at?: string;
};
```

Compatibility tolerance:

```ts
session_id?: string; // transitional only; do not display as primary identity
```

## Route model

V2 route targets:

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

Legacy/current routes may remain as redirects during the migration, but new navigation should use V2 targets.

## App shell requirements

The app shell has four conceptual regions:

```text
Left rail  = navigation + workspace organization
Main pane  = active work surface
Right pane = collapsible runtime/control/context inspector
Bottom     = optional developer/operator drawer
```

### Left rail requirements

MVP navigation:

```text
Projects
Chats / Recent
Runs
Plugins
Settings
Debug
```

Expanded long-term navigation:

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

Requirements:

- Sections must be collapsible where useful.
- Recent chats must sort by last activity.
- Projects should feel like organized work scopes, not just flat records.
- Plugins should group Skills, Connectors, Providers, and Workflows instead of flooding the top-level nav.
- The left rail should not expose both Sessions and Chats.

### Main pane requirements

Main pane is route-specific.

Chat view:

- Chat title and rename affordance.
- Project badge/context.
- Model/provider indicator when available.
- Experience/autonomy mode indicator when available.
- Runtime status indicator.
- Message timeline.
- Composer.
- Attach/context affordances later.

Project view:

- Project overview.
- Readiness cards: chat, planning, coding.
- Workspace binding state.
- Project chats.
- Project tasks.
- Recent runs.
- Artifacts later.

Runs view:

- Run list with status and timestamps.
- Run detail surface with runtime session, chat, project, phase, tool calls, errors, artifacts, and proposal state.

Plugins view:

- Plugin overview.
- Skills, connectors, providers, workflows grouped under plugin/capability management.

Settings/debug views:

- Admin configuration and diagnostics.
- Prefer tables/raw JSON only where inspection is the actual task.

### Right inspector requirements

The right pane must be:

- Collapsible.
- Route-aware.
- Preferably resizable.
- Persist its open/closed state locally.
- Able to render different tabs/sections depending on active view.

Chat inspector sections:

```text
Runtime
Context
Artifacts
Approvals
Files / Workspace
Memory
Debug
```

Project inspector sections:

```text
Readiness
Workspace
Active Chats
Recent Runs
Tasks
Project Memory
```

Run inspector sections:

```text
Phase
Tool Calls
Errors
Checkpoints
Artifacts
Proposal State
```

Raw JSON is allowed behind an explicit debug expansion, not as the default representation.

### Bottom drawer requirements

The bottom drawer is optional and should not appear by default.

Use it for:

```text
Logs
Events
Run trace
Terminal-like output
Streaming tool output
```

It should be enabled by route, operator/developer mode, or explicit toggle.

## Component-level implementation requirements

### Rename/replace Session UI

Current transitional UI may still have files/components named `Sessions`. V2 implementation should migrate toward:

```text
Sessions.tsx -> Chats.tsx or ChatHistory.tsx
SessionDetail -> ChatDetail or ChatInspectorSummary
Sessions.module.css -> Chats.module.css or route-local shell styles
```

User-facing strings must change:

- "Sessions" -> "Chats" or "History".
- "Create Session" -> "New Chat".
- "Select a session" -> "Select a chat".
- "Raw Session" -> "Raw Chat" or hidden under Debug.
- "Close session" -> remove or replace with "Archive chat" unless backend supports a non-archive chat close state.

### Chat page

The chat page should become the primary everyday local AI interaction surface. It should not be a metadata/detail table.

Required sections:

- Message timeline.
- Composer.
- Optional selected project context.
- Runtime status summary.
- Inspector toggle.

### Project page

The project page should become a workbench, not just CRUD.

Required sections:

- Header with project title/status/health.
- Readiness indicators.
- Workspace binding.
- Project chats.
- Tasks.
- Recent runs.

### Runtime/run inspection

Do not make RuntimeSession a noisy primary navigation item in the MVP unless needed. Prefer exposing RuntimeSession inside:

- Chat right inspector.
- Run detail page.
- Debug/operator pages.

Runs are primary enough to remain in navigation because they represent inspectable execution attempts.

## Visual/UX requirements

The Console should feel like:

```text
ChatGPT / Claude cleanliness
+ Cursor-style agent visibility
+ OpenClaw-style operational control
+ polished local admin panel
```

Rules:

- Clean defaults.
- Progressive disclosure.
- Collapsible sections.
- Strong active states.
- Minimal raw JSON outside debug areas.
- Summary cards for high-level status.
- Tables only for admin/debug lists.
- Smooth collapsible right pane.
- Avoid kitchen-sink top-level navigation.

## MVP acceptance criteria

Phase 1 V2 usability shell is acceptable when:

- The main navigation no longer exposes Sessions as a primary concept.
- Chat list/history uses `/api/navi/chats`.
- Project chat lists use `/api/projects/{id}/chats`.
- The right pane can be collapsed.
- Chat runtime summary is visible in a human-readable form.
- Raw JSON panels are hidden behind debug expansion or moved out of the default chat path.
- The default path supports opening/creating a chat and sending a message.
- Session compatibility fields are tolerated but not displayed as canonical terminology.

Phase 2 chat/project workbench is acceptable when:

- Users can open a project, see its chats, create a project chat, and chat within that project context.
- Project readiness and workspace binding are visible.
- Chat header clearly shows project context and runtime status.
- Basic artifact/run references are discoverable from the right pane.

Phase 3 runtime/control inspector is acceptable when:

- Current/last runtime session and run state are visible without raw JSON.
- Tool calls, proposals, artifacts, errors, and context sources have structured display areas.
- Run detail pages link back to chat and project context.

## Intake & Vault surfaces (post-MVP addendum)

These surfaces are **not** part of Console V2 MVP. They are added here so the Console contract is not silently broken when the [Context Intake Pipeline](context-intake-pipeline-v1.md) and the [Memory Vault](memory-projection-v1.md) land. They ride in alongside the corresponding pipeline phases (CIP P5 and Vault), not earlier.

### Intake sync surfaces

Surface intake calmly — not as a top-level nav item, but as inspectable state where it is relevant:

- **Per-connector sync state** in the connector detail view: last/next pass, records admitted, deduped, distilled, errors, and the configured cadence/budget/privacy class (per CIP §7 sync policy and §11 observability).
- **Recent intake** in the right inspector for a chat or project that has connector-sourced context: what was admitted and distilled, with provenance links back to the source record.
- **Pending synthesis Proposals** join the existing Proposal surface (one queue), tagged with `source: intake` and — for backfill mode — presented as **grouped Proposals** ("12 contact merges from Gmail backfill — review as a set") per the [synthesis seam](../design/intake-synthesis-seam.md) §12.

### Vault surfaces

The Vault is files on disk, not a Console-owned UI. Console exposes only the operational view of it:

- **Vault sync log** (per-file): last detected edit, diff summary, resulting dispositions, approvals, Proposals raised — per Vault §13.
- **Vault Proposals** appear in the same Proposal Queue as intake Proposals, tagged `source: vault`.
- **Drift report** from the periodic Vault reprojection sweep is visible in the operator/debug surface.

### Acceptance criteria for the addendum

- Adding intake/Vault surfaces does not require new top-level nav; they live inside existing connector, chat, project, and Proposal surfaces.
- The Proposal Queue presents intake and Vault Proposals with their `source:` tag and, where applicable, group affordance for backfill batches.
- The connector detail view shows the configured sync policy and the current cycle's metrics without requiring raw JSON.

## Non-goals

- Do not redesign every admin surface in one pass.
- Do not build permanent bottom drawer UX before the shell and inspector are right.
- Do not keep both Session and Chat navigation.
- Do not make RuntimeSession look like a conversation.
- Do not make raw JSON the primary detail view.
- Do not make plugin subtypes top-level nav items in MVP.

## Open questions

- Should the default route be `/chats`, `/projects`, or last active project/chat?
- Should the UI store open/closed inspector state globally, per route, or per chat/project?
- Should operator/developer mode be explicit, or should advanced surfaces appear through progressive disclosure only?
- Should project chat creation immediately navigate to `/projects/:projectId/chats/:chatId`?
- What minimum artifact preview should ship in Phase 2?
- Should Runtime become top-level nav after Phase 3, or stay nested under Runs/Debug?
