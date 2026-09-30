package app_test

import (
	"testing"

	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/DanielKirkwood/unwrap-gift/internal/app"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

func TestBootstrapOtelDisabled(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{Env: "development", LogLevel: "debug"}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}

	if a.Logger == nil {
		t.Error("Logger = nil, want non-nil")
	}

	otelFeature, ok := a.Registry.Feature("otel")
	if !ok {
		t.Fatal("otel feature not registered")
	}
	if otelFeature.Enabled {
		t.Error("otel feature Enabled = true, want false (no OTEL_EXPORTER_OTLP_ENDPOINT set)")
	}

	if _, isNoopTracer := a.Otel.TracerProvider.(tracenoop.TracerProvider); !isNoopTracer {
		t.Errorf("TracerProvider = %T, want tracenoop.TracerProvider", a.Otel.TracerProvider)
	}
	if _, isNoopMeter := a.Otel.MeterProvider.(noop.MeterProvider); !isNoopMeter {
		t.Errorf("MeterProvider = %T, want noop.MeterProvider", a.Otel.MeterProvider)
	}

	if shutdownErr := a.Shutdown(t.Context()); shutdownErr != nil {
		t.Errorf("Shutdown() error = %v, want nil", shutdownErr)
	}
}

func TestBootstrapOtelEnabled(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{
		Env:             "production",
		LogLevel:        "info",
		OtelExporterURL: "http://127.0.0.1:4318",
	}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}

	otelFeature, ok := a.Registry.Feature("otel")
	if !ok {
		t.Fatal("otel feature not registered")
	}
	if !otelFeature.Enabled {
		t.Error("otel feature Enabled = false, want true (OTEL_EXPORTER_OTLP_ENDPOINT set)")
	}

	if a.Otel.TracerProvider == nil {
		t.Error("TracerProvider = nil, want non-nil")
	}
	if a.Otel.MeterProvider == nil {
		t.Error("MeterProvider = nil, want non-nil")
	}

	// Not asserting Shutdown() succeeds: it flushes over the network to
	// OtelExporterURL, which points at nothing reachable in this unit test.
	_ = a.Shutdown(t.Context())
}

func TestBootstrapKratosDisabled(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{Env: "development", LogLevel: "debug"}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}

	kratosFeature, ok := a.Registry.Feature("kratos")
	if !ok {
		t.Fatal("kratos feature not registered")
	}
	if kratosFeature.Enabled {
		t.Error("kratos feature Enabled = true, want false (no KRATOS_*_URL set)")
	}
	if a.Kratos != nil {
		t.Errorf("Kratos = %+v, want nil", a.Kratos)
	}
}

func TestBootstrapKratosEnabled(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{
		Env:             "production",
		LogLevel:        "info",
		KratosPublicURL: "http://127.0.0.1:4433",
		KratosAdminURL:  "http://127.0.0.1:4434",
	}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}

	kratosFeature, ok := a.Registry.Feature("kratos")
	if !ok {
		t.Fatal("kratos feature not registered")
	}
	if !kratosFeature.Enabled {
		t.Error("kratos feature Enabled = false, want true (KRATOS_*_URL set)")
	}
	if a.Kratos == nil {
		t.Fatal("Kratos = nil, want non-nil")
	}
	if a.Kratos.Public == nil || a.Kratos.Admin == nil {
		t.Error("Kratos.Public/Admin = nil, want non-nil")
	}
}

func TestBootstrapSMSDisabled(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{Env: "development", LogLevel: "debug"}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}

	smsFeature, ok := a.Registry.Feature("sms")
	if !ok {
		t.Fatal("sms feature not registered")
	}
	if smsFeature.Enabled {
		t.Error("sms feature Enabled = true, want false (no SEVEN_* env set)")
	}
	if a.SMS == nil {
		t.Fatal("SMS = nil, want non-nil (sms client is never nil)")
	}
	if a.SMS.Enabled {
		t.Error("SMS.Enabled = true, want false")
	}
}

func TestBootstrapSMSEnabled(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{
		Env: "production", LogLevel: "info",
		SevenAPIKey: "test-key", SevenSenderID: "TestSender",
	}

	a, err := app.Bootstrap(t.Context(), env)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v, want nil", err)
	}

	smsFeature, ok := a.Registry.Feature("sms")
	if !ok {
		t.Fatal("sms feature not registered")
	}
	if !smsFeature.Enabled {
		t.Error("sms feature Enabled = false, want true (SEVEN_* env set)")
	}
	if a.SMS == nil || !a.SMS.Enabled {
		t.Fatal("SMS.Enabled = false or SMS nil, want enabled non-nil client")
	}
	if a.SMS.Sender == nil {
		t.Error("SMS.Sender = nil, want non-nil")
	}
}

func TestBootstrapUnknownLogLevel(t *testing.T) {
	t.Parallel()

	env := config.EnvVars{Env: "production", LogLevel: "trace"}

	if _, err := app.Bootstrap(t.Context(), env); err == nil {
		t.Fatal("Bootstrap() error = nil, want error for unknown log level")
	}
}
