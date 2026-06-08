# ADR-010: NAVI Console Gateway Hosting

**Status:** Accepted  
**Date:** 2026-05-02  
**Deciders:** Eric (Owner)  
**See also:** [Design: NAVI Console](../design/navi-console.md), [Spec: Gateway API](../specs/gateway-api.md), [Spec: NAVI Console V1](../specs/navi-console-v1.md)

---

## Context

NAVI Console needs a local, backend-hosted control surface for runtime clarity, debugging, inspection, onboarding, and operator workflows.

The existing runtime already has the core hosting substrate:

- `internal/gateway/server.go` owns the HTTP gateway and route registration.
- `gateway.Config` has `Addr` and `StaticDir` fields.
- `NewServer` defaults to `Addr: ":8080"` only when config does not provide one.
- `NewServer` defaults `StaticDir` to `web` only when config does not provide one.
- The gateway registers `GET /` as a static file server backed by `os.DirFS(s.cfg.StaticDir)`.
- `config/runtime.yaml` sets `gateway.addr: ":6284"` and `gateway.static_dir: "web"`.
- `internal/config/config.go` defaults the gateway to `:6284`, `StaticDir: "web"`, and localhost-oriented CORS origins.
- Existing authenticated API surfaces already cover much of the console inventory: status, chats, proposals, runs, activity, skills, tools, connectors, LLM, experience, presence, errors, debug events, runtime metrics, and `/ws/live`.

That means the console should not begin by creating another HTTP server, another port, or another independent auth boundary.

---

## Decisions

### D1. NAVI Console is hosted by the existing gateway

NAVI Console is served by the existing gateway started by `cmd/navid`.

The console must not introduce a second server process or a separate frontend-specific listener for V1.

### D2. `:6284` remains the default local console/gateway port

The default local operator URL is:

```txt
http://127.0.0.1:6284/
```

This aligns with the existing runtime config and config defaults.

### D3. Console static assets use the existing `gateway.static_dir` mechanism first

For V1, console assets live under the configured `gateway.static_dir` and are served at `/`.

The default remains:

```yaml
gateway:
  static_dir: "web"
```

A future embedded-assets path may be added later, but it is not required for the first console implementation.

### D4. Existing API surfaces must be reused before adding console-specific routes

NAVI Console should build from current gateway endpoints before adding new ones.

Existing surfaces include:

- `/api/status`
- `/api/agent/status`
- `/api/navi/chats`
- `/api/navi/chats/{id}`
- `/api/navi/chats/{id}/message`
- `/api/proposals`
- `/api/proposals/{id}/resolve`
- `/api/runs`
- `/api/runs/{id}`
- `/api/activity`
- `/api/skills`
- `/api/tools`
- `/api/connectors`
- `/api/llm/*`
- `/api/experience/*`
- `/api/presence/*`
- `/api/errors`
- `/api/errors/summary`
- `/api/debug/events`
- `/api/debug/runtime-metrics`
- `/ws/live`

New routes are allowed only when the existing API cannot express the console requirement without overloading route semantics.

### D5. `/ws/live` remains the first live event transport

The current gateway already exposes `/ws/live`.

NAVI Console V1 should consume `/ws/live` for live runtime updates instead of introducing `/api/v1/events` or a parallel Server-Sent Events stream.

A future SSE endpoint may still be useful, but it is not a V1 dependency.

### D6. Console authority follows gateway auth and governance

The console is not an authority bypass.

It uses the existing gateway auth model:

- loopback requests get local owner/admin-style access
- non-loopback requests require API key/shared-secret auth
- owner-sensitive operations remain protected by existing handler-level checks where applicable
- CORS remains localhost-restricted by default

State-changing console actions must call governed backend endpoints. The UI must not create alternate mutation paths.

### D7. Public exposure is out of scope for V1

V1 is local-first.

Non-loopback/public hosting requires a future hardening pass covering:

- TLS or equivalent secure transport
- origin restrictions
- CSRF protection for browser state-changing actions
- explicit bind-address warnings
- rate limits
- stronger browser auth/token UX
- review of loopback bypass assumptions

---

## Consequences

### Consequence 1: Implementation starts smaller

The first implementation can focus on frontend source, static asset output, and existing endpoint consumption.

No new server package is required for the initial console.

### Consequence 2: API gaps become visible through UI implementation

The console should prove which existing APIs are enough before expanding the gateway.

Likely future route gaps:

- scheduler job CRUD surface
- config read/write/apply surface
- appearance/theme configuration surface
- richer log tailing/export
- docs index/markdown serving

### Consequence 3: The console and onboarding page share a static-root concern

Because `/` already serves static files from `web`, the V1 implementation must decide whether the same app owns onboarding and console routing, or whether the static root serves a shell with routes for both.

### Consequence 4: Gateway API docs stay authoritative

Any new console endpoints must update `docs/specs/gateway-api.md` as part of implementation.

### Consequence 5: Embedded assets are deferred

Embedding console assets into the Go binary may improve distribution later, but it should not block V1.

The current `gateway.static_dir` mechanism is sufficient for the first pass.

---

## Rejected alternatives

### A. Create a separate console server on another port

Rejected because NAVI already has a gateway, runtime config already uses `:6284`, and a second server would duplicate auth, CORS, lifecycle, logging, and operational behavior.

### B. Create a new `/api/v1/*` console API from scratch

Rejected for V1 because existing gateway endpoints already expose most required runtime surfaces.

### C. Use SSE first instead of existing WebSocket live feed

Rejected for V1 because `/ws/live` already exists and is documented. SSE can be added later if the WebSocket path proves too heavy for simple event streaming.

### D. Embed static assets immediately

Rejected because the current static directory mechanism is already present and easier to iterate on during development.

---

## Status history

| Date | Change |
|---|---|
| 2026-05-02 | Accepted — NAVI Console V1 attaches to the existing gateway on `:6284`, serves from `gateway.static_dir`, and reuses existing gateway routes before adding new ones |
