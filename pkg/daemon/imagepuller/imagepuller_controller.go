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

package imagepuller

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"reflect"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/client"
	kruiseclient "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	listersalpha1 "github.com/openkruise/kruise/pkg/client/listers/apps/v1alpha1"
	daemonoptions "github.com/openkruise/kruise/pkg/daemon/options"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	utilimagejob "github.com/openkruise/kruise/pkg/util/imagejob"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

type Controller struct {
	scheme                *runtime.Scheme
	queue                 workqueue.RateLimitingInterface
	puller                puller
	imagePullNodeInformer cache.SharedIndexInformer
	imagePullNodeLister   listersalpha1.NodeImageLister
	statusUpdater         *statusUpdater
}

// NewController returns the controller for image pulling
func NewController(opts daemonoptions.Options, secretManager daemonutil.SecretManager, cfg *rest.Config) (*Controller, error) {
	genericClient := client.GetGenericClientWithName("kruise-daemon-imagepuller")
	informer := newNodeImageInformer(genericClient.KruiseClient, opts.NodeName)

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: genericClient.KubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(opts.Scheme, v1.EventSource{Component: "kruise-daemon-imagepuller", Host: opts.NodeName})

	queue := workqueue.NewNamedRateLimitingQueue(
		// Backoff duration from 500ms to 50~55s
		// For nodeimage controller will mark a image:tag task failed (not responded for a long time) if daemon does not report status in 60s.
		workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 50*time.Second+time.Millisecond*time.Duration(rand.Intn(5000))),
		"imagepuller",
	)

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			nodeImage, ok := obj.(*appsv1alpha1.NodeImage)
			if ok {
				enqueue(queue, nodeImage)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldNodeImage, oldOK := oldObj.(*appsv1alpha1.NodeImage)
			newNodeImage, newOK := newObj.(*appsv1alpha1.NodeImage)
			if !oldOK || !newOK {
				return
			}
			if reflect.DeepEqual(oldNodeImage.Spec, newNodeImage.Spec) {
				klog.V(5).InfoS("Find imagePullNode spec has not changed, skip enqueueing.", "nodeImage", newNodeImage.Name)
				return
			}
			logNewImages(oldNodeImage, newNodeImage)
			enqueue(queue, newNodeImage)
		},
	})

	puller, err := newRealPuller(opts.RuntimeFactory.GetImageService(), secretManager, recorder)
	if err != nil {
		return nil, fmt.Errorf("failed to new puller: %v", err)
	}

	opts.Healthz.RegisterFunc("nodeImageInformerSynced", func(_ *http.Request) error {
		if !informer.HasSynced() {
			return fmt.Errorf("not synced")
		}
		return nil
	})

	return &Controller{
		scheme:                opts.Scheme,
		queue:                 queue,
		puller:                puller,
		imagePullNodeInformer: informer,
		imagePullNodeLister:   listersalpha1.NewNodeImageLister(informer.GetIndexer()),
		statusUpdater:         newStatusUpdater(genericClient.KruiseClient.AppsV1alpha1().NodeImages()),
	}, nil
}

func newNodeImageInformer(client kruiseclient.Interface, nodeName string) cache.SharedIndexInformer {
	tweakListOptionsFunc := func(opt *metav1.ListOptions) {
		opt.FieldSelector = "metadata.name=" + nodeName
	}

	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				tweakListOptionsFunc(&options)
				return client.AppsV1alpha1().NodeImages().List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				tweakListOptionsFunc(&options)
				return client.AppsV1alpha1().NodeImages().Watch(context.TODO(), options)
			},
		},
		&appsv1alpha1.NodeImage{},
		0, // do not resync
		cache.Indexers{},
	)
}

func enqueue(queue workqueue.Interface, obj *appsv1alpha1.NodeImage) {
	if obj.DeletionTimestamp != nil {
		return
	}
	key, _ := cache.MetaNamespaceKeyFunc(obj)
	queue.Add(key)
}

func (c *Controller) Run(stop <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Info("Starting informer for NodeImage")
	go c.imagePullNodeInformer.Run(stop)
	if !cache.WaitForCacheSync(stop, c.imagePullNodeInformer.HasSynced) {
		return
	}

	klog.Info("Starting puller controller")
	// Launch one workers to process resources, for there is only one NodeImage per Node
	go wait.Until(func() {
		for c.processNextWorkItem() {
		}
	}, time.Second, stop)

	klog.Info("Started puller controller successfully")
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
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.InfoS("Invalid key", "key", key)
		return nil
	}

	nodeImage, err := c.imagePullNodeLister.Get(name)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		klog.ErrorS(err, "Failed to get NodeImage %s: %v", "nodeImage", name)
		return err
	}

	klog.V(3).InfoS("Start syncing", "name", name)
	defer func() {
		if retErr != nil {
			klog.ErrorS(retErr, "Failed to sync", "name", name)
		} else {
			klog.V(3).InfoS("Finished syncing", "name", name)
		}
	}()

	ref, _ := reference.GetReference(c.scheme, nodeImage)
	retErr = c.puller.Sync(nodeImage.DeepCopy(), ref)
	if retErr != nil {
		return
	}

	newStatus := appsv1alpha1.NodeImageStatus{
		ImageStatuses: make(map[string]appsv1alpha1.ImageStatus),
	}
	for imageName, imageSpec := range nodeImage.Spec.Images {
		newStatus.Desired += int32(len(imageSpec.Tags))

		imageStatus := c.puller.GetStatus(imageName)
		if klog.V(9).Enabled() {
			klog.V(9).InfoS("get image status", "imageName", imageName, "imageStatus", imageStatus)
		}
		if imageStatus == nil {
			continue
		}
		utilimagejob.SortStatusImageTags(imageStatus)
		newStatus.ImageStatuses[imageName] = *imageStatus
		for _, tagStatus := range imageStatus.Tags {
			switch tagStatus.Phase {
			case appsv1alpha1.ImagePhaseSucceeded:
				newStatus.Succeeded++
			case appsv1alpha1.ImagePhaseFailed:
				newStatus.Failed++
			case appsv1alpha1.ImagePhasePulling:
				newStatus.Pulling++
			}
		}
	}
	if len(newStatus.ImageStatuses) == 0 {
		newStatus.ImageStatuses = nil
	}

	var limited bool
	limited, retErr = c.statusUpdater.updateStatus(nodeImage, &newStatus)
	if retErr != nil {
		return retErr
	}

	if limited || isImageInPulling(&nodeImage.Spec, &newStatus) {
		// 3~5s
		c.queue.AddAfter(key, 3*time.Second+time.Millisecond*time.Duration(rand.Intn(2000)))
	} else {
		// 20~30m
		c.queue.AddAfter(key, 20*time.Minute+time.Millisecond*time.Duration(rand.Intn(600000)))
	}
	return nil
}
