**Status:** Evolving  
**Last Updated:** 2026-04-15  

# Tool Registry and Tool Index Design

## Purpose

Define the authoritative data model for tools and the searchable index built on top of it.

This document exists because the rest of the tool system depends on two different things:

- a **registry** that knows what tools actually exist
- an **index** that makes those tools discoverable and rankable

If these are blurred together, the system becomes ambiguous and hallucination-prone.

---

## Core Principle

The Tool Registry is the source of truth.

The Tool Index is a search/view layer over the registry.

The registry answers:
- what tools exist?
- what are their canonical identities?
- what metadata and execution contracts do they carry?

The index answers:
- what tools are relevant to this request?
- what tools are similar?
- what tools are discoverable in this context?

---

## Why Both Are Needed

A registry without an index gives you exact lookup but poor discovery.

An index without a registry gives you fuzzy search without authority.

NAVI needs both.

The hallucinated tool problem you are seeing is partly a failure of this distinction:

- the model inferred a plausible tool concept
- the runtime had no clean discovery/identity loop to resolve it safely
- rejection happened too late and too bluntly

---

## Tool Registry Responsibilities

The Tool Registry is responsible for:

1. Canonical tool identity.
2. Tool metadata storage.
3. Tool schema/version tracking.
4. Tool source tracking.
5. Collision prevention.
6. Environment partition awareness.
7. Governance metadata completeness.
8. Registry snapshot generation for runtime use.

The registry is not responsible for semantic ranking or fuzzy matching.

---

## Tool Index Responsibilities

The Tool Index is responsible for:

1. Searchable projection of registry content.
2. Semantic and lexical retrieval.
3. Alias and keyword matching.
4. Domain and tag-based filtering.
5. Related-tool suggestion.
6. Missing-capability support signals.
7. Policy-filtered discoverability views.

The index is not responsible for execution authority.

---

## Canonical Tool Identity

Every tool must have a stable canonical identity.

Suggested shape:

```text
namespace.category.tool_name
```

Examples:

- `navi.files.read`
- `repo.search`
- `gmail.create_draft`
- `calendar.read_events`
- `navi.diagnostics.session_errors`

Rules:

- identity must be globally unique within a registry
- identity must be stable across sessions
- identity must not depend on model/provider names
- identity must not be guessed from user-facing descriptions

The system must never rely on display names as identities.

---

## Tool Record

Each registry entry should be a first-class Tool record.

Minimum required fields:

```json
{
  "tool_id": "navi.files.read",
  "display_name": "Read File",
  "description": "Read a file from the workspace.",
  "source_type": "builtin | skill | plugin | connector | selfmod | diagnostic",
  "source_id": "...",
  "schema_version": "1.0.0",
  "input_schema": {},
  "output_schema": {},
  "category": "read_only",
  "risk_tier": 1,
  "side_effects": ["local_read"],
  "reversibility": "reversible",
  "environment_visibility": ["development", "staging", "production"],
  "required_mode": [],
  "required_authority": "user",
  "feature_flags": [],
  "connector_dependencies": [],
  "trust_tier": "trusted_internal",
  "aliases": ["read file", "open file"],
  "capability_tags": ["files", "workspace", "read"],
  "status": "active"
}
```

A tool that lacks required governance or execution metadata should not be considered registry-valid.

---

## Registry Validity Rules

A registry entry is valid only if:

- `tool_id` is unique
- required metadata fields are present
- schema versions are defined
- category and risk tier are defined
- source is traceable
- environment visibility is explicit
- governance metadata is present

Invalid tools must not be surfaced to discovery or broker loading.

---

## Registry Partitioning

The registry should be partition-aware.

At minimum, tool records should be scoped or filtered by:

- environment
- tenant/workspace if applicable
- feature flags
- installation state

Two acceptable models:

### Model A: Separate Registries Per Environment

- production registry
- staging registry
- development registry
- test registry

### Model B: Single Registry with Hard Environment Filtering

Tool records include explicit environment visibility and the runtime enforces hard partitioning.

Either model is acceptable, but soft prompt-only filtering is not.

---

## Source Types

The registry should explicitly track where tools come from.

Recommended source types:

- builtin
- skill
- plugin
- connector
- selfmod
- diagnostic
- mcp_remote

Why this matters:

- debugging provenance
- governance differentiation
- health/dependency mapping
- migration and deprecation

Tool source type is not sufficient for trust or risk on its own, but it is required context.

---

## Collision Handling

Tool identity collisions must be treated as registry errors.

Examples:

- a plugin tries to register `navi.files.read`
- two skills expose the same canonical tool id
- a dev/test tool shadows a production tool id

Allowed behavior:
- reject registration
- log collision with both sources
- require explicit human resolution

Not allowed:
- silently last-write-wins
- auto-renaming without traceability

---

## Schema Versioning

Each tool must have a schema version.

Why:

- provider adapters need stable contracts
- Active Tool Sets need version consistency
- stale exposure protection depends on this

Recommendations:

- semver-style versioning is fine
- breaking input changes require version bump
- runtime should detect version drift between load and exposure

The canonical identity of a tool is not the same thing as its schema version.

---

## Registry Snapshots

The runtime should be able to create immutable or traceable registry snapshots for:

- broker decisions
- active tool set creation
- exposure plans
- execution traces

This allows you to answer:

- what tools existed at this point in time?
- what version of a tool was selected?
- was the tool stale by the time execution happened?

---

## Tool Index Structure

The Tool Index is a projection derived from valid registry records.

Each indexed entry should include at least:

- canonical tool id
- display name
- description
- aliases
- category
- capability tags
- domain tags
- source type
- risk tier
- environment visibility
- current status
- optional embeddings or semantic vectors
- usage stats signals

The index may denormalize registry data for faster retrieval, but the registry remains authoritative.

---

## Index Search Modes

The index should support at least:

### Exact Lookup

Used when a tool id or exact name is requested.

Examples:
- `navi.files.read`
- `self_diagnostic_recent_errors`

Purpose:
- hallucination detection
- precise identity resolution

### Alias / Lexical Search

Used for keyword-based matching.

Examples:
- `read file`
- `send email`
- `calendar availability`

### Semantic Search

Used for fuzzy capability matching.

Examples:
- `inspect recent errors`
- `look up meeting conflicts`
- `create a draft but do not send`

### Related Tool Retrieval

Used when exact match fails but nearby capabilities may help.

Examples:
- exact miss for `self_diagnostic_recent_errors`
- related result: `navi.diagnostics.session_errors`

---

## Index Ranking Signals

Ranking should consider:

### Positive signals

- exact match
- alias match
- semantic similarity
- domain match
- capability tag overlap
- recent successful use
- workflow-state relevance
- low-risk fit

### Negative signals

- high risk relative to intent
- stale or deprecated status
- wrong source/context
- schema complexity for weak model
- past false-positive history

### Hard exclusions

- hidden by environment
- hidden by authority
- removed status
- registry invalidity

---

## Index Status vs Registry Status

The registry may know many statuses.

The index should expose only what is useful for discovery.

Examples:

Registry status values may include:
- active
- deprecated
- suspended
- removed
- invalid

Index/discovery-facing status values may include:
- discoverable
- visible_unavailable
- hidden
- related_only

Do not collapse all status concepts into one overloaded field.

---

## Missing Capability Detection

The index should help support missing-capability detection, but must not fabricate tools.

When no suitable tool exists:
- exact miss recorded
- semantic matches absent or weak
- related tools insufficient

The system may emit a missing-capability record, but the registry should remain unchanged until a real tool is added.

This is how you support future skill/plugin growth without runtime hallucination.

---

## Registry ↔ Index Sync

Whenever a tool is:
- added
- updated
- deprecated
- suspended
- removed
- version-bumped
- environment-restricted

…the index must be updated or rebuilt.

This sync can be:
- event-driven
- periodic rebuild
- hybrid

But drift between registry and index must be detectable and observable.

---

## Health and Dependency Awareness

The registry should know dependency declarations.

Examples:
- requires Gmail connector
- requires auth scope X
- requires workspace write access

The index may reflect availability projections derived from those dependencies, but those are still advisory for discovery.

Actual execution eligibility is decided later by broker + governance + runtime.

---

## Query Examples

### Exact Tool Identity Lookup

```json
{
  "mode": "exact",
  "query": "self_diagnostic_recent_errors"
}
```

Possible result:

```json
{
  "exact_match": null,
  "related_matches": [
    {
      "tool_id": "navi.diagnostics.session_errors",
      "relevance": 0.83,
      "status": "visible_unavailable",
      "reason": "requires_debug_mode"
    }
  ]
}
```

### Semantic Discovery Query

```json
{
  "mode": "semantic",
  "query": "inspect recent runtime errors",
  "mode_context": "companion",
  "environment": "production"
}
```

### Lexical Query

```json
{
  "mode": "lexical",
  "query": "calendar availability"
}
```

---

## Observability

Registry events to log:

- registration success/failure
- collision detection
- version change
- deprecation
- suspension
- removal
- environment visibility changes

Index events to log:

- query type
- candidate count
- exact match hit/miss
- top related matches
- hidden count
- ranker version
- missing-capability output

---

## Quality Metrics

Track:

### Registry Metrics

- invalid tool rate
- collision rate
- missing metadata rate
- source-type distribution
- schema version drift rate

### Index Metrics

- exact hit rate
- related-match rate
- no-result rate
- irrelevant-result rate
- hallucinated tool exact-miss rate
- search-to-load success rate

---

## Anti-Patterns

### Anti-Pattern 1: Registry as Fuzzy Search Layer

Wrong because authority and discovery get blurred.

### Anti-Pattern 2: Index as Source of Truth

Wrong because search results are not canonical execution contracts.

### Anti-Pattern 3: Display Name as Identity

Wrong because names drift, collide, and are not machine-stable.

### Anti-Pattern 4: Auto-Creating Tools from Missing Capability Signals

Wrong because discovery should identify gaps, not hallucinate implementations.

### Anti-Pattern 5: Silent Collision Resolution

Wrong because it makes debugging and safety impossible.

---

## Hard Constraints

- Every tool must have a canonical tool id.
- The registry is the only source of truth for tool existence.
- The index may only project valid registry entries.
- Discovery results must never imply execution authority.
- Collision handling must be explicit and fail closed.
- Schema version drift must be detectable.

---

## Design Position

The Registry gives NAVI authority.

The Index gives NAVI capability awareness.

The broker depends on both.

Without a clean registry, tools cannot be trusted.

Without a clean index, tools cannot be found safely.

This is the substrate beneath the rest of the tool system.
