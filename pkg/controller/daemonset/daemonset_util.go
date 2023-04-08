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

package daemonset

import (
	"fmt"
	"sort"
	"sync"
	"time"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseutil "github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/lifecycle"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	v1helper "k8s.io/component-helpers/scheduling/corev1"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/controller/daemon/util"
	"k8s.io/utils/integer"
)

var (
	// newPodForDSCache is a cache for NewPod, it is map[ds.UID]*newPodForDS
	newPodForDSCache sync.Map
	newPodForDSLock  sync.Mutex
)

type newPodForDS struct {
	generation int64
	pod        *corev1.Pod
}

func loadNewPodForDS(ds *appsv1alpha1.DaemonSet) *corev1.Pod {
	if val, ok := newPodForDSCache.Load(ds.UID); ok {
		newPodCache := val.(*newPodForDS)
		if newPodCache.generation >= ds.Generation {
			return newPodCache.pod
		}
	}
	return nil
}

// nodeInSameCondition returns true if all effective types ("Status" is true) equals;
// otherwise, returns false.
func nodeInSameCondition(old []corev1.NodeCondition, cur []corev1.NodeCondition) bool {
	if len(old) == 0 && len(cur) == 0 {
		return true
	}

	c1map := map[corev1.NodeConditionType]corev1.ConditionStatus{}
	for _, c := range old {
		if c.Status == corev1.ConditionTrue {
			c1map[c.Type] = c.Status
		}
	}

	for _, c := range cur {
		if c.Status != corev1.ConditionTrue {
			continue
		}

		if _, found := c1map[c.Type]; !found {
			return false
		}

		delete(c1map, c.Type)
	}

	return len(c1map) == 0
}

// nodeShouldRunDaemonPod checks a set of preconditions against a (node,daemonset) and returns a
// summary. Returned booleans are:
//   - shouldRun:
//     Returns true when a daemonset should run on the node if a daemonset pod is not already
//     running on that node.
//   - shouldContinueRunning:
//     Returns true when a daemonset should continue running on a node if a daemonset pod is already
//     running on that node.
func nodeShouldRunDaemonPod(node *corev1.Node, ds *appsv1alpha1.DaemonSet) (bool, bool) {
	pod := NewPod(ds, node.Name)

	// If the daemon set specifies a node name, check that it matches with node.Name.
	if !(ds.Spec.Template.Spec.NodeName == "" || ds.Spec.Template.Spec.NodeName == node.Name) {
		return false, false
	}

	taints := node.Spec.Taints
	fitsNodeName, fitsNodeAffinity, fitsTaints := Predicates(pod, node, taints)
	if !fitsNodeName || !fitsNodeAffinity {
		return false, false
	}

	if !fitsTaints {
		// Scheduled daemon pods should continue running if they tolerate NoExecute taint.
		_, hasUntoleratedTaint := v1helper.FindMatchingUntoleratedTaint(taints, pod.Spec.Tolerations, func(t *corev1.Taint) bool {
			return t.Effect == corev1.TaintEffectNoExecute
		})
		return false, !hasUntoleratedTaint
	}

	return true, true
}

func shouldIgnoreNodeUpdate(oldNode, curNode corev1.Node) bool {
	if !nodeInSameCondition(oldNode.Status.Conditions, curNode.Status.Conditions) {
		return false
	}
	oldNode.ResourceVersion = curNode.ResourceVersion
	oldNode.Status.Conditions = curNode.Status.Conditions
	return apiequality.Semantic.DeepEqual(oldNode, curNode)
}

func getBurstReplicas(ds *appsv1alpha1.DaemonSet) int {
	// Error caught by validation
	burstReplicas, _ := intstrutil.GetScaledValueFromIntOrPercent(
		intstrutil.ValueOrDefault(ds.Spec.BurstReplicas, intstrutil.FromInt(BurstReplicas)),
		int(ds.Status.DesiredNumberScheduled),
		false)
	return burstReplicas
}

// GetPodDaemonSets returns a list of DaemonSets that potentially match a pod.
// Only the one specified in the Pod's ControllerRef will actually manage it.
// Returns an error only if no matching DaemonSets are found.
func (dsc *ReconcileDaemonSet) GetPodDaemonSets(pod *corev1.Pod) ([]*appsv1alpha1.DaemonSet, error) {
	if len(pod.Labels) == 0 {
		return nil, fmt.Errorf("no daemon sets found for pod %v because it has no labels", pod.Name)
	}

	dsList, err := dsc.dsLister.DaemonSets(pod.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var selector labels.Selector
	var daemonSets []*appsv1alpha1.DaemonSet
	for _, ds := range dsList {
		selector, err = kruiseutil.ValidatedLabelSelectorAsSelector(ds.Spec.Selector)
		if err != nil {
			// this should not happen if the DaemonSet passed validation
			return nil, err
		}

		// If a daemonSet with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		daemonSets = append(daemonSets, ds)
	}

	if len(daemonSets) == 0 {
		return nil, fmt.Errorf("could not find daemon set for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
	}

	return daemonSets, nil
}

// GetPodRevision returns revision hash of this pod.
func GetPodRevision(controllerKey string, pod metav1.Object) string {
	return pod.GetLabels()[apps.ControllerRevisionHashLabelKey]
}

// isDaemonPodAvailable returns true if a pod is ready after update progress.
func isDaemonPodAvailable(pod *corev1.Pod, minReadySeconds int32, now metav1.Time) bool {
	if !podutil.IsPodAvailable(pod, minReadySeconds, now) {
		return false
	}

	if ContainsReadinessGate(pod) {
		condition := inplaceupdate.GetCondition(pod)
		if condition != nil && condition.Status != corev1.ConditionTrue {
			return false
		}
	}

	return true
}

// ContainsReadinessGate returns true if pod has inplace update readiness gate.
func ContainsReadinessGate(pod *corev1.Pod) bool {
	for _, r := range pod.Spec.ReadinessGates {
		if r.ConditionType == appspub.InPlaceUpdateReady {
			return true
		}
	}
	return false
}

// isDaemonSetCreationProgressively returns true if and only if the progressive annotation is set to true.
func isDaemonSetCreationProgressively(ds *appsv1alpha1.DaemonSet) bool {
	return ds.Annotations[ProgressiveCreatePod] == "true"
}

// GetNodesNeedingPods finds which nodes should run daemon pod according to progressive flag and parititon.
func GetNodesNeedingPods(newPodsNum, desire, partition int, progressive bool, nodesNeedingPods []string) []string {
	if !progressive {
		sort.Strings(nodesNeedingPods)
		return nodesNeedingPods
	}

	// partition must be less than total number and greater than zero.
	partition = integer.IntMax(integer.IntMin(partition, desire), 0)

	maxCreate := integer.IntMax(desire-newPodsNum-partition, 0)
	if maxCreate > len(nodesNeedingPods) {
		maxCreate = len(nodesNeedingPods)
	}

	if maxCreate > 0 {
		sort.Strings(nodesNeedingPods)
		nodesNeedingPods = nodesNeedingPods[:maxCreate]
	} else {
		nodesNeedingPods = []string{}
	}

	return nodesNeedingPods
}

func keyFunc(ds *appsv1alpha1.DaemonSet) string {
	return fmt.Sprintf("%s/%s", ds.Namespace, ds.Name)
}

func isDaemonSetPaused(ds *appsv1alpha1.DaemonSet) bool {
	return ds.Spec.UpdateStrategy.RollingUpdate != nil && ds.Spec.UpdateStrategy.RollingUpdate.Paused != nil && *ds.Spec.UpdateStrategy.RollingUpdate.Paused
}

// allowSurge returns true if the daemonset allows more than a single pod on any node.
func allowSurge(ds *appsv1alpha1.DaemonSet) bool {
	maxSurge, err := surgeCount(ds, 1)
	return err == nil && maxSurge > 0
}

// surgeCount returns 0 if surge is not requested, the expected surge number to allow
// out of numberToSchedule if surge is configured, or an error if the surge percentage
// requested is invalid.
func surgeCount(ds *appsv1alpha1.DaemonSet, numberToSchedule int) (int, error) {
	if ds.Spec.UpdateStrategy.Type != appsv1alpha1.RollingUpdateDaemonSetStrategyType {
		return 0, nil
	}
	r := ds.Spec.UpdateStrategy.RollingUpdate
	if r == nil {
		return 0, nil
	}
	if r.MaxSurge == nil {
		return 0, nil
	}
	return intstrutil.GetScaledValueFromIntOrPercent(r.MaxSurge, numberToSchedule, true)
}

// unavailableCount returns 0 if unavailability is not requested, the expected
// unavailability number to allow out of numberToSchedule if requested, or an error if
// the unavailability percentage requested is invalid.
func unavailableCount(ds *appsv1alpha1.DaemonSet, numberToSchedule int) (int, error) {
	if ds.Spec.UpdateStrategy.Type != appsv1alpha1.RollingUpdateDaemonSetStrategyType {
		return 0, nil
	}
	r := ds.Spec.UpdateStrategy.RollingUpdate
	if r == nil {
		return 0, nil
	}
	if r.MaxUnavailable == nil {
		return 0, nil
	}
	return intstrutil.GetScaledValueFromIntOrPercent(r.MaxUnavailable, numberToSchedule, true)
}

// getUnscheduledPodsWithoutNode returns list of unscheduled pods assigned to not existing nodes.
// Returned pods can't be deleted by PodGCController so they should be deleted by DaemonSetController.
func getUnscheduledPodsWithoutNode(runningNodesList []*corev1.Node, nodeToDaemonPods map[string][]*corev1.Pod) []string {
	var results []string
	isNodeRunning := make(map[string]bool)
	for _, node := range runningNodesList {
		isNodeRunning[node.Name] = true
	}
	for n, pods := range nodeToDaemonPods {
		if !isNodeRunning[n] {
			for _, pod := range pods {
				if len(pod.Spec.NodeName) == 0 {
					results = append(results, pod.Name)
				}
			}
		}
	}
	return results
}

// findUpdatedPodsOnNode looks at non-deleted pods on a given node and returns true if there
// is at most one of each old and new pods, or false if there are multiples. We can skip
// processing the particular node in those scenarios and let the manage loop prune the
// excess pods for our next time around.
func findUpdatedPodsOnNode(ds *appsv1alpha1.DaemonSet, podsOnNode []*corev1.Pod, hash string) (newPod, oldPod *corev1.Pod, ok bool) {
	for _, pod := range podsOnNode {
		if pod.DeletionTimestamp != nil {
			continue
		}
		generation, err := GetTemplateGeneration(ds)
		if err != nil {
			generation = nil
		}
		if util.IsPodUpdated(pod, hash, generation) {
			if newPod != nil {
				return nil, nil, false
			}
			newPod = pod
		} else {
			if oldPod != nil {
				return nil, nil, false
			}
			oldPod = pod
		}
	}
	return newPod, oldPod, true
}

// NodeShouldUpdateBySelector checks if the node is selected to upgrade for ds's gray update selector.
// This function does not check NodeShouldRunDaemonPod
func NodeShouldUpdateBySelector(node *corev1.Node, ds *appsv1alpha1.DaemonSet) bool {
	switch ds.Spec.UpdateStrategy.Type {
	case appsv1alpha1.OnDeleteDaemonSetStrategyType:
		return false
	case appsv1alpha1.RollingUpdateDaemonSetStrategyType:
		if ds.Spec.UpdateStrategy.RollingUpdate == nil || ds.Spec.UpdateStrategy.RollingUpdate.Selector == nil {
			return false
		}
		selector, err := kruiseutil.ValidatedLabelSelectorAsSelector(ds.Spec.UpdateStrategy.RollingUpdate.Selector)
		if err != nil {
			// this should not happen if the DaemonSet passed validation
			return false
		}
		return !selector.Empty() && selector.Matches(labels.Set(node.Labels))
	default:
		return false
	}
}

func isPodPreDeleting(pod *corev1.Pod) bool {
	return pod != nil && lifecycle.GetPodLifecycleState(pod) == appspub.LifecycleStatePreparingDelete
}

func isPodNilOrPreDeleting(pod *corev1.Pod) bool {
	return pod == nil || isPodPreDeleting(pod)
}

func podAvailableWaitingTime(pod *corev1.Pod, minReadySeconds int32, now time.Time) time.Duration {
	c := podutil.GetPodReadyCondition(pod.Status)
	minReadySecondsDuration := time.Duration(minReadySeconds) * time.Second
	if c == nil || c.LastTransitionTime.IsZero() {
		return minReadySecondsDuration
	}
	return minReadySecondsDuration - now.Sub(c.LastTransitionTime.Time)
}
