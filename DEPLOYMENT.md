# Deployment

This repo ships three different docker-compose configurations for three different jobs: a
single-container quickstart against SQLite, a local-dev stack with real Kratos/Keto auth, and the
full production stack behind Caddy. This document exists to route you to the right one — each row
below links to the doc with the actual runbook; this page deliberately does not repeat those
details.

## Which one do I want?

| Compose file | Purpose | Kratos/Keto? | Publishes host ports? | Start it |
|---|---|---|---|---|
| `docker-compose.yml` (root) | Quickstart: run unwrap-gift alone against SQLite, no auth backends | No | Yes — `8080`, `8081`, `8082` | `docker compose up --build` |
| `deploy/kratos/docker-compose.kratos.yml` + `deploy/keto/docker-compose.keto.yml` | Local dev with real Kratos + Keto auth; unwrap-gift itself runs on the host via `task run` (its own login/wishlist web UI binds `8083` on the host directly, not via these compose files) | Yes (both) | Yes — Kratos `4433`/`4434`/`4436`/`4437`, Postgres `5433`; Keto `4466`/`4467`, Postgres `5434` | `task auth:up` (brings up Kratos then Keto in order; `task auth:down` reverses it) — for full setup details see [`deploy/kratos/README.md`](deploy/kratos/README.md) and [`deploy/keto/README.md`](deploy/keto/README.md) |
| `deploy/production/docker-compose.yml` | Full stack for a single VPS: unwrap-gift, Kratos, Keto, their two Postgres instances, and Caddy in front | Yes | Only `caddy` — `80`, `443` | `cd deploy/production && docker compose --env-file .env.production up -d --build` — for the full production runbook see [`deploy/production/README.md`](deploy/production/README.md) |

Keto requires Kratos to be running (its relation tuples reference Kratos identity IDs), so use
`task auth:up`/`task auth:down` rather than starting `keto:up` on its own.

## Production deploys are manual

Merging to `main` triggers CI to build and push a new image to `ghcr.io/danielkirkwood/unwrap-gift`
(see [`CI-CD.md`](CI-CD.md) for the pipeline), but nothing auto-deploys from that. Rolling it out
to the VPS is a manual `pull` + `up -d` sequence a human runs — see
[`deploy/production/README.md`](deploy/production/README.md) for the exact commands.
