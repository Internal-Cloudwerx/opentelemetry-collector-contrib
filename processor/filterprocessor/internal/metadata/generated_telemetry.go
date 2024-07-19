// Code generated by mdatagen. DO NOT EDIT.

package metadata

import (
	"errors"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
)

func Meter(settings component.TelemetrySettings) metric.Meter {
	return settings.MeterProvider.Meter("otelcol/filter")
}

func Tracer(settings component.TelemetrySettings) trace.Tracer {
	return settings.TracerProvider.Tracer("otelcol/filter")
}

// TelemetryBuilder provides an interface for components to report telemetry
// as defined in metadata and user config.
type TelemetryBuilder struct {
	meter                             metric.Meter
	ProcessorFilterDatapointsFiltered metric.Int64Counter
	ProcessorFilterLogsFiltered       metric.Int64Counter
	ProcessorFilterSpansFiltered      metric.Int64Counter
	level                             configtelemetry.Level
}

// telemetryBuilderOption applies changes to default builder.
type telemetryBuilderOption func(*TelemetryBuilder)

// WithLevel sets the current telemetry level for the component.
func WithLevel(lvl configtelemetry.Level) telemetryBuilderOption {
	return func(builder *TelemetryBuilder) {
		builder.level = lvl
	}
}

// NewTelemetryBuilder provides a struct with methods to update all internal telemetry
// for a component
func NewTelemetryBuilder(settings component.TelemetrySettings, options ...telemetryBuilderOption) (*TelemetryBuilder, error) {
	builder := TelemetryBuilder{level: configtelemetry.LevelBasic}
	for _, op := range options {
		op(&builder)
	}
	var err, errs error
	if builder.level >= configtelemetry.LevelBasic {
		builder.meter = Meter(settings)
	} else {
		builder.meter = noop.Meter{}
	}
	builder.ProcessorFilterDatapointsFiltered, err = builder.meter.Int64Counter(
		"otelcol_processor_filter_datapoints.filtered",
		metric.WithDescription("Number of metric data points dropped by the filter processor"),
		metric.WithUnit("1"),
	)
	errs = errors.Join(errs, err)
	builder.ProcessorFilterLogsFiltered, err = builder.meter.Int64Counter(
		"otelcol_processor_filter_logs.filtered",
		metric.WithDescription("Number of logs dropped by the filter processor"),
		metric.WithUnit("1"),
	)
	errs = errors.Join(errs, err)
	builder.ProcessorFilterSpansFiltered, err = builder.meter.Int64Counter(
		"otelcol_processor_filter_spans.filtered",
		metric.WithDescription("Number of spans dropped by the filter processor"),
		metric.WithUnit("1"),
	)
	errs = errors.Join(errs, err)
	return &builder, errs
}
