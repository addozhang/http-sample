package main

import (
	"context"
	"errors"
	"fmt"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/contrib/propagators/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

const OtelExporterOTLPEndpoints = "OTEL_EXPORTER_OTLP_ENDPOINT"
const OtelPropagators = "OTEL_PROPAGATORS"

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOTelSDK(ctx context.Context, serviceName, serviceVersion string) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	if os.Getenv(OtelExporterOTLPEndpoints) == "" {
		fmt.Println("OTEL_EXPORTER_OTLP_ENDPOINT not provided, skip!!!")
		return
	}

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up resource.
	res, err := newResource(serviceName, serviceVersion)
	if err != nil {
		handleErr(err)
		return
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err := newTraceProvider(res)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider(res)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	return
}

func newResource(serviceName, serviceVersion string) (*resource.Resource, error) {
	return resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		))
}

func newPropagator() propagation.TextMapPropagator {
	propagatorString := os.Getenv(OtelPropagators)
	var propagators []propagation.TextMapPropagator
	if propagatorString != "" {
		for _, p := range strings.Split(propagatorString, ",") {
			switch p {
			case "tracecontext":
				propagators = append(propagators, propagation.TraceContext{})
			case "b3":
				propagators = append(propagators, b3.New(b3.WithInjectEncoding(b3.B3SingleHeader)))
			case "b3multi":
				propagators = append(propagators, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader)))
			case "baggage":
				propagators = append(propagators, propagation.Baggage{})
			case "jaeger":
				propagators = append(propagators, jaeger.Jaeger{})
			}
		}
	}
	if len(propagators) == 0 {
		propagators = append(propagators, b3.New(b3.WithInjectEncoding(b3.B3SingleHeader)))
	}
	return propagation.NewCompositeTextMapPropagator(propagators...)
}

func newTraceProvider(res *resource.Resource) (*trace.TracerProvider, error) {
	traceExporter, err := newTraceExporter()
	if err != nil {
		return nil, fmt.Errorf("failed to create trace provider: %w", err)
	}

	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)
	return tracerProvider, nil
}

func newTraceExporter() (*otlptrace.Exporter, error) {
	ctx := context.Background()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(os.Getenv(OtelExporterOTLPEndpoints)), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}
	return traceExporter, nil
}

func newMeterProvider(res *resource.Resource) (*metric.MeterProvider, error) {
	metricExporter, err := newMetricExporter()
	if err != nil {
		return nil, fmt.Errorf("failed to create meter provider: %w", err)
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			// Default is 1m. Set to 5s for demonstrative purposes.
			metric.WithInterval(5*time.Second))),
	)
	return meterProvider, nil
}

func newMetricExporter() (*otlpmetrichttp.Exporter, error) {
	ctx := context.Background()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpoint(os.Getenv(OtelExporterOTLPEndpoints)), otlpmetrichttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics exporter: %w", err)
	}

	return metricExporter, nil
}
