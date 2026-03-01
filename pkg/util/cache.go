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

package util

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

var controllerCacheSyncTimeout time.Duration

func SetControllerCacheSyncTimeout(t time.Duration) {
	controllerCacheSyncTimeout = t
}

func GetControllerCacheSyncTimeout() time.Duration {
	return controllerCacheSyncTimeout
}

// GlobalCache using GVK/namespace/name as key
var GlobalCache = cache.NewStore(func(obj interface{}) (string, error) {
	metaObj, ok := obj.(metav1.Object)
	if !ok {
		return "", fmt.Errorf("failed to convert obj to metav1.Object")
	}
	namespacedName := fmt.Sprintf("%s/%s", metaObj.GetNamespace(), metaObj.GetName())

	runtimeObj, ok := obj.(runtime.Object)
	if !ok {
		return "", fmt.Errorf("failed to convert obj to runtime.Object")
	}
	key := fmt.Sprintf("%v/%s", runtimeObj.GetObjectKind().GroupVersionKind(), namespacedName)
	return key, nil
})
