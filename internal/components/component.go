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
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	GrpcProtocol          = "grpc"
	HttpProtocol          = "http"
	UnsetPort       int32 = 0
	PortNotFoundErr       = errors.New("port should not be empty")
)

type PortRetriever interface {
	GetPortNum() (int32, error)
	GetPortNumOrDefault(logr.Logger, int32) int32
}

// mydecisive.
// PortUrlPaths represents a service port and a list of URL paths
type PortUrlPaths struct {
	Port     corev1.ServicePort
	UrlPaths []string
}

// mydecisive.
// ComponentsPortsUrlPaths maps  component name to the list of PortUrlPaths
type ComponentsPortsUrlPaths map[string][]PortUrlPaths

type PortBuilderOption func(portUrlPaths *PortUrlPaths)

// mydecisive
func WithTargetPort(targetPort int32) PortBuilderOption {
	return func(portUrlPaths *PortUrlPaths) {
		portUrlPaths.Port.TargetPort = intstr.FromInt32(targetPort)
	}
}

// mydecisive
func WithAppProtocol(proto *string) PortBuilderOption {
	return func(portUrlPaths *PortUrlPaths) {
		portUrlPaths.Port.AppProtocol = proto
	}
}

// mydecisive
func WithProtocol(proto corev1.Protocol) PortBuilderOption {
	return func(portUrlPaths *PortUrlPaths) {
		portUrlPaths.Port.Protocol = proto
	}
}

// mydecisive
func WithUrlPaths(urlPaths []string) PortBuilderOption {
	return func(portUrlPaths *PortUrlPaths) {
		portUrlPaths.UrlPaths = urlPaths
	}
}

// ComponentType returns the type for a given component name.
// components have a name like:
// - mycomponent/custom
// - mycomponent
// we extract the "mycomponent" part and see if we have a parser for the component.
func ComponentType(name string) string {
	if strings.Contains(name, "/") {
		return name[:strings.Index(name, "/")]
	}
	return name
}

func PortFromEndpoint(endpoint string) (int32, error) {
	var err error
	var port int64

	r := regexp.MustCompile(":[0-9]+")

	if r.MatchString(endpoint) {
		portStr := r.FindString(endpoint)
		cleanedPortStr := strings.Replace(portStr, ":", "", -1)
		port, err = strconv.ParseInt(cleanedPortStr, 10, 32)

		if err != nil {
			return UnsetPort, err
		}
	}

	if port == 0 {
		return UnsetPort, PortNotFoundErr
	}

	return int32(port), err
}

type ParserRetriever func(string) ComponentPortParser

type ComponentPortParser interface {
	// Ports returns the service ports parsed based on the component's configuration where name is the component's name
	// of the form "name" or "type/name"
	Ports(logger logr.Logger, name string, config interface{}) ([]corev1.ServicePort, error)

	// ParserType returns the type of this parser
	ParserType() string

	// ParserName is an internal name for the parser
	ParserName() string
	//mydecisive
	// PortsWithUrlPaths returns the service ports + URL paths parsed based on the receiver's configuration
	PortsWithUrlPaths(logger logr.Logger, name string, config interface{}) ([]PortUrlPaths, error)
}

func ConstructServicePort(current *corev1.ServicePort, port int32) corev1.ServicePort {
	svc := corev1.ServicePort{
		Name:        current.Name,
		Port:        port,
		NodePort:    current.NodePort,
		AppProtocol: current.AppProtocol,
		Protocol:    current.Protocol,
	}

	if port > 0 && current.TargetPort.IntValue() > 0 {
		svc.TargetPort = intstr.FromInt32(port)
	}
	return svc
}

func GetPortsForConfig(logger logr.Logger, config map[string]interface{}, retriever ParserRetriever) ([]corev1.ServicePort, error) {
	var ports []corev1.ServicePort
	for componentName, componentDef := range config {
		parser := retriever(componentName)
		if parsedPorts, err := parser.Ports(logger, componentName, componentDef); err != nil {
			return nil, err
		} else {
			ports = append(ports, parsedPorts...)
		}
	}
	return ports, nil
}
