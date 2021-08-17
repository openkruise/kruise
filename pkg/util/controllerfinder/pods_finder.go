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

package controllerfinder

import (
	"context"

	"github.com/openkruise/kruise/pkg/util/fieldindex"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ControllerFinder) GetPodsForRef(apiVersion, kind, ns, name string) ([]*corev1.Pod, error) {
	workloadUIDs := make([]types.UID, 0)
	switch kind {
	// ReplicaSet
	case controllerKindRS.Kind:
		rs, err := r.getReplicaSet(ControllerReference{APIVersion: apiVersion, Kind: kind, Name: name}, ns)
		if err != nil || rs == nil {
			return nil, err
		}
		workloadUIDs = append(workloadUIDs, rs.UID)
	// Deployment, get the corresponding ReplicaSet UID
	case controllerKindDep.Kind:
		rss, err := r.getReplicaSetsForDeployment(apiVersion, kind, ns, name)
		if err != nil || len(rss) == 0 {
			return nil, err
		}
		for _, rs := range rss {
			workloadUIDs = append(workloadUIDs, rs.UID)
		}
	// others, e.g. rc, cloneset, statefulset...
	default:
		obj, err := r.GetScaleAndSelectorForRef(apiVersion, kind, ns, name, "")
		if err != nil || obj == nil {
			return nil, err
		}
		workloadUIDs = append(workloadUIDs, obj.UID)
	}

	// List all Pods owned by workload UID.
	matchedPods := make([]*corev1.Pod, 0)
	for _, uid := range workloadUIDs {
		podList := &corev1.PodList{}
		listOption := &client.ListOptions{
			Namespace:     ns,
			FieldSelector: fields.SelectorFromSet(fields.Set{fieldindex.IndexNameForOwnerRefUID: string(uid)}),
		}
		if err := r.List(context.TODO(), podList, listOption); err != nil {
			return nil, err
		}
		for i := range podList.Items {
			pod := &podList.Items[i]
			if kubecontroller.IsPodActive(pod) {
				matchedPods = append(matchedPods, pod)
			}
		}
	}

	return matchedPods, nil
}

func (r *ControllerFinder) getReplicaSetsForDeployment(apiVersion, kind, ns, name string) ([]appsv1.ReplicaSet, error) {
	targetRef := ControllerReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
	}
	scaleNSelector, err := r.getPodDeployment(targetRef, ns)
	if err != nil || scaleNSelector == nil {
		return nil, err
	}
	// List ReplicaSets owned by this Deployment
	rsList := &appsv1.ReplicaSetList{}
	selector, err := metav1.LabelSelectorAsSelector(scaleNSelector.Selector)
	if err != nil {
		klog.Errorf("Deployment (%s/%s) get labelSelector failed: %s", ns, name, err.Error())
		return nil, nil
	}
	err = r.List(context.TODO(), rsList, &client.ListOptions{Namespace: ns, LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	rss := make([]appsv1.ReplicaSet, 0)
	for i := range rsList.Items {
		rs := rsList.Items[i]
		if ref := metav1.GetControllerOf(&rs); ref != nil {
			if ref.UID == scaleNSelector.UID {
				rss = append(rss, rs)
			}
		}
	}
	return rss, nil
}
