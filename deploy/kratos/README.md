# Ory Kratos (self-hosted)

This directory holds everything needed to run Ory Kratos for **local development**:
`kratos.yml`, `identity.schema.json`, `login_code.sms.jsonnet`, `docker-compose.kratos.yml`, and
(optionally) `docker-compose.app.yml`. Start/stop it with:

```sh
task kratos:up    # Postgres, Kratos, self-service UI, mailslurper
task kratos:down
task kratos:logs
```

Then set `KRATOS_PUBLIC_URL=http://localhost:4433`, `KRATOS_ADMIN_URL=http://localhost:4434`, and
`KRATOS_COURIER_WEBHOOK_SECRET=dev-courier-webhook-secret-not-secure` (matching `kratos.yml`'s
courier channel config below) in `.env` (see `.env.example`), and `task run` the app.

Identities are **organiser-provisioned only** — there is no self-service registration flow for
participants. Create one via the admin API:

```sh
curl -X POST http://localhost:4434/admin/identities \
  -H "Content-Type: application/json" \
  -d '{"schema_id":"default","traits":{"phone":"+15550001234","full_name":"Test User"}}'
```

Then log in with a **native** (non-browser) code flow — start one at
`GET http://localhost:4433/self-service/login/api`, submit
`{"method":"code","identifier":"<phone>"}` to the returned flow's action URL (expect a `400`
carrying the *updated*, still in-progress flow — that's Kratos's own native-flow signal to submit
the code next, not an error), watch `unwrap-gift`'s stdout for the logged code (with the `sms`
feature disabled, which is the local dev default), then submit
`{"method":"code","identifier":"<phone>","code":"<code>"}` (yes, `identifier` again — Kratos's
code-method login flow requires it on both steps) to receive a session.

`unwrap-gift` itself runs on the host, not inside this compose network — that's why the Kratos public/
admin ports are published to `localhost` here, and why `kratos.yml`'s `serve.public.base_url` is
`http://127.0.0.1:4433/` rather than a container hostname.

See `TESTING.md` in this directory for copy-pasteable curl commands walking through the whole
identity-provisioning → login → code-submission flow, including the OrbStack workaround below.

## Courier SMS webhook

Kratos never calls seven.io directly. Instead, its courier POSTs every outgoing SMS (login codes)
to `unwrap-gift`'s own `POST /webhooks/kratos/sms` on the **hidden** router, authenticated by a
shared secret Kratos sends in the `X-Courier-Webhook-Secret` header; `unwrap-gift` then hands the
message to `internal/clients/smsclient`, which calls seven.io (`sms` feature enabled) or logs it to
stdout (`sms` disabled — the local dev default, so login is testable without a real SMS account).

- The shared secret lives in **two** places that must match: `kratos.yml`'s
  `courier.channels[sms].request_config.auth.config.value` (hardcoded to
  `dev-courier-webhook-secret-not-secure` for local dev) and `.env`'s
  `KRATOS_COURIER_WEBHOOK_SECRET`.
- In local dev, Kratos (in its own compose network) reaches `unwrap-gift` (running on the host) via
  `host.docker.internal:8082` — `docker-compose.kratos.yml`'s `kratos` service sets
  `extra_hosts: host.docker.internal:host-gateway` for this.
- **GOTCHA, confirmed empirically (not just from Ory's docs):** the config key that actually
  dispatches SMS on this self-hosted Kratos version is `courier.channels` (an array,
  `type: http`) — **not** `courier.sms.request_config`, which is accepted by schema validation but
  silently never sends anything (no request, no error, even at trace log level; the message just
  sits "abandoned" after retries). If a future Kratos upgrade changes this back, re-verify via
  `GET /admin/courier/messages` on the admin API — a `"status": "sent"` entry with your login code
  in `"body"` is the only reliable confirmation delivery actually happened.
- **GOTCHA, OrbStack-specific:** on OrbStack, `host.docker.internal` resolves to a special address
  in the reserved `0.0.0.0/8` range, which Kratos's courier-channel HTTP client hard-refuses to
  dial (`prohibited IP address... denied by: 0.0.0.0/8`) — confirmed empirically, and not something
  `clients.http.disallow_private_ip_ranges` (set `false` in `kratos.yml`) controls, since that
  setting only covers `clients.http`'s general-purpose client, not the courier channel's own guard.
  Docker Desktop for Mac/Windows and plain Linux Docker Engine resolve `host.docker.internal` to an
  ordinary (non-reserved) address and are not known to hit this; **if you're on OrbStack and local
  SMS login codes never arrive, this is why** — there is no config-only fix found so far. The e2e
  test (`internal/clients/kratosclient/kratosclient_e2e_test.go`) sidesteps it by running its
  stand-in webhook as a container on the same Docker network rather than relying on
  `host.docker.internal` at all.
- Kratos's config schema also requires a `courier.templates.login_code.valid.email` template to be
  present even though this identity schema has no email trait and the `code` method here is
  SMS-only (`via: sms`) — it's never rendered or sent, but its absence fails schema validation.
  See `kratos.yml`'s comment on that block.

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
- **Courier SMTP is otherwise unused.** `courier.smtp.connection_uri` still points at `mailslurper`
  here purely because the courier worker may not start at all without *some* valid SMTP config
  (see `kratos.yml`'s GOTCHA comment on that block) — this identity schema has no email trait, so
  no email is ever actually rendered or sent. `kratos.production.yml` points it at
  `${SMTP_CONNECTION_URI}` for the same reason, not because production needs working email.
- **The SMS courier webhook target is a normal container hostname, confirmed working.**
  `kratos.production.yml` points `courier.channels[sms].request_config.url` at
  `http://unwrap-gift:8082/...` — both containers on the same compose network, no
  `host.docker.internal`/OrbStack complication like local dev has (see "Courier SMS webhook"
  above); this container-to-container path was verified end-to-end during this feature's e2e spike.
- **Postgres is managed infrastructure**, not the disposable `postgres-kratos` container here —
  backups, connection pooling, and version upgrades become real operational concerns.

None of the above is wired up by this repo yet — Phase 8 (build/deploy/CI) and Phase 9
(documentation) are where a production deployment target gets decided and automated. This section
exists so those decisions start from the right list of "must change," not from copying the dev
compose file as-is.
