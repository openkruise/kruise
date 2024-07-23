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
	"fmt"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"github.com/openkruise/kruise/apis"
	"github.com/openkruise/kruise/pkg/client"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var (
	internalScheme = runtime.NewScheme()

	errKindNotFound = fmt.Errorf("kind not found in group version resources")
	backOff         = wait.Backoff{
		Steps:    4,
		Duration: 500 * time.Millisecond,
		Factor:   5.0,
		Jitter:   0.1,
	}
)

func init() {
	utilruntime.Must(apis.AddToScheme(internalScheme))
}

func DiscoverGVK(gvk schema.GroupVersionKind) bool {
	genericClient := client.GetGenericClient()
	if genericClient == nil {
		return true
	}
	discoveryClient := genericClient.DiscoveryClient

	startTime := time.Now()
	err := retry.OnError(backOff, func(err error) bool { return true }, func() error {
		resourceList, err := discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
		if err != nil {
			return err
		}
		for _, r := range resourceList.APIResources {
			if r.Kind == gvk.Kind {
				return nil
			}
		}
		return errKindNotFound
	})

	if err != nil {
		if err == errKindNotFound {
			klog.InfoS("Not found kind in group version", "kind", gvk.Kind, "groupVersion", gvk.GroupVersion().String(), "cost", time.Since(startTime))
			return false
		}

		// This might be caused by abnormal apiserver or etcd, ignore it
		klog.ErrorS(err, "Failed to find resources in group version", "groupVersion", gvk.GroupVersion().String(), "cost", time.Since(startTime))
	}

	return true
}

func DiscoverObject(obj runtime.Object) bool {
	gvk, err := apiutil.GVKForObject(obj, internalScheme)
	if err != nil {
		klog.ErrorS(err, "Not recognized object in scheme", "object", obj)
		return false
	}
	return DiscoverGVK(gvk)
}
