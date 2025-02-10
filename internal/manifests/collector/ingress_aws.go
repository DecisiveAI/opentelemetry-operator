package collector

import (
	"errors"
	"maps"
	"sort"

	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	"github.com/decisiveai/opentelemetry-operator/internal/components"
	"github.com/decisiveai/opentelemetry-operator/internal/manifests"
	"github.com/decisiveai/opentelemetry-operator/internal/manifests/manifestutils"
	"github.com/decisiveai/opentelemetry-operator/internal/naming"
	"github.com/go-logr/logr"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mydecisive
func IngressAws(params manifests.Params) (*networkingv1.Ingress, error) {
	name := naming.Ingress(params.OtelCol.Name)
	labels := manifestutils.Labels(params.OtelCol.ObjectMeta, name, params.OtelCol.Spec.Image, ComponentOpenTelemetryCollector, params.Config.LabelsFilter())
	annotations, err := manifestutils.Annotations(params.OtelCol, params.Config.AnnotationsFilter())
	if err != nil {
		return nil, err
	}
	ingressAnnotations, err := manifestutils.IngressAnnotations(params.OtelCol, params.Config.AnnotationsFilter())
	if err != nil {
		return nil, err
	}
	maps.Copy(annotations, ingressAnnotations)

	var rules []networkingv1.IngressRule
	compPortsEndpoints, err := servicePortsUrlPathsFromCfg(params.Log, params.OtelCol)
	if len(compPortsEndpoints) == 0 || err != nil {
		params.Log.V(1).Info(
			"the instance's configuration didn't yield any ports to open, skipping ingress",
			"instance.name", params.OtelCol.Name,
			"instance.namespace", params.OtelCol.Namespace,
		)
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
	// if we have no grpc ports, we don't need an ingress entry
	if len(compPortsEndpoints) == 0 {
		params.Log.V(1).Info(
			"the instance's configuration didn't yield any grpc ports to open, skipping ingress",
			"instance.name", params.OtelCol.Name,
			"instance.namespace", params.OtelCol.Namespace,
		)
		return nil, nil
	}

	switch params.OtelCol.Spec.Ingress.RuleType {
	case v1beta1.IngressRuleTypePath, "":
		if params.OtelCol.Spec.Ingress.CollectorEndpoints == nil || len(params.OtelCol.Spec.Ingress.CollectorEndpoints) == 0 {
			return nil, errors.New("empty components to hostnames mapping")
		} else {
			rules = createPathIngressRulesUrlPaths(params.Log, params.OtelCol.Name, params.OtelCol.Spec.Ingress.CollectorEndpoints, compPortsEndpoints)
		}
	case v1beta1.IngressRuleTypeSubdomain:
		params.Log.V(1).Info("Only  IngressRuleType = \"path\" is supported for AWS",
			"ingress.type", v1beta1.IngressTypeAws,
			"ingress.ruleType", v1beta1.IngressRuleTypeSubdomain,
		)
		return nil, err
	}
	if rules == nil || len(rules) == 0 {
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
			Name:        name,
			Namespace:   params.OtelCol.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: networkingv1.IngressSpec{
			TLS:              params.OtelCol.Spec.Ingress.TLS,
			Rules:            rules,
			IngressClassName: params.OtelCol.Spec.Ingress.IngressClassName,
		},
	}, nil
}

// mydecisive.
func createPathIngressRulesUrlPaths(logger logr.Logger, otelcol string, colEndpoints map[string]string, compPortsUrlPaths components.ComponentsPortsUrlPaths) []networkingv1.IngressRule {
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
							Name: naming.GrpcService(otelcol),
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

// mydecisive.
func servicePortsUrlPathsFromCfg(logger logr.Logger, otelcol v1beta1.OpenTelemetryCollector) (components.ComponentsPortsUrlPaths, error) {
	portsUrlPaths, err := otelcol.Spec.Config.GetReceiverPortsWithUrlPaths(logger)
	if err != nil {
		logger.Error(err, "couldn't build the ingress for this instance")
		return nil, err
	}
	return portsUrlPaths, nil
}
