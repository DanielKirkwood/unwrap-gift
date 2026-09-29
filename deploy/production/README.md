# Production deployment (single VPS, docker compose)

Everything in this directory runs the full stack — unwrap-gift, Kratos, Keto, their two Postgres
instances, and Caddy — on one VPS via `docker-compose.yml`. Read `deploy/kratos/README.md` and
`deploy/keto/README.md` first: this compose file and the `*.production.yml` configs exist to apply
exactly what those two documents already said production requires.

## First-time setup

1. Point DNS at the VPS: the bare domain (or `www`), `api.<DOMAIN>`, `auth.<DOMAIN>`, and
   `login.<DOMAIN>` all need an A/AAAA record to this machine — Caddy issues a cert per hostname it
   actually receives traffic for.
2. Install Docker + the Compose plugin on the VPS.
3. Copy this repo to the VPS (or just `deploy/`, `Dockerfile`, `.dockerignore`, and the module
   files needed to build the image — `unwrap-gift`/`unwrap-gift-migrate` build from the repo root).
4. `cp deploy/production/.env.production.example deploy/production/.env.production` and fill in
   every value. Generate secrets with `openssl rand -hex 32`. **Never commit this file** — it's
   gitignored on purpose.

## Bringing the stack up

```sh
cd deploy/production
docker compose --env-file .env.production up -d --build
```

This builds and starts everything except Kratos/Keto's own database migrations, which run first as
one-shot `kratos-migrate`/`keto-migrate` services (`depends_on: condition:
service_completed_successfully` — `kratos`/`keto` won't start until their migration finishes), and
unwrap-gift's own migrations, which run the same way via `unwrap-gift-migrate`. None of the three run
migrations at `serve` time, matching what both READMEs called for.

Watch it come up:

```sh
docker compose --env-file .env.production logs -f
```

## Seeding the admin role

Same two-step process as local dev (`deploy/keto/README.md`), just against the production URLs:

1. Register a user at `https://login.<DOMAIN>/registration`, then find their Kratos identity ID
   (query `GET https://auth.<DOMAIN>/admin/identities` from a machine that can reach the compose
   network — the admin API isn't published to the host, so this has to run from inside the
   `unwrap-gift` container or over an SSH tunnel, never directly from your laptop).
2. Set `KETO_SEED_ADMIN_IDENTITY_ID` in `.env.production` to that ID, then run the task directly:

   ```sh
   docker compose --env-file .env.production run --rm unwrap-gift task exec grant-admin \
     --arg identity-id=<id>
   ```

## Deploying an update (manual — no CI auto-deploy)

CI builds and pushes `ghcr.io/danielkirkwood/unwrap-gift` on every merge to `main`
(`.github/workflows/go.yml`'s `docker` job). Nothing deploys automatically. On the VPS:

```sh
cd deploy/production
docker compose --env-file .env.production pull unwrap-gift unwrap-gift-migrate
docker compose --env-file .env.production up -d unwrap-gift-migrate   # runs any new migrations first
docker compose --env-file .env.production up -d unwrap-gift
```

Kratos/Keto version bumps follow the same shape: bump the image tag in `docker-compose.yml`, run
the corresponding `*-migrate` service, then the service itself.

## Postgres backups

Both Postgres instances are self-hosted containers on this VPS (a deliberate choice — see the
Phase 8 plan's Risks section for the tradeoff against managed Postgres). This is a lightweight
default, not a full disaster-recovery story:

```sh
# Run daily via cron. Adjust retention as needed.
docker compose --env-file .env.production exec -T postgres-kratos \
  pg_dump -U kratos kratos | gzip > kratos-$(date +%F).sql.gz
docker compose --env-file .env.production exec -T postgres-keto \
  pg_dump -U keto keto | gzip > keto-$(date +%F).sql.gz
```

Copy the resulting files off the VPS (rsync/rclone to object storage) — a backup that lives on the
same disk as the database it backs up isn't a backup.

## What's deliberately not here

- **No auto-deploy on merge.** Chosen deliberately while this topology is new — see the Phase 8
  plan. Revisit once the manual process is proven out a few times.
- **No public route for unwrap-gift's public router, the hidden router, Kratos's admin API, or either
  of Keto's APIs.** `api.<DOMAIN>` only reaches unwrap-gift's *protected* router (see
  `deploy/production/Caddyfile`) — the only one with real content today (`/widgets`). Add a route
  for the public router once it has unauthenticated endpoints worth exposing.
- **No network segmentation beyond "only Caddy publishes ports."** Every service shares one
  compose network. Splitting Kratos/Keto onto their own networks (like the dev compose files
  already do) is a reasonable next hardening step, not done here to keep one topology simple
  enough to actually deploy first.
