# MCP Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Bring the MCP tool surface to parity with the CLI — add the missing create/update/get/delete/list tools so an AI agent can fully manage organizations, deals, tasks, and signals, and expose `github_url` on person create/update.

**Architecture:** Every change is in `internal/mcp/server.go`. Each new tool is a `gomcp.NewTool(...)` registration in `NewServer` plus a `*Handler` function, both mirroring the *existing* tools in the same file (e.g. `crm_signal_update`/`signalUpdateHandler`, `crm_deal_create`/`dealCreateHandler`, `crm_person_get`/`personGetHandler`, `crm_person_delete`/`personDeleteHandler`, `crm_task_list`/`taskListHandler`). All repos are already constructed in `NewServer` (`pr, or, ir, dr, tr, tagr, sr`). Handlers wrap existing repo methods — no new business logic.

**Tech Stack:** Go, mcp-go (`gomcp`/`server`), modernc.org/sqlite.

## Global Constraints

- All edits in `internal/mcp/server.go` (+ `internal/mcp/server_test.go` for Task 6). Do not touch repos/CLI/migrations.
- Mirror the existing tool/handler patterns in the same file exactly: registration via `s.AddTool(gomcp.NewTool("crm_X", ...params...), xHandler(repo))`; handlers use `requireID(req, "id")` for required ids, `req.GetString/GetInt/GetBool` and `argString/argFloat/argInt64(args, key)` for optional fields, `mcpError(err)` on repo errors, `jsonResult(v)` for success output, `strPtr` for optional string→*string.
- Tool names: `crm_<entity>_<verb>` (snake_case). Tool param names: snake_case. Descriptions: one concise sentence.
- Repo method signatures and input structs are the source of truth — read `internal/db/repo/{org,person,deal,task,signal}.go` and `internal/model/types.go` for exact fields; do not invent fields.
- `internal/mcp/server_test.go`'s `TestToolCount` uses `assert.GreaterOrEqual(len(tools), 15)` — adding tools keeps it passing; do not lower it.
- Run `gofmt -w internal/` before each commit. `go build ./... && go vet ./...` must pass. Conventional Commits. Use the repo's configured git identity (no `-c user.email` overrides).
- One commit per task.

---

### Task 1: Organization create / update / delete tools

**Files:** Modify `internal/mcp/server.go`.

**Interfaces produced:** `crm_org_create`, `crm_org_update`, `crm_org_delete` MCP tools.

Add three tools + handlers, registered near the existing `crm_org_get`/`crm_org_search` block (after `crm_org_get`). Wire to `OrgRepo` (the `or` repo already in `NewServer`).

- `crm_org_create` — mirror `dealCreateHandler`. Params: `name` (Required), `domain`, `industry`, `notes`, `github_url` (all optional strings). Build `model.CreateOrgInput{Name: req.GetString("name",""), Domain: strPtr(...), Industry: strPtr(...), Notes: strPtr(...), GitHubURL: strPtr(...)}` → `or.Create(ctx, input)` → `jsonResult`. (`OrgRepo.Create` is already idempotent on `github_url`.)
- `crm_org_update` — mirror `signalUpdateHandler`/`dealUpdateHandler`. Params: `id` (Required number), then optional `name`, `domain`, `industry`, `notes`, `summary`, `github_url`. Use `requireID`; set each `model.UpdateOrgInput` pointer field only when `argString(args, key)` returns ok; → `or.Update(ctx, id, input)` → `jsonResult`.
- `crm_org_delete` — mirror `personDeleteHandler`. Param: `id` (Required number). `requireID` → `or.Archive(ctx, id)` → on success `gomcp.NewToolResultText(fmt.Sprintf("Organization #%d deleted", id))`.

- [ ] Step 1: Read `internal/db/repo/org.go` (Create/Update/Archive signatures) and `internal/model/types.go` (CreateOrgInput/UpdateOrgInput fields), and the existing `dealCreateHandler`, `signalUpdateHandler`, `personDeleteHandler` in `server.go` as templates.
- [ ] Step 2: Add the three `s.AddTool(...)` registrations after the `crm_org_get` registration.
- [ ] Step 3: Add the three handler funcs (`orgCreateHandler(or)`, `orgUpdateHandler(or)`, `orgDeleteHandler(or)`) after the org handlers section (near `orgGetHandler`).
- [ ] Step 4: `gofmt -w internal/ && go build ./... && go vet ./... && go test ./internal/mcp/` — all pass.
- [ ] Step 5: Commit `feat(mcp): add crm_org_create/update/delete tools`.

---

### Task 2: `github_url` on person create + update

**Files:** Modify `internal/mcp/server.go`.

**Interfaces produced:** `crm_person_create` and `crm_person_update` accept `github_url`.

- In the `crm_person_create` registration, add `gomcp.WithString("github_url", gomcp.Description("GitHub profile URL (unique dedup key)"))`; in `personCreateHandler`, set `GitHubURL: strPtr(req.GetString("github_url", ""))` on the `CreatePersonInput`.
- In the `crm_person_update` registration, add the same `github_url` param; in `personUpdateHandler`, add `if s, ok := argString(args, "github_url"); ok { input.GitHubURL = &s }`.

- [ ] Step 1: Read the existing `crm_person_create`/`personCreateHandler` and `crm_person_update`/`personUpdateHandler` in `server.go`, and confirm `model.CreatePersonInput`/`UpdatePersonInput` have `GitHubURL *string` (they do).
- [ ] Step 2: Add the `github_url` param + handler wiring to both create and update.
- [ ] Step 3: `gofmt -w internal/ && go build ./... && go vet ./... && go test ./internal/mcp/`.
- [ ] Step 4: Commit `feat(mcp): expose github_url on crm_person_create/update`.

---

### Task 3: Signal get / delete tools

**Files:** Modify `internal/mcp/server.go`.

**Interfaces produced:** `crm_signal_get`, `crm_signal_delete`.

Add near the existing `crm_signal_*` tools, wired to `sr` (SignalRepo).

- `crm_signal_get` — mirror `personGetHandler`. Param `id` (Required number) → `sr.FindByID(ctx, id)` → `jsonResult`.
- `crm_signal_delete` — mirror `personDeleteHandler`. Param `id` (Required number) → `sr.Archive(ctx, id)` → `NewToolResultText("Signal #%d deleted")`.

- [ ] Step 1: Read `internal/db/repo/signal.go` (FindByID/Archive) and the `personGetHandler`/`personDeleteHandler` templates.
- [ ] Step 2: Add the two registrations after `crm_signal_update`, and the two handlers after `signalUpdateHandler`.
- [ ] Step 3: `gofmt -w internal/ && go build ./... && go vet ./... && go test ./internal/mcp/`.
- [ ] Step 4: Commit `feat(mcp): add crm_signal_get/delete tools`.

---

### Task 4: Deal list / get / delete tools

**Files:** Modify `internal/mcp/server.go`.

**Interfaces produced:** `crm_deal_list`, `crm_deal_get`, `crm_deal_delete`.

Wired to `dr` (DealRepo), added after the existing `crm_deal_update`.

- `crm_deal_list` — mirror `taskListHandler`. Optional params: `stage` (string), `person_id` (number), `org_id` (number), `open` (bool, exclude won/lost), `limit` (number). Build `model.DealFilters` (read its fields in types.go: `Stage *string`, `PersonID *int64`, `OrgID *int64`, `ExcludeClosed bool`, `Limit int`) — set `ExcludeClosed` from the `open` arg — → `dr.FindAll(ctx, filters)` → `jsonResult`.
- `crm_deal_get` — mirror `personGetHandler`. `id` → `dr.FindByID` → `jsonResult`.
- `crm_deal_delete` — mirror `personDeleteHandler`. `id` → `dr.Archive` → `NewToolResultText("Deal #%d deleted")`.

- [ ] Step 1: Read `internal/db/repo/deal.go` (FindAll/FindByID/Archive) and `internal/model/types.go` (`DealFilters`), plus `taskListHandler`/`personGetHandler` templates.
- [ ] Step 2: Add the three registrations + three handlers.
- [ ] Step 3: `gofmt -w internal/ && go build ./... && go vet ./... && go test ./internal/mcp/`.
- [ ] Step 4: Commit `feat(mcp): add crm_deal_list/get/delete tools`.

---

### Task 5: Task get / update / complete / delete tools

**Files:** Modify `internal/mcp/server.go`.

**Interfaces produced:** `crm_task_get`, `crm_task_update`, `crm_task_complete`, `crm_task_delete`.

Wired to `tr` (TaskRepo), added after the existing `crm_task_list`. Read `internal/db/repo/task.go` for the exact method that marks a task done (e.g. `Complete`/`MarkDone`/`SetComplete` — use whatever the CLI `task done` calls) and `UpdateTaskInput` fields in `types.go`.

- `crm_task_get` — `id` → `tr.FindByID` → `jsonResult`.
- `crm_task_update` — mirror `dealUpdateHandler`. `id` (Required) + optional `title`, `description`, `person_id`, `deal_id`, `due`, `priority` mapped to `model.UpdateTaskInput` (verify field names; `due` likely maps to `DueAt`) → `tr.Update(ctx, id, input)` → `jsonResult`.
- `crm_task_complete` — `id` → call the repo's mark-done method → `jsonResult` (or `NewToolResultText` if it returns no row).
- `crm_task_delete` — `id` → `tr.Archive` → `NewToolResultText("Task #%d deleted")`.

- [ ] Step 1: Read `internal/db/repo/task.go` (FindByID, Update, the done method, Archive) and `internal/model/types.go` (`UpdateTaskInput`), plus `dealUpdateHandler` template.
- [ ] Step 2: Add the four registrations + four handlers.
- [ ] Step 3: `gofmt -w internal/ && go build ./... && go vet ./... && go test ./internal/mcp/`.
- [ ] Step 4: Commit `feat(mcp): add crm_task_get/update/complete/delete tools`.

---

### Task 6: MCP tests for the new tools

**Files:** Modify `internal/mcp/server_test.go`.

Following the existing `TestPersonCreateViaMessage` pattern (uses the test helpers `setupTestServer` and `callTool`), add tests that exercise a representative slice of the new tools end-to-end through the MCP layer:

- `crm_org_create` then `crm_org_update` (set `github_url`/`name`) → assert no error and the returned JSON reflects the change.
- `crm_org_create` twice with the same `github_url` → assert the same org id (idempotent) and not an error.
- `crm_signal_get` / `crm_deal_get` / `crm_task_get` round-trip after a create.

Read the existing `server_test.go` to reuse `setupTestServer`/`callTool` and the response-parsing shape. Keep assertions objective (no error, ids match, fields present).

- [ ] Step 1: Read `internal/mcp/server_test.go` (helpers + `TestPersonCreateViaMessage`).
- [ ] Step 2: Add the tests above.
- [ ] Step 3: `gofmt -w internal/ && go test -race ./internal/mcp/` — all pass.
- [ ] Step 4: Commit `test(mcp): cover new org/signal/deal/task tools`.

---

## Self-Review

- Coverage: org create/update/delete (T1), person github_url (T2), signal get/delete (T3), deal list/get/delete (T4), task get/update/complete/delete (T5), tests (T6) — every gap from the inventory is assigned.
- Each task touches only `server.go` (+ `server_test.go` in T6); no repo/CLI/migration changes; no new business logic.
- Type consistency: handlers wire to existing repo methods whose signatures the implementer reads first; param→input field names verified against `types.go` in each task's Step 1.
