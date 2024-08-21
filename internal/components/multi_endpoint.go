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

package components

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/mitchellh/mapstructure"
	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/internal/naming"
)

var _ ComponentPortParser = &MultiPortReceiver{}

// MultiProtocolEndpointConfig represents the minimal struct for a given YAML configuration input containing a map to
// a struct with either endpoint or listen_address.
type MultiProtocolEndpointConfig struct {
	Protocols map[string]*SingleEndpointConfig `mapstructure:"protocols"`
}

// MultiPortOption allows the setting of options for a MultiPortReceiver.
type MultiPortOption func(parser *MultiPortReceiver)

// MultiPortReceiver is a special parser for components with endpoints for each protocol.
type MultiPortReceiver struct {
	name string

	portMappings map[string]*corev1.ServicePort

	// mydecisive
	urlPaths []string
}

func (m *MultiPortReceiver) Ports(logger logr.Logger, name string, config interface{}) ([]corev1.ServicePort, error) {
	multiProtoEndpointCfg := &MultiProtocolEndpointConfig{}
	if err := mapstructure.Decode(config, multiProtoEndpointCfg); err != nil {
		return nil, err
	}
	var ports []corev1.ServicePort
	for protocol, ec := range multiProtoEndpointCfg.Protocols {
		if defaultSvc, ok := m.portMappings[protocol]; ok {
			port := defaultSvc.Port
			if ec != nil {
				port = ec.GetPortNumOrDefault(logger, port)
			}
			defaultSvc.Name = naming.PortName(fmt.Sprintf("%s-%s", name, protocol), port)
			ports = append(ports, ConstructServicePort(defaultSvc, port))
		} else {
			return nil, fmt.Errorf("unknown protocol set: %s", protocol)
		}
	}
	return ports, nil
}

// mydecisive
func (m *MultiPortReceiver) PortsWithUrlPaths(logger logr.Logger, name string, config interface{}) ([]PortUrlPaths, error) {
	multiProtoEndpointCfg := &MultiProtocolEndpointConfig{}
	if err := mapstructure.Decode(config, multiProtoEndpointCfg); err != nil {
		return []PortUrlPaths{}, nil
	}
	portsUrlParts := []PortUrlPaths{}
	for protocol, ec := range multiProtoEndpointCfg.Protocols {
		if defaultSvc, ok := m.portMappings[protocol]; ok {
			port := defaultSvc.Port
			if ec != nil {
				port = ec.GetPortNumOrDefault(logger, port)
			}
			defaultSvc.Name = naming.PortName(fmt.Sprintf("%s-%s", name, protocol), port)
			// TODO since this is used for gRPC only, we actually dont need to construct this, as gRPC urls are hardcoded
			portsUrlParts = append(portsUrlParts, PortUrlPaths{ConstructServicePort(defaultSvc, port), m.urlPaths})
		} else {
			return nil, fmt.Errorf("unknown protocol set: %s", protocol)
		}
	}
	return portsUrlParts, nil
}

func (m *MultiPortReceiver) ParserType() string {
	return ComponentType(m.name)
}

func (m *MultiPortReceiver) ParserName() string {
	return fmt.Sprintf("__%s", m.name)
}

func NewMultiPortReceiver(name string, opts ...MultiPortOption) *MultiPortReceiver {
	multiReceiver := &MultiPortReceiver{
		name:         name,
		portMappings: map[string]*corev1.ServicePort{},
		urlPaths:     []string{},
	}
	for _, opt := range opts {
		opt(multiReceiver)
	}
	return multiReceiver
}

func WithPortMapping(name string, port int32, opts ...PortBuilderOption) MultiPortOption {
	return func(parser *MultiPortReceiver) {
		servicePort := corev1.ServicePort{
			Name: naming.PortName(fmt.Sprintf("%s-%s", parser.name, name), port),
			Port: port,
		}
		// mydecisive
		servicePortWithUrlPaths := PortUrlPaths{
			Port:     servicePort,
			UrlPaths: []string{},
		}
		for _, opt := range opts {
			// mydecisive
			opt(&servicePortWithUrlPaths)
		}
		parser.portMappings[name] = &servicePortWithUrlPaths.Port
		if len(servicePortWithUrlPaths.UrlPaths) > 0 {
			parser.urlPaths = servicePortWithUrlPaths.UrlPaths
		}
	}
}
