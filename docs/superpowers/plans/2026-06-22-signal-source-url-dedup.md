# Signal `source_url` Dedup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `gtm-crm` signal creation idempotent on `source_url` so scanners can re-run without creating duplicate signals.

**Architecture:** Add a nullable `source_url` column to `signals` with a partial UNIQUE index over live rows (`WHERE archived = 0 AND source_url IS NOT NULL`). `SignalRepo.Create` becomes idempotent when `source_url` is set, using a single atomic `INSERT … ON CONFLICT … DO NOTHING RETURNING id` with a same-transaction `SELECT` fallback. When `source_url` is omitted, behavior is unchanged. Surface the field through the `signal add` CLI and the `crm_signal_create` MCP tool.

**Tech Stack:** Go 1.23+, `modernc.org/sqlite` (SQLite 3.51.2), Cobra CLI, `mcp-go`, `testify`.

## Global Constraints

- Module path: `github.com/Array-Ventures/gtm-crm`; binary `crm`.
- Parameterized queries only (`?` placeholders); never interpolate values.
- Soft deletes: filter `WHERE archived = 0`; never `DELETE`.
- stdout = data only; human messages → stderr.
- Migrations are append-only and forward-only; never edit an existing migration. Migration version = file position (this is the 3rd file → `user_version = 3`).
- `ON CONFLICT` target predicate must match the partial index predicate exactly: `WHERE archived = 0 AND source_url IS NOT NULL`.
- Run `gofmt -w` on changed files before each commit. Conventional Commits. Commit identity is the repo default (`modi.parth152@gmail.com`); do not pass `-c user.email` overrides.
- All work on branch `feat/signal-source-url-dedup`.

---

### Task 1: Schema + model + repo plumbing for `source_url`

Adds the column, the partial unique index, the model fields, and stores/returns `source_url` through `Create`/`FindByID`/`FindAll`. No dedup behavior yet (that is Task 2) — `source_url` is just a normal stored field. Existing tests pass unchanged because they never set `source_url` (NULLs do not collide).

**Files:**
- Create: `internal/db/migrations/003_signal_source_url.sql`
- Modify: `internal/model/types.go` (`Signal`, `CreateSignalInput`)
- Modify: `internal/db/repo/signal.go` (`scanSignal`, `Create`, `FindByID`, `FindAll` column lists)
- Modify: `internal/db/db_test.go` (migration version assertion)
- Test: `internal/db/repo/signal_test.go`

**Interfaces:**
- Produces: `model.Signal.SourceURL *string` (`json:"source_url"`); `model.CreateSignalInput.SourceURL *string`. Column order used by every `signals` SELECT and by `scanSignal`: `id, uuid, signal_type, description, source_url, person_id, org_id, detected_at, archived, created_at, updated_at`.

- [ ] **Step 1: Write the failing test**

Add to `internal/db/repo/signal_test.go`:

```go
func TestSignalCreate_StoresSourceURL(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	url := "https://github.com/collinear-ai/verl-trainer"

	created, err := sr.Create(context.Background(), model.CreateSignalInput{
		SignalType: "github",
		SourceURL:  &url,
	})
	require.NoError(t, err)
	require.NotNil(t, created.SourceURL)
	assert.Equal(t, url, *created.SourceURL)

	found, err := sr.FindByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.NotNil(t, found.SourceURL)
	assert.Equal(t, url, *found.SourceURL)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/repo/ -run TestSignalCreate_StoresSourceURL`
Expected: FAIL — compile error, `unknown field 'SourceURL' in struct literal of type model.CreateSignalInput`.

- [ ] **Step 3: Create the migration**

Create `internal/db/migrations/003_signal_source_url.sql`:

```sql
-- up
ALTER TABLE signals ADD COLUMN source_url TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS ux_signals_source_url
    ON signals(source_url) WHERE archived = 0 AND source_url IS NOT NULL;

-- down
DROP INDEX IF EXISTS ux_signals_source_url;
ALTER TABLE signals DROP COLUMN source_url;
```

- [ ] **Step 4: Add model fields**

In `internal/model/types.go`, add `SourceURL` to `Signal` (after `Description`):

```go
	Description *string `json:"description"`
	SourceURL   *string `json:"source_url"`
```

and to `CreateSignalInput` (after `Description`):

```go
	Description *string
	SourceURL   *string
```

- [ ] **Step 5: Thread `source_url` through the repo**

In `internal/db/repo/signal.go`:

Update `scanSignal` to scan the new column in order:

```go
func scanSignal(row interface{ Scan(...any) error }) (*model.Signal, error) {
	var s model.Signal
	err := row.Scan(
		&s.ID, &s.UUID, &s.SignalType, &s.Description, &s.SourceURL,
		&s.PersonID, &s.OrgID, &s.DetectedAt,
		&s.Archived, &s.CreatedAt, &s.UpdatedAt,
	)
	return &s, err
}
```

Update the `INSERT` in `Create` to include `source_url`:

```go
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO signals (uuid, signal_type, description, source_url, person_id, org_id, detected_at)
		 VALUES (?, ?, ?, ?, ?, ?, COALESCE(?, datetime('now')))`,
		id, input.SignalType, input.Description, input.SourceURL, input.PersonID, input.OrgID, input.DetectedAt)
```

Update the SELECT column list in **both** `FindByID` and `FindAll` to include `source_url` after `description`:

```go
		`SELECT id, uuid, signal_type, description, source_url, person_id, org_id, detected_at,
		        archived, created_at, updated_at
		 FROM signals WHERE id = ? AND archived = 0`
```

(`FindAll` uses the same column list with its existing `WHERE archived = 0` + filters.)

- [ ] **Step 6: Bump the migration-version assertion**

In `internal/db/db_test.go`, `TestOpen_MigrationVersion`:

```go
	assert.Equal(t, 3, version)
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `gofmt -w internal/ && go test ./internal/db/... ./internal/model/...`
Expected: PASS (including `TestSignalCreate_StoresSourceURL` and `TestOpen_MigrationVersion`).

- [ ] **Step 8: Commit**

```bash
git add internal/db/migrations/003_signal_source_url.sql internal/model/types.go internal/db/repo/signal.go internal/db/repo/signal_test.go internal/db/db_test.go
git commit -m "feat(signal): add source_url column and partial unique index"
```

---

### Task 2: Idempotent `Create` on `source_url`

Make `Create` dedup on the live partial unique index when `source_url` is set: return the existing signal on conflict instead of erroring or duplicating. When `source_url` is nil/empty, keep the plain insert.

**Files:**
- Modify: `internal/db/repo/signal.go` (`Create`)
- Test: `internal/db/repo/signal_test.go`

**Interfaces:**
- Consumes: `model.CreateSignalInput.SourceURL` and the `ux_signals_source_url` partial index from Task 1.
- Produces: `SignalRepo.Create` is idempotent on `source_url` — same URL → same id, one live row.

- [ ] **Step 1: Write the failing tests**

Add to `internal/db/repo/signal_test.go`:

```go
func TestSignalCreate_DedupsOnSourceURL(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()
	url := "https://github.com/acme/repo"

	first, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &url})
	require.NoError(t, err)
	second, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &url})
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "same source_url must return the same signal")

	all, err := sr.FindAll(ctx, model.SignalFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 1, "no duplicate row")
}

func TestSignalCreate_DistinctSourceURLsCoexist(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()
	a, b := "https://github.com/acme/a", "https://github.com/acme/b"

	_, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &a})
	require.NoError(t, err)
	_, err = sr.Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &b})
	require.NoError(t, err)

	all, err := sr.FindAll(ctx, model.SignalFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestSignalCreate_NullSourceURLsDoNotCollide(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()

	_, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github"})
	require.NoError(t, err)
	_, err = sr.Create(ctx, model.CreateSignalInput{SignalType: "github"})
	require.NoError(t, err)

	all, err := sr.FindAll(ctx, model.SignalFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 2, "NULL source_url never collides")
}

func TestSignalCreate_ArchivedDoesNotBlockReinsert(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()
	url := "https://github.com/acme/repo"

	first, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &url})
	require.NoError(t, err)
	require.NoError(t, sr.Archive(ctx, first.ID))

	second, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &url})
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID, "archived row is excluded by the partial index")

	all, err := sr.FindAll(ctx, model.SignalFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 1, "only the live row is listed")
}

func TestSignalCreate_DedupAcrossConnections(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	d1, err := db.Open(dbPath)
	require.NoError(t, err)
	defer d1.Close()
	d2, err := db.Open(dbPath)
	require.NoError(t, err)
	defer d2.Close()

	ctx := context.Background()
	url := "https://github.com/acme/repo"
	first, err := repo.NewSignalRepo(d1).Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &url})
	require.NoError(t, err)
	second, err := repo.NewSignalRepo(d2).Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &url})
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "DB-enforced dedup holds across separate connections")
}
```

Add `"path/to"` imports if missing: this file already imports `db`, `repo`, `model`, `testify`. Add `"path/filepath"` to the import block for `TestSignalCreate_DedupAcrossConnections`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/db/repo/ -run 'TestSignalCreate_Dedups|TestSignalCreate_Archived|TestSignalCreate_DedupAcross'`
Expected: FAIL — `TestSignalCreate_DedupsOnSourceURL` errors on the second `Create` (UNIQUE constraint violation) and/or returns two rows.

- [ ] **Step 3: Make `Create` idempotent**

Replace the body of `Create` in `internal/db/repo/signal.go` with:

```go
// Create inserts a new signal. When SourceURL is set it is idempotent: a second
// Create with the same live source_url returns the existing signal instead of
// inserting a duplicate (dedup is enforced by the ux_signals_source_url partial
// unique index).
func (r *SignalRepo) Create(ctx context.Context, input model.CreateSignalInput) (*model.Signal, error) {
	if strings.TrimSpace(input.SignalType) == "" {
		return nil, fmt.Errorf("signal_type is required: %w", model.ErrValidation)
	}

	id := uuid.New().String()

	if input.SourceURL != nil && strings.TrimSpace(*input.SourceURL) != "" {
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("begin upsert signal: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		var signalID int64
		err = tx.QueryRowContext(ctx,
			`INSERT INTO signals (uuid, signal_type, description, source_url, person_id, org_id, detected_at)
			 VALUES (?, ?, ?, ?, ?, ?, COALESCE(?, datetime('now')))
			 ON CONFLICT(source_url) WHERE archived = 0 AND source_url IS NOT NULL
			 DO NOTHING
			 RETURNING id`,
			id, input.SignalType, input.Description, input.SourceURL, input.PersonID, input.OrgID, input.DetectedAt,
		).Scan(&signalID)
		if errors.Is(err, sql.ErrNoRows) {
			// Conflict: the INSERT did nothing because a live row already exists.
			err = tx.QueryRowContext(ctx,
				`SELECT id FROM signals WHERE source_url = ? AND archived = 0`,
				*input.SourceURL).Scan(&signalID)
		}
		if err != nil {
			return nil, fmt.Errorf("upsert signal: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit upsert signal: %w", err)
		}
		return r.FindByID(ctx, signalID)
	}

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO signals (uuid, signal_type, description, source_url, person_id, org_id, detected_at)
		 VALUES (?, ?, ?, ?, ?, ?, COALESCE(?, datetime('now')))`,
		id, input.SignalType, input.Description, input.SourceURL, input.PersonID, input.OrgID, input.DetectedAt)
	if err != nil {
		return nil, fmt.Errorf("create signal: %w", err)
	}
	signalID, _ := result.LastInsertId()
	return r.FindByID(ctx, signalID)
}
```

(`errors`, `database/sql`, `fmt`, `strings`, `uuid` are already imported in this file.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `gofmt -w internal/ && go test -race ./internal/db/repo/`
Expected: PASS for all `TestSignal*` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/db/repo/signal.go internal/db/repo/signal_test.go
git commit -m "feat(signal): dedup signal creation on source_url"
```

---

### Task 3: `--source-url` on the `signal add` CLI

Expose `source_url` through the CLI so scanners write it, and surface it in output.

**Files:**
- Modify: `internal/cli/signal.go` (`signalAddCmd`, `signalToMap`, `signalColumns`)
- Test: `internal/cli/signal_test.go`

**Interfaces:**
- Consumes: `model.CreateSignalInput.SourceURL`; the idempotent `Create` from Task 2.
- Produces: `crm signal add <type> --source-url <url>`; `source_url` appears in JSON output and as a `Source` table column.

- [ ] **Step 1: Write the failing test**

Add to `internal/cli/signal_test.go`:

```go
func TestSignalAddSourceURLDedups(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	url := "https://github.com/acme/repo"

	out1, _, code1 := crm(t, dbPath, "signal", "add", "github", "--source-url", url, "-f", "json")
	assert.Equal(t, 0, code1)
	out2, _, code2 := crm(t, dbPath, "signal", "add", "github", "--source-url", url, "-f", "json")
	assert.Equal(t, 0, code2)

	var a, b []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out1), &a))
	require.NoError(t, json.Unmarshal([]byte(out2), &b))
	assert.Equal(t, url, a[0]["source_url"])
	assert.Equal(t, a[0]["id"], b[0]["id"], "same source_url returns the same signal id")

	list, _, _ := crm(t, dbPath, "signal", "list", "-f", "json")
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(list), &rows))
	assert.Len(t, rows, 1)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestSignalAddSourceURLDedups`
Expected: FAIL — `unknown flag: --source-url`.

- [ ] **Step 3: Add the flag and map/column**

In `internal/cli/signal.go`:

Add `"Source"` to `signalColumns` (after the `Type` entry):

```go
	{Header: "Type", Field: "signal_type"},
	{Header: "Source", Field: "source_url"},
```

In `signalToMap`, add after the `Description` block:

```go
	if s.SourceURL != nil {
		m["source_url"] = *s.SourceURL
	}
```

In `signalAddCmd`, add a `sourceURL` variable, set it on the input, and register the flag:

```go
	var description, detectedAt, sourceURL string
```

```go
			input := model.CreateSignalInput{
				SignalType:  args[0],
				Description: nilIfEmpty(description),
				SourceURL:   nilIfEmpty(sourceURL),
				DetectedAt:  nilIfEmpty(detectedAt),
			}
```

```go
	cmd.Flags().StringVar(&sourceURL, "source-url", "", "canonical source URL (dedup key)")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `gofmt -w internal/ && go test ./internal/cli/ -run TestSignalAdd`
Expected: PASS (new test and the existing `TestSignalAdd*` tests).

- [ ] **Step 5: Commit**

```bash
git add internal/cli/signal.go internal/cli/signal_test.go
git commit -m "feat(cli): add --source-url to signal add"
```

---

### Task 4: `source_url` on the `crm_signal_create` MCP tool

Expose the field on the MCP surface so agents/the skill can write it too.

**Files:**
- Modify: `internal/mcp/server.go` (`crm_signal_create` registration, `signalCreateHandler`)

**Interfaces:**
- Consumes: `model.CreateSignalInput.SourceURL`.
- Produces: `crm_signal_create` accepts an optional `source_url` string argument.

- [ ] **Step 1: Add the tool parameter**

In `internal/mcp/server.go`, in the `crm_signal_create` registration, add after the `description` param:

```go
			gomcp.WithString("source_url", gomcp.Description("Canonical source URL (dedup key)")),
```

- [ ] **Step 2: Set it in the handler**

In `signalCreateHandler`, add after the `input` initialization (alongside the other optional fields):

```go
		if u := req.GetString("source_url", ""); u != "" {
			input.SourceURL = &u
		}
```

- [ ] **Step 3: Build and run the MCP package tests**

Run: `gofmt -w internal/ && go build ./... && go test ./internal/mcp/`
Expected: PASS (`TestToolCount` still satisfied; build clean).

- [ ] **Step 4: Commit**

```bash
git add internal/mcp/server.go
git commit -m "feat(mcp): add source_url to crm_signal_create"
```

---

### Task 5: Full verification + live smoke test

**Files:** none (verification only).

- [ ] **Step 1: Full race suite + vet**

Run: `gofmt -l internal/ && go vet ./... && go test -race ./...`
Expected: `gofmt -l` prints nothing; vet clean; all packages PASS.

- [ ] **Step 2: Live idempotency smoke test**

```bash
go build -o /tmp/crm ./cmd/crm
export CRM_DB=/tmp/sig-dedup-smoke.db; rm -f "$CRM_DB"
id1=$(/tmp/crm signal add github --source-url https://github.com/acme/repo -q)
id2=$(/tmp/crm signal add github --source-url https://github.com/acme/repo -q)
echo "id1=$id1 id2=$id2 (expect equal)"
/tmp/crm -f table signal list   # expect exactly one row
rm -f "$CRM_DB" /tmp/crm
```
Expected: `id1 == id2`; `signal list` shows one row.

- [ ] **Step 3: Push and open PR**

```bash
git push -u origin feat/signal-source-url-dedup
gh pr create --repo Array-Ventures/gtm-crm --base main --head feat/signal-source-url-dedup \
  --title "feat(signal): dedup on source_url" \
  --body "Implements docs/superpowers/specs/2026-06-22-signal-source-url-dedup-design.md"
```

---

## Self-Review

- **Spec coverage:** migration 003 (Task 1) ✓; model fields (Task 1) ✓; idempotent `Create` with tx + DO NOTHING + fallback SELECT (Task 2) ✓; live-only/partial index (Task 1 schema, Task 2 archived test) ✓; NULLs don't collide (Task 2 test) ✓; CLI `--source-url` (Task 3) ✓; MCP `source_url` (Task 4) ✓; `db_test` version → 3 (Task 1) ✓; tests incl. cross-connection dedup (Task 2) ✓; build/vet/race + live smoke (Task 5) ✓. Deferred items (org `source_ref`, people `email`/`domain`, generic layer, field-updating upsert, FTS) intentionally absent.
- **Placeholders:** none — every code step shows full code.
- **Type consistency:** `SourceURL *string` used identically in `Signal`, `CreateSignalInput`, repo, CLI (`nilIfEmpty`), MCP (`&u`); SELECT column order matches `scanSignal` across `FindByID`/`FindAll`; `ON CONFLICT` predicate matches the index predicate verbatim.
