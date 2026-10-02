package config

// Feature config types hold each optional feature's settings, derived from
// EnvVars by the Build func each Feature registers in DefaultRegistry.
// Registry.Configure calls Build once ResolveFeatureEnabledState has
// confirmed the feature is enabled, storing the result in Feature.Config.
// Consumers (db.New, otelclient.New, kratosclient.New, ketoclient.New) take
// the typed config instead of EnvVars, so they never see fields for
// features they don't own.

// kratosFeatureName names the "kratos" feature, referenced by both its own
// Feature.Name and other features' Requires lists (keto, web) — a shared
// const instead of repeating the "kratos" string literal three times.
const kratosFeatureName = "kratos"

// ServiceConfig configures the three HTTP server ports. It's the one
// feature DefaultRegistry registers with no RequiredEnv, so
// ResolveFeatureEnabledState always enables it.
type ServiceConfig struct {
	PublicPort    string
	ProtectedPort string
	HiddenPort    string
}

// DatabaseConfig configures the SQLite connection db.New opens. It's only
// built once the "database" feature is enabled, i.e. once DatabasePath is
// set (see DefaultRegistry's RequiredEnv).
type DatabaseConfig struct {
	Path string
}

// OtelConfig configures the OpenTelemetry SDK bootstrap otelclient.New
// performs. It's only built once the "otel" feature is enabled, i.e. once
// OtelExporterURL is set.
type OtelConfig struct {
	ExporterURL string
}

// KratosConfig configures the Ory Kratos client kratosclient.New builds.
// It's only built once the "kratos" feature is enabled, i.e. once
// KratosPublicURL, KratosAdminURL, and KratosCourierWebhookSecret are all
// set. CourierWebhookSecret also configures
// api.CourierWebhookAuthMiddleware, which protects the courier webhook
// mounted on the hidden router only when the kratos feature is enabled
// (see internal/app/servers.go). BrowserURL is the browser-reachable
// Kratos public URL, consumed only by internal/web's login redirect —
// distinct from PublicURL because in production the latter is a
// container-internal hostname the browser can never reach.
type KratosConfig struct {
	PublicURL            string
	AdminURL             string
	CourierWebhookSecret string
	BrowserURL           string
}

// WebConfig configures the "web" router/server (internal/web). It's only
// built once the "web" feature is enabled, i.e. once its Requires
// dependencies ("kratos" and "database") are both enabled — it declares no
// RequiredEnv of its own, since WebPort always has a default (matching
// PublicPort/ProtectedPort/HiddenPort) and login no longer needs a
// separately-configured URL now that it's self-hosted rather than
// delegated to an external UI.
type WebConfig struct {
	Port string
}

// KetoConfig configures the Ory Keto client ketoclient.New builds. It's
// only built once the "keto" feature is enabled, i.e. once KetoReadURL and
// KetoWriteURL are set and its Requires dependency, "kratos", is also
// enabled (see DefaultRegistry).
type KetoConfig struct {
	ReadURL  string
	WriteURL string
}

// SMSConfig configures the seven.io client smsclient.New builds. It's
// only built once the "sms" feature is enabled, i.e. once both
// SevenAPIKey and SevenSenderID are set. Unlike KratosConfig/KetoConfig,
// a disabled "sms" feature does not mean smsclient.Client is nil — see
// smsclient.New's doc comment.
type SMSConfig struct {
	APIKey   string
	SenderID string
}

// DefaultRegistry returns a Registry with every known feature registered,
// unresolved (call ResolveFeatureEnabledState, then Configure, then
// ValidateReadiness on the result).
func DefaultRegistry() *Registry {
	r := NewRegistry()

	r.Register(Feature{
		Name: "service",
		Build: func(e EnvVars) any {
			return ServiceConfig{
				PublicPort:    e.PublicPort,
				ProtectedPort: e.ProtectedPort,
				HiddenPort:    e.HiddenPort,
			}
		},
	})

	r.Register(Feature{
		Name:        "database",
		RequiredEnv: []string{"DatabasePath"},
		Build: func(e EnvVars) any {
			return DatabaseConfig{Path: e.DatabasePath}
		},
	})

	r.Register(Feature{
		Name:        "otel",
		RequiredEnv: []string{"OtelExporterURL"},
		Build: func(e EnvVars) any {
			return OtelConfig{ExporterURL: e.OtelExporterURL}
		},
	})

	r.Register(Feature{
		Name:        kratosFeatureName,
		RequiredEnv: []string{"KratosPublicURL", "KratosAdminURL", "KratosCourierWebhookSecret"},
		Build: func(e EnvVars) any {
			browserURL := e.KratosBrowserURL
			if browserURL == "" {
				browserURL = e.KratosPublicURL
			}

			return KratosConfig{
				PublicURL:            e.KratosPublicURL,
				AdminURL:             e.KratosAdminURL,
				CourierWebhookSecret: e.KratosCourierWebhookSecret,
				BrowserURL:           browserURL,
			}
		},
	})

	r.Register(Feature{
		Name:        "keto",
		RequiredEnv: []string{"KetoReadURL", "KetoWriteURL"},
		Requires:    []string{kratosFeatureName},
		Build: func(e EnvVars) any {
			return KetoConfig{ReadURL: e.KetoReadURL, WriteURL: e.KetoWriteURL}
		},
	})

	r.Register(Feature{
		Name:        "sms",
		RequiredEnv: []string{"SevenAPIKey", "SevenSenderID"},
		Build: func(e EnvVars) any {
			return SMSConfig{APIKey: e.SevenAPIKey, SenderID: e.SevenSenderID}
		},
	})

	r.Register(Feature{
		Name:     "web",
		Requires: []string{kratosFeatureName, "database"},
		Build: func(e EnvVars) any {
			return WebConfig{Port: e.WebPort}
		},
	})

	return r
}
