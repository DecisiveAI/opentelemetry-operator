package collector

import (
	"maps"

	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	"github.com/decisiveai/opentelemetry-operator/internal/manifests"
	"github.com/decisiveai/opentelemetry-operator/internal/manifests/manifestutils"
	"github.com/decisiveai/opentelemetry-operator/internal/naming"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mydecisive.
func GrpcService(params manifests.Params) (*corev1.Service, error) {
	// we need this service for aws only
	if params.OtelCol.Spec.Ingress.Type != v1beta1.IngressTypeAws {
		return nil, nil
	}
	name := naming.GrpcService(params.OtelCol.Name)
	labels := manifestutils.Labels(params.OtelCol.ObjectMeta, name, params.OtelCol.Spec.Image, ComponentOpenTelemetryCollector, []string{})

	annotations, err := manifestutils.Annotations(params.OtelCol, params.Config.AnnotationsFilter())
	if err != nil {
		return nil, err
	}
	ports, err := servicePortsFromCfg(params.Log, params.OtelCol)
	if err != nil {
		return nil, err
	}

	for i := len(ports) - 1; i >= 0; i-- {
		if ports[i].AppProtocol == nil || (ports[i].AppProtocol != nil && *ports[i].AppProtocol != "grpc") {
			ports = append(ports[:i], ports[i+1:]...)
		}
	}

	if len(params.OtelCol.Spec.Ports) > 0 {
		// we should add all the ports from the CR
		// there are two cases where problems might occur:
		// 1) when the port number is already being used by a receiver
		// 2) same, but for the port name
		//
		// in the first case, we remove the port we inferred from the list
		// in the second case, we rename our inferred port to something like "port-%d"
		portNumbers, portNames := extractPortNumbersAndNames(params.OtelCol.Spec.Ports)
		var resultingInferredPorts []corev1.ServicePort
		for _, inferred := range ports {
			if filtered := filterPort(params.Log, inferred, portNumbers, portNames); filtered != nil {
				resultingInferredPorts = append(resultingInferredPorts, *filtered)
			}
		}
		ports = append(toServicePorts(params.OtelCol.Spec.Ports), resultingInferredPorts...)
	}

	// if we have no ports, we don't need a service
	if len(ports) == 0 {
		params.Log.V(1).Info("the instance's configuration didn't yield any ports to open, skipping service", "instance.name", params.OtelCol.Name, "instance.namespace", params.OtelCol.Namespace)
		return nil, err
	}

	trafficPolicy := corev1.ServiceInternalTrafficPolicyCluster
	if params.OtelCol.Spec.Mode == v1beta1.ModeDaemonSet {
		trafficPolicy = corev1.ServiceInternalTrafficPolicyLocal
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   params.OtelCol.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:                  "NodePort",
			InternalTrafficPolicy: &trafficPolicy,
			Selector:              manifestutils.SelectorLabels(params.OtelCol.ObjectMeta, ComponentOpenTelemetryCollector),
			Ports:                 ports,
		},
	}, nil
}

func NonGrpcService(params manifests.Params) (*corev1.Service, error) {
	// we need this service for aws only
	if params.OtelCol.Spec.Ingress.Type != v1beta1.IngressTypeAws {
		return nil, nil
	}

	name := naming.NonGrpcService(params.OtelCol.Name)
	labels := manifestutils.Labels(params.OtelCol.ObjectMeta, name, params.OtelCol.Spec.Image, ComponentOpenTelemetryCollector, []string{})

	metaAnnotations, err := manifestutils.Annotations(params.OtelCol, params.Config.AnnotationsFilter())
	if err != nil {
		return nil, err
	}

	annotations, err := manifestutils.LbServiceAnnotations(params.OtelCol, params.Config.AnnotationsFilter())
	if err != nil {
		return nil, err
	}

	// to merge 2 maps
	maps.Copy(metaAnnotations, annotations)
	// to get common keys values from ingress.lbServiceAnnotations
	maps.Copy(annotations, metaAnnotations)

	ports, err := params.OtelCol.Spec.Config.GetAllPorts(params.Log)
	if err != nil {
		return nil, err
	}

	for i := len(ports) - 1; i >= 0; i-- {
		if ports[i].AppProtocol != nil && *ports[i].AppProtocol == "grpc" {
			ports = append(ports[:i], ports[i+1:]...)
		}
	}

	if len(params.OtelCol.Spec.Ports) > 0 {
		// we should add all the ports from the CR
		// there are two cases where problems might occur:
		// 1) when the port number is already being used by a receiver
		// 2) same, but for the port name
		//
		// in the first case, we remove the port we inferred from the list
		// in the second case, we rename our inferred port to something like "port-%d"
		portNumbers, portNames := extractPortNumbersAndNames(params.OtelCol.Spec.Ports)
		var resultingInferredPorts []corev1.ServicePort
		for _, inferred := range ports {
			if filtered := filterPort(params.Log, inferred, portNumbers, portNames); filtered != nil {
				resultingInferredPorts = append(resultingInferredPorts, *filtered)
			}
		}

		ports = append(toServicePorts(params.OtelCol.Spec.Ports), resultingInferredPorts...)
	}

	// if we have no ports, we don't need a service
	if len(ports) == 0 {
		params.Log.V(1).Info("the instance's configuration didn't yield any ports to open, skipping service", "instance.name", params.OtelCol.Name, "instance.namespace", params.OtelCol.Namespace)
		return nil, err
	}

	trafficPolicy := corev1.ServiceInternalTrafficPolicyCluster
	if params.OtelCol.Spec.Mode == v1beta1.ModeDaemonSet {
		trafficPolicy = corev1.ServiceInternalTrafficPolicyLocal
	}

	var spec corev1.ServiceSpec
	spec = corev1.ServiceSpec{
		InternalTrafficPolicy: &trafficPolicy,
		Selector:              manifestutils.SelectorLabels(params.OtelCol.ObjectMeta, ComponentOpenTelemetryCollector),
		Type:                  "LoadBalancer",
		Ports:                 ports,
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   params.OtelCol.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: spec,
	}, nil
}
