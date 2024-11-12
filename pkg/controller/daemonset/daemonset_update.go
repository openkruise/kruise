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
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
)

// rollingUpdate identifies the set of old pods to in-place update, delete, or additional pods to create on nodes,
// remaining within the constraints imposed by the update strategy.
func (dsc *ReconcileDaemonSet) rollingUpdate(ctx context.Context, ds *appsv1alpha1.DaemonSet, nodeList []*corev1.Node, curRevision *apps.ControllerRevision, oldRevisions []*apps.ControllerRevision) error {
	hash := curRevision.Labels[apps.DefaultDaemonSetUniqueLabelKey]
	nodeToDaemonPods, err := dsc.getNodesToDaemonPods(ctx, ds)
	if err != nil {
		return fmt.Errorf("couldn't get node to daemon pod mapping for daemon set %q: %v", ds.Name, err)
	}
	maxSurge, maxUnavailable, err := dsc.updatedDesiredNodeCounts(ds, nodeList, nodeToDaemonPods)
	if err != nil {
		return fmt.Errorf("couldn't get unavailable numbers: %v", err)
	}

	// Advanced: filter the pods updated, updating and can update, according to partition and selector
	nodeToDaemonPods, err = dsc.filterDaemonPodsToUpdate(ds, nodeList, hash, nodeToDaemonPods)
	if err != nil {
		return fmt.Errorf("failed to filterDaemonPodsToUpdate: %v", err)
	}

	now := dsc.failedPodsBackoff.Clock.Now()

	// When not surging, we delete just enough pods to stay under the maxUnavailable limit, if any
	// are necessary, and let the core loop create new instances on those nodes.
	//
	// Assumptions:
	// * Expect manage loop to allow no more than one pod per node
	// * Expect manage loop will create new pods
	// * Expect manage loop will handle failed pods
	// * Deleted pods do not count as unavailable so that updates make progress when nodes are down
	// Invariants:
	// * The number of new pods that are unavailable must be less than maxUnavailable
	// * A node with an available old pod is a candidate for deletion if it does not violate other invariants
	//
	if maxSurge == 0 {
		var numUnavailable int
		var allowedReplacementPods []string
		var candidatePodsToDelete []string
		for nodeName, pods := range nodeToDaemonPods {
			newPod, oldPod, ok := findUpdatedPodsOnNode(ds, pods, hash)
			if !ok {
				// let the manage loop clean up this node, and treat it as an unavailable node
				klog.V(3).InfoS("DaemonSet had excess pods on node, skipped to allow the core loop to process", "daemonSet", klog.KObj(ds), "nodeName", nodeName)
				numUnavailable++
				continue
			}
			switch {
			case isPodNilOrPreDeleting(oldPod) && isPodNilOrPreDeleting(newPod), !isPodNilOrPreDeleting(oldPod) && !isPodNilOrPreDeleting(newPod):
				// the manage loop will handle creating or deleting the appropriate pod, consider this unavailable
				numUnavailable++
				klog.V(5).InfoS("DaemonSet found no pods (or pre-deleting) on node", "daemonSet", klog.KObj(ds), "nodeName", nodeName)
			case newPod != nil:
				// this pod is up to date, check its availability
				if !podutil.IsPodAvailable(newPod, ds.Spec.MinReadySeconds, metav1.Time{Time: now}) {
					// an unavailable new pod is counted against maxUnavailable
					numUnavailable++
					klog.V(5).InfoS("DaemonSet pod on node was new and unavailable", "daemonSet", klog.KObj(ds), "pod", klog.KObj(newPod), "nodeName", nodeName)
				}
				if isPodPreDeleting(newPod) {
					// a pre-deleting new pod is counted against maxUnavailable
					numUnavailable++
					klog.V(5).InfoS("DaemonSet pod on node was pre-deleting", "daemonSet", klog.KObj(ds), "pod", klog.KObj(newPod), "nodeName", nodeName)
				}
			default:
				// this pod is old, it is an update candidate
				switch {
				case !podutil.IsPodAvailable(oldPod, ds.Spec.MinReadySeconds, metav1.Time{Time: now}):
					// the old pod isn't available, so it needs to be replaced
					klog.V(5).InfoS("DaemonSet pod on node was out of date and not available, allowed replacement", "daemonSet", klog.KObj(ds), "pod", klog.KObj(oldPod), "nodeName", nodeName)
					// record the replacement
					if allowedReplacementPods == nil {
						allowedReplacementPods = make([]string, 0, len(nodeToDaemonPods))
					}
					allowedReplacementPods = append(allowedReplacementPods, oldPod.Name)
				case numUnavailable >= maxUnavailable:
					// no point considering any other candidates
					continue
				default:
					klog.V(5).InfoS("DaemonSet pod on node was out of date, it was a candidate to replace", "daemonSet", klog.KObj(ds), "pod", klog.KObj(oldPod), "nodeName", nodeName)
					// record the candidate
					if candidatePodsToDelete == nil {
						candidatePodsToDelete = make([]string, 0, maxUnavailable)
					}
					candidatePodsToDelete = append(candidatePodsToDelete, oldPod.Name)
				}
			}
		}

		// use any of the candidates we can, including the allowedReplacementPods
		klog.V(5).InfoS("DaemonSet allowing replacements, including some new unavailable and candidate pods, up to maxUnavailable",
			"daemonSet", klog.KObj(ds), "allowedReplacementPodCount", len(allowedReplacementPods), "maxUnavailable", maxUnavailable, "numUnavailable", numUnavailable, "candidatePodsToDeleteCount", len(candidatePodsToDelete))
		remainingUnavailable := maxUnavailable - numUnavailable
		if remainingUnavailable < 0 {
			remainingUnavailable = 0
		}
		if max := len(candidatePodsToDelete); remainingUnavailable > max {
			remainingUnavailable = max
		}
		oldPodsToDelete := append(allowedReplacementPods, candidatePodsToDelete[:remainingUnavailable]...)

		// Advanced: update pods in-place first and still delete the others
		if ds.Spec.UpdateStrategy.RollingUpdate.Type == appsv1alpha1.InplaceRollingUpdateType {
			oldPodsToDelete, err = dsc.inPlaceUpdatePods(ds, oldPodsToDelete, curRevision, oldRevisions)
			if err != nil {
				return err
			}
		}

		return dsc.syncNodes(ctx, ds, oldPodsToDelete, nil, hash)
	}

	// When surging, we create new pods whenever an old pod is unavailable, and we can create up
	// to maxSurge extra pods
	//
	// Assumptions:
	// * Expect manage loop to allow no more than two pods per node, one old, one new
	// * Expect manage loop will create new pods if there are no pods on node
	// * Expect manage loop will handle failed pods
	// * Deleted pods do not count as unavailable so that updates make progress when nodes are down
	// Invariants:
	// * A node with an unavailable old pod is a candidate for immediate new pod creation
	// * An old available pod is deleted if a new pod is available
	// * No more than maxSurge new pods are created for old available pods at any one time
	//
	var oldPodsToDelete []string
	var candidateNewNodes []string
	var allowedNewNodes []string
	var numSurge int

	for nodeName, pods := range nodeToDaemonPods {
		newPod, oldPod, ok := findUpdatedPodsOnNode(ds, pods, hash)
		if !ok {
			// let the manage loop clean up this node, and treat it as a surge node
			klog.V(3).InfoS("DaemonSet has excess pods on node, skipping to allow the core loop to process", "daemonSet", klog.KObj(ds), "nodeName", nodeName)
			numSurge++
			continue
		}
		switch {
		case isPodNilOrPreDeleting(oldPod):
			// we don't need to do anything to this node, the manage loop will handle it
		case newPod == nil:
			// this is a surge candidate
			switch {
			case !podutil.IsPodAvailable(oldPod, ds.Spec.MinReadySeconds, metav1.Time{Time: now}):
				// the old pod isn't available, allow it to become a replacement
				klog.V(5).InfoS("DaemonSet Pod on node was out of date and not available, allowed replacement", "daemonSet", klog.KObj(ds), "pod", klog.KObj(oldPod), "nodeName", nodeName)
				// record the replacement
				if allowedNewNodes == nil {
					allowedNewNodes = make([]string, 0, len(nodeToDaemonPods))
				}
				allowedNewNodes = append(allowedNewNodes, nodeName)
			case numSurge >= maxSurge:
				// no point considering any other candidates
				continue
			default:
				klog.V(5).InfoS("DaemonSet pod on node was out of date, so it was a surge candidate", "daemonSet", klog.KObj(ds), "pod", klog.KObj(oldPod), "nodeName", nodeName)
				// record the candidate
				if candidateNewNodes == nil {
					candidateNewNodes = make([]string, 0, maxSurge)
				}
				candidateNewNodes = append(candidateNewNodes, nodeName)
			}
		default:
			// we have already surged onto this node, determine our state
			if !podutil.IsPodAvailable(newPod, ds.Spec.MinReadySeconds, metav1.Time{Time: now}) {
				// we're waiting to go available here
				numSurge++
				continue
			}
			if isPodPreDeleting(newPod) {
				numSurge++
				continue
			}
			// we're available, delete the old pod
			klog.V(5).InfoS("DaemonSet pod on node was available, removed old pod", "daemonSet", klog.KObj(ds), "newPod", klog.KObj(newPod), "oldPod", klog.KObj(oldPod), "nodeName", nodeName)
			oldPodsToDelete = append(oldPodsToDelete, oldPod.Name)
		}
	}

	// use any of the candidates we can, including the allowedNewNodes
	klog.V(5).InfoS("DaemonSet allowing replacements, surge up to maxSurge",
		"daemonSet", klog.KObj(ds), "allowedNewNodeCount", len(allowedNewNodes), "maxSurge", maxSurge, "inProgressCount", numSurge, "candidateCount", len(candidateNewNodes))
	remainingSurge := maxSurge - numSurge
	if remainingSurge < 0 {
		remainingSurge = 0
	}
	if max := len(candidateNewNodes); remainingSurge > max {
		remainingSurge = max
	}
	newNodesToCreate := append(allowedNewNodes, candidateNewNodes[:remainingSurge]...)

	return dsc.syncNodes(ctx, ds, oldPodsToDelete, newNodesToCreate, hash)
}

// updatedDesiredNodeCounts calculates the true number of allowed unavailable or surge pods and
// updates the nodeToDaemonPods array to include an empty array for every node that is not scheduled.
func (dsc *ReconcileDaemonSet) updatedDesiredNodeCounts(ds *appsv1alpha1.DaemonSet, nodeList []*corev1.Node, nodeToDaemonPods map[string][]*corev1.Pod) (int, int, error) {
	var desiredNumberScheduled int
	for i := range nodeList {
		node := nodeList[i]
		wantToRun, _ := nodeShouldRunDaemonPod(node, ds)
		if !wantToRun {
			continue
		}
		desiredNumberScheduled++

		if _, exists := nodeToDaemonPods[node.Name]; !exists {
			nodeToDaemonPods[node.Name] = nil
		}
	}

	maxUnavailable, err := unavailableCount(ds, desiredNumberScheduled)
	if err != nil {
		return -1, -1, fmt.Errorf("invalid value for MaxUnavailable: %v", err)
	}

	maxSurge, err := surgeCount(ds, desiredNumberScheduled)
	if err != nil {
		return -1, -1, fmt.Errorf("invalid value for MaxSurge: %v", err)
	}

	// if the daemonset returned with an impossible configuration, obey the default of unavailable=1 (in the
	// event the apiserver returns 0 for both surge and unavailability)
	if desiredNumberScheduled > 0 && maxUnavailable == 0 && maxSurge == 0 {
		klog.InfoS("DaemonSet was not configured for surge or unavailability, defaulting to accepting unavailability", "daemonSet", klog.KObj(ds))
		maxUnavailable = 1
	}
	klog.V(5).InfoS("DaemonSet with maxSurge and maxUnavailable", "daemonSet", klog.KObj(ds), "maxSurge", maxSurge, "maxUnavailable", maxUnavailable)
	return maxSurge, maxUnavailable, nil
}

func GetTemplateGeneration(ds *appsv1alpha1.DaemonSet) (*int64, error) {
	annotation, found := ds.Annotations[apps.DeprecatedTemplateGeneration]
	if !found {
		return nil, nil
	}
	generation, err := strconv.ParseInt(annotation, 10, 64)
	if err != nil {
		return nil, err
	}
	return &generation, nil
}

func (dsc *ReconcileDaemonSet) filterDaemonPodsToUpdate(ds *appsv1alpha1.DaemonSet, nodeList []*corev1.Node, hash string, nodeToDaemonPods map[string][]*corev1.Pod) (map[string][]*corev1.Pod, error) {
	existingNodes := sets.NewString()
	for _, node := range nodeList {
		existingNodes.Insert(node.Name)
	}
	for nodeName := range nodeToDaemonPods {
		if !existingNodes.Has(nodeName) {
			delete(nodeToDaemonPods, nodeName)
		}
	}

	nodeNames, err := dsc.filterDaemonPodsNodeToUpdate(ds, hash, nodeToDaemonPods)
	if err != nil {
		return nil, err
	}

	ret := make(map[string][]*corev1.Pod, len(nodeNames))
	for _, name := range nodeNames {
		ret[name] = nodeToDaemonPods[name]
	}
	return ret, nil
}

func (dsc *ReconcileDaemonSet) filterDaemonPodsNodeToUpdate(ds *appsv1alpha1.DaemonSet, hash string, nodeToDaemonPods map[string][]*corev1.Pod) ([]string, error) {
	var err error
	var partition int32
	var selector labels.Selector
	if ds.Spec.UpdateStrategy.RollingUpdate != nil && ds.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		partition = *ds.Spec.UpdateStrategy.RollingUpdate.Partition
	}
	if ds.Spec.UpdateStrategy.RollingUpdate != nil && ds.Spec.UpdateStrategy.RollingUpdate.Selector != nil {
		if selector, err = util.ValidatedLabelSelectorAsSelector(ds.Spec.UpdateStrategy.RollingUpdate.Selector); err != nil {
			return nil, err
		}
	}

	var allNodeNames []string
	for nodeName := range nodeToDaemonPods {
		allNodeNames = append(allNodeNames, nodeName)
	}
	sort.Strings(allNodeNames)

	var updated []string
	var updating []string
	var selected []string
	var rest []string
	for i := len(allNodeNames) - 1; i >= 0; i-- {
		nodeName := allNodeNames[i]

		newPod, oldPod, ok := findUpdatedPodsOnNode(ds, nodeToDaemonPods[nodeName], hash)
		if !ok || newPod != nil {
			updated = append(updated, nodeName)
			continue
		}
		if isPodNilOrPreDeleting(oldPod) {
			updating = append(updating, nodeName)
			continue
		}

		if selector != nil {
			node, err := dsc.nodeLister.Get(nodeName)
			if err != nil {
				return nil, fmt.Errorf("failed to get node %v: %v", nodeName, err)
			}
			if selector.Matches(labels.Set(node.Labels)) {
				selected = append(selected, nodeName)
				continue
			}
		}

		rest = append(rest, nodeName)
	}

	sorted := append(updated, updating...)
	if selector != nil {
		sorted = append(sorted, selected...)
	} else {
		sorted = append(sorted, rest...)
	}
	if maxUpdate := len(allNodeNames) - int(partition); maxUpdate <= 0 {
		return nil, nil
	} else if maxUpdate < len(sorted) {
		sorted = sorted[:maxUpdate]
	}
	return sorted, nil
}

func getInPlaceUpdateOptions() *inplaceupdate.UpdateOptions {
	return &inplaceupdate.UpdateOptions{GetRevision: func(rev *apps.ControllerRevision) string {
		return rev.Labels[apps.DefaultDaemonSetUniqueLabelKey]
	}}
}

func (dsc *ReconcileDaemonSet) canPodInPlaceUpdate(pod *corev1.Pod, curRevision *apps.ControllerRevision, oldRevisions []*apps.ControllerRevision) bool {
	if !ContainsReadinessGate(pod) {
		return false
	}
	var oldRevision *apps.ControllerRevision
	for _, r := range oldRevisions {
		if clonesetutils.EqualToRevisionHash("", pod, r.Labels[apps.DefaultDaemonSetUniqueLabelKey]) {
			oldRevision = r
			break
		}
	}
	if oldRevision == nil {
		return false
	}
	return dsc.inplaceControl.CanUpdateInPlace(oldRevision, curRevision, getInPlaceUpdateOptions())
}

func (dsc *ReconcileDaemonSet) inPlaceUpdatePods(ds *appsv1alpha1.DaemonSet, podNames []string, curRevision *apps.ControllerRevision, oldRevisions []*apps.ControllerRevision) (podsNeedDelete []string, err error) {
	var podsToUpdate []*corev1.Pod
	for _, name := range podNames {
		pod, err := dsc.podLister.Pods(ds.Namespace).Get(name)
		if err != nil || !dsc.canPodInPlaceUpdate(pod, curRevision, oldRevisions) {
			podsNeedDelete = append(podsNeedDelete, name)
			continue
		}
		podsToUpdate = append(podsToUpdate, pod)
	}

	updateDiff := len(podsToUpdate)
	burstReplicas := getBurstReplicas(ds)
	if updateDiff > burstReplicas {
		updateDiff = burstReplicas
	}
	// error channel to communicate back failures.  make the buffer big enough to avoid any blocking
	errCh := make(chan error, updateDiff)
	deletePodsCh := make(chan string, updateDiff)

	updateWait := sync.WaitGroup{}
	updateWait.Add(updateDiff)
	for i := 0; i < updateDiff; i++ {
		go func(ix int, ds *appsv1alpha1.DaemonSet, pod *corev1.Pod) {
			defer updateWait.Done()

			var oldRevision *apps.ControllerRevision
			for _, r := range oldRevisions {
				if dsc.revisionAdapter.EqualToRevisionHash("", pod, r.Labels[apps.DefaultDaemonSetUniqueLabelKey]) {
					oldRevision = r
					break
				}
			}
			res := dsc.inplaceControl.Update(pod, oldRevision, curRevision, getInPlaceUpdateOptions())
			if res.InPlaceUpdate {
				if res.UpdateErr == nil {
					dsc.eventRecorder.Eventf(ds, corev1.EventTypeNormal, "SuccessfulUpdatePodInPlace", "successfully update pod %s in-place", pod.Name)
					dsc.resourceVersionExpectations.Expect(&metav1.ObjectMeta{UID: pod.UID, ResourceVersion: res.NewResourceVersion})
					return
				}

				dsc.eventRecorder.Eventf(ds, corev1.EventTypeWarning, "FailedUpdatePodInPlace", "failed to update pod %s in-place(revision %v): %v", pod.Name, curRevision.Name, res.UpdateErr)
				durationStore.Push(keyFunc(ds), res.DelayDuration)
				errCh <- res.UpdateErr
			} else {
				deletePodsCh <- pod.Name
			}
		}(i, ds, podsToUpdate[i])
	}
	updateWait.Wait()

	close(deletePodsCh)
	for name := range deletePodsCh {
		podsNeedDelete = append(podsNeedDelete, name)
	}

	// collect errors if any for proper reporting/retry logic in the controller
	var errors []error
	close(errCh)
	for err := range errCh {
		errors = append(errors, err)
	}

	return podsNeedDelete, utilerrors.NewAggregate(errors)
}
