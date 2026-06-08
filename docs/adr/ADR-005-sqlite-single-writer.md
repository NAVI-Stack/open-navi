# ADR-005: SQLite WAL Mode with Single-Writer Connection Pool

**Status:** Accepted
**Date:** 2026-03-11
**Deciders:** NAVI AI Core Team
**Related:** [ADR-002: NATS JetStream Bus](./ADR-002-nats-jetstream-bus.md) · [Project State](../../PROJECT_STATE.md)

---

## Context

NAVI AI uses SQLite as its state persistence layer for:
- Task records (CRUD across the task lifecycle)
- Agent session metadata
- Skill facts and short-term memory
- Surface claim locks (`surface_claims` table)
- Future: permission grant ledger integration

SQLite is accessed from multiple goroutines concurrently — the Orchestrator loop, Worker runners, and the Gateway API all read and write state independently. SQLite's default locking behavior (journal mode) serializes all writes and causes reader-writer contention that adds measurable latency to tight orchestration loops.

The question was: how do we configure SQLite for safe concurrent access from a Go process with multiple goroutines?

---

## Decision

**SQLite in WAL (Write-Ahead Log) mode with a single-writer connection pool.**

Concretely:
- Database is opened with `PRAGMA journal_mode=WAL` on first connection
- The application maintains exactly **one** `*sql.DB` connection pool dedicated to writes, with `SetMaxOpenConns(1)`
- A separate read pool (`SetMaxOpenConns(N)` where N ≤ CPU count) handles all `SELECT` queries
- All write operations are serialized through the single writer by the Go DB pool's internal queueing
- Read operations use the WAL snapshot and never block writers

---

## Rationale

**Why WAL mode?**

SQLite WAL allows concurrent readers and a single writer simultaneously, without readers blocking writers or writers blocking readers. In default journal mode (`DELETE`), a write locks the entire database file; all concurrent `SELECT`s must wait. In WAL mode, readers see a consistent snapshot at their transaction start time, while the writer appends to the WAL file. This eliminates read-write contention in the orchestration loop.

**Why a single-writer pool (`MaxOpenConns(1)`) instead of write transactions?**

While WAL allows one concurrent writer, SQLite still returns `SQLITE_BUSY` if two writers race at the OS level before one of them acquires the write lock. Using `SetMaxOpenConns(1)` on the write pool means Go's `database/sql` layer serializes all write calls in its connection queue — `SQLITE_BUSY` retries are eliminated entirely at the application level. This is simpler and more reliable than implementing exponential backoff retry logic around write transactions.

**Why not Postgres?**

At the current single-user, single-host scale, Postgres introduces operational overhead with no immediate benefit. Postgres becomes the right choice when NAVI needs multi-host state replication or more than one concurrent writer across separate processes. The schema layer (`internal/store/`) uses an interface that will allow swapping the backing store to Postgres when that threshold is reached.

**Why not an in-memory store?**

NAVI requires durability: tasks, runs, and HITL decisions must survive process restarts. An in-memory store cannot provide this. The ability to `ATTACH` a WAL-mode SQLite file to any file path also makes it trivially embeddable in tests using `t.TempDir()`.

---

## Consequences

**Positive:**
- Zero `SQLITE_BUSY` errors under normal operation; Go's pool serializes writes
- Readers (Gateway API, metrics) never block on writer (orchestrator, workers)
- Pure Go: `modernc.org/sqlite` driver requires no CGO, no OS-level SQLite installation
- Test-friendly: each test suite opens its own `file::memory:?cache=shared` or `t.TempDir()` database
- Simple migration path: store interface allows Postgres swap when multi-host is needed

**Negative:**
- All writes serialized through one connection: throughput bounded by single-threaded SQLite write speed
- WAL files accumulate if `PRAGMA wal_checkpoint` is not run periodically; a supervisor goroutine runs checkpoints on a configurable interval
- Two pools to manage (read + write); connection leak bugs must account for both

**Accepted risk:** At very high task throughput, the single-writer pool would become a bottleneck. This threshold is far beyond current scale and will be addressed before it is reached.

---

## Alternatives Considered

| Option | Rejected Because |
|---|---|
| SQLite default (journal mode) | Reader-writer contention causes jitter in orchestration; unacceptable |
| Multiple writer connections with retry | `SQLITE_BUSY` handling adds complexity; pool serialization is cleaner |
| Postgres | Over-engineered for single-host usage; adds ops overhead |
| BoltDB / BadgerDB | Excellent for KV workloads; SQL query flexibility needed for task filtering, provenance queries |
| In-memory only | No durability; tasks lost on process restart |

---

*See also: [ADR-002: NATS JetStream Bus](./ADR-002-nats-jetstream-bus.md)*
