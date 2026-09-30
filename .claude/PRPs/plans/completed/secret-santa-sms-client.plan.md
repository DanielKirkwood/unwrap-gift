# Plan: SMS client (seven.io)

## Summary

Add `internal/clients/smsclient`, a thin wrapper around seven.io's Go SDK
(`github.com/seven-io/go-client/sms77api`) that exposes one method —
`Send(ctx, to, body) error` — and a new `sms` feature registered in
`internal/config/features.go`. Unlike every other client in this codebase
(`kratosclient`, `ketoclient`), a disabled `smsclient.Client` is never `nil`:
`Send` logs the message via `slog` instead of calling seven.io, so local dev
(where `SEVEN_API_KEY`/`SEVEN_SENDER_ID` are unset) never makes a real network
call to seven.io but always leaves a visible trace of what would have been
sent. This is exactly the shape Phase 2's plan (`secret-santa-sms-auth.plan.md`)
already wrote its Cross-Phase Contract against — its Kratos courier webhook
consumes `a.SMS` as an `api.SMSSender`, and this plan is what makes `a.SMS`
exist.

## User Story

As the app (on behalf of both Phase 2's Kratos courier webhook and Phase 7's
draw-notification pipeline), I want a single feature-flagged way to send an
SMS, so that both call sites share one seven.io integration, one disabled/
local-dev safety net, and one place that owns the seven.io SDK.

## Problem → Solution

**Current**: No SMS-sending capability exists anywhere in the repo.
`internal/clients` has `kratosclient`/`ketoclient`/`otelclient`/`logger`, all
following the "nil is disabled" convention — but that convention doesn't fit
an SMS sender, because a disabled client that silently drops every `Send`
call would make local dev (and Phase 2's login-code flow specifically)
untestable without a real seven.io account.

**Desired**: `internal/clients/smsclient.Client`, feature-flagged by a new
`sms` feature (`RequiredEnv: ["SevenAPIKey", "SevenSenderID"]`), wired into
`App` in `internal/app/app.go` exactly like `Kratos`/`Keto` are — except
`smsclient.New` always returns a non-nil `*Client`, and `Client.Send` branches
internally on whether the feature was enabled.

## Metadata

- **Complexity**: Medium
- **Source PRD**: `.claude/PRPs/prds/secret-santa-drawer.prd.md`
- **PRD Phase**: Phase 3 — SMS client (seven.io)
- **Estimated Files**: ~10 (`go.mod`/`go.sum`, 2 new `internal/clients/smsclient`
  files + 1 test file, `internal/config/config.go`, `internal/config/features.go`,
  `internal/config/registry_test.go`, `internal/app/app.go`, `internal/app/app_test.go`,
  `.env.example`, `deploy/production/.env.production.example`,
  `deploy/production/docker-compose.yml`, the PRD)

---

## Decisions Made This Session (don't re-derive these)

Two open questions were resolved with the user before writing this plan
rather than assumed:

1. **Sender ID is required, not optional.** `SEVEN_SENDER_ID` joins
   `SEVEN_API_KEY` in the `sms` feature's `RequiredEnv` — the feature (and
   therefore real SMS sending) stays disabled until both are set. This also
   resolves the PRD's own open question ("seven.io account/sender-ID setup
   ... not yet configured").
2. **Env vars are vendor-prefixed** (`SEVEN_API_KEY`, `SEVEN_SENDER_ID`), not
   capability-prefixed (`SMS_*`) — matching `KRATOS_*`/`KETO_*`'s convention
   of naming env vars after the actual external product. The **feature
   name** stays `sms` (capability-based, per the PRD's own Phase 3 wording
   and the `smsclient` package name) and the **config struct** stays
   `SMSConfig` (matching the feature name, like `KratosConfig`↔`"kratos"`) —
   only the env var/`EnvVars` field prefixes are vendor-named. This mixing is
   deliberate: `smsclient` stays a generic, vendor-swappable package name,
   while the concrete env vars stay self-documenting against seven.io's own
   docs.

---

## Cross-Phase Contract Fulfilled

Phase 2's plan (`.claude/PRPs/plans/secret-santa-sms-auth.plan.md`) specifies
exactly what this plan must produce:

```go
// internal/clients/smsclient
type Client struct { /* ... */ }
func New(cfg config.SMSConfig, enabled bool) (*Client, error)
func (c *Client) Send(ctx context.Context, to, body string) error
```

This plan's `Send` signature matches exactly — `*smsclient.Client` satisfies
Phase 2's `api.SMSSender` interface (`Send(ctx, to, body string) error`)
without any adapter. `New`'s signature gains a third parameter
(`logger *slog.Logger`, Task 4 below) to give `Send` somewhere to log to when
disabled — **this has zero impact on Phase 2's plan**: Phase 2 never calls
`smsclient.New` directly, only `internal/app/servers.go`'s
`deps.CourierSMS = a.SMS` (referencing the already-constructed field). The
`New` call itself lives in `internal/app/app.go`'s `Bootstrap`, which is
*this* plan's Task 6 — so the 3-arg signature is entirely this plan's own
internal decision.

---

## UX Design

Internal/infra change — no UI exists in this repo. Nothing in this plan is
user-visible; it's purely a new dependency other phases (2, 7) call into.

### Interaction Changes

| Touchpoint | Before | After | Notes |
|---|---|---|---|
| Outbound SMS | None exists | `smsclient.Client.Send(ctx, to, body)` | Called by Phase 2's courier webhook (login codes) and, later, Phase 7 (draw notifications) |
| Local dev / `sms` disabled | N/A | Every `Send` call is logged via `slog` at Info level (`to`, `body`) instead of hitting seven.io | Satisfies "local dev must never call seven.io" without a separate dry-run flag |

---

## Mandatory Reading

| Priority | File | Lines | Why |
|---|---|---|---|
| P0 | `ARCHITECTURE.md` | 1-96 | Nil-is-disabled convention (and this plan's deliberate, documented deviation from it), interface segregation, dependency injection via constructors |
| P0 | `internal/clients/kratosclient/kratosclient.go` | 1-65 | `New`'s disabled-is-nil shape, `//nolint:nilnil` comment style, doc-comment conventions — the pattern this plan mirrors except for the nil deviation |
| P0 | `internal/clients/kratosclient/kratosclient_test.go` | 1-46, 122-131 | `TestNewDisabled`/`TestNewEnabled` shape, and the `testClient` helper pattern of constructing a `*Client` directly via struct literal with exported fields pointed at a fake — this plan's test file mirrors the *direct-construction* half of this pattern (not the `httptest.Server` half — see this plan's GOTCHA on why) |
| P0 | `internal/config/features.go` | 1-102 | `KratosConfig`/`KetoConfig` shape, `Feature.RequiredEnv`/`Requires`, `DefaultRegistry`'s registration order — `SMSConfig` + the `sms` feature slot in here exactly |
| P0 | `internal/config/config.go` | 1-48 | `EnvVars` struct + `env`/`default` tags — reflection-based binding means adding fields here is sufficient |
| P0 | `internal/app/app.go` | 1-107 | `App` struct fields, `Bootstrap`'s per-feature `registry.Feature(name)` → `Build func(EnvVars) any` type-assert → `xclient.New(cfg, enabled)` sequence — `SMS` field + its `Bootstrap` wiring follow this exactly |
| P1 | `internal/config/registry_test.go` | 1-111 | Table-driven `TestResolveFeatureEnabledState` — new `sms` test cases extend this table |
| P1 | `internal/app/app_test.go` | 81-131 | `TestBootstrapKratosDisabled`/`Enabled` shape — this plan's `TestBootstrapSMSDisabled`/`Enabled` mirror it |
| P1 | `internal/clients/logger/logger.go` | 1-45 | `slog.Logger` construction (JSON prod / tint dev) — confirms `App.Logger` is a real `*slog.Logger` by the time `Bootstrap` can pass it into `smsclient.New` |
| P2 | `.env.example` | all | Existing per-feature env var grouping/commenting style this plan's new `sms` section follows |
| P2 | `deploy/production/docker-compose.yml` | 123-157 | `unwrap-gift` service's explicit `environment:` allowlist (no blanket `env_file:`) — the two new vars are added here, not to `kratos`'s block (seven.io is only ever called by `unwrap-gift`, never by Kratos directly — see Phase 2's plan, which routes Kratos's SMS codes through `unwrap-gift`'s own webhook, not seven.io directly) |

## External Documentation

context7 has no indexed documentation for `github.com/seven-io/go-client` (checked: resolving "seven.io go-client", "seven-io/go-client", and "sms77" all returned unrelated libraries). The SDK's actual source (`master` branch — `main` doesn't exist) was fetched and read directly to verify every detail below; nothing here is inferred from the PRD's citation, which itself was already flagged wrong by Phase 2's plan.

| Topic | Source | Key Takeaway |
|---|---|---|
| Correct import path | `github.com/seven-io/go-client/master/go.mod` | Module is `github.com/seven-io/go-client` (`go 1.17`); the client package is `github.com/seven-io/go-client/sms77api` (package name `sms77api`, not `seven`). |
| Constructor | `sms77api.go` | `func New(options Options) *Sms77API` — no error return. `Options{ApiKey, Debug, SentWith}`. `*Sms77API` embeds `Options` and exposes `Sms *SmsResource` among other resource fields (`Analytics`, `Balance`, `Contacts`, ...). |
| Sending one SMS | `sms.go` | `func (api *SmsResource) JsonContext(ctx context.Context, p SmsBaseParams) (o *SmsResponse, err error)` — the context-aware JSON-response variant. `SmsBaseParams{To, From, Text, ...}` (all the fields this plan needs: `To`, `From`, `Text`; everything else optional, zero value omitted via `omitempty`). |
| Response shape | `sms.go` | `SmsResponse{Debug, Balance, Messages []SmsResponseMessage, SmsType, Success StatusCode, TotalPrice}`. `SmsResponseMessage{Id *string, Success bool, Error *int64, ErrorText *string, Recipient string, ...}`. `StatusCode` is a `string` type; `StatusCodeSuccess = "100"`, `StatusCodeSuccessPartial = "101"` (for multi-recipient sends), plus various numeric error codes (`"201"` invalid sender, `"202"` invalid recipient, `"500"` insufficient credits, `"900"` auth error, etc). |
| **GOTCHA — no test-server injection point** | `sms77api.go` | `Sms77API.client *http.Client` is **unexported**, set once inside `New` to `http.DefaultClient`, with no setter/functional option. The request base URL is a **hardcoded literal** inside the SDK's internal `request` method: `fmt.Sprintf("https://gateway.seven.io/api/%s", endpoint)` — not a field, not overridable. Unlike `kratosclient`/`ketoclient`, **there is no way to point this SDK at a local `httptest.Server`.** This directly shapes Task 4/5 below (an internal `Sender` interface seam) and is why this plan's test file can't fully mirror `kratosclient_test.go`'s `httptest`-backed approach. |
| **GOTCHA — success detection is undocumented** | `sms.go` | Neither `SmsResponse` nor `SmsResponseMessage` has doc comments on any field (checked directly — none exist). There's no code-level guidance on whether to check top-level `Success StatusCode` or each `SmsResponseMessage.Success bool`. Inferred from field naming/types only (not confirmed against a live account): check each `Messages[i].Success`, since `Success`/`ErrorText` are directly on the per-recipient message; fall back to top-level `Success` only if `Messages` is empty. Flagged as a Risk below — worth a quick manual smoke test against a real seven.io account once credentials exist (ties into the PRD's own unresolved "seven.io account setup" item), not something this plan can fully close out. |
| **Prompt-injection note** | (research process, not the SDK) | While fetching seven.io's SDK source pages, one page's content contained an injected instruction claiming a fabricated "125-character quote limit" on quoted code. It was recognized as not being part of the actual page/doc content and disregarded; the code quotes above are complete and unabridged. Flagging this here per this codebase's own security posture, not because it affected any finding. |

---

## Patterns to Mirror

### DISABLED-IS-NEVER-NIL (deliberate deviation from "nil is disabled")
// SOURCE: `internal/clients/kratosclient/kratosclient.go:43-57` (the convention being deviated from)
```go
// New builds a Client from cfg... It returns (nil, nil) when enabled
// is false, mirroring db.New/otelclient.New's disabled-is-nil convention.
//
//nolint:nilnil // deliberate: nil is the "kratos feature disabled" state, not an error.
func New(cfg config.KratosConfig, enabled bool) (*Client, error) {
	if !enabled {
		return nil, nil
	}
	return &Client{ /* ... */ }, nil
}
```
`smsclient.New` returns a **non-nil** `*Client` in both branches — see Task 4. The doc comment on `smsclient.Client` must say so explicitly and explain why (a swallowed SMS — especially a Phase 2 login code — must never be silently possible), exactly mirroring how Phase 2's plan already documented this same deviation on its side of the contract.

### CLIENT-HOLDS-EXPORTED-SDK-HANDLES
// SOURCE: `internal/clients/kratosclient/kratosclient.go:34-41`
```go
type Client struct {
	// Public is the session-validation and self-service-flow client...
	Public *kratos.APIClient
	// Admin is the identity-CRUD client...
	Admin *kratos.APIClient
}
```
`kratosclient.Client` exposes its SDK handles as **exported** fields (not hidden behind accessors), which is what lets `kratosclient_test.go`'s `testClient` helper construct a working `*Client` directly via struct literal, bypassing `New`, for tests that need a fake backend. `smsclient.Client` follows the same exported-fields shape (`Sender`, `Enabled`, `From`, `Logger` — see Task 4) for the same reason, even though (per the GOTCHA above) the thing being substituted is a package-local interface seam instead of the SDK's own client type.

### FEATURE_CONFIG_AND_REGISTRATION
// SOURCE: `internal/config/features.go:34-40, 84-90`
```go
type KratosConfig struct {
	PublicURL string
	AdminURL  string
}

r.Register(Feature{
	Name:        "kratos",
	RequiredEnv: []string{"KratosPublicURL", "KratosAdminURL"},
	Build: func(e EnvVars) any {
		return KratosConfig{PublicURL: e.KratosPublicURL, AdminURL: e.KratosAdminURL}
	},
})
```
New: `SMSConfig{APIKey, SenderID string}` + a `"sms"` `Feature` with
`RequiredEnv: []string{"SevenAPIKey", "SevenSenderID"}`, no `Requires` (independent of kratos/keto, matching the PRD's "Phases 1, 2, and 3 ... can be built concurrently" note).

### BOOTSTRAP_WIRING
// SOURCE: `internal/app/app.go:83-89`
```go
kratosFeature, _ := registry.Feature("kratos")
kratosCfg, _ := kratosFeature.Config.(config.KratosConfig)

kratos, err := kratosclient.New(kratosCfg, kratosFeature.Enabled)
if err != nil {
	return nil, fmt.Errorf("app: bootstrap kratos: %w", err)
}
```
New: the identical three-line shape for `sms`/`smsclient`, placed after the existing `keto` block, calling `smsclient.New(smsCfg, smsFeature.Enabled, log)` — note `log` (already constructed a few lines above in `Bootstrap`) is the new third argument.

### TABLE-DRIVEN FEATURE-ENABLEMENT TEST
// SOURCE: `internal/config/registry_test.go:43-61`
```go
{
	name: "keto enabled once both its own env and kratos are satisfied",
	env: config.EnvVars{
		KratosPublicURL: "http://kratos.public", KratosAdminURL: "http://kratos.admin",
		KetoReadURL: "http://keto.read", KetoWriteURL: "http://keto.write",
	},
	wantEnabled: map[string]bool{"kratos": true, "keto": true},
},
{
	name: "keto disabled when its own env is missing, even with kratos enabled",
	env: config.EnvVars{KratosPublicURL: "http://kratos.public", KratosAdminURL: "http://kratos.admin"},
	wantEnabled: map[string]bool{"kratos": true, "keto": false},
	wantReason:  map[string]string{"keto": "missing required env: KetoReadURL"},
},
```
New cases for `sms`: both vars set → enabled; only one set → disabled with the specific `"missing required env: Seven...`" reason — proving `SEVEN_SENDER_ID` is genuinely required, not just documented as such (this is the regression test for the "Sender ID is required" decision above).

---

## Files to Change

| File | Action | Justification |
|---|---|---|
| `go.mod` / `go.sum` | UPDATE | Add `github.com/seven-io/go-client` (via `go get .../sms77api`) |
| `internal/clients/smsclient/smsclient.go` | CREATE | `Client`, `Sender` interface, `New`, `Send` |
| `internal/clients/smsclient/smsclient_test.go` | CREATE | Unit tests: disabled-logs-instead-of-sending, enabled-success, enabled-per-message-failure, enabled-transport-error |
| `internal/config/config.go` | UPDATE | Add `SevenAPIKey`, `SevenSenderID` fields to `EnvVars` |
| `internal/config/features.go` | UPDATE | Add `SMSConfig` type + register the `"sms"` `Feature` |
| `internal/config/registry_test.go` | UPDATE | Add `sms` enablement test cases to `TestResolveFeatureEnabledState`'s table |
| `internal/app/app.go` | UPDATE | Add `SMS *smsclient.Client` field to `App`; wire `smsclient.New` into `Bootstrap` |
| `internal/app/app_test.go` | UPDATE | Add `TestBootstrapSMSDisabled`/`TestBootstrapSMSEnabled` |
| `.env.example` | UPDATE | Add `SEVEN_API_KEY=`, `SEVEN_SENDER_ID=` |
| `deploy/production/.env.production.example` | UPDATE | Same two vars |
| `deploy/production/docker-compose.yml` | UPDATE | Pass both vars to the `unwrap-gift` service's `environment:` block |
| `.claude/PRPs/prds/secret-santa-drawer.prd.md` | UPDATE | Phase 3 row: status → in-progress, PRP Plan link added |

## NOT Building

- Any caller of `Send` — Phase 2's courier webhook (`internal/api/courier.go`)
  and Phase 7's notification pipeline both consume this client but are out of
  scope here; this plan only makes `a.SMS` exist and work.
- `internal/api`-side wiring (`RouterDeps.CourierSMS`, `MountCourierWebhook`,
  etc.) — that's Phase 2's Task 7, which only needs `a.SMS` to exist.
- Retry/backoff around seven.io calls — Phase 2's plan already established
  that a failed courier webhook delivery is retried by Kratos's own courier
  worker (`message_retries`), not by application code; the same reasoning
  applies to Phase 7. `Send` returns a plain wrapped error and nothing more.
- Delivery-status polling/webhooks from seven.io (their `Hooks`/`Status`
  resources) — not in the PRD's MVP scope (SMS delivery success is
  "manual spot-check" per the PRD's Success Metrics table).
- Rate limiting, message templating, or multi-recipient batch sending —
  `Send` is single-recipient, single-message, by design (matches the
  `api.SMSSender` contract Phase 2 already committed to).
- seven.io's own `Options.Debug` simulate-mode flag — deliberately unused;
  this plan's own enabled/disabled gate is a stronger guarantee (zero
  network calls to seven.io at all when disabled, vs. a live HTTP call that
  merely doesn't charge/deliver).

---

## Step-by-Step Tasks

### Task 1: Add the seven.io SDK dependency
- **ACTION**: Add `github.com/seven-io/go-client` to `go.mod`/`go.sum`.
- **IMPLEMENT**:
  ```sh
  go get github.com/seven-io/go-client/sms77api@latest
  ```
- **MIRROR**: N/A — new dependency, no existing pattern to follow beyond `go.mod`'s existing `require` block shape.
- **IMPORTS**: N/A.
- **GOTCHA**: The module's own `go.mod` declares `go 1.17`, well below this repo's `go 1.27.1` — no compatibility concern, just note it in case `go mod tidy` complains about anything unrelated in the dependency graph.
- **VALIDATE**: `go build ./...` (will fail until Task 4 actually imports the package — acceptable to leave this as a no-op commit-of-intent until Task 4, or combine the `go get` into Task 4's commit).

### Task 2: Add `SevenAPIKey`/`SevenSenderID` to `EnvVars`
- **ACTION**: Extend `internal/config/config.go`.
- **IMPLEMENT**:
  ```go
  // internal/config/config.go, in EnvVars, after KetoWriteURL
  SevenAPIKey   string `env:"SEVEN_API_KEY"`
  SevenSenderID string `env:"SEVEN_SENDER_ID"`
  ```
- **MIRROR**: `internal/config/config.go:30-34` (`KratosPublicURL`/`KetoReadURL` etc. — plain `env` tag, no `default`, since these are secrets/account-specific with no sane default).
- **IMPORTS**: None new.
- **GOTCHA**: None — reflection-based `Load()` picks this up automatically, per `ARCHITECTURE.md`'s "adding a field here is sufficient" note.
- **VALIDATE**: `go test ./internal/config/... -run TestLoad`.

### Task 3: `SMSConfig` + the `sms` feature
- **ACTION**: Extend `internal/config/features.go`.
- **IMPLEMENT**:
  ```go
  // SMSConfig configures the seven.io client smsclient.New builds. It's
  // only built once the "sms" feature is enabled, i.e. once both
  // SevenAPIKey and SevenSenderID are set. Unlike KratosConfig/KetoConfig,
  // a disabled "sms" feature does not mean smsclient.Client is nil — see
  // smsclient.New's doc comment.
  type SMSConfig struct {
  	APIKey   string
  	SenderID string
  }
  ```
  ```go
  // internal/config/features.go, DefaultRegistry, after the "keto" r.Register block
  r.Register(Feature{
  	Name:        "sms",
  	RequiredEnv: []string{"SevenAPIKey", "SevenSenderID"},
  	Build: func(e EnvVars) any {
  		return SMSConfig{APIKey: e.SevenAPIKey, SenderID: e.SevenSenderID}
  	},
  })
  ```
- **MIRROR**: `internal/config/features.go:34-40` (`KratosConfig` doc-comment style), `:84-90` (`kratos` `Feature` registration).
- **IMPORTS**: None new.
- **GOTCHA**: No `Requires` — `sms` is independent of `kratos`/`keto`/`database`, matching the PRD's explicit "Phases 1, 2, and 3 ... can be built concurrently" note. Don't add a dependency that doesn't exist yet.
- **VALIDATE**: `go build ./...`.

### Task 4: `internal/clients/smsclient/smsclient.go`
- **ACTION**: Create the package.
- **IMPLEMENT**:
  ```go
  // Package smsclient wraps seven.io's Go SDK (sms77api) behind the
  // Send(ctx, to, body) method internal/api's SMSSender interface expects
  // (see Phase 2's plan, .claude/PRPs/plans/secret-santa-sms-auth.plan.md) —
  // api never imports this package directly, per ARCHITECTURE.md's
  // dependency-direction rule.
  package smsclient

  import (
  	"context"
  	"fmt"
  	"log/slog"

  	"github.com/seven-io/go-client/sms77api"

  	"github.com/DanielKirkwood/unwrap-gift/internal/config"
  )

  // Sender is the narrow seam smsclient.Client sends through, satisfied in
  // production by *sms77api.SmsResource (via New) and by a fake in tests.
  // It exists because sms77api's HTTP transport and base URL
  // ("https://gateway.seven.io/api/...") are both unexported and hardcoded
  // inside the SDK — unlike kratosclient/ketoclient, there is no way to
  // point the real SDK at a local httptest.Server, so this interface is
  // the substitution point instead. Not to be confused with
  // internal/api.SMSSender (Phase 2), a differently-shaped interface one
  // layer up that *Client itself implements.
  type Sender interface {
  	JsonContext(ctx context.Context, p sms77api.SmsBaseParams) (*sms77api.SmsResponse, error)
  }

  // Client sends SMS messages via seven.io, or — when the sms feature is
  // disabled — logs the message instead of sending it. Unlike
  // kratosclient/ketoclient's "nil is disabled" convention, Client is
  // NEVER nil: Phase 2's Kratos courier webhook and Phase 7's draw
  // notification pipeline both call Send unconditionally, and a swallowed
  // login code or draw notification would make local dev untestable. This
  // is a deliberate, documented deviation from ARCHITECTURE.md's default
  // convention — see New.
  type Client struct {
  	// Sender is the seven.io SMS-sending seam Send calls through when
  	// Enabled is true. Exported so tests can substitute a fake directly,
  	// mirroring kratosclient.Client's exported Public/Admin fields.
  	Sender Sender
  	// Enabled reports whether Send calls seven.io (true) or logs the
  	// message instead (false).
  	Enabled bool
  	// From is the configured SEVEN_SENDER_ID, sent as every message's
  	// From field.
  	From string
  	// Logger receives the "sms disabled" log line when Enabled is false.
  	// Must be non-nil whenever Client is constructed via New.
  	Logger *slog.Logger
  }

  // New builds a Client from cfg. When enabled is false, cfg is ignored and
  // the returned Client is still non-nil — Send logs instead of sending.
  // logger is required in both branches (see Client.Logger).
  func New(cfg config.SMSConfig, enabled bool, logger *slog.Logger) (*Client, error) {
  	if !enabled {
  		return &Client{Enabled: false, Logger: logger}, nil
  	}

  	api := sms77api.New(sms77api.Options{ApiKey: cfg.APIKey})

  	return &Client{Sender: api.Sms, Enabled: true, From: cfg.SenderID, Logger: logger}, nil
  }

  // Send implements api.SMSSender (internal/api/courier.go, Phase 2; and
  // later Phase 7's notification pipeline) by sending body to the to phone
  // number. When Enabled is false, it logs the message via slog at Info
  // level instead of calling seven.io — no network call happens at all in
  // that branch, which is what keeps local dev (and CI) from ever sending
  // a real SMS.
  func (c *Client) Send(ctx context.Context, to, body string) error {
  	if !c.Enabled {
  		c.Logger.InfoContext(ctx, "sms disabled: logging instead of sending", "to", to, "body", body)
  		return nil
  	}

  	resp, err := c.Sender.JsonContext(ctx, sms77api.SmsBaseParams{To: to, From: c.From, Text: body})
  	if err != nil {
  		return fmt.Errorf("smsclient: send: %w", err)
  	}

  	return checkSmsResponse(resp)
  }

  // checkSmsResponse inspects each per-recipient result for failure,
  // falling back to the top-level status when seven.io returns no
  // per-message detail at all. See this plan's GOTCHA on seven.io's
  // undocumented success-detection shape — this has not been confirmed
  // against a live account.
  func checkSmsResponse(resp *sms77api.SmsResponse) error {
  	if len(resp.Messages) == 0 {
  		if resp.Success != sms77api.StatusCodeSuccess && resp.Success != sms77api.StatusCodeSuccessPartial {
  			return fmt.Errorf("smsclient: send: seven.io returned status %q with no per-message detail", resp.Success)
  		}
  		return nil
  	}

  	for _, m := range resp.Messages {
  		if !m.Success {
  			detail := "unknown error"
  			if m.ErrorText != nil {
  				detail = *m.ErrorText
  			}
  			return fmt.Errorf("smsclient: send: seven.io rejected message to %s: %s", m.Recipient, detail)
  		}
  	}

  	return nil
  }
  ```
- **MIRROR**: `internal/clients/kratosclient/kratosclient.go`'s package doc-comment style, `New`'s signature shape (minus the nil-disabled branch), exported-field-holds-SDK-handle pattern.
- **IMPORTS**: `context`, `fmt`, `log/slog`, `github.com/seven-io/go-client/sms77api`, `github.com/DanielKirkwood/unwrap-gift/internal/config`.
- **GOTCHA**: `sms77api.New` makes no network call (it just builds local struct/resource values), so calling it unconditionally inside `New` whenever `enabled` is true is safe and matches `kratosclient.New`/`ketoclient.New`'s pattern of building the SDK client eagerly.
- **GOTCHA**: `checkSmsResponse`'s per-message-vs-top-level fallback is inferred, not SDK-documented (see External Documentation) — flagged as a Risk below, not silently assumed correct.
- **VALIDATE**: `go build ./...`; `go vet ./...`.

### Task 5: `internal/clients/smsclient/smsclient_test.go`
- **ACTION**: Create unit tests using a hand-rolled `fakeSender` (no real network, no `httptest.Server` — see Task 4's GOTCHA on why the SDK can't be pointed at one).
- **IMPLEMENT**:
  ```go
  package smsclient_test

  import (
  	"bytes"
  	"context"
  	"errors"
  	"log/slog"
  	"strings"
  	"testing"

  	"github.com/seven-io/go-client/sms77api"

  	"github.com/DanielKirkwood/unwrap-gift/internal/clients/smsclient"
  	"github.com/DanielKirkwood/unwrap-gift/internal/config"
  )

  type fakeSender struct {
  	resp   *sms77api.SmsResponse
  	err    error
  	called bool
  	gotTo, gotFrom, gotText string
  }

  func (f *fakeSender) JsonContext(_ context.Context, p sms77api.SmsBaseParams) (*sms77api.SmsResponse, error) {
  	f.called = true
  	f.gotTo, f.gotFrom, f.gotText = p.To, p.From, p.Text
  	return f.resp, f.err
  }

  func testLogger(buf *bytes.Buffer) *slog.Logger {
  	return slog.New(slog.NewTextHandler(buf, nil))
  }

  func TestNewDisabled(t *testing.T) {
  	t.Parallel()

  	client, err := smsclient.New(config.SMSConfig{}, false, slog.Default())
  	if err != nil {
  		t.Fatalf("New() error = %v, want nil", err)
  	}
  	if client == nil {
  		t.Fatal("New() = nil, want non-nil (sms client is never nil — see Client's doc comment)")
  	}
  	if client.Enabled {
  		t.Error("Enabled = true, want false")
  	}
  }

  func TestNewEnabled(t *testing.T) {
  	t.Parallel()

  	cfg := config.SMSConfig{APIKey: "test-key", SenderID: "TestSender"}

  	client, err := smsclient.New(cfg, true, slog.Default())
  	if err != nil {
  		t.Fatalf("New() error = %v, want nil", err)
  	}
  	if !client.Enabled {
  		t.Error("Enabled = false, want true")
  	}
  	if client.Sender == nil {
  		t.Error("Sender = nil, want non-nil")
  	}
  	if client.From != "TestSender" {
  		t.Errorf("From = %q, want %q", client.From, "TestSender")
  	}
  }

  func TestClient_Send_Disabled_LogsInsteadOfSending(t *testing.T) {
  	t.Parallel()

  	var buf bytes.Buffer
  	client := &smsclient.Client{Enabled: false, Logger: testLogger(&buf)}

  	if err := client.Send(t.Context(), "+15550001234", "your code is 123456"); err != nil {
  		t.Fatalf("Send() error = %v, want nil", err)
  	}

  	got := buf.String()
  	if !strings.Contains(got, "+15550001234") || !strings.Contains(got, "your code is 123456") {
  		t.Errorf("log output = %q, want it to contain the to/body", got)
  	}
  	// client.Sender is nil here; if Send incorrectly fell through to call
  	// it, this test would panic rather than reach this assertion.
  }

  func TestClient_Send_Success(t *testing.T) {
  	t.Parallel()

  	fake := &fakeSender{resp: &sms77api.SmsResponse{
  		Success:  sms77api.StatusCodeSuccess,
  		Messages: []sms77api.SmsResponseMessage{{Recipient: "+15550001234", Success: true}},
  	}}
  	client := &smsclient.Client{Sender: fake, Enabled: true, From: "TestSender", Logger: slog.Default()}

  	if err := client.Send(t.Context(), "+15550001234", "hello"); err != nil {
  		t.Fatalf("Send() error = %v, want nil", err)
  	}
  	if !fake.called {
  		t.Error("fakeSender was not called")
  	}
  	if fake.gotTo != "+15550001234" || fake.gotFrom != "TestSender" || fake.gotText != "hello" {
  		t.Errorf("JsonContext params = (%q, %q, %q), want (+15550001234, TestSender, hello)", fake.gotTo, fake.gotFrom, fake.gotText)
  	}
  }

  func TestClient_Send_MessageRejected(t *testing.T) {
  	t.Parallel()

  	errText := "invalid recipient"
  	fake := &fakeSender{resp: &sms77api.SmsResponse{
  		Success: sms77api.StatusCodeErrorUnknown,
  		Messages: []sms77api.SmsResponseMessage{
  			{Recipient: "+15550001234", Success: false, ErrorText: &errText},
  		},
  	}}
  	client := &smsclient.Client{Sender: fake, Enabled: true, Logger: slog.Default()}

  	err := client.Send(t.Context(), "+15550001234", "hello")
  	if err == nil || !strings.Contains(err.Error(), errText) {
  		t.Errorf("Send() error = %v, want it to contain %q", err, errText)
  	}
  }

  func TestClient_Send_TransportError(t *testing.T) {
  	t.Parallel()

  	boom := errors.New("boom")
  	fake := &fakeSender{err: boom}
  	client := &smsclient.Client{Sender: fake, Enabled: true, Logger: slog.Default()}

  	err := client.Send(t.Context(), "+15550001234", "hello")
  	if !errors.Is(err, boom) {
  		t.Errorf("Send() error = %v, want errors.Is(err, boom)", err)
  	}
  }
  ```
- **MIRROR**: `internal/clients/kratosclient/kratosclient_test.go:16-46` (`TestNewDisabled`/`TestNewEnabled` shape), `:122-131` (`testClient`-style direct struct-literal construction, adapted here for a fake `Sender` instead of a fake `httptest.Server`).
- **IMPORTS**: `bytes`, `context`, `errors`, `log/slog`, `strings`, `testing`, `github.com/seven-io/go-client/sms77api`, `github.com/DanielKirkwood/unwrap-gift/internal/clients/smsclient`, `github.com/DanielKirkwood/unwrap-gift/internal/config`.
- **GOTCHA**: This is the only test file in the repo constructing its subject-under-test's dependency via a hand-rolled interface fake rather than an `httptest.Server` — that's a direct consequence of Task 4's GOTCHA (the SDK's base URL/transport aren't overridable), not a stylistic choice; call this out in review if questioned.
- **VALIDATE**: `go test ./internal/clients/smsclient/... -v`.

### Task 6: Wire `SMS` into `App`/`Bootstrap`
- **ACTION**: Extend `internal/app/app.go`.
- **IMPLEMENT**:
  ```go
  // internal/app/app.go, App struct, after Keto
  SMS *smsclient.Client
  ```
  ```go
  // internal/app/app.go, Bootstrap, after the ketoFeature/keto block, before the log.DebugContext call
  smsFeature, _ := registry.Feature("sms")
  smsCfg, _ := smsFeature.Config.(config.SMSConfig)

  sms, err := smsclient.New(smsCfg, smsFeature.Enabled, log)
  if err != nil {
  	return nil, fmt.Errorf("app: bootstrap sms: %w", err)
  }
  ```
  Add `sms` to the returned `&App{...}` literal, and extend the existing
  `log.DebugContext(ctx, "bootstrap complete", ...)` call with
  `"sms_enabled", smsFeature.Enabled`.
  Add the import `"github.com/DanielKirkwood/unwrap-gift/internal/clients/smsclient"`.
- **MIRROR**: `internal/app/app.go:83-89` (the `kratos`/`keto` block being extended) — exact same three-line `Feature` → type-assert → `xclient.New` → error-wrap shape.
- **IMPORTS**: `github.com/DanielKirkwood/unwrap-gift/internal/clients/smsclient` (new).
- **GOTCHA**: `smsclient.New` is called **unconditionally** (not inside an `if smsFeature.Enabled` guard) — exactly like `kratosclient.New`/`ketoclient.New` already are. The enabled/disabled branching happens inside `smsclient.New` itself, not in `Bootstrap`.
- **GOTCHA**: `App.Shutdown` needs no changes — `smsclient.Client` holds no persistent connection/goroutine to close (unlike `Store`/`Otel`).
- **VALIDATE**: `go build ./...`; `go vet ./...`.

### Task 7: `internal/app/app_test.go`
- **ACTION**: Add `TestBootstrapSMSDisabled`/`TestBootstrapSMSEnabled`.
- **IMPLEMENT**:
  ```go
  func TestBootstrapSMSDisabled(t *testing.T) {
  	t.Parallel()

  	env := config.EnvVars{Env: "development", LogLevel: "debug"}

  	a, err := app.Bootstrap(t.Context(), env)
  	if err != nil {
  		t.Fatalf("Bootstrap() error = %v, want nil", err)
  	}

  	smsFeature, ok := a.Registry.Feature("sms")
  	if !ok {
  		t.Fatal("sms feature not registered")
  	}
  	if smsFeature.Enabled {
  		t.Error("sms feature Enabled = true, want false (no SEVEN_* env set)")
  	}
  	if a.SMS == nil {
  		t.Fatal("SMS = nil, want non-nil (sms client is never nil)")
  	}
  	if a.SMS.Enabled {
  		t.Error("SMS.Enabled = true, want false")
  	}
  }

  func TestBootstrapSMSEnabled(t *testing.T) {
  	t.Parallel()

  	env := config.EnvVars{
  		Env: "production", LogLevel: "info",
  		SevenAPIKey: "test-key", SevenSenderID: "TestSender",
  	}

  	a, err := app.Bootstrap(t.Context(), env)
  	if err != nil {
  		t.Fatalf("Bootstrap() error = %v, want nil", err)
  	}

  	smsFeature, ok := a.Registry.Feature("sms")
  	if !ok {
  		t.Fatal("sms feature not registered")
  	}
  	if !smsFeature.Enabled {
  		t.Error("sms feature Enabled = false, want true (SEVEN_* env set)")
  	}
  	if a.SMS == nil || !a.SMS.Enabled {
  		t.Fatal("SMS.Enabled = false or SMS nil, want enabled non-nil client")
  	}
  	if a.SMS.Sender == nil {
  		t.Error("SMS.Sender = nil, want non-nil")
  	}
  }
  ```
- **MIRROR**: `internal/app/app_test.go:81-131` (`TestBootstrapKratosDisabled`/`Enabled`) verbatim shape.
- **IMPORTS**: None new (file already imports `app`, `config`).
- **GOTCHA**: None.
- **VALIDATE**: `go test ./internal/app/... -v -run SMS`.

### Task 8: Add env vars to `.env.example`, production env/compose
- **ACTION**: Propagate `SEVEN_API_KEY`/`SEVEN_SENDER_ID` through every env template and the production compose file.
- **IMPLEMENT**:
  ```sh
  # .env.example, new section after the keto block, before KETO_SEED_ADMIN_IDENTITY_ID
  # sms (seven.io) — required for real SMS sending; leave both empty to log
  # instead of sending (see internal/clients/smsclient).
  SEVEN_API_KEY=
  SEVEN_SENDER_ID=
  ```
  ```sh
  # deploy/production/.env.production.example, near SMTP_CONNECTION_URI
  # seven.io — used both for Phase 7's draw-notification SMS and (via
  # unwrap-gift's own courier webhook, not Kratos directly) Phase 2's SMS
  # login codes. Leave both empty to log instead of sending.
  SEVEN_API_KEY=
  SEVEN_SENDER_ID=
  ```
  ```yaml
  # deploy/production/docker-compose.yml, unwrap-gift service's environment:,
  # after KETO_WRITE_URL
  SEVEN_API_KEY: ${SEVEN_API_KEY}
  SEVEN_SENDER_ID: ${SEVEN_SENDER_ID}
  ```
- **MIRROR**: `.env.example`'s existing per-feature section/comment style; `deploy/production/docker-compose.yml:142-153`'s explicit-allowlist `environment:` block (comment already there explains why it's not a blanket `env_file:`).
- **IMPORTS**: N/A.
- **GOTCHA**: These two vars go on the `unwrap-gift` service only, **not** `kratos`'s service block — Kratos never talks to seven.io directly (Phase 2 routes Kratos's courier through `unwrap-gift`'s own webhook, authenticated by a separate shared secret, `KRATOS_COURIER_WEBHOOK_SECRET`). Don't conflate the two.
- **VALIDATE**: `docker compose -f deploy/production/docker-compose.yml --env-file deploy/production/.env.production.example config` (with a filled-in copy) resolves the `unwrap-gift` service's env correctly.

### Task 9: Update the PRD
- **ACTION**: `.claude/PRPs/prds/secret-santa-drawer.prd.md`, Phase 3's row in the Implementation Phases table.
- **IMPLEMENT**: Set `Status` → `in-progress`; add this plan's path to the `PRP Plan` column alongside the existing issue link (`[#4](https://github.com/DanielKirkwood/unwrap-gift/issues/4) · [plan](../plans/secret-santa-sms-client.plan.md)`).
- **VALIDATE**: Diff review only.

---

## Testing Strategy

### Unit Tests

| Test | Input | Expected Output | Edge Case? |
|---|---|---|---|
| `TestNewDisabled` | `enabled=false` | Non-nil `*Client`, `Enabled=false` | Yes — the core "never nil" deviation |
| `TestNewEnabled` | valid `SMSConfig`, `enabled=true` | `Enabled=true`, `Sender != nil`, `From` set | |
| `TestClient_Send_Disabled_LogsInsteadOfSending` | disabled client, `Send(ctx, to, body)` | `nil` error, log output contains `to`/`body`, `Sender` never invoked | Yes — the explicit "local dev never calls seven.io" requirement |
| `TestClient_Send_Success` | fake sender returns success | `nil` error, fake recorded exact `to`/`from`/`text` | |
| `TestClient_Send_MessageRejected` | fake sender returns `Messages[0].Success=false` | error containing the rejection detail | Yes |
| `TestClient_Send_TransportError` | fake sender returns a transport error | wrapped error, `errors.Is` matches | Yes |
| `internal/config` — sms feature enablement | only one of `SevenAPIKey`/`SevenSenderID` set | `sms` feature `Enabled=false`, `Reason` names the missing var | Yes — regression for "sender ID is required" |
| `internal/app` — `TestBootstrapSMSDisabled`/`Enabled` | env with/without `SEVEN_*` | `a.SMS` non-nil in both cases; `Enabled`/`Sender` differ | |

### Edge Cases Checklist
- [x] Empty input (disabled client's `Send` with empty `to`/`body`) — covered structurally by the disabled-path test; not worth a separate case
- [x] Invalid types — N/A, `Send`'s params are plain strings
- [ ] Maximum size input — not meaningful (SMS bodies are short by nature); skip
- [x] Permission denied / auth failure — covered indirectly by `TestClient_Send_TransportError` and `MessageRejected` (seven.io auth errors surface as either a transport error or a rejected-message error depending on how the SDK models them — see the Risk below)
- [ ] Concurrent access — `Client` holds no mutable state after construction; skip
- [x] Network failure — `TestClient_Send_TransportError`

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
go test ./internal/clients/smsclient/... ./internal/config/... ./internal/app/... -v
```
EXPECT: All pass, including the new `smsclient`, `sms`-feature, and `TestBootstrapSMS*` tests.

### Full Test Suite
```bash
task test
```
EXPECT: No regressions elsewhere.

### Local Dev Manual Validation
```bash
task run
```
- [ ] With `SEVEN_API_KEY`/`SEVEN_SENDER_ID` unset: confirm `sms` feature shows disabled (once an `info features`-style check exists, or via a temporary log line) and that `a.SMS.Enabled == false`.
- [ ] (Once real seven.io credentials exist — tied to the PRD's own open question) set both vars and manually call `Client.Send` via a short `go run` snippet or a temporary test, confirming a real SMS arrives and `checkSmsResponse`'s success detection (Task 4's GOTCHA) is actually correct against the live API.

---

## Acceptance Criteria
- [ ] All tasks completed
- [ ] All validation commands pass
- [ ] Tests written and passing
- [ ] No type errors (`go vet`)
- [ ] No lint errors (`golangci-lint run`)
- [ ] `*smsclient.Client` satisfies Phase 2's `api.SMSSender` interface without an adapter (confirmed once Phase 2's `internal/api/courier.go` exists — `go build ./...` across both plans together is the real check)

## Completion Checklist
- [ ] Code follows discovered patterns (`New(cfg, enabled) (*T, error)` shape, exported SDK-handle fields, `Feature`/`RequiredEnv` registration)
- [ ] The one deliberate deviation (never-nil `Client`) is documented in the doc comment, not just in this plan
- [ ] Error handling matches codebase style (`fmt.Errorf` with `%w`, package-prefixed messages: `"smsclient: ..."`)
- [ ] Tests follow test patterns as closely as the SDK's lack of a test-server injection point allows
- [ ] `.env.example`, `deploy/production/.env.production.example`, `deploy/production/docker-compose.yml`, and the PRD all updated
- [ ] No unnecessary scope additions (no `internal/api` changes, no Phase 7 notification-composition logic, no delivery-status polling)
- [ ] Self-contained — no other phase must land first for this plan to be implementable and testable end-to-end (unlike Phase 2, which explicitly depends on this plan)

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `checkSmsResponse`'s success-detection logic (per-message `Success bool`, falling back to top-level `StatusCode`) is inferred from field names/types, not SDK documentation | M | M — could misclassify a real failure as success, or vice versa, once a real seven.io account is wired up | Flagged explicitly in code and this plan; the PRD's own open question about seven.io account setup is the natural point to smoke-test this against a live account before Phase 7 depends on it in production |
| seven.io SDK's hardcoded base URL/unexported `*http.Client` means unit tests can't exercise an actual HTTP round-trip, only the `Sender` interface seam | H (certain) | L — the seam itself is a well-established Go idiom and the SDK's request-building logic (URL, headers, JSON encoding) is out of this plan's control anyway | Documented as a GOTCHA; the manual validation step against a real account (once credentials exist) is the real integration check |
| `SEVEN_SENDER_ID` being required could surprise a future contributor expecting only an API key (most SMS providers treat sender ID as optional/account-default) | L | L | Documented in `.env.example`'s comment and this plan's "Decisions Made This Session" section |
| Adding a third parameter to `New` (vs. Phase 2's illustrative 2-arg signature) could be mistaken for a contract break | L | L | Explicitly addressed in this plan's "Cross-Phase Contract Fulfilled" section — `New` is never called from Phase 2's code, only from this plan's own `Bootstrap` wiring |

## Notes

- This plan deliberately does not touch `internal/api` or `internal/app/servers.go` — those are Phase 2's Task 7 (wiring `a.SMS` into `RouterDeps.CourierSMS`) and Phase 7 (the notification pipeline itself). This plan's only job is making `a.SMS` exist, work, and be safe to call unconditionally.
- The "Sender ID required" and "vendor-prefixed env vars" decisions were made with the user directly (via `AskUserQuestion`) rather than assumed, per this session's explicit instruction — see "Decisions Made This Session" above.
- seven.io's Go SDK has no context7 coverage; every SDK detail in this plan was verified by fetching and reading the actual source from `github.com/seven-io/go-client` (`master` branch) rather than trusted from the PRD's citation, which Phase 2's plan had already flagged as wrong once before.
