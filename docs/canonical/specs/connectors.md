# Connectors — Canonical Specification (v2)

**Status:** Canonical (Immutable — changes require human approval)
**Version:** 2.0.0
**Compatibility:** Breaking change from v1; this document defines the new deployment model
**Last Reviewed:** 2026-03-12

Connectors are how NAVI interfaces with the outside world. This document is the **single canonical specification** for connector behavior, boundaries, lifecycle, metadata, governance integration, and runtime contracts in NAVI.

* It **extends** the Connectors section in `docs/canonical/conceptual-design-overview.md`.
* It **normatively constrains** connector-related runtime, registry, manager, gateway, and plugin integration code.
* Architecture overviews, feature catalogues, plans, and reviews **must not contradict** this document. If they do, this document wins.

---

## 1. Definition and Role

### 1.1 Canonical definition

A **connector** is an integration adapter that lets NAVI interact with external channels, systems, services, devices, and identity-backed environments. Connectors:

* translate NAVI-internal effect execution into system-specific operations
* handle authentication, permissions, protocol details, and data normalization at the integration boundary
* normalize external events into NAVI’s internal event and message surfaces

Connectors are part of the **Capability Layer**. They sit below the Cognitive Layer and above concrete transports, provider SDKs, HTTP APIs, local processes, device bridges, and other execution-specific mechanisms.

### 1.2 Relationship to Commands, Skills, and Plugins

Within the Capability Layer:

* **Commands** define what NAVI wants to do
* **Connectors** define where and how an action crosses system boundaries
* **Plugins** package reusable capabilities built on top of Commands, Connectors, and Skills
* **Skills** are governed, declarative capability modules that may depend on connectors but do not replace them

Connectors are **distinct from**:

* **Skills** — declarative, versioned capability modules exposed as governed interfaces
* **Plugins** — installable packages that combine skills, commands, connectors, permissions, and interfaces into a reusable unit of behavior

### 1.3 Connector categories

Connector categories are conceptually exhaustive at the architecture level:

* Communication connectors
* Information connectors
* Service connectors
* Device and environment connectors

Identity and Access is a **cross-cutting plane**, not a peer connector category. Every connector depends on it for authentication, permissions, scopes, consent, and account linkage.

---

## 2. Design Principles

### 2.1 Capability-first, not provider-first

NAVI must reason about normalized connector capabilities, not vendor API methods.

Canonical examples:

* `messaging.send`
* `messaging.fetch`
* `artifact.search`
* `calendar.create_event`
* `issue_tracker.create_issue`
* `notifications.stream`

Provider-native operations such as Slack-, Telegram-, or GitHub-specific API calls are implementation details hidden behind connector drivers.

### 2.2 Runtime-managed, not hardcoded

Connector support must be runtime-managed. The canonical model is:

* runtime driver registration
* runtime instance registration
* metadata-based capability discovery
* lifecycle-managed activation and retirement

Static provider-shaped config branches are **not canonical** in v2.

### 2.3 Multi-instance by default

The system must support multiple live instances of the same connector kind.

Examples:

* multiple Telegram bots
* multiple Slack workspaces
* personal and work GitHub connectors
* sandbox and production SaaS instances

No connector design may assume a single global instance per provider.

### 2.4 Governed by default

Connectors never bypass permissions, policy, configuration, or risk checks. Skills are already required to execute under governance rather than bypass it, and connector-backed capability execution must preserve that same rule.

### 2.5 Deterministic surfaces

The connector system must expose stable identities, capability names, metadata, result envelopes, and health states so the Cognitive Layer and governance systems do not depend on connector-specific special cases. Skills already require deterministic surfaces and bounded structured results; connector contracts follow the same principle.

### 2.6 Thin adapters, not mini-apps

Connectors remain thin integration adapters. They do not own planning, reasoning, long-term world state, or business workflow logic. Cross-cutting workflows belong in Skills, Commands, Plugins, and the Cognitive Layer.

---

## 3. Core Invariants

The following invariants apply to all connectors:

1. **No cognition in connectors** — connectors never perform LLM reasoning, planning, or high-level decision-making.
2. **No governance bypass** — connectors never skip policy, permission, configuration, risk, or proposal checks. This mirrors the invariant that governed skills cannot bypass governance.
3. **No direct World Model writes** — connectors do not mutate the World Model directly.
4. **No proposal resolution authority** — connectors may participate in execution but may not resolve Proposals; proposal resolution is owner- or system-driven only. 
5. **No hidden durable truth** — connectors do not own private durable state that materially affects behavior outside declared connector instance state.
6. **Explicit side-effect semantics** — connector actions must declare side-effect class, idempotency, reversibility, and risk-relevant metadata.
7. **Explicit degradation** — failed connectors degrade visibly; they do not disappear silently.

---

## 4. Canonical Runtime Model

### 4.1 Connector Driver

A **Connector Driver** is reusable code that knows how to communicate with one provider or environment type.

Examples:

* `builtin.telegram`
* `builtin.slack`
* `builtin.github`
* `builtin.filesystem`
* `plugin.gmail`

A driver is not tied to a specific account, workspace, tenant, or credential set.

### 4.2 Connector Instance

A **Connector Instance** is a configured runtime deployment of a driver.

Examples:

* `telegram_owner`
* `telegram_support`
* `slack_internal_workspace`
* `github_company_org`
* `filesystem_local_home`

An instance is the canonical unit of operational identity.

### 4.3 Connector Registry

The **Connector Registry** is the source of truth for:

* available drivers
* registered instances
* capability declarations
* compatibility metadata
* policy attachment points
* health and lifecycle visibility

### 4.4 Connector Manager

The **Connector Manager** is the runtime supervisor responsible for:

* activating and deactivating instances
* routing actions
* ingesting events
* health supervision
* degradation propagation
* telemetry emission
* runtime reload and retirement

This aligns with the general plugin lifecycle model of discover, install, register, activate, invoke, update, and retire, but applies it specifically to connector runtime management.

---

## 5. Canonical Identities and Naming

### 5.1 Driver identity

Each driver must declare:

* `driver_id`
* `semver`
* `kind`
* `display_name`

### 5.2 Instance identity

Each instance must declare:

* `instance_id`
* `driver_id`
* `enabled`
* `labels`
* `policy_refs`
* `secret_refs`

`instance_id` is the stable runtime key for routing, telemetry, health, and governance references.

### 5.3 Capability naming

Connector-exposed capabilities must use canonical normalized names such as:

* `messaging.send`
* `messaging.edit`
* `artifact.read`
* `artifact.search`
* `calendar.create_event`

Provider-native method names are not canonical capability identities.

---

## 6. Required Metadata

Each driver must declare metadata sufficient for execution, governance, and capability discovery.

### 6.1 Driver metadata

Required fields:

* `driver_id`
* `semver`
* `kind`
* `display_name`
* `capabilities[]`
* `config_schema`
* `auth_schema`
* `event_model`
* `health_support`
* `compatibility`

### 6.2 Capability metadata

Each capability must declare at minimum:

* `name`
* `input_schema`
* `output_schema`
* `side_effect_class`
* `idempotency_class`
* `reversibility`
* `risk_hint`
* `required_permissions`
* `supports_dry_run` if applicable
* `supports_retry` if applicable

This follows the same design discipline as skills, which must declare structured inputs, structured outputs, effects, security posture, and governance-relevant metadata.

### 6.3 Instance metadata

Each instance must expose:

* `instance_id`
* `driver_id`
* `status`
* `health_state`
* `auth_state`
* `granted_scopes`
* `labels`
* `policy_refs`
* `last_healthy_at`
* `last_error_at`

---

## 7. Action Model

### 7.1 Action surface

Connector actions are outbound operations against external systems.

Examples:

* send message
* edit message
* search files
* fetch document
* create calendar event
* create issue
* invoke remote operation

Connector actions must be capability-based and schema-validated.

### 7.2 Action execution path

Canonical path:

Cognitive Layer → Validate/Govern → Capability resolution → Connector Manager → Connector Instance → Connector Driver → External system

The conceptual design already defines that Validate/Govern checks permissions, policy, configuration, priority alignment, and risk in deterministic order before Execute. Connector execution must fit inside that path, not bypass it. 

### 7.3 Result envelope

All connector actions must return a normalized result envelope.

Minimum fields:

* `status`
* `output`
* `error`
* `retryable`
* `connector_instance_id`
* `capability`
* `correlation_id`
* `duration_ms`
* `provider_metadata`

This parallels the skill execution result envelope requirement, where transport-specific execution is normalized into a structured result so upper layers do not need transport-specific logic.

### 7.4 Error classification

Connector action failures must distinguish at minimum:

* transient failure
* permanent failure
* auth failure
* permission failure
* rate limit
* timeout
* partial success
* circuit open
* misconfiguration

Returning plain untyped failure without classification is non-compliant.

---

## 8. Event Model

### 8.1 Event surface

Connector events are inbound observations from external systems.

Examples:

* webhook message received
* document changed
* issue updated
* calendar invite received
* device state changed
* stream event arrived

### 8.2 Supported ingress mechanisms

Connectors may ingest events through:

* webhooks
* long-poll
* streaming APIs
* periodic polling
* local process callbacks
* device bridges

### 8.3 Normalized event envelope

All inbound connector events must be normalized into an event envelope with at minimum:

* `connector_instance_id`
* `event_type`
* `payload`
* `provider_metadata`
* `received_at`
* `correlation_id` where available

### 8.4 Action/event separation

Action execution and event ingestion are separate canonical surfaces. A connector may support one, the other, or both.

---

## 9. Identity, Authentication, and Permissions

Identity and Access is cross-cutting and underpins every connector through authentication, permissions, scopes, consent, and account linkage.

### 9.1 Required auth separation

Connector implementation must separate:

* driver logic
* instance config
* secret references
* auth/session state
* granted scopes
* consent/account linkage

### 9.2 Canonical auth states

At minimum:

* `unconfigured`
* `configured`
* `authorized`
* `expired`
* `revoked`
* `invalid`

### 9.3 Least privilege

Connectors must request and operate with the narrowest scope consistent with declared capabilities.

---

## 10. Governance and Autonomy Integration

### 10.1 Governance visibility

Connector metadata must be visible to the governor and policy systems before execution.

### 10.2 Required governance fields

At minimum:

* `risk_hint`
* `side_effect_class`
* `idempotency_class`
* `reversibility`
* `required_permissions`
* `scope_sensitivity`
* `data_sensitivity`
* `external_network_required`

### 10.3 Autonomy limits

Connector execution remains bounded by system-tier policy, owner constraints, proposal-required categories, irreversible-action rules, and risk override behavior. Autonomy settings do not bypass hard floors.

### 10.4 Proposal rules

Any connector-backed action that falls into a proposal-required class must generate or depend on the normal Proposal path. Connectors may execute approved actions; they may not create alternative approval channels and may not resolve proposals themselves.

---

## 11. Reliability and Failure Model

### 11.1 Canonical health states

Each connector instance must expose one of:

* `healthy`
* `degraded`
* `unavailable`
* `auth_invalid`
* `rate_limited`
* `disabled`
* `misconfigured`

### 11.2 Required reliability behaviors

Connector runtime must support:

* retry policy
* backoff policy
* timeout enforcement
* circuit breaker behavior
* duplicate event handling
* idempotency-aware retry
* partial-failure reporting

### 11.3 Degradation rules

When a connector instance is degraded or unavailable:

* dependent capability routing must reflect that state
* health APIs must surface that state
* planner/governor logic must receive consistent visibility
* the instance must not be treated as silently absent

### 11.4 Idempotency expectations

Where connector capabilities map onto primitive command semantics, they must preserve those semantics. For example, Send is not inherently idempotent, Acquire may or may not be idempotent depending on source mutability, and Schedule is not inherently idempotent. Connector contracts must declare retry and deduplication behavior explicitly rather than assuming safety.

---

## 12. Observability and Audit

Each connector instance must emit structured telemetry and audit records.

Minimum observability surface:

* health state
* action latency
* success/failure counts
* retry counts
* rate-limit incidents
* auth failures
* circuit breaker state
* event lag
* correlation IDs

Every connector action attempt must produce an execution outcome record. This is consistent with the broader model in which command attempts are recorded whether they succeed, fail, time out, or are rejected. 

---

## 13. Configuration Model

### 13.1 Breaking-change rule

v2 rejects provider-shaped static config as canonical.

The following pattern is non-canonical in v2:

* `connectors.telegram`
* `connectors.slack`
* `connectors.github`

### 13.2 Canonical config shape

The canonical boot configuration contains only runtime bootstrap settings and optional bootstrap instances.

Conceptual shape:

```yaml
connectors:
  runtime:
    registry_backend: sqlite
    health_check_interval: 30s
  instances:
    - instance_id: telegram_owner
      driver_id: builtin.telegram
      enabled: true
      config:
        webhook_mode: true
      secret_refs:
        bot_token: secret://telegram/owner_bot_token
      policy_refs:
        - policy://messaging/default
      labels:
        domain: messaging
        trust_tier: builtin
```

### 13.3 Persistence

Connector instance state must be persisted separately from immutable boot config.

Persisted state must include:

* instance identity
* config schema version
* auth state
* granted scopes
* health state
* event cursors/checkpoints
* disablement reason
* audit metadata

### 13.4 Current implementation (non-canonical)

Current config and Go code still use **provider-shaped** connector configuration (`ConnectorsConfig.Telegram`, `ConnectorsConfig.Slack` in `internal/config/config.go` and optional env overrides). The full v2 config shape (§13.2 — `connectors.runtime`, `connectors.instances[]`, `secret_refs`, etc.) is not yet implemented. This note documents the gap; the canonical v2 spec above remains the target.

---

## 14. Remote and Out-of-Process Connectors

### 14.1 First-class support

Out-of-process and remote connectors are first-class citizens in v2, not second-class bridge exceptions.

### 14.2 Required contract

A remote connector must still participate in the same canonical model:

* driver identity
* instance identity
* capability metadata
* action/result envelope
* event envelope
* health reporting
* auth model
* compatibility version

### 14.3 Isolation principle

Connector isolation is desirable where practical. Remote connectors must be designed so their failure does not take down the core NAVI runtime.

---

## 15. Plugin and Skill Integration

### 15.1 Skills may depend on connectors

Skills may depend on connectors, but that dependency must be explicit and governed. Skills are runtime-discovered governed capabilities; connectors remain shared integration surfaces beneath them.

### 15.2 Plugins may package connector-heavy capability

Plugins may combine skills, commands, connectors, permissions, interfaces, and policies into reusable modules. Integration plugins are often connector-heavy, but plugins consume the connector runtime; they do not replace it.

### 15.3 No duplicated connector substrate

A plugin or skill must not create an alternative private connector model that bypasses registry, governance, health, or telemetry.

---

## 16. Go-Level Runtime Contracts

This document is canonical for architecture and invariants. Exact field names and method signatures are owned by Go code, but the runtime must implement the following conceptual interfaces:

* `ConnectorDriver`
* `ConnectorInstance`
* `ConnectorRegistry`
* `ConnectorManager`
* `ConnectorActionHandler`
* `ConnectorEventSource`
* `ConnectorHealthChecker`

### 16.1 Minimum driver responsibilities

A driver must provide:

* metadata
* config validation
* capability declaration
* instance construction
* compatibility information

### 16.2 Minimum instance responsibilities

An instance must provide:

* lifecycle control
* action execution
* event source handling where applicable
* health checking
* shutdown behavior
* metadata/state exposure

### 16.3 Extensibility rule

New optional capability-specific behavior should be added through separated interfaces or capability declarations, not by bloating one universal base interface.

---

## 17. Lifecycle

Each connector instance follows this lifecycle:

* discover
* validate
* register
* activate
* invoke / ingest
* degrade / recover
* update
* disable
* retire

Lifecycle transitions must be explicit and observable.

---

## 18. Versioning

### 18.1 Semver required

Connector drivers and connector spec contracts must use semantic versioning. Skills already require semver and treat breaking changes as new major versions; connectors follow the same rule.

### 18.2 Breaking changes

Breaking changes include:

* removing or renaming a capability
* incompatible input or output schema changes
* changing action/result envelope semantics
* changing governance-relevant metadata semantics
* changing auth contract or compatibility behavior in ways that break consumers

### 18.3 No backward-compatibility guarantee from v1

This document defines a new major line. Existing v1 connector assumptions are not canonical under v2.

---

## 19. Compliance Criteria

A connector runtime is compliant with v2 only if all of the following are true:

1. New connector types can be added without expanding a provider-shaped static config tree.
2. Multiple instances of one connector kind can operate at the same time.
3. Connector capabilities are declared through normalized capability metadata.
4. Action results use a normalized result envelope.
5. Inbound events use a normalized event envelope.
6. Governance can inspect connector metadata before execution.
7. Auth, permissions, and secret references are separated from connector business logic.
8. Connector health and degradation states are explicit and visible.
9. Plugins and skills can depend on connectors without bypassing connector runtime controls.
10. Connector execution does not bypass proposals, governance, or owner constraints.

---

## 20. Reference Implementations

Initial reference drivers should include at minimum:

* Telegram
* Slack
* GitHub
* Filesystem

These reference drivers should demonstrate:

* driver registration
* instance registration
* capability declaration
* action execution
* event handling
* health reporting
* auth/scopes separation
* degradation behavior

---

## 21. Explicit Non-Goals

This document does not:

* redefine primitive Commands
* make connectors responsible for cognition
* permit connectors to own business workflows
* allow connectors to resolve Proposals
* require every connector to support every action or event mode

---

## 22. Migration Note

There is no backward-compatibility requirement from v1 to v2 in this deployment model. Existing v1 connector assumptions, config branches, and interface expectations may be removed or rewritten to match this spec.

---

This is the right direction if you want connectors to be a real long-term substrate rather than just “the current Telegram/Slack layer.”

The next thing to do is draft the **Go interface section** that corresponds to this spec so engineering has a concrete contract to implement.
