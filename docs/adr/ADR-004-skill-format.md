# ADR-004: Skill Format Standard — SKILL.md vs. SKILL.yaml (OSS-27)

**Status:** Accepted
**Date:** 2026-03-11
**Deciders:** NAVI AI Core Team
**Related:** [Skills System](../concepts/skills.md) · [ADR-001](./ADR-001-go-orchestration-python-ai.md)

---

## Context

NAVI AI uses a zero-code skill system where agent capabilities are defined in files discovered at runtime. Two file formats have co-existed since the skills system was first introduced:

- **`SKILL.md`** — A Markdown file containing a display name, description, usage instructions, and optional examples. Simple to author, readable by humans, parsed via lightweight regex extraction.
- **`SKILL.yaml` (OSS-27 format)** — A structured YAML document following the proposed OSS-27 Skill Standard. Declares transport metadata, capability scopes, authentication requirements, sandbox constraints, and versioning.

The core team needed to decide: should the system standardize on one format, keep both, or adopt a migration path?

---

## Decision

**Both formats are supported. OSS-27 (`SKILL.yaml`) is required for all new skills that interact with external systems, require authentication, declare capability scopes beyond `fs.read`, or target NAVI Net distribution.**

`SKILL.md` remains valid for:
- Conversational and informational skills (no external I/O)
- Internal workflow skills that only read context provided by the CEO loop
- Skills authored by non-technical users who don't need the full metadata model

The skill loader (`internal/navi/skill/loader.go`) prefers `SKILL.yaml` when both are present in the same directory. A future linter will warn when `SKILL.md`-only skills attempt to declare network or filesystem capabilities in their instruction text.

---

## Rationale

**Why keep `SKILL.md` at all?**

The Markdown format has significantly lower authoring friction. A user writing a personal workflow automation skill should not need to understand capability scopes and semver to make something work. `SKILL.md` serves this population well. Removing it would break all existing third-party skills and eliminate the easiest on-ramp to the system.

**Why require OSS-27 for external-facing skills?**

Skills that invoke external APIs, write to the filesystem, or communicate over the network carry meaningful security surface. Without a structured manifest, the Go orchestration layer cannot:
- Enforce capability scope checks before skill execution begins
- Present meaningful permission prompts to the user on first install
- Pass the skill's capability declaration to NAVI OS for sandbox configuration
- Support NAVI Net distribution (the Registry requires a machine-readable manifest)

OSS-27 solves all four requirements with a format that is validated at load time by the skill loader's JSON Schema validator.

**Why OSS-27 specifically?**

OSS-27 is the emerging open standard for skill/tool manifests in the agentic AI space. Adopting it means NAVI AI skills can be shared with and consumed by other OSS-27-compatible runtimes. The Go schema validator was generated directly from the OSS-27 JSON Schema, so compatibility is maintained mechanically rather than by convention.

---

## Consequences

**Positive:**
- Clear authoring guidance: simple skills use Markdown, complex skills use YAML
- Security: capability scope enforcement is only possible with structured metadata
- Interoperability: OSS-27 skills are portable to other runtimes
- Registry-ready: NAVI Net distribution requires the manifest; all registry skills are already in OSS-27 format

**Negative:**
- Two codepaths to maintain in the skill loader
- Potential confusion for authors who start with `SKILL.md` and need to migrate to `SKILL.yaml` when they add external I/O
- OSS-27 spec is not yet finalized; format changes may require migrations

**Migration path:**
A migration utility (`navi skill migrate <dir>`) will be provided that reads a `SKILL.md`, extracts structured fields where possible, and outputs a `SKILL.yaml` scaffolded with the correct OSS-27 fields. The author then fills in the capability declarations.

---

## Alternatives Considered

| Option | Rejected Because |
|---|---|
| SKILL.md only | Cannot express capability scopes, authentication, or sandbox constraints in a machine-readable way |
| SKILL.yaml only | Too high authoring friction for simple conversational skills; breaks all existing skills |
| Custom proprietary format | Misses interoperability opportunity; maintenance burden without standards backing |
| JSON manifests | Less human-readable; no advantage over YAML for this use case |

---

*See also: [Skills System Concepts](../concepts/skills.md) · [ADR-001: Go/Python split](./ADR-001-go-orchestration-python-ai.md) · [NAVI Net Module Registry](../../../../projects/navi-net/docs/architecture/README.md)*
