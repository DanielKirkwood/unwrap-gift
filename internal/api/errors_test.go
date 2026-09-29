package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

var errWidgetNotFound = errors.New("widget not found")

func TestAdapter_Adapt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handlerErr error
		wantStatus int
		wantTitle  string
		wantLogged bool
	}{
		{
			name:       "no error",
			wantStatus: http.StatusOK,
		},
		{
			name:       "mapped error",
			handlerErr: errWidgetNotFound,
			wantStatus: http.StatusNotFound,
			wantTitle:  "Not Found",
		},
		{
			name:       "wrapped mapped error",
			handlerErr: fmtErrorf(errWidgetNotFound),
			wantStatus: http.StatusNotFound,
			wantTitle:  "Not Found",
		},
		{
			name:       "unmapped error",
			handlerErr: errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantTitle:  "Internal Server Error",
			wantLogged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runAdapterCase(t, tt.handlerErr, tt.wantStatus, tt.wantTitle, tt.wantLogged)
		})
	}
}

func runAdapterCase(t *testing.T, handlerErr error, wantStatus int, wantTitle string, wantLogged bool) {
	t.Helper()

	var logBuf bytes.Buffer
	adapter := api.Adapter{
		Logger: slog.New(slog.NewTextHandler(&logBuf, nil)),
		ErrorsMap: api.ErrorsMap{
			{Match: errWidgetNotFound, Problem: api.Problem{Status: http.StatusNotFound, Title: "Not Found"}},
		},
	}

	handler := adapter.Adapt(func(w http.ResponseWriter, _ *http.Request) error {
		if handlerErr != nil {
			return handlerErr
		}
		return api.WriteData(w, http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != wantStatus {
		t.Errorf("status = %d, want %d", rec.Code, wantStatus)
	}

	if wantTitle != "" {
		var p api.Problem
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if p.Title != wantTitle {
			t.Errorf("Title = %q, want %q", p.Title, wantTitle)
		}
	}

	if wantLogged && logBuf.Len() == 0 {
		t.Error("expected error to be logged")
	}
}

func fmtErrorf(err error) error {
	return errors.Join(errors.New("context"), err)
}
