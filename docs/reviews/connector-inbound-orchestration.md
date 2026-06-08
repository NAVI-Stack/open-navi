# Connector Inbound Orchestration Note

## Summary

Telegram and Slack both manage their own session-streaming UX directly from `/ws/live`.
They create a per-session placeholder themselves and update that same message in place
as partial and final assistant events arrive.

## Gateway Rule

For connector-originated `POST /api/navi/sessions/{id}/message` calls, the gateway must
not also invoke the connector manager's generic `OnInbound(...)` orchestration for
connectors that already manage that lifecycle themselves.

Without that guard, a self-managed connector can end up with:

- one placeholder from its own bot-side streaming flow
- one placeholder from the connector manager's generic inbound orchestration

That produces duplicate "Thinking..." indicators and leaves orphaned placeholders when
only one of the two paths is later edited.

## Current Implementation

The runtime now exposes an optional connector capability:

- `InboundOrchestrationManaged`

Connectors that return `true` from `ManagesInboundOrchestration()` tell the gateway to
skip generic manager-driven inbound typing / placeholder orchestration for those
session API calls.

Current self-managed connectors:

- Telegram
- Slack
