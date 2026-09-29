# Plan: Secret Santa Drawer — Data Model & Migrations (Phase 1)

## Summary

Add the persistent schema for the Secret Santa Drawer feature: six new SQLite tables (`drawers`, `members`, `relationships`, `draws`, `assignments`, `wishlist_items`), their sqlc-annotated queries, sqlc-generated Go code, and DB-layer integration tests — following the exact layering the `widgets` resource already establishes in this codebase. This phase is schema + data-access only: no `internal/api` or `internal/app` changes.

## User Story

As the organiser, I want the domain data (drawers, members, exclusions, draws, assignments, wishlists) durably and correctly persisted, so that later phases (assignment engine, organiser/participant APIs, notifications) have a schema to build on that already enforces the invariants that matter (no self-gifting, no duplicate pairings per draw, valid drawer references).

## Problem → Solution

Currently `internal/db` only has the `widgets` example table — no domain schema exists for drawers, members, exclusions, draws, assignments, or wishlists. This phase creates that schema plus its query/generated-code/test triad, unblocking Phase 4 (assignment engine), Phase 5 (organiser API), and Phase 6 (participant API), all of which depend on it per the PRD's phase table.

## Metadata

- **Complexity**: Large (1 migration, 6 query files, 6 generated files + 2 updated generated files, 6 test files, 1 existing-file edit ≈ 20 files touched)
- **Source PRD**: `.claude/PRPs/prds/secret-santa-drawer.prd.md`
- **PRD Phase**: Phase 1 — Data model & migrations ([issue #2](https://github.com/DanielKirkwood/unwrap-gift/issues/2))
- **Estimated Files**: ~20 (1 migration, 6 `.sql` query files, 6 `.sql.go` generated files, `models.go` + `querier.go` updates, 6 `_test.go` files, `store.go` edit)

---

## UX Design

N/A — internal change. No `internal/api` or user-facing surface in this phase; this is schema and data-access only.

---

## Mandatory Reading

| Priority | File | Lines | Why |
|---|---|---|---|
| P0 | `ARCHITECTURE.md` | 98–127 ("Adding a new resource") | The authoritative step-by-step this plan follows for layers 1–4 |
| P0 | `internal/db/migrations/00002_widgets.sql` | 1–9 | Exact migration format: `-- +goose Up`/`Down`, `INTEGER PRIMARY KEY AUTOINCREMENT`, `TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP` |
| P0 | `internal/db/queries/widgets.sql` | 1–15 | sqlc query annotation style (`:one`/`:many`/`:exec`, `?` placeholders, `RETURNING *`) |
| P0 | `internal/db/sqlc/widgets.sql.go` | 1–85 | What sqlc generates — do not hand-author files shaped like this; run `task sqlc:generate` |
| P0 | `internal/db/sqlc/models.go` | 1–16 | Generated struct shape/naming (sqlc singularizes the plural table name: `widgets` → `Widget`) |
| P0 | `internal/db/widgets_test.go` | 1–59 | DB integration test pattern: `package db_test`, `newTestStore(t)`, `db.Migrate`, `t.Parallel()`, `errors.Is(err, sql.ErrNoRows)` |
| P0 | `internal/db/store.go` | 120–125 | `dsn()` — the function this plan edits to enable FK enforcement |
| P1 | `internal/db/store_test.go` | 98–112 | `newTestStore` helper every new `_test.go` file reuses (already in-package, no import needed) |
| P1 | `internal/db/migrate.go` | 1–106 | `db.Migrate`/`db.Status`/`db.Rollback` — confirms migrations are embedded via `//go:embed migrations/*.sql`, auto-picked up, no registration step needed |
| P1 | `CONTRIBUTING.md` | 88–105 ("sqlc: regenerate and commit") | The regenerate-and-commit workflow this plan's Task 9 follows; CI fails on drift |
| P2 | `sqlc.yaml` | 1–12 | Confirms `sqlite` engine, `database/sql` package, `emit_json_tags`, `emit_interface` — governs generated code shape |
| P2 | `Taskfile.yml` | 44–102 | Exact commands: `task db:migrate`, `task sqlc:generate`, `task sqlc:vet`, `task sqlc:check`, `task test` |
| P2 | `.golangci.yml` | 46–475 | Lint gotchas below are sourced from here |

## External Documentation

| Topic | Source | Key Takeaway |
|---|---|---|
| modernc.org/sqlite FK pragma | [pkg.go.dev/modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite), community DSN examples | SQLite disables foreign-key enforcement by default; modernc.org/sqlite's `database/sql` driver enables it via a DSN query param — `_foreign_keys=on` (or `_fk=on`, or `_pragma=foreign_keys(1)`) — not automatically. This repo's current `dsn()` doesn't set it. |

KEY_INSIGHT: `ON DELETE CASCADE` and FK referential-integrity checks in this migration are silently no-ops until the connection DSN enables `foreign_keys`.
APPLIES_TO: Task 1 (store.go `dsn()` edit) and every `REFERENCES ... ON DELETE CASCADE` clause in the migration.
GOTCHA: `CHECK` constraints are unaffected by this pragma — they're always enforced regardless. Only `FOREIGN KEY`/`REFERENCES` behavior depends on it. Don't confuse the two when reasoning about why a constraint test does or doesn't fail.

---

## Patterns to Mirror

### MIGRATION_FORMAT
// SOURCE: internal/db/migrations/00002_widgets.sql:1-9
```sql
-- +goose Up
CREATE TABLE widgets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE widgets;
```

### QUERY_ANNOTATION
// SOURCE: internal/db/queries/widgets.sql:1-15
```sql
-- name: CreateWidget :one
INSERT INTO widgets (name) VALUES (?) RETURNING *;

-- name: GetWidget :one
SELECT * FROM widgets WHERE id = ?;

-- name: UpdateWidget :one
UPDATE widgets SET name = ? WHERE id = ? RETURNING *;
```
Multi-param queries generate a `<Query>Params` struct whose field order matches `?` order left-to-right in the SQL, e.g. `UpdateWidgetParams{Name string; ID int64}` for `SET name = ? WHERE id = ?`.

### DB_TEST_STRUCTURE
// SOURCE: internal/db/widgets_test.go:1-59
```go
package db_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

func TestWidgetQueries_CRUD(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	created, err := store.Queries.CreateWidget(t.Context(), "sprocket")
	// ...
}
```
`newTestStore(t)` lives in `store_test.go`, same `db_test` package — every new `_test.go` file gets it for free, no import needed. Every test calls `db.Migrate` itself (idempotent, no shared fixture).

### DSN_PATTERN
// SOURCE: internal/db/store.go:120-125
```go
func dsn(path string) string {
	return fmt.Sprintf("file:%s?_journal=WAL&_timeout=%d", path, busyTimeoutMS)
}
```

---

## Files to Change

| File | Action | Justification |
|---|---|---|
| `internal/db/store.go` | UPDATE | Add `_foreign_keys=on` to `dsn()` so the new tables' `REFERENCES`/`ON DELETE CASCADE` clauses are actually enforced (see GOTCHA above) |
| `internal/db/migrations/00003_secret_santa_drawer.sql` | CREATE | The six new tables + indexes in one migration (they're one cohesive schema addition — see Notes on why this isn't split per-table) |
| `internal/db/queries/drawers.sql` | CREATE | CRUD for `drawers` |
| `internal/db/queries/members.sql` | CREATE | CRUD for `members`, plus cross-drawer lookup by phone number |
| `internal/db/queries/relationships.sql` | CREATE | Create/list/delete for the global exclusion graph |
| `internal/db/queries/draws.sql` | CREATE | CRUD for `draws`, including status transition |
| `internal/db/queries/assignments.sql` | CREATE | Create + list for the gifter→giftee results of a draw |
| `internal/db/queries/wishlist_items.sql` | CREATE | CRUD for a phone number's wishlist items |
| `internal/db/sqlc/*.sql.go` (6 new files) | GENERATE | `task sqlc:generate` output — do not hand-write |
| `internal/db/sqlc/models.go` | GENERATE (updated) | sqlc appends `Drawer`, `Member`, `Relationship`, `Draw`, `Assignment`, `WishlistItem` structs |
| `internal/db/sqlc/querier.go` | GENERATE (updated) | sqlc appends the new methods to the `Querier` interface |
| `internal/db/drawers_test.go` | CREATE | DB integration test, CRUD + FK/cascade behavior |
| `internal/db/members_test.go` | CREATE | DB integration test, CRUD + FK violation + cascade-on-drawer-delete |
| `internal/db/relationships_test.go` | CREATE | DB integration test, CRUD + the `CHECK(phone_number_a < phone_number_b)` ordering constraint |
| `internal/db/draws_test.go` | CREATE | DB integration test, CRUD + status transition + FK to drawer |
| `internal/db/assignments_test.go` | CREATE | DB integration test, CRUD + self-assignment CHECK + duplicate-pairing UNIQUE constraints |
| `internal/db/wishlist_items_test.go` | CREATE | DB integration test, CRUD, no FK (phone-number-keyed, intentionally decoupled — see Notes) |

## NOT Building

- No `internal/api` handlers, DTOs, or router wiring — that's Phase 5 (Organiser API) and Phase 6 (Participant API).
- No `internal/app` adapters wiring `db.Store` to an `api` interface — same reason.
- No assignment-engine logic (the actual randomized backtracking) — that's Phase 4, and it's pure domain logic with no DB dependency; it will *call* the queries this phase creates, but doesn't live here.
- No Kratos/Keto changes — Phase 2 and Phase 5 respectively. This phase stores an `organiser_kratos_identity_id`/phone number as plain `TEXT`, with no FK into any Kratos-side table (Kratos identities aren't in this SQLite database).
- No currency-code column on `draws.budget_amount` — single-currency assumption for v1 (see Notes); add one later if genuinely needed, not speculatively now.

---

## Step-by-Step Tasks

### Task 1: Enable SQLite foreign-key enforcement
- **ACTION**: Edit `dsn()` in `internal/db/store.go`.
- **IMPLEMENT**:
  ```go
  func dsn(path string) string {
  	return fmt.Sprintf("file:%s?_journal=WAL&_timeout=%d&_foreign_keys=on", path, busyTimeoutMS)
  }
  ```
- **MIRROR**: DSN_PATTERN above — same `fmt.Sprintf` shape, just one more query param appended.
- **IMPORTS**: None new.
- **GOTCHA**: Without this, every `REFERENCES`/`ON DELETE CASCADE` clause added below is silently unenforced — inserts with dangling FKs would succeed and cascade deletes wouldn't cascade, with no error at any layer. This is a single-connection pool (`maxOpenConns = 1`, see `store.go:24`), so a DSN-level pragma reliably applies to the one connection every query runs on — no per-connection-from-pool inconsistency to worry about.
- **VALIDATE**: Covered by Task 10's FK-violation test (a `CreateMember` with a nonexistent `drawer_id` must return an error after this change, and must *not* error — silently succeeding — before it, if you want to confirm the gotcha is real).

### Task 2: Write the migration
- **ACTION**: Create `internal/db/migrations/00003_secret_santa_drawer.sql`.
- **IMPLEMENT**:
  ```sql
  -- +goose Up
  CREATE TABLE drawers (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      name TEXT NOT NULL,
      organiser_kratos_identity_id TEXT NOT NULL,
      created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
      updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
  );

  CREATE TABLE members (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      drawer_id INTEGER NOT NULL REFERENCES drawers (id) ON DELETE CASCADE,
      full_name TEXT NOT NULL,
      -- E.164 format (e.g. +447700900000), no spaces/dashes. Must match
      -- whatever Kratos's session phone trait returns for lookups in Phase
      -- 6 to work — see the plan's Notes, point 6.
      phone_number TEXT NOT NULL,
      created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
      updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
      UNIQUE (drawer_id, phone_number)
  );

  CREATE INDEX idx_members_phone_number ON members (phone_number);

  -- Global exclusion graph, reused across every drawer a person belongs to
  -- (not scoped to a single drawer_id — see the PRD's Decisions Log). Keyed
  -- by phone number (E.164, e.g. +447700900000 — see plan Notes, point 6)
  -- rather than member_id/person_id, since phone_number is this system's
  -- only stable, drawer-independent identifier for a person.
  -- Canonically ordered (phone_number_a < phone_number_b) so (a, b) and
  -- (b, a) can't both be inserted as distinct rows; callers MUST sort the
  -- pair before INSERT — the CHECK below only rejects unsorted input, it
  -- does not sort it for you.
  CREATE TABLE relationships (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      phone_number_a TEXT NOT NULL,
      phone_number_b TEXT NOT NULL,
      created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
      CHECK (phone_number_a < phone_number_b),
      UNIQUE (phone_number_a, phone_number_b)
  );

  CREATE INDEX idx_relationships_phone_number_a ON relationships (phone_number_a);
  CREATE INDEX idx_relationships_phone_number_b ON relationships (phone_number_b);

  CREATE TABLE draws (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      drawer_id INTEGER NOT NULL REFERENCES drawers (id) ON DELETE CASCADE,
      exchange_date TIMESTAMP NOT NULL,
      -- Minor currency units (e.g. pence). Single-currency assumption for
      -- v1 — see the plan's Notes for why no currency column exists yet.
      budget_amount INTEGER NOT NULL,
      status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'assigned', 'notified')),
      created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
      updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
  );

  CREATE INDEX idx_draws_drawer_id ON draws (drawer_id);

  CREATE TABLE assignments (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      draw_id INTEGER NOT NULL REFERENCES draws (id) ON DELETE CASCADE,
      gifter_member_id INTEGER NOT NULL REFERENCES members (id) ON DELETE CASCADE,
      giftee_member_id INTEGER NOT NULL REFERENCES members (id) ON DELETE CASCADE,
      created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
      CHECK (gifter_member_id != giftee_member_id),
      UNIQUE (draw_id, gifter_member_id),
      UNIQUE (draw_id, giftee_member_id)
  );

  -- Phone-number-keyed, not member_id/drawer_id-keyed: a wishlist is
  -- intentionally global and persistent across drawers/years (PRD Decisions
  -- Log). No FK to members/drawers is correct here, not an oversight.
  CREATE TABLE wishlist_items (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      phone_number TEXT NOT NULL,
      item_name TEXT NOT NULL,
      size TEXT,
      url TEXT,
      created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
      updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
  );

  CREATE INDEX idx_wishlist_items_phone_number ON wishlist_items (phone_number);

  -- +goose Down
  DROP TABLE wishlist_items;
  DROP TABLE assignments;
  DROP TABLE draws;
  DROP TABLE relationships;
  DROP TABLE members;
  DROP TABLE drawers;
  ```
- **MIRROR**: MIGRATION_FORMAT above (`-- +goose Up`/`Down`, `INTEGER PRIMARY KEY AUTOINCREMENT`, `TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP`).
- **IMPORTS**: N/A (SQL file).
- **GOTCHA**: Down-migration drop order is the reverse of creation, respecting FK dependencies (`assignments` before `draws`/`members`, `members`/`draws` before `drawers`) — goose runs the whole file as one step, but a wrong order would still be a latent footgun for anyone manually replaying it.
- **VALIDATE**: `task db:migrate` (via `task build` dependency, against `dev.db`) applies cleanly; `task db:rollback` reverts cleanly; both exercised automatically by `TestMigrateStatusRollback`-style coverage in Task 10.

### Task 3: `queries/drawers.sql`
- **ACTION**: Create the file.
- **IMPLEMENT**:
  ```sql
  -- name: CreateDrawer :one
  INSERT INTO drawers (name, organiser_kratos_identity_id) VALUES (?, ?) RETURNING *;

  -- name: GetDrawer :one
  SELECT * FROM drawers WHERE id = ?;

  -- name: ListDrawersByOrganiser :many
  SELECT * FROM drawers WHERE organiser_kratos_identity_id = ? ORDER BY id;

  -- name: UpdateDrawer :one
  UPDATE drawers SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

  -- name: DeleteDrawer :exec
  DELETE FROM drawers WHERE id = ?;
  ```
- **MIRROR**: QUERY_ANNOTATION above.
- **VALIDATE**: `task sqlc:vet` passes once the migration (Task 2) exists — sqlc vets queries against the live schema.

### Task 4: `queries/members.sql`
- **ACTION**: Create the file.
- **IMPLEMENT**:
  ```sql
  -- name: CreateMember :one
  INSERT INTO members (drawer_id, full_name, phone_number) VALUES (?, ?, ?) RETURNING *;

  -- name: GetMember :one
  SELECT * FROM members WHERE id = ?;

  -- name: ListMembersByDrawer :many
  SELECT * FROM members WHERE drawer_id = ? ORDER BY id;

  -- name: ListMembersByPhoneNumber :many
  SELECT * FROM members WHERE phone_number = ? ORDER BY drawer_id;

  -- name: UpdateMember :one
  UPDATE members SET full_name = ?, phone_number = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

  -- name: DeleteMember :exec
  DELETE FROM members WHERE id = ?;
  ```
- **MIRROR**: QUERY_ANNOTATION above.
- **GOTCHA**: `ListMembersByPhoneNumber` is what Phase 6 (Participant API) will use to resolve "every drawer this authenticated phone number belongs to" — it's not dead code even though nothing in this phase calls it yet.
- **VALIDATE**: `task sqlc:vet`.

### Task 5: `queries/relationships.sql`
- **ACTION**: Create the file.
- **IMPLEMENT**:
  ```sql
  -- name: CreateRelationship :one
  INSERT INTO relationships (phone_number_a, phone_number_b) VALUES (?, ?) RETURNING *;

  -- name: ListRelationshipsForPhoneNumber :many
  SELECT * FROM relationships WHERE phone_number_a = ? OR phone_number_b = ? ORDER BY id;

  -- name: DeleteRelationship :exec
  DELETE FROM relationships WHERE id = ?;
  ```
- **MIRROR**: QUERY_ANNOTATION above.
- **GOTCHA**: `CreateRelationship` does **not** sort its two arguments — the migration's `CHECK (phone_number_a < phone_number_b)` will reject a call where `phone_number_a >= phone_number_b`. The caller (Phase 5's organiser API adapter) is responsible for sorting the pair (e.g. `if a > b { a, b = b, a }`) before calling this query. This plan's `relationships_test.go` (Task 10) proves the CHECK actually rejects unsorted input, so the constraint is verified even though nothing calls it "for real" yet.
- **VALIDATE**: `task sqlc:vet`.

### Task 6: `queries/draws.sql`
- **ACTION**: Create the file.
- **IMPLEMENT**:
  ```sql
  -- name: CreateDraw :one
  INSERT INTO draws (drawer_id, exchange_date, budget_amount) VALUES (?, ?, ?) RETURNING *;

  -- name: GetDraw :one
  SELECT * FROM draws WHERE id = ?;

  -- name: ListDrawsByDrawer :many
  SELECT * FROM draws WHERE drawer_id = ? ORDER BY exchange_date DESC;

  -- name: UpdateDrawStatus :one
  UPDATE draws SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

  -- name: DeleteDraw :exec
  DELETE FROM draws WHERE id = ?;
  ```
- **MIRROR**: QUERY_ANNOTATION above.
- **GOTCHA**: `ListDrawsByDrawer` orders by `exchange_date DESC` — this is exactly what Phase 4's assignment engine will call (paired with a slice-to-N applied in Go, since sqlc's `:many` doesn't take a dynamic limit without an explicit `LIMIT ?` clause) to fetch "the last N draws." This phase doesn't add a `LIMIT ?` variant query since Phase 4 hasn't specified N's source yet (PRD Open Questions) — fetch all and slice in Go for now.
- **VALIDATE**: `task sqlc:vet`.

### Task 7: `queries/assignments.sql`
- **ACTION**: Create the file.
- **IMPLEMENT**:
  ```sql
  -- name: CreateAssignment :one
  INSERT INTO assignments (draw_id, gifter_member_id, giftee_member_id) VALUES (?, ?, ?) RETURNING *;

  -- name: ListAssignmentsByDraw :many
  SELECT * FROM assignments WHERE draw_id = ? ORDER BY id;
  ```
- **MIRROR**: QUERY_ANNOTATION above.
- **GOTCHA**: No `Update`/`Delete` — assignments are the immutable output of a draw run; re-running a draw should create a new `draws` row, not mutate `assignments` in place (this keeps history for the last-N-draws check honest). If Phase 5 needs to "undo" a draw before notification, that's a `DeleteDraw` (cascades to its assignments), not an assignment-level delete — deliberately not adding one here to avoid inviting partial-assignment mutation.
- **VALIDATE**: `task sqlc:vet`.

### Task 8: `queries/wishlist_items.sql`
- **ACTION**: Create the file.
- **IMPLEMENT**:
  ```sql
  -- name: CreateWishlistItem :one
  INSERT INTO wishlist_items (phone_number, item_name, size, url) VALUES (?, ?, ?, ?) RETURNING *;

  -- name: ListWishlistItemsByPhoneNumber :many
  SELECT * FROM wishlist_items WHERE phone_number = ? ORDER BY id;

  -- name: UpdateWishlistItem :one
  UPDATE wishlist_items SET item_name = ?, size = ?, url = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

  -- name: DeleteWishlistItem :exec
  DELETE FROM wishlist_items WHERE id = ?;
  ```
- **MIRROR**: QUERY_ANNOTATION above.
- **GOTCHA**: `size` and `url` are nullable (`TEXT` with no `NOT NULL`), so sqlc (with `sql_package: database/sql`, per `sqlc.yaml`) generates them as `sql.NullString` on the `WishlistItem` model and as `sql.NullString` params on `CreateWishlistItem`/`UpdateWishlistItem` — not plain `string`. Phase 6's API DTOs will need to convert `sql.NullString` ↔ a plain `*string` or `string`, the same shape of conversion `internal/app`'s widget adapter already does for other type mismatches between `sqlc` and `api` types (see `ARCHITECTURE.md`'s "Interface segregation" section on why that conversion belongs in `internal/app`, not here).
- **VALIDATE**: `task sqlc:vet`.

### Task 9: Generate and commit sqlc code
- **ACTION**: Run the generator, then verify no drift.
- **IMPLEMENT**:
  ```sh
  task sqlc:generate
  task sqlc:vet
  git status internal/db/sqlc   # confirm the 6 new .sql.go files + updated models.go/querier.go
  ```
- **MIRROR**: `CONTRIBUTING.md`'s "sqlc: regenerate and commit" section — generated files are committed, not gitignored.
- **GOTCHA**: Do not hand-edit anything under `internal/db/sqlc/` — CI's `sqlc` job (`task sqlc:check`) fails the build on any diff between a fresh `sqlc generate` and what's committed.
- **VALIDATE**: `task sqlc:check` (regenerates into a clean tree and diffs) exits 0.

### Task 10: DB integration tests
- **ACTION**: Create six `_test.go` files under `internal/db/`, one per table, mirroring `widgets_test.go`.
- **IMPLEMENT** (representative — full CRUD-per-file, plus the constraint-specific cases called out):
  - `drawers_test.go`: `TestDrawerQueries_CRUD` (create/get/list/update/delete, mirroring `TestWidgetQueries_CRUD` exactly).
  - `members_test.go`: `TestMemberQueries_CRUD`, plus `TestCreateMember_FKViolation` (a `CreateMember` with a `drawer_id` that doesn't exist must return a non-nil error now that Task 1's `_foreign_keys=on` is set), plus `TestDeleteDrawer_CascadesMembers` (create a drawer + member, delete the drawer, assert `GetMember` now returns `sql.ErrNoRows`).
  - `relationships_test.go`: `TestRelationshipQueries_CRUD`, plus `TestCreateRelationship_RejectsUnsortedPair` (call `CreateRelationship` with `phone_number_a > phone_number_b` and assert it errors — proving the `CHECK` constraint is live).
  - `draws_test.go`: `TestDrawQueries_CRUD`, including `UpdateDrawStatus` transitioning `draft` → `assigned` → `notified`, plus a case asserting an invalid status string (e.g. `"bogus"`) is rejected by the `CHECK (status IN (...))` constraint.
  - `assignments_test.go`: `TestAssignmentQueries_CRUD` (create two members under one drawer + one draw, create an assignment, list it back), plus `TestCreateAssignment_RejectsSelfAssignment` (`gifter_member_id == giftee_member_id` must error) and `TestCreateAssignment_RejectsDuplicateGifterInSameDraw` (the `UNIQUE (draw_id, gifter_member_id)` constraint).
  - `wishlist_items_test.go`: `TestWishlistItemQueries_CRUD`, including a case with `size`/`url` left as their Go zero value (`sql.NullString{}`) to confirm optional fields round-trip correctly.
- **MIRROR**: DB_TEST_STRUCTURE above — `package db_test`, call `db.Migrate` at the top of every test function (not a shared `TestMain`, matching the existing per-test-function migrate pattern), use `newTestStore(t)` from `store_test.go`, `t.Parallel()` on every test func, `errors.Is(err, sql.ErrNoRows)` for not-found assertions.
- **IMPORTS**: `database/sql`, `errors`, `testing`, `github.com/DanielKirkwood/unwrap-gift/internal/db`, `github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc`.
- **GOTCHA**: `testpackage` linter requires the external `db_test` package (not `package db`) — copy the existing files' package declaration exactly. `paralleltest` requires `t.Parallel()` at the top of every test func including subtests.
- **VALIDATE**: `task test` (runs `go test ./...`, which includes these as part of the default fast+DB-integration tier per `CONTRIBUTING.md`).

### Task 11: Lint and full validation
- **ACTION**: Run the full local validation loop.
- **IMPLEMENT**:
  ```sh
  task fmt
  task lint
  task test
  task sqlc:check
  ```
- **MIRROR**: `CONTRIBUTING.md`'s "Submitting changes" section.
- **GOTCHA**: `.golangci.yml`'s `godot` linter requires every comment (including any new Go doc comments) to end in a period; `gochecknoglobals` forbids new package-level vars (none needed in this phase); `cyclop`/`funlen` complexity budgets are generous (30/100 lines) and shouldn't be an issue for CRUD-shaped code.
- **VALIDATE**: All four commands exit 0.

---

## Testing Strategy

### Unit Tests

This phase has no pure-Go unit tests (no business logic yet — that's Phase 4). All testing here is DB integration testing against real SQLite, per `CONTRIBUTING.md`'s tier definitions.

| Test | Input | Expected Output | Edge Case? |
|---|---|---|---|
| `TestDrawerQueries_CRUD` | name + organiser id | Full CRUD round-trip | No |
| `TestMemberQueries_CRUD` | drawer_id + full_name + phone_number | Full CRUD round-trip | No |
| `TestCreateMember_FKViolation` | nonexistent drawer_id | Non-nil error | Yes — proves Task 1's pragma is active |
| `TestDeleteDrawer_CascadesMembers` | drawer with 1 member, delete drawer | Member row gone (`sql.ErrNoRows` on `GetMember`) | Yes — proves `ON DELETE CASCADE` is active |
| `TestRelationshipQueries_CRUD` | sorted phone pair | Full CRUD round-trip | No |
| `TestCreateRelationship_RejectsUnsortedPair` | `phone_number_a > phone_number_b` | Non-nil error | Yes — proves the `CHECK` ordering constraint |
| `TestDrawQueries_CRUD` | drawer_id + exchange_date + budget_amount | Full CRUD round-trip incl. status transitions | No |
| `TestCreateAssignment_RejectsSelfAssignment` | gifter == giftee | Non-nil error | Yes |
| `TestCreateAssignment_RejectsDuplicateGifterInSameDraw` | two assignments, same draw_id + gifter_member_id | Second insert errors | Yes |
| `TestWishlistItemQueries_CRUD` | phone_number + item fields, incl. nil size/url | Full CRUD round-trip, `sql.NullString{}` handled | Yes — nullable-field zero value |

### Edge Cases Checklist
- [x] Empty input — N/A at this layer (no validation logic yet; that's Phase 5/6's job at the API boundary)
- [x] Maximum size input — not relevant to a family/friends-scale dataset; not tested
- [x] Invalid types — covered by `CHECK` constraint tests (status enum, self-assignment, relationship ordering)
- [ ] Concurrent access — not tested in this phase; `maxOpenConns = 1` already serializes all writes at the connection-pool level (existing `store.go` behavior, unchanged by this plan)
- [x] FK/cascade behavior — covered by `TestCreateMember_FKViolation` and `TestDeleteDrawer_CascadesMembers`
- N/A Permission denied — no auth/authz in this phase (Phase 5/6)

---

## Validation Commands

### Static Analysis
```bash
task lint
```
EXPECT: Zero lint errors.

### Unit Tests
```bash
task test
```
EXPECT: All tests pass, including the 6 new `_test.go` files under `internal/db/`.

### Full Test Suite
```bash
task test
```
EXPECT: No regressions in existing tests (`widgets_test.go`, `store_test.go`, everything else).

### Database Validation
```bash
task db:reset   # drops dev.db, re-runs migrations from scratch
task db:status  # confirm 00003_secret_santa_drawer.sql shows as applied
task sqlc:check # confirms committed generated code matches a fresh `sqlc generate`
```
EXPECT: Migration applies cleanly from zero; `sqlc:check` exits 0 (no drift).

### Browser Validation
N/A — no HTTP surface in this phase.

### Manual Validation
- [ ] `task db:migrate` then `task db:rollback` then `task db:migrate` again — confirms the Down migration is syntactically and referentially valid, not just the Up.
- [ ] Open `dev.db` with a SQLite client and confirm all 6 tables + their indexes exist with the expected columns.

---

## Acceptance Criteria
- [ ] All 11 tasks completed
- [ ] All validation commands pass
- [ ] Tests written and passing (10+ new test functions across 6 files)
- [ ] No type errors
- [ ] No lint errors
- [ ] N/A — no UX to match (internal change)

## Completion Checklist
- [ ] Code follows discovered patterns (migration format, query annotation style, test structure)
- [ ] Error handling matches codebase style (plain error returns from `database/sql`, no custom wrapping needed at this layer — `internal/app`'s adapters handle that in later phases)
- [ ] Logging follows codebase conventions — N/A, no logging at this layer (widgets' DB layer doesn't log either)
- [ ] Tests follow test patterns (`db_test` package, `newTestStore`, `t.Parallel()`)
- [ ] No hardcoded values beyond what CRUD tests need
- [ ] Documentation updated — the migration's inline comments explain the non-obvious decisions (phone-number-keyed relationships/wishlists, canonical ordering); no separate doc file needed
- [ ] No unnecessary scope additions — no `internal/api`/`internal/app` touched
- [ ] Self-contained — no questions needed during implementation (three data-model calls made below are documented, not left open)

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| FK pragma change (Task 1) affects existing `widgets` tests if they ever relied on lax FK behavior | L | L | `widgets` has no FK columns at all — unaffected. Confirmed by reading `00002_widgets.sql`. |
| `relationships`' phone-number-keyed design (no `person`/`member_id` FK) turns out to be wrong once Phase 5/6 are built | M | M | Documented explicitly as a plan-time architectural call (see Notes) with reasoning; cheap to revisit since it's an additive migration, not a rename, if it needs to change |
| sqlc's nullable-column type mapping (`sql.NullString` for `size`/`url`) surprises whoever writes Phase 6's DTOs | L | L | Called out explicitly as a GOTCHA in Task 8 and the Files table |

## Notes

**Architectural calls made in this phase that the PRD left open** (flagging per the plan's "no questions needed during implementation" bar — these are decisions, not gaps, but worth a deliberate look before Phase 5/6 build on them):

1. **No separate `people`/`person` table.** The PRD's "relationship graph reused across drawers" and "wishlist maintained by the user" requirements both need a drawer-independent identity for a human. Rather than introducing a normalized `people` table (with `members` and `wishlist_items` both foreign-keying into it), this plan uses **phone number as the natural global key** directly on `relationships` and `wishlist_items`, while `members` stays drawer-scoped (holding its own `full_name`/`phone_number` copy per drawer). This avoids an extra join/entity for a dataset this small, at the cost of `full_name` being denormalized per drawer if the same person is in two drawers. If that denormalization becomes a real problem (e.g. name edits need to propagate), introducing a `people` table later is an additive migration, not a breaking one.
2. **No `kratos_identity_id` column anywhere in this schema.** The link between a `members`/`wishlist_items` row and an authenticated session is resolved at the API layer (Phase 5/6) by matching the Kratos session's phone-number trait against `members.phone_number`/`wishlist_items.phone_number` — Kratos is the source of truth for identity, this database doesn't need to cache identity IDs to make that join work.
3. **`draws.budget_amount` has no currency column.** Single-currency assumption for v1 (matches the PRD's known user base — two UK-based groups). Documented in the migration's inline comment so it's a visible, deliberate simplification rather than an oversight if it's revisited.
4. **`assignments` has no delete/update queries.** Assignments are treated as an immutable record of a completed draw; "redo the draw" means a new `draws` row (and its own `assignments`), not mutating an existing draw's results — this keeps the last-N-draws history (Phase 4) honest.
5. **`kratos_identity_id` stays deferred, not added in this phase.** There's no real FK target for it yet — Kratos identities live in Kratos's own store, not this SQLite database, and Phase 2 (auth) doesn't exist yet — so the column would sit empty and untested through this phase. Add it later (additive migration) once Phase 6 actually needs to cache "this phone number claimed this Kratos identity at first login." Revisit once Phase 2/6 land, not before.
6. **`phone_number` columns must store E.164 format** (`+<countrycode><number>`, no spaces/dashes, e.g. `+447700900000`), consistently across `members.phone_number`, `relationships.phone_number_a`/`_b`, and `wishlist_items.phone_number`. This matters now, not later: the whole cross-drawer relationship graph and wishlist lookup design (Decision 1 above) depends on plain string equality between these columns and whatever Kratos's session phone trait returns — if organiser-entered numbers and Kratos-registered numbers use different formats (`07700 900000` vs `+447700900000`), the match silently fails and a real person's membership/wishlist becomes invisible to them with no error anywhere. This phase doesn't add a `CHECK` constraint enforcing the format (SQLite's regex support is limited and validation belongs at the API input boundary, not the DB layer per this codebase's existing conventions — `widgets` has no such validation either), but every new migration column comment and DB test's sample data should use E.164-formatted numbers so the convention is visible and consistent from the first commit. Phase 5/6's API-layer input handling is responsible for normalizing organiser/participant-supplied numbers to E.164 before they ever reach these queries.

These six points are worth a quick confirm-or-correct pass before Phase 5/6 implementation starts, since they shape the API surface those phases will build.

## Process

- **Branch**: create a dedicated branch for this work off `main` before starting implementation, named after the tracking issue, e.g. `2-data-model-and-migrations` (issue [#2](https://github.com/DanielKirkwood/unwrap-gift/issues/2)) — don't implement directly on `main`.
- **Commits**: follow [Conventional Commits](https://www.conventionalcommits.org/) for every commit in this phase (e.g. `feat(db): add secret santa drawer schema`, `test(db): add drawer/member CRUD integration tests`), consistent scoping by the layer touched (`db`, or no scope for cross-cutting changes like the `store.go` DSN edit). This repo's existing single `initial commit` doesn't establish a prior convention either way, so Conventional Commits is an explicit choice for this feature's history, not a continuation of an existing pattern.
