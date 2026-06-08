# Project-Centered Programming MVP Plan

**Status:** Draft  
**Last Updated:** 2026-05-12  
**Related Specs:** [Project System V1](../../../../navi/docs/specs/project-system-v1.md), [Workspace V1](../../../../navi/docs/specs/workspace-v1.md), [NAVI Programmer Task Execution](../specs/navi-programmer-task-execution.md)  
**Related Plan:** [NAVI Programmer Implementation Plan](navi-programmer-implementation.plan.md)

## Purpose

This plan defines the minimum usable slice for project-centered programming work in NAVI.

The target user workflow is:

1. Create a project.
2. Use the project to organize chats, artifacts, and tasks.
3. Optionally bind the project to a workspace.
4. Require a workspace binding before coding-specific execution.
5. Assign one or more project tasks to NAVI.
6. Have NAVI inspect, write code, run validation in a sandboxed environment, and report review-ready results.

This plan intentionally starts with Project CRUD. Project CRUD is the entry point, but it is not sufficient by itself for coding work.

## MVP Capability Answer

The MVP should be capable of both writing code for projects and executing validation commands inside a sandboxed environment.

Current state is closer, but still not enough for that full claim:

- NAVI has workspace file tools and a path governor.
- NAVI has workspace entities, active workspace selection, project-like workspace binding, allowed-action masks, protected paths, and boundary proposals.
- NAVI has coder/critic/strategist/scout workers and ACT directive decomposition.
- NAVI has self-mod tools for git status, git diff, git commit, go build, and go test.
- NAVI has first-class Project CRUD, project workspace binding, project task
  intake, project-aware coder file roots, and sandbox profile readiness gates.
- NAVI has a core local Docker sandbox runner surface that can prepare a
  workspace-mounted sandbox command and enforce basic command/env policy.
- NAVI Programmer validation now has an internal NAVI handler that resolves a
  sandbox profile and executes validation commands through the core sandbox
  runner while preserving structured validation evidence.

But current execution is not yet a complete project sandbox:

- Self-mod command execution runs in the configured workspace directory.
- Docker Compose is the supported NaviD runtime, but that is not the same thing as a per-project task sandbox.
- End-to-end task orchestration is not yet feeding all skill invocations,
  sandbox-backed validation results, and bounded mutation runner state updates
  through one automatic project task loop.

Therefore, sandboxed project execution is an explicit MVP workstream below. Coding projects are not "execution ready" until they have both a workspace binding and an execution sandbox profile.

## Locked MVP Decisions

1. A Project is the work boundary.
2. A Workspace is the execution and trust boundary.
3. Workspace binding is optional for general projects.
4. Workspace binding is required for coding projects before mutative coding tasks can execute.
5. A coding project with no workspace binding may still hold chats, notes, artifacts, and planning tasks, but mutative coding tasks must block.
6. Project switching is explicit or high-confidence only; low-confidence routing asks the user.
7. Project context is retrieved and compiled; it is not prompt-dumped wholesale.
8. Sandboxed execution is required before the programming MVP can be called complete.
9. The first sandbox target is local Docker/container execution with bounded mounts, timeouts, resource limits, and network-off by default.
10. Host command execution may exist as a trusted development fallback, but it does not satisfy the sandboxed coding MVP.

## MVP Data Model

### Project

Add a first-class Project model through the World Model facade.

Minimum logical fields:

```yaml
project_id: string
title: string
slug: string
description: string
project_kind: general|coding
status: draft|active|on_hold|completed|archived
health: on_track|at_risk|blocked|unknown
workspace_id: optional string
sandbox_profile_id: optional string
created_at: timestamp
updated_at: timestamp
created_by: owner
attributes: json
```

Implementation note:

- The Project System spec prefers Attribute-composed World Model entities.
- The MVP may use a typed project table or projection only if it preserves `attributes`, provenance, and typed relationships.
- Project-scoped settings should use existing `configuration` rows with `scope = "project"` and `scope_id = project_id`.
- Relationships should use existing entity relationships, for example `project:<id> contains task:<id>` and `project:<id> bound_to workspace:<id>`.

### Project Kind

`general`

- Workspace binding optional.
- Suitable for chats, planning, notes, artifacts, and non-mutative tasks.

`coding`

- Workspace binding required before code mutation or command execution.
- Requires a resolved execution root.
- Requires a sandbox profile before validation commands can run.

### Project Readiness

Expose a derived readiness state:

```yaml
project_id: string
ready_for_chat: true
ready_for_planning: true
ready_for_coding: boolean
workspace_binding_required: boolean
workspace_id: optional string
sandbox_profile_id: optional string
degraded_reasons: []
```

Coding readiness is true only when:

- project kind is `coding`
- project has an active workspace binding
- workspace has at least one usable local or repo root
- sandbox profile is available and healthy
- workspace policy allows the needed action type

## API Surface

### Project CRUD

Required endpoints:

```http
GET    /api/projects
POST   /api/projects
GET    /api/projects/{id}
PUT    /api/projects/{id}
POST   /api/projects/{id}/archive
GET    /api/projects/{id}/readiness
```

Create request:

```json
{
  "title": "NAVI Project System",
  "slug": "navi-project-system",
  "description": "Build first-class projects in NAVI",
  "project_kind": "coding",
  "workspace_id": ""
}
```

Rules:

- `workspace_id` may be empty on create.
- `sandbox_profile_id` may be empty on create.
- If `project_kind = "coding"` and `workspace_id` is empty, create succeeds but readiness reports `ready_for_coding = false`.
- Updating a coding project to add or change `workspace_id` requires owner authority.
- Updating a coding project to add or change `sandbox_profile_id` requires owner authority.
- Archiving a project hides it from active lists and blocks new mutative tasks.

### Sandbox Profiles

Implemented first-pass endpoints:

```http
GET    /api/sandbox-profiles
POST   /api/sandbox-profiles
GET    /api/sandbox-profiles/{id}
```

Rules:

- Coding readiness requires an active sandbox profile.
- The first profile backend is Docker.
- `network_mode` defaults to `none`.
- Profiles carry command and environment allowlists.

### Workspace Binding

Required endpoints:

```http
PUT    /api/projects/{id}/workspace-binding
DELETE /api/projects/{id}/workspace-binding
GET    /api/projects/{id}/workspace
```

Rules:

- Binding is project-owned in the MVP model.
- Existing `workspaces.related_project_id` remains supported for compatibility.
- Active workspace resolution should check the project record first, then fall back to legacy workspace `related_project_id`.

### Project Sessions

Required endpoints:

```http
GET  /api/projects/{id}/sessions
POST /api/projects/{id}/sessions
```

Existing `POST /api/navi/sessions` should continue to accept `project_id`, but it should validate that the project exists once Project CRUD exists.

### Project Tasks

Required endpoints:

```http
GET  /api/projects/{id}/tasks
POST /api/projects/{id}/tasks
GET  /api/projects/{id}/tasks/{taskID}
POST /api/projects/{id}/tasks/{taskID}/cancel
```

Create request:

```json
{
  "title": "Add project CRUD",
  "raw_input": "Implement first-class Project CRUD for NAVI.",
  "task_class": "mutative",
  "assigned_to": "navi",
  "acceptance_target": "API, store, world model facade, tests, and docs index updated."
}
```

Task classes:

- `read_only`
- `planning`
- `mutative`
- `self_update`

Rules:

- General project tasks may be read-only or planning without workspace binding.
- Coding project mutative and self-update tasks require workspace binding.
- Task intake must preserve `raw_input`.
- Mutative tasks must produce validation evidence or explicitly record why validation was not applicable.

## Execution Model

### Project to Workspace Resolution

Before any coding task executes:

1. Load project.
2. Resolve project workspace binding.
3. Verify workspace is active.
4. Resolve execution root from workspace `repo_roots` or `local_roots`.
5. Verify workspace policy allows the requested operation.
6. Resolve sandbox profile.
7. Start task run or block with a readiness reason.

### Dynamic Execution Root

The coder runner and file tools must stop assuming only global `NAVI_WORKSPACE_DIR`.

Required changes:

- pass project ID and workspace ID through task creation, directive creation, and execution outcomes
- resolve task execution root per project
- run file read/write against the resolved root
- preserve the configured `NAVI_WORKSPACE_DIR` as default storage for NAVI-owned data, not as the only coding target

### Sandboxed Execution

Add a sandbox abstraction before broad command execution:

```go
type SandboxRunner interface {
    Prepare(ctx context.Context, req SandboxPrepareRequest) (SandboxHandle, error)
    Run(ctx context.Context, handle SandboxHandle, cmd SandboxCommand) (SandboxResult, error)
    Cleanup(ctx context.Context, handle SandboxHandle) error
}
```

Minimum sandbox profile:

```yaml
sandbox_profile_id: local-docker-default
backend: docker
network: off
mount_mode: project_workspace_rw
working_dir: repo_root
timeout: 10m
memory_limit: 2g
cpu_limit: 2
allowed_commands:
  - git status
  - git diff
  - go test
  - go build
  - npm test
  - npm run
```

MVP sandbox behavior:

- run as non-root when possible
- mount only the resolved project workspace or repo root
- network disabled by default
- no host home directory mount
- no SSH, cloud, Docker, kube, or credential directories mounted
- command timeout required
- stdout/stderr captured
- exit code captured
- result linked to task, run, workspace, and project
- cleanup required even on failure

Sandbox failure behavior:

- If sandbox is unavailable, coding tasks block.
- NAVI may still plan, inspect metadata, or ask for setup help.
- NAVI must not silently fall back to host command execution for coding tasks that require sandboxing.

## Implementation Phases

### Phase 0 - Plan and Contract Alignment

Goal:

Lock the MVP shape before implementation.

Deliverables:

- this plan
- docs index link
- implementation plan cross-link

Completion criteria:

- Project CRUD, optional workspace binding, coding readiness, and sandbox requirements are explicit.

### Phase 1 - Project Core CRUD

Goal:

Create first-class Project persistence and API.

Work:

- add `schema.Project` and enums
- add store migrations and tests
- add worldmodel Project facade methods
- add gateway handlers and route tests
- add provenance writes
- add relationships for owner/project and project/workspace when bound

Completion criteria:

- create/list/get/update/archive projects
- general and coding projects both supported
- workspace binding optional at create time
- archived projects excluded from active project lists

### Phase 2 - Project Workspace Binding and Readiness

Goal:

Make project readiness deterministic.

Work:

- add project-owned workspace binding endpoints
- add readiness calculation
- resolve workspace from project binding first, then legacy workspace binding
- validate active workspace status
- validate workspace roots are usable
- block coding readiness when no workspace is bound

Completion criteria:

- general project without workspace is usable for chat/planning
- coding project without workspace reports degraded readiness
- coding project with active workspace reports coding readiness only when roots and policy are valid

### Phase 3 - Project Sessions and Active Project Context

Goal:

Let chats center on projects.

Work:

- validate `project_id` on session creation
- add project-specific session create/list APIs
- add active project resolution helper
- include project identity in session runtime summaries
- add CLI flags or commands for project session creation

Completion criteria:

- user can create a chat inside a project
- session list can be filtered by project
- active project context is visible in API responses

### Phase 4 - Project Task Intake

Goal:

Let the owner assign tasks to NAVI under one or more projects.

Work:

- add project task create/list/get APIs
- extend task persistence with `project_id`, `workspace_id`, `task_class`, `raw_input`, `acceptance_target`, and lifecycle phase
- map project tasks into existing directive/worker flow without forking orchestration
- preserve raw input for audit
- block mutative coding tasks when project readiness is insufficient

Completion criteria:

- user can create tasks for a project
- tasks are visible by project
- read-only/planning tasks can run without workspace binding
- mutative coding tasks block without workspace binding

### Phase 5 - Project-Aware Coding Execution

Goal:

Make coder execution use project scope instead of only global workspace scope.

Work:

- carry `project_id` and `workspace_id` through directives, tasks, worker events, and execution outcomes
- resolve per-task execution root
- run file tools against the project execution root
- record changed files and task output against the project
- surface project artifacts and execution outcomes in project views

Completion criteria:

- a project coding task can inspect and mutate files inside the bound project workspace
- out-of-scope paths are blocked or routed to boundary proposals
- execution outcomes include project and workspace context

### Phase 6 - Sandboxed Validation Execution

Goal:

Run validation commands inside a sandbox before declaring coding tasks review-ready.

Work:

- add sandbox profile model
- add Docker sandbox runner
- add command allowlist and timeout enforcement
- add validation command execution through sandbox runner
- capture stdout/stderr/exit code
- link validation evidence to task result
- block if sandbox is unavailable

Completion criteria:

- mutative coding tasks run validation inside the sandbox
- validation evidence is visible
- failed validation produces `partially_succeeded` or `failed`, not `completed`
- unavailable sandbox blocks coding execution instead of falling back silently

### Phase 7 - Minimal Operator Surfaces

Goal:

Make the MVP usable without hand-crafted API calls.

Work:

- add web project list/editor
- add project session creation
- add project task creation/list/status
- add readiness badges
- add CLI project commands for create/list/use/task

Completion criteria:

- owner can create a project, bind a workspace, open a project chat, assign a task, and inspect result state from the operator surface

## Acceptance Tests

Minimum test bill:

1. Create general project without workspace succeeds.
2. Create coding project without workspace succeeds but `ready_for_coding` is false.
3. Bind active workspace to coding project makes readiness true when roots are usable.
4. Bind archived or missing workspace fails.
5. Create project session with valid project ID succeeds.
6. Create project session with missing project ID fails after Project CRUD is enabled.
7. Create read-only task on general project succeeds.
8. Create mutative coding task without workspace blocks.
9. Create mutative coding task with workspace creates a directive/task with project and workspace context.
10. File mutation outside project workspace is blocked.
11. Sandbox unavailable blocks validation execution.
12. Sandbox validation captures stdout, stderr, exit code, and task linkage.
13. Failed validation prevents clean `completed` status.
14. Archived project blocks new mutative tasks.
15. Existing workspace APIs continue to work.

## First Coding Pass

Start with Phase 1.

Concrete first task:

> Implement Project Core CRUD with optional workspace binding metadata, no task execution yet.

Suggested file ownership:

- `internal/schema/project.go`
- `internal/store/project.go`
- `internal/store/db.go`
- `internal/worldmodel/worldmodel.go`
- `internal/gateway/projects.go`
- `internal/gateway/server.go`
- `internal/store/project_test.go`
- `internal/gateway/projects_test.go`

Do not implement sandbox execution in the first pass. Instead, include the readiness fields that will let coding tasks block cleanly until the sandbox workstream lands.

## Not Done Until

The minimum usable slice is not done until:

- Projects are first-class CRUD objects.
- General projects can exist without workspaces.
- Coding projects require workspace binding for mutative tasks.
- Project tasks can be assigned to NAVI.
- Project-aware coder execution uses the bound workspace root.
- Validation commands run in a sandbox.
- Results are tied back to project, task, workspace, run, artifacts, and validation evidence.
