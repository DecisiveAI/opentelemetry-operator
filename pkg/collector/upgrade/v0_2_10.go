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

package upgrade

import (
	"github.com/decisiveai/opentelemetry-operator/apis/v1alpha1"
)

// this is our first version under otel/opentelemetry-collector.
func upgrade0_2_10(u VersionUpgrade, otelcol *v1alpha1.OpenTelemetryCollector) (*v1alpha1.OpenTelemetryCollector, error) {
	// this is a no-op, but serves to keep the skeleton here for the future versions
	return otelcol, nil
}
