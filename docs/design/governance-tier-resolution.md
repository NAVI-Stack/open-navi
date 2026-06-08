# Governance tier resolution (GOV-04)

**Status:** Design (implemented in code; this doc captures behavior)  
**Package:** `internal/governor`

## Summary

Governance uses three tiers: **System** > **Owner** > **Plugin**. Between tiers, the highest-precedence tier with a non-Approved outcome wins. Within a tier, the **most restrictive** outcome wins.

## Tier order

| Tier    | Precedence | Typical steps |
|---------|------------|----------------|
| System  | 1 (highest)| Permissions, Policy, Risk Assessment |
| Owner   | 2          | Configuration, Priority Alignment |
| Plugin  | 3 (lowest) | Plugin-scoped checks (future) |

Resolution: iterate in tier order; the first tier that has at least one non-Approved result is chosen, and within that tier the outcome with the highest restrictiveness is returned.

## Restrictiveness order

From least to most restrictive:

| Outcome                  | Restrictiveness |
|--------------------------|-----------------|
| Approved                 | 0 (ignored in resolution) |
| RequiresConfirmation     | 1 |
| Modified                 | 2 |
| Rejected                 | 3 |

Within a single tier, if multiple steps return different non-Approved outcomes, the one with the highest restrictiveness wins (e.g. Rejected over RequiresConfirmation).

## Pipeline step → tier mapping

In `Pipeline.Run` (validate.go):

- Permissions       → System  
- Policy             → System  
- Configuration      → Owner  
- PriorityAlignment  → Owner  
- RiskAssessment     → System  

So two System steps returning RequiresConfirmation and Rejected yield Rejected (most restrictive within System). System tier is then considered first; if it has any non-Approved result, that tier’s outcome is returned and Owner/Plugin are not used for resolution.

## Edge cases

- **All steps Approved:** Result is Approved; no tier/reason needed.
- **Same tier, same restrictiveness:** The first step in execution order that produced that outcome is retained (by construction in the current implementation, one outcome per tier is stored).
- **Future multi-outcome steps:** If a CheckFunc ever returns multiple tier-scoped results, the Pipeline must aggregate them per tier (e.g. by max restrictiveness per tier) before applying tier order.

## References

- `internal/governor/validate.go`: `outcomeRestrictiveness`, `tierOrder`, `Pipeline.Run`
- Conceptual design: GOV-04 (tier authority, most restrictive within tier)
- Checklist: concept implementation audit (GOV-04)
