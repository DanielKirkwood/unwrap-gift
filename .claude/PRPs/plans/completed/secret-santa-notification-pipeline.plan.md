# Plan: Secret Santa Notification Pipeline

## Summary

On a successful `POST /drawers/{drawerID}/draws/{id}/run`, the organiser API already computes and
persists a valid gifter→giftee assignment and flips the draw's status to `'assigned'`. This plan
extends that same code path to compose and send one SMS per member (exchange date, budget,
giftee's full name, giftee's current wishlist) via the existing `smsclient.Client`, then flips the
draw's status to `'notified'` only once every send has succeeded. No new HTTP endpoint, migration,
or sqlc query is needed — this is a pure extension of `storeDraws.RunDraw`'s existing success path.

## User Story

As the organiser, I want every member to automatically receive an SMS with their giftee's name,
wishlist, the exchange date, and the budget as soon as I run the draw, so that I send zero manual
coordination messages.

## Problem → Solution

Today, `RunDraw` computes and persists assignments and stops at `status = 'assigned'` — members
have no way to learn who they're buying for except an organiser manually reading the
`GET .../assignments` response and messaging each one by hand. After this plan, `RunDraw`'s success
path also sends each gifter a complete SMS and moves the draw to `status = 'notified'`, matching the
schema's existing three-state `draft → assigned → notified` lifecycle (the `'notified'` value has
been in the `CHECK` constraint since Phase 1's migration but nothing writes it today).

## Metadata

- **Complexity**: Medium
- **Source PRD**: `.claude/PRPs/prds/secret-santa-drawer.prd.md`
- **PRD Phase**: Phase 7 — Notification pipeline
- **Estimated Files**: 4 (`internal/api/draws.go`, `internal/app/draws.go`, `internal/app/servers.go`,
  `internal/app/draws_test.go`)

---

## UX Design

Internal change — no user-facing UX transformation (no frontend exists yet; this is a side effect
of an existing API call). The only observable difference to an API caller is: `POST .../run`'s
response `draw.status` is `"notified"` instead of `"assigned"` on full success, and a new possible
error response (`502 Bad Gateway`, see Patterns to Mirror) on partial/total SMS failure.

### Interaction Changes

| Touchpoint | Before | After | Notes |
|---|---|---|---|
| `POST /drawers/{drawerID}/draws/{id}/run` success response | `draw.status = "assigned"`, no SMS sent | `draw.status = "notified"`, one SMS sent per member | Assignments payload shape unchanged |
| Member's phone | Nothing | One SMS: drawer name, giftee's full name, exchange date, budget, giftee's wishlist (or "haven't added a wishlist yet") | Sent to the **gifter's** phone, about their **giftee** |
| `POST .../run` on SMS failure | N/A (didn't exist) | `502 Bad Gateway` Problem Details response; assignments are still persisted and visible via `GET .../assignments`, but `draw.status` stays `"assigned"` | See Risks — no retry-notify endpoint exists in v1 |

---

## Mandatory Reading

| Priority | File | Lines | Why |
|---|---|---|---|
| P0 | `internal/app/draws.go` | 1–320 (whole file) | `storeDraws.RunDraw`/`persistAssignment` — the exact function this plan extends |
| P0 | `internal/clients/smsclient/smsclient.go` | 1–113 (whole file) | `Client.Send`'s contract: never nil, logs instead of sending when disabled — this is what makes `storeDraws`'s `sms` field safe to call unconditionally once wired from `a.SMS` |
| P0 | `internal/app/servers.go` | 167–267 (`wireOrganiserAPI`) | Exactly where `storeDraws` is constructed and `DrawsAdapter`'s `ErrorsMap` is built — both need one new line each |
| P1 | `internal/api/draws.go` | 1–68 | `DrawStore` interface (unchanged) and the existing sentinel-error pattern (`ErrDrawNotFound`, `ErrDrawAlreadyRun`, …) the new `ErrNotificationFailed` must match |
| P1 | `internal/app/app.go` | 34–44, 100–117 | Confirms `a.SMS` is **always non-nil** (built unconditionally in `Bootstrap` regardless of whether the `sms` feature is enabled) — this is why `wireOrganiserAPI` can pass it to `storeDraws` with no nil-check, unlike `Kratos`/`Keto` |
| P1 | `internal/app/wishlist_items.go` | 1–55 | `ListWishlistItemsByPhoneNumber` call shape — reused as-is to fetch the giftee's wishlist, per `[[secret-santa-participant-api-decisions]]`'s note that Phase 7 needs no coordination here |
| P1 | `internal/db/sqlc/models.go` | 12–69 | Exact field names on `sqlc.Assignment`/`sqlc.Draw`/`sqlc.Drawer`/`sqlc.Member`/`sqlc.WishlistItem` used when composing the message |
| P2 | `internal/app/draws_test.go` | 1–320 (whole file) | `newAppTestStore`, `createTestMember`/`createCompletedDraw`/`createTestAssignment` helpers, and the **exact two call sites** (`TestRunDraw_RelaxesHistoryWindowByOne`, `TestRunDraw_AlreadyRun`) that construct `storeDraws{store: store}` and assert `draw.Status == drawStatusAssigned` — both must be updated (see Task 5) |
| P2 | `internal/clients/smsclient/smsclient_test.go` | 1–136 (whole file) | The exact `fakeSender`/`sms77api.SmsBaseParams` shape to mirror for this plan's own fake (can't reuse it directly — it's unexported in `smsclient_test`, a different package) |
| P2 | `internal/db/queries/draws.sql` | 1–21 | Confirms `UpdateDrawStatus` (`:one`, takes `ID`+`Status`) is the only query needed for the status transition — no new query |

## External Documentation

No external research needed — feature uses established internal patterns (`smsclient.Client.Send`,
`db.Store.Queries`, the existing `ErrorsMap`/sentinel-error convention). seven.io's API itself was
already researched in Phase 3's plan.

---

## Patterns to Mirror

### SENTINEL_ERROR
```go
// SOURCE: internal/api/draws.go:19-21
// ErrDrawAlreadyRun is returned by DrawStore.RunDraw when the draw's
// status is no longer 'draft'.
var ErrDrawAlreadyRun = errors.New("api: draw has already been run")
```
New error follows the same shape: `ErrNotificationFailed = errors.New("api: failed to send
notification sms to one or more members")`.

### ERROR_MAPPING (ErrorsMap wiring)
```go
// SOURCE: internal/app/servers.go:242-256
{
    Match: api.ErrNoValidAssignment,
    Problem: api.Problem{
        Status: http.StatusConflict, Title: titleConflict,
        Detail: "no valid assignment exists for this drawer's current members/exclusions/history",
    },
},
{
    Match: api.ErrTooFewMembers,
    Problem: api.Problem{
        Status: http.StatusUnprocessableEntity,
        Title:  "Unprocessable Entity",
        Detail: "drawer has fewer than two members",
    },
},
```

### APP_LAYER_STORE_WITH_EXTERNAL_CLIENT
```go
// SOURCE: internal/app/draws.go:28-30 (struct), internal/app/servers.go:234 (construction)
type storeDraws struct {
    store *db.Store
}
// ...
deps.Draws = storeDraws{store: a.Store}
```
This plan adds a second field (`sms *smsclient.Client`) and a second constructor argument — the
same shape `a.SMS` is already passed around (e.g. `deps.CourierSMS = a.SMS` at
`internal/app/servers.go:76`), just stored on `storeDraws` instead of `RouterDeps` directly.

### RETRY_LOOP_STRUCTURE (existing RunDraw, for context on where the new step attaches)
```go
// SOURCE: internal/app/draws.go:137-155
window := drawer.HistoryWindowDraws
for {
    history, historyErr := s.buildHistory(ctx, drawerID, drawID, window)
    if historyErr != nil {
        return api.Draw{}, nil, historyErr
    }

    result, assignErr := assign.Assign(memberIDs, exclusions, history)
    switch {
    case assignErr == nil:
        return s.persistAssignment(ctx, draw, result)
    case errors.Is(assignErr, assign.ErrTooFewMembers):
        return api.Draw{}, nil, api.ErrTooFewMembers
    case errors.Is(assignErr, assign.ErrNoValidAssignment) && window > 0:
        window--
        continue
    default:
        return api.Draw{}, nil, api.ErrNoValidAssignment
    }
}
```
The `case assignErr == nil:` line changes from `return s.persistAssignment(...)` directly to
persisting first, then calling the new `finalizeNotifications` (Task 2) on success — see Task 3.

### ERROR_JOIN_BEST_EFFORT
```go
// SOURCE: internal/app/servers.go:132-143 (Servers.Shutdown)
errs := make(chan error, 3)
for _, srv := range []*http.Server{s.Public, s.Protected, s.Hidden} {
    go func(srv *http.Server) { errs <- srv.Shutdown(ctx) }(srv)
}
var joined error
for range 3 {
    joined = errors.Join(joined, <-errs)
}
return joined
```
This plan's `notifyMembers` uses the same `errors.Join` idea (sequentially, not concurrently — see
GOTCHA on Task 2) to keep one member's send failure from blocking the rest.

### TEST_FAKE_FOR_SMSCLIENT_SENDER
```go
// SOURCE: internal/clients/smsclient/smsclient_test.go:17-29
type fakeSender struct {
    resp                    *sms77api.SmsResponse
    err                     error
    called                  bool
    gotTo, gotFrom, gotText string
}

//nolint:revive,staticcheck // name must match smsclient.Sender's JsonContext exactly
func (f *fakeSender) JsonContext(_ context.Context, p sms77api.SmsBaseParams) (*sms77api.SmsResponse, error) {
    f.called = true
    f.gotTo, f.gotFrom, f.gotText = p.To, p.From, p.Text
    return f.resp, f.err
}
```
This plan's `fakeSMSSender` (internal/app package, Task 5) is the same idea generalized to capture
**every** call (a slice, not single `got*` fields) since `RunDraw` sends multiple SMS per run.

### APP_LAYER_TEST_DIRECT_CONSTRUCTION
```go
// SOURCE: internal/app/draws_test.go:85 and :196
s := storeDraws{store: store}
// ...
draw, assignments, runErr := s.RunDraw(t.Context(), drawer.ID, current.ID)
```
After this plan, every such call site must also set `sms:` — see Task 5's exact diffs.

---

## Files to Change

| File | Action | Justification |
|---|---|---|
| `internal/api/draws.go` | UPDATE | Add `ErrNotificationFailed` sentinel error (no `DrawStore` interface change) |
| `internal/app/draws.go` | UPDATE | Add `sms *smsclient.Client` field to `storeDraws`; add `drawStatusNotified` const; add `notifyMembers`/`finalizeNotifications`/`composeNotificationMessage`/`formatWishlistItem`/`formatPence` functions; wire `finalizeNotifications` into `RunDraw`'s success case; build `memberByID` map alongside the existing `memberIDs`/`phoneToMemberID` maps |
| `internal/app/servers.go` | UPDATE | `wireOrganiserAPI`: pass `a.SMS` into `storeDraws{...}`; add `ErrNotificationFailed` → `502 Bad Gateway` to `DrawsAdapter.ErrorsMap` |
| `internal/app/draws_test.go` | UPDATE | Add `sms:` field to every existing `storeDraws{store: store}` literal; change the two post-`RunDraw`-success status assertions from `drawStatusAssigned` to `drawStatusNotified`; add `fakeSMSSender` type + two new tests (success-with-wishlist, failure-leaves-draw-assigned) |

## NOT Building

- A dedicated "retry notification" endpoint for a draw stuck at `'assigned'` after a partial/total
  SMS failure — out of scope for this phase; see Risks. The organiser (also the developer) can
  intervene manually for v1's tiny, known groups.
- Reminder SMS before the exchange date — that's a separate `Should`-priority PRD row, not Phase 7.
- Any new HTTP route — notification is purely a side effect of the existing `POST .../run` endpoint.
- seven.io delivery-status (DLR) webhook polling — the PRD's success metric allows "manual
  spot-check"; no DLR callback integration is added here.
- New migrations or sqlc queries — `'notified'` is already a valid `draws.status` value (Phase 1
  migration `00003`), and `ListWishlistItemsByPhoneNumber` already exists (Phase 6).
- Multi-currency/localized budget formatting — hardcoded `£` (GBP), matching the PRD's explicit
  exclusion of international SMS formatting.

---

## Step-by-Step Tasks

### Task 1: Add the `ErrNotificationFailed` sentinel error
- **ACTION**: Add one new exported error var to `internal/api/draws.go`.
- **IMPLEMENT**:
  ```go
  // ErrNotificationFailed is returned by DrawStore.RunDraw when assignment
  // succeeds and is persisted, but sending the notification SMS to one or
  // more members fails. The draw's status stays 'assigned' (not
  // 'notified') in this case — see internal/app's
  // storeDraws.finalizeNotifications.
  var ErrNotificationFailed = errors.New("api: failed to send notification sms to one or more members")
  ```
  Place it directly after `ErrTooFewMembers` (same file, same declaration style).
- **MIRROR**: SENTINEL_ERROR pattern above.
- **IMPORTS**: None new — `errors` is already imported in this file.
- **GOTCHA**: `errname` (golangci-lint) requires sentinel errors to be prefixed `Err` — already
  satisfied.
- **VALIDATE**: `go build ./...` compiles; no `DrawStore` interface signature changed.

### Task 2: Add notification-composition and -sending logic to `internal/app/draws.go`
- **ACTION**: Add a `sms *smsclient.Client` field to `storeDraws`, a `drawStatusNotified` const, and
  four new unexported functions/methods.
- **IMPLEMENT**:
  ```go
  const (
      drawStatusDraft    = "draft"
      drawStatusAssigned = "assigned"
      drawStatusNotified = "notified"
  )

  // penceInPound converts draws.budget_amount's minor-unit pence into whole
  // pounds for SMS display.
  const penceInPound = 100

  type storeDraws struct {
      store *db.Store
      sms   *smsclient.Client
  }
  ```
  (add the `sms` field to the existing struct; add `drawStatusNotified` to the existing const block)

  ```go
  // notifyMembers sends one SMS per assignment (to the gifter, about their
  // giftee) and joins every per-member send error instead of stopping at
  // the first failure — a transient failure for one member in a 20-person
  // drawer shouldn't block the other 19 from being notified. Runs
  // sequentially, not concurrently: group sizes are tiny (~5-30) and
  // seven.io's SDK has no documented concurrency guarantees to lean on.
  func (s storeDraws) notifyMembers(
      ctx context.Context,
      drawerName string,
      draw api.Draw,
      assignments []api.Assignment,
      memberByID map[int64]sqlc.Member,
  ) error {
      var errs []error

      for _, assignment := range assignments {
          gifter, giftee := memberByID[assignment.GifterMemberID], memberByID[assignment.GifteeMemberID]

          wishlist, err := s.store.Queries.ListWishlistItemsByPhoneNumber(ctx, giftee.PhoneNumber)
          if err != nil {
              errs = append(errs, fmt.Errorf("app: list wishlist items for giftee %d: %w", giftee.ID, err))
              continue
          }

          body := composeNotificationMessage(drawerName, draw, giftee.FullName, wishlist)
          if sendErr := s.sms.Send(ctx, gifter.PhoneNumber, body); sendErr != nil {
              errs = append(errs, fmt.Errorf("app: notify gifter %d: %w", gifter.ID, sendErr))
          }
      }

      return errors.Join(errs...)
  }

  // finalizeNotifications sends every notification SMS for a
  // just-persisted draw and, only if every send succeeded, flips its
  // status to 'notified'. A partial or total send failure leaves the draw
  // at 'assigned' — the assignments it already computed stay valid and
  // visible via ListAssignmentsByDraw, but RunDraw cannot be called again
  // for this draw (it's no longer 'draft') and there is no retry-notify
  // endpoint in v1. This is a known, surfaced gap (see this plan's Risks),
  // not a silently swallowed one.
  func (s storeDraws) finalizeNotifications(
      ctx context.Context,
      drawerName string,
      draw api.Draw,
      assignments []api.Assignment,
      memberByID map[int64]sqlc.Member,
  ) (api.Draw, []api.Assignment, error) {
      if notifyErr := s.notifyMembers(ctx, drawerName, draw, assignments, memberByID); notifyErr != nil {
          return api.Draw{}, nil, fmt.Errorf("%w: %w", api.ErrNotificationFailed, notifyErr)
      }

      updated, err := s.store.Queries.UpdateDrawStatus(ctx, sqlc.UpdateDrawStatusParams{
          ID:     draw.ID,
          Status: drawStatusNotified,
      })
      if err != nil {
          return api.Draw{}, nil, fmt.Errorf("app: update draw status to notified: %w", err)
      }

      return toAPIDraw(updated), assignments, nil
  }

  // composeNotificationMessage builds the single SMS each gifter receives.
  // The drawer name is prefixed because the same phone number can be a
  // member of more than one drawer (the PRD's Should-priority "Multi-drawer
  // support" row) — without it, two drawers notifying on the same day
  // would read ambiguously. The wishlist is inlined rather than linked:
  // there is no public, unauthenticated wishlist page to link to —
  // wishlist_items are only readable by their own owner via the
  // Auth-protected /wishlist-items endpoints (Phase 6).
  func composeNotificationMessage(drawerName string, draw api.Draw, gifteeName string, wishlist []sqlc.WishlistItem) string {
      var b strings.Builder

      fmt.Fprintf(
          &b,
          "%s Secret Santa: you have %s! Exchange date: %s. Budget: %s.",
          drawerName,
          gifteeName,
          draw.ExchangeDate.Format("2 January 2006"),
          formatPence(draw.BudgetAmount),
      )

      if len(wishlist) == 0 {
          b.WriteString(" They haven't added a wishlist yet.")
          return b.String()
      }

      b.WriteString(" Wishlist: ")
      for i, item := range wishlist {
          if i > 0 {
              b.WriteString("; ")
          }
          b.WriteString(formatWishlistItem(item))
      }
      b.WriteString(".")

      return b.String()
  }

  func formatWishlistItem(item sqlc.WishlistItem) string {
      s := item.ItemName
      if item.Size.Valid {
          s += " (size " + item.Size.String + ")"
      }
      if item.Url.Valid {
          s += " - " + item.Url.String
      }
      return s
  }

  // formatPence renders minor-unit pence as a GBP string ("£25.00"). pence
  // is always non-negative in practice — nothing validates that on
  // draws.budget_amount today, so a negative budget would render with a
  // stray sign in the pence component; not worth guarding for v1 (the
  // organiser is the only writer of this field, via the hidden router).
  func formatPence(pence int64) string {
      return fmt.Sprintf("£%d.%02d", pence/penceInPound, pence%penceInPound)
  }
  ```
- **MIRROR**: ERROR_JOIN_BEST_EFFORT and APP_LAYER_STORE_WITH_EXTERNAL_CLIENT patterns above.
- **IMPORTS**: add `"strings"` and
  `"github.com/DanielKirkwood/unwrap-gift/internal/clients/smsclient"` to `internal/app/draws.go`'s
  import block (both currently absent from that file).
- **GOTCHA**: `mnd` (golangci-lint) flags bare numeric literals — `penceInPound` as a named const is
  required, not `pence/100` inline. `funlen`/`gocognit` limits (100 lines / 20 complexity) are why
  this is split into `notifyMembers` + `finalizeNotifications` rather than one large function,
  mirroring the existing `buildExclusions`/`buildHistory`/`persistAssignment` split.
- **VALIDATE**: `go build ./...`; `golangci-lint run` (via `task lint`) passes with no new findings.

### Task 3: Wire `finalizeNotifications` into `RunDraw`'s success path
- **ACTION**: Modify `RunDraw` in `internal/app/draws.go` to build a `memberByID` map and call
  `finalizeNotifications` after a successful `persistAssignment`, instead of returning
  `persistAssignment`'s result directly.
- **IMPLEMENT**: In the member-list loop (currently building `memberIDs`/`phoneToMemberID`), add a
  third map:
  ```go
  memberIDs := make([]assign.MemberID, len(members))
  phoneToMemberID := make(map[string]assign.MemberID, len(members))
  memberByID := make(map[int64]sqlc.Member, len(members))
  for i, m := range members {
      memberIDs[i] = assign.MemberID(m.ID)
      phoneToMemberID[m.PhoneNumber] = assign.MemberID(m.ID)
      memberByID[m.ID] = m
  }
  ```
  Then change the `switch` statement's success case from:
  ```go
  case assignErr == nil:
      return s.persistAssignment(ctx, draw, result)
  ```
  to:
  ```go
  case assignErr == nil:
      persistedDraw, assignments, persistErr := s.persistAssignment(ctx, draw, result)
      if persistErr != nil {
          return api.Draw{}, nil, persistErr
      }
      return s.finalizeNotifications(ctx, drawer.Name, persistedDraw, assignments, memberByID)
  ```
  `drawer` (the `sqlc.Drawer` fetched earlier in `RunDraw` via `GetDrawer`) is already in scope —
  `drawer.Name` is the exact field needed.
- **MIRROR**: RETRY_LOOP_STRUCTURE pattern above (this is a surgical edit to that existing switch).
- **IMPORTS**: None new beyond Task 2's.
- **GOTCHA**: Do not move the notification step *inside* `persistAssignment`'s transaction — SMS
  sending is network I/O and must not hold the SQLite write-lock; `persistAssignment`'s existing
  transactional behavior (and its race-safety re-check, see `TestPersistAssignment_RaceAgainstConcurrentRun`)
  must stay unchanged.
- **VALIDATE**: `go build ./...` compiles; `go vet ./...` clean.

### Task 4: Wire `a.SMS` and the new error mapping into `internal/app/servers.go`
- **ACTION**: Update `wireOrganiserAPI` to pass `a.SMS` into `storeDraws` and add
  `ErrNotificationFailed` to `DrawsAdapter.ErrorsMap`.
- **IMPLEMENT**: Change:
  ```go
  deps.Draws = storeDraws{store: a.Store}
  ```
  to:
  ```go
  deps.Draws = storeDraws{store: a.Store, sms: a.SMS}
  ```
  and add a fifth entry to `deps.DrawsAdapter.ErrorsMap` (after the existing `ErrTooFewMembers`
  entry):
  ```go
  {
      Match: api.ErrNotificationFailed,
      Problem: api.Problem{
          Status: http.StatusBadGateway,
          Title:  "Bad Gateway",
          Detail: "draw was assigned but one or more notification SMS messages failed to send; " +
              "assignments are saved and visible via GET .../assignments, but the draw's status " +
              "remains 'assigned' until notifications succeed",
      },
  },
  ```
- **MIRROR**: ERROR_MAPPING pattern above.
- **IMPORTS**: None new — `net/http` and `api` already imported in `servers.go`.
- **GOTCHA**: `a.SMS` is guaranteed non-nil by `Bootstrap` (see Mandatory Reading, `app.go:100-117`)
  regardless of whether the `sms` feature is actually enabled — do **not** add a
  `smsFeature.Enabled` guard around this wiring the way `kratos`/`keto`/`database` features are
  guarded elsewhere in this function. `storeDraws.sms` must always be set once `wireOrganiserAPI`
  runs (i.e. whenever database+kratos+keto are enabled), exactly like `deps.CourierSMS = a.SMS` is
  unconditional within its own `if kratosFeature.Enabled` block.
- **VALIDATE**: `go build ./...`; `task test` — existing `TestBuildServers_OrganiserAPI_*` tests in
  `servers_test.go` must still pass unchanged (they don't exercise `RunDraw`'s body, only route
  mounting, so this wiring change doesn't affect them).

### Task 5: Update `internal/app/draws_test.go` for the new `sms` field and add notification tests
- **ACTION**: Fix every existing `storeDraws{...}` literal that's missing the new `sms` field, fix
  the two status assertions that now expect `"notified"` instead of `"assigned"`, and add a
  `fakeSMSSender` plus two new tests.
- **IMPLEMENT**:
  1. Add imports: `"database/sql"`, `"log/slog"`, `"strings"`,
     `"github.com/seven-io/go-client/sms77api"`,
     `"github.com/DanielKirkwood/unwrap-gift/internal/clients/smsclient"`. Confirm whether
     `"context"` is already imported in this file before adding the `fakeSMSSender.JsonContext`
     method below — add it if not.
  2. Add a helper and fake near the top of the file (after `newAppTestStore`):
     ```go
     // newDisabledSMSClient returns a non-nil, disabled *smsclient.Client —
     // RunDraw's notify step logs instead of sending, matching this
     // codebase's "SMS is never nil" convention (see
     // internal/clients/smsclient/smsclient.go's doc comment on Client).
     // Tests that need to assert on the SMS *content* construct their own
     // enabled client with a fakeSMSSender instead (see
     // TestRunDraw_SendsNotificationSMSWithWishlist).
     func newDisabledSMSClient(t *testing.T) *smsclient.Client {
         t.Helper()
         return &smsclient.Client{Logger: slog.New(slog.DiscardHandler)}
     }

     // fakeSMSSender is the smsclient.Sender fake this plan's notification
     // tests substitute via smsclient.Client.Sender. It mirrors
     // internal/clients/smsclient/smsclient_test.go's fakeSender, generalized
     // to capture every call in a slice (RunDraw sends one SMS per member,
     // not just one) — that file's fakeSender can't be reused directly since
     // it's unexported in a different test package (smsclient_test).
     type fakeSMSSender struct {
         calls   []sms77api.SmsBaseParams
         failFor map[string]string // phone number -> error text
     }

     func (f *fakeSMSSender) JsonContext(_ context.Context, p sms77api.SmsBaseParams) (*sms77api.SmsResponse, error) {
         f.calls = append(f.calls, p)

         if errText, fail := f.failFor[p.To]; fail {
             detail := errText
             return &sms77api.SmsResponse{
                 Success:  sms77api.StatusCodeErrorUnknown,
                 Messages: []sms77api.SmsResponseMessage{{Recipient: p.To, Success: false, ErrorText: &detail}},
             }, nil
         }

         return &sms77api.SmsResponse{
             Success:  sms77api.StatusCodeSuccess,
             Messages: []sms77api.SmsResponseMessage{{Recipient: p.To, Success: true}},
         }, nil
     }
     ```
  3. In `TestRunDraw_RelaxesHistoryWindowByOne` (currently `s := storeDraws{store: store}` at line
     ~85): change to `s := storeDraws{store: store, sms: newDisabledSMSClient(t)}`, and change the
     status assertion (currently comparing against `drawStatusAssigned`) to compare against
     `drawStatusNotified` — with a disabled (log-only) SMS client, `Send` never errors, so
     `finalizeNotifications` always completes and flips the draw to `'notified'`.
  4. In `TestRunDraw_NeverRelaxesExclusions`: change `s := storeDraws{store: store}` to include
     `sms: newDisabledSMSClient(t)` (RunDraw never reaches `finalizeNotifications` in this test
     since it asserts `api.ErrNoValidAssignment`, but keep every `storeDraws` literal consistent to
     avoid nil-pointer surprises if the test evolves).
  5. In `TestRunDraw_AlreadyRun`: same `sms: newDisabledSMSClient(t)` addition. No status-string
     assertion changes needed here (it only asserts the *second* call's error).
  6. In `TestPersistAssignment_RaceAgainstConcurrentRun`: same `sms: newDisabledSMSClient(t)`
     addition for consistency (this test calls `persistAssignment` directly, never reaching
     `notifyMembers`, but the field should still be set).
  7. Add two new tests at the end of the file:
     ```go
     // TestRunDraw_SendsNotificationSMSWithWishlist proves RunDraw's new
     // success path: one SMS per member, mentioning their giftee's name,
     // the budget, and (for the member with a wishlist item) that item's
     // name and size — and that the draw ends at 'notified', not 'assigned'.
     func TestRunDraw_SendsNotificationSMSWithWishlist(t *testing.T) {
         t.Parallel()

         store := newAppTestStore(t)

         drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
             Name:                      "Kirkwood Family Christmas",
             OrganiserKratosIdentityID: "organiser-1",
         })
         if err != nil {
             t.Fatalf("CreateDrawer() error = %v, want nil", err)
         }

         alice := createTestMember(t, store, drawer.ID, "Alice", "+447700900000")
         bob := createTestMember(t, store, drawer.ID, "Bob", "+447700900001")

         if _, itemErr := store.Queries.CreateWishlistItem(t.Context(), sqlc.CreateWishlistItemParams{
             PhoneNumber: bob.PhoneNumber,
             ItemName:    "Lego set",
             Size:        sql.NullString{String: "Large", Valid: true},
         }); itemErr != nil {
             t.Fatalf("CreateWishlistItem() error = %v, want nil", itemErr)
         }

         draw, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
             DrawerID:     drawer.ID,
             ExchangeDate: time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC),
             BudgetAmount: 2500,
         })
         if err != nil {
             t.Fatalf("CreateDraw() error = %v, want nil", err)
         }

         fake := &fakeSMSSender{}
         s := storeDraws{
             store: store,
             sms:   &smsclient.Client{Enabled: true, Sender: fake, From: "Santa", Logger: slog.New(slog.DiscardHandler)},
         }

         result, _, runErr := s.RunDraw(t.Context(), drawer.ID, draw.ID)
         if runErr != nil {
             t.Fatalf("RunDraw() error = %v, want nil", runErr)
         }
         if result.Status != drawStatusNotified {
             t.Errorf("RunDraw() draw.Status = %q, want %q", result.Status, drawStatusNotified)
         }
         if len(fake.calls) != 2 {
             t.Fatalf("len(fake.calls) = %d, want 2 (one SMS per member)", len(fake.calls))
         }

         var sentToBob bool
         for _, call := range fake.calls {
             switch call.To {
             case alice.PhoneNumber:
                 if !strings.Contains(call.Text, "Bob") {
                     t.Errorf("sms to Alice = %q, want it to mention Bob (her giftee)", call.Text)
                 }
                 if !strings.Contains(call.Text, "£25.00") {
                     t.Errorf("sms to Alice = %q, want it to mention the budget", call.Text)
                 }
             case bob.PhoneNumber:
                 sentToBob = true
                 if !strings.Contains(call.Text, "Alice") {
                     t.Errorf("sms to Bob = %q, want it to mention Alice (his giftee)", call.Text)
                 }
                 if !strings.Contains(call.Text, "Lego set") || !strings.Contains(call.Text, "Large") {
                     t.Errorf("sms to Bob = %q, want it to include their wishlist item and size", call.Text)
                 }
             }
         }
         if !sentToBob {
             t.Error("no SMS sent to Bob")
         }
     }

     // TestRunDraw_NotificationFailure_DrawStaysAssigned proves a send
     // failure surfaces as api.ErrNotificationFailed and leaves the draw
     // (and its already-persisted assignments) at 'assigned', not
     // 'notified' — the "known gap, not silently swallowed" behavior
     // documented on finalizeNotifications.
     func TestRunDraw_NotificationFailure_DrawStaysAssigned(t *testing.T) {
         t.Parallel()

         store := newAppTestStore(t)

         drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
             Name:                      "Office Secret Santa",
             OrganiserKratosIdentityID: "organiser-1",
         })
         if err != nil {
             t.Fatalf("CreateDrawer() error = %v, want nil", err)
         }

         alice := createTestMember(t, store, drawer.ID, "Alice", "+447700900000")
         _ = createTestMember(t, store, drawer.ID, "Bob", "+447700900001")

         draw, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
             DrawerID:     drawer.ID,
             ExchangeDate: time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC),
             BudgetAmount: 2000,
         })
         if err != nil {
             t.Fatalf("CreateDraw() error = %v, want nil", err)
         }

         fake := &fakeSMSSender{failFor: map[string]string{alice.PhoneNumber: "invalid recipient"}}
         s := storeDraws{
             store: store,
             sms:   &smsclient.Client{Enabled: true, Sender: fake, From: "Santa", Logger: slog.New(slog.DiscardHandler)},
         }

         _, _, runErr := s.RunDraw(t.Context(), drawer.ID, draw.ID)
         if !errors.Is(runErr, api.ErrNotificationFailed) {
             t.Fatalf("RunDraw() error = %v, want errors.Is(err, api.ErrNotificationFailed)", runErr)
         }

         stored, getErr := store.Queries.GetDraw(t.Context(), draw.ID)
         if getErr != nil {
             t.Fatalf("GetDraw() error = %v, want nil", getErr)
         }
         if stored.Status != drawStatusAssigned {
             t.Errorf(
                 "draw.Status = %q, want %q (assignments persist even when notification fails)",
                 stored.Status, drawStatusAssigned,
             )
         }

         assignments, listErr := store.Queries.ListAssignmentsByDraw(t.Context(), draw.ID)
         if listErr != nil {
             t.Fatalf("ListAssignmentsByDraw() error = %v, want nil", listErr)
         }
         if len(assignments) != 2 {
             t.Errorf("len(assignments) = %d, want 2 (persisted despite notify failure)", len(assignments))
         }
     }
     ```
- **MIRROR**: TEST_FAKE_FOR_SMSCLIENT_SENDER and APP_LAYER_TEST_DIRECT_CONSTRUCTION patterns above.
- **IMPORTS**: see step 1.
- **GOTCHA**: This file is intentionally `package app` (not `app_test`) with an existing
  `//nolint:testpackage` file-level comment — new tests go in this same file/package, not a new
  `_test`-suffixed package. `slog.DiscardHandler` requires Go 1.24+ (this module is on 1.27.1, see
  `go.mod`, so it's available).
- **VALIDATE**: `go test ./internal/app/... -run TestRunDraw -v` — all `TestRunDraw_*` and
  `TestPersistAssignment_*` tests pass.

---

## Testing Strategy

### Unit Tests

| Test | Input | Expected Output | Edge Case? |
|---|---|---|---|
| `TestRunDraw_SendsNotificationSMSWithWishlist` | 2-member drawer, one member has a wishlist item | 2 SMS sent, each mentioning the correct giftee/budget; Bob's SMS includes his wishlist item + size; `draw.Status == "notified"` | No |
| `TestRunDraw_NotificationFailure_DrawStaysAssigned` | 2-member drawer, fake sender fails for one recipient | `RunDraw` returns `errors.Is(err, api.ErrNotificationFailed)`; draw stays `"assigned"` in the DB; both assignment rows still exist | Yes — partial failure |
| `TestRunDraw_RelaxesHistoryWindowByOne` (existing, updated) | 3-member drawer, history forces a relax | Succeeds; `draw.Status == "notified"` (was `"assigned"`) | No — existing test, updated expectation |
| `TestRunDraw_AlreadyRun` (existing, updated) | Same draw run twice | First succeeds (now ends at `"notified"`); second returns `api.ErrDrawAlreadyRun` | No — existing test, construction updated only |

### Edge Cases Checklist
- [x] Empty wishlist — covered by `TestRunDraw_SendsNotificationSMSWithWishlist`'s Alice (no
      wishlist item): message falls back to "They haven't added a wishlist yet."
- [x] One member's SMS send fails — `TestRunDraw_NotificationFailure_DrawStaysAssigned`
- [ ] All members' SMS sends fail — not separately tested; `errors.Join` behavior is identical
      whether one or all fail, so the single-failure test already exercises the relevant code path
- [x] Draw already notified / re-run attempted — covered by existing `TestRunDraw_AlreadyRun`
      (status check happens before notification logic is ever reached)
- N/A Concurrent access — no new concurrency introduced; existing
      `TestPersistAssignment_RaceAgainstConcurrentRun` coverage is untouched by this plan
- N/A Network failure — simulated via `fakeSMSSender`'s `failFor`, not a real network call (no real
      seven.io account exists yet per the PRD's Open Questions)

---

## Validation Commands

### Static Analysis
```bash
go build ./...
go vet ./...
task lint
```
EXPECT: Zero build errors, zero vet findings, `golangci-lint run` clean (no new `mnd`, `errname`,
`funlen`, or `gocognit` findings from the new code).

### Unit Tests
```bash
go test ./internal/app/... -run TestRunDraw -v
go test ./internal/api/... -run TestMountDraws -v
```
EXPECT: All pass, including the two updated existing tests and the two new ones.

### Full Test Suite
```bash
task test
```
EXPECT: No regressions anywhere else in the suite (this plan touches no other package's behavior).

### Database Validation
Not applicable — no migration or sqlc change in this plan.

### Browser Validation
Not applicable — no frontend exists yet (Phase 8).

### Manual Validation
- [ ] Once a real seven.io account is configured (per the PRD's Open Questions), run a real draw
      for a small test drawer end-to-end and confirm every member receives a correct, complete SMS
      — this is the PRD's own Phase 7 success signal and cannot be verified by automated tests
      alone.
- [ ] Spot-check that a drawer member in two drawers (multi-drawer case) receives two
      distinguishable SMS messages, each correctly prefixed with its own drawer's name.

---

## Acceptance Criteria
- [ ] All 5 tasks completed
- [ ] All validation commands pass
- [ ] `TestRunDraw_SendsNotificationSMSWithWishlist` and
      `TestRunDraw_NotificationFailure_DrawStaysAssigned` written and passing
- [ ] No type errors (`go build ./...`)
- [ ] No lint errors (`task lint`)
- [ ] `POST .../run`'s success response shows `status: "notified"`, matching the UX Design table

## Completion Checklist
- [ ] Code follows discovered patterns (SENTINEL_ERROR, ERROR_MAPPING, APP_LAYER_STORE_WITH_EXTERNAL_CLIENT)
- [ ] Error handling matches codebase style (`fmt.Errorf("app: ...: %w", err)`, sentinel errors in `internal/api`)
- [ ] No new logging added beyond what `smsclient.Client.Send` already does when disabled
- [ ] Tests follow the existing `internal/app/draws_test.go` structure (in-package tests, raw
      `t.Fatalf`/`t.Errorf`, no testify)
- [ ] No hardcoded values beyond the documented `£`/GBP decision (see Risks)
- [ ] No unnecessary scope additions (no new endpoint, no retry mechanism, no DLR webhook)
- [ ] Self-contained — no questions needed during implementation

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| A partial/total notification failure leaves a draw permanently at `'assigned'` with no retry endpoint in v1 | M | M | `ErrNotificationFailed` (502) surfaces the failure clearly instead of silently; assignments remain intact and inspectable; acceptable for v1's tiny, organiser-is-the-developer context — a retry endpoint can be added later if this is actually hit |
| seven.io's per-recipient success/failure response shape is unconfirmed against a live account | L | M | Inherited from Phase 3 (`smsclient.Client.checkSmsResponse`'s own doc comment flags this); not newly introduced here — first live confirmation happens during a real draw run |
| A long/many-item wishlist could push the composed SMS past one segment, incurring multi-part SMS cost | L | L | seven.io splits multi-part messages automatically; no truncation added given v1's small expected wishlists |
| Prefixing the drawer name wasn't explicitly requested in the PRD text | L | L | Reasonable default given the PRD's own "Should: Multi-drawer support" row; trivial one-line removal from `composeNotificationMessage` if undesired |

## Notes

- This phase makes **no** changes to `internal/api`'s `DrawStore` interface — `RunDraw`'s signature
  is unchanged. Everything new is an internal implementation detail of `internal/app.storeDraws`,
  which is why there's no new API-layer unit test required beyond what Task 1 already covers
  (compilation).
- The money-formatting (`£` + `formatPence`) and date-formatting (`"2 January 2006"`) choices have
  no prior precedent anywhere else in this codebase (confirmed via search) — this plan establishes
  them as the first instance; a future phase adding more currency/date display should reuse these
  rather than inventing a second convention.
- `drawStatusNotified = "notified"` has been a valid value in the `draws.status` `CHECK` constraint
  since the Phase 1 migration (`00003_secret_santa_drawer.sql:53`) but nothing has written it until
  this plan — confirmed by reading the migration directly rather than assuming from the PRD text,
  consistent with `[[secret-santa-drawer-prd-status]]`'s guidance to verify against code.
