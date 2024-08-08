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
	"github.com/openkruise/kruise/pkg/util/pvc"
)

// update pvc resource and attach some necessary information
func resizeClaim(set *appsv1beta1.StatefulSet, claim, template *v1.PersistentVolumeClaim) (needUpdate bool) {
	if len(claim.Annotations) == 0 {
		claim.Annotations = map[string]string{}
	}
	claim.Annotations[PVCOwnedByStsAnnotationKey] = set.Name
	if pvc.IsPVCNeedExpand(claim, template) {
		claim.Spec.Resources = template.Spec.Resources
		return true
	}
	return false
}

// TODO: if rollback, pass when kep1790 enabled
func (spc *StatefulPodControl) TryPatchPVCSize(set *appsv1beta1.StatefulSet, pod *v1.Pod) error {
	fn := func(claim, template *v1.PersistentVolumeClaim) (bool, error) {
		matched, needExpand := pvc.CompareWithCheckFn(claim, template, pvc.IsPVCNeedExpand)
		if matched {
			return true, nil
		}
		if !needExpand {
			spc.recorder.Eventf(set, v1.EventTypeWarning, "FailedUpdatePVC", "failed to update pvc %s: contains diff other than spec resource, wait pvc to be deleted", claim.Name)
			return false, fmt.Errorf("can not patch pvc %s: contains diff other than spec resource, wait pvc to be deleted", claim.Name)
		}
		// only pvc expand => check storage class allow expansion
		if claim.Spec.StorageClassName != nil {
			scName := *claim.Spec.StorageClassName
			sc, err := spc.objectMgr.GetStorageClass(scName)
			if err != nil {
				return false, fmt.Errorf("could not get sc %s for %s when checking PVC spec: %v", scName, pod.Name, err)
			}
			if sc == nil {
				return false, fmt.Errorf("could not get sc %s for %s when checking PVC spec", scName, pod.Name)
			}
			if sc.AllowVolumeExpansion == nil || !(*sc.AllowVolumeExpansion) {
				return false, fmt.Errorf("storage class %s for %s does not support volume expansion", scName, pod.Name)
			}
		}

		claimClone := claim.DeepCopy()
		needsUpdate := resizeClaim(set, claimClone, template)
		if needsUpdate {
			err := spc.objectMgr.UpdateClaim(claimClone)
			spc.recordClaimEvent("Resize", set, pod, claimClone, err)
			if err != nil {
				return false, fmt.Errorf("could not update claim %s: %w", claim.Name, err)
			}
		}
		return true, nil
	}
	_, err := spc.handlePVCWithCustomFn(set, pod, true, fn)
	return err
}

func (spc *StatefulPodControl) IsClaimsCompatible(set *appsv1beta1.StatefulSet, pod *v1.Pod) (bool, error) {
	fn := func(claim, template *v1.PersistentVolumeClaim) (bool, error) {
		if matched, _ := pvc.CompareWithCheckFn(claim, template, pvc.IsPVCNeedExpand); !matched {
			return false, nil
		}
		return true, nil
	}
	return spc.handlePVCWithCustomFn(set, pod, true, fn)
}

func (spc *StatefulPodControl) checkOwnedPVCStatus(set *appsv1beta1.StatefulSet, pod *v1.Pod, fn pvc.CheckClaimFn) (bool, error) {
	innerFn := func(claim, template *v1.PersistentVolumeClaim) (bool, error) {
		ready := fn(claim, template)
		if !ready {
			return false, nil
		}
		return true, nil
	}
	return spc.handlePVCWithCustomFn(set, pod, true, innerFn)
}

func (ssc *defaultStatefulSetControl) updatePVCStatus(status *appsv1beta1.StatefulSetStatus, set *appsv1beta1.StatefulSet, pods []*v1.Pod) {
	templates := set.Spec.VolumeClaimTemplates
	status.VolumeClaims = make([]appsv1beta1.VolumeClaimStatus, len(templates))
	templateNameMap := map[string]*appsv1beta1.VolumeClaimStatus{}
	for i := range templates {
		status.VolumeClaims[i].VolumeClaimName = templates[i].Name
		templateNameMap[templates[i].Name] = &status.VolumeClaims[i]
	}

	fn := func(claim, template *v1.PersistentVolumeClaim) (bool, error) {
		if compatible, ready := pvc.IsPVCCompatibleAndReady(claim, template); compatible {
			templateStatus := templateNameMap[template.Name]
			templateStatus.CompatibleReplicas++
			if ready {
				templateStatus.CompatibleReadyReplicas++
			}
		}
		return true, nil
	}

	for _, pod := range pods {
		if pod == nil {
			continue
		}

		success, err := ssc.podControl.handlePVCWithCustomFn(set, pod, true, fn)
		if err != nil || !success {
			return
		}
	}
}

type handlePVCWithFailFastFn = func(claim, template *v1.PersistentVolumeClaim) (success bool, err error)

func (spc *StatefulPodControl) handlePVCWithCustomFn(set *appsv1beta1.StatefulSet, pod *v1.Pod, ignoreTerminatingPVC bool, fn handlePVCWithFailFastFn) (bool, error) {
	// ignore nil pod
	if pod == nil {
		return true, nil
	}

	ordinal := getOrdinal(pod)
	templates := set.Spec.VolumeClaimTemplates
	for i := range templates {
		claimName := getPersistentVolumeClaimName(set, &templates[i], ordinal)
		claim, err := spc.objectMgr.GetClaim(set.Namespace, claimName)
		if apierrors.IsNotFound(err) {
			klog.V(4).InfoS("Expected claim missing, continuing to pick up in next iteration", "claimName", claimName)
			continue
		} else if err != nil {
			return false, fmt.Errorf("could not retrieve claim %s for %s when checking PVC spec", claimName, pod.Name)
		}

		if ignoreTerminatingPVC && claim.DeletionTimestamp != nil {
			continue
		}

		if success, err := fn(claim, &templates[i]); err != nil || !success {
			return false, err
		}
	}
	return true, nil
}
