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

package upgrade_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"github.com/decisiveai/opentelemetry-operator/apis/v1alpha1"
	"github.com/decisiveai/opentelemetry-operator/pkg/collector/upgrade"
)

func TestHealthCheckEndpointMigration(t *testing.T) {
	// prepare
	nsn := types.NamespacedName{Name: "my-instance", Namespace: "default"}
	existing := v1alpha1.OpenTelemetryCollector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nsn.Name,
			Namespace: nsn.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "opentelemetry-operator",
			},
		},
		Spec: v1alpha1.OpenTelemetryCollectorSpec{
			Config: `extensions:
  health_check/2:
    endpoint: "localhost:13133"
  health_check/3:
    port: 13133

receivers:
  otlp: {}
exporters:
  debug: {}

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlp]
`,
		},
	}
	existing.Status.Version = "0.23.0"

	// test
	up := &upgrade.VersionUpgrade{
		Log:      logger,
		Version:  makeVersion("0.24.0"),
		Client:   nil,
		Recorder: record.NewFakeRecorder(upgrade.RecordBufferSize),
	}
	resV1beta1, err := up.ManagedInstance(context.Background(), convertTov1beta1(t, existing))
	assert.NoError(t, err)
	res := convertTov1alpha1(t, resV1beta1)

	// verify
	assert.YAMLEq(t, `extensions:
  health_check/2:
    endpoint: localhost:13133
  health_check/3:
    endpoint: 0.0.0.0:13133

receivers:
  otlp: {}
exporters:
  debug: {}

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlp]
`, res.Spec.Config)
}
