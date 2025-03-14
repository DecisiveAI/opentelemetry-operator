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

package targetallocator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	colfg "go.opentelemetry.io/collector/featuregate"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/decisiveai/opentelemetry-operator/internal/autodetect/certmanager"
	"github.com/decisiveai/opentelemetry-operator/internal/config"
	"github.com/decisiveai/opentelemetry-operator/pkg/featuregate"
)

func TestServicePorts(t *testing.T) {
	targetAllocator := targetAllocatorInstance()
	cfg := config.New()

	params := Params{
		TargetAllocator: targetAllocator,
		Config:          cfg,
		Log:             logger,
	}

	ports := []v1.ServicePort{{Name: "targetallocation", Port: 80, TargetPort: intstr.FromString("http")}}

	s := Service(params)

	assert.Equal(t, ports[0].Name, s.Spec.Ports[0].Name)
	assert.Equal(t, ports[0].Port, s.Spec.Ports[0].Port)
	assert.Equal(t, ports[0].TargetPort, s.Spec.Ports[0].TargetPort)
}

func TestServicePortsWithTargetAllocatorMTLS(t *testing.T) {
	targetAllocator := targetAllocatorInstance()
	cfg := config.New(config.WithCertManagerAvailability(certmanager.Available))

	flgs := featuregate.Flags(colfg.GlobalRegistry())
	err := flgs.Parse([]string{"--feature-gates=operator.targetallocator.mtls"})
	require.NoError(t, err)

	params := Params{
		TargetAllocator: targetAllocator,
		Config:          cfg,
		Log:             logger,
	}

	ports := []v1.ServicePort{
		{Name: "targetallocation", Port: 80, TargetPort: intstr.FromString("http")},
		{Name: "targetallocation-https", Port: 443, TargetPort: intstr.FromString("https")},
	}

	s := Service(params)

	assert.Equal(t, ports[0].Name, s.Spec.Ports[0].Name)
	assert.Equal(t, ports[0].Port, s.Spec.Ports[0].Port)
	assert.Equal(t, ports[0].TargetPort, s.Spec.Ports[0].TargetPort)
	assert.Equal(t, ports[1].Name, s.Spec.Ports[1].Name)
	assert.Equal(t, ports[1].Port, s.Spec.Ports[1].Port)
	assert.Equal(t, ports[1].TargetPort, s.Spec.Ports[1].TargetPort)
}
