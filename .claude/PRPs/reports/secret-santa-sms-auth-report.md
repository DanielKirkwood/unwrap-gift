# Implementation Report: SMS-code auth via Kratos + seven.io

## Summary

Replaced the repo's only identity schema (email + password) with a phone-number + full-name
schema using Ory Kratos's passwordless `code` method, so drawer members log in with an SMS
one-time code. Kratos's courier is reconfigured to POST every outgoing login code to a new
shared-secret-protected webhook on unwrap-gift's hidden router; that webhook hands the message to
Phase 3's already-implemented `internal/clients/smsclient`. The organiser provisions each Member's
Kratos identity via the existing admin `CreateIdentity` pattern — no self-service registration
flow exists for participants.

Phase 3 (`internal/clients/smsclient`, the `sms` feature, `App.SMS`) was already fully implemented
before this session started, so this plan's Cross-Phase Contract was satisfied from the outset —
Task 7's wiring needed no rework.

## Assessment vs Reality

| Metric | Predicted (Plan) | Actual |
|---|---|---|
| Complexity | Large | Large — matched, plus significant undocumented Kratos-version-specific discoveries in Task 10 |
| Confidence | High (per plan's Notes) | Lower than planned for the courier config shape specifically; the plan's own "External Documentation" citation for `courier.sms.request_config` turned out to be a non-functional dead end on this Kratos version — see Deviations |
| Files Changed | ~15 | 22 (17 modified, 5 created) |

## Tasks Completed

| # | Task | Status | Notes |
|---|---|---|---|
| 1 | Rewrite identity schema | Done | Exactly as planned |
| 2 | Update `kratos.yml` / `kratos.production.yml` | Done | Deviated — see below: `courier.channels` not `courier.sms.request_config`; added a required `login_code.valid.email` template stub; added `clients.http.disallow_private_ip_ranges: false` |
| 3 | Add webhook secret to config | Done | Exactly as planned; also extended `registry_test.go`/`app_test.go`/`servers_test.go` fixtures the plan flagged as needing updates |
| 4 | `SMSSender` interface + webhook handler | Done | Exactly as planned |
| 5 | Webhook auth middleware | Done | Exactly as planned |
| 6 | Unit tests for webhook + auth | Done | Exactly as planned; also added two `servers_test.go` regression tests proving the route mounts/is protected end-to-end |
| 7 | Wire into `RouterDeps`/`NewHiddenRouter`/`servers.go` | Done | Exactly as planned — Phase 3 already existed, no rework needed |
| 8 | `docker-compose.kratos.yml` + jsonnet body file | Done | Exactly as planned |
| 9 | `.env.example` / production env / production compose | Done | Exactly as planned |
| 10 | Rewrite Kratos e2e spike test | Done — deviated significantly | See below; this task is where nearly every real discovery in this report came from |
| 11 | Update `deploy/kratos/README.md` | Done | Documents the courier webhook plus every GOTCHA found in Task 10 |
| 12 | Update the PRD | No change needed | Phase 2's row already showed `Status: in-progress`, `Depends: 3`, and the plan link — set during an earlier planning step before this session |

## Validation Results

| Level | Status | Notes |
|---|---|---|
| Static Analysis | Pass | `go vet ./...`, `go vet -tags=e2e ./...`, `golangci-lint run` — 0 issues (one `gosec` false positive on a header-name constant silenced with `//nolint:gosec`, matching repo convention) |
| Unit Tests | Pass | `go test ./internal/api/... ./internal/config/... ./internal/app/... -v` — all green, including 7 new Courier* tests, 1 new config RequiredEnv test, 2 new servers_test.go regression tests |
| Build | Pass | `go build ./...` and `go build -tags=e2e ./...` |
| Integration | Pass | `task test:e2e` (`TestPhoneCodeLogin_E2E`) — run 3× consecutively, passed every time |
| Edge Cases | Pass | Covered per plan's Testing Strategy checklist (empty/malformed webhook body, wrong/missing/empty secret, sender error) |
| Manual Local Dev | Partial | `docker compose -f deploy/kratos/docker-compose.kratos.yml config` resolves cleanly; `task kratos:up` itself could not be run to completion in this environment because ports 4433/4434/4436/4437/5433 are already bound by an unrelated pre-existing project's own Kratos stack on this machine (`go-api-kratos-*`) — not a defect in this change. The identical courier/networking logic was independently verified via `docker run` against the real committed config files and via the e2e test's `testcontainers` containers, both using the exact same `kratos.yml`/`docker-compose.kratos.yml` port/mount/`extra_hosts` shape. |

## Files Changed

| File | Action | Lines |
|---|---|---|
| `deploy/kratos/identity.schema.json` | UPDATED | +19/-6 |
| `deploy/kratos/kratos.yml` | UPDATED | +54/-5 |
| `deploy/kratos/kratos.production.yml` | UPDATED | +40/-2 |
| `deploy/kratos/login_code.sms.jsonnet` | CREATED | +4 |
| `deploy/kratos/docker-compose.kratos.yml` | UPDATED | +3 |
| `deploy/kratos/README.md` | UPDATED | +75/-4 |
| `deploy/production/docker-compose.yml` | UPDATED | +2 |
| `deploy/production/.env.production.example` | UPDATED | +6 |
| `internal/config/config.go` | UPDATED | +2/-1 |
| `internal/config/features.go` | UPDATED | +16/-5 |
| `internal/config/registry_test.go` | UPDATED | +25/-9 |
| `internal/api/courier.go` | CREATED | +56 |
| `internal/api/courier_test.go` | CREATED | +91 |
| `internal/api/courier_auth.go` | CREATED | +42 |
| `internal/api/courier_auth_test.go` | CREATED | +98 |
| `internal/api/router.go` | UPDATED | +15/-3 |
| `internal/app/servers.go` | UPDATED | +9/-3 |
| `internal/app/servers_test.go` | UPDATED | +68/-24 |
| `internal/app/app_test.go` | UPDATED | +7/-4 |
| `internal/clients/kratosclient/kratosclient_e2e_test.go` | REWRITTEN | +234/-151 |
| `.env.example` | UPDATED | +3 |
| `go.mod` | UPDATED | +1/-1 (promoted `github.com/moby/moby/api` to a direct dependency via `go mod tidy`) |

`.claude/PRPs/prds/secret-santa-drawer.prd.md` — no change (already up to date).

## Deviations from Plan

All deviations were discovered empirically while getting Task 10's e2e spike test to actually pass
against a real `oryd/kratos:v1.3.1` container — none were assumed or guessed.

1. **`courier.sms.request_config` does not dispatch on this Kratos version — `courier.channels`
   does.** The plan's own "External Documentation" section cited Ory's self-hosted SMS doc page,
   which shows `courier.sms.request_config`. That shape passes Kratos's config schema validation
   with no error, but the courier worker never actually sends a request for it — no network call,
   no logged error even at `trace` level; the message just sits `"abandoned"` after its retry
   budget. Switching to `courier.channels: [{id: sms, type: http, request_config: {...}}]` (the
   shape Ory's own SMS *setup guide*, not the self-hosted page, uses) fixed it immediately —
   confirmed via Kratos's admin `/admin/courier/messages` endpoint showing `"status": "sent"`.
   **Why**: not discoverable by reading docs alone; required running a real Kratos container,
   watching `/admin/courier/messages`, and bisecting which config shape actually dispatches.

2. **`courier.templates.login_code.valid` requires an `email` sub-key even for an SMS-only
   identifier.** Kratos's config schema validation failed at startup with
   `missing properties: "email"` until an unused `login_code.valid.email` template (subject +
   plaintext + html, all base64) was added, even though this identity schema has no email trait.
   Added with a comment explaining it's dead config, present only to satisfy schema validation.

3. **OrbStack's `host.docker.internal` resolves to a reserved-range address Kratos's courier
   HTTP client hard-refuses to dial.** `prohibited IP address: 0.250.250.254 is not a permitted
   destination (denied by: 0.0.0.0/8)`. `clients.http.disallow_private_ip_ranges: false` (added to
   both `kratos.yml` and `kratos.production.yml`, matching Ory's own production-hardening guide
   language) does **not** cover this — that setting only applies to `clients.http`'s
   general-purpose client, not the courier channel's own separate guard, and no config-only
   override was found for the courier-channel guard itself. Confirmed this is specific to
   `host.docker.internal`/OrbStack's special forwarding address: container-to-container delivery
   over an ordinary Docker-bridge private IP (exactly `kratos.production.yml`'s
   `http://unwrap-gift:8082/...` architecture) worked immediately, with no guard issue at all. This
   is a **known, documented, unresolved limitation for OrbStack local dev** specifically — Docker
   Desktop for Mac/Windows and plain Linux Docker Engine are not known to hit it, since their
   `host.docker.internal` resolves to an ordinary address. No code or config change in this repo
   can fix it; `deploy/kratos/README.md`'s "Courier SMS webhook" section documents it as a known
   gap.

4. **The e2e test's stand-in webhook had to run as a container on the shared Docker network, not
   a host-side `httptest.Server`.** A direct consequence of #3: since `host.docker.internal` on
   OrbStack can't reach a host-bound listener at all (the guard rejects it before the connection
   even completes), the test's stand-in webhook now runs as its own `python:3.12-slim` container
   on the same `testcontainers` network, with `host.docker.internal` mapped (via
   `HostConfigModifier.ExtraHosts`) directly to that container's own ordinary IP address — this
   keeps `kratos.yml` itself byte-for-byte unmodified (its URL is still literally
   `http://host.docker.internal:8082/...`) while sidestepping the reserved-range block entirely.
   The stand-in prints each accepted request body to its own stdout; the test polls
   `container.Logs(ctx)` for the delivered code instead of receiving it over a Go channel, since
   the webhook is no longer an in-process `net/http` server.

5. **Kratos's native code-method login flow requires `identifier` to be resent on the second
   (code-submission) step, not just `method` + `code`.** The plan's "External Documentation"
   summary of `UpdateLoginFlowWithCodeMethod` implied a two-field second step
   (`{method, csrf_token, code}`); submitting that shape failed with
   `"Property identifier is missing."` Resending `{method: "code", identifier: <phone>, code}`
   succeeded. `submitLoginCode`'s signature gained a `phone` parameter to carry this.

6. **The first (identifier-submission) step intentionally returns HTTP 400, not 200, carrying the
   updated (still in-progress, not erroneous) flow.** This is Kratos's own native-flow signal that
   another step is needed; `doJSON`'s blanket "≥400 is fatal" check doesn't fit this one call, so
   `submitLoginIdentifier` uses a dedicated request path that accepts either `200` or `400`.

None of these deviations affect production behavior negatively — deviation 3/4 are local-dev-only
(OrbStack-specific) and don't apply to `kratos.production.yml`'s container-to-container courier
target, which was verified working directly.

## Issues Encountered

All captured above as Deviations — each was root-caused empirically (real Kratos container,
`/admin/courier/messages`, `docker network inspect`, manual `curl`/`wget` from inside running
containers) before being fixed, not guessed or worked around blindly.

## Tests Written

| Test File | Tests | Coverage |
|---|---|---|
| `internal/api/courier_test.go` | 3 (`TestMountCourierWebhook_Success/SenderError/InvalidBody`) | Webhook handler: happy path, sender error → 500, malformed body → 500 |
| `internal/api/courier_auth_test.go` | 4 (`TestCourierWebhookAuthMiddleware_ValidSecret/MissingHeader/WrongSecret/EmptyConfiguredSecret`) | Auth middleware: valid secret, missing header, wrong secret, fail-closed empty-configured-secret case |
| `internal/config/registry_test.go` | +1 case in `TestResolveFeatureEnabledState` | `kratos` feature disabled when `KratosCourierWebhookSecret` is missing, even with its other two vars set |
| `internal/app/servers_test.go` | +2 (`TestBuildServers_KratosDisabled_CourierWebhookNotMounted`, `TestBuildServers_KratosEnabled_CourierWebhookRequiresSecret`) | End-to-end regression: webhook route absent when kratos disabled; present + secret-protected + reaches a real (disabled) `*smsclient.Client` when enabled |
| `internal/clients/kratosclient/kratosclient_e2e_test.go` | 1 (`TestPhoneCodeLogin_E2E`, rewritten) | Full real-Kratos spike: admin-create identity (no self-service registration) → native code login → courier webhook delivery → session issuance |

## Next Steps
- [ ] Code review via `/code-review`
- [ ] Create PR via `/prp-pr`
- [ ] Follow-up (not this session's scope): investigate whether a real fix exists for the
      OrbStack `host.docker.internal` courier-channel SSRF block, or whether it should just stay a
      documented known-limitation indefinitely
