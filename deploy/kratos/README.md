# Ory Kratos (self-hosted)

This directory holds everything needed to run Ory Kratos for **local development**:
`kratos.yml`, `identity.schema.json`, and `docker-compose.kratos.yml`. Start/stop it with:

```sh
task kratos:up    # Postgres, Kratos, self-service UI, mailslurper
task kratos:down
task kratos:logs
```

Then set `KRATOS_PUBLIC_URL=http://localhost:4433` and `KRATOS_ADMIN_URL=http://localhost:4434`
in `.env` (see `.env.example`) and `task run` the app. Register a user at
`http://localhost:4455/registration` to get a real session cookie for testing the protected/hidden
routers; sent emails (verification/recovery) land in mailslurper's UI at `http://localhost:4436`.

`unwrap-gift` itself runs on the host, not inside this compose network — that's why the Kratos public/
admin ports are published to `localhost` here, and why `kratos.yml`'s `serve.public.base_url` is
`http://127.0.0.1:4433/` rather than a container hostname.

## What changes for production

Nothing here (`kratos.yml`, the compose file) is meant to be deployed as-is — it's tuned for a
single developer's laptop. Running self-hosted Kratos for real means changing at least the
following:

- **No `--dev` flag.** `serve --dev` (used in `docker-compose.kratos.yml`) relaxes cookie
  `SameSite` handling for plain HTTP and is explicitly a development convenience. Production
  Kratos must run behind TLS with `--dev` removed.
- **Explicit secrets.** `kratos.yml`'s `secrets.cookie`/`secrets.cipher` here are placeholder dev
  values checked into git on purpose (they're worthless outside this compose network). Production
  needs its own cryptographically random values, provided via a secret manager or environment
  variable substitution — never committed.
- **Migrations run as a release step, not at `serve` time.** This compose file's `kratos-migrate`
  container runs `migrate sql -e --yes` before `kratos` starts, which is fine for a disposable dev
  stack. In production, run `kratos migrate sql -e` as an explicit, auditable release/deploy step
  against the production DSN, separately from starting the `serve` process.
- **`session.cookie.domain` set to the real domain**, with `same_site: Lax` (or `Strict`) unless
  cross-site embedding is a genuine requirement — `None` requires HTTPS and a legacy-browser
  workaround Ory's own docs call out explicitly.
- **The admin API (port 4434) must never be reachable from the public internet.** Only `unwrap-gift`
  (via `KRATOS_ADMIN_URL`) should be able to reach it — bind it to a private network/VPC, the same
  way this repo's own hidden router is meant to sit behind an interface that isn't
  internet-facing. Kratos's admin API has no authentication of its own; network position is the
  only thing protecting it.
- **TLS terminates in front of Kratos**, not inside it — a reverse proxy or load balancer, same as
  for `unwrap-gift` itself.
- **Courier points at a real SMTP provider.** `mailslurper` (used here) is a dev-only mock SMTP
  server that never actually delivers mail; production needs `courier.smtp.connection_uri` set to
  a real provider for verification/recovery emails to work.
- **Postgres is managed infrastructure**, not the disposable `postgres-kratos` container here —
  backups, connection pooling, and version upgrades become real operational concerns.

None of the above is wired up by this repo yet — Phase 8 (build/deploy/CI) and Phase 9
(documentation) are where a production deployment target gets decided and automated. This section
exists so those decisions start from the right list of "must change," not from copying the dev
compose file as-is.
