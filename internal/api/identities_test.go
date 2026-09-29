package api_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	kratos "github.com/ory/kratos-client-go/v26"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

var errFakeIdentityNotFound = errors.New("fake: identity not found")

// fakeIdentityAdmin is a minimal api.IdentityAdmin for testing the admin
// identity handlers without a real Kratos instance. It records the
// arguments its last call was made with, so tests can assert on what the
// handlers passed through.
type fakeIdentityAdmin struct {
	identity *kratos.Identity
	list     []kratos.Identity
	err      error

	lastTraits   map[string]any
	lastPassword string
	lastState    string
}

func (f *fakeIdentityAdmin) CreateIdentity(
	_ context.Context,
	traits map[string]any,
	password string,
) (*kratos.Identity, error) {
	f.lastTraits, f.lastPassword = traits, password
	return f.identity, f.err
}

func (f *fakeIdentityAdmin) ListIdentities(_ context.Context) ([]kratos.Identity, error) {
	return f.list, f.err
}

func (f *fakeIdentityAdmin) GetIdentity(_ context.Context, _ string) (*kratos.Identity, error) {
	return f.identity, f.err
}

func (f *fakeIdentityAdmin) UpdateIdentity(
	_ context.Context,
	_, state string,
	traits map[string]any,
) (*kratos.Identity, error) {
	f.lastState, f.lastTraits = state, traits
	return f.identity, f.err
}

func (f *fakeIdentityAdmin) DeleteIdentity(_ context.Context, _ string) error {
	return f.err
}

func testAdapter() api.Adapter {
	return api.Adapter{
		Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		ErrorsMap: api.ErrorsMap{
			{Match: errFakeIdentityNotFound, Problem: api.Problem{Status: http.StatusNotFound, Title: "Not Found"}},
		},
	}
}

func mountTestIdentities(identities api.IdentityAdmin) http.Handler {
	r := chi.NewRouter()
	api.MountIdentities(r, identities, testAdapter())
	return r
}

func TestMountIdentities_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			"create",
			http.MethodPost,
			"/admin/identities",
			`{"traits":{"email":"a@b.com"},"password":"pw"}`,
			http.StatusCreated,
		},
		{"list", http.MethodGet, "/admin/identities", "", http.StatusOK},
		{"get", http.MethodGet, "/admin/identities/identity-1", "", http.StatusOK},
		{"update", http.MethodPut, "/admin/identities/identity-1", `{"traits":{"email":"a@b.com"}}`, http.StatusOK},
		{"delete", http.MethodDelete, "/admin/identities/identity-1", "", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			identity := kratos.Identity{Id: "identity-1", SchemaId: "default"}
			fake := &fakeIdentityAdmin{identity: &identity, list: []kratos.Identity{identity}}
			router := mountTestIdentities(fake)

			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestMountIdentities_CreatePassesTraitsAndPassword(t *testing.T) {
	t.Parallel()

	identity := kratos.Identity{Id: "identity-1"}
	fake := &fakeIdentityAdmin{identity: &identity}
	router := mountTestIdentities(fake)

	body := bytes.NewBufferString(`{"traits":{"email":"a@b.com"},"password":"s3cret"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/identities", body)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	if fake.lastPassword != "s3cret" {
		t.Errorf("lastPassword = %q, want s3cret", fake.lastPassword)
	}
	if fake.lastTraits["email"] != "a@b.com" {
		t.Errorf("lastTraits[email] = %v, want a@b.com", fake.lastTraits["email"])
	}
}

func TestMountIdentities_UpdateDefaultsState(t *testing.T) {
	t.Parallel()

	identity := kratos.Identity{Id: "identity-1"}
	fake := &fakeIdentityAdmin{identity: &identity}
	router := mountTestIdentities(fake)

	req := httptest.NewRequest(http.MethodPut, "/admin/identities/identity-1", bytes.NewBufferString(`{"traits":{}}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if fake.lastState != "active" {
		t.Errorf("lastState = %q, want active (default)", fake.lastState)
	}
}

func TestMountIdentities_UpdateExplicitState(t *testing.T) {
	t.Parallel()

	identity := kratos.Identity{Id: "identity-1"}
	fake := &fakeIdentityAdmin{identity: &identity}
	router := mountTestIdentities(fake)

	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/identities/identity-1",
		bytes.NewBufferString(`{"traits":{},"state":"inactive"}`),
	)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if fake.lastState != "inactive" {
		t.Errorf("lastState = %q, want inactive", fake.lastState)
	}
}

func TestMountIdentities_NotFound(t *testing.T) {
	t.Parallel()

	fake := &fakeIdentityAdmin{err: errFakeIdentityNotFound}
	router := mountTestIdentities(fake)

	req := httptest.NewRequest(http.MethodGet, "/admin/identities/missing", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestMountIdentities_InvalidBody(t *testing.T) {
	t.Parallel()

	router := mountTestIdentities(&fakeIdentityAdmin{})

	req := httptest.NewRequest(http.MethodPost, "/admin/identities", bytes.NewBufferString("not json"))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (unmapped decode error)", rec.Code)
	}
}
