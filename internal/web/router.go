package web

import (
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/riandyrn/otelchi"
	"go.opentelemetry.io/otel/trace"
)

const requestTimeout = 30 * time.Second

// RouterDeps holds everything NewWebRouter needs. Auth, LoginFlows, and
// WishlistItems are left nil when the "web" feature is disabled, matching
// this codebase's disabled-is-a-no-op convention (internal/api.RouterDeps).
type RouterDeps struct {
	Logger         *slog.Logger
	TracerProvider trace.TracerProvider

	// Auth, when non-nil, guards /wishlist*'s route group. It is never
	// applied to /login — that's what Auth redirects unauthenticated
	// requests TO, so applying it there would loop.
	Auth func(http.Handler) http.Handler

	// LoginFlows and KratosBrowserURL, when LoginFlows is non-nil, mount
	// GET /login.
	LoginFlows       LoginFlowProvider
	KratosBrowserURL string

	// WishlistItems, when non-nil, mounts the wishlist page and its htmx
	// CRUD endpoints behind the Auth group.
	WishlistItems WishlistItemStore

	Templates *template.Template
}

// NewWebRouter builds the fourth router: the browser-facing login + wishlist
// UI. Unlike internal/api's three routers, it does NOT apply EnforceJSON
// (this router serves text/html and accepts form-urlencoded POSTs) and has
// no /health/* endpoint (see the plan's NOT Building section).
func NewWebRouter(deps RouterDeps) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(RequestLogger(deps.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(requestTimeout))
	r.Use(SecurityHeaders)
	r.Use(otelchi.Middleware("web", otelchi.WithChiRoutes(r), otelchi.WithTracerProvider(deps.TracerProvider)))

	if deps.LoginFlows != nil {
		MountLogin(r, deps.LoginFlows, deps.KratosBrowserURL, deps.Templates)
	}

	r.Group(func(r chi.Router) {
		if deps.Auth != nil {
			r.Use(deps.Auth)
		}
		if deps.WishlistItems != nil {
			adapter := Adapter{Logger: deps.Logger, Templates: deps.Templates}
			MountWishlist(r, deps.WishlistItems, deps.Templates, adapter)
		}
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/wishlist", http.StatusSeeOther)
	})

	return r
}
