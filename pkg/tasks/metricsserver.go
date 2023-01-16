package tasks

import (
	"context"

	"github.com/openshift/cluster-monitoring-operator/pkg/client"
	"github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MetricsServerTask struct {
	client    *client.Client
	ctx       context.Context
	factory   *manifests.Factory
	config    *manifests.Config
	namespace string
}

func NewMetricsServerTask(ctx context.Context, namespace string, client *client.Client, factory *manifests.Factory, config *manifests.Config) *MetricsServerTask {
	return &MetricsServerTask{
		client:    client,
		factory:   factory,
		config:    config,
		namespace: namespace,
		ctx:       ctx,
	}
}

func (t *MetricsServerTask) Run(ctx context.Context) error {
	{
		sa, err := t.factory.MetricsServerServiceAccount()
		if err != nil {
			return errors.Wrap(err, "initializing MetricsServer ServiceAccount failed")
		}

		err = t.client.CreateOrUpdateServiceAccount(ctx, sa)
		if err != nil {
			return errors.Wrap(err, "reconciling MetricsServer ServiceAccount failed")
		}
	}
	{
		crb, err := t.factory.MetricsServerAuthDelegator()
		if err != nil {
			return errors.Wrap(err, "initializing metrics-server ClusterRoleBinding failed")
		}

		err = t.client.CreateOrUpdateClusterRoleBinding(ctx, crb)
		if err != nil {
			return errors.Wrap(err, "reconciling metrics-server ClusterRoleBinding failed")
		}
	}
	{
		rb, err := t.factory.MetricsServerAuthReader()
		if err != nil {
			return errors.Wrap(err, "initializing metrics-server RoleBinding failed")
		}

		err = t.client.CreateOrUpdateRoleBinding(ctx, rb)
		if err != nil {
			return errors.Wrap(err, "reconciling metrics-server RoleBinding failed")
		}
	}
	{
		api, err := t.factory.MetricsServerAPIService()
		if err != nil {
			return errors.Wrap(err, "initializing MetricsServer APIService failed")
		}

		err = t.client.CreateOrUpdateAPIService(ctx, api)
		if err != nil {
			return errors.Wrap(err, "reconciling MetricsServer APIService failed")
		}
	}
	{
		cr, err := t.factory.MetricsServerClusterRole()
		if err != nil {
			return errors.Wrap(err, "initializing metrics-server ClusterRolefailed")
		}

		err = t.client.CreateOrUpdateClusterRole(ctx, cr)
		if err != nil {
			return errors.Wrap(err, "reconciling metrics-server ClusterRole failed")
		}
	}
	{
		crb, err := t.factory.MetricsServerClusterRoleBinding()
		if err != nil {
			return errors.Wrap(err, "initializing MetricsServer ClusterRoleBinding failed")
		}

		err = t.client.CreateOrUpdateClusterRoleBinding(ctx, crb)
		if err != nil {
			return errors.Wrap(err, "reconciling MetricsServer ClusterRoleBinding failed")
		}
	}
	{
		dep, err := t.factory.MetricsServerDeployment()
		if err != nil {
			return errors.Wrap(err, "initializing MetricsServer Deployment failed")
		}

		err = t.client.CreateOrUpdateDeployment(ctx, dep)
		if err != nil {
			return errors.Wrap(err, "reconciling MetricsServer Deployment failed")
		}
	}
	{
		s, err := t.factory.MetricsServerService()
		if err != nil {
			return errors.Wrap(err, "initializing MetricsServer Service failed")
		}

		err = t.client.CreateOrUpdateService(ctx, s)
		if err != nil {
			return errors.Wrap(err, "reconciling MetricsServer Service failed")
		}
	}
	return t.removePrometheusAdapterResources()
}

func (t *MetricsServerTask) removePrometheusAdapterResources() error {
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus-adapter",
			Namespace: "openshift-monitoring",
		},
	}
	err := t.client.DeleteDeployment(t.ctx, d)
	if err != nil {
		return errors.Wrap(err, "deleting PrometheusAdapter Deployment failed")
	}
	{
		api, err := t.factory.PrometheusAdapterAPIService()
		if err != nil {
			return errors.Wrap(err, "initializing PrometheusAdapter APIService failed")
		}

		err = t.client.DeleteAPIService(t.ctx, api)
		if err != nil {
			return errors.Wrap(err, "reconciling PrometheusAdapter APIService failed")
		}
	}
	return nil
}
