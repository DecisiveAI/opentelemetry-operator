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
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/decisiveai/opentelemetry-operator/internal/autodetect/certmanager"
	"github.com/decisiveai/opentelemetry-operator/internal/manifests"
	"github.com/decisiveai/opentelemetry-operator/internal/manifests/manifestutils"
	ta "github.com/decisiveai/opentelemetry-operator/internal/manifests/targetallocator/adapters"
	"github.com/decisiveai/opentelemetry-operator/internal/naming"
	"github.com/decisiveai/opentelemetry-operator/pkg/constants"
	"github.com/decisiveai/opentelemetry-operator/pkg/featuregate"
)

func ConfigMap(params manifests.Params) (*corev1.ConfigMap, error) {
	hash, err := manifestutils.GetConfigMapSHA(params.OtelCol.Spec.Config)
	if err != nil {
		return nil, err
	}
	name := naming.ConfigMap(params.OtelCol.Name, hash)
	collectorName := naming.Collector(params.OtelCol.Name)
	labels := manifestutils.Labels(params.OtelCol.ObjectMeta, collectorName, params.OtelCol.Spec.Image, ComponentOpenTelemetryCollector, []string{})

	annotations, err := manifestutils.Annotations(params.OtelCol, params.Config.AnnotationsFilter())
	if err != nil {
		return nil, err
	}

	replaceCfgOpts := []ta.TAOption{}

	if params.Config.CertManagerAvailability() == certmanager.Available && featuregate.EnableTargetAllocatorMTLS.IsEnabled() {
		replaceCfgOpts = append(replaceCfgOpts, ta.WithTLSConfig(
			filepath.Join(constants.TACollectorTLSDirPath, constants.TACollectorCAFileName),
			filepath.Join(constants.TACollectorTLSDirPath, constants.TACollectorTLSCertFileName),
			filepath.Join(constants.TACollectorTLSDirPath, constants.TACollectorTLSKeyFileName),
			naming.TAService(params.OtelCol.Name)),
		)
	}

	replacedConf, err := ReplaceConfig(params.OtelCol, params.TargetAllocator, replaceCfgOpts...)

	if err != nil {
		params.Log.V(2).Info("failed to update prometheus config to use sharded targets: ", "err", err)
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   params.OtelCol.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Data: map[string]string{
			"collector.yaml": replacedConf,
		},
	}, nil
}
