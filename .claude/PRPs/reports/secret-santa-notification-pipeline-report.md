# Implementation Report: Secret Santa Notification Pipeline

## Summary

Extended `storeDraws.RunDraw`'s existing success path so that, after an assignment is computed and
persisted, the system composes and sends one SMS per member (exchange date, budget, giftee's full
name, giftee's current wishlist) via `smsclient.Client`, then flips the draw's status from
`'assigned'` to `'notified'` only once every send has succeeded. A partial/total SMS failure leaves
the draw at `'assigned'` and surfaces as a new `502 Bad Gateway` (`api.ErrNotificationFailed`); the
already-persisted assignments remain valid and inspectable via `GET .../assignments`. No new HTTP
endpoint, migration, or sqlc query was needed.

## Assessment vs Reality

| Metric | Predicted (Plan) | Actual |
|---|---|---|
| Complexity | Medium | Medium |
| Confidence | (not scored in plan) | High — matched plan almost exactly, one test-assertion bug found and fixed |
| Files Changed | 4 | 4 (same files: `internal/api/draws.go`, `internal/app/draws.go`, `internal/app/servers.go`, `internal/app/draws_test.go`) |

## Tasks Completed

| # | Task | Status | Notes |
|---|---|---|---|
| 1 | Add `ErrNotificationFailed` sentinel error | Complete | Exact match to plan |
| 2 | Add notification-composition/-sending logic to `internal/app/draws.go` | Complete | Exact match to plan |
| 3 | Wire `finalizeNotifications` into `RunDraw`'s success path | Complete | Exact match to plan |
| 4 | Wire `a.SMS` and error mapping into `internal/app/servers.go` | Complete | Deviated — extracted `wireDraws` helper (see Deviations) |
| 5 | Update `draws_test.go` for `sms` field + new tests | Complete | Deviated — fixed a wishlist-assertion bug in the plan's own test code, extracted an assertion helper, added a missing `nolint` comment (see Deviations) |

## Validation Results

| Level | Status | Notes |
|---|---|---|
| Static Analysis | Pass | `go build ./...`, `go vet ./...`, `golangci-lint run ./...` — 0 issues after fixes |
| Unit Tests | Pass | 2 new tests written; all 5 `TestRunDraw_*` tests pass |
| Build | Pass | `go build ./...` clean |
| Integration | N/A | No new HTTP endpoint; `TestMountDraws_*` (existing route-mounting tests) pass unchanged |
| Edge Cases | Pass | Empty wishlist, single-member send failure, already-notified re-run all covered |

## Files Changed

| File | Action | Lines |
|---|---|---|
| `internal/api/draws.go` | UPDATED | +7 |
| `internal/app/draws.go` | UPDATED | +147 |
| `internal/app/servers.go` | UPDATED | +19/-11 (net, includes `wireDraws` extraction) |
| `internal/app/draws_test.go` | UPDATED | +220 |
| `.claude/PRPs/prds/secret-santa-drawer.prd.md` | UPDATED | Phase 7 row: `in-progress` → `complete`, links fixed |

## Deviations from Plan

1. **`wireOrganiserAPI` split into `wireDraws`** (Task 4) — WHAT: extracted the `deps.Draws`/
   `deps.DrawsAdapter` wiring (now 5 `ErrorsMap` entries) into a new `wireDraws(deps, a)` function
   instead of inlining it in `wireOrganiserAPI`. WHY: `golangci-lint`'s `funlen` linter flagged
   `wireOrganiserAPI` at 103 lines (limit 100) once the fifth `ErrNotificationFailed` entry was
   added; this mirrors the existing `wireWishlistItems` precedent of splitting wiring into its own
   function rather than raising the lint threshold.
2. **Fixed a bug in the plan's own test code** (Task 5) — WHAT:
   `TestRunDraw_SendsNotificationSMSWithWishlist`, as written in the plan, asserted that the SMS
   sent *to* Bob contained his own wishlist item ("Lego set"/"Large"). WHY: this contradicts
   `composeNotificationMessage`'s (and the plan's own) spec — a member's SMS reports their
   **giftee's** wishlist, not their own. With a 2-member drawer the only valid assignment is the
   swap (Alice→Bob, Bob→Alice), so Bob's SMS is about Alice (who has no wishlist items, triggering
   the "haven't added a wishlist yet" fallback) and Alice's SMS is about Bob (who does have the
   item). Running the test as originally written against the plan's own `composeNotificationMessage`
   implementation failed for exactly this reason, confirming it was a test bug, not an
   implementation bug. Fixed by moving the wishlist-content assertion to the message checked against
   Alice's phone number, and asserting Bob's message hits the no-wishlist fallback instead.
3. **Extracted `assertNotificationSMS` helper** (Task 5) — WHAT: pulled the per-call assertion
   switch out of `TestRunDraw_SendsNotificationSMSWithWishlist`'s loop into a standalone
   `assertNotificationSMS(t, call, alicePhone, bobPhone)` helper. WHY: `golangci-lint`'s `gocognit`
   flagged the test at complexity 26 (limit 20) once the extra wishlist assertion was added to fix
   deviation #2; this is a pure test-structure split, no behavior change.
4. **Added missing `//nolint:revive,staticcheck` comment on `fakeSMSSender.JsonContext`** (Task 5)
   — WHAT: added the same nolint comment the plan's own Mandatory Reading cites as present on the
   existing `fakeSender.JsonContext` in `smsclient_test.go`, but which the plan's pasted
   `fakeSMSSender.JsonContext` code omitted. WHY: `golangci-lint`'s `revive` var-naming check flagged
   `JsonContext` (should be `JSONContext` per Go initialism conventions) — the method name must
   match `smsclient.Sender`'s interface exactly, so the existing codebase's established suppression
   applies here too.

## Issues Encountered

- A handful of LSP diagnostics (`drawer.HistoryWindowDraws undefined`,
  `ListRecentCompletedDrawsByDrawer undefined`) appeared transiently mid-edit on lines this plan
  never touched. Confirmed via `go build ./...` (clean) immediately after that these were stale
  editor-side diagnostics, not real compile errors — no action needed.

## Tests Written

| Test File | Tests | Coverage |
|---|---|---|
| `internal/app/draws_test.go` | 2 new (`TestRunDraw_SendsNotificationSMSWithWishlist`, `TestRunDraw_NotificationFailure_DrawStaysAssigned`) + 4 existing updated (`sms:` field added; one status assertion changed `assigned`→`notified`) | Success path (2-member drawer, one wishlist item, draw ends `notified`), partial-failure path (one send fails, draw stays `assigned`, assignments persist) |

## Next Steps
- [ ] Code review via `/code-review`
- [ ] Create PR via `/prp-pr`
