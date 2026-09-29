package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"

	"github.com/DanielKirkwood/unwrap-gift/internal/clients/ketoclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/clients/kratosclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/clients/logger"
	"github.com/DanielKirkwood/unwrap-gift/internal/clients/otelclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
	"github.com/DanielKirkwood/unwrap-gift/internal/db"
)

// App holds every dependency constructed at startup: resolved config, the
// feature registry, the logger, and the otel providers. Bootstrap builds
// one; nothing here is a package-level singleton.
//
// Servers is nil until a command that serves HTTP (start and its
// subcommands) calls BuildServers and assigns the result — commands like
// `info` never need routers built.
//
// Store, Kratos, and Keto are nil when their feature is disabled, mirroring
// their constructors' (db.New, kratosclient.New, ketoclient.New)
// disabled-is-nil convention. Store's methods are nil-receiver safe since
// Store is used unconditionally elsewhere; Kratos and Keto are not — they're
// only wired into api.RouterDeps by BuildServers once their feature is known
// to be enabled, so nothing ever calls them while nil.
type App struct {
	Env      config.EnvVars
	Registry *config.Registry
	Logger   *slog.Logger
	Otel     *otelclient.Providers
	Store    *db.Store
	Kratos   *kratosclient.Client
	Keto     *ketoclient.Client
	Servers  *Servers
}

// Bootstrap resolves env's feature-enabled state and configures every
// enabled feature, then constructs the logger and otel providers from the
// result. It deliberately does not call Registry.ValidateReadiness: that
// stays an opt-in step for commands (such as `info features`) that need
// full readiness, so read-only commands keep working even if a feature is
// misconfigured.
func Bootstrap(ctx context.Context, env config.EnvVars) (*App, error) {
	registry := config.DefaultRegistry()
	if err := registry.ResolveFeatureEnabledState(env); err != nil {
		return nil, fmt.Errorf("app: resolve features: %w", err)
	}
	if err := registry.Configure(env); err != nil {
		return nil, fmt.Errorf("app: configure features: %w", err)
	}

	log, err := logger.New(logger.Config{
		Dev:   env.Env == "development",
		Level: env.LogLevel,
	})
	if err != nil {
		return nil, fmt.Errorf("app: build logger: %w", err)
	}

	otelFeature, _ := registry.Feature("otel")
	otelCfg, _ := otelFeature.Config.(config.OtelConfig)

	providers, err := otelclient.New(ctx, otelCfg, otelFeature.Enabled, buildInfo())
	if err != nil {
		return nil, fmt.Errorf("app: bootstrap otel: %w", err)
	}

	dbFeature, _ := registry.Feature("database")
	dbCfg, _ := dbFeature.Config.(config.DatabaseConfig)

	store, err := db.New(dbCfg, dbFeature.Enabled, providers.TracerProvider, providers.MeterProvider)
	if err != nil {
		return nil, fmt.Errorf("app: bootstrap database: %w", err)
	}

	kratosFeature, _ := registry.Feature("kratos")
	kratosCfg, _ := kratosFeature.Config.(config.KratosConfig)

	kratos, err := kratosclient.New(kratosCfg, kratosFeature.Enabled)
	if err != nil {
		return nil, fmt.Errorf("app: bootstrap kratos: %w", err)
	}

	ketoFeature, _ := registry.Feature("keto")
	ketoCfg, _ := ketoFeature.Config.(config.KetoConfig)

	keto, err := ketoclient.New(ketoCfg, ketoFeature.Enabled)
	if err != nil {
		return nil, fmt.Errorf("app: bootstrap keto: %w", err)
	}

	log.DebugContext(ctx, "bootstrap complete",
		"env", env.Env, "otel_enabled", otelFeature.Enabled, "database_enabled", dbFeature.Enabled,
		"kratos_enabled", kratosFeature.Enabled, "keto_enabled", ketoFeature.Enabled)

	return &App{
		Env: env, Registry: registry, Logger: log, Otel: providers,
		Store: store, Kratos: kratos, Keto: keto,
	}, nil
}

// Shutdown gracefully closes every running server (if any), then the Store,
// then flushes the otel providers. Servers close first so no in-flight
// request's spans/metrics/queries are lost to a premature Store/otel
// shutdown; the Store closes before otel so its instrumentation can still
// flush.
func (a *App) Shutdown(ctx context.Context) error {
	if a.Servers != nil {
		if err := a.Servers.Shutdown(ctx); err != nil {
			return errors.Join(err, a.Store.Close(), a.Otel.Shutdown(ctx))
		}
	}

	return errors.Join(a.Store.Close(), a.Otel.Shutdown(ctx))
}

// Version is set via -ldflags "-X .../internal/app.Version=..." at build
// time (see Dockerfile, .github/workflows/go.yml). It's empty for `go run`/
// `go build` without that flag — buildInfo falls back to
// [debug.ReadBuildInfo]'s VCS-stamped version there, which works for local
// dev but not for a Docker build context: that copies source without .git,
// so VCS stamping produces nothing useful inside a container image.
//
//nolint:gochecknoglobals // deliberate: the only way -ldflags -X can inject a value is into a package-level var.
var Version string

// buildInfo derives the otel resource's service name/version, preferring
// the build-time-injected Version over the running binary's VCS-stamped
// build info (the same source cmd's moduleName() uses for the name).
func buildInfo() otelclient.BuildInfo {
	info, ok := debug.ReadBuildInfo()

	name := "unwrap-gift"
	if ok {
		parts := strings.Split(info.Main.Path, "/")
		name = parts[len(parts)-1]
	}

	version := Version
	if version == "" && ok {
		version = info.Main.Version
	}

	return otelclient.BuildInfo{Name: name, Version: version}
}
