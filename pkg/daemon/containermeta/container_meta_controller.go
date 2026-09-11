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
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	criapi "k8s.io/cri-api/pkg/apis"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	kubeletcontainer "k8s.io/kubernetes/pkg/kubelet/container"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	daemonruntime "github.com/openkruise/kruise/pkg/daemon/criruntime"
	"github.com/openkruise/kruise/pkg/daemon/kuberuntime"
	daemonoptions "github.com/openkruise/kruise/pkg/daemon/options"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilcontainermeta "github.com/openkruise/kruise/pkg/util/containermeta"
	"github.com/openkruise/kruise/pkg/util/expectations"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

var (
	// TODO: make it a configurable flag
	workers        = 5
	restartWorkers = 10

	resourceVersionExpectation = expectations.NewResourceVersionExpectation()
	maxExpectationWaitDuration = 30 * time.Second
)

type Controller struct {
	queue          workqueue.RateLimitingInterface
	runtimeClient  runtimeclient.Client
	podLister      corelisters.PodLister
	runtimeFactory daemonruntime.Factory

	restarter *restartController
}

// NewController returns the Controller for containermeta reporting
func NewController(opts daemonoptions.Options) (*Controller, error) {
	if opts.PodInformer == nil {
		return nil, fmt.Errorf("containermeta Controller can not run without pod informer")
	}

	queue := workqueue.NewNamedRateLimitingQueue(
		// Backoff duration from 500ms to 50~55s
		workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 50*time.Second+time.Millisecond*time.Duration(rand.Intn(5000))),
		"runtime_container_meta",
	)

	opts.PodInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod, ok := obj.(*v1.Pod)
			if ok {
				if containerMetaSet, _ := appspub.GetRuntimeContainerMetaSet(pod); containerMetaSet == nil {
					// For existing pods without meta, don't immediately report meta annotations to avoid apiserver pressure
					return
				}
				if eventFilter(nil, pod) {
					enqueue(queue, pod)
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldPod := oldObj.(*v1.Pod)
			newPod := newObj.(*v1.Pod)
			if eventFilter(oldPod, newPod) {
				enqueue(queue, newPod)
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod, ok := obj.(*v1.Pod)
			if ok {
				resourceVersionExpectation.Delete(pod)
			}
		},
	})

	genericClient := client.GetGenericClientWithName("kruise-daemon-containermeta")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: genericClient.KubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(opts.Scheme, v1.EventSource{Component: "kruise-daemon-containermeta", Host: opts.NodeName})

	return &Controller{
		queue:          queue,
		runtimeClient:  opts.RuntimeClient,
		podLister:      corelisters.NewPodLister(opts.PodInformer.GetIndexer()),
		runtimeFactory: opts.RuntimeFactory,
		restarter: &restartController{
			queue: workqueue.NewNamedRateLimitingQueue(
				workqueue.NewItemExponentialFailureRateLimiter(3*time.Second, 10*time.Minute),
				"runtime_container_meta_restarter",
			),
			eventRecorder:  recorder,
			runtimeFactory: opts.RuntimeFactory,
		},
	}, nil
}

func eventFilter(oldPod, newPod *v1.Pod) bool {
	// Is pod owned or injected by Kruise workloads?
	var ownedByKruise bool
	if owner := metav1.GetControllerOf(newPod); owner != nil {
		if gv, err := schema.ParseGroupVersion(owner.APIVersion); err == nil {
			ownedByKruise = gv.Group == appsv1alpha1.GroupVersion.Group
		}
	}
	_, injectedBySidecarSet := newPod.Annotations[sidecarcontrol.SidecarSetHashAnnotation]
	if !ownedByKruise && !injectedBySidecarSet {
		return false
	}

	// Is containerID changed?
	if oldPod != nil && len(oldPod.Spec.Containers) != len(newPod.Spec.Containers) {
		return true
	}
	containerMetaSet, _ := appspub.GetRuntimeContainerMetaSet(newPod)

	// Look up the reported meta by container name rather than by index, because the meta set
	// contains the restartable init containers (native sidecar containers) as well and therefore
	// no longer aligns with status.containerStatuses positionally.
	var metaByName map[string]*appspub.RuntimeContainerMeta
	if containerMetaSet != nil {
		metaByName = make(map[string]*appspub.RuntimeContainerMeta, len(containerMetaSet.Containers))
		for i := range containerMetaSet.Containers {
			metaByName[containerMetaSet.Containers[i].Name] = &containerMetaSet.Containers[i]
		}
	}

	// expectedCount counts the containers that should be reported, so that a stale meta set which
	// still contains a removed container can be detected.
	var expectedCount int
	changed := func(statuses []v1.ContainerStatus, onlyRestartableInit bool) bool {
		for i := range statuses {
			cs := &statuses[i]
			containerSpec := util.GetContainer(cs.Name, newPod)
			if containerSpec == nil {
				continue
			}
			if onlyRestartableInit && !util.IsRestartableInitContainer(containerSpec) {
				continue
			}
			expectedCount++
			if cs.ContainerID == "" {
				continue
			}
			if containerMetaSet == nil {
				return true
			}
			meta, ok := metaByName[cs.Name]
			if !ok || meta.ContainerID != cs.ContainerID {
				return true
			}
			if utilfeature.DefaultFeatureGate.Enabled(features.InPlaceUpdateEnvFromMetadata) {
				hasher := utilcontainermeta.NewEnvFromMetadataHasher()
				newHash := hasher.GetExpectHash(containerSpec, newPod)
				if newHash != meta.Hashes.ExtractedEnvFromMetadataHash {
					return true
				}
				if oldPod != nil {
					if oldSpec := util.GetContainer(cs.Name, oldPod); oldSpec != nil &&
						newHash != hasher.GetExpectHash(oldSpec, oldPod) {
						return true
					}
				}
			}
		}
		return false
	}

	if changed(newPod.Status.InitContainerStatuses, true) {
		return true
	}
	if changed(newPod.Status.ContainerStatuses, false) {
		return true
	}
	if containerMetaSet != nil && expectedCount > 0 && len(containerMetaSet.Containers) != expectedCount {
		return true
	}
	return false
}

func enqueue(q workqueue.DelayingInterface, pod *v1.Pod) {
	enqueueAfter(q, pod, time.Second)
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

	klog.Info("Starting containermeta Controller")
	go c.restarter.Run(stop)
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
		klog.InfoS("Invalid key", "key", key)
		return nil
	}

	pod, err := c.podLister.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		klog.ErrorS(err, "Failed to get Pod from lister", "namespace", namespace, "name", name)
		return err
	} else if pod.DeletionTimestamp != nil || len(pod.Status.ContainerStatuses) == 0 {
		return nil
	}
	resourceVersionExpectation.Observe(pod)
	if satisfied, duration := resourceVersionExpectation.IsSatisfied(pod); !satisfied {
		if duration < maxExpectationWaitDuration {
			return nil
		}
		klog.InfoS("Waiting Pod resourceVersion expectation time out", "namespace", namespace, "name", name, "duration", duration)
		resourceVersionExpectation.Delete(pod)
	}

	criRuntime, kubeRuntime, err := c.getRuntimeForPod(pod)
	if err != nil {
		klog.ErrorS(err, "Failed to get runtime for Pod", "namespace", namespace, "name", name)
		return nil
	} else if criRuntime == nil {
		return nil
	}

	klog.V(3).InfoS("Start syncing", "namespace", namespace, "name", name)
	defer func() {
		if retErr != nil {
			klog.ErrorS(retErr, "Failed to sync", "namespace", namespace, "name", name)
		} else {
			klog.V(3).InfoS("Finished syncing", "namespace", namespace, "name", name)
		}
	}()

	kubePodStatus, err := kubeRuntime.GetPodStatus(context.TODO(), pod.UID, name, namespace)
	if err != nil {
		return fmt.Errorf("failed to GetPodStatus: %v", err)
	}

	oldMetaSet, err := appspub.GetRuntimeContainerMetaSet(pod)
	if err != nil {
		klog.ErrorS(err, "Failed to get old runtime meta from Pod", "namespace", namespace, "name", name)
	}
	newMetaSet := c.manageContainerMetaSet(pod, kubePodStatus, oldMetaSet, criRuntime)

	return c.reportContainerMetaSet(pod, oldMetaSet, newMetaSet)
}

func (c *Controller) reportContainerMetaSet(pod *v1.Pod, oldMetaSet, newMetaSet *appspub.RuntimeContainerMetaSet) error {
	if reflect.DeepEqual(oldMetaSet, newMetaSet) {
		return nil
	}

	newPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: pod.Namespace, Name: pod.Name},
	}
	containerMetaSetStr := util.DumpJSON(newMetaSet)
	klog.InfoS("Reporting container meta changed in Pod", "namespace", pod.Namespace, "name", pod.Name, "containerMetaSetStr", containerMetaSetStr)
	mergePatch, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]string{
				appspub.RuntimeContainerMetaKey: containerMetaSetStr,
			},
		},
	})
	err := c.runtimeClient.Status().Patch(context.TODO(), newPod, runtimeclient.RawPatch(types.StrategicMergePatchType, mergePatch))
	if err != nil {
		return fmt.Errorf("failed to patch pod: %v", err)
	}
	resourceVersionExpectation.Expect(newPod)
	return nil
}

func (c *Controller) manageContainerMetaSet(pod *v1.Pod, kubePodStatus *kubeletcontainer.PodStatus, oldMetaSet *appspub.RuntimeContainerMetaSet, criRuntime criapi.RuntimeService) *appspub.RuntimeContainerMetaSet {
	metaSet := appspub.RuntimeContainerMetaSet{
		Containers: make([]appspub.RuntimeContainerMeta, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses)),
	}

	// Report the restartable init containers (native sidecar containers) as well, so that the
	// workload controllers can verify their in-place update with the container hash, instead of
	// falling back to the weaker imageID comparison.
	//
	// Regular init containers are skipped on purpose: they exit after completion and kubelet
	// never restarts them when their spec changes, so their runtime meta would never become
	// consistent and their containerID may already have been garbage collected.
	//
	// Note that the order here must be stable, for the meta set is compared with the previous one
	// to decide whether the Pod annotation has to be patched.
	for i := range pod.Status.InitContainerStatuses {
		containerSpec := util.GetContainer(pod.Status.InitContainerStatuses[i].Name, pod)
		if containerSpec == nil || !util.IsRestartableInitContainer(containerSpec) {
			continue
		}
		if meta := c.buildContainerMeta(pod, containerSpec, kubePodStatus, oldMetaSet, criRuntime); meta != nil {
			metaSet.Containers = append(metaSet.Containers, *meta)
		}
	}

	for i := range pod.Status.ContainerStatuses {
		containerSpec := util.GetContainer(pod.Status.ContainerStatuses[i].Name, pod)
		if containerSpec == nil {
			continue
		}
		if meta := c.buildContainerMeta(pod, containerSpec, kubePodStatus, oldMetaSet, criRuntime); meta != nil {
			metaSet.Containers = append(metaSet.Containers, *meta)
		}
	}

	return &metaSet
}

// buildContainerMeta builds the runtime meta for a single container. It returns nil if the
// container has not been created by the runtime yet.
func (c *Controller) buildContainerMeta(pod *v1.Pod, containerSpec *v1.Container, kubePodStatus *kubeletcontainer.PodStatus, oldMetaSet *appspub.RuntimeContainerMetaSet, criRuntime criapi.RuntimeService) *appspub.RuntimeContainerMeta {
	var err error
	status := kubePodStatus.FindContainerStatusByName(containerSpec.Name)
	if status == nil {
		return nil
	}

	var containerMeta *appspub.RuntimeContainerMeta
	if oldMetaSet != nil {
		for i := range oldMetaSet.Containers {
			if oldMetaSet.Containers[i].ContainerID == status.ID.String() {
				containerMeta = oldMetaSet.Containers[i].DeepCopy()
			}
		}
	}
	if containerMeta == nil {
		containerMeta = &appspub.RuntimeContainerMeta{
			Name:         status.Name,
			ContainerID:  status.ID.String(),
			RestartCount: int32(status.RestartCount),
			Hashes: appspub.RuntimeContainerHashes{
				PlainHash: status.Hash,
			},
		}
	}
	if utilfeature.DefaultFeatureGate.Enabled(features.InPlaceUpdateEnvFromMetadata) {
		envHasher := utilcontainermeta.NewEnvFromMetadataHasher()

		if containerMeta.Hashes.ExtractedEnvFromMetadataHash == 0 && status.State == kubeletcontainer.ContainerStateRunning {
			envGetter := wrapEnvGetter(criRuntime, status.ID.ID, fmt.Sprintf("container %s (%s) in Pod %s/%s", containerSpec.Name, status.ID.String(), pod.Namespace, pod.Name))
			containerMeta.Hashes.ExtractedEnvFromMetadataHash, err = envHasher.GetCurrentHash(containerSpec, envGetter)
			if err != nil {
				klog.ErrorS(err, "Failed to hash container with env for Pod", "containerName", containerSpec.Name, "containerID", status.ID.String(), "namespace", pod.Namespace, "podName", pod.Name)
				enqueueAfter(c.queue, pod, time.Second*3)
			} else {
				klog.V(4).InfoS("Extracted env from metadata for container",
					"containerName", containerSpec.Name, "containerID", status.ID.String(), "namespace", pod.Namespace, "podName", pod.Name, "hash", containerMeta.Hashes.ExtractedEnvFromMetadataHash)
			}
		}

		// Trigger restarting only if it is in-place updating
		_, condition := podutil.GetPodCondition(&pod.Status, appspub.InPlaceUpdateReady)
		if condition != nil && condition.Status == v1.ConditionFalse {
			// Trigger restarting when expected env hash is not equal to current hash
			if containerMeta.Hashes.ExtractedEnvFromMetadataHash > 0 && containerMeta.Hashes.ExtractedEnvFromMetadataHash != envHasher.GetExpectHash(containerSpec, pod) {
				// Maybe checking PlainHash inconsistent here can skip to trigger restart. But it is not a good idea for some special scenarios.
				klog.V(2).InfoS("Triggering container in Pod to restart, for it has inconsistent hash of env from metadata", "containerName", containerSpec.Name, "containerID", status.ID.String(), "namespace", pod.Namespace, "podName", pod.Name)
				c.restarter.queue.AddRateLimited(status.ID)
			}
		}
	}

	return containerMeta
}

func wrapEnvGetter(criRuntime criapi.RuntimeService, containerID, logID string) func(string) (string, error) {
	var once sync.Once
	envMap := make(map[string]string)
	var getErr error
	return func(key string) (string, error) {
		once.Do(func() {
			var stdout []byte
			stdout, _, getErr = criRuntime.ExecSync(context.TODO(), containerID, []string{"env"}, time.Second*3)
			if getErr != nil {
				return
			}
			lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
			for _, l := range lines {
				words := strings.SplitN(strings.TrimSpace(l), "=", 2)
				if len(words) == 2 {
					envMap[words[0]] = words[1]
				}
			}
		})
		if getErr != nil {
			return "", getErr
		}
		klog.V(4).InfoS("Got env", "key", key, "value", envMap[key], "logID", logID)
		return envMap[key], nil
	}
}

func (c *Controller) getRuntimeForPod(pod *v1.Pod) (criapi.RuntimeService, kuberuntime.Runtime, error) {
	if len(pod.Status.ContainerStatuses) == 0 {
		return nil, nil, fmt.Errorf("empty containerStatuses in pod status")
	}

	var existingID string
	for _, cs := range pod.Status.ContainerStatuses {
		if len(cs.ContainerID) > 0 {
			existingID = cs.ContainerID
			break
		}
	}
	if existingID == "" {
		return nil, nil, nil
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
