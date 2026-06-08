# PACMAN example-code review

**Subject:** `example-code/pacman-master` (PACMAN — Private Alternative for Contact Management And Networking)  
**Reviewed for:** NAVI skill integration  
**Date:** 2026-03-08

---

## 1. Summary

The codebase is **not** the classic Berkeley Pac-Man game. It is **PACMAN** (Private Alternative for Contact Management And Networking): a small Go web application for contact management with SQLite storage, HTTP server on port 8080, Basic Auth, and VCF export.

---

## 2. Structure and entry points

| Item | Detail |
|------|--------|
| **Language** | Go 1.23 |
| **Layout** | Single main package: `main.go` (HTTP server + handlers). `database/` (SQLite file), `templates/` (HTML), `dist/` (prebuilt binaries). |
| **Entry point** | `main()` — starts HTTP server on port 8080. No CLI; server only. |
| **Build** | `go build` or `build.sh` (cross-compiles to `dist/pacman_<os>_<arch>`). |

There is no “run one game and return result” style entry point. The app is a long-running service. Skill integration is done by talking to the running server over HTTP (or by starting the binary from a wrapper and then calling HTTP).

---

## 3. Dependencies and environment

| Item | Detail |
|------|--------|
| **Dependencies** | `go.mod`: `github.com/glebarez/sqlite`, `github.com/google/uuid`, `gorm.io/gorm`. All standard for Go. |
| **Hardcoded values** | Port `8080`, `username = "username"`, `password = "password"`, `dbFile = "database/contacts.db"`, `vcfFile = "contacts.vcf"` in `main.go`. Paths are relative to CWD. |
| **OS** | No OS-specific code; cross-build via `build.sh`. |
| **Reliability** | No `requirements.txt`/Python; Go module only. Minimum Go version from `go.mod` (1.23.2). |

---

## 4. Tests and determinism

- **Tests:** None. No `*_test.go` files, no test references in the repo.
- **Determinism:** N/A for “game” logic. Contact CRUD is deterministic; UUID and DB state depend on inputs and order of operations.

**Reliability impact:** No automated tests — **low** from a test-coverage perspective. Integration can still proceed; recommend adding at least smoke tests later.

---

## 5. I/O and side effects

| Type | Detail |
|------|--------|
| **Filesystem** | Reads/writes `database/contacts.db` (GORM/SQLite). Creates `contacts.vcf` on export. Reads `templates/*.html`. |
| **Network** | Binds and serves HTTP on port 8080. No outbound calls. |
| **Display** | None; server only. |

For NAVI `subprocess_python` (or a wrapper that only calls the app via HTTP): the **skill wrapper** need not touch the filesystem if it only issues HTTP requests to an already-running instance. If the wrapper starts the binary, it must run from a working directory where `database/` and `templates/` exist (i.e. the app root).

---

## 6. Reliability summary

| Level | Assessment |
|-------|------------|
| **Overall** | **Medium.** Clear structure and minimal deps; no tests and hardcoded creds/paths. |
| **Integration readiness** | Suitable for a skill that calls the app over HTTP (health check, add contact, export contacts). No JSON API in the app; wrapper uses form POST and HTML/export endpoint. |

**Risks:**

- Hardcoded Basic Auth credentials — skill or config must pass correct `username`/`password`.
- No JSON API — add/list operations require form POST or HTML parsing; export is a single GET that returns VCF.
- Server must be running (or started by the wrapper) before skill actions.

---

## 7. Integration approach (chosen)

- **Skill transport:** `subprocess_python` with a thin Python wrapper.
- **Wrapper behavior:** Reads JSON from stdin (action + params). Assumes PACMAN server is reachable at a configurable `base_url` (e.g. `http://localhost:8080`). Uses Basic Auth to:
  - `health_check`: GET `/` → return status.
  - `add_contact`: POST to `/add` with form data.
  - `export_contacts`: GET `/export` → return VCF content in SkillResult output.
- **No change** to the Go app required. Optional future improvement: add a small JSON API to the app for more robust listing/adding.

---

## 8. References

- Example code path: `projects/navi/example-code/pacman-master/`
- NAVI skill spec: `internal/navi/skill/spec.go`
- Skill concepts: `docs/concepts/skills.md`
