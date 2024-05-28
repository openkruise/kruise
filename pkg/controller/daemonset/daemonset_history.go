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

package daemonset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	labelsutil "k8s.io/kubernetes/pkg/util/labels"
)

func (dsc *ReconcileDaemonSet) constructHistory(ctx context.Context, ds *appsv1alpha1.DaemonSet) (cur *apps.ControllerRevision, old []*apps.ControllerRevision, err error) {
	var histories []*apps.ControllerRevision
	var currentHistories []*apps.ControllerRevision
	histories, err = dsc.controlledHistories(ctx, ds)
	if err != nil {
		return nil, nil, err
	}
	for _, history := range histories {
		// Add the unique label if it's not already added to the history
		// We use history name instead of computing hash, so that we don't need to worry about hash collision
		if _, ok := history.Labels[apps.DefaultDaemonSetUniqueLabelKey]; !ok {
			toUpdate := history.DeepCopy()
			toUpdate.Labels[apps.DefaultDaemonSetUniqueLabelKey] = toUpdate.Name
			history, err = dsc.kubeClient.AppsV1().ControllerRevisions(ds.Namespace).Update(ctx, toUpdate, metav1.UpdateOptions{})
			if err != nil {
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
		cur, err = dsc.snapshot(ctx, ds, currRevision)
		if err != nil {
			return nil, nil, err
		}
	default:
		cur, err = dsc.dedupCurHistories(ctx, ds, currentHistories)
		if err != nil {
			return nil, nil, err
		}
		// Update revision number if necessary
		if cur.Revision < currRevision {
			toUpdate := cur.DeepCopy()
			toUpdate.Revision = currRevision
			_, err = dsc.kubeClient.AppsV1().ControllerRevisions(ds.Namespace).Update(ctx, toUpdate, metav1.UpdateOptions{})
			if err != nil {
				return nil, nil, err
			}
		}
	}
	return cur, old, err
}

// controlledHistories returns all ControllerRevisions controlled by the given DaemonSet.
// This also reconciles ControllerRef by adopting/orphaning.
// Note that returned histories are pointers to objects in the cache.
// If you want to modify one, you need to deep-copy it first.
func (dsc *ReconcileDaemonSet) controlledHistories(ctx context.Context, ds *appsv1alpha1.DaemonSet) ([]*apps.ControllerRevision, error) {
	selector, err := util.ValidatedLabelSelectorAsSelector(ds.Spec.Selector)
	if err != nil {
		return nil, err
	}

	// List all histories to include those that don't match the selector anymore
	// but have a ControllerRef pointing to the controller.
	histories, err := dsc.historyLister.ControllerRevisions(ds.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Pods (see #42639).
	canAdoptFunc := kubecontroller.RecheckDeletionTimestamp(func(ctx context.Context) (metav1.Object, error) {
		fresh, err := dsc.kruiseClient.AppsV1alpha1().DaemonSets(ds.Namespace).Get(ctx, ds.Name, metav1.GetOptions{})
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
	return cm.ClaimControllerRevisions(ctx, histories)
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

func (dsc *ReconcileDaemonSet) snapshot(ctx context.Context, ds *appsv1alpha1.DaemonSet, revision int64) (*apps.ControllerRevision, error) {
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

	history, err = dsc.kubeClient.AppsV1().ControllerRevisions(ds.Namespace).Create(ctx, history, metav1.CreateOptions{})
	if outerErr := err; errors.IsAlreadyExists(outerErr) {
		// TODO: Is it okay to get from historyLister?
		existedHistory, getErr := dsc.kubeClient.AppsV1().ControllerRevisions(ds.Namespace).Get(ctx, name, metav1.GetOptions{})
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
		currDS, getErr := dsc.kubeClient.AppsV1().DaemonSets(ds.Namespace).Get(ctx, ds.Name, metav1.GetOptions{})
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
		_, updateErr := dsc.kubeClient.AppsV1().DaemonSets(ds.Namespace).UpdateStatus(ctx, currDS, metav1.UpdateOptions{})
		if updateErr != nil {
			return nil, updateErr
		}
		klog.V(2).InfoS("Found a hash collision for DaemonSet - bumping collisionCount to resolve it", "daemonSet", klog.KObj(ds), "collisionCount", *currDS.Status.CollisionCount)
		return nil, outerErr
	}
	return history, err
}

func (dsc *ReconcileDaemonSet) dedupCurHistories(ctx context.Context, ds *appsv1alpha1.DaemonSet, curHistories []*apps.ControllerRevision) (*apps.ControllerRevision, error) {
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
	// Relabel pods before dedup
	pods, err := dsc.getDaemonPods(ctx, ds)
	if err != nil {
		return nil, err
	}
	for _, pod := range pods {
		if pod.Labels[apps.DefaultDaemonSetUniqueLabelKey] != keepCur.Labels[apps.DefaultDaemonSetUniqueLabelKey] {
			patchRaw := map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						apps.DefaultDaemonSetUniqueLabelKey: keepCur.Labels[apps.DefaultDaemonSetUniqueLabelKey],
					},
				},
			}
			patchJson, err := json.Marshal(patchRaw)
			if err != nil {
				return nil, err
			}
			_, err = dsc.kubeClient.CoreV1().Pods(ds.Namespace).Patch(ctx, pod.Name, types.MergePatchType, patchJson, metav1.PatchOptions{})
			if err != nil {
				return nil, err
			}
		}
	}
	// Clean up duplicates and relabel pods
	for _, cur := range curHistories {
		if cur.Name == keepCur.Name {
			continue
		}
		// Remove duplicates
		err = dsc.kubeClient.AppsV1().ControllerRevisions(ds.Namespace).Delete(ctx, cur.Name, metav1.DeleteOptions{})
		if err != nil {
			return nil, err
		}
	}
	return keepCur, nil
}

type historiesByRevision []*apps.ControllerRevision

func (h historiesByRevision) Len() int      { return len(h) }
func (h historiesByRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByRevision) Less(i, j int) bool {
	return h[i].Revision < h[j].Revision
}
