/*
Copyright 2019 The Kruise Authors.
Copyright 2016 The Kubernetes Authors.

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

package uniteddeployment

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/util"
	"github.com/openkruise/kruise/pkg/util/expectations"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
)

const updateRetries = 5

// ParseSubsetReplicas parses the subsetReplicas, and returns the replicas number depending on the sum replicas.
func ParseSubsetReplicas(udReplicas int32, subsetReplicas intstr.IntOrString) (int32, error) {
	if subsetReplicas.Type == intstr.Int {
		if subsetReplicas.IntVal < 0 {
			return 0, fmt.Errorf("subset replicas (%d) should not be less than 0", subsetReplicas.IntVal)
		}
		return subsetReplicas.IntVal, nil
	}

	if udReplicas < 0 {
		return 0, fmt.Errorf("subsetReplicas (%v) should not be string when unitedDeployment replicas is empty", subsetReplicas.StrVal)
	}

	strVal := subsetReplicas.StrVal
	if !strings.HasSuffix(strVal, "%") {
		return 0, fmt.Errorf("subset replicas (%s) only support integer value or percentage value with a suffix '%%'", strVal)
	}

	intPart := strVal[:len(strVal)-1]
	percent64, err := strconv.ParseInt(intPart, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("subset replicas (%s) should be correct percentage integer: %s", strVal, err)
	}

	if percent64 > int64(100) || percent64 < int64(0) {
		return 0, fmt.Errorf("subset replicas (%s) should be in range [0, 100]", strVal)
	}

	return int32(round(float64(udReplicas) * float64(percent64) / 100)), nil
}

func round(x float64) int {
	return int(math.Floor(x + 0.5))
}

func getSubsetNameFrom(metaObj metav1.Object) (string, error) {
	name, exist := metaObj.GetLabels()[appsv1alpha1.SubSetNameLabelKey]
	if !exist {
		return "", fmt.Errorf("fail to get subSet name from reserved of subset %s/%s: no reserved %s found", metaObj.GetNamespace(), metaObj.GetName(), appsv1alpha1.SubSetNameLabelKey)
	}

	if len(name) == 0 {
		return "", fmt.Errorf("fail to get subSet name from reserved of subset %s/%s: reserved %s has an empty value", metaObj.GetNamespace(), metaObj.GetName(), appsv1alpha1.SubSetNameLabelKey)
	}

	return name, nil
}

// NewUnitedDeploymentCondition creates a new UnitedDeployment condition.
func NewUnitedDeploymentCondition(condType appsv1alpha1.UnitedDeploymentConditionType, status corev1.ConditionStatus, reason, message string) *appsv1alpha1.UnitedDeploymentCondition {
	return &appsv1alpha1.UnitedDeploymentCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// GetUnitedDeploymentCondition returns the condition with the provided type.
func GetUnitedDeploymentCondition(status *appsv1alpha1.UnitedDeploymentStatus, condType appsv1alpha1.UnitedDeploymentConditionType) *appsv1alpha1.UnitedDeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// SetUnitedDeploymentCondition updates the UnitedDeployment to include the provided condition. If the condition that
// we are about to add already exists and has the same status, reason and message then we are not going to update.
func SetUnitedDeploymentCondition(status *appsv1alpha1.UnitedDeploymentStatus, condition *appsv1alpha1.UnitedDeploymentCondition) {
	currentCond := GetUnitedDeploymentCondition(status, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}

	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}
	newConditions := filterOutCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, *condition)
}

// RemoveUnitedDeploymentCondition removes the UnitedDeployment condition with the provided type.
func RemoveUnitedDeploymentCondition(status *appsv1alpha1.UnitedDeploymentStatus, condType appsv1alpha1.UnitedDeploymentConditionType) {
	status.Conditions = filterOutCondition(status.Conditions, condType)
}

func filterOutCondition(conditions []appsv1alpha1.UnitedDeploymentCondition, condType appsv1alpha1.UnitedDeploymentConditionType) []appsv1alpha1.UnitedDeploymentCondition {
	var newConditions []appsv1alpha1.UnitedDeploymentCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}

func getUnitedDeploymentKey(ud *appsv1alpha1.UnitedDeployment) string {
	return ud.GetNamespace() + "/" + ud.GetName()
}

var ResourceVersionExpectation = expectations.NewResourceVersionExpectation()

// CheckPodReserved checks whether the Pod is in the Reserved state.
// The Reserved state is defined as: the Pod enters Reserved after being in the Pending state for a specified duration,
// and exits Reserved after remaining in the Ready state for a certain period.
//
// Parameters:
//   - pod: the Pod object to be checked.
//   - subset: the Subset object associated with the Pod.
//   - updatedCondition: "Updated" Condition of UnitedDeployment.
//   - pendingTimeout: the timeout duration for the Pod to remain in the Pending state.
//   - minReadySeconds: the duration threshold for the Pod to remain in the Ready state.
//
// Returns:
//   - isReserved: true if the Pod is in the Reserved state; otherwise, false.
//   - nextCheckAfter: the duration until the next check if the Pod has not reached the Reserved state; otherwise, -1.
func CheckPodReserved(pod *corev1.Pod, subset *Subset, updatedCondition *appsv1alpha1.UnitedDeploymentCondition,
	pendingTimeout time.Duration, minReadySeconds time.Duration, now time.Time) (isReserved bool, nextCheckAfter time.Duration) {
	podRevision, _ := GetPodLabel(pod, appsv1alpha1.ControllerRevisionHashLabelKey)
	if reserved, _ := GetPodReserved(pod); reserved {
		if pod.Status.Phase == corev1.PodRunning && podRevision == subset.Status.UpdatedRevision {
			readyCondition := getPodCondition(pod, corev1.PodReady)
			if readyCondition != nil && readyCondition.Status == corev1.ConditionTrue {
				if readyCondition.LastTransitionTime.Time.Add(minReadySeconds).Before(now) {
					return false, -1
				} else {
					return true, readyCondition.LastTransitionTime.Time.Add(minReadySeconds).Sub(now)
				}
			}
		}
		// at least after minReadySeconds will the pod get out of Reserved state
		return true, minReadySeconds
	} else {
		var timeouted bool
		var after time.Duration
		if podRevision == subset.Status.UpdatedRevision {
			timeouted, after = util.GetTimeBeforePendingTimeout(pod, pendingTimeout, now)
		} else {
			timeouted, after = util.GetTimeBeforeLegacyTimeout(pod, updatedCondition, pendingTimeout, now)
			if timeouted {
				subset.Status.UnschedulableStatus.TimeoutLegacyPods++
			}
			klog.InfoS("GetTimeBeforeLegacyTimeout", "pod", pod.Name, "after", after, "timeouted", timeouted)
		}
		if timeouted {
			return true, minReadySeconds
		} else {
			return false, after
		}
	}
}

// GetPodReserved checks whether the Pod is in the Reserved state.
// The Reserved state is defined as: the Pod has the reserved "apps.kruise.io/is-reserved-pod" set to "true".
//
// Parameters:
//   - pod: the Pod object to be checked.
//
// Returns:
//   - reserved: true if the Pod is in the Reserved state; otherwise, false.
//   - ok: true if the Pod has the "apps.kruise.io/is-reserved-pod" reserved; otherwise, false.
func GetPodReserved(pod *corev1.Pod) (reserved bool, ok bool) {
	if label, ok := GetPodLabel(pod, appsv1alpha1.ReservedPodLabelKey); ok {
		return label == "true", true
	} else {
		return false, false
	}
}

// GetPodLabel makes it compatible with the tests where pod.Label not set.
func GetPodLabel(pod *corev1.Pod, key string) (string, bool) {
	if pod.Labels != nil {
		label, ok := pod.Labels[key]
		return label, ok
	} else {
		return "", false
	}
}

func getPodCondition(pod *corev1.Pod, tp corev1.PodConditionType) *corev1.PodCondition {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == tp {
			return &condition
		}
	}
	return nil
}

func initStatus(u *appsv1alpha1.UnitedDeployment) {
	for _, subset := range u.Spec.Topology.Subsets {
		if u.Status.GetSubsetStatus(subset.Name) == nil {
			u.Status.SubsetStatuses = append(u.Status.SubsetStatuses, appsv1alpha1.UnitedDeploymentSubsetStatus{Name: subset.Name})
		}
	}
	return
}
