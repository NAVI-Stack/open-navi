# Delegated Task — Token/Model Usage Data Capability (data-driven render)

**Status:** Not started (delegated follow-up)
**Context:** The data-driven UI render slice currently ships ONE read-only data
capability — `tool_usage` (see `internal/navi/render/`). A user asked for a graph
of *"token usage"*, which is not yet implemented. This task adds a second
read-only capability so token/model usage renders the same way.

> Keep the canonical axiom: **NAVI owns meaning; OpenUI renders meaning.** The new
> capability produces a renderer-neutral `NaviDataView` + OpenUI Lang + fallback
> markdown, exactly like `tool_usage`. It must be **read-only** (empty
> `Governance.AllowedActions`) and must **never fabricate** data — an absent source
> yields an honest empty/degraded view.

## Goal
When a user asks to see/graph/chart/visualize/tabulate **token usage** or **model
usage** for the current chat (e.g. "show me a graph of token usage lately",
"chart tokens used in this conversation"), NAVI renders a chart/table/card of
token/model usage, with markdown fallback.

## Where the data comes from (investigate first)
Token/cost usage is tracked via cost attribution and usage events. Start at:
- `internal/schema` — `CostAttribution`, `CostTier`, and any token-usage fields on
  task/run/event payloads.
- `internal/store` — event log (`eventlog.go`) and any usage/cost tables; look for
  per-run/per-model token counts (prompt/completion tokens, USD cost).
- `internal/runtime` / `internal/navi` — where model calls record usage (the
  `RunState.LLMProvider`/`LLMModel` fields and any usage events emitted per turn).
- The orchestration/inference layer records the chosen provider/model per run; map
  usage to the current chat via run → chat linkage (same approach as tool usage:
  prefer a chat-scoped persisted source; document the gap if only run-scoped).

If complete per-chat token data is not available, implement the best current
source and document the limitation in code + `docs/architecture/data-driven-ui-rendering.md`
(mirror the `ToolEvent` limitation note in `internal/navi/render/types.go`).

## Implementation (mirror the tool_usage pattern)
1. **Classifier** (`internal/navi/render/classify.go`): add a `model_usage`
   capability. Extend `Classify` so a tool/visual request about **tokens / model
   usage** routes to `RenderIntentOpenUIDataRender` with `Capability:
   CapabilityModelUsage`. Add the synonyms ("token", "tokens", "token usage",
   "model usage") as a usage subject distinct from `tool`. Add positive AND
   negative tests (a plain "how many tokens did that cost?" question without a
   visual cue stays prose).
2. **Aggregator** (`internal/navi/render/model_usage.go`): add
   `BuildModelUsageDataView(events []ModelUsageEvent, preferredView string, trace
   Trace) *NaviDataView` and a `ModelUsageEvent` type (model name, prompt/completion
   tokens, cost, timestamp). Columns typed (`string`/`number`/`duration`/`datetime`).
   Reuse `FallbackMarkdown`-style summary. Empty input → honest degraded view.
3. **Payload** (`internal/navi/render/payload.go`): add
   `BuildModelUsagePayload(...)` returning a `RenderPayload{Mode: openui, ...}`.
4. **Runtime wiring** (`internal/navi/render_intake.go` +
   `internal/navi/runtime_executor.go`): in `executeDataDrivenRender`, branch on
   `classification.Capability` (`tool_usage` vs `model_usage`) and build the
   matching payload from the right source. Add an extractor (e.g.
   `extractModelUsageFromThread`/`...FromEvents`).
5. **Model-callable tool**: extend `navi.render.visualize` with a `capability`
   argument (`tool_usage` | `model_usage`, default `tool_usage`) OR add a sibling
   tool `navi.render.visualize_model_usage`. Keep read-only governance. Update the
   render tool executor (`newRenderToolExecutor`) accordingly.
6. **System prompt** (`internal/prompts/defaults/chat/system.md` and
   `ncos/chat_behavior.md`): broaden the data-driven-visualization note to mention
   token/model usage now that it is supported.
7. **Frontend**: the existing OpenUI vocabulary + components
   (`NaviMetricCard`/`NaviUsageChart`/`NaviDataTable`) already render any
   `NaviDataView`. No new components needed unless a token-specific view is wanted.

## Tests
- Go: classifier (token/model positive + negative), aggregation, empty/degraded,
  fallback markdown, read-only governance, render-tool executor for the new
  capability. Mirror `internal/navi/render/render_test.go` and
  `internal/navi/render_tool_test.go`.
- Go e2e: extend `test/e2e/scenarios/render_test.go` with a token-usage request.
- Frontend: a data view with token columns renders (reuse `NaviToolUsagePanel`).

## Acceptance
- "show me a graph of token usage lately" renders a chart/table/card (or honest
  empty state), with markdown fallback; OpenUI stays non-canonical; read-only.
- `make test` + `make test-frontend` green; no regression to `tool_usage`.
