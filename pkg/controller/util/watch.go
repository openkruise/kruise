package util

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openkruise/kruise/pkg/client"
)

var watcherMap = sync.Map{}

func DiscoverGVK(gvk schema.GroupVersionKind) bool {
	genericClient := client.GetGenericClient()
	if genericClient == nil {
		return true
	}
	discoveryClient := genericClient.DiscoveryClient

	startTime := time.Now()
	err := retry.OnError(retry.DefaultRetry, func(err error) bool { return true }, func() error {
		resourceList, err := discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
		if err != nil {
			return err
		}
		for _, r := range resourceList.APIResources {
			if r.Kind == gvk.Kind {
				return nil
			}
		}
		return errors.NewNotFound(schema.GroupResource{Group: gvk.GroupVersion().String(), Resource: gvk.Kind}, "")
	})

	if err != nil {
		if errors.IsNotFound(err) {
			klog.InfoS("Kind not found in group version after waiting", "kind", gvk.Kind, "groupVersion", gvk.GroupVersion(), "waitingTime", time.Since(startTime))
			return false
		}

		// This might be caused by abnormal apiserver or etcd, ignore it
		klog.ErrorS(err, "Failed to find resources in group version after waiting", "groupVersion", gvk.GroupVersion(), "waitingTime", time.Since(startTime))
	}

	return true
}

func AddWatcherDynamically(mgr manager.Manager, c controller.Controller, h handler.EventHandler, gvk schema.GroupVersionKind, controllerKey string) (bool, error) {
	cacheKey := fmt.Sprintf("controller:%s, gvk:%s", controllerKey, gvk.String())
	if _, ok := watcherMap.Load(cacheKey); ok {
		return false, nil
	}

	if !DiscoverGVK(gvk) {
		klog.ErrorS(nil, "Failed to find GVK in cluster for controller", "GVK", gvk, "controllerKey", controllerKey)
		return false, nil
	}

	object := &unstructured.Unstructured{}
	object.SetGroupVersionKind(gvk)
	watcherMap.Store(cacheKey, true)
	return true, c.Watch(source.Kind(mgr.GetCache(), crclient.Object(object), h))
}
