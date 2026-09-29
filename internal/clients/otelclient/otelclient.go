// Package otelclient bootstraps the OpenTelemetry SDK: a TracerProvider and
// MeterProvider exporting over OTLP/HTTP, or no-op providers when the otel
// feature is disabled.
package otelclient

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

// BuildInfo names the service for otel's resource attributes.
type BuildInfo struct {
	Name    string
	Version string
}

// Providers holds the constructed TracerProvider and MeterProvider, plus a
// Shutdown func that flushes and closes both. Providers is safe to use
// whether or not otel is enabled: when disabled, both providers are no-op
// and Shutdown is a no-op too.
type Providers struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Shutdown       func(context.Context) error
}

// New builds Providers from cfg. When enabled is false, cfg is ignored and
// no-op providers are returned.
func New(ctx context.Context, cfg config.OtelConfig, enabled bool, build BuildInfo) (*Providers, error) {
	if !enabled {
		return &Providers{
			TracerProvider: tracenoop.NewTracerProvider(),
			MeterProvider:  metricnoop.NewMeterProvider(),
			Shutdown:       func(context.Context) error { return nil },
		}, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(build.Name),
			semconv.ServiceVersion(build.Version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otelclient: build resource: %w", err)
	}

	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(cfg.ExporterURL))
	if err != nil {
		return nil, fmt.Errorf("otelclient: build trace exporter: %w", err)
	}

	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(cfg.ExporterURL))
	if err != nil {
		return nil, fmt.Errorf("otelclient: build metric exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	shutdown := func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx))
	}

	return &Providers{TracerProvider: tp, MeterProvider: mp, Shutdown: shutdown}, nil
}
