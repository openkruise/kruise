/*
Copyright 2019 The Kruise Authors.

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
	"reflect"
	"regexp"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	v1 "k8s.io/api/core/v1"
	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metavalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	corevalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	sidecarSetNameMaxLen = 63
)

var (
	validateSidecarSetNameMsg   = "sidecarset name must consist of alphanumeric characters or '-'"
	validateSidecarSetNameRegex = regexp.MustCompile(validSidecarSetNameFmt)
	validSidecarSetNameFmt      = `^[a-zA-Z0-9\-]+$`
)

// SidecarSetCreateUpdateHandler handles SidecarSet
type SidecarSetCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

func (h *SidecarSetCreateUpdateHandler) validatingSidecarSetFn(ctx context.Context, obj *appsv1alpha1.SidecarSet, older *appsv1alpha1.SidecarSet) (bool, string, error) {
	allErrs := h.validateSidecarSet(obj, older)
	if len(allErrs) != 0 {
		return false, "", allErrs.ToAggregate()
	}
	return true, "allowed to be admitted", nil
}

func (h *SidecarSetCreateUpdateHandler) validateSidecarSet(obj *appsv1alpha1.SidecarSet, older *appsv1alpha1.SidecarSet) field.ErrorList {
	// validating ObjectMeta
	allErrs := genericvalidation.ValidateObjectMeta(&obj.ObjectMeta, false, validateSidecarSetName, field.NewPath("metadata"))
	// validating spec
	allErrs = append(allErrs, validateSidecarSetSpec(obj, field.NewPath("spec"))...)
	// when operation is update, older isn't empty, and validating whether old and new containers conflict
	if older != nil {
		allErrs = append(allErrs, validateSidecarContainerConflict(obj.Spec.Containers, older.Spec.Containers, field.NewPath("spec.containers"))...)
	}
	// iterate across all containers in other sidecarsets to avoid duplication of name
	sidecarSets := &appsv1alpha1.SidecarSetList{}
	if err := h.Client.List(context.TODO(), sidecarSets, &client.ListOptions{}); err != nil {
		allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("query other sidecarsets failed, err: %v", err)))
	}
	allErrs = append(allErrs, validateSidecarConflict(sidecarSets, obj, field.NewPath("spec"))...)
	return allErrs
}

func validateSidecarSetName(name string, prefix bool) (allErrs []string) {
	if !validateSidecarSetNameRegex.MatchString(name) {
		allErrs = append(allErrs, validationutil.RegexError(validateSidecarSetNameMsg, validSidecarSetNameFmt, "example-com"))
	}
	if len(name) > sidecarSetNameMaxLen {
		allErrs = append(allErrs, validationutil.MaxLenError(sidecarSetNameMaxLen))
	}
	return allErrs
}

func validateSidecarSetSpec(obj *appsv1alpha1.SidecarSet, fldPath *field.Path) field.ErrorList {
	spec := &obj.Spec
	allErrs := field.ErrorList{}

	//validate spec selector
	if spec.Selector == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector"), "no selector defined for SidecarSet"))
	} else {
		allErrs = append(allErrs, validateSelector(spec.Selector, fldPath.Child("selector"))...)
	}
	//validating SidecarSetUpdateStrategy
	allErrs = append(allErrs, validateSidecarSetUpdateStrategy(&spec.UpdateStrategy, fldPath.Child("strategy"))...)
	//validating volumes
	vols, vErrs := getCoreVolumes(spec.Volumes, fldPath.Child("volumes"))
	allErrs = append(allErrs, vErrs...)
	//validating sidecar container
	// if don't have any initContainers, containers
	if len(spec.InitContainers) == 0 && len(spec.Containers) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Root(), "no initContainer or container defined for SidecarSet"))
	} else {
		allErrs = append(allErrs, validateContainersForSidecarSet(spec.InitContainers, spec.Containers, vols, fldPath.Root())...)
	}

	return allErrs
}

func validateSelector(selector *metav1.LabelSelector, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, metavalidation.ValidateLabelSelector(selector, fldPath)...)
	if len(selector.MatchLabels)+len(selector.MatchExpressions) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, selector, "empty selector is not valid for sidecarset."))
	}
	return allErrs
}

func validateSidecarSetUpdateStrategy(strategy *appsv1alpha1.SidecarSetUpdateStrategy, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	// if SidecarSet update strategy is RollingUpdate
	if strategy.Type == appsv1alpha1.RollingUpdateSidecarSetStrategyType {
		if strategy.Selector != nil {
			allErrs = append(allErrs, validateSelector(strategy.Selector, fldPath.Child("selector"))...)
		}
		if strategy.Partition != nil {
			allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(*(strategy.Partition), fldPath.Child("partition"))...)
		}
		if strategy.MaxUnavailable != nil {
			allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(*(strategy.MaxUnavailable), fldPath.Child("maxUnavailable"))...)
		}
		if strategy.ScatterStrategy != nil {
			if err := strategy.ScatterStrategy.FieldsValidation(); err != nil {
				allErrs = append(allErrs, field.Required(fldPath.Child("scatterStrategy"), err.Error()))
			}
		}
	}
	return allErrs
}

func validateContainersForSidecarSet(
	initContainers, containers []appsv1alpha1.SidecarContainer,
	coreVolumes []core.Volume, fldPath *field.Path) field.ErrorList {

	allErrs := field.ErrorList{}
	//validating initContainer
	var coreInitContainers []core.Container
	for _, container := range initContainers {
		coreContainer := core.Container{}
		if err := corev1.Convert_v1_Container_To_core_Container(&container.Container, &coreContainer, nil); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("initContainer"), container.Container, fmt.Sprintf("Convert_v1_Container_To_core_Container failed: %v", err)))
			return allErrs
		}
		coreInitContainers = append(coreInitContainers, coreContainer)
	}

	//validating container
	var coreContainers []core.Container
	for _, container := range containers {
		if container.PodInjectPolicy != appsv1alpha1.BeforeAppContainerType && container.PodInjectPolicy != appsv1alpha1.AfterAppContainerType {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("container").Child("podInjectPolicy"), container.PodInjectPolicy, "unsupported pod inject policy"))
		}
		if container.ShareVolumePolicy.Type != appsv1alpha1.ShareVolumePolicyEnabled && container.ShareVolumePolicy.Type != appsv1alpha1.ShareVolumePolicyDisabled {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("container").Child("shareVolumePolicy"), container.ShareVolumePolicy, "unsupported share volume policy"))
		}
		coreContainer := core.Container{}
		if err := corev1.Convert_v1_Container_To_core_Container(&container.Container, &coreContainer, nil); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("container"), container.Container, fmt.Sprintf("Convert_v1_Container_To_core_Container failed: %v", err)))
			return allErrs
		}
		coreContainers = append(coreContainers, coreContainer)
	}

	// hack, use fakePod to reuse unexported 'validateContainers' function
	var fakePod *core.Pod
	if len(coreContainers) == 0 {
		// hack, the ValidatePod requires containers, so create a fake coreContainer
		coreContainers = []core.Container{
			{
				Name:                     "test",
				Image:                    "busybox",
				ImagePullPolicy:          core.PullIfNotPresent,
				TerminationMessagePolicy: core.TerminationMessageReadFile,
			},
		}
	}

	fakePod = &core.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: core.PodSpec{
			DNSPolicy:      core.DNSClusterFirst,
			RestartPolicy:  core.RestartPolicyAlways,
			InitContainers: coreInitContainers,
			Containers:     coreContainers,
			Volumes:        coreVolumes,
		},
	}

	allErrs = append(allErrs, corevalidation.ValidatePod(fakePod)...)

	return allErrs
}

func validateSidecarContainerConflict(newContainers, oldContainers []appsv1alpha1.SidecarContainer, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	oldStrategy := make(map[string]appsv1alpha1.SidecarContainerUpgradeType)
	for _, container := range oldContainers {
		oldStrategy[container.Name] = container.UpgradeStrategy.UpgradeType
	}
	for _, container := range newContainers {
		if strategy, ok := oldStrategy[container.Name]; ok {
			if strategy != "" && container.UpgradeStrategy.UpgradeType != strategy {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("upgradeStrategy").Child("upgradeType"),
					container.Name, fmt.Sprintf("container %v upgradeType is immutable", container.Name)))
			}
		}
	}
	return allErrs
}

// validate the sidecarset spec.container.name, spec.initContainer.name, volume.name conflicts with others in cluster
func validateSidecarConflict(sidecarSets *appsv1alpha1.SidecarSetList, sidecarSet *appsv1alpha1.SidecarSet, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// record initContainer, container, volume name of other sidecarsets in cluster
	// container name -> sidecarset
	containerInOthers := make(map[string]*appsv1alpha1.SidecarSet)
	// volume name -> sidecarset
	volumeInOthers := make(map[string]*appsv1alpha1.SidecarSet)
	// init container name -> sidecarset
	initContainerInOthers := make(map[string]*appsv1alpha1.SidecarSet)
	for i := range sidecarSets.Items {
		set := &sidecarSets.Items[i]
		//ignore this sidecarset
		if set.Name == sidecarSet.Name {
			continue
		}
		for _, container := range set.Spec.InitContainers {
			initContainerInOthers[container.Name] = set
		}
		for _, container := range set.Spec.Containers {
			containerInOthers[container.Name] = set
		}
		for _, volume := range set.Spec.Volumes {
			volumeInOthers[volume.Name] = set
		}
	}

	// whether initContainers conflict
	for _, container := range sidecarSet.Spec.InitContainers {
		if other, ok := initContainerInOthers[container.Name]; ok {
			//if the two sidecarset scope namespace is different, continue
			if isSidecarSetNamespaceDiff(sidecarSet, other) {
				continue
			}
			// if the two sidecarset will selector same pod, then judge conflict
			if util.IsSelectorOverlapping(sidecarSet.Spec.Selector, other.Spec.Selector) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("containers"), container.Name, fmt.Sprintf(
					"container %v already exist in %v", container.Name, other.Name)))
			}
		}
	}

	// whether containers conflict
	for _, container := range sidecarSet.Spec.Containers {
		if other, ok := containerInOthers[container.Name]; ok {
			// if the two sidecarset scope namespace is different, continue
			if isSidecarSetNamespaceDiff(sidecarSet, other) {
				continue
			}
			// if the two sidecarset will selector same pod, then judge conflict
			if util.IsSelectorOverlapping(sidecarSet.Spec.Selector, other.Spec.Selector) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("containers"), container.Name, fmt.Sprintf(
					"container %v already exist in %v", container.Name, other.Name)))
			}
		}
	}

	// whether volumes conflict
	for _, volume := range sidecarSet.Spec.Volumes {
		if other, ok := volumeInOthers[volume.Name]; ok {
			//if the two sidecarset scope namespace is different, continue
			if isSidecarSetNamespaceDiff(sidecarSet, other) {
				continue
			}
			// if the two sidecarset will selector same pod, then judge conflict
			if util.IsSelectorOverlapping(sidecarSet.Spec.Selector, other.Spec.Selector) {
				if !reflect.DeepEqual(&volume, getSidecarsetVolume(volume.Name, other)) {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("volumes"), volume.Name, fmt.Sprintf(
						"volume %s is in conflict with sidecarset %s", volume.Name, other.Name)))
				}
			}
		}
	}

	return allErrs
}

func getSidecarsetVolume(volumeName string, sidecarset *appsv1alpha1.SidecarSet) *v1.Volume {
	for _, volume := range sidecarset.Spec.Volumes {
		if volume.Name == volumeName {
			return &volume
		}
	}
	return nil
}

var _ admission.Handler = &SidecarSetCreateUpdateHandler{}

// Handle handles admission requests.
func (h *SidecarSetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.SidecarSet{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var oldSidecarSet *appsv1alpha1.SidecarSet
	//when Operation is update, decode older object
	if req.AdmissionRequest.Operation == admissionv1beta1.Update {
		oldSidecarSet = new(appsv1alpha1.SidecarSet)
		if err := h.Decoder.Decode(
			admission.Request{AdmissionRequest: admissionv1beta1.AdmissionRequest{Object: req.AdmissionRequest.OldObject}},
			oldSidecarSet); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	}
	allowed, reason, err := h.validatingSidecarSetFn(ctx, obj, oldSidecarSet)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.ValidationResponse(allowed, reason)
}

var _ inject.Client = &SidecarSetCreateUpdateHandler{}

// InjectClient injects the client into the SidecarSetCreateUpdateHandler
func (h *SidecarSetCreateUpdateHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &SidecarSetCreateUpdateHandler{}

// InjectDecoder injects the decoder into the SidecarSetCreateUpdateHandler
func (h *SidecarSetCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
