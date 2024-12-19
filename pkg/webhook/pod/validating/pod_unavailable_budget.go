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
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
)

// parameters:
// 1. allowed(bool) whether to allow this request
// 2. reason(string)
// 3. err(error)
func (p *PodCreateHandler) podUnavailableBudgetValidatingPod(ctx context.Context, req admission.Request) (bool, string, error) {
	var checkPod *corev1.Pod
	var dryRun bool
	var operation policyv1alpha1.PubOperation
	switch req.AdmissionRequest.Operation {
	// filter out invalid Update operation, we only validate update Pod.MetaData, Pod.Spec
	case admissionv1.Update:
		newPod := &corev1.Pod{}
		//decode new pod
		err := p.Decoder.Decode(req, newPod)
		if err != nil {
			return false, "", err
		}
		if newPod.Annotations[pubcontrol.PodRelatedPubAnnotation] == "" {
			return true, "", nil
		}
		oldPod := &corev1.Pod{}
		if err = p.Decoder.Decode(
			admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{Object: req.AdmissionRequest.OldObject}},
			oldPod); err != nil {
			return false, "", err
		}
		// the change will not cause pod unavailability, then pass
		if !pubcontrol.PubControl.IsPodUnavailableChanged(oldPod, newPod) {
			klog.V(6).InfoS("validate pod changed can not cause unavailability, then don't need check pub", "namespace", newPod.Namespace, "name", newPod.Name)
			return true, "", nil
		}
		checkPod = oldPod
		options := &metav1.UpdateOptions{}
		err = p.Decoder.DecodeRaw(req.Options, options)
		if err != nil {
			return false, "", err
		}
		// if dry run
		dryRun = dryrun.IsDryRun(options.DryRun)
		operation = policyv1alpha1.PubUpdateOperation

	// filter out invalid Delete operation, only validate delete pods resources
	case admissionv1.Delete:
		if req.AdmissionRequest.SubResource != "" {
			klog.V(6).InfoS("pod AdmissionRequest operation(DELETE) subResource, then admit", "namespace", req.Namespace, "name", req.Name, "subResource", req.SubResource)
			return true, "", nil
		}
		checkPod = &corev1.Pod{}
		if err := p.Decoder.DecodeRaw(req.OldObject, checkPod); err != nil {
			return false, "", err
		}
		deletion := &metav1.DeleteOptions{}
		err := p.Decoder.DecodeRaw(req.Options, deletion)
		if err != nil {
			return false, "", err
		}
		// if dry run
		dryRun = dryrun.IsDryRun(deletion.DryRun)
		operation = policyv1alpha1.PubDeleteOperation

	// filter out invalid Create operation, only validate create pod eviction subresource
	case admissionv1.Create:
		// ignore create operation other than subresource eviction
		if req.AdmissionRequest.SubResource != "eviction" {
			klog.V(6).InfoS("pod AdmissionRequest operation(CREATE) Resource and subResource, then admit", "namespace", req.Namespace, "name", req.Name, "subResource", req.SubResource, "resource", req.Resource)
			return true, "", nil
		}
		eviction := &policy.Eviction{}
		//decode eviction
		err := p.Decoder.Decode(req, eviction)
		if err != nil {
			return false, "", err
		}
		// if dry run
		if eviction.DeleteOptions != nil {
			dryRun = dryrun.IsDryRun(eviction.DeleteOptions.DryRun)
		}
		checkPod = &corev1.Pod{}
		key := types.NamespacedName{
			Namespace: req.AdmissionRequest.Namespace,
			Name:      req.AdmissionRequest.Name,
		}
		if err = p.Client.Get(ctx, key, checkPod); err != nil {
			return false, "", err
		}
		operation = policyv1alpha1.PubEvictOperation
	}

	if checkPod.Annotations[pubcontrol.PodRelatedPubAnnotation] == "" {
		return true, "", nil
	}
	return pubcontrol.PodUnavailableBudgetValidatePod(checkPod, operation, req.UserInfo.Username, dryRun)
}
