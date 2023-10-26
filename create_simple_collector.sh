kubectl apply -f - <<EOF
apiVersion: opentelemetry.io/v1alpha1
kind: OpenTelemetryCollector
metadata:
  name: my-collector
spec:
  mode: deployment # This configuration is omittable.
  config: |
    receivers:
      jaeger:
        protocols:
          grpc:
    processors:

    exporters:
      debug:

    service:
      pipelines:
        traces:
          receivers: [jaeger]
          processors: []
          exporters: [debug]
EOF
