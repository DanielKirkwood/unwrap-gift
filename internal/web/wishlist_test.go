package web_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	kratos "github.com/ory/kratos-client-go/v26"

	"github.com/DanielKirkwood/unwrap-gift/internal/web"
)

// fakeWishlistItemStore is a minimal web.WishlistItemStore for testing the
// wishlist handlers without a real database — mirrors
// internal/api/wishlist_items_test.go's fakeWishlistItemStore.
type fakeWishlistItemStore struct {
	item web.WishlistItem
	list []web.WishlistItem
	err  error

	lastPhoneNumber string
	lastID          int64
	lastItemName    string
}

func (f *fakeWishlistItemStore) CreateWishlistItem(
	_ context.Context,
	phoneNumber, itemName string,
	_, _ *string,
) (web.WishlistItem, error) {
	f.lastPhoneNumber, f.lastItemName = phoneNumber, itemName
	return f.item, f.err
}

func (f *fakeWishlistItemStore) ListWishlistItems(_ context.Context, phoneNumber string) ([]web.WishlistItem, error) {
	f.lastPhoneNumber = phoneNumber
	return f.list, f.err
}

func (f *fakeWishlistItemStore) UpdateWishlistItem(
	_ context.Context,
	phoneNumber string,
	id int64,
	itemName string,
	_, _ *string,
) (web.WishlistItem, error) {
	f.lastPhoneNumber, f.lastID, f.lastItemName = phoneNumber, id, itemName
	return f.item, f.err
}

func (f *fakeWishlistItemStore) DeleteWishlistItem(_ context.Context, phoneNumber string, id int64) error {
	f.lastPhoneNumber, f.lastID = phoneNumber, id
	return f.err
}

func testAdapter(t *testing.T) web.Adapter {
	t.Helper()
	return web.Adapter{Logger: testLogger(), Templates: mustParseTemplates(t)}
}

func mountTestWishlist(items web.WishlistItemStore, validator web.SessionValidator, t *testing.T) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	r.Use(web.AuthenticationMiddleware(validator))
	web.MountWishlist(r, items, mustParseTemplates(t), testAdapter(t))
	return r
}

func authedWishlistRequest(method, path string, form url.Values) *http.Request {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Cookie", "ory_kratos_session=test")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req
}

func testValidator() fakeSessionValidator {
	return fakeSessionValidator{identity: &kratos.Identity{
		Id:     "identity-1",
		Traits: map[string]any{"phone": testPhoneNumber, "full_name": testFullName},
	}}
}

func TestMountWishlist_Show_RendersGreetingAndItems(t *testing.T) {
	t.Parallel()

	fake := &fakeWishlistItemStore{list: []web.WishlistItem{{ID: 1, ItemName: "Board game"}}}
	router := mountTestWishlist(fake, testValidator(), t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authedWishlistRequest(http.MethodGet, "/wishlist", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Dan") {
		t.Error("expected greeting to contain the identity's full_name")
	}
	if !strings.Contains(body, "Board game") {
		t.Error("expected the item name to appear in the rendered list")
	}
}

func TestMountWishlist_Create_Success(t *testing.T) {
	t.Parallel()

	item := web.WishlistItem{ID: 1, ItemName: "Board game"}
	fake := &fakeWishlistItemStore{item: item, list: []web.WishlistItem{item}}
	router := mountTestWishlist(fake, testValidator(), t)

	form := url.Values{"item_name": {"Board game"}}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authedWishlistRequest(http.MethodPost, "/wishlist", form))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Board game") {
		t.Error("expected the new item in the returned item_list partial")
	}
	if fake.lastPhoneNumber != testPhoneNumber {
		t.Errorf("lastPhoneNumber = %q, want %q (derived from identity)", fake.lastPhoneNumber, testPhoneNumber)
	}
}

func TestMountWishlist_Create_BlankName_DoesNotCallStore(t *testing.T) {
	t.Parallel()

	fake := &fakeWishlistItemStore{}
	router := mountTestWishlist(fake, testValidator(), t)

	form := url.Values{"item_name": {"   "}}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authedWishlistRequest(http.MethodPost, "/wishlist", form))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (error rendered in-band, not a 4xx)", rec.Code)
	}
	if fake.lastItemName != "" {
		t.Errorf("store should not have been called for a blank name, lastItemName = %q", fake.lastItemName)
	}
	if !strings.Contains(rec.Body.String(), "enter an item name") {
		t.Error("expected a validation message in the response body")
	}
}

func TestMountWishlist_Update_StoreError_RendersInBandAt200(t *testing.T) {
	t.Parallel()

	fake := &fakeWishlistItemStore{err: errors.New("not owned")}
	router := mountTestWishlist(fake, testValidator(), t)

	form := url.Values{"item_name": {"hijacked"}}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authedWishlistRequest(http.MethodPut, "/wishlist/1", form))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (store error rendered in-band)", rec.Code)
	}
	// html/template escapes the apostrophe (Couldn&#39;t) — correct
	// auto-escaping behavior, so the assertion avoids it.
	if !strings.Contains(rec.Body.String(), "save that change") {
		t.Errorf("expected an error callout in the response body, got: %s", rec.Body.String())
	}
}

func TestMountWishlist_Delete_Success(t *testing.T) {
	t.Parallel()

	fake := &fakeWishlistItemStore{}
	router := mountTestWishlist(fake, testValidator(), t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authedWishlistRequest(http.MethodDelete, "/wishlist/1", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if fake.lastID != 1 {
		t.Errorf("lastID = %d, want 1", fake.lastID)
	}
}

func TestMountWishlist_NoSession_RedirectsToLogin(t *testing.T) {
	t.Parallel()

	fake := &fakeWishlistItemStore{}
	router := mountTestWishlist(fake, testValidator(), t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/wishlist", nil))

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 (no session cookie)", rec.Code)
	}
}
