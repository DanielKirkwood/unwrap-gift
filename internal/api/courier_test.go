package api_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

var errFakeSMSSend = errors.New("fake: sms send failed")

// fakeSMSSender is a minimal api.SMSSender for testing the courier webhook
// handler without a real SMS client. It records the arguments its last
// call was made with, so tests can assert on what the handler passed
// through.
type fakeSMSSender struct {
	err              error
	lastTo, lastBody string
}

func (f *fakeSMSSender) Send(_ context.Context, to, body string) error {
	f.lastTo, f.lastBody = to, body
	return f.err
}

func mountTestCourierWebhook(sender api.SMSSender) http.Handler {
	r := chi.NewRouter()
	api.MountCourierWebhook(r, sender, testAdapter())
	return r
}

func TestMountCourierWebhook_Success(t *testing.T) {
	t.Parallel()

	fake := &fakeSMSSender{}
	router := mountTestCourierWebhook(fake)

	body := bytes.NewBufferString(`{"to":"+15550001234","body":"Your code is 123456"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/kratos/sms", body)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body: %s)", rec.Code, rec.Body.String())
	}
	if fake.lastTo != "+15550001234" {
		t.Errorf("lastTo = %q, want +15550001234", fake.lastTo)
	}
	if fake.lastBody != "Your code is 123456" {
		t.Errorf("lastBody = %q, want %q", fake.lastBody, "Your code is 123456")
	}
}

func TestMountCourierWebhook_SenderError(t *testing.T) {
	t.Parallel()

	fake := &fakeSMSSender{err: errFakeSMSSend}
	router := mountTestCourierWebhook(fake)

	body := bytes.NewBufferString(`{"to":"+15550001234","body":"Your code is 123456"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/kratos/sms", body)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (unmapped sender error)", rec.Code)
	}
}

func TestMountCourierWebhook_InvalidBody(t *testing.T) {
	t.Parallel()

	router := mountTestCourierWebhook(&fakeSMSSender{})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/kratos/sms", bytes.NewBufferString("not json"))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (unmapped decode error)", rec.Code)
	}
}
