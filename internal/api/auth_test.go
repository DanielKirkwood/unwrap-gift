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

// fakeSessionValidator is a minimal api.SessionValidator for testing
// AuthenticationMiddleware without a real Kratos instance.
type fakeSessionValidator struct {
	session *kratos.Session
	err     error
	calls   int
}

func (f *fakeSessionValidator) ToSession(_ context.Context, _ string) (*kratos.Session, error) {
	f.calls++
	return f.session, f.err
}

func TestAuthenticationMiddleware(t *testing.T) {
	t.Parallel()

	validSession := &kratos.Session{Identity: &kratos.Identity{Id: "identity-1"}}

	tests := []struct {
		name       string
		validator  *fakeSessionValidator
		cookie     string
		wantStatus int
		wantCalled bool
		wantCalls  int
	}{
		{
			name:       "no validator (kratos disabled) passes through",
			validator:  nil,
			cookie:     "",
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "missing cookie",
			validator:  &fakeSessionValidator{session: validSession},
			cookie:     "",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
			wantCalls:  0,
		},
		{
			name:       "validator error",
			validator:  &fakeSessionValidator{err: errors.New("boom")},
			cookie:     "ory_kratos_session=whatever",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
			wantCalls:  1,
		},
		{
			name:       "session with no identity",
			validator:  &fakeSessionValidator{session: &kratos.Session{}},
			cookie:     "ory_kratos_session=whatever",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
			wantCalls:  1,
		},
		{
			name:       "valid session",
			validator:  &fakeSessionValidator{session: validSession},
			cookie:     "ory_kratos_session=valid",
			wantStatus: http.StatusOK,
			wantCalled: true,
			wantCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runAuthMiddlewareCase(t, tt.validator, tt.cookie, tt.wantStatus, tt.wantCalled, tt.wantCalls)
		})
	}
}

func runAuthMiddlewareCase(
	t *testing.T,
	fake *fakeSessionValidator,
	cookie string,
	wantStatus int,
	wantCalled bool,
	wantCalls int,
) {
	t.Helper()

	var validator api.SessionValidator
	if fake != nil {
		validator = fake
	}

	called := false
	var gotIdentity *kratos.Identity
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		gotIdentity, _ = api.IdentityFromContext(r.Context())
	})

	handler := api.AuthenticationMiddleware(validator)(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
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
		t.Errorf("validator calls = %d, want %d", fake.calls, wantCalls)
	}
	if wantCalled && fake != nil && (gotIdentity == nil || gotIdentity.Id != "identity-1") {
		t.Errorf("identity in context = %+v, want id identity-1", gotIdentity)
	}
}

func TestIdentityFromContext_Empty(t *testing.T) {
	t.Parallel()

	if _, ok := api.IdentityFromContext(t.Context()); ok {
		t.Error("IdentityFromContext() ok = true on bare context, want false")
	}
}
