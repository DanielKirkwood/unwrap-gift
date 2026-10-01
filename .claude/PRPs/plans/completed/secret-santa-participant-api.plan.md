# Plan: Participant API (Wishlist CRUD)

## Summary
Add a new, authenticated (no authz), DB-backed resource — wishlist items — mounted on the protected router. A logged-in participant can create, list, update, and delete their own wishlist items; the owning phone number is derived from their Kratos session identity's `phone` trait, never from the request body, and ownership is enforced server-side so no participant can read or mutate another participant's items.

## User Story
As a drawer member (participant), I want to log in via my SMS code and manage my own wishlist (item name, optional size, optional URL), so that my Secret Santa gifter knows what I want — without being able to see or change anyone else's wishlist.

## Problem → Solution
Today `wishlist_items` has a full DB migration, sqlc queries, and DB-layer tests (Phase 1), but no HTTP surface at all — there's no way for a participant to actually read or write their own wishlist. This plan adds the `internal/api` + `internal/app` layers (DTO, interface, handlers, adapter, wiring) that the `Widgets` resource already demonstrates, with one new ingredient Widgets didn't need: deriving the resource's owner from the caller's **Kratos identity trait** (phone), not a URL param, and enforcing that ownership server-side.

## Metadata
- **Complexity**: Medium
- **Source PRD**: `.claude/PRPs/prds/secret-santa-drawer.prd.md`
- **PRD Phase**: Phase 6 — Participant API
- **Estimated Files**: 8 (2 new, 6 updated)

---

## UX Design

N/A — internal/API change only. (Phase 8, "Wishlist web UI", is the user-facing surface that calls these endpoints; out of scope here.)

### Interaction Changes
| Touchpoint | Before | After | Notes |
|---|---|---|---|
| Participant's wishlist | No API exists; `wishlist_items` rows can only be inserted directly against the DB | `POST/GET/PUT/DELETE /wishlist-items` on the protected router, scoped to the caller's own phone number | Auth only (`deps.Auth`), no `deps.Authz` — same authorization shape as Widgets |

---

## Decisions Confirmed With User (2026-10-01)

These were genuinely open — not derivable from the PRD or existing code — and were confirmed before writing this plan:

1. **Ownership enforcement for Update/Delete**: add a new `GetWishlistItem(ctx, id)` sqlc query; the app-layer adapter fetches the row first and compares its `phone_number` to the caller's phone trait, returning `api.ErrWishlistItemNotFound` on any mismatch (same error on "doesn't exist" and "belongs to someone else" — deliberately ambiguous, mirroring `storeMembers`' `GetMember`-then-`DrawerID`-check pattern in `internal/app/members.go`).
2. **Endpoint scope**: Create, List (own items only), Update, Delete — **no** single-item `GET /wishlist-items/{id}`. Mirrors `Relationships` (the one existing resource with the same flat, phone-scoped, no-parent-FK shape), and nothing in the PRD's user flow needs to fetch one item standalone.
3. **Phone-format mismatch risk**: `identity.schema.json`'s `phone` trait has no E.164 pattern/validation, but `wishlist_items.phone_number` (like `members.phone_number`) assumes E.164. **No new validation is being added in this phase** — trusted as-is for v1 since Daniel is the only registrant and controls the format at registration/member-add time. Documented below as a Risk, not coded around.

---

## Mandatory Reading

| Priority | File | Lines | Why |
|---|---|---|---|
| P0 | `internal/api/widgets.go` | 1-155 | The exact template: DTO, `Store` interface, sentinel error, `Mount*`, handlers, `*ID` URL-param helper. Copy this shape. |
| P0 | `internal/app/widgets.go` | 1-45 | The app-layer adapter template: `store*` struct wrapping `*db.Store`, `toAPI*` converter, `map*Err` helper. |
| P0 | `internal/api/drawers.go` | 65-107 | **The only existing precedent for deriving an owner from the caller's identity instead of a URL param or request body** — `createDrawer`/`listDrawers` call `IdentityFromContext` and use `identity.Id`. This plan needs the same shape, but reading `identity.Traits["phone"]` instead of `identity.Id`. |
| P0 | `internal/api/auth.go` | 1-77 | `IdentityFromContext`, `unauthorizedProblem()`, and the `*kratos.Identity` shape (`Traits interface{}`) every new handler needs. |
| P0 | `internal/app/members.go` | 1-143 | The exact "fetch-then-compare ownership field, same error either way" pattern (`GetMember` → compare `DrawerID`) this plan's `UpdateWishlistItem`/`DeleteWishlistItem` must mirror, substituting `PhoneNumber` for `DrawerID`. |
| P1 | `internal/api/router.go` | 1-135 | `RouterDeps` fields + `NewProtectedRouter`'s `Auth`-only group Widgets mounts into — the new resource mounts into the same group. |
| P1 | `internal/app/servers.go` | 1-120, 142-242 | `BuildServers`' feature-gated wiring; `wireOrganiserAPI` as the pattern for a dedicated wiring function when a condition needs an explanatory comment. |
| P1 | `internal/db/queries/wishlist_items.sql` | 1-13 | Existing queries (`Create`, `ListByPhoneNumber`, `Update`, `Delete`) — this plan adds one (`GetWishlistItem`) in the same file. |
| P1 | `internal/db/sqlc/wishlist_items.sql.go` | all | Generated code for the above — shows exact Go param/row types (`CreateWishlistItemParams`, `WishlistItem{..., Size sql.NullString, Url sql.NullString}`). Regenerate, don't hand-edit. |
| P1 | `internal/db/migrations/00003_secret_santa_drawer.sql` | 68-81 | The `wishlist_items` table + its comment explaining why it's phone-number-keyed with no FK — confirms wishlist is global/persistent, not per-drawer/per-member. |
| P2 | `internal/api/drawers_test.go` | 64-99 | `fakeAuthedValidator` + `mountTestDrawers`/`newAuthedDrawerRequest` — the existing pattern for testing a handler that reads `IdentityFromContext`, behind a *real* `AuthenticationMiddleware`. This plan's test needs the same thing, extended to set `Traits` (not just `Id`). |
| P2 | `internal/api/members_test.go` | 1-80 | Fake-store + `httptest` pattern for a nested/parameterized resource — structurally close to what the new `fakeWishlistItemStore` needs. |
| P2 | `internal/app/draws_test.go` | 1-30 | Precedent for `package app` (not `app_test`) when a test needs direct access to an unexported `store*` type to test non-passthrough logic against a real temp SQLite `*db.Store` — reuse its `newAppTestStore(t)` helper. This plan's ownership-check test belongs here, for the same reason `draws_test.go` does (not pure CRUD passthrough). |
| P2 | `internal/app/servers_test.go` | 150-171, 234-320 | `TestBuildServers_DatabaseDisabled_WidgetsNotMounted` and `TestBuildServers_OrganiserAPI_RequiresAllThreeFeatures` — the exact style for a "feature combination gates mounting" test, and `TestBuildServers_DatabaseAndKratosEnabled_ProtectedRouterRequiresAuth` for the "no cookie → 401, no live Kratos needed" trick. |
| P2 | `ARCHITECTURE.md` | 98-127 | "Adding a new resource" — Widgets vs. Identities as the two templates; this plan is explicitly the Widgets template plus the identity-derived-owner wrinkle. |

## External Documentation

No external research needed — feature uses established internal patterns (Widgets resource shape, Members ownership-check shape, Drawers identity-derivation shape). The only "new" ingredient — reading a specific trait out of `kratos.Identity.Traits` — is a plain Go type assertion on a JSON-decoded `interface{}`, not a library/API question.

---

## Patterns to Mirror

### NAMING_CONVENTION
// SOURCE: internal/api/widgets.go:15-46
```go
var ErrWidgetNotFound = errors.New("api: widget not found")

type Widget struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type WidgetStore interface {
	CreateWidget(ctx context.Context, name string) (Widget, error)
	GetWidget(ctx context.Context, id int64) (Widget, error)
	ListWidgets(ctx context.Context) ([]Widget, error)
	UpdateWidget(ctx context.Context, id int64, name string) (Widget, error)
	DeleteWidget(ctx context.Context, id int64) error
}
```
Apply the same shape: `ErrWishlistItemNotFound`, `WishlistItem` DTO, `WishlistItemStore` interface — but every method takes `phoneNumber string` as its scoping parameter instead of a URL-derived id (there is no parent resource to nest under).

### IDENTITY_DERIVED_OWNER (the one deviation from Widgets)
// SOURCE: internal/api/drawers.go:65-87
```go
func createDrawer(drawers DrawerStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			return WriteProblem(w, unauthorizedProblem())
		}

		var req drawerCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return fmt.Errorf("api: decode create drawer request: %w", err)
		}

		drawer, err := drawers.CreateDrawer(r.Context(), req.Name, identity.Id)
		// ...
	}
}
```
Mirror exactly, but resolve `phoneNumber` from `identity.Traits` instead of using `identity.Id`:
```go
// phoneTraitFromIdentity extracts the "phone" trait from identity.Traits.
// kratos-client-go declares Traits as interface{} (see model_identity.go);
// after JSON decoding it is always map[string]any for this schema. ok is
// false only if the trait is missing or not a string, which shouldn't
// happen given identity.schema.json requires "phone" — callers treat that
// as an invariant violation (mapped to a generic 500), not a 4xx.
func phoneTraitFromIdentity(identity *kratos.Identity) (string, bool) {
	traits, ok := identity.Traits.(map[string]any)
	if !ok {
		return "", false
	}
	phone, ok := traits["phone"].(string)
	return phone, ok
}
```

### ERROR_HANDLING (ownership check, same error either way)
// SOURCE: internal/app/members.go:46-56, 72-98
```go
func (s storeMembers) GetMember(ctx context.Context, drawerID, id int64) (api.Member, error) {
	member, err := s.store.Queries.GetMember(ctx, id)
	if err != nil {
		return api.Member{}, mapMemberErr(err)
	}
	if member.DrawerID != drawerID {
		return api.Member{}, api.ErrMemberNotFound
	}
	return toAPIMember(member), nil
}
```
Mirror exactly for `UpdateWishlistItem`/`DeleteWishlistItem`: fetch via the new `GetWishlistItem` query, compare `existing.PhoneNumber != phoneNumber`, return `api.ErrWishlistItemNotFound` on mismatch — **not** a different error/status than "truly doesn't exist".

### LOGGING_PATTERN
No resource-specific logging exists anywhere in `internal/api`/`internal/app` (Widgets, Members, Drawers, Relationships, Draws all log nothing beyond what `RequestLogger`/`Adapter.logUnmapped` already do automatically). Add none here either — don't introduce a new pattern.

### ADAPTER_WIRING_PATTERN
// SOURCE: internal/app/servers.go:89-109, 142-148
```go
dbFeature, _ := a.Registry.Feature("database")
if dbFeature.Enabled {
	deps.Widgets = storeWidgets{store: a.Store}
	deps.WidgetsAdapter = api.Adapter{
		Logger: a.Logger,
		ErrorsMap: api.ErrorsMap{
			{Match: api.ErrWidgetNotFound, Problem: api.Problem{Status: http.StatusNotFound, Title: titleNotFound, Detail: "widget not found"}},
		},
	}
}
```
This plan's wiring needs **both** `dbFeature` and `kratosFeature` enabled (identity resolution requires Kratos) but **not** `ketoFeature` (no authz) — a condition combination that doesn't exist yet. Add a small dedicated function, same style as `wireOrganiserAPI`:
```go
// wireWishlistItems sets deps.WishlistItems/WishlistItemsAdapter — an
// authenticated (not authorized) resource on the protected router, same
// shape as Widgets, except scoped by the caller's own phone trait instead
// of being global. Needs kratos (to resolve identity.Traits["phone"]) but
// not keto (no deps.Authz group involved). Called only when database and
// kratos are both enabled.
func wireWishlistItems(deps *api.RouterDeps, a *App) {
	deps.WishlistItems = storeWishlistItems{store: a.Store}
	deps.WishlistItemsAdapter = api.Adapter{
		Logger: a.Logger,
		ErrorsMap: api.ErrorsMap{
			{Match: api.ErrWishlistItemNotFound, Problem: api.Problem{Status: http.StatusNotFound, Title: titleNotFound, Detail: "wishlist item not found"}},
		},
	}
}
```
Called from `BuildServers` as:
```go
if dbFeature.Enabled && kratosFeature.Enabled {
	wireWishlistItems(&deps, a)
}
```

### REPOSITORY_PATTERN (query file addition)
// SOURCE: internal/db/queries/members.sql:1-7 (Create/Get ordering convention)
```sql
-- name: CreateMember :one
INSERT INTO members (drawer_id, full_name, phone_number) VALUES (?, ?, ?) RETURNING *;

-- name: GetMember :one
SELECT * FROM members WHERE id = ?;
```
Add to `internal/db/queries/wishlist_items.sql`, in the same Create→Get→List→Update→Delete order:
```sql
-- name: GetWishlistItem :one
SELECT * FROM wishlist_items WHERE id = ?;
```
Insert it between the existing `CreateWishlistItem` and `ListWishlistItemsByPhoneNumber` entries.

### SERVICE_PATTERN (app-layer adapter)
// SOURCE: internal/app/widgets.go full file + internal/app/members.go:136-142
```go
type storeWidgets struct {
	store *db.Store
}
// ... CreateWidget/GetWidget/ListWidgets/UpdateWidget/DeleteWidget, each:
//   1. calls s.store.Queries.X(ctx, params)
//   2. wraps a non-ErrNoRows error with fmt.Errorf("app: ...: %w", err)
//   3. converts sqlc.Widget -> api.Widget via toAPIWidget

func isUniqueConstraintErr(err error) bool { /* already exists, reusable if needed */ }
```
`wishlist_items` has no unique constraint, so `isUniqueConstraintErr` isn't needed here — don't add a conflict-error sentinel.

Nullable-column conversion has **no existing precedent** anywhere in `internal/app` (no other resource has an optional string column) — this plan introduces it. Keep it local to the new file, don't over-engineer a shared helper:
```go
func toNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func fromNullString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}
```

### TEST_STRUCTURE
// SOURCE: internal/api/drawers_test.go:64-99 (identity-in-context) + internal/app/draws_test.go:1-30 (whitebox app test)
Three test files, three different styles — use the one matching what's actually being proven:
1. `internal/db/wishlist_items_test.go` (extend existing) — real SQLite, add `GetWishlistItem` coverage (found + `sql.ErrNoRows` on a missing id), following the existing `TestWishlistItemQueries_CRUD` function's style exactly.
2. `internal/api/wishlist_items_test.go` (new, `package api_test`) — fake `WishlistItemStore` + `httptest`, behind a real `AuthenticationMiddleware` backed by an identity-with-traits fake validator (extend `drawers_test.go`'s `fakeAuthedValidator` pattern to also set `Traits`). Proves: handlers extract the phone trait correctly, decode/encode JSON correctly, 401 with no session, 500-class error if a trait is somehow missing.
3. `internal/app/wishlist_items_test.go` (new, **`package app`**, not `app_test` — same `//nolint:testpackage` justification as `draws_test.go`) — real temp SQLite via `newAppTestStore(t)` (already defined in `draws_test.go`, same package). Proves the actual ownership enforcement: phone A can create/list/update/delete their own items; phone A attempting to update/delete phone B's item gets `api.ErrWishlistItemNotFound`. This is the test that actually proves the PRD's Phase 6 success signal ("cannot see or edit anyone else's").
4. `internal/app/servers_test.go` (extend) — add a `TestBuildServers_WishlistItems_RequiresDatabaseAndKratos` table test mirroring `TestBuildServers_OrganiserAPI_RequiresAllThreeFeatures`'s shape (mounted iff both features enabled), using the existing "no cookie → 401 means mounted, 404 means not mounted" trick from `TestBuildServers_DatabaseAndKratosEnabled_ProtectedRouterRequiresAuth` (no live Kratos instance needed for any of this).

---

## Files to Change

| File | Action | Justification |
|---|---|---|
| `internal/db/queries/wishlist_items.sql` | UPDATE | Add `GetWishlistItem :one` query for the ownership-check fetch |
| `internal/db/sqlc/wishlist_items.sql.go` | UPDATE (regenerate via `task sqlc:generate`) | Generated code for the new query — never hand-edit |
| `internal/db/wishlist_items_test.go` | UPDATE | Add `GetWishlistItem` coverage (found + not-found) to the existing CRUD test |
| `internal/api/wishlist_items.go` | CREATE | `WishlistItem` DTO, `WishlistItemStore` interface, `ErrWishlistItemNotFound`, `phoneTraitFromIdentity`, `MountWishlistItems`, 4 handlers |
| `internal/api/wishlist_items_test.go` | CREATE | Fake-store + httptest unit tests for the handlers, incl. identity/trait edge cases |
| `internal/api/router.go` | UPDATE | Add `WishlistItems`/`WishlistItemsAdapter` to `RouterDeps`; mount in `NewProtectedRouter`'s existing `Auth`-only group |
| `internal/app/wishlist_items.go` | CREATE | `storeWishlistItems` adapter implementing `api.WishlistItemStore`, with the fetch-then-compare ownership check |
| `internal/app/wishlist_items_test.go` | CREATE | Whitebox (`package app`) test proving ownership enforcement against a real SQLite store |
| `internal/app/servers.go` | UPDATE | Add `wireWishlistItems` + call it when `dbFeature.Enabled && kratosFeature.Enabled` |
| `internal/app/servers_test.go` | UPDATE | Add a feature-gating test for the new mount, mirroring the Organiser API's combination test |

## NOT Building

- Single-item `GET /wishlist-items/{id}` — explicitly deferred (see Decisions Confirmed With User, #2).
- Any new validation/normalization of the `phone` trait's format (E.164 or otherwise) — explicitly deferred (see Decisions Confirmed With User, #3). Documented as a Risk instead.
- Any change to `identity.schema.json`, Kratos courier config, or the SMS-code login flow itself — Phase 2's concern, already complete, out of scope here.
- Any change to `members`/`drawers`/`relationships` tables or endpoints — Phase 6 only touches `wishlist_items`.
- Linking `wishlist_items` to `members` via a foreign key, or scoping a wishlist per-drawer/per-draw — the schema is deliberately global/phone-keyed (Phase 1 decision, confirmed by the migration's own comment); this plan does not revisit that.
- The notification pipeline reading a giftee's wishlist to compose an SMS — that's Phase 7, which depends on this phase's `ListWishlistItemsByPhoneNumber` query existing (it already does) but adds no new API surface itself.
- The wishlist web UI — Phase 8.

---

## Step-by-Step Tasks

### Task 1: Add `GetWishlistItem` query
- **ACTION**: Add a new sqlc query to fetch one wishlist item by id.
- **IMPLEMENT**: In `internal/db/queries/wishlist_items.sql`, insert between `CreateWishlistItem` and `ListWishlistItemsByPhoneNumber`:
  ```sql
  -- name: GetWishlistItem :one
  SELECT * FROM wishlist_items WHERE id = ?;
  ```
- **MIRROR**: REPOSITORY_PATTERN above / `internal/db/queries/members.sql`'s `GetMember`.
- **IMPORTS**: N/A (SQL file).
- **GOTCHA**: Must run `task sqlc:generate` after editing (or `sqlc generate` directly) — CI's `sqlc` job (`task sqlc:check`) fails the build if generated code is stale relative to this file.
- **VALIDATE**: `task sqlc:generate && git diff --exit-code internal/db/sqlc` (same check CI runs) shows the new `GetWishlistItem` function appended to `internal/db/sqlc/wishlist_items.sql.go`, with no unrelated diff.

### Task 2: Extend DB-layer test for `GetWishlistItem`
- **ACTION**: Add coverage for the new query to the existing integration test.
- **IMPLEMENT**: In `internal/db/wishlist_items_test.go`'s `TestWishlistItemQueries_CRUD`, after the `created, err := store.Queries.CreateWishlistItem(...)` call, add:
  ```go
  fetched, err := store.Queries.GetWishlistItem(t.Context(), created.ID)
  if err != nil {
      t.Fatalf("GetWishlistItem() error = %v, want nil", err)
  }
  if fetched.ID != created.ID || fetched.PhoneNumber != created.PhoneNumber {
      t.Errorf("GetWishlistItem() = %+v, want id/phone to match created", fetched)
  }
  ```
  And after the final `DeleteWishlistItem` call, add:
  ```go
  if _, err := store.Queries.GetWishlistItem(t.Context(), created.ID); !errors.Is(err, sql.ErrNoRows) {
      t.Errorf("GetWishlistItem() after delete error = %v, want sql.ErrNoRows", err)
  }
  ```
  (add `"errors"` to the import block).
- **MIRROR**: The existing function's style — table-free, sequential `t.Fatalf`/`t.Errorf` calls against one real temp SQLite store.
- **IMPORTS**: `errors` (new), `database/sql` (already imported).
- **GOTCHA**: `newTestStore(t)` already runs `db.Migrate` in this file's existing setup — don't duplicate it.
- **VALIDATE**: `go test ./internal/db/... -run TestWishlistItemQueries_CRUD -v` passes.

### Task 3: API layer — DTO, interface, handlers
- **ACTION**: Create `internal/api/wishlist_items.go`.
- **IMPLEMENT**:
  ```go
  package api

  import (
      "context"
      "encoding/json"
      "errors"
      "fmt"
      "net/http"
      "strconv"
      "time"

      "github.com/go-chi/chi/v5"
      kratos "github.com/ory/kratos-client-go/v26"
  )

  var ErrWishlistItemNotFound = errors.New("api: wishlist item not found")

  type WishlistItem struct {
      ID          int64     `json:"id"`
      PhoneNumber string    `json:"phone_number"`
      ItemName    string    `json:"item_name"`
      Size        *string   `json:"size,omitempty"`
      URL         *string   `json:"url,omitempty"`
      CreatedAt   time.Time `json:"created_at"`
      UpdatedAt   time.Time `json:"updated_at"`
  }

  type WishlistItemStore interface {
      CreateWishlistItem(ctx context.Context, phoneNumber, itemName string, size, url *string) (WishlistItem, error)
      ListWishlistItems(ctx context.Context, phoneNumber string) ([]WishlistItem, error)
      UpdateWishlistItem(ctx context.Context, phoneNumber string, id int64, itemName string, size, url *string) (WishlistItem, error)
      DeleteWishlistItem(ctx context.Context, phoneNumber string, id int64) error
  }

  type wishlistItemRequest struct {
      ItemName string  `json:"item_name"`
      Size     *string `json:"size"`
      URL      *string `json:"url"`
  }

  func MountWishlistItems(r chi.Router, items WishlistItemStore, adapter Adapter) {
      r.Post("/wishlist-items", adapter.Adapt(createWishlistItem(items)))
      r.Get("/wishlist-items", adapter.Adapt(listWishlistItems(items)))
      r.Put("/wishlist-items/{id}", adapter.Adapt(updateWishlistItem(items)))
      r.Delete("/wishlist-items/{id}", adapter.Adapt(deleteWishlistItem(items)))
  }

  // phoneTraitFromIdentity — see Patterns to Mirror, IDENTITY_DERIVED_OWNER.

  func createWishlistItem(items WishlistItemStore) HandlerFunc { /* identity -> phone -> decode body -> items.CreateWishlistItem -> 201 */ }
  func listWishlistItems(items WishlistItemStore) HandlerFunc  { /* identity -> phone -> items.ListWishlistItems -> 200 */ }
  func updateWishlistItem(items WishlistItemStore) HandlerFunc { /* identity -> phone -> id -> decode body -> items.UpdateWishlistItem -> 200 */ }
  func deleteWishlistItem(items WishlistItemStore) HandlerFunc { /* identity -> phone -> id -> items.DeleteWishlistItem -> 204 */ }

  func wishlistItemID(r *http.Request) (int64, error) {
      id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
      if err != nil {
          return 0, fmt.Errorf("api: parse wishlist item id: %w", err)
      }
      return id, nil
  }
  ```
  Every one of the four handlers starts with:
  ```go
  identity, ok := IdentityFromContext(r.Context())
  if !ok {
      return WriteProblem(w, unauthorizedProblem())
  }
  phoneNumber, ok := phoneTraitFromIdentity(identity)
  if !ok {
      return fmt.Errorf("api: identity missing phone trait")
  }
  ```
- **MIRROR**: `internal/api/widgets.go` (overall shape) + `internal/api/drawers.go`'s `createDrawer`/`listDrawers` (identity extraction).
- **IMPORTS**: `kratos "github.com/ory/kratos-client-go/v26"` (for the `*kratos.Identity` param type on `phoneTraitFromIdentity` — already a dependency, used in `auth.go`/`drawers.go`).
- **GOTCHA**: Don't accept `phone_number` in `wishlistItemRequest` — it must never be client-suppliable, exactly like `drawerCreateRequest` never has an `organiser_kratos_identity_id` field.
- **VALIDATE**: `go build ./...` succeeds; `go vet ./...` clean.

### Task 4: API-layer unit tests
- **ACTION**: Create `internal/api/wishlist_items_test.go`.
- **IMPLEMENT**: `fakeWishlistItemStore` implementing `WishlistItemStore` (record last args, return configured item/list/err) + a `fakeAuthedValidator`-style helper that also sets `Traits: map[string]any{"phone": phoneNumber}` on the returned `*kratos.Identity`. Table-drive the 4 methods × {authenticated success, no-cookie → 401} the way `TestMountDrawers_CRUD` does; add one explicit test for the "identity has no phone trait" 500-class path (construct an identity with `Traits: map[string]any{}` or `Traits: nil`, assert the adapter's unmapped-error path fires — i.e. status 500 from the adapter's default Problem).
- **MIRROR**: `internal/api/drawers_test.go` (identity-in-context harness) + `internal/api/members_test.go` (fake-store table shape).
- **IMPORTS**: `bytes`, `context`, `errors`, `log/slog`, `net/http`, `net/http/httptest`, `testing`, `time`, `github.com/go-chi/chi/v5`, `kratos "github.com/ory/kratos-client-go/v26"`, `github.com/DanielKirkwood/unwrap-gift/internal/api`.
- **GOTCHA**: `fakeAuthedValidator` in `drawers_test.go` is unexported to that file's test package instance — since this is a new file in the same `api_test` package, either reuse it directly (same package, no import needed) or define a local variant with `traits map[string]any` added to the struct. Prefer extending it in place only if it doesn't break `drawers_test.go`'s existing usage — safest is a **new**, separate fake validator type in this file (`fakeTraitedValidator`) rather than modifying the shared one, to avoid coupling two resources' tests together.
- **VALIDATE**: `go test ./internal/api/... -run WishlistItem -v` passes.

### Task 5: App layer — adapter with ownership enforcement
- **ACTION**: Create `internal/app/wishlist_items.go`.
- **IMPLEMENT**:
  ```go
  package app

  import (
      "context"
      "database/sql"
      "errors"
      "fmt"

      "github.com/DanielKirkwood/unwrap-gift/internal/api"
      "github.com/DanielKirkwood/unwrap-gift/internal/db"
      "github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
  )

  type storeWishlistItems struct {
      store *db.Store
  }

  func (s storeWishlistItems) CreateWishlistItem(
      ctx context.Context, phoneNumber, itemName string, size, url *string,
  ) (api.WishlistItem, error) {
      item, err := s.store.Queries.CreateWishlistItem(ctx, sqlc.CreateWishlistItemParams{
          PhoneNumber: phoneNumber,
          ItemName:    itemName,
          Size:        toNullString(size),
          Url:         toNullString(url),
      })
      if err != nil {
          return api.WishlistItem{}, fmt.Errorf("app: create wishlist item: %w", err)
      }
      return toAPIWishlistItem(item), nil
  }

  func (s storeWishlistItems) ListWishlistItems(ctx context.Context, phoneNumber string) ([]api.WishlistItem, error) {
      rows, err := s.store.Queries.ListWishlistItemsByPhoneNumber(ctx, phoneNumber)
      if err != nil {
          return nil, fmt.Errorf("app: list wishlist items: %w", err)
      }
      out := make([]api.WishlistItem, len(rows))
      for i, row := range rows {
          out[i] = toAPIWishlistItem(row)
      }
      return out, nil
  }

  func (s storeWishlistItems) UpdateWishlistItem(
      ctx context.Context, phoneNumber string, id int64, itemName string, size, url *string,
  ) (api.WishlistItem, error) {
      existing, err := s.store.Queries.GetWishlistItem(ctx, id)
      if err != nil {
          return api.WishlistItem{}, mapWishlistItemErr(err)
      }
      if existing.PhoneNumber != phoneNumber {
          return api.WishlistItem{}, api.ErrWishlistItemNotFound
      }

      item, err := s.store.Queries.UpdateWishlistItem(ctx, sqlc.UpdateWishlistItemParams{
          ID: id, ItemName: itemName, Size: toNullString(size), Url: toNullString(url),
      })
      if err != nil {
          return api.WishlistItem{}, fmt.Errorf("app: update wishlist item: %w", err)
      }
      return toAPIWishlistItem(item), nil
  }

  func (s storeWishlistItems) DeleteWishlistItem(ctx context.Context, phoneNumber string, id int64) error {
      existing, err := s.store.Queries.GetWishlistItem(ctx, id)
      if err != nil {
          return mapWishlistItemErr(err)
      }
      if existing.PhoneNumber != phoneNumber {
          return api.ErrWishlistItemNotFound
      }

      if err := s.store.Queries.DeleteWishlistItem(ctx, id); err != nil {
          return fmt.Errorf("app: delete wishlist item: %w", err)
      }
      return nil
  }

  func toAPIWishlistItem(i sqlc.WishlistItem) api.WishlistItem {
      return api.WishlistItem{
          ID: i.ID, PhoneNumber: i.PhoneNumber, ItemName: i.ItemName,
          Size: fromNullString(i.Size), URL: fromNullString(i.Url),
          CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
      }
  }

  func mapWishlistItemErr(err error) error {
      if errors.Is(err, sql.ErrNoRows) {
          return api.ErrWishlistItemNotFound
      }
      return fmt.Errorf("app: wishlist item store: %w", err)
  }

  func toNullString(s *string) sql.NullString { /* see Patterns to Mirror, SERVICE_PATTERN */ }
  func fromNullString(ns sql.NullString) *string { /* see Patterns to Mirror, SERVICE_PATTERN */ }
  ```
- **MIRROR**: `internal/app/members.go` (`GetMember` ownership-check shape) + `internal/app/widgets.go` (overall CRUD-adapter shape).
- **IMPORTS**: as listed above.
- **GOTCHA**: `GetWishlistItem` from Task 1 must exist in `sqlc.Queries` before this compiles — Task 1 is a hard dependency.
- **VALIDATE**: `go build ./...` succeeds.

### Task 6: Wire into `RouterDeps` and the protected router
- **ACTION**: Update `internal/api/router.go`.
- **IMPLEMENT**: Add fields to `RouterDeps` (near the `Widgets`/`WidgetsAdapter` fields):
  ```go
  // WishlistItems and WishlistItemsAdapter, when non-nil, mount the
  // participant wishlist CRUD endpoints on the protected router, scoped to
  // the caller's own phone trait. Both are left nil unless the database and
  // kratos features are both enabled.
  WishlistItems        WishlistItemStore
  WishlistItemsAdapter Adapter
  ```
  In `NewProtectedRouter`, inside the existing `r.Group` (same group as `Widgets`):
  ```go
  if deps.WishlistItems != nil {
      MountWishlistItems(r, deps.WishlistItems, deps.WishlistItemsAdapter)
  }
  ```
- **MIRROR**: The existing `Widgets`/`WidgetsAdapter` field pair and mount call immediately above it.
- **IMPORTS**: None new.
- **GOTCHA**: Must go in the **same** `r.Group` as Widgets (the one with `deps.Auth` only) — not the hidden router's group, which also applies `deps.Authz`.
- **VALIDATE**: `go build ./...` succeeds; `go test ./internal/api/... -run Router` (existing router tests) still pass.

### Task 7: Wire `BuildServers`
- **ACTION**: Update `internal/app/servers.go`.
- **IMPLEMENT**: Add the `wireWishlistItems` function (see Patterns to Mirror, ADAPTER_WIRING_PATTERN) and call it in `BuildServers`, right after the existing `dbFeature.Enabled` Widgets block:
  ```go
  if dbFeature.Enabled && kratosFeature.Enabled {
      wireWishlistItems(&deps, a)
  }
  ```
- **MIRROR**: `wireOrganiserAPI`'s call-site placement and doc-comment style.
- **IMPORTS**: None new (reuses `net/http`, `api` already imported in this file).
- **GOTCHA**: This condition is independent of `dbFeature.Enabled && kratosFeature.Enabled && ketoFeature.Enabled` (the organiser API's condition, one line below) — don't fold them together; Wishlist deliberately skips the keto check.
- **VALIDATE**: `go build ./...` succeeds.

### Task 8: App-layer ownership test
- **ACTION**: Create `internal/app/wishlist_items_test.go` as `package app` (whitebox).
- **IMPLEMENT**: Using `newAppTestStore(t)` (from `draws_test.go`, same package):
  ```go
  func TestStoreWishlistItems_OwnershipEnforced(t *testing.T) {
      t.Parallel()
      store := newAppTestStore(t)
      items := storeWishlistItems{store: store}

      created, err := items.CreateWishlistItem(t.Context(), "+447700900000", "Board game", nil, nil)
      // ... assert err == nil

      // Owner can update/delete their own item.
      _, err = items.UpdateWishlistItem(t.Context(), "+447700900000", created.ID, "Board game (deluxe)", nil, nil)
      // ... assert err == nil

      // A different phone number cannot.
      _, err = items.UpdateWishlistItem(t.Context(), "+447700900001", created.ID, "hijacked", nil, nil)
      if !errors.Is(err, api.ErrWishlistItemNotFound) {
          t.Errorf("UpdateWishlistItem() (wrong owner) error = %v, want api.ErrWishlistItemNotFound", err)
      }

      deleteErr := items.DeleteWishlistItem(t.Context(), "+447700900001", created.ID)
      if !errors.Is(deleteErr, api.ErrWishlistItemNotFound) {
          t.Errorf("DeleteWishlistItem() (wrong owner) error = %v, want api.ErrWishlistItemNotFound", deleteErr)
      }

      // List is scoped per phone number too.
      list, err := items.ListWishlistItems(t.Context(), "+447700900001")
      // ... assert err == nil, len(list) == 0
  }
  ```
- **MIRROR**: `internal/app/draws_test.go`'s `package app` + `newAppTestStore(t)` usage.
- **IMPORTS**: `errors`, `testing`, `github.com/DanielKirkwood/unwrap-gift/internal/api`.
- **GOTCHA**: Add `//nolint:testpackage // intentional: needs direct access to unexported storeWishlistItems` at the top of the file (same justification comment style as `draws_test.go`), or the linter will flag a whitebox test file.
- **VALIDATE**: `go test ./internal/app/... -run TestStoreWishlistItems -v` passes.

### Task 9: Server-wiring feature-gate test
- **ACTION**: Update `internal/app/servers_test.go`.
- **IMPLEMENT**: Add a table test mirroring `TestBuildServers_OrganiserAPI_RequiresAllThreeFeatures`'s shape, but for two features and the protected router:
  ```go
  func TestBuildServers_WishlistItems_RequiresDatabaseAndKratos(t *testing.T) {
      t.Parallel()
      tests := []struct {
          name        string
          env         config.EnvVars
          wantMounted bool
      }{
          {"all disabled", config.EnvVars{Env: "development", LogLevel: "debug"}, false},
          {"only database enabled", config.EnvVars{Env: "development", LogLevel: "debug", DatabasePath: filepath.Join(t.TempDir(), "test.db")}, false},
          {"database and kratos enabled", config.EnvVars{
              Env: "production", LogLevel: "info",
              DatabasePath: filepath.Join(t.TempDir(), "test.db"),
              KratosPublicURL: "http://127.0.0.1:4433", KratosAdminURL: "http://127.0.0.1:4434",
              KratosCourierWebhookSecret: "test-webhook-secret",
          }, true},
      }
      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              t.Parallel()
              a, err := app.Bootstrap(t.Context(), tt.env)
              // ... (migrate if a.Store != nil, same as TestBuildServers_OrganiserAPI_RequiresAllThreeFeatures)
              servers, err := app.BuildServers(a)
              // ...
              rec := httptest.NewRecorder()
              servers.Protected.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/wishlist-items", nil))
              gotMounted := rec.Code != http.StatusNotFound
              if gotMounted != tt.wantMounted {
                  t.Errorf("GET /wishlist-items status = %d, mounted = %v, want mounted = %v", rec.Code, gotMounted, tt.wantMounted)
              }
          })
      }
  }
  ```
  No cookie is sent, so when mounted the response is 401 (not 404) — exactly the existing "mounted iff non-404" trick, no live Kratos instance required.
- **MIRROR**: `TestBuildServers_OrganiserAPI_RequiresAllThreeFeatures` (table shape) + `TestBuildServers_DatabaseAndKratosEnabled_ProtectedRouterRequiresAuth` (no-cookie-needed trick).
- **IMPORTS**: None new (all already imported in this file).
- **GOTCHA**: Don't try to assert a full authenticated CRUD flow here — that needs a real Kratos instance and belongs in the `e2e`-tagged suite if ever added, not this test.
- **VALIDATE**: `go test ./internal/app/... -run TestBuildServers_WishlistItems -v` passes.

---

## Testing Strategy

### Unit Tests

| Test | Input | Expected Output | Edge Case? |
|---|---|---|---|
| `TestWishlistItemQueries_CRUD` (extended) | `GetWishlistItem(created.ID)` | Row matches created | No |
| `TestWishlistItemQueries_CRUD` (extended) | `GetWishlistItem(deletedID)` | `sql.ErrNoRows` | Yes — not-found |
| `internal/api` handler tests | Authenticated request, valid phone trait | 201/200/204 as appropriate | No |
| `internal/api` handler tests | No `Cookie` header | 401 | Yes — unauthenticated |
| `internal/api` handler tests | Identity with no `phone` trait | 500 (unmapped error) | Yes — malformed identity (shouldn't happen given schema, but defensive) |
| `TestStoreWishlistItems_OwnershipEnforced` | Owner updates/deletes their own item | Success | No |
| `TestStoreWishlistItems_OwnershipEnforced` | Different phone number updates/deletes item | `api.ErrWishlistItemNotFound` | Yes — the core security property |
| `TestStoreWishlistItems_OwnershipEnforced` | `ListWishlistItems` for a phone with no items | Empty slice, no error | Yes — empty result |
| `TestBuildServers_WishlistItems_RequiresDatabaseAndKratos` | Each feature-flag combination | Mounted iff db+kratos both enabled | Yes — all 3 combinations |

### Edge Cases Checklist
- [x] Empty input — covered by "no size/url" case already in the existing DB test; item_name empty string is accepted (no validation anywhere else in this codebase either — consistent, not a gap).
- [x] Maximum size input — not applicable, no length limits exist elsewhere in this codebase.
- [x] Invalid types — malformed JSON body → existing `json.NewDecoder` decode-error path (500-class, same as every other resource).
- [x] Concurrent access — not a new concern; SQLite + existing `*db.Store` concurrency handling is unchanged by this plan.
- [ ] Network failure — N/A, no outbound calls in this phase.
- [x] Permission denied — the ownership-mismatch case above (`api.ErrWishlistItemNotFound`) is a 404, not a distinct 403 — Problem shape is "not found", not "forbidden" — mirrors Members exactly.

---

## Validation Commands

### Static Analysis
```bash
go build ./...
golangci-lint run
```
EXPECT: Zero build errors, zero lint errors.

### sqlc Freshness
```bash
task sqlc:check
```
EXPECT: No diff — `GetWishlistItem` was generated and committed, nothing else changed unexpectedly.

### Unit Tests
```bash
go test ./internal/db/... ./internal/api/... ./internal/app/... -v
```
EXPECT: All pass, including the three new/extended tests above.

### Full Test Suite
```bash
task test
```
EXPECT: No regressions anywhere else in the suite.

### Database Validation
```bash
task db:migrate
```
EXPECT: Applies cleanly (no new migration in this phase — only a new query against the existing `wishlist_items` table — so this should be a no-op if already migrated).

### Manual Validation
- [ ] `task build && task run`, then with a valid Kratos session cookie for phone A: `POST /wishlist-items {"item_name":"Board game"}` → 201 with `phone_number` set to A's phone, never client-supplied.
- [ ] `GET /wishlist-items` as phone A → only A's items.
- [ ] `PUT /wishlist-items/{id}` (an item belonging to phone B) as phone A → 404.
- [ ] `DELETE /wishlist-items/{id}` (own item) as phone A → 204, confirmed gone via a follow-up `GET /wishlist-items`.

---

## Acceptance Criteria
- [ ] All 9 tasks completed.
- [ ] All validation commands pass.
- [ ] Tests written and passing (DB-layer, API-layer, app-layer-ownership, server-wiring).
- [ ] No type errors, no lint errors.
- [ ] A logged-in member can add/edit/remove their own wishlist items and cannot see or edit anyone else's (PRD Phase 6 success signal) — proven by `TestStoreWishlistItems_OwnershipEnforced`.

## Completion Checklist
- [ ] Code follows discovered patterns (Widgets shape + Drawers identity-derivation + Members ownership-check).
- [ ] Error handling matches codebase style (`ErrorsMap`, sentinel errors, ambiguous not-found-vs-wrong-owner shape).
- [ ] Logging follows codebase conventions (none added — none exists elsewhere either).
- [ ] Tests follow test patterns identified above (right package, right fixture style per layer).
- [ ] No hardcoded values.
- [ ] `ARCHITECTURE.md`'s "Adding a new resource" section doesn't need updating — Widgets/Identities remain the two canonical templates; this plan is a composition of both, not a third template worth documenting there.
- [ ] No unnecessary scope additions (no single-item GET, no phone validation — per confirmed decisions).
- [ ] Self-contained — no questions needed during implementation.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `identity.schema.json`'s `phone` trait has no E.164 pattern, so a future registrant entering a differently-formatted phone number than the organiser used when adding their `members.phone_number` row would see an empty wishlist / be unable to have their wishlist matched to their Member row by phone-string equality | Low (single organiser, Daniel controls both inputs for v1) | Medium (silent empty-list, not a crash — `ListWishlistItems` just returns `[]` for an unmatched phone) | Explicitly deferred per Decisions Confirmed With User #3; revisit if/when a second organiser or less-controlled registration flow is introduced |
| A handler reaches `phoneTraitFromIdentity` and gets `ok=false` (malformed/missing trait) in production despite the schema requiring it | Very low (would require a malformed session Kratos itself shouldn't produce) | Low (surfaces as a generic 500, logged via `Adapter.logUnmapped` — not a silent failure) | None needed — existing unmapped-error path already handles this safely |
| `GetWishlistItem` query addition causes sqlc generation drift if run with a different sqlc version than CI's | Low | Low (caught immediately by `task sqlc:check` in CI) | Run `task sqlc:generate` locally before committing, same as every prior resource |

## Notes
- This phase deliberately reuses the Widgets template (`ARCHITECTURE.md`'s primary worked example) almost verbatim, with the Drawers-derived "owner comes from the session identity, never the request body" idiom layered on top — there was no single existing resource that was a perfect match, so this plan explicitly synthesizes two precedents rather than inventing a third.
- The three confirmed decisions (ownership-check mechanism, no single-item GET, no phone-format validation) are the only genuinely open questions found during research; everything else in this plan follows directly from reading the actual Phase 1/5 code, not from the PRD text alone — consistent with this repo's general lesson (see memory: the PRD's own phase-status table lags real implementation progress, so the code is the source of truth).
- Phase 7 (Notification pipeline) will reuse `ListWishlistItemsByPhoneNumber` (already exists, unchanged by this plan) to compose each gifter's SMS — no coordination needed with that phase beyond not breaking that query's signature.
