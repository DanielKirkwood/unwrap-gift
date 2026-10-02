package db_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

func setupDrawWithTwoMembers(t *testing.T, store *db.Store) (sqlc.Draw, sqlc.Member, sqlc.Member) {
	t.Helper()

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	memberA, err := store.Queries.CreateMember(t.Context(), sqlc.CreateMemberParams{
		DrawerID:    drawer.ID,
		FullName:    "Alice",
		PhoneNumber: "+447700900000",
	})
	if err != nil {
		t.Fatalf("CreateMember() error = %v, want nil", err)
	}

	memberB, err := store.Queries.CreateMember(t.Context(), sqlc.CreateMemberParams{
		DrawerID:    drawer.ID,
		FullName:    "Bob",
		PhoneNumber: "+447700900001",
	})
	if err != nil {
		t.Fatalf("CreateMember() error = %v, want nil", err)
	}

	draw, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawer.ID,
		ExchangeDate: time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC),
		BudgetAmount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateDraw() error = %v, want nil", err)
	}

	return draw, memberA, memberB
}

func TestAssignmentQueries_CRUD(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	draw, memberA, memberB := setupDrawWithTwoMembers(t, store)

	created, err := store.Queries.CreateAssignment(t.Context(), sqlc.CreateAssignmentParams{
		DrawID:            draw.ID,
		GifterMemberID:    sql.NullInt64{Int64: memberA.ID, Valid: true},
		GifteeMemberID:    sql.NullInt64{Int64: memberB.ID, Valid: true},
		GifterPhoneNumber: memberA.PhoneNumber,
		GifteePhoneNumber: memberB.PhoneNumber,
	})
	if err != nil {
		t.Fatalf("CreateAssignment() error = %v, want nil", err)
	}
	if created.GifterMemberID.Int64 != memberA.ID || created.GifteeMemberID.Int64 != memberB.ID {
		t.Errorf("CreateAssignment() = %+v, want gifter=%d giftee=%d", created, memberA.ID, memberB.ID)
	}
	if created.GifterPhoneNumber != memberA.PhoneNumber || created.GifteePhoneNumber != memberB.PhoneNumber {
		t.Errorf(
			"CreateAssignment() phone snapshot = %q/%q, want %q/%q",
			created.GifterPhoneNumber, created.GifteePhoneNumber, memberA.PhoneNumber, memberB.PhoneNumber,
		)
	}

	list, err := store.Queries.ListAssignmentsByDraw(t.Context(), draw.ID)
	if err != nil {
		t.Fatalf("ListAssignmentsByDraw() error = %v, want nil", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListAssignmentsByDraw() len = %d, want 1", len(list))
	}
}

func TestCreateAssignment_RejectsSelfAssignment(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	draw, memberA, _ := setupDrawWithTwoMembers(t, store)

	_, err := store.Queries.CreateAssignment(t.Context(), sqlc.CreateAssignmentParams{
		DrawID:            draw.ID,
		GifterMemberID:    sql.NullInt64{Int64: memberA.ID, Valid: true},
		GifteeMemberID:    sql.NullInt64{Int64: memberA.ID, Valid: true},
		GifterPhoneNumber: memberA.PhoneNumber,
		GifteePhoneNumber: memberA.PhoneNumber,
	})
	if err == nil {
		t.Fatal("CreateAssignment() with gifter == giftee error = nil, want non-nil")
	}
}

func TestCreateAssignment_RejectsDuplicateGifterInSameDraw(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	draw, memberA, memberB := setupDrawWithTwoMembers(t, store)

	if _, err := store.Queries.CreateAssignment(t.Context(), sqlc.CreateAssignmentParams{
		DrawID:            draw.ID,
		GifterMemberID:    sql.NullInt64{Int64: memberA.ID, Valid: true},
		GifteeMemberID:    sql.NullInt64{Int64: memberB.ID, Valid: true},
		GifterPhoneNumber: memberA.PhoneNumber,
		GifteePhoneNumber: memberB.PhoneNumber,
	}); err != nil {
		t.Fatalf("CreateAssignment() first error = %v, want nil", err)
	}

	_, err := store.Queries.CreateAssignment(t.Context(), sqlc.CreateAssignmentParams{
		DrawID:            draw.ID,
		GifterMemberID:    sql.NullInt64{Int64: memberA.ID, Valid: true},
		GifteeMemberID:    sql.NullInt64{Int64: memberB.ID, Valid: true},
		GifterPhoneNumber: memberA.PhoneNumber,
		GifteePhoneNumber: memberB.PhoneNumber,
	})
	if err == nil {
		t.Fatal("CreateAssignment() duplicate gifter in same draw error = nil, want non-nil")
	}
}

func TestCreateAssignment_RejectsDuplicateGifteeInSameDraw(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	draw, memberA, memberB := setupDrawWithTwoMembers(t, store)

	memberC, err := store.Queries.CreateMember(t.Context(), sqlc.CreateMemberParams{
		DrawerID:    draw.DrawerID,
		FullName:    "Carol",
		PhoneNumber: "+447700900002",
	})
	if err != nil {
		t.Fatalf("CreateMember() error = %v, want nil", err)
	}

	if _, firstErr := store.Queries.CreateAssignment(t.Context(), sqlc.CreateAssignmentParams{
		DrawID:            draw.ID,
		GifterMemberID:    sql.NullInt64{Int64: memberA.ID, Valid: true},
		GifteeMemberID:    sql.NullInt64{Int64: memberB.ID, Valid: true},
		GifterPhoneNumber: memberA.PhoneNumber,
		GifteePhoneNumber: memberB.PhoneNumber,
	}); firstErr != nil {
		t.Fatalf("CreateAssignment() first error = %v, want nil", firstErr)
	}

	_, err = store.Queries.CreateAssignment(t.Context(), sqlc.CreateAssignmentParams{
		DrawID:            draw.ID,
		GifterMemberID:    sql.NullInt64{Int64: memberC.ID, Valid: true},
		GifteeMemberID:    sql.NullInt64{Int64: memberB.ID, Valid: true},
		GifterPhoneNumber: memberC.PhoneNumber,
		GifteePhoneNumber: memberB.PhoneNumber,
	})
	if err == nil {
		t.Fatal("CreateAssignment() duplicate giftee in same draw error = nil, want non-nil")
	}
}
