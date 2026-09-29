package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

func TestEnforceJSON(t *testing.T) {
	t.Parallel()

	ok := api.EnforceJSON(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name        string
		contentType string
		body        string
		accept      string
		wantStatus  int
	}{
		{name: "no body no accept", wantStatus: http.StatusOK},
		{name: "json body", contentType: "application/json", body: "{}", wantStatus: http.StatusOK},
		{name: "bad content type", contentType: "text/plain", body: "x", wantStatus: http.StatusUnsupportedMediaType},
		{name: "accepts json", accept: "application/json", wantStatus: http.StatusOK},
		{name: "accepts anything", accept: "*/*", wantStatus: http.StatusOK},
		{name: "rejects accept", accept: "text/plain", wantStatus: http.StatusNotAcceptable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}

			rec := httptest.NewRecorder()
			ok.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	handler := api.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	wantHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
		"X-XSS-Protection":       "0",
	}
	for h, want := range wantHeaders {
		if got := rec.Header().Get(h); got != want {
			t.Errorf("%s = %q, want %q", h, got, want)
		}
	}
}
