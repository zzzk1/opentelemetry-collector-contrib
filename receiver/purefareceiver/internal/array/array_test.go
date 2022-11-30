// Copyright 2022 The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package array // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/purefareceiver/internal/array"

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configauth"
	"go.opentelemetry.io/collector/consumer/consumertest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/bearertokenauthextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver"
)

func TestToPrometheusConfig(t *testing.T) {
	// prepare
	prFactory := prometheusreceiver.NewFactory()
	baFactory := bearertokenauthextension.NewFactory()

	baCfg := baFactory.CreateDefaultConfig().(*bearertokenauthextension.Config)
	baCfg.BearerToken = "the-token"

	baExt, err := baFactory.CreateExtension(context.Background(), componenttest.NewNopExtensionCreateSettings(), baCfg)
	require.NoError(t, err)

	host := &mockHost{
		extensions: map[component.ID]component.Component{
			component.NewIDWithName("bearertokenauth", "array01"): baExt,
		},
	}

	endpoint := "http://example.com"
	interval := 15 * time.Second
	arrs := []Config{
		{
			Address: "gse-array01",
			Auth: configauth.Authentication{
				AuthenticatorID: component.NewIDWithName("bearertokenauth", "array01"),
			},
		},
	}

	scraper := NewScraper(context.Background(), componenttest.NewNopReceiverCreateSettings(), consumertest.NewNop(), endpoint, arrs, interval)

	// test
	promRecvCfg, err := scraper.ToPrometheusReceiverConfig(host, prFactory)

	// verify
	assert.NoError(t, err)

	scCfgs := promRecvCfg.PrometheusConfig.ScrapeConfigs
	assert.Len(t, scCfgs, 1)
	assert.EqualValues(t, "the-token", scCfgs[0].HTTPClientConfig.BearerToken)
	assert.Equal(t, "gse-array01", scCfgs[0].Params.Get("endpoint"))
	assert.Equal(t, "/metrics/array", scCfgs[0].MetricsPath)
	assert.Equal(t, "purefa/arrays/gse-array01", scCfgs[0].JobName)
	assert.EqualValues(t, interval, scCfgs[0].ScrapeTimeout)
	assert.EqualValues(t, interval, scCfgs[0].ScrapeInterval)
}

func TestBearerToken(t *testing.T) {
	// prepare
	baFactory := bearertokenauthextension.NewFactory()

	baCfg := baFactory.CreateDefaultConfig().(*bearertokenauthextension.Config)
	baCfg.BearerToken = "the-token"

	baExt, err := baFactory.CreateExtension(context.Background(), componenttest.NewNopExtensionCreateSettings(), baCfg)
	require.NoError(t, err)

	baComponentName := component.NewIDWithName("bearertokenauth", "array01")

	host := &mockHost{
		extensions: map[component.ID]component.Component{
			baComponentName: baExt,
		},
	}

	cfgAuth := configauth.Authentication{
		AuthenticatorID: baComponentName,
	}

	// test
	token, err := retrieveBearerToken(cfgAuth, host.GetExtensions())

	// verify
	assert.NoError(t, err)
	assert.Equal(t, "the-token", token)
}

func TestStart(t *testing.T) {
	// prepare
	sink := &consumertest.MetricsSink{}
	arrs := []Config{}
	interval := 15 * time.Second
	scr := NewScraper(context.Background(), componenttest.NewNopReceiverCreateSettings(), sink, "http://example.com", arrs, interval)

	// test
	err := scr.Start(context.Background(), componenttest.NewNopHost())

	// verify
	assert.NoError(t, err)
}

func TestShutdown(t *testing.T) {
	// prepare
	sink := &consumertest.MetricsSink{}
	arrs := []Config{}
	interval := 15 * time.Second
	scr := NewScraper(context.Background(), componenttest.NewNopReceiverCreateSettings(), sink, "http://example.com", arrs, interval)
	err := scr.Start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	// test
	err = scr.Shutdown(context.Background())

	// verify
	assert.NoError(t, err)
}
