/*
Copyright 2020 The Kruise Authors.

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

	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/webhook/util/convertor"

	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metavalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog"
	corevalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ValidateDaemonSetName can be used to check whether the given daemon set name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateDaemonSetName = genericvalidation.NameIsDNSSubdomain

// DaemonSetCreateUpdateHandler handles DaemonSet
type DaemonSetCreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder *admission.Decoder
}

func (h *DaemonSetCreateUpdateHandler) validatingDaemonSetFn(ctx context.Context, obj *appsv1alpha1.DaemonSet) (bool, string, error) {
	allErrs := validateDaemonSet(obj)
	if len(allErrs) != 0 {
		return false, "", allErrs.ToAggregate()
	}
	return true, "allowed to be admitted", nil
}

func validateDaemonSet(ds *appsv1alpha1.DaemonSet) field.ErrorList {
	allErrs := genericvalidation.ValidateObjectMeta(&ds.ObjectMeta, true, ValidateDaemonSetName, field.NewPath("metadata"))
	allErrs = append(allErrs, validateDaemonSetSpec(&ds.Spec, field.NewPath("spec"))...)
	return allErrs
}

// ValidateDaemonSetSpec tests if required fields in the DaemonSetSpec are set.
func validateDaemonSetSpec(spec *appsv1alpha1.DaemonSetSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, metavalidation.ValidateLabelSelector(spec.Selector, fldPath.Child("selector"))...)

	selector, err := metav1.LabelSelectorAsSelector(spec.Selector)
	if err == nil && !selector.Matches(labels.Set(spec.Template.Labels)) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("template", "metadata", "labels"), spec.Template.Labels, "`selector` does not match template `labels`"))
	}
	if spec.Selector != nil && len(spec.Selector.MatchLabels)+len(spec.Selector.MatchExpressions) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "empty selector is invalid for daemonset"))
	}

	// Daemons typically run on more than one node, so mark Read-Write persistent disks as invalid.
	coreVolumes, err := convertor.ConvertCoreVolumes(spec.Template.Spec.Volumes)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.Template, fmt.Sprintf("Convert_v1_Volume_To_core_Volume failed: %v", err)))
		return allErrs
	}
	allErrs = append(allErrs, corevalidation.ValidateReadOnlyPersistentDisks(coreVolumes, fldPath.Child("template", "spec", "volumes"))...)
	allErrs = append(allErrs, corevalidation.ValidateNonnegativeField(int64(spec.MinReadySeconds), fldPath.Child("minReadySeconds"))...)

	allErrs = append(allErrs, validateDaemonSetUpdateStrategy(&spec.UpdateStrategy, fldPath.Child("updateStrategy"))...)
	if spec.RevisionHistoryLimit != nil {
		// zero is a valid RevisionHistoryLimit
		allErrs = append(allErrs, corevalidation.ValidateNonnegativeField(int64(*spec.RevisionHistoryLimit), fldPath.Child("revisionHistoryLimit"))...)
	}
	return allErrs
}

func validateDaemonSetUpdateStrategy(strategy *appsv1alpha1.DaemonSetUpdateStrategy, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	switch strategy.Type {
	case appsv1alpha1.OnDeleteDaemonSetStrategyType:
	case appsv1alpha1.RollingUpdateDaemonSetStrategyType:
		// Make sure RollingUpdate field isn't nil.
		if strategy.RollingUpdate == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("rollingUpdate"), ""))
			return allErrs
		}
		if strategy.RollingUpdate.Partition != nil {
			allErrs = append(allErrs,
				corevalidation.ValidateNonnegativeField(
					int64(*strategy.RollingUpdate.Partition),
					fldPath.Child("rollingUpdate").Child("partition"))...)
		}
		switch strategy.RollingUpdate.Type {
		case appsv1alpha1.StandardRollingUpdateType, appsv1alpha1.SurgingRollingUpdateType:
		default:
			validValues := []string{string(appsv1alpha1.StandardRollingUpdateType), string(appsv1alpha1.SurgingRollingUpdateType)}
			allErrs = append(allErrs, field.NotSupported(fldPath.Child("rollingUpdate").Child("rollingUpdateType"), strategy, validValues))
		}
		if strategy.RollingUpdate.MaxUnavailable != nil {
			allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(*strategy.RollingUpdate.MaxUnavailable, fldPath.Child("rollingUpdate").Child("maxUnavailable"))...)
			allErrs = append(allErrs, appsvalidation.IsNotMoreThan100Percent(*strategy.RollingUpdate.MaxUnavailable, fldPath.Child("rollingUpdate").Child("maxUnavailable"))...)
			if convertor.GetIntOrPercentValue(*strategy.RollingUpdate.MaxUnavailable) == 0 {
				// MaxUnavailable cannot be 0.
				allErrs = append(allErrs, field.Invalid(fldPath.Child("maxUnavailable"), strategy.RollingUpdate.MaxUnavailable, "cannot be 0"))
			}
		}
	default:
		validValues := []string{string(appsv1alpha1.RollingUpdateDaemonSetStrategyType), string(appsv1alpha1.OnDeleteDaemonSetStrategyType)}
		allErrs = append(allErrs, field.NotSupported(fldPath, strategy, validValues))
	}
	return allErrs
}

var _ admission.Handler = &DaemonSetCreateUpdateHandler{}

// Handle handles admission requests.
func (h *DaemonSetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	klog.V(4).Infof("get req %#v", req)
	obj := &appsv1alpha1.DaemonSet{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	allowed, reason, err := h.validatingDaemonSetFn(ctx, obj)
	if err != nil {
		klog.Warningf("ds %s/%s action %v fail:%s", obj.Namespace, obj.Name, req.AdmissionRequest.Operation, err.Error())
		return admission.Errored(http.StatusInternalServerError, err)
	}
	klog.V(4).Infof("ds %s/%s action: %v validate ok", obj.Namespace, obj.Name, req.AdmissionRequest.Operation)
	return admission.ValidationResponse(allowed, reason)
}

var _ admission.DecoderInjector = &DaemonSetCreateUpdateHandler{}

// InjectDecoder injects the decoder into the DaemonSetCreateUpdateHandler
func (h *DaemonSetCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
