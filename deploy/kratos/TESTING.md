# Testing the SMS-code auth flow locally

Step-by-step commands to exercise phone+code login end to end, including the courier webhook
that delivers the login code to `unwrap-gift`. Two setups are covered — pick one:

- **Setup A — app in a container (recommended)**: runs `unwrap-gift` on the same Docker network
  as Kratos. The courier webhook actually delivers. Works on OrbStack.
- **Setup B — app on the host**: matches real day-to-day `task run` development. On OrbStack, the
  courier webhook **cannot** reach the host — see the GOTCHA in `README.md`'s "Courier SMS
  webhook" section. You'll still complete the flow, but by reading the code from Kratos's own
  admin API instead of `unwrap-gift`'s logs.

Both setups use the same identity/login API calls — only how you retrieve the delivered code
differs.

---

## Setup A — app in a container (recommended)

```sh
task kratos:down       # if a plain `task kratos:up` stack is already running
cp .env.example .env   # only needed if you don't already have one
task dev:up            # Postgres x2, Kratos, Keto, mailslurper, AND unwrap-gift itself
```

This builds `unwrap-gift` from the repo root and starts it (hot-reloading via air) on the same
Docker network as Kratos and Keto (see `deploy/dev/docker-compose.yml`), reachable at the usual
`:8080`/`:8081`/`:8082`/`:8083` host ports. Most env vars are set directly in the compose file;
`.env` only needs to exist so compose has something to read `SEVEN_API_KEY`/`SEVEN_SENDER_ID`/
`KETO_SEED_ADMIN_IDENTITY_ID`/`DISABLE_FEATURES` from (all optional for this walkthrough).

Tear down with `task dev:down`.

### 1. Create an identity (organiser provisioning)

There's no self-service registration — an identity is created directly via Kratos's admin API.
**Use a real-looking phone number**: Kratos's `format: "tel"` validation is stricter than it
looks and rejects obviously-fake numbers like `+15550001234`; a NANP number like
`+12025550123` passes.

```sh
curl -s -X POST http://localhost:4434/admin/identities \
  -H "Content-Type: application/json" \
  -d '{"schema_id":"default","traits":{"phone":"+12025550123","full_name":"Test User"}}' \
  | jq .
```

Note the returned `"id"` — that's the identity you should see logged in when the flow completes.

### 2. Start a native login flow

```sh
FLOW=$(curl -s http://localhost:4433/self-service/login/api | jq -r '.id')
echo "flow: $FLOW"
```

### 3. Submit the phone number as the identifier

This triggers Kratos to generate a code and send it via the courier webhook.

```sh
curl -s -X POST "http://localhost:4433/self-service/login?flow=$FLOW" \
  -H "Content-Type: application/json" \
  -d '{"method":"code","identifier":"+12025550123"}' \
  -o /dev/null -w "status: %{http_code}\n"
```

**Expect `status: 400`.** This is normal and not an error — Kratos's native API returns 400 with
the updated (still in-progress) flow to signal "submit the code next," not a real failure.

### 4. Read the delivered code from unwrap-gift's own logs

```sh
docker logs unwrap-gift-dev-unwrap-gift-1 --tail 5
# or: task dev:logs
```

Look for a line like:

```
INF smsclient/smsclient.go:76 sms disabled: logging instead of sending to=+12025550123 body="Your Secret Santa login code is: 775008"
```

That confirms the full path worked: Kratos's courier → `unwrap-gift`'s `/webhooks/kratos/sms` →
`internal/clients/smsclient` (logging instead of sending, since `SEVEN_API_KEY`/`SEVEN_SENDER_ID`
aren't set). Copy the 6-digit code.

### 5. Submit the code to complete login

**`identifier` must be resent here too** — Kratos's code method requires it on both steps, not
just the first.

```sh
curl -s -X POST "http://localhost:4433/self-service/login?flow=$FLOW" \
  -H "Content-Type: application/json" \
  -d '{"method":"code","identifier":"+12025550123","code":"<code from step 4>"}' \
  | jq .
```

A successful response has a top-level `"session_token"` and `"session"."identity"."id"` matching
the identity ID from step 1 — that's a full login, proving admin-provisioning → code login →
webhook delivery → session issuance all work together.

---

## Setup B — app on the host (`task run`)

```sh
task kratos:up
```

Then create `.env` from `.env.example` and fill in:

```sh
KRATOS_PUBLIC_URL=http://localhost:4433
KRATOS_ADMIN_URL=http://localhost:4434
KRATOS_COURIER_WEBHOOK_SECRET=dev-courier-webhook-secret-not-secure
```

```sh
task build && set -a && source .env && set +a && ./bin/unwrap-gift start
```

Steps 1–3 and 5 above are identical. For step 4, **on OrbStack the webhook never arrives** (see
the GOTCHA in `README.md`) — `unwrap-gift`'s own logs will show nothing. Instead, read the
generated code straight from Kratos's admin API:

```sh
curl -s http://localhost:4434/admin/courier/messages | jq .
```

Find the most recent entry with `"channel": "sms"` and pull the code out of its `"body"` field
(e.g. `"Your Secret Santa login code is: 775008"`), then continue with step 5 as normal. This
proves Kratos's side of the flow (identity, login, code generation, session issuance) works, even
though the webhook delivery leg specifically can't be exercised this way on OrbStack.

---

## Cleanup

```sh
task dev:down     # Setup A
task kratos:down  # Setup B (also stops a plain kratos:up stack)
```
