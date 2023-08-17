package util

import (
	"fmt"
	"sync"
	"time"

	"github.com/openkruise/kruise/pkg/client"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
			klog.Warningf("Not found kind %s in group version %s, waiting time %s", gvk.Kind, gvk.GroupVersion().String(), time.Since(startTime))
			return false
		}

		// This might be caused by abnormal apiserver or etcd, ignore it
		klog.Errorf("Failed to find resources in group version %s: %v, waiting time %s", gvk.GroupVersion().String(), err, time.Since(startTime))
	}

	return true
}

func AddWatcherDynamically(c controller.Controller, h handler.EventHandler, gvk schema.GroupVersionKind, controllerKey string) (bool, error) {
	cacheKey := fmt.Sprintf("controller:%s, gvk:%s", controllerKey, gvk.String())
	if _, ok := watcherMap.Load(cacheKey); ok {
		return false, nil
	}

	if !DiscoverGVK(gvk) {
		klog.Errorf("Failed to find GVK(%v) in cluster for %s", gvk.String(), controllerKey)
		return false, nil
	}

	object := &unstructured.Unstructured{}
	object.SetGroupVersionKind(gvk)
	watcherMap.Store(cacheKey, true)
	return true, c.Watch(&source.Kind{Type: object}, h)
}
