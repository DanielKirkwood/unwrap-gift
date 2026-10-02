package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	kratos "github.com/ory/kratos-client-go/v26"
)

// errMissingIdentity and errMissingPhoneTrait are invariant violations, not
// expected 4xx conditions: a handler reaching either case means
// AuthenticationMiddleware ran but didn't populate the context (shouldn't
// happen) or the identity lacks a "phone" trait despite
// identity.schema.json requiring it (shouldn't happen either). Adapter's
// generic error.html/500 path is the right outcome for both — mirrors
// internal/api's errMissingPhoneTrait.
var (
	errMissingIdentity   = errors.New("web: identity missing from context")
	errMissingPhoneTrait = errors.New("web: identity missing phone trait")
)

// identityContextKey is unexported so no other package can collide with
// this key, mirroring internal/api/auth.go's identityContextKey.
type identityContextKey struct{}

// withIdentity returns a copy of ctx carrying identity, retrievable via
// IdentityFromContext.
func withIdentity(ctx context.Context, identity *kratos.Identity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, identity)
}

// IdentityFromContext returns the Kratos identity AuthenticationMiddleware
// stored on ctx, if any.
func IdentityFromContext(ctx context.Context) (*kratos.Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(*kratos.Identity)
	return identity, ok
}

// SessionValidator resolves a request's Cookie header into a Kratos
// session, the way Kratos's public ToSession endpoint does. Implemented by
// internal/clients/kratosclient.Client; web depends on this interface
// instead of that concrete type so it never imports internal/clients, per
// ARCHITECTURE.md's dependency-direction rule — a separate declaration from
// internal/api's identically-shaped SessionValidator, since internal/web
// must not import internal/api either.
type SessionValidator interface {
	ToSession(ctx context.Context, cookieHeader string) (*kratos.Session, error)
}

// AuthenticationMiddleware validates the request's session cookie via
// validator and, on success, stores the resolved identity on the request
// context (retrievable with IdentityFromContext) before calling next. A nil
// validator (the web feature disabled) makes it a no-op passthrough.
//
// Unlike internal/api's AuthenticationMiddleware, failure redirects to our
// own /login page (relative, same-origin — no absolute-URL construction
// needed here) instead of writing a 401 Problem response: this middleware
// guards browser-facing HTML pages, not a JSON API. /login must never be
// mounted behind this middleware, or the redirect loops.
func AuthenticationMiddleware(validator SessionValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if validator == nil {
				next.ServeHTTP(w, r)
				return
			}

			cookie := r.Header.Get("Cookie")
			if cookie != "" {
				session, err := validator.ToSession(r.Context(), cookie)
				if err == nil && session.Identity != nil {
					next.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), session.Identity)))
					return
				}
			}

			returnTo := url.QueryEscape(r.URL.RequestURI())
			http.Redirect(w, r, "/login?return_to="+returnTo, http.StatusSeeOther)
		})
	}
}
