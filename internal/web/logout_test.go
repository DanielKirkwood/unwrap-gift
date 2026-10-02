package web_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	kratos "github.com/ory/kratos-client-go/v26"

	"github.com/DanielKirkwood/unwrap-gift/internal/web"
)

// fakeLogoutFlowProvider is a minimal web.LogoutFlowProvider for testing the
// logout handler without a real Kratos instance.
type fakeLogoutFlowProvider struct {
	flow *kratos.LogoutFlow
	err  error

	lastCookie, lastReturnTo string
}

func (f *fakeLogoutFlowProvider) CreateBrowserLogoutFlow(
	_ context.Context,
	cookieHeader, returnTo string,
) (*kratos.LogoutFlow, error) {
	f.lastCookie, f.lastReturnTo = cookieHeader, returnTo
	return f.flow, f.err
}

func mountTestLogout(flows web.LogoutFlowProvider) http.Handler {
	r := chi.NewRouter()
	web.MountLogout(r, flows)
	return r
}

func TestMountLogout_ActiveSession_RedirectsToKratosLogoutURL(t *testing.T) {
	t.Parallel()

	fake := &fakeLogoutFlowProvider{
		flow: &kratos.LogoutFlow{LogoutUrl: "http://127.0.0.1:4433/self-service/logout?token=abc"},
	}
	router := mountTestLogout(fake)

	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	req.Header.Set("Cookie", "ory_kratos_session=test")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != fake.flow.LogoutUrl {
		t.Errorf("Location = %q, want %q", loc, fake.flow.LogoutUrl)
	}
	if fake.lastCookie != "ory_kratos_session=test" {
		t.Errorf("lastCookie = %q, want the request's Cookie header forwarded", fake.lastCookie)
	}
	if fake.lastReturnTo != "/login" {
		t.Errorf("lastReturnTo = %q, want /login", fake.lastReturnTo)
	}
}

func TestMountLogout_NoActiveSession_RedirectsToLogin(t *testing.T) {
	t.Parallel()

	fake := &fakeLogoutFlowProvider{err: errors.New("no active session")}
	router := mountTestLogout(fake)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/logout", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}
