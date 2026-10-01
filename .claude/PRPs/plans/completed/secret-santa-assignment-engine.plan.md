# Plan: Assignment Engine

## Summary

A new pure, I/O-free `internal/assign` package that computes a valid, randomized gifter→giftee
mapping for a Secret Santa drawer, given a member list, a set of permanent (relationship-graph)
exclusions, and a set of directional history pairs from recent draws. Returns a clear sentinel
error when no valid assignment exists rather than hanging or guessing.

## User Story

As the organiser, I want the system to compute a provably-valid random Secret Santa assignment
that respects couple exclusions and avoids repeating last year's exact pairings, so that I never
have to manually check or re-draw by hand.

## Problem → Solution

Today there is no code that performs the actual "draw" — only the data model (Phase 1, in
progress) to store members, relationships, draws, and assignments. This phase adds the solver
itself: a small, heavily-tested, dependency-free package that Phase 5 (Organiser API) will call
and then persist the result via `db.Store`.

## Metadata

- **Complexity**: Medium
- **Source PRD**: `.claude/PRPs/prds/secret-santa-drawer.prd.md`
- **PRD Phase**: Phase 4 — Assignment engine
- **Estimated Files**: 3 new (`internal/assign/{doc,assign,assign_test}.go`), 1 updated (PRD phase table)
- **Tracking issue**: [#5](https://github.com/DanielKirkwood/unwrap-gift/issues/5)

---

## UX Design

N/A — internal change. This is a pure Go package with no HTTP surface, UI, or external I/O. It is
exercised only by Phase 5 (Organiser API) and by its own unit tests in this phase.

---

## Planning Decisions (resolved 2026-10-01, asked rather than assumed)

The PRD flags two open questions that directly affect this engine's signature and correctness
definition. Resolved with the user before writing this plan:

1. **History exclusion direction**: **directional only**. If Alice gifted Bob in a past draw,
   only Alice→Bob is forbidden again in future draws; Bob→Alice remains allowed. (Relationship
   exclusions — couples — remain symmetric/bidirectional, since those are a different kind of
   constraint.)
2. **No-solution handling**: the engine **just returns a clear sentinel error**. It does not
   attempt to auto-relax constraints (e.g. dropping the oldest history year). That orchestration —
   deciding *how* to retry with a smaller forbidden set — is explicitly Phase 5's job, not this
   package's. This phase only needs to guarantee the error is reliable (no false "no solution" on
   a genuinely solvable input, no hang on a genuinely unsolvable one).
3. **Constraint input shape**: exclusions and history are **two distinct inputs** to `Assign`, not
   pre-merged by the caller. This keeps the two concerns traceable and leaves room for a future,
   more specific diagnostic (e.g. "unsolvable even ignoring history") without a signature change.

These decisions are binding for this plan; they are not re-litigated during implementation.

---

## Mandatory Reading

| Priority | File | Lines | Why |
|---|---|---|---|
| P0 | `ARCHITECTURE.md` | all | Dependency-direction rule (only `app` imports both `api` and `db`) and interface-segregation convention — `internal/assign` must stay dependency-free, no `internal/db`/`internal/api` imports |
| P0 | `internal/api/widgets.go` | 1-28 | Sentinel error naming (`ErrWidgetNotFound`, `"api: "`-prefixed message) and exported-type doc-comment style to mirror for `assign`'s own sentinels |
| P0 | `internal/db/migrations/00003_secret_santa_drawer.sql` | 1-69 | Confirms the real shapes this package's types must be compatible with: `members.id`/`assignments.gifter_member_id`/`giftee_member_id` are `INTEGER` (int64), `relationships` is symmetric (`phone_number_a < phone_number_b` canonical ordering), `assignments` has a `CHECK (gifter_member_id != giftee_member_id)` |
| P1 | `internal/db/sqlc/models.go` | 12-18, 49-54 | Exact Go field names/types for `sqlc.Assignment` and `sqlc.Relationship` that Phase 5 will eventually convert to/from this package's types — confirms `int64` is the right underlying type for `MemberID` |
| P1 | `internal/api/widgets_test.go` | 1-69 | Test file structure to mirror: external `_test` package, plain stdlib `t.Errorf`/`t.Fatalf` (no testify), `t.Parallel()` on every subtest |
| P1 | `.golangci.yml` | depguard rule (search `math/rand$`), `mnd`, `funlen`, `cyclop`, `gocognit`, `nolintlint` settings | Hard constraints the solver implementation must satisfy — see Gotchas below |
| P2 | `internal/db/doc.go` | all | One-line package-doc-comment pattern to mirror for `internal/assign/doc.go` |
| P2 | `.claude/PRPs/prds/secret-santa-drawer.prd.md` | 143-241 | Phase 4 goal/scope, Decisions Log ("hand-rolled randomized backtracking... group sizes small ~5-30"), Parallelism Notes (confirms Phase 4 doesn't need Phase 1 *complete*, only its data shapes, which are already migrated) |

## External Documentation

No external research needed — feature uses only Go's standard library (`math/rand/v2`) and
established internal patterns (sentinel errors, package-local DTOs decoupled from `sqlc`).

---

## Patterns to Mirror

### NAMING_CONVENTION
// SOURCE: internal/api/widgets.go:15-28
```go
// ErrWidgetNotFound is returned by WidgetStore implementations when no
// widget exists for the given id. app maps it to a 404 Problem via the
// Adapter it builds for the protected router.
var ErrWidgetNotFound = errors.New("api: widget not found")

// Widget is api's own representation of a widget row, decoupled from
// internal/db/sqlc.Widget — api never imports internal/db, per
// ARCHITECTURE.md's dependency-direction rule.
type Widget struct {
	ID        int64     `json:"id"`
	...
}
```
Apply the same shape to `assign`: `"assign: "`-prefixed sentinel error messages, exported types
with a doc comment explaining *why* they're decoupled from `sqlc` types.

### ERROR_HANDLING
// SOURCE: internal/api/widgets.go:15-18
Sentinel errors, `Err`-prefixed (satisfies the `errname` linter), compared with `errors.Is` by
callers — not custom error structs, since there's no extra data to carry beyond "which failure
mode."

### DOC_COMMENT
// SOURCE: internal/db/doc.go:1-3
```go
// Package db holds the database layer: connection setup, migrations, and
// generated sqlc queries.
package db
```
Mirror exactly for `internal/assign/doc.go`.

### TEST_STRUCTURE
// SOURCE: internal/api/widgets_test.go:1-20, 70-105
```go
package api_test

import (
	...
	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

func TestMountWidgets_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct{ name string; ... }{ ... }

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			...
		})
	}
}
```
External `_test` package (required by the `testpackage` linter), table-driven subtests, explicit
`t.Parallel()` on both outer and inner tests (required by `paralleltest`/`tparallel`), plain stdlib
assertions — no `testify`, even though it's present as an indirect dependency elsewhere.

---

## Files to Change

| File | Action | Justification |
|---|---|---|
| `internal/assign/doc.go` | CREATE | Package doc comment, mirrors `internal/db/doc.go` |
| `internal/assign/assign.go` | CREATE | `MemberID`, `ExclusionPair`, `HistoryPair` types, sentinel errors, `Assign` entry point, backtracking solver |
| `internal/assign/assign_test.go` | CREATE | Table-driven validation tests, deterministic adversarial cases, randomized property-style tests |
| `.claude/PRPs/prds/secret-santa-drawer.prd.md` | UPDATE | Phase 4 row: status `pending` → `in-progress`, add this plan's link |

## NOT Building

- Any database/persistence code — Phase 1 already owns the schema; this package has zero I/O and
  never imports `internal/db`.
- Any HTTP endpoint or organiser-facing trigger — that's Phase 5 (Organiser API), which will call
  `assign.Assign` and persist the result.
- Resolution of "last N years" into concrete `HistoryPair` values from `draws`/`assignments` rows
  — this package only accepts already-resolved pairs. Translating drawer history + a window size
  into those pairs is Phase 5's responsibility (and the PRD's open question on what N should
  default to is explicitly *not* answered by this phase).
- Automatic constraint relaxation on `ErrNoValidAssignment` (e.g. dropping the oldest history
  year and retrying) — per the Planning Decisions above, this is deliberately left to the caller.
- A general-purpose/exported CSP solver API, generics over arbitrary ID types, or configurable
  retry/attempt-count knobs — YAGNI for the PRD's stated group sizes (~5-30); can be added later
  if real usage demands it.

---

## Step-by-Step Tasks

### Task 1: Package doc comment
- **ACTION**: Create `internal/assign/doc.go`.
- **IMPLEMENT**:
  ```go
  // Package assign implements the Secret Santa drawer's randomized
  // assignment solver: given a member list, pairwise exclusions, and
  // directional history from prior draws, it computes a valid
  // gifter→giftee mapping or returns a clear error when none exists. It
  // performs no I/O and has no dependency on internal/db or internal/api;
  // callers convert to and from their own ID types.
  package assign
  ```
- **MIRROR**: `internal/db/doc.go`.
- **IMPORTS**: none.
- **GOTCHA**: `godot` requires the comment end in a period; `godoclint`/`revive` expect it to start
  with "Package assign".
- **VALIDATE**: `go build ./internal/assign/...` succeeds (empty package compiles).

### Task 2: Core types and sentinel errors
- **ACTION**: In `internal/assign/assign.go`, define the package's own types (decoupled from
  `sqlc`) and its three sentinel errors.
- **IMPLEMENT**:
  ```go
  package assign

  import "errors"

  // MemberID identifies a member for the purposes of a single Assign call.
  // It is its own type, not a bare int64, so a drawer or draw id can't be
  // passed to Assign by mistake — callers convert from sqlc.Member.ID.
  type MemberID int64

  // ExclusionPair is an unordered pair of members who must never be
  // matched as gifter/giftee in either direction (e.g. a couple). Order
  // does not matter: {A, B} forbids both A→B and B→A.
  type ExclusionPair struct {
  	A MemberID
  	B MemberID
  }

  // HistoryPair records a directional gifter→giftee pairing from a past
  // draw. Only that exact direction is forbidden again — the reverse
  // pairing remains allowed.
  type HistoryPair struct {
  	Gifter MemberID
  	Giftee MemberID
  }

  // ErrTooFewMembers is returned when fewer than two members are given —
  // no valid assignment can exist below that.
  var ErrTooFewMembers = errors.New("assign: at least two members are required")

  // ErrDuplicateMember is returned when the member list contains the same
  // MemberID more than once.
  var ErrDuplicateMember = errors.New("assign: duplicate member id in input")

  // ErrNoValidAssignment is returned when every possible gifter→giftee
  // mapping violates at least one exclusion or history constraint.
  var ErrNoValidAssignment = errors.New("assign: no valid assignment exists for the given constraints")
  ```
- **MIRROR**: `internal/api/widgets.go`'s `ErrWidgetNotFound` — `Err`-prefixed, package-qualified
  `"assign: "` message prefix.
- **IMPORTS**: `errors`.
- **GOTCHA**: `gochecknoglobals` is enabled repo-wide, but `ErrWidgetNotFound` already proves
  package-level sentinel `error` vars pass this repo's lint config — no `//nolint` needed.
- **VALIDATE**: `go build ./internal/assign/...`.

### Task 3: Input validation
- **ACTION**: Add an unexported `validate(members []MemberID) (map[MemberID]struct{}, error)`
  helper in `assign.go`.
- **IMPLEMENT**: Returns `ErrTooFewMembers` if `len(members) < minMembers` (named const
  `minMembers = 2`, not a bare literal — `mnd` is enabled). Otherwise builds and returns a
  `map[MemberID]struct{}` of the member set, returning `ErrDuplicateMember` the moment an id is
  seen twice.
- **MIRROR**: n/a — new logic; keep it under `funlen`'s 100-line/50-statement cap by itself (it
  will be well under).
- **GOTCHA**: `nonamedreturns` is enabled — do not use named return values anywhere in this
  package, including here.
- **VALIDATE**: Unit tests in Task 7 cover 0 members, 1 member, and a duplicate-id case.

### Task 4: Forbidden-candidate index
- **ACTION**: Add an unexported helper that turns `exclusions` and `history` into a lookup the
  solver can query in O(1): `buildForbidden(members map[MemberID]struct{}, exclusions []ExclusionPair, history []HistoryPair) map[MemberID]map[MemberID]bool`.
- **IMPLEMENT**: For every member, forbid self (gifter == giftee is always invalid — this also
  covers the DB's own `CHECK (gifter_member_id != giftee_member_id)` constraint). For each
  `ExclusionPair{A, B}`, forbid both A→B and B→A. For each `HistoryPair{Gifter, Giftee}`, forbid
  only Gifter→Giftee. **Silently skip** any pair where `A`/`B`/`Gifter`/`Giftee` is not in the
  `members` set — the relationship graph is global across drawers (per the PRD's Decisions Log)
  and may legitimately reference people outside the current drawer; this package assumes the
  caller has already scoped history but defensively tolerates an unscoped relationship graph.
- **MIRROR**: n/a — new logic.
- **GOTCHA**: build this on top of the `map[MemberID]struct{}` returned by Task 3's `validate`, so
  the "is this id actually in the group" check is O(1), not a linear scan per pair.
- **VALIDATE**: a dedicated unit test asserts a pair referencing a non-member id is a silent no-op
  (doesn't error, doesn't affect the result for in-set members).

### Task 5: Backtracking solver core
- **ACTION**: Add the actual search: an unexported
  `solve(members []MemberID, forbidden map[MemberID]map[MemberID]bool) (map[MemberID]MemberID, bool)`,
  decomposed into small helpers to stay under `funlen`/`cyclop`/`gocognit` limits.
- **IMPLEMENT**: Classic backtracking over "assign a giftee to each gifter":
  1. `mostConstrainedGifter(unassigned []MemberID, candidatesRemaining func(MemberID) int) MemberID`
     — most-constrained-variable (MRV) heuristic: pick the unassigned gifter with the fewest
     remaining valid candidates, so infeasible branches fail fast instead of being discovered only
     after exploring everything else first. Break ties randomly.
  2. For that gifter, compute its candidate giftees (not yet used as a giftee by anyone, not in
     `forbidden[gifter]`), shuffle them with `math/rand/v2`, and try each in turn: tentatively
     assign, recurse on the remaining unassigned gifters, and if the recursive call fails,
     backtrack (unassign, try the next shuffled candidate).
  3. Base case: no unassigned gifters left → success, return the accumulated map.
  4. A node-visit counter bounded by a generous named constant
     (`maxSearchNodes = 1_000_000`) is a pure hang-guard: if exceeded, return `(nil, false)` — the
     same outward signal as genuine infeasibility. For the PRD's stated group sizes (~5-30) with
     the MRV heuristic this should never trigger; it exists so a future, much larger drawer fails
     cleanly instead of hanging, which is the exact risk the PRD calls out.
- **MIRROR**: n/a — new domain logic. Per the PRD's Decisions Log, this is intentionally
  hand-rolled rather than a general CSP library, since group sizes are small enough that a narrow,
  exhaustively-tested function is easier to verify than adapting a general solver's API.
- **IMPORTS**: `math/rand/v2`.
- **GOTCHA 1**: `.golangci.yml`'s `depguard` **forbids importing plain `math/rand`** in non-test
  files ("Use math/rand/v2 instead") — import `math/rand/v2`, not `math/rand`.
- **GOTCHA 2**: `gosec` may still flag `math/rand/v2` usage as a weak-RNG finding (G404). If it
  does, this is a legitimate false positive — the randomness here is for draw variety, not
  security — and the fix is a targeted suppression, not switching to `crypto/rand`:
  `//nolint:gosec // not security-sensitive: only shuffles candidate order in the Secret Santa solver`.
  `.golangci.yml`'s `nolintlint` settings require both an explanation and naming the specific
  linter, so a bare `//nolint` will itself fail lint.
- **GOTCHA 3**: `nonamedreturns` — no named returns, including on `solve` and its helpers.
- **GOTCHA 4**: Keep `solve` itself short by extracting `mostConstrainedGifter` and a
  candidate-shuffling helper into their own functions — a single function containing the full
  recursive backtracking loop inline will likely trip `funlen` (100 lines/50 statements), `cyclop`
  (30), and/or `gocognit` (20).
- **VALIDATE**: exercised indirectly by Task 7's tests; `go vet ./internal/assign/...` and
  `golangci-lint run ./internal/assign/...` both clean.

### Task 6: Public `Assign` entry point
- **ACTION**: Add the exported function in `assign.go`:
  `func Assign(members []MemberID, exclusions []ExclusionPair, history []HistoryPair) (map[MemberID]MemberID, error)`.
- **IMPLEMENT**: Call `validate(members)`; on error, return it immediately. Call
  `buildForbidden(memberSet, exclusions, history)`. Call `solve(members, forbidden)`. If it
  succeeded, return `(result, nil)`; otherwise return `(nil, ErrNoValidAssignment)`.
- **MIRROR**: `internal/api/widgets.go`'s doc-comment style for exported functions — explain *what*
  it does and *why* the error cases exist, not just restate the signature.
- **GOTCHA**: `math/rand/v2`'s package-level functions (`rand.N`, `rand.Shuffle`, etc.) are already
  safely auto-seeded — unlike `math/rand` v1, no manual `rand.Seed`/source plumbing is needed or
  wanted here.
- **VALIDATE**: `go doc ./internal/assign Assign` renders a clear doc comment; full test suite
  (Task 7) passes.

### Task 7: Unit and property-style tests
- **ACTION**: Create `internal/assign/assign_test.go`.
- **IMPLEMENT**:
  - Table-driven validation tests: 0 members, 1 member, and a duplicate member id, each asserting
    the specific sentinel via `errors.Is`.
  - Deterministic adversarial cases:
    - Two mutually-excluded members → `ErrNoValidAssignment`.
    - A 3-member group where exclusions + history leave one member with zero valid candidates →
      `ErrNoValidAssignment`.
    - A tightly-constrained but solvable case (e.g. 3 members in a fixed exclusion cycle that
      still has exactly one valid derangement) asserting the *exact* expected mapping, not just
      "no error" — proves the solver isn't accidentally permissive.
    - A case with a history pair that reverses cleanly: given `HistoryPair{A, B}`, confirm a
      resulting assignment is allowed to pair B→A (proves direction-only, per the Planning
      Decision).
  - The "ignores unrelated relationship pairs" case from Task 4's validate step.
  - A randomized property-style test: loop ~200-500 iterations generating a random group size
    (e.g. 3-15 members), random exclusion pairs, and random history pairs (including some that
    reference ids outside the member set, to exercise the ignore path). Call `Assign`; whenever it
    returns a non-nil result, assert the invariants that must *always* hold: every member appears
    exactly once as a gifter and exactly once as a giftee (it's a permutation with no fixed
    points), no `ExclusionPair` is violated in either direction, and no `HistoryPair`'s exact
    gifter→giftee direction is repeated.
  - A no-hang regression guard at the top of the PRD's stated range (~30 members, no constraints),
    asserting `Assign` returns promptly — not a benchmark, just a guard against an accidental
    non-terminating code path being introduced later.
- **MIRROR**: `internal/api/widgets_test.go` — external `package assign_test`, table-driven
  subtests, `t.Parallel()` on every subtest, plain stdlib `t.Errorf`/`t.Fatalf` (no testify).
- **IMPORTS**: `testing`, `errors`, `math/rand/v2` (test-side random input generation), the
  `assign` package itself.
- **GOTCHA**: `paralleltest`/`tparallel` require `t.Parallel()` on both outer and inner subtests;
  `testpackage` requires the external `_test` package name.
- **VALIDATE**: `go test ./internal/assign/... -v` all green; `go test ./internal/assign/... -race`
  clean.

### Task 8: Lint and format pass
- **ACTION**: Run `task fmt` then `task lint`, fix anything flagged in the new package.
- **VALIDATE**: `golangci-lint run ./internal/assign/...` exits 0.

---

## Testing Strategy

### Unit Tests

| Test | Input | Expected Output | Edge Case? |
|---|---|---|---|
| Too few members | 0 members | `ErrTooFewMembers` | Yes |
| Too few members | 1 member | `ErrTooFewMembers` | Yes |
| Duplicate member | `[1, 2, 2]` | `ErrDuplicateMember` | Yes |
| Mutually excluded pair | 2 members, `ExclusionPair{1,2}` | `ErrNoValidAssignment` | Yes |
| Over-constrained triple | 3 members, exclusions/history leaving one member with 0 candidates | `ErrNoValidAssignment` | Yes |
| Exact solvable derangement | 3 members, cycle-forcing exclusions | The one valid mapping, exactly | Yes |
| Directional history | `HistoryPair{A,B}` present | A result where B→A is allowed | Yes |
| Unrelated relationship pair | Exclusion pair referencing an id not in `members` | No error; result unaffected for in-set members | Yes |
| Property: random valid groups | ~200-500 random (size, exclusions, history) tuples, size 3-15 | Every success is a valid permutation respecting all constraints | Yes (fuzz-ish) |
| No-hang at scale | 30 members, no constraints | Returns promptly with a valid result | Yes |

### Edge Cases Checklist
- [x] Empty input (0 members)
- [x] Single member (1 — can't derange)
- [x] Maximum stated size input (~30 members)
- [x] Invalid/duplicate ids
- [x] Over-constrained / no-solution input
- [x] Constraint pairs referencing ids outside the member set
- [N/A] Concurrent access — package is a pure function with no shared state; not applicable
- [N/A] Network failure — package has no I/O

---

## Validation Commands

### Static Analysis
```bash
go vet ./internal/assign/...
golangci-lint run ./internal/assign/...
```
EXPECT: Zero issues from either command.

### Unit Tests
```bash
go test ./internal/assign/... -v -race
```
EXPECT: All tests pass, race detector clean.

### Full Test Suite
```bash
task test
```
EXPECT: No regressions elsewhere in the repo.

### Database Validation
N/A — this phase makes no schema or migration changes.

### Browser Validation
N/A — pure backend package, no UI or HTTP surface.

### Manual Validation
- [ ] `go doc ./internal/assign` renders a coherent package summary and the `Assign` signature.
- [ ] Skim the property-style test's failure output format once (e.g. by temporarily breaking the
      solver) to confirm a real violation would produce an actionable test failure message, not
      just a bare `t.Fail()`.

---

## Acceptance Criteria
- [ ] All tasks completed
- [ ] All validation commands pass
- [ ] Tests written and passing, including the randomized property-style test
- [ ] No type errors
- [ ] No lint errors (including the `math/rand/v2` and `nolint:gosec` gotchas above)
- [ ] N/A — no UX to match (internal change)

## Completion Checklist
- [ ] Code follows discovered patterns (sentinel errors, doc comments, external `_test` packages)
- [ ] Error handling matches codebase style (`Err`-prefixed sentinels, package-qualified messages)
- [ ] No logging added — this is a pure library package; logging on `ErrNoValidAssignment` etc. is
      the caller's (Phase 5's) concern
- [ ] Tests follow test patterns (table-driven, `t.Parallel()`, no testify)
- [ ] No hardcoded values — `minMembers`, `maxSearchNodes` etc. are named constants
- [ ] PRD updated: Phase 4 row status and plan link
- [ ] GitHub issue #5 updated with a summary comment linking this plan
- [ ] No unnecessary scope additions (no DB code, no HTTP handlers, no auto-relaxation logic)
- [ ] Self-contained — no questions needed during implementation (the two PRD open questions this
      phase touches were resolved during planning; see Planning Decisions above)

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Backtracking performance degrades on pathological inputs | Low | Medium | MRV heuristic fails fast on infeasible branches; `maxSearchNodes` hang-guard as defense in depth; explicit no-hang regression test at ~30 members |
| `gosec` flags `math/rand/v2` as a weak RNG (G404) | Medium | Low | Documented, specific `//nolint:gosec` with explanation, satisfying this repo's `nolintlint` requirements |
| Future Phase 5 misunderstands the directional-history / no-auto-relax contract | Low | Medium | Both decisions are spelled out in this plan's Planning Decisions section and in `Assign`'s doc comment, so Phase 5's own plan can quote them directly |

## Notes

- This phase intentionally does **not** resolve the PRD's open question on the default repeat-match
  window (N years). Phase 5 (Organiser API) owns translating "last N draws" into concrete
  `HistoryPair` values before calling `Assign` — this package only ever sees already-resolved pairs.
- Per the Decisions Log in the PRD, hand-rolled backtracking (not a general CSP library) is a
  deliberate choice given the stated group sizes (~5-30); if real-world usage ever needs much
  larger groups, that's a signal to revisit, not a reason to add complexity now.
- The three Planning Decisions at the top of this document were obtained by asking the user
  directly during `/ecc:prp-plan`, per their explicit instruction to ask rather than assume — they
  are binding scope boundaries for implementation, not suggestions.
