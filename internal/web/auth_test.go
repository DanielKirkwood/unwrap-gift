package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kratos "github.com/ory/kratos-client-go/v26"

	"github.com/DanielKirkwood/unwrap-gift/internal/web"
)

// fakeSessionValidator is a minimal web.SessionValidator for testing
// AuthenticationMiddleware without a real Kratos instance.
type fakeSessionValidator struct {
	identity *kratos.Identity
	err      error
}

func (f fakeSessionValidator) ToSession(context.Context, string) (*kratos.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &kratos.Session{Identity: f.identity}, nil
}

func TestAuthenticationMiddleware_NilValidator(t *testing.T) {
	t.Parallel()

	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })

	handler := web.AuthenticationMiddleware(nil)(next)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/wishlist", nil))

	if !called {
		t.Error("next was not called with a nil validator")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (default recorder status, no redirect written)", rec.Code)
	}
}

func TestAuthenticationMiddleware_NoCookie_RedirectsToLogin(t *testing.T) {
	t.Parallel()

	validator := fakeSessionValidator{identity: &kratos.Identity{Id: "identity-1"}}
	handler := web.AuthenticationMiddleware(validator)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("next should not be called without a session")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/wishlist", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login?return_to=") {
		t.Errorf("Location = %q, want prefix /login?return_to=", loc)
	}
}

func TestAuthenticationMiddleware_InvalidSession_RedirectsToLogin(t *testing.T) {
	t.Parallel()

	validator := fakeSessionValidator{err: context.DeadlineExceeded}
	handler := web.AuthenticationMiddleware(validator)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("next should not be called with an invalid session")
	}))

	req := httptest.NewRequest(http.MethodGet, "/wishlist", nil)
	req.Header.Set("Cookie", "ory_kratos_session=invalid")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rec.Code)
	}
}

func TestAuthenticationMiddleware_ValidSession_SetsIdentityInContext(t *testing.T) {
	t.Parallel()

	identity := &kratos.Identity{Id: "identity-1", Traits: map[string]any{"phone": "+15550001234"}}
	validator := fakeSessionValidator{identity: identity}

	var gotIdentity *kratos.Identity
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotIdentity, _ = web.IdentityFromContext(r.Context())
	})
	handler := web.AuthenticationMiddleware(validator)(next)

	req := httptest.NewRequest(http.MethodGet, "/wishlist", nil)
	req.Header.Set("Cookie", "ory_kratos_session=valid")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if gotIdentity == nil || gotIdentity.Id != "identity-1" {
		t.Errorf("IdentityFromContext = %+v, want identity-1", gotIdentity)
	}
}
