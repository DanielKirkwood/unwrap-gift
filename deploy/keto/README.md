# Ory Keto (self-hosted)

This directory holds everything needed to run Ory Keto for **local development**: `keto.yml`,
`identities.ts`, and `docker-compose.keto.yml`.

**Most contributors want `task dev:up` instead** (see the root [`README.md`](../../README.md) and
[`deploy/dev/docker-compose.yml`](../dev/docker-compose.yml)) — it brings up this stack, Kratos,
and unwrap-gift itself together, with hot reload. The commands here start Keto on its own, for the
lighter host-run alternative:

```sh
task keto:up    # Postgres, Keto
task keto:down
task keto:logs
```

Keto requires Kratos to also be running (relation tuples grant permissions to Kratos identity
IDs), so use `task auth:up`/`task auth:down` instead to bring up both stacks together in the
right order.

Then set `KETO_READ_URL=http://localhost:4466` and `KETO_WRITE_URL=http://localhost:4467` in
`.env` (see `.env.example`) and `task run` the app.

## Seeding the admin role

1. Create an identity via Kratos's admin API to get a Kratos identity ID — identities are
   organiser-provisioned only, there is no self-service registration (see
   `deploy/kratos/README.md`'s curl example), or query `GET {KRATOS_ADMIN_URL}/admin/identities`
   for an identity created some other way.
2. Set `KETO_SEED_ADMIN_IDENTITY_ID` in `.env` to that identity's ID.
3. Run `task db:seed` (host-run flow) or `task dev:seed` (`task dev:up` flow). This creates two
   relation tuples (see `identities.ts`):
   - `Role:admin#members@<identity-id>`
   - `Identities:admin#managers@Role:admin`

That identity can now call the hidden router's `/admin/identities/*` endpoints; any other
identity gets a 403.

Under `task keto:up` + `task run`, `unwrap-gift` runs on the host, not inside this compose
network — that's why the Keto read/write ports are published to `localhost` here. Under `task
dev:up`, `unwrap-gift` runs as a container on the same network as Keto instead, reaching it via
`http://keto:4466`/`http://keto:4467` directly (see `deploy/dev/docker-compose.yml`); these ports
are still published to `localhost` there too, for debugging.

## What changes for production

Nothing here (`keto.yml`, the compose file) is meant to be deployed as-is — it's tuned for a
single developer's laptop. Running self-hosted Keto for real means changing at least the
following:

- **The write API (port 4467) must never be reachable from the public internet.** Only `unwrap-gift`
  (via `KETO_WRITE_URL`) should be able to reach it — bind it to a private network/VPC, the same
  way this repo's own hidden router and Kratos's admin API are meant to sit behind an interface
  that isn't internet-facing. Keto's write API has no authentication of its own; network position
  is the only thing protecting it.
- **The read API (port 4466) should also stay internal.** Only `unwrap-gift` calls `CheckPermission` —
  end users never talk to Keto directly.
- **Migrations run as a release step, not at `serve` time.** This compose file's `keto-migrate`
  container runs `migrate up -y` before `keto` starts, which is fine for a disposable dev stack.
  In production, run `keto migrate up` as an explicit, auditable release/deploy step against the
  production DSN, separately from starting the `serve` process.
- **The OPL namespace file (`identities.ts`) changes become a reviewed, versioned deploy step** —
  changing a permission's logic changes access control for the whole app.
- **Postgres is managed infrastructure**, not the disposable `postgres-keto` container here —
  backups, connection pooling, and version upgrades become real operational concerns.

None of the above is wired up by this repo yet — Phase 8 (build/deploy/CI) and Phase 9
(documentation) are where a production deployment target gets decided and automated. This section
exists so those decisions start from the right list of "must change," not from copying the dev
compose file as-is.
