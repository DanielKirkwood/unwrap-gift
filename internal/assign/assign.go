package assign

import (
	"errors"
	"math/rand/v2"
)

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

// minMembers is the fewest members Assign can ever produce a valid
// mapping for — a single member would have to gift themselves.
const minMembers = 2

// maxSearchNodes bounds a single backtracking attempt purely as a
// hang-guard. For the group sizes this package is designed for (~5-30,
// per the PRD), the most-constrained-variable heuristic in solve should
// never come close to this; it exists so a much larger, pathological
// input fails cleanly instead of hanging.
const maxSearchNodes = 1_000_000

// maxRestarts bounds how many fresh, re-randomized attempts solve makes
// when an attempt is abandoned only because it hit maxSearchNodes —
// which is inconclusive, not proof of infeasibility — before solve
// gives up. A search that exhausts its tree without hitting the node
// cap is conclusive regardless of random order and is never retried.
// Restarting guards the PRD's binding requirement that Assign never
// report a false ErrNoValidAssignment for a genuinely solvable input:
// without it, an unlucky random search order on a heavily-constrained
// (but solvable) input could hit the node cap and be indistinguishable
// from true infeasibility.
const maxRestarts = 5

// Assign computes a random gifter→giftee mapping across members that
// satisfies every exclusion and history constraint, or returns
// ErrNoValidAssignment if no such mapping exists. exclusions is
// symmetric (couples etc. — forbidden in both directions); history is
// directional (a past Gifter→Giftee pairing forbids only that exact
// direction again, not the reverse). Any exclusion or history pair
// referencing a MemberID not present in members is silently ignored,
// since the relationship graph is shared across drawers and may
// reference people outside this one.
func Assign(members []MemberID, exclusions []ExclusionPair, history []HistoryPair) (map[MemberID]MemberID, error) {
	memberSet, err := validate(members)
	if err != nil {
		return nil, err
	}

	forbidden := buildForbidden(memberSet, exclusions, history)

	result, ok := solve(members, forbidden)
	if !ok {
		return nil, ErrNoValidAssignment
	}

	return result, nil
}

// validate checks that members is usable and returns it as a set for
// O(1) membership checks elsewhere.
func validate(members []MemberID) (map[MemberID]struct{}, error) {
	if len(members) < minMembers {
		return nil, ErrTooFewMembers
	}

	memberSet := make(map[MemberID]struct{}, len(members))
	for _, m := range members {
		if _, exists := memberSet[m]; exists {
			return nil, ErrDuplicateMember
		}
		memberSet[m] = struct{}{}
	}

	return memberSet, nil
}

// buildForbidden turns exclusions and history into a gifter -> forbidden
// giftees lookup. Self-assignment is always forbidden. A pair where
// either side is outside memberSet is silently skipped.
func buildForbidden(
	memberSet map[MemberID]struct{},
	exclusions []ExclusionPair,
	history []HistoryPair,
) map[MemberID]map[MemberID]bool {
	forbidden := make(map[MemberID]map[MemberID]bool, len(memberSet))
	for m := range memberSet {
		forbidden[m] = map[MemberID]bool{m: true}
	}

	for _, pair := range exclusions {
		if !inSet(memberSet, pair.A) || !inSet(memberSet, pair.B) {
			continue
		}
		forbidden[pair.A][pair.B] = true
		forbidden[pair.B][pair.A] = true
	}

	for _, pair := range history {
		if !inSet(memberSet, pair.Gifter) || !inSet(memberSet, pair.Giftee) {
			continue
		}
		forbidden[pair.Gifter][pair.Giftee] = true
	}

	return forbidden
}

func inSet(set map[MemberID]struct{}, id MemberID) bool {
	_, ok := set[id]
	return ok
}

// solver holds the mutable state of one backtracking search attempt.
type solver struct {
	forbidden map[MemberID]map[MemberID]bool
	used      map[MemberID]bool
	result    map[MemberID]MemberID
	nodes     int
	// nodeLimitHit is set when this attempt was abandoned because it hit
	// maxSearchNodes, as opposed to exhaustively ruling out every
	// candidate. It tells solve whether the negative result is
	// conclusive or merely inconclusive and worth retrying.
	nodeLimitHit bool
}

// solve runs a randomized, most-constrained-variable-first backtracking
// search for a valid gifter→giftee mapping across members. It is
// hand-rolled rather than built on a general CSP library — see the PRD's
// Decisions Log — since the group sizes this package targets (~5-30) are
// small enough that a narrow, exhaustively-tested function is easier to
// verify than adapting a general solver's API.
//
// A single attempt that hits maxSearchNodes is inconclusive (it did not
// prove no assignment exists, it just gave up), so solve retries up to
// maxRestarts times with a fresh random order before reporting failure.
// An attempt that exhausts its search tree without hitting the node cap
// is conclusive regardless of order and is returned immediately.
func solve(members []MemberID, forbidden map[MemberID]map[MemberID]bool) (map[MemberID]MemberID, bool) {
	for range maxRestarts {
		s := &solver{
			forbidden: forbidden,
			used:      make(map[MemberID]bool, len(members)),
			result:    make(map[MemberID]MemberID, len(members)),
		}

		unassigned := make([]MemberID, len(members))
		copy(unassigned, members)

		if s.backtrack(unassigned) {
			return s.result, true
		}

		if !s.nodeLimitHit {
			return nil, false
		}
	}

	return nil, false
}

// backtrack assigns a giftee to the most constrained gifter in
// unassigned, recurses on the rest, and undoes the assignment to try the
// next candidate if that recursive call fails.
func (s *solver) backtrack(unassigned []MemberID) bool {
	if len(unassigned) == 0 {
		return true
	}

	s.nodes++
	if s.nodes > maxSearchNodes {
		s.nodeLimitHit = true
		return false
	}

	gifter, candidates, rest := s.pickGifter(unassigned)

	//nolint:gosec // not security-sensitive: only shuffles candidate order in the Secret Santa solver
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })

	for _, giftee := range candidates {
		s.result[gifter] = giftee
		s.used[giftee] = true

		if s.backtrack(rest) {
			return true
		}

		delete(s.result, gifter)
		s.used[giftee] = false
	}

	return false
}

// pickGifter returns the unassigned gifter with the fewest remaining
// valid candidates — so infeasible branches fail fast instead of being
// discovered only after exploring everything else first — that
// gifter's candidate list (computed once here rather than rebuilt by
// the caller), and the remaining unassigned gifters with it removed.
// Ties are broken randomly by shuffling unassigned before scanning it.
func (s *solver) pickGifter(unassigned []MemberID) (MemberID, []MemberID, []MemberID) {
	//nolint:gosec // not security-sensitive: only randomizes MRV tie-breaking in the Secret Santa solver
	rand.Shuffle(len(unassigned), func(i, j int) { unassigned[i], unassigned[j] = unassigned[j], unassigned[i] })

	bestIdx := 0
	bestCandidates := s.candidates(unassigned[0])
	for i := 1; i < len(unassigned); i++ {
		if candidates := s.candidates(unassigned[i]); len(candidates) < len(bestCandidates) {
			bestIdx, bestCandidates = i, candidates
		}
	}

	gifter := unassigned[bestIdx]
	rest := make([]MemberID, 0, len(unassigned)-1)
	rest = append(rest, unassigned[:bestIdx]...)
	rest = append(rest, unassigned[bestIdx+1:]...)

	return gifter, bestCandidates, rest
}

// candidates returns the giftees gifter could still be assigned: not
// already used by another gifter, and not forbidden for gifter.
func (s *solver) candidates(gifter MemberID) []MemberID {
	forbidden := s.forbidden[gifter]

	candidates := make([]MemberID, 0, len(s.forbidden))
	for giftee := range s.forbidden {
		if s.used[giftee] || forbidden[giftee] {
			continue
		}
		candidates = append(candidates, giftee)
	}

	return candidates
}
