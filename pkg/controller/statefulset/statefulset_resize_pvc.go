/*
Copyright 2024 The Kruise Authors.

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

package statefulset

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

// todo: if rollback, pass when kep1790 enabled
func (spc *StatefulPodControl) tryPatchPVCSize(set *appsv1beta1.StatefulSet, pod *v1.Pod) error {
	ordinal := getOrdinal(pod)
	templates := set.Spec.VolumeClaimTemplates
	for i := range templates {
		claimName := getPersistentVolumeClaimName(set, &templates[i], ordinal)
		claim, err := spc.objectMgr.GetClaim(set.Namespace, claimName)
		switch {
		case apierrors.IsNotFound(err):
			klog.V(4).InfoS("Expected claim missing, continuing to pick up in next iteration", "claimName", claimName)
		case err != nil:
			return fmt.Errorf("could not retrieve claim %s for %s when checking PVC spec", claimName, pod.Name)
		default:
			if matched, needExpand := CompareWithCheckFn(claim, &templates[i], PVCNeedExpand); !matched && needExpand {
				if claim.Spec.StorageClassName != nil {
					scName := *claim.Spec.StorageClassName
					sc, err := spc.objectMgr.GetStorageClass(scName)
					if err != nil {
						return fmt.Errorf("could not get sc %s for %s when checking PVC spec: %v", scName, pod.Name, err)
					}
					if sc == nil {
						return fmt.Errorf("could not get sc %s for %s when checking PVC spec", scName, pod.Name)
					}
					if sc.AllowVolumeExpansion == nil || !(*sc.AllowVolumeExpansion) {
						return fmt.Errorf("sc %s for %s disallow volume expansion", scName, pod.Name)
					}
				}

				claimClone := claim.DeepCopy()
				needsUpdate := resizeClaim(set, claimClone, &templates[i])
				if needsUpdate {
					err := spc.objectMgr.UpdateClaim(claimClone)
					spc.recordClaimEvent("resize", set, pod, claimClone, err)
					if err != nil {
						return fmt.Errorf("could not update claim %s: %w", claimName, err)
					}
				}
			} else if !matched {
				spc.recorder.Eventf(set, v1.EventTypeWarning, "FailedUpdatePVC", "failed to update pvc %s: contains diff other than spec resource, wait pvc to be deleted", claimName)
				return fmt.Errorf("can not patch pvc %s: contains diff other than spec resource, wait pvc to be deleted", claimName)
			}
		}
	}
	return nil
}

func (spc *StatefulPodControl) ClaimsMatchSpec(set *appsv1beta1.StatefulSet, pod *v1.Pod) (bool, error) {
	ordinal := getOrdinal(pod)
	templates := set.Spec.VolumeClaimTemplates
	for i := range templates {
		claimName := getPersistentVolumeClaimName(set, &templates[i], ordinal)
		claim, err := spc.objectMgr.GetClaim(set.Namespace, claimName)
		switch {
		case apierrors.IsNotFound(err):
			klog.V(4).InfoS("Expected claim missing, continuing to pick up in next iteration", "claimName", claimName)
		case err != nil:
			return false, fmt.Errorf("could not retrieve claim %s for %s when checking PVC spec", claimName, pod.Name)
		default:
			if matched, _ := CompareWithCheckFn(claim, &templates[i], PVCNeedExpand); !matched {
				return false, nil
			}
		}
	}
	return true, nil
}

type CheckClaimFn = func(*v1.PersistentVolumeClaim, *v1.PersistentVolumeClaim) bool

func CheckClaimWithoutSize(claim, template *v1.PersistentVolumeClaim) bool {
	// when there is default sc,
	// template StorageClassName is nil but claim is not nil
	if template.Spec.StorageClassName != nil &&
		claim.Spec.StorageClassName != nil &&
		*claim.Spec.StorageClassName != *template.Spec.StorageClassName {
		return false
	}
	// use set compare?
	if len(claim.Spec.AccessModes) != len(template.Spec.AccessModes) {
		return false
	}
	for i, mode := range claim.Spec.AccessModes {
		if template.Spec.AccessModes[i] != mode {
			return false
		}
	}
	return true
}

func CompareWithCheckFn(claim, template *v1.PersistentVolumeClaim, cmp CheckClaimFn) (matched, cmpResult bool) {
	if !CheckClaimWithoutSize(claim, template) {
		return false, false
	}
	if cmp(claim, template) {
		return false, true
	}
	return true, false
}

func CheckPatchPVCCompleted(claim, template *v1.PersistentVolumeClaim) bool {
	compatible, ready := PVCCompatibleAndReady(claim, template)
	if compatible && ready {
		return true
	}
	if compatible {
		pending := false
		for _, condition := range claim.Status.Conditions {
			if condition.Type == v1.PersistentVolumeClaimFileSystemResizePending {
				pending = true
			}
		}
		// if pending, patch PVC completed
		return pending
	}
	return false
}

func PVCCompatibleAndReady(claim, template *v1.PersistentVolumeClaim) (compatible bool, ready bool) {
	if !CheckClaimWithoutSize(claim, template) {
		return false, false
	}
	compatible = func(claim, template *v1.PersistentVolumeClaim) bool {
		// claim >= template => compatible
		// claim < template => not compatible (need patch by controller)
		if claim.Spec.Resources.Requests.Storage() != nil &&
			template.Spec.Resources.Requests.Storage() != nil &&
			claim.Spec.Resources.Requests.Storage().Cmp(*template.Spec.Resources.Requests.Storage()) >= 0 {
			return true
		}
		return false
	}(claim, template)

	ready = func(claim, template *v1.PersistentVolumeClaim) bool {
		// cap >= spec => ready
		// cap < spec => need storage expansion by csi
		if claim.Status.Capacity.Storage() != nil &&
			claim.Spec.Resources.Requests.Storage() != nil &&
			claim.Status.Capacity.Storage().Cmp(*claim.Spec.Resources.Requests.Storage()) >= 0 {
			return true
		}
		return false
	}(claim, template)
	return
}

// PVCNeedExpand checks if the given PersistentVolumeClaim (PVC) has expanded based on a template.
// Parameters:
//
//	claim: The PVC object to be inspected.
//	template: The PVC template object for comparison.
//
// Return:
//
//	A boolean indicating whether the PVC has expanded beyond the template's definitions in terms of storage requests or limits.
//
// This function determines if the PVC has expanded by comparing the storage requests and limits between the PVC and the template.
// If either the storage request or limit of the PVC exceeds that defined in the template, it is considered expanded.
func PVCNeedExpand(claim, template *v1.PersistentVolumeClaim) bool {
	// pvc spec < template spec => need expand
	if claim.Spec.Resources.Requests.Storage() != nil &&
		template.Spec.Resources.Requests.Storage() != nil &&
		claim.Spec.Resources.Limits.Storage() != nil &&
		template.Spec.Resources.Limits.Storage() != nil &&
		claim.Spec.Resources.Requests.Storage().Cmp(*template.Spec.Resources.Requests.Storage()) < 0 ||
		claim.Spec.Resources.Limits.Storage().Cmp(*template.Spec.Resources.Limits.Storage()) < 0 {
		return true
	}
	return false
}

func resizeClaim(set *appsv1beta1.StatefulSet, claim, template *v1.PersistentVolumeClaim) (needUpdate bool) {
	if len(claim.Annotations) == 0 {
		claim.Annotations = map[string]string{}
	}
	claim.Annotations[PVCOwnedByStsAnnotationKey] = set.Name
	if PVCNeedExpand(claim, template) {
		claim.Spec.Resources = template.Spec.Resources
		return true
	}
	return false
}
