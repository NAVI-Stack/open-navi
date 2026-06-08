# API Authentication

**Status:** Active  
**Last Updated:** 2026-03-27  
**Source of truth:** `internal/gateway/middleware.go`, `internal/gateway/server.go`

This concept note describes the auth model that the gateway currently applies in code.

## Current Auth Paths

Protected routes are reached through these mechanisms:

1. loopback bypass for the local CLI and local operator tooling
2. API keys sent in `X-API-Key`
3. API keys sent as `Authorization: Bearer navi_...`
4. shared gateway secret accepted as `X-API-Key` for connector compatibility

There is no longer a primary JWT-based auth flow in the gateway middleware.

## Loopback Behavior

Requests from loopback are treated as locally trusted and get admin-like scope for local operator use. This is what allows the CLI to work smoothly against a local daemon during development and onboarding.

## Onboarding And Setup Exceptions

- `/api/onboarding/*` is public
- `/api/setup/*` is open until setup is complete

This allows a new instance to be claimed and configured before an API key already exists.

## API Key Management

API key records are stored in SQLite. Raw keys are only shown at creation time; the database stores hashes and metadata.

Key management routes:

- `POST /api/keys`
- `GET /api/keys`
- `DELETE /api/keys/{id}`

These routes are authenticated and also perform owner-secret checks inside the handler.

## Scope Model

Scopes currently used by the gateway include:

- `read`
- `execute`
- `admin`

Local loopback traffic effectively gets `admin`. Other clients depend on the scopes attached to their API key record.

## Using API Keys

Examples:

```http
GET /api/status
X-API-Key: navi_sk_...
```

```http
POST /v1/chat/completions
Authorization: Bearer navi_sk_...
Content-Type: application/json
```

## Related Docs

- [../specs/gateway-api.md](../specs/gateway-api.md)
- [onboarding-flow.md](onboarding-flow.md)
- [../runbooks/run-navi.md](../runbooks/run-navi.md)
