package app //nolint:testpackage // intentional: needs direct access to unexported storeWishlistItems

// This file is intentionally in package app (not app_test): it needs
// direct access to the unexported storeWishlistItems type to exercise the
// ownership-check logic (fetch-then-compare phone_number) against a real
// temp SQLite *db.Store — the one deviation from the Widgets precedent (no
// app/widgets_test.go exists) the plan calls for, since this is genuinely
// new logic, not CRUD passthrough. Mirrors draws_test.go's justification
// for the same deviation.

import (
	"errors"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

func TestStoreWishlistItems_OwnershipEnforced(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	items := storeWishlistItems{store: store}

	const ownerPhone = "+447700900000"
	const otherPhone = "+447700900001"

	created, err := items.CreateWishlistItem(t.Context(), ownerPhone, "Board game", nil, nil)
	if err != nil {
		t.Fatalf("CreateWishlistItem() error = %v, want nil", err)
	}

	updated, err := items.UpdateWishlistItem(t.Context(), ownerPhone, created.ID, "Board game (deluxe)", nil, nil)
	if err != nil {
		t.Fatalf("UpdateWishlistItem() (owner) error = %v, want nil", err)
	}
	if updated.ItemName != "Board game (deluxe)" {
		t.Errorf("UpdateWishlistItem() (owner) item_name = %q, want Board game (deluxe)", updated.ItemName)
	}

	_, updateWrongOwnerErr := items.UpdateWishlistItem(t.Context(), otherPhone, created.ID, "hijacked", nil, nil)
	if !errors.Is(updateWrongOwnerErr, api.ErrWishlistItemNotFound) {
		t.Errorf(
			"UpdateWishlistItem() (wrong owner) error = %v, want api.ErrWishlistItemNotFound",
			updateWrongOwnerErr,
		)
	}

	deleteWrongOwnerErr := items.DeleteWishlistItem(t.Context(), otherPhone, created.ID)
	if !errors.Is(deleteWrongOwnerErr, api.ErrWishlistItemNotFound) {
		t.Errorf(
			"DeleteWishlistItem() (wrong owner) error = %v, want api.ErrWishlistItemNotFound",
			deleteWrongOwnerErr,
		)
	}

	otherList, err := items.ListWishlistItems(t.Context(), otherPhone)
	if err != nil {
		t.Fatalf("ListWishlistItems() (other phone) error = %v, want nil", err)
	}
	if len(otherList) != 0 {
		t.Errorf("ListWishlistItems() (other phone) len = %d, want 0", len(otherList))
	}

	ownerList, err := items.ListWishlistItems(t.Context(), ownerPhone)
	if err != nil {
		t.Fatalf("ListWishlistItems() (owner) error = %v, want nil", err)
	}
	if len(ownerList) != 1 {
		t.Fatalf("ListWishlistItems() (owner) len = %d, want 1", len(ownerList))
	}

	deleteOwnerErr := items.DeleteWishlistItem(t.Context(), ownerPhone, created.ID)
	if deleteOwnerErr != nil {
		t.Fatalf("DeleteWishlistItem() (owner) error = %v, want nil", deleteOwnerErr)
	}

	ownerListAfterDelete, err := items.ListWishlistItems(t.Context(), ownerPhone)
	if err != nil {
		t.Fatalf("ListWishlistItems() (owner, after delete) error = %v, want nil", err)
	}
	if len(ownerListAfterDelete) != 0 {
		t.Errorf("ListWishlistItems() (owner, after delete) len = %d, want 0", len(ownerListAfterDelete))
	}
}

// TestStoreWishlistItems_SizeAndURLRoundTrip exercises toNullString/
// fromNullString's "value present" branch — the one genuinely new piece of
// conversion logic this resource introduces (no other resource has a
// nullable string column) — which TestStoreWishlistItems_OwnershipEnforced
// never touches, since it only ever passes nil for size/url.
func TestStoreWishlistItems_SizeAndURLRoundTrip(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	items := storeWishlistItems{store: store}

	size := "Large"
	url := "https://example.com/board-game"

	created, err := items.CreateWishlistItem(t.Context(), "+447700900000", "Board game", &size, &url)
	if err != nil {
		t.Fatalf("CreateWishlistItem() error = %v, want nil", err)
	}
	if created.Size == nil || *created.Size != size {
		t.Errorf("CreateWishlistItem() Size = %v, want %q", created.Size, size)
	}
	if created.URL == nil || *created.URL != url {
		t.Errorf("CreateWishlistItem() URL = %v, want %q", created.URL, url)
	}

	newSize := "Extra Large"
	updated, err := items.UpdateWishlistItem(t.Context(), "+447700900000", created.ID, "Board game", &newSize, nil)
	if err != nil {
		t.Fatalf("UpdateWishlistItem() error = %v, want nil", err)
	}
	if updated.Size == nil || *updated.Size != newSize {
		t.Errorf("UpdateWishlistItem() Size = %v, want %q", updated.Size, newSize)
	}
	if updated.URL != nil {
		t.Errorf("UpdateWishlistItem() URL = %v, want nil (cleared)", updated.URL)
	}
}

func TestStoreWishlistItems_UpdateDeleteNonexistentReturnsNotFound(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	items := storeWishlistItems{store: store}

	_, updateErr := items.UpdateWishlistItem(t.Context(), "+447700900000", 999, "x", nil, nil)
	if !errors.Is(updateErr, api.ErrWishlistItemNotFound) {
		t.Errorf("UpdateWishlistItem() (nonexistent) error = %v, want api.ErrWishlistItemNotFound", updateErr)
	}

	deleteErr := items.DeleteWishlistItem(t.Context(), "+447700900000", 999)
	if !errors.Is(deleteErr, api.ErrWishlistItemNotFound) {
		t.Errorf("DeleteWishlistItem() (nonexistent) error = %v, want api.ErrWishlistItemNotFound", deleteErr)
	}
}
