package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

func TestWriteData(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	err := api.WriteData(rec, 200, map[string]string{"name": "widget"})
	if err != nil {
		t.Fatalf("WriteData() error = %v", err)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	want := `{"data":{"name":"widget"}}` + "\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
