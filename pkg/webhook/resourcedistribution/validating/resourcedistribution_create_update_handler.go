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
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"net/http"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
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

func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionSpec(obj, oldObj *appsv1alpha1.ResourceDistribution, fldPath *field.Path) (allErrs field.ErrorList) {
	spec := &obj.Spec

	// validate WritePolicy
	switch spec.WritePolicy {
	case "",
		appsv1alpha1.RESOURCEDISTRIBUTION_STRICT_WRITEPOLICY,
		appsv1alpha1.RESOURCEDISTRIBUTION_IGNORE_WRITEPOLICY,
		appsv1alpha1.RESOURCEDISTRIBUTION_OVERWRITE_WRITEPOLICY:
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("writePolicy"), spec.WritePolicy, fmt.Sprintf("Unknown or unsupported WritePolicy type %s.", spec.WritePolicy)))
	}

	// decode resource
	resource, errs := DecodeResource(&spec.Resource, fldPath)
	allErrs = append(allErrs, errs...)

	if resource != nil {
		// get old resource
		var oldResource UnifiedResource = nil
		if oldObj != nil {
			oldResource, errs = DecodeResource(&oldObj.Spec.Resource, fldPath)
			allErrs = append(allErrs, errs...)
		}

		// validate resource
		errs = h.validateResourceDistributionSpecResource(resource, oldResource, fldPath.Child("resource"))
		allErrs = append(allErrs, errs...)

		// validate targets and conflict
		allErrs = append(allErrs, h.validateResourceDistributionSpecTargets(obj, resource, fldPath.Child("targets"))...)
	}

	return
}

func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionSpecResource(resource, oldResource UnifiedResource, fldPath *field.Path) (allErrs field.ErrorList) {
	// validate resource metadata
	allErrs = append(allErrs, apimachineryvalidation.ValidateObjectMeta(resource.GetObjectMeta(), false, apimachineryvalidation.NameIsDNSSubdomain, fldPath.Child("metadata"))...)

	// validate resource groupVersionKind and name when updating
	if oldResource != nil {
		if resource.GetName() != oldResource.GetName() || !apiequality.Semantic.DeepEqual(resource.GetGroupVersionKind(), oldResource.GetGroupVersionKind()) {
			allErrs = append(allErrs, field.Invalid(fldPath, nil, "resource apiVersion, kind and name are immutable"))
		}
	}

	return
}

func (h *ResourceDistributionCreateUpdateHandler) validateResourceDistributionSpecTargets(obj *appsv1alpha1.ResourceDistribution, resource UnifiedResource, fldPath *field.Path) (allErrs field.ErrorList) {
	spec := &obj.Spec

	//validate whether WorkloadLabelSelector.APIVersion and Kind is empty
	if (spec.Targets.WorkloadLabelSelector.APIVersion == "" || spec.Targets.WorkloadLabelSelector.Kind == "") &&
		(len(spec.Targets.WorkloadLabelSelector.MatchLabels) != 0 || len(spec.Targets.WorkloadLabelSelector.MatchExpressions) != 0) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("Targets").Child("WorkloadLabelSelector"),
			spec.Targets.WorkloadLabelSelector, "must specify the apiVersion and kind of workload in workloadLabelSelector"))
	}

	//select target namespaces
	targetNamespaces, errs1 := GetTargetNamespaces(h.Client, &spec.Targets, fldPath.Child("targets"))
	allErrs = append(allErrs, errs1...)

	//detect conflicting namespaces
	conflictingNamespaces, errs2 := GetConflictingNamespaces(h.Client, resource, targetNamespaces, obj.Name, fldPath)
	allErrs = append(allErrs, errs2...)

	//resource may conflict with existing resources in cluster if writePolicy is STRICT
	if len(conflictingNamespaces) != 0 && (spec.WritePolicy == "" || spec.WritePolicy == appsv1alpha1.RESOURCEDISTRIBUTION_STRICT_WRITEPOLICY) {
		allErrs = append(allErrs, field.Invalid(fldPath, spec, fmt.Sprintf("writePolicy is 'strict', however conflict happens in namespaces %v", conflictingNamespaces)))
	}

	return
}

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
