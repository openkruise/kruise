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

package utils

import (
	"context"
	"fmt"
	"strings"
	"sync"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/utils/integer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/expectations"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/requeueduration"
	"github.com/openkruise/kruise/pkg/util/revision"
)

var (
	// ControllerKind is GroupVersionKind for CloneSet.
	ControllerKind      = appsv1alpha1.SchemeGroupVersion.WithKind("CloneSet")
	RevisionAdapterImpl = &revisionAdapterImpl{}
	EqualToRevisionHash = RevisionAdapterImpl.EqualToRevisionHash
	WriteRevisionHash   = RevisionAdapterImpl.WriteRevisionHash

	ScaleExpectations           = expectations.NewScaleExpectations()
	ResourceVersionExpectations = expectations.NewResourceVersionExpectation()

	// DurationStore is a short cut for any sub-functions to notify the reconcile how long to wait to requeue
	DurationStore = requeueduration.DurationStore{}
)

type revisionAdapterImpl struct {
}

func (r *revisionAdapterImpl) EqualToRevisionHash(_ string, obj metav1.Object, hash string) bool {
	objHash := obj.GetLabels()[apps.ControllerRevisionHashLabelKey]
	if objHash == hash {
		return true
	}
	return GetShortHash(hash) == GetShortHash(objHash)
}

func (r *revisionAdapterImpl) WriteRevisionHash(obj metav1.Object, hash string) {
	if obj.GetLabels() == nil {
		obj.SetLabels(make(map[string]string, 1))
	}
	// Note that controller-revision-hash defaults to be "{CLONESET_NAME}-{HASH}",
	// and it will be "{HASH}" if CloneSetShortHash feature-gate has been enabled.
	// But pod-template-hash should always be the short format.
	shortHash := GetShortHash(hash)
	if utilfeature.DefaultFeatureGate.Enabled(features.CloneSetShortHash) {
		obj.GetLabels()[apps.ControllerRevisionHashLabelKey] = shortHash
	} else {
		obj.GetLabels()[apps.ControllerRevisionHashLabelKey] = hash
	}
	obj.GetLabels()[apps.DefaultDeploymentUniqueLabelKey] = shortHash
}

func GetShortHash(hash string) string {
	// This makes sure the real hash must be the last '-' substring of revision name
	// vendor/k8s.io/kubernetes/pkg/controller/history/controller_history.go#82
	list := strings.Split(hash, "-")
	return list[len(list)-1]
}

// GetControllerKey return key of CloneSet.
func GetControllerKey(cs *appsv1alpha1.CloneSet) string {
	return types.NamespacedName{Namespace: cs.Namespace, Name: cs.Name}.String()
}

// GetActiveAndInactivePods get activePods and inactivePods
func GetActiveAndInactivePods(reader client.Reader, opts *client.ListOptions) ([]*v1.Pod, []*v1.Pod, error) {
	podList := &v1.PodList{}
	if err := reader.List(context.TODO(), podList, opts, utilclient.DisableDeepCopy); err != nil {
		return nil, nil, err
	}
	var activePods, inactivePods []*v1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if kubecontroller.IsPodActive(pod) {
			activePods = append(activePods, pod)
		} else {
			inactivePods = append(inactivePods, pod)
		}
	}
	return activePods, inactivePods, nil
}

// NextRevision finds the next valid revision number based on revisions. If the length of revisions
// is 0 this is 1. Otherwise, it is 1 greater than the largest revision's Revision. This method
// assumes that revisions has been sorted by Revision.
func NextRevision(revisions []*apps.ControllerRevision) int64 {
	count := len(revisions)
	if count <= 0 {
		return 1
	}
	return revisions[count-1].Revision + 1
}

// IsRunningAndReady returns true if pod is in the PodRunning Phase, if it is ready.
func IsRunningAndReady(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodRunning && podutil.IsPodReady(pod)
}

// IsRunningAndAvailable returns true if pod is in the PodRunning Phase, if it is available.
func IsRunningAndAvailable(pod *v1.Pod, minReadySeconds int32) bool {
	return pod.Status.Phase == v1.PodRunning && podutil.IsPodAvailable(pod, minReadySeconds, metav1.Now())
}

// SplitPodsByRevision returns Pods matched and unmatched the given revision
func SplitPodsByRevision(pods []*v1.Pod, rev string) (matched, unmatched []*v1.Pod) {
	for _, p := range pods {
		if EqualToRevisionHash("", p, rev) {
			matched = append(matched, p)
		} else {
			unmatched = append(unmatched, p)
		}
	}
	return
}

func GroupUpdateAndNotUpdatePods(pods []*v1.Pod, updateRevision string) (update, notUpdate []*v1.Pod) {
	for _, p := range pods {
		if revision.IsPodUpdate(p, updateRevision) {
			update = append(update, p)
		} else {
			notUpdate = append(notUpdate, p)
		}
	}
	return
}

// UpdateStorage insert volumes generated by cs.Spec.VolumeClaimTemplates into Pod.
func UpdateStorage(cs *appsv1alpha1.CloneSet, pod *v1.Pod) {
	currentVolumes := pod.Spec.Volumes
	claims := GetPersistentVolumeClaims(cs, pod)
	newVolumes := make([]v1.Volume, 0, len(claims))
	for name, claim := range claims {
		newVolumes = append(newVolumes, v1.Volume{
			Name: name,
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: claim.Name,
					ReadOnly:  false,
				},
			},
		})
	}
	for i := range currentVolumes {
		if _, ok := claims[currentVolumes[i].Name]; !ok {
			newVolumes = append(newVolumes, currentVolumes[i])
		}
	}
	pod.Spec.Volumes = newVolumes
}

func GetInstanceID(obj metav1.Object) string {
	return obj.GetLabels()[appsv1alpha1.CloneSetInstanceID]
}

// GetPersistentVolumeClaims gets a map of PersistentVolumeClaims to their template names, as defined in set. The
// returned PersistentVolumeClaims are each constructed with a the name specific to the Pod. This name is determined
// by getPersistentVolumeClaimName.
func GetPersistentVolumeClaims(cs *appsv1alpha1.CloneSet, pod *v1.Pod) map[string]v1.PersistentVolumeClaim {
	templates := cs.Spec.VolumeClaimTemplates
	claims := make(map[string]v1.PersistentVolumeClaim, len(templates))
	for i := range templates {
		claim := templates[i].DeepCopy()
		claim.Name = getPersistentVolumeClaimName(cs, claim, pod.Labels[appsv1alpha1.CloneSetInstanceID])
		claim.Namespace = cs.Namespace
		if claim.Labels == nil {
			claim.Labels = make(map[string]string)
		}
		for k, v := range cs.Spec.Selector.MatchLabels {
			claim.Labels[k] = v
		}
		claim.Labels[appsv1alpha1.CloneSetInstanceID] = pod.Labels[appsv1alpha1.CloneSetInstanceID]
		if ref := metav1.GetControllerOf(pod); ref != nil {
			// set pvc.owner to cloneSet
			claim.OwnerReferences = append(claim.OwnerReferences, *ref)
		}
		claims[templates[i].Name] = *claim
	}
	return claims
}

// getPersistentVolumeClaimName gets the name of PersistentVolumeClaim for a Pod with an instance id. claim
// must be a PersistentVolumeClaim from set's VolumeClaims template.
func getPersistentVolumeClaimName(cs *appsv1alpha1.CloneSet, claim *v1.PersistentVolumeClaim, id string) string {
	return fmt.Sprintf("%s-%s-%s", claim.Name, cs.Name, id)
}

// DoItSlowly tries to call the provided function a total of 'count' times,
// starting slow to check for errors, then speeding up if calls succeed.
//
// It groups the calls into batches, starting with a group of initialBatchSize.
// Within each batch, it may call the function multiple times concurrently.
//
// If a whole batch succeeds, the next batch may get exponentially larger.
// If there are any failures in a batch, all remaining batches are skipped
// after waiting for the current batch to complete.
//
// It returns the number of successful calls to the function.
func DoItSlowly(count int, initialBatchSize int, fn func() error) (int, error) {
	remaining := count
	successes := 0
	for batchSize := integer.IntMin(remaining, initialBatchSize); batchSize > 0; batchSize = integer.IntMin(2*batchSize, remaining) {
		errCh := make(chan error, batchSize)
		var wg sync.WaitGroup
		wg.Add(batchSize)
		for i := 0; i < batchSize; i++ {
			go func() {
				defer wg.Done()
				if err := fn(); err != nil {
					errCh <- err
				}
			}()
		}
		wg.Wait()
		curSuccesses := batchSize - len(errCh)
		successes += curSuccesses
		if len(errCh) > 0 {
			return successes, <-errCh
		}
		remaining -= batchSize
	}
	return successes, nil
}
