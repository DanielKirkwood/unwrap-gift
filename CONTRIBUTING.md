# Contributing

This is an internal application template, not a published library — this
guide covers the contributor workflow (clone, build, test, lint, extend),
not API/godoc conventions.

## Prerequisites

- **Go 1.27.1** (pinned in `go.mod`; CI uses `go-version-file: go.mod`, so
  install exactly this version to avoid drift).
- **Docker** — needed for `task test:e2e` and for the local Kratos/Keto dev
  stacks (`task auth:up`). Not needed for the default build/test/lint loop.
- **[go-task](https://taskfile.dev)** — all commands below run through it.
- **golangci-lint v2.14** — pinned in `.github/workflows/golangci-lint.yml`
  (`golangci-lint-action@v9`, `version: v2.14`). Install the matching
  version locally so `task lint` matches CI exactly.
- **sqlc v1.27.0** — pinned in `.github/workflows/go.yml`'s `sqlc` job
  (`sqlc-dev/setup-sqlc@v5`, `sqlc-version: 1.27.0`). Only needed if you're
  touching `internal/db/queries/*.sql` or migrations.

## Local setup

```sh
git clone <repo-url>
cd unwrap-gift
cp .env.example .env
task build
task db:migrate
```

`task db:migrate` builds first (it depends on `build`) and applies
migrations against a local SQLite file (`dev.db`). See `.env.example` for
what each variable enables — the app runs with database/otel/kratos/keto
each individually optional, gated on their required vars being set.

## Running tests

Tests are split across three tiers by cost, not by package:

- **`task test`** (`go test ./...`) — the default. Covers both unit tests
  (fakes, no I/O) and DB integration tests that hit a real SQLite database,
  no external services. This is what CI's `test` job runs (as
  `go test ./... -race -cover`). Run this before every commit.
- **`task test:e2e`** (`go test -tags=e2e ./...`) — needs Docker. Spins up
  real Postgres+Kratos+mailslurper or Postgres+Keto via testcontainers-go.
  Excluded from the default build and from `task test` by the `e2e` build
  tag. **This does not run in CI today** — neither CI workflow
  (`go.yml` or `golangci-lint.yml`) passes `-tags=e2e`, so these tests are
  dev-only. Run them locally when you change auth/authz-adjacent code.
- **`task lint`** and `sqlc:vet`/`sqlc:check` (below) act as a further,
  non-test correctness tier.

## Linting

```sh
task fmt    # golangci-lint fmt — fixes formatting (goimports, golines)
task lint   # golangci-lint run
```

`.golangci.yml` is strict; a few gotchas that will surprise you if you
haven't touched this repo in a while:

- **No package-level vars or `init()`** (`gochecknoglobals`,
  `gochecknoinits`) — with one sanctioned exception: `internal/tasks`' self-
  registration pattern (`grant_admin.go`, `reseed_keto.go`,
  `revoke_admin.go`, `task.go`), each carrying an explicit
  `//nolint:gochecknoglobals`/`//nolint:gochecknoinits` with a reason.
  Don't add new package-level state anywhere else without the same
  justification.
- **Comments must end in a period** (`godot`).
- **Complexity budgets**: `cyclop` (max 30 per function, 10.0 package
  average), `gocognit` (min 20 to report), `funlen` (100 lines / 50
  statements per function), `dupl` (clone detection) — refactor rather than
  reach for `//nolint` first.
- **Import grouping**: `goimports` with `local-prefixes:
  github.com/DanielKirkwood/unwrap-gift` — this module's own imports must group
  separately from third-party ones. `golines` enforces a 120-char line
  length on top of this.
- **`depguard`** blocks a short denylist outright: `github.com/golang/protobuf`,
  `github.com/satori/go.uuid`, `github.com/gofrs/uuid` below v5, `math/rand`
  in non-test files (use `math/rand/v2`), and the standard `log` package
  outside `main.go` (use `log/slog`).
- **`testpackage`** requires external test packages (`foo_test`, not `foo`)
  and **`paralleltest`** expects `t.Parallel()` in table-driven tests.
- Any `//nolint` needs both a specific linter name and an explanation
  (`nolintlint`, `require-explanation: true`, `require-specific: true`).

## sqlc: regenerate and commit

If you edit anything under `internal/db/queries/*.sql` or add/change a
migration under `internal/db/migrations/`, you must regenerate and commit
the generated code:

```sh
task sqlc:generate   # sqlc generate — regenerates internal/db/sqlc/*
task sqlc:vet        # sqlc vet — lints queries against the live schema
```

Commit the regenerated files in `internal/db/sqlc/` along with your query
or migration change. CI enforces this: the `sqlc` job in
`.github/workflows/go.yml` runs the same drift check (`sqlc generate`
followed by `git diff --exit-code internal/db/sqlc`) and fails the build on
any uncommitted diff, plus a separate `sqlc vet` step. Running
`task sqlc:check` locally before pushing reproduces that check exactly.

## Adding a new resource

Widgets (`internal/api/widgets.go` + `internal/app/widgets.go` +
`internal/db/migrations/00002_widgets.sql` +
`internal/db/queries/widgets.sql` + the generated
`internal/db/sqlc/widgets.sql.go`) is the canonical worked example of an
authenticated, DB-backed CRUD resource in this codebase — a chi-routed
handler set with its own store interface in `internal/api`, an adapter in
`internal/app` that converts between the sqlc-generated type and the api
package's own type (api never imports `internal/db`), and the
migration/query/generated-code triad in `internal/db`. The full step-by-step
pattern for adding a resource like this is documented in
[ARCHITECTURE.md](ARCHITECTURE.md)'s "Adding a new resource" section — refer
there rather than re-deriving the steps from widgets alone.

## Submitting changes

Keep commits and PRs focused on one change, with commit messages that
explain why the change is needed, not just what changed. Run `task test`
and `task lint` before opening a PR — CI runs the same checks (plus the
`sqlc` drift check above) and will fail on anything those two don't already
catch locally.
