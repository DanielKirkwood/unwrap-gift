# Implementation Report: Organiser API (Phase 5)

## Summary

Added the organiser-facing HTTP surface for the Secret Santa Drawer: CRUD for Drawers, Members,
and Relationships (exclusion pairs), Draw creation, and a run-draw endpoint that invokes the
already-complete `internal/assign` engine and persists the resulting Assignments. All four
resources mount on the hidden router behind the existing `deps.Auth` + `deps.Authz` group
(reusing the existing global admin Keto role — no new Keto/OPL work). `RunDraw` auto-relaxes the
per-drawer history window by one on `ErrNoValidAssignment`, down to zero, before giving up;
exclusions are never relaxed.

## Assessment vs Reality

| Metric | Predicted (Plan) | Actual |
|---|---|---|
| Complexity | Large | Large — matched |
| Confidence | Self-contained, no questions needed (per plan's Completion Checklist) | Confirmed — no user input needed during implementation |
| Files Changed | ~20 | 26 (13 created, 13 modified) |

## Tasks Completed

| # | Task | Status | Notes |
|---|---|---|---|
| 1 | Migration — `history_window_draws` | Done | Round-tripped via `db:migrate`/`db:rollback`/`db:migrate` |
| 2 | Query edits (`UpdateDrawer` + `ListRecentCompletedDrawsByDrawer`) | Done | |
| 3 | DB-layer test coverage | Done | |
| 4 | `internal/api/drawers.go` | Done | |
| 5 | `internal/api/drawers_test.go` | Done | Identity injection via `AuthenticationMiddleware` + fake `SessionValidator`, mirroring `auth_test.go` |
| 6 | `internal/api/members.go` | Done | |
| 7 | `internal/api/members_test.go` | Done | |
| 8 | `internal/api/relationships.go` | Done | |
| 9 | `internal/api/relationships_test.go` | Done | |
| 10 | `internal/api/draws.go` | Done | Deviated — added `GET .../draws/{id}/assignments` (see Deviations) |
| 11 | `internal/api/draws_test.go` | Done | |
| 12 | Router wiring (`router.go`) | Done | |
| 13 | `router_test.go` | Done | |
| 14 | `internal/app/drawers.go` | Done | |
| 15 | `internal/app/members.go` | Done | |
| 16 | `internal/app/relationships.go` | Done | |
| 17 | `internal/app/draws.go` (RunDraw orchestration) | Done | |
| 18 | `internal/app/draws_test.go` | Done | Deviated — 3-member fixture, not 2 (see Deviations) |
| 19 | `internal/app/servers.go` wiring | Done | Deviated — extracted `wireOrganiserAPI` helper to satisfy `funlen` |
| 20 | `internal/app/servers_test.go` | Done | |

## Validation Results

| Level | Status | Notes |
|---|---|---|
| Static Analysis | Pass | `go build ./...` clean; `task lint` → 0 issues |
| Unit Tests | Pass | `task test` — all packages green |
| Build | Pass | |
| Integration | N/A | No separate integration-test tier beyond `task test` |
| Edge Cases | Pass | Cross-drawer 404, same-phone-number 400, already-run 409, no-valid-assignment 409, too-few-members 422 |
| SQL/Generated Code | Pass | `sqlc:vet` clean; `sqlc:check`'s `git diff` only shows the expected uncommitted drawers/draws changes from Task 2 — resolves once committed |
| Database | Pass | `db:migrate`/`db:status` show migration `00004` applied |
| Race Detector | Pass | `go test -race -cover ./internal/app/...` → 54.6% coverage, no races |

## Files Changed

| File | Action | Notes |
|---|---|---|
| `internal/db/migrations/00004_drawer_history_window.sql` | CREATED | |
| `internal/db/queries/drawers.sql` | UPDATED | |
| `internal/db/queries/draws.sql` | UPDATED | |
| `internal/db/sqlc/{drawers,draws}.sql.go`, `models.go`, `querier.go` | UPDATED (regenerated) | |
| `internal/db/drawers_test.go` | UPDATED | |
| `internal/db/draws_test.go` | UPDATED | |
| `internal/api/drawers.go` | CREATED | |
| `internal/api/drawers_test.go` | CREATED | |
| `internal/api/members.go` | CREATED | |
| `internal/api/members_test.go` | CREATED | |
| `internal/api/relationships.go` | CREATED | |
| `internal/api/relationships_test.go` | CREATED | |
| `internal/api/draws.go` | CREATED | |
| `internal/api/draws_test.go` | CREATED | |
| `internal/api/router.go` | UPDATED | |
| `internal/api/router_test.go` | UPDATED | |
| `internal/app/drawers.go` | CREATED | |
| `internal/app/members.go` | CREATED | |
| `internal/app/relationships.go` | CREATED | |
| `internal/app/draws.go` | CREATED | |
| `internal/app/draws_test.go` | CREATED | |
| `internal/app/servers.go` | UPDATED | |
| `internal/app/servers_test.go` | UPDATED | |

## Deviations from Plan

1. **`internal/api/draws.go`**: added `GET /drawers/{drawerID}/draws/{id}/assignments`, a route
   the plan's prose didn't explicitly spell out. The plan's own `DrawStore` interface requires
   `ListAssignmentsByDraw`, and leaving it unreachable from any handler would have made it dead
   code — this mirrors the create/list/get nested-resource shape already used by `MountMembers`.

2. **`internal/app/draws_test.go`**: the relax-by-one test uses **3 members, not 2** as the plan's
   Task 18 prose describes. Working the 2-member case through `assign.go`'s actual forbidden-pair
   logic (as the plan's own GOTCHA instructs) shows it can't produce a window-2-unsolvable/
   window-1-solvable split: with only 2 members, forbidding either single direction already
   strands the other gifter with zero candidates, regardless of window. A 3-member drawer with
   its two derangements (D1: A→B→C→A, D2: A→C→B→A) is the smallest fixture where relaxing by
   exactly one genuinely flips the outcome — the behavior Task 18 asks the test to prove.

3. **`internal/app/servers.go`**: extracted the Phase 5 wiring block into a new private
   `wireOrganiserAPI(deps *api.RouterDeps, a *App)` helper, and added `titleNotFound`/
   `titleConflict` constants. Required to satisfy `golangci-lint`'s `funlen` (BuildServers was
   157 lines) and `goconst` (repeated "Not Found"/"Conflict" literals) — not a plan requirement,
   but needed for `task lint` to pass.

4. **`internal/app/draws.go`'s `persistAssignment`**: added a transactional re-check of the
   draw's status, found during `/code-review` after implementation. `RunDraw`'s own draft-status
   check runs *before* `persistAssignment`'s transaction opens, so two concurrent `RunDraw` calls
   on the same draft draw could both pass that check before either commits. SQLite's
   single-connection pool serializes the two transactions, so without this fix the second
   transaction would blindly try to insert assignment rows colliding with the first's
   already-committed ones (every full assignment over the same member set reuses every member as
   a gifter), surfacing as an unmapped UNIQUE-constraint 500 instead of a clean
   `api.ErrDrawAlreadyRun`. Fixed by re-fetching the draw's status inside `persistAssignment`'s
   `WithTx` closure and returning `api.ErrDrawAlreadyRun` if it's no longer `'draft'`, before any
   insert runs. Covered by a new regression test,
   `TestPersistAssignment_RaceAgainstConcurrentRun`, which calls `persistAssignment` directly
   twice for the same draw to simulate the race without needing real goroutine concurrency.

## Issues Encountered

- `sqlc:check` (`git diff --exit-code internal/db/sqlc` after a fresh `sqlc generate`) reports a
  diff locally because the Task 2 schema changes are legitimately uncommitted at report time —
  not drift from regeneration itself (confirmed: re-running `sqlc generate` produces no additional
  diff beyond what's already in the working tree). Resolves once this work is committed.
- The IDE's gopls diagnostics repeatedly showed stale "field/method undefined" errors for
  `HistoryWindowDraws` and `ListRecentCompletedDrawsByDrawer` immediately after each edit; every
  one was confirmed stale by a real `go build ./...`, which was clean throughout.

## Tests Written

| Test File | Tests | Coverage |
|---|---|---|
| `internal/db/drawers_test.go` | +1 assertion block | `history_window_draws` round-trip |
| `internal/db/draws_test.go` | +1 test | `ListRecentCompletedDrawsByDrawer` ordering/filtering |
| `internal/api/drawers_test.go` | 7 tests | CRUD, organiser-from-context, caller-identity listing, invalid ID, not found |
| `internal/api/members_test.go` | 6 tests | CRUD, cross-drawer 404, invalid ID |
| `internal/api/relationships_test.go` | 5 tests | CRUD, missing query param 400, same-phone 400 |
| `internal/api/draws_test.go` | 7 tests | CRUD, assignments listing, run success envelope, already-run/no-valid-assignment/too-few-members error mapping |
| `internal/api/router_test.go` | +1 test | Organiser routes nil-vs-set across all four resources |
| `internal/app/draws_test.go` | 4 tests | Relax-by-one (the plan's flagged most-important behavior), never-relax-exclusions, already-run guard, concurrent-run race regression — real temp SQLite, no mocking |
| `internal/app/servers_test.go` | +1 table test (4 cases) | Organiser API wiring requires all three features (database+kratos+keto) enabled |

## Next Steps

- [ ] Code review via `/code-review`
- [ ] Create PR via `/prp-pr`
