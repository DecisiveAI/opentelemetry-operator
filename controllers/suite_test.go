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

package controllers_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/open-telemetry/opentelemetry-operator/internal/autodetect"
	"github.com/open-telemetry/opentelemetry-operator/internal/autodetect/openshift"
	"github.com/open-telemetry/opentelemetry-operator/internal/config"
	"github.com/open-telemetry/opentelemetry-operator/internal/manifests"
	"github.com/open-telemetry/opentelemetry-operator/internal/manifests/collector/testdata"
	"github.com/open-telemetry/opentelemetry-operator/internal/rbac"
	// +kubebuilder:scaffold:imports
)

var (
	k8sClient  client.Client
	testEnv    *envtest.Environment
	testScheme *runtime.Scheme = scheme.Scheme
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	cfg        *rest.Config
	logger     = logf.Log.WithName("unit-tests")

	instanceUID      = uuid.NewUUID()
	mockAutoDetector = &mockAutoDetect{
		OpenShiftRoutesAvailabilityFunc: func() (openshift.RoutesAvailability, error) {
			return openshift.RoutesAvailable, nil
		},
	}
)

const (
	defaultCollectorImage    = "default-collector"
	defaultTaAllocationImage = "default-ta-allocator"
	defaultOpAMPBridgeImage  = "default-opamp-bridge"
	promFile                 = "testdata/test.yaml"
	updatedPromFile          = "testdata/test_ta_update.yaml"
	testFileIngress          = "testdata/ingress_testdata.yaml"
)

var _ autodetect.AutoDetect = (*mockAutoDetect)(nil)

type mockAutoDetect struct {
	OpenShiftRoutesAvailabilityFunc func() (openshift.RoutesAvailability, error)
}

func (m *mockAutoDetect) OpenShiftRoutesAvailability() (openshift.RoutesAvailability, error) {
	if m.OpenShiftRoutesAvailabilityFunc != nil {
		return m.OpenShiftRoutesAvailabilityFunc()
	}
	return openshift.RoutesNotAvailable, nil
}

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.TODO())
	defer cancel()

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
		CRDs:              []*apiextensionsv1.CustomResourceDefinition{testdata.OpenShiftRouteCRD},
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "config", "webhook")},
		},
	}
	cfg, err = testEnv.Start()
	if err != nil {
		fmt.Printf("failed to start testEnv: %v", err)
		os.Exit(1)
	}

	if err = routev1.AddToScheme(testScheme); err != nil {
		fmt.Printf("failed to register scheme: %v", err)
		os.Exit(1)
	}

	if err = v1alpha1.AddToScheme(testScheme); err != nil {
		fmt.Printf("failed to register scheme: %v", err)
		os.Exit(1)
	}
	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		fmt.Printf("failed to setup a Kubernetes client: %v", err)
		os.Exit(1)
	}

	// start webhook server using Manager
	webhookInstallOptions := &testEnv.WebhookInstallOptions
	mgr, mgrErr := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:         testScheme,
		LeaderElection: false,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    webhookInstallOptions.LocalServingHost,
			Port:    webhookInstallOptions.LocalServingPort,
			CertDir: webhookInstallOptions.LocalServingCertDir,
		}),
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	if mgrErr != nil {
		fmt.Printf("failed to start webhook server: %v", mgrErr)
		os.Exit(1)
	}
	clientset, clientErr := kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Printf("failed to setup kubernetes clientset %v", clientErr)
	}
	reviewer := rbac.NewReviewer(clientset)

	if err = v1alpha1.SetupCollectorWebhook(mgr, config.New(), reviewer); err != nil {
		fmt.Printf("failed to SetupWebhookWithManager: %v", err)
		os.Exit(1)
	}

	if err = v1alpha1.SetupOpAMPBridgeWebhook(mgr, config.New()); err != nil {
		fmt.Printf("failed to SetupWebhookWithManager: %v", err)
		os.Exit(1)
	}

	ctx, cancel = context.WithCancel(context.TODO())
	defer cancel()
	go func() {
		if err = mgr.Start(ctx); err != nil {
			fmt.Printf("failed to start manager: %v", err)
			os.Exit(1)
		}
	}()

	// wait for the webhook server to get ready
	wg := &sync.WaitGroup{}
	wg.Add(1)
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		if err = retry.OnError(wait.Backoff{
			Steps:    20,
			Duration: 10 * time.Millisecond,
			Factor:   1.5,
			Jitter:   0.1,
			Cap:      time.Second * 30,
		}, func(error) bool {
			return true
		}, func() error {
			// #nosec G402
			conn, tlsErr := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
			if tlsErr != nil {
				return tlsErr
			}
			_ = conn.Close()
			return nil
		}); err != nil {
			fmt.Printf("failed to wait for webhook server to be ready: %v", err)
			os.Exit(1)
		}
	}(wg)
	wg.Wait()

	code := m.Run()

	err = testEnv.Stop()
	if err != nil {
		fmt.Printf("failed to stop testEnv: %v", err)
		os.Exit(1)
	}

	os.Exit(code)
}

func paramsWithMode(mode v1alpha1.Mode) manifests.Params {
	replicas := int32(2)
	return paramsWithModeAndReplicas(mode, replicas)
}

func paramsWithModeAndReplicas(mode v1alpha1.Mode, replicas int32) manifests.Params {
	configYAML, err := os.ReadFile("testdata/test.yaml")
	if err != nil {
		fmt.Printf("Error getting yaml file: %v", err)
	}
	return manifests.Params{
		Config: config.New(config.WithCollectorImage(defaultCollectorImage), config.WithTargetAllocatorImage(defaultTaAllocationImage)),
		Client: k8sClient,
		OtelCol: v1alpha1.OpenTelemetryCollector{
			TypeMeta: metav1.TypeMeta{
				Kind:       "opentelemetry.io",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Spec: v1alpha1.OpenTelemetryCollectorSpec{
				Image: "ghcr.io/open-telemetry/opentelemetry-operator/opentelemetry-operator:0.47.0",
				Ports: []v1.ServicePort{{
					Name: "web",
					Port: 80,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 80,
					},
					NodePort: 0,
				}},
				Replicas: &replicas,
				Config:   string(configYAML),
				Mode:     mode,
			},
		},
		Scheme:   testScheme,
		Log:      logger,
		Recorder: record.NewFakeRecorder(10),
	}
}

func newParams(taContainerImage string, file string) (manifests.Params, error) {
	replicas := int32(1)
	var configYAML []byte
	var err error

	if file == "" {
		configYAML, err = os.ReadFile("testdata/test.yaml")
	} else {
		configYAML, err = os.ReadFile(file)
	}
	if err != nil {
		return manifests.Params{}, fmt.Errorf("Error getting yaml file: %w", err)
	}

	cfg := config.New(config.WithCollectorImage(defaultCollectorImage), config.WithTargetAllocatorImage(defaultTaAllocationImage))

	return manifests.Params{
		Config: cfg,
		Client: k8sClient,
		OtelCol: v1alpha1.OpenTelemetryCollector{
			TypeMeta: metav1.TypeMeta{
				Kind:       "opentelemetry.io",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Spec: v1alpha1.OpenTelemetryCollectorSpec{
				Mode: v1alpha1.ModeStatefulSet,
				Ports: []v1.ServicePort{{
					Name: "web",
					Port: 80,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 80,
					},
					NodePort: 0,
				}},
				TargetAllocator: v1alpha1.OpenTelemetryTargetAllocator{
					Enabled: true,
					Image:   taContainerImage,
				},
				Replicas: &replicas,
				Config:   string(configYAML),
			},
		},
		Scheme: testScheme,
		Log:    logger,
	}, nil
}

func paramsWithHPA(minReps, maxReps int32) manifests.Params {
	configYAML, err := os.ReadFile("testdata/test.yaml")
	if err != nil {
		fmt.Printf("Error getting yaml file: %v", err)
	}

	cpuUtilization := int32(90)

	configuration := config.New(config.WithCollectorImage(defaultCollectorImage), config.WithTargetAllocatorImage(defaultTaAllocationImage))

	return manifests.Params{
		Config: configuration,
		Client: k8sClient,
		OtelCol: v1alpha1.OpenTelemetryCollector{
			TypeMeta: metav1.TypeMeta{
				Kind:       "opentelemetry.io",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hpatest",
				Namespace: "default",
				UID:       instanceUID,
			},
			Spec: v1alpha1.OpenTelemetryCollectorSpec{
				Ports: []v1.ServicePort{{
					Name: "web",
					Port: 80,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 80,
					},
					NodePort: 0,
				}},
				Config: string(configYAML),
				Autoscaler: &v1alpha1.AutoscalerSpec{
					MinReplicas:          &minReps,
					MaxReplicas:          &maxReps,
					TargetCPUUtilization: &cpuUtilization,
				},
			},
		},
		Scheme:   testScheme,
		Log:      logger,
		Recorder: record.NewFakeRecorder(10),
	}
}

func paramsWithPolicy(minAvailable, maxUnavailable int32) manifests.Params {
	configYAML, err := os.ReadFile("testdata/test.yaml")
	if err != nil {
		fmt.Printf("Error getting yaml file: %v", err)
	}

	configuration := config.New(config.WithAutoDetect(mockAutoDetector), config.WithCollectorImage(defaultCollectorImage), config.WithTargetAllocatorImage(defaultTaAllocationImage))
	err = configuration.AutoDetect()
	if err != nil {
		logger.Error(err, "configuration.autodetect failed")
	}

	pdb := &v1alpha1.PodDisruptionBudgetSpec{}

	if maxUnavailable > 0 && minAvailable > 0 {
		fmt.Printf("worng configuration: %v", fmt.Errorf("minAvailable and maxUnavailable cannot be both set"))
	}
	if maxUnavailable > 0 {
		pdb.MaxUnavailable = &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: maxUnavailable,
		}
	} else {
		pdb.MinAvailable = &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: minAvailable,
		}
	}

	return manifests.Params{
		Config: configuration,
		Client: k8sClient,
		OtelCol: v1alpha1.OpenTelemetryCollector{
			TypeMeta: metav1.TypeMeta{
				Kind:       "opentelemetry.io",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policytest",
				Namespace: "default",
				UID:       instanceUID,
			},
			Spec: v1alpha1.OpenTelemetryCollectorSpec{
				Ports: []v1.ServicePort{{
					Name: "web",
					Port: 80,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 80,
					},
					NodePort: 0,
				}},
				Config:              string(configYAML),
				PodDisruptionBudget: pdb,
			},
		},
		Scheme:   testScheme,
		Log:      logger,
		Recorder: record.NewFakeRecorder(10),
	}
}

func opampBridgeParams() manifests.Params {
	return manifests.Params{
		Config: config.New(config.WithOperatorOpAMPBridgeImage(defaultOpAMPBridgeImage)),
		Client: k8sClient,
		OpAMPBridge: v1alpha1.OpAMPBridge{
			TypeMeta: metav1.TypeMeta{
				Kind:       "opentelemetry.io",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
				UID:       instanceUID,
			},
			Spec: v1alpha1.OpAMPBridgeSpec{
				Image: "ghcr.io/open-telemetry/opentelemetry-operator/operator-opamp-bridge:0.69.0",
				Ports: []v1.ServicePort{
					{
						Name: "metrics",
						Port: 8081,
						TargetPort: intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: 8081,
						},
					},
				},
				Endpoint: "ws://127.0.0.1:4320/v1/opamp",
				Capabilities: map[v1alpha1.OpAMPBridgeCapability]bool{
					v1alpha1.OpAMPBridgeCapabilityReportsStatus:                  true,
					v1alpha1.OpAMPBridgeCapabilityAcceptsRemoteConfig:            true,
					v1alpha1.OpAMPBridgeCapabilityReportsEffectiveConfig:         true,
					v1alpha1.OpAMPBridgeCapabilityReportsOwnTraces:               true,
					v1alpha1.OpAMPBridgeCapabilityReportsOwnMetrics:              true,
					v1alpha1.OpAMPBridgeCapabilityReportsOwnLogs:                 true,
					v1alpha1.OpAMPBridgeCapabilityAcceptsOpAMPConnectionSettings: true,
					v1alpha1.OpAMPBridgeCapabilityAcceptsOtherConnectionSettings: true,
					v1alpha1.OpAMPBridgeCapabilityAcceptsRestartCommand:          true,
					v1alpha1.OpAMPBridgeCapabilityReportsHealth:                  true,
					v1alpha1.OpAMPBridgeCapabilityReportsRemoteConfig:            true,
				},
				ComponentsAllowed: map[string][]string{"receivers": {"otlp"}, "processors": {"memory_limiter"}, "exporters": {"logging"}},
			},
		},
		Scheme:   testScheme,
		Log:      logger,
		Recorder: record.NewFakeRecorder(10),
	}
}

func populateObjectIfExists(t testing.TB, object client.Object, namespacedName types.NamespacedName) (bool, error) {
	t.Helper()
	err := k8sClient.Get(context.Background(), namespacedName, object)
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
