package app //nolint:testpackage // intentional: needs direct access to unexported storeMembers, see newAppTestStore (draws_test.go)

// This file is intentionally in package app (not app_test): it needs direct
// access to the unexported storeMembers type to exercise CreateMember/
// UpdateMember's E.164 phone-number validation (issue #22) against a real
// temp SQLite *db.Store.

import (
	"errors"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

func createTestDrawer(t *testing.T, store *db.Store) sqlc.Drawer {
	t.Helper()

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	return drawer
}

func TestStoreMembers_CreateInvalidPhoneNumberReturnsError(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	members := storeMembers{store: store}
	drawer := createTestDrawer(t, store)

	for _, phone := range []string{
		"07700900000",       // missing leading +
		"+0123",             // leading digit after + is zero
		"not-a-number",      // not numeric at all
		"+1234567890123456", // 16 digits after + (E.164 caps at 15)
	} {
		_, createErr := members.CreateMember(t.Context(), drawer.ID, "Alice", phone)
		if !errors.Is(createErr, api.ErrMemberInvalidPhoneNumber) {
			t.Errorf("CreateMember(%q) error = %v, want api.ErrMemberInvalidPhoneNumber", phone, createErr)
		}
	}
}

func TestStoreMembers_CreateValidE164PhoneNumberSucceeds(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	members := storeMembers{store: store}
	drawer := createTestDrawer(t, store)

	member, createErr := members.CreateMember(t.Context(), drawer.ID, "Alice", "+447700900000")
	if createErr != nil {
		t.Fatalf("CreateMember() error = %v, want nil", createErr)
	}
	if member.PhoneNumber != "+447700900000" {
		t.Errorf("CreateMember() PhoneNumber = %q, want +447700900000", member.PhoneNumber)
	}
}

func TestStoreMembers_UpdateInvalidPhoneNumberReturnsError(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	members := storeMembers{store: store}
	drawer := createTestDrawer(t, store)

	member, createErr := members.CreateMember(t.Context(), drawer.ID, "Alice", "+447700900000")
	if createErr != nil {
		t.Fatalf("CreateMember() error = %v, want nil", createErr)
	}

	_, updateErr := members.UpdateMember(t.Context(), drawer.ID, member.ID, "Alice", "07700900000")
	if !errors.Is(updateErr, api.ErrMemberInvalidPhoneNumber) {
		t.Errorf("UpdateMember() error = %v, want api.ErrMemberInvalidPhoneNumber", updateErr)
	}
}
