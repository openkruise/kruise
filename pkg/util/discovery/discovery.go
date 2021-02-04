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

package discovery

import (
	"github.com/openkruise/kruise/apis"
	"github.com/openkruise/kruise/pkg/client"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var (
	internalScheme = runtime.NewScheme()
	isNotNotFound  = func(err error) bool { return !errors.IsNotFound(err) }
)

func init() {
	_ = apis.AddToScheme(internalScheme)
}

func DiscoverGVK(gvk schema.GroupVersionKind) bool {
	genericClient := client.GetGenericClient()
	if genericClient == nil {
		return true
	}
	discoveryClient := genericClient.DiscoveryClient

	var resourceList *metav1.APIResourceList
	err := retry.OnError(retry.DefaultBackoff, isNotNotFound, func() error {
		var err error
		resourceList, err = discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
		if err != nil && !errors.IsNotFound(err) {
			klog.Infof("Failed to get groupVersionKind %v: %v", gvk, err)
		}
		return err
	})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("Not found groupVersionKind %v: %v", gvk, err)
			return false
		}
		// This might be caused by abnormal apiserver or etcd, ignore it
		return true
	}

	for _, r := range resourceList.APIResources {
		if r.Kind == gvk.Kind {
			return true
		}
	}

	return false
}

func DiscoverObject(obj runtime.Object) bool {
	gvk, err := apiutil.GVKForObject(obj, internalScheme)
	if err != nil {
		klog.Warningf("Not recognized object %T in scheme: %v", obj, err)
		return false
	}
	return DiscoverGVK(gvk)
}
