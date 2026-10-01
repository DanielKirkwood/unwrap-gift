# Plan: Organiser API

## Summary

Add the organiser-facing surface for the Secret Santa Drawer: CRUD for Drawers, Members, and
Relationships (exclusion pairs), plus Draw creation and a run-draw endpoint that invokes the
already-complete `internal/assign` engine and persists the resulting Assignments. Everything is
mounted on the hidden router behind the existing `deps.Auth` + `deps.Authz` (no new Keto/OPL work).

## User Story

As the organiser (Daniel), I want to create a drawer, add members and exclusion pairs, create a
draw with a date/budget, and trigger the draw via API calls alone, so that I get back a valid
gifter→giftee assignment without gathering anyone in person or writing any SQL by hand.

## Problem → Solution

Today: Phase 1's schema/queries exist (Drawer, Member, Relationship, Draw, Assignment) and Phase
4's `internal/assign` engine is complete and fully unit-tested, but nothing calls either of them —
there is no HTTP surface to create a drawer, add members, record exclusions, or run a draw.

After this plan: an organiser can, through the hidden router, create a drawer, manage its members
and the shared exclusion graph, create a draw, and POST to a run-draw endpoint that computes and
persists a constraint-respecting assignment — auto-relaxing the repeat-history window on failure,
never relaxing exclusions.

## Metadata

- **Complexity**: Large
- **Source PRD**: `.claude/PRPs/prds/secret-santa-drawer.prd.md`
- **PRD Phase**: Phase 5 — Organiser API
- **Estimated Files**: ~20 (1 new migration, 2 query file edits, regenerated sqlc, 4 new
  `internal/api/*.go` resource files + tests, 4 new `internal/app/*.go` adapters (+1 test),
  `router.go`/`servers.go` wiring + their tests)

---

## UX Design

Internal change — no user-facing UX transformation. This is an API-only surface; the organiser is
the sole consumer, calling it directly (curl/Postman) per the PRD's "API-driven organiser flows for
v1" decision. No frontend work is in scope here (that's Phase 8).

---

## Decisions settled during this planning session

These were open questions in the PRD itself (or revealed by reading Phase 1's actual schema) that
needed a human call before implementation, not assumptions:

| Question | Decision | Why |
|---|---|---|
| How do organiser endpoints enforce drawer ownership? | **Reuse the existing global admin Keto role** (`AuthorizationMiddleware(a.Keto, "Identities", "admin", "manage")`, unchanged). No new Keto/OPL namespace, no dynamic per-drawer tuples. `drawers.organiser_kratos_identity_id` is set from the caller's identity at creation time for audit/attribution and used to scope the "list my drawers" endpoint, but is **not** the enforcement mechanism — the existing hidden-router Authz group already restricts every hidden-router route to the one seeded admin identity, which is sufficient for this solo-organiser v1. | Avoids building a whole new per-object dynamic-Keto-tuple pattern (new OPL namespace, a request-scoped `AuthorizationMiddleware` variant, a dual SQLite/Keto write on every drawer creation) for a need (multiple distinct organisers) that doesn't exist yet. Revisit if/when the PRD's "Should: multi-drawer support per person" ever needs isolation between two *different* organisers. |
| Repeat-match history window N | **Per-drawer configurable column**, `drawers.history_window_draws INTEGER NOT NULL DEFAULT 2`. (new migration `00004`), settable via `PUT /drawers/{id}`. | Explicit user instruction: "a per drawer config ie do not allow repeat assignments from past 2 years for this draw." |
| Relationship (exclusion pair) endpoint input shape | **Raw phone numbers** (`phone_number_a`, `phone_number_b`) in the request body, not Member IDs. | Relationships are deliberately global and independent of any Member row (PRD Decisions Log) specifically so a couple can be excluded before either person is added as a Member anywhere. Member-ID input would defeat that. |
| Behavior when `assign.Assign` returns `ErrNoValidAssignment` | **Auto-relax by decrementing the history window by 1 and retry**, down to 0 (full history ignored), before giving up with a clear error. **Exclusions are never relaxed.** | Explicit user instruction. |

---

## Mandatory Reading

| Priority | File | Lines | Why |
|---|---|---|---|
| P0 | `ARCHITECTURE.md` | 1-137 | The three-router model, nil-is-disabled features, interface segregation, error/envelope model, and the step-by-step "Adding a new resource" walkthrough this plan follows 4×. |
| P0 | `internal/api/widgets.go` | 1-155 | The exact shape to mirror for each new DB-backed resource: `XStore` interface, DTO, `MountX`, per-handler `HandlerFunc`, `xID` URL-param parsing. |
| P0 | `internal/app/widgets.go` | 1-85 | The exact shape to mirror for each new `app`-layer adapter: struct wrapping `*db.Store`, `sqlc.X ↔ api.X` conversion, `mapXErr` translating `sql.ErrNoRows`. |
| P0 | `internal/api/identities.go` | 1-126 | The hidden-router, Auth+Authz-gated mounting pattern (vs. Widgets' protected-router, Auth-only pattern) — this plan's resources use this one. |
| P0 | `internal/api/router.go` | 1-137 | `RouterDeps` fields, `NewHiddenRouter`'s existing Auth+Authz group (where the 4 new `MountX` calls join `MountIdentities`), and that there is **no existing nested-resource routing example** — this plan introduces the first one. |
| P0 | `internal/app/servers.go` | 40-110 | Exactly where/how each feature's deps get wired into `api.RouterDeps`, conditioned on `Registry.Feature(...).Enabled`. |
| P0 | `internal/assign/assign.go` | 1-87 | `Assign(members []MemberID, exclusions []ExclusionPair, history []HistoryPair) (map[MemberID]MemberID, error)` is the exact signature `RunDraw` calls. Note `MemberID`/`ExclusionPair`/`HistoryPair` are `assign`'s own types — `internal/app` converts `int64` ↔ `assign.MemberID` at the boundary. `ErrNoValidAssignment`, `ErrTooFewMembers`, `ErrDuplicateMember` are the only errors it returns. |
| P1 | `internal/db/migrations/00003_secret_santa_drawer.sql` | 1-84 | The exact current schema — note `relationships` has no `drawer_id` (global), `members` has `UNIQUE(drawer_id, phone_number)`, `draws.status` is a 3-value CHECK enum, `assignments` has `UNIQUE(draw_id, gifter_member_id)` and `UNIQUE(draw_id, giftee_member_id)`. |
| P1 | `internal/db/sqlc/{drawers,members,relationships,draws}.sql.go` | all | Exact generated `Params` struct field names/order this plan's app-layer adapters call. |
| P1 | `internal/api/authz.go` | 1-67 | `AuthorizationMiddleware`'s current static-triple shape — confirms it is reused unchanged, not modified. |
| P1 | `internal/api/auth.go` | 1-77 | `IdentityFromContext(ctx) (*kratos.Identity, bool)` — how `CreateDrawer`'s handler gets the caller's identity ID for `organiser_kratos_identity_id`. |
| P1 | `internal/api/errors.go` | 1-80 | `ErrorMapping`/`ErrorsMap`/`Adapter.Adapt` — every new sentinel error this plan adds needs one entry per resource's `ErrorsMap` in `servers.go`. |
| P1 | `internal/api/widgets_test.go` | 1-160 | The fake-store + `httptest` unit-test shape to mirror for each new resource's `*_test.go`. |
| P1 | `internal/db/drawers_test.go`, `relationships_test.go` | all | The DB-integration-test shape (real temp SQLite, `db.Migrate`, no mocking) for the new migration/query coverage. |
| P2 | `internal/api/problem.go`, `internal/api/envelope.go` | all | `Problem`/`WriteProblem`/`WriteData` — nothing new needed here, just confirms the shapes every handler already uses. |
| P2 | `Taskfile.yml` | 85-100 | Exact `sqlc:generate`/`sqlc:check`/`db:migrate` commands for the Validation Commands section below. |

## External Documentation

No external research needed — feature uses established internal patterns (chi routing, sqlc,
goose). The one genuinely new piece of knowledge needed — how to detect a SQLite UNIQUE/CHECK
constraint violation with `modernc.org/sqlite` — is captured as its own pattern below, confirmed
directly against the installed module source (`v1.60.0`), not assumed.

---

## Patterns to Mirror

### RESOURCE_FILE_SHAPE (api layer)
// SOURCE: internal/api/widgets.go:1-60
```go
package api

// ErrXNotFound is returned by XStore implementations when no X exists for
// the given id.
var ErrXNotFound = errors.New("api: x not found")

// X is api's own representation, decoupled from internal/db/sqlc.X.
type X struct { ... }

type XStore interface {
    CreateX(ctx context.Context, ...) (X, error)
    GetX(ctx context.Context, id int64) (X, error)
    ListXByY(ctx context.Context, yID int64) ([]X, error)
    UpdateX(ctx context.Context, id int64, ...) (X, error)
    DeleteX(ctx context.Context, id int64) error
}

func MountX(r chi.Router, xs XStore, adapter Adapter) {
    r.Post("/xs", adapter.Adapt(createX(xs)))
    ...
}
```
Every handler is a closure `func(xs XStore) HandlerFunc` returning
`func(w http.ResponseWriter, r *http.Request) error`.

### NESTED_RESOURCE_ROUTING (new pattern — no existing example in this codebase)
chi supports URL param nesting natively; neither Widgets (flat `/widgets`) nor Identities (flat
`/admin/identities`) demonstrates it, so this is new but idiomatic chi, not a deviation:
```go
func MountMembers(r chi.Router, members MemberStore, adapter Adapter) {
    r.Route("/drawers/{drawerID}/members", func(r chi.Router) {
        r.Post("/", adapter.Adapt(createMember(members)))
        r.Get("/", adapter.Adapt(listMembers(members)))
        r.Route("/{id}", func(r chi.Router) {
            r.Get("/", adapter.Adapt(getMember(members)))
            r.Put("/", adapter.Adapt(updateMember(members)))
            r.Delete("/", adapter.Adapt(deleteMember(members)))
        })
    })
}
```
Every nested handler must parse **both** `{drawerID}` and `{id}`, then verify the fetched row's
`DrawerID` actually equals the path's `drawerID` — on mismatch, return the resource's `ErrXNotFound`
(not a generic 403/400), so a caller can't distinguish "wrong drawer" from "doesn't exist" (no
cross-drawer enumeration via error-shape).

### ID_PARAM_PARSING
// SOURCE: internal/api/widgets.go:147-154 (mirror per-file, do not extract a shared helper — this
// codebase duplicates this exact 6-line helper per resource file rather than factoring it out;
// matches the project's "three similar lines over premature abstraction" convention)
```go
func memberID(r *http.Request) (int64, error) {
    id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
    if err != nil {
        return 0, fmt.Errorf("api: parse member id: %w", err)
    }
    return id, nil
}

func drawerIDParam(r *http.Request) (int64, error) {
    id, err := strconv.ParseInt(chi.URLParam(r, "drawerID"), 10, 64)
    if err != nil {
        return 0, fmt.Errorf("api: parse drawer id: %w", err)
    }
    return id, nil
}
```

### ADAPTER_CONVERSION (app layer)
// SOURCE: internal/app/widgets.go:14-85
```go
type storeX struct{ store *db.Store }

func (s storeX) CreateX(ctx context.Context, ...) (api.X, error) {
    row, err := s.store.Queries.CreateX(ctx, sqlc.CreateXParams{...})
    if err != nil {
        return api.X{}, fmt.Errorf("app: create x: %w", err)
    }
    return toAPIX(row), nil
}

func (s storeX) GetX(ctx context.Context, id int64) (api.X, error) {
    row, err := s.store.Queries.GetX(ctx, id)
    if err != nil {
        return api.X{}, mapXErr(err)
    }
    return toAPIX(row), nil
}

func toAPIX(w sqlc.X) api.X { return api.X{ID: w.ID, ...} }

func mapXErr(err error) error {
    if errors.Is(err, sql.ErrNoRows) {
        return api.ErrXNotFound
    }
    return fmt.Errorf("app: x store: %w", err)
}
```

### SQLITE_CONSTRAINT_DETECTION (new pattern — confirmed against installed modernc.org/sqlite v1.60.0 source)
No existing code in this repo detects a SQLite constraint violation by type; `mapWidgetErr`'s
`sql.ErrNoRows` check is the only error-translation precedent. This plan's Member/Relationship
creation needs to turn a UNIQUE violation into a 409, not a generic 500:
```go
import (
    sqlite "modernc.org/sqlite"
    sqlite3 "modernc.org/sqlite/lib"
)

// isUniqueConstraintErr reports whether err is a SQLite UNIQUE constraint
// violation (code 2067), confirmed against modernc.org/sqlite@v1.60.0's
// lib.SQLITE_CONSTRAINT_UNIQUE constant.
func isUniqueConstraintErr(err error) bool {
    var sqliteErr *sqlite.Error
    return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

// isCheckConstraintErr reports whether err is a SQLite CHECK constraint
// violation (code 275) — used to translate CreateRelationship's "unsorted
// pair" CHECK failure into api.ErrRelationshipInvalidPair.
func isCheckConstraintErr(err error) bool {
    var sqliteErr *sqlite.Error
    return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_CHECK
}
```
GOTCHA: `modernc.org/sqlite`'s `*sqlite.Error` wraps the low-level code; always use `errors.As`
(not a direct type assertion) since `database/sql` may wrap it further depending on driver path.

### ERRORS_MAP_WIRING (app layer → servers.go)
// SOURCE: internal/app/servers.go:83-103
```go
if dbFeature.Enabled && kratosFeature.Enabled && ketoFeature.Enabled {
    deps.Drawers = storeDrawers{store: a.Store}
    deps.DrawersAdapter = api.Adapter{
        Logger: a.Logger,
        ErrorsMap: api.ErrorsMap{
            {Match: api.ErrDrawerNotFound, Problem: api.Problem{Status: http.StatusNotFound, Title: "Not Found", Detail: "drawer not found"}},
        },
    }
    // ... deps.Members/Relationships/Draws + their adapters, same group
}
```

### HANDLER_IDENTITY_ACCESS
// SOURCE: internal/api/auth.go:20-25, used the way CreateDrawer's handler needs it
```go
func createDrawer(drawers DrawerStore) HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) error {
        identity, ok := IdentityFromContext(r.Context())
        if !ok {
            return unauthorizedProblem() // should be unreachable: Auth middleware runs first
        }
        var req drawerCreateRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            return fmt.Errorf("api: decode create drawer request: %w", err)
        }
        drawer, err := drawers.CreateDrawer(r.Context(), req.Name, identity.Id)
        ...
    }
}
```
GOTCHA: `unauthorizedProblem()` is a `Problem`, not an `error` — return
`WriteProblem(w, unauthorizedProblem())` directly (writing the response) and `nil`, not the Problem
itself; this branch should be unreachable in production since `deps.Auth` always runs first in the
hidden router's group, but must still compile/behave correctly for API-layer unit tests that mount
handlers without the middleware chain.

### DB_INTEGRATION_TEST_SHAPE
// SOURCE: internal/db/drawers_test.go:1-66 — real temp SQLite via `newTestStore(t)`, `db.Migrate`,
// no mocking, asserting on the concrete `sqlc.X` row shape.

### API_UNIT_TEST_SHAPE
// SOURCE: internal/api/widgets_test.go:1-160 — a hand-rolled `fakeXStore` implementing the new
// `XStore` interface (no mocking library used anywhere in this codebase), table-driven CRUD status
// assertions, plus one dedicated test per interesting edge case (invalid ID, not-found).

---

## Files to Change

| File | Action | Justification |
|---|---|---|
| `internal/db/migrations/00004_drawer_history_window.sql` | CREATE | Adds `drawers.history_window_draws` — the per-drawer repeat-exclusion window the Decisions table above settles. |
| `internal/db/queries/drawers.sql` | UPDATE | `UpdateDrawer` must set `history_window_draws` alongside `name`. |
| `internal/db/queries/draws.sql` | UPDATE | New `ListRecentCompletedDrawsByDrawer` query — "last N non-draft draws for this drawer, excluding the one being run" — needed by `RunDraw`'s history lookup; no existing query does this. |
| `internal/db/sqlc/*.go` | REGENERATE | Via `task sqlc:generate` after the two query-file edits above. Commit the result — CI's `sqlc:check` fails the build on drift. |
| `internal/db/drawers_test.go` | UPDATE | Cover `history_window_draws` through create/update/get. |
| `internal/db/draws_test.go` | UPDATE | Cover `ListRecentCompletedDrawsByDrawer` (ordering, `status != 'draft'` filter, excluded-draw-id, `LIMIT`). |
| `internal/api/drawers.go` | CREATE | `Drawer` DTO, `DrawerStore` interface, `MountDrawers` — create/list(-mine)/get/update/delete. |
| `internal/api/drawers_test.go` | CREATE | Fake-store unit tests mirroring `widgets_test.go`. |
| `internal/api/members.go` | CREATE | `Member` DTO, `MemberStore` interface, `MountMembers` — nested under `/drawers/{drawerID}/members`. |
| `internal/api/members_test.go` | CREATE | Fake-store unit tests, including the cross-drawer-mismatch-returns-404 case. |
| `internal/api/relationships.go` | CREATE | `Relationship` DTO, `RelationshipStore` interface, `MountRelationships` — flat `/relationships` (global, not drawer-scoped). |
| `internal/api/relationships_test.go` | CREATE | Fake-store unit tests, including same-phone-number rejection. |
| `internal/api/draws.go` | CREATE | `Draw`/`Assignment` DTOs, `DrawStore` interface (create/get/list/list-assignments/**RunDraw**), `MountDraws` — nested under `/drawers/{drawerID}/draws`. |
| `internal/api/draws_test.go` | CREATE | Fake-store unit tests for create/get/list/run, including the "already run" 409 and "no valid assignment" error-mapping cases. |
| `internal/api/router.go` | UPDATE | Add `Drawers`/`DrawersAdapter`, `Members`/`MembersAdapter`, `Relationships`/`RelationshipsAdapter`, `Draws`/`DrawsAdapter` to `RouterDeps`; mount all four inside `NewHiddenRouter`'s existing Auth+Authz group, alongside `MountIdentities`. |
| `internal/api/router_test.go` | UPDATE | Cover the four new mounts being present/absent per nil-deps, mirroring the existing Identities/Widgets coverage. |
| `internal/app/drawers.go` | CREATE | `storeDrawers` adapter: `api.DrawerStore` over `*db.Store`. |
| `internal/app/members.go` | CREATE | `storeMembers` adapter: `api.MemberStore` over `*db.Store`, mapping the UNIQUE(drawer_id, phone_number) violation to a 409 sentinel. |
| `internal/app/relationships.go` | CREATE | `storeRelationships` adapter: `api.RelationshipStore` over `*db.Store` — **sorts the phone-number pair before calling `CreateRelationship`** (the sqlc query's documented caller responsibility) and maps the CHECK/UNIQUE violations. |
| `internal/app/draws.go` | CREATE | `storeDraws` adapter: `api.DrawStore` over `*db.Store` **and** `internal/assign` — this is the one file in this plan with real orchestration logic (exclusion/history translation + the relax-and-retry loop), not pure passthrough. |
| `internal/app/draws_test.go` | CREATE | **Deviates from the Widgets precedent (no `app/widgets_test.go` exists)** — justified because `RunDraw`'s translation + retry logic is genuinely new logic, not CRUD passthrough, and needs direct coverage of the relax-by-one behavior and the "never relax exclusions" guarantee. Uses a real temp SQLite `*db.Store` (like the `internal/db` tier), not a fake, since the thing under test *is* the DB↔assign translation. |
| `internal/app/servers.go` | UPDATE | Wire all four new stores/adapters into `api.RouterDeps`, conditioned on `dbFeature.Enabled && kratosFeature.Enabled && ketoFeature.Enabled`. |
| `internal/app/servers_test.go` | UPDATE | Cover the new wiring appearing/not-appearing per feature-enabled state, mirroring existing Widgets/Identities coverage. |

## NOT Building

- Any Keto/OPL changes (`deploy/keto/identities.ts` is untouched — see Decisions table).
- Wishlist CRUD or anything participant-facing (that's Phase 6, protected router, Auth only).
- SMS sending on draw completion (that's Phase 7 — `RunDraw` only persists Assignments and flips
  `draws.status` to `'assigned'`; it never transitions to `'notified'`).
- Any organiser web UI (API-driven only, per the PRD's explicit v1 decision).
- `UpdateDrawStatus`/`DeleteDraw` exposed as their own endpoints — the PRD's Phase 5 scope line says
  "Draw **creation** + run-draw endpoint" only (not full Draw CRUD). `UpdateDrawStatus` is called
  internally by `RunDraw`, never exposed directly.
- Multi-organiser isolation / per-drawer dynamic authorization (explicitly deferred — see Decisions
  table; revisit only if a second real organiser shows up).
- A richer Assignment response (gifter/giftee full name, wishlist, formatted budget) — the run-draw
  response returns the raw `Assignment` DTO (member IDs only); composing the human-readable
  notification message is Phase 7's job, not this one's.

---

## Step-by-Step Tasks

### Task 1: Migration — `history_window_draws`
- **ACTION**: Add `internal/db/migrations/00004_drawer_history_window.sql`.
- **IMPLEMENT**:
  ```sql
  -- +goose Up
  ALTER TABLE drawers ADD COLUMN history_window_draws INTEGER NOT NULL DEFAULT 2;

  -- +goose Down
  ALTER TABLE drawers DROP COLUMN history_window_draws;
  ```
- **MIRROR**: `internal/db/migrations/00003_secret_santa_drawer.sql`'s `-- +goose Up`/`Down` comment
  style (inline comments explaining *why*, not *what*).
- **IMPORTS**: n/a (raw SQL).
- **GOTCHA**: `DROP COLUMN` requires SQLite ≥ 3.35; `modernc.org/sqlite v1.60.0` bundles a modern
  enough SQLite that this works, but if `task db:rollback` is ever run against an older on-disk
  SQLite build, this specific Down step is the one most likely to fail — not a concern for this
  repo's pinned dependency, just worth knowing if debugging a rollback failure later.
- **VALIDATE**: `task db:migrate` (applies cleanly against `dev.db`), `task db:rollback` then
  `task db:migrate` again (round-trips cleanly).

### Task 2: Query edits — `UpdateDrawer` + new history-lookup query
- **ACTION**: Edit `internal/db/queries/drawers.sql` and `internal/db/queries/draws.sql`.
- **IMPLEMENT**:
  ```sql
  -- drawers.sql: replace the existing UpdateDrawer
  -- name: UpdateDrawer :one
  UPDATE drawers SET name = ?, history_window_draws = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;
  ```
  ```sql
  -- draws.sql: add alongside the existing queries
  -- ListRecentCompletedDrawsByDrawer returns the drawer's most recent draws
  -- that actually produced an assignment ('assigned' or 'notified'
  -- status), excluding the draw currently being run, most-recent-first,
  -- capped at limit. RunDraw uses this to build the history window; the
  -- 'draft' filter keeps an unrelated in-progress draw for the same
  -- drawer from ever being treated as history.
  -- name: ListRecentCompletedDrawsByDrawer :many
  SELECT * FROM draws
  WHERE drawer_id = ? AND id != ? AND status != 'draft'
  ORDER BY exchange_date DESC
  LIMIT ?;
  ```
- **MIRROR**: Existing `drawers.sql`/`draws.sql` sqlc annotation style (`-- name: X :one/:many/:exec`).
- **IMPORTS**: n/a.
- **GOTCHA**: sqlc generates a 3-field `Params` struct for the new query in `drawer_id, id, limit`
  order (matching the `?` positions) — name them clearly when writing the app-layer call
  (`DrawerID`, `ExcludeDrawID`, `Limit`) rather than guessing; check the generated
  `ListRecentCompletedDrawsByDrawerParams` struct after `sqlc generate` to confirm exact field
  names before using them.
- **VALIDATE**: `task sqlc:generate && task sqlc:vet` (no errors), `git diff internal/db/sqlc`
  (review the regenerated diff is exactly these two queries' changes).

### Task 3: DB-layer test coverage for the schema/query changes
- **ACTION**: Extend `internal/db/drawers_test.go` and `internal/db/draws_test.go`.
- **IMPLEMENT**: In `drawers_test.go`, assert `UpdateDrawer` with a non-default
  `HistoryWindowDraws` round-trips through `GetDrawer`. In `draws_test.go`, create 3 draws for one
  drawer (two `'assigned'`, one left `'draft'` via `UpdateDrawStatus`), call
  `ListRecentCompletedDrawsByDrawer` with `limit=1` excluding the draft draw's ID, assert exactly
  the most-recent assigned draw comes back (not the draft one, not the older assigned one).
- **MIRROR**: `internal/db/drawers_test.go:12-65`, `internal/db/draws_test.go` (read it in full
  before writing — it already has a `TestDrawQueries_CRUD`-shaped test to extend, not replace).
- **IMPORTS**: none beyond what's already imported in those files.
- **GOTCHA**: `CreateDraw` doesn't take `status` (it defaults to `'draft'` per the migration) —
  use `UpdateDrawStatus` after creation to get a draw into `'assigned'` state for the history test.
- **VALIDATE**: `task test` — both files' tests pass against a real temp SQLite file.

### Task 4: `internal/api/drawers.go` — Drawer DTO, store interface, handlers
- **ACTION**: Create the file.
- **IMPLEMENT**:
  ```go
  var ErrDrawerNotFound = errors.New("api: drawer not found")

  type Drawer struct {
      ID                        int64     `json:"id"`
      Name                      string    `json:"name"`
      OrganiserKratosIdentityID string    `json:"organiser_kratos_identity_id"`
      HistoryWindowDraws        int64     `json:"history_window_draws"`
      CreatedAt                 time.Time `json:"created_at"`
      UpdatedAt                 time.Time `json:"updated_at"`
  }

  type DrawerStore interface {
      CreateDrawer(ctx context.Context, name, organiserIdentityID string) (Drawer, error)
      GetDrawer(ctx context.Context, id int64) (Drawer, error)
      ListDrawersByOrganiser(ctx context.Context, organiserIdentityID string) ([]Drawer, error)
      UpdateDrawer(ctx context.Context, id int64, name string, historyWindowDraws int64) (Drawer, error)
      DeleteDrawer(ctx context.Context, id int64) error
  }

  func MountDrawers(r chi.Router, drawers DrawerStore, adapter Adapter) {
      r.Post("/drawers", adapter.Adapt(createDrawer(drawers)))
      r.Get("/drawers", adapter.Adapt(listDrawers(drawers)))        // lists the CALLER's drawers
      r.Get("/drawers/{id}", adapter.Adapt(getDrawer(drawers)))
      r.Put("/drawers/{id}", adapter.Adapt(updateDrawer(drawers)))
      r.Delete("/drawers/{id}", adapter.Adapt(deleteDrawer(drawers)))
  }
  ```
  `createDrawer` reads `IdentityFromContext` for `organiserIdentityID` (see
  HANDLER_IDENTITY_ACCESS pattern above); `listDrawers` does the same and calls
  `ListDrawersByOrganiser(ctx, identity.Id)` — this is a convenience "my drawers" view, not an
  authorization boundary (any admin can still `GET /drawers/{id}` directly by ID regardless of who
  created it — see Decisions table). Request bodies: `drawerCreateRequest{Name string}`,
  `drawerUpdateRequest{Name string, HistoryWindowDraws int64}` (full PUT replace, matching
  `widgetRequest`'s style — not a partial PATCH).
- **MIRROR**: RESOURCE_FILE_SHAPE and ID_PARAM_PARSING patterns above; `internal/api/widgets.go` end
  to end.
- **IMPORTS**: `context`, `encoding/json`, `errors`, `fmt`, `net/http`, `strconv`, `time`,
  `github.com/go-chi/chi/v5`.
- **GOTCHA**: `createDrawer`/`listDrawers` need `IdentityFromContext` — unlike `widgets.go`, which
  needs nothing from the request context. If `IdentityFromContext` returns `ok == false` (should be
  unreachable since `deps.Auth` always runs first in production), write `unauthorizedProblem()` via
  `WriteProblem` directly and return `nil` — don't try to return the `Problem` as an `error`.
- **VALIDATE**: `go build ./...` compiles; Task 5's tests pass.

### Task 5: `internal/api/drawers_test.go`
- **ACTION**: Create the file.
- **IMPLEMENT**: `fakeDrawerStore` implementing `DrawerStore` (mirror `fakeWidgetStore`'s shape:
  capture `lastX` fields for assertion). Table-driven CRUD status test. A dedicated test that mounts
  the router with a middleware injecting a known identity into context (use `withIdentity` — note
  it's unexported; either add a small test-only context-setting helper in the test file using
  `context.WithValue` directly against the same unexported key is not possible from `api_test`
  package — instead, build the test request through a tiny middleware wrapper in the test that
  calls a context-setting func exported for this purpose, OR mount test handlers behind a fake
  `AuthenticationMiddleware`-shaped function the same way `router_test.go` likely already does for
  Auth-gated routes — **read `internal/api/router_test.go` and `internal/api/middleware_test.go`
  first to see how existing tests inject an authenticated identity into a test request**, and mirror
  that exact mechanism rather than inventing a new one).
- **MIRROR**: `internal/api/widgets_test.go` end to end for the CRUD/status-code shape.
- **IMPORTS**: `bytes`, `context`, `errors`, `log/slog`, `net/http`, `net/http/httptest`, `testing`,
  `time`, `github.com/go-chi/chi/v5`, `github.com/DanielKirkwood/unwrap-gift/internal/api`.
- **GOTCHA**: Don't guess the identity-injection mechanism — this is the one place existing test
  infrastructure for "a request with an authenticated identity already resolved" must already exist
  somewhere (Widgets never needed it since it doesn't touch the identity; Identities' tests don't
  either, since Kratos admin calls don't need the caller's own ID). If no such helper exists yet,
  add the smallest one that does (e.g. an exported `api.WithIdentityForTest` or constructing the
  request through `AuthenticationMiddleware` with a fake `SessionValidator` that always succeeds) —
  check first before adding.
- **VALIDATE**: `task test`.

### Task 6: `internal/api/members.go` — nested-resource Member CRUD
- **ACTION**: Create the file.
- **IMPLEMENT**:
  ```go
  var ErrMemberNotFound = errors.New("api: member not found")
  var ErrMemberPhoneAlreadyExists = errors.New("api: member with this phone number already exists in this drawer")

  type Member struct {
      ID          int64     `json:"id"`
      DrawerID    int64     `json:"drawer_id"`
      FullName    string    `json:"full_name"`
      PhoneNumber string    `json:"phone_number"`
      CreatedAt   time.Time `json:"created_at"`
      UpdatedAt   time.Time `json:"updated_at"`
  }

  type MemberStore interface {
      CreateMember(ctx context.Context, drawerID int64, fullName, phoneNumber string) (Member, error)
      GetMember(ctx context.Context, drawerID, id int64) (Member, error)       // returns ErrMemberNotFound if member.DrawerID != drawerID
      ListMembersByDrawer(ctx context.Context, drawerID int64) ([]Member, error)
      UpdateMember(ctx context.Context, drawerID, id int64, fullName, phoneNumber string) (Member, error)
      DeleteMember(ctx context.Context, drawerID, id int64) error
  }
  ```
  Use the NESTED_RESOURCE_ROUTING pattern for `MountMembers`. The `drawerID`/`id` mismatch check
  (fetch by `id`, compare `DrawerID`) happens in the **app-layer adapter** (`storeMembers`), not the
  handler — the handler just passes both IDs through; this keeps the "is this member actually in
  this drawer" business rule in one place (app) rather than duplicated across every handler.
- **MIRROR**: RESOURCE_FILE_SHAPE + NESTED_RESOURCE_ROUTING patterns above.
- **IMPORTS**: same as Task 4, no `time` dependency beyond what's already needed.
- **GOTCHA**: `full_name` and `phone_number` are both caller-supplied free text in this phase — no
  E.164 format validation is added here (out of scope; the migration's comment already flags E.164
  as an assumption the Kratos phone trait must match in Phase 2/6, not something Phase 5 enforces).
- **VALIDATE**: `go build ./...`; Task 7's tests pass.

### Task 7: `internal/api/members_test.go`
- **ACTION**: Create the file.
- **IMPLEMENT**: Mirror `widgets_test.go`'s shape. Add one dedicated test:
  `TestMountMembers_GetWrongDrawerReturns404` — fake store configured to return
  `ErrMemberNotFound` when the requested `drawerID` doesn't match the member's real drawer, assert
  404 (not 403, not a generic 500) — this is the cross-drawer-enumeration-prevention behavior
  documented in NESTED_RESOURCE_ROUTING.
- **MIRROR**: `internal/api/widgets_test.go`.
- **IMPORTS**: same as Task 5 minus the identity-injection machinery (Member endpoints don't need
  the caller's own identity, only Drawer creation/listing does).
- **VALIDATE**: `task test`.

### Task 8: `internal/api/relationships.go` — global exclusion-pair CRUD
- **ACTION**: Create the file.
- **IMPLEMENT**:
  ```go
  var ErrRelationshipNotFound = errors.New("api: relationship not found")
  var ErrRelationshipSamePhoneNumber = errors.New("api: relationship phone numbers must differ")
  var ErrRelationshipAlreadyExists = errors.New("api: relationship already exists for this pair")

  type Relationship struct {
      ID           int64     `json:"id"`
      PhoneNumberA string    `json:"phone_number_a"`
      PhoneNumberB string    `json:"phone_number_b"`
      CreatedAt    time.Time `json:"created_at"`
  }

  type RelationshipStore interface {
      CreateRelationship(ctx context.Context, phoneNumberA, phoneNumberB string) (Relationship, error)
      ListRelationshipsForPhoneNumber(ctx context.Context, phoneNumber string) ([]Relationship, error)
      DeleteRelationship(ctx context.Context, id int64) error
  }

  func MountRelationships(r chi.Router, relationships RelationshipStore, adapter Adapter) {
      r.Post("/relationships", adapter.Adapt(createRelationship(relationships)))
      r.Get("/relationships", adapter.Adapt(listRelationships(relationships))) // requires ?phone_number= query param
      r.Delete("/relationships/{id}", adapter.Adapt(deleteRelationship(relationships)))
  }
  ```
  `listRelationships` reads `r.URL.Query().Get("phone_number")`; an empty value is a 400, written
  directly via `WriteProblem` inside the handler (the way `EnforceJSON` writes Problems directly
  rather than returning an `error`), not via the `ErrorsMap` path — this is request-shape
  validation, not a store error. No sorting is done at the handler/DTO level — see Task 16, the
  app-layer adapter sorts.
- **MIRROR**: RESOURCE_FILE_SHAPE pattern above. Note this resource has **no** `GetRelationship`
  (not in the PRD's Phase 5 scope, and the underlying `querier.go` has no such query either) and
  **no** `UpdateRelationship` (an exclusion pair is either present or not; "updating" one is
  delete-and-recreate, which the two existing endpoints already support).
- **IMPORTS**: `context`, `encoding/json`, `errors`, `fmt`, `net/http`, `strconv`, `time`,
  `github.com/go-chi/chi/v5`.
- **GOTCHA**: This is the one resource in this plan that is **not** nested under `/drawers/{id}` —
  it's intentionally flat/global, matching the schema (no `drawer_id` column on `relationships`).
- **VALIDATE**: `go build ./...`; Task 9's tests pass.

### Task 9: `internal/api/relationships_test.go`
- **ACTION**: Create the file.
- **IMPLEMENT**: Mirror `widgets_test.go`. Dedicated tests: missing `?phone_number=` → 400; create
  with a fake store returning `ErrRelationshipSamePhoneNumber` (simulating what the real
  `storeRelationships` adapter validates per Task 16, before it ever reaches sqlc) → 400.
- **MIRROR**: `internal/api/widgets_test.go`.
- **IMPORTS**: same as Task 7.
- **VALIDATE**: `task test`.

### Task 10: `internal/api/draws.go` — Draw creation + run-draw
- **ACTION**: Create the file.
- **IMPLEMENT**:
  ```go
  var ErrDrawNotFound = errors.New("api: draw not found")
  var ErrDrawAlreadyRun = errors.New("api: draw has already been run")
  var ErrNoValidAssignment = errors.New("api: no valid assignment exists for this drawer's current members/exclusions/history")
  var ErrTooFewMembers = errors.New("api: drawer has fewer than two members")

  type Draw struct {
      ID           int64     `json:"id"`
      DrawerID     int64     `json:"drawer_id"`
      ExchangeDate time.Time `json:"exchange_date"`
      BudgetAmount int64     `json:"budget_amount"` // minor units (pence)
      Status       string    `json:"status"`
      CreatedAt    time.Time `json:"created_at"`
      UpdatedAt    time.Time `json:"updated_at"`
  }

  type Assignment struct {
      ID             int64     `json:"id"`
      DrawID         int64     `json:"draw_id"`
      GifterMemberID int64     `json:"gifter_member_id"`
      GifteeMemberID int64     `json:"giftee_member_id"`
      CreatedAt      time.Time `json:"created_at"`
  }

  type DrawStore interface {
      CreateDraw(ctx context.Context, drawerID int64, exchangeDate time.Time, budgetAmount int64) (Draw, error)
      GetDraw(ctx context.Context, drawerID, id int64) (Draw, error)
      ListDrawsByDrawer(ctx context.Context, drawerID int64) ([]Draw, error)
      ListAssignmentsByDraw(ctx context.Context, drawerID, drawID int64) ([]Assignment, error)
      RunDraw(ctx context.Context, drawerID, drawID int64) (Draw, []Assignment, error)
  }
  ```
  `MountDraws` nests under `/drawers/{drawerID}/draws` (same NESTED_RESOURCE_ROUTING pattern) plus
  `POST /drawers/{drawerID}/draws/{id}/run`. `runDraw`'s handler calls `drawStore.RunDraw` and
  writes the returned `(Draw, []Assignment)` as `{"data": {"draw": ..., "assignments": [...]}}` —
  define a small unexported `runDrawResponse{Draw Draw; Assignments []Assignment}` struct for this
  composite body rather than two separate top-level fields in `Envelope` (which stays `Data any`,
  unchanged).
- **MIRROR**: RESOURCE_FILE_SHAPE + NESTED_RESOURCE_ROUTING patterns above.
- **IMPORTS**: `context`, `encoding/json`, `errors`, `fmt`, `net/http`, `strconv`, `time`,
  `github.com/go-chi/chi/v5`.
- **GOTCHA**: `ErrNoValidAssignment`/`ErrTooFewMembers` are **this package's own sentinels**,
  declared here and mapped by `servers.go`'s `ErrorsMap` — `internal/api` must never import
  `internal/assign` directly (dependency-direction rule); `internal/app`'s `storeDraws.RunDraw`
  translates `assign.ErrNoValidAssignment`/`assign.ErrTooFewMembers` into these before returning.
- **VALIDATE**: `go build ./...`; Task 11's tests pass.

### Task 11: `internal/api/draws_test.go`
- **ACTION**: Create the file.
- **IMPLEMENT**: Mirror `widgets_test.go`. Dedicated tests: run-draw success returns 200 with both
  `draw` and `assignments` in the envelope; fake store returning `ErrDrawAlreadyRun` → 409; fake
  store returning `ErrNoValidAssignment` → 409 (a conflict between the current drawer state and the
  constraint set, not malformed input — 422 is reserved for `ErrTooFewMembers`, a distinct
  request/state validation failure); fake store returning `ErrTooFewMembers` → 422.
- **MIRROR**: `internal/api/widgets_test.go`.
- **IMPORTS**: same as Task 7.
- **VALIDATE**: `task test`.

### Task 12: Router wiring — `internal/api/router.go`
- **ACTION**: Edit `RouterDeps` and `NewHiddenRouter`.
- **IMPLEMENT**: Add 8 new fields to `RouterDeps` (`Drawers`/`DrawersAdapter`,
  `Members`/`MembersAdapter`, `Relationships`/`RelationshipsAdapter`, `Draws`/`DrawsAdapter`),
  documented with the same "left nil when X feature is disabled" comment style as the existing
  `Widgets`/`Identities` fields. In `NewHiddenRouter`, inside the **existing** `r.Group` that
  already wraps `deps.Auth`/`deps.Authz`/`MountIdentities`, add:
  ```go
  if deps.Drawers != nil {
      MountDrawers(r, deps.Drawers, deps.DrawersAdapter)
  }
  if deps.Members != nil {
      MountMembers(r, deps.Members, deps.MembersAdapter)
  }
  if deps.Relationships != nil {
      MountRelationships(r, deps.Relationships, deps.RelationshipsAdapter)
  }
  if deps.Draws != nil {
      MountDraws(r, deps.Draws, deps.DrawsAdapter)
  }
  ```
- **MIRROR**: `internal/api/router.go:87-97`'s existing `if deps.Identities != nil { MountIdentities(...) }`.
- **IMPORTS**: none new.
- **GOTCHA**: These four **must** land in the same `r.Group` as `MountIdentities` (the one with
  `deps.Auth`/`deps.Authz` applied), not a new group — a new group without re-adding
  `deps.Auth`/`deps.Authz` would accidentally mount these resources unauthenticated.
- **VALIDATE**: `go build ./...`; Task 13's tests pass.

### Task 13: `internal/api/router_test.go`
- **ACTION**: Extend the existing test file.
- **IMPLEMENT**: Add coverage mirroring however `Identities`/`Widgets` presence-vs-nil is already
  tested (read the existing test first — likely asserting a 404 on the new routes when the
  corresponding `RouterDeps` field is nil, and a non-404 when it's set with a fake store).
- **MIRROR**: `internal/api/router_test.go` (read in full first).
- **IMPORTS**: whatever the existing file already imports.
- **VALIDATE**: `task test`.

### Task 14: `internal/app/drawers.go`
- **ACTION**: Create the file.
- **IMPLEMENT**: `storeDrawers{store *db.Store}` implementing `api.DrawerStore`, mirroring
  ADAPTER_CONVERSION exactly — `CreateDrawer` calls `sqlc.CreateDrawerParams{Name, OrganiserKratosIdentityID}`
  (history window is **not** set here; it gets the DB `DEFAULT 2` by omission from the INSERT
  column list, which is already true of the existing `CreateDrawer` query — no query change needed
  for create, only for update, per Task 2). `UpdateDrawer` calls
  `sqlc.UpdateDrawerParams{Name, HistoryWindowDraws, ID}` (field order matches the regenerated
  struct from Task 2 — confirm exact names after `sqlc generate`, don't guess).
- **MIRROR**: `internal/app/widgets.go` end to end.
- **IMPORTS**: `context`, `database/sql`, `errors`, `fmt`,
  `github.com/DanielKirkwood/unwrap-gift/internal/api`, `.../internal/db`, `.../internal/db/sqlc`.
- **GOTCHA**: none beyond the standard `sql.ErrNoRows → api.ErrDrawerNotFound` translation.
- **VALIDATE**: `go build ./...`.

### Task 15: `internal/app/members.go`
- **ACTION**: Create the file.
- **IMPLEMENT**: `storeMembers{store *db.Store}` implementing `api.MemberStore`. `GetMember`,
  `UpdateMember`, `DeleteMember` all take `(ctx, drawerID, id)` — call `sqlc.GetMember(ctx, id)`
  first, check `row.DrawerID != drawerID` → return `api.ErrMemberNotFound` (this is the "is this
  member actually in this drawer" check NESTED_RESOURCE_ROUTING assigns to this layer). `CreateMember`
  maps a UNIQUE-constraint error (via `isUniqueConstraintErr`, SQLITE_CONSTRAINT_DETECTION pattern)
  to `api.ErrMemberPhoneAlreadyExists`.
- **MIRROR**: ADAPTER_CONVERSION + SQLITE_CONSTRAINT_DETECTION patterns above.
- **IMPORTS**: Task 14's imports plus `modernc.org/sqlite` and `modernc.org/sqlite/lib` (aliased
  `sqlite3`, per the pattern above — check `go.mod` already lists `modernc.org/sqlite`; `lib` is a
  subpackage of the same module, no new `go.mod` entry needed).
- **GOTCHA**: `UpdateMember`/`DeleteMember` must still do the drawer-ownership check (fetch-then-
  compare) **before** calling the underlying update/delete query — otherwise a caller could update
  or delete a member in a drawer they didn't intend to touch just by guessing a member ID that
  happens to belong elsewhere.
- **VALIDATE**: `go build ./...`.

### Task 16: `internal/app/relationships.go`
- **ACTION**: Create the file.
- **IMPLEMENT**: `storeRelationships{store *db.Store}` implementing `api.RelationshipStore`.
  `CreateRelationship(ctx, a, b string)`:
  1. if `a == b`, return `api.ErrRelationshipSamePhoneNumber` immediately (before touching the DB).
  2. sort so `a < b` lexicographically (`if a > b { a, b = b, a }`) — the sqlc query's CHECK
     constraint requires this; **this file is responsible for the sort**, not the caller.
  3. call `sqlc.CreateRelationship`; on a UNIQUE violation (`isUniqueConstraintErr`), return
     `api.ErrRelationshipAlreadyExists`.
- **MIRROR**: ADAPTER_CONVERSION + SQLITE_CONSTRAINT_DETECTION patterns above.
- **IMPORTS**: same as Task 15.
- **GOTCHA**: Go string comparison (`<`) on E.164 numbers (`+447700900000` etc.) is lexicographic,
  which matches SQLite's own `TEXT` ordering (`CHECK (phone_number_a < phone_number_b)` uses the
  same collation) — no numeric parsing needed, a plain string `<` is correct and sufficient.
- **VALIDATE**: `go build ./...`.

### Task 17: `internal/app/draws.go` — including `RunDraw` orchestration
- **ACTION**: Create the file.
- **IMPLEMENT**: `storeDraws{store *db.Store}` implementing `api.DrawStore`. `CreateDraw`,
  `GetDraw`/`ListDrawsByDrawer` (with the same drawer-ownership check pattern as Task 15's
  `GetMember`), `ListAssignmentsByDraw` are straightforward `ADAPTER_CONVERSION`. `RunDraw` is the
  real logic:
  ```go
  func (s storeDraws) RunDraw(ctx context.Context, drawerID, drawID int64) (api.Draw, []api.Assignment, error) {
      draw, err := s.store.Queries.GetDraw(ctx, drawID)
      if err != nil { return api.Draw{}, nil, mapDrawErr(err) }
      if draw.DrawerID != drawerID { return api.Draw{}, nil, api.ErrDrawNotFound }
      if draw.Status != drawStatusDraft { return api.Draw{}, nil, api.ErrDrawAlreadyRun }

      drawer, err := s.store.Queries.GetDrawer(ctx, drawerID)
      if err != nil { return api.Draw{}, nil, mapDrawerErr(err) }

      members, err := s.store.Queries.ListMembersByDrawer(ctx, drawerID)
      if err != nil { return api.Draw{}, nil, fmt.Errorf("app: list members for run-draw: %w", err) }

      memberIDs := make([]assign.MemberID, len(members))
      phoneToMemberID := make(map[string]assign.MemberID, len(members))
      for i, m := range members {
          memberIDs[i] = assign.MemberID(m.ID)
          phoneToMemberID[m.PhoneNumber] = assign.MemberID(m.ID)
      }

      exclusions, err := buildExclusions(ctx, s.store.Queries, members, phoneToMemberID)
      if err != nil { return api.Draw{}, nil, err }

      window := drawer.HistoryWindowDraws // confirm this exact field name after sqlc generate
      for {
          history, err := buildHistory(ctx, s.store.Queries, drawerID, drawID, window)
          if err != nil { return api.Draw{}, nil, err }

          result, assignErr := assign.Assign(memberIDs, exclusions, history)
          switch {
          case assignErr == nil:
              return s.persistAssignment(ctx, draw, result)
          case errors.Is(assignErr, assign.ErrTooFewMembers):
              return api.Draw{}, nil, api.ErrTooFewMembers
          case errors.Is(assignErr, assign.ErrNoValidAssignment) && window > 0:
              window--
              continue
          default: // ErrNoValidAssignment at window == 0, or ErrDuplicateMember (shouldn't happen: members come from one query)
              return api.Draw{}, nil, api.ErrNoValidAssignment
          }
      }
  }
  ```
  `buildExclusions` loops `members`, calls `ListRelationshipsForPhoneNumber(m.PhoneNumber)` for
  each, and for every returned row resolves the *other* phone number via `phoneToMemberID` — only
  appending an `assign.ExclusionPair` when **both** sides resolve to a member of *this* drawer
  (people excluded who aren't in this drawer are correctly irrelevant to this specific draw).
  `buildHistory` calls the new `ListRecentCompletedDrawsByDrawer(drawerID, drawID, window)` (skip
  entirely, returning `nil`, when `window == 0`), then `ListAssignmentsByDraw` for each returned
  past draw, converting every row directly to `assign.HistoryPair{Gifter: assign.MemberID(a.GifterMemberID), Giftee: assign.MemberID(a.GifteeMemberID)}`
  — no phone-number translation needed here, since `members` rows (and their `.ID`s) persist across
  years within one drawer; a past member's ID that no longer belongs to the current member set is
  silently ignored by `assign.Assign` itself (see `assign.go`'s own documented tolerance for stale
  IDs), not something this code needs to filter first.
  `persistAssignment` runs inside `s.store.WithTx`: creates one `sqlc.CreateAssignment` row per
  `result` entry, then `UpdateDrawStatus(drawID, drawStatusAssigned)`, returning the updated `Draw`
  plus the full `[]api.Assignment` list.
- **MIRROR**: ADAPTER_CONVERSION pattern for the CRUD methods; `internal/db/store.go`'s `WithTx`
  doc comment for the transactional persist step; `internal/assign/assign.go`'s exact
  signature/types/errors (re-read before writing this file — getting `assign.MemberID`/
  `assign.ExclusionPair`/`assign.HistoryPair`'s field names wrong is the single easiest mistake in
  this whole plan).
- **IMPORTS**: Task 15's imports plus `github.com/DanielKirkwood/unwrap-gift/internal/assign`. This
  file is the **only** one in this plan that imports `internal/assign` — confirms the
  dependency-direction rule (`internal/api` never imports it; `internal/app` is the composition
  root that's allowed to import both `api` and `assign`, same as it's the only one allowed to
  import both `api` and `db`).
- **GOTCHA 1**: The relax-and-retry loop must **never** touch `exclusions` — it's built once, before
  the loop, and reused on every iteration. Only `history`'s window shrinks.
- **GOTCHA 2**: `window` starts at `drawer.HistoryWindowDraws` (the sqlc-generated field name after
  Task 2's regeneration — confirm it's exactly this before writing this code) and the loop must
  terminate at `window == 0` with a definitive `api.ErrNoValidAssignment`, not loop forever.
- **GOTCHA 3**: `draw.Status != "draft"` — define local constants
  (`const drawStatusDraft, drawStatusAssigned = "draft", "assigned"`) matching the migration's CHECK
  constraint values (`'draft'`/`'assigned'`/`'notified'`) rather than repeating bare string literals
  — there's no Go-side enum/const for these anywhere in the codebase yet.
- **VALIDATE**: `go build ./...`; Task 18's tests pass.

### Task 18: `internal/app/draws_test.go`
- **ACTION**: Create the file (deviates from the no-app-test-file Widgets precedent — justified in
  the Files to Change table above).
- **IMPLEMENT**: Using a real temp SQLite `*db.Store` (same `newTestStore(t)` + `db.Migrate` helper
  `internal/db`'s tests use — check whether that helper is exported for reuse or needs a small
  `internal/app`-local equivalent built the same way):
  1. **Relax-by-one test**: seed a drawer with `HistoryWindowDraws = 2`, 2 members, 2 completed past
     draws whose single possible non-repeating assignment is *exactly* what the most recent of the
     2 past draws already used (i.e. with the full 2-draw window, no valid assignment exists; with
     the window relaxed to 1, it becomes solvable). Assert `RunDraw` succeeds and the resulting
     assignment is the one only possible once the window shrank.
  2. **Exclusions never relax**: seed 2 members with a relationship excluding the only possible
     pairing between them, no history at all. Assert `RunDraw` returns `api.ErrNoValidAssignment`
     (not success) — proving the retry loop's relaxation never touches exclusions regardless of how
     many times it loops.
  3. **Already-run guard**: call `RunDraw` twice on the same draw; assert the second call returns
     `api.ErrDrawAlreadyRun`.
- **MIRROR**: `internal/db/drawers_test.go`'s real-SQLite test setup style, adapted to construct a
  `storeDraws` (unexported — this test file must live in package `app`, not `app_test`, to reach
  it; check `internal/app/app_test.go`'s existing package declaration and mirror it).
- **IMPORTS**: `context`, `testing`, `time`, `.../internal/db`, `.../internal/db/sqlc`.
- **GOTCHA**: Constructing a genuinely over-constrained-then-relaxable fixture by hand is the
  fiddliest part of this whole plan — work the member/history numbers out on paper first (2 members
  A/B can only ever assign A→B/B→A as a pair; if the single most recent past draw already used
  A→B, a window of 1 forbids only A→B, leaving B→A valid; a window of 2 that also includes an
  older B→A draws leaves *both* directions forbidden, which is unsolvable) before writing the
  fixture, rather than iterating on the actual test.
- **VALIDATE**: `task test` — `go test -race -cover ./internal/app/...` passes, specifically
  exercising the relax-by-one and never-relax-exclusions behaviors directly.

### Task 19: Wiring — `internal/app/servers.go`
- **ACTION**: Extend `BuildServers`.
- **IMPLEMENT**: Follow ERRORS_MAP_WIRING pattern above, conditioned on
  `dbFeature.Enabled && kratosFeature.Enabled && ketoFeature.Enabled` (read the three features'
  `Enabled` the same way the existing `ketoFeature.Enabled` check at line 75 does). Map every new
  sentinel from Tasks 4/6/8/10 to its Problem:
  `ErrDrawerNotFound/ErrMemberNotFound/ErrRelationshipNotFound/ErrDrawNotFound` → 404;
  `ErrMemberPhoneAlreadyExists/ErrRelationshipAlreadyExists` → 409;
  `ErrRelationshipSamePhoneNumber` → 400;
  `ErrDrawAlreadyRun/ErrNoValidAssignment` → 409;
  `ErrTooFewMembers` → 422.
- **MIRROR**: `internal/app/servers.go:74-103` end to end.
- **IMPORTS**: `net/http` (already imported).
- **GOTCHA**: Place this new block logically near the existing `ketoFeature.Enabled` block (it
  depends on all three of `database`/`kratos`/`keto`, not just one), and keep the comment style
  explaining *which* phase/resource this wiring is for, matching the existing Widgets/Identities
  comments.
- **VALIDATE**: `go build ./...`; Task 20's tests pass.

### Task 20: `internal/app/servers_test.go`
- **ACTION**: Extend the existing test file.
- **IMPLEMENT**: Mirror however the existing tests assert `deps.Widgets`/`deps.Identities` are
  nil/non-nil per feature-enabled combination (read the file first) — add the equivalent for
  `deps.Drawers`/`Members`/`Relationships`/`Draws`, including the case where only 1 or 2 of
  `database`/`kratos`/`keto` are enabled (all four new deps should stay nil unless *all three* are
  enabled).
- **MIRROR**: `internal/app/servers_test.go` (read in full first).
- **IMPORTS**: whatever the existing file already imports.
- **VALIDATE**: `task test`.

---

## Testing Strategy

### Unit Tests

| Test | Input | Expected Output | Edge Case? |
|---|---|---|---|
| `storeDraws.RunDraw` relax-by-one | 2 members, `HistoryWindowDraws=2`, 2 past draws making both directions forbidden at window=2 | Succeeds once window relaxes to 1; assignment matches the only pairing valid at that window | Yes — this is the single most important behavior in this plan |
| `storeDraws.RunDraw` never relaxes exclusions | 2 members, a relationship excluding their only possible pairing, no history | `api.ErrNoValidAssignment`, never succeeds regardless of relaxation | Yes |
| `storeDraws.RunDraw` already-run | A draw with `status='assigned'` | `api.ErrDrawAlreadyRun` | Yes |
| `MountMembers` cross-drawer 404 | `GET /drawers/1/members/5` where member 5 belongs to drawer 2 | 404, `api.ErrMemberNotFound` shape | Yes |
| `createRelationship` same-phone rejection | `{"phone_number_a": "+447700900000", "phone_number_b": "+447700900000"}` | 400 | Yes |
| `storeRelationships.CreateRelationship` sorts the pair | `a="+447700900001", b="+447700900000"` (unsorted) | Underlying sqlc call receives them already sorted; no CHECK-constraint error surfaces | Yes |
| `MountDrawers` create sets organiser from context | authenticated request, no `organiser_kratos_identity_id` in body | Created drawer's `OrganiserKratosIdentityID` equals the caller's identity, not anything client-supplied | Yes — confirms the field can't be spoofed by request body |

### Edge Cases Checklist
- [x] Empty input — e.g. `ListMembersByDrawer` on a drawer with zero members returns `[]`, not an error.
- [x] Maximum size input — not newly relevant here (assign engine's own hang-guard is Phase 4's concern, already tested there).
- [x] Invalid types — malformed JSON body on any `POST`/`PUT` → existing unmapped-500 path (matches `widgetID`'s precedent; not newly solved here).
- [ ] Concurrent access — not applicable; SQLite's single-connection pool (`maxOpenConns = 1`) already serializes writes, and `RunDraw`'s transaction is the only multi-statement write this plan adds.
- [x] Permission denied — covered by the existing, unchanged `deps.Auth`/`deps.Authz` chain; no new test needed since nothing about that chain changes.

---

## Validation Commands

### Static Analysis
```bash
go build ./...
task lint
```
EXPECT: Zero build errors, zero lint findings.

### SQL / Generated Code
```bash
task sqlc:vet
task sqlc:check
```
EXPECT: Queries lint clean against the live schema; `sqlc:check` reports no drift (regenerated code
matches what's committed).

### Unit + DB Integration Tests
```bash
task test
```
EXPECT: All tests pass, including the new `internal/db`, `internal/api`, and `internal/app` test
files from Tasks 3/5/7/9/11/13/18/20.

### Database Validation
```bash
task db:migrate
task db:status
```
EXPECT: Migration `00004` applies cleanly; `db:status` shows it applied.

### Manual Validation
- [ ] `POST /drawers` with a valid session cookie for the seeded admin identity → 201, body's
      `organiser_kratos_identity_id` matches that identity, `history_window_draws` is `2`.
- [ ] `POST /drawers/{id}/members` twice with the same phone number → second call 409.
- [ ] `POST /relationships` with two phone numbers, then `GET /relationships?phone_number=<one of them>` → the pair comes back.
- [ ] `POST /drawers/{id}/draws` then `POST /drawers/{id}/draws/{draw_id}/run` → 200 with `draw.status == "assigned"` and a full `assignments` array covering every member exactly once as both gifter and giftee.
- [ ] Re-running the same draw → 409.
- [ ] A drawer whose members/exclusions make the full history window unsolvable but a relaxed window solvable → still succeeds (manually construct via Task 18's fixture numbers against a real running instance).

---

## Acceptance Criteria
- [ ] All 20 tasks completed.
- [ ] All validation commands pass.
- [ ] Tests written and passing, including the three `RunDraw`-specific behavioral tests in Task 18.
- [ ] No type errors, no lint errors.
- [ ] Matches UX design — N/A (no UX change).

## Completion Checklist
- [ ] Code follows discovered patterns (RESOURCE_FILE_SHAPE, ADAPTER_CONVERSION, etc. above).
- [ ] Error handling matches codebase style (sentinel errors + `ErrorsMap`, RFC 9457 Problems).
- [ ] Logging follows codebase conventions (unmapped errors logged via `Adapter.logUnmapped`,
      nothing added here needs its own logging — no new log statements are introduced by this plan).
- [ ] Tests follow test patterns (fake stores for `api`, real SQLite for `db`/the one `app` deviation).
- [ ] No hardcoded values beyond the agreed `DEFAULT 2` and the draw-status constants.
- [ ] Documentation updated — N/A, no public docs describe these endpoints yet (API-only, v1).
- [ ] No unnecessary scope additions — Draw has no Update/Delete endpoint; no Wishlist, no SMS, no
      Keto changes, no organiser UI.
- [ ] Self-contained — no questions needed during implementation, except the two explicitly flagged
      "check before guessing" spots (Task 5's identity-injection test mechanism, Task 2's generated
      `Params` field names) — both are "verify, don't assume" checks, not open design decisions.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| The relax-by-one `RunDraw` fixture (Task 18) is fiddly to construct correctly by hand | M | Could waste significant time iterating on the fixture before the real test runs | Work the 2-member/2-draw example out on paper first (worked through above in Task 18's GOTCHA) before writing code |
| No existing nested-resource (`/drawers/{id}/members/{id}`) routing example in this codebase | M | First implementation of this pattern could drift from chi idioms or this plan's own NESTED_RESOURCE_ROUTING sketch | Pattern is standard `chi.Route` nesting, confirmed against chi's own docs during this planning session; not experimental |
| Detecting SQLite UNIQUE/CHECK violations via `modernc.org/sqlite`'s `*sqlite.Error`/`Code()` is new to this codebase | L | Could misdetect and fall through to a generic 500 instead of the intended 409/400 | Exact constants (`SQLITE_CONSTRAINT_UNIQUE = 2067`, `SQLITE_CONSTRAINT_CHECK = 275`) confirmed directly against the installed `modernc.org/sqlite@v1.60.0` module source during this planning session, not guessed |
| Reusing the global admin Keto role (Decision 1) instead of per-drawer tuples means any future second organiser can see/edit every drawer, not just their own | L (today: solo project) | M if a second organiser is ever onboarded before this is revisited | Explicitly flagged as a deferred decision, not a silent gap — revisit if the PRD's "Should: multi-drawer support per person" ever means *distinct* organisers, not just Daniel across two drawers |

## Notes

- Phase 1's PRD status shows `in-progress`, but reading the actual repo shows its full scope
  (migrations, queries, sqlc, DB tests for every Phase-1 table) is already done — this plan treats
  Phase 1 as complete and adds exactly one incremental migration (`00004`) on top of it for the
  history-window column that Phase 1 didn't anticipate (it was still an open PRD question at the
  time Phase 1 was built).
- `organiser_kratos_identity_id` is set but not enforced in this phase (see Decisions table) — if a
  future phase needs real multi-organiser isolation, the natural extension point is adding a check
  in each adapter's `GetX`/`UpdateX`/`DeleteX` comparing `IdentityFromContext` to the stored
  `organiser_kratos_identity_id`, entirely within `internal/app`/`internal/api`, with no Keto
  changes required even then — Keto per-drawer tuples are only needed if authorization must be
  delegable to a third party (e.g. Keto admin UI), which isn't a stated requirement anywhere in the
  PRD.
