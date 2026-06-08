# Gateway HTTP And WebSocket API

**Status:** Active
**Last Updated:** 2026-05-29
**Source of truth:** `internal/gateway/server.go`, `internal/gateway/middleware.go`

This is the live route inventory for the current gateway. It is a curated subset: every route listed here exists in code, but a few live routes (some `/api/experience/*` mutators, `/api/capabilities/graph`, `/api/skill-ui`, `/api/errors*`, `/v2/api/connectors/instances`, and CORS `OPTIONS` preflight routes) are not yet tabulated. See `docs/tasks/documentation-debt.md` (route-doc parity) and `docs/tasks/testing-eval-debt.md` for the gap and the proposed parity check.

## Auth Model

The current implementation is API-key based and has a few important shortcuts:

- loopback requests get local admin-like access for the CLI
- `/api/onboarding/*` and `/onboarding` bypass API-key auth but are restricted to local-machine/loopback only
- `/api/setup/*` is open until setup is complete
- authenticated requests use `X-API-Key`
- `Authorization: Bearer navi_...` is accepted as an API-key alias
- the gateway shared secret is also accepted as `X-API-Key`
- some handlers additionally require specific scopes such as `read` or `execute`
- some owner-sensitive routes also require `X-Owner-Secret` inside the handler

`POST /webhooks/{name}` is intentionally not protected by `AuthMiddleware`; the connector itself may still validate secrets.

## Public And Browser Bootstrap Routes

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/health` | public | Health check |
| GET | `/api/health` | public | Health alias |
| GET | `/api/version` | public | Version and schema version |
| GET | `/` | public | Serves the Console from `web/`; redirects to `/onboarding` if first-run is incomplete |
| GET | `/onboarding` | local-only | Serves the Onboarding application |
| POST | `/auth/token` | public, rate-limited | Shared-secret token exchange |
| GET | `/api/onboarding/status` | local-only | Claim/bootstrap status |
| POST | `/api/onboarding/recovery` | local-only | Claim instance and create initial passport |
| POST | `/api/onboarding/provider` | local-only | Persist and activate LLM config |
| POST | `/api/onboarding/connection` | local-only | Start/configure optional connector |
| POST | `/api/onboarding/complete` | local-only | Mark first-run complete and redirect to `/` |

The gateway also registers CORS preflight handlers for onboarding, auth-me, API key, instance-reset, and OpenAI-compatible browser routes.

## Setup, Identity, And Chat Routes

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/setup` | API key or loopback; public before setup complete | Legacy setup state |
| POST | `/api/setup/llm` | API key or loopback; public before setup complete | Persist and activate LLM config |
| POST | `/api/setup/connector` | API key or loopback; public before setup complete | Start/configure connector |
| GET | `/api/auth/me` | API key or loopback | Current auth identity |
| GET | `/v1/api/auth/me` | API key or loopback | Alias for auth me |
| GET | `/api/identity` | API key or loopback | Active local agent identity |
| GET | `/api/console/appearance` | API key or loopback, `read` scope | Read durable Console Appearance preferences |
| PATCH | `/api/console/appearance` | API key or loopback, `execute` scope | Persist durable Console Appearance preferences |
| GET | `/api/experience` | Owner read access or loopback | Read stored experience configuration and inferred relationship profile |
| GET | `/api/experience/inspect` | Owner read access or loopback | Inspect effective experience configuration for a user chat/runtime context (records observation snapshot) |
| GET | `/api/experience/module-registry` | Owner read access or loopback | List available experience modules with scope, version, kind, and source metadata |
| PUT | `/api/experience/core-identity` | Owner admin access or loopback | Replace owner core identity trait overrides |
| PUT | `/api/experience/output-preferences` | Owner admin access or loopback | Replace owner output preferences |
| PUT | `/api/experience/modules` | Owner admin access or loopback | Replace owner-selected experience modules, including scope-aware kind/version metadata |
| POST | `/api/navi/chats` | API key or loopback | Create chat |
| GET | `/api/navi/chats` | API key or loopback | List recent user-facing chats |
| GET | `/api/navi/chats/{id}` | API key or loopback | Get one chat |
| GET | `/api/navi/chats/{id}/runtime_summary` | API key or loopback | Get runtime summary for one chat |
| PATCH | `/api/navi/chats/{id}` | API key or loopback | Rename a chat |

**Note on `/api/experience/inspect`**: This endpoint accepts an optional `mode` query parameter (`standard`/`onboarding`/`balanced`, defaults to `balanced`) and returns the *effective* configuration merging stored rules and contextual bounds. Calling this endpoint has the side-effect of recording an `owner_inspection` snapshot in the event log.

**Response Schema (Inspect):**
```json
{
  "owner_id": "...",
  "stored_config": {
    "core_identity": { ... },
    "output_preferences": { ... },
    "relationship_profile": { ... },
    "persona_modules": [ ... ]
  },
  "effective_state": {
    "schema_version": "...",
    "state_id": "...",
    "generated_at": "...",
    "merge_engine_version": "...",
    "role_context": { ... },
    "resolved_traits": { ... },
    "governance_trace": { ... },
    "live_context_trace": { ... },
    "explicit_turn_overrides": { ... },
    "output_preferences": { ... },
    "audit": { ... }
  },
  "compiled_payload": {
    "schema_version": "...",
    "payload_id": "...",
    "compiled_at": "...",
    "source_state_id": "...",
    "compiler_version": "...",
    "budget": { ... },
    "role_adapter": { ... },
    "cognitive_modulation": { ... },
    "expression_policy": { ... },
    "behavior_policy": { ... },
    "output_contract": { ... },
    "gates_and_clamps": { ... },
    "serialization_hints": { ... },
    "audit": { ... }
  },
  "snapshot_history": [
    { "type": "navi.fact.experience_snapshot", ... }
  ]
}
```

| POST | `/api/navi/chats/{id}/message` | API key or loopback | Queue a user message into a chat/runtime session |
| POST | `/api/navi/chats/{id}/archive` | API key or loopback | Archive a chat |
| DELETE | `/api/navi/chats/{id}` | API key or loopback | Delete a chat |

### Chat Message Operations

These routes back the Console chat UX (`web-src/navi-console/src/api/chats.ts`, `web-src/navi-console/src/components/chat/ChatMessage.tsx`). Registered in `internal/gateway/server.go:324-329`; handlers in `chat_edit.go` and `chat_feedback.go`. Each returns `501 NOT_IMPLEMENTED` when the underlying runtime does not support the operation.

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/navi/chats/{id}/messages/{messageId}/feedback` | API key or loopback | Persist owner 👍/👎 feedback on an assistant message |
| POST | `/api/navi/chats/{id}/messages/{messageId}/edit-resend` | API key or loopback | Edit a prior user message and resubmit it through the runtime |
| GET | `/api/navi/chats/{id}/messages/{messageId}/variants` | API key or loopback | List response variants for a message |
| POST | `/api/navi/chats/{id}/messages/{messageId}/variants/select` | API key or loopback | Select the active response variant |
| POST | `/api/navi/chats/{id}/regenerate` | API key or loopback | Regenerate the most recent assistant reply |
| POST | `/api/navi/chats/{id}/continue` | API key or loopback | Continue the most recent assistant reply without truncating it |

### Chat Rename Detail

Renames the human-readable title of a chat. This is user-facing metadata and does not affect the chat ID or persistent state beyond the `title` field.

**Request Body:**
```json
{
  "title": "New Chat Title"
}
```

**Response:**
- `204 No Content`: Successful rename.
- `400 Bad Request`: Title missing, empty, or whitespace-only.
- `404 Not Found`: Chat ID unknown or hidden.


## Directive, Proposal, Run, And Activity Routes

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/directives` | API key or loopback | List directives |
| POST | `/api/directives` | API key or loopback | Create directive |
| GET | `/api/directives/{id}` | API key or loopback | Get directive plus messages |
| POST | `/api/directives/{id}/message` | API key or loopback | Append directive message |
| PUT | `/api/directives/{id}/mode` | API key or loopback | Change directive mode |
| GET | `/api/proposals` | API key or loopback | List pending proposals |
| POST | `/api/proposals/{id}/resolve` | API key or loopback | Approve or decline a proposal |
| GET | `/api/runs` | API key or loopback | List execution runs |
| GET | `/api/runs/{id}` | API key or loopback | Get one run |
| POST | `/api/context/query` | API key or loopback | Governed read surface (`query_context`): purpose- and scope-bound, redacted, provenance-tagged, audited context. Read-only. |
| GET | `/api/activity` | API key or loopback | Operator activity feed |
| GET | `/api/status` | API key or loopback | Aggregated system status |
| GET | `/api/operator/overview` | API key or loopback | Console/operator dashboard aggregate route |
| GET | `/api/agent/status` | API key or loopback | Agent runtime status |
| GET | `/api/presence` | API key or loopback | Presence registry snapshot |
| GET | `/api/presence/navi` | API key or loopback | NAVI presence state |
| POST | `/api/presence/user` | API key or loopback | Update user presence |
| GET | `/api/presence/heartbeat` | API key or loopback | Presence heartbeat |
| GET | `/api/governor` | API key or loopback | Governor state |
| POST | `/api/ai/conversations/title` | API key or loopback | Generate a conversation title |

## Project, Workspace, And Artifact Routes

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/projects` | API key or loopback | List projects |
| POST | `/api/projects` | API key or loopback | Create project |
| GET | `/api/projects/{id}` | API key or loopback | Get one project |
| PUT | `/api/projects/{id}` | API key or loopback | Update project |
| POST | `/api/projects/{id}/archive` | API key or loopback | Archive project |
| GET | `/api/projects/{id}/readiness` | API key or loopback | Project readiness summary |
| GET | `/api/projects/{id}/chats` | API key or loopback | List project chats |
| POST | `/api/projects/{id}/chats` | API key or loopback | Create project chat |
| GET | `/api/projects/{id}/tasks` | API key or loopback | List project tasks |
| POST | `/api/projects/{id}/tasks` | API key or loopback | Create project task |
| GET | `/api/projects/{id}/tasks/{taskID}` | API key or loopback | Get project task |
| POST | `/api/projects/{id}/tasks/{taskID}/cancel` | API key or loopback | Cancel project task |
| PUT | `/api/projects/{id}/workspace-binding` | API key or loopback | Bind project workspace |
| DELETE | `/api/projects/{id}/workspace-binding` | API key or loopback | Unbind project workspace |
| GET | `/api/projects/{id}/workspace` | API key or loopback | Get project workspace |
| GET | `/api/sandbox-profiles` | API key or loopback | List sandbox profiles |
| POST | `/api/sandbox-profiles` | API key or loopback | Create sandbox profile |
| GET | `/api/sandbox-profiles/{id}` | API key or loopback | Get one sandbox profile |
| GET | `/api/workspace-mode` | API key or loopback | Read workspace mode |
| PUT | `/api/workspace-mode` | API key or loopback | Set workspace mode |
| GET | `/api/workspaces/active` | API key or loopback | Read active workspace |
| PUT | `/api/workspaces/active` | API key or loopback | Set active workspace |
| GET | `/api/workspaces/path-roots` | API key or loopback | List server-visible path roots for workspace boundary selection |
| GET | `/api/workspaces/path-children?path=...` | API key or loopback | List child directories under a server-visible path |
| GET | `/api/workspaces` | API key or loopback | List workspaces |
| POST | `/api/workspaces` | API key or loopback | Create workspace |
| POST | `/api/workspaces/boundary/resolve` | API key or loopback | Resolve workspace boundary |
| GET | `/api/workspaces/{id}` | API key or loopback | Get one workspace |
| PUT | `/api/workspaces/{id}` | API key or loopback | Update workspace |
| DELETE | `/api/workspaces/{id}` | API key or loopback | Delete an unreferenced workspace |
| POST | `/api/workspaces/{id}/archive` | API key or loopback | Archive workspace |
| PUT | `/api/workspaces/{id}/project-binding` | API key or loopback | Bind workspace project |
| GET | `/api/workspaces/{id}/whitelist-rules` | API key or loopback | List workspace whitelist rules |
| POST | `/api/workspaces/{id}/whitelist-rules` | API key or loopback | Create workspace whitelist rule |
| POST | `/api/workspaces/{id}/whitelist-rules/{ruleID}/revoke` | API key or loopback | Revoke workspace whitelist rule |
| GET | `/api/artifacts` | API key or loopback | List artifacts |
| GET | `/api/artifacts/{id}` | API key or loopback | Get one artifact |
| GET | `/api/artifacts/{id}/content` | API key or loopback | Get artifact content |
| POST | `/api/artifacts/{id}/content` | API key or loopback | Save artifact content |
| GET | `/api/artifacts/{id}/versions/{versionID}/content` | API key or loopback | Get artifact version content |
| GET | `/api/artifacts/{id}/versions/{versionID}/diff` | API key or loopback | Get artifact version diff |
| POST | `/api/artifacts/{id}/versions/{versionID}/restore` | API key or loopback | Restore artifact version |
| POST | `/api/artifacts/{id}/branches` | API key or loopback | Create artifact branch |
| GET | `/api/artifacts/{id}/exports` | API key or loopback | List artifact exports |
| POST | `/api/artifacts/{id}/exports` | API key or loopback | Create artifact export |
| GET | `/api/artifacts/{id}/shares` | API key or loopback | List artifact shares |
| POST | `/api/artifacts/{id}/shares` | API key or loopback | Create artifact share |
| GET | `/api/artifact-exports/{exportID}/content` | API key or loopback | Download artifact export |

## LLM, Knowledge, Skill, And Tool Routes

Some of these routes enforce `read` or `execute` scopes in addition to base authentication.

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/llm/catalog` | API key or loopback, `read` scope | Current provider catalog |
| GET | `/api/llm/profiles` | API key or loopback, `read` scope | Model profiles |
| GET | `/api/llm/active` | API key or loopback, `read` scope | Active provider and model |
| PUT | `/api/llm/active` | API key or loopback, `execute` scope | Change active provider and model |
| GET | `/api/llm/preferences` | API key or loopback, `read` scope | Persisted routing preferences |
| PATCH | `/api/llm/preferences` | API key or loopback, `execute` scope | Update routing preferences |
| GET | `/api/llm/proposals` | API key or loopback, `read` scope | Routing proposals from LLM-KB |
| POST | `/api/llm/proposals/{id}/resolve` | API key or loopback, `execute` scope | Resolve routing proposal |
| GET | `/api/llm/providers` | API key or loopback | List provider descriptors |
| GET | `/api/llm/providers/{id}` | API key or loopback | Get one provider descriptor |
| GET | `/api/llm/providers/{id}/health` | API key or loopback | Provider health |
| GET | `/api/llm/providers/{id}/models` | API key or loopback | Provider model catalog |
| GET | `/api/llm/providers/{id}/running` | API key or loopback | Provider running models |
| POST | `/api/llm/providers/{id}/actions/pull` | API key or loopback | Pull/download provider model |
| POST | `/api/llm/providers/{id}/actions/delete` | API key or loopback | Delete provider model |
| POST | `/api/llm/providers/{id}/actions/warm` | API key or loopback | Warm provider model |
| GET | `/api/llm/operations/{id}` | API key or loopback | Inspect provider operation |
| GET | `/api/plugins` | API key or loopback | List plugin manifests |
| POST | `/api/plugins/{id}/enable` | API key or loopback | Enable a plugin (mutates in-memory enabled state; `501` if the `SetPluginEnabled` hook is unset) |
| POST | `/api/plugins/{id}/disable` | API key or loopback | Disable a plugin (`501` if unsupported) |
| POST | `/api/plugins/{id}/validate` | API key or loopback | Re-run manifest validation for one plugin and return the result |
| POST | `/api/plugins/{id}/reload` | API key or loopback | Re-run the plugin loader into the registry (`501` if the `ReloadPlugins` hook is unset) |
| GET | `/api/capabilities/graph` | API key or loopback | Read-only capability graph for plugins, skills, tools, connectors, UI surfaces, docs, and edges |
| GET | `/api/skill-ui` | API key or loopback | List skill UI surface declarations |
| GET | `/api/skills` | API key or loopback | List skills |
| GET | `/api/skills/{id}` | API key or loopback | Get one skill |
| GET | `/api/skills/{id}/ui` | API key or loopback | Get UI surfaces for one skill |
| POST | `/api/skills/reload` | API key or loopback | Reload skills from disk |
| POST | `/api/skills/{id}/validate` | API key or loopback | Validate a skill |
| POST | `/api/skills/{id}/interfaces/{interface}/invoke` | API key or loopback | Invoke one skill interface with `{ "arguments": {} }` and return `SkillExecutionResult` |
| POST | `/api/skills/build` | API key or loopback | Build a skill from a gap request |
| GET | `/api/tools` | API key or loopback | List registered tools |
| GET | `/api/tools/{name}` | API key or loopback | Inspect one tool |
| GET | `/api/knowledge` | API key or loopback | Knowledge surface |
| GET | `/api/gaps` | API key or loopback | List open capability gaps |
| POST | `/api/gaps/{id}/scaffold` | API key or loopback | Trigger gap-to-skill scaffolding |
| POST | `/api/prompts/reload` | API key or loopback | Reload prompt templates |

## Connector And Integration Routes

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/connectors` | API key or loopback | List connectors |
| GET | `/api/connectors/factories` | API key or loopback | List connector factories |
| GET | `/api/connectors/setup-schema` | API key or loopback | Connector setup descriptors |
| GET | `/api/connectors/{name}` | API key or loopback | Get one connector |
| POST | `/api/connectors` | API key or loopback | Register a connector |
| DELETE | `/api/connectors/{name}` | API key or loopback, `execute` scope | Deregister a connector |
| GET | `/v2/api/connectors/instances` | API key or loopback | Driver/instance-style connector view |
| GET | `/api/health/connectors` | API key or loopback | Connector health summary |
| GET | `/api/diagnostics/connectors` | API key or loopback | Connector diagnostics |
| GET | `/api/webhooks` | API key or loopback | List generic webhook registrations |
| PUT | `/api/webhooks/{source}` | API key or loopback | Create or update a generic webhook registration |
| DELETE | `/api/webhooks/{source}` | API key or loopback | Delete a generic webhook registration |
| POST | `/api/webhooks/{source}` | public, signature-validated | Generic webhook ingestion into a configured NAVI chat/runtime intake |
| POST | `/webhooks/{name}` | connector-defined | Connector webhook ingress |

## API Key, Reset, And Error Routes

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/keys` | API key or loopback, plus owner secret in handler | Create API key |
| GET | `/api/keys` | API key or loopback, plus owner secret in handler | List API keys |
| DELETE | `/api/keys/{id}` | API key or loopback, plus owner secret in handler | Revoke API key |
| POST | `/api/instance/reset` | API key or loopback, plus owner secret in handler | Reset instance data |
| GET | `/api/errors` | API key or loopback | Structured error log |
| GET | `/api/errors/summary` | API key or loopback | Error summary window |
| GET | `/api/debug/events` | API key or loopback | Raw event stream snapshot |
| GET | `/api/debug/runtime-metrics` | API key or loopback | Runtime metrics |
| GET | `/api/debug/llmkb/profiles` | API key or loopback | LLM-KB profile snapshot |
| GET | `/api/debug/llmkb/profiles/{id}` | API key or loopback | One LLM-KB profile |
| GET | `/ws/live` | API key or loopback | WebSocket live feed |

## OpenAI-Compatible Surface

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/v1/chat/completions` | API key or loopback | OpenAI-compatible chat completions |
| GET | `/v1/models` | API key or loopback | OpenAI-compatible model list |

The OpenAI-compatible surface exposes NAVI runtime chat behavior through an OpenAI-style interface. Streaming is simulated for compatibility; it is not equivalent to full provider-native token streaming. Public experience configuration now lives on the owner-gated `/api/experience` routes; the OpenAI-compatible surface does not expose mode or profile switching.

## Notes

- The gateway default fallback address is `:8080`, but `config/runtime.yaml` sets `:6284`.
- Chat APIs intentionally hide internal/system runtime chats from the public surface.
- Console Appearance (`#/appearance`) persists owner UI preferences through `/api/console/appearance`. The payload is intentionally narrow: selected preset ID, saved named presets, mode, accent, density, font scale, and light/dark semantic tokens.
- Generic webhook ingress stores source registrations in settings and supports `none`, `header-value`, and `hmac-sha256` signature validation. GitHub-style defaults are applied for `github` registrations.
- Connector webhook auth is connector-specific even though the route itself is not wrapped by the main auth middleware.
- `POST /api/context/query` is the governed read surface (`query_context`) defined by the [Language-Layer Contract](../architecture/language-layer-contract.md) §4. Request body: `{ "run_id": string, "purpose": string, "scope": string }`. `purpose` and `scope` are **required** and **enumerated** — currently `purpose="eval_scoring"` with `scope="current_run_summary"`; missing or unknown values are rejected `400`, and an unknown run is `404`. The response is `{ run_id, purpose, scope, context, provenance, generated_at }` where `context` is a redacted, least-context summary (sensitive free-text and entity references are stripped and listed in `provenance.redactions`) and `provenance` carries `source`/`kernel_mediated` markers. The endpoint is **read-only**: it reuses existing world-model/execution-ledger read paths behind a kernel mediation helper (`internal/contextread`), records a `context.read` audit event per call, and exposes no write, schedule, or generic-query path.

[specs INDEX](INDEX.md) | [docs INDEX](../INDEX.md)
