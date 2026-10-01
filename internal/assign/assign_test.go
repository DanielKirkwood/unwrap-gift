package assign_test

import (
	"errors"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/DanielKirkwood/unwrap-gift/internal/assign"
)

func TestAssign_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		members []assign.MemberID
		wantErr error
	}{
		{"no members", nil, assign.ErrTooFewMembers},
		{"one member", []assign.MemberID{1}, assign.ErrTooFewMembers},
		{"duplicate member", []assign.MemberID{1, 2, 2}, assign.ErrDuplicateMember},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := assign.Assign(tt.members, nil, nil)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Assign() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAssign_MutuallyExcludedPairHasNoSolution(t *testing.T) {
	t.Parallel()

	members := []assign.MemberID{1, 2}
	exclusions := []assign.ExclusionPair{{A: 1, B: 2}}

	_, err := assign.Assign(members, exclusions, nil)
	if !errors.Is(err, assign.ErrNoValidAssignment) {
		t.Errorf("Assign() error = %v, want ErrNoValidAssignment", err)
	}
}

func TestAssign_OverConstrainedTripleHasNoSolution(t *testing.T) {
	t.Parallel()

	// Member 1's only possible giftees are 2 and 3; excluding 1<->2 and
	// forbidding history 1->3 leaves member 1 with zero candidates, so no
	// assignment can ever succeed regardless of how 2 and 3 are paired.
	members := []assign.MemberID{1, 2, 3}
	exclusions := []assign.ExclusionPair{{A: 1, B: 2}}
	history := []assign.HistoryPair{{Gifter: 1, Giftee: 3}}

	_, err := assign.Assign(members, exclusions, history)
	if !errors.Is(err, assign.ErrNoValidAssignment) {
		t.Errorf("Assign() error = %v, want ErrNoValidAssignment", err)
	}
}

func TestAssign_ExactSolvableDerangement(t *testing.T) {
	t.Parallel()

	// A 3-member group has exactly two derangements: {1:2,2:3,3:1} and
	// {1:3,3:2,2:1}. Forbidding every directional edge of the second
	// cycle via history leaves only the first as a valid result.
	members := []assign.MemberID{1, 2, 3}
	history := []assign.HistoryPair{
		{Gifter: 1, Giftee: 3},
		{Gifter: 3, Giftee: 2},
		{Gifter: 2, Giftee: 1},
	}

	got, err := assign.Assign(members, nil, history)
	if err != nil {
		t.Fatalf("Assign() error = %v, want nil", err)
	}

	want := map[assign.MemberID]assign.MemberID{1: 2, 2: 3, 3: 1}
	assertExactMapping(t, got, want)
}

func TestAssign_HistoryIsDirectionalOnly(t *testing.T) {
	t.Parallel()

	// Forbidding only 1->2 (not 2->1) eliminates the derangement
	// {1:2,2:3,3:1} but leaves {1:3,3:2,2:1} — which uses 2->1 — valid.
	// A bidirectional interpretation would also have had to forbid
	// 2->1, which would leave no solution at all.
	members := []assign.MemberID{1, 2, 3}
	history := []assign.HistoryPair{{Gifter: 1, Giftee: 2}}

	got, err := assign.Assign(members, nil, history)
	if err != nil {
		t.Fatalf("Assign() error = %v, want nil", err)
	}

	want := map[assign.MemberID]assign.MemberID{1: 3, 3: 2, 2: 1}
	assertExactMapping(t, got, want)
}

func TestAssign_IgnoresUnrelatedRelationshipPairs(t *testing.T) {
	t.Parallel()

	members := []assign.MemberID{1, 2}
	// 99 and 100 aren't in members — must be a silent no-op, not an error.
	exclusions := []assign.ExclusionPair{{A: 99, B: 100}}
	history := []assign.HistoryPair{{Gifter: 100, Giftee: 99}}

	got, err := assign.Assign(members, exclusions, history)
	if err != nil {
		t.Fatalf("Assign() error = %v, want nil", err)
	}

	want := map[assign.MemberID]assign.MemberID{1: 2, 2: 1}
	assertExactMapping(t, got, want)
}

func TestAssign_PropertiesHoldAcrossRandomInputs(t *testing.T) {
	t.Parallel()

	const iterations = 300

	for i := range iterations {
		members, exclusions, history := randomInput()

		got, err := assign.Assign(members, exclusions, history)
		if err != nil {
			if !errors.Is(err, assign.ErrNoValidAssignment) {
				t.Fatalf("iteration %d: Assign() error = %v, want nil or ErrNoValidAssignment", i, err)
			}
			continue
		}

		assertValidAssignment(t, members, exclusions, history, got)
	}
}

func TestAssign_NoHangAtUpperBoundSize(t *testing.T) {
	t.Parallel()

	const (
		size    = 30
		timeout = 5 * time.Second
	)

	members := make([]assign.MemberID, size)
	for i := range members {
		members[i] = assign.MemberID(i + 1)
	}

	var err error
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err = assign.Assign(members, nil, nil)
	}()

	select {
	case <-done:
		if err != nil {
			t.Errorf("Assign() error = %v, want nil", err)
		}
	case <-time.After(timeout):
		t.Fatalf("Assign() did not return within %s for %d unconstrained members", timeout, size)
	}
}

// assertExactMapping fails the test unless got is exactly want.
func assertExactMapping(t *testing.T, got, want map[assign.MemberID]assign.MemberID) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("Assign() = %v, want %v", got, want)
	}
	for gifter, giftee := range want {
		if got[gifter] != giftee {
			t.Errorf("Assign()[%v] = %v, want %v", gifter, got[gifter], giftee)
		}
	}
}

// assertValidAssignment fails the test unless got is a valid permutation
// of members with no self-assignment and no violated constraint.
func assertValidAssignment(
	t *testing.T,
	members []assign.MemberID,
	exclusions []assign.ExclusionPair,
	history []assign.HistoryPair,
	got map[assign.MemberID]assign.MemberID,
) {
	t.Helper()

	if len(got) != len(members) {
		t.Fatalf("Assign() produced %d pairings, want %d", len(got), len(members))
	}

	giftees := make(map[assign.MemberID]bool, len(members))
	for gifter, giftee := range got {
		if gifter == giftee {
			t.Errorf("Assign() self-assigned member %v", gifter)
		}
		if giftees[giftee] {
			t.Errorf("Assign() assigned giftee %v more than once", giftee)
		}
		giftees[giftee] = true
	}

	for _, pair := range exclusions {
		if got[pair.A] == pair.B || got[pair.B] == pair.A {
			t.Errorf("Assign() violated exclusion pair %+v", pair)
		}
	}

	for _, pair := range history {
		if got[pair.Gifter] == pair.Giftee {
			t.Errorf("Assign() repeated history pair %+v", pair)
		}
	}
}

// randomInput generates a random, usually-solvable group: 3-15 members,
// 0-2 random exclusion pairs, 0-2 random history pairs. Some generated
// pairs deliberately reference ids outside the member set, to also
// exercise Assign's "unrelated pair" ignore path.
func randomInput() ([]assign.MemberID, []assign.ExclusionPair, []assign.HistoryPair) {
	const (
		minSize  = 3
		maxSize  = 15
		maxExtra = 2
	)

	size := minSize + rand.IntN(maxSize-minSize+1)
	members := make([]assign.MemberID, size)
	for i := range members {
		members[i] = assign.MemberID(i + 1)
	}

	exclusions := make([]assign.ExclusionPair, 0, maxExtra)
	for range rand.IntN(maxExtra + 1) {
		exclusions = append(exclusions, assign.ExclusionPair{A: randomID(size), B: randomID(size)})
	}

	history := make([]assign.HistoryPair, 0, maxExtra)
	for range rand.IntN(maxExtra + 1) {
		history = append(history, assign.HistoryPair{Gifter: randomID(size), Giftee: randomID(size)})
	}

	return members, exclusions, history
}

// randomID returns a MemberID usually inside [1, size], occasionally
// deliberately out of range.
func randomID(size int) assign.MemberID {
	const outOfRangeChance = 5 // 1-in-5 chance of an out-of-range id

	if rand.IntN(outOfRangeChance) == 0 {
		return assign.MemberID(size + 100)
	}

	return assign.MemberID(1 + rand.IntN(size))
}
