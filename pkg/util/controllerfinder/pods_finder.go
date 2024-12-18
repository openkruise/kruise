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
	"fmt"

	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetPodsForRef return target workload's podList and spec.replicas.
func (r *ControllerFinder) GetPodsForRef(apiVersion, kind, ns, name string, active bool) ([]*corev1.Pod, int32, error) {
	var workloadUIDs []types.UID
	var workloadReplicas int32
	var labelSelector *metav1.LabelSelector
	switch kind {
	// ReplicaSet
	case ControllerKindRS.Kind:
		rs, err := r.getReplicaSet(ControllerReference{APIVersion: apiVersion, Kind: kind, Name: name}, ns)
		if err != nil {
			return nil, -1, err
		} else if rs == nil || !rs.DeletionTimestamp.IsZero() {
			return nil, 0, nil
		}
		workloadReplicas = *rs.Spec.Replicas
		workloadUIDs = append(workloadUIDs, rs.UID)
	// statefulset, rc, cloneSet
	case ControllerKindSS.Kind, ControllerKindRC.Kind, ControllerKruiseKindCS.Kind, ControllerKruiseKindSS.Kind:
		obj, err := r.GetScaleAndSelectorForRef(apiVersion, kind, ns, name, "")
		if err != nil {
			return nil, -1, err
		} else if obj == nil || !obj.Metadata.DeletionTimestamp.IsZero() {
			return nil, 0, nil
		}
		workloadReplicas = obj.Scale
		workloadUIDs = append(workloadUIDs, obj.UID)
	// Deployment
	case ControllerKindDep.Kind:
		obj, err := r.GetScaleAndSelectorForRef(apiVersion, kind, ns, name, "")
		if err != nil {
			return nil, -1, err
		} else if obj == nil || !obj.Metadata.DeletionTimestamp.IsZero() {
			return nil, 0, nil
		}
		workloadReplicas = obj.Scale
		// try to get replicaSets
		rss, err := r.getReplicaSetsForObject(obj)
		if err != nil {
			return nil, -1, err
		}
		if len(rss) == 0 {
			workloadUIDs = append(workloadUIDs, obj.UID)
		} else {
			for _, rs := range rss {
				workloadUIDs = append(workloadUIDs, rs.UID)
			}
		}
	// The Other custom workload(support scale sub-resources)
	default:
		obj, err := r.GetScaleAndSelectorForRef(apiVersion, kind, ns, name, "")
		if err != nil {
			return nil, -1, err
		} else if obj == nil || !obj.Metadata.DeletionTimestamp.IsZero() {
			return nil, 0, nil
		}
		workloadReplicas = obj.Scale
		labelSelector = obj.Selector
		workloadUIDs = append(workloadUIDs, obj.UID)
	}
	klog.V(5).InfoS("find pods and replicas result", "target", fmt.Sprintf("%s/%s", ns, name), "kind", kind,
		"workloadReplicas", workloadReplicas, "workloadUIDs", workloadUIDs, "labelSelector", labelSelector)
	if workloadReplicas == 0 {
		return nil, workloadReplicas, nil
	}

	listPods := func(listOption *client.ListOptions) ([]*corev1.Pod, error) {
		matchedPods := make([]*corev1.Pod, 0)
		podList := &corev1.PodList{}
		if err := r.List(context.TODO(), podList, listOption, utilclient.DisableDeepCopy); err != nil {
			return nil, err
		}
		for i := range podList.Items {
			pod := &podList.Items[i]
			// filter not active Pod if active is true.
			if active && !kubecontroller.IsPodActive(pod) {
				continue
			}
			matchedPods = append(matchedPods, pod)
		}
		return matchedPods, nil
	}

	var err error
	var matchedPods []*corev1.Pod
	for _, uid := range workloadUIDs {
		listOption := client.ListOptions{
			Namespace:     ns,
			FieldSelector: fields.SelectorFromSet(fields.Set{fieldindex.IndexNameForOwnerRefUID: string(uid)}),
		}
		pods, err := listPods(&listOption)
		klog.V(5).InfoS("result of list pods with owner ref uid",
			"target", fmt.Sprintf("%s/%s", ns, name), "kind", kind, "pods", len(pods), "err", err, "refUid", uid)
		if err != nil {
			return nil, -1, err
		}
		matchedPods = append(matchedPods, pods...)
	}

	// For such workloads like Deployment that do not manage the Pods directly,
	// Pods' ownerReferences do not contain the workload, so we have to retry
	// to use the label selector to list the Pods.
	if labelSelector != nil && len(matchedPods) == 0 {
		selector, _ := metav1.LabelSelectorAsSelector(labelSelector)
		listOption := client.ListOptions{
			Namespace:     ns,
			LabelSelector: selector,
		}
		matchedPods, err = listPods(&listOption)
		if err != nil {
			return nil, -1, err
		}
	}
	return matchedPods, workloadReplicas, nil
}

func (r *ControllerFinder) getReplicaSetsForObject(scale *ScaleAndSelector) ([]appsv1.ReplicaSet, error) {
	// List ReplicaSets owned by this Deployment
	rsList := &appsv1.ReplicaSetList{}
	selector, err := util.ValidatedLabelSelectorAsSelector(scale.Selector)
	if err != nil {
		klog.ErrorS(err, "Object get labelSelector failed", "namespace", scale.Metadata.Namespace, "name", scale.Metadata.Name)
		return nil, nil
	}
	err = r.List(context.TODO(), rsList, &client.ListOptions{Namespace: scale.Metadata.Namespace, LabelSelector: selector}, utilclient.DisableDeepCopy)
	if err != nil {
		return nil, err
	}
	rss := make([]appsv1.ReplicaSet, 0)
	for i := range rsList.Items {
		rs := rsList.Items[i]
		// This method is used to list the pods of the deployment, so the rs of spec.replicas == 0 cannot be ignored,
		// because even if rs.spec.replicas == 0, rs may still contain pods.
		if !rs.DeletionTimestamp.IsZero() {
			continue
		}
		if ref := metav1.GetControllerOf(&rs); ref != nil && ref.UID == scale.UID {
			rss = append(rss, rs)
		}
	}
	return rss, nil
}
