// Copyright 2019 The Cluster Monitoring Operator Authors
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

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/openshift/cluster-monitoring-operator/pkg/manifests"
)

func TestPrometheusMetrics(t *testing.T) {
	for service, expected := range map[string]int{
		"prometheus-operator":           1,
		"prometheus-k8s":                2,
		"prometheus-k8s-thanos-sidecar": 2,
		"thanos-querier":                2,
		"prometheus-adapter":            2,
		"alertmanager-main":             3,
		"kube-state-metrics":            2, // one for the kube metrics + one for the metrics of the process itself.
		"openshift-state-metrics":       2, // ditto.
		"telemeter-client":              1,
		"grafana":                       1,
	} {
		t.Run(service, func(t *testing.T) {
			f.ThanosQuerierClient.WaitForQueryReturn(
				t, 10*time.Minute, fmt.Sprintf(`count(up{service="%s",namespace="openshift-monitoring"} == 1)`, service),
				func(i int) error {
					if i != expected {
						return fmt.Errorf("expected %d targets to be up but got %d", expected, i)
					}

					return nil
				},
			)
		})
	}
}

func TestPrometheusAlertmanagerAntiAffinity(t *testing.T) {
	ctx := context.Background()
	pods, err := f.KubeClient.CoreV1().Pods(f.Ns).List(ctx, metav1.ListOptions{FieldSelector: "status.phase=Running"})
	if err != nil {
		t.Fatal(err)
	}

	var (
		testPod1      = "alertmanager-main"
		testPod2      = "prometheus-k8s"
		testNameSpace = "openshift-monitoring"

		almOk = false
		k8sOk = false
	)

	for _, p := range pods.Items {
		if strings.Contains(p.Namespace, testNameSpace) &&
			strings.Contains(p.Name, testPod1) {
			if p.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
				almOk = true
			} else {
				t.Fatal("Failed to find preferredDuringSchedulingIgnoredDuringExecution in alertmanager pod spec")
			}
		}

		if strings.Contains(p.Namespace, testNameSpace) &&
			strings.Contains(p.Name, testPod2) {
			if p.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
				k8sOk = true
			} else {
				t.Fatal("Failed to find preferredDuringSchedulingIgnoredDuringExecution in prometheus pod spec")
			}
		}
	}

	if !almOk == true || !k8sOk == true {
		t.Fatal("Can not find pods: prometheus-k8s or alertmanager-main")
	}
}

func TestPrometheusRemoteWrite(t *testing.T) {
	ctx := context.Background()

	targetDeployment := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: instrumented-sample-app
  namespace: openshift-monitoring
  labels:
    group: test
spec:
  replicas: 1
  selector:
    matchLabels:
      group: test
  template:
    metadata:
      labels:
        group: test
    spec:
      containers:
      - name: example-app
        args:
        - --cert-path=/etc/certs
        image: quay.io/coreos/instrumented-sample-app:0.2.0-bearer-mtls-1
        imagePullPolicy: IfNotPresent
        ports:
        - name: web
          containerPort: 8080
        - name: mtls
          containerPort: 8081
        volumeMounts:
        - mountPath: /etc/certs
          name: certs
      volumes:
      - name: certs
        secret:
          secretName: selfsigned-mtls-bundle
        items:
        - key: server-ca.pem
        path: cert.pem
        - key: server.key
        path: key.pem
`
	rwTestDeployment, err := manifests.NewDeployment(bytes.NewReader([]byte(targetDeployment)))
	if err != nil {
		t.Fatal(err)
	}

	// create service with annotation for service-ca operator
	name := "test"
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"group": name,
			},
			Namespace: f.Ns,
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
			Ports: []v1.ServicePort{
				{
					Name: "web",
					Port: 8080,
				},
				{
					Name: "mtls",
					Port: 8081,
				},
			},
			Selector: map[string]string{
				"group": name,
			},
		},
	}

	if err := f.OperatorClient.CreateOrUpdateService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	deployedService, err := f.KubeClient.CoreV1().Services(f.Ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tlsSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "selfsigned-mtls-bundle",
		},
		Data: map[string][]byte{
			"client-cert-name": []byte("test-client"),
			"serving-cert-url": []byte(deployedService.Spec.ClusterIP),
		},
	}
	sClient := f.OperatorClient.kclient.CoreV1().Secrets(f.Ns)
	sec := sClient.Create(ctx, tlsSecret, metav1.CreateOptions{})
	if err := createSelfSignedMTLSArtifacts(sec); err != nil {
		t.Fatal(err)
	}

	// deploy remote write target
	if err := f.OperatorClient.CreateOrUpdateDeployment(ctx, rwTestDeployment); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name   string
		port   string
		rwSpec string
	}{
		// check remote write logs
		{
			name: "assert remote write to http works",
			port: "8080",
			rwSpec: `
  - url: "http://%s"`,
		},
		{
			name: "assert mtls remote write works",
			port: "8081",
			rwSpec: `
  - url: "https://%s"
    tlsConfig:
      ca:
        secret:
          name: selfsigned-tls-bundle
          key: ca.crt
      cert:
        secret:
          name: selfsigned-tls-bundle
          key: client.crt
      keySecret:
        secret:
          name: selfsigned-tls-bundle
          key: client.key
`,
		},
	} {
		rw := fmt.Sprintf(scenario.rwSpec, deployedService.Spec.ClusterIP+":"+scenario.port)

		cmoConfigMap := fmt.Sprintf(`prometheusK8s:
  logLevel: debug
  retention: 10h
  tolerations:
    - operator: "Exists"
  remoteWrite: "%s"
`, rw)
		t.Run(scenario.name, assertRemoteWriteInLogs(name, cmoConfigMap, ctx))
	}
}

func assertRemoteWriteInLogs(name string, cmoConfigMap string, ctx context.Context) func(*testing.T) {
	return func(t *testing.T) {
		if err := f.OperatorClient.CreateOrUpdateConfigMap(ctx, configMapWithData(t, cmoConfigMap)); err != nil {
			t.Fatal(err)
		}

		promLogs0, err := f.GetLogs(f.Ns, "prometheus-k8s-0", "prometheus")
		if err != nil {
			t.Fatal(err)
		}

		promLogs1, err := f.GetLogs(f.Ns, "prometheus-k8s-1", "prometheus")
		if err != nil {
			t.Fatal(err)
		}
		var promLogs strings.Builder
		promLogErr := "prometheus logs are empty, expected to find log messages"
		if i, _ := promLogs.WriteString(promLogs0); i == 0 {
			t.Fatal(promLogErr)
		}
		if i, _ := promLogs.WriteString(promLogs1); i == 0 {
			t.Fatal(promLogErr)
		}

		rwEndpointOpts := metav1.ListOptions{LabelSelector: labels.FormatLabels(map[string]string{"group": name})}

		rwEndpointPodList, err := f.KubeClient.CoreV1().Pods(f.Ns).List(ctx, rwEndpointOpts)
		if err != nil {
			t.Fatal(err)
		}
		rwEndpointLogs, err := f.GetLogs(f.Ns, rwEndpointPodList.Items[0].ObjectMeta.Name, "")
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(promLogs.String(), `msg="Failed to send batch, retrying`) {
			t.Fatal("unexpected prometheus log message, failed to send batch to remote write endpoint")
		}
		if strings.Contains(rwEndpointLogs, "remote error: tls: bad certificate") {
			t.Fatal("remote write tls endpoint sees bad or no certificate")
		}
	}
}
