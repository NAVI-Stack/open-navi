# Task Queue

**Authority:** Agent workflow; agents update status (PENDING -> IN_PROGRESS -> DONE).  
**Last Updated:** 2026-03-22  
**Strategy:** Self-extension first, then learning pipeline, then capability breadth.

## Active (In Progress)

| Task ID | Title | Priority | Status | Linear |
|---------|-------|----------|--------|--------|
| NAVI-SE-005 | Connector Plugin System (runtime-extensible) | HIGH | IN_PROGRESS | OMN-26 |

## Urgent Bugs

| Task ID | Title | Priority | Status | Linear |
|---------|-------|----------|--------|--------|
| NAVI-BF-008 | LLM Output Runaway — Infinite Repetition Loop | CRITICAL | PENDING | OMN-71 |
| NAVI-BF-009 | Tool Calling Non-Functional with Ollama (No Agentic Fallback) | CRITICAL | PENDING | OMN-72 |
| NAVI-BF-004 | Duplicate Orphaned "Thinking…" Placeholders (every message) | CRITICAL | PENDING | OMN-66 |
| NAVI-BF-005 | Silent Timeout — No Error Feedback, Stuck Placeholders | CRITICAL | PENDING | OMN-67 |
| NAVI-BF-002 | Telegram Session Creation Timeout (gateway unreachable) | CRITICAL | PENDING | OMN-63 |
| NAVI-BF-003 | LLM Model Switching Fails via Telegram (catalog mismatch) | CRITICAL | PENDING | OMN-62 |
| NAVI-BF-010 | System Prompt Persona Overrides Agent Identity | HIGH | PENDING | OMN-73 |
| NAVI-UX-003 | Governor Approval Missing Details + No Inline Buttons | HIGH | PENDING | OMN-69 |
| NAVI-BF-006 | Raw HTML `<br>` Tags in Telegram Messages | HIGH | PENDING | OMN-68 |
| NAVI-BF-007 | Self-Diagnostic Reads Log Files Instead of error_log DB | MEDIUM | PENDING | OMN-70 |

## Backlog — Features & Infrastructure

| Task ID | Title | Priority | Depends On | Linear |
|---------|-------|----------|------------|--------|
| NAVI-PA-004 | Explicit Session-Close Summarization Trigger | MEDIUM | - | OMN-64 |
| NAVI-MC-001 | Memory v2 (interconnected knowledge + embeddings) | MEDIUM | - | OMN-29 |
| NAVI-CA-002 | Browser Automation Skill (Playwright) | MEDIUM | - | OMN-30 |
| NAVI-CA-003 | Email Skill (IMAP/SMTP + Gmail API) | MEDIUM | - | OMN-31 |
| NAVI-CA-004 | Calendar Skill (Google Calendar + wm_events) | MEDIUM | - | OMN-32 |
| NAVI-MC-002 | Context Governance (retention, compression, scratchpad) | MEDIUM | - | OMN-33 |
| NAVI-MC-003 | Reflection-Driven Memory Consolidation | MEDIUM | NAVI-MC-001 | OMN-34 |
| NAVI-CA-005 | Document / Knowledge Ingestion Skill | MEDIUM | - | OMN-35 |
| NAVI-PA-002 | Webhook Ingestion Endpoint | MEDIUM | - | OMN-36 |
| NAVI-PA-003 | Heartbeat / Proactive Check-Ins | LOW | - | OMN-37 |
| NAVI-LI-001 | LLM Streaming Partial-Event Pipeline | LOW | - | OMN-38 |

## Backlog — PET Web Client Bugs

| Task ID | Title | Priority | Linear |
|---------|-------|----------|--------|
| - | Wizard session never created after claim | Urgent | OMN-5 |
| - | Admin secret copy button no feedback | High | OMN-6 |
| - | Onboarding Wizard AI Elements experience | High | OMN-7 |
| - | Wizard LLM inventing personas/menus | Urgent | OMN-8 |
| - | Factory reset leaves claimed:true | Urgent | OMN-9 |
| - | Scanning animation never visible | Medium | OMN-10 |
| - | Frontend claim screen deadlock | Urgent | OMN-11 |
| - | App hangs on spinner (checkIsOwner) | Urgent | OMN-12 |
| - | WizardChatView double bootstrap | High | OMN-13 |
| - | sendMessage stale state / rapid messages | High | OMN-14 |
| - | Completion detection fires multiple times | High | OMN-15 |
| - | Dev mode checkClaimed proxy 404 | High | OMN-16 |
| - | Auto-title shares AbortController | Medium | OMN-17 |
| - | Loopback bypass security issue | Medium | OMN-18 |
| - | IndexedDB no migration path | Medium | OMN-19 |

## Completed (18 issues)

| Task ID | Title | Linear | Completed |
|---------|-------|--------|-----------|
| NAVI-SE-001 | Gap Detection Pipeline | OMN-20 | 2026-03-20 |
| NAVI-SE-002 | Skill Scaffolding Pipeline (Self-Build) | OMN-21 | 2026-03-20 |
| NAVI-SE-003 | Skill Synthesizer (OpenClaw + Claude) | OMN-23 | 2026-03-20 |
| NAVI-SE-004 | Codebase Self-Modification (Git+Build+Test) | OMN-24 | 2026-03-20 |
| NAVI-CA-001 | Scout Web Search (Brave API) | OMN-25 | 2026-03-20 |
| NAVI-PA-001 | Cron / Scheduled Tasks | OMN-27 | 2026-03-20 |
| NAVI-SE-006 | Skill Registry / Hub | OMN-28 | 2026-03-20 |
| NAVI-BF-001 | LLM Timeout Fix (35s → 120s) | OMN-51 | 2026-03-21 |
| NAVI-OB-001 | Structured Error Logging + Self-Diagnostic | OMN-52 | 2026-03-21 |
| NAVI-UX-001 | Thinking Indicator (Telegram + PET) | OMN-53 | 2026-03-21 |
| NAVI-LN-001 | Fact Extraction from Conversations | OMN-56 | 2026-03-21 |
| NAVI-LN-002 | Memory Scope Fix | OMN-57 | 2026-03-21 |
| NAVI-LN-003 | Task Outcome Facts (Feedback Loop) | OMN-58 | 2026-03-21 |
| NAVI-LN-004 | Orchestrator Reflection Events | OMN-59 | 2026-03-21 |
| NAVI-LN-005 | skill-creator as OSS-27 Tool | OMN-60 | 2026-03-21 |
| NAVI-LN-006 | Session Summarization | OMN-61 | 2026-03-21 |
| NAVI-LLM-001 | Contextual LLM Routing + Complexity Tiering | OMN-54 | 2026-03-22 |
| NAVI-UX-002 | Slack Placeholder Thinking Indicator | OMN-55 | 2026-03-22 |
| NAVI-ARCH-001 | Unified Tool Registry (Phases 1–3) | OMN-83 | 2026-03-24 |

## Selection Rules

Priority order: CRITICAL → HIGH → MEDIUM → LOW.  
Bugs take precedence over features at the same priority level.  
Within the same priority, respect dependency order.

## ID Scheme

| Prefix | Domain |
|--------|--------|
| NAVI-BF | Bug Fixes |
| NAVI-UX | User Experience |
| NAVI-LN | Learning Pipeline |
| NAVI-LLM | LLM Intelligence |
| NAVI-SE | Self-Extension |
| NAVI-RC | Remote Communication |
| NAVI-CA | Core Agent Capabilities |
| NAVI-PA | Proactive Agent Infrastructure |
| NAVI-MC | Memory & Context Engineering |
| NAVI-OB | Observability |
| NAVI-LI | LLM Infrastructure |
