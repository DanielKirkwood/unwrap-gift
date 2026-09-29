package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
	"github.com/DanielKirkwood/unwrap-gift/internal/clients/kratosclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

const (
	requestHeaderTimeout = 10 * time.Second
	shutdownTimeout      = 10 * time.Second
)

// Servers holds the three independently-bound HTTP servers: public,
// protected, and hidden. Each wraps its own *chi.Mux (built in internal/api)
// with otelhttp for request metrics.
type Servers struct {
	Public    *http.Server
	Protected *http.Server
	Hidden    *http.Server
}

// BuildServers constructs the three routers and the [http.Server] each is
// bound to, using a's already-built Logger/Otel/Registry. It does not start
// them — that's cmd/start.go's job.
//
// Each optional api.RouterDeps field (Auth, Authz, Widgets/WidgetsAdapter,
// Identities/IdentitiesAdapter) is wired only when its backing feature
// (kratos, keto, database) is enabled in a.Registry; otherwise it's left
// nil, and the router constructors in internal/api skip the corresponding
// middleware/routes.
func BuildServers(a *App) (*Servers, error) {
	serviceFeature, _ := a.Registry.Feature("service")
	svcCfg, _ := serviceFeature.Config.(config.ServiceConfig)

	deps := api.RouterDeps{
		Logger:         a.Logger,
		TracerProvider: a.Otel.TracerProvider,
		Ready:          a.Store.Ping,
	}

	kratosFeature, _ := a.Registry.Feature("kratos")
	if kratosFeature.Enabled {
		deps.Auth = api.AuthenticationMiddleware(a.Kratos)
		deps.Identities = a.Kratos
		deps.IdentitiesAdapter = api.Adapter{
			Logger: a.Logger,
			ErrorsMap: api.ErrorsMap{
				{
					Match: kratosclient.ErrIdentityNotFound,
					Problem: api.Problem{
						Status: http.StatusNotFound,
						Title:  "Not Found",
						Detail: "identity not found",
					},
				},
			},
		}
	}

	ketoFeature, _ := a.Registry.Feature("keto")
	if ketoFeature.Enabled {
		// The hidden router's admin identity CRUD endpoints are the
		// concrete resource Phase 6 protects. See deploy/keto/identities.ts
		// for the OPL namespace this namespace/object/relation triple
		// checks against.
		deps.Authz = api.AuthorizationMiddleware(a.Keto, "Identities", "admin", "manage")
	}

	dbFeature, _ := a.Registry.Feature("database")
	if dbFeature.Enabled {
		// The protected router's widget CRUD endpoints are Phase 7's
		// example resource: sqlc-backed persistence behind Auth only (no
		// authz), the one combination the hidden-router identity example
		// doesn't already cover.
		deps.Widgets = storeWidgets{store: a.Store}
		deps.WidgetsAdapter = api.Adapter{
			Logger: a.Logger,
			ErrorsMap: api.ErrorsMap{
				{
					Match: api.ErrWidgetNotFound,
					Problem: api.Problem{
						Status: http.StatusNotFound,
						Title:  "Not Found",
						Detail: "widget not found",
					},
				},
			},
		}
	}

	return &Servers{
		Public:    newServer(svcCfg.PublicPort, "public", api.NewPublicRouter(deps), a),
		Protected: newServer(svcCfg.ProtectedPort, "protected", api.NewProtectedRouter(deps), a),
		Hidden:    newServer(svcCfg.HiddenPort, "hidden", api.NewHiddenRouter(deps), a),
	}, nil
}

// Shutdown gracefully stops all three servers in parallel, each bounded by
// shutdownTimeout, and joins any errors.
func (s *Servers) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	errs := make(chan error, 3) //nolint:mnd // one error slot per server

	for _, srv := range []*http.Server{s.Public, s.Protected, s.Hidden} {
		go func(srv *http.Server) { errs <- srv.Shutdown(ctx) }(srv)
	}

	var joined error
	for range 3 {
		joined = errors.Join(joined, <-errs)
	}

	return joined
}

func newServer(port, name string, handler http.Handler, a *App) *http.Server {
	wrapped := otelhttp.NewHandler(handler, name, otelhttp.WithMeterProvider(a.Otel.MeterProvider))

	return &http.Server{
		Addr:              ":" + port,
		Handler:           wrapped,
		ReadHeaderTimeout: requestHeaderTimeout,
	}
}
