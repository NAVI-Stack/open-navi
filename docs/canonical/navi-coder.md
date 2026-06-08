**Status:** Active  
**Last Updated:** 2026-06-05  
**Updated By:** ChatGPT, with explicit owner approval

# NAVI Coder

## Canonical Definition

NAVI Coder is NAVI’s first-class programming product domain.

It is not a plugin, not a single skill, not a connector, and not a special self-coding subsystem. It is a top-level product surface inside the NAVI platform for understanding software work, operating on repositories, executing coding tasks, validating changes, producing reviewable outputs, and safely applying the same coding capability to NAVI itself when appropriate.

The product name is **NAVI Coder**. The term **NAVI Programmer** is deprecated except when referring to legacy package names or migration history.

---

## Canonical Position

NAVI is the platform. NAVI Coder is one of its first-class product domains.

```text
NAVI Platform
├── NAVI Chat
├── NAVI Coder
├── NAVI Automations / Agents
├── NAVI Knowledge / Memory
├── NAVI Skills and Connectors
└── NAVI Settings / Governance
```

From the user’s point of view, Coder should feel like entering a dedicated product workspace inside NAVI, not like invoking a hidden plugin from ordinary chat.

The console should therefore present Coder as a top-level destination, such as:

```text
NAVI Console
├── Chat
├── Coder
├── Agents / Automations
├── Knowledge
├── Skills
└── Settings
```

This canonical distinction matters because the user-facing product boundary, UI boundary, runtime boundary, and execution boundary are different things.

---

## Product Boundary

NAVI Coder is the user-facing programming product.

It owns the product experience for:

- coding tasks
- repository onboarding
- project/repo selection
- task planning
- coding runs
- diffs and file changes
- validation logs
- branch and commit review
- pull request proposals
- candidate runtime checks for self-update work
- coding-specific run history and evaluation evidence

Coder should not be surfaced as “a plugin.” A plugin may power part of the implementation, but the product experience is Coder.

---

## UI Boundary

NAVI Coder must have a dedicated console surface.

At minimum, the console should support a top-level Coder route and Coder-specific subroutes:

```text
/coder
/coder/projects
/coder/projects/:projectId
/coder/repos/:repoId
/coder/tasks/:taskId
/coder/runs/:runId
/coder/reviews/:reviewId
/coder/settings
```

A Coder screen may include conversational interaction, but chat is not the whole interface. The Coder surface should combine task conversation, plan, file tree, diff, validation output, terminal/log output, review state, and promotion controls.

The intended product layout is:

```text
[Task / Run State] + [Code / Diff / Logs] + [Conversation]
```

not:

```text
Ordinary chat with invisible coding tools underneath
```

---

## Backend Domain Boundary

NAVI Coder must be represented as a first-class backend domain inside NAVI Core.

The backend domain should define and persist Coder-specific objects, including but not limited to:

- `CoderProject`
- `CoderRepository`
- `CoderTask`
- `CoderRun`
- `CoderRunStep`
- `CoderArtifact`
- `CoderDiff`
- `CoderValidationResult`
- `CoderReview`
- `CoderCandidateRuntime`
- `CoderPromotionDecision`

These objects should be authoritative NAVI state, not loose plugin scratchpad state.

NAVI Core remains authoritative for:

- identity
- owner and workspace policy
- governance
- world model integration
- connector credentials
- model/provider settings
- autonomy settings
- confirmation policy
- audit and event history
- promotion decisions

Coder may use skills, connectors, plugins, and workers, but those implementation parts do not own the product domain.

---

## Runtime Boundary

NAVI Coder should have a Coder control plane inside `navid`.

The Coder control plane is responsible for:

- accepting Coder tasks
- binding tasks to projects and repositories
- creating Coder runs
- selecting or launching workers
- enforcing governance before risky actions
- streaming progress and events
- persisting structured outcomes
- presenting approval, pause, cancel, and promotion decisions
- coordinating candidate runtime checks for NAVI self-update work

The Coder control plane may reuse NAVI’s existing runtime/session/orchestration infrastructure, but Coder must not be modeled only as generic chat tool usage.

---

## Execution Boundary

Risky programming operations must run in isolated Coder workers or equivalent sandboxed execution environments.

Coder workers are responsible for:

- cloning or opening approved repositories/worktrees
- reading files
- applying patches
- running build/test/lint commands
- performing git lifecycle operations
- preparing review artifacts
- launching candidate runtimes when needed
- returning structured evidence to NAVI Core

Coder workers must not become independent authorities. They execute under NAVI Core’s control and report evidence back to the Coder domain.

A preferred deployment shape is:

```text
navid
  ├── owns Coder control plane
  ├── records state and events
  ├── enforces governance
  └── schedules isolated Coder work

coder-worker
  ├── performs repository work
  ├── runs validation
  ├── produces diffs and evidence
  └── exits or waits for next assigned job

candidate-navi
  ├── temporary runtime for self-update validation
  ├── isolated config/state/ports
  └── never replaces the trusted instance automatically
```

---

## Relationship to Skills, Plugins, and Connectors

Skills, plugins, and connectors remain essential implementation mechanisms.

They are not the Coder product boundary.

Coder may use:

- file inspection skills
- file mutation skills
- patch application skills
- validation skills
- git lifecycle skills
- GitHub connectors
- Linear connectors
- sandbox runners
- candidate runtime launchers
- review and PR tools

These are subordinate implementation parts of the Coder product domain.

The current `navi.programmer` package is a transitional implementation substrate. It should be repositioned, renamed, or migrated into a Coder-owned package such as:

```text
internal/coder/
plugins/coder-core/
```

or an equivalent structure that makes the product hierarchy clear:

```text
NAVI Coder product domain
└── Coder core capability pack
    ├── skills
    ├── connectors
    ├── workflows
    └── worker execution adapters
```

---

## Relationship to Self-Coding

Self-coding is not a separate architecture.

Self-coding is the result of NAVI Coder being capable enough to work on NAVI’s own repository under stricter safety constraints.

The canonical rule is:

> NAVI self-update is an application of NAVI Coder to the NAVI codebase, not a special self-coding subsystem.

Therefore:

- self-update must use the same Coder task/run/review model where possible
- self-update must use stricter candidate runtime safety gates
- self-update must not become a deeply coupled or privileged bypass around ordinary Coder capability
- live in-place self-mutation must not be the normal path

Candidate runtime containers are required for meaningful NAVI self-update validation.

---

## Canonical Naming Rules

Use these names going forward:

| Term | Status | Meaning |
| --- | --- | --- |
| NAVI Coder | Canonical | First-class programming product domain |
| Coder | Canonical shorthand | Product/domain shorthand when context is clear |
| Coder worker | Canonical | Isolated execution worker for coding tasks |
| Coder control plane | Canonical | Backend domain/control logic in NAVI Core |
| Coder core capability pack | Preferred | Built-in execution substrate for Coder |
| NAVI Programmer | Deprecated | Legacy prototype name; only valid in migration history |
| `navi.programmer` | Transitional | Legacy package/manifest identifier until migrated |

Any new document that presents “NAVI Programmer” as the first-class product is stale.

---

## Canonical Architecture Statement

NAVI Coder is a first-class product domain inside NAVI, implemented as a dedicated console surface, domain API, persistent Coder state model, and Coder control plane inside `navid`. It uses built-in Coder capability packs, skills, connectors, and sandboxed worker runtimes to perform programming work. Risky filesystem, process, git, validation, and candidate-runtime operations run in isolated worker environments. NAVI Core remains the authority for identity, governance, world model, policy, task state, event recording, and promotion decisions.

---

## Required Product Commitments

NAVI Coder must eventually provide:

- top-level console navigation
- dedicated Coder workspace UI
- repo/project binding
- task intake from user and task systems
- plan/run lifecycle
- diff and file review
- validation and logs
- branch/commit/PR workflow
- governed approval gates
- isolated worker execution
- candidate runtime validation for NAVI self-update
- structured run evidence and evaluation history

---

## Non-Goals

NAVI Coder is not:

- a generic plugin exposed from the plugin manager
- a hidden chat-only tool surface
- a self-coding-only subsystem
- an unrestricted shell for arbitrary host mutation
- a separate authority outside NAVI governance
- a standalone product that loses NAVI identity, policy, or world-model continuity

---

## Migration Rule

Existing `navi.programmer` docs and code are transitional. They should be retained only while they are actively being migrated into the NAVI Coder product/domain structure.

The migration direction is:

```text
NAVI Programmer plugin prototype
        ↓
Coder core capability pack
        ↓
NAVI Coder first-class product domain
```

Documentation, Linear issues, package names, UI labels, API routes, and task language must move toward NAVI Coder.
