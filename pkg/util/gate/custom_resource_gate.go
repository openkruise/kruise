/*
Copyright 2019 The Kruise Authors.

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

package gate

import (
	"os"
	"strings"

	"github.com/openkruise/kruise/apis"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	envCustomResourceEnable = "CUSTOM_RESOURCE_ENABLE"
)

var (
	internalScheme  = runtime.NewScheme()
	discoveryClient discovery.DiscoveryInterface
)

func init() {
	_ = apis.AddToScheme(internalScheme)
	cfg, err := config.GetConfig()
	if err == nil {
		discoveryClient = discovery.NewDiscoveryClientForConfigOrDie(cfg)
	}
}

// ResourceEnabled help runnable check if the custom resource is valid and enabled
// 1. If this CRD is not found from kueb-apiserver, it is invalid.
// 2. If 'CUSTOM_RESOURCE_ENABLE' env is not empty and this CRD kind is not in ${CUSTOM_RESOURCE_ENABLE}.
func ResourceEnabled(obj runtime.Object) bool {
	gvk, err := apiutil.GVKForObject(obj, internalScheme)
	if err != nil {
		klog.Warningf("custom resource gate not recognized object %T in scheme: %v", obj, err)
		return false
	}

	return discoveryEnabled(gvk) && envEnabled(gvk)
}

func discoveryEnabled(gvk schema.GroupVersionKind) bool {
	if discoveryClient == nil {
		return true
	}
	resourceList, err := discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		klog.V(4).Infof("custom resource gate not found groupVersionKind %v in discovery: %v", gvk, err)
		return false
	}

	for _, r := range resourceList.APIResources {
		if r.Kind == gvk.Kind {
			return true
		}
	}

	return false
}

func envEnabled(gvk schema.GroupVersionKind) bool {
	limits := strings.TrimSpace(os.Getenv(envCustomResourceEnable))
	if len(limits) == 0 {
		// all enabled by default
		return true
	}

	if !sets.NewString(strings.Split(limits, ",")...).Has(gvk.Kind) {
		klog.Warningf("custom resource gate not found groupVersionKind %v in CUSTOM_RESOURCE_ENABLE: %v", gvk, limits)
		return false
	}

	return true
}
