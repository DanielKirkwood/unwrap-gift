# Implementation Report: SMS client (seven.io)

## Summary

Added `internal/clients/smsclient`, a thin wrapper around seven.io's Go SDK
(`github.com/seven-io/go-client/sms77api`) exposing `Send(ctx, to, body) error`,
and registered a new `sms` feature (`RequiredEnv: ["SevenAPIKey", "SevenSenderID"]`)
in `internal/config/features.go`. `smsclient.Client` is deliberately never
`nil` — unlike `kratosclient`/`ketoclient`'s disabled-is-nil convention — so
`Send` always succeeds: when the feature is disabled it logs the message via
`slog` instead of calling seven.io. `a.SMS` is now wired into `App`/`Bootstrap`
exactly like `Kratos`/`Keto`, fulfilling the Cross-Phase Contract Phase 2's
plan (`secret-santa-sms-auth.plan.md`) already wrote against.

## Assessment vs Reality

| Metric | Predicted (Plan) | Actual |
|---|---|---|
| Complexity | Medium | Medium — matched exactly, no surprises |
| Confidence | High (per plan's detailed SDK research) | High — SDK source matched the plan's documented shapes verbatim |
| Files Changed | ~10 | 11 tracked modified + 1 new package dir (2 files) + `go.sum` |

## Tasks Completed

| # | Task | Status | Notes |
|---|---|---|---|
| 1 | Add seven.io SDK dependency | Complete | `go get github.com/seven-io/go-client/sms77api@latest`; verified downloaded source matches plan's documented API exactly |
| 2 | `SevenAPIKey`/`SevenSenderID` in `EnvVars` | Complete | |
| 3 | `SMSConfig` + `sms` feature | Complete | No `Requires` — independent of kratos/keto |
| 4 | `internal/clients/smsclient/smsclient.go` | Complete | `Client`, `Sender` interface seam, `New`, `Send`, `checkSmsResponse` |
| 5 | `internal/clients/smsclient/smsclient_test.go` | Complete | Deviated — added a `//nolint:revive,staticcheck` on `fakeSender.JsonContext` (see Deviations) |
| 6 | Wire `SMS` into `App`/`Bootstrap` | Complete | |
| 7 | `TestBootstrapSMSDisabled`/`Enabled` | Complete | |
| 8 | Env vars in `.env.example`, production env/compose | Complete | Verified via `docker compose config` that both vars resolve correctly on the `unwrap-gift` service only |
| 9 | Update the PRD | Already done | Phase 3's row was already `in-progress` with the plan link in the working tree at session start — no additional edit needed |

## Validation Results

| Level | Status | Notes |
|---|---|---|
| Static Analysis | Pass | `go vet ./...` clean; `golangci-lint run ./...` — 0 issues after two small fixes (see Deviations) |
| Unit Tests | Pass | 6 new `smsclient` tests, 2 new `registry_test.go` cases, 2 new `app_test.go` cases |
| Build | Pass | `go build ./...` clean throughout |
| Integration | N/A | No HTTP integration test possible — SDK's base URL/transport are unexported and hardcoded (documented GOTCHA); `docker compose config` used instead to validate the production wiring |
| Edge Cases | Pass | Disabled-logs-instead-of-sending, per-message rejection, transport error, sender-ID-required regression — all covered per Testing Strategy table |

## Files Changed

| File | Action | Lines |
|---|---|---|
| `go.mod` / `go.sum` | UPDATED | +3 |
| `internal/clients/smsclient/smsclient.go` | CREATED | +108 |
| `internal/clients/smsclient/smsclient_test.go` | CREATED | +150 |
| `internal/config/config.go` | UPDATED | +3 |
| `internal/config/features.go` | UPDATED | +18 |
| `internal/config/registry_test.go` | UPDATED | +14 |
| `internal/app/app.go` | UPDATED | +15/-5 |
| `internal/app/app_test.go` | UPDATED | +53 |
| `.env.example` | UPDATED | +5 |
| `deploy/production/.env.production.example` | UPDATED | +6 |
| `deploy/production/docker-compose.yml` | UPDATED | +4/-1 |
| `.claude/PRPs/prds/secret-santa-drawer.prd.md` | No change needed | Phase 3 row was already updated before this session |

## Deviations from Plan

1. **`fakeSender.JsonContext` naming lint** — the test file's fake `Sender`
   implementation triggered `revive` (var-naming) and `staticcheck` (ST1003)
   warnings because `JsonContext` isn't Go-initialism-cased (`JSONContext`).
   The method name must match `smsclient.Sender`'s interface method exactly,
   which itself mirrors the seven.io SDK's own (non-idiomatic) naming. Added
   `//nolint:revive,staticcheck` with an explanatory comment rather than
   renaming, since renaming would break the interface satisfaction the test
   depends on. Not called out in the plan's code block, but a mechanical
   consequence of matching the SDK's actual method name.
2. **`golines` formatting** — one `t.Errorf` call in the plan's test code
   exceeded the line-length limit; fixed via `golangci-lint fmt` (no content
   change, just reflowed onto multiple lines).

## Issues Encountered

None beyond the two lint deviations above. The seven.io SDK's live source
(fetched at `go get` time) matched the plan's documented API shapes exactly
— no drift from the plan's External Documentation research.

## Tests Written

| Test File | Tests | Coverage |
|---|---|---|
| `internal/clients/smsclient/smsclient_test.go` | 6 tests | `New` disabled/enabled, `Send` disabled-logs/success/rejected/transport-error |
| `internal/config/registry_test.go` | +2 table cases | `sms` feature enablement (both vars set, sender-ID-missing regression) |
| `internal/app/app_test.go` | 2 tests | `Bootstrap` wiring of `a.SMS` disabled/enabled |

## Next Steps
- [ ] Code review via `/code-review`
- [ ] Create PR via `/prp-pr`
