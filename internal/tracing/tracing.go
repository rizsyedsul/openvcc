// Package tracing wires OpenTelemetry: builds a TracerProvider that exports
// to an OTLP/HTTP collector, sets the global propagator to W3C Trace Context,
// and returns a Shutdown func the engine calls on graceful stop.
package tracing

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/syedsumx/openvcc/internal/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Shutdown gracefully drains spans. Always safe to call even if Init was a no-op.
type Shutdown func(context.Context) error

// Init builds and registers a TracerProvider from cfg. If cfg is nil, returns
// a no-op Shutdown so callers can defer it unconditionally.
func Init(ctx context.Context, cfg *config.Tracing, version string) (Shutdown, error) {
	if cfg == nil {
		return func(context.Context) error { return nil }, nil
	}

	endpoint, insecure, err := parseEndpoint(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint),
	}
	if insecure || cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(cfg.Headers))
	}

	exporter, err := otlptrace.New(ctx, otlptracehttp.NewClient(opts...))
	if err != nil {
		return nil, fmt.Errorf("otlp trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(version),
		),
		resource.WithProcess(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithMaxExportBatchSize(256)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SamplerRatio)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(ctx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return tp.Shutdown(shutdownCtx)
	}, nil
}

// parseEndpoint accepts http://host:port, https://host:port, or host:port and
// returns the host:port and whether to use insecure transport.
func parseEndpoint(s string) (string, bool, error) {
	if s == "" {
		return "", false, fmt.Errorf("empty endpoint")
	}
	if !strings.Contains(s, "://") {
		return s, false, nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", false, fmt.Errorf("parse endpoint: %w", err)
	}
	host := u.Host
	insecure := strings.EqualFold(u.Scheme, "http")
	return host, insecure, nil
}
