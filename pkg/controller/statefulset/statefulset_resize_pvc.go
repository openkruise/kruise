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
	ordinal := getOrdinal(pod)
	templates := set.Spec.VolumeClaimTemplates
	for i := range templates {
		claimName := getPersistentVolumeClaimName(set, &templates[i], ordinal)
		claim, err := spc.objectMgr.GetClaim(set.Namespace, claimName)
		if apierrors.IsNotFound(err) {
			klog.V(4).InfoS("Expected claim missing, continuing to pick up in next iteration", "claimName", claimName)
			continue
		} else if err != nil {
			return fmt.Errorf("could not retrieve claim %s for %s when checking PVC spec", claimName, pod.Name)
		}
		if matched, needExpand := pvc.CompareWithCheckFn(claim, &templates[i], pvc.IsPVCNeedExpand); !matched && needExpand {
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
	return nil
}

func (spc *StatefulPodControl) IsClaimsCompatible(set *appsv1beta1.StatefulSet, pod *v1.Pod) (bool, error) {
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
		if matched, _ := pvc.CompareWithCheckFn(claim, &templates[i], pvc.IsPVCNeedExpand); !matched {
			return false, nil
		}
	}
	return true, nil
}

func (ssc *defaultStatefulSetControl) checkOwnedPVCStatus(set *appsv1beta1.StatefulSet, pod *v1.Pod, fn pvc.CheckClaimFn) (bool, error) {
	templates := set.Spec.VolumeClaimTemplates
	ordinal := getOrdinal(pod)
	for i := range templates {
		claimName := getPersistentVolumeClaimName(set, &templates[i], ordinal)
		claim, err := ssc.podControl.objectMgr.GetClaim(set.Namespace, claimName)
		if apierrors.IsNotFound(err) {
			klog.V(4).InfoS("Expected claim missing", "claim", claimName)
			return false, err
		} else if err != nil {
			klog.V(4).ErrorS(err, "Could not retrieve claim", "claim", claimName, "pod", pod.Name)
			return false, err
		}
		ready := fn(claim, &templates[i])
		if !ready {
			return false, err
		}
	}
	return true, nil
}

func (ssc *defaultStatefulSetControl) updatePVCStatus(status *appsv1beta1.StatefulSetStatus, set *appsv1beta1.StatefulSet, pods []*v1.Pod) {
	templates := set.Spec.VolumeClaimTemplates
	status.VolumeClaimTemplates = make([]appsv1beta1.VolumeClaimStatus, len(templates))
	templateNameMap := map[string]*appsv1beta1.VolumeClaimStatus{}
	for i := range templates {
		status.VolumeClaimTemplates[i].VolumeClaimName = templates[i].Name
		templateNameMap[templates[i].Name] = &status.VolumeClaimTemplates[i]
	}
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		ordinal := getOrdinal(pod)
		for i := range templates {
			claimName := getPersistentVolumeClaimName(set, &templates[i], ordinal)
			claim, err := ssc.podControl.objectMgr.GetClaim(set.Namespace, claimName)
			if apierrors.IsNotFound(err) {
				klog.V(4).InfoS("Expected claim missing", "claim", claimName)
				continue
			} else if err != nil {
				klog.V(4).ErrorS(err, "Could not retrieve claim", "claim", claimName, "pod", pod.Name)
				return
			}
			if compatible, ready := pvc.IsPVCCompatibleAndReady(claim, &templates[i]); compatible {
				templateStatus := templateNameMap[templates[i].Name]
				templateStatus.CompatibleReplicas++
				if ready {
					templateStatus.CompatibleReadyReplicas++
				}
			}
		}
	}
}
