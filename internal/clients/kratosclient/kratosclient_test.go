package kratosclient_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	kratos "github.com/ory/kratos-client-go/v26"

	"github.com/DanielKirkwood/unwrap-gift/internal/clients/kratosclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

func TestNewDisabled(t *testing.T) {
	t.Parallel()

	client, err := kratosclient.New(config.KratosConfig{}, false)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if client != nil {
		t.Errorf("New() = %+v, want nil", client)
	}
}

func TestNewEnabled(t *testing.T) {
	t.Parallel()

	cfg := config.KratosConfig{PublicURL: "http://public.example", AdminURL: "http://admin.example"}

	client, err := kratosclient.New(cfg, true)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if client == nil {
		t.Fatal("New() = nil, want non-nil")
	}
	if client.Public == nil {
		t.Error("Public = nil, want non-nil")
	}
	if client.Admin == nil {
		t.Error("Admin = nil, want non-nil")
	}
}

// fakeKratos serves the small slice of Kratos's public/admin HTTP surface
// kratosclient.Client's methods call, so its behavior can be exercised
// without a real Kratos instance.
func fakeKratos(t *testing.T) *httptest.Server {
	t.Helper()

	const validCookie = "ory_kratos_session=valid"

	mux := http.NewServeMux()

	mux.HandleFunc("GET /sessions/whoami", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != validCookie {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":       "session-1",
			"identity": identityJSON("identity-1"),
		})
	})

	mux.HandleFunc("POST /admin/identities", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusCreated, identityJSON("identity-new"))
	})

	mux.HandleFunc("GET /admin/identities", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, []map[string]any{identityJSON("identity-1"), identityJSON("identity-2")})
	})

	mux.HandleFunc("GET /admin/identities/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, identityJSON(r.PathValue("id")))
	})

	mux.HandleFunc("PUT /admin/identities/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, identityJSON(r.PathValue("id")))
	})

	mux.HandleFunc("DELETE /admin/identities/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func identityJSON(id string) map[string]any {
	return map[string]any{
		"id":         id,
		"schema_id":  "default",
		"schema_url": "http://example.test/schema.json",
		"traits":     map[string]any{"email": "user@example.test"},
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func testClient(t *testing.T, server *httptest.Server) *kratosclient.Client {
	t.Helper()

	cfg := kratos.NewConfiguration()
	cfg.Servers = kratos.ServerConfigurations{{URL: server.URL}}
	cfg.HTTPClient = server.Client()
	apiClient := kratos.NewAPIClient(cfg)

	return &kratosclient.Client{Public: apiClient, Admin: apiClient}
}

func TestClient_ToSession(t *testing.T) {
	t.Parallel()

	client := testClient(t, fakeKratos(t))

	t.Run("valid cookie", func(t *testing.T) {
		t.Parallel()

		session, err := client.ToSession(t.Context(), "ory_kratos_session=valid")
		if err != nil {
			t.Fatalf("ToSession() error = %v, want nil", err)
		}
		if session.Identity == nil || session.Identity.Id != "identity-1" {
			t.Errorf("ToSession() identity = %+v, want id identity-1", session.Identity)
		}
	})

	t.Run("invalid cookie", func(t *testing.T) {
		t.Parallel()

		if _, err := client.ToSession(t.Context(), "ory_kratos_session=invalid"); err == nil {
			t.Fatal("ToSession() error = nil, want error for invalid cookie")
		}
	})
}

func TestClient_IdentityCRUD(t *testing.T) {
	t.Parallel()

	client := testClient(t, fakeKratos(t))

	created, err := client.CreateIdentity(t.Context(), map[string]any{"email": "user@example.test"}, "s3cr3t-password")
	if err != nil {
		t.Fatalf("CreateIdentity() error = %v, want nil", err)
	}
	if created.Id != "identity-new" {
		t.Errorf("CreateIdentity() id = %q, want identity-new", created.Id)
	}

	list, err := client.ListIdentities(t.Context())
	if err != nil {
		t.Fatalf("ListIdentities() error = %v, want nil", err)
	}
	if len(list) != 2 {
		t.Errorf("ListIdentities() len = %d, want 2", len(list))
	}

	got, err := client.GetIdentity(t.Context(), "identity-1")
	if err != nil {
		t.Fatalf("GetIdentity() error = %v, want nil", err)
	}
	if got.Id != "identity-1" {
		t.Errorf("GetIdentity() id = %q, want identity-1", got.Id)
	}

	updated, err := client.UpdateIdentity(
		t.Context(),
		"identity-1",
		"active",
		map[string]any{"email": "new@example.test"},
	)
	if err != nil {
		t.Fatalf("UpdateIdentity() error = %v, want nil", err)
	}
	if updated.Id != "identity-1" {
		t.Errorf("UpdateIdentity() id = %q, want identity-1", updated.Id)
	}

	if deleteErr := client.DeleteIdentity(t.Context(), "identity-1"); deleteErr != nil {
		t.Errorf("DeleteIdentity() error = %v, want nil", deleteErr)
	}
}

func TestClient_IdentityNotFound(t *testing.T) {
	t.Parallel()

	client := testClient(t, fakeKratos(t))

	tests := map[string]func() error{
		"get": func() error {
			_, err := client.GetIdentity(t.Context(), "missing")
			return err
		},
		"update": func() error {
			_, err := client.UpdateIdentity(t.Context(), "missing", "active", map[string]any{})
			return err
		},
		"delete": func() error {
			return client.DeleteIdentity(t.Context(), "missing")
		},
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := call()
			if !errors.Is(err, kratosclient.ErrIdentityNotFound) {
				t.Errorf("error = %v, want errors.Is(err, ErrIdentityNotFound)", err)
			}
		})
	}
}
