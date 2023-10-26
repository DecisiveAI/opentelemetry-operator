helm install \
	--set admissionWebhooks.certManager.enabled=false \
	--set admissionWebhooks.certManager.autoGenerateCert=false \
	--set admissionWebhooks.cert_file=crt/tls.crt \
	--set admissionWebhooks.key_file=crt/tls.key \
	--set admissionWebhooks.ca_file=crt/tls.key \
  opentelemetry-operator open-telemetry/opentelemetry-operator
