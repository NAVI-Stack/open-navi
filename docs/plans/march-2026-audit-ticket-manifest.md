# NAVI Gap Ticket Manifest — March 2026 Audit

Filed: 2026-03-29

## Actions Taken

### Reopened Tickets (19 total)
All set back to **In Progress** with audit comments explaining gaps.

**LLM-KB (OMN-74–82):** Generic KB infra was built, but spec's first-class entities never implemented.
**Workspace (OMN-99–108):** Entity model and persistence built, but stored policy not enforced across execution paths.

### New Tickets Filed (25 total)

#### LLM-KB Entity Layer (8 tickets)
| Ticket | Title | Priority | Blocked By |
|--------|-------|----------|------------|
| OMN-134 | Structs, Controlled Vocabularies & Schema | High | — |
| OMN-135 | Storage & Migrations | High | OMN-134 |
| OMN-136 | Seed Loader & Initial Seed Records | High | OMN-135 |
| OMN-137 | Read/Query API & World Model Integration | High | OMN-136 |
| OMN-138 | Runtime State from Connector Probes | High | OMN-137 |
| OMN-139 | Execution Telemetry (ExecutionRecord & RouterDecision) | High | OMN-138 |
| OMN-140 | Router Consumes KB for Model Selection | High | OMN-139 |
| OMN-141 | Reflection-Driven Adaptation & Promotion Pipeline | Medium | OMN-140 |

#### Existing LLM-KB Backlog Updated
| Ticket | Now Blocked By |
|--------|---------------|
| OMN-96 (Data Retention) | OMN-139 |
| OMN-97 (Cost Fidelity) | OMN-139 |
| OMN-98 (Closed-Loop Automation) | OMN-141 |

#### Workspace Enforcement (5 tickets)
| Ticket | Title | Priority | Blocked By |
|--------|-------|----------|------------|
| OMN-142 | Audit & Integration Gap Map | High | — |
| OMN-143 | Wire Allowed-Actions Deny-Mask | High | OMN-142 |
| OMN-144 | Wire Boundary Policy & Protected Paths | High | OMN-142 |
| OMN-145 | Whitelist Rule Lifecycle | Medium | OMN-144 |
| OMN-146 | No-Active-Workspace Blocking & Mid-Execution Switch | Medium | OMN-143 |

#### Project System (10 tickets)
| Ticket | Title | Priority | Blocked By |
|--------|-------|----------|------------|
| OMN-147 | Core Entity, Store & Migrations | High | — |
| OMN-148 | Active Project State & Session Integration | High | OMN-147 |
| OMN-149 | Artifact & Workspace Relationship Validation | High | OMN-147 |
| OMN-150 | CRUD Service & API | High | OMN-148, OMN-149 |
| OMN-151 | Work State Entities (Objective, Milestone, Decision) | Medium | OMN-147 |
| OMN-152 | Work State Entities (Risk, OpenQuestion, StatusUpdate) | Medium | OMN-151 |
| OMN-153 | Runtime Profile & Cognitive Integration | High | OMN-150, OMN-151 |
| OMN-154 | Scoped Configuration (Instructions, Capability, Autonomy) | Medium | OMN-150 |
| OMN-155 | Lifecycle, Archive & Collaboration Roles | Medium | OMN-150 |
| OMN-156 | Bootstrapping & Migration | Low | OMN-150 |

#### Artifact Gaps (2 tickets)
| Ticket | Title | Priority | Blocked By |
|--------|-------|----------|------------|
| OMN-157 | Merge/Reconciliation Strategy | Medium | — |
| OMN-158 | API Surface Audit & Completion | Medium | — |

## Critical Path

```
LLM-KB:    134 → 135 → 136 → 137 → 138 → 139 → 140 → 141
Workspace: 142 → 143 + 144 (parallel) → 145, 146
Project:   147 → 148 + 149 (parallel) → 150 → 153
           147 → 151 → 152
           150 → 154, 155, 156
Artifact:  157, 158 (independent)
```

## Parallelism Opportunities

These chains can execute simultaneously:
1. LLM-KB chain (OMN-134→141)
2. Workspace enforcement chain (OMN-142→146)
3. Project system chain (OMN-147→156)
4. Artifact gaps (OMN-157, OMN-158) — independent, can start anytime
