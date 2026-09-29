# CI/CD Pipeline

## Overview

This repo runs two independent GitHub Actions workflows: `.github/workflows/go.yml`
(test, build, sqlc drift-check, and — on `main` only — a Docker image build/push) and
`.github/workflows/golangci-lint.yml` (lint only). Both `go.yml` and
`golangci-lint.yml` trigger on pushes and pull requests, but with different branch
scopes (see the trigger table below). **There is no separate CD workflow.** Merging to
`main` produces a new container image in GHCR; getting that image running on a server
is a manual step. See `DEPLOYMENT.md` for the manual VPS runbook.

## `go.yml` jobs

| Job | Runs | Depends on (`needs`) |
|---|---|---|
| `test` | `go vet ./...`, then `go test ./... -race -cover` | none |
| `build` | `go get .`, then `go build -o ./out/service -ldflags="-w -s" .` | none |
| `sqlc` | Installs sqlc v1.27.0, runs `sqlc generate` then `git diff --exit-code internal/db/sqlc` (fails if generated code is stale), then `sqlc vet` | none |
| `docker` | Computes an image version, logs into GHCR, builds and pushes the image | `[test, build, sqlc]` |

The `docker` job only runs when:

```yaml
if: github.event_name == 'push' && github.ref == 'refs/heads/main'
```

That is: a direct push event to `refs/heads/main` — not a PR merge event, not a push
to any other branch. (A PR merged into `main` produces a `push` event to `main`, so it
does satisfy this condition — but a PR being opened/updated against `main` does not,
since that's a `pull_request` event.)

## `golangci-lint.yml`

Single job `golangci`, runs `golangci-lint` (pinned tool version `v2.14`) via
`golangci/golangci-lint-action@v9`. Triggers:

- `push` to `main` only
- `pull_request` with **no branch restriction** — runs for a PR targeting any branch

This is a **separate required check** from `go.yml`'s own `go vet ./...` step in the
`test` job — `go vet` catches a narrower set of issues than golangci-lint's configured
linters, and both checks run independently on PRs.

## Trigger summary

| Event | `test` | `build` | `sqlc` | `docker` | `golangci-lint` |
|---|---|---|---|---|---|
| Push to `main` | yes | yes | yes | **yes** | yes |
| Push to `dev` | yes | yes | yes | no | **no** |
| PR → `main` | yes | yes | yes | no | yes |
| PR → `dev` | yes | yes | yes | no | yes |

Key things to notice:
- Pushing to `dev` runs the Go checks but **not** golangci-lint (its `push` trigger is
  scoped to `main` only) and **not** the docker build.
- A PR into `dev` still runs golangci-lint (its `pull_request` trigger has no branch
  filter) even though a push to `dev` doesn't.
- The docker image is only ever built/pushed on a direct push to `main`.

## What does NOT happen automatically

- **No auto-deploy.** Nothing in this repo's workflows SSHes into a server, calls a
  deploy webhook, or restarts anything. Getting a new image running is a manual step —
  see `DEPLOYMENT.md`.
- **No git tag is created by CI.** Tags are entirely a manual/local action
  (`git tag vX.Y.Z && git push --tags`).
- **No CHANGELOG update.** Nothing generates or edits a changelog.
- Merging to `main` only results in a new container image being pushed to GHCR
  (`ghcr.io/<owner>/<repo>:latest` and `ghcr.io/<owner>/<repo>:<version>`).

## Versioning / image tagging

The `docker` job computes the version like this:

```yaml
echo "version=$(git describe --tags --always --dirty)" >> "$GITHUB_OUTPUT"
```

`git describe --tags --always --dirty` behavior:
- If the current commit has a reachable tag, it outputs something like `v0.1.0` (exact
  tag) or `v0.1.0-3-gabc1234` (3 commits past the last tag).
- `--always` makes it fall back to the abbreviated commit SHA when there are **no tags
  at all** reachable from the commit.
- `--dirty` would append `-dirty` if the working tree had uncommitted changes, but CI
  always checks out a clean tree, so this suffix never appears in a CI-built image.

**As of now, `git tag` on this repo returns nothing — no tags exist.** That means every
image built today is tagged with a short commit SHA (via the `--always` fallback), e.g.
`ghcr.io/danielkirkwood/unwrap-gift:abc1234`, alongside `:latest`. There is no semver
tagging happening yet, not because anything is broken, but because no one has cut a
tag.

Once the maintainer starts cutting real tags — e.g.:

```sh
git tag v0.1.0
git push --tags
```

the exact same `docker` job, with no workflow changes, starts producing meaningful
semver-based image tags (`v0.1.0`, then `v0.1.0-N-gHASH` for commits after it, until
the next tag).

## sqlc drift-check gotcha

The `sqlc` job in `go.yml` runs `sqlc generate` in CI and then fails the build via
`git diff --exit-code internal/db/sqlc` if that regenerates anything different from
what's committed.

**Failure mode:** you edit a file under `internal/db/queries/*.sql` (or add/change a
migration), run `sqlc generate` locally to update your local `internal/db/sqlc/*.go`,
but forget to `git add` and commit the regenerated files. CI then regenerates the same
code, finds it differs from what's in your commit (because your commit has none of the
regenerated changes), and the `sqlc` job — and therefore the whole `docker` job, which
depends on it — fails.

**Fix:**

```sh
task sqlc:generate   # regenerate internal/db/sqlc/*.go
git add internal/db/sqlc
git commit -m "..."
```

(`task sqlc:check` in the Taskfile runs the identical check locally — `sqlc generate`
then `git diff --exit-code internal/db/sqlc` — useful to run before pushing.)

## Pinned action versions

| Workflow | `uses:` | Pinned to | Notes |
|---|---|---|---|
| `go.yml` | `actions/checkout` | `v6` | used in all 4 jobs |
| `go.yml` | `actions/setup-go` | `v7` | used in `test`, `build`, `sqlc`; reads Go version from `go.mod` |
| `go.yml` | `sqlc-dev/setup-sqlc` | `v5` | installs sqlc `1.27.0` (input `sqlc-version`, not the action's own version) |
| `go.yml` | `docker/login-action` | `v4` | logs into `ghcr.io` |
| `go.yml` | `docker/build-push-action` | `v7` | builds and pushes the image |
| `golangci-lint.yml` | `actions/checkout` | `v6` | |
| `golangci-lint.yml` | `actions/setup-go` | `v7` | reads Go version from `go.mod` |
| `golangci-lint.yml` | `golangci/golangci-lint-action` | `v9` | pins golangci-lint tool itself to `v2.14` via the `version` input |

Last action-version bump: commit `d9aba9d upgrade github actions` (most recent commit
touching `go.yml`); the job structure itself was introduced in `fe198c1 Implement
Phase 8 build, deploy, and CI`.
