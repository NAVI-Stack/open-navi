# NAVI Coder Capability

Status: Draft / Design Lock
Last Updated: 2026-05-20
Branch: feat/programmer

## Purpose

NAVI Coder is NAVI's built-in governed software work capability domain.

It gives NAVI a dedicated coding mode for understanding, planning, modifying, testing, debugging, delegating, reconciling, and documenting software work across real workspaces.

NAVI Coder is a local-first coding operator that can touch real projects. Separate IDE or workbench products may build richer surfaces around it later, but those product layers are outside this NAVI capability definition.

## North Star

NAVI Coder is NAVI's built-in governed software work capability: a local-first coding mode that can inspect, plan, edit, test, debug, delegate, reconcile, and document software work across real workspaces, while respecting explicit operating modes, permission levels, proposals, and audit trails.

## Product Identity

NAVI Coder is not a standalone product and not a normal plugin.

It is a first-class built-in capability domain. It may be enabled, disabled, surfaced, categorized, and extended like a plugin category, but it has core product status inside NAVI.

Recommended names:

- User-facing name: NAVI Coder
- Internal capability domain: coder
- Legacy / package-compatible namespace: navi-programmer or navi-coder
- Skill tags: coder, programming, software-work, repo, devtools

## What It Is

NAVI Coder is the coding mode for NAVI.

It exists to:

- enable non-coders to buy, shape, and supervise software work
- enable developers and software engineers to power-code
- let NAVI operate on local workspaces through explicit authority levels
- coordinate virtual, local, and remote software work under a coherent mental model
- integrate coding work with NAVI's Proposal Queue, task state, audit trail, and capability system

## What It Is Not

NAVI Coder is not:

- a toy
- a shallow chat wrapper
- a generic agent mode
- an IDE/workbench product layer
- a replacement for an IDE product layer
- a single monolithic programmer plugin
- a hidden second NAVI inside NAVI

It should not absorb every development feature into one package. It should remain a built-in capability domain with plugin-expandable edges.

## Product Boundaries

### Core NAVI

Core NAVI owns:

- world model
- proposals
- governance where applicable
- task state
- virtual reasoning
- virtual / sandbox / cloud execution
- agent delegation
- trace and audit model
- mental model of workspaces

Core NAVI may operate like Jules, Codex-web, or similar cloud coding agents: virtual containers, sandboxes, API-based repo operations, cloud tasks, and remote workspace mirrors.

### PET

PET owns the physical local coding harness.

PET is responsible for:

- local desktop harness
- physical workspace access
- file edits on disk
- local command execution
- local test runs
- local workspace dashboards and views
- environment mediation and local permission boundaries

PET is where the Claude-Code-on-desktop behavior physically runs.

### NAVI Console

NAVI Console owns broad visibility and oversight.

It should expose:

- runtime inspection
- capability state
- task history
- Proposal Queue
- agent session history
- workspace mirrors
- logs
- traces
- diffs
- approvals

NAVI Console is not the only cockpit. NAVI Coder may also have dedicated dashboards and visual components inside PET or other Coder-specific surfaces.

### External IDE / Workbench Product Layer

A separate IDE or workbench product layer may become more IDE-like and provide richer coding surfaces later.

NAVI Coder should not try to become that product layer too early.

## Built-In Capability Domain Model

NAVI Coder is built in, but plugin-expandable.

The default capability domain should include first-party baseline capabilities such as:

- coder.core
- coder.workspace
- coder.files
- coder.git
- coder.shell
- coder.test
- coder.debug
- coder.docs
- coder.diff
- coder.proposals

Plugin-expanded edges may include:

- GitHub
- GitLab
- Bitbucket
- Jira
- Linear
- CI providers
- Docker / devcontainers
- language-specific tooling
- security scanners
- code review systems
- external agent backends

Repo connectors should mostly be plugin-based. Native Git and basic generic CI support can exist as first-party baseline capabilities. Rich provider-specific behavior belongs in plugins.

## Workflow Behavior Controls

Workflow behavior controls how NAVI Coder thinks and works.

### Ask Mode

Ask Mode is inspection and explanation first.

It may:

- read files
- inspect repo state
- explain code
- answer questions
- search the workspace
- summarize architecture
- identify likely issues

It does not mutate files or execute meaningful commands unless separately authorized.

### Plan Mode

Plan Mode is design and architecture first.

It may:

- produce implementation plans
- decompose tasks
- design architecture
- compare approaches
- identify risks
- prepare proposals
- outline tests
- prepare migration plans

Plan Mode produces plans and proposed work, not direct implementation by default.

### Agent Mode

Agent Mode is goal-directed execution.

NAVI Coder takes initiative to complete the requested software task using available skills, connectors, tools, and workflow delegation.

Agent Mode behavior is still shaped by the selected permission level unless the permission level is Unlocked.

### Debug Mode

Debug Mode is investigation-first execution.

It may:

- reproduce bugs
- inspect logs
- run diagnostics
- isolate root causes
- compare failing and passing states
- propose fixes
- apply fixes depending on permission level
- validate fixes with tests or targeted checks

Debug Mode is distinct from general Agent Mode because the primary objective is diagnosis, not feature completion.

## Permission Levels

Permission levels control what NAVI Coder is allowed to do.

### Ask First

NAVI Coder asks before meaningful mutation or moderate/risky commands.

Examples that require approval:

- editing files
- creating files
- deleting files
- running commands with side effects
- installing dependencies
- changing repo configuration
- committing
- pushing
- opening pull requests
- accepting delegated agent output that mutates the workspace

### Auto

NAVI Coder may proceed with normal bounded coding work.

It may autonomously perform low-risk local coding actions such as:

- reading and inspecting files
- creating bounded local edits
- running tests
- updating docs
- preparing patches
- performing routine repo work

It asks when work is risky, destructive, credentialed, remote, irreversible, or requires escalation.

### Unlocked

Unlocked means no governance.

No proposal stops. No confirmation gates. NAVI Coder executes freely within the selected environment.

Unlocked is intended for pure sandboxes, disposable environments, throwaway containers, isolated worktrees, or other environments where destructive changes are acceptable.

Warning language:

> Unlocked disables NAVI governance and confirmation gates. Use only in disposable sandboxes, isolated worktrees, throwaway containers, or environments where destructive changes are acceptable. NAVI may create, edit, delete, execute, install, rewrite, or otherwise mutate the workspace without stopping for approval.

Unlocked must be visually and operationally distinct from Ask First and Auto.

## Workspace Model

NAVI Coder must keep a hard distinction between virtual work, physical work, and remote work.

### Virtual Work

Virtual work includes:

- plans
- patches
- drafts
- sandbox branches
- cloud containers
- analysis
- proposed edits
- delegated-agent output not yet applied to disk

### Physical Work

Physical work includes:

- actual file edits on disk
- local commands
- local git state mutation
- local dependency installs
- local test execution
- local workspace mutation

PET is the primary physical-work execution surface.

### Remote Work

Remote work includes:

- pushes
- pull requests
- issue tracker mutations
- CI actions
- remote repository mutations
- provider-specific operations through GitHub, GitLab, Linear, Jira, or similar plugins

Remote work usually has higher escalation requirements than local physical work unless the user selected Unlocked in a pure sandbox flow.

## Proposal Queue Integration

Proposal Queue is the approval bus for NAVI Coder.

NAVI Coder should use Proposal Queue for approval and recovery around:

- patch approval
- file delete approval
- dependency install approval
- command execution approval
- branch creation approval
- commit approval
- push approval
- pull request creation approval
- destructive refactor approval
- accepting delegated agent output
- Full Access / Unlocked escalation
- workspace trust approval
- recovery from partial execution

Proposal Queue is the primary way NAVI Coder interfaces with approval flows. Coder-specific UI may present proposals differently, but the underlying entity and lifecycle should remain Proposal Queue based.

Unlocked bypasses Proposal Queue stops by design.

## Skills and Plugins

Skills stay small, declarative, and governed where governance applies.

A skill is one governed capability interface, not a hidden coding agent.

Good skill examples:

- coder.repo.inspect
- coder.repo.map
- coder.repo.find_symbol
- coder.repo.explain_architecture
- coder.plan.implementation
- coder.plan.refactor
- coder.plan.test_strategy
- coder.plan.migration
- coder.patch.create
- coder.patch.apply
- coder.file.create
- coder.file.edit
- coder.file.delete
- coder.test.run
- coder.lint.run
- coder.typecheck.run
- coder.build.run
- coder.debug.reproduce
- coder.debug.inspect_logs
- coder.debug.isolate_root_cause
- coder.diff.review
- coder.agent.delegate
- coder.agent.monitor
- coder.agent.collect_result
- coder.agent.reconcile
- coder.docs.refresh
- coder.commit.prepare

Bad skill shape:

- coder.do_everything
- programmer.agent.run_anything
- navi.programmer.hidden_brain

Plugins provide expansion surfaces around connectors, toolchains, workflow bundles, and agent backends.

## Agentic Workflow Plugins

Agentic workflow plugins are a major part of NAVI Coder.

They may provide:

- delegation to Codex, Claude Code, local coding agents, or future agent backends
- task monitoring
- result collection
- diff reconciliation
- conflict resolution
- test-loop automation
- multi-agent planning and review flows

These plugins should not replace NAVI's Cognitive Layer. They are execution and delegation tools used by NAVI Coder.

## Default Capability Map

Initial NAVI Coder capability map:

1. Repo understanding
   - inspect repo
   - map architecture
   - find symbols
   - explain subsystems

2. Planning
   - implementation plans
   - refactor plans
   - migration plans
   - test strategy
   - risk review

3. Workspace mutation
   - create files
   - edit files
   - delete files
   - apply patches
   - manage diffs

4. Validation
   - run tests
   - run lint
   - run typecheck
   - run builds
   - summarize failures

5. Debugging
   - reproduce issues
   - inspect logs
   - isolate root cause
   - propose fixes
   - validate fixes

6. Version control
   - inspect git state
   - prepare commits
   - create branches
   - manage local diffs
   - push only when allowed

7. Documentation
   - refresh docs
   - reconcile implementation drift
   - update architecture notes
   - generate implementation reports

8. Delegation
   - delegate tasks to coding agents
   - monitor delegated work
   - collect results
   - reconcile patches

9. Recovery
   - identify partial execution
   - preserve failed state
   - propose recovery paths
   - reconcile conflicts

## Failure and Recovery Principles

NAVI Coder must not pretend partial work succeeded.

It must explicitly track:

- failed edits
- partial agent output
- dirty workspace states
- failed tests
- dependency install failures
- command timeouts
- rejected proposals
- merge conflicts
- unreconciled delegated output

Partial execution must become visible either through Coder-specific UI, NAVI Console, or Proposal Queue depending on severity and permission mode.

## UI Surfaces

NAVI Coder may appear across multiple surfaces:

- PET coding harness for local physical work
- NAVI Coder dashboard/views for coding-specific state
- NAVI Console for system-level oversight
- future external IDE/product surfaces

Console is important but not the only cockpit.

## Locked Decisions

The following decisions are locked for the next design pass:

- NAVI Coder is a built-in first-class capability domain.
- It is enabled, disabled, categorized, and extended like a plugin category.
- It is not a single monolithic programmer plugin.
- Repo/provider connectors are largely plugin-based.
- Native Git and basic generic CI may exist as first-party baseline capabilities.
- Skills stay small and declarative.
- Workflow controls are Ask Mode, Plan Mode, Agent Mode, and Debug Mode.
- Permission levels are Ask First, Auto, and Unlocked.
- Unlocked means no governance and is intended for pure sandboxes.
- Proposal Queue is the approval mechanism for governed modes.
- PET owns physical local workspace mutation.
- Core NAVI owns virtual/sandbox/cloud reasoning and execution.
- NAVI Console maintains system oversight and mental-model visibility.
- NAVI Coder may have its own dashboards and visual components.
- Separate external IDE/product layers may exist later and should not be collapsed into NAVI Coder now.

## Next Design Pass

The next design pass should produce a concrete architecture document that maps:

- operating modes to allowed behaviors
- permission levels to command gates
- Coder skills to transports and effects
- PET responsibilities to local execution boundaries
- Console responsibilities to audit and visibility
- plugin categories to default and future integrations
- Unlocked behavior to sandbox assumptions and warnings
