# unwrap-gift — Codebase Review (2026-10-02)

Multi-agent deep review covering Go architecture/code quality, security (auth/authz/web),
CI/CD & deployment, database schema/migrations, local dev experience, and product
completeness vs. the Secret Santa drawer PRD. Organized by urgency given the real
deadline (two live Christmas draws this year).

---

## Fix before you run the real draws

These directly threaten the product's core promise (0 missed notifications, 0 manual
coordination, no data loss):

1. **No backup of the actual product data.** `deploy/production/README.md` only
   documents backing up Kratos/Keto Postgres — the SQLite file holding wishlists, phone
   numbers, and draw history (`unwrap-gift-data` volume) is never backed up anywhere.
   Use `sqlite3 .backup` (not a raw file copy — WAL mode is on, so a plain `cp`/`tar`
   risks an inconsistent snapshot).
2. **A failed draw can get stuck forever with no recovery.** If `RunDraw` persists an
   assignment but SMS sending partially/fully fails, the draw sits at status
   `'assigned'` with no retry-notify endpoint (`internal/app/draws.go:337-377`,
   acknowledged in a code comment as a known v1 gap). Add `POST
   .../draws/{id}/notify` reusing the existing `finalizeNotifications` — this is the
   single highest-leverage fix given the actual deadline.
3. **`task run`, run exactly as the README documents, never starts the server** — it
   just prints the Cobra help and exits (verified by actually running it). Fix: default
   `CLI_ARGS` to `start` in `Taskfile.yml`, or doc it as `task run -- start`.
4. **`.env` is silently never loaded** by `task run`/`task build` (no `dotenv:` key in
   `Taskfile.yml`) — the documented 4-line quickstart leaves every feature (database,
   Kratos, Keto) silently disabled, with no warning, by the "nil is disabled" design.
   One-line fix: `dotenv: ['.env']`.
5. **`task db:seed` prints a false "seeded" success message** when Keto is disabled —
   sits right on the documented admin-bootstrap path, so you can follow the docs
   exactly and still get silent 403s later with no clue why.
6. **Member and Kratos Identity are two separate, unlinked admin records** maintained
   by hand (`internal/app/members.go` never touches Kratos) with no phone-format
   validation anywhere — a typo in either place silently breaks that person's login or
   wishlist lookup, with zero system-level detection. Add E.164 validation at creation
   time at minimum.
7. **Deleting a member or drawer cascade-deletes `assignments`** (`ON DELETE CASCADE`,
   migration `00003`), which erases real pairing history and undermines the "no repeats
   vs. last N years" feature if that person ever rejoins. `relationships`/
   `wishlist_items` were deliberately made phone-keyed to survive churn — `assignments`
   wasn't given the same treatment.
8. **No rate limit on the SMS login-code endpoint.** Anyone can POST an arbitrary phone
   number to Kratos's public login flow repeatedly, triggering unlimited billed SMS to
   that number — a real cost/harassment vector, not gated by app-level auth since
   there's no self-service signup to prevent enumeration.

## Correctness bugs

- **DELETE silently no-ops instead of 404** on Drawers, Relationships, and Widgets
  (`internal/app/{drawers,relationships,widgets}.go`) — no rows-affected check, unlike
  Members/WishlistItems which correctly `Get`-then-404. Untested in both directions.
- **Timestamp/timezone handling risks wrong draw ordering.** The SQLite DSN
  (`internal/db/store.go:124`) sets no `_loc`, so `exchange_date` is stored with
  whatever offset the client sent and sorted as a raw TEXT comparison — `ORDER BY
  exchange_date DESC` feeds directly into the history-window exclusion logic, so a
  sort-order bug here has real correctness impact on who gets excluded as a repeat
  pairing. Add `&_loc=UTC` and normalize the date before persisting.
- **No session logout anywhere** in the web UI — Kratos's logout flow is configured but
  never linked to. Real concern for SMS-OTP login on a shared family device.
- Draws have Create/Read but no Update/Delete, unlike Drawers/Members — a mistyped
  exchange date or budget can't be corrected before running the draw.

## Security (defense-in-depth, not actively exploited today)

- CSRF protection relies entirely on `SameSite=Lax` with no app-level token — works
  today, but single point of failure; dev config doesn't even set `same_site`
  explicitly.
- Kratos's `disallow_private_ip_ranges: false` is a blanket SSRF-hardening relaxation
  (needed for the courier webhook) rather than scoped narrowly to it.
- Wishlist URL field isn't server-side scheme-validated (template auto-escaping
  currently neutralizes it, so no live exploit, but worth an allowlist).
- Everything else checked out clean: courier webhook auth, router/authz boundary
  enforcement, IDOR on wishlist ownership, XSS, secrets management — all verified
  correct.

## CI/CD & deployment

- **Branch protection on `main` is off** (confirmed via GitHub API) — nothing stops a
  red-check push from triggering an image build.
- **Lint failures don't block the image build** — `golangci-lint.yml` is fully
  independent of `go.yml`'s `docker` job.
- **Secrets silently default to empty string** in
  `deploy/production/docker-compose.yml` (no `${VAR:?error}`) — Postgres/Kratos can
  start with blank passwords if `.env.production` is incomplete.
- No image vulnerability scanning, no SBOM, no Dependabot/Renovate, no resource limits
  or log rotation in production compose (one runaway container can OOM the whole VPS),
  no rollback runbook, no deployment smoke test, e2e tests never run in CI (only
  verified once, manually, per the Kratos README's own admission). Production also runs
  fully blind on observability — no OTEL collector deployed anywhere.

## Database (beyond cascade-delete above)

- Missing indexes: `drawers.organiser_kratos_identity_id` (full scan on every "list my
  drawers"), and `assignments.gifter_member_id`/`giftee_member_id` (full scan on every
  cascade-delete check).
- N+1 query patterns in `buildExclusions`, `buildHistory`, and `notifyMembers`
  (`internal/app/draws.go`) — harmless at current group sizes (~5-30) given the
  explicit design comment, but easy to batch into `IN (...)` queries if that ever
  changes.
- `draws.budget_amount` has no non-negativity CHECK constraint (acknowledged gap in
  code comment).

## Code quality / simplification

- Widgets scaffolding was never deleted despite the README's own instruction — it's
  still live and CRUD-able on the protected router in production, not just reference
  code. Worth an explicit decision.
- Six byte-for-byte-identical `strconv.ParseInt(chi.URLParam(...))` ID-parsing helpers
  across `internal/api/*.go` — safe to collapse into one shared helper.
- `internal/api/wishlist_items.go` repeats a 4-line identity/phone-extraction block in
  all four handlers; `internal/web/wishlist.go` already extracted this pattern — mirror
  it.
- `internal/assign.Assign` takes no `context.Context` (no live risk today given the
  node-cap bounds, but worth a note if group sizes ever grow).

## Local dev experience (remaining)

- Admin bootstrap docs never mention the much simpler `unwrap-gift task exec
  grant-admin --arg identity-id=<id>`, which sidesteps the whole `.env`-edit-and-reseed
  dance.
- `unwrap-gift info features`/`info env` — exactly the right tool for diagnosing "why
  is my feature disabled" — are never mentioned in onboarding docs.
- No pre-commit hook despite `task lint`/`task sqlc:check` existing as exact mirrors of
  CI's hard gates.
- No `.vscode/settings.json` mirroring `.zed/settings.json`'s `-tags=e2e` gopls hint —
  non-Zed editors silently gray out the e2e test files with no explanation.

## Feature gaps worth considering (not blockers)

- **Reminder SMS** before exchange date — the client/formatting helpers already exist,
  just needs a cron/CLI trigger.
- A **thin Cobra CLI** wrapping the hidden-router organiser API (`santa drawer create`,
  `santa draw run`, etc.) — far cheaper than a web UI and removes the hand-rolled-curl
  risk for your own two real draws.
- A **"my assignment" participant page** — right now the SMS is the *only* record of a
  giftee/date/budget; if it's deleted, it's gone.
- "Mark as claimed" gift-tracking is explicitly out of scope per the PRD — flagging as
  a deliberate decision to revisit or confirm, not an oversight.
