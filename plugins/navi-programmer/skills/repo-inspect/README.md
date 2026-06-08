# repo-inspect

Read-only repository comprehension skill for NAVI Programmer.

This skill is intentionally narrow: it can inspect repository structure and
content inside a caller-provided root, but it cannot mutate files or call the
network. The subprocess entrypoint is implemented in `main.py` with stdlib-only
Python so the skill can run before deeper NAVI Programmer runtime wiring exists.

## Interfaces

| Interface | Purpose |
| --- | --- |
| `list_tree` | List a scoped directory tree with entry and depth limits. |
| `read_file` | Read a scoped text file with byte and optional line-window limits. |
| `search_text` | Search scoped text files with substring or regex matching. |
| `repo_status` | Report Git availability, repo root, branch, head, dirty state, and status lines. |
| `inspect_diff` | Report a scoped working-tree or staged diff with structured changed-file metadata. |

## Scope Rules

- `root` defaults to `NAVI_WORKSPACE_DIR`, then the skill process cwd.
- Absolute paths are allowed only when they resolve inside `root`.
- `.git`, `.navi`, virtualenv, build output, dependency, cache, and hidden paths
  are denied or skipped by default.
- `include_hidden: true` allows hidden dot-prefixed paths, but never overrides
  explicitly denied names such as `.git` or `node_modules`.
- Binary files are skipped by search and rejected by read.

## Runtime

`SKILL.yaml` uses `subprocess_python` with `function: run`, which means NAVI
core invokes `main.py` directly with a JSON payload shaped as:

```json
{"interface":"read_file","arguments":{"root":"C:/repo","path":"README.md"}}
```

The entrypoint returns one NAVI `SkillResult` envelope on stdout. No NAVI core
handler is required for this first executable contract.
