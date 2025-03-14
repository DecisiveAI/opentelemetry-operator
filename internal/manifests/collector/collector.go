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
	"errors"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	"github.com/decisiveai/opentelemetry-operator/internal/autodetect/rbac"
	"github.com/decisiveai/opentelemetry-operator/internal/manifests"
	"github.com/decisiveai/opentelemetry-operator/pkg/featuregate"
)

const (
	ComponentOpenTelemetryCollector = "opentelemetry-collector"
)

// Build creates the manifest for the collector resource.
func Build(params manifests.Params) ([]client.Object, error) {
	var resourceManifests []client.Object
	var manifestFactories []manifests.K8sManifestFactory[manifests.Params]
	switch params.OtelCol.Spec.Mode {
	case v1beta1.ModeDeployment:
		manifestFactories = append(manifestFactories, manifests.Factory(Deployment))
		manifestFactories = append(manifestFactories, manifests.Factory(PodDisruptionBudget))
	case v1beta1.ModeStatefulSet:
		manifestFactories = append(manifestFactories, manifests.Factory(StatefulSet))
		manifestFactories = append(manifestFactories, manifests.Factory(PodDisruptionBudget))
	case v1beta1.ModeDaemonSet:
		manifestFactories = append(manifestFactories, manifests.Factory(DaemonSet))
	case v1beta1.ModeSidecar:
		params.Log.V(5).Info("not building sidecar...")
	}
	manifestFactories = append(manifestFactories, []manifests.K8sManifestFactory[manifests.Params]{
		manifests.Factory(ConfigMap),
		manifests.Factory(HorizontalPodAutoscaler),
		manifests.Factory(ServiceAccount),
		manifests.Factory(Service),
		manifests.Factory(HeadlessService),
		manifests.Factory(MonitoringService),
		manifests.Factory(ExtensionService),
		manifests.Factory(Ingress),
		// mydecisive
		manifests.Factory(GrpcService),
		manifests.Factory(NonGrpcService),
	}...)

	if featuregate.CollectorUsesTargetAllocatorCR.IsEnabled() {
		manifestFactories = append(manifestFactories, manifests.Factory(TargetAllocator))
	}

	if params.OtelCol.Spec.Observability.Metrics.EnableMetrics && featuregate.PrometheusOperatorIsAvailable.IsEnabled() {
		if params.OtelCol.Spec.Mode == v1beta1.ModeSidecar {
			manifestFactories = append(manifestFactories, manifests.Factory(PodMonitor))
		} else {
			manifestFactories = append(manifestFactories, manifests.Factory(ServiceMonitor), manifests.Factory(ServiceMonitorMonitoring))
		}
	}

	if params.Config.CreateRBACPermissions() == rbac.Available {
		manifestFactories = append(manifestFactories,
			manifests.Factory(ClusterRole),
			manifests.Factory(ClusterRoleBinding),
		)
	}

	for _, factory := range manifestFactories {
		res, err := factory(params)
		if err != nil {
			return nil, err
		} else if manifests.ObjectIsNotNil(res) {
			resourceManifests = append(resourceManifests, res)
		}
	}

	if needsCheckSaPermissions(params) {
		warnings, err := CheckRbacRules(params, params.OtelCol.Spec.ServiceAccount)
		if err != nil {
			return nil, fmt.Errorf("error checking RBAC rules for serviceAccount %s: %w", params.OtelCol.Spec.ServiceAccount, err)
		}

		var w []error
		for _, warning := range warnings {
			w = append(w, fmt.Errorf("RBAC rules are missing: %s", warning))
		}
		return nil, errors.Join(w...)
	}

	routes, err := Routes(params)
	if err != nil {
		return nil, err
	}
	// NOTE: we cannot just unpack the slice, the type checker doesn't coerce the type correctly.
	for _, route := range routes {
		resourceManifests = append(resourceManifests, route)
	}
	return resourceManifests, nil
}

func needsCheckSaPermissions(params manifests.Params) bool {
	return params.ErrorAsWarning &&
		params.Config.CreateRBACPermissions() == rbac.NotAvailable &&
		params.Reviewer != nil &&
		params.OtelCol.Spec.ServiceAccount != ""
}
