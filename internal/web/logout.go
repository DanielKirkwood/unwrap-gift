package web

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	kratos "github.com/ory/kratos-client-go/v26"
)

// LogoutFlowProvider creates a one-time Kratos browser logout URL for a
// session, implemented by internal/clients/kratosclient.Client. web depends
// on this interface instead of that concrete type so it never imports
// internal/clients, per ARCHITECTURE.md's dependency-direction rule.
type LogoutFlowProvider interface {
	CreateBrowserLogoutFlow(ctx context.Context, cookieHeader, returnTo string) (*kratos.LogoutFlow, error)
}

// MountLogout adds GET /logout to r, meant to sit behind the same Auth group
// as /wishlist (unlike /login, which must stay outside it — see
// router.go). It asks Kratos for a logout URL for the request's session
// cookie and 303s the browser there, which actually performs the logout
// (clearing the session cookie) before landing back on /login. If no active
// session exists — the user is already logged out, or hit this directly —
// CreateBrowserLogoutFlow errors and this just redirects to /login, matching
// login.go's own "don't show an error, just restart the flow" convention.
func MountLogout(r chi.Router, flows LogoutFlowProvider) {
	r.Get("/logout", func(w http.ResponseWriter, r *http.Request) {
		flow, err := flows.CreateBrowserLogoutFlow(r.Context(), r.Header.Get("Cookie"), "/login")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		//nolint:gosec // G710: flow.LogoutUrl comes from Kratos's own CreateBrowserLogoutFlow response, not user input.
		http.Redirect(w, r, flow.LogoutUrl, http.StatusSeeOther)
	})
}
