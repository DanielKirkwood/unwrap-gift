# Plan: SMS-code auth via Kratos + seven.io

## Summary

Replace the repo's only identity schema (email + password) with a phone-number + full-name
schema using Ory Kratos's passwordless **`code`** method, so drawer members log in with an SMS
one-time code instead of a password. Kratos's self-hosted courier is reconfigured to POST every
outgoing SMS (login codes) to a new shared-secret-protected webhook on unwrap-gift's **hidden**
router; that webhook hands the message to Phase 3's SMS-sending abstraction, which either calls
seven.io's Go SDK (production) or logs the message to stdout (local dev / `sms` feature
disabled). The organiser provisions each Member's Kratos identity via the existing admin
`CreateIdentity` pattern — there is no self-service registration flow for participants.

## User Story

As a drawer member, I want to log in with just my phone number and a one-time SMS code, so that
I don't need to remember a password and the organiser controls who's in the group.

## Problem → Solution

**Current**: `deploy/kratos/identity.schema.json` only supports email + password identities;
`deploy/kratos/kratos.yml`'s courier only speaks SMTP (to a dev-only mailslurper mock); there is
no SMS delivery mechanism anywhere in the repo.

**Desired**: A `phone` + `full_name` identity schema with the `code` method (`via: sms`) as the
sole login method; Kratos's `courier.sms.request_config` points at a new
`POST /webhooks/kratos/sms` endpoint on the hidden router, authenticated by a shared API-key
header Kratos itself sends; that handler relays the message through an `api.SMSSender` interface
that Phase 3 implements.

## Metadata

- **Complexity**: Large
- **Source PRD**: `.claude/PRPs/prds/secret-santa-drawer.prd.md`
- **PRD Phase**: Phase 2 — SMS-code auth via Kratos + seven.io
- **Estimated Files**: ~15 (2 kratos configs, 1 identity schema, 1 jsonnet template, 1
  docker-compose file, 1 production compose file, 2 new `internal/api` files + 2 test files,
  `internal/config/config.go`, `internal/config/features.go`, `internal/app/servers.go`,
  `.env.example`, the existing e2e test rewritten, `deploy/kratos/README.md`)

---

## ⚠️ Cross-Phase Contract (read first)

Per explicit direction, this plan's webhook handler **depends on Phase 3** (`internal/clients/
smsclient` + the `sms` feature flag) rather than shipping its own seven.io call. Phase 3 does not
exist yet as of this writing. **Implement Phase 3 before finishing this plan's Task 7
(app wiring)** — everything through Task 6 (schema, courier config, webhook handler + its own
unit tests) can be built and tested in complete isolation from Phase 3 (the webhook's unit tests
use a fake `api.SMSSender`), but Task 7 and the e2e spike test need a real `*App.SMS` to wire in.

Phase 3 must produce something satisfying this exact shape so Task 7 doesn't need rework:

```go
// internal/clients/smsclient
type Client struct { /* ... */ }

func New(cfg config.SMSConfig, enabled bool) (*Client, error)

// Send must NEVER be a silent no-op, even when the sms feature is disabled —
// unlike kratosclient/ketoclient's "nil is disabled" convention, a swallowed
// login code makes local dev login untestable. When enabled is false, New
// should still return a non-nil *Client whose Send logs the message via
// slog instead of calling seven.io. This is a deliberate, documented
// deviation from the nil-is-disabled convention used elsewhere.
func (c *Client) Send(ctx context.Context, to, body string) error
```

`*smsclient.Client` must satisfy this plan's `api.SMSSender` interface (Task 4):

```go
type SMSSender interface {
    Send(ctx context.Context, to, body string) error
}
```

If Phase 3 lands with a different method name/signature, only Task 7's three wiring lines in
`internal/app/servers.go` need to change — nothing else in this plan is affected.

---

## UX Design

Internal/infra change — no UI exists in this repo yet (Phase 8 builds it). The sequence that
changes:

**Before**: No SMS delivery path exists; the only identity schema is email+password; a login
flow would show a password field.

**After**:
```
Organiser                 unwrap-gift                  Kratos                  seven.io/console
    │  POST /admin/identities   │                          │                          │
    │  {phone, full_name}       │                          │                          │
    │──────────────────────────>│──(kratosclient, admin)──>│  identity created        │
    │                           │                          │  (active, no password)   │

Member's phone              unwrap-gift                  Kratos                  seven.io/console
    │  submit login flow        │                          │                          │
    │  {method:"code",          │                          │                          │
    │   identifier: phone}      │                          │                          │
    │───────────────────────────────────────────────────── >│  generates code,        │
    │                           │                          │  renders login_code      │
    │                           │  POST /webhooks/kratos/sms│  SMS template            │
    │                           │<─────────────────────────│  (shared-secret header)  │
    │                           │──SMSSender.Send(to,body)────────────────────────────>│  real SMS or
    │                           │                          │                          │  console log
    │  submit code               │                          │                          │
    │  {method:"code", code}    │                          │                          │
    │───────────────────────────────────────────────────── >│  verifies, issues session│
```

### Interaction Changes

| Touchpoint | Before | After | Notes |
|---|---|---|---|
| Identity schema | email + password | phone + full_name, code method | `deploy/kratos/identity.schema.json` |
| Login | password field | phone number → SMS code → session | Kratos-native, no app code changes needed for the flow itself |
| Registration | self-service, email+password | organiser-only, admin API | No self-service registration flow is used by the app (still nominally reachable via Kratos's own reference UI — see Risks) |
| Courier | SMTP → mailslurper (dev only, unused after this change) | SMTP unchanged (kept, see Task 2 GOTCHA) + new SMS HTTP channel → our webhook | |

---

## Mandatory Reading

| Priority | File | Lines | Why |
|---|---|---|---|
| P0 | `ARCHITECTURE.md` | all | Three-router model, nil-is-disabled, interface segregation — every convention this plan follows |
| P0 | `internal/api/router.go` | 1-119 | Exactly how to add a new route group to the hidden router without inheriting Auth/Authz |
| P0 | `internal/api/auth.go` | 1-77 | `AuthenticationMiddleware`'s nil-is-passthrough shape — the new webhook auth middleware mirrors its structure (not its semantics — see Task 4) |
| P0 | `internal/clients/kratosclient/kratosclient.go` | 1-65, 84-104 | `New`'s nil-is-disabled constructor shape and `CreateIdentity` — reused as-is for organiser Member provisioning (Phase 5/6, not this plan, but confirms the pattern already exists) |
| P0 | `internal/config/features.go` | 84-90 | `KratosConfig`/`RequiredEnv` shape — `KratosCourierWebhookSecret` is added here |
| P0 | `internal/app/servers.go` | 39-97 | Exactly where/how `RouterDeps` fields get wired per-feature; the new webhook deps slot into the existing `if kratosFeature.Enabled` block |
| P1 | `internal/api/identities.go` | 1-55 | `IdentityAdmin` interface + `MountIdentities` — the `Mount*(r chi.Router, dep, adapter Adapter)` naming/shape convention the new `MountCourierWebhook` follows |
| P1 | `internal/api/widgets.go` | 1-60 | Second worked example of the same `Mount*` pattern, for DB-backed resources (contrast, not copied) |
| P1 | `internal/api/errors.go` | 1-79 | `HandlerFunc`, `Adapter`, `ErrorsMap` — every new handler returns `error`, never writes directly |
| P1 | `internal/api/problem.go` | 1-46 | RFC 9457 `Problem` shape for the webhook's own auth-failure response |
| P1 | `internal/api/identities_test.go` | 1-77 | Fake-implementation + `httptest` test structure to mirror for the new webhook's unit tests |
| P1 | `internal/clients/kratosclient/kratosclient_e2e_test.go` | all | The `testcontainers-go` e2e pattern this plan's Task 10 rewrites — network setup, config-file mounting, native-flow HTTP helpers (`doJSON`) all reused verbatim |
| P2 | `deploy/kratos/kratos.yml` / `kratos.production.yml` | all | Current courier/selfservice/identity config being edited |
| P2 | `deploy/kratos/README.md` | all | Local-dev-vs-production delta this plan's changes must extend, not duplicate |
| P2 | `internal/config/config.go` | all | `EnvVars` reflection-based binding — confirms adding a field is sufficient, no other code to touch |

## External Documentation

| Topic | Source | Key Takeaway |
|---|---|---|
| seven.io Go SDK — correct import path | github.com/seven-io/go-client (verified against raw `sms.go`/`sms77api.go`) | **The PRD's cited path is wrong.** It is `github.com/seven-io/go-client/sms77api` (package `sms77api`), not `.../seven`. `sms77api.New(sms77api.Options{ApiKey: ...})`, then `client.Sms.JsonContext(ctx, sms77api.SmsBaseParams{To, From, Text})` → `(*SmsResponse, error)`. This matters for Phase 3, not this plan directly, but the webhook's `SMSSender` contract above is shaped to make that call trivial to drop in. |
| Kratos self-hosted SMS courier | `docs/kratos/self-hosted/10_sending-sms.mdx` (Ory docs, via context7) | Self-hosted Kratos (our `version: v1.3.0`) uses `courier.sms.request_config: {url, method, body (jsonnet), headers, auth}` — NOT the `courier.channels` array format (that's a separate/newer Ory Network config shape seen in other doc pages; don't mix the two). `auth.type: api_key` sends a configurable header — exactly the shared-secret mechanism this plan uses. |
| Kratos courier jsonnet `ctx` | `docs/kratos/emails-sms/10_sending-sms.mdx` | The jsonnet function receives `ctx` with `recipient`, `body`, `template_type`, `template_data`, `message_type`, `request_headers`. `function(ctx) { to: ctx.recipient, body: ctx.body }` returned as a plain object is serialized as the JSON request body when `Content-Type` defaults to `application/json` (no header override needed, unlike the Twilio/form-encoded examples). |
| Kratos `code` identity schema | `docs/kratos/manage-identities/15_customize-identity-schema.mdx` | `"code": {"identifier": true, "via": "sms"}` under a trait's `ory.sh/kratos.credentials` marks that trait as the phone-code login identifier. |
| Enabling passwordless code | `src/components/Shared/kratos/passwordless/07_code.mdx` | `selfservice.methods.code.passwordless_enabled: true` is the one flag needed; there's no separate registration-vs-login toggle for the code method itself. |
| Login flow submission shape | `github.com/ory/kratos-client-go` `UpdateLoginFlowWithCodeMethod` docs | Two-step native submission: (1) `{method:"code", csrf_token, identifier: <phone>}` triggers code send; (2) `{method:"code", csrf_token, code: <6 digits>}` completes login and returns a session. **Kratos's own docs note the identifier step "requires that the user has already completed registration or settings with the code flow"** — for an admin-created identity (no self-service registration ever run), this needs to be verified by this plan's Task 10 spike; if it doesn't just work off the trait alone, Task 10 documents the workaround found. |
| SMS courier templates | `docs/kratos/emails-sms/10_sending-sms.mdx` | `courier.templates.login_code.valid.sms.body.plaintext` is a `base64://`-prefixed Go template string, e.g. templating `{{ .LoginCode }}`. |

---

## Patterns to Mirror

### NAMING_CONVENTION — `Mount*` + interface-per-consumer
// SOURCE: `internal/api/identities.go:20-26,49`
```go
type IdentityAdmin interface {
    CreateIdentity(ctx context.Context, traits map[string]any, password string) (*kratos.Identity, error)
    // ...
}

func MountIdentities(r chi.Router, identities IdentityAdmin, adapter Adapter) {
    r.Post("/admin/identities", adapter.Adapt(createIdentity(identities)))
    // ...
}
```
New code: `SMSSender` interface + `MountCourierWebhook(r chi.Router, sender SMSSender, adapter Adapter)`, same shape.

### ERROR_HANDLING — `HandlerFunc` returns `error`, `Adapter` maps it
// SOURCE: `internal/api/errors.go:12-15,40-57`
```go
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

func (a Adapter) Adapt(fn HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        err := fn(w, r)
        if err == nil { return }
        problem, ok := a.ErrorsMap.lookup(err)
        if !ok { a.logUnmapped(r.Context(), err); problem = Problem{Status: http.StatusInternalServerError, ...} }
        _ = WriteProblem(w, problem)
    }
}
```
The webhook handler never writes to `w` directly; an `SMSSender.Send` error just returns unmapped (→ 500, and Kratos's courier retries per its own `message_retries` config — desired, since a transient seven.io failure should be retried, not dropped).

### MIDDLEWARE_PATTERN — nil-is-passthrough, but NOT for this one
// SOURCE: `internal/api/auth.go:53-76`
```go
func AuthenticationMiddleware(validator SessionValidator) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if validator == nil { next.ServeHTTP(w, r); return }
            // ...
        })
    }
}
```
**Deliberate deviation**: `CourierWebhookAuthMiddleware` is only ever constructed (never nil-checked at the call site) inside the existing `if kratosFeature.Enabled` block in `servers.go`, because `KratosCourierWebhookSecret` becomes part of `KratosConfig.RequiredEnv` (Task 3) — the kratos feature simply isn't enabled without it. No nil-passthrough branch is needed or wanted here: an auth middleware protecting a route that triggers billed SMS sends must fail closed, not pass through silently.

### FEATURE_FLAG_PATTERN — RequiredEnv gates a whole feature
// SOURCE: `internal/config/features.go:84-90`
```go
r.Register(Feature{
    Name:        "kratos",
    RequiredEnv: []string{"KratosPublicURL", "KratosAdminURL"},
    Build: func(e EnvVars) any {
        return KratosConfig{PublicURL: e.KratosPublicURL, AdminURL: e.KratosAdminURL}
    },
})
```
`KratosCourierWebhookSecret` is added as a third `RequiredEnv` entry and a third `KratosConfig` field — the webhook is inseparable from the kratos feature being on at all, so it doesn't need its own `Feature`/`RequiredEnv` entry.

### ROUTER_GROUPING — sibling `r.Group`s, not nested
// SOURCE: `internal/api/router.go:75-91`
```go
func NewHiddenRouter(deps RouterDeps) *chi.Mux {
    r := newRouter("hidden", deps)
    r.Group(func(r chi.Router) {
        if deps.Auth != nil { r.Use(deps.Auth) }
        if deps.Authz != nil { r.Use(deps.Authz) }
        if deps.Identities != nil { MountIdentities(r, deps.Identities, deps.IdentitiesAdapter) }
    })
    return r
}
```
The courier webhook group is a **second, sibling** `r.Group` in the same function — it must not be nested inside the Auth/Authz group, or Kratos's webhook calls (which carry no session cookie) would 401 against `AuthenticationMiddleware`.

### TEST_STRUCTURE — fake + `httptest`, no real dependency
// SOURCE: `internal/api/identities_test.go:24-77`
```go
type fakeIdentityAdmin struct { /* records last call args */ }
func mountTestIdentities(identities api.IdentityAdmin) http.Handler {
    r := chi.NewRouter()
    api.MountIdentities(r, identities, testAdapter())
    return r
}
```
New: `fakeSMSSender` records `(to, body)` and returns a configurable `err`; `mountTestCourierWebhook` mirrors `mountTestIdentities`.

### E2E_TEST_STRUCTURE — testcontainers-go, real Kratos, no compose
// SOURCE: `internal/clients/kratosclient/kratosclient_e2e_test.go:51-94,154-196,233-255`
Spins up Postgres + Kratos (+ previously mailslurper) via `testcontainers-go` on a private network,
mounting `deploy/kratos/{kratos.yml,identity.schema.json}` unmodified so the real config file is
under test, not a duplicate. Task 10 extends this file rather than replacing its scaffolding
(`startPostgres`, `runKratosMigrate`, `kratosConfigFiles`, `doJSON`, `flowResponse` all reused
verbatim).

---

## Files to Change

| File | Action | Justification |
|---|---|---|
| `deploy/kratos/identity.schema.json` | REWRITE | New `phone` (code identifier, via sms) + `full_name` traits, no more email/password |
| `deploy/kratos/kratos.yml` | UPDATE | Drop `methods.password`, add `methods.code.passwordless_enabled`, add `courier.sms.request_config` + `courier.templates.login_code`, add dev webhook secret |
| `deploy/kratos/kratos.production.yml` | UPDATE | Same shape, `${KRATOS_COURIER_WEBHOOK_SECRET}` templated, container-name webhook URL |
| `deploy/kratos/login_code.sms.jsonnet` | CREATE | Jsonnet body template Kratos evaluates per outgoing SMS |
| `deploy/kratos/docker-compose.kratos.yml` | UPDATE | `extra_hosts: host.docker.internal:host-gateway` on the `kratos` service so it can reach the host-run app |
| `deploy/kratos/README.md` | UPDATE | Document the new courier webhook, shared secret, and `host.docker.internal` reachability requirement |
| `deploy/production/docker-compose.yml` | UPDATE | Pass `KRATOS_COURIER_WEBHOOK_SECRET` to both `kratos` and `unwrap-gift` services |
| `deploy/production/.env.production.example` | UPDATE | Add `KRATOS_COURIER_WEBHOOK_SECRET=` placeholder |
| `internal/config/config.go` | UPDATE | Add `KratosCourierWebhookSecret` field to `EnvVars` |
| `internal/config/features.go` | UPDATE | Add field to `KratosConfig`, third entry in `kratos` feature's `RequiredEnv` |
| `internal/config/registry_test.go` | UPDATE (if it exercises the kratos feature's RequiredEnv) | Extend any existing kratos-feature-enablement test cases to cover the new required env var |
| `internal/api/courier.go` | CREATE | `SMSSender` interface, `MountCourierWebhook`, handler |
| `internal/api/courier_test.go` | CREATE | Unit tests: fake `SMSSender`, happy path, decode-error, sender-error |
| `internal/api/courier_auth.go` | CREATE | `CourierWebhookAuthMiddleware` |
| `internal/api/courier_auth_test.go` | CREATE | Unit tests: correct secret passes, wrong/missing secret 401s, constant-time comparison |
| `internal/api/router.go` | UPDATE | New `RouterDeps` fields; new sibling `r.Group` in `NewHiddenRouter` |
| `internal/app/servers.go` | UPDATE | Wire `deps.CourierWebhookAuth`/`CourierSMS`/`CourierAdapter` inside the existing `if kratosFeature.Enabled` block |
| `.env.example` | UPDATE | Add `KRATOS_COURIER_WEBHOOK_SECRET=` with comment |
| `internal/clients/kratosclient/kratosclient_e2e_test.go` | REWRITE | Replace email/password/verification flow with admin-create + phone/code login flow; capture the SMS code via a test-side webhook stand-in instead of mailslurper |
| `.claude/PRPs/prds/secret-santa-drawer.prd.md` | UPDATE | Phase 2 row: status → in-progress, `Depends` column `-` → `3` (per the cross-phase contract above), PRP Plan link added |

## NOT Building

- Phase 3's actual `internal/clients/smsclient` implementation (seven.io call, `sms` feature
  registration) — only its required contract shape is specified above for Task 7 to consume.
- Self-service registration UI/flow for participants — organiser-only admin provisioning, per the
  chosen design. The Kratos reference self-service UI's registration page remains technically
  reachable (see Risks) but is not part of this plan's scope to close off.
- Recovery or verification SMS flows — the PRD's MVP scope only requires login; the identity
  schema's `phone` trait does not set `recovery`/`verification` via SMS.
- MFA / `mfa_enabled` on the code method — single-factor passwordless only.
- Any Drawer/Member/domain persistence — that's Phase 1 (already in progress) and Phase 5/6; this
  plan touches only Kratos config and the courier webhook.
- Removing `mailslurper` from the dev compose stack or `courier.smtp` from `kratos.yml` — see
  Task 2's GOTCHA; left in place pending confirmation Kratos's courier worker doesn't require a
  valid SMTP config to start at all.

---

## Step-by-Step Tasks

### Task 1: Rewrite the identity schema
- **ACTION**: Replace `deploy/kratos/identity.schema.json` entirely.
- **IMPLEMENT**:
  ```json
  {
    "$id": "https://schemas.unwrap-gift.dev/identity.schema.json",
    "$schema": "http://json-schema.org/draft-07/schema#",
    "title": "Person",
    "type": "object",
    "properties": {
      "traits": {
        "type": "object",
        "properties": {
          "phone": {
            "type": "string",
            "format": "tel",
            "title": "Phone Number",
            "ory.sh/kratos": {
              "credentials": {
                "code": { "identifier": true, "via": "sms" }
              }
            }
          },
          "full_name": {
            "type": "string",
            "title": "Full Name",
            "minLength": 1
          }
        },
        "required": ["phone", "full_name"],
        "additionalProperties": false
      }
    }
  }
  ```
- **MIRROR**: Same top-level shape as the current file (`deploy/kratos/identity.schema.json`, read in full above); only the `properties.traits.properties` contents and `required` list change.
- **IMPORTS**: N/A (JSON config).
- **GOTCHA**: `format: "tel"` on `phone` does light validation but does not normalize to E.164 — decide at Member-creation time (Phase 5/6, not here) whether to require callers to submit E.164-formatted numbers, since seven.io's `To` field expects that format.
- **VALIDATE**: `jq . deploy/kratos/identity.schema.json` parses cleanly; `task kratos:up` boots without a schema-validation error in `task kratos:logs`.

### Task 2: Update `kratos.yml` (dev) and `kratos.production.yml`
- **ACTION**: In both files: remove `selfservice.methods.password`; add `selfservice.methods.code.passwordless_enabled: true`; add `courier.sms.request_config` and `courier.templates.login_code`.
- **IMPLEMENT** (dev `kratos.yml`):
  ```yaml
  selfservice:
    methods:
      code:
        passwordless_enabled: true

  courier:
    smtp:
      connection_uri: smtps://test:test@mailslurper:1025/?skip_ssl_verify=true
    sms:
      request_config:
        url: http://host.docker.internal:8082/webhooks/kratos/sms
        method: POST
        body: file:///etc/config/kratos/login_code.sms.jsonnet
        headers:
          Content-Type: application/json
        auth:
          type: api_key
          config:
            name: X-Courier-Webhook-Secret
            value: dev-courier-webhook-secret-not-secure
            in: header
    templates:
      login_code:
        valid:
          sms:
            body:
              plaintext: "base64://<computed — see below>"
  ```
  Compute the base64 template body with:
  ```sh
  printf 'Your Secret Santa login code is: {{ .LoginCode }}' | base64
  ```
  and paste the result after `base64://`.
- **MIRROR**: `deploy/kratos/kratos.yml:25-27` (method block being replaced), `:86-93` (courier block being extended, not replaced).
- **IMPORTS**: N/A.
- **GOTCHA**: Left `courier.smtp` untouched deliberately — self-hosted Kratos's courier worker may fail to start without *some* valid SMTP config even when no email templates are rendered. Confirm this empirically in Task 10's e2e run (`task kratos:logs` after `task kratos:up` with this change) — if Kratos boots fine with `courier.smtp` removed, that's an easy follow-up cleanup, but don't remove it preemptively in this task.
- **GOTCHA**: `host.docker.internal` requires Task 8's `extra_hosts` change on the `kratos` service — without it, this URL won't resolve on Linux Docker Engine (works automatically on Docker Desktop for Mac).
- **VALIDATE**: `task kratos:up && task kratos:logs` — no config-parse errors, Kratos reaches "ready" (matches the existing e2e test's `wait.ForHTTP("/health/ready")` check).

For `kratos.production.yml`, mirror the same `selfservice.methods.code` change, and:
```yaml
courier:
  smtp:
    connection_uri: ${SMTP_CONNECTION_URI}
  sms:
    request_config:
      url: http://unwrap-gift:8082/webhooks/kratos/sms
      method: POST
      body: file:///etc/config/kratos/login_code.sms.jsonnet
      headers:
        Content-Type: application/json
      auth:
        type: api_key
        config:
          name: X-Courier-Webhook-Secret
          value: ${KRATOS_COURIER_WEBHOOK_SECRET}
          in: header
  templates:
    login_code:
      valid:
        sms:
          body:
            plaintext: "base64://<same value as dev>"
```
No `host.docker.internal` needed here — `unwrap-gift` is a named service on the same default
compose network (`deploy/production/docker-compose.yml` has no explicit per-service `networks:`,
so every service shares the implicit default network and can reach each other by service name).

### Task 3: Add the webhook secret to config
- **ACTION**: Add `KratosCourierWebhookSecret` end-to-end through the config layer.
- **IMPLEMENT**:
  ```go
  // internal/config/config.go, in EnvVars, next to the other Kratos* fields
  KratosCourierWebhookSecret string `env:"KRATOS_COURIER_WEBHOOK_SECRET"`
  ```
  ```go
  // internal/config/features.go
  type KratosConfig struct {
      PublicURL            string
      AdminURL             string
      CourierWebhookSecret string
  }

  r.Register(Feature{
      Name:        "kratos",
      RequiredEnv: []string{"KratosPublicURL", "KratosAdminURL", "KratosCourierWebhookSecret"},
      Build: func(e EnvVars) any {
          return KratosConfig{
              PublicURL:            e.KratosPublicURL,
              AdminURL:             e.KratosAdminURL,
              CourierWebhookSecret: e.KratosCourierWebhookSecret,
          }
      },
  })
  ```
- **MIRROR**: `internal/config/config.go:30-31`, `internal/config/features.go:84-90` (exact blocks being extended).
- **IMPORTS**: None new.
- **GOTCHA**: This makes the `kratos` feature's enablement stricter — any existing `.env`/CI config that sets only `KRATOS_PUBLIC_URL`/`KRATOS_ADMIN_URL` will now see `kratos` (and therefore `keto`, which `Requires: ["kratos"]`) silently disable itself per `ResolveFeatureEnabledState`'s existing "missing required env" behavior. Update `.env.example` (Task 9) in the same change so this isn't a surprise.
- **VALIDATE**: `go build ./...`; `go test ./internal/config/...`.

### Task 4: `SMSSender` interface, webhook handler, and its mount function
- **ACTION**: Create `internal/api/courier.go`.
- **IMPLEMENT**:
  ```go
  package api

  import (
      "context"
      "encoding/json"
      "fmt"
      "net/http"

      "github.com/go-chi/chi/v5"
  )

  // SMSSender sends a single SMS message. Implemented by
  // internal/clients/smsclient.Client (Phase 3); api depends on this
  // interface instead of that concrete type so it never imports
  // internal/clients, per ARCHITECTURE.md's dependency-direction rule.
  //
  // Unlike SessionValidator/PermissionChecker, a nil SMSSender is never
  // wired here — servers.go only sets RouterDeps.CourierSMS once the kratos
  // feature (and therefore the webhook route) is enabled, and Phase 3's
  // Client.Send must itself never silently drop a message (see this plan's
  // Cross-Phase Contract) even when the sms feature is disabled.
  type SMSSender interface {
      Send(ctx context.Context, to, body string) error
  }

  // courierWebhookRequest is the JSON body our own login_code.sms.jsonnet
  // template produces — see deploy/kratos/login_code.sms.jsonnet.
  type courierWebhookRequest struct {
      To   string `json:"to"`
      Body string `json:"body"`
  }

  // MountCourierWebhook adds the Kratos courier's outgoing-SMS webhook to r,
  // under /webhooks/kratos/sms. It must be mounted on a route group that
  // does NOT carry AuthenticationMiddleware/AuthorizationMiddleware — the
  // caller is Kratos itself, authenticated instead by
  // CourierWebhookAuthMiddleware (see courier_auth.go).
  func MountCourierWebhook(r chi.Router, sender SMSSender, adapter Adapter) {
      r.Post("/webhooks/kratos/sms", adapter.Adapt(sendCourierSMS(sender)))
  }

  func sendCourierSMS(sender SMSSender) HandlerFunc {
      return func(w http.ResponseWriter, r *http.Request) error {
          var req courierWebhookRequest
          if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
              return fmt.Errorf("api: decode courier webhook request: %w", err)
          }

          if err := sender.Send(r.Context(), req.To, req.Body); err != nil {
              return fmt.Errorf("api: send courier sms: %w", err)
          }

          w.WriteHeader(http.StatusNoContent)
          return nil
      }
  }
  ```
- **MIRROR**: `internal/api/identities.go`'s `createIdentity`/`MountIdentities` shape exactly.
- **IMPORTS**: `context`, `encoding/json`, `fmt`, `net/http`, `github.com/go-chi/chi/v5`.
- **GOTCHA**: Returns `204 No Content` on success rather than `WriteData`'s `{"data": ...}` envelope — there's no payload to return, and Kratos's courier only checks the HTTP status for delivery success (any 2xx), not a response body. Confirm this assumption during Task 10's spike; if Kratos does inspect the body, switch to `WriteData(w, http.StatusOK, struct{}{})`.
- **VALIDATE**: `go build ./...` (interface satisfied once Task 7 wires a concrete type).

### Task 5: Shared-secret webhook auth middleware
- **ACTION**: Create `internal/api/courier_auth.go`.
- **IMPLEMENT**:
  ```go
  package api

  import (
      "crypto/subtle"
      "net/http"
  )

  const courierWebhookSecretHeader = "X-Courier-Webhook-Secret"

  func courierWebhookUnauthorizedProblem() Problem {
      return Problem{
          Status: http.StatusUnauthorized,
          Title:  "Unauthorized",
          Detail: "a valid courier webhook secret is required",
      }
  }

  // CourierWebhookAuthMiddleware requires every request to carry the
  // configured secret in the X-Courier-Webhook-Secret header — the same
  // header name and value Kratos's courier.sms.request_config.auth
  // (type: api_key) is configured to send. Unlike
  // AuthenticationMiddleware/AuthorizationMiddleware, there is no
  // nil/empty-is-passthrough case: this route triggers a billed SMS send,
  // so it must fail closed. servers.go only constructs this middleware
  // inside its existing "kratos feature enabled" branch, where
  // KratosConfig.CourierWebhookSecret is guaranteed non-empty (it's a
  // RequiredEnv entry — see internal/config/features.go).
  func CourierWebhookAuthMiddleware(secret string) func(http.Handler) http.Handler {
      return func(next http.Handler) http.Handler {
          return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
              got := r.Header.Get(courierWebhookSecretHeader)
              if secret == "" || got == "" ||
                  subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
                  _ = WriteProblem(w, courierWebhookUnauthorizedProblem())
                  return
              }

              next.ServeHTTP(w, r)
          })
      }
  }
  ```
- **MIRROR**: `internal/api/auth.go`'s overall middleware shape (constructor takes the dependency, returns `func(http.Handler) http.Handler`), but with fail-closed instead of nil-passthrough semantics per the pattern note above.
- **IMPORTS**: `crypto/subtle`, `net/http`.
- **GOTCHA**: `subtle.ConstantTimeCompare` requires equal-length slices to be meaningful against timing attacks for equal-but-wrong-length secrets; comparing against a fixed-length HMAC would be stronger, but a shared high-entropy random string (this plan's dev/prod secret) is adequate for a single-organiser, low-traffic internal webhook — don't over-engineer this beyond what a service-to-service shared secret needs.
- **VALIDATE**: Unit tests in Task 6.

### Task 6: Unit tests for the webhook and its auth middleware
- **ACTION**: Create `internal/api/courier_test.go` and `internal/api/courier_auth_test.go`.
- **IMPLEMENT** (`courier_test.go`, mirroring `identities_test.go`'s `fakeIdentityAdmin`):
  ```go
  type fakeSMSSender struct {
      err              error
      lastTo, lastBody string
  }

  func (f *fakeSMSSender) Send(_ context.Context, to, body string) error {
      f.lastTo, f.lastBody = to, body
      return f.err
  }
  ```
  Test cases: valid JSON body → 204 and `fakeSMSSender` recorded `to`/`body` correctly; sender
  returns an error → 500 (unmapped, matching `TestMountIdentities_InvalidBody`'s assertion style);
  malformed JSON body → 500.

  `courier_auth_test.go`: request with correct header → next handler called (use a spy
  `http.HandlerFunc` that sets a bool); missing header → 401; wrong value → 401; empty configured
  secret → 401 even with an empty request header (fail-closed case).
- **MIRROR**: `internal/api/identities_test.go:34-41,79-120` end to end — same `httptest.NewRequest`/`httptest.NewRecorder`/`router.ServeHTTP` structure.
- **IMPORTS**: `bytes`, `context`, `net/http`, `net/http/httptest`, `testing`, `github.com/go-chi/chi/v5`, `github.com/DanielKirkwood/unwrap-gift/internal/api`.
- **GOTCHA**: None beyond the existing suite's conventions.
- **VALIDATE**: `go test ./internal/api/... -run Courier -v`.

### Task 7: Wire into `RouterDeps`, `NewHiddenRouter`, and `servers.go`
- **ACTION**: Extend `internal/api/router.go` and `internal/app/servers.go`.
- **IMPLEMENT** (`router.go`, add to `RouterDeps`):
  ```go
  // CourierWebhookAuth, CourierSMS, and CourierAdapter, when CourierSMS is
  // non-nil, mount the Kratos courier's outgoing-SMS webhook on the hidden
  // router, in its own route group (NOT behind Auth/Authz — the caller is
  // Kratos itself). All three are set only when the kratos feature is
  // enabled.
  CourierWebhookAuth func(http.Handler) http.Handler
  CourierSMS         SMSSender
  CourierAdapter     Adapter
  ```
  In `NewHiddenRouter`, add a **sibling** group (not nested in the existing Auth/Authz one):
  ```go
  r.Group(func(r chi.Router) {
      if deps.CourierWebhookAuth != nil {
          r.Use(deps.CourierWebhookAuth)
      }
      if deps.CourierSMS != nil {
          MountCourierWebhook(r, deps.CourierSMS, deps.CourierAdapter)
      }
  })
  ```
- **IMPLEMENT** (`servers.go`, inside the existing `if kratosFeature.Enabled` block, after the current `deps.Identities`/`deps.IdentitiesAdapter` assignment):
  ```go
  deps.CourierWebhookAuth = api.CourierWebhookAuthMiddleware(kratosCfg.CourierWebhookSecret)
  deps.CourierSMS = a.SMS // *smsclient.Client from Phase 3 — see Cross-Phase Contract
  deps.CourierAdapter = api.Adapter{Logger: a.Logger, ErrorsMap: api.ErrorsMap{}}
  ```
  This requires `App.SMS` to exist (Phase 3's `internal/app/app.go` addition, mirroring
  `App.Kratos`/`App.Keto`'s field + `Bootstrap` wiring exactly).
- **MIRROR**: `internal/app/servers.go:49-66` (the existing kratos-feature block being extended).
- **IMPORTS**: None new in `servers.go` (already imports `internal/api`, `internal/clients/kratosclient`).
- **GOTCHA**: If Phase 3 isn't implemented yet when this task is reached, this is the one place
  the plan cannot compile without it — see the Cross-Phase Contract section. Do not stub `a.SMS`
  with a fake here; implement Phase 3 first.
- **VALIDATE**: `go build ./...`; `go vet ./...`.

### Task 8: `docker-compose.kratos.yml` networking + jsonnet body file
- **ACTION**: Add `extra_hosts` to the `kratos` service; create `deploy/kratos/login_code.sms.jsonnet`; mount it into the container.
- **IMPLEMENT**:
  ```yaml
  # deploy/kratos/docker-compose.kratos.yml, kratos service
  kratos:
    # ...unchanged...
    extra_hosts:
      - "host.docker.internal:host-gateway"
    volumes:
      - ./kratos.yml:/etc/config/kratos/kratos.yml
      - ./identity.schema.json:/etc/config/kratos/identity.schema.json
      - ./login_code.sms.jsonnet:/etc/config/kratos/login_code.sms.jsonnet
  ```
  ```jsonnet
  // deploy/kratos/login_code.sms.jsonnet
  function(ctx) {
    to: ctx.recipient,
    body: ctx.body,
  }
  ```
- **MIRROR**: `deploy/kratos/docker-compose.kratos.yml`'s existing `kratos` service block (volumes list already mounts two files the same way).
- **IMPORTS**: N/A.
- **GOTCHA**: `host.docker.internal:host-gateway` is a Docker Engine 20.10+ feature; confirm the
  local Docker version supports it (`docker version`) — extremely likely on any current install,
  but note it rather than assume silently.
- **VALIDATE**: `task kratos:down && task kratos:up` picks up both changes; `docker compose -f deploy/kratos/docker-compose.kratos.yml config` shows the new mount and `extra_hosts` entry.

### Task 9: `.env.example`, `deploy/production/.env.production.example`, `deploy/production/docker-compose.yml`
- **ACTION**: Propagate the new secret through every env template and the production compose file.
- **IMPLEMENT**:
  ```sh
  # .env.example, under the kratos section
  KRATOS_COURIER_WEBHOOK_SECRET=
  # must match deploy/kratos/kratos.yml's courier.sms.request_config.auth.config.value
  # (dev-courier-webhook-secret-not-secure) when running against the local dev Kratos stack.
  ```
  ```sh
  # deploy/production/.env.production.example
  KRATOS_COURIER_WEBHOOK_SECRET=
  ```
  ```yaml
  # deploy/production/docker-compose.yml, kratos service environment: add
  KRATOS_COURIER_WEBHOOK_SECRET: ${KRATOS_COURIER_WEBHOOK_SECRET}
  # unwrap-gift service environment: add
  KRATOS_COURIER_WEBHOOK_SECRET: ${KRATOS_COURIER_WEBHOOK_SECRET}
  ```
- **MIRROR**: `.env.example`'s existing kratos section; `docker-compose.yml`'s existing per-service `environment:` allowlisting (explicit vars only, never a blanket `env_file:` — see that file's own header comment).
- **IMPORTS**: N/A.
- **GOTCHA**: Both services need the *same* value — it's one shared secret, not a keypair. Say so in a comment so a future edit doesn't desync them.
- **VALIDATE**: `docker compose -f deploy/production/docker-compose.yml --env-file deploy/production/.env.production.example config` (with a filled-in copy) resolves both services' env correctly.

### Task 10: Rewrite the Kratos e2e spike test
- **ACTION**: Replace `TestRegistrationAndVerification_E2E` in `internal/clients/kratosclient/kratosclient_e2e_test.go` with a test proving the full admin-create → code-login → webhook-delivery path, using a stand-in HTTP server in place of a real unwrap-gift instance for the webhook target.
- **IMPLEMENT**: Keep `startPostgres`/`runKratosMigrate`/`kratosConfigFiles`/`doJSON`/`flowResponse` unchanged (still correct — they just mount whatever's now in `deploy/kratos/`). Replace `startMailslurper` + the email-specific helpers with:
  ```go
  const testWebhookSecret = "dev-courier-webhook-secret-not-secure"

  var loginCodeRe = regexp.MustCompile(`code is:\s*(\d{6})`)

  func TestPhoneCodeLogin_E2E(t *testing.T) {
      t.Parallel()
      ctx := t.Context()

      nw, err := tcnetwork.New(ctx)
      if err != nil {
          t.Fatalf("create network: %v", err)
      }
      testcontainers.CleanupNetwork(t, nw)

      startPostgres(ctx, t, nw.Name)
      runKratosMigrate(ctx, t, nw.Name)

      // Stand-in for unwrap-gift's /webhooks/kratos/sms: a local HTTP
      // server the Kratos container reaches via host.docker.internal
      // (same mechanism as local dev — see docker-compose.kratos.yml's
      // extra_hosts). Captures the delivered login code from the SMS
      // body text via the login_code.sms.jsonnet template's plaintext.
      codeCh := make(chan string, 1)
      webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          if r.Header.Get("X-Courier-Webhook-Secret") != testWebhookSecret {
              w.WriteHeader(http.StatusUnauthorized)
              return
          }
          var body struct {
              To   string `json:"to"`
              Body string `json:"body"`
          }
          _ = json.NewDecoder(r.Body).Decode(&body)
          if m := loginCodeRe.FindStringSubmatch(body.Body); m != nil {
              codeCh <- m[1]
          }
          w.WriteHeader(http.StatusNoContent)
      }))
      defer webhook.Close()

      publicURL, adminURL := startKratos(ctx, t, nw.Name) // extended to set
          // HostConfigModifier: func(hc *container.HostConfig) {
          //     hc.ExtraHosts = []string{"host.docker.internal:host-gateway"}
          // }

      client, err := kratosclient.New(config.KratosConfig{PublicURL: publicURL, AdminURL: adminURL}, true)
      if err != nil {
          t.Fatalf("kratosclient.New: %v", err)
      }

      const phone = "+15550001234"
      identity, err := client.CreateIdentity(ctx, map[string]any{
          "phone": phone, "full_name": "Test Participant",
      }, "")
      if err != nil {
          t.Fatalf("CreateIdentity: %v", err)
      }

      flowID := startNativeLoginFlow(ctx, t, publicURL)
      submitLoginIdentifier(ctx, t, publicURL, flowID, phone)

      var code string
      select {
      case code = <-codeCh:
      case <-time.After(verificationCodeTimeout):
          t.Fatal("no SMS delivered to webhook within timeout")
      }

      loggedInIdentityID := submitLoginCode(ctx, t, publicURL, flowID, code)
      if loggedInIdentityID != identity.Id {
          t.Fatalf("logged-in identity = %s, want %s", loggedInIdentityID, identity.Id)
      }
  }
  ```
  Add small `startNativeLoginFlow`/`submitLoginIdentifier`/`submitLoginCode` helpers mirroring
  `registerNative`'s `doJSON` usage, hitting `/self-service/login/api` and
  `/self-service/login?flow=...` with `{"method":"code", ...}` bodies per the
  `UpdateLoginFlowWithCodeMethod` shape documented above.
- **MIRROR**: The rest of `kratosclient_e2e_test.go`'s helper style (`t.Helper()`, `doJSON`, `wait.ForHTTP`).
- **IMPORTS**: Add `net/http/httptest`, keep `regexp` (already imported); drop `slices`/`strings`/mailslurper-specific helpers if no longer used, but keep `startPostgres`/`runKratosMigrate`/`kratosConfigFiles`/`doJSON`/`flowResponse` and their imports.
- **GOTCHA**: This is the task that actually answers the open question flagged in External
  Documentation — whether an admin-created identity (no self-service registration ever run) can
  complete a code login immediately. **If `submitLoginIdentifier` errors** (e.g. Kratos requires
  the code credential to be "completed via registration or settings" first, per its own docs
  wording), the fallback is to additionally call Kratos's admin identity-credentials import (seen
  in the Ory docs' "Import Identity" examples) — but try the direct path first; it's very likely
  the credential requirement is about the trait being schema-marked as an identifier at all
  (which `CreateIdentity` already satisfies), not a separate setup step, since `code` isn't a
  storable credential blob the way `password`'s hash is.
- **GOTCHA**: `startKratos`'s `testcontainers.ContainerRequest` needs `HostConfigModifier` setting
  `ExtraHosts: []string{"host.docker.internal:host-gateway"}` — a new addition to the existing
  helper, not present in the current file.
- **VALIDATE**: `task test:e2e` (i.e. `go test -tags=e2e ./...`) passes. This is the task's actual
  "spike" — the PRD's stated success signal for Phase 2 ("a test identity can register with phone
  + full name and complete a login flow via a real SMS code") is satisfied by this test, adjusted
  for the chosen admin-provisioning design (no self-service registration).

### Task 11: Update `deploy/kratos/README.md`
- **ACTION**: Document the new courier webhook, its shared secret, and the `host.docker.internal` requirement; remove/adjust any instructions that referenced registering via email at `http://127.0.0.1:4455/registration` (now phone+code, and admin-provisioned rather than self-service).
- **IMPLEMENT**: Short new subsection, e.g. "Courier SMS webhook" explaining: Kratos POSTs login
  codes to `unwrap-gift`'s `/webhooks/kratos/sms` on the hidden port; in local dev this crosses
  from the Kratos compose network to the host-run app via `host.docker.internal`; the shared
  secret in `kratos.yml` must match `KRATOS_COURIER_WEBHOOK_SECRET` in `.env`; with `sms`
  disabled (Phase 3, local dev default), the message is logged to `unwrap-gift`'s stdout instead
  of sent.
- **MIRROR**: The file's existing prose style and "What changes for production" section structure.
- **IMPORTS**: N/A.
- **GOTCHA**: None.
- **VALIDATE**: Read-through only; no automated check.

### Task 12: Update the PRD
- **ACTION**: `.claude/PRPs/prds/secret-santa-drawer.prd.md`, Phase 2's row in the Implementation Phases table.
- **IMPLEMENT**: Set `Status` → `in-progress`; set `Depends` → `3` (this plan's webhook consumes
  Phase 3's `SMSSender` implementation per the chosen cross-phase design — see this plan's
  Cross-Phase Contract section); add this plan's path to the `PRP Plan` column alongside the
  existing issue link.
- **VALIDATE**: Diff review only.

---

## Testing Strategy

### Unit Tests

| Test | Input | Expected Output | Edge Case? |
|---|---|---|---|
| `TestMountCourierWebhook_Success` | valid `{"to":"+1...","body":"..."}`, fake sender returns nil | 204, fake recorded exact to/body | |
| `TestMountCourierWebhook_SenderError` | valid body, fake sender returns an error | 500 (unmapped) | Yes — Kratos's own retry relies on this staying a 5xx, not swallowed |
| `TestMountCourierWebhook_InvalidBody` | malformed JSON | 500 | Yes |
| `TestCourierWebhookAuthMiddleware_ValidSecret` | correct header value | next handler invoked | |
| `TestCourierWebhookAuthMiddleware_MissingHeader` | no header | 401 | Yes |
| `TestCourierWebhookAuthMiddleware_WrongSecret` | wrong header value | 401 | Yes |
| `TestCourierWebhookAuthMiddleware_EmptyConfiguredSecret` | empty `secret` param, any/no header | 401 (fail closed) | Yes — misconfiguration case |
| `internal/config` — kratos feature enablement | `KRATOS_COURIER_WEBHOOK_SECRET` unset, other two Kratos vars set | `kratos` feature `Enabled == false`, `Reason` mentions the missing var | Yes — regression for Task 3's stricter `RequiredEnv` |

### Edge Cases Checklist
- [x] Empty input (malformed/empty webhook body) — Task 6
- [x] Invalid types (non-string JSON fields) — covered by the same decode-error path
- [ ] Maximum size input — not meaningful here (SMS bodies are inherently short); skip
- [x] Permission denied (wrong/missing shared secret) — Task 6
- [ ] Concurrent access — not meaningful for a stateless webhook handler; skip
- [x] Network failure — covered indirectly: sender error → 500 → Kratos's own `message_retries` handles retry, not application code

---

## Validation Commands

### Static Analysis
```bash
go vet ./...
golangci-lint run
```
EXPECT: Zero errors/warnings.

### Unit Tests
```bash
go test ./internal/api/... ./internal/config/... -v
```
EXPECT: All pass, including the new Courier* and config RequiredEnv tests.

### Full Test Suite
```bash
task test
```
EXPECT: No regressions elsewhere (e.g. `internal/app` tests still construct `RouterDeps` correctly with the new fields defaulting to zero values when kratos is disabled).

### E2E (Kratos spike)
```bash
task test:e2e
```
EXPECT: `TestPhoneCodeLogin_E2E` passes — proves the courier webhook, shared secret, and
admin-provisioned code login all work together against a real Kratos container.

### Local Dev Manual Validation
```bash
task kratos:up
task run
```
- [ ] `docker compose -f deploy/kratos/docker-compose.kratos.yml logs kratos` shows no courier
      config errors
- [ ] `curl -X POST http://localhost:4434/admin/identities -d '{"schema_id":"default","traits":{"phone":"+15550001234","full_name":"Test User"}}'` creates an identity
- [ ] Start a native login flow (`GET /self-service/login/api`) and submit `{"method":"code","identifier":"+15550001234", csrf/flow as required}` — watch `unwrap-gift`'s stdout (sms feature disabled locally) for the logged code (once Phase 3's console fallback exists)
- [ ] Submit `{"method":"code","code":"<logged code>"}` — receive a session for the identity created above

---

## Acceptance Criteria
- [ ] All tasks completed
- [ ] All validation commands pass
- [ ] Tests written and passing (unit + e2e)
- [ ] No type errors (`go vet`)
- [ ] No lint errors (`golangci-lint run`)
- [ ] Matches UX design (organiser-provision → code-login sequence works end to end against a real Kratos container)

## Completion Checklist
- [ ] Code follows discovered patterns (`Mount*`, `HandlerFunc`/`Adapter`, nil-is-disabled except where explicitly deviated and documented)
- [ ] Error handling matches codebase style (errors returned, mapped via `ErrorsMap`, never written directly)
- [ ] Logging follows codebase conventions (unmapped errors logged via `Adapter.logUnmapped`, no ad-hoc logging in handlers)
- [ ] Tests follow test patterns (fake + `httptest` for unit; `testcontainers-go` for e2e)
- [ ] No hardcoded values outside the documented dev-only placeholder secret
- [ ] `deploy/kratos/README.md` and the PRD updated
- [ ] No unnecessary scope additions (mailslurper/SMTP left alone per Task 2's GOTCHA; no self-service registration UI work; no Phase 3/5/6/7/8 work pulled forward beyond the specified `SMSSender` contract)
- [ ] Self-contained — Phase 3 must land first (Cross-Phase Contract), but no other external questions remain

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Admin-created identity can't complete code login without a prior registration/settings step (per Kratos docs' ambiguous wording) | M | H — blocks the entire auth model | Task 10's e2e spike tests this directly first; fallback is admin credentials-import API if needed |
| Self-hosted Kratos courier worker requires a valid `courier.smtp` config to start even with no email templates | L | M — would break `task kratos:up` entirely if removed prematurely | Not removed in this plan (Task 2 GOTCHA); confirmed empirically before any future removal |
| `host.docker.internal` unavailable in some local Docker setups (older Docker Engine, some Linux configs without `host-gateway` support) | L | M — breaks local dev courier delivery only, not production | `extra_hosts: host-gateway` (Task 8) covers the common case; documented as a known limitation in Task 11 |
| Kratos's reference self-service UI still technically allows registration via the code method, letting anyone with a phone number create an identity outside organiser control | M | L — creates an orphaned, harmless identity with no Drawer/Member link; not a data-exposure risk given the tiny real-world user base | Accepted for v1 per explicit scope decision (organiser-provisioning only); flagged here rather than silently ignored, revisit if it ever matters |
| Phase 3 lands with a different `SMSSender`-shaped method name/signature than specified | M | L | Only Task 7's three lines need updating; everything else in this plan is decoupled via the interface |
| seven.io's `To` field expects E.164 format but the schema's `phone` trait doesn't enforce it | M | M (deferred to Phase 5/6, where Member phone numbers are actually collected) | Noted in Task 1's GOTCHA; not this plan's problem to solve, but flagged so it isn't lost |

## Notes

- The PRD's Decisions Log already settled "code method, not TOTP" and "phone as identifier" —
  this plan only adds the concrete webhook/courier wiring and the identity-provisioning-model
  decision (organiser admin API, confirmed via this planning session's questions) on top of that.
- `internal/db/` is untouched by this plan — Phase 1's Drawer/Member schema is a separate,
  parallel concern; this plan only makes Kratos identities, not domain `Member` rows, and does
  not link the two (that's Phase 5/6's job: organiser's "add Member" endpoint will call both
  `db.Store`'s member-insert query and `kratosclient.CreateIdentity`).
