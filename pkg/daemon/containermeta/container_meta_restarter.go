/*
Copyright 2021 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package containermeta

import (
	"context"
	"fmt"
	"net/http"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/klog/v2"
	kubeletcontainer "k8s.io/kubernetes/pkg/kubelet/container"

	daemonruntime "github.com/openkruise/kruise/pkg/daemon/criruntime"
	"github.com/openkruise/kruise/pkg/daemon/kuberuntime"
)

type restartController struct {
	queue          workqueue.RateLimitingInterface
	eventRecorder  record.EventRecorder
	runtimeFactory daemonruntime.Factory
}

func (c *restartController) Run(stop <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	for i := 0; i < restartWorkers; i++ {
		go wait.Until(func() {
			for c.processNextWorkItem() {
			}
		}, time.Second, stop)
	}
	<-stop
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *restartController) processNextWorkItem() bool {
	// pull the next work item from queue.  It should be a key we use to lookup
	// something in a cache
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.sync(key.(kubeletcontainer.ContainerID))

	if err == nil {
		// No error, tell the queue to stop tracking history
		c.queue.Forget(key)
	} else {
		// requeue the item to work on later
		c.queue.AddRateLimited(key)
	}

	return true
}

func (c *restartController) sync(containerID kubeletcontainer.ContainerID) error {
	criRuntime := c.runtimeFactory.GetRuntimeServiceByName(containerID.Type)
	if criRuntime == nil {
		klog.InfoS("Not found runtime service in daemon", "type", containerID.Type)
		return nil
	}

	containers, err := criRuntime.ListContainers(context.TODO(), &runtimeapi.ContainerFilter{Id: containerID.ID})
	if err != nil {
		klog.ErrorS(err, "Failed to list containers", "id", containerID.String())
		return err
	}
	if len(containers) == 0 || containers[0].State != runtimeapi.ContainerState_CONTAINER_RUNNING {
		klog.V(4).InfoS("Skip to kill container because of not found or non-running state.", "id", containerID.String())
		return nil
	}

	klog.V(3).InfoS("Preparing to stop container", "id", containerID.String())
	kubeRuntime := kuberuntime.NewGenericRuntime(containerID.Type, criRuntime, c.eventRecorder, &http.Client{})
	msg := fmt.Sprintf("Stopping containerID %s by container meta restarter", containerID.String())
	err = kubeRuntime.KillContainer(nil, containerID, "", msg, nil)
	if err != nil {
		return err
	}
	return nil
}
