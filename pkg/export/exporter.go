// Copyright 2024 The Cluster Monitoring Operator Authors
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

package export

import (
	"context"
	"fmt"
	"time"

	osmv1alpha1 "github.com/openshift/api/monitoring/v1alpha1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/openshift/cluster-monitoring-operator/pkg/client"
)

const (
	resyncPeriod   = 15 * time.Minute
	queueBaseDelay = 50 * time.Millisecond
	queueMaxDelay  = 3 * time.Minute
)

type DataExportController struct {
	odeConfigInformer cache.SharedIndexInformer
	client            *client.Client
	queue             workqueue.RateLimitingInterface
}

func NewDataExportController(ctx context.Context, client *client.Client) (*DataExportController, error) {
	lw := client.ObservabilityDataExportListWatch(ctx)

	odeConfigInformer := cache.NewSharedIndexInformer(
		lw,
		&osmv1alpha1.ObservabilityDataExport{},
		resyncPeriod,
		cache.Indexers{},
	)
	queue := workqueue.NewNamedRateLimitingQueue(
		workqueue.NewItemExponentialFailureRateLimiter(queueBaseDelay, queueMaxDelay),
		"data-export-configs",
	)

	controller := &DataExportController{
		odeConfigInformer: odeConfigInformer,
		client:            client,
		queue:             queue,
	}

	_, err := odeConfigInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.handleOdeConfigAdd,
		UpdateFunc: controller.handleOdeConfigUpdate,
		DeleteFunc: controller.handleOdeConfigDelete,
	})
	if err != nil {
		return nil, err
	}

	return controller, nil

}

// Run starts the controller, and blocks until the done channel for the given
// context is closed.
func (dec *DataExportController) Run(ctx context.Context, workers int) {
	klog.Info("Starting alerting rules controller")

	defer dec.queue.ShutDown()

	go dec.odeConfigInformer.Run(ctx.Done())

	cache.WaitForNamedCacheSync("DataExport controller", ctx.Done(),
		dec.odeConfigInformer.HasSynced,
	)

	go dec.worker(ctx)

	<-ctx.Done()
}

// worker starts processing of the controller's work queue.
func (dec *DataExportController) worker(ctx context.Context) {
	for dec.processNextWorkItem(ctx) {
	}
}

// processNextWorkItem processes the next item on the work queue.
func (dec *DataExportController) processNextWorkItem(ctx context.Context) bool {
	key, quit := dec.queue.Get()
	if quit {
		return false
	}

	defer dec.queue.Done(key)

	if err := dec.sync(ctx, key.(string)); err != nil {
		utilruntime.HandleError(fmt.Errorf("Error syncing DataExport (%s): %w", key.(string), err))

		// Re-queue failed sync.
		dec.queue.AddRateLimited(key)

		return true
	}

	klog.V(4).Infof("DataExport successfully synced: %s", key.(string))
	dec.queue.Forget(key) // Reset rate-limiting.

	return true
}

func (dec *DataExportController) sync(ctx context.Context, key string) error {
}
