# Deployment

This repo ships four different docker-compose configurations for four different jobs: a
single-container quickstart against SQLite, a full local-dev stack with real Kratos/Keto auth and
hot reload, a lighter host-run local-dev alternative, and the full production stack behind Caddy.
This document exists to route you to the right one — each row below links to the doc with the
actual runbook; this page deliberately does not repeat those details.

## Which one do I want?

| Compose file | Purpose | Kratos/Keto? | Publishes host ports? | Start it |
|---|---|---|---|---|
| `docker-compose.yml` (root) | Quickstart: run unwrap-gift alone against SQLite, no auth backends | No | Yes — `8080`, `8081`, `8082` | `docker compose up --build` |
| `deploy/dev/docker-compose.yml` | Full local dev with real Kratos + Keto auth; unwrap-gift itself runs as a container with hot reload (via air) on the same network — the recommended default, and the only setup where the courier SMS webhook reliably delivers | Yes (both) | Yes — unwrap-gift `8080`-`8083`, Kratos `4433`/`4434`/`4436`/`4437`, Postgres `5433`; Keto `4466`/`4467`, Postgres `5434` | `task dev:up` — for full setup details see [`deploy/kratos/README.md`](deploy/kratos/README.md) and [`deploy/keto/README.md`](deploy/keto/README.md) |
| `deploy/kratos/docker-compose.kratos.yml` + `deploy/keto/docker-compose.keto.yml` | Lighter local-dev alternative: real Kratos + Keto auth, but unwrap-gift itself runs on the host via `task run` (faster native edit/rebuild loop; the courier SMS webhook does not reliably deliver here — see `deploy/kratos/README.md`'s GOTCHA) | Yes (both) | Yes — Kratos `4433`/`4434`/`4436`/`4437`, Postgres `5433`; Keto `4466`/`4467`, Postgres `5434` | `task auth:up` (brings up Kratos then Keto in order; `task auth:down` reverses it) |
| `deploy/production/docker-compose.yml` | Full stack for a single VPS: unwrap-gift, Kratos, Keto, their two Postgres instances, and Caddy in front | Yes | Only `caddy` — `80`, `443` | `cd deploy/production && docker compose --env-file .env.production up -d --build` — for the full production runbook see [`deploy/production/README.md`](deploy/production/README.md) |

Keto requires Kratos to be running (its relation tuples reference Kratos identity IDs) — `task
dev:up` brings up both together already; for the host-run alternative, use `task auth:up`/`task
auth:down` rather than starting `keto:up` on its own.

## Production deploys are manual

Merging to `main` triggers CI to build and push a new image to `ghcr.io/danielkirkwood/unwrap-gift`
(see [`CI-CD.md`](CI-CD.md) for the pipeline), but nothing auto-deploys from that. Rolling it out
to the VPS is a manual `pull` + `up -d` sequence a human runs — see
[`deploy/production/README.md`](deploy/production/README.md) for the exact commands.
