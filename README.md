# unwrap-gift

[![Go Version](https://img.shields.io/github/go-mod/go-version/DanielKirkwood/unwrap-gift)](go.mod)
[![License: MIT](https://img.shields.io/github/license/DanielKirkwood/unwrap-gift)](LICENSE)
[![Go](https://img.shields.io/github/actions/workflow/status/DanielKirkwood/unwrap-gift/go.yml?branch=main&label=go)](https://github.com/DanielKirkwood/unwrap-gift/actions/workflows/go.yml)
[![golangci-lint](https://img.shields.io/github/actions/workflow/status/DanielKirkwood/unwrap-gift/golangci-lint.yml?branch=main&label=lint)](https://github.com/DanielKirkwood/unwrap-gift/actions/workflows/golangci-lint.yml)

A Go API boilerplate: a [chi](https://github.com/go-chi/chi) HTTP layer with [Ory Kratos](https://www.ory.sh/kratos/)
authentication, [Ory Keto](https://www.ory.sh/keto/) authorization, SQLite persistence via
[sqlc](https://sqlc.dev/), OpenTelemetry observability, and a [Cobra](https://github.com/spf13/cobra) CLI —
built to be forked as the starting point for a new project, not run as-is.

## Demo

```sh
docker compose up --build
curl localhost:8080/health/alive
```

```json
{"data":{"status":"alive"}}
```

That's the whole stack with every optional feature (database, otel, Kratos, Keto) disabled except a SQLite
database — enough to prove the built image runs standalone. See [Getting Started](#getting-started) for the
full local-dev setup with authentication.

## Getting Started

### Quickstart (no auth, SQLite only)

The fastest way to see it run, using the root [`docker-compose.yml`](docker-compose.yml):

```sh
docker compose up --build
```

This builds the image, runs migrations as a one-shot step, then starts the app on ports `8080`
(public), `8081` (protected), `8082` (hidden). No Kratos/Keto — the `kratos`/`keto` features stay
disabled because their required env vars aren't set (see [ARCHITECTURE.md](ARCHITECTURE.md#feature-flags-and-the-nil-is-disabled-convention)).

### Full local dev (with auth)

Requires Docker and [go-task](https://taskfile.dev/):

```sh
cp .env.example .env
task dev:up
```

This starts Postgres x2, Kratos, Keto, mailslurper, and unwrap-gift itself — hot-reloading on every
source change via [air](https://github.com/air-verse/air) — all on one Docker network (see
[`deploy/dev/docker-compose.yml`](deploy/dev/docker-compose.yml)). Migrations run automatically.
Running unwrap-gift as a container here, rather than on the host, is deliberate: it's what makes
Kratos's courier SMS webhook reach it reliably regardless of platform (see
[`deploy/kratos/README.md`](deploy/kratos/README.md)'s "Courier SMS webhook" section).

Identities are organiser-provisioned only (no self-service registration) — create one via Kratos's
admin API (see [`deploy/kratos/README.md`](deploy/kratos/README.md) for the curl example), then set
`KETO_SEED_ADMIN_IDENTITY_ID` in `.env` to that identity's ID and run `task dev:seed` to grant it the
admin role. Once auth and the database are both enabled, the login/wishlist web UI (`internal/web`)
is at `http://localhost:8083`. See [`deploy/kratos/README.md`](deploy/kratos/README.md) and
[`deploy/keto/README.md`](deploy/keto/README.md) for what each stack does and what changes in
production.

Prefer running unwrap-gift natively instead (faster edit/rebuild loop, easier to attach a debugger,
no working SMS courier webhook)? `task auth:up` starts just Kratos + Keto, then `task db:migrate`
and `task run` build and run the binary on the host — see [`CONTRIBUTING.md`](CONTRIBUTING.md) for
that workflow's prerequisites.

For the full contributor workflow — running tests, linting, regenerating sqlc code — see
[CONTRIBUTING.md](CONTRIBUTING.md).

## Features

- **Three independent HTTP routers** — public, protected (authenticated), hidden (authenticated +
  authorized, meant for a non-public-facing interface) — each its own port and `http.Server`.
- **Feature flags with a "nil is disabled" convention** — `database`/`otel`/`kratos`/`keto` each
  auto-enable once their required env vars are set; disabled features leave their dependency `nil`
  rather than scattering `if enabled` checks through the code.
- **Auth via Ory Kratos, authz via Ory Keto** — session validation and relation-tuple permission
  checks, with `internal/api` decoupled from both via narrow interfaces it declares itself.
- **RFC 9457 Problem Details errors** and a uniform `{"data": ...}` success envelope.
- **SQLite via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)** (pure Go, no cgo) with
  [goose](https://github.com/pressly/goose) migrations and [sqlc](https://sqlc.dev/)-generated queries.
- **OpenTelemetry** traces and metrics throughout (otelchi/otelhttp/otelsql), OTLP/HTTP export.
- **A Cobra CLI** (`start`, `db`, `task`, `info`) — run `go run . --help` to see all of it.
- **A small ad-hoc tasks framework** (`internal/tasks`) for operator-run jobs like granting admin
  access — not a background job queue.
- **Three test tiers**: fast unit tests (fakes, no I/O), DB integration tests (real SQLite), and
  Docker-backed e2e tests (testcontainers-go, real Kratos/Keto) behind a build tag.
- **Single-VPS deployment** via docker compose behind Caddy — no Kubernetes. See
  [DEPLOYMENT.md](DEPLOYMENT.md).

The full rationale behind each of these lives in [ARCHITECTURE.md](ARCHITECTURE.md); CI/CD mechanics
are in [CI-CD.md](CI-CD.md).

### Using this as a template

This repo is meant to be forked. Before starting a new project from it:

- Rename the module path in `go.mod` (and every import of it).
- Replace `DOMAIN` and the generated secrets in `deploy/production/.env.production` (copy from
  `.env.production.example`).
- If your resource model needs different roles/permissions, extend or replace the OPL namespaces
  in [`deploy/keto/identities.ts`](deploy/keto/identities.ts) rather than inventing a second authz
  mechanism.
- Delete the Widgets example (`internal/api/widgets.go`, `internal/app/widgets.go`,
  `internal/db/{migrations,queries,sqlc}/*widgets*`) once you've used it as the template for your
  first real resource — see [ARCHITECTURE.md](ARCHITECTURE.md#adding-a-new-resource) for the
  step-by-step walkthrough.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for local setup, running the test suite, linting, and the
sqlc regenerate-and-commit workflow.

## Contributors

[![Contributors](https://img.shields.io/github/contributors/DanielKirkwood/unwrap-gift)](https://github.com/DanielKirkwood/unwrap-gift/graphs/contributors)

## License

[MIT](LICENSE)
