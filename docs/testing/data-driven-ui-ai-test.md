# AI-on-AI Test — Data-Driven UI Rendering (Claude in Chrome)

A copy-paste test script for driving a **browser AI agent** (e.g. Claude in
Chrome / a computer-use agent) to exercise NAVI's data-driven UI rendering
end-to-end against a running console. This complements the automated layers
(Go unit + e2e, frontend vitest + accessibility). It is **manual / on-demand** —
it is not part of CI.

The agent should perform each step and report **PASS/FAIL** with a one-line note.

---

## 0. Operator setup (human, once)

```bash
# From the repo root — build & start the supported NaviD runtime.
docker compose -f compose.yml up -d --build

# Wait for health.
curl -fsS http://localhost:6284/health
```

Open `http://localhost:6284` in the browser the AI agent controls. On first run
you will be redirected to `/onboarding`; complete the claim form (owner name,
handle, secret) to reach the console, then open the **Chat** page. Make sure a
chat model is configured (Ollama/Anthropic/etc.) so ordinary replies work — the
render path itself is deterministic and does not require the model, but the
negative/awareness checks do.

---

## 1. Prompt to paste into the browser AI agent

> You are testing the NAVI chat console at **http://localhost:6284** (already
> onboarded; open the **Chat** view). You are verifying NAVI's data-driven UI
> rendering feature. Do the following steps **in order, in a single fresh chat**,
> and after each step report `PASS` or `FAIL` with a one-line reason. Do not
> assume — read what actually renders on screen.
>
> **Step 1 — Empty state (honest degraded view).**
> Send: `show me a graph of tool usage in this chat`
> EXPECT: NAVI responds with a small data view (a card/table) indicating there is
> **no tool usage yet** (e.g. "No tool usage … recorded"), NOT a refusal and NOT a
> hallucinated chart. PASS if it shows an honest empty/▁zero state; FAIL if it says
> it "can't generate graphs" or invents data.
>
> **Step 2 — Induce real tool usage.**
> Send: `read the file README.md and tell me the first heading` (or any request
> that makes NAVI use a file/list tool). Wait for the reply. This records real
> tool calls in this chat. PASS if NAVI performs a tool action (you may see a tool
> chip / "Running…" indicator); FAIL if no tool runs.
>
> **Step 3 — Render the chart (primary slice).**
> Send: `show me a graph of tool usage in this chat`
> EXPECT: a visual data view renders inline in the chat — a metric card ("Total
> tool calls"), a horizontal **bar chart** ("Calls per tool"), and/or a table of
> tools with counts. PASS if a chart/metric/table renders (not plain prose, not
> just a markdown list); FAIL otherwise.
>
> **Step 4 — Capability awareness (model-driven, reworded).**
> Send: `can you visualize the tools you've used here as a chart?`
> EXPECT: NAVI affirms it can and renders a visualization (it may call its render
> tool). PASS if it renders or clearly confirms it can render tool usage; FAIL if
> it denies having any graphing/visualization capability.
>
> **Step 5 — Negative (no hijack).**
> Send: `which tools did you use in this chat?`
> EXPECT: a normal prose/markdown answer (it may list tools in text). PASS if it
> answers in text WITHOUT forcing a chart/graph widget; FAIL if it renders a chart
> widget for this plain question.
>
> **Step 6 — Unsupported data (graceful).**
> Send: `show me a graph of token usage lately`
> EXPECT: NAVI does NOT flatly refuse; it explains it can chart **tool usage** in
> this chat today (token/model usage not yet supported) — ideally offering the
> tool-usage view. PASS if it names what it can chart instead of a bare "I can't";
> FAIL if it denies all graphing ability.
>
> **Step 7 — Resilience.**
> Throughout, confirm the chat surface never crashes or shows a blank/broken
> panel. If any visualization fails to render, a readable markdown fallback (totals
> + a tool table) must appear instead. PASS if the surface stays usable and a
> fallback shows on any render failure; FAIL if the chat crashes or shows an empty
> broken box.
>
> Finally, output a summary table: Step | PASS/FAIL | note.

---

## 2. What "good" looks like (oracle for the operator)

| Step | Expected |
| --- | --- |
| 1 | Honest empty/zero tool-usage view; no refusal, no fabricated data |
| 2 | A tool actually runs (file/list); tool chip visible |
| 3 | Metric card + bar chart and/or table renders inline |
| 4 | NAVI affirms + renders (model may call `navi.render.visualize`) |
| 5 | Plain text answer; **no** chart widget forced |
| 6 | Names chartable data (tool usage) instead of a blanket refusal |
| 7 | No crash; markdown fallback on any render failure |

## 3. Notes & limitations
- Steps 1 and 3 exercise the **deterministic short-circuit** (no model needed).
- Step 4 exercises the **model-callable render tool** + system-prompt awareness.
- Step 6 reflects the current scope: only `tool_usage` is implemented; a
  token/model-usage capability is a delegated follow-up
  (`docs/tasks/token-usage-capability.md`).
- For a fully scripted variant, the same assertions are covered headlessly by the
  Go e2e scenario `test/e2e/scenarios/render_test.go` (`make test-e2e`).

---

## 4. Prototype rendering scenario (Block 2)

Generated UI **prototypes** are a second render lane: NAVI materializes a visual
mockup from natural language, using clearly-labeled **placeholder** data. Run these
in a fresh chat; report `PASS`/`FAIL` per step.

> **Step P1 — Dashboard mockup (primary proving slice).**
> Send: `Create a dashboard mockup for connector health.`
> EXPECT: an inline visual panel titled **"Connector Health Dashboard"** with a
> visible **prototype / placeholder** label (a "PROTOTYPE" badge and/or a
> "Prototype · placeholder data" chip), metric cards, and a status table of
> placeholder connectors. PASS if a labeled prototype renders; FAIL if it refuses,
> renders unlabeled data that looks real, or claims live connector data.
>
> **Step P2 — Explicit weather mockup (secondary).**
> Send: `Create a weather display mockup.`
> EXPECT: a clearly-labeled placeholder weather **display concept** (not real
> weather). PASS if it renders a labeled prototype; FAIL if it implies the values
> are real weather.
>
> **Step P3 — Negative: real-data weather is NOT a prototype.**
> Send: `Show me the weather for this week.`
> EXPECT: a normal response (NAVI may explain it has no weather data wired). PASS if
> it does NOT render a placeholder weather widget as if it were real; FAIL if it
> answers a real-data question with a placeholder prototype.
>
> **Step P4 — Read-only affordances.**
> If the prototype shows any buttons/controls, confirm they are inert prototype
> affordances (clicking does nothing / triggers no backend call). PASS if controls
> are non-bound; FAIL if a generated control performs a privileged action.
>
> **Step P5 — Resilience.** As with the data lane, the chat surface must never crash;
> a readable markdown fallback (with the prototype/placeholder banner) must appear on
> any render failure.
>
> Finally, output a summary table: Step | PASS/FAIL | note.

| Step | Expected |
| --- | --- |
| P1 | Labeled "Connector Health Dashboard" prototype with placeholder data |
| P2 | Labeled placeholder weather display concept |
| P3 | Real-data weather request is NOT answered with a placeholder prototype |
| P4 | Generated controls are inert (no privileged action) |
| P5 | No crash; honest prototype markdown fallback on any failure |

Notes:
- Steps P1–P3 exercise the **deterministic prototype short-circuit** (no model needed).
- This block is **in-chat only**: prototypes are not saved as artifacts and use no
  live/real data source.
- The same assertions are covered headlessly by `TestE2E_PrototypeRender` in
  `test/e2e/scenarios/render_test.go`.
