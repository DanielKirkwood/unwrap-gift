package api

import (
	"context"
	"net/http"

	kratos "github.com/ory/kratos-client-go/v26"
)

// identityContextKey is unexported so no other package can collide with
// this key, mirroring cmd/context.go's app-context pattern.
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
// internal/clients/kratosclient.Client; api depends on this interface
// instead of that concrete type so it never imports internal/clients, per
// ARCHITECTURE.md's dependency-direction rule.
type SessionValidator interface {
	ToSession(ctx context.Context, cookieHeader string) (*kratos.Session, error)
}

// unauthorizedProblem is returned by AuthenticationMiddleware for any
// request that doesn't carry a valid session.
func unauthorizedProblem() Problem {
	return Problem{
		Status: http.StatusUnauthorized,
		Title:  "Unauthorized",
		Detail: "a valid session is required",
	}
}

// AuthenticationMiddleware validates the request's session cookie via
// validator and, on success, stores the resolved identity on the request
// context (retrievable with IdentityFromContext) before calling next. A nil
// validator (the kratos feature disabled) makes it a no-op passthrough —
// app only ever constructs it with a nil validator in tests, never through
// the real wiring path, where a disabled kratos feature simply leaves
// RouterDeps.Auth unset instead.
func AuthenticationMiddleware(validator SessionValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if validator == nil {
				next.ServeHTTP(w, r)
				return
			}

			cookie := r.Header.Get("Cookie")
			if cookie == "" {
				_ = WriteProblem(w, unauthorizedProblem())
				return
			}

			session, err := validator.ToSession(r.Context(), cookie)
			if err != nil || session.Identity == nil {
				_ = WriteProblem(w, unauthorizedProblem())
				return
			}

			next.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), session.Identity)))
		})
	}
}
