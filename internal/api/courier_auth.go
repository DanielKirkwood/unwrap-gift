package api

import (
	"crypto/subtle"
	"net/http"
)

//nolint:gosec // deliberate: this is a header name, not a credential value.
const courierWebhookSecretHeader = "X-Courier-Webhook-Secret"

func courierWebhookUnauthorizedProblem() Problem {
	return Problem{
		Status: http.StatusUnauthorized,
		Title:  "Unauthorized",
		Detail: "a valid courier webhook secret is required",
	}
}

// CourierWebhookAuthMiddleware requires every request to carry the
// configured secret in the X-Courier-Webhook-Secret header — the same
// header name and value Kratos's courier.sms.request_config.auth
// (type: api_key) is configured to send. Unlike
// AuthenticationMiddleware/AuthorizationMiddleware, there is no
// nil/empty-is-passthrough case: this route triggers a billed SMS send,
// so it must fail closed. servers.go only constructs this middleware
// inside its existing "kratos feature enabled" branch, where
// KratosConfig.CourierWebhookSecret is guaranteed non-empty (it's a
// RequiredEnv entry — see internal/config/features.go).
func CourierWebhookAuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get(courierWebhookSecretHeader)
			if secret == "" || got == "" ||
				subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
				_ = WriteProblem(w, courierWebhookUnauthorizedProblem())
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
