package config

// Feature config types hold each optional feature's settings, derived from
// EnvVars by the Build func each Feature registers in DefaultRegistry.
// Registry.Configure calls Build once ResolveFeatureEnabledState has
// confirmed the feature is enabled, storing the result in Feature.Config.
// Consumers (db.New, otelclient.New, kratosclient.New, ketoclient.New) take
// the typed config instead of EnvVars, so they never see fields for
// features they don't own.

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
// It's only built once the "kratos" feature is enabled, i.e. once both
// KratosPublicURL and KratosAdminURL are set.
type KratosConfig struct {
	PublicURL string
	AdminURL  string
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
		Name:        "kratos",
		RequiredEnv: []string{"KratosPublicURL", "KratosAdminURL"},
		Build: func(e EnvVars) any {
			return KratosConfig{PublicURL: e.KratosPublicURL, AdminURL: e.KratosAdminURL}
		},
	})

	r.Register(Feature{
		Name:        "keto",
		RequiredEnv: []string{"KetoReadURL", "KetoWriteURL"},
		Requires:    []string{"kratos"},
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

	return r
}
