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
	"sort"
	"testing"

	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const testFileServiceAws = "testdata/service_aws_testdata.yaml"

func TestDesiredServiceAws(t *testing.T) {

	grpc := "grpc"

	t.Run("create gRPC and non-gRPC Services", func(t *testing.T) {
		params, err := newParams("something:tag", testFileServiceAws)
		if err != nil {
			t.Fatal(err)
		}
		params.OtelCol.Spec.Ingress.Type = v1beta1.IngressTypeAws
		params.OtelCol.Spec.Ports = []v1beta1.PortsSpec{}
		trafficPolicy := corev1.ServiceInternalTrafficPolicyCluster

		desiredGrpcSpec := corev1.ServiceSpec{
			Type:                  corev1.ServiceTypeNodePort,
			InternalTrafficPolicy: &trafficPolicy,
			Ports: []corev1.ServicePort{
				{
					Name:        "jaeger-grpc",
					Port:        14260,
					TargetPort:  intstr.FromInt32(14260),
					Protocol:    corev1.ProtocolTCP,
					AppProtocol: &grpc,
				},
				{
					Name:        "otlp-1-grpc",
					Port:        12345,
					TargetPort:  intstr.FromInt32(12345),
					Protocol:    "",
					AppProtocol: &grpc,
				},
				{
					Name:        "otlp-2-grpc",
					Port:        98765,
					TargetPort:  intstr.FromInt32(98765),
					Protocol:    "",
					AppProtocol: &grpc,
				},
			},
		}
		desiredNonGrpcSpec := corev1.ServiceSpec{
			Type:                  corev1.ServiceTypeLoadBalancer,
			InternalTrafficPolicy: &trafficPolicy,
			Ports: []corev1.ServicePort{
				{
					Name:        "otlp-1-http",
					Port:        4318,
					TargetPort:  intstr.FromInt32(4318),
					Protocol:    "",
					AppProtocol: &grpc,
				},
				{
					Name:        "otlp-2-grpc",
					Port:        12121,
					TargetPort:  intstr.FromInt32(12121),
					Protocol:    "",
					AppProtocol: &grpc,
				},
			},
		}

		actualGrpc, err := GrpcService(params)
		assert.NoError(t, err)
		assert.Equal(t, desiredGrpcSpec.Type, actualGrpc.Spec.Type)

		desiredPorts := desiredGrpcSpec.Ports
		actualPorts := actualGrpc.Spec.Ports
		sort.Slice(desiredPorts, func(i, j int) bool { return desiredPorts[i].Name < desiredPorts[j].Name })
		sort.Slice(actualPorts, func(i, j int) bool { return actualPorts[i].Name < actualPorts[j].Name })
		assert.Equal(t, desiredPorts, actualPorts)

		actualNonGrpc, err := NonGrpcService(params)
		assert.NoError(t, err)
		assert.Equal(t, desiredNonGrpcSpec.Type, actualNonGrpc.Spec.Type)

		desiredPorts = desiredGrpcSpec.Ports
		actualPorts = actualGrpc.Spec.Ports
		sort.Slice(desiredPorts, func(i, j int) bool { return desiredPorts[i].Name < desiredPorts[j].Name })
		sort.Slice(actualPorts, func(i, j int) bool { return actualPorts[i].Name < actualPorts[j].Name })
		assert.Equal(t, desiredPorts, actualPorts)
	})

}
