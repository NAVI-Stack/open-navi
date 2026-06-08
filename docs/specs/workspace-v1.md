# NAVI Workspace Specification — V1

> [!NOTE]
> Part of the [NAVI Systems Map](../architecture/navi-systems-map.md).


## Status

Draft 2 — Revised
Single-user foundation spec

## Change log

| Draft | Changes |
|-------|---------|
| Draft 1 | Initial spec |
| Draft 2 | Resolved 9 open issues: default no-workspace behavior (§9.4), removed `mode` from entity and `global` from `workspace_kind` (§7, §8), renamed `default_access` to `allowed_actions` with deny-mask semantics (§8, §14), added whitelist lifecycle fields (§8, §12), removed service scope from V1 entity (§8, §20), added workspace switching contract (§12), clarified workspace as World Model Control Entity (§5), added subconscious interaction constraints (§15), clarified audit writes into History (§18) |

---

## 1. Purpose

This spec defines **Workspace** as a built-in NAVI concept for controlling **where NAVI can operate, what scope is active, and what rules govern execution inside that scope**.

Workspace is not just a folder path. It is a **durable execution boundary plus a context boundary**. That fits NAVI's existing architecture, where execution belongs to the Capability layer, governance sits between decision and execution, History records every command outcome, and owner-set configuration remains authoritative.

This V1 is intentionally lean:

* single-user first
* local and project-oriented use first
* strong boundaries
* minimal but real auditability
* no enterprise hierarchy or multi-user tenancy yet

---

## 2. Goals

V1 must support all of the following:

1. **Global access mode** for users who want NAVI to operate broadly across the machine.
2. **Scoped workspaces** for users who want NAVI limited to defined areas.
3. **Hybrid behavior** where a user can allow global access and still define named workspaces.
4. **Project compatibility** so a project can bind to one workspace.
5. **Deterministic enforcement** so workspace boundaries are real, not advisory.
6. **Lean auditability** so NAVI can explain what it did, where, and under what workspace.
7. **A clean foundation** that can later grow into stronger governance without redesign.

---

## 3. Non-goals

V1 does **not** attempt to solve:

* multi-user shared workspaces
* org-admin policy hierarchies
* enterprise compliance frameworks
* nested workspace inheritance
* automatic replanning within scope
* advanced workspace suggestion/routing
* full remote/container execution model
* cross-workspace projects
* service identity model or service-level scoping

Those are V2+ concerns.

---

## 4. Core definitions

### 4.1 Workspace

A **Workspace** is a first-class NAVI Control Entity that defines:

* allowed local roots
* allowed repos
* active execution scope
* applicable boundary rules
* allowed action types within scope
* protected paths
* workspace metadata
* workspace audit context

### 4.2 Active Workspace

The **Active Workspace** is the single workspace currently governing execution.

Only one workspace governs an action in V1.

### 4.3 Global Access

A system-level owner-set Configuration setting that allows NAVI to operate broadly across the device and available surfaces, still subject to governance, protected paths, and user approval where required.

Global access is a mode of operation, not the absence of a workspace model. It lives in Configuration, not on workspace entities.

### 4.4 Boundary Crossing

An attempted action against a path, repo, or resource outside the Active Workspace's allowed scope.

### 4.5 Protected Path

A path or path pattern that NAVI must treat as blocked or specially restricted, even if broad access exists.

---

## 5. Architectural position

Workspace is a **first-class Control Entity in the World Model**, not a plugin and not loose configuration.

It belongs in the World Model because it has durable identity, structured fields, and participates in governance decisions. However, its mutable fields are predominantly owner-set control state, so they follow Configuration-style governance rules: owner authority is required for changes, and no internal process may silently modify workspace boundaries, permissions, or protected paths.

This positioning works because:

* the World Model already separates Capabilities from Control state
* Configuration is explicit or inferred and owner-set state cannot be silently overridden
* Governance checks permissions, policy, configuration, and risk before execution
* History records every execution attempt, including failures and blocked actions
* Proposals already exist as the mechanism for actions requiring authorization

Workspace plugs into the existing flow:

**Decide → Validate/Govern → Execute → Record**

Workspace does not bypass or replace governance. It narrows and structures it.

---

## 6. Operating modes

V1 supports three system-level operating modes, stored in owner-set Configuration.

### 6.1 Global mode

NAVI may act broadly across the user's allowed environment.

Still subject to:

* system rules
* user configuration
* protected paths
* confirmation requirements
* risk thresholds

### 6.2 Scoped mode

NAVI may act only inside defined workspace boundaries.

### 6.3 Hybrid mode

The user enables broad access overall but also defines one or more named workspaces for specific contexts such as projects, repos, or functional areas.

This is a first-class supported mode in V1.

### 6.4 Mode storage

The operating mode (`global`, `scoped`, `hybrid`) is an owner-set Configuration value. It is not a field on workspace entities. This keeps the system posture separate from individual workspace definitions.

---

## 7. Workspace kinds

V1 supports the following `workspace_kind` values:

* `general`
* `project`
* `repository`
* `functional`

These do **not** create inheritance. They only classify intent and shape.

### 7.1 Meanings

**general**
A broad named workspace not tied to one repo or one project.

**project**
A workspace centered around a specific project.

**repository**
A workspace centered around a repo root and repo-specific work.

**functional**
A workspace centered around a purpose, such as docs, assets, exports, or reports.

### 7.2 No `global` workspace kind

Global access is a system-level operating mode stored in Configuration, not a workspace object. V1 does not support a `global` workspace kind. This avoids conflating system posture with workspace identity.

---

## 8. Workspace entity model

### 8.1 Required fields

```yaml
workspace_id: string
name: string
description: string?
workspace_kind: enum(general, project, repository, functional)
status: enum(active, inactive, archived, suspended)

local_roots:
  - path

repo_roots:
  - path_or_repo_ref

protected_paths:
  - pattern

allowed_actions:
  read: boolean
  write: boolean
  create: boolean
  modify: boolean
  rename_move: boolean
  delete: boolean
  execute: boolean

boundary_policy:
  out_of_scope_default: enum(prompt, deny)

audit_enabled: boolean

created_at: timestamp
updated_at: timestamp
created_by: owner
```

### 8.2 Optional fields

```yaml
tags:
  - string

related_project_id: string?
notes: string?

whitelist_rules:
  - rule_id: string
    scope: string
    action_types: [string]
    status: enum(active, revoked)
    created_at: timestamp
    revoked_at: timestamp?
    expires_at: timestamp?
    created_by: owner

metadata:
  arbitrary_json
```

### 8.3 Removed from V1

The following fields appeared in Draft 1 and have been removed:

* `mode` — operating mode belongs in system-level Configuration, not per-workspace
* `allowed_services` — no V1 service identity or enforcement model exists
* `blocked_services` — same reason; service scoping is deferred to V2

### 8.4 Semantics of `allowed_actions`

`allowed_actions` is a **workspace-local deny mask** over in-scope resources. It does not grant permissions. Its semantics are:

1. The action has already passed Governance (system rules, autonomy, risk, owner constraints).
2. The target resource is within the Active Workspace's scope (local roots, repo roots, minus protected paths).
3. `allowed_actions` then narrows which action types the workspace permits on that in-scope target.

If `allowed_actions.delete` is `false`, the workspace blocks delete operations on in-scope resources even if Governance would otherwise permit them.

`allowed_actions` does not interact with the Autonomy Model directly. Autonomy controls how much NAVI does autonomously; `allowed_actions` controls what action types are structurally permitted within the workspace. Autonomy never expands what is permitted, and neither does the workspace.

---

## 9. Active workspace selection

V1 keeps this strict and simple.

### 9.1 Selection rules

1. **Explicit user-selected workspace wins.**
2. **Project-bound workspace wins when operating inside that project context.**
3. Otherwise, NAVI does **not** silently choose a workspace in V1.

### 9.2 Out of scope for V1

Deferred:

* intelligent auto-selection
* confidence-based switching
* workspace suggestion engine

NAVI may support suggestion later, but V1 should stay deterministic.

### 9.3 Invariant

There is exactly **one Active Workspace** governing execution at a time.

### 9.4 Default behavior when no workspace is active

If no Active Workspace exists and global access is off, any scoped file or repo action is **blocked pre-execution**. NAVI prompts the user to select or create a workspace before proceeding.

Plain chat, reasoning, and non-scoped operations may continue. Scoped execution cannot.

This fits the existing validation/rejection flow: a blocked action due to missing workspace context is a first-class recorded outcome, not a silent skip.

---

## 10. Overlap and topology rules

This is one of the most important V1 decisions.

### 10.1 Multiple workspaces

A user may define multiple workspaces.

### 10.2 Global + workspaces

A user may enable global access and still define one or more workspaces.

### 10.3 Overlap

Overlapping workspaces are allowed in V1.

Examples:

* ecosystem root contains project root
* repo workspace overlaps project workspace
* docs workspace overlaps repo workspace

### 10.4 No nested inheritance

True nested workspaces with inherited policy are **not supported** in V1.

### 10.5 No permission union

If multiple workspaces overlap, NAVI must **not** merge their permissions.

Overlap does not mean:

* combined allowlists
* combined tool access
* combined policy loosening

### 10.6 Governing rule

Only the **Active Workspace** governs the action.

### 10.7 No silent "most specific path wins"

V1 must not silently switch authority just because a more specific workspace also matches the path.

If NAVI is acting under Workspace A and touches a path also contained by Workspace B, Workspace A still governs unless the user explicitly switches.

This avoids hidden workspace switching and policy ambiguity.

---

## 11. Scope enforcement

Workspace boundaries must be machine-enforced, not prompt-enforced. Prompt instructions are not a real security boundary; execution controls must exist at the runtime/policy layer.

### 11.1 Local path scope

A scoped workspace may only access paths under its allowed roots, minus protected paths and explicit denies.

### 11.2 Path normalization

All path checks must be performed on normalized resolved paths.

The implementation must account for:

* `..` traversal
* symlinks
* junctions
* aliases/shortcuts where relevant
* mount boundaries where relevant

Otherwise workspace enforcement is fake.

### 11.3 Repo scope

If a workspace is repository-based, repo roots may carry separate meaning from generic folders.

---

## 12. Boundary crossing behavior

V1 supports explicit user-mediated boundary crossing.

### 12.1 Trigger

If NAVI attempts an action outside the Active Workspace scope, it must pause and prompt.

### 12.2 Allowed user responses

V1 supports these outcomes:

* **Deny**
* **Allow once**
* **Always allow**
* **Switch workspace** if another workspace is explicitly chosen by the user

### 12.3 Whitelist behavior

"Always allow" must create a durable approved rule stored as structured configuration tied to the Active Workspace.

It must not be a vague remembered preference. It must be a visible, owner-controlled rule.

### 12.4 Whitelist lifecycle

Every whitelist rule must have:

* `status` — `active` or `revoked`
* `revoked_at` — timestamp when revoked, if applicable
* `expires_at` — optional expiration timestamp

V1 requires a view/revoke UI for whitelist rules. There must be no silent permanent growth of "always allow" rules. The owner must be able to see all active rules and revoke any of them.

### 12.5 Workspace switching mid-execution

When the user chooses to switch workspaces during an in-flight plan:

1. The current plan **pauses**.
2. All completed steps remain recorded with their outcomes under the original workspace.
3. The Active Workspace switches to the user-selected workspace.
4. Remaining plan steps are **re-validated** under the new workspace before continuing.
5. Prior approvals from the original workspace do **not** carry forward. Each remaining step must pass validation under the new workspace independently.
6. If prior steps already produced side effects, normal partial-execution semantics apply (side effects are recorded, not rolled back).

There is no blind carry-forward of prior governance decisions across workspace boundaries.

### 12.6 Deferred

Not in V1:

* automatic replanning within current scope
* intelligent constraint-aware replanning
* silent fallback behavior

---

## 13. Protected paths

Protected paths are mandatory in V1.

### 13.1 Purpose

Even when global access is enabled, some paths should remain blocked or specially controlled.

### 13.2 Sources

Protected paths may come from:

* system defaults
* user-defined entries
* workspace-defined entries

### 13.3 Example categories

Examples include:

* secret stores
* auth/token folders
* OS-critical areas
* sensitive config directories
* user-marked private directories

### 13.4 Precedence

Protected path rules override ordinary workspace allow scope unless explicitly overridden through an approved rule path.

### 13.5 Pattern model

V1 can implement this simply as path and pattern matching. It does not need a huge policy language yet.

---

## 14. Permission surface

V1 supports a minimal action model.

### 14.1 Action classes

At minimum:

* `read`
* `write`
* `create`
* `modify`
* `rename_move`
* `delete`
* `execute`

### 14.2 Deny-mask principle

Workspace `allowed_actions` is a deny mask. It narrows what is permitted on in-scope resources after an action has already passed Governance.

The baseline is not "everything allowed." The baseline is: the action already passed Governance, and the target is in scope. The workspace then further restricts which action types are structurally permitted.

Workspace permissions never grant permissions beyond what broader governance allows. This stays aligned with NAVI's architecture, where autonomy never expands what is permitted and owner/system constraints remain authoritative.

---

## 15. Relationship to autonomy, governance, and reflection

Workspace is not a second autonomy system.

### 15.1 Governance

Workspace participates in validation as a scope constraint.

### 15.2 Autonomy

Workspace does not itself determine autonomy level. It only constrains what is in scope.

### 15.3 Interaction

If an action is in scope but still risky, ordinary NAVI confirmation/proposal behavior still applies.

That matches the existing model where Governance defines what is allowed and Autonomy only affects how much NAVI does autonomously within those bounds.

### 15.4 Subconscious and Deep Reflection constraints

Deep Reflection must **not** silently change:

* workspace boundaries (local roots, repo roots)
* the Active Workspace selection
* whitelist rules
* protected paths
* `allowed_actions` values
* any workspace field that constitutes owner-set control state

Deep Reflection may **propose** changes to any of the above through the Proposal Queue. This is consistent with the existing rule that Deep Reflection can only propose changes to owner-tier Configuration and Priorities, never apply them directly.

Shallow Reflections and Consolidation do not interact with workspace state.

---

## 16. Relationship to projects

Projects are defined in the [Project System Specification](project-system-v1.md). The contract for V1 is:

### 16.1 Project binding

A project may bind to one workspace.

### 16.2 Project authority

When operating in a project context, the project-bound workspace becomes the Active Workspace.

---

## 17. Relationship to skills and extensions

Workspace and skills should remain compatible with NAVI's broader capability model, where skills are versioned capability units with permission profiles and contracts.

### 17.1 V1 rule

Workspace may later host workspace-local skills or configuration, but full local override behavior is not required for initial V1 workspace enforcement.

### 17.2 Safety note

If workspace-local skills are later supported, they must still pass through the same governance and permission controls. The broader skills research is clear that installable local skills are powerful but risky without provenance, policy, and auditability.

---

## 18. Audit model

V1 keeps auditability lean but real. Workspace audit data writes directly into History by extending execution outcome records with workspace fields.

### 18.1 Additional fields on execution outcome records

When an action is governed by a workspace, the execution outcome record in History must include:

```yaml
workspace_id: string?
boundary_crossing: boolean
approval_required: boolean
approval_outcome: enum(na, denied, allow_once, always_allow, switched_workspace)
```

These fields extend the existing execution outcome record. They do not create a separate audit stream.

### 18.2 Required event categories

At minimum, the following events must produce History records:

* workspace created
* workspace updated
* workspace activated
* workspace archived or suspended
* protected path blocked
* out-of-scope access attempted
* user approval prompt issued
* boundary crossing approved
* boundary crossing denied
* action executed under workspace

### 18.3 Architectural alignment

This stays consistent with NAVI's rule that every command attempt produces a History record and failure is a first-class recorded state. A separate audit stream may be introduced in later versions if compliance or volume requirements demand it.

---

## 19. Invariants

These are hard V1 rules.

1. Workspace is a first-class Control Entity in the World Model.
2. Workspace is not just a folder label.
3. A user may enable global access and still define workspaces.
4. Only one Active Workspace governs execution at a time.
5. Overlapping workspaces may exist.
6. Overlapping workspaces do not merge permissions.
7. Nested inherited workspaces are out of scope for V1.
8. Boundary crossing must be explicit and user-mediated.
9. Protected paths exist even in global mode.
10. Workspace boundaries must be enforced at execution/policy level, not just in prompt text.
11. Workspace-governed actions and boundary crossing events are recorded in History.
12. Workspace does not override broader governance or owner-set constraints.
13. The operating mode (global/scoped/hybrid) lives in Configuration, not on workspace entities.
14. Deep Reflection may propose but never directly modify workspace state.
15. If no Active Workspace exists and global access is off, scoped execution is blocked and the user is prompted.
16. Whitelist rules must be visible, revocable, and must not grow silently.
17. Workspace switching mid-execution requires re-validation of remaining steps under the new workspace.

---

## 20. V1 exclusions

Explicitly deferred to later versions:

* nested workspace inheritance
* shared workspaces
* team/org governance
* role-based access control
* automatic workspace suggestion
* automatic within-scope replanning
* multi-workspace execution graphs
* service identity model and service-level scoping
* workspace-level service allow/block lists
* advanced service/account policy model
* workspace-local signed extension policies
* full compliance/audit package
* separate audit stream

---

## 21. Implementation order

### Phase 1 — Core model

* define workspace entity
* define workspace kinds
* define Active Workspace selection
* define overlap rules
* implement Configuration-level operating mode setting

### Phase 2 — Enforcement

* path normalization
* root scope checks
* `allowed_actions` deny-mask enforcement
* protected path checks
* boundary crossing prompt flow
* whitelist rule storage with lifecycle fields
* whitelist view/revoke surface
* workspace switching mid-execution (pause, re-validate, resume)
* no-workspace-active blocking for scoped actions

### Phase 3 — UX surface

* settings: global/scoped/hybrid (Configuration)
* workspace create/edit/delete
* project-to-workspace binding
* boundary crossing prompt UI
* whitelist rule management UI

### Phase 4 — Audit

* History record extension with workspace fields
* workspace lifecycle event recording
* basic inspection/explanation surface

---

## 22. Bottom line

V1 is enough if it does these things well:

* supports **global**, **scoped**, and **hybrid** as system-level modes
* keeps **one Active Workspace** with deterministic selection
* blocks scoped execution when no workspace is active and global access is off
* allows **overlap without merging**
* blocks **nested inheritance**
* uses **explicit user-mediated boundary crossing** with lifecycle-managed whitelist rules
* re-validates remaining steps on **workspace switch mid-execution**
* includes **protected paths**
* records **lean but real audit data directly in History**
* keeps workspace state under **owner authority** with Deep Reflection limited to proposals

That gives NAVI a real workspace foundation instead of a fake folder picker, while avoiding the enterprise overdesign that would just slow the system down.
