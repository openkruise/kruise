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

package mutating

import (
	"context"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/utils/sidecarcontrol"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// mutate pod based on SidecarSet Object
func (h *PodCreateHandler) sidecarSetMutatingPod(ctx context.Context, req admission.Request, pod *corev1.Pod) (skip bool, err error) {
	if len(req.AdmissionRequest.SubResource) > 0 ||
		(req.AdmissionRequest.Operation != admissionv1.Create && req.AdmissionRequest.Operation != admissionv1.Update) ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return true, nil
	}
	// filter out pods that don't require inject
	if !sidecarcontrol.IsActivePod(pod) {
		return true, nil
	}

	var oldPod *corev1.Pod
	//when Operation is update, decode older object
	if req.AdmissionRequest.Operation == admissionv1.Update {
		oldPod = new(corev1.Pod)
		if err = h.Decoder.Decode(
			admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{Object: req.AdmissionRequest.OldObject}},
			oldPod); err != nil {
			return false, err
		}
	}

	// DisableDeepCopy:true, indicates must be deep copy before update sidecarSet objection
	sidecarSetList := &appsv1alpha1.SidecarSetList{}
	if err = h.Client.List(ctx, sidecarSetList, utilclient.DisableDeepCopy); err != nil {
		return false, err
	}
	sidecarSets := make([]*appsv1alpha1.SidecarSet, 0)
	for i := range sidecarSetList.Items {
		sidecarSet := &sidecarSetList.Items[i]
		sidecarSets = append(sidecarSets, sidecarSet)
	}
	return sidecarcontrol.SidecarSetMutatingPod(pod, oldPod, sidecarSets, h.sidecarSetControl)
}
