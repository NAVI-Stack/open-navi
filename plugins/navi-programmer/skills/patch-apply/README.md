# patch-apply

Structured patch preview and apply skill for NAVI Programmer.

This skill applies bounded text operations to scoped files. It intentionally
uses structured operations instead of free-form shell patch execution so the
runtime can validate paths, preconditions, and expected anchors before writing.

## Interfaces

| Interface | Purpose |
| --- | --- |
| `preview_patch` | Evaluate operations, return changed files and diffs, but do not write. |
| `apply_patch` | Apply operations after all preconditions pass. `dry_run: true` behaves like preview. |

## Supported Operations

| Operation | Required Fields |
| --- | --- |
| `create_file` | `path`, `content` |
| `replace_file` | `path`, `content` |
| `replace_text` | `path`, `old_text`, `new_text` |
| `insert_after` | `path`, `anchor`, `text` |
| `insert_before` | `path`, `anchor`, `text` |
| `append_text` | `path`, `text` |
| `prepend_text` | `path`, `text` |

`replace_text` supports `occurrence: first`, `last`, or `all`. When using
`all`, callers may set `max_replacements` to prevent accidental broad edits.

## Safety Rules

- Paths must resolve inside `root`.
- `.git`, `.navi`, dependency folders, caches, build output, and hidden paths are
  denied by default.
- Hidden paths require `allow_hidden: true`, but explicitly denied names remain
  denied.
- Existing-file `replace_file`, `append_text`, and `prepend_text` operations
  require per-operation `expected_sha256`.
- `replace_text`, `insert_after`, and `insert_before` use their text anchor as
  a precondition; callers may also provide `expected_sha256`.
- Existing-file overwrites through `create_file` require `allow_overwrite: true`
  and `expected_sha256`.
- Binary files are rejected.
- Parent directory creation requires per-operation `create_parent_dirs: true`.
- Apply is planned as all-preconditions-first; write failures report `partial`
  or `failed` outcome with written file evidence.
