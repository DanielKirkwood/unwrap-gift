package api

import (
	"context"
	"net/http"
)

// PermissionChecker resolves whether a subject holds a relation on a
// namespace/object pair, the way Keto's read API's /relation-tuples/check
// endpoint does. Implemented by internal/clients/ketoclient.Client; api
// depends on this interface instead of that concrete type so it never
// imports internal/clients, per ARCHITECTURE.md's dependency-direction
// rule.
type PermissionChecker interface {
	CheckPermission(ctx context.Context, namespace, object, relation, subjectID string) (bool, error)
}

// forbiddenProblem is returned by AuthorizationMiddleware for any request
// whose identity doesn't hold the required relation.
func forbiddenProblem() Problem {
	return Problem{
		Status: http.StatusForbidden,
		Title:  "Forbidden",
		Detail: "the authenticated identity is not permitted to perform this action",
	}
}

// AuthorizationMiddleware requires the identity AuthenticationMiddleware
// stored on the request context (via IdentityFromContext) to hold relation
// on the namespace/object pair, checked via checker. A nil checker (the
// keto feature disabled) makes it a no-op passthrough, mirroring
// AuthenticationMiddleware's nil-validator convention — app only ever
// constructs it with a nil checker in tests, never through the real wiring
// path, where a disabled keto feature simply leaves RouterDeps.Authz unset
// instead.
//
// It must run after AuthenticationMiddleware in the middleware chain, since
// it relies on that middleware having already resolved and stored the
// identity.
func AuthorizationMiddleware(
	checker PermissionChecker,
	namespace, object, relation string,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if checker == nil {
				next.ServeHTTP(w, r)
				return
			}

			identity, ok := IdentityFromContext(r.Context())
			if !ok {
				_ = WriteProblem(w, unauthorizedProblem())
				return
			}

			allowed, err := checker.CheckPermission(r.Context(), namespace, object, relation, identity.Id)
			if err != nil || !allowed {
				_ = WriteProblem(w, forbiddenProblem())
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
