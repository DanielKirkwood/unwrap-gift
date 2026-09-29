package db_test

import (
	"database/sql"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

func TestWishlistItemQueries_CRUD(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	created, err := store.Queries.CreateWishlistItem(t.Context(), sqlc.CreateWishlistItemParams{
		PhoneNumber: "+447700900000",
		ItemName:    "Board game",
		Size:        sql.NullString{String: "Large", Valid: true},
		Url:         sql.NullString{String: "https://example.com/board-game", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateWishlistItem() error = %v, want nil", err)
	}
	if created.ItemName != "Board game" {
		t.Errorf("CreateWishlistItem() item_name = %q, want Board game", created.ItemName)
	}

	withoutOptional, err := store.Queries.CreateWishlistItem(t.Context(), sqlc.CreateWishlistItemParams{
		PhoneNumber: "+447700900000",
		ItemName:    "Socks",
	})
	if err != nil {
		t.Fatalf("CreateWishlistItem() (no size/url) error = %v, want nil", err)
	}
	if withoutOptional.Size.Valid || withoutOptional.Url.Valid {
		t.Errorf("CreateWishlistItem() (no size/url) = %+v, want Size/Url both Valid=false", withoutOptional)
	}

	list, err := store.Queries.ListWishlistItemsByPhoneNumber(t.Context(), "+447700900000")
	if err != nil {
		t.Fatalf("ListWishlistItemsByPhoneNumber() error = %v, want nil", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListWishlistItemsByPhoneNumber() len = %d, want 2", len(list))
	}

	updated, err := store.Queries.UpdateWishlistItem(t.Context(), sqlc.UpdateWishlistItemParams{
		ID:       created.ID,
		ItemName: "Board game (deluxe)",
		Size:     sql.NullString{String: "Extra Large", Valid: true},
		Url:      created.Url,
	})
	if err != nil {
		t.Fatalf("UpdateWishlistItem() error = %v, want nil", err)
	}
	if updated.ItemName != "Board game (deluxe)" {
		t.Errorf("UpdateWishlistItem() item_name = %q, want Board game (deluxe)", updated.ItemName)
	}

	if deleteErr := store.Queries.DeleteWishlistItem(t.Context(), created.ID); deleteErr != nil {
		t.Fatalf("DeleteWishlistItem() error = %v, want nil", deleteErr)
	}

	listAfterDelete, err := store.Queries.ListWishlistItemsByPhoneNumber(t.Context(), "+447700900000")
	if err != nil {
		t.Fatalf("ListWishlistItemsByPhoneNumber() after delete error = %v, want nil", err)
	}
	if len(listAfterDelete) != 1 {
		t.Fatalf("ListWishlistItemsByPhoneNumber() after delete len = %d, want 1", len(listAfterDelete))
	}
}
