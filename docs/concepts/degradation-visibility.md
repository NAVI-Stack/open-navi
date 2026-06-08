# Degradation visibility (failure → UX)

Failure visibility levels define how the system surfaces failures to the user. The Experience Layer uses these levels to present messages consistently (e.g. tone, blocking vs advisory, recovery links).

## Four levels

| Level | Value | Behavior |
|-------|--------|----------|
| **Silent retry** | `silent_retry` | Retry without surfacing to the user. Used for transient or non–user-facing failures (e.g. connector temporarily unavailable, non-fatal timeout). |
| **Advisory** | `advisory` | Surface as an insight; do not block. User sees a “Note: …” style message. Used when the user should be informed but can continue. |
| **Blocking** | `blocking` | Block the action; require user acknowledgment. User sees an “Error: …” style message. Used for validation rejection, permission denial, irreversible or high-impact failures. |
| **Deferred recovery** | `deferred_recovery` | Create a recovery proposal for later; surface that recovery is needed and optionally include the proposal ID. User can resolve via the proposals API. |

Selection is driven by failure class, user-facing impact, and reversibility (see `schema.DegradationVisibilityFor` in `internal/schema/command.go`).

## API contract

### Error responses (4xx / 5xx)

Relevant error responses may include a structured body with:

- **`error`** (string): Human-readable message.
- **`degradation_type`** (string, optional): One of the four levels above, or the governance-specific value below.
- **`recovery_proposal_id`** (string, optional): Present when `degradation_type` is `deferred_recovery` (or when a recovery proposal was created); clients can use the proposals API to resolve.

Governance outcomes use the same body shape. When the action is rejected or requires confirmation, the API may return:

- **`degradation_type: "blocking"`** — Governance rejected the action.
- **`degradation_type: "requires_confirmation"`** — Action is allowed only after owner approval (proposal or confirmation flow).

Frontends should treat `blocking` as “show error, do not proceed”; `requires_confirmation` as “prompt for approval then retry”; `advisory` as “show notice, allow continuation”; `deferred_recovery` as “show notice and optional link to resolve proposal”.

### Session and message responses

Session and message payloads (e.g. `GET /api/navi/sessions/{id}`) return conversation content. Degradation that occurred during a turn is currently reflected in the **message content** (e.g. “Note: …”, “Error: …”, “Recovery needed: … (Recovery proposal ID: …)”). A future revision may add optional `degradation_type` (and `recovery_proposal_id`) to message or session objects for structured UI handling.

## Relevant code

- **Schema:** `internal/schema/command.go` — `DegradationVisibility`, `DegradationVisibilityFor`, `FailureClass`.
- **Loop:** `internal/navi/loop.go` — Tool execution uses `DegradationVisibilityFor`; formats message text and creates recovery proposals for `deferred_recovery`.
- **Gateway:** `internal/gateway/server.go` — `ErrorPayload`, `replyErrorStructured`; governance handlers set `degradation_type` and `recovery_proposal_id` on 403 responses.
