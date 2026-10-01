# Implementation Report: Assignment Engine

## Summary

Implemented `internal/assign`, a pure, I/O-free package that computes a randomized, valid
gifter→giftee mapping for a Secret Santa drawer given a member list, symmetric relationship
exclusions, and directional history pairs from prior draws. Uses a hand-rolled, most-constrained-
variable-first backtracking solver with randomized candidate order (`math/rand/v2`), returning
`ErrNoValidAssignment` when no mapping satisfies every constraint.

## Assessment vs Reality

| Metric | Predicted (Plan) | Actual |
|---|---|---|
| Complexity | Medium | Medium — matched |
| Confidence | 8/10 | Implemented exactly as planned, no replanning needed |
| Files Changed | 3 new, 1 updated | 3 new, 1 updated (PRD) — matched |

## Tasks Completed

| # | Task | Status | Notes |
|---|---|---|---|
| 1 | Package doc comment | Complete | |
| 2 | Core types and sentinel errors | Complete | |
| 3 | Input validation | Complete | |
| 4 | Forbidden-candidate index | Complete | |
| 5 | Backtracking solver core | Complete | Deviated — see below |
| 6 | Public `Assign` entry point | Complete | |
| 7 | Unit and property-style tests | Complete | |
| 8 | Lint and format pass | Complete | |

## Validation Results

| Level | Status | Notes |
|---|---|---|
| Static Analysis | Pass | `go vet ./internal/assign/...` and `golangci-lint run` (both package-scoped and full-repo) clean |
| Unit Tests | Pass | 8 top-level tests (3 subtests under one), including a 300-iteration property-style test |
| Build | Pass | `go build ./...` succeeds |
| Integration | N/A | Pure library package, no HTTP surface |
| Edge Cases | Pass | 0/1 members, duplicate ids, mutual exclusion, over-constrained triple, directional history, unrelated relationship pairs, ~30-member no-hang guard |

## Files Changed

| File | Action | Lines |
|---|---|---|
| `internal/assign/doc.go` | CREATED | +7 |
| `internal/assign/assign.go` | CREATED | +239 |
| `internal/assign/assign_test.go` | CREATED | +266 |
| `.claude/PRPs/prds/secret-santa-drawer.prd.md` | UPDATED | Phase 4 row: status + plan link |

## Deviations from Plan

- **What**: Added two `//nolint:gosec` directives in `assign.go` (on the two `rand.Shuffle` calls),
  each with a specific explanation, as the plan's Task 5 Gotcha anticipated.
- **Why**: `gosec`'s G404 rule flagged `math/rand/v2` usage as a weak-RNG finding even though the
  randomness here is for draw variety, not security. This was explicitly predicted in the plan
  (Task 5, Gotcha 2) and the fix matches the plan's prescribed approach exactly — no new judgment
  call was needed.

No other deviations — implementation followed the plan's types, function signatures, algorithm
design, and test list as written.

## Issues Encountered

None beyond the anticipated `gosec` finding above.

## Tests Written

| Test File | Tests | Coverage |
|---|---|---|
| `internal/assign/assign_test.go` | 8 top-level (10 incl. subtests) | Validation errors, mutual exclusion, over-constrained no-solution, exact derangement correctness, directional-history semantics, unrelated-pair ignore path, 300-iteration randomized property test (permutation validity, no self-assignment, no exclusion/history violation), no-hang guard at the PRD's stated upper bound (30 members) |

## Next Steps
- [ ] Code review via `/code-review`
- [ ] Create PR via `/prp-pr`
- [ ] `/prp-plan` for Phase 5 (Organiser API), which will call `assign.Assign` and resolve the
      PRD's still-open "last N draws" window question before building `HistoryPair` inputs
