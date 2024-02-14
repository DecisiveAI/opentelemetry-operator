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
	"sort"

	"github.com/open-telemetry/opentelemetry-operator/internal/manifests/collector/parser"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/internal/manifests"
	"github.com/open-telemetry/opentelemetry-operator/internal/manifests/collector/adapters"
	"github.com/open-telemetry/opentelemetry-operator/internal/naming"
)

func Ingress(params manifests.Params) (*networkingv1.Ingress, error) {
	// mydecisive.
	var rules []networkingv1.IngressRule

	switch params.OtelCol.Spec.Ingress.Type {
	case v1alpha1.IngressTypeNginx:
		{
			ports, err := servicePortsFromCfg(params.Log, params.OtelCol)
			if err != nil {
				return nil, err
			}
			switch params.OtelCol.Spec.Ingress.RuleType {
			case v1alpha1.IngressRuleTypePath, "":
				rules = []networkingv1.IngressRule{createPathIngressRules(params.OtelCol.Name, params.OtelCol.Spec.Ingress.Hostname, ports)}
			case v1alpha1.IngressRuleTypeSubdomain:
				rules = createSubdomainIngressRules(params.OtelCol.Name, params.OtelCol.Spec.Ingress.Hostname, ports)
			}
		} // v1alpha1.IngressTypeNginx
	case v1alpha1.IngressTypeAws:
		{
			compPortsEndpoints, err := servicePortsUrlPathsFromCfg(params.Log, params.OtelCol)
			if err != nil {
				return nil, err
			}
			for comp, portsEndpoints := range compPortsEndpoints {
				// deleting all non-grpc ports
				for i := len(portsEndpoints) - 1; i >= 0; i-- {
					if portsEndpoints[i].Port.AppProtocol == nil || (portsEndpoints[i].Port.AppProtocol != nil && *portsEndpoints[i].Port.AppProtocol != "grpc") {
						portsEndpoints = append(portsEndpoints[:i], portsEndpoints[i+1:]...)
					}
				}
				// if component does not have grpc ports, delete it from the result map
				if len(portsEndpoints) > 0 {
					compPortsEndpoints[comp] = portsEndpoints
				} else {
					delete(compPortsEndpoints, comp)

				}
			}
			// if we have no ports, we don't need a ingress entry
			if len(compPortsEndpoints) == 0 {
				params.Log.V(1).Info(
					"the instance's configuration didn't yield any ports to open, skipping ingress",
					"instance.name", params.OtelCol.Name,
					"instance.namespace", params.OtelCol.Namespace,
				)
				return nil, err
			}
			switch params.OtelCol.Spec.Ingress.RuleType {
			case v1alpha1.IngressRuleTypePath, "":
				if (params.OtelCol.Spec.Ingress.CollectorEndpoints != nil) || (len(params.OtelCol.Spec.Ingress.CollectorEndpoints) == 0) {
					rules = createPathIngressRulesUrlPaths(params.Log, params.OtelCol.Name, params.OtelCol.Spec.Ingress.CollectorEndpoints, compPortsEndpoints)
				} else {
					return nil, errors.New("empty components to hostnames mapping")
				}
			case v1alpha1.IngressRuleTypeSubdomain:
				params.Log.V(1).Info("Only  IngressRuleType = \"path\" is supported for AWS",
					"ingress.type", v1alpha1.IngressTypeAws,
					"ingress.ruleType", v1alpha1.IngressRuleTypeSubdomain,
				)
				return nil, err
			}
		} // v1alpha1.IngressTypeAws
	case v1alpha1.IngressTypeRoute:
		return nil, nil
	default:
		return nil, nil
	}

	if (rules == nil) || (len(rules) == 0) {
		params.Log.V(1).Info(
			"could not configure any ingress rules for the instance's configuration, skipping ingress",
			"instance.name", params.OtelCol.Name,
			"instance.namespace", params.OtelCol.Namespace,
		)
		return nil, nil
	}

	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Host < rules[j].Host
	})

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        naming.Ingress(params.OtelCol.Name),
			Namespace:   params.OtelCol.Namespace,
			Annotations: params.OtelCol.Spec.Ingress.Annotations,
			Labels: map[string]string{
				"app.kubernetes.io/name":       naming.Ingress(params.OtelCol.Name),
				"app.kubernetes.io/instance":   fmt.Sprintf("%s.%s", params.OtelCol.Namespace, params.OtelCol.Name),
				"app.kubernetes.io/managed-by": "opentelemetry-operator",
			},
		},
		Spec: networkingv1.IngressSpec{
			TLS:              params.OtelCol.Spec.Ingress.TLS,
			Rules:            rules,
			IngressClassName: params.OtelCol.Spec.Ingress.IngressClassName,
		},
	}, nil
}

func createPathIngressRules(otelcol string, hostname string, ports []corev1.ServicePort) networkingv1.IngressRule {
	pathType := networkingv1.PathTypePrefix
	paths := make([]networkingv1.HTTPIngressPath, len(ports))
	for i, port := range ports {
		portName := naming.PortName(port.Name, port.Port)
		paths[i] = networkingv1.HTTPIngressPath{
			Path:     "/" + port.Name,
			PathType: &pathType,
			Backend: networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: naming.Service(otelcol),
					Port: networkingv1.ServiceBackendPort{
						Name: portName,
					},
				},
			},
		}
	}
	return networkingv1.IngressRule{
		Host: hostname,
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: paths,
			},
		},
	}
}

func createSubdomainIngressRules(otelcol string, hostname string, ports []corev1.ServicePort) []networkingv1.IngressRule {
	var rules []networkingv1.IngressRule
	pathType := networkingv1.PathTypePrefix
	for _, port := range ports {
		portName := naming.PortName(port.Name, port.Port)

		host := fmt.Sprintf("%s.%s", portName, hostname)
		// This should not happen due to validation in the webhook.
		if hostname == "" || hostname == "*" {
			host = portName
		}
		rules = append(rules, networkingv1.IngressRule{
			Host: host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{
						{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: naming.Service(otelcol),
									Port: networkingv1.ServiceBackendPort{
										Name: portName,
									},
								},
							},
						},
					},
				},
			},
		})
	}
	return rules
}

// mydecisive.
func createPathIngressRulesUrlPaths(logger logr.Logger, otelcol string, colEndpoints map[string]string, compPortsUrlPaths parser.CompPortUrlPaths) []networkingv1.IngressRule {
	pathType := networkingv1.PathTypePrefix
	var ingressRules []networkingv1.IngressRule
	for comp, portsUrlPaths := range compPortsUrlPaths {
		var totalPaths = 0
		for _, portUrlPaths := range portsUrlPaths {
			totalPaths += len(portUrlPaths.UrlPaths)
		}
		paths := make([]networkingv1.HTTPIngressPath, totalPaths)
		var i = 0
		for _, portUrlPaths := range portsUrlPaths {
			portName := naming.PortName(portUrlPaths.Port.Name, portUrlPaths.Port.Port)
			for _, endpoint := range portUrlPaths.UrlPaths {
				paths[i] = networkingv1.HTTPIngressPath{
					Path:     endpoint,
					PathType: &pathType,
					Backend: networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: naming.BehindIngressService(otelcol),
							Port: networkingv1.ServiceBackendPort{
								Name: portName,
							},
						},
					},
				}
				i++
			}
			if host, ok := colEndpoints[comp]; ok {
				ingressRule := networkingv1.IngressRule{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: paths,
						},
					},
				}
				ingressRules = append(ingressRules, ingressRule)
			} else {
				err := errors.New("missing or invalid mapping")
				logger.V(1).Error(err, "missing or invalid mapping for", "component", comp)
				continue
			}
		}
	}
	return ingressRules
}

func servicePortsFromCfg(logger logr.Logger, otelcol v1alpha1.OpenTelemetryCollector) ([]corev1.ServicePort, error) {
	configFromString, err := adapters.ConfigFromString(otelcol.Spec.Config)
	if err != nil {
		logger.Error(err, "couldn't extract the configuration from the context")
		return nil, err
	}

	ports, err := adapters.ConfigToComponentPorts(logger, adapters.ComponentTypeReceiver, configFromString)
	if err != nil {
		logger.Error(err, "couldn't build the ingress for this instance")
		return nil, err
	}

	if len(otelcol.Spec.Ports) > 0 {
		// we should add all the ports from the CR
		// there are two cases where problems might occur:
		// 1) when the port number is already being used by a receiver
		// 2) same, but for the port name
		//
		// in the first case, we remove the port we inferred from the list
		// in the second case, we rename our inferred port to something like "port-%d"
		portNumbers, portNames := extractPortNumbersAndNames(otelcol.Spec.Ports)
		var resultingInferredPorts []corev1.ServicePort
		for _, inferred := range ports {
			if filtered := filterPort(logger, inferred, portNumbers, portNames); filtered != nil {
				resultingInferredPorts = append(resultingInferredPorts, *filtered)
			}
		}
		ports = append(otelcol.Spec.Ports, resultingInferredPorts...)
	}
	return ports, err
}

// mydecisive.
func servicePortsUrlPathsFromCfg(logger logr.Logger, otelcol v1alpha1.OpenTelemetryCollector) (parser.CompPortUrlPaths, error) {
	configFromString, err := adapters.ConfigFromString(otelcol.Spec.Config)
	if err != nil {
		logger.Error(err, "couldn't extract the configuration from the context")
		return nil, err
	}

	compPortsUrlPaths, err := adapters.ConfigToComponentPortsUrlPaths(logger, adapters.ComponentTypeReceiver, configFromString)
	if err != nil {
		logger.Error(err, "couldn't build the ingress for this instance")
		return nil, err
	}

	return compPortsUrlPaths, nil
}
