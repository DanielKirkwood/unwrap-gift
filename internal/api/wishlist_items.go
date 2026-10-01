package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	kratos "github.com/ory/kratos-client-go/v26"
)

// ErrWishlistItemNotFound is returned by WishlistItemStore implementations
// when no wishlist item exists for the given id, or it exists but belongs
// to a different phone number than the caller's — the same error either
// way, mirroring MemberStore's "wrong drawer" vs. "doesn't exist" shape in
// internal/app/members.go, so a caller can't distinguish the two cases via
// error-shape.
var ErrWishlistItemNotFound = errors.New("api: wishlist item not found")

// errMissingPhoneTrait is returned when an authenticated identity lacks a
// "phone" trait — an invariant violation given identity.schema.json
// requires it, mapped to a generic 500 by the adapter's unmapped-error
// path rather than a 4xx. Unexported: this is an internal defensive check,
// not a condition any caller should match against.
var errMissingPhoneTrait = errors.New("api: identity missing phone trait")

// WishlistItem is api's own representation of a wishlist_items row,
// decoupled from internal/db/sqlc.WishlistItem — api never imports
// internal/db, per ARCHITECTURE.md's dependency-direction rule. Size and
// URL are optional (nil when unset), unlike every other DTO in this
// package, which has no nullable string columns.
type WishlistItem struct {
	ID          int64     `json:"id"`
	PhoneNumber string    `json:"phone_number"`
	ItemName    string    `json:"item_name"`
	Size        *string   `json:"size,omitempty"`
	URL         *string   `json:"url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WishlistItemStore is the subset of persistence the wishlist item handlers
// need, implemented by an adapter app constructs over *db.Store and injects
// via RouterDeps. Every method takes phoneNumber explicitly, derived by the
// handler from the caller's own Kratos identity (never from the request
// body or a URL param) — there is no parent resource to nest under, unlike
// MemberStore's drawerID.
type WishlistItemStore interface {
	CreateWishlistItem(ctx context.Context, phoneNumber, itemName string, size, url *string) (WishlistItem, error)
	ListWishlistItems(ctx context.Context, phoneNumber string) ([]WishlistItem, error)
	UpdateWishlistItem(
		ctx context.Context,
		phoneNumber string,
		id int64,
		itemName string,
		size, url *string,
	) (WishlistItem, error)
	DeleteWishlistItem(ctx context.Context, phoneNumber string, id int64) error
}

// wishlistItemRequest is the request body for both POST and PUT wishlist
// item endpoints. It deliberately has no phone_number field — the owner is
// always derived from the caller's session, never client-supplied.
type wishlistItemRequest struct {
	ItemName string  `json:"item_name"`
	Size     *string `json:"size"`
	URL      *string `json:"url"`
}

// MountWishlistItems adds the participant wishlist CRUD endpoints to r,
// under /wishlist-items. There is no single-item GET — see the plan's
// Decisions Confirmed With User, #2.
func MountWishlistItems(r chi.Router, items WishlistItemStore, adapter Adapter) {
	r.Post("/wishlist-items", adapter.Adapt(createWishlistItem(items)))
	r.Get("/wishlist-items", adapter.Adapt(listWishlistItems(items)))
	r.Put("/wishlist-items/{id}", adapter.Adapt(updateWishlistItem(items)))
	r.Delete("/wishlist-items/{id}", adapter.Adapt(deleteWishlistItem(items)))
}

// phoneTraitFromIdentity extracts the "phone" trait from identity.Traits.
// kratos-client-go declares Traits as interface{} (see model_identity.go);
// after JSON decoding it is always map[string]any for this schema. ok is
// false only if the trait is missing or not a string, which shouldn't
// happen given identity.schema.json requires "phone" — callers treat that
// as an invariant violation (mapped to a generic 500), not a 4xx.
func phoneTraitFromIdentity(identity *kratos.Identity) (string, bool) {
	traits, ok := identity.Traits.(map[string]any)
	if !ok {
		return "", false
	}

	phone, ok := traits["phone"].(string)
	return phone, ok
}

func createWishlistItem(items WishlistItemStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			return WriteProblem(w, unauthorizedProblem())
		}
		phoneNumber, ok := phoneTraitFromIdentity(identity)
		if !ok {
			return errMissingPhoneTrait
		}

		var req wishlistItemRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			return fmt.Errorf("api: decode create wishlist item request: %w", decodeErr)
		}

		item, err := items.CreateWishlistItem(r.Context(), phoneNumber, req.ItemName, req.Size, req.URL)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusCreated, item)
	}
}

func listWishlistItems(items WishlistItemStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			return WriteProblem(w, unauthorizedProblem())
		}
		phoneNumber, ok := phoneTraitFromIdentity(identity)
		if !ok {
			return errMissingPhoneTrait
		}

		list, err := items.ListWishlistItems(r.Context(), phoneNumber)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, list)
	}
}

func updateWishlistItem(items WishlistItemStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			return WriteProblem(w, unauthorizedProblem())
		}
		phoneNumber, ok := phoneTraitFromIdentity(identity)
		if !ok {
			return errMissingPhoneTrait
		}

		id, err := wishlistItemID(r)
		if err != nil {
			return err
		}

		var req wishlistItemRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			return fmt.Errorf("api: decode update wishlist item request: %w", decodeErr)
		}

		item, err := items.UpdateWishlistItem(r.Context(), phoneNumber, id, req.ItemName, req.Size, req.URL)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, item)
	}
}

func deleteWishlistItem(items WishlistItemStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			return WriteProblem(w, unauthorizedProblem())
		}
		phoneNumber, ok := phoneTraitFromIdentity(identity)
		if !ok {
			return errMissingPhoneTrait
		}

		id, err := wishlistItemID(r)
		if err != nil {
			return err
		}

		if deleteErr := items.DeleteWishlistItem(r.Context(), phoneNumber, id); deleteErr != nil {
			return deleteErr
		}

		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

func wishlistItemID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("api: parse wishlist item id: %w", err)
	}

	return id, nil
}
