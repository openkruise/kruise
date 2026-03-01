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

package containerrecreate

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"reflect"
	"sort"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	kubeletcontainer "k8s.io/kubernetes/pkg/kubelet/container"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/client"
	kruiseclient "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	listersalpha1 "github.com/openkruise/kruise/pkg/client/listers/apps/v1alpha1"
	daemonruntime "github.com/openkruise/kruise/pkg/daemon/criruntime"
	"github.com/openkruise/kruise/pkg/daemon/kuberuntime"
	daemonoptions "github.com/openkruise/kruise/pkg/daemon/options"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/expectations"
)

const (
	// TODO: make it a configurable flag
	workers = 32

	maxExpectationWaitDuration = 10 * time.Second
)

var (
	resourceVersionExpectation = expectations.NewResourceVersionExpectation()
)

type Controller struct {
	queue          workqueue.RateLimitingInterface
	runtimeClient  runtimeclient.Client
	crrInformer    cache.SharedIndexInformer
	crrLister      listersalpha1.ContainerRecreateRequestLister
	eventRecorder  record.EventRecorder
	runtimeFactory daemonruntime.Factory
}

// NewController returns the controller for CRR
func NewController(opts daemonoptions.Options) (*Controller, error) {
	genericClient := client.GetGenericClientWithName("kruise-daemon-crr")
	informer := newCRRInformer(genericClient.KruiseClient, opts.NodeName)

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: genericClient.KubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(opts.Scheme, v1.EventSource{Component: "kruise-daemon-crr", Host: opts.NodeName})

	queue := workqueue.NewNamedRateLimitingQueue(
		// Backoff duration from 500ms to 50~55s
		workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 50*time.Second+time.Millisecond*time.Duration(rand.Intn(5000))),
		"crr",
	)

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			crr, ok := obj.(*appsv1alpha1.ContainerRecreateRequest)
			if ok {
				enqueue(queue, crr)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			crr, ok := newObj.(*appsv1alpha1.ContainerRecreateRequest)
			if ok {
				enqueue(queue, crr)
			}
		},
		DeleteFunc: func(obj interface{}) {
			crr, ok := obj.(*appsv1alpha1.ContainerRecreateRequest)
			if ok {
				resourceVersionExpectation.Delete(crr)
			}
		},
	})

	opts.Healthz.RegisterFunc("crrInformerSynced", func(_ *http.Request) error {
		if !informer.HasSynced() {
			return fmt.Errorf("not synced")
		}
		return nil
	})

	return &Controller{
		queue:          queue,
		runtimeClient:  opts.RuntimeClient,
		crrInformer:    informer,
		crrLister:      listersalpha1.NewContainerRecreateRequestLister(informer.GetIndexer()),
		eventRecorder:  recorder,
		runtimeFactory: opts.RuntimeFactory,
	}, nil
}

func newCRRInformer(client kruiseclient.Interface, nodeName string) cache.SharedIndexInformer {
	tweakListOptionsFunc := func(opt *metav1.ListOptions) {
		opt.LabelSelector = fmt.Sprintf("%s=%s,%s=%s",
			appsv1alpha1.ContainerRecreateRequestNodeNameKey, nodeName,
			appsv1alpha1.ContainerRecreateRequestActiveKey, "true",
		)
	}

	im := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				tweakListOptionsFunc(&options)
				return client.AppsV1alpha1().ContainerRecreateRequests(v1.NamespaceAll).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				tweakListOptionsFunc(&options)
				return client.AppsV1alpha1().ContainerRecreateRequests(v1.NamespaceAll).Watch(context.TODO(), options)
			},
		},
		&appsv1alpha1.ContainerRecreateRequest{},
		0, // do not resync
		cache.Indexers{CRRPodNameIndex: SpecPodNameIndexFunc, cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	return im
}

func enqueue(queue workqueue.Interface, obj *appsv1alpha1.ContainerRecreateRequest) {
	if obj.DeletionTimestamp != nil || obj.Status.CompletionTime != nil {
		return
	}
	queue.Add(objectKey(obj))
}

func objectKey(obj *appsv1alpha1.ContainerRecreateRequest) string {
	return obj.Namespace + "/" + obj.Spec.PodName
}

func (c *Controller) Run(stop <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Info("Starting informer for ContainerRecreateRequest")
	go c.crrInformer.Run(stop)
	if !cache.WaitForCacheSync(stop, c.crrInformer.HasSynced) {
		return
	}

	klog.Info("Starting crr daemon controller")
	for i := 0; i < workers; i++ {
		go wait.Until(func() {
			for c.processNextWorkItem() {
			}
		}, time.Second, stop)
	}

	klog.Info("Started crr daemon controller successfully")
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
	namespace, podName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.InfoS("Invalid key", "key", key)
		return nil
	}

	objectList, err := c.crrInformer.GetIndexer().ByIndex(CRRPodNameIndex, podName)
	if err != nil {
		return err
	}

	crrList := make([]*appsv1alpha1.ContainerRecreateRequest, 0, len(objectList))
	for _, obj := range objectList {
		crr, ok := obj.(*appsv1alpha1.ContainerRecreateRequest)
		if ok && crr != nil && crr.Namespace == namespace {
			crrList = append(crrList, crr)
		}
	}
	if len(crrList) == 0 {
		return nil
	}

	crr, err := c.pickRecreateRequest(crrList)
	if err != nil || crr == nil {
		return err
	}

	klog.V(3).InfoS("Start syncing", "namespace", namespace, "name", crr.Name)
	defer func() {
		if retErr != nil {
			klog.ErrorS(retErr, "Failed to sync", "namespace", namespace, "name", crr.Name)
		} else {
			klog.V(3).InfoS("Finished syncing", "namespace", namespace, "name", crr.Name)
		}
	}()

	// once first update its phase to recreating
	if crr.Status.Phase != appsv1alpha1.ContainerRecreateRequestRecreating {
		return c.updateCRRPhase(crr, appsv1alpha1.ContainerRecreateRequestRecreating)
	}

	if crr.Spec.Strategy.UnreadyGracePeriodSeconds != nil {
		unreadyTimeStr := crr.Annotations[appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey]
		if unreadyTimeStr == "" {
			klog.InfoS("CRR is waiting for unready acquirement", "namespace", crr.Namespace, "name", crr.Name)
			return nil
		}

		unreadyTime, err := time.Parse(time.RFC3339, unreadyTimeStr)
		if err != nil {
			klog.ErrorS(err, "CRR failed to parse unready time", "namespace", crr.Namespace, "name", crr.Name, "unreadyTimeStr", unreadyTimeStr)
			return c.completeCRRStatus(crr, fmt.Sprintf("failed to parse unready time %s: %v", unreadyTimeStr, err))
		}

		leftTime := time.Duration(*crr.Spec.Strategy.UnreadyGracePeriodSeconds)*time.Second - time.Since(unreadyTime)
		if leftTime > 0 {
			klog.InfoS("CRR is waiting for unready grace period", "namespace", crr.Namespace, "name", crr.Name, "leftTime", leftTime)
			c.queue.AddAfter(crr.Namespace+"/"+crr.Spec.PodName, leftTime+100*time.Millisecond)
			return nil
		}
	}

	return c.manage(crr)
}

func (c *Controller) pickRecreateRequest(crrList []*appsv1alpha1.ContainerRecreateRequest) (*appsv1alpha1.ContainerRecreateRequest, error) {
	sort.Sort(crrListByPhaseAndCreated(crrList))
	var picked *appsv1alpha1.ContainerRecreateRequest
	for _, crr := range crrList {
		if crr.DeletionTimestamp != nil || crr.Status.CompletionTime != nil {
			resourceVersionExpectation.Delete(crr)
			continue
		}

		resourceVersionExpectation.Observe(crr)
		if satisfied, duration := resourceVersionExpectation.IsSatisfied(crr); !satisfied {
			if duration < maxExpectationWaitDuration {
				break
			}
			klog.InfoS("Wait for CRR resourceVersion expectation", "namespace", crr.Namespace, "name", crr.Name, "duration", duration)
			resourceVersionExpectation.Delete(crr)
		}

		// Only update non-picked CRR to Pending, for the picked one will directly update to Recreating
		if picked == nil {
			picked = crr
		} else if crr.Status.Phase == "" {
			if err := c.updateCRRPhase(crr, appsv1alpha1.ContainerRecreateRequestPending); err != nil {
				klog.ErrorS(err, "Failed to update CRR status to Pending", "namespace", crr.Namespace, "name", crr.Name)
				return nil, err
			}
		}
	}
	return picked, nil
}

func (c *Controller) manage(crr *appsv1alpha1.ContainerRecreateRequest) error {
	runtimeManager, err := c.newRuntimeManager(c.runtimeFactory, crr)
	if err != nil {
		klog.ErrorS(err, "Failed to find runtime service", "namespace", crr.Namespace, "name", crr.Name)
		return c.completeCRRStatus(crr, fmt.Sprintf("failed to find runtime service: %v", err))
	}

	pod := convertCRRToPod(crr)

	podStatus, err := runtimeManager.GetPodStatus(context.TODO(), pod.UID, pod.Name, pod.Namespace)
	if err != nil {
		return fmt.Errorf("failed to GetPodStatus %s/%s with uid %s: %v", pod.Namespace, pod.Name, pod.UID, err)
	}
	klog.V(5).InfoS("CRR for Pod GetPodStatus", "namespace", crr.Namespace, "name", crr.Name, "podName", pod.Name, "podStatus", util.DumpJSON(podStatus))

	newCRRContainerRecreateStates := getCurrentCRRContainersRecreateStates(crr, podStatus)
	if !reflect.DeepEqual(crr.Status.ContainerRecreateStates, newCRRContainerRecreateStates) {
		return c.patchCRRContainerRecreateStates(crr, newCRRContainerRecreateStates)
	}

	var completedCount int
	for i := range newCRRContainerRecreateStates {
		state := &newCRRContainerRecreateStates[i]
		switch state.Phase {
		case appsv1alpha1.ContainerRecreateRequestSucceeded:
			completedCount++
			continue
		case appsv1alpha1.ContainerRecreateRequestFailed:
			completedCount++
			if crr.Spec.Strategy.FailurePolicy == appsv1alpha1.ContainerRecreateRequestFailurePolicyIgnore {
				continue
			}
			return c.completeCRRStatus(crr, "")
		case appsv1alpha1.ContainerRecreateRequestPending, appsv1alpha1.ContainerRecreateRequestRecreating:
		}

		if state.Phase == appsv1alpha1.ContainerRecreateRequestRecreating {
			state.IsKilled = true
			if crr.Spec.Strategy.OrderedRecreate {
				break
			}
			continue
		}

		kubeContainerStatus := podStatus.FindContainerStatusByName(state.Name)
		if kubeContainerStatus == nil {
			break
		}

		msg := fmt.Sprintf("Stopping container %s by ContainerRecreateRequest %s", state.Name, crr.Name)
		err := runtimeManager.KillContainer(pod, kubeContainerStatus.ID, state.Name, msg, nil)
		if err != nil {
			klog.ErrorS(err, "Failed to kill container in Pod for CRR", "containerName", state.Name, "podNamespace", pod.Namespace, "podName", pod.Name, "crrNamespace", crr.Namespace, "crrName", crr.Name)
			state.Phase = appsv1alpha1.ContainerRecreateRequestFailed
			state.Message = fmt.Sprintf("kill container error: %v", err)
			if crr.Spec.Strategy.FailurePolicy == appsv1alpha1.ContainerRecreateRequestFailurePolicyIgnore {
				continue
			}
			return c.patchCRRContainerRecreateStates(crr, newCRRContainerRecreateStates)
		}
		state.IsKilled = true
		state.Phase = appsv1alpha1.ContainerRecreateRequestRecreating
		break
	}

	if !reflect.DeepEqual(crr.Status.ContainerRecreateStates, newCRRContainerRecreateStates) {
		return c.patchCRRContainerRecreateStates(crr, newCRRContainerRecreateStates)
	}

	// check if all containers have completed
	if completedCount == len(newCRRContainerRecreateStates) {
		return c.completeCRRStatus(crr, "")
	}

	if crr.Spec.Strategy != nil && crr.Spec.Strategy.MinStartedSeconds > 0 {
		c.queue.AddAfter(objectKey(crr), time.Duration(crr.Spec.Strategy.MinStartedSeconds)*time.Second)
	}
	return nil
}

func (c *Controller) patchCRRContainerRecreateStates(crr *appsv1alpha1.ContainerRecreateRequest, newCRRContainerRecreateStates []appsv1alpha1.ContainerRecreateRequestContainerRecreateState) error {
	klog.V(3).InfoS("CRR patch containerRecreateStates", "namespace", crr.Namespace, "name", crr.Name, "states", util.DumpJSON(newCRRContainerRecreateStates))
	crr = crr.DeepCopy()
	body := fmt.Sprintf(`{"status":{"containerRecreateStates":%s}}`, util.DumpJSON(newCRRContainerRecreateStates))
	oldRev := crr.ResourceVersion
	defer func() {
		if crr.ResourceVersion != oldRev {
			resourceVersionExpectation.Expect(crr)
		}
	}()
	return c.runtimeClient.Status().Patch(context.TODO(), crr, runtimeclient.RawPatch(types.MergePatchType, []byte(body)))
}

func (c *Controller) updateCRRPhase(crr *appsv1alpha1.ContainerRecreateRequest, phase appsv1alpha1.ContainerRecreateRequestPhase) error {
	crr = crr.DeepCopy()
	crr.Status.Phase = phase
	oldRev := crr.ResourceVersion
	defer func() {
		if crr.ResourceVersion != oldRev {
			resourceVersionExpectation.Expect(crr)
		}
	}()
	return c.runtimeClient.Status().Update(context.TODO(), crr)
}

func (c *Controller) completeCRRStatus(crr *appsv1alpha1.ContainerRecreateRequest, msg string) error {
	crr = crr.DeepCopy()
	now := metav1.Now()
	crr.Status.Phase = appsv1alpha1.ContainerRecreateRequestCompleted
	crr.Status.CompletionTime = &now
	crr.Status.Message = msg
	oldRev := crr.ResourceVersion
	defer func() {
		if crr.ResourceVersion != oldRev {
			resourceVersionExpectation.Expect(crr)
		}
	}()
	return c.runtimeClient.Status().Update(context.TODO(), crr)
}

func (c *Controller) newRuntimeManager(runtimeFactory daemonruntime.Factory, crr *appsv1alpha1.ContainerRecreateRequest) (kuberuntime.Runtime, error) {
	var runtimeName string
	for i := range crr.Spec.Containers {
		c := &crr.Spec.Containers[i]
		if c.StatusContext == nil || c.StatusContext.ContainerID == "" {
			return nil, fmt.Errorf("no statusContext or empty containerID in %s container", c.Name)
		}

		containerID := kubeletcontainer.ContainerID{}
		if err := containerID.ParseString(c.StatusContext.ContainerID); err != nil {
			return nil, fmt.Errorf("failed to parse containerID %s in %s container: %v", c.StatusContext.ContainerID, c.Name, err)
		}
		if runtimeName == "" {
			runtimeName = containerID.Type
		} else if runtimeName != containerID.Type {
			return nil, fmt.Errorf("has different runtime types %s, %s in Pod", runtimeName, containerID.Type)
		}
	}
	if runtimeName == "" {
		return nil, fmt.Errorf("no runtime type found in Pod")
	}
	runtimeService := runtimeFactory.GetRuntimeServiceByName(runtimeName)
	if runtimeService == nil {
		return nil, fmt.Errorf("not found runtime service for %s in daemon", runtimeName)
	}

	return kuberuntime.NewGenericRuntime(runtimeName, runtimeService, c.eventRecorder, &http.Client{}), nil
}
