# Evals

Evaluation cases for NAVI Programmer task execution, validation, reporting, and
self-update safety.

The starter suite now lives in
[starter-evaluation-suite.json](starter-evaluation-suite.json). It records
structured evaluation records and emits a weekly maturity report envelope over
the minimum starter set categories from the evaluation plan.

| Artifact | Purpose |
| --- | --- |
| [eval_harness.py](eval_harness.py) | Loads the starter suite, validates fixtures, builds evaluation records, and generates the weekly report. |
| [starter-evaluation-suite.json](starter-evaluation-suite.json) | Declares the minimum starter set coverage and all starter scenarios. |
| [read-only-repo-comprehension.eval.json](read-only-repo-comprehension.eval.json) | Preserved standalone read-only fixture check. |
| [bounded-docs-mutation.eval.json](bounded-docs-mutation.eval.json) | Preserved standalone bounded mutation gate check. |

Run the full starter suite:

```powershell
python -B .\tests\evals\run_eval_fixtures.py
```

Write the full structured report to disk:

```powershell
python -B .\tests\evals\run_eval_fixtures.py --output .\tests\evals\latest-report.json
```
