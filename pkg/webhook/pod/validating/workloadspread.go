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

package validating

import (
	"context"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/util/dryrun"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/apis/policy"

	wsutil "github.com/openkruise/kruise/pkg/util/workloadspread"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// parameters:
// 1. allowed(bool) whether to allow this request
// 2. reason(string)
// 3. err(error)
func (p *PodCreateHandler) workloadSpreadValidatingPod(ctx context.Context, req admission.Request) (bool, string, error) {
	pod := &corev1.Pod{}
	var dryRun bool
	var err error
	workloadSpreadHandler := wsutil.NewWorkloadSpreadHandler(p.Client)

	klog.V(6).InfoS("workloadSpread validate Operation", "operation", req.Operation, "namespace", req.Namespace, "name", req.Name)

	switch req.AdmissionRequest.Operation {
	case admissionv1.Delete:
		if req.AdmissionRequest.SubResource != "" {
			klog.V(6).InfoS("Pod AdmissionRequest operation(DELETE) subResource, then admit", "namespace", req.Namespace, "name", req.Name, "subResource", req.SubResource)
			return true, "", nil
		}

		err = p.Decoder.DecodeRaw(req.OldObject, pod)
		if err != nil {
			return false, "", err
		}

		// check dry run
		deletion := &metav1.DeleteOptions{}
		err = p.Decoder.DecodeRaw(req.Options, deletion)
		if err != nil {
			return false, "", err
		}
		dryRun = dryrun.IsDryRun(deletion.DryRun)
		if dryRun {
			klog.V(5).InfoS("Operation is a dry run, then admit", "operation", req.AdmissionRequest.Operation, "namespace", pod.Namespace, "name", pod.Name)
			return true, "", err
		}

		err = workloadSpreadHandler.HandlePodDeletion(pod, wsutil.DeleteOperation)
		if err != nil {
			return false, "", err
		}
	case admissionv1.Create:
		// ignore create operation other than subresource eviction
		if req.AdmissionRequest.SubResource != "eviction" {
			klog.V(6).InfoS("Pod AdmissionRequest operation(CREATE) Resource and subResource, then admit", "namespace", req.Namespace, "name", req.Name, "resource", req.Resource, "subResource", req.SubResource)
			return true, "", nil
		}

		eviction := &policy.Eviction{}
		err = p.Decoder.Decode(req, eviction)
		if err != nil {
			return false, "", err
		}
		// check dry run
		if eviction.DeleteOptions != nil {
			dryRun = dryrun.IsDryRun(eviction.DeleteOptions.DryRun)
			if dryRun {
				klog.V(5).InfoS("Operation[Eviction] is a dry run, then admit", "namespace", req.AdmissionRequest.Namespace, "name", req.AdmissionRequest.Name)
				return true, "", nil
			}
		}

		key := types.NamespacedName{
			Namespace: req.AdmissionRequest.Namespace,
			Name:      req.AdmissionRequest.Name,
		}
		err = p.Client.Get(ctx, key, pod)
		if err != nil {
			return false, "", err
		}

		err = workloadSpreadHandler.HandlePodDeletion(pod, wsutil.EvictionOperation)
		if err != nil {
			return false, "", err
		}
	}

	return true, "", nil
}
