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

package client

import (
	"context"
	"reflect"

	v1 "github.com/openshift/api/config/v1"
	clientv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	cmostr "github.com/openshift/cluster-monitoring-operator/pkg/strings"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	unavailableMessage                        string = "Rollout of the monitoring stack failed and is degraded. Please investigate the degraded status error."
	asExpectedReason                          string = "AsExpected"
	StorageNotConfiguredMessage                      = "Prometheus is running without persistent storage which can lead to data loss during upgrades and cluster disruptions. Please refer to the official documentation to see how to configure storage for Prometheus: "
	StorageNotConfiguredReason                       = "PrometheusDataPersistenceNotConfigured"
	UserAlermanagerConfigMisconfiguredMessage        = "Misconfigured Alertmanager:  Alertmanager for user-defined alerting is enabled in the openshift-monitoring/cluster-monitoring-config configmap by setting 'enableUserAlertmanagerConfig: true' field. This conflicts with a dedicated Alertmanager instance enabled in  openshift-user-workload-monitoring/user-workload-monitoring-config. Alertmanager enabled in openshift-user-workload-monitoring takes precedence over the one in openshift-monitoring, so please remove the 'enableUserAlertmanagerConfig' field in openshift-monitoring/cluster-monitoring-config."
	UserAlermanagerConfigMisconfiguredReason         = "UserAlertmanagerMisconfigured"
)

// Status represents if the state being reported is known to be True, False, or Unknown.
//
// NOTE: redefined instead of reusing openshift/config/v1 to avoid coupling
type Status string

const (
	TrueStatus    Status = "True"
	FalseStatus   Status = "False"
	UnknownStatus Status = "Unknown"
)

// StateInfo provides all information required by the StatusReporter to report
// a condition's state such as Status, Reason, and Message
type StateInfo interface {
	Status() Status
	Reason() string
	Message() string
}

// StatesReport represents State Information about various States that the
// StatusReporter will report on such as the Degraded and Available.
type StatesReport interface {
	Degraded() StateInfo
	Available() StateInfo
}

type StatusReporter struct {
	client                clientv1.ClusterOperatorInterface
	clusterOperatorName   string
	namespace             string
	userWorkloadNamespace string
	version               string
}

func NewStatusReporter(client clientv1.ClusterOperatorInterface, name, namespace, userWorkloadNamespace, version string) *StatusReporter {
	return &StatusReporter{
		client:                client,
		clusterOperatorName:   name,
		namespace:             namespace,
		userWorkloadNamespace: userWorkloadNamespace,
		version:               version,
	}
}

func (r *StatusReporter) relatedObjects() []v1.ObjectReference {
	return []v1.ObjectReference{
		// Gather pods, services, daemonsets, deployments, replicasets, statefulsets, and routes.
		{Resource: "namespaces", Name: r.namespace},
		{Resource: "namespaces", Name: r.userWorkloadNamespace},
		// Gather all ServiceMonitors, PodMonitors, PrometheusRules, Alertmanagers, AlertmanagerConfigs, ThanosRulers and Prometheus CRs
		{Group: "monitoring.coreos.com", Resource: "servicemonitors"},
		{Group: "monitoring.coreos.com", Resource: "podmonitors"},
		{Group: "monitoring.coreos.com", Resource: "prometheusrules"},
		{Group: "monitoring.coreos.com", Resource: "alertmanagers"},
		{Group: "monitoring.coreos.com", Resource: "prometheuses"},
		{Group: "monitoring.coreos.com", Resource: "thanosrulers"},
		{Group: "monitoring.coreos.com", Resource: "alertmanagerconfigs"},
	}
}

func (r *StatusReporter) Get(ctx context.Context) (*v1.ClusterOperator, error) {
	return r.client.Get(ctx, r.clusterOperatorName, metav1.GetOptions{})
}

func (r *StatusReporter) Create(ctx context.Context, co *v1.ClusterOperator) (*v1.ClusterOperator, error) {
	return r.client.Create(ctx, co, metav1.CreateOptions{})
}

func (r *StatusReporter) getOrCreateClusterOperator(ctx context.Context) (*v1.ClusterOperator, error) {
	co, err := r.Get(ctx)
	if apierrors.IsNotFound(err) {
		co = r.newClusterOperator()
		co, err = r.Create(ctx, co)
	}
	if err != nil {
		return nil, err
	}
	return co, nil
}

func (r *StatusReporter) newClusterOperator() *v1.ClusterOperator {
	time := metav1.Now()
	co := &v1.ClusterOperator{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "config.openshift.io/v1",
			Kind:       "ClusterOperator",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: r.clusterOperatorName,
		},
		Spec:   v1.ClusterOperatorSpec{},
		Status: v1.ClusterOperatorStatus{},
	}
	co.Status.RelatedObjects = r.relatedObjects()
	co.Status.Conditions = newConditions(co.Status, r.version, time).entries()

	return co
}

func (r *StatusReporter) setConditions(ctx context.Context, co *v1.ClusterOperator, conditions *conditions) error {
	updatedStatus := co.Status.DeepCopy()
	updatedStatus.Conditions = conditions.entries()
	updatedStatus.RelatedObjects = r.relatedObjects()
	if reflect.DeepEqual(co.Status, *updatedStatus) {
		klog.V(4).Infof("No status update necessary, objects are identical")
		return nil
	}
	co.Status = *updatedStatus
	_, err := r.client.UpdateStatus(ctx, co, metav1.UpdateOptions{})
	return err
}

func (r *StatusReporter) SetRollOutDone(ctx context.Context, degradedConditionMessage string, degradedConditionReason string) error {
	co, err := r.getOrCreateClusterOperator(ctx)
	if err != nil {
		return err
	}

	time := metav1.Now()
	conditions := newConditions(co.Status, r.version, time)
	conditions.setCondition(v1.OperatorAvailable, v1.ConditionTrue, "Successfully rolled out the stack.", "RollOutDone", time)
	conditions.setCondition(v1.OperatorProgressing, v1.ConditionFalse, "", "", time)
	conditions.setCondition(v1.OperatorDegraded, v1.ConditionFalse, degradedConditionMessage, degradedConditionReason, time)

	// If we have reached "level" for the operator, report that we are at the version
	// injected into us during update. We require that all components be rolled out
	// and available at the new version before reporting this value.
	if len(r.version) > 0 {
		co.Status.Versions = []v1.OperandVersion{
			{
				Name:    "operator",
				Version: r.version,
			},
		}
	} else {
		co.Status.Versions = nil
	}

	return r.setConditions(ctx, co, conditions)
}

// SetRollOutInProgress sets the OperatorProgressing condition to true, either:
// 1. If there has been no previous status yet
// 2. If the previous ClusterOperator OperatorAvailable condition was false
//
// This will ensure that the progressing state will be only set initially or in case of failure.
// Once controller operator versions are available, an additional check will be introduced that toggles
// the OperatorProgressing state in case of version upgrades.
func (r *StatusReporter) SetRollOutInProgress(ctx context.Context) error {
	co, err := r.getOrCreateClusterOperator(ctx)
	if err != nil {
		return err
	}

	time := metav1.Now()
	conditions := newConditions(co.Status, r.version, metav1.Now())
	conditions.setCondition(v1.OperatorProgressing, v1.ConditionTrue, "Rolling out the stack.", "RollOutInProgress", time)

	return r.setConditions(ctx, co, conditions)
}

// ReportState sets the Degraded and the Availability condition as reported by the
// StatesReport passed. If degraded or availability is nil, no changes are make
// to the existing condition
func (r *StatusReporter) ReportState(ctx context.Context, report StatesReport) error {
	co, err := r.getOrCreateClusterOperator(ctx)
	if err != nil {
		return err
	}

	time := metav1.Now()

	conditions := newConditions(co.Status, r.version, time)

	if degraded := report.Degraded(); degraded != nil {
		// The Reason should be upper case camelCase (PascalCase) according to the API docs.
		reason := cmostr.ToPascalCase(degraded.Reason())

		conditions.setCondition(v1.OperatorDegraded, v1.ConditionStatus(degraded.Status()),
			degraded.Message(), reason, time)
	}

	if available := report.Available(); available != nil {
		reason := cmostr.ToPascalCase(available.Reason())
		conditions.setCondition(v1.OperatorAvailable, v1.ConditionStatus(available.Status()),
			available.Message(), reason, time)
	}

	return r.setConditions(ctx, co, conditions)
}

func (r *StatusReporter) SetUpgradeable(ctx context.Context, cond v1.ConditionStatus, message, reason string) error {
	co, err := r.getOrCreateClusterOperator(ctx)
	if err != nil {
		return err
	}

	time := metav1.Now()
	conditions := newConditions(co.Status, r.version, metav1.Now())
	conditions.setCondition(v1.OperatorUpgradeable, cond, message, reason, time)

	return r.setConditions(ctx, co, conditions)
}
