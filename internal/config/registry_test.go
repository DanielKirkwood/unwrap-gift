package config_test

import (
	"errors"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

func featureState(t *testing.T, r *config.Registry, name string) config.Feature {
	t.Helper()
	f, ok := r.Feature(name)
	if !ok {
		t.Fatalf("feature %q not registered", name)
	}
	return f
}

func TestResolveFeatureEnabledState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		env         config.EnvVars
		wantEnabled map[string]bool
		wantReason  map[string]string // only checked when non-empty
	}{
		{
			name: "only service enabled by default",
			env:  config.EnvVars{},
			wantEnabled: map[string]bool{
				"service": true, "database": false, "otel": false,
				"kratos": false, "keto": false,
			},
		},
		{
			name: "database enabled once its env is set",
			env:  config.EnvVars{DatabasePath: "/tmp/app.db"},
			wantEnabled: map[string]bool{
				"service": true, "database": true,
			},
		},
		{
			name: "keto enabled once both its own env and kratos are satisfied",
			env: config.EnvVars{
				KratosPublicURL:            "http://kratos.public",
				KratosAdminURL:             "http://kratos.admin",
				KratosCourierWebhookSecret: "test-webhook-secret",
				KetoReadURL:                "http://keto.read",
				KetoWriteURL:               "http://keto.write",
			},
			wantEnabled: map[string]bool{"kratos": true, "keto": true},
		},
		{
			name: "keto disabled when its own env is missing, even with kratos enabled",
			env: config.EnvVars{
				KratosPublicURL:            "http://kratos.public",
				KratosAdminURL:             "http://kratos.admin",
				KratosCourierWebhookSecret: "test-webhook-secret",
			},
			wantEnabled: map[string]bool{"kratos": true, "keto": false},
			wantReason:  map[string]string{"keto": "missing required env: KetoReadURL"},
		},
		{
			name: "kratos disabled when the courier webhook secret is missing, even with its other env set",
			env: config.EnvVars{
				KratosPublicURL: "http://kratos.public",
				KratosAdminURL:  "http://kratos.admin",
			},
			wantEnabled: map[string]bool{"kratos": false},
			wantReason:  map[string]string{"kratos": "missing required env: KratosCourierWebhookSecret"},
		},
		{
			name: "keto disabled when kratos is disabled, even with its own env set",
			env: config.EnvVars{
				KetoReadURL:  "http://keto.read",
				KetoWriteURL: "http://keto.write",
			},
			wantEnabled: map[string]bool{"kratos": false, "keto": false},
			wantReason:  map[string]string{"keto": "requires disabled feature: kratos"},
		},
		{
			name: "DISABLE_FEATURES forces a feature off despite satisfied requirements",
			env: config.EnvVars{
				KratosPublicURL:            "http://kratos.public",
				KratosAdminURL:             "http://kratos.admin",
				KratosCourierWebhookSecret: "test-webhook-secret",
				KetoReadURL:                "http://keto.read",
				KetoWriteURL:               "http://keto.write",
				DisableFeatures:            "kratos",
			},
			wantEnabled: map[string]bool{"kratos": false, "keto": false},
			wantReason: map[string]string{
				"kratos": "disabled via DISABLE_FEATURES",
				"keto":   "requires disabled feature: kratos",
			},
		},
		{
			name: "sms enabled once both seven.io vars are set",
			env: config.EnvVars{
				SevenAPIKey:   "test-key",
				SevenSenderID: "TestSender",
			},
			wantEnabled: map[string]bool{"sms": true},
		},
		{
			name:        "sms disabled when sender ID is missing, even with api key set",
			env:         config.EnvVars{SevenAPIKey: "test-key"},
			wantEnabled: map[string]bool{"sms": false},
			wantReason:  map[string]string{"sms": "missing required env: SevenSenderID"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := config.DefaultRegistry()
			if err := r.ResolveFeatureEnabledState(tt.env); err != nil {
				t.Fatalf("ResolveFeatureEnabledState() error = %v", err)
			}

			for name, want := range tt.wantEnabled {
				got := featureState(t, r, name)
				if got.Enabled != want {
					t.Errorf("feature %q enabled = %v, want %v (reason: %q)", name, got.Enabled, want, got.Reason)
				}
			}
			for name, want := range tt.wantReason {
				got := featureState(t, r, name)
				if got.Reason != want {
					t.Errorf("feature %q reason = %q, want %q", name, got.Reason, want)
				}
			}
		})
	}
}

func TestConfigure(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{DatabasePath: "/tmp/app.db"}

	r := config.DefaultRegistry()
	if err := r.ResolveFeatureEnabledState(env); err != nil {
		t.Fatalf("ResolveFeatureEnabledState() error = %v", err)
	}
	if err := r.Configure(env); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	db := featureState(t, r, "database")
	cfg, ok := db.Config.(config.DatabaseConfig)
	if !ok {
		t.Fatalf("database Config is %T, want config.DatabaseConfig", db.Config)
	}
	if cfg.Path != "/tmp/app.db" {
		t.Errorf("database Config.Path = %q, want %q", cfg.Path, "/tmp/app.db")
	}

	otel := featureState(t, r, "otel")
	if otel.Config != nil {
		t.Errorf("disabled feature otel got Config = %+v, want nil (never built)", otel.Config)
	}
}

func TestValidateReadiness(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")

	r := config.NewRegistry()
	r.Register(config.Feature{
		Name:     "ok",
		Build:    func(config.EnvVars) any { return "ok-config" },
		Validate: func(any) error { return nil },
	})
	r.Register(config.Feature{
		Name:     "broken",
		Build:    func(config.EnvVars) any { return "broken-config" },
		Validate: func(any) error { return errBoom },
	})

	if err := r.ResolveFeatureEnabledState(config.EnvVars{}); err != nil {
		t.Fatalf("ResolveFeatureEnabledState() error = %v", err)
	}
	if err := r.Configure(config.EnvVars{}); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	err := r.ValidateReadiness()
	if err == nil {
		t.Fatal("ValidateReadiness() error = nil, want an error from the broken feature")
	}
	if !errors.Is(err, errBoom) {
		t.Errorf("ValidateReadiness() error = %v, want it to wrap %v", err, errBoom)
	}
}
