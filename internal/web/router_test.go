package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/web"
)

func TestParseTemplates_NoError(t *testing.T) {
	t.Parallel()

	if _, err := web.ParseTemplates(); err != nil {
		t.Fatalf("ParseTemplates() error = %v, want nil", err)
	}
}

func TestNewWebRouter_RootRedirectsToWishlist(t *testing.T) {
	t.Parallel()

	router := web.NewWebRouter(web.RouterDeps{Logger: testLogger()})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/wishlist" {
		t.Errorf("Location = %q, want /wishlist", loc)
	}
}

func TestNewWebRouter_FeatureDisabled_LoginAndWishlistNotMounted(t *testing.T) {
	t.Parallel()

	router := web.NewWebRouter(web.RouterDeps{Logger: testLogger()})

	for _, path := range []string{"/login", "/wishlist"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404 (web feature disabled, route never mounted)", path, rec.Code)
		}
	}
}

// TestNewWebRouter_LoginNotBehindAuth proves /login is reachable (not stuck
// behind AuthenticationMiddleware) even with Auth set and no session
// present — if it were mounted inside the Auth group, this request would
// 303 back to /login itself, an infinite loop from the browser's
// perspective. Following at most one redirect (to Kratos, not back to
// /login) confirms the invariant.
func TestNewWebRouter_LoginNotBehindAuth(t *testing.T) {
	t.Parallel()

	router := web.NewWebRouter(web.RouterDeps{
		Logger:           testLogger(),
		Auth:             web.AuthenticationMiddleware(fakeSessionValidator{err: errAlwaysFail}),
		LoginFlows:       fakeLoginFlowProvider{},
		KratosBrowserURL: testKratosBrowserURL,
		Templates:        mustParseTemplates(t),
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc == "/login" || loc == "" {
		t.Fatalf("Location = %q, want a redirect to Kratos, not back to /login (would loop)", loc)
	}
}
