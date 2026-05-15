package otel

import (
	"context"

	gootel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// NewJaegerExporter returns a trace span exporter that ships spans to a
// Jaeger collector via OTLP/HTTP. Modern Jaeger (>=1.35) accepts OTLP
// natively; the legacy go.opentelemetry.io/otel/exporters/jaeger module is
// deprecated and removed in recent releases, so OTLP is the supported path.
// endpoint is the Jaeger collector's OTLP/HTTP address (host:port, e.g.
// "localhost:4318").
func NewJaegerExporter(endpoint string) (sdktrace.SpanExporter, error) {
	return NewOTLPExporter(endpoint)
}

// NewOTLPExporter returns a trace span exporter that ships spans to an
// OTLP/HTTP collector (e.g. the OpenTelemetry Collector) at endpoint
// (host:port, e.g. "localhost:4318"). The connection is plain HTTP; callers
// needing TLS should construct an exporter directly.
func NewOTLPExporter(endpoint string) (sdktrace.SpanExporter, error) {
	return otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
}

// NewPrometheusExporter returns a metric.Reader that exports metrics in
// Prometheus format. Register its Collectors with a Prometheus registry to
// expose them at /metrics.
func NewPrometheusExporter() (sdkmetric.Reader, error) {
	return prometheus.New()
}

// InstallNewProvider installs a TracerProvider that batches spans to exp and
// tags them with service.name=serviceName as the global trace provider. The
// returned cleanup function shuts the provider down (flushing pending spans)
// and should be deferred by the caller.
func InstallNewProvider(exp sdktrace.SpanExporter, serviceName string) func() {
	res := resource.NewWithAttributes("",
		attribute.String("service.name", serviceName),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
		sdktrace.WithResource(res),
	)
	gootel.SetTracerProvider(tp)
	return func() {
		_ = tp.Shutdown(context.Background())
	}
}
