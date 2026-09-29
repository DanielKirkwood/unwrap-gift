package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	kratos "github.com/ory/kratos-client-go/v26"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

// fakePermissionChecker is a minimal api.PermissionChecker for testing
// AuthorizationMiddleware without a real Keto instance.
type fakePermissionChecker struct {
	allowed bool
	err     error
	calls   int
}

func (f *fakePermissionChecker) CheckPermission(_ context.Context, _, _, _, _ string) (bool, error) {
	f.calls++
	return f.allowed, f.err
}

func TestAuthorizationMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		checker      *fakePermissionChecker
		withIdentity bool
		wantStatus   int
		wantCalled   bool
		wantCalls    int
	}{
		{
			name:         "no checker (keto disabled) passes through",
			checker:      nil,
			withIdentity: false,
			wantStatus:   http.StatusOK,
			wantCalled:   true,
		},
		{
			name:         "no identity in context",
			checker:      &fakePermissionChecker{allowed: true},
			withIdentity: false,
			wantStatus:   http.StatusUnauthorized,
			wantCalled:   false,
			wantCalls:    0,
		},
		{
			name:         "checker error",
			checker:      &fakePermissionChecker{err: errors.New("boom")},
			withIdentity: true,
			wantStatus:   http.StatusForbidden,
			wantCalled:   false,
			wantCalls:    1,
		},
		{
			name:         "checker denies",
			checker:      &fakePermissionChecker{allowed: false},
			withIdentity: true,
			wantStatus:   http.StatusForbidden,
			wantCalled:   false,
			wantCalls:    1,
		},
		{
			name:         "checker allows",
			checker:      &fakePermissionChecker{allowed: true},
			withIdentity: true,
			wantStatus:   http.StatusOK,
			wantCalled:   true,
			wantCalls:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runAuthzMiddlewareCase(t, tt.checker, tt.withIdentity, tt.wantStatus, tt.wantCalled, tt.wantCalls)
		})
	}
}

func runAuthzMiddlewareCase(
	t *testing.T,
	fake *fakePermissionChecker,
	withIdentity bool,
	wantStatus int,
	wantCalled bool,
	wantCalls int,
) {
	t.Helper()

	var checker api.PermissionChecker
	if fake != nil {
		checker = fake
	}

	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	handler := api.AuthorizationMiddleware(checker, "Identities", "admin", "manage")(next)

	// withIdentity simulates AuthenticationMiddleware having already run —
	// AuthorizationMiddleware only ever runs after it in the real router,
	// so this composes the two exactly as NewHiddenRouter does, rather than
	// reaching into api's unexported context-key helper.
	if withIdentity {
		validSession := &kratos.Session{Identity: &kratos.Identity{Id: "identity-1"}}
		handler = api.AuthenticationMiddleware(&fakeSessionValidator{session: validSession})(handler)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if withIdentity {
		req.Header.Set("Cookie", "ory_kratos_session=valid")
	}
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != wantStatus {
		t.Errorf("status = %d, want %d", rec.Code, wantStatus)
	}
	if called != wantCalled {
		t.Errorf("next called = %v, want %v", called, wantCalled)
	}
	if fake != nil && fake.calls != wantCalls {
		t.Errorf("checker calls = %d, want %d", fake.calls, wantCalls)
	}
}
