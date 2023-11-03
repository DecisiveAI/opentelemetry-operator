package parser

import corev1 "k8s.io/api/core/v1"

type PortUrlPaths struct {
	Port     corev1.ServicePort
	UrlPaths []string
}
