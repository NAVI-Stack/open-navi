# NAVI Console — Frontend Implementation

**Status:** Implemented but Undertested (e2e)
**Last Updated:** 2026-05-29
**Source of truth:** `web-src/navi-console/`

This document describes the **actual React frontend** that ships as the NAVI browser Console — what is implemented in code today, with exact file paths. It is implementation truth, not design intent. For the control-plane *design* direction and product framing, see [../design/navi-console.md](../design/navi-console.md), [../design/navi-console-control-plane.md](../design/navi-console-control-plane.md), and the specs [../specs/navi-console-v1.md](../specs/navi-console-v1.md) / [../specs/navi-console-v2.md](../specs/navi-console-v2.md).

> This supersedes any older claim that the browser UI is "a placeholder" or that "the full frontend does not exist." It does exist and is feature-complete for chat. The real gap is automated test coverage.

---

## Stack

| Aspect | Value | Evidence |
|--------|-------|----------|
| Framework | React 19 | `web-src/navi-console/package.json` (`react`, `react-dom` `^19.1.0`) |
| Build tool | Vite 6 | `web-src/navi-console/package.json` (`vite ^6.3.5`) |
| Language | TypeScript | `build` script runs `tsc --noEmit` before `vite build` |
| Test runner | Vitest 2 + jsdom + Testing Library | `web-src/navi-console/vite.config.ts`, `web-src/navi-console/src/test/setup.ts` |
| Build output | `web/` | `web-src/navi-console/vite.config.ts` → `build.outDir: '../../web'` |

The gateway serves the built output from `web/` at `GET /` and redirects to `/onboarding` on first run (`internal/gateway/server.go`; see [../specs/gateway-api.md](../specs/gateway-api.md)).

---

## Chat UX surface — Implemented

All features below are present in code. Primary files:
`web-src/navi-console/src/components/chat/ChatMessage.tsx`,
`web-src/navi-console/src/components/chat/MarkdownRenderer.tsx`,
`web-src/navi-console/src/pages/ChatPage.tsx`,
`web-src/navi-console/src/api/chats.ts`.

| Feature | Status | Evidence |
|---------|--------|----------|
| Markdown rendering | Implemented | `MarkdownRenderer.tsx` — `react-markdown` + `remark-gfm` + `rehype-highlight` |
| Code-block copy button | Implemented | `MarkdownRenderer.tsx` `CodeBlock` (copy → "Copied" feedback) |
| Edit & resend a user message | Implemented | `ChatMessage.tsx` `startEdit`/`submitEdit`; API `editAndResendMessage` (`chats.ts`) → `POST .../messages/{messageId}/edit-resend` |
| Regenerate last reply | Implemented | `ChatMessage.tsx` regenerate control; API `regenerateLastReply` → `POST .../regenerate` |
| Continue last reply | Implemented | `ChatMessage.tsx` continue control; API `continueLastReply` → `POST .../continue` |
| Response variants (switcher) | Implemented | `ChatMessage.tsx` prev/next variant UI with "X/Y" counter; API `selectMessageVariant` → `POST .../messages/{messageId}/variants/select` |
| Feedback (👍/👎) | Implemented | `ChatMessage.tsx` thumbs controls; API `sendMessageFeedback` → `POST .../messages/{messageId}/feedback` |
| Tool-call chips / indicators | Implemented | `ChatMessage.tsx` `ToolChips` (`call`/`partial-call`/`result` states, running spinner, error badge); live events accumulated in `ChatPage.tsx` |
| Streaming / pending / failure states | Implemented | `ChatMessage.tsx` `data-pending` / `data-streaming` / `data-failed` attributes; "Sending…", "Writing…", "Not sent" labels |

The corresponding backend routes are documented in [../specs/gateway-api.md](../specs/gateway-api.md) under *Chat Message Operations* (`internal/gateway/server.go:324-329`, `chat_edit.go`, `chat_feedback.go`).

---

## Test coverage

**Present** (`web-src/navi-console`, `*.test.tsx`):

- `src/components/chat/ChatMessage.test.tsx` — copy, feedback, edit/resend, regenerate, continue, variants, tool chips, failed state
- `src/components/chat/MarkdownRenderer.test.tsx` — markdown + fenced code copy
- `src/app/router.test.tsx`, `src/components/shell/PageRouter.test.tsx` — routing
- `src/pages/PluginsPage.test.tsx`, `src/pages/plugin-detail/*` — plugin pages
- `src/components/model-browser/ModelBrowser.test.tsx`, `src/components/appearance/ContextualChatMessageRenderer.test.tsx`

**Gaps (Untested):**

- `src/pages/ChatPage.tsx` — the orchestration layer (send/receive, live-event accumulation, variant switching, feedback flow) has **no** test.
- No end-to-end harness: there is no Playwright/Cypress config and no browser-level e2e suite.

These gaps are tracked in [../tasks/testing-eval-debt.md](../tasks/testing-eval-debt.md).

---

## How to run

```bash
cd web-src/navi-console
npm install
npm test          # vitest run
npm run build     # tsc --noEmit && vite build → outputs to ../../web
```
