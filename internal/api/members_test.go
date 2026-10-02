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

var errFakeMemberNotFound = errors.New("fake: member not found")

// fakeMemberStore is a minimal api.MemberStore for testing the member
// handlers without a real database.
type fakeMemberStore struct {
	member api.Member
	list   []api.Member
	err    error

	lastDrawerID    int64
	lastID          int64
	lastFullName    string
	lastPhoneNumber string
}

func (f *fakeMemberStore) CreateMember(
	_ context.Context,
	drawerID int64,
	fullName, phoneNumber string,
) (api.Member, error) {
	f.lastDrawerID, f.lastFullName, f.lastPhoneNumber = drawerID, fullName, phoneNumber
	return f.member, f.err
}

func (f *fakeMemberStore) GetMember(_ context.Context, drawerID, id int64) (api.Member, error) {
	f.lastDrawerID, f.lastID = drawerID, id
	return f.member, f.err
}

func (f *fakeMemberStore) ListMembersByDrawer(_ context.Context, drawerID int64) ([]api.Member, error) {
	f.lastDrawerID = drawerID
	return f.list, f.err
}

func (f *fakeMemberStore) UpdateMember(
	_ context.Context,
	drawerID, id int64,
	fullName, phoneNumber string,
) (api.Member, error) {
	f.lastDrawerID, f.lastID, f.lastFullName, f.lastPhoneNumber = drawerID, id, fullName, phoneNumber
	return f.member, f.err
}

func (f *fakeMemberStore) DeleteMember(_ context.Context, drawerID, id int64) error {
	f.lastDrawerID, f.lastID = drawerID, id
	return f.err
}

func testMemberAdapter() api.Adapter {
	return api.Adapter{
		Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		ErrorsMap: api.ErrorsMap{
			{Match: errFakeMemberNotFound, Problem: api.Problem{Status: http.StatusNotFound, Title: "Not Found"}},
			{
				Match:   api.ErrMemberInvalidPhoneNumber,
				Problem: api.Problem{Status: http.StatusBadRequest, Title: "Bad Request"},
			},
		},
	}
}

func mountTestMembers(members api.MemberStore) http.Handler {
	r := chi.NewRouter()
	api.MountMembers(r, members, testMemberAdapter())
	return r
}

func TestMountMembers_CRUD(t *testing.T) {
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
			"/drawers/1/members/",
			`{"full_name":"Alice","phone_number":"+447700900000"}`,
			http.StatusCreated,
		},
		{"list", http.MethodGet, "/drawers/1/members/", "", http.StatusOK},
		{"get", http.MethodGet, "/drawers/1/members/1", "", http.StatusOK},
		{
			"update",
			http.MethodPut,
			"/drawers/1/members/1",
			`{"full_name":"Alice B","phone_number":"+447700900000"}`,
			http.StatusOK,
		},
		{"delete", http.MethodDelete, "/drawers/1/members/1", "", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			member := api.Member{
				ID:          1,
				DrawerID:    1,
				FullName:    "Alice",
				PhoneNumber: "+447700900000",
				CreatedAt:   time.Now(),
			}
			fake := &fakeMemberStore{member: member, list: []api.Member{member}}
			router := mountTestMembers(fake)

			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestMountMembers_CreatePassesDrawerIDAndBody(t *testing.T) {
	t.Parallel()

	fake := &fakeMemberStore{member: api.Member{ID: 1}}
	router := mountTestMembers(fake)

	req := httptest.NewRequest(
		http.MethodPost, "/drawers/7/members/",
		bytes.NewBufferString(`{"full_name":"Alice","phone_number":"+447700900000"}`),
	)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	if fake.lastDrawerID != 7 {
		t.Errorf("lastDrawerID = %d, want 7", fake.lastDrawerID)
	}
	if fake.lastFullName != "Alice" || fake.lastPhoneNumber != "+447700900000" {
		t.Errorf(
			"lastFullName/lastPhoneNumber = %q/%q, want Alice/+447700900000",
			fake.lastFullName,
			fake.lastPhoneNumber,
		)
	}
}

// TestMountMembers_GetWrongDrawerReturns404 covers the
// cross-drawer-enumeration-prevention behavior: a fake store simulating
// "member 5 belongs to a different drawer" returns ErrMemberNotFound, and
// the handler must surface a plain 404 — not a 403, not a generic 500 —
// so a caller can't distinguish "wrong drawer" from "doesn't exist".
func TestMountMembers_GetWrongDrawerReturns404(t *testing.T) {
	t.Parallel()

	fake := &fakeMemberStore{err: errFakeMemberNotFound}
	router := mountTestMembers(fake)

	req := httptest.NewRequest(http.MethodGet, "/drawers/1/members/5", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
	if fake.lastDrawerID != 1 || fake.lastID != 5 {
		t.Errorf("lastDrawerID/lastID = %d/%d, want 1/5", fake.lastDrawerID, fake.lastID)
	}
}

func TestMountMembers_CreateInvalidPhoneNumberReturns400(t *testing.T) {
	t.Parallel()

	fake := &fakeMemberStore{err: api.ErrMemberInvalidPhoneNumber}
	router := mountTestMembers(fake)

	req := httptest.NewRequest(
		http.MethodPost, "/drawers/1/members",
		bytes.NewBufferString(`{"full_name":"Alice","phone_number":"07700900000"}`),
	)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestMountMembers_GetInvalidID(t *testing.T) {
	t.Parallel()

	fake := &fakeMemberStore{}
	router := mountTestMembers(fake)

	req := httptest.NewRequest(http.MethodGet, "/drawers/1/members/not-a-number", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (unmapped parse error, body: %s)", rec.Code, rec.Body.String())
	}
}
