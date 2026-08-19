package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

// Config controls optional OpenTelemetry tracing. Metrics remain available
// through Registry even when tracing is disabled.
type Config struct {
	Enabled          bool
	ServiceName      string
	OTLPEndpoint     string
	Insecure         bool
	TraceSampleRatio float64
}

// Runtime owns the provider created for one Mesa instance. It does not change
// global OpenTelemetry providers, which keeps multiple Mesa instances safe in
// tests and in embedding applications.
type Runtime struct {
	tracerProvider *sdktrace.TracerProvider
	tracer         trace.Tracer
	propagator     propagation.TextMapPropagator
}

func New(ctx context.Context, cfg Config) (*Runtime, error) {
	r := &Runtime{propagator: propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	)}
	// go-micro's built-in wrapper reads the process propagator. The W3C
	// propagator is process-wide by definition and is safe to share between
	// Mesa instances; providers themselves remain instance-local.
	otel.SetTextMapPropagator(r.propagator)
	if !cfg.Enabled {
		r.tracer = otel.Tracer("wego")
		return r, nil
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return nil, errors.New("telemetry service name is required")
	}
	if strings.TrimSpace(cfg.OTLPEndpoint) == "" {
		return nil, errors.New("telemetry OTLP endpoint is required")
	}
	if cfg.TraceSampleRatio < 0 || cfg.TraceSampleRatio > 1 {
		return nil, fmt.Errorf("telemetry trace sample ratio must be between 0 and 1")
	}
	if cfg.TraceSampleRatio == 0 {
		cfg.TraceSampleRatio = 0.1
	}
	if ctx == nil {
		ctx = context.Background()
	}
	options := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint)}
	if cfg.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}
	exporter, err := otlptracegrpc.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)))
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.TraceSampleRatio))
	r.tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	r.tracer = r.tracerProvider.Tracer("github.com/Jinchenyuan/wego")
	return r, nil
}

func (r *Runtime) Tracer(name string) trace.Tracer {
	if r == nil || r.tracerProvider == nil {
		return otel.Tracer(name)
	}
	return r.tracerProvider.Tracer(name)
}

func (r *Runtime) TracerProvider() trace.TracerProvider {
	if r == nil || r.tracerProvider == nil {
		return otel.GetTracerProvider()
	}
	return r.tracerProvider
}

func (r *Runtime) Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return r.Tracer(name).Start(ctx, name, trace.WithAttributes(attrs...))
}

func (r *Runtime) Propagator() propagation.TextMapPropagator {
	if r == nil || r.propagator == nil {
		return propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	}
	return r.propagator
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil || r.tracerProvider == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return r.tracerProvider.Shutdown(ctx)
}
