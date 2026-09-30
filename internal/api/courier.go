package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// SMSSender sends a single SMS message. Implemented by
// internal/clients/smsclient.Client (Phase 3); api depends on this
// interface instead of that concrete type so it never imports
// internal/clients, per ARCHITECTURE.md's dependency-direction rule.
//
// Unlike SessionValidator/PermissionChecker, a nil SMSSender is never
// wired here — servers.go only sets RouterDeps.CourierSMS once the kratos
// feature (and therefore the webhook route) is enabled, and Phase 3's
// Client.Send must itself never silently drop a message (see this plan's
// Cross-Phase Contract) even when the sms feature is disabled.
type SMSSender interface {
	Send(ctx context.Context, to, body string) error
}

// courierWebhookRequest is the JSON body our own login_code.sms.jsonnet
// template produces — see deploy/kratos/login_code.sms.jsonnet.
type courierWebhookRequest struct {
	To   string `json:"to"`
	Body string `json:"body"`
}

// MountCourierWebhook adds the Kratos courier's outgoing-SMS webhook to r,
// under /webhooks/kratos/sms. It must be mounted on a route group that
// does NOT carry AuthenticationMiddleware/AuthorizationMiddleware — the
// caller is Kratos itself, authenticated instead by
// CourierWebhookAuthMiddleware (see courier_auth.go).
func MountCourierWebhook(r chi.Router, sender SMSSender, adapter Adapter) {
	r.Post("/webhooks/kratos/sms", adapter.Adapt(sendCourierSMS(sender)))
}

func sendCourierSMS(sender SMSSender) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		var req courierWebhookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return fmt.Errorf("api: decode courier webhook request: %w", err)
		}

		if err := sender.Send(r.Context(), req.To, req.Body); err != nil {
			return fmt.Errorf("api: send courier sms: %w", err)
		}

		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}
