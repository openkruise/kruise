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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

func (r *ReconcileSidecarTerminator) executeInPlaceUpdateAction(originalPod *corev1.Pod, sidecars sets.String) error {
	uncompletedSidecars := filterUncompletedSidecars(originalPod, sidecars)
	if uncompletedSidecars.Len() == 0 {
		return nil
	}

	var changed bool
	var pod *corev1.Pod
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pod = &corev1.Pod{}
		getErr := r.Client.Get(context.TODO(), types.NamespacedName{Name: originalPod.Name, Namespace: originalPod.Namespace}, pod)
		if getErr != nil {
			if errors.IsNotFound(getErr) {
				return nil
			}
			return getErr
		}

		changed, pod = updateSidecarsForInPlaceUpdate(pod, uncompletedSidecars)
		if !changed || pod == nil {
			return nil
		}

		return r.Update(context.TODO(), pod)
	})

	if err != nil {
		klog.ErrorS(err, "SidecarTerminator -- Error occurred when inPlace update pod", "pod", klog.KObj(originalPod))
	} else {
		klog.V(3).InfoS("SidecarTerminator -- InPlace update pod successfully", "pod", klog.KObj(originalPod))

		r.recorder.Eventf(originalPod, corev1.EventTypeNormal, "SidecarTerminator",
			"Kruise SidecarTerminator is trying to terminate sidecar %v using in-place update", uncompletedSidecars.List())
	}

	return err
}

// updateSidecarsForInPlaceUpdate replace sidecar image with KruiseTerminateSidecarWithImageEnv
func updateSidecarsForInPlaceUpdate(pod *corev1.Pod, sidecars sets.String) (bool, *corev1.Pod) {
	changed := false
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if !sidecars.Has(container.Name) {
			continue
		}
		changed = true
		container.Image = getImageFromEnv(container)
	}
	return changed, pod
}

// getImageFromEnv return the image set in sidecar env
func getImageFromEnv(container *corev1.Container) string {
	for i := range container.Env {
		if container.Env[i].Name == appsv1alpha1.KruiseTerminateSidecarWithImageEnv {
			return container.Env[i].Value
		}
	}
	return ""
}
