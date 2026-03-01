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

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	"github.com/openkruise/kruise/pkg/controller/podunavailablebudget"
)

// mutating relate-pub annotation in pod
func (h *PodCreateHandler) pubMutatingPod(ctx context.Context, req admission.Request, pod *corev1.Pod) (skip bool, err error) {
	if len(req.AdmissionRequest.SubResource) > 0 || req.AdmissionRequest.Operation != admissionv1.Create ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return true, nil
	}
	pub, err := podunavailablebudget.GetPubForPod(h.Client, pod)
	if err != nil {
		klog.ErrorS(err, "Failed to get pub for pod", "pod", klog.KObj(pod))
		return false, nil
	} else if pub == nil {
		return true, nil
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[pubcontrol.PodRelatedPubAnnotation] = pub.Name
	klog.V(3).InfoS("mutating add pod annotation", "namespace", pod.Namespace, "name", pod.Name, "key", pubcontrol.PodRelatedPubAnnotation, "value", pub.Name)
	return false, nil
}
