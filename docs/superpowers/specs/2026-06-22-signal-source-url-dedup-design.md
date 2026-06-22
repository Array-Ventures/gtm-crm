# Signal `source_url` dedup — design

**Date:** 2026-06-22
**Repo:** `gtm-crm`
**Status:** approved for planning

## Context

The GTM plugin's signal scanners (first one: GitHub via `gh`) run repeatedly and
on a schedule. Each run re-discovers the same items, so writing signals into
`gtm-crm` must be **idempotent on a stable key** — otherwise `signal list` fills
with duplicates every run.

For GitHub, the natural key is the repo's **`source_url`** (e.g.
`https://github.com/collinear-ai/verl-trainer`): single-valued and globally
unique, unlike `email`/`domain` (see Deferred). This spec adds `source_url` to
the `signal` entity and makes signal creation idempotent on it.

This is a **`gtm-crm` prerequisite** for the plugin's slice-A scanner. The
scanner itself (the `gh` wrapper, the skill) is a separate spec in the plugin
repo.

## Goal

`crm signal add github --source-url <url> …` can be run any number of times for
the same URL and results in exactly one live signal, returning that signal's id
each time (exit 0). Concurrent runs (parallel scanners + the `mcp` server + CLI,
all separate processes on one WAL file) must not create duplicates.

## Design decisions

1. **DB-enforced, not app-level.** Uniqueness is guaranteed by a **partial
   UNIQUE index**, and inserts use a single atomic `INSERT … ON CONFLICT …
   RETURNING` statement. App-level check-then-insert is rejected: it is a
   cross-process TOCTOU race (verified the setup is multi-process). The index is
   the conflict check, moved into the DB where it is atomic. (Verified on
   SQLite 3.51.2 / modernc v1.46.1: atomic upsert returns the same id on insert
   and conflict; row count stays 1; partial index allows multiple NULLs.)
2. **Live-only dedup.** The unique index is partial:
   `WHERE archived = 0 AND source_url IS NOT NULL`. Uniqueness applies only among
   live rows; a soft-deleted signal can reappear if the source still shows it
   ("it's current again"), and `NULL` source_urls never collide (manual signals
   without a URL are unrestricted).
3. **Idempotency folded into `Create`.** No separate `--upsert` flag. When
   `source_url` is set and already exists live, `Create` returns the existing
   signal; otherwise it inserts. When `source_url` is omitted, behavior is
   unchanged (plain insert).
4. **Slice-A signals are org-less.** `org_id` stays `NULL` for now; the scanner
   records owner/repo in `description` and the URL in `source_url`. Find-or-create
   org linkage is a later slice (see Deferred), which is what lets us skip an org
   natural key here.

## Changes

### 1. Migration `internal/db/migrations/003_signal_source_url.sql`
```sql
-- up
ALTER TABLE signals ADD COLUMN source_url TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS ux_signals_source_url
    ON signals(source_url) WHERE archived = 0 AND source_url IS NOT NULL;
-- down
DROP INDEX IF EXISTS ux_signals_source_url;
ALTER TABLE signals DROP COLUMN source_url;
```
(The migration runner is forward-only — it never executes `-- down` sections — so
`down` is documentation only; included for completeness.)
Migration runner is position-indexed, so this is `user_version = 3`.

### 2. Model `internal/model/types.go`
- `Signal`: add `SourceURL *string \`json:"source_url"\``.
- `CreateSignalInput`: add `SourceURL *string`.
- (No change to `UpdateSignalInput` for now.)

### 3. Repo `internal/db/repo/signal.go`
- `scanSignal` and all `SELECT` column lists gain `source_url`.
- `Create` becomes idempotent when `input.SourceURL` is non-nil/non-empty, inside
  a transaction:
  - `INSERT INTO signals(uuid, signal_type, description, source_url, person_id, org_id, detected_at) VALUES (…, COALESCE(?, datetime('now'))) ON CONFLICT(source_url) WHERE archived = 0 AND source_url IS NOT NULL DO NOTHING RETURNING id`
  - If a row is returned → newly inserted → `FindByID(id)`.
  - If `sql.ErrNoRows` (conflict, DO NOTHING) → `SELECT id FROM signals WHERE source_url = ? AND archived = 0` → `FindByID(existing)`.
  - When `SourceURL` is nil/empty → existing plain-insert path (unchanged).
- The `ON CONFLICT` predicate must match the partial index predicate exactly.

### 4. CLI `internal/cli/signal.go`
- `signal add`: add `--source-url string` flag → `input.SourceURL = nilIfEmpty(sourceURL)`.
- Add `source_url` to `signalToMap` (omit when nil) and to `signalColumns`.

### 5. MCP `internal/mcp/server.go`
- `crm_signal_create`: add optional `source_url` string param → `input.SourceURL`.

### 6. Tests
- Repo (`signal_test.go`): add twice with same `source_url` → same id, count stays 1;
  different `source_url` → 2 rows; nil `source_url` twice → 2 rows (NULLs don't
  collide); archive then re-add same `source_url` → new row (live-only); a
  concurrency test (two goroutines/connections, same url → one live row).
- CLI (`signal_test.go`): `signal add --source-url X` twice → same id, exit 0.
- `db_test.go`: `TestOpen_MigrationVersion` expects **3**.

## Out of scope / deferred
- **Org `source_ref` + find-or-create org linkage** — later slice; slice-A signals are org-less.
- **People `email`, Org `domain` as natural keys** — blocked on a multiplicity
  decision (a person/company can have multiple emails/domains; current schema has
  a single nullable column for each, no uniqueness). Needs its own modeling slice
  (single "primary" vs. one-to-many table).
- **Generic config-driven upsert layer** (registry/dispatch) — explicitly rejected;
  keep per-entity typed methods.
- **`--upsert` semantics that UPDATE fields on conflict** — only DO NOTHING / dedup now.
- **FTS on signals**, signal `score`/`strength`.

## Testing & acceptance
- `go build`, `go vet`, `go test -race ./...` green.
- Live check: `crm signal add github --source-url https://x/r -q` twice prints the
  same id; `crm signal list` shows one row.
