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

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

var errFakeRelationshipNotFound = errors.New("fake: relationship not found")

// fakeRelationshipStore is a minimal api.RelationshipStore for testing the
// relationship handlers without a real database.
type fakeRelationshipStore struct {
	relationship api.Relationship
	list         []api.Relationship
	err          error

	lastPhoneNumberA string
	lastPhoneNumberB string
	lastPhoneNumber  string
	lastID           int64
}

func (f *fakeRelationshipStore) CreateRelationship(
	_ context.Context,
	phoneNumberA, phoneNumberB string,
) (api.Relationship, error) {
	f.lastPhoneNumberA, f.lastPhoneNumberB = phoneNumberA, phoneNumberB
	return f.relationship, f.err
}

func (f *fakeRelationshipStore) ListRelationshipsForPhoneNumber(
	_ context.Context,
	phoneNumber string,
) ([]api.Relationship, error) {
	f.lastPhoneNumber = phoneNumber
	return f.list, f.err
}

func (f *fakeRelationshipStore) DeleteRelationship(_ context.Context, id int64) error {
	f.lastID = id
	return f.err
}

func testRelationshipAdapter() api.Adapter {
	return api.Adapter{
		Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		ErrorsMap: api.ErrorsMap{
			{Match: errFakeRelationshipNotFound, Problem: api.Problem{Status: http.StatusNotFound, Title: "Not Found"}},
			{
				Match:   api.ErrRelationshipSamePhoneNumber,
				Problem: api.Problem{Status: http.StatusBadRequest, Title: "Bad Request"},
			},
		},
	}
}

func mountTestRelationships(relationships api.RelationshipStore) http.Handler {
	r := chi.NewRouter()
	api.MountRelationships(r, relationships, testRelationshipAdapter())
	return r
}

func TestMountRelationships_CRUD(t *testing.T) {
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
			"/relationships",
			`{"phone_number_a":"+447700900000","phone_number_b":"+447700900001"}`,
			http.StatusCreated,
		},
		{"list", http.MethodGet, "/relationships?phone_number=%2B447700900000", "", http.StatusOK},
		{"delete", http.MethodDelete, "/relationships/1", "", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			relationship := api.Relationship{
				ID:           1,
				PhoneNumberA: "+447700900000",
				PhoneNumberB: "+447700900001",
				CreatedAt:    time.Now(),
			}
			fake := &fakeRelationshipStore{relationship: relationship, list: []api.Relationship{relationship}}
			router := mountTestRelationships(fake)

			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestMountRelationships_ListMissingPhoneNumberReturns400(t *testing.T) {
	t.Parallel()

	fake := &fakeRelationshipStore{}
	router := mountTestRelationships(fake)

	req := httptest.NewRequest(http.MethodGet, "/relationships", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestMountRelationships_CreateSamePhoneNumberReturns400 simulates what
// the real storeRelationships adapter validates (Task 16) before it ever
// reaches sqlc: both phone numbers equal.
func TestMountRelationships_CreateSamePhoneNumberReturns400(t *testing.T) {
	t.Parallel()

	fake := &fakeRelationshipStore{err: api.ErrRelationshipSamePhoneNumber}
	router := mountTestRelationships(fake)

	req := httptest.NewRequest(
		http.MethodPost, "/relationships",
		bytes.NewBufferString(`{"phone_number_a":"+447700900000","phone_number_b":"+447700900000"}`),
	)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}
