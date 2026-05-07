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

	switch req.AdmissionRequest.Resource.Version {
	case appsv1beta1.GroupVersion.Version:
		obj := &appsv1beta1.ResourceDistribution{}
		if err := h.Decoder.Decode(req, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		var oldObj *appsv1beta1.ResourceDistribution
		if req.AdmissionRequest.Operation == admissionv1.Update {
			oldObj = &appsv1beta1.ResourceDistribution{}
			if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
		}
		if allErrs := h.validateResourceDistributionV1beta1(obj, oldObj); len(allErrs) != 0 {
			klog.V(3).InfoS("all errors of validation", "errors", fmt.Sprintf("%v", allErrs))
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
		return admission.ValidationResponse(true, "")
	case appsv1alpha1.GroupVersion.Version:
		obj := &appsv1alpha1.ResourceDistribution{}
		if err := h.Decoder.Decode(req, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		var oldObj *appsv1alpha1.ResourceDistribution
		if req.AdmissionRequest.Operation == admissionv1.Update {
			oldObj = &appsv1alpha1.ResourceDistribution{}
			if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
		}
		if allErrs := h.validateResourceDistribution(obj, oldObj); len(allErrs) != 0 {
			klog.V(3).InfoS("all errors of validation", "errors", fmt.Sprintf("%v", allErrs))
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
		return admission.ValidationResponse(true, "")
	default:
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unsupported version: %s", req.AdmissionRequest.Resource.Version))
	}
}

func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistribution(obj, oldObj *appsv1alpha1.ResourceDistribution) (allErrs field.ErrorList) {
	allErrs = apimachineryvalidation.ValidateObjectMeta(&obj.ObjectMeta, false, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	return append(allErrs, h.validateResourceDistributionSpec(specResourceView{resource: &obj.Spec.Resource, targets: targetsViewFromV1alpha1(&obj.Spec.Targets)}, oldSpecResourceView(oldObj), field.NewPath("spec"), false)...)
}

func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionV1beta1(obj, oldObj *appsv1beta1.ResourceDistribution) (allErrs field.ErrorList) {
	allErrs = apimachineryvalidation.ValidateObjectMeta(&obj.ObjectMeta, false, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	return append(allErrs, h.validateResourceDistributionSpec(specResourceView{resource: &obj.Spec.Resource, targets: targetsViewFromV1beta1(&obj.Spec.Targets)}, oldSpecResourceViewV1beta1(oldObj), field.NewPath("spec"), true)...)
}

type specResourceView struct {
	resource *runtime.RawExtension
	targets  resourceDistributionTargetsView
}

type resourceDistributionTargetsView struct {
	allNamespaces          bool
	includedNamespaces     []string
	excludedNamespaces     []string
	namespaceLabelSelector metav1.LabelSelector
}

func targetsViewFromV1alpha1(targets *appsv1alpha1.ResourceDistributionTargets) resourceDistributionTargetsView {
	if targets == nil {
		return resourceDistributionTargetsView{}
	}
	return resourceDistributionTargetsView{
		allNamespaces:          targets.AllNamespaces,
		includedNamespaces:     namespaceNamesV1alpha1(targets.IncludedNamespaces.List),
		excludedNamespaces:     namespaceNamesV1alpha1(targets.ExcludedNamespaces.List),
		namespaceLabelSelector: targets.NamespaceLabelSelector,
	}
}

func targetsViewFromV1beta1(targets *appsv1beta1.ResourceDistributionTargets) resourceDistributionTargetsView {
	if targets == nil {
		return resourceDistributionTargetsView{}
	}
	return resourceDistributionTargetsView{
		allNamespaces:          targets.AllNamespaces,
		includedNamespaces:     namespaceNamesV1beta1(targets.IncludedNamespaces.List),
		excludedNamespaces:     namespaceNamesV1beta1(targets.ExcludedNamespaces.List),
		namespaceLabelSelector: targets.NamespaceLabelSelector,
	}
}

func namespaceNamesV1alpha1(namespaces []appsv1alpha1.ResourceDistributionNamespace) []string {
	values := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		values = append(values, namespace.Name)
	}
	return values
}

func namespaceNamesV1beta1(namespaces []appsv1beta1.ResourceDistributionNamespace) []string {
	values := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		values = append(values, namespace.Name)
	}
	return values
}

func oldSpecResourceView(oldObj *appsv1alpha1.ResourceDistribution) *specResourceView {
	if oldObj == nil {
		return nil
	}
	return &specResourceView{resource: &oldObj.Spec.Resource, targets: targetsViewFromV1alpha1(&oldObj.Spec.Targets)}
}

func oldSpecResourceViewV1beta1(oldObj *appsv1beta1.ResourceDistribution) *specResourceView {
	if oldObj == nil {
		return nil
	}
	return &specResourceView{resource: &oldObj.Spec.Resource, targets: targetsViewFromV1beta1(&oldObj.Spec.Targets)}
}

func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionSpec(view specResourceView, oldView *specResourceView, fldPath *field.Path, strict bool) (allErrs field.ErrorList) {
	resource, errs := DeserializeResource(view.resource, fldPath.Child("resource"))
	allErrs = append(allErrs, errs...)
	if resource == nil {
		return allErrs
	}

	var oldResource runtime.Object
	if oldView != nil {
		oldResource, errs = DeserializeResource(oldView.resource, fldPath.Child("resource"))
		allErrs = append(allErrs, errs...)
	}

	allErrs = append(allErrs, h.validateResourceDistributionSpecResource(resource, oldResource, fldPath.Child("resource"), strict)...)
	allErrs = append(allErrs, validateResourceDistributionTargets(view.targets, fldPath.Child("targets"), strict)...)
	return allErrs
}

func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionSpecResource(resource, oldResource runtime.Object, fldPath *field.Path, strict bool) (allErrs field.ErrorList) {
	if !isSupportedGK(resource) {
		return append(allErrs, field.Invalid(fldPath, resource.GetObjectKind().GroupVersionKind().GroupKind(), fmt.Sprintf("unknown or unsupported resource GroupKind, only support %v", supportedGKList)))
	}
	if oldResource != nil && !haveSameGVKAndName(resource, oldResource) {
		return append(allErrs, field.Invalid(fldPath, nil, "resource apiVersion, kind, and name are immutable"))
	}

	mice := resource.DeepCopyObject().(client.Object)
	if strict {
		if resourceNamespace := ConvertToUnstructured(mice).GetNamespace(); resourceNamespace != "" {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("metadata").Child("namespace"), "spec.resource.metadata.namespace is not allowed; target namespaces come from spec.targets"))
			return allErrs
		}
	}
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

func validateResourceDistributionTargets(targets resourceDistributionTargetsView, fldPath *field.Path, strict bool) (allErrs field.ErrorList) {
	conflicted := make([]string, 0)
	includedNS := sets.NewString()
	for _, namespace := range targets.includedNamespaces {
		includedNS.Insert(namespace)
		for _, msg := range coreval.ValidateNamespaceName(namespace, false) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("includedNamespaces"), namespace, msg))
		}
	}
	for _, namespace := range targets.excludedNamespaces {
		for _, msg := range coreval.ValidateNamespaceName(namespace, false) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("excludedNamespaces"), namespace, msg))
		}
		if includedNS.Has(namespace) {
			conflicted = append(conflicted, namespace)
		}
	}
	if len(conflicted) != 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, targets, fmt.Sprintf("ambiguous targets because namespace %v is in both IncludedNamespaces.List and ExcludedNamespaces.List", conflicted)))
	}

	if _, err := metav1.LabelSelectorAsSelector(&targets.namespaceLabelSelector); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("namespaceLabelSelector"), targets.namespaceLabelSelector, fmt.Sprintf("labelSelectorAsSelector error: %v", err)))
	}

	if strict && !targets.allNamespaces && len(targets.includedNamespaces) == 0 &&
		len(targets.namespaceLabelSelector.MatchLabels) == 0 && len(targets.namespaceLabelSelector.MatchExpressions) == 0 {
		allErrs = append(allErrs, field.Required(fldPath, "at least one target selector must be configured"))
	}
	return allErrs
}
