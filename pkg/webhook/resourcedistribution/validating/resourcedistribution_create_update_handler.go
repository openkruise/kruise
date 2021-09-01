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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog"
	coreval "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ResourceDistributionCreateUpdateHandler handles ResourceDistribution
type ResourceDistributionCreateUpdateHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &ResourceDistributionCreateUpdateHandler{}

// validateResourceDistributionSpec validate Spec when creating and updating
// (1). validate resource itself
// (2). validate targets
func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionSpec(obj, oldObj *appsv1alpha1.ResourceDistribution, fldPath *field.Path) (allErrs field.ErrorList) {
	spec := &obj.Spec
	// deserialize resource from runtime.rawExtension
	resource, errs := DeserializeResource(&spec.Resource, fldPath)
	allErrs = append(allErrs, errs...)
	if resource == nil {
		return
	}
	// deserialize old resource if need
	var oldResource runtime.Object = nil
	if oldObj != nil {
		oldResource, errs = DeserializeResource(&oldObj.Spec.Resource, fldPath)
		allErrs = append(allErrs, errs...)
	}
	// 1. validate resource
	allErrs = append(allErrs, h.validateResourceDistributionResource(resource, oldResource, fldPath.Child("resource"))...)
	// 2. validate targets
	allErrs = append(allErrs, h.validateResourceDistributionSpecTargets(&obj.Spec.Targets, fldPath.Child("targets"))...)
	return
}

// validateResourceDistributionResource validate Spec.Resource when creating and updating
// (1). check whether type of the resource is supported
// (2). detect updating conflict, i.e., GK and name cannot be modified
// (3). dry run to check whether resource can be created
func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionResource(resource, oldResource runtime.Object, fldPath *field.Path) (allErrs field.ErrorList) {
	// 1. check whether the GK of the resource is in supportedGKList
	if !isSupportedGK(resource) {
		return append(allErrs, field.Invalid(fldPath, resource, fmt.Sprintf("unknown or unsupported resource GroupVersionKind, only support %v", supportedGKList)))
	}
	// 2. validate resource group, kind and name when updating
	if oldResource != nil && !haveSameGKAndName(resource, oldResource) {
		return append(allErrs, field.Invalid(fldPath, nil, "resource apiVersion, kind, and name are immutable"))
	}
	// 3. dry run to check resource
	mice := resource.DeepCopyObject()
	ConvertToUnstructured(mice).SetNamespace(DefaultNamespace)
	if err := h.Client.Create(context.TODO(), mice, &client.CreateOptions{DryRun: []string{metav1.DryRunAll}}); err != nil {
		return append(allErrs, field.InternalError(fldPath, fmt.Errorf("dry-run to validate resource failed, err: %v", err)))
	}
	return
}

// validateResourceDistributionSpecTargets validate Spec.Targets
// (1). validate target namespace names
// (2). validate conflict between existing resources
func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionSpecTargets(targets *appsv1alpha1.ResourceDistributionTargets, fldPath *field.Path) (allErrs field.ErrorList) {
	// 1. validate namespace of IncludedNamespaces.List and ExcludedNamespaces.List
	forbidden := make([]string, 0)
	conflicted := make([]string, 0)
	included := make(map[string]struct{})
	for _, namespace := range targets.IncludedNamespaces.List {
		included[namespace.Name] = struct{}{}
		// validate namespace name
		for _, msg := range coreval.ValidateNamespaceName(namespace.Name, false) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("includedNamespaces"), targets.IncludedNamespaces, msg))
		}
		// validate whether namespace is forbidden
		if isForbiddenNamespace(namespace.Name) {
			forbidden = append(forbidden, namespace.Name)
		}
	}
	for _, namespace := range targets.ExcludedNamespaces.List {
		// validate namespace name
		for _, msg := range coreval.ValidateNamespaceName(namespace.Name, false) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("excludedNamespaces"), targets.ExcludedNamespaces, msg))
		}
		// validate conflict between IncludedNamespaces and ExcludedNamespaces
		if _, ok := included[namespace.Name]; ok {
			conflicted = append(conflicted, namespace.Name)
		}
	}
	if len(conflicted) != 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, targets, fmt.Sprintf("ambiguous targets because namespace %v is in both IncludedNamespaces.List and ExcludedNamesapces.List", conflicted)))
	}
	if len(forbidden) != 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("includedNamespaces"), targets.IncludedNamespaces, fmt.Sprintf("cannot distribute rsource to forbidden namespaces %v", ForbiddenNamespaces)))
	}

	// 2. validate targets.NamespaceLabelSelector
	if _, err := metav1.LabelSelectorAsSelector(&targets.NamespaceLabelSelector); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("namespaceLabelSelector"), targets.IncludedNamespaces, fmt.Sprintf("labelSelectorAsSelector error: %v", err)))
	}

	return
}

// validateResourceDistribution is an entrance to validate ResourceDistribution when creating and updating
// (1). validate ResourceDistribution ObjectMeta
// (2). validate ResourceDistribution Spec
func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistribution(obj, oldObj *appsv1alpha1.ResourceDistribution) (allErrs field.ErrorList) {
	// 1. validate metadata
	allErrs = apimachineryvalidation.ValidateObjectMeta(&obj.ObjectMeta, false, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	// 2. validate spec
	return append(allErrs, h.validateResourceDistributionSpec(obj, oldObj, field.NewPath("spec"))...)
}

// Handle handles admission requests.
func (h *ResourceDistributionCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.ResourceDistribution{}
	if err := h.Decoder.Decode(req, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var oldObj *appsv1alpha1.ResourceDistribution
	if req.AdmissionRequest.Operation == admissionv1beta1.Update {
		oldObj = &appsv1alpha1.ResourceDistribution{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	}
	if allErrs := h.validateResourceDistribution(obj, oldObj); len(allErrs) != 0 {
		klog.V(3).Infof("all errors of validation: %v", allErrs)
		return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
	}
	return admission.ValidationResponse(true, "")
}

var _ inject.Client = &ResourceDistributionCreateUpdateHandler{}

// InjectClient injects the client into the ResourceDistributionCreateUpdateHandler
func (h *ResourceDistributionCreateUpdateHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &ResourceDistributionCreateUpdateHandler{}

// InjectDecoder injects the decoder into the ResourceDistributionCreateUpdateHandler
func (h *ResourceDistributionCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
