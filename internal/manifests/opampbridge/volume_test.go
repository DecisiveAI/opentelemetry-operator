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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/decisiveai/opentelemetry-operator/apis/v1alpha1"
	"github.com/decisiveai/opentelemetry-operator/internal/config"
	"github.com/decisiveai/opentelemetry-operator/internal/naming"
)

func TestVolumeNewDefault(t *testing.T) {
	// prepare
	opampBridge := v1alpha1.OpAMPBridge{}
	cfg := config.New()

	// test
	volumes := Volumes(cfg, opampBridge)

	// verify
	assert.Len(t, volumes, 1)

	// check if the number of elements in the volume source items list is 1
	assert.Len(t, volumes[0].VolumeSource.ConfigMap.Items, 1)

	// check that it's the opamp-bridge-internal volume, with the config map
	assert.Equal(t, naming.OpAMPBridgeConfigMapVolume(), volumes[0].Name)
}
