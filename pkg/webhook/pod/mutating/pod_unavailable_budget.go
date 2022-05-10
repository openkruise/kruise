/*
Copyright 2022 The Kruise Authors.

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

package mutating

import (
	"context"

	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// mutating relate-pub annotation in pod
func (h *PodCreateHandler) pubMutatingPod(ctx context.Context, req admission.Request, pod *corev1.Pod) error {
	if len(req.AdmissionRequest.SubResource) > 0 || req.AdmissionRequest.Operation != admissionv1.Create ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return nil
	}
	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return nil
	}
	workload, err := h.finder.GetScaleAndSelectorForRef(ref.APIVersion, ref.Kind, pod.Namespace, ref.Name, "")
	if err != nil {
		return err
	} else if workload == nil {
		return nil
	}
	// fetch pub for workload
	pub, err := pubcontrol.GetPubForWorkload(h.Client, workload)
	if err != nil {
		return err
	} else if pub == nil {
		return nil
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[pubcontrol.PodRelatedPubAnnotation] = pub.Name
	klog.V(3).Infof("mutating add pod(%s/%s) annotation[%s]=%s", pod.Namespace, pod.Name, pubcontrol.PodRelatedPubAnnotation, pub.Name)
	return nil
}
