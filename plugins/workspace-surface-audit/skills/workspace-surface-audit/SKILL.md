---
name: workspace-surface-audit
description: "Audit the active NAVI workspace, repo, tools, connectors, and env-backed surfaces, then recommend the highest-value next capabilities to add or refine."
metadata:
  navi:
    emoji: "🧭"
---
# Workspace Surface Audit

Use this skill when the user asks what NAVI can already do in the current workspace, what is wired up, what is missing, or what should be added next.

This is a NAVI-native, repo-local port of a high-signal setup-audit pattern. It is advisory and read-only unless the user explicitly asks for follow-up changes.

## When To Use

- "What can this repo do right now?"
- "What skills, connectors, or MCP servers are available?"
- "What should we add next to improve the agentic pipeline?"
- "Can you review the current setup before we install more things?"
- "What is missing between the repo, runtime, and operator surfaces?"

## Non-Negotiable Rules

- Never print secret values. Surface only provider names, file paths, and whether relevant config exists.
- Separate what is available now from what is only partially wrapped and from what is genuinely missing.
- Prefer NAVI-native skills, runbooks, or docs updates before recommending a new external shim.
- Treat external harness files as inspiration, not as authoritative boundaries for NAVI.

## Audit Inputs

Inspect only the surfaces needed to answer well:

1. Repo surface
   - `README.md`, `AGENTS.md`, `CLAUDE.md`, `docs/`, `skills/`, `config/`, `compose*.yml`, `Makefile`
2. Runtime surface
   - `config/runtime.yaml`, `workspace/`, `workspace/connectors/`, prompt/template files, gateway/operator docs
3. Integration surface
   - connector configs, plugin manifests, MCP config files, bridge or subprocess connector folders
4. Environment surface
   - `.env*` files, local config overlays, and only the names of important keys
5. Skill surface
   - existing OSS-27 skills, advisory `SKILL.md` guides, and operator capabilities already documented in NAVI

## Audit Process

### Phase 1: Inventory What Exists

Produce a compact inventory of:

- active runtime entry points
- built-in and workspace skills
- configured connectors and bridges
- MCP or plugin-style surfaces
- env-backed integrations implied by key names
- docs or runbooks that already cover operator workflows

### Phase 2: Distinguish Wrapped vs Primitive

For each meaningful surface, say whether it is:

- already usable as a clean NAVI workflow
- present only as a primitive that lacks a polished skill or runbook
- missing entirely and still requiring a new integration

### Phase 3: Recommend The Right NAVI Shape

When you find a gap, recommend the correct shape:

| Gap Type | Prefer |
|----------|--------|
| Repeatable operator workflow | Skill |
| Docs or setup drift | Doc update or runbook |
| Specialized delegated role | Worker or agent prompt update |
| External tool bridge | Connector, plugin, or MCP integration |
| Repo-local execution truth | `WORKING_CONTEXT.md` or workspace docs |

## Output Format

Return five sections in this order:

1. **Current surface**
2. **Already wrapped well**
3. **Primitive-only gaps**
4. **Missing integrations**
5. **Top 3-5 next moves**

## Good Outcomes

- The user can tell what NAVI can do right now without guessing.
- Recommendations are concrete enough to implement without another discovery pass.
- The answer stays organized around workflows and operator value, not just tool brands.
