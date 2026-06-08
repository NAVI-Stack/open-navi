# Internal Runtime Visibility

This follow-up closes the remaining internal-runtime leakage paths in the NAVI repo.

Historical naming note: this document previously used "session"; canonical terminology is now Chat for transcript/context and RuntimeSession for execution lifecycle.

## What changed

- runtime sessions now persist a typed `kind` on `runtime_sessions`.
- `heartbeat-auto` is backfilled and auto-created as `kind=internal`.
- `ListChats()` hides chats attached only to internal runtimes at the store layer instead of relying on every caller to remember to filter them.
- Gateway chat routes (`GET /api/navi/chats`, `GET /api/navi/chats/{id}`, message, close, archive) treat internal chats as not found.
- `GET /ws/live` rejects internal chats on every stream, not just `stream=user`.

## Why

The old model treated internal work as a single hardcoded chat ID. That left visibility checks split across the store, gateway, and runtime layers. Persisting runtime `kind` makes internal classification explicit and lets the public APIs hide system work consistently.
