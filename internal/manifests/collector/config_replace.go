// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"time"

	promconfig "github.com/prometheus/prometheus/config"
	_ "github.com/prometheus/prometheus/discovery/install" // Package install has the side-effect of registering all builtin.
	"gopkg.in/yaml.v3"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"github.com/open-telemetry/opentelemetry-operator/internal/manifests/collector/adapters"
	ta "github.com/open-telemetry/opentelemetry-operator/internal/manifests/targetallocator/adapters"
	"github.com/open-telemetry/opentelemetry-operator/internal/naming"
)

type targetAllocator struct {
	Endpoint    string        `yaml:"endpoint"`
	Interval    time.Duration `yaml:"interval"`
	CollectorID string        `yaml:"collector_id"`
	// HTTPSDConfig is a preference that can be set for the collector's target allocator, but the operator doesn't
	// care about what the value is set to. We just need this for validation when unmarshalling the configmap.
	HTTPSDConfig interface{} `yaml:"http_sd_config,omitempty"`
}

type Config struct {
	PromConfig        *promconfig.Config `yaml:"config"`
	TargetAllocConfig *targetAllocator   `yaml:"target_allocator,omitempty"`
}

func ReplaceConfig(otelcol v1beta1.OpenTelemetryCollector, targetAllocator *v1alpha1.TargetAllocator) (string, error) {
	collectorSpec := otelcol.Spec
	taEnabled := targetAllocator != nil
	cfgStr, err := collectorSpec.Config.Yaml()
	if err != nil {
		return "", err
	}
	// Check if TargetAllocator is present, if not, return the original config
	if !taEnabled {
		return cfgStr, nil
	}

	config, err := adapters.ConfigFromString(cfgStr)
	if err != nil {
		return "", err
	}

	promCfgMap, getCfgPromErr := ta.ConfigToPromConfig(cfgStr)
	if getCfgPromErr != nil {
		return "", getCfgPromErr
	}

	validateCfgPromErr := ta.ValidatePromConfig(promCfgMap, taEnabled)
	if validateCfgPromErr != nil {
		return "", validateCfgPromErr
	}

	// To avoid issues caused by Prometheus validation logic, which fails regex validation when it encounters
	// $$ in the prom config, we update the YAML file directly without marshaling and unmarshalling.
	updPromCfgMap, getCfgPromErr := ta.AddTAConfigToPromConfig(promCfgMap, naming.TAService(targetAllocator.Name), targetAllocator.Namespace)
	if getCfgPromErr != nil {
		return "", getCfgPromErr
	}

	// type coercion checks are handled in the AddTAConfigToPromConfig method above
	config["receivers"].(map[interface{}]interface{})["prometheus"] = updPromCfgMap

	out, updCfgMarshalErr := yaml.Marshal(config)
	if updCfgMarshalErr != nil {
		return "", updCfgMarshalErr
	}

	return string(out), nil
}
