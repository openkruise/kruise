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

package sorting

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	synccontrol "github.com/openkruise/kruise/pkg/controller/cloneset/sync"
	sidecarsetcontroller "github.com/openkruise/kruise/pkg/controller/sidecarset"
	statefulsetcontroller "github.com/openkruise/kruise/pkg/controller/statefulset"
	"github.com/openkruise/kruise/pkg/util/updatesort"
)

// SortPods sorts the given Pods according the owner workload logic.
// Currently only for image pre-downloading.
func SortPods(reader client.Reader, ns string, owner metav1.OwnerReference, pods []*v1.Pod) ([]*v1.Pod, error) {
	gv, err := schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("parse apiVersion of owner reference %v error: %v", owner, err)
	}

	// ignore no Kruise owners
	if gv.Group != appsv1alpha1.GroupVersion.Group {
		return pods, nil
	}

	var indexes []int
	namespacedName := types.NamespacedName{Namespace: ns, Name: owner.Name}
	switch owner.Kind {
	case "CloneSet":
		set := &appsv1alpha1.CloneSet{}
		if err := reader.Get(context.TODO(), namespacedName, set); err != nil {
			if errors.IsNotFound(err) {
				return pods, nil
			}
			return nil, fmt.Errorf("get cloneset %s error: %v", namespacedName, err)
		}
		indexes = sortPodsForCloneSet(set, pods)

	case "StatefulSet":
		set := &appsv1beta1.StatefulSet{}
		if err := reader.Get(context.TODO(), namespacedName, set); err != nil {
			if errors.IsNotFound(err) {
				return pods, nil
			}
			return nil, fmt.Errorf("get statefulset %s error: %v", namespacedName, err)
		}
		indexes = sortPodsForStatefulSet(set, pods)

	case "SidecarSet":
		set := &appsv1alpha1.SidecarSet{}
		if err := reader.Get(context.TODO(), namespacedName, set); err != nil {
			if errors.IsNotFound(err) {
				return pods, nil
			}
			return nil, fmt.Errorf("get sidecarset %s error: %v", namespacedName, err)
		}
		indexes = sortPodsForSidecarSet(set, pods)

	default:
		// unsupported sorting workload
		return pods, nil
	}

	newPods := make([]*v1.Pod, 0, len(pods))
	for _, i := range indexes {
		newPods = append(newPods, pods[i])
	}
	return newPods, nil
}

func sortPodsForCloneSet(set *appsv1alpha1.CloneSet, pods []*v1.Pod) []int {
	indexes := make([]int, 0, len(pods))
	for i := 0; i < len(pods); i++ {
		indexes = append(indexes, i)
	}

	indexes = synccontrol.SortUpdateIndexes(clonesetcore.New(set), set.Spec.UpdateStrategy, pods, indexes)
	return indexes
}

func sortPodsForStatefulSet(set *appsv1beta1.StatefulSet, pods []*v1.Pod) []int {
	indexes := make([]int, 0, len(pods))
	for i := len(pods) - 1; i >= 0; i-- {
		indexes = append(indexes, i)
	}

	statefulsetcontroller.SortPodsAscendingOrdinal(pods)
	if set.Spec.UpdateStrategy.RollingUpdate != nil &&
		set.Spec.UpdateStrategy.RollingUpdate.UnorderedUpdate != nil &&
		set.Spec.UpdateStrategy.RollingUpdate.UnorderedUpdate.PriorityStrategy != nil {
		indexes = updatesort.NewPrioritySorter(set.Spec.UpdateStrategy.RollingUpdate.UnorderedUpdate.PriorityStrategy).Sort(pods, indexes)
	}
	return indexes
}

func sortPodsForSidecarSet(set *appsv1alpha1.SidecarSet, pods []*v1.Pod) []int {
	indexes := make([]int, 0, len(pods))
	for i := 0; i < len(pods); i++ {
		indexes = append(indexes, i)
	}

	indexes = sidecarsetcontroller.SortUpdateIndexes(set.Spec.UpdateStrategy, pods, indexes)
	return indexes
}
