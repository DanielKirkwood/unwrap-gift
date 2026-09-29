package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

func TestMountHealth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ready      api.ReadyCheck
		path       string
		wantStatus int
	}{
		{name: "alive, nil ready", path: "/health/alive", wantStatus: http.StatusOK},
		{name: "ready, nil ready check", path: "/health/ready", wantStatus: http.StatusOK},
		{
			name:       "ready, failing check",
			path:       "/health/ready",
			wantStatus: http.StatusServiceUnavailable,
			ready:      func(context.Context) error { return errors.New("db down") },
		},
		{
			name:       "alive stays up even when not ready",
			path:       "/health/alive",
			wantStatus: http.StatusOK,
			ready:      func(context.Context) error { return errors.New("db down") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := chi.NewRouter()
			api.MountHealth(r, tt.ready)

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
