package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/app"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
	"github.com/DanielKirkwood/unwrap-gift/internal/db"
)

func TestBuildServers_KratosDisabled_IdentitiesNotMounted(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{Env: "development", LogLevel: "debug"}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}

	servers, err := app.BuildServers(a)
	if err != nil {
		t.Fatalf("BuildServers() error = %v, want nil", err)
	}

	rec := httptest.NewRecorder()
	servers.Hidden.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/identities", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /admin/identities status = %d, want 404 (kratos disabled, route never mounted)", rec.Code)
	}
}

func TestBuildServers_KratosEnabled_HiddenRouterRequiresAuth(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{
		Env:                        "production",
		LogLevel:                   "info",
		KratosPublicURL:            "http://127.0.0.1:4433",
		KratosAdminURL:             "http://127.0.0.1:4434",
		KratosCourierWebhookSecret: "test-webhook-secret",
	}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}

	servers, err := app.BuildServers(a)
	if err != nil {
		t.Fatalf("BuildServers() error = %v, want nil", err)
	}

	// No Cookie header: AuthenticationMiddleware rejects before ever
	// reaching Kratos, so this needs no live Kratos instance to assert 401.
	rec := httptest.NewRecorder()
	servers.Hidden.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/identities", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /admin/identities status = %d, want 401 (no session cookie)", rec.Code)
	}

	// /health/* stays exempt even with kratos enabled.
	healthRec := httptest.NewRecorder()
	servers.Hidden.Handler.ServeHTTP(healthRec, httptest.NewRequest(http.MethodGet, "/health/alive", nil))

	if healthRec.Code != http.StatusOK {
		t.Errorf("GET /health/alive status = %d, want 200", healthRec.Code)
	}
}

func TestBuildServers_KratosDisabled_CourierWebhookNotMounted(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{Env: "development", LogLevel: "debug"}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}

	servers, err := app.BuildServers(a)
	if err != nil {
		t.Fatalf("BuildServers() error = %v, want nil", err)
	}

	rec := httptest.NewRecorder()
	servers.Hidden.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/webhooks/kratos/sms", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("POST /webhooks/kratos/sms status = %d, want 404 (kratos disabled, route never mounted)", rec.Code)
	}
}

func TestBuildServers_KratosEnabled_CourierWebhookRequiresSecret(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{
		Env:                        "production",
		LogLevel:                   "info",
		KratosPublicURL:            "http://127.0.0.1:4433",
		KratosAdminURL:             "http://127.0.0.1:4434",
		KratosCourierWebhookSecret: "test-webhook-secret",
	}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}

	servers, err := app.BuildServers(a)
	if err != nil {
		t.Fatalf("BuildServers() error = %v, want nil", err)
	}

	// No X-Courier-Webhook-Secret header: CourierWebhookAuthMiddleware
	// rejects before ever reaching the handler/SMS client.
	rec := httptest.NewRecorder()
	servers.Hidden.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/webhooks/kratos/sms", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /webhooks/kratos/sms status = %d, want 401 (no webhook secret header)", rec.Code)
	}

	// With the correct secret, the request reaches the handler; a.SMS is
	// disabled (no SEVEN_* env set) so Send logs instead of calling
	// seven.io.
	authedRec := httptest.NewRecorder()
	authedReq := httptest.NewRequest(
		http.MethodPost,
		"/webhooks/kratos/sms",
		bytes.NewBufferString(`{"to":"+15550001234","body":"your code is 123456"}`),
	)
	authedReq.Header.Set("X-Courier-Webhook-Secret", "test-webhook-secret")
	authedReq.Header.Set("Content-Type", "application/json")
	servers.Hidden.Handler.ServeHTTP(authedRec, authedReq)

	if authedRec.Code != http.StatusNoContent {
		t.Errorf("POST /webhooks/kratos/sms status = %d, want 204 (valid secret + body)", authedRec.Code)
	}
}

func TestBuildServers_DatabaseDisabled_WidgetsNotMounted(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{Env: "development", LogLevel: "debug"}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}

	servers, err := app.BuildServers(a)
	if err != nil {
		t.Fatalf("BuildServers() error = %v, want nil", err)
	}

	rec := httptest.NewRecorder()
	servers.Protected.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/widgets", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /widgets status = %d, want 404 (database disabled, route never mounted)", rec.Code)
	}
}

// TestBuildServers_DatabaseEnabled_WidgetsCRUD proves the combination
// nothing before Phase 7 exercised end-to-end: a real sqlc-backed resource,
// served through the protected router. Kratos stays disabled so
// RouterDeps.Auth is nil and the request needs no session cookie — auth
// itself is covered separately below.
func TestBuildServers_DatabaseEnabled_WidgetsCRUD(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{
		Env:          "development",
		LogLevel:     "debug",
		DatabasePath: filepath.Join(t.TempDir(), "test.db"),
	}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = a.Store.Close() })

	if migrateErr := db.Migrate(t.Context(), a.Store.DB); migrateErr != nil {
		t.Fatalf("Migrate() error = %v, want nil", migrateErr)
	}

	servers, err := app.BuildServers(a)
	if err != nil {
		t.Fatalf("BuildServers() error = %v, want nil", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/widgets", bytes.NewBufferString(`{"name":"sprocket"}`))
	createReq.Header.Set("Content-Type", "application/json")

	createRec := httptest.NewRecorder()
	servers.Protected.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("POST /widgets status = %d, want 201 (body: %s)", createRec.Code, createRec.Body.String())
	}

	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if decodeErr := json.NewDecoder(createRec.Body).Decode(&created); decodeErr != nil {
		t.Fatalf("decode create response: %v", decodeErr)
	}

	getRec := httptest.NewRecorder()
	getPath := "/widgets/" + strconv.FormatInt(created.Data.ID, 10)
	servers.Protected.Handler.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, getPath, nil))
	if getRec.Code != http.StatusOK {
		t.Errorf("GET %s status = %d, want 200 (body: %s)", getPath, getRec.Code, getRec.Body.String())
	}

	deleteRec := httptest.NewRecorder()
	servers.Protected.Handler.ServeHTTP(deleteRec, httptest.NewRequest(http.MethodDelete, getPath, nil))
	if deleteRec.Code != http.StatusNoContent {
		t.Errorf("DELETE %s status = %d, want 204 (body: %s)", getPath, deleteRec.Code, deleteRec.Body.String())
	}
}

// TestBuildServers_OrganiserAPI_RequiresAllThreeFeatures covers the
// Phase 5 organiser API wiring appearing only when database, kratos, and
// keto are all enabled — not just one or two of them — mirroring the
// existing Widgets/Identities nil-vs-set coverage.
func TestBuildServers_OrganiserAPI_RequiresAllThreeFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		env         config.EnvVars
		wantMounted bool
	}{
		{
			name:        "all disabled",
			env:         config.EnvVars{Env: "development", LogLevel: "debug"},
			wantMounted: false,
		},
		{
			name: "only database enabled",
			env: config.EnvVars{
				Env: "development", LogLevel: "debug",
				DatabasePath: filepath.Join(t.TempDir(), "test.db"),
			},
			wantMounted: false,
		},
		{
			name: "database and kratos enabled, keto disabled",
			env: config.EnvVars{
				Env: "production", LogLevel: "info",
				DatabasePath:               filepath.Join(t.TempDir(), "test.db"),
				KratosPublicURL:            "http://127.0.0.1:4433",
				KratosAdminURL:             "http://127.0.0.1:4434",
				KratosCourierWebhookSecret: "test-webhook-secret",
			},
			wantMounted: false,
		},
		{
			name: "database, kratos, and keto all enabled",
			env: config.EnvVars{
				Env: "production", LogLevel: "info",
				DatabasePath:               filepath.Join(t.TempDir(), "test.db"),
				KratosPublicURL:            "http://127.0.0.1:4433",
				KratosAdminURL:             "http://127.0.0.1:4434",
				KratosCourierWebhookSecret: "test-webhook-secret",
				KetoReadURL:                "http://127.0.0.1:4466",
				KetoWriteURL:               "http://127.0.0.1:4467",
			},
			wantMounted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a, err := app.Bootstrap(t.Context(), tt.env)
			if err != nil {
				t.Fatalf("Bootstrap() error = %v, want nil", err)
			}
			if a.Store != nil {
				t.Cleanup(func() { _ = a.Store.Close() })
				if migrateErr := db.Migrate(t.Context(), a.Store.DB); migrateErr != nil {
					t.Fatalf("Migrate() error = %v, want nil", migrateErr)
				}
			}

			servers, err := app.BuildServers(a)
			if err != nil {
				t.Fatalf("BuildServers() error = %v, want nil", err)
			}

			rec := httptest.NewRecorder()
			servers.Hidden.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/drawers", nil))

			gotMounted := rec.Code != http.StatusNotFound
			if gotMounted != tt.wantMounted {
				t.Errorf(
					"GET /drawers status = %d, mounted = %v, want mounted = %v",
					rec.Code,
					gotMounted,
					tt.wantMounted,
				)
			}
		})
	}
}

func TestBuildServers_DatabaseAndKratosEnabled_ProtectedRouterRequiresAuth(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{
		Env:                        "production",
		LogLevel:                   "info",
		DatabasePath:               filepath.Join(t.TempDir(), "test.db"),
		KratosPublicURL:            "http://127.0.0.1:4433",
		KratosAdminURL:             "http://127.0.0.1:4434",
		KratosCourierWebhookSecret: "test-webhook-secret",
	}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = a.Store.Close() })

	if migrateErr := db.Migrate(t.Context(), a.Store.DB); migrateErr != nil {
		t.Fatalf("Migrate() error = %v, want nil", migrateErr)
	}

	servers, err := app.BuildServers(a)
	if err != nil {
		t.Fatalf("BuildServers() error = %v, want nil", err)
	}

	// No Cookie header: AuthenticationMiddleware rejects before ever
	// reaching Kratos or the widgets store, so this needs no live Kratos
	// instance to assert 401.
	rec := httptest.NewRecorder()
	servers.Protected.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/widgets", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /widgets status = %d, want 401 (no session cookie)", rec.Code)
	}
}

// TestBuildServers_WishlistItems_RequiresDatabaseAndKratos covers
// wireWishlistItems appearing only when database and kratos are both
// enabled — not just database alone (unlike Widgets, which needs no
// identity) and not requiring keto at all (unlike the organiser API) —
// mirroring TestBuildServers_OrganiserAPI_RequiresAllThreeFeatures' shape.
// No cookie is sent, so a mounted route answers 401 (AuthenticationMiddleware
// rejecting before reaching any store), while an unmounted route answers
// 404 — no live Kratos instance is needed for either outcome.
func TestBuildServers_WishlistItems_RequiresDatabaseAndKratos(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		env         config.EnvVars
		wantMounted bool
	}{
		{
			name:        "all disabled",
			env:         config.EnvVars{Env: "development", LogLevel: "debug"},
			wantMounted: false,
		},
		{
			name: "only database enabled",
			env: config.EnvVars{
				Env: "development", LogLevel: "debug",
				DatabasePath: filepath.Join(t.TempDir(), "test.db"),
			},
			wantMounted: false,
		},
		{
			name: "database and kratos enabled",
			env: config.EnvVars{
				Env: "production", LogLevel: "info",
				DatabasePath:               filepath.Join(t.TempDir(), "test.db"),
				KratosPublicURL:            "http://127.0.0.1:4433",
				KratosAdminURL:             "http://127.0.0.1:4434",
				KratosCourierWebhookSecret: "test-webhook-secret",
			},
			wantMounted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a, err := app.Bootstrap(t.Context(), tt.env)
			if err != nil {
				t.Fatalf("Bootstrap() error = %v, want nil", err)
			}
			if a.Store != nil {
				t.Cleanup(func() { _ = a.Store.Close() })
				if migrateErr := db.Migrate(t.Context(), a.Store.DB); migrateErr != nil {
					t.Fatalf("Migrate() error = %v, want nil", migrateErr)
				}
			}

			servers, err := app.BuildServers(a)
			if err != nil {
				t.Fatalf("BuildServers() error = %v, want nil", err)
			}

			rec := httptest.NewRecorder()
			servers.Protected.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/wishlist-items", nil))

			gotMounted := rec.Code != http.StatusNotFound
			if gotMounted != tt.wantMounted {
				t.Errorf(
					"GET /wishlist-items status = %d, mounted = %v, want mounted = %v",
					rec.Code,
					gotMounted,
					tt.wantMounted,
				)
			}
		})
	}
}
