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
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/util/dryrun"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (h *PodCreateHandler) workloadSpreadMutatingPod(ctx context.Context, req admission.Request,
	pod *corev1.Pod) error {
	if len(req.AdmissionRequest.SubResource) > 0 ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return nil
	}

	oldPod := &corev1.Pod{}
	workloadSpreadHandler := wsutil.NewWorkloadSpreadHandler(h.Client)
	var dryRun bool

	switch req.AdmissionRequest.Operation {
	case admissionv1beta1.Create:
		options := &metav1.CreateOptions{}
		err := h.Decoder.DecodeRaw(req.Options, options)
		if err != nil {
			return err
		}
		// if dry run
		dryRun = dryrun.IsDryRun(options.DryRun)
		if dryRun {
			klog.V(5).Infof("Operation[%s] Pod (%s/%s) is a dry run", req.AdmissionRequest.Operation, pod.Namespace, pod.Name)
			return nil
		}

		return workloadSpreadHandler.HandlePodCreation(pod)
	case admissionv1beta1.Delete:
		if len(req.OldObject.Raw) == 0 {
			key := types.NamespacedName{
				Namespace: req.AdmissionRequest.Namespace,
				Name:      req.AdmissionRequest.Name,
			}
			if err := h.Client.Get(ctx, key, oldPod); err != nil {
				return nil
			}
		} else {
			if err := h.Decoder.DecodeRaw(req.OldObject, oldPod); err != nil {
				return err
			}
			if oldPod.Namespace == "" {
				oldPod.Namespace = req.Namespace
			}
		}

		options := &metav1.DeleteOptions{}
		err := h.Decoder.DecodeRaw(req.Options, options)
		if err != nil {
			return err
		}
		// if dry run
		dryRun = dryrun.IsDryRun(options.DryRun)
		if dryRun {
			klog.V(5).Infof("Operation[%s] Pod (%s/%s) is a dry run", req.AdmissionRequest.Operation, oldPod.Namespace, oldPod.Name)
			return nil
		}

		return workloadSpreadHandler.HandlePodDeletion(oldPod)
	case admissionv1beta1.Update:
		if err := h.Decoder.DecodeRaw(req.OldObject, oldPod); err != nil {
			return err
		}
		if oldPod.Namespace == "" {
			oldPod.Namespace = req.Namespace
		}

		options := &metav1.UpdateOptions{}
		err := h.Decoder.DecodeRaw(req.Options, options)
		if err != nil {
			return err
		}
		// if dry run
		dryRun = dryrun.IsDryRun(options.DryRun)
		if dryRun {
			klog.V(5).Infof("Operation[%s] Pod (%s/%s) is a dry run", req.AdmissionRequest.Operation, oldPod.Namespace, oldPod.Name)
			return nil
		}

		return workloadSpreadHandler.HandlePodUpdate(oldPod, pod)
	default:
		return nil
	}
}
