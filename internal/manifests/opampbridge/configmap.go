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

package opampbridge

import (
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/decisiveai/opentelemetry-operator/internal/manifests"
	"github.com/decisiveai/opentelemetry-operator/internal/manifests/manifestutils"
	"github.com/decisiveai/opentelemetry-operator/internal/naming"
)

const (
	OpAMPBridgeFilename = "remoteconfiguration.yaml"
)

func ConfigMap(params manifests.Params) (*corev1.ConfigMap, error) {
	name := naming.OpAMPBridgeConfigMap(params.OpAMPBridge.Name)
	labels := manifestutils.Labels(params.OpAMPBridge.ObjectMeta, name, params.OpAMPBridge.Spec.Image, ComponentOpAMPBridge, []string{})

	config := make(map[interface{}]interface{})

	if len(params.OpAMPBridge.Spec.Endpoint) > 0 {
		config["endpoint"] = params.OpAMPBridge.Spec.Endpoint
	}

	if len(params.OpAMPBridge.Spec.Headers) > 0 {
		config["headers"] = params.OpAMPBridge.Spec.Headers
	}

	if params.OpAMPBridge.Spec.Capabilities != nil {
		config["capabilities"] = params.OpAMPBridge.Spec.Capabilities
	}

	if params.OpAMPBridge.Spec.ComponentsAllowed != nil {
		config["componentsAllowed"] = params.OpAMPBridge.Spec.ComponentsAllowed
	}

	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return &corev1.ConfigMap{}, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   params.OpAMPBridge.Namespace,
			Labels:      labels,
			Annotations: params.OpAMPBridge.Annotations,
		},
		Data: map[string]string{
			OpAMPBridgeFilename: string(configYAML),
		},
	}, nil
}
