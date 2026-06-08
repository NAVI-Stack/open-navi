# NAVI System Architecture Guide

Visual architecture artifact: [Open the HTML guide](./navi-system-architecture-guide.html)

**Status:** Living architecture reference
**Last reviewed:** 2026-06-04
**Scope:** NAVI core runtime, daemon, CLI, gateway, inference control, governance, tools/plugins, world model, memory/state, runtime infrastructure, Experience Layer / Persona System, and first-class React/web data-driven rendering boundaries.

## Purpose

This wrapper exists so the architecture guide is discoverable in normal GitHub Markdown browsing while the full visual artifact remains a self-contained HTML file.

## Source of Truth

The guide is grounded in repository code and implementation documents. Planned, partial, inferred, and missing areas are labeled inside the HTML artifact rather than treated as implemented.

For generated UI, data-driven rendering, and OpenUI integration policy, use [Data-Driven UI Rendering — Canonical Principle](../canonical/data-driven-ui-rendering.md), [Data-Driven UI Rendering Architecture](./data-driven-ui-rendering.md), [ADR-013: OpenUI as NAVI's First-Class React/Web Data-Driven Renderer](../adr/ADR-013-openui-data-driven-rendering.md), and [NAVI UI Library (`@navi/ui`) — Architecture & Roadmap](./navi-ui-library.md). The locked decision is that NAVI owns meaning, component semantics, action authority, governance, artifacts, and renderer selection; OpenUI is the first-class React/web data-driven rendering lane.

## Placement

Recommended file placement:

```text
docs/architecture/navi-system-architecture-guide.md
docs/architecture/navi-system-architecture-guide.html
```

## Maintenance Rule

Update this artifact after major changes to any of these areas:

- `cmd/navid` daemon composition
- `internal/navi` agent loop/runtime execution
- `internal/navi/inference` Inference Control System
- `internal/governor` governance and validation rules
- `internal/tool`, `internal/navi/skill`, `plugins`, or `skills`
- `internal/worldmodel`, `internal/store`, or `internal/schema`
- `internal/navi/experience`, `config/personas`, or gateway experience endpoints
- `internal/gateway` API surface
- `web-src/packages/navi-ui`, generated UI engines, renderer adapters, data-driven render payloads, OpenUI integration, or chat render surfaces
- `Dockerfile`, `compose*.yml`, or runtime deployment model
