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
	"sync"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
)

func (dsc *ReconcileDaemonSet) inplaceRollingUpdate(ds *appsv1alpha1.DaemonSet, hash string) (reconcile.Result, error) {
	nodeToDaemonPods, err := dsc.getNodesToDaemonPods(ds)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("couldn't get node to daemon pod mapping for daemon set %q: %v", ds.Name, err)
	}

	maxUnavailable, numUnavailable, err := dsc.getUnavailableNumbers(ds, nodeToDaemonPods)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("couldn't get unavailable numbers: %v", err)
	}

	nodeToDaemonPods, err = dsc.filterDaemonPodsToUpdate(ds, hash, nodeToDaemonPods)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to filterDaemonPodsToUpdate: %v", err)
	}

	newPods, oldPods := dsc.getAllDaemonSetPods(ds, nodeToDaemonPods, hash)

	oldAvailablePods, oldUnavailablePods := SplitByAvailablePods(ds.Spec.MinReadySeconds, oldPods)

	// for oldPods delete all not running pods
	var oldPodsToInplaceUpdate []*corev1.Pod
	var oldPodsToDelete []string
	for _, pod := range oldUnavailablePods {
		// Skip terminating pods. We won't delete them again
		if pod.DeletionTimestamp != nil {
			continue
		}

		// if the pod does not contain readiness gate, delete it and recreate one in
		// next reconcile loop.
		if ContainsReadinessGate(pod) {
			oldPodsToInplaceUpdate = append(oldPodsToInplaceUpdate, pod)
		} else {
			oldPodsToDelete = append(oldPodsToDelete, pod.Name)
		}
	}

	for _, pod := range oldAvailablePods {
		if numUnavailable >= maxUnavailable {
			klog.V(6).Infof("Number of unavailable DaemonSet pods: %d, is equal to or exceeds allowed maximum: %d", numUnavailable, maxUnavailable)
			dsc.eventRecorder.Eventf(ds, corev1.EventTypeWarning, "numUnavailable >= maxUnavailable", "%s/%s number of unavailable DaemonSet pods: %d, is equal to or exceeds allowed maximum: %d", ds.Namespace, ds.Name, numUnavailable, maxUnavailable)
			break
		}
		klog.V(6).Infof("Marking pod %s/%s for deletion", ds.Name, pod.Name)
		if ContainsReadinessGate(pod) {
			oldPodsToInplaceUpdate = append(oldPodsToInplaceUpdate, pod)
		} else {
			oldPodsToDelete = append(oldPodsToDelete, pod.Name)
		}
		numUnavailable++
	}

	if len(oldPodsToDelete) > 0 {
		return reconcile.Result{}, dsc.syncNodes(ds, oldPodsToDelete, []string{}, hash)
	}

	cur, old, err := dsc.constructHistory(ds)
	if err != nil {
		klog.Errorf("failed to construct revisions of DaemonSet: %v", err)
	}
	if len(old) == 0 {
		return reconcile.Result{}, nil
	}

	// Refresh update expectations
	key, _ := kubecontroller.KeyFunc(ds)
	// Refresh update expectations
	for _, pod := range newPods {
		dsc.updateExp.ObserveUpdated(key, cur.Labels[apps.DefaultDaemonSetUniqueLabelKey], pod)
	}

	return dsc.syncNodesWhenInplaceUpdate(ds, oldPodsToInplaceUpdate, hash, old, cur)
}

func (dsc *ReconcileDaemonSet) syncNodesWhenInplaceUpdate(ds *appsv1alpha1.DaemonSet, oldPodsToInplaceUpdate []*corev1.Pod, hash string, oldRevisions []*apps.ControllerRevision, cur *apps.ControllerRevision) (reconcile.Result, error) {
	updateDiff := len(oldPodsToInplaceUpdate)

	burstReplicas := getBurstReplicas(ds)
	if updateDiff > burstReplicas {
		updateDiff = burstReplicas
	}

	// error channel to communicate back failures.  make the buffer big enough to avoid any blocking
	errCh := make(chan error, updateDiff)
	deletePodsCh := make(chan string, updateDiff)

	klog.V(6).Infof("Daemon set %s/%s: inplace updating %d pods", ds.Namespace, ds.Name, updateDiff)
	updateWait := sync.WaitGroup{}
	updateWait.Add(updateDiff)
	for i := 0; i < updateDiff; i++ {
		go func(ix int, ds *appsv1alpha1.DaemonSet, pod *corev1.Pod) {
			defer updateWait.Done()

			var old *apps.ControllerRevision
			for _, r := range oldRevisions {
				if dsc.revisionAdapter.EqualToRevisionHash("", pod, getShortHash(r.Name)) {
					old = r
					break
				}
			}
			res := dsc.inplaceControl.Update(pod, old, cur, &inplaceupdate.UpdateOptions{GetRevision: func(rev *apps.ControllerRevision) string {
				return rev.Labels[apps.DefaultDaemonSetUniqueLabelKey]
			}})
			if res.InPlaceUpdate {
				if res.UpdateErr == nil {
					dsc.eventRecorder.Eventf(ds, corev1.EventTypeNormal, "SuccessfulUpdatePodInPlace", "successfully update pod %s in-place", pod.Name)
					dsKey, err := kubecontroller.KeyFunc(ds)
					if err != nil {
						klog.Errorf("couldn't get key for object %#v: %v", ds, err)
						return
					}
					dsc.updateExp.ExpectUpdated(dsKey, cur.Name, pod)
					return
				}

				klog.Warningf("DaemonSet %s/%s failed to in-place update Pod %s, so it will back off to ReCreate", ds.Namespace, ds.Name, pod.Name)
				errCh <- res.UpdateErr
				utilruntime.HandleError(res.UpdateErr)
				return
			} else {
				deletePodsCh <- pod.Name
			}
		}(i, ds, oldPodsToInplaceUpdate[i])
	}
	updateWait.Wait()

	// Some pods did not update, which means the upgrading contains replace operations other than container image
	// and cannot use inplace-update. Mark these pods as deleted and delete them in syncNodes method.
	if len(deletePodsCh) > 0 {
		podsToDelete := make([]string, 0)
		close(deletePodsCh)
		for pod := range deletePodsCh {
			podsToDelete = append(podsToDelete, pod)
		}
		klog.V(6).Infof("DaemonSet %s/%s found %d pods cannot use inplace update, delete and recreate them", ds.Namespace, ds.Name, len(podsToDelete))
		if err := dsc.syncNodes(ds, podsToDelete, []string{}, hash); err != nil {
			klog.Warningf("DaemonSet %s/%s delete pods that cannot use inplace update error: %v", ds.Namespace, ds.Name, err)
			errCh <- err
		}
	}

	// collect errors if any for proper reporting/retry logic in the controller
	var errors []error
	close(errCh)
	for err := range errCh {
		errors = append(errors, err)
	}

	return reconcile.Result{}, utilerrors.NewAggregate(errors)
}
