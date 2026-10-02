package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
	"github.com/DanielKirkwood/unwrap-gift/internal/clients/kratosclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
	"github.com/DanielKirkwood/unwrap-gift/internal/web"
)

const (
	requestHeaderTimeout = 10 * time.Second
	shutdownTimeout      = 10 * time.Second
)

// Problem titles reused across several of BuildServers' ErrorsMap entries.
const (
	titleNotFound = "Not Found"
	titleConflict = "Conflict"
)

// defaultWebPort mirrors internal/config/config.go's WebPort default tag —
// needed here because a disabled "web" feature leaves config.WebConfig at
// its zero value (Build never ran), but Web is always built regardless of
// feature state (matching Public/Protected/Hidden), so it still needs a
// port to bind.
const defaultWebPort = "8083"

// Servers holds the four independently-bound HTTP servers: public,
// protected, hidden, and web. Each wraps its own *chi.Mux (built in
// internal/api or internal/web) with otelhttp for request metrics.
type Servers struct {
	Public    *http.Server
	Protected *http.Server
	Hidden    *http.Server
	Web       *http.Server
}

// BuildServers constructs the three routers and the [http.Server] each is
// bound to, using a's already-built Logger/Otel/Registry. It does not start
// them — that's cmd/start.go's job.
//
// Each optional api.RouterDeps field (Auth, Authz, Widgets/WidgetsAdapter,
// Identities/IdentitiesAdapter, CourierWebhookAuth/CourierSMS/
// CourierAdapter) is wired only when its backing feature (kratos, keto,
// database) is enabled in a.Registry; otherwise it's left nil, and the
// router constructors in internal/api skip the corresponding
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
	kratosCfg, _ := kratosFeature.Config.(config.KratosConfig)
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

		deps.CourierWebhookAuth = api.CourierWebhookAuthMiddleware(kratosCfg.CourierWebhookSecret)
		deps.CourierSMS = a.SMS
		deps.CourierAdapter = api.Adapter{Logger: a.Logger, ErrorsMap: api.ErrorsMap{}}
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
						Title:  titleNotFound,
						Detail: "widget not found",
					},
				},
			},
		}
	}

	if dbFeature.Enabled && kratosFeature.Enabled {
		wireWishlistItems(&deps, a)
	}

	if dbFeature.Enabled && kratosFeature.Enabled && ketoFeature.Enabled {
		wireOrganiserAPI(&deps, a)
	}

	webFeature, _ := a.Registry.Feature("web")
	webCfg, _ := webFeature.Config.(config.WebConfig)
	webDeps, err := buildWebDeps(a, webFeature.Enabled, kratosCfg)
	if err != nil {
		return nil, err
	}

	webPort := webCfg.Port
	if webPort == "" {
		webPort = defaultWebPort
	}

	return &Servers{
		Public:    newServer(svcCfg.PublicPort, "public", api.NewPublicRouter(deps), a),
		Protected: newServer(svcCfg.ProtectedPort, "protected", api.NewProtectedRouter(deps), a),
		Hidden:    newServer(svcCfg.HiddenPort, "hidden", api.NewHiddenRouter(deps), a),
		Web:       newServer(webPort, "web", web.NewWebRouter(webDeps), a),
	}, nil
}

// buildWebDeps constructs internal/web's RouterDeps, left mostly zero-value
// (nil Auth/LoginFlows/WishlistItems) when the "web" feature is disabled —
// NewWebRouter's own nil checks then mount only /health-equivalent routes
// (just the "/" redirect), matching the other three routers'
// disabled-is-a-no-op convention. Split out of BuildServers to keep it
// under the funlen limit.
func buildWebDeps(a *App, enabled bool, kratosCfg config.KratosConfig) (web.RouterDeps, error) {
	deps := web.RouterDeps{Logger: a.Logger, TracerProvider: a.Otel.TracerProvider}
	if !enabled {
		return deps, nil
	}

	templates, err := web.ParseTemplates()
	if err != nil {
		return web.RouterDeps{}, fmt.Errorf("app: parse web templates: %w", err)
	}

	deps.Templates = templates
	deps.Auth = web.AuthenticationMiddleware(a.Kratos)
	deps.LoginFlows = a.Kratos
	deps.KratosBrowserURL = kratosCfg.BrowserURL
	deps.WishlistItems = webStoreWishlistItems{store: a.Store}

	return deps, nil
}

// Shutdown gracefully stops all four servers in parallel, each bounded by
// shutdownTimeout, and joins any errors.
func (s *Servers) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	servers := []*http.Server{s.Public, s.Protected, s.Hidden, s.Web}
	errs := make(chan error, len(servers))

	for _, srv := range servers {
		go func(srv *http.Server) { errs <- srv.Shutdown(ctx) }(srv)
	}

	var joined error
	for range servers {
		joined = errors.Join(joined, <-errs)
	}

	return joined
}

// wireWishlistItems sets deps.WishlistItems/WishlistItemsAdapter — an
// authenticated (not authorized) resource on the protected router, same
// shape as Widgets, except scoped by the caller's own phone trait instead
// of being global. Needs kratos (to resolve identity.Traits["phone"]) but
// not keto (no deps.Authz group involved). Called only when database and
// kratos are both enabled.
func wireWishlistItems(deps *api.RouterDeps, a *App) {
	deps.WishlistItems = storeWishlistItems{store: a.Store}
	deps.WishlistItemsAdapter = api.Adapter{
		Logger: a.Logger,
		ErrorsMap: api.ErrorsMap{
			{
				Match: api.ErrWishlistItemNotFound,
				Problem: api.Problem{
					Status: http.StatusNotFound, Title: titleNotFound, Detail: "wishlist item not found",
				},
			},
		},
	}
}

// wireOrganiserAPI sets deps.Drawers/Members/Relationships/Draws and their
// adapters — Phase 5's organiser API, mounted on the hidden router behind
// the same Auth+Authz group as MountIdentities (reusing the existing
// global admin Keto role, not a new per-drawer authorization scheme — see
// the Phase 5 plan's Decisions table). Called only when database, kratos,
// and keto are all enabled.
func wireOrganiserAPI(deps *api.RouterDeps, a *App) {
	deps.Drawers = storeDrawers{store: a.Store}
	deps.DrawersAdapter = api.Adapter{
		Logger: a.Logger,
		ErrorsMap: api.ErrorsMap{
			{
				Match:   api.ErrDrawerNotFound,
				Problem: api.Problem{Status: http.StatusNotFound, Title: titleNotFound, Detail: "drawer not found"},
			},
		},
	}

	deps.Members = storeMembers{store: a.Store}
	deps.MembersAdapter = api.Adapter{
		Logger: a.Logger,
		ErrorsMap: api.ErrorsMap{
			{
				Match:   api.ErrMemberNotFound,
				Problem: api.Problem{Status: http.StatusNotFound, Title: titleNotFound, Detail: "member not found"},
			},
			{
				Match: api.ErrMemberPhoneAlreadyExists,
				Problem: api.Problem{
					Status: http.StatusConflict, Title: titleConflict,
					Detail: "a member with this phone number already exists in this drawer",
				},
			},
			{
				Match: api.ErrMemberInvalidPhoneNumber,
				Problem: api.Problem{
					Status: http.StatusBadRequest, Title: "Bad Request",
					Detail: "member phone number must be in E.164 format (e.g. +447700900000)",
				},
			},
		},
	}

	deps.Relationships = storeRelationships{store: a.Store}
	deps.RelationshipsAdapter = api.Adapter{
		Logger: a.Logger,
		ErrorsMap: api.ErrorsMap{
			{
				Match: api.ErrRelationshipNotFound,
				Problem: api.Problem{
					Status: http.StatusNotFound,
					Title:  titleNotFound,
					Detail: "relationship not found",
				},
			},
			{
				Match: api.ErrRelationshipSamePhoneNumber,
				Problem: api.Problem{
					Status: http.StatusBadRequest,
					Title:  "Bad Request",
					Detail: "relationship phone numbers must differ",
				},
			},
			{
				Match: api.ErrRelationshipAlreadyExists,
				Problem: api.Problem{
					Status: http.StatusConflict,
					Title:  titleConflict,
					Detail: "relationship already exists for this pair",
				},
			},
		},
	}

	wireDraws(deps, a)
}

// wireDraws sets deps.Draws/DrawsAdapter — split out of wireOrganiserAPI to
// keep that function under the funlen limit now that the notification
// pipeline (Phase 7) adds a fifth ErrorsMap entry.
func wireDraws(deps *api.RouterDeps, a *App) {
	deps.Draws = storeDraws{store: a.Store, sms: a.SMS}
	deps.DrawsAdapter = api.Adapter{
		Logger: a.Logger,
		ErrorsMap: api.ErrorsMap{
			{
				Match:   api.ErrDrawNotFound,
				Problem: api.Problem{Status: http.StatusNotFound, Title: titleNotFound, Detail: "draw not found"},
			},
			{
				Match: api.ErrDrawAlreadyRun,
				Problem: api.Problem{
					Status: http.StatusConflict,
					Title:  titleConflict,
					Detail: "draw has already been run",
				},
			},
			{
				Match: api.ErrNoValidAssignment,
				Problem: api.Problem{
					Status: http.StatusConflict, Title: titleConflict,
					Detail: "no valid assignment exists for this drawer's current members/exclusions/history",
				},
			},
			{
				Match: api.ErrTooFewMembers,
				Problem: api.Problem{
					Status: http.StatusUnprocessableEntity,
					Title:  "Unprocessable Entity",
					Detail: "drawer has fewer than two members",
				},
			},
			{
				Match: api.ErrNotificationFailed,
				Problem: api.Problem{
					Status: http.StatusBadGateway,
					Title:  "Bad Gateway",
					Detail: "draw was assigned but one or more notification SMS messages failed to send; " +
						"assignments are saved and visible via GET .../assignments, but the draw's status " +
						"remains 'assigned' until notifications succeed",
				},
			},
		},
	}
}

func newServer(port, name string, handler http.Handler, a *App) *http.Server {
	wrapped := otelhttp.NewHandler(handler, name, otelhttp.WithMeterProvider(a.Otel.MeterProvider))

	return &http.Server{
		Addr:              ":" + port,
		Handler:           wrapped,
		ReadHeaderTimeout: requestHeaderTimeout,
	}
}
