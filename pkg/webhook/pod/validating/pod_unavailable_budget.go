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

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/util/dryrun"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/apis/policy"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:rbac:groups=policy.kruise.io,resources=podunavailablebudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy.kruise.io,resources=podunavailablebudgets/status,verbs=get;update;patch

var (
	// IgnoredNamespaces specifies the namespaces where Pods won't get injected
	IgnoredNamespaces = []string{"kube-system", "kube-public"}
)

// parameters:
// 1. allowed(bool) whether to allow this request
// 2. reason(string)
// 3. err(error)
func (p *PodCreateHandler) podUnavailableBudgetValidatingPod(ctx context.Context, req admission.Request) (bool, string, error) {
	// ignore kube-system, kube-public
	for _, namespace := range IgnoredNamespaces {
		if req.Namespace == namespace {
			return true, "", nil
		}
	}

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
		oldPod := &corev1.Pod{}
		if err = p.Decoder.Decode(
			admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{Object: req.AdmissionRequest.OldObject}},
			oldPod); err != nil {
			return false, "", err
		}
		// the change will not cause pod unavailability, then pass
		if !p.pubControl.IsPodUnavailableChanged(oldPod, newPod) {
			klog.V(6).Infof("validate pod(%s/%s) changed can not cause unavailability, then don't need check pub", newPod.Namespace, newPod.Name)
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
			klog.V(6).Infof("pod(%s/%s) AdmissionRequest operation(DELETE) subResource(%s), then admit", req.Namespace, req.Name, req.SubResource)
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
			klog.V(6).Infof("pod(%s/%s) AdmissionRequest operation(CREATE) Resource(%s) subResource(%s), then admit", req.Namespace, req.Name, req.Resource, req.SubResource)
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
		operation = policyv1alpha1.PubDeleteOperation
	}

	// Get the workload corresponding to the pod, if it has been deleted then it is not protected
	if ref := metav1.GetControllerOf(checkPod); ref != nil {
		workload, err := p.finders.GetScaleAndSelectorForRef(ref.APIVersion, ref.Kind, checkPod.Namespace, ref.Name, ref.UID)
		if err != nil {
			return false, "", err
		} else if workload == nil || !workload.Metadata.DeletionTimestamp.IsZero() {
			return true, "", nil
		}
	}
	return pubcontrol.PodUnavailableBudgetValidatePod(p.Client, p.pubControl, checkPod, operation, dryRun)
}
