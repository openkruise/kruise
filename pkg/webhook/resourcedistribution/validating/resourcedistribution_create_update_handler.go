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
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	coreval "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
)

// ResourceDistributionCreateUpdateHandler handles ResourceDistribution
type ResourceDistributionCreateUpdateHandler struct {
	Client  client.Client
	Decoder admission.Decoder
}

var _ admission.Handler = &ResourceDistributionCreateUpdateHandler{}

func (h *ResourceDistributionCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if !utilfeature.DefaultFeatureGate.Enabled(features.ResourceDistributionGate) {
		return admission.Errored(http.StatusForbidden, fmt.Errorf("feature-gate %s is not enabled", features.ResourceDistributionGate))
	}

	obj, err := h.decodeObject(req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	var oldObj *appsv1beta1.ResourceDistribution
	if req.AdmissionRequest.Operation == admissionv1.Update {
		oldObj, err = h.decodeOldObject(req)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	}

	if allErrs := h.validateResourceDistribution(obj, oldObj); len(allErrs) != 0 {
		klog.V(3).InfoS("all errors of validation", "errors", fmt.Sprintf("%v", allErrs))
		return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
	}
	return admission.ValidationResponse(true, "")
}

// decodeObject decodes the incoming object as v1beta1, converting from v1alpha1 if needed.
func (h *ResourceDistributionCreateUpdateHandler) decodeObject(req admission.Request) (*appsv1beta1.ResourceDistribution, error) {
	switch req.AdmissionRequest.Resource.Version {
	case appsv1beta1.GroupVersion.Version:
		obj := &appsv1beta1.ResourceDistribution{}
		if err := h.Decoder.Decode(req, obj); err != nil {
			return nil, err
		}
		return obj, nil
	case appsv1alpha1.GroupVersion.Version:
		alpha := &appsv1alpha1.ResourceDistribution{}
		if err := h.Decoder.Decode(req, alpha); err != nil {
			return nil, err
		}
		beta := &appsv1beta1.ResourceDistribution{}
		if err := alpha.ConvertTo(beta); err != nil {
			return nil, fmt.Errorf("failed to convert v1alpha1->v1beta1: %v", err)
		}
		return beta, nil
	default:
		return nil, fmt.Errorf("unsupported version: %s", req.AdmissionRequest.Resource.Version)
	}
}

// decodeOldObject decodes the old object (for updates) as v1beta1, converting from v1alpha1 if needed.
func (h *ResourceDistributionCreateUpdateHandler) decodeOldObject(req admission.Request) (*appsv1beta1.ResourceDistribution, error) {
	switch req.AdmissionRequest.Resource.Version {
	case appsv1beta1.GroupVersion.Version:
		obj := &appsv1beta1.ResourceDistribution{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, obj); err != nil {
			return nil, err
		}
		return obj, nil
	case appsv1alpha1.GroupVersion.Version:
		alpha := &appsv1alpha1.ResourceDistribution{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, alpha); err != nil {
			return nil, err
		}
		beta := &appsv1beta1.ResourceDistribution{}
		if err := alpha.ConvertTo(beta); err != nil {
			return nil, fmt.Errorf("failed to convert v1alpha1->v1beta1: %v", err)
		}
		return beta, nil
	default:
		return nil, fmt.Errorf("unsupported version: %s", req.AdmissionRequest.Resource.Version)
	}
}

func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistribution(obj, oldObj *appsv1beta1.ResourceDistribution) (allErrs field.ErrorList) {
	allErrs = apimachineryvalidation.ValidateObjectMeta(&obj.ObjectMeta, false, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	allErrs = append(allErrs, h.validateResourceDistributionSpec(obj, oldObj, field.NewPath("spec"))...)
	return allErrs
}

func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionSpec(obj, oldObj *appsv1beta1.ResourceDistribution, fldPath *field.Path) (allErrs field.ErrorList) {
	resource, errs := DeserializeResource(&obj.Spec.Resource, fldPath.Child("resource"))
	allErrs = append(allErrs, errs...)
	if resource == nil {
		return allErrs
	}

	var oldResource runtime.Object
	if oldObj != nil {
		oldResource, errs = DeserializeResource(&oldObj.Spec.Resource, fldPath.Child("resource"))
		allErrs = append(allErrs, errs...)
	}

	allErrs = append(allErrs, h.validateResourceDistributionSpecResource(resource, oldResource, fldPath.Child("resource"))...)
	allErrs = append(allErrs, validateResourceDistributionTargets(obj.Spec.Targets, fldPath.Child("targets"))...)
	return allErrs
}

func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionSpecResource(resource, oldResource runtime.Object, fldPath *field.Path) (allErrs field.ErrorList) {
	if !isSupportedGK(resource) {
		return append(allErrs, field.Invalid(fldPath, resource.GetObjectKind().GroupVersionKind().GroupKind(), fmt.Sprintf("unknown or unsupported resource GroupKind, only support %v", supportedGKList)))
	}
	if oldResource != nil && !haveSameGVKAndName(resource, oldResource) {
		return append(allErrs, field.Invalid(fldPath, nil, "resource apiVersion, kind, and name are immutable"))
	}

	mice := resource.DeepCopyObject().(client.Object)
	// metadata.namespace on spec.resource is silently ignored; the controller always
	// overrides it with the target namespace when distributing.
	ConvertToUnstructured(mice).SetNamespace(webhookutil.GetNamespace())
	err := h.Client.Create(context.TODO(), mice, &client.CreateOptions{DryRun: []string{metav1.DryRunAll}})
	if err == nil || errors.IsAlreadyExists(err) {
		return allErrs
	}
	if errors.IsInvalid(err) || errors.IsBadRequest(err) {
		return append(allErrs, field.Invalid(fldPath, nil, fmt.Sprintf("spec.resource is invalid: %v", err)))
	}
	if errors.IsForbidden(err) {
		return append(allErrs, field.Forbidden(fldPath, fmt.Sprintf("spec.resource is forbidden: %v", err)))
	}
	return append(allErrs, field.InternalError(fldPath, fmt.Errorf("failed to dry-run validate spec.resource: %v", err)))
}

func validateResourceDistributionTargets(targets appsv1beta1.ResourceDistributionTargets, fldPath *field.Path) (allErrs field.ErrorList) {
	conflicted := make([]string, 0)
	includedNS := sets.NewString()
	for _, namespace := range targets.IncludedNamespaces.List {
		includedNS.Insert(namespace.Name)
		for _, msg := range coreval.ValidateNamespaceName(namespace.Name, false) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("includedNamespaces"), namespace.Name, msg))
		}
	}
	for _, namespace := range targets.ExcludedNamespaces.List {
		for _, msg := range coreval.ValidateNamespaceName(namespace.Name, false) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("excludedNamespaces"), namespace.Name, msg))
		}
		if includedNS.Has(namespace.Name) {
			conflicted = append(conflicted, namespace.Name)
		}
	}
	if len(conflicted) != 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, targets, fmt.Sprintf("ambiguous targets because namespace %v is in both IncludedNamespaces.List and ExcludedNamespaces.List", conflicted)))
	}

	if _, err := metav1.LabelSelectorAsSelector(&targets.NamespaceSelector); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("namespaceSelector"), targets.NamespaceSelector, fmt.Sprintf("labelSelectorAsSelector error: %v", err)))
	}

	return allErrs
}
