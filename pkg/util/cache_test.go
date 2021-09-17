/*
Copyright 2020 The Kruise Authors.

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
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestGlobalCache(t *testing.T) {
	podObj := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "pod-name",
			Namespace:       "pod-namespace",
			ResourceVersion: "1",
		},
	}
	ws1Obj := &appsv1alpha1.WorkloadSpread{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps.kruise.io/v1alpha1",
			Kind:       "WorkloadSpread",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "ws1-name",
			Namespace:       "ws1-namespace",
			ResourceVersion: "1",
		},
	}
	ws2Obj := &appsv1alpha1.WorkloadSpread{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps.kruise.io/v1alpha1",
			Kind:       "WorkloadSpread",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "ws2-name",
			Namespace:       "ws2-namespace",
			ResourceVersion: "1",
		},
	}
	ws3Obj := &appsv1alpha1.WorkloadSpread{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps.kruise.io/v1alpha1",
			Kind:       "WorkloadSpread",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "ws3-name",
			Namespace:       "ws3-namespace",
			ResourceVersion: "1",
		},
	}

	// add
	if err := GlobalCache.Add(podObj); err != nil {
		t.Fatalf("Failed to add obj to GlobalCache, %v", err)
	}
	if err := GlobalCache.Add(ws1Obj); err != nil {
		t.Fatalf("Failed to add obj to GlobalCache, %v", err)
	}
	if err := GlobalCache.Add(ws2Obj); err != nil {
		t.Fatalf("Failed to add obj to GlobalCache, %v", err)
	}
	if err := GlobalCache.Add(ws3Obj); err != nil {
		t.Fatalf("Failed to add obj to GlobalCache, %v", err)
	}

	// update
	podObj.SetResourceVersion("2")
	ws1Obj.SetResourceVersion("2")
	if err := GlobalCache.Add(podObj); err != nil {
		t.Fatalf("Failed to update obj to GlobalCache, %v", err)
	}
	if err := GlobalCache.Add(ws1Obj); err != nil {
		t.Fatalf("Failed to update obj to GlobalCache, %v", err)
	}

	// delete
	if err := GlobalCache.Delete(&appsv1alpha1.WorkloadSpread{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps.kruise.io/v1alpha1",
			Kind:       "WorkloadSpread",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ws3Obj.GetNamespace(),
			Name:      ws3Obj.GetName(),
		},
	}); err != nil {
		t.Fatalf("Failed to delete obj to GlobalCache, %v", err)
	}

	// list & Get
	objs := GlobalCache.List()
	expected := map[string]runtime.Object{
		"pod-name": podObj,
		"ws1-name": ws1Obj,
		"ws2-name": ws2Obj,
	}
	expectedResourceVersion := map[string]string{
		"pod-name": "2",
		"ws1-name": "2",
		"ws2-name": "1",
	}
	if len(objs) != len(expected) {
		t.Fatalf("the number of expected cached objects is %d, but got %d", len(expected), len(objs))
	}
	for key, obj := range expected {
		item, ok, err := GlobalCache.Get(obj)
		if !ok || err != nil {
			t.Fatalf("expected object(%s) not found in GlobalCache", key)
		}
		cachedObj := item.(metav1.Object)
		if cachedObj.GetName() != key ||
			cachedObj.GetResourceVersion() != expectedResourceVersion[key] {
			t.Fatalf("get wrong version of cached object")
		}
	}
}
