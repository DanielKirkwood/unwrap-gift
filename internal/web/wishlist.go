package web

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	kratos "github.com/ory/kratos-client-go/v26"
)

// ErrWishlistItemNotFound is returned by WishlistItemStore implementations
// when the requested item doesn't exist or doesn't belong to the caller —
// the web-package-local counterpart to api.ErrWishlistItemNotFound. A
// separate sentinel (rather than reusing api's) keeps internal/app's
// webStoreWishlistItems adapter from leaking an internal/api type across
// the dependency-direction boundary ARCHITECTURE.md documents: internal/web
// must not depend on internal/api, including transitively through error
// values a caller might want to check with [errors.Is].
var ErrWishlistItemNotFound = errors.New("web: wishlist item not found")

// HandlerFunc is like [http.HandlerFunc] but returns an error instead of
// writing an error response directly. Adapter.Adapt turns one into a real
// [http.HandlerFunc]. Mirrors internal/api's HandlerFunc/Adapter shape, with
// an HTML error fragment in place of a Problem JSON body — reserved for
// genuinely unexpected errors (e.g. a template execution failure); store
// errors like "item not found" are rendered in-band at 200 by the handlers
// below instead of returned here.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Adapter turns HandlerFunc values into [http.HandlerFunc], rendering any
// returned error as error.html at 500 instead of letting it propagate
// unhandled.
type Adapter struct {
	Logger    *slog.Logger
	Templates *template.Template
}

// Adapt wraps fn, rendering fn's returned error (if any) as error.html
// instead of letting it propagate unhandled.
func (a Adapter) Adapt(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			a.Logger.ErrorContext(r.Context(), "web: handler error", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = a.Templates.ExecuteTemplate(w, "error.html", map[string]string{"Message": "Something went wrong."})
		}
	}
}

// WishlistItem is web's own representation of a wishlist_items row,
// decoupled from api.WishlistItem/sqlc.WishlistItem — web never imports
// internal/api or internal/db, per ARCHITECTURE.md's dependency-direction
// rule. Consumed by templates, not marshaled, so it carries no struct tags.
type WishlistItem struct {
	ID        int64
	ItemName  string
	Size      *string
	URL       *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WishlistItemStore is the subset of persistence the wishlist handlers
// need — the same method set as api.WishlistItemStore, separately declared
// here since web must not import internal/api. Every method takes
// phoneNumber explicitly, derived from the caller's Kratos identity.
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

// wishlistPageData is wishlist.html's template data.
type wishlistPageData struct {
	FullName string
	Items    []WishlistItem
	Error    string
}

// itemListData is item_list.html's template data — the htmx swap-target
// partial, also reused (with Error set) to show a validation/store error
// in-band at 200 rather than via Adapter's 500 path (htmx does not swap
// content on non-2xx responses by default).
type itemListData struct {
	Items []WishlistItem
	Error string
}

// itemEditData is item_edit_row.html's template data: the item being
// edited (WishlistItem is embedded so templates access .ID/.ItemName/etc.
// directly) plus an optional Error, used to re-render the same edit form
// in-band (Decision #6) when a PUT fails validation or the store, instead
// of discarding the user's in-progress edit by swapping in the full list.
type itemEditData struct {
	WishlistItem

	Error string
}

// MountWishlist adds the participant wishlist page and its htmx-driven CRUD
// endpoints to r, under /wishlist. There is no single-item GET, mirroring
// the JSON API's Phase 6 decision — GET /wishlist/{id}/edit renders an
// inline edit form instead, by filtering the caller's own list in Go.
func MountWishlist(r chi.Router, items WishlistItemStore, templates *template.Template, adapter Adapter) {
	r.Get("/wishlist", adapter.Adapt(showWishlist(items, templates)))
	r.Post("/wishlist", adapter.Adapt(createWishlistItem(items, templates)))
	r.Get("/wishlist/{id}/edit", adapter.Adapt(editWishlistItem(items, templates)))
	r.Put("/wishlist/{id}", adapter.Adapt(updateWishlistItem(items, templates)))
	r.Delete("/wishlist/{id}", adapter.Adapt(deleteWishlistItem(items)))
}

// phoneTraitFromIdentity extracts the "phone" trait from identity.Traits.
// Reimplemented locally (internal/api's copy is unexported and web must not
// import internal/api) — see internal/api/wishlist_items.go for the
// identical logic and its rationale.
func phoneTraitFromIdentity(identity *kratos.Identity) (string, bool) {
	traits, ok := identity.Traits.(map[string]any)
	if !ok {
		return "", false
	}

	phone, ok := traits["phone"].(string)
	return phone, ok
}

// fullNameTraitFromIdentity extracts the "full_name" trait for the
// wishlist page's greeting. Falls back to "" (template renders a generic
// greeting) rather than failing — a missing display name isn't worth a 500.
func fullNameTraitFromIdentity(identity *kratos.Identity) string {
	traits, ok := identity.Traits.(map[string]any)
	if !ok {
		return ""
	}

	name, _ := traits["full_name"].(string)
	return name
}

func showWishlist(items WishlistItemStore, templates *template.Template) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			return errMissingIdentity
		}
		phoneNumber, ok := phoneTraitFromIdentity(identity)
		if !ok {
			return errMissingPhoneTrait
		}

		list, err := items.ListWishlistItems(r.Context(), phoneNumber)
		if err != nil {
			return err
		}

		data := wishlistPageData{FullName: fullNameTraitFromIdentity(identity), Items: list}
		return templates.ExecuteTemplate(w, "wishlist.html", data)
	}
}

func createWishlistItem(items WishlistItemStore, templates *template.Template) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		phoneNumber, err := identityAndPhone(r)
		if err != nil {
			return err
		}

		if parseErr := r.ParseForm(); parseErr != nil {
			return parseErr
		}
		itemName := strings.TrimSpace(r.FormValue("item_name"))
		if itemName == "" {
			return renderItemList(r.Context(), w, templates, items, phoneNumber, "Please enter an item name.")
		}

		size, url := formFieldPtr(r, "size"), formFieldPtr(r, "url")

		_, createErr := items.CreateWishlistItem(r.Context(), phoneNumber, itemName, size, url)
		if createErr != nil {
			return renderItemList(
				r.Context(),
				w,
				templates,
				items,
				phoneNumber,
				"Couldn't add that item — please try again.",
			)
		}

		return renderItemList(r.Context(), w, templates, items, phoneNumber, "")
	}
}

func editWishlistItem(items WishlistItemStore, templates *template.Template) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		phoneNumber, err := identityAndPhone(r)
		if err != nil {
			return err
		}

		id, err := wishlistItemID(r)
		if err != nil {
			return err
		}

		list, err := items.ListWishlistItems(r.Context(), phoneNumber)
		if err != nil {
			return err
		}

		item, ok := findWishlistItem(list, id)
		if !ok {
			// Deleted elsewhere since the page loaded (rare, low-concurrency
			// app) — ask the browser to reload rather than swap in a
			// mismatched fragment shape.
			w.Header().Set("Hx-Refresh", "true")
			return nil
		}

		return templates.ExecuteTemplate(w, "item_edit_row.html", itemEditData{WishlistItem: item})
	}
}

func updateWishlistItem(items WishlistItemStore, templates *template.Template) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		phoneNumber, err := identityAndPhone(r)
		if err != nil {
			return err
		}

		id, err := wishlistItemID(r)
		if err != nil {
			return err
		}
		if parseErr := r.ParseForm(); parseErr != nil {
			return parseErr
		}

		itemName := strings.TrimSpace(r.FormValue("item_name"))
		size, url := formFieldPtr(r, "size"), formFieldPtr(r, "url")
		attempted := itemEditData{WishlistItem{ID: id, ItemName: itemName, Size: size, URL: url}, ""}

		if itemName == "" {
			attempted.Error = "Please enter an item name."
			return templates.ExecuteTemplate(w, "item_edit_row.html", attempted)
		}

		item, updateErr := items.UpdateWishlistItem(r.Context(), phoneNumber, id, itemName, size, url)
		if updateErr != nil {
			attempted.Error = "Couldn't save that change — please try again."
			return templates.ExecuteTemplate(w, "item_edit_row.html", attempted)
		}

		return templates.ExecuteTemplate(w, "item_row.html", item)
	}
}

func deleteWishlistItem(items WishlistItemStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		phoneNumber, err := identityAndPhone(r)
		if err != nil {
			return err
		}

		id, err := wishlistItemID(r)
		if err != nil {
			return err
		}

		// Ownership mismatches/not-found are treated the same as success —
		// the row is gone from the caller's perspective either way, and
		// htmx's hx-swap="delete" removes it client-side regardless of body.
		_ = items.DeleteWishlistItem(r.Context(), phoneNumber, id)

		w.WriteHeader(http.StatusOK)
		return nil
	}
}

// identityAndPhone resolves the caller's phone trait from its Kratos
// identity in ctx — the first step every mutating handler above needs. It
// returns only the phone (not the identity itself) since no caller needs
// anything else from it.
func identityAndPhone(r *http.Request) (string, error) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		return "", errMissingIdentity
	}
	phoneNumber, ok := phoneTraitFromIdentity(identity)
	if !ok {
		return "", errMissingPhoneTrait
	}

	return phoneNumber, nil
}

// renderItemList re-fetches the caller's list and renders item_list.html —
// the htmx swap-target partial for every mutating endpoint's success path.
func renderItemList(
	ctx context.Context,
	w http.ResponseWriter,
	templates *template.Template,
	items WishlistItemStore,
	phoneNumber, errMsg string,
) error {
	list, err := items.ListWishlistItems(ctx, phoneNumber)
	if err != nil {
		return err
	}

	return templates.ExecuteTemplate(w, "item_list.html", itemListData{Items: list, Error: errMsg})
}

func findWishlistItem(list []WishlistItem, id int64) (WishlistItem, bool) {
	for _, item := range list {
		if item.ID == id {
			return item, true
		}
	}

	return WishlistItem{}, false
}

// formFieldPtr returns a *string for name's form value, or nil if blank —
// matches api's wishlistItemRequest's nil-is-unset convention for the
// optional size/url fields.
func formFieldPtr(r *http.Request, name string) *string {
	v := strings.TrimSpace(r.FormValue(name))
	if v == "" {
		return nil
	}

	return &v
}

func wishlistItemID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}
