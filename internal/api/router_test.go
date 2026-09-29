package api_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	kratos "github.com/ory/kratos-client-go/v26"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

func TestRouters_HealthAndSecurityHeaders(t *testing.T) {
	t.Parallel()

	deps := api.RouterDeps{Logger: slog.Default(), TracerProvider: noop.NewTracerProvider()}

	constructors := map[string]func(api.RouterDeps) http.Handler{
		"public":    func(d api.RouterDeps) http.Handler { return api.NewPublicRouter(d) },
		"protected": func(d api.RouterDeps) http.Handler { return api.NewProtectedRouter(d) },
		"hidden":    func(d api.RouterDeps) http.Handler { return api.NewHiddenRouter(d) },
	}

	for name, build := range constructors {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			router := build(deps)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/alive", nil))

			if rec.Code != http.StatusOK {
				t.Errorf("GET /health/alive status = %d, want 200", rec.Code)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
		})
	}
}

// blockAllAuth is a minimal Auth middleware for router tests: it rejects
// every request, so tests can tell whether a route ran behind it.
func blockAllAuth(_ http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
}

func TestProtectedAndHiddenRouters_HealthExemptFromAuth(t *testing.T) {
	t.Parallel()

	deps := api.RouterDeps{Logger: slog.Default(), TracerProvider: noop.NewTracerProvider(), Auth: blockAllAuth}

	constructors := map[string]func(api.RouterDeps) http.Handler{
		"protected": func(d api.RouterDeps) http.Handler { return api.NewProtectedRouter(d) },
		"hidden":    func(d api.RouterDeps) http.Handler { return api.NewHiddenRouter(d) },
	}

	for name, build := range constructors {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			router := build(deps)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/alive", nil))

			if rec.Code != http.StatusOK {
				t.Errorf("GET /health/alive status = %d, want 200 even with Auth set", rec.Code)
			}
		})
	}
}

func TestNewHiddenRouter_AuthGatesIdentities(t *testing.T) {
	t.Parallel()

	deps := api.RouterDeps{
		Logger:            slog.Default(),
		TracerProvider:    noop.NewTracerProvider(),
		Auth:              blockAllAuth,
		Identities:        &blockedIdentityAdmin{},
		IdentitiesAdapter: api.Adapter{Logger: slog.Default()},
	}

	router := api.NewHiddenRouter(deps)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/identities", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /admin/identities status = %d, want 401 (blocked by Auth)", rec.Code)
	}
}

// blockedIdentityAdmin is only used to satisfy api.IdentityAdmin's type for
// TestNewHiddenRouter_AuthGatesIdentities — Auth rejects the request before
// any of its methods would be called.
type blockedIdentityAdmin struct{}

func (blockedIdentityAdmin) CreateIdentity(context.Context, map[string]any, string) (*kratos.Identity, error) {
	panic("not called: request should be blocked by Auth")
}

func (blockedIdentityAdmin) ListIdentities(context.Context) ([]kratos.Identity, error) {
	panic("not called: request should be blocked by Auth")
}

func (blockedIdentityAdmin) GetIdentity(context.Context, string) (*kratos.Identity, error) {
	panic("not called: request should be blocked by Auth")
}

func (blockedIdentityAdmin) UpdateIdentity(context.Context, string, string, map[string]any) (*kratos.Identity, error) {
	panic("not called: request should be blocked by Auth")
}

func (blockedIdentityAdmin) DeleteIdentity(context.Context, string) error {
	panic("not called: request should be blocked by Auth")
}
