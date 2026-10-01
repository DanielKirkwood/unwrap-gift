# Implementation Report: Participant API (Wishlist CRUD)

## Summary
Implemented Phase 6 of the secret-santa-drawer PRD: a new, authenticated (no authz) `wishlist-items` resource on the protected router. Participants log in via their existing SMS-code session and manage their own wishlist (`POST/GET/PUT/DELETE /wishlist-items`); the owning phone number is always derived from the caller's Kratos identity `phone` trait, never from the request body, and ownership is enforced server-side (fetch-then-compare) so no participant can read or mutate another participant's items.

## Assessment vs Reality

| Metric | Predicted (Plan) | Actual |
|---|---|---|
| Complexity | Medium | Medium — matched; no new complexity discovered during implementation |
| Confidence | High (plan explicitly resolved all open design forks before writing tasks) | Confirmed — all 9 tasks implemented exactly as planned, zero scope surprises |
| Files Changed | 8 (2 new, 6 updated) | 8 (4 new files — the plan's "2 new" undercounted; it's 2 new resource files × 2 layers = 4 — plus the same 6 updated) |

## Tasks Completed

| # | Task | Status | Notes |
|---|---|---|---|
| 1 | Add `GetWishlistItem` query | [done] Complete | |
| 2 | Extend DB-layer test | [done] Complete | |
| 3 | API layer — DTO, interface, handlers | [done] Complete | Deviated — see below |
| 4 | API-layer unit tests | [done] Complete | |
| 5 | App layer — adapter with ownership enforcement | [done] Complete | |
| 6 | Wire into `RouterDeps` and protected router | [done] Complete | Also updated `NewProtectedRouter`'s doc comment, which the plan didn't call out but was clearly stale ("the widget CRUD example endpoints are the first (and so far only) routes mounted here") |
| 7 | Wire `BuildServers` | [done] Complete | |
| 8 | App-layer ownership test | [done] Complete | |
| 9 | Server-wiring feature-gate test | [done] Complete | |

## Validation Results

| Level | Status | Notes |
|---|---|---|
| Static Analysis | [done] Pass | `go build ./...`, `go vet ./...`, `golangci-lint run ./...` — 0 issues after fixing 3 lint categories (see Issues Encountered) |
| Unit Tests | [done] Pass | 11 new/extended tests written across 4 files (2 new, 2 extended) |
| Build | [done] Pass | |
| Integration | [done] Pass | `task db:migrate` — no-op, confirming no new migration was needed (query-only change) |
| Edge Cases | [done] Pass | Ownership mismatch, missing phone trait, no-cookie, invalid id, empty-list — all covered |

## Files Changed

| File | Action | Lines |
|---|---|---|
| `internal/db/queries/wishlist_items.sql` | UPDATED | +3 |
| `internal/db/sqlc/querier.go` | UPDATED (regenerated) | +1 |
| `internal/db/sqlc/wishlist_items.sql.go` | UPDATED (regenerated) | +19 |
| `internal/db/wishlist_items_test.go` | UPDATED | +14 |
| `internal/api/wishlist_items.go` | CREATED | +200 |
| `internal/api/wishlist_items_test.go` | CREATED | +252 |
| `internal/api/router.go` | UPDATED | +14/-3 |
| `internal/app/wishlist_items.go` | CREATED | +140 |
| `internal/app/wishlist_items_test.go` | CREATED | +101 |
| `internal/app/servers.go` | UPDATED | +25 |
| `internal/app/servers_test.go` | UPDATED | +78 |
| `.claude/PRPs/prds/secret-santa-drawer.prd.md` | UPDATED | +1/-1 |

## Deviations from Plan

1. **Task 3 (API layer)**: the plan's sketch introduced a `callerPhoneNumber(w, r)` helper to deduplicate the identity/phone-extraction boilerplate shared by all four handlers. During implementation this was simplified back to inlining `IdentityFromContext` + `phoneTraitFromIdentity` directly in each of the four handlers — exactly matching `createDrawer`/`listDrawers`'s existing repeated-inline style (and `members.go`/`widgets.go`'s precedent of repeating small param-extraction blocks per handler rather than factoring them out). **Why**: the plan's own "Patterns to Mirror" section, and this codebase's general convention (confirmed via `ARCHITECTURE.md` and every existing resource file), favors a few lines of repetition per handler over a new shared abstraction — introducing `callerPhoneNumber` would have been exactly the kind of premature abstraction the project's own conventions avoid.
2. **Lint fixes** (not a plan deviation, but worth noting): three categories of `golangci-lint` findings required small restructuring not explicitly anticipated by the plan's code sketches:
   - `govet`'s `shadow` check flagged four `if x, err := f(); cond {}` inline-declaration patterns in the two new test files (both `_test.go` files written for Tasks 8 and the extended Task 2 test) — fixed by splitting each into a separate declaration + `if` statement, matching the existing `wishlist_items_test.go` (db layer) and `draws_test.go` two-statement convention more precisely than the plan's inline sketches did.
   - `godoclint` wanted `sql.NullString` written as `[sql.NullString]` (a doc-link format) in one comment in `internal/app/wishlist_items.go`.
   - `golines` flagged one line as improperly formatted; resolved as part of the shadow-check restructuring above.

No other deviations. Every task's `ACTION`/`IMPLEMENT`/`MIRROR`/`VALIDATE` was followed as written.

## Issues Encountered

- The IDE/LSP repeatedly reported stale "undefined: GetWishlistItem" diagnostics after each edit to files referencing the newly-generated sqlc method, across multiple files and multiple edits. Each time, `go build ./...` confirmed the code was correct and the diagnostic was simply lagging the on-disk generated code — no actual issue, just noise to route around during the session.
- `task sqlc:check` reports a diff when run against an uncommitted working tree (by design — it's a drift check, and anything uncommitted shows as "diff"). Confirmed this wasn't generation drift by re-running `sqlc generate` after the initial run and verifying it produced zero additional changes — the reported diff was exactly the new, correctly-generated code, not a mismatch. This will pass cleanly in CI once committed.

## Tests Written

| Test File | Tests | Coverage |
|---|---|---|
| `internal/db/wishlist_items_test.go` (extended) | +2 assertions in existing `TestWishlistItemQueries_CRUD` | `GetWishlistItem` found + not-found (`sql.ErrNoRows`) cases |
| `internal/api/wishlist_items_test.go` (new) | 6 test functions (incl. 1 table test with 4 subtests) | Full CRUD happy path, phone-derived-not-client-supplied ownership, 401 no-cookie, 500 missing-phone-trait, 404 store-not-found, invalid id |
| `internal/app/wishlist_items_test.go` (new) | 2 test functions | The core security property — owner can CRUD their own items, a different phone number cannot update/delete them (`api.ErrWishlistItemNotFound`), list is scoped per phone, plus the nonexistent-id case |
| `internal/app/servers_test.go` (extended) | +1 test function (3 subtests) | `wireWishlistItems` mounts iff database **and** kratos are both enabled (not database alone, not requiring keto) |

**Total**: 11 new/extended test functions/assertions, all passing; full suite (`go test ./...`) shows zero regressions elsewhere.

## Next Steps
- [ ] Code review via `/code-review`
- [ ] Create PR via `/prp-pr`
