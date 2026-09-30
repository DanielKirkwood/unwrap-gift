package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/riandyrn/otelchi"
	"go.opentelemetry.io/otel/trace"
)

const requestTimeout = 30 * time.Second

// RouterDeps holds everything a router constructor needs. It's built once
// in app and passed to each of NewPublicRouter/NewProtectedRouter/
// NewHiddenRouter — no router reaches for a package-level dependency.
type RouterDeps struct {
	Logger         *slog.Logger
	TracerProvider trace.TracerProvider
	Ready          ReadyCheck

	// Auth, when non-nil, is applied to the protected and hidden routers'
	// routes (not /health/*). It is left nil when the kratos feature is
	// disabled, matching this codebase's disabled-is-a-no-op convention.
	Auth func(http.Handler) http.Handler

	// Authz, when non-nil, is applied to the hidden router's routes, after
	// Auth. It is left nil when the keto feature is disabled.
	Authz func(http.Handler) http.Handler

	// Identities and IdentitiesAdapter, when non-nil, mount the admin
	// identity CRUD example endpoints on the hidden router. Both are left
	// nil when the kratos feature is disabled.
	Identities        IdentityAdmin
	IdentitiesAdapter Adapter

	// CourierWebhookAuth, CourierSMS, and CourierAdapter, when CourierSMS is
	// non-nil, mount the Kratos courier's outgoing-SMS webhook on the hidden
	// router, in its own route group (NOT behind Auth/Authz — the caller is
	// Kratos itself). All three are set only when the kratos feature is
	// enabled.
	CourierWebhookAuth func(http.Handler) http.Handler
	CourierSMS         SMSSender
	CourierAdapter     Adapter

	// Widgets and WidgetsAdapter, when non-nil, mount the widget CRUD
	// example endpoints on the protected router. Both are left nil when the
	// database feature is disabled.
	Widgets        WidgetStore
	WidgetsAdapter Adapter
}

// NewPublicRouter builds the public router: the externally-facing surface.
func NewPublicRouter(deps RouterDeps) *chi.Mux {
	return newRouter("public", deps)
}

// NewProtectedRouter builds the protected router: authenticated but
// non-admin endpoints. Every route other than /health/* runs behind
// deps.Auth — the widget CRUD example endpoints are the first (and so far
// only) routes mounted here.
func NewProtectedRouter(deps RouterDeps) *chi.Mux {
	r := newRouter("protected", deps)

	r.Group(func(r chi.Router) {
		if deps.Auth != nil {
			r.Use(deps.Auth)
		}
		if deps.Widgets != nil {
			MountWidgets(r, deps.Widgets, deps.WidgetsAdapter)
		}
	})

	return r
}

// NewHiddenRouter builds the hidden router: admin-only endpoints, meant to
// be bound to an interface not exposed to the public internet. Every route
// other than /health/* also runs behind deps.Auth, then deps.Authz — the
// admin identity CRUD endpoints require both a valid session and the
// "manage" relation on the Identities:admin object (see deploy/keto).
func NewHiddenRouter(deps RouterDeps) *chi.Mux {
	r := newRouter("hidden", deps)

	r.Group(func(r chi.Router) {
		if deps.Auth != nil {
			r.Use(deps.Auth)
		}
		if deps.Authz != nil {
			r.Use(deps.Authz)
		}
		if deps.Identities != nil {
			MountIdentities(r, deps.Identities, deps.IdentitiesAdapter)
		}
	})

	r.Group(func(r chi.Router) {
		if deps.CourierWebhookAuth != nil {
			r.Use(deps.CourierWebhookAuth)
		}
		if deps.CourierSMS != nil {
			MountCourierWebhook(r, deps.CourierSMS, deps.CourierAdapter)
		}
	})

	return r
}

func newRouter(name string, deps RouterDeps) *chi.Mux {
	r := chi.NewRouter()

	r.NotFound(problemHandler(Problem{Status: http.StatusNotFound, Title: "Not Found"}))
	r.MethodNotAllowed(problemHandler(Problem{Status: http.StatusMethodNotAllowed, Title: "Method Not Allowed"}))

	r.Use(middleware.RequestID)
	r.Use(RequestLogger(deps.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(requestTimeout))
	r.Use(EnforceJSON)
	r.Use(SecurityHeaders)
	r.Use(otelchi.Middleware(name, otelchi.WithChiRoutes(r), otelchi.WithTracerProvider(deps.TracerProvider)))

	MountHealth(r, deps.Ready)

	return r
}

// problemHandler returns an [http.HandlerFunc] that always writes p, for
// use as a router's NotFound/MethodNotAllowed handler.
func problemHandler(p Problem) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_ = WriteProblem(w, p)
	}
}
