package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

const testCourierWebhookSecret = "test-courier-webhook-secret"

func spyNextHandler() (http.Handler, *bool) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	return handler, &called
}

func TestCourierWebhookAuthMiddleware_ValidSecret(t *testing.T) {
	t.Parallel()

	next, called := spyNextHandler()
	handler := api.CourierWebhookAuthMiddleware(testCourierWebhookSecret)(next)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/kratos/sms", nil)
	req.Header.Set("X-Courier-Webhook-Secret", testCourierWebhookSecret)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !*called {
		t.Error("next handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestCourierWebhookAuthMiddleware_MissingHeader(t *testing.T) {
	t.Parallel()

	next, called := spyNextHandler()
	handler := api.CourierWebhookAuthMiddleware(testCourierWebhookSecret)(next)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/kratos/sms", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if *called {
		t.Error("next handler was called, want it rejected before reaching it")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestCourierWebhookAuthMiddleware_WrongSecret(t *testing.T) {
	t.Parallel()

	next, called := spyNextHandler()
	handler := api.CourierWebhookAuthMiddleware(testCourierWebhookSecret)(next)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/kratos/sms", nil)
	req.Header.Set("X-Courier-Webhook-Secret", "wrong-secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if *called {
		t.Error("next handler was called, want it rejected before reaching it")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestCourierWebhookAuthMiddleware_EmptyConfiguredSecret(t *testing.T) {
	t.Parallel()

	next, called := spyNextHandler()
	handler := api.CourierWebhookAuthMiddleware("")(next)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/kratos/sms", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if *called {
		t.Error("next handler was called, want fail-closed rejection with an empty configured secret")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
