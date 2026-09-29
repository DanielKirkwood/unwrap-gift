package db_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

func TestMemberQueries_CRUD(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	created, err := store.Queries.CreateMember(t.Context(), sqlc.CreateMemberParams{
		DrawerID:    drawer.ID,
		FullName:    "Alice",
		PhoneNumber: "+447700900000",
	})
	if err != nil {
		t.Fatalf("CreateMember() error = %v, want nil", err)
	}
	if created.FullName != "Alice" {
		t.Errorf("CreateMember() full_name = %q, want Alice", created.FullName)
	}

	got, err := store.Queries.GetMember(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetMember() error = %v, want nil", err)
	}
	if got != created {
		t.Errorf("GetMember() = %+v, want %+v", got, created)
	}

	updated, err := store.Queries.UpdateMember(t.Context(), sqlc.UpdateMemberParams{
		ID:          created.ID,
		FullName:    "Alicia",
		PhoneNumber: "+447700900001",
	})
	if err != nil {
		t.Fatalf("UpdateMember() error = %v, want nil", err)
	}
	if updated.FullName != "Alicia" {
		t.Errorf("UpdateMember() full_name = %q, want Alicia", updated.FullName)
	}

	byDrawer, err := store.Queries.ListMembersByDrawer(t.Context(), drawer.ID)
	if err != nil {
		t.Fatalf("ListMembersByDrawer() error = %v, want nil", err)
	}
	if len(byDrawer) != 1 {
		t.Fatalf("ListMembersByDrawer() len = %d, want 1", len(byDrawer))
	}

	byPhone, err := store.Queries.ListMembersByPhoneNumber(t.Context(), "+447700900001")
	if err != nil {
		t.Fatalf("ListMembersByPhoneNumber() error = %v, want nil", err)
	}
	if len(byPhone) != 1 {
		t.Fatalf("ListMembersByPhoneNumber() len = %d, want 1", len(byPhone))
	}

	if deleteErr := store.Queries.DeleteMember(t.Context(), created.ID); deleteErr != nil {
		t.Fatalf("DeleteMember() error = %v, want nil", deleteErr)
	}

	if _, getErr := store.Queries.GetMember(t.Context(), created.ID); !errors.Is(getErr, sql.ErrNoRows) {
		t.Errorf("GetMember() after delete error = %v, want sql.ErrNoRows", getErr)
	}
}

func TestCreateMember_FKViolation(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	_, err := store.Queries.CreateMember(t.Context(), sqlc.CreateMemberParams{
		DrawerID:    999999,
		FullName:    "Ghost",
		PhoneNumber: "+447700900002",
	})
	if err == nil {
		t.Fatal("CreateMember() with nonexistent drawer_id error = nil, want non-nil")
	}
}

func TestDeleteDrawer_CascadesMembers(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	member, err := store.Queries.CreateMember(t.Context(), sqlc.CreateMemberParams{
		DrawerID:    drawer.ID,
		FullName:    "Alice",
		PhoneNumber: "+447700900003",
	})
	if err != nil {
		t.Fatalf("CreateMember() error = %v, want nil", err)
	}

	if deleteErr := store.Queries.DeleteDrawer(t.Context(), drawer.ID); deleteErr != nil {
		t.Fatalf("DeleteDrawer() error = %v, want nil", deleteErr)
	}

	if _, getErr := store.Queries.GetMember(t.Context(), member.ID); !errors.Is(getErr, sql.ErrNoRows) {
		t.Errorf("GetMember() after drawer delete error = %v, want sql.ErrNoRows", getErr)
	}
}
