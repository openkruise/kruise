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
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"reflect"
	"time"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	daemonruntime "github.com/openkruise/kruise/pkg/daemon/criruntime"
	"github.com/openkruise/kruise/pkg/daemon/kuberuntime"
	daemonoptions "github.com/openkruise/kruise/pkg/daemon/options"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/expectations"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	criapi "k8s.io/cri-api/pkg/apis"
	"k8s.io/klog/v2"
	kubeletcontainer "k8s.io/kubernetes/pkg/kubelet/container"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// TODO: make it a configurable flag
	workers = 5

	resourceVersionExpectation = expectations.NewResourceVersionExpectation()
	maxExpectationWaitDuration = 30 * time.Second
)

type Controller struct {
	queue          workqueue.RateLimitingInterface
	runtimeClient  runtimeclient.Client
	podLister      corelisters.PodLister
	runtimeFactory daemonruntime.Factory
}

// NewController returns the Controller for containermeta reporting
func NewController(opts daemonoptions.Options) (*Controller, error) {
	if opts.PodInformer == nil {
		return nil, fmt.Errorf("containermeta Controller can not run without pod informer")
	}

	queue := workqueue.NewNamedRateLimitingQueue(
		// Backoff duration from 500ms to 50~55s
		workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 50*time.Second+time.Millisecond*time.Duration(rand.Intn(5000))),
		"runtimecontainermeta",
	)

	opts.PodInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod, ok := obj.(*v1.Pod)
			if ok {
				if containerMetaSet, _ := appspub.GetRuntimeContainerMetaSet(pod); containerMetaSet == nil {
					// For existing pods without meta, don't immediately report meta annotations to avoid apiserver pressure
					return
				}
				if shouldReportMeta(pod) {
					enqueue(queue, pod)
				}
			}
		},
		UpdateFunc: func(_, newObj interface{}) {
			pod, ok := newObj.(*v1.Pod)
			if ok && shouldReportMeta(pod) {
				enqueue(queue, pod)
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod, ok := obj.(*v1.Pod)
			if ok {
				resourceVersionExpectation.Delete(pod)
			}
		},
	})

	return &Controller{
		queue:          queue,
		runtimeClient:  opts.RuntimeClient,
		podLister:      corelisters.NewPodLister(opts.PodInformer.GetIndexer()),
		runtimeFactory: opts.RuntimeFactory,
	}, nil
}

func shouldReportMeta(pod *v1.Pod) bool {
	// Is pod owned or injected by Kruise workloads?
	var ownedByKruise bool
	if owner := metav1.GetControllerOf(pod); owner != nil {
		if gv, err := schema.ParseGroupVersion(owner.APIVersion); err == nil {
			ownedByKruise = gv.Group == appsv1alpha1.GroupVersion.Group
		}
	}
	_, injectedBySidecarSet := pod.Annotations[sidecarcontrol.SidecarSetHashAnnotation]
	if !ownedByKruise && !injectedBySidecarSet {
		return false
	}

	// Is containerID changed?
	containerMetaSet, _ := appspub.GetRuntimeContainerMetaSet(pod)
	if containerMetaSet == nil {
		return len(pod.Status.ContainerStatuses) != 0
	}
	if len(containerMetaSet.Containers) != len(pod.Status.ContainerStatuses) {
		return true
	}
	for i := range pod.Status.ContainerStatuses {
		if containerMetaSet.Containers[i].ContainerID != pod.Status.ContainerStatuses[i].ContainerID {
			return true
		}
	}
	return false
}

func enqueue(q workqueue.DelayingInterface, pod *v1.Pod) {
	enqueueAfter(q, pod, 0)
}

func enqueueAfter(q workqueue.DelayingInterface, pod *v1.Pod, duration time.Duration) {
	if pod.DeletionTimestamp == nil && len(pod.Status.ContainerStatuses) > 0 {
		key := pod.Namespace + "/" + pod.Name
		if duration == 0 {
			q.Add(key)
		} else {
			q.AddAfter(key, duration)
		}
	}
}

func (c *Controller) Run(stop <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Infof("Starting containermeta Controller")
	for i := 0; i < workers; i++ {
		go wait.Until(func() {
			for c.processNextWorkItem() {
			}
		}, time.Second, stop)
	}

	klog.Info("Started containermeta Controller successfully")
	<-stop
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	// pull the next work item from queue.  It should be a key we use to lookup
	// something in a cache
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.sync(key.(string))

	if err == nil {
		// No error, tell the queue to stop tracking history
		c.queue.Forget(key)
	} else {
		// requeue the item to work on later
		c.queue.AddRateLimited(key)
	}

	return true
}

func (c *Controller) sync(key string) (retErr error) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.Warningf("Invalid key: %s", key)
		return nil
	}

	pod, err := c.podLister.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		klog.Errorf("Failed to get Pod %s/%s from lister: %v", namespace, name, err)
		return err
	} else if pod.DeletionTimestamp != nil || len(pod.Status.ContainerStatuses) == 0 {
		return nil
	}
	resourceVersionExpectation.Observe(pod)
	if satisfied, duration := resourceVersionExpectation.IsSatisfied(pod); !satisfied {
		if duration < maxExpectationWaitDuration {
			return nil
		}
		klog.Warningf("Wait for Pod %s/%s resourceVersion expectation over %v", namespace, name, duration)
		resourceVersionExpectation.Delete(pod)
	}

	_, kubeRuntime, err := c.getRuntimeForPod(pod)
	if err != nil {
		klog.Errorf("Failed to get runtime for Pod %s/%s: %v", namespace, name, err)
		return nil
	}

	klog.V(3).Infof("Start syncing for %s/%s", namespace, name)
	defer func() {
		if retErr != nil {
			klog.Errorf("Failed to sync for %s/%s: %v", namespace, name, retErr)
		} else {
			klog.V(3).Infof("Finished syncing for %s/%s", namespace, name)
		}
	}()

	kubePodStatus, err := kubeRuntime.GetPodStatus(pod.UID, name, namespace)
	if err != nil {
		return fmt.Errorf("failed to GetPodStatus: %v", err)
	}

	containerMetaSet := c.generateContainerMetaSet(pod, kubePodStatus)
	oldContainerMetaSet, err := appspub.GetRuntimeContainerMetaSet(pod)
	if err != nil {
		klog.Warningf("Failed to get old runtime meta from Pod %s/%s: %v", namespace, name, err)
	} else if reflect.DeepEqual(containerMetaSet, oldContainerMetaSet) {
		return nil
	}

	newPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: pod.Namespace, Name: pod.Name},
	}
	containerMetaSetStr := util.DumpJSON(containerMetaSet)
	klog.Infof("Find container meta changed in Pod %s/%s, new: %v", namespace, name, containerMetaSetStr)
	mergePatch, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]string{
				appspub.RuntimeContainerMetaKey: containerMetaSetStr,
			},
		},
	})
	err = c.runtimeClient.Status().Patch(context.TODO(), newPod, runtimeclient.RawPatch(types.StrategicMergePatchType, mergePatch))
	if err != nil {
		return fmt.Errorf("failed to patch pod: %v", err)
	}
	resourceVersionExpectation.Expect(newPod)

	return nil
}

func (c *Controller) generateContainerMetaSet(pod *v1.Pod, kubePodStatus *kubeletcontainer.PodStatus) *appspub.RuntimeContainerMetaSet {
	s := appspub.RuntimeContainerMetaSet{Containers: make([]appspub.RuntimeContainerMeta, 0, len(pod.Status.ContainerStatuses))}
	for _, cs := range pod.Status.ContainerStatuses {
		status := kubePodStatus.FindContainerStatusByName(cs.Name)
		if status != nil {
			s.Containers = append(s.Containers, appspub.RuntimeContainerMeta{
				Name:         status.Name,
				ContainerID:  status.ID.String(),
				RestartCount: int32(status.RestartCount),
				Hashes:       appspub.RuntimeContainerHashes{PlainHash: status.Hash},
			})
		}
	}
	return &s
}

func (c *Controller) getRuntimeForPod(pod *v1.Pod) (criapi.RuntimeService, kuberuntime.Runtime, error) {
	if len(pod.Status.ContainerStatuses) == 0 {
		return nil, nil, fmt.Errorf("empty containerStatuses in pod status")
	}

	containerID := kubeletcontainer.ContainerID{}
	if err := containerID.ParseString(pod.Status.ContainerStatuses[0].ContainerID); err != nil {
		return nil, nil, fmt.Errorf("failed to parse containerID %s: %v", pod.Status.ContainerStatuses[0].ContainerID, err)
	} else if containerID.Type == "" {
		return nil, nil, fmt.Errorf("no runtime name in containerID %s", pod.Status.ContainerStatuses[0].ContainerID)
	}

	runtimeName := containerID.Type
	runtimeService := c.runtimeFactory.GetRuntimeServiceByName(runtimeName)
	if runtimeService == nil {
		return nil, nil, fmt.Errorf("not found runtime service for %s in daemon", runtimeName)
	}

	return runtimeService, kuberuntime.NewGenericRuntime(runtimeName, runtimeService, nil, &http.Client{}), nil
}
