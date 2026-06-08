# NAVI Console Redesign Roadmap

## Status

Draft. This roadmap breaks the NAVI Console V2 direction into phased, agent-taskable work. It should be updated as `chore/core-cleanup` continues to replace legacy Session surfaces with Chat and RuntimeSession.

## Design anchor

NAVI Console is the owner/admin interface and AI workspace control plane for NAVI.

It must support normal local AI chat, project-centered work, runtime/run inspection, plugin/capability management, and full owner/admin control without looking like a rough prototype dashboard.

## Dependency note

The redesign should follow the core cleanup rather than race it. The `chore/core-cleanup` branch already moves API and frontend bindings toward Chat:

- Adds `/api/navi/chats` routes.
- Adds `/api/projects/{id}/chats` routes.
- Removes the old frontend `api/sessions.ts` file.
- Adds `api/chats.ts`.
- Introduces `MessageIntakeService` to resolve Chat + RuntimeSession before runtime submission.

The Console roadmap should assume this direction, but tolerate temporary compatibility fields during the transition.

## Phase 0 — Documentation and alignment

Goal: prevent drift before frontend work starts.

Tasks:

- Maintain [NAVI Console Control Plane Direction](../design/navi-console-control-plane.md).
- Maintain [NAVI Console V2 Spec](../specs/navi-console-v2.md).
- Keep design/spec index links updated.
- Track open questions in the design/spec docs.
- Review `chore/core-cleanup` before each implementation tranche.

Acceptance criteria:

- Product statement is durable in GitHub docs.
- V2 terminology rules are documented.
- The redesign phases are taskable.
- GitHub is the canonical implementation source of truth.

## Phase 1 — Usability shell and terminology cleanup

Goal: make the existing Console coherent around Chat, Project, Run, and Plugin without redesigning every page.

Scope:

- Rename primary user-facing Sessions navigation to Chats or History.
- Replace remaining user-facing strings:
  - Sessions -> Chats / History.
  - Create Session -> New Chat.
  - Select a session -> Select a chat.
  - Raw Session -> Raw Chat or Debug payload.
- Keep transitional compatibility fields internal only.
- Introduce a clean app shell with:
  - left navigation rail,
  - route-aware main pane,
  - collapsible right pane placeholder.
- Add inspector open/closed state.
- Make right pane collapsible on current chat/detail surfaces.
- Keep raw JSON behind explicit debug expansion.

Likely files/components:

```text
web-src/navi-console/src/pages/Sessions.tsx
web-src/navi-console/src/pages/Chat.tsx
web-src/navi-console/src/app/Router.tsx
web-src/navi-console/src/components/NavSidebar.tsx
web-src/navi-console/src/types/api.ts
web-src/navi-console/src/api/chats.ts
web-src/navi-console/src/api/projects.ts
```

Recommended rename path:

```text
Sessions.tsx -> Chats.tsx or ChatHistory.tsx
Sessions.module.css -> Chats.module.css
SessionDetail -> ChatDetail or ChatInspectorSummary
```

Acceptance criteria:

- No primary navigation item is named Sessions.
- Chat list uses `/api/navi/chats`.
- Project chat list uses `/api/projects/{id}/chats`.
- The right pane can be collapsed.
- Chat runtime summary is visible in a human-readable card/section.
- Creating and opening a chat works.
- Sending a chat message works.

## Phase 2 — Chat + project workbench

Goal: make the primary owner workflow strong.

Primary workflow:

```text
Open project -> open chat -> send message -> see runtime status -> inspect output
```

Scope:

- Build a modern chat page:
  - chat title,
  - project badge,
  - model/provider indicator when available,
  - experience/autonomy mode indicator when available,
  - runtime status indicator,
  - message timeline,
  - composer.
- Build project workbench sections:
  - overview,
  - readiness cards,
  - workspace binding,
  - project chats,
  - project tasks,
  - recent runs.
- Add project-scoped chat creation and navigation.
- Show project context on chat routes.
- Start displaying artifacts/recent run links in the inspector if data exists.

Acceptance criteria:

- A user can create/open a project chat and send messages in project context.
- Project readiness is visible.
- Workspace binding state is visible.
- Project chats are visible and sorted by recent activity.
- The chat header clearly shows project context.
- The right inspector surfaces runtime/project context without raw JSON by default.

## Phase 3 — Runtime/control inspector

Goal: expose NAVI's power without cluttering normal chat.

Scope:

- Build route-aware inspector sections:
  - Chat Runtime,
  - Context,
  - Artifacts,
  - Approvals,
  - Files/Workspace,
  - Memory,
  - Debug.
- Show RuntimeSession details as execution lifecycle, not conversation.
- Build richer Run detail surface:
  - status,
  - phase,
  - runtime session,
  - chat/project links,
  - tool calls,
  - errors,
  - checkpoints,
  - artifacts,
  - proposal state.
- Add links between Chat, Project, RuntimeSession, and Run surfaces.

Acceptance criteria:

- The current/last runtime session can be inspected from a chat.
- The current/last run can be inspected from a chat.
- Runs link back to chat/project context.
- Tool calls, approvals/proposals, artifacts, and errors have structured display areas.
- Raw payloads remain available only under explicit debug disclosure.

## Phase 4 — Admin/system surfaces

Goal: preserve and polish the full owner/admin capability layer.

Scope:

- Reorganize Plugins area:
  - Skills,
  - Connectors,
  - LLM Providers,
  - Workflows.
- Keep top-level navigation streamlined.
- Improve Settings and Config structure.
- Improve Debug and observability pages:
  - events,
  - logs,
  - runtime metrics,
  - connector events,
  - LLM routing,
  - context assembly traces when available.
- Add governance/approval surfaces where appropriate.

Acceptance criteria:

- Plugins feels like one organized capability-management area, not several disconnected nav items.
- Admin surfaces are discoverable but do not dominate normal chat/project use.
- Debug pages expose enough raw data for operators without leaking it into everyday UI.

## Phase 5 — Polish, customization, and power-user affordances

Goal: make the Console feel production-grade.

Scope:

- Theme customization.
- Layout persistence.
- Density modes.
- Keyboard shortcuts.
- Command palette.
- Saved views.
- Operator/developer mode toggle.
- Bottom developer drawer for logs/events/run trace/terminal-like output.

Acceptance criteria:

- Layout preferences persist.
- The UI supports both clean default mode and dense operator mode.
- Bottom drawer is opt-in, not permanent.
- Console feels like a polished product, not a prototype dashboard.

## Cross-phase rules

- Do not expose legacy Session as a new UI concept.
- Do not make RuntimeSession look like a Chat.
- Do not promote every backend object to top-level navigation.
- Do not make raw JSON the first view of important data.
- Do not add a permanent bottom panel until the shell and inspector are stable.
- Keep implementation docs in GitHub as canonical.
- Use Notion for live thinking only.
- Use Google Drive for polished conceptual summaries only.

## Recommended first implementation prompt

Use this prompt after `chore/core-cleanup` stabilizes enough for frontend work:

```text
Review `chore/core-cleanup` and update NAVI Console Phase 1 toward the V2 shell.

Primary goals:
- Replace user-facing Sessions terminology with Chats/History.
- Keep backend compatibility fields internal only.
- Use `/api/navi/chats` and `/api/projects/{id}/chats` as canonical chat APIs.
- Rename or wrap `Sessions.tsx` into a Chat/History page.
- Add a collapsible right inspector region.
- Move raw JSON into debug disclosure.
- Preserve existing functionality while improving clarity.

Do not redesign every page. Do not build the bottom drawer yet. Do not expose RuntimeSession as a primary chat-like object.

Acceptance criteria:
- The left nav no longer exposes Sessions.
- Creating/opening/sending chat still works.
- Project chat listing still works.
- Right pane can collapse.
- Chat runtime summary is human-readable.
- Tests/build pass or failures are clearly documented.
```
