package api_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	kratos "github.com/ory/kratos-client-go/v26"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

var errFakeWishlistItemNotFound = errors.New("fake: wishlist item not found")

// fakeWishlistItemStore is a minimal api.WishlistItemStore for testing the
// wishlist item handlers without a real database.
type fakeWishlistItemStore struct {
	item api.WishlistItem
	list []api.WishlistItem
	err  error

	lastPhoneNumber string
	lastID          int64
	lastItemName    string
	lastSize        *string
	lastURL         *string
}

func (f *fakeWishlistItemStore) CreateWishlistItem(
	_ context.Context,
	phoneNumber, itemName string,
	size, url *string,
) (api.WishlistItem, error) {
	f.lastPhoneNumber, f.lastItemName, f.lastSize, f.lastURL = phoneNumber, itemName, size, url
	return f.item, f.err
}

func (f *fakeWishlistItemStore) ListWishlistItems(_ context.Context, phoneNumber string) ([]api.WishlistItem, error) {
	f.lastPhoneNumber = phoneNumber
	return f.list, f.err
}

func (f *fakeWishlistItemStore) UpdateWishlistItem(
	_ context.Context,
	phoneNumber string,
	id int64,
	itemName string,
	size, url *string,
) (api.WishlistItem, error) {
	f.lastPhoneNumber, f.lastID, f.lastItemName, f.lastSize, f.lastURL = phoneNumber, id, itemName, size, url
	return f.item, f.err
}

func (f *fakeWishlistItemStore) DeleteWishlistItem(_ context.Context, phoneNumber string, id int64) error {
	f.lastPhoneNumber, f.lastID = phoneNumber, id
	return f.err
}

// fakeTraitedValidator is a SessionValidator that always succeeds with a
// fixed identity carrying the given traits — the wishlist-item-specific
// counterpart to drawers_test.go's fakeAuthedValidator, which only sets
// Id. Kept separate (not added to the shared fakeAuthedValidator) so this
// resource's tests don't couple to Drawers' test fixture.
type fakeTraitedValidator struct{ traits map[string]any }

func (f fakeTraitedValidator) ToSession(context.Context, string) (*kratos.Session, error) {
	return &kratos.Session{Identity: &kratos.Identity{Id: "identity-1", Traits: f.traits}}, nil
}

func testWishlistItemAdapter() api.Adapter {
	return api.Adapter{
		Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		ErrorsMap: api.ErrorsMap{
			{Match: errFakeWishlistItemNotFound, Problem: api.Problem{Status: http.StatusNotFound, Title: "Not Found"}},
		},
	}
}

// mountTestWishlistItems mounts the wishlist item routes behind a real
// AuthenticationMiddleware backed by validator, so IdentityFromContext
// resolves inside the handlers exactly as it would in production. A
// request must carry a non-empty Cookie header for the middleware to call
// the validator at all.
func mountTestWishlistItems(items api.WishlistItemStore, validator api.SessionValidator) http.Handler {
	r := chi.NewRouter()
	r.Use(api.AuthenticationMiddleware(validator))
	api.MountWishlistItems(r, items, testWishlistItemAdapter())
	return r
}

func newAuthedWishlistItemRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Cookie", "ory_kratos_session=test")
	return req
}

func TestMountWishlistItems_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"create", http.MethodPost, "/wishlist-items", `{"item_name":"Board game"}`, http.StatusCreated},
		{"list", http.MethodGet, "/wishlist-items", "", http.StatusOK},
		{"update", http.MethodPut, "/wishlist-items/1", `{"item_name":"Board game (deluxe)"}`, http.StatusOK},
		{"delete", http.MethodDelete, "/wishlist-items/1", "", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			item := api.WishlistItem{
				ID:          1,
				PhoneNumber: "+447700900000",
				ItemName:    "Board game",
				CreatedAt:   time.Now(),
			}
			fake := &fakeWishlistItemStore{item: item, list: []api.WishlistItem{item}}
			validator := fakeTraitedValidator{traits: map[string]any{"phone": "+447700900000"}}
			router := mountTestWishlistItems(fake, validator)

			req := newAuthedWishlistItemRequest(tt.method, tt.path, tt.body)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if fake.lastPhoneNumber != "+447700900000" {
				t.Errorf(
					"lastPhoneNumber = %q, want +447700900000 (derived from identity, not client-supplied)",
					fake.lastPhoneNumber,
				)
			}
		})
	}
}

// TestMountWishlistItems_CreateNeverAcceptsPhoneNumberFromBody proves the
// owner is always derived from the caller's session, never the request
// body — mirroring createDrawer's organiser_kratos_identity_id guarantee.
func TestMountWishlistItems_CreateNeverAcceptsPhoneNumberFromBody(t *testing.T) {
	t.Parallel()

	fake := &fakeWishlistItemStore{item: api.WishlistItem{ID: 1}}
	validator := fakeTraitedValidator{traits: map[string]any{"phone": "+447700900000"}}
	router := mountTestWishlistItems(fake, validator)

	req := newAuthedWishlistItemRequest(
		http.MethodPost, "/wishlist-items",
		`{"item_name":"Board game","phone_number":"+447700900099"}`,
	)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	if fake.lastPhoneNumber != "+447700900000" {
		t.Errorf("lastPhoneNumber = %q, want +447700900000 (the session's phone, not the body's)", fake.lastPhoneNumber)
	}
}

func TestMountWishlistItems_NoCookieReturns401(t *testing.T) {
	t.Parallel()

	fake := &fakeWishlistItemStore{}
	validator := fakeTraitedValidator{traits: map[string]any{"phone": "+447700900000"}}
	router := mountTestWishlistItems(fake, validator)

	req := httptest.NewRequest(http.MethodGet, "/wishlist-items", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestMountWishlistItems_MissingPhoneTraitReturns500 covers the defensive
// path: identity.schema.json requires "phone", so a real session should
// never lack it, but a handler reaching phoneTraitFromIdentity with ok=false
// must surface a generic 500 via the adapter's unmapped-error path, not
// panic or silently misbehave.
func TestMountWishlistItems_MissingPhoneTraitReturns500(t *testing.T) {
	t.Parallel()

	fake := &fakeWishlistItemStore{}
	validator := fakeTraitedValidator{traits: map[string]any{}}
	router := mountTestWishlistItems(fake, validator)

	req := newAuthedWishlistItemRequest(http.MethodGet, "/wishlist-items", "")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestMountWishlistItems_StoreNotFoundReturns404 proves a
// ErrWishlistItemNotFound from the store (e.g. an ownership mismatch
// detected in internal/app) surfaces as a plain 404, via the same
// ErrorsMap mechanism every other resource uses.
func TestMountWishlistItems_StoreNotFoundReturns404(t *testing.T) {
	t.Parallel()

	fake := &fakeWishlistItemStore{err: errFakeWishlistItemNotFound}
	validator := fakeTraitedValidator{traits: map[string]any{"phone": "+447700900000"}}
	router := mountTestWishlistItems(fake, validator)

	req := newAuthedWishlistItemRequest(http.MethodPut, "/wishlist-items/1", `{"item_name":"hijacked"}`)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestMountWishlistItems_UpdateInvalidID(t *testing.T) {
	t.Parallel()

	fake := &fakeWishlistItemStore{}
	validator := fakeTraitedValidator{traits: map[string]any{"phone": "+447700900000"}}
	router := mountTestWishlistItems(fake, validator)

	req := newAuthedWishlistItemRequest(http.MethodPut, "/wishlist-items/not-a-number", `{"item_name":"x"}`)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (body: %s)", rec.Code, rec.Body.String())
	}
}
