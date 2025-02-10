/*
Copyright 2022 The Kruise Authors.

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

package podprobe

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"reflect"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/gengo/examples/set-gen/sets"
	"k8s.io/klog/v2"
	kubelettypes "k8s.io/kubelet/pkg/types"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/client"
	kruiseclient "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	clientalpha1 "github.com/openkruise/kruise/pkg/client/clientset/versioned/typed/apps/v1alpha1"
	listersalpha1 "github.com/openkruise/kruise/pkg/client/listers/apps/v1alpha1"
	daemonruntime "github.com/openkruise/kruise/pkg/daemon/criruntime"
	daemonoptions "github.com/openkruise/kruise/pkg/daemon/options"
	"github.com/openkruise/kruise/pkg/daemon/util"
	commonutil "github.com/openkruise/kruise/pkg/util"
)

const (
	EventKruiseProbeSucceeded = "KruiseProbeSucceeded"
	EventKruiseProbeFailed    = "KruiseProbeFailed"
)

// Key uniquely identifying container probes
type probeKey struct {
	podNs         string
	podName       string
	podUID        string
	podIP         string
	containerName string
	probeName     string
}

type Controller struct {
	queue                workqueue.RateLimitingInterface
	updateQueue          workqueue.RateLimitingInterface
	nodePodProbeInformer cache.SharedIndexInformer
	nodePodProbeLister   listersalpha1.NodePodProbeLister
	nodePodProbeClient   clientalpha1.NodePodProbeInterface
	// event
	eventRecorder record.EventRecorder
	// Map of active workers for probes
	workers map[probeKey]*worker
	// Lock for accessing & mutating workers
	workerLock     sync.RWMutex
	runtimeFactory daemonruntime.Factory
	// prober executes the probe actions.
	prober *prober
	// pod probe result manager
	result *resultManager
	// node name
	nodeName string
	// kruise daemon start time
	start time.Time
}

// NewController returns the controller for pod probe
func NewController(opts daemonoptions.Options) (*Controller, error) {
	// pull the next work item from queue.  It should be a key we use to lookup
	// something in a cache
	nodeName, err := util.NodeName()
	if err != nil {
		return nil, err
	}
	randInt, _ := rand.Int(rand.Reader, big.NewInt(5000))
	queue := workqueue.NewNamedRateLimitingQueue(
		// Backoff duration from 500ms to 50~55s
		workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 50*time.Second+time.Millisecond*time.Duration(randInt.Int64())),
		"sync_node_pod_probe",
	)
	updateQueue := workqueue.NewNamedRateLimitingQueue(
		// Backoff duration from 500ms to 50~55s
		workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 50*time.Second+time.Millisecond*time.Duration(randInt.Int64())),
		"update_node_pod_probe_status",
	)
	genericClient := client.GetGenericClientWithName("kruise-daemon-podprobe")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: genericClient.KubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(opts.Scheme, corev1.EventSource{Component: "kruise-daemon-podprobe", Host: opts.NodeName})
	informer := newNodePodProbeInformer(genericClient.KruiseClient, opts.NodeName)
	c := &Controller{
		nodePodProbeInformer: informer,
		nodePodProbeLister:   listersalpha1.NewNodePodProbeLister(informer.GetIndexer()),
		runtimeFactory:       opts.RuntimeFactory,
		workers:              make(map[probeKey]*worker),
		queue:                queue,
		updateQueue:          updateQueue,
		nodePodProbeClient:   genericClient.KruiseClient.AppsV1alpha1().NodePodProbes(),
		result:               newResultManager(updateQueue),
		nodeName:             nodeName,
		eventRecorder:        recorder,
		start:                time.Now(),
	}
	c.prober = newProber(c.runtimeFactory.GetRuntimeService())
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			npp, ok := obj.(*appsv1alpha1.NodePodProbe)
			if ok {
				enqueue(queue, npp)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldNodePodProbe, oldOK := oldObj.(*appsv1alpha1.NodePodProbe)
			newNodePodProbe, newOK := newObj.(*appsv1alpha1.NodePodProbe)
			if !oldOK || !newOK {
				return
			}
			if reflect.DeepEqual(oldNodePodProbe.Spec, newNodePodProbe.Spec) {
				return
			}
			enqueue(queue, newNodePodProbe)
		},
	})

	opts.Healthz.RegisterFunc("nodePodProbeInformerSynced", func(_ *http.Request) error {
		if !informer.HasSynced() {
			return fmt.Errorf("not synced")
		}
		return nil
	})
	return c, nil
}

func newNodePodProbeInformer(client kruiseclient.Interface, nodeName string) cache.SharedIndexInformer {
	tweakListOptionsFunc := func(opt *metav1.ListOptions) {
		opt.FieldSelector = "metadata.name=" + nodeName
	}

	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				tweakListOptionsFunc(&options)
				return client.AppsV1alpha1().NodePodProbes().List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				tweakListOptionsFunc(&options)
				return client.AppsV1alpha1().NodePodProbes().Watch(context.TODO(), options)
			},
		},
		&appsv1alpha1.NodePodProbe{},
		0, // do not resync
		cache.Indexers{},
	)
}

func enqueue(queue workqueue.Interface, obj *appsv1alpha1.NodePodProbe) {
	if obj.DeletionTimestamp != nil {
		return
	}
	key, _ := cache.MetaNamespaceKeyFunc(obj)
	queue.Add(key)
}

func (c *Controller) Run(stop <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Info("Starting informer for NodePodProbe")
	go c.nodePodProbeInformer.Run(stop)
	if !cache.WaitForCacheSync(stop, c.nodePodProbeInformer.HasSynced) {
		return
	}

	klog.Info("Starting NodePodProbe controller")
	// Launch a worker to process resources, for there is only one nodePodProbe per Node
	go wait.Until(func() {
		for c.processNextWorkItem() {
		}
	}, time.Second, stop)

	go wait.Until(func() {
		for c.processUpdateWorkItem() {
		}
	}, time.Second, stop)

	klog.Info("Started NodePodProbe controller successfully")
	<-stop
}

// run probe worker based on NodePodProbe.Spec configuration
func (c *Controller) processNextWorkItem() bool {
	// pull the next work item from queue.  It should be a key we use to lookup
	// something in a cache
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.sync()
	if err == nil {
		// No error, tell the queue to stop tracking history
		c.queue.Forget(key)
	} else {
		// requeue the item to work on later
		c.queue.AddRateLimited(key)
	}

	return true
}

func (c *Controller) sync() error {
	// indicates must be deep copy before update pod objection
	npp, err := c.nodePodProbeLister.Get(c.nodeName)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		klog.ErrorS(err, "Failed to get nodePodProbe", "nodeName", c.nodeName)
		return err
	}

	// run probe worker
	c.workerLock.Lock()
	validWorkers := map[probeKey]struct{}{}
	for _, podProbe := range npp.Spec.PodProbes {
		key := probeKey{podNs: podProbe.Namespace, podName: podProbe.Name, podUID: podProbe.UID, podIP: podProbe.IP}
		for i := range podProbe.Probes {
			probe := podProbe.Probes[i]
			key.containerName = probe.ContainerName
			key.probeName = probe.Name
			validWorkers[key] = struct{}{}
			if worker, ok := c.workers[key]; ok {
				if !reflect.DeepEqual(probe.Probe, worker.getProbeSpec()) {
					klog.InfoS("NodePodProbe pod container probe changed",
						"podUID", key.podUID, "containerName", key.containerName, "from", commonutil.DumpJSON(worker.getProbeSpec()), "to", commonutil.DumpJSON(probe.Probe))
					worker.updateProbeSpec(&probe.Probe)
				}
				continue
			}
			w := newWorker(c, key, &probe.Probe)
			c.workers[key] = w
			klog.InfoS("NodePodProbe run pod container probe spec worker", "podUID", key.podUID, "containerName", key.containerName, "probeName", key.probeName, "probeSpec", commonutil.DumpJSON(probe.Probe))
			go w.run()
		}
	}
	for key, worker := range c.workers {
		if _, ok := validWorkers[key]; !ok {
			klog.InfoS("NodePodProbe stop pod container probe worker", "namespace", key.podNs, "podName", key.podName, "containerName", key.containerName, "probeName", key.probeName)
			worker.stop()
		}
	}
	c.workerLock.Unlock()
	c.updateQueue.Add("updateStatus")
	return nil
}

// Record the execution result of probe worker to NodePodProbe Status
func (c *Controller) processUpdateWorkItem() bool {
	key, quit := c.updateQueue.Get()
	if quit {
		return false
	}
	defer c.updateQueue.Done(key)

	err := c.syncUpdateNodePodProbeStatus()
	if err == nil {
		// No error, tell the queue to stop tracking history
		c.queue.Forget(key)
	} else {
		// requeue the item to work on later
		c.queue.AddRateLimited(key)
	}

	return true
}

func (c *Controller) syncUpdateNodePodProbeStatus() error {
	// indicates must be deep copy before update pod objection
	npp, err := c.nodePodProbeLister.Get(c.nodeName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		klog.ErrorS(err, "Get NodePodProbe failed", "nodeName", c.nodeName)
		return err
	}
	validSets := sets.NewString()
	for _, podProbe := range npp.Spec.PodProbes {
		for _, probe := range podProbe.Probes {
			validSets.Insert(fmt.Sprintf("%s/%s", podProbe.UID, probe.Name))
		}
	}
	// If the PodProbe is deleted, the corresponding status will be clear
	newStatus := &appsv1alpha1.NodePodProbeStatus{}
	for _, probeStatus := range npp.Status.PodProbeStatuses {
		newProbeStatus := appsv1alpha1.PodProbeStatus{
			Namespace: probeStatus.Namespace,
			Name:      probeStatus.Name,
			UID:       probeStatus.UID,
		}
		for i := range probeStatus.ProbeStates {
			probeState := probeStatus.ProbeStates[i]
			if validSets.Has(fmt.Sprintf("%s/%s", probeStatus.UID, probeState.Name)) {
				newProbeStatus.ProbeStates = append(newProbeStatus.ProbeStates, probeState)
			}
		}
		if len(newProbeStatus.ProbeStates) > 0 {
			newStatus.PodProbeStatuses = append(newStatus.PodProbeStatuses, newProbeStatus)
		}
	}
	// update nodePodProbe.Status according to probe worker
	updates := c.result.listResults()
	for _, update := range updates {
		if !validSets.Has(fmt.Sprintf("%s/%s", update.Key.podUID, update.Key.probeName)) {
			continue
		}
		//record probe result in pod event
		ref := &corev1.ObjectReference{Kind: "Pod", Namespace: update.Key.podNs, Name: update.Key.podName, UID: types.UID(update.Key.podUID),
			APIVersion: corev1.SchemeGroupVersion.String()}
		if update.State == appsv1alpha1.ProbeSucceeded {
			c.eventRecorder.Event(ref, corev1.EventTypeNormal, EventKruiseProbeSucceeded, update.Msg)
		} else {
			c.eventRecorder.Event(ref, corev1.EventTypeNormal, EventKruiseProbeFailed, update.Msg)
		}
		// update probe result in status
		updateNodePodProbeStatus(update, newStatus)
	}
	if reflect.DeepEqual(npp.Status, newStatus) {
		return nil
	}
	nppClone := npp.DeepCopy()
	nppClone.Status = *newStatus
	_, err = c.nodePodProbeClient.UpdateStatus(context.TODO(), nppClone, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "NodePodProbe update status failed", "nodeName", c.nodeName)
		return err
	}
	klog.InfoS("NodePodProbe(%s) update status success", "nodeName", c.nodeName, "from", commonutil.DumpJSON(npp.Status), "to", commonutil.DumpJSON(nppClone.Status))
	return nil
}

// Called by the worker after exiting.
func (c *Controller) removeWorker(key probeKey) {
	c.workerLock.Lock()
	defer c.workerLock.Unlock()
	delete(c.workers, key)
}

func (c *Controller) fetchLatestPodContainer(podUID, name string) (*runtimeapi.ContainerStatus, error) {
	// runtimeService, for example docker
	if c.runtimeFactory == nil {
		klog.Warning("NodePodProbe not found runtimeFactory")
		return nil, nil
	}
	runtimeService := c.runtimeFactory.GetRuntimeService()
	if runtimeService == nil {
		klog.Warning("NodePodProbe not found runtimeService")
		return nil, nil
	}
	containers, err := runtimeService.ListContainers(context.TODO(), &runtimeapi.ContainerFilter{
		LabelSelector: map[string]string{kubelettypes.KubernetesPodUIDLabel: podUID},
	})
	if err != nil {
		klog.ErrorS(err, "NodePodProbe pod list containers failed", "podUID", podUID)
		return nil, err
	}
	var container *runtimeapi.Container
	for i := range containers {
		obj := containers[i]
		if obj.Metadata.Name != name {
			continue
		}
		if container == nil || obj.CreatedAt > container.CreatedAt {
			container = obj
		}
	}
	var containerStatus *runtimeapi.ContainerStatusResponse
	if container != nil {
		containerStatus, err = runtimeService.ContainerStatus(context.TODO(), container.Id, false)
	}
	return containerStatus.Status, err
}

func updateNodePodProbeStatus(update Update, newStatus *appsv1alpha1.NodePodProbeStatus) {
	var probeStatus *appsv1alpha1.PodProbeStatus
	for i := range newStatus.PodProbeStatuses {
		status := &newStatus.PodProbeStatuses[i]
		if status.UID == update.Key.podUID {
			probeStatus = status
			break
		}
	}
	if probeStatus == nil {
		newStatus.PodProbeStatuses = append(newStatus.PodProbeStatuses, appsv1alpha1.PodProbeStatus{Namespace: update.Key.podNs, Name: update.Key.podName, UID: update.Key.podUID})
		probeStatus = &newStatus.PodProbeStatuses[len(newStatus.PodProbeStatuses)-1]
	}
	for i, obj := range probeStatus.ProbeStates {
		if obj.Name == update.Key.probeName {
			probeStatus.ProbeStates[i].State = update.State
			probeStatus.ProbeStates[i].Message = update.Msg
			probeStatus.ProbeStates[i].LastProbeTime = update.LastProbeTime
			if obj.State != update.State {
				probeStatus.ProbeStates[i].LastTransitionTime = metav1.Now()
			}
			return
		}
	}
	probeStatus.ProbeStates = append(probeStatus.ProbeStates, appsv1alpha1.ContainerProbeState{
		Name:               update.Key.probeName,
		State:              update.State,
		LastProbeTime:      update.LastProbeTime,
		LastTransitionTime: metav1.Now(),
		Message:            update.Msg,
	})
}
