package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

var errFakeDrawNotFound = errors.New("fake: draw not found")

// fakeDrawStore is a minimal api.DrawStore for testing the draw handlers
// without a real database.
type fakeDrawStore struct {
	draw        api.Draw
	list        []api.Draw
	assignments []api.Assignment
	err         error

	lastDrawerID int64
	lastID       int64
}

func (f *fakeDrawStore) CreateDraw(_ context.Context, drawerID int64, _ time.Time, _ int64) (api.Draw, error) {
	f.lastDrawerID = drawerID
	return f.draw, f.err
}

func (f *fakeDrawStore) GetDraw(_ context.Context, drawerID, id int64) (api.Draw, error) {
	f.lastDrawerID, f.lastID = drawerID, id
	return f.draw, f.err
}

func (f *fakeDrawStore) ListDrawsByDrawer(_ context.Context, drawerID int64) ([]api.Draw, error) {
	f.lastDrawerID = drawerID
	return f.list, f.err
}

func (f *fakeDrawStore) ListAssignmentsByDraw(_ context.Context, drawerID, drawID int64) ([]api.Assignment, error) {
	f.lastDrawerID, f.lastID = drawerID, drawID
	return f.assignments, f.err
}

func (f *fakeDrawStore) UpdateDraw(
	_ context.Context,
	drawerID, id int64,
	_ time.Time,
	_ int64,
) (api.Draw, error) {
	f.lastDrawerID, f.lastID = drawerID, id
	return f.draw, f.err
}

func (f *fakeDrawStore) RunDraw(_ context.Context, drawerID, drawID int64) (api.Draw, []api.Assignment, error) {
	f.lastDrawerID, f.lastID = drawerID, drawID
	return f.draw, f.assignments, f.err
}

func testDrawAdapter() api.Adapter {
	return api.Adapter{
		Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		ErrorsMap: api.ErrorsMap{
			{Match: errFakeDrawNotFound, Problem: api.Problem{Status: http.StatusNotFound, Title: "Not Found"}},
			{Match: api.ErrDrawAlreadyRun, Problem: api.Problem{Status: http.StatusConflict, Title: "Conflict"}},
			{Match: api.ErrNoValidAssignment, Problem: api.Problem{Status: http.StatusConflict, Title: "Conflict"}},
			{
				Match:   api.ErrTooFewMembers,
				Problem: api.Problem{Status: http.StatusUnprocessableEntity, Title: "Unprocessable Entity"},
			},
		},
	}
}

func mountTestDraws(draws api.DrawStore) http.Handler {
	r := chi.NewRouter()
	api.MountDraws(r, draws, testDrawAdapter())
	return r
}

func TestMountDraws_CRUD(t *testing.T) {
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
			"/drawers/1/draws/",
			`{"exchange_date":"2026-12-25T00:00:00Z","budget_amount":2000}`,
			http.StatusCreated,
		},
		{"list", http.MethodGet, "/drawers/1/draws/", "", http.StatusOK},
		{"get", http.MethodGet, "/drawers/1/draws/1", "", http.StatusOK},
		{
			"update",
			http.MethodPut,
			"/drawers/1/draws/1",
			`{"exchange_date":"2026-12-26T00:00:00Z","budget_amount":2500}`,
			http.StatusOK,
		},
		{"assignments", http.MethodGet, "/drawers/1/draws/1/assignments", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			draw := api.Draw{ID: 1, DrawerID: 1, Status: "draft", CreatedAt: time.Now()}
			fake := &fakeDrawStore{draw: draw, list: []api.Draw{draw}}
			router := mountTestDraws(fake)

			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

// TestMountDraws_RunSuccess asserts the run-draw response envelope
// contains both "draw" and "assignments".
func TestMountDraws_RunSuccess(t *testing.T) {
	t.Parallel()

	draw := api.Draw{ID: 1, DrawerID: 1, Status: "assigned"}
	assignments := []api.Assignment{
		{ID: 1, DrawID: 1, GifterMemberID: 10, GifteeMemberID: 20},
		{ID: 2, DrawID: 1, GifterMemberID: 20, GifteeMemberID: 10},
	}
	fake := &fakeDrawStore{draw: draw, assignments: assignments}
	router := mountTestDraws(fake)

	req := httptest.NewRequest(http.MethodPost, "/drawers/1/draws/1/run", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Draw        api.Draw         `json:"draw"`
			Assignments []api.Assignment `json:"assignments"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Draw.Status != "assigned" {
		t.Errorf("draw.status = %q, want assigned", body.Data.Draw.Status)
	}
	if len(body.Data.Assignments) != 2 {
		t.Errorf("len(assignments) = %d, want 2", len(body.Data.Assignments))
	}
}

func TestMountDraws_UpdateAlreadyRunReturns409(t *testing.T) {
	t.Parallel()

	fake := &fakeDrawStore{err: api.ErrDrawAlreadyRun}
	router := mountTestDraws(fake)

	req := httptest.NewRequest(
		http.MethodPut, "/drawers/1/draws/1",
		bytes.NewBufferString(`{"exchange_date":"2026-12-26T00:00:00Z","budget_amount":2500}`),
	)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestMountDraws_RunAlreadyRunReturns409(t *testing.T) {
	t.Parallel()

	fake := &fakeDrawStore{err: api.ErrDrawAlreadyRun}
	router := mountTestDraws(fake)

	req := httptest.NewRequest(http.MethodPost, "/drawers/1/draws/1/run", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestMountDraws_RunNoValidAssignmentReturns409(t *testing.T) {
	t.Parallel()

	fake := &fakeDrawStore{err: api.ErrNoValidAssignment}
	router := mountTestDraws(fake)

	req := httptest.NewRequest(http.MethodPost, "/drawers/1/draws/1/run", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestMountDraws_RunTooFewMembersReturns422(t *testing.T) {
	t.Parallel()

	fake := &fakeDrawStore{err: api.ErrTooFewMembers}
	router := mountTestDraws(fake)

	req := httptest.NewRequest(http.MethodPost, "/drawers/1/draws/1/run", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
	}
}
