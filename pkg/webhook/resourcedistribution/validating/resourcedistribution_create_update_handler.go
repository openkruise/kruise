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

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

// ResourceDistributionCreateUpdateHandler handles ResourceDistribution
type ResourceDistributionCreateUpdateHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &ResourceDistributionCreateUpdateHandler{}

// validateResourceDistributionSpec validate Spec when creating and updating
// 1. validate resource deserialization
// 2. validate resource itself
// 3. validate targets
func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionSpec(obj, oldObj *appsv1alpha1.ResourceDistribution, fldPath *field.Path) (allErrs field.ErrorList) {
	spec := &obj.Spec

	// decode resource
	resource, errs := DeserializeResource(&spec.Resource, fldPath)
	if resource == nil {
		allErrs = append(allErrs, errs...)
		return
	}

	// get old resource
	var oldResource UnifiedResource = nil
	if oldObj != nil {
		oldResource, errs = DeserializeResource(&oldObj.Spec.Resource, fldPath)
		allErrs = append(allErrs, errs...)
	}

	// validate resource
	errs = h.validateResourceDistributionSpecResource(resource, oldResource, fldPath.Child("resource"))
	allErrs = append(allErrs, errs...)

	// validate targets and conflict
	allErrs = append(allErrs, h.validateResourceDistributionSpecTargets(obj, resource, fldPath.Child("targets"))...)

	return
}

// validateResourceDistributionSpecResource validate Spec.Resource when creating and updating
// 1. validate resource metadata
// 2. validate updating conflict, GVK and name cannot be modified
func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionSpecResource(resource, oldResource UnifiedResource, fldPath *field.Path) (allErrs field.ErrorList) {
	// validate resource metadata
	allErrs = append(allErrs, apimachineryvalidation.ValidateObjectMeta(resource.GetObjectMeta(), false, apimachineryvalidation.NameIsDNSSubdomain, fldPath.Child("metadata"))...)

	// validate resource groupVersionKind and name when updating
	if len(allErrs) == 0 && oldResource != nil {
		if resource.GetName() != oldResource.GetName() || !apiequality.Semantic.DeepEqual(resource.GetGroupVersionKind(), oldResource.GetGroupVersionKind()) {
			allErrs = append(allErrs, field.Invalid(fldPath, nil, "resource apiVersion, kind, and name are immutable"))
		}
	}

	return
}

// validateResourceDistributionSpecTargets validate Spec.Targets
// 1. parse target namespaces
// 2. validate conflict between existing resources
func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionSpecTargets(obj *appsv1alpha1.ResourceDistribution, resource UnifiedResource, fldPath *field.Path) (allErrs field.ErrorList) {
	spec := &obj.Spec

	// select target namespaces
	targetNamespaces, errs := GetTargetNamespaces(h.Client, &spec.Targets)
	allErrs = append(allErrs, errs...)

	// detect conflicting namespaces
	// resource may conflict with existing resources in cluster
	conflictingNamespaces, errs := GetConflictingNamespaces(h.Client, resource, targetNamespaces, obj.Name, fldPath)
	allErrs = append(allErrs, errs...)
	if len(conflictingNamespaces) != 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, nil, fmt.Sprintf("resource with the same name already exists in namespaces %v", conflictingNamespaces)))
	}

	return
}

// validateResourceDistribution is an entrance to validate ResourceDistribution when creating and updating
// 1. validate ResourceDistribution metadata
// 2. validate Spec
func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistribution(obj, oldObj *appsv1alpha1.ResourceDistribution, fldPath *field.Path) (allErrs field.ErrorList) {
	// validate metadata
	allErrs = apimachineryvalidation.ValidateObjectMeta(&obj.ObjectMeta, false, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	// validate spec
	allErrs = append(allErrs, h.validateResourceDistributionSpec(obj, oldObj, fldPath.Child("spec"))...)
	return allErrs
}

// Handle handles admission requests.
func (h *ResourceDistributionCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.ResourceDistribution{}
	oldObj := &appsv1alpha1.ResourceDistribution{}

	switch req.AdmissionRequest.Operation {
	case admissionv1beta1.Create:
		if err := h.Decoder.Decode(req, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if allErrs := h.validateResourceDistribution(obj, nil, nil); len(allErrs) > 0 {
			klog.V(3).Infof("all errors when creating: %v", allErrs)
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	case admissionv1beta1.Update:
		if err := h.Decoder.Decode(req, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if allErrs := h.validateResourceDistribution(obj, oldObj, nil); len(allErrs) != 0 {
			klog.V(3).Infof("all errors when updating: %v", allErrs)
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
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
