# Implementation Report: Wishlist Web UI (Secret Santa Phase 8)

## Summary

Implemented a fourth HTTP router/server (`internal/web`, port `WEB_PORT`/8083) that serves a
custom-built login page (driving Kratos's SMS one-time-code browser flow directly, no delegated
self-service UI service) and the participant wishlist CRUD UI, both server-rendered with Go's
stdlib `html/template` + htmx + WebAwesome web components loaded from CDN. `kratos-selfservice-ui-node`
was removed from both dev and production compose stacks; `internal/web/login.go` renders whatever
Kratos login-flow UI nodes are current via a generic, node-driven template (no per-step hardcoding),
special-casing a `code`-named field as a `<wa-otp-input>` for WebOTP autofill support.

## Assessment vs Reality

| Metric | Predicted (Plan) | Actual |
|---|---|---|
| Complexity | Large | Large — confirmed |
| Confidence | 7/10 (prior draft) | Delivered with no architectural blockers; one real Go-type-system gap (see Deviations) |
| Files Changed | ~24 (14 new, ~10 updated) | 41 (19 new, 22 updated) |

## Tasks Completed

| # | Task | Status | Notes |
|---|---|---|---|
| 1 | Config — WebConfig + "web" feature | Complete | |
| 2 | Config — KratosConfig gains BrowserURL | Complete | |
| 3 | kratosclient — GetLoginFlow | Complete | |
| 4 | `internal/web` package skeleton + doc.go | Complete | |
| 5 | Web-local middleware | Complete | |
| 6 | Auth — SessionValidator, context key, AuthenticationMiddleware | Complete | |
| 7 | Login — LoginFlowProvider, GET /login handler, field-flattening helper | Complete | |
| 8 | Wishlist DTO, store interface, handlers, mount | Complete | Deviated — needed a second app-layer adapter (`webStoreWishlistItems`), see Deviations |
| 9 | Templates — heavy WebAwesome usage, mobile-first | Complete | Deviated — `{{template "base.html" .}}` composition rewritten as a named `{{define "head"}}` sub-template; Cancel button's `hx-select` target corrected during writing |
| 10 | Router — NewWebRouter | Complete | |
| 11 | Wire into `internal/app` | Complete | |
| 12 | Tests | Complete | 22 new tests in `internal/web`, plus 3 new cases each in `internal/config`'s existing test files and `internal/app/servers_test.go` |
| 13 | Deploy config — dev | Complete | |
| 14 | Deploy config — production | Complete | Extended beyond the plan's exact file list — see Deviations |
| 15 | Update `deploy/kratos/README.md` | Complete | Also updated `README.md`, `DEPLOYMENT.md`, `deploy/keto/README.md` — see Deviations |

## Validation Results

| Level | Status | Notes |
|---|---|---|
| Static Analysis | Pass | `go vet ./...` and `golangci-lint run ./...` both zero issues |
| Unit Tests | Pass | `go test ./...` all green (22 new tests in `internal/web`, 6 new in `internal/config`, 1 new in `internal/app`) |
| Build | Pass | `go build ./...` |
| Integration | N/A | No live Kratos instance available in this environment — login round-trip validated via fake `LoginFlowProvider`/`SessionValidator`, not a real Kratos browser flow. Flagged as a required manual step before calling this phase fully done (see Next Steps) |
| Edge Cases | Pass | Blank item name, expired/invalid login flow, not-owned item update, login redirect loop (explicit test), web feature disabled (routes 404, server still built) |

## Files Changed

| File | Action | Lines |
|---|---|---|
| `internal/web/doc.go` | CREATED | +16 |
| `internal/web/router.go` | CREATED | +71 |
| `internal/web/middleware.go` | CREATED | +68 |
| `internal/web/auth.go` | CREATED | +72 |
| `internal/web/login.go` | CREATED | +117 |
| `internal/web/wishlist.go` | CREATED | +290 |
| `internal/web/templates.go` | CREATED | +16 |
| `internal/web/templates/base.html` | CREATED | +49 |
| `internal/web/templates/login.html` | CREATED | +42 |
| `internal/web/templates/wishlist.html` | CREATED | +28 |
| `internal/web/templates/item_list.html` | CREATED | +20 |
| `internal/web/templates/item_row.html` | CREATED | +25 |
| `internal/web/templates/item_edit_row.html` | CREATED | +22 |
| `internal/web/templates/error.html` | CREATED | +4 |
| `internal/web/auth_test.go` | CREATED | +102 |
| `internal/web/login_test.go` | CREATED | +163 |
| `internal/web/wishlist_test.go` | CREATED | +214 |
| `internal/web/router_test.go` | CREATED | +69 |
| `internal/web/helpers_test.go` | CREATED | +22 |
| `internal/config/config.go` | UPDATED | +9/-0 |
| `internal/config/features.go` | UPDATED | +34/-6 |
| `internal/config/config_test.go` | UPDATED | +3/-4 |
| `internal/config/registry_test.go` | UPDATED | +83/-0 |
| `internal/clients/kratosclient/kratosclient.go` | UPDATED | +20/-0 |
| `internal/app/servers.go` | UPDATED | +60/-4 |
| `internal/app/servers_test.go` | UPDATED | +71/-0 |
| `internal/app/wishlist_items.go` | UPDATED | +101/-1 |
| `cmd/start.go` | UPDATED | +25/-5 |
| `.env.example` | UPDATED | +9/-2 |
| `deploy/kratos/docker-compose.kratos.yml` | UPDATED | -15 (removed `kratos-selfservice-ui-node` service) |
| `deploy/kratos/kratos.yml` | UPDATED | +22/-12 |
| `deploy/kratos/kratos.production.yml` | UPDATED | +22/-12 |
| `deploy/kratos/README.md` | UPDATED | +8/-2 |
| `deploy/keto/README.md` | UPDATED | +4/-3 |
| `deploy/production/docker-compose.yml` | UPDATED | +5/-12 |
| `deploy/production/Caddyfile` | UPDATED | +6/-6 |
| `deploy/production/README.md` | UPDATED | +12/-5 |
| `deploy/production/.env.production.example` | UPDATED | +1/-5 |
| `ARCHITECTURE.md` | UPDATED | +25/-11 |
| `README.md` | UPDATED | +7/-6 |
| `DEPLOYMENT.md` | UPDATED | +1/-1 |

## Deviations from Plan

1. **WHAT**: Added a second app-layer adapter, `webStoreWishlistItems`, in `internal/app/wishlist_items.go` — not in the plan's Files to Change table.
   **WHY**: The plan's "Patterns to Mirror" section claimed `internal/app`'s existing `storeWishlistItems` would "structurally satisfy" both `api.WishlistItemStore` and the new `web.WishlistItemStore` via Go's duck typing. This is incorrect: Go interface satisfaction requires an *exact* method-signature match, including return types — `storeWishlistItems.CreateWishlistItem` returns `(api.WishlistItem, error)`, and `api.WishlistItem`/`web.WishlistItem` are distinct named types in different packages (deliberately, since `internal/web` must not import `internal/api`). Structural identity of the two DTOs' fields doesn't make them interchangeable at the interface level. Fixed by adding `webStoreWishlistItems` alongside the existing adapter, reusing the same sqlc queries with a separate `toWebWishlistItem` conversion function. Documented in `ARCHITECTURE.md`'s "Interface segregation" section so this isn't rediscovered the same way next time.

2. **WHAT**: `base.html`'s composition rewritten from the plan's illustrative `{{template "base.html" .}}` snippet to a named `{{define "head"}}...{{end}}` sub-template, invoked as `{{template "head" .}}` from each page.
   **WHY**: `html/template`'s `ParseFS` shares one template namespace across every parsed file; a literal `{{template "base.html" .}}` would paste base.html's *entire* content (including its own `<html>`/`<body>` if it had them) inline rather than "extending" a layout the way Jinja/Django templates do. The plan's snippet was illustrative, not literally correct Go template composition — this was caught and fixed during Task 9's implementation, verified by `TestParseTemplates_NoError` and the rendering tests actually asserting on output content.

3. **WHAT**: `item_edit_row.html`'s Cancel button initially used `hx-select="#wishlist-items"` targeting `#item-{{.ID}}` (a shape mismatch — swapping a whole list wrapper into a single card's slot); corrected to `hx-select="#item-{{.ID}}"` so it fetches and swaps in just the matching read-mode row from a full-page re-fetch.
   **WHY**: Caught while writing the template, not from the plan (which didn't specify Cancel's exact htmx wiring in enough detail to catch this). No functional regression — this was corrected before any test ran against it.

4. **WHAT**: Deploy-config updates extended beyond the plan's explicit file list to also touch `deploy/production/.env.production.example` (removed now-orphaned `UI_COOKIE_SECRET`/`UI_CSRF_COOKIE_SECRET`), `deploy/production/README.md` and `deploy/keto/README.md` (registration/seeding instructions referencing the removed `:4455` self-service UI), and the top-level `README.md`/`DEPLOYMENT.md` (same class of stale reference).
   **WHY**: These weren't in the plan's Files to Change table, but removing `kratos-selfservice-ui-node` made every one of them either reference a secret for a service that no longer exists or document a now-broken URL (`:4455/registration`, `login.<DOMAIN>`). Leaving them stale would have been a real, discoverable inconsistency bug in the deploy docs — treated as in-scope for "remove the service cleanly," not new scope. A repo-wide `grep` for `4455|selfservice-ui-node|login\.\${DOMAIN}|login\.<DOMAIN>` after all edits confirms no remaining stale references outside historical plan documents (which are records of what was planned, not live docs).

5. **WHAT**: `kratos.yml`/`kratos.production.yml`'s `recovery`/`verification` flows set to `enabled: false` (the plan's Decision #7, correctly anticipated) rather than left configured with a dead `ui_url`.
   **WHY**: Confirms the plan's own stated decision — not a deviation in substance, noting it here only because it's a less common Kratos config choice worth a reader double-checking if this phase is ever revisited.

No deviations were judged architecturally significant enough to require stopping and asking — all were caught and resolved during implementation with the validation loop (build → test → lint after each file).

## Issues Encountered

- `html/template` auto-escapes apostrophes (`Couldn't` → `Couldn&#39;t`) — correct XSS-safe behavior, but one test's substring assertion initially didn't account for it. Fixed the test, not the escaping.
- `golangci-lint`'s `goconst` flagged the `"kratos"` string literal (3 occurrences across `features.go` once the new `"web"` feature's `Requires` list added a third) — fixed with a `kratosFeatureName` const, used consistently in the `kratos` feature's own `Name`, `keto`'s `Requires`, and `web`'s `Requires`.
- Several `unparam` findings in test helper functions (`mountTestLogin`'s `kratosBrowserURL`, `testValidator`'s `phone`/`fullName`) where every call site passed the same value — simplified by hardcoding shared test fixtures (`testKratosBrowserURL`, `testPhoneNumber`, `testFullName` consts in a new `helpers_test.go`) rather than threading unused flexibility through parameters.
- No live Kratos/Postgres instance was available in this environment to exercise the real browser-flow round-trip (Task 13's manual validation step) — covered by fakes instead. This is a genuine gap between "tests pass" and "the real integration works," called out explicitly below.

## Tests Written

| Test File | Tests | Coverage |
|---|---|---|
| `internal/web/auth_test.go` | 4 | `AuthenticationMiddleware`: nil validator passthrough, no-cookie redirect, invalid-session redirect, valid-session identity-in-context |
| `internal/web/login_test.go` | 5 | `MountLogin`: no-flow redirect to Kratos, flow-present rendering (identifier step, code step → `wa-otp-input`), expired-flow fresh redirect |
| `internal/web/wishlist_test.go` | 7 | `MountWishlist`: show/greeting, create success, create blank-name validation, update store-error in-band rendering, delete, no-session redirect |
| `internal/web/router_test.go` | 4 | `NewWebRouter`: template parsing, root redirect, feature-disabled 404s, login-not-behind-auth (loop prevention) |
| `internal/config/registry_test.go` | +6 cases | `"web"` feature gating (enabled/disabled combinations), `KratosConfig.BrowserURL` fallback and explicit-value behavior |
| `internal/config/config_test.go` | 0 new, 3 updated | Existing `TestLoad` cases extended to assert the new `WebPort` default/override |
| `internal/app/servers_test.go` | 1 | `Servers.Web` always built, routes mounted only when database+kratos both enabled |

## Next Steps

- [ ] Code review via `/code-review`
- [ ] **Manual validation against a live Kratos instance** (Task 13's VALIDATE step) — this report's test suite uses fakes throughout; the actual browser round-trip (phone → SMS code → `wa-otp-input` → session → `/wishlist`) has not been exercised against a real `task kratos:up` stack. Do this before considering the phase fully proven, not just lint/test-green.
- [ ] Spot-check the rendered pages at a real phone-width viewport in a browser — automated tests check for the presence of expected tags/attributes, not actual visual layout.
- [ ] Confirm exact CDN version pins for htmx and WebAwesome (`internal/web/templates/base.html` currently has `{VERSION}` placeholders, per the plan's own explicit GOTCHA — not resolved during this implementation since it requires a live docs check, not something to guess).
- [ ] Verify `<wa-otp-input length="6">` actually matches Kratos's configured code length against a live flow (also an explicit plan GOTCHA, unresolved here for the same reason).
- [ ] Production deploy verification: `session.cookie.domain`/`app.${DOMAIN}` cookie-sharing alignment (plan's Risk table) can only be confirmed against a real deployed stack.
- [ ] Create PR via `/prp-pr`
