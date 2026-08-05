// Copyright (c) Abstract Machines
// SPDX-License-Identifier: Apache-2.0

package otel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/absmach/fluxmq/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc/credentials"
)

// InitProvider initializes OpenTelemetry SDK with OTLP exporters.
// Returns a shutdown function that should be called on application exit.
func InitProvider(cfg config.TelemetryConfig, nodeID string) (func(context.Context) error, error) {
	ctx := context.Background()

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
			semconv.ServiceInstanceIDKey.String(nodeID),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	var shutdownFuncs []func(context.Context) error

	// Initialize TracerProvider
	if cfg.TracesEnabled {
		traceShutdown, err := initTracerProvider(ctx, cfg, res)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize tracer provider: %w", err)
		}
		shutdownFuncs = append(shutdownFuncs, traceShutdown)
	} else {
		// Use noop tracer provider for zero overhead
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
	}

	// Initialize MeterProvider (always enabled if metrics are enabled)
	if cfg.MetricsEnabled {
		meterShutdown, err := initMeterProvider(ctx, cfg, res)
		if err != nil {
			// Cleanup trace provider if meter fails
			for _, fn := range shutdownFuncs {
				_ = fn(ctx)
			}
			return nil, fmt.Errorf("failed to initialize meter provider: %w", err)
		}
		shutdownFuncs = append(shutdownFuncs, meterShutdown)
	}

	// Return combined shutdown function
	return func(ctx context.Context) error {
		var errs []error
		for _, fn := range shutdownFuncs {
			if err := fn(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("shutdown errors: %w", errors.Join(errs...))
		}
		return nil
	}, nil
}

// buildExporterTLS constructs the transport-security configuration shared by
// the trace and metric OTLP exporters. Returns (transportOption, error) where
// transportOption is either otlptracegrpc.WithInsecure / otlpmetricgrpc.WithInsecure
// equivalent or otlp...grpc.WithTLSCredentials.
func buildTLSCredentials(cfg config.TelemetryConfig) (*tls.Config, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read telemetry CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("telemetry.ca_file %q contains no valid certificates", cfg.CAFile)
		}
		tlsCfg.RootCAs = pool
	}

	if cfg.CertFile != "" || cfg.KeyFile != "" {
		if cfg.CertFile == "" || cfg.KeyFile == "" {
			return nil, fmt.Errorf("telemetry client mTLS requires both telemetry.cert_file and telemetry.key_file")
		}
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load telemetry client keypair: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}

// initTracerProvider creates and registers a TracerProvider with OTLP exporter.
func initTracerProvider(ctx context.Context, cfg config.TelemetryConfig, res *resource.Resource) (func(context.Context) error, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithTimeout(30 * time.Second),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	} else {
		tlsCfg, err := buildTLSCredentials(cfg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsCfg)))
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	sampler := trace.ParentBased(trace.TraceIDRatioBased(cfg.TraceSampleRate))

	tp := trace.NewTracerProvider(
		trace.WithResource(res),
		trace.WithSampler(sampler),
		trace.WithBatcher(exporter,
			trace.WithMaxExportBatchSize(512),
			trace.WithBatchTimeout(5*time.Second),
		),
	)

	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// initMeterProvider creates and registers a MeterProvider with OTLP exporter.
func initMeterProvider(ctx context.Context, cfg config.TelemetryConfig, res *resource.Resource) (func(context.Context) error, error) {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
		otlpmetricgrpc.WithTimeout(30 * time.Second),
	}
	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	} else {
		tlsCfg, err := buildTLSCredentials(cfg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(tlsCfg)))
	}

	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(exporter,
			metric.WithInterval(10*time.Second),
		)),
	)

	otel.SetMeterProvider(mp)

	return mp.Shutdown, nil
}
