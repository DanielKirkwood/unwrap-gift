package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
	"github.com/DanielKirkwood/unwrap-gift/internal/web"
)

// storeWishlistItems adapts *db.Store to api.WishlistItemStore, converting
// between sqlc's generated WishlistItem type and api.WishlistItem.
// UpdateWishlistItem/DeleteWishlistItem both enforce the "does this item
// actually belong to this phone number" check here — the one place in this
// plan the business rule lives, rather than duplicated across every
// handler — mirroring storeMembers' DrawerID check in internal/app/members.go.
type storeWishlistItems struct {
	store *db.Store
}

func (s storeWishlistItems) CreateWishlistItem(
	ctx context.Context,
	phoneNumber, itemName string,
	size, url *string,
) (api.WishlistItem, error) {
	item, err := s.store.Queries.CreateWishlistItem(ctx, sqlc.CreateWishlistItemParams{
		PhoneNumber: phoneNumber,
		ItemName:    itemName,
		Size:        toNullString(size),
		Url:         toNullString(url),
	})
	if err != nil {
		return api.WishlistItem{}, fmt.Errorf("app: create wishlist item: %w", err)
	}

	return toAPIWishlistItem(item), nil
}

func (s storeWishlistItems) ListWishlistItems(ctx context.Context, phoneNumber string) ([]api.WishlistItem, error) {
	rows, err := s.store.Queries.ListWishlistItemsByPhoneNumber(ctx, phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("app: list wishlist items: %w", err)
	}

	out := make([]api.WishlistItem, len(rows))
	for i, row := range rows {
		out[i] = toAPIWishlistItem(row)
	}

	return out, nil
}

func (s storeWishlistItems) UpdateWishlistItem(
	ctx context.Context,
	phoneNumber string,
	id int64,
	itemName string,
	size, url *string,
) (api.WishlistItem, error) {
	existing, err := s.store.Queries.GetWishlistItem(ctx, id)
	if err != nil {
		return api.WishlistItem{}, mapWishlistItemErr(err)
	}
	if existing.PhoneNumber != phoneNumber {
		return api.WishlistItem{}, api.ErrWishlistItemNotFound
	}

	item, err := s.store.Queries.UpdateWishlistItem(ctx, sqlc.UpdateWishlistItemParams{
		ID:       id,
		ItemName: itemName,
		Size:     toNullString(size),
		Url:      toNullString(url),
	})
	if err != nil {
		return api.WishlistItem{}, fmt.Errorf("app: update wishlist item: %w", err)
	}

	return toAPIWishlistItem(item), nil
}

func (s storeWishlistItems) DeleteWishlistItem(ctx context.Context, phoneNumber string, id int64) error {
	existing, err := s.store.Queries.GetWishlistItem(ctx, id)
	if err != nil {
		return mapWishlistItemErr(err)
	}
	if existing.PhoneNumber != phoneNumber {
		return api.ErrWishlistItemNotFound
	}

	if deleteErr := s.store.Queries.DeleteWishlistItem(ctx, id); deleteErr != nil {
		return fmt.Errorf("app: delete wishlist item: %w", deleteErr)
	}

	return nil
}

// webStoreWishlistItems adapts *db.Store to web.WishlistItemStore. A
// separate type from storeWishlistItems (not just a reuse of it) because
// api.WishlistItem and web.WishlistItem are distinct named types declared
// in their own packages (internal/web must not import internal/api, per
// ARCHITECTURE.md's dependency-direction rule) — Go interface satisfaction
// requires an exact method-signature match, including return types, so the
// same struct can't satisfy both WishlistItemStore interfaces no matter how
// structurally identical api.WishlistItem and web.WishlistItem are. The
// underlying queries are identical to storeWishlistItems'; only the
// sqlc→DTO conversion function differs.
type webStoreWishlistItems struct {
	store *db.Store
}

func (s webStoreWishlistItems) CreateWishlistItem(
	ctx context.Context,
	phoneNumber, itemName string,
	size, url *string,
) (web.WishlistItem, error) {
	item, err := s.store.Queries.CreateWishlistItem(ctx, sqlc.CreateWishlistItemParams{
		PhoneNumber: phoneNumber,
		ItemName:    itemName,
		Size:        toNullString(size),
		Url:         toNullString(url),
	})
	if err != nil {
		return web.WishlistItem{}, fmt.Errorf("app: create wishlist item: %w", err)
	}

	return toWebWishlistItem(item), nil
}

func (s webStoreWishlistItems) ListWishlistItems(ctx context.Context, phoneNumber string) ([]web.WishlistItem, error) {
	rows, err := s.store.Queries.ListWishlistItemsByPhoneNumber(ctx, phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("app: list wishlist items: %w", err)
	}

	out := make([]web.WishlistItem, len(rows))
	for i, row := range rows {
		out[i] = toWebWishlistItem(row)
	}

	return out, nil
}

func (s webStoreWishlistItems) UpdateWishlistItem(
	ctx context.Context,
	phoneNumber string,
	id int64,
	itemName string,
	size, url *string,
) (web.WishlistItem, error) {
	existing, err := s.store.Queries.GetWishlistItem(ctx, id)
	if err != nil {
		return web.WishlistItem{}, mapWebWishlistItemErr(err)
	}
	if existing.PhoneNumber != phoneNumber {
		return web.WishlistItem{}, web.ErrWishlistItemNotFound
	}

	item, err := s.store.Queries.UpdateWishlistItem(ctx, sqlc.UpdateWishlistItemParams{
		ID:       id,
		ItemName: itemName,
		Size:     toNullString(size),
		Url:      toNullString(url),
	})
	if err != nil {
		return web.WishlistItem{}, fmt.Errorf("app: update wishlist item: %w", err)
	}

	return toWebWishlistItem(item), nil
}

func (s webStoreWishlistItems) DeleteWishlistItem(ctx context.Context, phoneNumber string, id int64) error {
	existing, err := s.store.Queries.GetWishlistItem(ctx, id)
	if err != nil {
		return mapWebWishlistItemErr(err)
	}
	if existing.PhoneNumber != phoneNumber {
		return web.ErrWishlistItemNotFound
	}

	if deleteErr := s.store.Queries.DeleteWishlistItem(ctx, id); deleteErr != nil {
		return fmt.Errorf("app: delete wishlist item: %w", deleteErr)
	}

	return nil
}

func toWebWishlistItem(i sqlc.WishlistItem) web.WishlistItem {
	return web.WishlistItem{
		ID:        i.ID,
		ItemName:  i.ItemName,
		Size:      fromNullString(i.Size),
		URL:       fromNullString(i.Url),
		CreatedAt: i.CreatedAt,
		UpdatedAt: i.UpdatedAt,
	}
}

func toAPIWishlistItem(i sqlc.WishlistItem) api.WishlistItem {
	return api.WishlistItem{
		ID:          i.ID,
		PhoneNumber: i.PhoneNumber,
		ItemName:    i.ItemName,
		Size:        fromNullString(i.Size),
		URL:         fromNullString(i.Url),
		CreatedAt:   i.CreatedAt,
		UpdatedAt:   i.UpdatedAt,
	}
}

// mapWishlistItemErr translates [sql.ErrNoRows] into api.ErrWishlistItemNotFound.
func mapWishlistItemErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return api.ErrWishlistItemNotFound
	}

	return fmt.Errorf("app: wishlist item store: %w", err)
}

// mapWebWishlistItemErr is mapWishlistItemErr's web-facing counterpart,
// translating [sql.ErrNoRows] into web.ErrWishlistItemNotFound instead of
// api's sentinel — kept separate so webStoreWishlistItems never returns an
// internal/api error value across the internal/web dependency-direction
// boundary.
func mapWebWishlistItemErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return web.ErrWishlistItemNotFound
	}

	return fmt.Errorf("app: wishlist item store: %w", err)
}

// toNullString and fromNullString convert between api's *string
// (nil-is-unset) and sqlc's [sql.NullString] — wishlist_items is the only
// resource in this codebase with a nullable string column, so there's no
// existing shared helper for this; kept local rather than factored out
// pre-need.
func toNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}

	return sql.NullString{String: *s, Valid: true}
}

func fromNullString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}

	return &ns.String
}
