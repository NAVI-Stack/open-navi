# NAVI Programmer Tests

This directory will hold plugin-level verification assets.

Use it for fixtures, task evals, and workflow smoke tests that prove NAVI
Programmer can execute bounded programming work safely and repeatably.

The directory also contains direct unit tests for executable skill contracts
such as `repo-inspect` and `run-validation`.

## Starter Eval Set

The current starter set covers:

- structured evaluation records for all starter categories
- weekly maturity reporting over plugin scaffold, repo comprehension, mutation,
  validation, repo lifecycle, task execution, self-update candidate work,
  ticket-driven work, and reliability
- standalone fixture checks for read-only repository comprehension and bounded
  documentation mutation

Run the local fixture checker with:

```powershell
python -B .\tests\evals\run_eval_fixtures.py
```
