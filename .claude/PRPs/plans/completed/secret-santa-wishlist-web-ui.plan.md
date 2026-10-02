# Plan: Wishlist Web UI

## Summary

Add a fourth HTTP server/router (`internal/web`, following the existing three-router model) that
serves **both** a custom-built login page and a wishlist CRUD UI, server-rendered with Go's stdlib
`html/template`, htmx for interactivity, and WebAwesome web components used as extensively as
possible for the visual layer — all loaded from CDN, no JS build step. Kratos is headless by design
("bring your own UI" is its documented primary integration path); this plan builds that UI directly
against Kratos's public browser-flow API rather than deploying Ory's generic reference UI
(`kratos-selfservice-ui-node`), which is **removed** from both compose stacks. The payoff: one
visually consistent app (login included), WebAwesome's purpose-built `<wa-otp-input>` component for
a materially better code-entry experience (WebOTP autofill support), and one fewer deployed service.

## User Story

As a drawer member (family/friend), I want to log in with my phone number on a page that looks and
feels like the rest of the app, and manage my wishlist, so that the whole experience is simple and
consistent on my phone.

## Problem → Solution

Today, wishlist items can only be created/edited/deleted via raw JSON API calls to the protected
router (`POST/GET/PUT/DELETE /wishlist-items`, Phase 6) — unusable by a non-technical family member,
and there is no login UI at all. → A phone-shaped web app (new "web" router, port `WEB_PORT`) with
its own `/login` page (driving Kratos's SMS one-time-code browser flow directly) and `/wishlist`
page, both built from WebAwesome components, backed by the exact same `storeWishlistItems`/
`kratosclient.Client` the rest of the app already uses.

## Metadata

- **Complexity**: Large (new package, new router/server, new feature-flag wiring, a from-scratch
  Kratos browser-flow integration, deploy config changes/removals across 6 files)
- **Source PRD**: `.claude/PRPs/prds/secret-santa-drawer.prd.md`
- **PRD Phase**: Phase 8 — Wishlist web UI
- **Estimated Files**: ~24 (14 new, ~10 updated, 1 service removed from 2 compose files)

---

## Why build our own login UI instead of Ory's reference UI (revised decision)

An earlier draft of this plan delegated login entirely to `kratos-selfservice-ui-node` (Ory's
official prebuilt reference UI, already present in both compose stacks from Phase 2). That's a valid
approach, but **not the better one here**, for three concrete reasons surfaced during review:

1. **WebAwesome ships a purpose-built `<wa-otp-input>` component** (confirmed:
   https://webawesome.com/docs/components/otp-input/) with `autocomplete="one-time-code"` (prompts
   browser/OS SMS-code autofill) and WebOTP API support on Android (Chrome can read the code from an
   incoming SMS automatically — **zero manual typing**). Ory's generic reference UI has no equivalent.
   For the PRD's actual bar — "a real (non-technical) family/friend-group member can, unassisted, log
   in" — this is a meaningfully better fit than a generic text input.
2. **Visual consistency**: a hand-off to a differently-styled Node.js app for login, then back to our
   WebAwesome-styled wishlist page, is a jarring seam for a two-page app. Building both pages from the
   same component library removes it entirely.
3. **Kratos is explicitly headless** — "bring your own UI" is its documented, first-class integration
   path (confirmed: https://www.ory.com/docs/kratos/bring-your-own-ui/custom-ui-basic-integration).
   `kratos-selfservice-ui-node` is Ory's own *reference implementation* of that pattern, not a
   required dependency — and our actual needed surface (SMS-code login only; no registration, no
   password recovery, no email verification — see NOT Building) is narrower than what that reference
   app renders.

**Cost**: we now own the Kratos browser-flow integration (fetching a flow by ID, rendering its
`ui.nodes` generically, letting the browser POST directly to Kratos's public endpoint) instead of
getting it for free. This plan's design renders `ui.nodes` **generically** (loop + special-case by
node name/type, not two hardcoded step-1/step-2 templates), so the SAME handler and template work for
both steps of the code flow, and any Kratos-added node (e.g. a "resend code" button) renders
automatically without new code — see Task 9.

---

## UX Design

### Before

```
┌─────────────────────────────────────────┐
│  No UI exists at all. A member wants to  │
│  log in or add a wishlist item — only    │
│  option is a raw HTTP request (curl) to  │
│  Kratos's native API or the JSON         │
│  wishlist-items endpoints by hand.       │
└─────────────────────────────────────────┘
```

### After

```
┌─────────────────────────────────────────┐
│  1. Member visits app.<domain>/wishlist  │
│  2. Not logged in → our own /login page: │
│     <wa-card> with phone number input    │
│  3. Submits → Kratos sends SMS code      │
│  4. Same page now shows <wa-otp-input>   │
│     — on Android, code often autofills   │
│     via WebOTP, no typing needed         │
│  5. Submit → Kratos redirects back to    │
│     /wishlist, now logged in             │
│  6. "Hi <full name>" + items as          │
│     <wa-card>s, an "Add item" form below │
│  7. Add/edit/delete via htmx partial     │
│     swaps — no full page reloads         │
└─────────────────────────────────────────┘
```

### Interaction Changes

| Touchpoint | Before | After | Notes |
|---|---|---|---|
| Login — phone entry | None | `<wa-card>` with `<wa-input type="tel">`, submits directly to Kratos's public API | Our own page, WebAwesome-styled |
| Login — code entry | None | Same page, now showing `<wa-otp-input length="6">` | WebOTP autofill on supporting devices |
| Add item | Raw `POST /wishlist-items` JSON | `<wa-card>` form with `<wa-input>` fields, `<wa-button>` submit, swaps item list via htmx | |
| Edit item | Raw `PUT /wishlist-items/{id}` JSON | `<wa-icon>` edit button → row swaps to inline edit form | |
| Delete item | Raw `DELETE /wishlist-items/{id}` JSON | `<wa-icon>` delete button (confirm dialog) → `hx-delete`, row removed | |

---

## Mandatory Reading

| Priority | File | Lines | Why |
|---|---|---|---|
| P0 | `internal/api/router.go` | 1–173 | Three-router model this extends to a fourth; shared middleware stack to mirror (minus `EnforceJSON`) |
| P0 | `internal/api/wishlist_items.go` | 1–208 | `WishlistItemStore` interface/handler shape to re-declare in `internal/web` |
| P0 | `internal/app/wishlist_items.go` | 1–141 | `storeWishlistItems` — the concrete adapter this plan reuses unchanged |
| P0 | `internal/app/servers.go` | 1–295 | `BuildServers`/`Servers`/`Shutdown` — exact spot to wire the 4th server |
| P0 | `internal/api/auth.go` | 1–77 | `AuthenticationMiddleware`/`SessionValidator`/identity-context-key pattern to re-implement |
| P0 | `internal/api/middleware.go` | 1–143 | `EnforceJSON` (must NOT apply to the web router), `RequestLogger`, `SecurityHeaders` to adapt |
| P0 | `internal/clients/kratosclient/kratosclient.go` | 1–188 | `Client.ToSession`'s exact shape (builder pattern, `closeBody`, error wrapping) — `GetLoginFlow` (Task 3) mirrors it precisely |
| P0 | `internal/config/features.go` | 1–129 | `Feature`/`Registry` pattern for the new `"web"` feature and `KratosConfig`'s extension |
| P0 | `internal/config/config.go` | ~20–45 | `EnvVars` fields to mirror for `WebPort`/`KratosBrowserURL` |
| P0 | Go module: `github.com/ory/kratos-client-go/v26` — `model_login_flow.go`, `model_ui_container.go`, `model_ui_node.go`, `model_ui_node_input_attributes.go`, `model_ui_node_meta.go`, `model_ui_text.go` (found locally at `$(go env GOMODCACHE)/github.com/ory/kratos-client-go/v26@v26.2.0/`) | — | **The exact, version-pinned SDK struct shapes this plan's node-rendering code depends on** — verified directly against the installed module during planning, not guessed. See Patterns to Mirror's KRATOS_SDK_SHAPES block for the exact fields. |
| P1 | `internal/api/errors.go` | 1–60 | `HandlerFunc = func(w,r) error` + `Adapter` pattern to mirror for HTML error rendering |
| P1 | `internal/api/wishlist_items_test.go` | 1–90+ | Fake-store + `httptest` test pattern to mirror |
| P1 | `internal/app/servers_test.go` | 1–70+ | How `BuildServers` is tested end-to-end |
| P1 | `cmd/start.go` | full | `selectFunc`/subcommand pattern — needs a `webApi` subcommand + `selectWeb` |
| P1 | `deploy/kratos/kratos.yml` + `kratos.production.yml` | `selfservice` block | `login.ui_url`, `default_browser_return_url`, `allowed_return_urls` all need repointing at the new web app; `kratos-selfservice-ui-node` service removal |
| P1 | `deploy/kratos/docker-compose.kratos.yml` | `kratos-selfservice-ui-node` block | Confirms its exact env shape (`KRATOS_PUBLIC_URL` vs `KRATOS_BROWSER_URL` — the two-URL pattern Task 2 reuses) before removing the service itself |
| P1 | `deploy/production/Caddyfile` + `docker-compose.yml` | full | `login.{$DOMAIN}` block to remove, `app.{$DOMAIN}` block to add |
| P2 | `deploy/kratos/README.md` | full | Manual walkthrough needs updating (no more `:4455`); confirms identities are organiser-provisioned only — **no registration UI needed** |
| P2 | `ARCHITECTURE.md` | "Adding a new resource", "Dependency direction" | The one-directional `internal/{api,web} ← app` rule this plan must not violate |

## External Documentation

| Topic | Source | Key Takeaway |
|---|---|---|
| Ory Kratos self-hosted UI integration | https://www.ory.com/docs/kratos/bring-your-own-ui/custom-ui-basic-integration | Browser flow: redirect-to-Ory pattern, form `action`/`method` taken directly from `ui.action`/`ui.method`, submitted directly browser→Kratos, not proxied |
| Ory Kratos login flow / cookies | https://www.ory.com/docs/kratos/self-service/flows/user-login | Session cookie set via `303` redirect on success (`SameSite=Lax`); redirect target is `return_to` (if allow-listed) else `default_browser_return_url` |
| WebAwesome `wa-otp-input` | https://webawesome.com/docs/components/otp-input/ | `autocomplete="one-time-code"` default; WebOTP API auto-read on Android; `length` attribute (default 6), `type="numeric"` |
| WebAwesome components (verified set) | https://webawesome.com/docs/components/ | `wa-card`, `wa-callout` (the alert component — no separate `wa-alert`), `wa-icon` (bundled Font Awesome), `wa-divider`, `wa-spinner`, `wa-badge`, `wa-button`, `wa-input` — exact attributes in Patterns to Mirror below |
| WebAwesome layout utilities | https://webawesome.com/docs/ (utility classes) | `wa-stack`/`wa-cluster`/`wa-grid` CSS utility classes exist for layout without a JS component; `wa-page` exists for full app shells with nav/sidebar (considered, not used — overkill for a 2-page app) |
| WebAwesome design tokens | https://webawesome.com/docs/tokens/ | All `--wa-` prefixed: spacing scale `--wa-space-3xs` … `--wa-space-3xl`; color palette + semantic tokens e.g. `--wa-color-brand-fill-normal`, `--wa-color-danger-*` — use these in custom CSS instead of hardcoded values |
| htmx | https://htmx.org/docs/ | HTML-attribute-driven, CDN-only; default behavior does not swap content on non-2xx responses — this plan always responds 200 from htmx-targeted endpoints (Decision, unchanged from prior draft) |

---

## Patterns to Mirror

### THREE-ROUTER MODEL → FOUR-ROUTER MODEL
```go
// SOURCE: internal/api/router.go:147-164
func newRouter(name string, deps RouterDeps) *chi.Mux {
	r := chi.NewRouter()
	r.NotFound(problemHandler(Problem{Status: http.StatusNotFound, Title: "Not Found"}))
	r.MethodNotAllowed(problemHandler(Problem{Status: http.StatusMethodNotAllowed, Title: "Method Not Allowed"}))
	r.Use(middleware.RequestID)
	r.Use(RequestLogger(deps.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(requestTimeout))
	r.Use(EnforceJSON)          // <-- internal/web must NOT include this; it 406s text/html
	r.Use(SecurityHeaders)
	r.Use(otelchi.Middleware(name, otelchi.WithChiRoutes(r), otelchi.WithTracerProvider(deps.TracerProvider)))
	MountHealth(r, deps.Ready)
	return r
}
```
`internal/web` writes its own `NewWebRouter` with the same third-party middleware calls
(`middleware.RequestID`/`Recoverer`/`Timeout`, `otelchi.Middleware`) but a web-local
`RequestLogger`/`SecurityHeaders` (Task 5) and no `EnforceJSON`.

### KRATOS_SDK_SHAPES (verified against the installed `kratos-client-go/v26@v26.2.0` module — not guessed)
```go
// github.com/ory/kratos-client-go/v26, confirmed field names:

type LoginFlow struct {
	Id         string
	ExpiresAt  time.Time
	RequestUrl string
	ReturnTo   *string        // Kratos echoes the return_to it was given, if you need it
	Ui         UiContainer
	// ...
}

type UiContainer struct {
	Action   string    // <form action="{{ .Ui.Action }}">
	Method   string    // <form method="{{ .Ui.Method }}">
	Nodes    []UiNode
	Messages []UiText  // flow-level messages (e.g. "a code has been sent")
}

type UiNode struct {
	Attributes UiNodeAttributes // a Go "oneOf" struct — see below
	Group      string           // "default", "code", etc.
	Messages   []UiText         // per-node validation/info messages
	Meta       UiNodeMeta
	Type       string           // "input", "text", ...
}

// UiNodeAttributes is a struct of pointer fields, one non-nil per actual node kind —
// Go's OpenAPI generator's way of representing a union type. For every node this plan
// cares about (type == "input"), access via .UiNodeInputAttributes:
type UiNodeAttributes struct {
	UiNodeInputAttributes *UiNodeInputAttributes // non-nil for Type == "input"
	// UiNodeTextAttributes, UiNodeAnchorAttributes, etc. — not needed for login/code
}

type UiNodeInputAttributes struct {
	Name     string       // "identifier", "code", "csrf_token", "method", ...
	Type     string       // "text", "hidden", "submit", ...
	Value    interface{}  // present for hidden/prefilled fields
	Required *bool
	Disabled bool
	Autocomplete *string  // e.g. "tel", "one-time-code"
}

type UiNodeMeta struct {
	Label *UiText // human-readable label/button text, e.g. "Sign in with code"
}

type UiText struct {
	Id   int64
	Text string
	Type string // "info" | "error" | "success"
}
```
`FrontendAPIService.GetLoginFlow(ctx).Id(flowID).Cookie(cookieHeader).Execute()` returns
`(*LoginFlow, *http.Response, error)` — same builder-pattern shape as `ToSession` already uses.

### KRATOSCLIENT (mirror ToSession exactly, for GetLoginFlow)
```go
// SOURCE: internal/clients/kratosclient/kratosclient.go:67-82
func (c *Client) ToSession(ctx context.Context, cookieHeader string) (*kratos.Session, error) {
	session, resp, err := c.Public.FrontendAPI.ToSession(ctx).Cookie(cookieHeader).Execute()
	closeBody(resp)
	if err != nil {
		return nil, fmt.Errorf("kratosclient: to session: %w", err)
	}
	return session, nil
}
// GetLoginFlow (new, Task 3) follows this exact shape with FrontendAPI.GetLoginFlow(ctx).Id(flowID).Cookie(cookieHeader).Execute()
```

### INTERFACE SEGREGATION (declare where consumed, not where implemented)
```go
// SOURCE: internal/api/wishlist_items.go:52-63 — mirrored verbatim-shape in internal/web
type WishlistItemStore interface {
	CreateWishlistItem(ctx context.Context, phoneNumber, itemName string, size, url *string) (WishlistItem, error)
	ListWishlistItems(ctx context.Context, phoneNumber string) ([]WishlistItem, error)
	UpdateWishlistItem(ctx context.Context, phoneNumber string, id int64, itemName string, size, url *string) (WishlistItem, error)
	DeleteWishlistItem(ctx context.Context, phoneNumber string, id int64) error
}
```
`internal/app`'s existing `storeWishlistItems{store: a.Store}` structurally satisfies this new,
separately-declared `web.WishlistItemStore` too — no new adapter code. Same reasoning applies to
`kratosclient.Client`, which will structurally satisfy both `api.SessionValidator` and the new
`web.SessionValidator`/`web.LoginFlowProvider` interfaces once `GetLoginFlow` is added to it.

### ERROR_HANDLING (HandlerFunc + Adapter → HTML instead of Problem JSON)
```go
// SOURCE: internal/api/errors.go:14-56 (mirror structure, swap output format)
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

type Adapter struct {
	Logger    *slog.Logger
	Templates *template.Template
}

func (a Adapter) Adapt(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			a.Logger.ErrorContext(r.Context(), "web: handler error", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = a.Templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Something went wrong."})
		}
	}
}
```

### APP-LAYER WIRING (feature-gated, nil-is-disabled)
```go
// SOURCE: internal/app/servers.go:111-117 (pattern to extend)
webFeature, _ := a.Registry.Feature("web")
webCfg, _ := webFeature.Config.(config.WebConfig)
webDeps := web.RouterDeps{Logger: a.Logger, TracerProvider: a.Otel.TracerProvider}
if webFeature.Enabled {
	templates, err := web.ParseTemplates()
	if err != nil {
		return nil, fmt.Errorf("app: parse web templates: %w", err)
	}
	webDeps.Templates = templates
	webDeps.Auth = web.AuthenticationMiddleware(a.Kratos)
	webDeps.LoginFlows = a.Kratos
	webDeps.KratosBrowserURL = kratosCfg.BrowserURL
	webDeps.WishlistItems = storeWishlistItems{store: a.Store}
}
```

### TEST_STRUCTURE (fake store + httptest, mirrored into internal/web)
```go
// SOURCE: internal/api/wishlist_items_test.go:1-90
package web_test // testpackage linter requires external test package

type fakeWishlistItemStore struct { /* ... */ }
type fakeLoginFlowProvider struct {
	flow *kratos.LoginFlow
	err  error
}
func (f fakeLoginFlowProvider) GetLoginFlow(context.Context, string, string) (*kratos.LoginFlow, error) {
	return f.flow, f.err
}
```

---

## Files to Change

| File | Action | Justification |
|---|---|---|
| `internal/config/config.go` | UPDATE | Add `WebPort` (`env:"WEB_PORT" default:"8083"`), `KratosBrowserURL` (`env:"KRATOS_BROWSER_URL"`) to `EnvVars` |
| `internal/config/features.go` | UPDATE | Add `WebConfig{Port string}`; register `"web"` feature (`Requires: ["kratos", "database"]`, no own `RequiredEnv`); extend `KratosConfig` with `BrowserURL string` (falls back to `PublicURL` when `KRATOS_BROWSER_URL` unset) |
| `internal/clients/kratosclient/kratosclient.go` | UPDATE | Add `GetLoginFlow(ctx, cookieHeader, flowID string) (*kratos.LoginFlow, error)`, mirroring `ToSession` exactly |
| `internal/web/doc.go` | CREATE | Package doc, mirroring `internal/api/doc.go`'s style |
| `internal/web/router.go` | CREATE | `RouterDeps`, `NewWebRouter` — `/login` mounted outside the `Auth` group, `/wishlist*` inside it |
| `internal/web/middleware.go` | CREATE | Web-local `RequestLogger` (trimmed) + `SecurityHeaders` |
| `internal/web/auth.go` | CREATE | `SessionValidator`, identity context key, `AuthenticationMiddleware` (redirects to our own `/login`, same-origin) |
| `internal/web/login.go` | CREATE | `LoginFlowProvider` interface, `GET /login` handler (redirect-to-Kratos when no `flow`, render-flow when present), `buildLoginFields` node-flattening helper |
| `internal/web/wishlist.go` | CREATE | `WishlistItem` DTO, `WishlistItemStore` interface, handlers, `MountWishlist` |
| `internal/web/templates.go` | CREATE | `//go:embed templates` + `html/template` parsing/render helper |
| `internal/web/templates/base.html` | CREATE | Layout: viewport meta, htmx + WebAwesome CDN tags, design-token-based custom CSS |
| `internal/web/templates/login.html` | CREATE | `<wa-card>` wrapping a form built generically from `LoginFields`, special-casing `code`→`wa-otp-input` |
| `internal/web/templates/wishlist.html` | CREATE | Full wishlist page — greeting, `<wa-badge>` item count, add-item `<wa-card>` form |
| `internal/web/templates/item_list.html` | CREATE | Partial: items as `<wa-card>`s in a responsive grid, `<wa-divider>` between sections, empty-state `<wa-callout>` |
| `internal/web/templates/item_row.html` | CREATE | Partial: one item read-mode, `<wa-icon>` edit/delete buttons |
| `internal/web/templates/item_edit_row.html` | CREATE | Partial: one item inline edit form |
| `internal/web/templates/error.html` | CREATE | `<wa-callout variant="danger">` error fragment |
| `internal/web/login_test.go` | CREATE | Redirect-when-no-flow, render-when-flow-present tests |
| `internal/web/wishlist_test.go` | CREATE | Fake-store CRUD handler tests |
| `internal/web/auth_test.go` | CREATE | Redirect-to-/login-on-no-session tests |
| `internal/app/servers.go` | UPDATE | Add `Servers.Web`, wire `"web"` feature, `Shutdown` server slice/channel size 3→4 |
| `internal/app/servers_test.go` | UPDATE | Add web-router coverage |
| `cmd/start.go` | UPDATE | Add `webApi` subcommand, `selectWeb`, include `Web` in `selectAll` |
| `.env.example` | UPDATE | Add `WEB_PORT=8083`; add `KRATOS_BROWSER_URL=` (comment: optional, defaults to `KRATOS_PUBLIC_URL`) |
| `deploy/kratos/docker-compose.kratos.yml` | UPDATE | **Remove** the `kratos-selfservice-ui-node` service entirely (port 4455 freed) |
| `deploy/kratos/kratos.yml` | UPDATE | `login.ui_url` → `http://127.0.0.1:8083/login`; `default_browser_return_url` → `http://127.0.0.1:8083/wishlist`; `allowed_return_urls` += `http://127.0.0.1:8083`; disable `recovery`/`verification` (`enabled: false` — semantically inapplicable to passwordless SMS, see Decisions) |
| `deploy/kratos/kratos.production.yml` | UPDATE | Same repointing with `https://app.${DOMAIN}`; `serve.public.cors.allowed_origins` drops `login.${DOMAIN}`, adds `app.${DOMAIN}`; disable `recovery`/`verification` |
| `deploy/production/docker-compose.yml` | UPDATE | **Remove** `kratos-selfservice-ui-node` service; add `KRATOS_BROWSER_URL: https://auth.${DOMAIN}` to the `unwrap-gift` service's `environment:` (reusing the exact value the removed service used) |
| `deploy/production/Caddyfile` | UPDATE | **Remove** `login.{$DOMAIN}` block; **add** `app.{$DOMAIN} { reverse_proxy unwrap-gift:8083 }` |
| `deploy/kratos/README.md` | UPDATE | Manual walkthrough now visits `http://127.0.0.1:8083/login`, not `:4455` |
| `ARCHITECTURE.md` | UPDATE | One short paragraph documenting the fourth ("web") router |

## NOT Building

- **Registration UI** — identities remain organiser-provisioned only (`deploy/kratos/README.md`'s
  existing, unchanged Decisions Log entry); no self-service sign-up.
- **Recovery/verification/settings UI** — semantically inapplicable to a passwordless-SMS-only,
  organiser-provisioned identity model (no password to recover, no email to verify); their Kratos
  flows are disabled outright rather than left half-configured (see Decisions).
- **Logout button** — not in the PRD's MVP scope.
- **Organiser-facing web UI** — explicitly out of v1 scope per the PRD's MoSCoW table.
- **A `/health` endpoint on the web router** — the other three routers' health checks remain the
  orchestration signal.
- **CSRF token middleware of our own** — Kratos's own CSRF cookie protects the direct-to-Kratos form
  submission (see Decisions #4); our own `/wishlist` mutating endpoints rely on `SameSite=Lax` on the
  Kratos session cookie, same reasoning as the prior draft.
- **`wa-page`'s full app-shell layout (nav/sidebar/drawer)** — considered; this app is two pages with
  no navigation structure to speak of, so the lighter `wa-stack`/`wa-cluster` utility classes plus
  `wa-card` are a better fit than a full page-shell component.
- **A JS bundler/build step, npm, or `package.json`** — htmx and WebAwesome load from CDN tags only.

---

## Decisions Confirmed (recommended options, not re-asked)

1. **Build our own login UI, not `kratos-selfservice-ui-node`** — see the dedicated section above.
   The reference UI is removed from both compose stacks.
2. **Templating**: Go stdlib `html/template` via `embed.FS`, not `templ` — zero new Go dependency.
3. **Login flow rendering is generic, not hardcoded per step**: `GET /login?flow=X` always re-fetches
   the current flow and renders whatever `ui.nodes` it has, via a small `buildLoginFields` helper
   (Go code, not template logic) that flattens `[]kratos.UiNode` into `[]loginField{Name, Type,
   Value, Label, Hidden, Required}` — hidden-type nodes (csrf_token, resubmitted identifier) always
   render as plain `<input type="hidden">`; a node named `code` renders as `<wa-otp-input>`; a
   `submit`-type node renders as `<wa-button>`; everything else renders as `<wa-input>`. This means
   step 1 (phone entry), step 2 (code entry), and any Kratos-added node (e.g. resend) all render
   correctly from the same handler/template with no per-step branching.
4. **CSRF cookie reasoning**: Kratos's CSRF cookie is set by `/self-service/login/browser` for
   Kratos's **own** host (the browser-facing Kratos URL) and is only ever read back by Kratos itself
   on the direct-to-Kratos form POST — it never needs to reach our web app's origin, so there is no
   cross-origin cookie concern for the login step regardless of whether our app and Kratos share a
   top-level domain. The **session** cookie (set on successful login) already uses
   `session.cookie.domain: ${DOMAIN}` in `kratos.production.yml:105` — a parent-domain cookie
   deliberately shared across subdomains, already correctly configured for `app.${DOMAIN}` to read it.
5. **Data access from the web router**: web handlers call `WishlistItemStore` directly in Go (same
   process), not an HTTP round-trip to the JSON API — unchanged from the prior draft.
6. **htmx error handling**: htmx-targeted endpoints always respond 200, embedding the error state
   in-band — unchanged from the prior draft.
7. **Disable `recovery`/`verification` Kratos flows** rather than leave their `ui_url`s pointing at
   nothing: they don't apply to this identity/auth model (no password, no email), and leaving them
   configured-but-dead is more confusing than turning them off. `settings` stays enabled (harmless,
   theoretically reachable via Kratos's admin API even with no UI) but its `ui_url` is left pointing
   at the web app's root as an inert placeholder — nothing in this plan ever triggers it.
8. **`KRATOS_BROWSER_URL` is optional, not a new required-env gate**: when unset, it falls back to
   `KRATOS_PUBLIC_URL` — correct for local dev, where both already point at the same
   browser-reachable `localhost:4433`. Production must set it explicitly (`https://auth.${DOMAIN}`,
   the exact value the now-removed `kratos-selfservice-ui-node` service used for the same purpose),
   since `KRATOS_PUBLIC_URL` there is a container-internal hostname (`http://kratos:4433`) the
   browser can never reach directly.

---

## Step-by-Step Tasks

### Task 1: Config — WebConfig + "web" feature
- **ACTION**: Add `WebPort` to `EnvVars`; add `WebConfig` struct + register `"web"` feature.
- **IMPLEMENT**:
  ```go
  // internal/config/config.go
  WebPort string `env:"WEB_PORT" default:"8083"`
  ```
  ```go
  // internal/config/features.go
  type WebConfig struct{ Port string }

  r.Register(Feature{
	  Name:     "web",
	  Requires: []string{"kratos", "database"},
	  Build:    func(e EnvVars) any { return WebConfig{Port: e.WebPort} },
  })
  ```
- **MIRROR**: `KetoConfig`'s `Requires: []string{"kratos"}` pattern; `ServiceConfig`'s no-`RequiredEnv` shape (always resolves once its `Requires` do).
- **GOTCHA**: no `RequiredEnv` here — "web" is enabled automatically whenever kratos+database are,
  unlike the prior draft's `WebLoginURL`-gated design (removed; no longer needed now that login is
  self-hosted).
- **VALIDATE**: `go test ./internal/config/...` — "web" enabled iff kratos+database both enabled.

### Task 2: Config — KratosConfig gains BrowserURL
- **ACTION**: Add `KratosBrowserURL` to `EnvVars`; extend `KratosConfig`'s `Build` with a fallback.
- **IMPLEMENT**:
  ```go
  // internal/config/config.go
  KratosBrowserURL string `env:"KRATOS_BROWSER_URL"`
  ```
  ```go
  // internal/config/features.go — KratosConfig gains BrowserURL string; Build becomes:
  Build: func(e EnvVars) any {
	  browserURL := e.KratosBrowserURL
	  if browserURL == "" {
		  browserURL = e.KratosPublicURL
	  }
	  return KratosConfig{
		  PublicURL: e.KratosPublicURL, AdminURL: e.KratosAdminURL,
		  CourierWebhookSecret: e.KratosCourierWebhookSecret, BrowserURL: browserURL,
	  }
  },
  ```
- **MIRROR**: `deploy/kratos/docker-compose.kratos.yml`'s `kratos-selfservice-ui-node` service, which
  already draws exactly this two-URL distinction (`KRATOS_PUBLIC_URL` server-internal vs
  `KRATOS_BROWSER_URL` browser-facing) — this task generalizes a pattern that already existed in this
  repo's deploy config, just not in Go code, since the service being removed was the only consumer.
- **VALIDATE**: config test — `KratosBrowserURL` unset → `KratosConfig.BrowserURL == PublicURL`; set → used as-is.

### Task 3: kratosclient — GetLoginFlow
- **ACTION**: Add a method to `internal/clients/kratosclient/kratosclient.go`.
- **IMPLEMENT**:
  ```go
  func (c *Client) GetLoginFlow(ctx context.Context, cookieHeader, flowID string) (*kratos.LoginFlow, error) {
	  flow, resp, err := c.Public.FrontendAPI.GetLoginFlow(ctx).Id(flowID).Cookie(cookieHeader).Execute()
	  closeBody(resp)
	  if err != nil {
		  return nil, fmt.Errorf("kratosclient: get login flow: %w", err)
	  }
	  return flow, nil
  }
  ```
- **MIRROR**: `ToSession` (kratosclient.go:67-82) verbatim shape.
- **GOTCHA**: no sentinel-error mapping needed here (unlike `GetIdentity`'s `ErrIdentityNotFound`) —
  an expired/invalid flow ID is a normal, expected user-facing case (flow expired while they were
  slow to respond), not a 404-worthy programming error; `internal/web/login.go` (Task 7) handles it
  by re-redirecting to start a fresh flow, not by propagating a typed error.
- **VALIDATE**: this method needs no new unit test file (no branching logic beyond the existing
  `closeBody`/wrap pattern already covered by `ToSession`'s tests) — covered by `internal/web`'s
  own fake-provider tests (Task 12) instead.

### Task 4: `internal/web` package skeleton + doc.go
- **ACTION**: Create the package with a doc comment.
- **IMPLEMENT**: `internal/web/doc.go` mirroring `internal/api/doc.go` — states that `internal/web`
  never imports `internal/api`/`internal/clients`/`internal/db` (ARCHITECTURE.md), and that it owns
  a from-scratch Kratos browser-flow login integration (not delegated to a separate UI service).
- **MIRROR**: `internal/api/doc.go:1-10`.
- **VALIDATE**: `go build ./internal/web/...`.

### Task 5: Web-local middleware
- **ACTION**: Create `internal/web/middleware.go` with trimmed `RequestLogger`/`SecurityHeaders`.
- **IMPLEMENT**: same as the prior draft — method/path/status/duration/request_id/trace_id logging
  (no Problem-JSON parsing), verbatim `SecurityHeaders`.
- **MIRROR**: `internal/api/middleware.go:62-95` (trim), `:130-142` (verbatim).
- **GOTCHA**: no `EnforceJSON`.
- **VALIDATE**: unit test asserting INFO for 200, WARN for 4xx.

### Task 6: Auth — SessionValidator, context key, AuthenticationMiddleware
- **ACTION**: Create `internal/web/auth.go`.
- **IMPLEMENT**:
  ```go
  type identityContextKey struct{}
  func withIdentity(ctx context.Context, identity *kratos.Identity) context.Context { /* ... */ }
  func IdentityFromContext(ctx context.Context) (*kratos.Identity, bool) { /* ... */ }

  type SessionValidator interface {
	  ToSession(ctx context.Context, cookieHeader string) (*kratos.Session, error)
  }

  func AuthenticationMiddleware(validator SessionValidator) func(http.Handler) http.Handler {
	  return func(next http.Handler) http.Handler {
		  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			  if validator == nil {
				  next.ServeHTTP(w, r)
				  return
			  }
			  cookie := r.Header.Get("Cookie")
			  if cookie != "" {
				  if session, err := validator.ToSession(r.Context(), cookie); err == nil && session.Identity != nil {
					  next.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), session.Identity)))
					  return
				  }
			  }
			  returnTo := url.QueryEscape(r.URL.RequestURI()) // same-origin, relative is enough
			  http.Redirect(w, r, "/login?return_to="+returnTo, http.StatusSeeOther)
		  })
	  }
  }
  ```
- **MIRROR**: `internal/api/auth.go:10-76`.
- **GOTCHA (loop prevention)**: `/login` must NEVER be mounted inside the group this middleware
  guards — confirm in Task 10's router wiring. An accidental overlap creates an infinite redirect.
- **GOTCHA (simplified vs. prior draft)**: this redirects to our own **relative** `/login` path, not
  an absolute external URL — no `X-Forwarded-Proto`/absolute-URL construction needed here anymore
  (that complexity moves to `login.go`, Task 7, which only needs it once).
- **VALIDATE**: nil validator passes through; no/invalid cookie → 303 to `/login?return_to=...`; valid cookie → identity in context.

### Task 7: Login — LoginFlowProvider, GET /login handler, field-flattening helper
- **ACTION**: Create `internal/web/login.go`.
- **IMPLEMENT**:
  ```go
  type LoginFlowProvider interface {
	  GetLoginFlow(ctx context.Context, cookieHeader, flowID string) (*kratos.LoginFlow, error)
  }

  type loginField struct {
	  Name, Type, Value, Label string
	  Hidden, Required         bool
  }

  func buildLoginFields(nodes []kratos.UiNode) []loginField {
	  fields := make([]loginField, 0, len(nodes))
	  for _, node := range nodes {
		  attrs := node.Attributes.UiNodeInputAttributes
		  if attrs == nil {
			  continue // non-input nodes (e.g. UiNodeTextAttributes) — none expected for the code method
		  }
		  f := loginField{Name: attrs.Name, Type: attrs.Type, Hidden: attrs.Type == "hidden"}
		  if attrs.Value != nil {
			  f.Value = fmt.Sprint(attrs.Value)
		  }
		  if attrs.Required != nil {
			  f.Required = *attrs.Required
		  }
		  if node.Meta.Label != nil {
			  f.Label = node.Meta.Label.Text
		  }
		  fields = append(fields, f)
	  }
	  return fields
  }

  func MountLogin(r chi.Router, flows LoginFlowProvider, kratosBrowserURL string, templates *template.Template) {
	  r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		  flowID := r.URL.Query().Get("flow")
		  if flowID == "" {
			  returnTo := r.URL.Query().Get("return_to")
			  if returnTo == "" {
				  returnTo = "/wishlist"
			  }
			  absoluteReturnTo := (&url.URL{Scheme: schemeOf(r), Host: r.Host, Path: returnTo}).String()
			  target := kratosBrowserURL + "/self-service/login/browser?return_to=" + url.QueryEscape(absoluteReturnTo)
			  http.Redirect(w, r, target, http.StatusSeeOther)
			  return
		  }

		  flow, err := flows.GetLoginFlow(r.Context(), r.Header.Get("Cookie"), flowID)
		  if err != nil {
			  // Expired/invalid flow — start a fresh one rather than show an error.
			  http.Redirect(w, r, "/login", http.StatusSeeOther)
			  return
		  }

		  data := struct {
			  Fields   []loginField
			  Action   string
			  Method   string
			  Messages []kratos.UiText
		  }{Fields: buildLoginFields(flow.Ui.Nodes), Action: flow.Ui.Action, Method: flow.Ui.Method, Messages: flow.Ui.Messages}
		  _ = templates.ExecuteTemplate(w, "login.html", data)
	  })
  }
  ```
- **MIRROR**: Task 3's `GetLoginFlow` shape; `buildLoginFields`'s loop structure mirrors
  `internal/api/wishlist_items.go`'s `toAPIWishlistItem`-style conversion-function pattern (plain
  loop, no error unless genuinely exceptional).
- **IMPORTS**: `context`, `fmt`, `html/template`, `net/http`, `net/url`, `github.com/go-chi/chi/v5`,
  `kratos "github.com/ory/kratos-client-go/v26"`.
- **GOTCHA**: `schemeOf(r)` needs a small helper (`if r.Header.Get("X-Forwarded-Proto") != "" { return r.Header.Get("X-Forwarded-Proto") }; return "http"`) — Caddy's default `reverse_proxy` sets this header, confirmed by Caddy's own docs; dev has no TLS so this correctly falls through to `"http"`.
- **GOTCHA**: `kratosBrowserURL` must be the **browser-reachable** Kratos URL (Task 2's `KratosBrowserURL`/fallback), never the server-internal `KratosPublicURL` directly in production — using the wrong one here means the user's browser gets redirected to an unreachable container hostname.
- **VALIDATE**: table test — no `flow` param → 303 to `{kratosBrowserURL}/self-service/login/browser?return_to=...`; `flow` param + provider returns a flow → 200 with rendered fields; `flow` param + provider errors (expired) → 303 to `/login` (fresh start, no error shown to user — matches Kratos's own UX convention for expired flows).

### Task 8: Wishlist DTO, store interface, handlers, mount
- **ACTION**: Create `internal/web/wishlist.go` — unchanged in substance from the prior draft.
- **IMPLEMENT**: `WishlistItem` struct, `WishlistItemStore` interface (identical method set to
  `api.WishlistItemStore`), `MountWishlist(r, items, templates, adapter)` registering `GET/POST
  /wishlist`, `GET /wishlist/{id}/edit`, `PUT/DELETE /wishlist/{id}`.
- **MIRROR**: `internal/api/wishlist_items.go:100-198`.
- **IMPORTS**: `html/template`, `net/http`, `strconv`, `strings`, `github.com/go-chi/chi/v5`, `kratos "github.com/ory/kratos-client-go/v26"`.
- **GOTCHA**: `phoneTraitFromIdentity` reimplemented locally (unexported in `api`, can't be imported) — same as prior draft.
- **GOTCHA**: store errors render in-band at 200 (Decision #6), not via `Adapter`'s 500 path.
- **VALIDATE**: fake-store tests per handler, success + error cases.

### Task 9: Templates — heavy WebAwesome usage, mobile-first
- **ACTION**: Create all template files + `internal/web/templates.go`.
- **IMPLEMENT**:
  ```go
  //go:embed templates
  var templateFS embed.FS

  func ParseTemplates() (*template.Template, error) {
	  return template.ParseFS(templateFS, "templates/*.html")
  }
  ```
  `base.html`'s `<head>` — **mobile-first is non-negotiable here**, add the viewport meta tag (the
  prior draft omitted this — corrected):
  ```html
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <script type="module" src="https://ka-f.webawesome.com/{VERSION}/webawesome.loader.js"></script>
  <link rel="stylesheet" href="https://ka-f.webawesome.com/{VERSION}/styles/webawesome.css">
  <script src="https://unpkg.com/htmx.org@{VERSION}/dist/htmx.min.js"></script>
  <style>
	  body { margin: 0; background: var(--wa-color-neutral-fill-quiet); }
	  .page { max-width: 480px; margin-inline: auto; padding: var(--wa-space-m); }
  </style>
  ```
  (exact CDN versions TBD at implementation time — see the brief's original GOTCHA, unchanged).

  `login.html` — WebAwesome-heavy, generic field rendering:
  ```html
  {{template "base.html" .}}
  <div class="page">
	<wa-card class="login-card">
	  <div slot="header"><wa-icon name="gift"></wa-icon> Secret Santa — Sign in</div>
	  {{range .Messages}}<wa-callout variant="{{if eq .Type "error"}}danger{{else}}brand{{end}}">{{.Text}}</wa-callout>{{end}}
	  <form action="{{.Action}}" method="{{.Method}}">
		{{range .Fields}}
		  {{if .Hidden}}
			<input type="hidden" name="{{.Name}}" value="{{.Value}}">
		  {{else if eq .Name "code"}}
			<wa-otp-input name="{{.Name}}" length="6" label="{{.Label}}" autofocus></wa-otp-input>
		  {{else if eq .Type "submit"}}
			<wa-button type="submit" variant="brand" appearance="filled">{{.Label}}</wa-button>
		  {{else}}
			<wa-input name="{{.Name}}" type="tel" label="{{.Label}}" {{if .Required}}required{{end}} autofocus></wa-input>
		  {{end}}
		{{end}}
	  </form>
	</wa-card>
  </div>
  ```
  `wishlist.html`/`item_list.html`/`item_row.html` — heavy WebAwesome usage per the confirmed
  component set: `<wa-card>` per item (grid layout via a `wa-grid`/custom CSS grid, single-column on
  phones), `<wa-badge>` for the item count next to the greeting, `<wa-divider>` between the add-item
  form and the list, `<wa-icon name="pen">`/`<wa-icon name="trash">` on edit/delete `<wa-button
  appearance="plain">` icon buttons, `<wa-callout variant="neutral">` empty-state when the list is
  empty, `<wa-spinner>` shown via htmx's `hx-indicator` during in-flight requests.
- **MIRROR**: no existing precedent (first frontend surface) — follow htmx's "hypermedia" pattern for
  partial-swap structure; follow WebAwesome's documented component attributes verbatim (Patterns above).
- **IMPORTS**: `embed`, `html/template`.
- **GOTCHA**: `html/template`'s auto-escaping is a real XSS defense for user-supplied item names/URLs
  — never bypass with `template.HTML()` on user input.
- **GOTCHA**: `<wa-otp-input>`'s exact `length` default is 6 per its docs, but Kratos's actual
  configured code length should be confirmed against a live flow during implementation (not assumed)
  — if it differs, set `length` to match or the input will visually mismatch the SMS code's digit count.
- **VALIDATE**: `ParseTemplates()` no error; render `login.html` with a 1-field and a 2-field fixture;
  render `wishlist.html` with 0 and N items.

### Task 10: Router — NewWebRouter
- **ACTION**: Create `internal/web/router.go`.
- **IMPLEMENT**:
  ```go
  type RouterDeps struct {
	  Logger           *slog.Logger
	  TracerProvider   trace.TracerProvider
	  Auth             func(http.Handler) http.Handler // nil when "web" feature disabled
	  LoginFlows       LoginFlowProvider                // nil when "web" feature disabled
	  KratosBrowserURL string
	  WishlistItems    WishlistItemStore
	  Templates        *template.Template
  }

  func NewWebRouter(deps RouterDeps) *chi.Mux {
	  r := chi.NewRouter()
	  r.Use(middleware.RequestID)
	  r.Use(RequestLogger(deps.Logger))
	  r.Use(middleware.Recoverer)
	  r.Use(middleware.Timeout(requestTimeout))
	  r.Use(SecurityHeaders)
	  r.Use(otelchi.Middleware("web", otelchi.WithChiRoutes(r), otelchi.WithTracerProvider(deps.TracerProvider)))

	  if deps.LoginFlows != nil { // NOT behind Auth — this is what Auth redirects TO
		  MountLogin(r, deps.LoginFlows, deps.KratosBrowserURL, deps.Templates)
	  }

	  r.Group(func(r chi.Router) {
		  if deps.Auth != nil {
			  r.Use(deps.Auth)
		  }
		  if deps.WishlistItems != nil {
			  MountWishlist(r, deps.WishlistItems, deps.Templates, Adapter{Logger: deps.Logger, Templates: deps.Templates})
		  }
	  })
	  r.Get("/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/wishlist", http.StatusSeeOther) })
	  return r
  }
  ```
- **MIRROR**: `internal/api/router.go:85-101`'s nil-guarded group-mount shape.
- **GOTCHA**: `/login` is mounted **outside** the `Auth`-guarded group — this is the loop-prevention
  invariant Task 6 flagged. Double-check this explicitly in review/tests, not just by eye.
- **VALIDATE**: `/` redirects to `/wishlist`; `/login` reachable with no session; `/wishlist` without
  a session redirects to `/login` (not a loop — confirm via a test that follows at most one redirect).

### Task 11: Wire into `internal/app`
- **ACTION**: Update `internal/app/servers.go` and `cmd/start.go`.
- **IMPLEMENT**: Add `Web *http.Server` to `Servers`; always construct it (mirroring Public/
  Protected/Hidden):
  ```go
  webFeature, _ := a.Registry.Feature("web")
  webCfg, _ := webFeature.Config.(config.WebConfig)
  webDeps := web.RouterDeps{Logger: a.Logger, TracerProvider: a.Otel.TracerProvider}
  if webFeature.Enabled {
	  templates, err := web.ParseTemplates()
	  if err != nil {
		  return nil, fmt.Errorf("app: parse web templates: %w", err)
	  }
	  webDeps.Templates = templates
	  webDeps.Auth = web.AuthenticationMiddleware(a.Kratos)
	  webDeps.LoginFlows = a.Kratos
	  webDeps.KratosBrowserURL = kratosCfg.BrowserURL
	  webDeps.WishlistItems = storeWishlistItems{store: a.Store}
  }
  webPort := webCfg.Port
  if webPort == "" {
	  webPort = "8083"
  }
  ```
  Update `Shutdown`'s server slice/channel size 3→4 (unchanged mechanics from prior draft). In
  `cmd/start.go`: add `selectWeb`, a `webApi` subcommand, include `Web` in `selectAll`.
- **MIRROR**: `internal/app/servers.go:146-165` (`wireWishlistItems`); `cmd/start.go:36-61`.
- **IMPORTS**: `"github.com/DanielKirkwood/unwrap-gift/internal/web"` added to `internal/app/servers.go`.
- **GOTCHA**: `a.Kratos` (a `*kratosclient.Client`) must structurally satisfy both the new
  `web.SessionValidator` (`ToSession`) and `web.LoginFlowProvider` (`GetLoginFlow`, added in Task 3)
  — confirm `go build` catches it if either method signature drifts.
- **VALIDATE**: `go build ./...`; new `servers_test.go` case: web feature disabled → `servers.Web`
  non-nil, `/wishlist` and `/login` both 404 (routes never mounted, matching the existing disabled-
  feature pattern).

### Task 12: Tests
- **ACTION**: `internal/web/{login,wishlist,auth,router}_test.go`.
- **IMPLEMENT**: fake `LoginFlowProvider`/`WishlistItemStore`/`SessionValidator` per TEST_STRUCTURE;
  assert HTML fragment content via string-contains.
- **MIRROR**: `internal/api/wishlist_items_test.go` in full for the wishlist handlers.
- **VALIDATE**: `go test ./internal/web/... -v`.

### Task 13: Deploy config — dev
- **ACTION**: Update `deploy/kratos/docker-compose.kratos.yml`, `deploy/kratos/kratos.yml`, `.env.example`.
- **IMPLEMENT**: Remove the `kratos-selfservice-ui-node` service block entirely (and its `4455:3000`
  port mapping) from `docker-compose.kratos.yml`. In `kratos.yml`: `login.ui_url:
  http://127.0.0.1:8083/login`; `default_browser_return_url: http://127.0.0.1:8083/wishlist`;
  `allowed_return_urls` += `http://127.0.0.1:8083`; `recovery.enabled: false`,
  `verification.enabled: false`. `.env.example` gains `WEB_PORT=8083` and a commented
  `KRATOS_BROWSER_URL=` note (optional, defaults to `KRATOS_PUBLIC_URL`).
- **VALIDATE**: `task kratos:up && task run`, then manually: visit `http://127.0.0.1:8083/wishlist`
  logged out → redirected to `/login` → redirected to Kratos → back to `/login?flow=X` showing the
  phone input → submit → `wa-otp-input` step appears → enter code (logged to stdout per existing dev
  convention, `sms` feature disabled) → redirected to `/wishlist`, logged in.

### Task 14: Deploy config — production
- **ACTION**: Update `deploy/kratos/kratos.production.yml`, `deploy/production/Caddyfile`,
  `deploy/production/docker-compose.yml`.
- **IMPLEMENT**: Remove the `kratos-selfservice-ui-node` service from `docker-compose.yml`. Add
  `KRATOS_BROWSER_URL: https://auth.${DOMAIN}` to the `unwrap-gift` service's `environment:` (the
  exact value the removed service used for its own `KRATOS_BROWSER_URL`). `kratos.production.yml`:
  `login.ui_url: https://app.${DOMAIN}/login`; `default_browser_return_url:
  https://app.${DOMAIN}/wishlist`; `allowed_return_urls` → drop `login.${DOMAIN}`, add
  `app.${DOMAIN}`; `cors.allowed_origins` same swap; `recovery.enabled`/`verification.enabled: false`.
  Caddyfile: remove the `login.{$DOMAIN}` block, add `app.{$DOMAIN} { reverse_proxy unwrap-gift:8083 }`.
- **GOTCHA**: confirm `session.cookie.domain: ${DOMAIN}` (unchanged, already correct per Decision #4)
  actually covers `app.${DOMAIN}` once that subdomain exists — this is config that already should
  work, but "should" isn't "confirmed against a real deploy."
- **VALIDATE**: manual, documented in the PR description — not automatable in CI.

### Task 15: Update `deploy/kratos/README.md`
- **ACTION**: Replace the manual browser-login walkthrough.
- **IMPLEMENT**: Change references from `http://localhost:4455/...` to
  `http://127.0.0.1:8083/login`; remove any remaining mention of the self-service-ui-node container
  as part of the standard dev loop (the existing curl-based **native**-flow walkthrough for
  scripting/testing stays valid and unchanged — it never used the removed service).
- **VALIDATE**: read-through only; no automated check.

---

## Testing Strategy

### Unit Tests

| Test | Input | Expected Output | Edge Case? |
|---|---|---|---|
| `TestMountLogin_NoFlow_RedirectsToKratosBrowser` | `GET /login` | 303 to `{kratosBrowserURL}/self-service/login/browser?return_to=...` | |
| `TestMountLogin_FlowPresent_RendersFields` | `GET /login?flow=X`, fake provider returns a 2-node flow | 200, body contains both field names | |
| `TestMountLogin_FlowExpired_RedirectsFresh` | fake provider returns error | 303 to `/login` (no flow param) | invalid/expired flow |
| `TestBuildLoginFields_CodeNode_MapsToOtpInput` | a node named `code` | resulting field has `Name == "code"` (template special-cases it) | |
| `TestBuildLoginFields_HiddenNode_MarkedHidden` | a node with `type: "hidden"` | `Hidden == true` | csrf_token, resubmitted identifier |
| `TestAuthenticationMiddleware_NilValidator` | nil validator | `next` called, no redirect | |
| `TestAuthenticationMiddleware_NoCookie` | no `Cookie` header | 303 to `/login?return_to=...` | |
| `TestNewWebRouter_LoginNotBehindAuth` | `GET /login` with no session cookie, `Auth` set | 200 or 303-to-Kratos, **not** 303-to-`/login` (would be a loop) | loop-prevention |
| `TestMountWishlist_Create_Success` | valid form POST | 200, item list partial contains new item | |
| `TestMountWishlist_Update_NotOwned` | store returns not-found-equivalent error | 200, error callout, no panic | |
| `TestBuildServers_WebFeatureDisabled` | kratos or database disabled | `servers.Web` non-nil, `/login` and `/wishlist` both 404 | |

### Edge Cases Checklist
- [x] Empty input (blank item name)
- [x] Expired/invalid Kratos login flow (Task 7's redirect-to-fresh behavior)
- [x] Invalid types (non-numeric `id` path param)
- [ ] Concurrent access (SQLite single-writer semantics already handle this)
- [x] Permission denied / not-owned item (render error in-band)
- [x] Login redirect loop (explicit test, Task 10/12)

---

## Validation Commands

### Static Analysis
```bash
go vet ./...
golangci-lint run
```
EXPECT: zero issues.

### Unit Tests
```bash
go test ./internal/web/... ./internal/config/... ./internal/clients/kratosclient/... ./internal/app/... -v
```
EXPECT: all pass.

### Full Test Suite
```bash
task test
```
EXPECT: no regressions.

### Browser Validation
```bash
task auth:up
task db:migrate
task run
```
EXPECT: full login round-trip (phone → SMS code logged to stdout → OTP entry → wishlist), add/edit/
delete via htmx partial swaps, no full page reloads except the Kratos redirect hops.

### Manual Validation
- [ ] Logged-out visit to `/wishlist` → `/login` → Kratos → back to `/login?flow=X`, phone form shown
- [ ] Submitting phone number shows the `wa-otp-input` step on the same page
- [ ] On a real Android device with the `sms` feature enabled, confirm WebOTP autofill actually
      triggers (cannot be verified in dev with SMS logged to stdout — flag as a production-only check)
- [ ] Successful code entry redirects to `/wishlist`, logged in, greeting shows the correct full name
- [ ] Add/edit/delete all work via htmx without full page reloads
- [ ] A second identity cannot see/edit the first identity's items
- [ ] Every page is usable and legible at a 375px-wide viewport (iPhone SE class)
- [ ] No `kratos-selfservice-ui-node` container is running after `task kratos:up` (confirms clean removal)

---

## Acceptance Criteria
- [ ] All tasks completed
- [ ] All validation commands pass
- [ ] Tests written and passing
- [ ] No type errors (`go vet`)
- [ ] No lint errors (`golangci-lint run`)
- [ ] Matches UX design: own-brand login (phone → OTP) and wishlist CRUD, both WebAwesome-styled, mobile-first

## Completion Checklist
- [ ] Code follows discovered patterns (four-router model; interface segregation; nil-is-disabled
      feature gating; generic flow-node rendering, not per-step hardcoding)
- [ ] `internal/web` does not import `internal/api`, `internal/clients`, or `internal/db` anywhere
      (`go list -deps ./internal/web/... | grep -E 'internal/(api|clients|db)'` → no output)
- [ ] `kratos-selfservice-ui-node` fully removed from both compose stacks, both Caddyfiles/configs updated consistently
- [ ] `<meta name="viewport">` present; spot-checked at phone width
- [ ] WebAwesome components used for every interactive/visual element where a suitable one exists
      (card, callout, icon, divider, spinner, badge, button, input, otp-input) — custom CSS limited
      to layout (container width, grid) and design-token references, not component reimplementation
- [ ] No hardcoded CDN version strings left unresolved
- [ ] `ARCHITECTURE.md` updated with the fourth router
- [ ] No unnecessary scope additions (no registration, no recovery/verification/settings UI)
- [ ] Self-contained — no questions needed during implementation

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Hand-built Kratos flow integration misses an edge case the reference UI handled (e.g. a specific error-message node shape, flow refresh semantics) | M | M | The generic `buildLoginFields` rendering (Task 7/9) is deliberately permissive — it renders whatever nodes/messages Kratos sends rather than assuming a fixed shape, which covers most drift; still, test against a **live** Kratos instance (Task 13's manual validation), not just fakes, before calling this phase done |
| `session.cookie.domain`/`app.${DOMAIN}` alignment in production (Decision #4's reasoning, unverified against a real deploy) | L (reasoning is sound and matches already-existing config) | H if wrong | Explicit manual check in Task 14 |
| WebOTP autofill doesn't actually trigger in practice (carrier SMS format, Android version, browser) | M | L (graceful degradation — `wa-otp-input` still works as a manual 6-digit entry either way) | Manual validation checklist item explicitly calls this out as unverifiable in dev; treat as a nice-to-have, not a hard requirement |
| `funlen`/`cyclop` lint failures on `login.go`'s flow-fetch handler or `wishlist.go`'s CRUD handlers | M | L | Split helpers early (`buildLoginFields` is already split out for exactly this reason) |
| CDN unavailability (htmx/WebAwesome hosts down) breaks the entire UI, **including login** now (previously only wishlist) | L | M (slightly higher blast radius than the delegated-login design, since login itself now depends on the same CDN) | Out of scope to self-host for v1 given the tiny, known user base; worth noting as a slightly increased (but still low-probability) risk versus the prior draft |

## Notes

This plan reverses an earlier draft's decision to delegate login to `kratos-selfservice-ui-node`.
That reversal came from two concrete facts surfaced on review, not just preference: WebAwesome has a
purpose-built `wa-otp-input` component with WebOTP autofill support (verified against
webawesome.com/docs), and Kratos's "bring your own UI" browser-flow pattern is well-documented and
already fully researched (both from the original Phase 8 planning pass and the Go SDK's actual
installed struct shapes, verified directly against
`$(go env GOMODCACHE)/github.com/ory/kratos-client-go/v26@v26.2.0/` during this rewrite). The
generic, node-driven rendering approach (Task 7/9) is the single most load-bearing design decision in
this plan — it's what makes "build our own" tractable without reimplementing Kratos's step logic by
hand.
