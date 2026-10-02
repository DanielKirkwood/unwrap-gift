package app //nolint:testpackage // intentional: needs direct access to unexported storeDraws, see newAppTestStore below

// This file is intentionally in package app (not app_test): it needs
// direct access to the unexported storeDraws type to exercise RunDraw's
// relax-and-retry orchestration against a real temp SQLite *db.Store — the
// one deviation from the Widgets precedent (no app/widgets_test.go exists)
// the Phase 5 plan calls for, since this is genuinely new logic, not CRUD
// passthrough.

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/seven-io/go-client/sms77api"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
	"github.com/DanielKirkwood/unwrap-gift/internal/assign"
	"github.com/DanielKirkwood/unwrap-gift/internal/clients/smsclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

// newAppTestStore opens a Store against a fresh temp-dir SQLite file,
// migrated and closed automatically at test cleanup — the internal/app
// equivalent of internal/db's newTestStore (unexported there, so not
// reusable directly from this package).
func newAppTestStore(t *testing.T) *db.Store {
	t.Helper()

	cfg := config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "test.db")}

	store, err := db.New(cfg, true, tracenoop.NewTracerProvider(), noop.NewMeterProvider())
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if migrateErr := db.Migrate(t.Context(), store.DB); migrateErr != nil {
		t.Fatalf("Migrate() error = %v, want nil", migrateErr)
	}

	return store
}

// newDisabledSMSClient returns a non-nil, disabled *smsclient.Client —
// RunDraw's notify step logs instead of sending, matching this
// codebase's "SMS is never nil" convention (see
// internal/clients/smsclient/smsclient.go's doc comment on Client).
// Tests that need to assert on the SMS *content* construct their own
// enabled client with a fakeSMSSender instead (see
// TestRunDraw_SendsNotificationSMSWithWishlist).
func newDisabledSMSClient(t *testing.T) *smsclient.Client {
	t.Helper()
	return &smsclient.Client{Logger: slog.New(slog.DiscardHandler)}
}

// fakeSMSSender is the smsclient.Sender fake this plan's notification
// tests substitute via smsclient.Client.Sender. It mirrors
// internal/clients/smsclient/smsclient_test.go's fakeSender, generalized
// to capture every call in a slice (RunDraw sends one SMS per member,
// not just one) — that file's fakeSender can't be reused directly since
// it's unexported in a different test package (smsclient_test).
type fakeSMSSender struct {
	calls   []sms77api.SmsBaseParams
	failFor map[string]string // phone number -> error text
}

//nolint:revive,staticcheck // name must match smsclient.Sender's JsonContext exactly
func (f *fakeSMSSender) JsonContext(_ context.Context, p sms77api.SmsBaseParams) (*sms77api.SmsResponse, error) {
	f.calls = append(f.calls, p)

	if errText, fail := f.failFor[p.To]; fail {
		detail := errText
		return &sms77api.SmsResponse{
			Success:  sms77api.StatusCodeErrorUnknown,
			Messages: []sms77api.SmsResponseMessage{{Recipient: p.To, Success: false, ErrorText: &detail}},
		}, nil
	}

	return &sms77api.SmsResponse{
		Success:  sms77api.StatusCodeSuccess,
		Messages: []sms77api.SmsResponseMessage{{Recipient: p.To, Success: true}},
	}, nil
}

// TestRunDraw_RelaxesHistoryWindowByOne is the single most important
// behavior in this plan: with a 3-member drawer whose two derangements
// (D1: A→B→C→A, D2: A→C→B→A) are the only possible full assignments, a
// history window of 2 (both past draws) forbids every directed pair
// across both derangements — unsolvable. Relaxing to 1 (dropping the
// older draw) forbids only D1's pairs, leaving D2 as the unique solution.
// 3 members are used, not 2, because with only 2 members any single
// historical pairing already blocks the only non-self giftee either
// gifter has — relaxing by one could never flip the outcome for N=2 short
// of discarding history entirely (window 0), which isn't what this test
// is meant to demonstrate.
func TestRunDraw_RelaxesHistoryWindowByOne(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	a := createTestMember(t, store, drawer.ID, "Alice", "+447700900000")
	b := createTestMember(t, store, drawer.ID, "Bob", "+447700900001")
	c := createTestMember(t, store, drawer.ID, "Carol", "+447700900002")

	// older past draw: D2 (A→C, C→B, B→A).
	older := createCompletedDraw(t, store, drawer.ID, time.Date(2024, time.December, 25, 0, 0, 0, 0, time.UTC))
	createTestAssignment(t, store, older.ID, a.ID, c.ID)
	createTestAssignment(t, store, older.ID, c.ID, b.ID)
	createTestAssignment(t, store, older.ID, b.ID, a.ID)

	// most recent past draw: D1 (A→B, B→C, C→A).
	mostRecent := createCompletedDraw(t, store, drawer.ID, time.Date(2025, time.December, 25, 0, 0, 0, 0, time.UTC))
	createTestAssignment(t, store, mostRecent.ID, a.ID, b.ID)
	createTestAssignment(t, store, mostRecent.ID, b.ID, c.ID)
	createTestAssignment(t, store, mostRecent.ID, c.ID, a.ID)

	current, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawer.ID,
		ExchangeDate: time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC),
		BudgetAmount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateDraw() error = %v, want nil", err)
	}

	s := storeDraws{store: store, sms: newDisabledSMSClient(t)}

	draw, assignments, runErr := s.RunDraw(t.Context(), drawer.ID, current.ID)
	if runErr != nil {
		t.Fatalf("RunDraw() error = %v, want nil (should succeed once window relaxes to 1)", runErr)
	}
	if draw.Status != drawStatusNotified {
		t.Errorf("RunDraw() draw.Status = %q, want %q", draw.Status, drawStatusNotified)
	}

	want := map[int64]int64{a.ID: c.ID, c.ID: b.ID, b.ID: a.ID} // D2
	if len(assignments) != len(want) {
		t.Fatalf("len(assignments) = %d, want %d", len(assignments), len(want))
	}
	for _, as := range assignments {
		if want[as.GifterMemberID] != as.GifteeMemberID {
			t.Errorf(
				"assignment gifter %d -> giftee %d, want giftee %d (D2, the only assignment not fully forbidden at window=1)",
				as.GifterMemberID,
				as.GifteeMemberID,
				want[as.GifterMemberID],
			)
		}
	}
}

// TestRunDraw_NeverRelaxesExclusions proves the retry loop's relaxation
// never touches exclusions: with 2 members excluding each other and no
// history at all, every iteration (window 2, 1, 0) hits the same
// exclusion-caused infeasibility, and RunDraw must still fail.
func TestRunDraw_NeverRelaxesExclusions(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	createTestMember(t, store, drawer.ID, "Alice", "+447700900000")
	createTestMember(t, store, drawer.ID, "Bob", "+447700900001")

	if _, relErr := store.Queries.CreateRelationship(t.Context(), sqlc.CreateRelationshipParams{
		PhoneNumberA: "+447700900000",
		PhoneNumberB: "+447700900001",
	}); relErr != nil {
		t.Fatalf("CreateRelationship() error = %v, want nil", relErr)
	}

	draw, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawer.ID,
		ExchangeDate: time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC),
		BudgetAmount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateDraw() error = %v, want nil", err)
	}

	s := storeDraws{store: store, sms: newDisabledSMSClient(t)}

	_, _, runErr := s.RunDraw(t.Context(), drawer.ID, draw.ID)
	if !errors.Is(runErr, api.ErrNoValidAssignment) {
		t.Errorf("RunDraw() error = %v, want api.ErrNoValidAssignment (exclusions must never relax)", runErr)
	}
}

// TestRunDraw_AlreadyRun asserts a second RunDraw call on an already-run
// draw returns api.ErrDrawAlreadyRun rather than re-computing.
func TestRunDraw_AlreadyRun(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	createTestMember(t, store, drawer.ID, "Alice", "+447700900000")
	createTestMember(t, store, drawer.ID, "Bob", "+447700900001")

	draw, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawer.ID,
		ExchangeDate: time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC),
		BudgetAmount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateDraw() error = %v, want nil", err)
	}

	s := storeDraws{store: store, sms: newDisabledSMSClient(t)}

	if _, _, runErr := s.RunDraw(t.Context(), drawer.ID, draw.ID); runErr != nil {
		t.Fatalf("first RunDraw() error = %v, want nil", runErr)
	}

	_, _, runErr := s.RunDraw(t.Context(), drawer.ID, draw.ID)
	if !errors.Is(runErr, api.ErrDrawAlreadyRun) {
		t.Errorf("second RunDraw() error = %v, want api.ErrDrawAlreadyRun", runErr)
	}
}

// TestPersistAssignment_RaceAgainstConcurrentRun simulates the race two
// concurrent RunDraw calls on the same draft draw could hit: both pass
// RunDraw's own draft-status check before either commits, then both reach
// persistAssignment. This calls persistAssignment directly twice for the
// same draw — bypassing RunDraw's outer check the way a real race would —
// to prove the transactional re-check inside persistAssignment reports
// api.ErrDrawAlreadyRun on the second call instead of an unmapped
// UNIQUE-constraint error.
func TestPersistAssignment_RaceAgainstConcurrentRun(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	a := createTestMember(t, store, drawer.ID, "Alice", "+447700900000")
	b := createTestMember(t, store, drawer.ID, "Bob", "+447700900001")

	draw, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawer.ID,
		ExchangeDate: time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC),
		BudgetAmount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateDraw() error = %v, want nil", err)
	}

	s := storeDraws{store: store, sms: newDisabledSMSClient(t)}
	assignResult := map[assign.MemberID]assign.MemberID{
		assign.MemberID(a.ID): assign.MemberID(b.ID),
		assign.MemberID(b.ID): assign.MemberID(a.ID),
	}

	if _, _, firstErr := s.persistAssignment(t.Context(), draw, assignResult); firstErr != nil {
		t.Fatalf("first persistAssignment() error = %v, want nil", firstErr)
	}

	_, _, secondErr := s.persistAssignment(t.Context(), draw, assignResult)
	if !errors.Is(secondErr, api.ErrDrawAlreadyRun) {
		t.Errorf(
			"second persistAssignment() error = %v, want api.ErrDrawAlreadyRun (not an unmapped constraint error)",
			secondErr,
		)
	}
}

func createTestMember(t *testing.T, store *db.Store, drawerID int64, fullName, phoneNumber string) sqlc.Member {
	t.Helper()

	member, err := store.Queries.CreateMember(t.Context(), sqlc.CreateMemberParams{
		DrawerID:    drawerID,
		FullName:    fullName,
		PhoneNumber: phoneNumber,
	})
	if err != nil {
		t.Fatalf("CreateMember() error = %v, want nil", err)
	}

	return member
}

func createCompletedDraw(t *testing.T, store *db.Store, drawerID int64, exchangeDate time.Time) sqlc.Draw {
	t.Helper()

	draw, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawerID,
		ExchangeDate: exchangeDate,
		BudgetAmount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateDraw() error = %v, want nil", err)
	}

	updated, err := store.Queries.UpdateDrawStatus(t.Context(), sqlc.UpdateDrawStatusParams{
		ID:     draw.ID,
		Status: drawStatusAssigned,
	})
	if err != nil {
		t.Fatalf("UpdateDrawStatus() error = %v, want nil", err)
	}

	return updated
}

func createTestAssignment(t *testing.T, store *db.Store, drawID, gifterID, gifteeID int64) {
	t.Helper()

	if _, err := store.Queries.CreateAssignment(t.Context(), sqlc.CreateAssignmentParams{
		DrawID:         drawID,
		GifterMemberID: gifterID,
		GifteeMemberID: gifteeID,
	}); err != nil {
		t.Fatalf("CreateAssignment() error = %v, want nil", err)
	}
}

// TestRunDraw_SendsNotificationSMSWithWishlist proves RunDraw's new
// success path: one SMS per member, mentioning their giftee's name,
// the budget, and (for the member with a wishlist item) that item's
// name and size — and that the draw ends at 'notified', not 'assigned'.
func TestRunDraw_SendsNotificationSMSWithWishlist(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Kirkwood Family Christmas",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	alice := createTestMember(t, store, drawer.ID, "Alice", "+447700900000")
	bob := createTestMember(t, store, drawer.ID, "Bob", "+447700900001")

	if _, itemErr := store.Queries.CreateWishlistItem(t.Context(), sqlc.CreateWishlistItemParams{
		PhoneNumber: bob.PhoneNumber,
		ItemName:    "Lego set",
		Size:        sql.NullString{String: "Large", Valid: true},
	}); itemErr != nil {
		t.Fatalf("CreateWishlistItem() error = %v, want nil", itemErr)
	}

	draw, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawer.ID,
		ExchangeDate: time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC),
		BudgetAmount: 2500,
	})
	if err != nil {
		t.Fatalf("CreateDraw() error = %v, want nil", err)
	}

	fake := &fakeSMSSender{}
	s := storeDraws{
		store: store,
		sms:   &smsclient.Client{Enabled: true, Sender: fake, From: "Santa", Logger: slog.New(slog.DiscardHandler)},
	}

	result, _, runErr := s.RunDraw(t.Context(), drawer.ID, draw.ID)
	if runErr != nil {
		t.Fatalf("RunDraw() error = %v, want nil", runErr)
	}
	if result.Status != drawStatusNotified {
		t.Errorf("RunDraw() draw.Status = %q, want %q", result.Status, drawStatusNotified)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("len(fake.calls) = %d, want 2 (one SMS per member)", len(fake.calls))
	}

	var sentToBob bool
	for _, call := range fake.calls {
		if call.To == bob.PhoneNumber {
			sentToBob = true
		}
		assertNotificationSMS(t, call, alice.PhoneNumber, bob.PhoneNumber)
	}
	if !sentToBob {
		t.Error("no SMS sent to Bob")
	}
}

// assertNotificationSMS checks one fakeSMSSender call against the two
// expected message shapes in TestRunDraw_SendsNotificationSMSWithWishlist's
// 2-member drawer: Alice gifts Bob (who has a wishlist item), Bob gifts
// Alice (who has none). Split out to keep that test's cognitive complexity
// under golangci-lint's gocognit limit.
func assertNotificationSMS(t *testing.T, call sms77api.SmsBaseParams, alicePhone, bobPhone string) {
	t.Helper()

	switch call.To {
	case alicePhone:
		// Alice gifts Bob, so her SMS is about Bob — including his
		// wishlist item, since the SMS reports the giftee's wishlist to
		// the gifter (not the gifter's own).
		if !strings.Contains(call.Text, "Bob") {
			t.Errorf("sms to Alice = %q, want it to mention Bob (her giftee)", call.Text)
		}
		if !strings.Contains(call.Text, "£25.00") {
			t.Errorf("sms to Alice = %q, want it to mention the budget", call.Text)
		}
		if !strings.Contains(call.Text, "Lego set") || !strings.Contains(call.Text, "Large") {
			t.Errorf("sms to Alice = %q, want it to include Bob's wishlist item and size", call.Text)
		}
	case bobPhone:
		// Bob gifts Alice, who has no wishlist items — his SMS should
		// fall back to the "haven't added a wishlist yet" message.
		if !strings.Contains(call.Text, "Alice") {
			t.Errorf("sms to Bob = %q, want it to mention Alice (his giftee)", call.Text)
		}
		if !strings.Contains(call.Text, "haven't added a wishlist yet") {
			t.Errorf("sms to Bob = %q, want the no-wishlist fallback (Alice has no items)", call.Text)
		}
	}
}

// TestRunDraw_NotificationFailure_DrawStaysAssigned proves a send
// failure surfaces as api.ErrNotificationFailed and leaves the draw
// (and its already-persisted assignments) at 'assigned', not
// 'notified' — the "known gap, not silently swallowed" behavior
// documented on finalizeNotifications.
func TestRunDraw_NotificationFailure_DrawStaysAssigned(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	alice := createTestMember(t, store, drawer.ID, "Alice", "+447700900000")
	_ = createTestMember(t, store, drawer.ID, "Bob", "+447700900001")

	draw, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawer.ID,
		ExchangeDate: time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC),
		BudgetAmount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateDraw() error = %v, want nil", err)
	}

	fake := &fakeSMSSender{failFor: map[string]string{alice.PhoneNumber: "invalid recipient"}}
	s := storeDraws{
		store: store,
		sms:   &smsclient.Client{Enabled: true, Sender: fake, From: "Santa", Logger: slog.New(slog.DiscardHandler)},
	}

	_, _, runErr := s.RunDraw(t.Context(), drawer.ID, draw.ID)
	if !errors.Is(runErr, api.ErrNotificationFailed) {
		t.Fatalf("RunDraw() error = %v, want errors.Is(err, api.ErrNotificationFailed)", runErr)
	}

	stored, getErr := store.Queries.GetDraw(t.Context(), draw.ID)
	if getErr != nil {
		t.Fatalf("GetDraw() error = %v, want nil", getErr)
	}
	if stored.Status != drawStatusAssigned {
		t.Errorf(
			"draw.Status = %q, want %q (assignments persist even when notification fails)",
			stored.Status, drawStatusAssigned,
		)
	}

	assignments, listErr := store.Queries.ListAssignmentsByDraw(t.Context(), draw.ID)
	if listErr != nil {
		t.Fatalf("ListAssignmentsByDraw() error = %v, want nil", listErr)
	}
	if len(assignments) != 2 {
		t.Errorf("len(assignments) = %d, want 2 (persisted despite notify failure)", len(assignments))
	}
}

// TestStoreDraws_CreateDrawNormalizesExchangeDateForOrdering exercises the
// exact scenario codebase-review-2026-10-02.md's "Correctness bugs" #2
// (issue #26) describes: two draws submitted with different client UTC
// offsets, chosen so the later UTC instant has an earlier *local* calendar
// day than the earlier UTC instant. Without normalizing to UTC first, the
// DB's raw-TEXT "ORDER BY exchange_date DESC" would return them in the
// wrong order, since each stored string would embed its own offset.
func TestStoreDraws_CreateDrawNormalizesExchangeDateForOrdering(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	drawers := storeDrawers{store: store}
	draws := storeDraws{store: store, sms: newDisabledSMSClient(t)}

	drawer, err := drawers.CreateDrawer(t.Context(), "Office Secret Santa", "organiser-1")
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	// later UTC instant (2026-12-25 04:00 UTC), but expressed in a client
	// offset where the local calendar day is still the 24th.
	later, err := time.Parse(time.RFC3339, "2026-12-24T23:00:00-05:00")
	if err != nil {
		t.Fatalf("time.Parse(later) error = %v, want nil", err)
	}

	// earlier UTC instant (2026-12-24 16:00 UTC), but expressed in a client
	// offset where the local calendar day is already the 25th.
	earlier, err := time.Parse(time.RFC3339, "2026-12-25T01:00:00+09:00")
	if err != nil {
		t.Fatalf("time.Parse(earlier) error = %v, want nil", err)
	}

	if _, createEarlierErr := draws.CreateDraw(t.Context(), drawer.ID, earlier, 2000); createEarlierErr != nil {
		t.Fatalf("CreateDraw(earlier) error = %v, want nil", createEarlierErr)
	}
	if _, createLaterErr := draws.CreateDraw(t.Context(), drawer.ID, later, 2000); createLaterErr != nil {
		t.Fatalf("CreateDraw(later) error = %v, want nil", createLaterErr)
	}

	list, err := draws.ListDrawsByDrawer(t.Context(), drawer.ID)
	if err != nil {
		t.Fatalf("ListDrawsByDrawer() error = %v, want nil", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}

	if !list[0].ExchangeDate.Equal(normalizeExchangeDate(later)) {
		t.Errorf(
			"ListDrawsByDrawer()[0].ExchangeDate = %v, want %v (the later UTC instant, sorted first)",
			list[0].ExchangeDate, normalizeExchangeDate(later),
		)
	}
	if !list[1].ExchangeDate.Equal(normalizeExchangeDate(earlier)) {
		t.Errorf(
			"ListDrawsByDrawer()[1].ExchangeDate = %v, want %v (the earlier UTC instant, sorted second)",
			list[1].ExchangeDate, normalizeExchangeDate(earlier),
		)
	}
}
