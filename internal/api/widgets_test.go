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

var errFakeWidgetNotFound = errors.New("fake: widget not found")

// fakeWidgetStore is a minimal api.WidgetStore for testing the widget
// handlers without a real database.
type fakeWidgetStore struct {
	widget api.Widget
	list   []api.Widget
	err    error

	lastName string
	lastID   int64
}

func (f *fakeWidgetStore) CreateWidget(_ context.Context, name string) (api.Widget, error) {
	f.lastName = name
	return f.widget, f.err
}

func (f *fakeWidgetStore) GetWidget(_ context.Context, id int64) (api.Widget, error) {
	f.lastID = id
	return f.widget, f.err
}

func (f *fakeWidgetStore) ListWidgets(_ context.Context) ([]api.Widget, error) {
	return f.list, f.err
}

func (f *fakeWidgetStore) UpdateWidget(_ context.Context, id int64, name string) (api.Widget, error) {
	f.lastID, f.lastName = id, name
	return f.widget, f.err
}

func (f *fakeWidgetStore) DeleteWidget(_ context.Context, id int64) error {
	f.lastID = id
	return f.err
}

func testWidgetAdapter() api.Adapter {
	return api.Adapter{
		Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		ErrorsMap: api.ErrorsMap{
			{Match: errFakeWidgetNotFound, Problem: api.Problem{Status: http.StatusNotFound, Title: "Not Found"}},
		},
	}
}

func mountTestWidgets(widgets api.WidgetStore) http.Handler {
	r := chi.NewRouter()
	api.MountWidgets(r, widgets, testWidgetAdapter())
	return r
}

func TestMountWidgets_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"create", http.MethodPost, "/widgets", `{"name":"sprocket"}`, http.StatusCreated},
		{"list", http.MethodGet, "/widgets", "", http.StatusOK},
		{"get", http.MethodGet, "/widgets/1", "", http.StatusOK},
		{"update", http.MethodPut, "/widgets/1", `{"name":"new-name"}`, http.StatusOK},
		{"delete", http.MethodDelete, "/widgets/1", "", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			widget := api.Widget{ID: 1, Name: "sprocket", CreatedAt: time.Now()}
			fake := &fakeWidgetStore{widget: widget, list: []api.Widget{widget}}
			router := mountTestWidgets(fake)

			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestMountWidgets_CreatePassesName(t *testing.T) {
	t.Parallel()

	fake := &fakeWidgetStore{widget: api.Widget{ID: 1, Name: "sprocket"}}
	router := mountTestWidgets(fake)

	req := httptest.NewRequest(http.MethodPost, "/widgets", bytes.NewBufferString(`{"name":"sprocket"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	if fake.lastName != "sprocket" {
		t.Errorf("lastName = %q, want sprocket", fake.lastName)
	}
}

func TestMountWidgets_GetInvalidID(t *testing.T) {
	t.Parallel()

	fake := &fakeWidgetStore{}
	router := mountTestWidgets(fake)

	req := httptest.NewRequest(http.MethodGet, "/widgets/not-a-number", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (unmapped parse error, body: %s)", rec.Code, rec.Body.String())
	}
}

func TestMountWidgets_GetNotFound(t *testing.T) {
	t.Parallel()

	fake := &fakeWidgetStore{err: errFakeWidgetNotFound}
	router := mountTestWidgets(fake)

	req := httptest.NewRequest(http.MethodGet, "/widgets/1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
	if fake.lastID != 1 {
		t.Errorf("lastID = %d, want 1", fake.lastID)
	}
}

func TestMountWidgets_DeleteNotFound(t *testing.T) {
	t.Parallel()

	fake := &fakeWidgetStore{err: errFakeWidgetNotFound}
	router := mountTestWidgets(fake)

	req := httptest.NewRequest(http.MethodDelete, "/widgets/1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}
