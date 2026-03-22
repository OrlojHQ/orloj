package telemetry

import (
	"context"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	TracerName = "github.com/OrlojHQ/orloj"
)

// Config holds OTel initialization settings.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Endpoint       string // OTLP gRPC endpoint; empty disables export.
	Insecure       bool
}

// Init sets up the global OTel trace provider and W3C propagator.
// Returns a shutdown function that must be called on process exit.
// When no endpoint is configured, a no-op provider is installed so
// instrumented code paths work unconditionally.
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	if endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}
	if cfg.Insecure || strings.EqualFold(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"), "true") {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// Tracer returns a tracer for orloj instrumentation.
func Tracer() trace.Tracer {
	return otel.Tracer(TracerName)
}

// Annotation keys used to carry W3C trace context through resource metadata.
const (
	AnnotationTraceparent = "orloj.io/traceparent"
	AnnotationTracestate  = "orloj.io/tracestate"
)

// InjectTraceContext serializes the current W3C trace context from ctx into
// annotations using the global propagator. Reports whether any trace context
// was available to inject. Safe to call when OTel is disabled (no-op).
func InjectTraceContext(ctx context.Context, annotations map[string]string) bool {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	tp := carrier.Get("traceparent")
	if tp == "" {
		return false
	}
	annotations[AnnotationTraceparent] = tp
	if ts := carrier.Get("tracestate"); ts != "" {
		annotations[AnnotationTracestate] = ts
	}
	return true
}

// ExtractTraceContext returns a context carrying the remote span described by
// trace annotations stored on the resource. Returns ctx unchanged when no
// valid trace context is found in annotations.
func ExtractTraceContext(ctx context.Context, annotations map[string]string) context.Context {
	tp, ok := annotations[AnnotationTraceparent]
	if !ok || tp == "" {
		return ctx
	}
	carrier := propagation.MapCarrier{"traceparent": tp}
	if ts, ok := annotations[AnnotationTracestate]; ok && ts != "" {
		carrier["tracestate"] = ts
	}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}
