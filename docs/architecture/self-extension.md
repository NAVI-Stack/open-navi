# Architecture Spec: Self-Extension Pipeline

**Status:** Active  
**Last Updated:** 2026-04-01  
**Implementation:** `internal/navi/gap_detector.go`, `internal/navi/skill/builder.go`

## Overview

The Self-Extension Pipeline allows NAVI to overcome its own capability limits by detecting "gaps" in its toolset and autonomously synthesizing new skills. This transforms NAVI from a fixed-capability agent into an evolving system.

## The Gap Detection Cycle

The pipeline is triggered by two classes of signals in the `internal/navi` loop:

### Signal A: Cognitive Inability (LLM Rejection)
When the LLM explicitly states it cannot fulfill a request because a required tool or information source is absent.

### Signal B: Tool Failure Classification (OMN-20)
When a tool call returns an error, the `GapDetector` classifies it into one of eight failure classes:
- **Class A-C:** Transient/Logic errors (Retry/Replan)
- **Class D-F:** Missing Capability/Permission (Trigger Self-Extension)

## The Self-Build Loop (SkillBuilder)

Once a gap is confirmed, the **SkillBuilder** (OMN-21) takes over:

1. **Gap Analysis:** Deconstructs the failed turn to identify the missing interface.
2. **Synthesis:** Uses the `llm.Provider` (Control Plane) to generate an OSS27-compliant `SKILL.yaml` or `SKILL.md`.
3. **Internal Transport Mapping:** Maps the synthesized skill to a supported transport (REST, MCP, or Python).
4. **Validation:** Runs the skill through the `internal/navi/skill` validator.
5. **Registration:** Hot-loads the new skill into the `SkillRegistry` without restarting the daemon.
6. **Notification:** Informs the foreground session that the gap has been closed and the task can resume.

## Architectural Boundaries

- **Governor Control:** The SkillBuilder must respect `governor.CheckPath` when writing new skills to the workspace.
- **Identity Awareness:** Every synthesized skill carries provenance metadata (Inferred from GapID) in the `World Model`.
- **Fallbacks:** If synthesis fails or validation rejects the skill, the loop returns a standard failure to the user rather than entering an infinite build cycle.

---
*For the high-level roadmap, see [../VISION.md](../VISION.md).*
