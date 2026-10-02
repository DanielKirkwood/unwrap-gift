package api_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	kratos "github.com/ory/kratos-client-go/v26"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

var errFakeDrawerNotFound = errors.New("fake: drawer not found")

// fakeDrawerStore is a minimal api.DrawerStore for testing the drawer
// handlers without a real database.
type fakeDrawerStore struct {
	drawer api.Drawer
	list   []api.Drawer
	err    error

	lastName                string
	lastOrganiserIdentityID string
	lastID                  int64
	lastHistoryWindowDraws  int64
}

func (f *fakeDrawerStore) CreateDrawer(_ context.Context, name, organiserIdentityID string) (api.Drawer, error) {
	f.lastName, f.lastOrganiserIdentityID = name, organiserIdentityID
	return f.drawer, f.err
}

func (f *fakeDrawerStore) GetDrawer(_ context.Context, id int64) (api.Drawer, error) {
	f.lastID = id
	return f.drawer, f.err
}

func (f *fakeDrawerStore) ListDrawersByOrganiser(_ context.Context, organiserIdentityID string) ([]api.Drawer, error) {
	f.lastOrganiserIdentityID = organiserIdentityID
	return f.list, f.err
}

func (f *fakeDrawerStore) UpdateDrawer(
	_ context.Context,
	id int64,
	name string,
	historyWindowDraws int64,
) (api.Drawer, error) {
	f.lastID, f.lastName, f.lastHistoryWindowDraws = id, name, historyWindowDraws
	return f.drawer, f.err
}

func (f *fakeDrawerStore) DeleteDrawer(_ context.Context, id int64) error {
	f.lastID = id
	return f.err
}

// fakeAuthedValidator is a SessionValidator that always succeeds with a
// fixed identity, the way auth_test.go's "valid session" case uses one —
// the mechanism this codebase already has for a test request with an
// authenticated identity already resolved.
type fakeAuthedValidator struct{ identityID string }

func (f fakeAuthedValidator) ToSession(context.Context, string) (*kratos.Session, error) {
	return &kratos.Session{Identity: &kratos.Identity{Id: f.identityID}}, nil
}

func testDrawerAdapter() api.Adapter {
	return api.Adapter{
		Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		ErrorsMap: api.ErrorsMap{
			{Match: errFakeDrawerNotFound, Problem: api.Problem{Status: http.StatusNotFound, Title: "Not Found"}},
		},
	}
}

// mountTestDrawers mounts the drawer routes behind a real
// AuthenticationMiddleware backed by fakeAuthedValidator, so
// IdentityFromContext resolves inside the handlers exactly as it would in
// production. A request must carry a non-empty Cookie header for the
// middleware to call the validator at all.
func mountTestDrawers(drawers api.DrawerStore, identityID string) http.Handler {
	r := chi.NewRouter()
	r.Use(api.AuthenticationMiddleware(fakeAuthedValidator{identityID: identityID}))
	api.MountDrawers(r, drawers, testDrawerAdapter())
	return r
}

func newAuthedDrawerRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Cookie", "ory_kratos_session=test")
	return req
}

func TestMountDrawers_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"create", http.MethodPost, "/drawers", `{"name":"Office Secret Santa"}`, http.StatusCreated},
		{"list", http.MethodGet, "/drawers", "", http.StatusOK},
		{"get", http.MethodGet, "/drawers/1", "", http.StatusOK},
		{"update", http.MethodPut, "/drawers/1", `{"name":"new-name","history_window_draws":3}`, http.StatusOK},
		{"delete", http.MethodDelete, "/drawers/1", "", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			drawer := api.Drawer{
				ID:                        1,
				Name:                      "Office Secret Santa",
				OrganiserKratosIdentityID: "identity-1",
				CreatedAt:                 time.Now(),
			}
			fake := &fakeDrawerStore{drawer: drawer, list: []api.Drawer{drawer}}
			router := mountTestDrawers(fake, "identity-1")

			req := newAuthedDrawerRequest(tt.method, tt.path, tt.body)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestMountDrawers_CreateSetsOrganiserFromContext(t *testing.T) {
	t.Parallel()

	fake := &fakeDrawerStore{drawer: api.Drawer{ID: 1, Name: "Office Secret Santa"}}
	router := mountTestDrawers(fake, "identity-1")

	// The request body deliberately omits organiser_kratos_identity_id —
	// drawerCreateRequest has no such field, so it can't be spoofed.
	req := newAuthedDrawerRequest(http.MethodPost, "/drawers", `{"name":"Office Secret Santa"}`)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	if fake.lastOrganiserIdentityID != "identity-1" {
		t.Errorf(
			"lastOrganiserIdentityID = %q, want identity-1 (the caller's own identity)",
			fake.lastOrganiserIdentityID,
		)
	}
}

func TestMountDrawers_ListUsesCallerIdentity(t *testing.T) {
	t.Parallel()

	fake := &fakeDrawerStore{}
	router := mountTestDrawers(fake, "identity-2")

	req := newAuthedDrawerRequest(http.MethodGet, "/drawers", "")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if fake.lastOrganiserIdentityID != "identity-2" {
		t.Errorf("lastOrganiserIdentityID = %q, want identity-2", fake.lastOrganiserIdentityID)
	}
}

func TestMountDrawers_GetInvalidID(t *testing.T) {
	t.Parallel()

	fake := &fakeDrawerStore{}
	router := mountTestDrawers(fake, "identity-1")

	req := newAuthedDrawerRequest(http.MethodGet, "/drawers/not-a-number", "")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (unmapped parse error, body: %s)", rec.Code, rec.Body.String())
	}
}

func TestMountDrawers_GetNotFound(t *testing.T) {
	t.Parallel()

	fake := &fakeDrawerStore{err: errFakeDrawerNotFound}
	router := mountTestDrawers(fake, "identity-1")

	req := newAuthedDrawerRequest(http.MethodGet, "/drawers/1", "")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
	if fake.lastID != 1 {
		t.Errorf("lastID = %d, want 1", fake.lastID)
	}
}

func TestMountDrawers_DeleteNotFound(t *testing.T) {
	t.Parallel()

	fake := &fakeDrawerStore{err: errFakeDrawerNotFound}
	router := mountTestDrawers(fake, "identity-1")

	req := newAuthedDrawerRequest(http.MethodDelete, "/drawers/1", "")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}
