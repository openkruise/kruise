/*
Copyright 2023 The Kruise Authors.

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

package sidecarterminator

import (
	"context"
	"fmt"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ReconcileSidecarTerminator) executeKillContainerAction(pod *corev1.Pod, sidecars sets.String) error {
	uncompletedSidecars := filterUncompletedSidecars(pod, sidecars)
	if uncompletedSidecars.Len() == 0 {
		return nil
	}

	// if the CRR has been created, this func will do nothing
	existingCRR := &appsv1alpha1.ContainerRecreateRequest{}
	err := r.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: getCRRName(pod)}, existingCRR)
	if err == nil {
		klog.V(3).InfoS("SidecarTerminator -- CRR exists, waiting for this CRR to complete", "containerRecreateRequest", klog.KObj(existingCRR))
		return nil
	} else if client.IgnoreNotFound(err) != nil {
		klog.ErrorS(err, "SidecarTerminator -- Error occurred when try to get CRR", "containerRecreateRequest", klog.KObj(existingCRR))
		return err
	}

	var sidecarContainers []appsv1alpha1.ContainerRecreateRequestContainer
	for _, name := range uncompletedSidecars.List() {
		sidecarContainers = append(sidecarContainers, appsv1alpha1.ContainerRecreateRequestContainer{
			Name: name,
		})
	}

	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pod.Namespace,
			Name:      getCRRName(pod),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(pod, pod.GroupVersionKind()),
			},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName:    pod.Name,
			Containers: sidecarContainers,
			Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
				ForceRecreate: true,
				FailurePolicy: appsv1alpha1.ContainerRecreateRequestFailurePolicyIgnore,
			},
		},
	}

	err = r.Create(context.TODO(), crr)
	if err != nil {
		klog.ErrorS(err, "SidecarTerminator -- Error occurred when creating", "containerRecreateRequest", klog.KObj(crr))
	} else {
		klog.V(3).InfoS("SidecarTerminator -- Creating CRR successfully", "containerRecreateRequest", klog.KObj(crr))

		r.recorder.Eventf(pod, corev1.EventTypeNormal, "SidecarTerminator",
			"Kruise SidecarTerminator is trying to terminate sidecar %v using crr", uncompletedSidecars.List())
	}

	return err
}

func filterUncompletedSidecars(pod *corev1.Pod, sidecars sets.String) sets.String {
	uncompletedSidecars := sets.NewString(sidecars.List()...)
	for i := range pod.Status.ContainerStatuses {
		status := &pod.Status.ContainerStatuses[i]
		if sidecars.Has(status.Name) && status.State.Terminated != nil {
			uncompletedSidecars.Delete(status.Name)
		}
	}
	return uncompletedSidecars
}

func getCRRName(pod *corev1.Pod) string {
	return fmt.Sprintf("sidecar-termination-%v", pod.UID)
}
