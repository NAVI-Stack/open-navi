# NAVI Console Smoke Checklist

**Status:** Active  
**Last Updated:** 2026-05-06  
**Scope:** Lightweight manual smoke pass for the gateway-hosted NAVI Console and onboarding shell.

Run this after Console route, gateway static-serving, onboarding, appearance, or capability-surface changes.

## Preconditions

- Start NaviD through either supported runtime mode: Docker Compose or `./bin/navi.exe daemon start`.
- Use loopback access, for example `http://127.0.0.1:6284`.
- Keep Scheduler CRUD out of scope until backend CRUD routes exist.
- Keep live Markdown docs rendering out of scope.

## Static Build And Deploy

- [ ] `make build-console` succeeds.
- [ ] `make build-all` succeeds.
- [ ] Docker frontend build stage runs `npm ci` and `npm run build`.
- [ ] Generated `web/index.html` remains ignored.
- [ ] Generated `web/assets/` remains ignored.
- [ ] Committed `web/onboarding.html` survives Docker image build and is served by `/onboarding`.
- [ ] Docker image serves the Console at `/` after setup is complete.
- [ ] Docker image redirects first-run `/` requests to `/onboarding`.

## First Run And Onboarding

- [ ] Fresh first-run `GET /` redirects to `/onboarding`.
- [ ] `GET /onboarding` serves the committed onboarding shell from loopback.
- [ ] Remote/non-loopback onboarding requests are rejected.
- [ ] Completed setup `GET /` serves the Console.

## Console Pages

- [ ] Sidebar includes exactly: Overview, Chat, Chats, Capabilities, Proposals, Usage, Scheduler, Docs, Config, Appearance, Logs, Debug.
- [ ] No sidebar route lands on a missing page.
- [ ] Overview loads and visibly reports endpoint failures.
- [ ] Chat can create a chat and send a message.
- [ ] Chats detail loads for a selected chat.
- [ ] Logs shows live connection state and visible reconnect/failure state.
- [ ] Proposals approve and decline paths refresh visibly.
- [ ] Capabilities lists the capability graph, or shows a visible degraded state if the graph is unavailable.
- [ ] Usage loads and keeps token/cost data honest when unavailable.
- [ ] Scheduler shows management unavailable, handles absent capability graph, and does not claim "No scheduler tools found" unless the graph loaded authoritatively.
- [ ] Docs loads static route references and does not attempt live Markdown rendering.
- [ ] Appearance loads, saves, and reapplies theme preferences across major pages.

## API Honesty

- [ ] Endpoint failures render visibly on each page that depends on that endpoint.
- [ ] Empty states are shown only for successful empty responses, not failed requests.
- [ ] Capability graph absence does not break Scheduler.
- [ ] Scheduler plugin/skill fallback treats manifest or metadata without an `enabled` field as available, not disabled.
- [ ] Cost/token unavailable states remain labeled as unavailable or unknown.

[runbooks INDEX](INDEX.md) | [docs INDEX](../INDEX.md)
