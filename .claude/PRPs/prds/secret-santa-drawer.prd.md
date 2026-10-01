# Secret Santa Drawer

## Problem Statement

Daniel runs a recurring Christmas Secret Santa for two separate groups (family, friend group) using a physical draw. This requires everyone present at once, makes it hard to guarantee variety year-on-year (avoiding repeat pairings) or keep certain people apart (e.g. couples), and gives gifters no way to know what their giftee actually wants.

## Evidence

- Direct account from the product owner/organiser (also the primary user): physical draw requires full-group presence, has no mechanism for exclusions or repeat-avoidance, and gifters get no wishlist information.
- Market confirms exclusions + wishlists are table-stakes for existing tools (Elfster, DrawNames, GiftList) — validating the core need — but none of the surveyed tools foreground "avoid repeats vs. prior years' draws" as a feature, and SMS is not native delivery in most (link-sharing via SMS/WhatsApp, not app-originated SMS). See Research Summary.

## Proposed Solution

A small Go web service (built on the existing `unwrap-gift` boilerplate: chi + Ory Kratos + Ory Keto + SQLite/sqlc + Cobra) that lets an organiser create a recurring "drawer" per group, add members with a shared cross-drawer relationship graph (for exclusions like couples), run a randomized assignment respecting exclusions and the last N years of that drawer's history, and automatically SMS every participant their giftee's name, wishlist, the exchange date, and budget via seven.io. Participants log in via SMS one-time-code (not TOTP — see Decisions Log) to maintain their own wishlist on a minimal web UI.

## Key Hypothesis

We believe a drawer/member/relationship-graph data model with a randomized-backtracking assignment engine and SMS delivery via seven.io will let the organiser run both real-world draws (family, friend group) with zero manual coordination.
We'll know we're right when both groups' Christmas 2026 draws run end-to-end through the app — every participant receives their SMS, no exclusion or repeat-match constraint is violated, and the organiser sends zero manual messages to coordinate the draw.

## What We're NOT Building

- Gift purchase tracking (marking gifts as bought/received) — not a stated need, adds a whole state-tracking surface for no validated benefit yet.
- In-app messaging/group chat between participants — SMS notification is the only communication channel in scope.
- Support for non-Christmas / arbitrary recurring event types — the domain model (Drawer → yearly Draw) is generalizable later, but v1 only needs to serve the two known annual draws.
- International/multi-country SMS formatting or carrier compliance beyond what seven.io handles out of the box — not needed for the known user base.
- A polished organiser-facing web UI — see Decisions Log; organiser flows can be API-driven for v1 since the organiser is the builder.

## Success Metrics

| Metric | Target | How Measured |
|--------|--------|---------------|
| Draws run successfully | 2/2 (family + friend group) for Christmas 2026 | Manual confirmation after each draw's SMS send |
| Constraint violations | 0 | No assignment violates an exclusion pair or last-N-draws repeat, verified by the assignment engine's own test suite plus a post-draw audit query |
| SMS delivery | 100% of members receive their notification | seven.io delivery status / manual spot-check |
| Manual coordination messages sent by organiser | 0 | Self-reported by organiser after each draw |

## Open Questions

- [ ] What N (number of prior years) should the repeat-match exclusion window default to? Configurable per drawer, or a single global constant?
- [ ] Frontend stack for the participant wishlist web UI is undecided (Go html/template server-rendered vs. a small separate SPA) — this repo currently has no frontend surface at all.
- [ ] Should the relationship graph (exclusions) be visible/editable by anyone other than the organiser, or fully organiser-only?
- [ ] Wishlist scope: is a member's wishlist a single persistent list they maintain year-round, or does it get reset/re-collected per Draw?
- [ ] What happens when no valid assignment exists (over-constrained group)? Organiser needs a clear failure signal and a way to relax constraints (e.g., drop the oldest year from the repeat-exclusion window) rather than a silent/opaque error.
- [ ] seven.io account/sender-ID setup and per-message cost — not yet configured, needed before end-to-end testing.

---

## Users & Context

**Primary User**

- **Who**: Daniel, acting as organiser for two recurring groups (family, friends). Same person is also a participant in at least one of those groups.
- **Current behavior**: Runs a physical, everyone-present draw once a year around the Christmas season.
- **Trigger**: Christmas season approaching — draw needs to happen without gathering everyone physically.
- **Success state**: Creates/reuses a drawer, adds/confirms members and exclusions, runs the draw, and every participant has received their SMS with correct, constraint-respecting details — with no manual follow-up.

**Secondary User**

- **Who**: Drawer members (family/friends) — participants, not organisers.
- **Current behavior**: Physically drawing a name; no digital wishlist today.
- **Trigger**: Receiving the draw-result SMS; separately, wanting to maintain their wishlist before/after the draw.
- **Success state**: Receives one clear SMS with giftee name, wishlist, date, and budget; can log in via SMS code to view/edit their own wishlist at any time.

**Job to Be Done**

When December approaches, I (the organiser) want to run our Secret Santa draw without gathering everyone in person, so I can guarantee no couples or repeat pairings from recent years and have everyone notified automatically with what their giftee wants.

**Non-Users**

Public/anonymous drawers, people without a phone number, non-Christmas event organisers, anyone wanting gift-purchase tracking or in-app chat — all explicitly out of scope for v1.

**Constraints**

- Time: Christmas season is approaching now (2026-09-29) — the real-world deadline is this year's exchange date for both groups.
- Team: solo project (Daniel).
- SMS provider: seven.io (`github.com/seven-io/go-client/seven`), cost/volume unconfirmed (see Open Questions).
- Must build on the existing `unwrap-gift` boilerplate and its conventions (three-router model, nil-is-disabled feature flags, interface segregation, Widgets-pattern resource layering).

---

## Solution Detail

### Core Capabilities (MoSCoW)

| Priority | Capability | Rationale |
|----------|------------|-----------|
| Must | Drawer setup (create/edit a recurring group) | Core unit the rest of the product hangs off |
| Must | Member management (add/remove members, full name + phone number) | Needed to invite and identify participants; full name explicitly required per organiser |
| Must | Relationship/exclusion graph, shared across drawers | Organiser's core pain point — prevents couples etc. being matched, reusable if the same two people are in multiple groups |
| Must | Last-N-draws history exclusion per drawer | Organiser's other core pain point — guarantees year-on-year variety |
| Must | Randomized valid assignment engine (exclusions + history, clear failure on no solution) | The actual "draw" — must be provably correct via tests, not just "looks random" |
| Must | SMS notification (date, budget, giftee name, giftee wishlist) via seven.io | Primary distribution mechanism — replaces the physical draw entirely |
| Must | Participant self-serve wishlist (web, item name + optional size + optional URL) | Explicitly required for MVP per organiser |
| Must | SMS one-time-code login for participants (Kratos `code` method, phone as identifier) | Matches an SMS-native product and avoids passwords for a low-frequency-use group |
| Should | Multi-drawer support per person (same phone number/identity across family + friend group) | Falls out naturally from a shared relationship graph but isn't required for a single group's first run |
| Should | Reminder SMS before the exchange date | Nice reliability improvement, not required to prove the core hypothesis |
| Could | Organiser web UI (vs. API-driven organiser flows) | Organiser is the builder for v1; can drive setup via API/CLI initially |
| Won't | Gift purchase tracking | Explicitly out of scope |
| Won't | In-app messaging/chat | Explicitly out of scope |

### MVP Scope

Both real draws (family, friends) run end-to-end: organiser creates a drawer, adds members + exclusions, the system computes a valid assignment respecting exclusions and history, and every member gets an SMS with their assignment details. Members can log in via SMS code and maintain their wishlist through a minimal web UI before the draw runs.

### User Flow

1. Organiser creates a Drawer (name, e.g. "Kirkwood Family Christmas").
2. Organiser adds Members (full name + phone number) to the Drawer.
3. Organiser (or the relationship graph, if already populated from a prior drawer) marks exclusion pairs (e.g. couples).
4. Members log in via SMS code, land on their wishlist page, add items (name, optional size, optional URL).
5. Organiser sets the Draw's exchange date and budget, then triggers the draw.
6. Assignment engine computes a valid gifter→giftee mapping (excluding relationship-graph pairs and any pairing present in the drawer's last N draws), or returns a clear "no valid assignment" error.
7. On success, each member receives one SMS: exchange date, budget, giftee's full name, giftee's current wishlist (or a link to it).
8. The Draw and its resulting Assignments are stored as that Drawer's history, feeding next year's repeat-exclusion check.

---

## Technical Approach

**Feasibility**: HIGH — this repo *is* the intended starting point (per its own README/ARCHITECTURE.md), with auth, authz, persistence, observability, and CLI already wired, and a documented worked example (`Widgets`) for adding exactly this shape of new resource.

**Architecture Notes**

- Follow the `Widgets` resource pattern (`ARCHITECTURE.md`, "Adding a new resource") for each new resource: migration → sqlc query → generated code → DB integration test → `internal/api` DTO/handler/interface → API unit test (fake store) → `internal/app` adapter → router wiring. Apply this per-resource for Drawer, Member, Relationship, Draw, Assignment, Wishlist/WishlistItem.
- New resources are authenticated (protected router, `deps.Auth` only) for participant-facing actions (own wishlist); organiser-only actions (drawer/member/relationship management, triggering a draw) likely belong on the hidden router behind `deps.Auth` + `deps.Authz`, using Keto relation tuples the same way `internal/api/identities.go` does — needs an OPL namespace addition in `deploy/keto/identities.ts` for "organiser of drawer X".
- New `sms` feature: add to `internal/config/features.go`/`registry.go` (required env: an API key, following the existing `RequiredEnv`/"nil is disabled" convention) and a new `internal/clients/smsclient` wrapping `github.com/seven-io/go-client/seven`, mirroring `kratosclient`/`ketoclient`'s shape (narrow interface declared in `internal/api` or wherever it's consumed, concrete client wired in `internal/app`).
- Auth correction: the organiser asked for "TOTP" for SMS-based login, but TOTP (RFC 6238, authenticator-app codes) is unrelated to SMS — what actually fits is Kratos's **`code` method** (one-time code delivered via a channel Kratos calls through its courier), with the identity schema's phone-number trait as the identifier instead of email. This requires: (1) a new `identity.schema.json` with `phone` (identifier, required) and `full_name` (required trait, per organiser's explicit ask to track full names) instead of the current `email`-based preset; (2) courier delivery reconfigured to call out to seven.io for the code itself (Kratos courier supports a custom HTTP request-config delivery strategy) rather than SMTP/mailslurper. Flagged as a Decision below, not silently assumed.
- Assignment engine (exclusions + history-aware randomized backtracking) is pure domain logic with no I/O — candidate for its own package (e.g. `internal/assign`) rather than living in `internal/app`, so it can be exhaustively unit-tested (including adversarial/over-constrained cases) independent of the DB. `internal/app` would call it and persist the result via the `db.Store`.
- No frontend currently exists in this repo (it's an API-only boilerplate); the MVP requirement for web-based wishlist entry means a minimal frontend surface must be added or hosted separately — stack TBD (see Open Questions).

**Technical Risks**

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Kratos self-hosted courier doesn't cleanly support arbitrary SMS providers out of the box | M | Confirm seven.io integration via Kratos's custom courier HTTP request-config early, as a spike, before building dependent flows |
| Over-constrained group (exclusions + history leave no valid assignment) | M | Assignment engine must detect and report this explicitly (not hang/silently fail); organiser needs a defined recovery path (see Open Questions) |
| No frontend exists yet — wishlist UI is new scope, not just "add a resource" | H | Treat as its own phase with an explicit stack decision before starting; keep it minimal (login + wishlist CRUD only) for v1 |
| seven.io cost/volume/account setup unconfirmed | L | Small group sizes (family/friends) keep SMS volume trivial; confirm account setup as an early spike |

---

## Implementation Phases

<!--
  STATUS: pending | in-progress | complete
  PARALLEL: phases that can run concurrently (e.g., "with 3" or "-")
  DEPENDS: phases that must complete first (e.g., "1, 2" or "-")
  PRP: link to generated plan file once created
-->

| # | Phase | Description | Status | Parallel | Depends | PRP Plan |
|---|-------|-------------|--------|----------|---------|----------|
| 1 | Data model & migrations | Drawer, Member, Relationship, Draw, Assignment, Wishlist/WishlistItem tables + sqlc queries + DB tests | in-progress | with 2, 3 | - | [#2](https://github.com/DanielKirkwood/unwrap-gift/issues/2) · [plan](../plans/secret-santa-data-model.plan.md) |
| 2 | SMS-code auth via Kratos + seven.io | New identity schema (phone + full name), courier reconfigured to call seven.io, spike to confirm feasibility | in-progress | with 1 | 3 | [#3](https://github.com/DanielKirkwood/unwrap-gift/issues/3) · [plan](../plans/secret-santa-sms-auth.plan.md) |
| 3 | SMS client (seven.io) | `internal/clients/smsclient` + `sms` feature flag wiring | in-progress | with 1, 2 | - | [#4](https://github.com/DanielKirkwood/unwrap-gift/issues/4) · [plan](../plans/secret-santa-sms-client.plan.md) |
| 4 | Assignment engine | Randomized backtracking solver honoring exclusions + last-N-draws history, heavily unit-tested incl. no-solution case | complete | - | 1 | [#5](https://github.com/DanielKirkwood/unwrap-gift/issues/5) · [plan](../plans/completed/secret-santa-assignment-engine.plan.md) · [report](../reports/secret-santa-assignment-engine-report.md) |
| 5 | Organiser API | Drawer/Member/Relationship CRUD, Draw creation + run-draw endpoint (hidden router, Auth+Authz) | complete | with 6 | 1, 4 | [#6](https://github.com/DanielKirkwood/unwrap-gift/issues/6) · [plan](../plans/completed/secret-santa-organiser-api.plan.md) · [report](../reports/secret-santa-organiser-api-report.md) |
| 6 | Participant API | Wishlist CRUD, self-serve (protected router, Auth only) | complete | with 5 | 1, 2 | [#7](https://github.com/DanielKirkwood/unwrap-gift/issues/7) · [plan](../plans/completed/secret-santa-participant-api.plan.md) · [report](../reports/secret-santa-participant-api-report.md) |
| 7 | Notification pipeline | On draw finalize, compose + send SMS per member via smsclient | pending | - | 3, 5 | [#8](https://github.com/DanielKirkwood/unwrap-gift/issues/8) |
| 8 | Wishlist web UI | Minimal frontend: SMS-code login + wishlist CRUD | pending | - | 2, 6 | [#9](https://github.com/DanielKirkwood/unwrap-gift/issues/9) |

**Tracking**: [Epic #1](https://github.com/DanielKirkwood/unwrap-gift/issues/1) · [Project board](https://github.com/users/DanielKirkwood/projects/6)

### Phase Details

**Phase 1: Data model & migrations**
- **Goal**: Persistent schema for every new domain entity.
- **Scope**: goose migrations + sqlc queries + generated code + DB integration tests for Drawer, Member, Relationship, Draw, Assignment, Wishlist, WishlistItem.
- **Success signal**: `task db:migrate` applies cleanly; DB-layer tests pass against real SQLite.

**Phase 2: SMS-code auth via Kratos + seven.io**
- **Goal**: Participants can authenticate via an SMS one-time code, identified by phone number, with full name captured at registration.
- **Scope**: New `identity.schema.json`, courier config change to route codes through seven.io, spike confirming Kratos's custom courier delivery works with seven.io's API.
- **Success signal**: A test identity can register with phone + full name and complete a login flow via a real SMS code.

**Phase 3: SMS client (seven.io)**
- **Goal**: A reusable, feature-flagged client for sending arbitrary SMS (the draw-result notification, separate from the Kratos auth codes).
- **Scope**: `internal/clients/smsclient`, `sms` feature registered in `internal/config/registry.go`.
- **Success signal**: A unit-testable client with a narrow interface, wired only when its required env var(s) are set, following the `kratosclient`/`ketoclient` shape.

**Phase 4: Assignment engine**
- **Goal**: Given a member list, an exclusion set (from the relationship graph, filtered to the drawer), and a set of forbidden pairs (from the last N draws), produce a valid random gifter→giftee mapping or a clear "no solution" error.
- **Scope**: Pure package, no I/O, randomized backtracking with retry/backoff, exhaustive unit tests including adversarial over-constrained inputs.
- **Success signal**: Property-style tests confirm no output ever violates an exclusion or history constraint, and an intentionally over-constrained input returns a clear error rather than hanging.

**Phase 5: Organiser API**
- **Goal**: Organiser can manage drawers/members/relationships and trigger a draw.
- **Scope**: Hidden-router endpoints (Auth + Authz) for Drawer/Member/Relationship CRUD and a run-draw endpoint that calls the assignment engine and persists the resulting Assignments.
- **Success signal**: Organiser can, via API calls alone, fully set up and run a drawer end-to-end.

**Phase 6: Participant API**
- **Goal**: Members can manage their own wishlist.
- **Scope**: Protected-router endpoints (Auth only) for Wishlist/WishlistItem CRUD, scoped to the authenticated member.
- **Success signal**: A logged-in member can add/edit/remove their own wishlist items and cannot see or edit anyone else's.

**Phase 7: Notification pipeline**
- **Goal**: Every member automatically receives their assignment via SMS once a draw is finalized.
- **Scope**: On successful draw run, compose one message per member (date, budget, giftee full name, giftee's current wishlist) and send via `smsclient`.
- **Success signal**: Running a real draw for a small test drawer results in every member receiving a correct, complete SMS.

**Phase 8: Wishlist web UI**
- **Goal**: Non-technical participants can log in and manage their wishlist without touching the API directly.
- **Scope**: Minimal web frontend — SMS-code login flow, wishlist view/edit. Stack TBD (see Open Questions).
- **Success signal**: A real (non-technical) family/friend-group member can, unassisted, log in and add wishlist items.

### Parallelism Notes

Phases 1, 2, and 3 touch independent parts of the system (schema, auth, SMS client) and can be built concurrently. Phase 4 (assignment engine) only depends on the data shapes from Phase 1 and can be developed test-first against in-memory types before the DB layer is even final. Phases 5 and 6 both depend on Phase 1 (and 5 additionally on 4) but are independent of each other (organiser vs. participant surfaces) and can run in parallel. Phase 7 needs both the SMS client (3) and a working draw-run endpoint (5). Phase 8 is last since it depends on both the auth flow (2) and the participant API (6) being stable.

---

## Decisions Log

| Decision | Choice | Alternatives | Rationale |
|----------|--------|--------------|-----------|
| Build vs. buy for assignment logic | Hand-rolled randomized backtracking | General CSP solver library (e.g. `centipede`) | Group sizes are small (~5-30); a narrow, exhaustively-tested purpose-built function is easier to verify and debug than adapting a general solver's API |
| Relationship/exclusion graph scope | Global, shared across all of a person's drawers | Per-drawer exclusion list | Explicit organiser choice — a couple should stay excluded whether they're in the family or friend-group drawer |
| Repeat-match history scope | Per drawer (each drawer keeps its own last-N-draws history) | Global across all drawers | Explicit organiser choice — family and friend-group histories are independent |
| Participant auth method | Kratos `code` method (SMS one-time code), phone as identifier | TOTP (authenticator app) as originally requested; password auth | Organiser's actual requirement is SMS-native login, which is Kratos's `code` method, not TOTP (an unrelated 2FA mechanism) — corrected during technical grounding |
| Organiser-facing UI for v1 | API-driven (no dedicated organiser UI) | Full web UI for organiser flows too | Organiser is the sole builder for v1; a UI only pays off once other people need to organise drawers |

---

## Research Summary

**Market Context**

Elfster, DrawNames, and GiftList all already support random draws with pairwise exclusions and shareable wishlists; SMS is typically used for link-sharing (WhatsApp/text), not app-originated notification SMS. None foreground "avoid repeats vs. prior years" as a feature — this is the clearest differentiator for a recurring-group tool over a one-off generator. Sources: [GiftList 2026 roundup](https://giftlist.com/blog/best-secret-santa-apps-2026), [Elfster](https://play.google.com/store/apps/details?id=com.elfster.elfdroid), [drawnames](https://play.google.com/store/apps/details?id=com.meetrixonline.drawnames), [uplup.com](https://uplup.com/secret-santa-generator).

**Technical Context**

- This repo is the organiser's own Go API boilerplate (`README.md`/`ARCHITECTURE.md`), explicitly meant to be forked as the starting point for exactly this kind of project, with `Widgets` as a documented worked example for adding new authenticated, DB-backed resources.
- Ory Kratos (self-hosted) supports SMS one-time-code login (`code` method) via a custom courier delivery strategy, not natively bundled SMS sending — confirmed via Kratos GitHub issues/discussions and docs on SMS-provider integration (e.g. WhatsApp Business OTP courier as a precedent pattern). No current SMS courier wiring exists in this repo's `deploy/kratos/kratos.yml` (SMTP/mailslurper only, email-based identity schema).
- Go CSP/backtracking libraries exist (`centipede`, `troydai/csp`, `BaseMax/go-constraint-solver`) but are unnecessary for this problem's scale — see Decisions Log.

---

*Generated: 2026-09-29*
*Status: DRAFT - needs validation*
