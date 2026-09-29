package db_test

import (
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

func TestRelationshipQueries_CRUD(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	created, err := store.Queries.CreateRelationship(t.Context(), sqlc.CreateRelationshipParams{
		PhoneNumberA: "+447700900000",
		PhoneNumberB: "+447700900001",
	})
	if err != nil {
		t.Fatalf("CreateRelationship() error = %v, want nil", err)
	}
	if created.PhoneNumberA != "+447700900000" || created.PhoneNumberB != "+447700900001" {
		t.Errorf("CreateRelationship() = %+v, want phone_number_a/_b set", created)
	}

	list, err := store.Queries.ListRelationshipsForPhoneNumber(t.Context(), sqlc.ListRelationshipsForPhoneNumberParams{
		PhoneNumberA: "+447700900000",
		PhoneNumberB: "+447700900000",
	})
	if err != nil {
		t.Fatalf("ListRelationshipsForPhoneNumber() error = %v, want nil", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListRelationshipsForPhoneNumber() len = %d, want 1", len(list))
	}

	if deleteErr := store.Queries.DeleteRelationship(t.Context(), created.ID); deleteErr != nil {
		t.Fatalf("DeleteRelationship() error = %v, want nil", deleteErr)
	}

	listAfterDelete, err := store.Queries.ListRelationshipsForPhoneNumber(
		t.Context(),
		sqlc.ListRelationshipsForPhoneNumberParams{
			PhoneNumberA: "+447700900000",
			PhoneNumberB: "+447700900000",
		},
	)
	if err != nil {
		t.Fatalf("ListRelationshipsForPhoneNumber() after delete error = %v, want nil", err)
	}
	if len(listAfterDelete) != 0 {
		t.Fatalf("ListRelationshipsForPhoneNumber() after delete len = %d, want 0", len(listAfterDelete))
	}
}

func TestCreateRelationship_RejectsUnsortedPair(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	_, err := store.Queries.CreateRelationship(t.Context(), sqlc.CreateRelationshipParams{
		PhoneNumberA: "+447700900001",
		PhoneNumberB: "+447700900000",
	})
	if err == nil {
		t.Fatal("CreateRelationship() with unsorted pair error = nil, want non-nil")
	}
}
