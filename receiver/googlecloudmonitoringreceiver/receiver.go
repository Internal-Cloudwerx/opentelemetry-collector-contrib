// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package googlecloudmonitoringreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/googlecloudmonitoringreceiver"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/googlecloudmonitoringreceiver/internal"
)

type monitoringReceiver struct {
	config            *Config
	logger            *zap.Logger
	client            *monitoring.MetricClient
	metricsBuilder    *internal.MetricsBuilder
	mutex             sync.Mutex
	metricDescriptors map[string]*metric.MetricDescriptor
}

func newGoogleCloudMonitoringReceiver(cfg *Config, logger *zap.Logger) *monitoringReceiver {
	return &monitoringReceiver{
		config:         cfg,
		logger:         logger,
		metricsBuilder: internal.NewMetricsBuilder(logger),
	}
}

func (mr *monitoringReceiver) Start(ctx context.Context, _ component.Host) error {
	mr.metricDescriptors = make(map[string]*metric.MetricDescriptor)

	// Lock to ensure thread-safe access to mr.client
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	// If the client is already initialized, return nil
	if mr.client != nil {
		return nil
	}

	var client *monitoring.MetricClient
	var err error

	// Use google.FindDefaultCredentials to find the credentials
	creds, _ := google.FindDefaultCredentials(ctx)
	// If a valid credentials file path is found, use it
	if creds != nil && creds.JSON != nil {
		client, err = monitoring.NewMetricClient(ctx, option.WithCredentials(creds))
	} else {
		// Set a default credentials file path for testing
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "testdata/serviceAccount.json")
		client, err = monitoring.NewMetricClient(ctx)
	}

	// Attempt to create the monitoring client
	if err != nil {
		return fmt.Errorf("failed to create a monitoring client: %w", err)
	}
	mr.client = client
	mr.logger.Info("Monitoring client successfully created.")

	// Call the metricDescriptorAPI method to start processing metric descriptors.
	if err := mr.metricDescriptorAPI(ctx); err != nil {
		return err
	}

	// Return nil after client creation
	return nil
}

func (mr *monitoringReceiver) Shutdown(context.Context) error {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	var err error
	if mr.client != nil {
		err = mr.client.Close()
	}
	return err
}

func (mr *monitoringReceiver) Scrape(ctx context.Context) (pmetric.Metrics, error) {
	var (
		gInternal    time.Duration
		gDelay       time.Duration
		calStartTime time.Time
		calEndTime   time.Time
		filterQuery  string
		gErr         error
	)

	metrics := pmetric.NewMetrics()

	// Iterate over each metric in the configuration to calculate start/end times and construct the filter query.
	for _, metric := range mr.config.MetricsList {
		metricDesc, exists := mr.metricDescriptors[metric.MetricName]
		if !exists {
			mr.logger.Warn("Metric descriptor not found", zap.String("metric_name", metric.MetricName))
			continue
		}

		// Set interval and delay times, using defaults if not provided
		gInternal = mr.config.CollectionInterval
		if gInternal <= 0 {
			gInternal = defaultCollectionInterval
		}

		gDelay = metric.FetchDelay
		if gDelay <= 0 {
			gDelay = defaultFetchDelay
		}

		// Calculate the start and end times
		calStartTime, calEndTime = calculateStartEndTime(gInternal, gDelay)

		// Get the filter query for the metric
		filterQuery = getFilterQuery(metric)

		// Define the request to list time series data
		tsReq := &monitoringpb.ListTimeSeriesRequest{
			Name:   "projects/" + mr.config.ProjectID,
			Filter: filterQuery,
			Interval: &monitoringpb.TimeInterval{
				EndTime:   &timestamppb.Timestamp{Seconds: calEndTime.Unix()},
				StartTime: &timestamppb.Timestamp{Seconds: calStartTime.Unix()},
			},
			View: monitoringpb.ListTimeSeriesRequest_FULL,
		}

		// Create an iterator for the time series data
		tsIter := mr.client.ListTimeSeries(ctx, tsReq)
		mr.logger.Debug("Retrieving time series data")

		// Iterate over the time series data
		for {
			timeSeries, err := tsIter.Next()
			if errors.Is(err, iterator.Done) {
				break
			}

			// Handle errors and break conditions for the iterator
			if err != nil {
				gErr = fmt.Errorf("failed to retrieve time series data: %w", err)
				return metrics, gErr
			}

			// Convert and append the metric directly within the loop
			mr.convertGCPTimeSeriesToMetrics(metrics, metricDesc, timeSeries)
		}
	}

	return metrics, gErr
}

// metricDescriptorAPI fetches and processes metric descriptors from the monitoring API.
func (mr *monitoringReceiver) metricDescriptorAPI(ctx context.Context) error {
	// Iterate over each metric in the configuration to calculate start/end times and construct the filter query.
	for _, metric := range mr.config.MetricsList {
		// Get the filter query for the metric
		filterQuery := getFilterQuery(metric)

		// Define the request to list metric descriptors
		metricReq := &monitoringpb.ListMetricDescriptorsRequest{
			Name:   "projects/" + mr.config.ProjectID,
			Filter: filterQuery,
		}

		// Create an iterator for the metric descriptors
		metricIter := mr.client.ListMetricDescriptors(ctx, metricReq)

		// Iterate over the time series data
		for {
			metricDesc, err := metricIter.Next()
			if errors.Is(err, iterator.Done) {
				break
			}

			// Handle errors and break conditions for the iterator
			if err != nil {
				return fmt.Errorf("failed to retrieve metric descriptors data: %w", err)
			}
			mr.metricDescriptors[metricDesc.Type] = metricDesc
		}
	}

	mr.logger.Info("Successfully retrieved all metric descriptors.")
	return nil
}

// calculateStartEndTime calculates the start and end times based on the current time, interval, and delay.
func calculateStartEndTime(interval, delay time.Duration) (time.Time, time.Time) {
	// Get the current time
	now := time.Now()

	// Calculate end time by subtracting delay
	endTime := now.Add(-delay)

	// Calculate start time by subtracting interval from end time
	startTime := endTime.Add(-interval)

	// Return start and end times
	return startTime, endTime
}

// getFilterQuery constructs a filter query string based on the provided metric.
func getFilterQuery(metric MetricConfig) string {
	var filterQuery string
	const baseQuery = `metric.type =`

	// If a specific metric name is provided, use it in the filter query
	filterQuery = fmt.Sprintf(`%s "%s"`, baseQuery, metric.MetricName)
	return filterQuery
}

// ConvertGCPTimeSeriesToMetrics converts GCP Monitoring TimeSeries to pmetric.Metrics
func (mr *monitoringReceiver) convertGCPTimeSeriesToMetrics(metrics pmetric.Metrics, metricDesc *metric.MetricDescriptor, timeSeries *monitoringpb.TimeSeries) {
	// Map to track existing ResourceMetrics by resource attributes
	resourceMetricsMap := make(map[string]pmetric.ResourceMetrics)

	// Generate a unique key based on resource attributes
	resourceKey := generateResourceKey(timeSeries.Resource.Type, timeSeries.Resource.Labels, timeSeries)

	// Check if ResourceMetrics for this resource already exists
	rm, exists := resourceMetricsMap[resourceKey]

	if !exists {
		// Create a new ResourceMetrics if not already present
		rm = metrics.ResourceMetrics().AppendEmpty()

		// Set resource labels
		resource := rm.Resource()
		resource.Attributes().PutStr("gcp.resource_type", timeSeries.Resource.Type)
		for k, v := range timeSeries.Resource.Labels {
			resource.Attributes().PutStr(k, v)
		}

		// Set metadata (user and system labels)
		if timeSeries.Metadata != nil {
			for k, v := range timeSeries.Metadata.UserLabels {
				resource.Attributes().PutStr(k, v)
			}
			if timeSeries.Metadata.SystemLabels != nil {
				for k, v := range timeSeries.Metadata.SystemLabels.Fields {
					resource.Attributes().PutStr(k, fmt.Sprintf("%v", v))
				}
			}
		}

		// Store the newly created ResourceMetrics in the map
		resourceMetricsMap[resourceKey] = rm
	}

	// Ensure we have a ScopeMetrics to append the metric to
	var sm pmetric.ScopeMetrics
	if rm.ScopeMetrics().Len() == 0 {
		sm = rm.ScopeMetrics().AppendEmpty()
	} else {
		// For simplicity, let's assume all metrics will share the same ScopeMetrics
		sm = rm.ScopeMetrics().At(0)
	}

	// Create a new Metric
	m := sm.Metrics().AppendEmpty()

	// Set metric name, description, and unit
	m.SetName(metricDesc.GetName())
	m.SetDescription(metricDesc.GetDescription())
	m.SetUnit(metricDesc.Unit)

	// Convert the TimeSeries to the appropriate metric type
	switch timeSeries.GetMetricKind() {
	case metric.MetricDescriptor_GAUGE:
		mr.metricsBuilder.ConvertGaugeToMetrics(timeSeries, m)
	case metric.MetricDescriptor_CUMULATIVE:
		mr.metricsBuilder.ConvertSumToMetrics(timeSeries, m)
	case metric.MetricDescriptor_DELTA:
		mr.metricsBuilder.ConvertDeltaToMetrics(timeSeries, m)
	// TODO: Add support for HISTOGRAM
	// TODO: Add support for EXPONENTIAL_HISTOGRAM
	default:
		metricError := fmt.Sprintf("\n Unsupported metric kind: %v\n", timeSeries.GetMetricKind())
		mr.logger.Info(metricError)
	}
}

// Helper function to generate a unique key for a resource based on its attributes
func generateResourceKey(resourceType string, labels map[string]string, timeSeries *monitoringpb.TimeSeries) string {
	key := resourceType
	for k, v := range labels {
		key += k + v
	}
	if timeSeries != nil {
		for k, v := range timeSeries.Metric.Labels {
			key += k + v
		}
		if timeSeries.Resource.Labels != nil {
			for k, v := range timeSeries.Resource.Labels {
				key += k + v
			}
		}
	}
	return key
}

