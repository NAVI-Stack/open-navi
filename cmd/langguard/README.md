# langguard — language-layer conformance guard

`langguard` mechanically enforces the [Language-Layer Contract](../../docs/architecture/language-layer-contract.md)
§8 so violations **fail the build** instead of relying on review. It is the checker behind the
`conformance` CI job (see `.github/workflows/ci.yml`).

It enforces three rules and exits non-zero on any violation:

| Rule | What it catches | Contract |
|------|-----------------|----------|
| `forbidden-import` | A module under `python/` imports a store/DB handle, a database driver, a privileged connector, or NAVI Go internals. | §3.1, §6.2, §6.3 |
| `ungoverned-read` | A Python file calls the gateway context endpoint (`/api/context/query`) directly instead of via the `navi` SDK, **or** a `query_context(...)` call omits `purpose` or `scope`. | §4 |
| `generated-drift` | A generated governed DTO file was hand-edited (or codegen wasn't re-run): it differs from freshly regenerated output. | §7 |

## How each rule is detected

1. **Forbidden imports.** Static scan of every `*.py` under `python/`. Each `import`/`from … import`
   line is reduced to its top-level module (relative `from . import …` is ignored — it stays
   in-package) and matched against an explicit denylist (`forbiddenImports` in `main.go`): `store`,
   `sqlite3`/`aiosqlite`/`psycopg2`/`asyncpg`/`pymysql`/`pymongo`/`redis`/`sqlalchemy`/…,
   `connector(s)`, `internal`, `navid`.

2. **Ungoverned reads.** Two sub-checks, both skipping test files (which deliberately exercise the
   rejection paths) and the `python/navi/` SDK package (which legitimately defines and documents the
   endpoint it wraps):
   - **raw endpoint** — any other Python file containing the literal `/api/context/query`;
   - **call sites** — every `query_context(...)` *call* (definitions are skipped) whose argument list
     does not contain both `purpose` and `scope`.

3. **Generated drift.** Regenerates the two governed contract files from the canonical Go types and
   compares (line-ending-normalized, so git autocrlf doesn't cause false positives):
   - `schema/python/navi_schema/governed.py` ← `go run ./schema/python/gen`
   - `web-src/navi-console/src/types/generated/governed.ts` ← `go run ./schema/ts/gen`

   A non-matching file means it was hand-edited or codegen is stale. Requires `go` on `PATH`; pass
   `-skip-drift=true` to skip this rule where Go isn't available (rules 1 and 2 still run).

## Run it locally (reproduce CI)

```bash
# Build to bin/ and run (the make target does both):
make conformance

# Or directly, without building:
go run ./cmd/langguard -root .

# Skip the drift check (no Go toolchain for codegen):
go run ./cmd/langguard -root . -skip-drift=true
```

Exit codes: `0` conformant · `1` violation(s) found (each printed with `file:line` and a fix) ·
`2` the checker could not run (e.g. `go` missing for the drift check).

## Fixing a failure

- **forbidden-import** — remove the import; reach the kernel only through the `navi` SDK's governed
  surfaces, never raw store/DB/connector handles.
- **ungoverned-read** — call `navi.query_context(run_id=…, purpose=…, scope=…)`; never hit the
  gateway endpoint directly and never drop `purpose`/`scope`.
- **generated-drift** — do not edit generated files. Run `make generate` and commit the regenerated
  output.

## Build artifact policy

`make langguard` writes to `bin/langguard` (gitignored). Never `go build ./cmd/langguard` without
`-o bin/…` — that drops a binary in the repo root. Use `go build -o /dev/null ./cmd/langguard` for a
compile-only check.
