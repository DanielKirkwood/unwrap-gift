// Package config holds the EnvVars struct and the feature registry: the
// single source of runtime configuration for the app.
package config

import (
	"reflect"
	"strings"

	"github.com/spf13/viper"
)

// EnvVars is the full set of environment-driven settings for the app. Every
// field must be a string and carry an `env` tag naming the environment
// variable it binds to; an optional `default` tag supplies a fallback.
//
// Load() binds and populates this struct via reflection, so adding a field
// here is sufficient to wire up a new setting — no binding code to update.
type EnvVars struct {
	Env      string `env:"ENV"       default:"development"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`

	PublicPort    string `env:"PUBLIC_PORT"    default:"8080"`
	ProtectedPort string `env:"PROTECTED_PORT" default:"8081"`
	HiddenPort    string `env:"HIDDEN_PORT"    default:"8082"`

	DatabasePath string `env:"DATABASE_PATH"`

	OtelExporterURL string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`

	KratosPublicURL string `env:"KRATOS_PUBLIC_URL"`
	KratosAdminURL  string `env:"KRATOS_ADMIN_URL"`

	KetoReadURL  string `env:"KETO_READ_URL"`
	KetoWriteURL string `env:"KETO_WRITE_URL"`

	SevenAPIKey   string `env:"SEVEN_API_KEY"`
	SevenSenderID string `env:"SEVEN_SENDER_ID"`

	// KetoSeedAdminIdentityID is the Kratos identity ID `db seed` grants
	// the Role:admin relation tuple to. Not part of KetoConfig/the keto
	// feature's readiness — it's only consumed directly by `db seed`, the
	// same way DisableFeatures is read straight off EnvVars rather than
	// through a Feature.
	KetoSeedAdminIdentityID string `env:"KETO_SEED_ADMIN_IDENTITY_ID"`

	// DisableFeatures is a comma-separated, case-insensitive list of feature
	// names (e.g. "otel,keto") to force-disable even when their RequiredEnv
	// is present. Registry.ResolveFeatureEnabledState reads it straight off
	// EnvVars rather than through a Feature's Config.
	DisableFeatures string `env:"DISABLE_FEATURES"`
}

// Load reads EnvVars from the environment. Each field is bound to its `env`
// tag's variable name and, if unset, falls back to its `default` tag.
func Load() (EnvVars, error) {
	v := viper.New()
	v.AutomaticEnv()

	for field := range reflect.TypeFor[EnvVars]().Fields() {
		envName := field.Tag.Get("env")
		if envName == "" {
			continue
		}

		key := strings.ToLower(field.Name)
		if err := v.BindEnv(key, envName); err != nil {
			return EnvVars{}, err
		}
		if def, ok := field.Tag.Lookup("default"); ok {
			v.SetDefault(key, def)
		}
	}

	var envVars EnvVars
	if err := v.Unmarshal(&envVars); err != nil {
		return EnvVars{}, err
	}

	return envVars, nil
}
