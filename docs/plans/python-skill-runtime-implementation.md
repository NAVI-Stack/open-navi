# Python Skill Runtime — Implementation Verification Prompt

## Context for the agent receiving this prompt

You are working in the NAVI codebase at `C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi`.

In a previous session, the following schema work was completed and committed:

1. **`internal/navi/skill/spec.go`** — Extended with:
   - `PythonRuntimeSpec` struct (top-level `python_runtime:` block in SKILL.yaml)
   - `SubprocessPythonTransport` struct (new `interfaces[].transport.type: subprocess_python`)
   - `SkillResult`, `SkillResultStatus`, `SkillResultError`, `SkillResultMetadata` types
   - `TransportSpec.SubprocessPython *SubprocessPythonTransport` field
   - `OSS27Spec.PythonRuntime *PythonRuntimeSpec` field

2. **`schema/jsonschema/skill-python-runtime.json`** — Canonical JSON Schema for all new types

3. **`docs/concepts/skill-python-runtime.md`** — Full concept doc covering the Python skill contract

4. **`skills/llm-embed/`** — Reference skill (SKILL.yaml + main.py + requirements.txt) using `subprocess_python` transport

The schema is locked. Your job is to **implement the runner layer** and **close all gaps** between the schema and a working execution path. The system is currently at Phase 13. BLOCKER-5 in `docs/tasks/blockers.md` describes the mock execution gap you are resolving for Python skills specifically.

---

## Decisions already locked — do not revisit

| Decision | Value |
|---|---|
| Transport type name | `subprocess_python` |
| I/O format | JSON over stdin/stdout (newline-delimited) |
| Default isolation | Subprocess per invocation (cold spawn) |
| Venv strategy default | `per_category` (category = first segment of skill_id) |
| Warm pools | Per-category, not global; only when `lifecycle: warm` |
| Result envelope | `SkillResult{status, output, error, duration_ms, metadata}` |
| Status values | `success`, `error`, `timeout`, `killed`, `truncated` |
| Termination sequence | SIGTERM → 500ms grace → SIGKILL |
| Timeout override | `python_runtime.timeout_ms` overrides `performance.timeout_ms` |
| Default timeout | 30000ms |
| Default memory cap | 256 MiB (advisory; enforce via runner if platform supports it) |
| Default max input | 1 MiB |
| Default max output | 5 MiB |

---

## What to implement

### 1. Python runner wrapper (`skills/_runtime/runner.py`)

Create a small Python module that the Go runner installs into every skill's venv. Its job:

- Read one JSON object from stdin
- Call `entrypoint_module.function(params)`
- Catch all exceptions
- Write one `SkillResult` JSON object to stdout
- Exit with code 0 always (the Go side reads status from the envelope, not exit code)

The runner wrapper must never write anything to stdout except the final `SkillResult`. Log/debug output goes to stderr only.

```python
# skills/_runtime/runner.py
# Invoked by Go as: python runner.py <entrypoint_module> <function_name>
```

### 2. Go subprocess runner (`internal/navi/skill/runner/python.go`)

Create package `runner` inside `internal/navi/skill/runner/`. Implement:

```go
// PythonRunner executes a subprocess_python skill interface.
type PythonRunner struct {
    spec    *skill.PythonRuntimeSpec
    entry   *skill.SubprocessPythonTransport
    skillID string
    baseDir string // absolute path to skill directory
}

func NewPythonRunner(entry *skill.SkillEntry, iface *skill.Interface) (*PythonRunner, error)
func (r *PythonRunner) Run(ctx context.Context, params map[string]any) (*skill.SkillResult, error)
```

`Run` must:
- Resolve the venv path for this skill (using `venv_strategy` and `skill_id`)
- Validate input size against `max_input_bytes`
- Marshal params to JSON, write to subprocess stdin
- Enforce `timeout_ms` with context cancellation + SIGTERM → grace → SIGKILL
- Read stdout up to `max_output_bytes`; truncate if exceeded (status: truncated)
- Unmarshal the `SkillResult` envelope
- Populate `SkillResult.Metadata` fields (invocation_id, skill_id, interface, duration_ms)
- Return the result — never return a Go error for Python-side failures; those are `status: error`

### 3. Venv manager (`internal/navi/skill/runner/venv.go`)

Implement venv provisioning:

```go
type VenvManager struct { ... }

func (vm *VenvManager) Resolve(skillID, strategy, baseDir string) (venvPath string, err error)
func (vm *VenvManager) Provision(venvPath string, deps []string, reqFile string) error
func (vm *VenvManager) IsProvisioned(venvPath string) bool
```

- `per_category`: venv at `~/.navi/venvs/<category>/` where category = first segment of skill_id
- `per_skill`: venv at `~/.navi/venvs/<skill_id>/`
- `shared`: venv at `~/.navi/venvs/shared/`
- `Provision` runs `python -m venv`, then `pip install` for deps; idempotent if already provisioned
- Install `skills/_runtime/runner.py` into the venv (copy or symlink)

### 4. Warm pool (`internal/navi/skill/runner/pool.go`) — stub acceptable for Phase 14

```go
type WarmPool struct { ... }

func (wp *WarmPool) Get(category string) (*WarmWorker, bool)
func (wp *WarmPool) Put(category string, w *WarmWorker)
func (wp *WarmPool) Close() error
```

A no-op stub that always returns `(nil, false)` from `Get` is acceptable for Phase 14. The runner falls back to cold spawn when the pool returns nothing. Mark the stub clearly with a `// TODO Phase 15: implement warm pool` comment.

### 5. Wire into the skill executor (`internal/navi/loop.go`)

Locate the mock execution block (currently returns `fmt.Sprintf("Executed %s_%s successfully (Mock)", ...)`).

Add a branch: if the matched interface's transport type is `subprocess_python`, call `runner.NewPythonRunner` and `Run`. Return the result output as the tool result string on success, or return a structured error message on non-success status.

The existing mock path should remain for non-Python transports until those are also implemented.

### 6. Policy extension (`internal/navi/skill/policy.go`)

Extend `PolicyEngine.Check` to gate Python skills on maturity:

- `prototype` in a high-risk context (parent task `risk_tier == high`): `PolicyConfirmRequired`
- `network_access: true` with no declared `security.sandbox.network_egress`: `PolicyDeny` with explanation
- Otherwise: pass through to existing risk tier logic

### 7. Loader extension (`internal/navi/skill/loader.go`)

In `loadSkillEntry`, after parsing the OSS-27 spec, validate Python skills:

- If any interface has `transport.type == subprocess_python`, `spec.PythonRuntime` must be non-nil
- If `python_runtime.network_access == true` and `security.sandbox.network_egress` is empty, emit a warning log
- Populate `PythonRuntimeSpec` defaults for omitted fields (timeout_ms, max_input_bytes, etc.)

---

## Gaps to close (non-runner)

These were identified from reading the codebase and are independent of the runner:

### G1 — `normalize.go` does not handle OSS-27 YAML

`NormalizeOpenClawSkill` only looks for `SKILL.md`. `Normalize()` calls it when a directory is found. If a skill directory contains only `SKILL.yaml`, `Normalize()` falls through to `NormalizeRawText` and treats the directory path as raw text content.

**Fix:** In `Normalize()`, check for `SKILL.yaml` first. If present, use `yaml.Unmarshal` → `OSS27Spec` path (same as `loadSkillEntry` in loader.go).

### G2 — `SkillResultError` constant name inconsistency

In `spec.go`, the error status constant is named `SkillResultStatusError` (inconsistent with `SkillResultSuccess`, `SkillResultTimeout`, `SkillResultKilled`, `SkillResultTruncated`).

**Fix:** Rename to `SkillResultError` throughout. Verify no other files reference `SkillResultStatusError`.

### G3 — `meetsRequirements` does not gate Python version

`meetsRequirements` checks `requires_bins` and `requires_env` but not `python_runtime.python_version`. If a skill requires Python 3.11 and only 3.9 is available, the skill loads silently and fails at runtime.

**Fix:** In `loadSkillEntry` (or in a new `meetsPythonRequirements` helper called from `meetsRequirements`), check `python_runtime.python_version` against `python3 --version` output. Skip the skill with a log entry if the version is insufficient.

### G4 — `SkillEntry.Tier` field missing

`SkillEntry` doesn't record which tier the skill was loaded from (`TierWorkspace`, `TierGlobal`, `TierBuiltin`). The policy engine and future governor rules may need this. The loader knows the tier when it loads but discards it.

**Fix:** Add `Tier SkillTier` to `SkillEntry`. Populate it in `ListSkills`.

### G5 — `registry.go` tool name sanitizer duplicated

The tool name sanitizer (replacing non-alphanumeric chars with `_`) is copy-pasted verbatim in both `Tools()` and `FindTool()`. A mismatch here would silently break tool call routing.

**Fix:** Extract to a package-level helper `func sanitizeToolName(raw string) string` and call it from both methods.

### G6 — `docs/concepts/skills.md` does not reference Python runtime

The existing skills doc is unaware of `subprocess_python` transport. The "Execution (Current State vs Target)" section lists `mcp_tool`, `rest`, and `internal` but not `subprocess_python`.

**Fix:** Add a row to the transport type table and link to `skill-python-runtime.md`.

### G7 — `schema/jsonschema/enums.json` `Tool` enum is stale

`Tool` enum in `enums.json` doesn't include `run_python_skill`. The agent capability system uses `permitted_tools` to gate what agents can call. Python skill invocation should be a gated tool type.

**Fix:** Add `"run_python_skill"` to the `Tool` enum.

---

## Tests to write

| File | Test |
|---|---|
| `internal/navi/skill/runner/python_test.go` | `TestPythonRunner_Success` — writes a trivial skill that echoes params, verifies SkillResult |
| `internal/navi/skill/runner/python_test.go` | `TestPythonRunner_Timeout` — writes a skill that sleeps 10s, asserts status=timeout |
| `internal/navi/skill/runner/python_test.go` | `TestPythonRunner_Exception` — writes a skill that raises, asserts status=error with type/message |
| `internal/navi/skill/runner/python_test.go` | `TestPythonRunner_MaxOutputExceeded` — writes a skill that outputs >max_output_bytes, asserts status=truncated |
| `internal/navi/skill/runner/venv_test.go` | `TestVenvManager_Provision` — provisions a real venv with `requests` dep, verifies importable |
| `internal/navi/skill/runner/venv_test.go` | `TestVenvManager_CategoryStrategy` — verifies two `navi.llm.*` skills share a venv path |
| `internal/navi/skill/spec_test.go` | `TestPythonRuntimeSpec_Defaults` — verifies that zero-value fields read from YAML get sensible defaults applied |
| `internal/navi/skill/loader_test.go` | `TestLoadSkillEntry_PythonRuntimeNilRejected` — verifies that a subprocess_python skill without python_runtime fails validation |

---

## File map

```
internal/navi/skill/
  spec.go              ← already updated (PythonRuntimeSpec, SkillResult, etc.)
  loader.go            ← needs G1, G3, G4 gaps closed + Python validation
  registry.go          ← needs G5 gap closed
  policy.go            ← needs Python maturity/network gates
  runner/
    python.go          ← NEW: PythonRunner
    venv.go            ← NEW: VenvManager
    pool.go            ← NEW: WarmPool stub

skills/
  _runtime/
    runner.py          ← NEW: Python-side SkillResult wrapper
  llm-embed/
    SKILL.yaml         ← already written (reference skill)
    main.py            ← already written
    requirements.txt   ← already written

schema/jsonschema/
  skill-python-runtime.json  ← already written
  enums.json                 ← needs G7: add run_python_skill

docs/concepts/
  skill-python-runtime.md    ← already written
  skills.md                  ← needs G6: subprocess_python row in transport table
```

---

## Definition of done

- [ ] `go build ./...` passes with no errors
- [ ] `go test ./internal/navi/skill/...` passes including new runner tests
- [ ] `skills/llm-embed` loads without warnings when `OPENAI_API_KEY` is set
- [ ] A manual invocation of the llm-embed skill via NAVI chat returns a real vector, not a mock string
- [ ] `policy.go` rejects a `prototype` Python skill with `network_access: true` and empty `network_egress`
- [ ] All gaps G1–G7 are closed with corresponding test coverage
- [ ] `docs/tasks/blockers.md` BLOCKER-5 updated: mark Python skills as resolved, note remaining transport types (mcp_tool, rest, internal) as still pending
