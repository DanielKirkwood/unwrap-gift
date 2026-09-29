# Implementation Report: Secret Santa Drawer — Data Model & Migrations (Phase 1)

## Summary

Added the persistent schema for the Secret Santa Drawer feature: six new SQLite tables (`drawers`, `members`, `relationships`, `draws`, `assignments`, `wishlist_items`) via migration `00003_secret_santa_drawer.sql`, their sqlc-annotated queries, sqlc-generated Go code, and DB-layer integration tests — following the exact layering the `widgets` resource already establishes in this codebase. Enabled SQLite foreign-key enforcement (`_foreign_keys=on`) so the new `REFERENCES`/`ON DELETE CASCADE` clauses are actually enforced. No `internal/api` or `internal/app` changes, per scope.

## Assessment vs Reality

| Metric | Predicted (Plan) | Actual |
|---|---|---|
| Complexity | Large (~20 files) | Large — matched |
| Confidence | N/A (not stated in plan) | High — all 11 tasks completed with zero deviations |
| Files Changed | ~20 | 21 (1 store.go edit, 1 migration, 6 query files, 6 generated `.sql.go` files, 2 updated generated files, 6 test files) |

## Tasks Completed

| # | Task | Status | Notes |
|---|---|---|---|
| 1 | Enable SQLite foreign-key enforcement | Complete | |
| 2 | Write the migration | Complete | |
| 3 | `queries/drawers.sql` | Complete | |
| 4 | `queries/members.sql` | Complete | |
| 5 | `queries/relationships.sql` | Complete | |
| 6 | `queries/draws.sql` | Complete | |
| 7 | `queries/assignments.sql` | Complete | |
| 8 | `queries/wishlist_items.sql` | Complete | |
| 9 | Generate and commit sqlc code | Complete | `task sqlc:generate` + `task sqlc:vet` both clean; regeneration is idempotent (verified by staging then regenerating — zero diff) |
| 10 | DB integration tests | Complete | 12 test functions across 6 files (plan estimated 10+) |
| 11 | Lint and full validation | Complete | Fixed one `govet` shadow finding during this pass (see Issues Encountered) |

## Validation Results

| Level | Status | Notes |
|---|---|---|
| Static Analysis | Pass | `task lint` — 0 issues |
| Unit Tests | Pass | 12 new test functions, all passing |
| Build | Pass | `task build` succeeds |
| Integration | Pass | DB integration tests exercise real SQLite via `t.TempDir()` |
| Edge Cases | Pass | FK violation, cascade delete, CHECK constraints (status enum, self-assignment, relationship ordering), UNIQUE constraint (duplicate gifter), nullable-field round-trip |

Additional manual DB validation performed:
- `task db:migrate` → `task db:rollback` → `task db:migrate` round-trip: clean.
- `task db:reset` + `task db:status`: all three migrations (`00001`–`00003`) show applied.
- Inspected `dev.db` directly via `sqlite3`: all 6 new tables + all 5 indexes present; `PRAGMA foreign_key_list(members)` confirms the `drawers` FK with `ON DELETE CASCADE`.

## Files Changed

| File | Action | Lines |
|---|---|---|
| `internal/db/store.go` | UPDATED | +1/-1 (dsn `_foreign_keys=on`) |
| `internal/db/migrations/00003_secret_santa_drawer.sql` | CREATED | +91 |
| `internal/db/queries/drawers.sql` | CREATED | +14 |
| `internal/db/queries/members.sql` | CREATED | +18 |
| `internal/db/queries/relationships.sql` | CREATED | +8 |
| `internal/db/queries/draws.sql` | CREATED | +14 |
| `internal/db/queries/assignments.sql` | CREATED | +6 |
| `internal/db/queries/wishlist_items.sql` | CREATED | +13 |
| `internal/db/sqlc/assignments.sql.go` | GENERATED | +66 |
| `internal/db/sqlc/drawers.sql.go` | GENERATED | +113 |
| `internal/db/sqlc/draws.sql.go` | GENERATED | +123 |
| `internal/db/sqlc/members.sql.go` | GENERATED | +153 |
| `internal/db/sqlc/relationships.sql.go` | GENERATED | +77 |
| `internal/db/sqlc/wishlist_items.sql.go` | GENERATED | +117 |
| `internal/db/sqlc/models.go` | GENERATED (updated) | +53 |
| `internal/db/sqlc/querier.go` | GENERATED (updated) | +25 |
| `internal/db/drawers_test.go` | CREATED | 1 test |
| `internal/db/members_test.go` | CREATED | 3 tests |
| `internal/db/relationships_test.go` | CREATED | 2 tests |
| `internal/db/draws_test.go` | CREATED | 2 tests |
| `internal/db/assignments_test.go` | CREATED | 3 tests + 1 shared helper |
| `internal/db/wishlist_items_test.go` | CREATED | 1 test |

## Deviations from Plan

None — implemented exactly as planned. The migration SQL, query annotations, and generated code all match the plan's specified content verbatim.

## Issues Encountered

- `golangci-lint`'s `govet` shadow check flagged a shadowed `err` in `TestDeleteDrawer_CascadesMembers` (`members_test.go`) where a second `if err := ...; err != nil` shadowed the outer `err` from `CreateMember`. Fixed by renaming the inner variable to `deleteErr`, consistent with the `deleteErr`/`getErr` naming already used elsewhere in the same file and in `widgets_test.go`.
- gopls repeatedly reported stale "undefined method/type" diagnostics on newly created test files referencing freshly `sqlc generate`-created symbols (e.g. `sqlc.CreateDrawerParams`). Each time, `go build ./...` and `go test ./...` confirmed these were stale editor-cache false positives, not real compile errors — the generated files were already on disk and valid.

## Tests Written

| Test File | Tests | Coverage |
|---|---|---|
| `drawers_test.go` | 1 | Full CRUD round-trip |
| `members_test.go` | 3 | CRUD, FK violation (nonexistent `drawer_id`), cascade delete from parent `drawers` row |
| `relationships_test.go` | 2 | CRUD, `CHECK (phone_number_a < phone_number_b)` ordering constraint |
| `draws_test.go` | 2 | CRUD incl. status transitions (`draft`→`assigned`→`notified`), invalid-status `CHECK` rejection |
| `assignments_test.go` | 3 | CRUD, self-assignment `CHECK`, duplicate-gifter `UNIQUE` constraint |
| `wishlist_items_test.go` | 1 | CRUD incl. `sql.NullString{}` zero-value round-trip for optional `size`/`url` |

12 test functions total (plan's acceptance criterion: 10+).

## Next Steps
- [ ] Code review via `/code-review`
- [ ] Create PR via `/prp-pr`
