package api_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

func TestWriteProblem(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	err := api.WriteProblem(rec, api.Problem{Status: 404, Title: "Not Found", Detail: "no such widget"})
	if err != nil {
		t.Fatalf("WriteProblem() error = %v", err)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", got)
	}
	if rec.Code != 404 {
		t.Errorf("status = %d, want 404", rec.Code)
	}

	var got api.Problem
	if unmarshalErr := json.Unmarshal(rec.Body.Bytes(), &got); unmarshalErr != nil {
		t.Fatalf("unmarshal body: %v", unmarshalErr)
	}

	want := api.Problem{Type: "about:blank", Status: 404, Title: "Not Found", Detail: "no such widget"}
	if got != want {
		t.Errorf("body = %+v, want %+v", got, want)
	}
}

func TestWriteProblem_CustomType(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	err := api.WriteProblem(rec, api.Problem{Type: "https://example.com/probs/widget", Status: 409, Title: "Conflict"})
	if err != nil {
		t.Fatalf("WriteProblem() error = %v", err)
	}

	var got api.Problem
	if unmarshalErr := json.Unmarshal(rec.Body.Bytes(), &got); unmarshalErr != nil {
		t.Fatalf("unmarshal body: %v", unmarshalErr)
	}

	if got.Type != "https://example.com/probs/widget" {
		t.Errorf("Type = %q, want custom type preserved", got.Type)
	}
}
