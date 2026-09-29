# Architecture

Three conventions apply across this codebase, from the first commit onward — not just in the
packages where they happen to be visible today.

## `internal/`, not `pkg/`

`pkg/` is an unofficial convention that signals "importable by anyone." Nothing in this module is
meant to be imported by another module. `internal/` is enforced by the Go compiler and correctly
says "application-private."

## Dependency direction

`internal/app` is the composition root: it constructs dependencies and wires them together
explicitly. Only `app` imports `internal/{api,db,clients,tasks}` — never the reverse. Keeping this
one-directional avoids import cycles once handlers need client/config access.

## Dependency injection via constructors, no singletons

Dependencies (the DB `Store`, and later the Kratos/Keto clients) are constructed once in `app` and
passed down explicitly through constructors — not fetched via package-level `GetXClient()`-style
singleton getters. This is what makes each piece testable as a plain Go value instead of requiring
global state to be mocked.

## The three-router model

The app runs three independent `chi.Mux` routers, each its own port and `http.Server`
(`internal/api/router.go`, wired up in `internal/app/servers.go`):

- **Public** (`PUBLIC_PORT`, default `8080`) — the externally-facing surface. Only `/health/*`
  today.
- **Protected** (`PROTECTED_PORT`, default `8081`) — authenticated, non-admin endpoints. Every
  route other than `/health/*` runs behind `deps.Auth`. The widget CRUD example lives here.
- **Hidden** (`HIDDEN_PORT`, default `8082`) — admin-only endpoints, meant to be bound to an
  interface not exposed to the public internet (a private network, a sidecar, an SSH tunnel — not
  a public DNS name). Every route other than `/health/*` runs behind `deps.Auth`, then
  `deps.Authz`. The admin identity CRUD example lives here.

Separate servers rather than one router with middleware groups means a misconfigured reverse
proxy can only ever leak the hidden router's surface if its port is actually published — see
[DEPLOYMENT.md](DEPLOYMENT.md) for how the production Caddy config only exposes the protected
router (`api.$DOMAIN` → `unwrap-gift:8081`), never public or hidden.

## Feature flags and the "nil is disabled" convention

`internal/config.EnvVars` (populated from env vars via Viper + reflection) is the single source of
truth for configuration. `internal/config/features.go` derives per-feature configs
(`ServiceConfig`/`DatabaseConfig`/`OtelConfig`/`KratosConfig`/`KetoConfig`) from it, and
`internal/config/registry.go`'s `Registry` resolves which features are actually enabled: a feature
is enabled once every env var in its `RequiredEnv` is set (`ServiceConfig` has none, so it's always
on), a feature can declare `Requires` on another (`keto` requires `kratos`, since Keto relation
tuples reference Kratos identity IDs), and `DISABLE_FEATURES` (comma-separated) force-disables
anything regardless of env vars.

The consuming convention everywhere else in the codebase is: **a disabled feature's dependency is
`nil`**, not a separate boolean flag checked at every call site. `db.Store`, `*kratosclient.Client`,
`*ketoclient.Client`, `otelclient`'s providers, and the `Auth`/`Authz`/`Widgets`/`Identities` fields
on `api.RouterDeps` all follow this: every method that can be called on a disabled dependency is
nil-receiver-safe (for concrete types) or simply never wired in (for the `RouterDeps` function
fields, which the router constructors check with a plain `!= nil` before using). This keeps
feature-off code paths a single `nil` check at the point of use instead of an `if enabled` branch
threaded through every layer.

## Interface segregation for external dependencies

`internal/api` never imports `internal/clients` or `internal/db`. Instead it declares narrow
interfaces for exactly what it needs — `SessionValidator`, `PermissionChecker`, `WidgetStore`,
`IdentityAdmin` — and `internal/clients/kratosclient`, `internal/clients/ketoclient`, and
`internal/app` (via its `storeWidgets` adapter) implement them. `internal/app`, as the composition
root, is the only package that imports both sides and wires the concrete type into the interface.

This is the same dependency-direction rule above applied at the type level: it's what makes
`internal/api`'s handlers testable with a fake `WidgetStore`/`IdentityAdmin` instead of a real
database or a running Kratos instance, and it's the pattern to follow for any new external
dependency — declare the interface where it's consumed, not where it's implemented.

## Error and envelope model

Handlers in `internal/api` have the signature `HandlerFunc = func(w, r) error` instead of the
standard `http.HandlerFunc`. An `Adapter{Logger, ErrorsMap}` wraps each one into a real
`http.HandlerFunc`: a returned error is matched against the `Adapter`'s `ErrorsMap` (an ordered list
of `errors.Is` checks) and turned into an [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) Problem
Details response; anything unmapped becomes a logged 500. Success responses go through
`WriteData`, which wraps the payload as `{"data": ...}` — the same uniform shape `WriteProblem`
gives every error. This keeps handler bodies free of manual `w.WriteHeader`/`json.Marshal` calls
and error-response boilerplate.

## Observability wiring

OpenTelemetry traces and metrics run through the app whenever `OTEL_EXPORTER_OTLP_ENDPOINT` is
set (the `otel` feature's only required env var): `internal/clients/otelclient` builds a real
`TracerProvider`/`MeterProvider` over OTLP/HTTP; when the feature is disabled it returns no-op
providers instead, so every consumer (`otelchi` at the router level, `otelhttp` on outbound Kratos/
Keto clients, `otelsql` on the DB connection pool) can be wired unconditionally in `internal/app`
without a separate "is otel on" check. `internal/clients/logger` is separate: `log/slog`, JSON in
production, `tint`-colorized text in dev (auto-disabled when stdout isn't a terminal).

## Adding a new resource

Widgets (`internal/api/widgets.go`, `internal/app/widgets.go`, and
`internal/db/{migrations,queries,sqlc}/*widgets*`) is the worked example for adding a new
authenticated, database-backed resource. The layers, bottom-up:

1. **Migration** — `internal/db/migrations/00002_widgets.sql` (goose, `-- +goose Up`/`Down`).
2. **Query** — `internal/db/queries/widgets.sql`, sqlc-annotated SQL (`:one`/`:many`/`:exec`).
3. **Generated code** — `task sqlc:generate` produces `internal/db/sqlc/widgets.sql.go`; commit it
   (CI's `sqlc` job fails the build if it's stale — see [CI-CD.md](CI-CD.md)).
4. **DB-layer integration test** — `internal/db/widgets_test.go`, against a real temp SQLite file,
   no mocking.
5. **API layer** (`internal/api/widgets.go`) — an `api.Widget` DTO decoupled from `sqlc.Widget`
   (`internal/api` doesn't import `internal/db`), a `WidgetStore` interface, an `ErrWidgetNotFound`
   sentinel, and `MountWidgets(r, widgets, adapter)` registering the CRUD routes.
6. **API-layer unit test** — `internal/api/widgets_test.go`, a fake `WidgetStore` + `httptest`, no
   DB.
7. **App-layer adapter** (`internal/app/widgets.go`) — `storeWidgets` implements `api.WidgetStore`
   by calling `db.Store.Queries` and converting `sqlc.Widget` ↔ `api.Widget`; this conversion lives
   in `internal/app` specifically because it's the only package allowed to import both `api` and
   `db`.
8. **Wiring** (`internal/app/servers.go`) — `BuildServers` sets `deps.Widgets`/`deps.WidgetsAdapter`
   only when the `database` feature is enabled; `NewProtectedRouter` mounts widgets behind
   `deps.Auth` only (no `deps.Authz` — widgets are authenticated but not authorized).

Identities (`internal/api/identities.go`) is the other worked example, for a different
combination: no database at all (backed entirely by the Kratos admin API via `IdentityAdmin`),
mounted on the **hidden** router behind both `deps.Auth` and `deps.Authz`. Use Widgets as the
template for a new authenticated app resource with its own persistence; use Identities as the
template for a new admin resource that wraps an external service and needs authorization.

## See also

- [DEPLOYMENT.md](DEPLOYMENT.md) — which docker-compose setup to use and when.
- [CI-CD.md](CI-CD.md) — what the GitHub Actions workflows do and don't do.
- [`deploy/kratos/README.md`](deploy/kratos/README.md) — running self-hosted Ory Kratos locally
  and what changes for production.
- [`deploy/keto/README.md`](deploy/keto/README.md) — running self-hosted Ory Keto locally and
  what changes for production.
