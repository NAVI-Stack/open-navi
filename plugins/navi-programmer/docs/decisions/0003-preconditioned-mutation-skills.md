# 0003 - Preconditioned Mutation Skills

**Status:** Accepted
**Date:** 2026-04-28

## Decision

NAVI Programmer mutation starts with two plugin-owned `subprocess_python` skills:

- `skills/file-mutate/` for whole-file create and replace operations
- `skills/patch-apply/` for structured text patch preview and apply operations

Existing-file writes require explicit preconditions:

- `file-mutate.write_file` requires `expected_sha256` or `expected_content`.
- `file-mutate.create_file` can overwrite only with `allow_overwrite: true` and
  `expected_sha256`.
- `patch-apply.replace_file`, `append_text`, and `prepend_text` require
  `expected_sha256` for existing files.
- `patch-apply.replace_text`, `insert_after`, and `insert_before` use their text
  anchors as preconditions and may also include `expected_sha256`.

Both skills reject path escapes, denied repository internals, hidden paths by
default, and binary content.

## Rationale

The first mutation substrate must be useful without becoming a blind file
overwrite tool. Preconditions force callers to prove they inspected or anchored
the target state before writing. Structured patch operations keep patch behavior
reviewable without relying on shell patch execution.

## Consequences

- Mutations now produce changed-file, hash, byte-count, and diff evidence.
- Broad rewrites must use whole-file writes with explicit preconditions.
- Local workflow runners can preview patch evidence before applying changes.
- Some legitimate edits will require an extra read or hash step first, which is
  intentional for V1 safety.
