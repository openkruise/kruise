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
	"bytes"
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"time"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/daemon/util"
	labelsutil "k8s.io/kubernetes/pkg/util/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func (dsc *ReconcileDaemonSet) constructHistory(ds *appsv1alpha1.DaemonSet) (cur *apps.ControllerRevision, old []*apps.ControllerRevision, err error) {
	var histories []*apps.ControllerRevision
	var currentHistories []*apps.ControllerRevision
	histories, err = dsc.controlledHistories(ds)
	if err != nil {
		return nil, nil, err
	}
	for _, history := range histories {
		// Add the unique label if it's not already added to the history
		// We use history name instead of computing hash, so that we don't need to worry about hash collision
		if _, ok := history.Labels[apps.DefaultDaemonSetUniqueLabelKey]; !ok {
			history.Labels[apps.DefaultDaemonSetUniqueLabelKey] = history.Name
			if err = dsc.client.Update(context.TODO(), history); err != nil {
				return nil, nil, err
			}
		}
		// Compare histories with ds to separate cur and old history
		found := false
		found, err = Match(ds, history)
		if err != nil {
			return nil, nil, err
		}
		if found {
			currentHistories = append(currentHistories, history)
		} else {
			old = append(old, history)
		}
	}

	currRevision := maxRevision(old) + 1
	switch len(currentHistories) {
	case 0:
		// Create a new history if the current one isn't found
		cur, err = dsc.snapshot(ds, currRevision)
		if err != nil {
			return nil, nil, err
		}
	default:
		cur, err = dsc.dedupCurHistories(ds, currentHistories)
		if err != nil {
			return nil, nil, err
		}
		// Update revision number if necessary
		if cur.Revision < currRevision {
			toUpdate := cur.DeepCopy()
			toUpdate.Revision = currRevision
			err = dsc.client.Update(context.TODO(), toUpdate)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	return cur, old, err
}

// rollingUpdate would update DaemonSet according to its rollingUpdateType
func (dsc *ReconcileDaemonSet) rollingUpdate(ds *appsv1alpha1.DaemonSet, hash string) (delay time.Duration, err error) {

	if ds.Spec.UpdateStrategy.RollingUpdate.Type == appsv1alpha1.StandardRollingUpdateType {
		return delay, dsc.standardRollingUpdate(ds, hash)
	} else if ds.Spec.UpdateStrategy.RollingUpdate.Type == appsv1alpha1.SurgingRollingUpdateType {
		return dsc.surgingRollingUpdate(ds, hash)
		//} else if ds.Spec.UpdateStrategy.RollingUpdate.Type == appsv1alpha1.InplaceRollingUpdateType {
		//	return dsc.inplaceRollingUpdate(ds, hash)
	} else {
		klog.Errorf("no matched RollingUpdate type")
	}
	return
}

// standardRollingUpdate deletes old daemon set pods making sure that no more than
// ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable pods are unavailable
func (dsc *ReconcileDaemonSet) standardRollingUpdate(ds *appsv1alpha1.DaemonSet, hash string) error {
	nodeToDaemonPods, err := dsc.getNodesToDaemonPods(ds)
	if err != nil {
		return fmt.Errorf("couldn't get node to daemon pod mapping for daemon set %q: %v", ds.Name, err)
	}

	maxUnavailable, numUnavailable, err := dsc.getUnavailableNumbers(ds, nodeToDaemonPods)
	if err != nil {
		return fmt.Errorf("couldn't get unavailable numbers: %v", err)
	}

	// calculate the cluster scope numUnavailable.
	if numUnavailable >= maxUnavailable {
		_, oldPods := dsc.getAllDaemonSetPods(ds, nodeToDaemonPods, hash)
		_, oldUnavailablePods := util.SplitByAvailablePods(ds.Spec.MinReadySeconds, oldPods)

		// for oldPods delete all not running pods
		var oldPodsToDelete []string
		for _, pod := range oldUnavailablePods {
			// Skip terminating pods. We won't delete them again
			if pod.DeletionTimestamp != nil {
				continue
			}
			oldPodsToDelete = append(oldPodsToDelete, pod.Name)
		}
		return dsc.syncNodes(ds, oldPodsToDelete, []string{}, hash)
	}

	nodeToDaemonPods, err = dsc.filterDaemonPodsToUpdate(ds, hash, nodeToDaemonPods)
	if err != nil {
		return fmt.Errorf("failed to filterDaemonPodsToUpdate: %v", err)
	}

	_, oldPods := dsc.getAllDaemonSetPods(ds, nodeToDaemonPods, hash)

	oldAvailablePods, oldUnavailablePods := util.SplitByAvailablePods(ds.Spec.MinReadySeconds, oldPods)

	// for oldPods delete all not running pods
	var oldPodsToDelete []string
	for _, pod := range oldUnavailablePods {
		// Skip terminating pods. We won't delete them again
		if pod.DeletionTimestamp != nil {
			continue
		}
		oldPodsToDelete = append(oldPodsToDelete, pod.Name)
	}

	for _, pod := range oldAvailablePods {
		if numUnavailable >= maxUnavailable {
			klog.V(0).Infof("%s/%s number of unavailable DaemonSet pods: %d, is equal to or exceeds allowed maximum: %d", ds.Namespace, ds.Name, numUnavailable, maxUnavailable)
			dsc.eventRecorder.Eventf(ds, corev1.EventTypeWarning, "numUnavailable >= maxUnavailable", "%s/%s number of unavailable DaemonSet pods: %d, is equal to or exceeds allowed maximum: %d", ds.Namespace, ds.Name, numUnavailable, maxUnavailable)
			break
		}

		// Skip deleting if there are some pods stuck at terminating but still report as available.
		if pod.DeletionTimestamp != nil {
			continue
		}

		klog.V(6).Infof("Marking pod %s/%s for deletion", ds.Name, pod.Name)
		oldPodsToDelete = append(oldPodsToDelete, pod.Name)
		numUnavailable++
	}
	return dsc.syncNodes(ds, oldPodsToDelete, []string{}, hash)
}

func (dsc *ReconcileDaemonSet) getAllDaemonSetPods(ds *appsv1alpha1.DaemonSet, nodeToDaemonPods map[string][]*corev1.Pod, hash string) ([]*corev1.Pod, []*corev1.Pod) {
	var newPods []*corev1.Pod
	var oldPods []*corev1.Pod

	for _, pods := range nodeToDaemonPods {
		for _, pod := range pods {
			// If the returned error is not nil we have a parse error.
			// The controller handles this via the hash.
			generation, err := GetTemplateGeneration(ds)
			if err != nil {
				generation = nil
			}

			if util.IsPodUpdated(pod, hash, generation) {
				newPods = append(newPods, pod)
			} else {
				oldPods = append(oldPods, pod)
			}
		}
	}
	return newPods, oldPods
}

type historiesByRevision []*apps.ControllerRevision

func (h historiesByRevision) Len() int      { return len(h) }
func (h historiesByRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByRevision) Less(i, j int) bool {
	return h[i].Revision < h[j].Revision
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

func (dsc *ReconcileDaemonSet) getUnavailableNumbers(ds *appsv1alpha1.DaemonSet, nodeToDaemonPods map[string][]*corev1.Pod) (int, int, error) {
	klog.V(6).Infof("Getting unavailable numbers")
	nodeList, err := dsc.nodeLister.List(labels.Everything())
	if err != nil {
		return -1, -1, fmt.Errorf("couldn't get list of nodes during rolling update of daemon set %#v: %v", ds, err)
	}

	var numUnavailable, desiredNumberScheduled int
	for i := range nodeList {
		node := nodeList[i]
		if !CanNodeBeDeployed(node, ds) {
			continue
		}
		wantToRun, _, _, err := NodeShouldRunDaemonPod(dsc.client, node, ds)
		if err != nil {
			return -1, -1, err
		}
		if !wantToRun {
			continue
		}
		desiredNumberScheduled++
		daemonPods, exists := nodeToDaemonPods[node.Name]
		if !exists {
			numUnavailable++
			continue
		}
		available := false
		for _, pod := range daemonPods {
			//for the purposes of update we ensure that the Pod is both available and not terminating
			if podutil.IsPodAvailable(pod, ds.Spec.MinReadySeconds, metav1.Now()) && pod.DeletionTimestamp == nil {
				available = true
				break
			}
		}
		if !available {
			numUnavailable++
		}
	}
	maxUnavailable, err := intstrutil.GetValueFromIntOrPercent(ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable, desiredNumberScheduled, true)
	if err != nil {
		return -1, -1, fmt.Errorf("invalid value for MaxUnavailable: %v", err)
	}
	klog.V(6).Infof(" DaemonSet %s/%s, maxUnavailable: %d, numUnavailable: %d", ds.Namespace, ds.Name, maxUnavailable, numUnavailable)
	return maxUnavailable, numUnavailable, nil
}

// controlledHistories returns all ControllerRevisions controlled by the given DaemonSet.
// This also reconciles ControllerRef by adopting/orphaning.
// Note that returned histories are pointers to objects in the cache.
// If you want to modify one, you need to deep-copy it first.
func (dsc *ReconcileDaemonSet) controlledHistories(ds *appsv1alpha1.DaemonSet) ([]*apps.ControllerRevision, error) {
	selector, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
	if err != nil {
		return nil, err
	}

	// List all histories to include those that don't match the selector anymore
	// but have a ControllerRef pointing to the controller.
	histories, err := dsc.historyLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Pods (see #42639).
	canAdoptFunc := kubecontroller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		fresh := &appsv1alpha1.DaemonSet{}
		key := types.NamespacedName{
			Namespace: ds.Namespace,
			Name:      ds.Name,
		}
		err = dsc.client.Get(context.TODO(), key, fresh)
		if err != nil {
			return nil, err
		}
		if fresh.UID != ds.UID {
			return nil, fmt.Errorf("original DaemonSet %v/%v is gone: got uid %v, wanted %v", ds.Namespace, ds.Name, fresh.UID, ds.UID)
		}
		return fresh, nil
	})
	// Use ControllerRefManager to adopt/orphan as needed.
	cm := kubecontroller.NewControllerRevisionControllerRefManager(dsc.crControl, ds, selector, controllerKind, canAdoptFunc)
	return cm.ClaimControllerRevisions(histories)
}

// Match check if the given DaemonSet's template matches the template stored in the given history.
func Match(ds *appsv1alpha1.DaemonSet, history *apps.ControllerRevision) (bool, error) {
	patch, err := getPatch(ds)
	if err != nil {
		return false, err
	}
	return bytes.Equal(patch, history.Data.Raw), nil
}

// getPatch returns a strategic merge patch that can be applied to restore a Daemonset to a
// previous version. If the returned error is nil the patch is valid. The current state that we save is just the
// PodSpecTemplate. We can modify this later to encompass more state (or less) and remain compatible with previously
// recorded patches.
func getPatch(ds *appsv1alpha1.DaemonSet) ([]byte, error) {
	dsBytes, err := json.Marshal(ds)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	err = json.Unmarshal(dsBytes, &raw)
	if err != nil {
		return nil, err
	}
	objCopy := make(map[string]interface{})
	specCopy := make(map[string]interface{})

	// Create a patch of the DaemonSet that replaces spec.template
	spec := raw["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	specCopy["template"] = template
	template["$patch"] = "replace"
	objCopy["spec"] = specCopy
	patch, err := json.Marshal(objCopy)
	return patch, err
}

// maxRevision returns the max revision number of the given list of histories
func maxRevision(histories []*apps.ControllerRevision) int64 {
	max := int64(0)
	for _, history := range histories {
		if history.Revision > max {
			max = history.Revision
		}
	}
	return max
}

func (dsc *ReconcileDaemonSet) snapshot(ds *appsv1alpha1.DaemonSet, revision int64) (*apps.ControllerRevision, error) {
	patch, err := getPatch(ds)
	if err != nil {
		return nil, err
	}
	hash := kubecontroller.ComputeHash(&ds.Spec.Template, ds.Status.CollisionCount)
	name := ds.Name + "-" + hash
	history := &apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ds.Namespace,
			Labels:          labelsutil.CloneAndAddLabel(ds.Spec.Template.Labels, apps.DefaultDaemonSetUniqueLabelKey, hash),
			Annotations:     ds.Annotations,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(ds, controllerKind)},
		},
		Data:     runtime.RawExtension{Raw: patch},
		Revision: revision,
	}

	err = dsc.client.Create(context.TODO(), history)
	if outerErr := err; errors.IsAlreadyExists(outerErr) {
		// TODO: Is it okay to get from historyLister?
		existedHistory := &apps.ControllerRevision{}
		getErr := dsc.client.Get(context.TODO(), client.ObjectKey{Namespace: ds.Namespace, Name: name}, existedHistory)
		if getErr != nil {
			return nil, getErr
		}
		// Check if we already created it
		done, matchErr := Match(ds, existedHistory)
		if matchErr != nil {
			return nil, matchErr
		}
		if done {
			return existedHistory, nil
		}

		// Handle name collisions between different history
		// Get the latest DaemonSet from the API server to make sure collision count is only increased when necessary
		currDS := &appsv1alpha1.DaemonSet{}
		getErr = dsc.client.Get(context.TODO(), client.ObjectKey{Namespace: ds.Namespace, Name: name}, currDS)
		if getErr != nil {
			return nil, getErr
		}
		// If the collision count used to compute hash was in fact stale, there's no need to bump collision count; retry again
		if !reflect.DeepEqual(currDS.Status.CollisionCount, ds.Status.CollisionCount) {
			return nil, fmt.Errorf("found a stale collision count (%d, expected %d) of DaemonSet %q while processing; will retry until it is updated", ds.Status.CollisionCount, currDS.Status.CollisionCount, ds.Name)
		}
		if currDS.Status.CollisionCount == nil {
			currDS.Status.CollisionCount = new(int32)
		}
		*currDS.Status.CollisionCount++
		updateErr := dsc.client.Status().Update(context.TODO(), currDS)
		if updateErr != nil {
			return nil, updateErr
		}
		klog.V(2).Infof("Found a hash collision for DaemonSet %q - bumping collisionCount to %d to resolve it", ds.Name, *currDS.Status.CollisionCount)
		return nil, outerErr
	}
	return history, err
}

func (dsc *ReconcileDaemonSet) dedupCurHistories(ds *appsv1alpha1.DaemonSet, curHistories []*apps.ControllerRevision) (*apps.ControllerRevision, error) {
	if len(curHistories) == 1 {
		return curHistories[0], nil
	}
	var maxRevision int64
	var keepCur *apps.ControllerRevision
	for _, cur := range curHistories {
		if cur.Revision >= maxRevision {
			keepCur = cur
			maxRevision = cur.Revision
		}
	}
	// Clean up duplicates and relabel pods
	for _, cur := range curHistories {
		if cur.Name == keepCur.Name {
			continue
		}
		// Relabel pods before dedup
		pods, err := dsc.getDaemonPods(ds)
		if err != nil {
			return nil, err
		}
		for _, pod := range pods {
			if pod.Labels[apps.DefaultDaemonSetUniqueLabelKey] != keepCur.Labels[apps.DefaultDaemonSetUniqueLabelKey] {
				toUpdate := pod.DeepCopy()
				if toUpdate.Labels == nil {
					toUpdate.Labels = make(map[string]string)
				}
				toUpdate.Labels[apps.DefaultDaemonSetUniqueLabelKey] = keepCur.Labels[apps.DefaultDaemonSetUniqueLabelKey]
				err = dsc.client.Update(context.TODO(), toUpdate)
				if err != nil {
					return nil, err
				}
			}
		}
		// Remove duplicates
		err = dsc.client.Delete(context.TODO(), cur)
		if err != nil {
			return nil, err
		}
	}
	return keepCur, nil
}

func (dsc *ReconcileDaemonSet) filterDaemonPodsNodeToUpdate(ds *appsv1alpha1.DaemonSet, hash string, nodeToDaemonPods map[string][]*corev1.Pod) ([]string, error) {
	var err error
	var partition int32
	var selector labels.Selector
	var generation *int64
	if ds.Spec.UpdateStrategy.RollingUpdate != nil && ds.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		partition = *ds.Spec.UpdateStrategy.RollingUpdate.Partition
	}
	if ds.Spec.UpdateStrategy.RollingUpdate != nil && ds.Spec.UpdateStrategy.RollingUpdate.Selector != nil {
		if selector, err = metav1.LabelSelectorAsSelector(ds.Spec.UpdateStrategy.RollingUpdate.Selector); err != nil {
			return nil, err
		}
	}
	if generation, err = GetTemplateGeneration(ds); err != nil {
		return nil, err
	}

	var allNames []string
	for nodeName := range nodeToDaemonPods {
		allNames = append(allNames, nodeName)
	}
	sort.Strings(allNames)

	var updated []string
	var selected []string
	var rest []string
	for i := len(allNames) - 1; i >= 0; i-- {
		nodeName := allNames[i]
		pods := nodeToDaemonPods[nodeName]

		var hasUpdated bool
		var terminatingCount int
		for i := range pods {
			pod := pods[i]
			if util.IsPodUpdated(pod, hash, generation) {
				hasUpdated = true
				break
			}
			if pod.DeletionTimestamp != nil {
				terminatingCount++
			}
		}
		if hasUpdated || terminatingCount == len(pods) {
			updated = append(updated, nodeName)
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

	var sorted []string
	if selector != nil {
		sorted = append(updated, selected...)
	} else {
		sorted = append(updated, rest...)
	}
	if maxUpdate := len(allNames) - int(partition); maxUpdate <= 0 {
		return nil, nil
	} else if maxUpdate < len(sorted) {
		sorted = sorted[:maxUpdate]
	}
	return sorted, nil
}

func (dsc *ReconcileDaemonSet) filterDaemonPodsToUpdate(ds *appsv1alpha1.DaemonSet, hash string, nodeToDaemonPods map[string][]*corev1.Pod) (map[string][]*corev1.Pod, error) {
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

// getSurgeNumbers returns the max allowable number of surging pods and the current number of
// surging pods. The number of surging pods is computed as the total number pods above the first
// on each node.
func (dsc *ReconcileDaemonSet) getSurgeNumbers(ds *appsv1alpha1.DaemonSet, nodeToDaemonPods map[string][]*corev1.Pod, hash string) (int, int, error) {
	nodeList, err := dsc.nodeLister.List(labels.Everything())
	if err != nil {
		return -1, -1, fmt.Errorf("couldn't get list of nodes during surging rolling update of daemon set %#v: %v", ds, err)
	}

	generation, err := GetTemplateGeneration(ds)
	if err != nil {
		generation = nil
	}
	var desiredNumberScheduled, numSurge int
	for i := range nodeList {
		node := nodeList[i]
		if !CanNodeBeDeployed(node, ds) {
			continue
		}
		wantToRun, _, _, err := NodeShouldRunDaemonPod(dsc.client, node, ds)
		if err != nil {
			return -1, -1, err
		}
		if !wantToRun {
			continue
		}

		desiredNumberScheduled++

		for _, pod := range nodeToDaemonPods[node.Name] {
			if util.IsPodUpdated(pod, hash, generation) && len(nodeToDaemonPods[node.Name]) > 1 {
				numSurge++
				break
			}
		}
	}

	maxSurge, err := intstrutil.GetValueFromIntOrPercent(ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge, desiredNumberScheduled, true)
	if err != nil {
		return -1, -1, fmt.Errorf("invalid value for MaxSurge: %v", err)
	}
	return maxSurge, numSurge, nil
}

// get all nodes where the ds should run
func (dsc *ReconcileDaemonSet) getNodesShouldRunDaemonPod(ds *appsv1alpha1.DaemonSet) (
	nodesWantToRun sets.String,
	nodesShouldContinueRunning sets.String,
	err error) {
	nodeList, err := dsc.nodeLister.List(labels.Everything())
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't get list of nodes when updating daemon set %#v: %v", ds, err)
	}

	nodesWantToRun = sets.String{}
	nodesShouldContinueRunning = sets.String{}
	for _, node := range nodeList {
		if !CanNodeBeDeployed(node, ds) {
			continue
		}
		wantToRun, _, shouldContinueRunning, err := NodeShouldRunDaemonPod(dsc.client, node, ds)
		if err != nil {
			return nil, nil, err
		}

		if wantToRun {
			nodesWantToRun.Insert(node.Name)
		}

		if shouldContinueRunning {
			nodesShouldContinueRunning.Insert(node.Name)
		}
	}
	return nodesWantToRun, nodesShouldContinueRunning, nil
}

// pruneSurgingDaemonPods prunes the list of pods to only contain pods belonging to the current
// generation. This method only applies when the update strategy is SurgingRollingUpdate.
// This allows the daemon set controller to temporarily break its contract that only one daemon
// pod can run per node by ignoring the pods that belong to previous generations, which are
// cleaned up by the surgingRollingUpdate() method above.
func (dsc *ReconcileDaemonSet) pruneSurgingDaemonPods(ds *appsv1alpha1.DaemonSet, pods []*corev1.Pod, hash string) []*corev1.Pod {
	if len(pods) <= 1 || ds.Spec.UpdateStrategy.RollingUpdate.Type != appsv1alpha1.SurgingRollingUpdateType {
		return pods
	}
	var currentPods []*corev1.Pod
	for _, pod := range pods {
		generation, err := GetTemplateGeneration(ds)
		if err != nil {
			generation = nil
		}
		if util.IsPodUpdated(pod, hash, generation) {
			currentPods = append(currentPods, pod)
		}
	}
	// Escape hatch if no new pods of the current generation are present yet.
	if len(currentPods) == 0 {
		currentPods = pods
	}
	return currentPods
}

// surgingRollingUpdate creates new daemon set pods to replace old ones making sure that no more
// than ds.Spec.UpdateStrategy.SurgingRollingUpdate.MaxSurge extra pods are scheduled at any
// given time.
func (dsc *ReconcileDaemonSet) surgingRollingUpdate(ds *appsv1alpha1.DaemonSet, hash string) (delay time.Duration, err error) {
	nodeToDaemonPods, err := dsc.getNodesToDaemonPods(ds)
	if err != nil {
		return delay, fmt.Errorf("couldn't get node to daemon pod mapping for daemon set %q: %v", ds.Name, err)
	}

	nodeToDaemonPods, err = dsc.filterDaemonPodsToUpdate(ds, hash, nodeToDaemonPods)
	if err != nil {
		return delay, fmt.Errorf("failed to filterDaemonPodsToUpdate: %v", err)
	}

	maxSurge, numSurge, err := dsc.getSurgeNumbers(ds, nodeToDaemonPods, hash)
	if err != nil {
		return delay, fmt.Errorf("couldn't get surge numbers: %v", err)
	}

	nodesWantToRun, nodesShouldContinueRunning, err := dsc.getNodesShouldRunDaemonPod(ds)
	if err != nil {
		return delay, fmt.Errorf("couldn't get nodes which want to run ds pod: %v", err)
	}

	var nodesToSurge []string
	var oldPodsToDelete []string
	for node, pods := range nodeToDaemonPods {
		var newPod, oldPod *corev1.Pod
		wantToRun := nodesWantToRun.Has(node)
		shouldContinueRunning := nodesShouldContinueRunning.Has(node)
		// if node has new taint, then the already existed pod does not want to run but should continue running
		if !wantToRun && !shouldContinueRunning {
			for _, pod := range pods {
				klog.V(6).Infof("Marking pod %s/%s on unsuitable node %s for deletion", ds.Name, pod.Name, node)
				if pod.DeletionTimestamp == nil {
					oldPodsToDelete = append(oldPodsToDelete, pod.Name)
				}
			}
			continue
		}
		foundAvailable := false
		for _, pod := range pods {
			generation, err := GetTemplateGeneration(ds)
			if err != nil {
				generation = nil
			}
			if util.IsPodUpdated(pod, hash, generation) {
				if newPod != nil {
					klog.Warningf("Multiple new pods on node %s: %s, %s", node, newPod.Name, pod.Name)
				}
				newPod = pod
			} else {
				if oldPod != nil {
					klog.Warningf("Multiple old pods on node %s: %s, %s", node, oldPod.Name, pod.Name)
				}
				oldPod = pod
			}
			if !foundAvailable && podutil.IsPodAvailable(pod, ds.Spec.MinReadySeconds, metav1.Now()) {
				foundAvailable = true
			}
		}

		if newPod != nil {
			klog.Infof("newPod IsPodAvailable is %v", podutil.IsPodAvailable(newPod, ds.Spec.MinReadySeconds, metav1.Now()))
		}
		if newPod == nil && numSurge < maxSurge && wantToRun {
			if !foundAvailable && len(pods) >= 2 {
				klog.Warningf("Node %s already has %d unavailable pods, need clean first, skip surge new pod", node, len(pods))
			} else {
				klog.V(6).Infof("Surging new pod on node %s", node)
				numSurge++
				nodesToSurge = append(nodesToSurge, node)
			}
		} else if newPod != nil && podutil.IsPodAvailable(newPod, ds.Spec.MinReadySeconds, metav1.Now()) && oldPod != nil && oldPod.DeletionTimestamp == nil {
			klog.V(6).Infof("Marking pod %s/%s for deletion", ds.Name, oldPod.Name)
			oldPodsToDelete = append(oldPodsToDelete, oldPod.Name)
		} else {
			if ds.Spec.MinReadySeconds > 0 {
				if newPod != nil {
					if podutil.IsPodAvailable(newPod, ds.Spec.MinReadySeconds, metav1.Now()) {
						if oldPod != nil && oldPod.DeletionTimestamp == nil {
							klog.V(6).Infof("Marking pod %s/%s for deletion", ds.Name, oldPod.Name)
							oldPodsToDelete = append(oldPodsToDelete, oldPod.Name)
						}
					} else {
						return time.Duration(ds.Spec.MinReadySeconds) * time.Second, nil
					}
				}
			}
		}
	}
	return delay, dsc.syncNodes(ds, oldPodsToDelete, nodesToSurge, hash)
}
