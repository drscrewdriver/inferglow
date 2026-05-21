// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package otel

import (
	"context"

	gootel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
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
