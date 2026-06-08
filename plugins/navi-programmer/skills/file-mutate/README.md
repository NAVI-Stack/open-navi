# file-mutate

Governed whole-file mutation skill for NAVI Programmer.

This skill creates or replaces scoped text files and returns structured mutation
evidence. It is meant for bounded edits where replacing the whole file is
clearer and safer than applying a text patch.

## Interfaces

| Interface | Purpose |
| --- | --- |
| `create_file` | Create a new text file, optionally creating parent directories. Existing files are rejected unless `allow_overwrite` is true and preconditions pass. |
| `write_file` | Replace an existing text file, or create it when `create_if_missing` is true. |

## Safety Rules

- `root` defaults to `NAVI_WORKSPACE_DIR`, then the skill process cwd.
- Paths must resolve inside `root`.
- `.git`, `.navi`, dependency folders, caches, build output, and hidden paths are
  denied by default.
- Hidden paths require `allow_hidden: true`, but explicitly denied names remain
  denied.
- Existing-file writes require `expected_sha256` or `expected_content`.
- Existing-file overwrites through `create_file` require `allow_overwrite: true`
  and `expected_sha256`.
- Binary files are rejected.
- `dry_run: true` returns the same evidence without writing.

## Evidence

Successful results include:

- changed file path
- action type
- before and after SHA-256 hashes
- byte counts
- unified diff summary
- dry-run flag
