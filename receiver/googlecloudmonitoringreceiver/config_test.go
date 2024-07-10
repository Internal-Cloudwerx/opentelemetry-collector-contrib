// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package googlecloudmonitoringreceiver

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap/confmaptest"
	"go.opentelemetry.io/collector/receiver/scraperhelper"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/googlecloudmonitoringreceiver/internal/metadata"
)

func TestLoadConfig(t *testing.T) {
	cm, err := confmaptest.LoadConf(filepath.Join("testdata", "config.yaml"))
	require.NoError(t, err)
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()

	sub, err := cm.Sub(component.NewIDWithName(metadata.Type, "").String())
	require.NoError(t, err)
	require.NoError(t, sub.Unmarshal(cfg))

	assert.Equal(t,
		&Config{
			ControllerConfig: scraperhelper.ControllerConfig{
				CollectionInterval: 120 * time.Second,
				InitialDelay:       1 * time.Second,
			},
			Region:            "us-central1",
			ProjectID:         "my-project-id",
			ServiceAccountKey: "path/to/service_account.json",
			Services: []Service{
				{
					ServiceName: "compute",
					Delay:       60 * time.Second,
					MetricName:  "compute.googleapis.com/instance/cpu/usage_time",
				},
				{
					ServiceName: "connectors",
					Delay:       60 * time.Second,
					MetricName:  "connectors.googleapis.com/flex/instance/cpu/usage_time",
				},
			},
		},
		cfg,
	)
}

func TestValidateService(t *testing.T) {
	testCases := map[string]struct {
		service      Service
		requireError bool
	}{
		"Valid Service": {
			Service{
				ServiceName: "service_name",
				Delay:       0,
			}, false},
		"Empty ServiceName": {
			Service{
				ServiceName: "",
				Delay:       0,
			}, true},
		"Negative Delay": {
			Service{
				ServiceName: "service_name",
				Delay:       -1,
			}, true},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			err := testCase.service.Validate()
			if testCase.requireError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	validService := Service{
		ServiceName: "service_name",
		Delay:       0 * time.Second,
	}

	testCases := map[string]struct {
		services           []Service
		collectionInterval time.Duration
		requireError       bool
	}{
		"Valid Config":                {[]Service{validService}, 60 * time.Second, false},
		"Empty Services":              {nil, 60 * time.Second, true},
		"Invalid Service in Services": {[]Service{{}}, 60 * time.Second, true},
		"Invalid Collection Interval": {[]Service{validService}, 0 * time.Second, true},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			cfg := &Config{
				ControllerConfig: scraperhelper.ControllerConfig{
					CollectionInterval: testCase.collectionInterval,
				},
				Services: testCase.services,
			}

			err := cfg.Validate()
			if testCase.requireError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
