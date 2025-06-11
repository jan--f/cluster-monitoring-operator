// Copyright 2018 The Cluster Monitoring Operator Authors
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

package tasks

import (
	"context"
	"fmt"

	apiutilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"

	"github.com/openshift/cluster-monitoring-operator/pkg/client"
	"github.com/openshift/cluster-monitoring-operator/pkg/manifests"
)

type PrometheusTelemetryTask struct {
	client  *client.Client
	factory *manifests.Factory
	config  *manifests.Config
}

func NewPrometheusTelemetryTask(client *client.Client, factory *manifests.Factory, config *manifests.Config) *PrometheusTelemetryTask {
	return &PrometheusTelemetryTask{
		client:  client,
		factory: factory,
		config:  config,
	}
}

func (t *PrometheusTelemetryTask) Run(ctx context.Context) error {
	errs := []error{}

	err := t.create(ctx)
	if err != nil {
		klog.V(4).ErrorS(err, "updation of prometheus failed")
		errs = append(errs, err)
	}

	// NOTE: the validation task is run even if creation fails so that
	// existing deployment is validated.
	validation := NewPrometheusValidationTask(t.client, t.factory)
	errs = append(errs, validation.Run(ctx))

	return apiutilerrors.NewAggregate(errs)
}

func (t *PrometheusTelemetryTask) create(ctx context.Context) error {
	cacm, err := t.factory.PrometheusTelemetryServingCertsCABundle()
	if err != nil {
		return fmt.Errorf("initializing serving certs CA Bundle ConfigMap failed: %w", err)
	}

	_, err = t.client.CreateIfNotExistConfigMap(ctx, cacm)
	if err != nil {
		return fmt.Errorf("creating serving certs CA Bundle ConfigMap failed: %w", err)
	}

	sa, err := t.factory.PrometheusTelemetryServiceAccount()
	if err != nil {
		return fmt.Errorf("initializing Prometheus ServiceAccount failed: %w", err)
	}

	err = t.client.CreateOrUpdateServiceAccount(ctx, sa)
	if err != nil {
		return fmt.Errorf("reconciling Prometheus ServiceAccount failed: %w", err)
	}

	cr, err := t.factory.PrometheusTelemetryClusterRole()
	if err != nil {
		return fmt.Errorf("initializing Prometheus ClusterRole failed: %w", err)
	}

	err = t.client.CreateOrUpdateClusterRole(ctx, cr)
	if err != nil {
		return fmt.Errorf("reconciling Prometheus ClusterRole failed: %w", err)
	}

	crb, err := t.factory.PrometheusTelemetryClusterRoleBinding()
	if err != nil {
		return fmt.Errorf("initializing Prometheus ClusterRoleBinding failed: %w", err)
	}

	err = t.client.CreateOrUpdateClusterRoleBinding(ctx, crb)
	if err != nil {
		return fmt.Errorf("reconciling Prometheus ClusterRoleBinding failed: %w", err)
	}

	scrapeSec, err := t.factory.PrometheusTelemetryScrapeSecret()
	if err != nil {
		return fmt.Errorf("initializing Prometheus Telemetry Scrape secret failed: %w", err)
	}

	err = t.client.CreateOrUpdateSecret(ctx, scrapeSec)
	if err != nil {
		return fmt.Errorf("error creating Prometheus Telemetry Scrape secret: %w", err)
	}

	_, err = t.client.WaitForSecret(ctx, scrapeSec)
	if err != nil {
		return fmt.Errorf("waiting for Prometheus Telemetry Scrape secret failed: %w", err)
	}

	klog.V(4).Info("initializing Prometheus object")
	p, err := t.factory.PrometheusTelemetry()
	if err != nil {
		return fmt.Errorf("initializing Prometheus object failed: %w", err)
	}

	klog.V(4).Info("reconciling Prometheus object")
	err = t.client.CreateOrUpdatePrometheus(ctx, p)
	if err != nil {
		return fmt.Errorf("reconciling Prometheus object failed: %w", err)
	}

	return nil
}
