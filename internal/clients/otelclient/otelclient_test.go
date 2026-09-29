package otelclient_test

import (
	"testing"

	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/DanielKirkwood/unwrap-gift/internal/clients/otelclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

func TestNewDisabled(t *testing.T) {
	t.Parallel()

	build := otelclient.BuildInfo{Name: "unwrap-gift", Version: "test"}

	providers, err := otelclient.New(t.Context(), config.OtelConfig{}, false, build)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	if _, ok := providers.TracerProvider.(tracenoop.TracerProvider); !ok {
		t.Errorf("TracerProvider = %T, want tracenoop.TracerProvider", providers.TracerProvider)
	}
	if _, ok := providers.MeterProvider.(noop.MeterProvider); !ok {
		t.Errorf("MeterProvider = %T, want noop.MeterProvider", providers.MeterProvider)
	}

	if shutdownErr := providers.Shutdown(t.Context()); shutdownErr != nil {
		t.Errorf("Shutdown() error = %v, want nil", shutdownErr)
	}
}

func TestNewEnabled(t *testing.T) {
	t.Parallel()

	cfg := config.OtelConfig{ExporterURL: "http://127.0.0.1:4318"}

	providers, err := otelclient.New(t.Context(), cfg, true, otelclient.BuildInfo{Name: "unwrap-gift", Version: "test"})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	if providers.TracerProvider == nil {
		t.Error("TracerProvider = nil, want non-nil")
	}
	if providers.MeterProvider == nil {
		t.Error("MeterProvider = nil, want non-nil")
	}
	if providers.Shutdown == nil {
		t.Fatal("Shutdown = nil, want non-nil")
	}

	// Shutdown still needs to run to stop the batcher/reader goroutines, but
	// it errors here because cfg.ExporterURL points at nothing reachable in
	// this unit test: it tries a real flush over the network on shutdown.
	// Exercising a successful flush belongs in an integration test against a
	// live collector, not here.
	_ = providers.Shutdown(t.Context())
}
