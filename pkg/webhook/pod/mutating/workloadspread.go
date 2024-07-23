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

package mutating

import (
	"context"

	wsutil "github.com/openkruise/kruise/pkg/util/workloadspread"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/util/dryrun"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (h *PodCreateHandler) workloadSpreadMutatingPod(ctx context.Context, req admission.Request, pod *corev1.Pod) (skip bool, err error) {
	if len(req.AdmissionRequest.SubResource) > 0 ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return true, nil
	}

	workloadSpreadHandler := wsutil.NewWorkloadSpreadHandler(h.Client)
	var dryRun bool

	switch req.AdmissionRequest.Operation {
	case admissionv1.Create:
		options := &metav1.CreateOptions{}
		err := h.Decoder.DecodeRaw(req.Options, options)
		if err != nil {
			return false, err
		}
		// check dry run
		dryRun = dryrun.IsDryRun(options.DryRun)
		if dryRun {
			klog.V(5).InfoS("Operation is a dry run, then admit", "operation", req.AdmissionRequest.Operation, "namespace", pod.Namespace, "podName", pod.Name)
			return true, nil
		}
		return workloadSpreadHandler.HandlePodCreation(pod)
	default:
		return true, nil
	}
}
