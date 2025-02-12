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

package statefulset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	apiutil "github.com/openkruise/kruise/pkg/util/api"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/history"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	"github.com/openkruise/kruise/pkg/util/revision"
)

var patchCodec = scheme.Codecs.LegacyCodec(appsv1beta1.SchemeGroupVersion)

// statefulPodRegex is a regular expression that extracts the parent StatefulSet and ordinal from the Name of a Pod
var statefulPodRegex = regexp.MustCompile("(.*)-([0-9]+)$")

// getParentNameAndOrdinal gets the name of pod's parent StatefulSet and pod's ordinal as extracted from its Name. If
// the Pod was not created by a StatefulSet, its parent is considered to be empty string, and its ordinal is considered
// to be -1.
func getParentNameAndOrdinal(pod *v1.Pod) (string, int) {
	parent := ""
	ordinal := -1
	subMatches := statefulPodRegex.FindStringSubmatch(pod.Name)
	if len(subMatches) < 3 {
		return parent, ordinal
	}
	parent = subMatches[1]
	if i, err := strconv.ParseInt(subMatches[2], 10, 32); err == nil {
		ordinal = int(i)
	}
	return parent, ordinal
}

// getParentName gets the name of pod's parent StatefulSet. If pod has not parent, the empty string is returned.
func getParentName(pod *v1.Pod) string {
	parent, _ := getParentNameAndOrdinal(pod)
	return parent
}

// getOrdinal gets pod's ordinal. If pod has no ordinal, -1 is returned.
func getOrdinal(pod *v1.Pod) int {
	_, ordinal := getParentNameAndOrdinal(pod)
	return ordinal
}

/**
 * Determines if the given pod's ordinal number is within the permissible range
 * managed by this StatefulSet and is not listed in the reserveOrdinals.
 *
 * @return {boolean} True if the pod's ordinal is both within the allowed range and
 *                   not reserved; false otherwise.
 */
func podInOrdinalRange(pod *v1.Pod, set *appsv1beta1.StatefulSet) bool {
	startOrdinal, endOrdinal, reserveOrdinals := getStatefulSetReplicasRange(set)
	return podInOrdinalRangeWithParams(pod, startOrdinal, endOrdinal, reserveOrdinals)
}

func podInOrdinalRangeWithParams(pod *v1.Pod, startOrdinal, endOrdinal int, reserveOrdinals sets.Set[int]) bool {
	ordinal := getOrdinal(pod)
	return ordinal >= startOrdinal && ordinal < endOrdinal &&
		!reserveOrdinals.Has(ordinal)
}

// getPodName gets the name of set's child Pod with an ordinal index of ordinal
func getPodName(set *appsv1beta1.StatefulSet, ordinal int) string {
	return fmt.Sprintf("%s-%d", set.Name, ordinal)
}

// getPersistentVolumeClaimName gets the name of PersistentVolumeClaim for a Pod with an ordinal index of ordinal. claim
// must be a PersistentVolumeClaim from set's VolumeClaimTemplates template.
func getPersistentVolumeClaimName(set *appsv1beta1.StatefulSet, claim *v1.PersistentVolumeClaim, ordinal int) string {
	// NOTE: This name format is used by the heuristics for zone spreading in ChooseZoneForVolume
	return fmt.Sprintf("%s-%s-%d", claim.Name, set.Name, ordinal)
}

// isMemberOf tests if pod is a member of set.
func isMemberOf(set *appsv1beta1.StatefulSet, pod *v1.Pod) bool {
	return getParentName(pod) == set.Name
}

// identityMatches returns true if pod has a valid identity and network identity for a member of set.
func identityMatches(set *appsv1beta1.StatefulSet, pod *v1.Pod) bool {
	parent, ordinal := getParentNameAndOrdinal(pod)
	return ordinal >= 0 &&
		set.Name == parent &&
		pod.Name == getPodName(set, ordinal) &&
		pod.Namespace == set.Namespace &&
		pod.Labels[apps.StatefulSetPodNameLabel] == pod.Name
}

// storageMatches returns true if pod's Volumes cover the set of PersistentVolumeClaims
func storageMatches(set *appsv1beta1.StatefulSet, pod *v1.Pod) bool {
	ordinal := getOrdinal(pod)
	if ordinal < 0 {
		return false
	}
	volumes := make(map[string]v1.Volume, len(pod.Spec.Volumes))
	for _, volume := range pod.Spec.Volumes {
		volumes[volume.Name] = volume
	}
	for _, claim := range set.Spec.VolumeClaimTemplates {
		volume, found := volumes[claim.Name]
		if !found ||
			volume.VolumeSource.PersistentVolumeClaim == nil ||
			volume.VolumeSource.PersistentVolumeClaim.ClaimName !=
				getPersistentVolumeClaimName(set, &claim, ordinal) {
			return false
		}
	}
	return true
}

// getPersistentVolumeClaimPolicy returns the PVC policy for a StatefulSet, returning a retain policy if the set policy is nil.
func getPersistentVolumeClaimRetentionPolicy(set *appsv1beta1.StatefulSet) appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy {
	policy := appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
		WhenDeleted: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
		WhenScaled:  appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
	}
	if set.Spec.PersistentVolumeClaimRetentionPolicy != nil {
		policy = *set.Spec.PersistentVolumeClaimRetentionPolicy
	}
	return policy
}

// claimOwnerMatchesSetAndPod returns false if the ownerRefs of the claim are not set consistently with the
// PVC deletion policy for the StatefulSet.
func claimOwnerMatchesSetAndPod(claim *v1.PersistentVolumeClaim, set *appsv1beta1.StatefulSet, pod *v1.Pod) bool {
	policy := getPersistentVolumeClaimRetentionPolicy(set)
	const retain = appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType
	const delete = appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType
	switch {
	default:
		klog.InfoS("Unknown policy, treating as Retain", "policy", set.Spec.PersistentVolumeClaimRetentionPolicy)
		fallthrough
	case policy.WhenScaled == retain && policy.WhenDeleted == retain:
		if hasOwnerRef(claim, set) ||
			hasOwnerRef(claim, pod) {
			return false
		}
	case policy.WhenScaled == retain && policy.WhenDeleted == delete:
		if !hasOwnerRef(claim, set) ||
			hasOwnerRef(claim, pod) {
			return false
		}
	case policy.WhenScaled == delete && policy.WhenDeleted == retain:
		if hasOwnerRef(claim, set) {
			return false
		}
		podScaledDown := !podInOrdinalRange(pod, set)
		if podScaledDown != hasOwnerRef(claim, pod) {
			return false
		}
	case policy.WhenScaled == delete && policy.WhenDeleted == delete:
		podScaledDown := !podInOrdinalRange(pod, set)
		// If a pod is scaled down, there should be no set ref and a pod ref;
		// if the pod is not scaled down it's the other way around.
		if podScaledDown == hasOwnerRef(claim, set) {
			return false
		}
		if podScaledDown != hasOwnerRef(claim, pod) {
			return false
		}
	}
	return true
}

// updateClaimOwnerRefForSetAndPod updates the ownerRefs for the claim according to the deletion policy of
// the StatefulSet. Returns true if the claim was changed and should be updated and false otherwise.
func updateClaimOwnerRefForSetAndPod(claim *v1.PersistentVolumeClaim, set *appsv1beta1.StatefulSet, pod *v1.Pod) bool {
	needsUpdate := false
	// Sometimes the version and kind are not set {pod,set}.TypeMeta. These are necessary for the ownerRef.
	// This is the case both in real clusters and the unittests.
	// TODO: there must be a better way to do this other than hardcoding the pod version?
	updateMeta := func(tm *metav1.TypeMeta, kind string) {
		if tm.APIVersion == "" {
			if kind == "StatefulSet" {
				tm.APIVersion = "apps.kruise.io/v1beta1"
			} else {
				tm.APIVersion = "v1"
			}
		}
		if tm.Kind == "" {
			tm.Kind = kind
		}
	}
	podMeta := pod.TypeMeta
	updateMeta(&podMeta, "Pod")
	setMeta := set.TypeMeta
	updateMeta(&setMeta, "StatefulSet")
	policy := getPersistentVolumeClaimRetentionPolicy(set)
	const retain = appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType
	const delete = appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType
	switch {
	default:
		klog.InfoS("Unknown policy, treating as Retain", "policy", set.Spec.PersistentVolumeClaimRetentionPolicy)
		fallthrough
	case policy.WhenScaled == retain && policy.WhenDeleted == retain:
		needsUpdate = removeOwnerRef(claim, set) || needsUpdate
		needsUpdate = removeOwnerRef(claim, pod) || needsUpdate
	case policy.WhenScaled == retain && policy.WhenDeleted == delete:
		needsUpdate = setOwnerRef(claim, set, &setMeta) || needsUpdate
		needsUpdate = removeOwnerRef(claim, pod) || needsUpdate
	case policy.WhenScaled == delete && policy.WhenDeleted == retain:
		needsUpdate = removeOwnerRef(claim, set) || needsUpdate
		podScaledDown := !podInOrdinalRange(pod, set)
		if podScaledDown {
			needsUpdate = setOwnerRef(claim, pod, &podMeta) || needsUpdate
		}
		if !podScaledDown {
			needsUpdate = removeOwnerRef(claim, pod) || needsUpdate
		}
	case policy.WhenScaled == delete && policy.WhenDeleted == delete:
		podScaledDown := !podInOrdinalRange(pod, set)
		if podScaledDown {
			needsUpdate = removeOwnerRef(claim, set) || needsUpdate
			needsUpdate = setOwnerRef(claim, pod, &podMeta) || needsUpdate
		}
		if !podScaledDown {
			needsUpdate = setOwnerRef(claim, set, &setMeta) || needsUpdate
			needsUpdate = removeOwnerRef(claim, pod) || needsUpdate
		}
	}
	return needsUpdate
}

// hasOwnerRef returns true if target has an ownerRef to owner.
func hasOwnerRef(target, owner metav1.Object) bool {
	ownerUID := owner.GetUID()
	for _, ownerRef := range target.GetOwnerReferences() {
		if ownerRef.UID == ownerUID {
			return true
		}
	}
	return false
}

// hasStaleOwnerRef returns true if target has a ref to owner that appears to be stale.
func hasStaleOwnerRef(target, owner metav1.Object) bool {
	for _, ownerRef := range target.GetOwnerReferences() {
		if ownerRef.Name == owner.GetName() && ownerRef.UID != owner.GetUID() {
			return true
		}
	}
	return false
}

// setOwnerRef adds owner to the ownerRefs of target, if necessary. Returns true if target needs to be
// updated and false otherwise.
func setOwnerRef(target, owner metav1.Object, ownerType *metav1.TypeMeta) bool {
	if hasOwnerRef(target, owner) {
		return false
	}
	ownerRefs := append(
		target.GetOwnerReferences(),
		metav1.OwnerReference{
			APIVersion: ownerType.APIVersion,
			Kind:       ownerType.Kind,
			Name:       owner.GetName(),
			UID:        owner.GetUID(),
		})
	target.SetOwnerReferences(ownerRefs)
	return true
}

// removeOwnerRef removes owner from the ownerRefs of target, if necessary. Returns true if target needs
// to be updated and false otherwise.
func removeOwnerRef(target, owner metav1.Object) bool {
	if !hasOwnerRef(target, owner) {
		return false
	}
	ownerUID := owner.GetUID()
	oldRefs := target.GetOwnerReferences()
	newRefs := make([]metav1.OwnerReference, len(oldRefs)-1)
	skip := 0
	for i := range oldRefs {
		if oldRefs[i].UID == ownerUID {
			skip = -1
		} else {
			newRefs[i+skip] = oldRefs[i]
		}
	}
	target.SetOwnerReferences(newRefs)
	return true
}

// getPersistentVolumeClaims gets a map of PersistentVolumeClaims to their template names, as defined in set. The
// returned PersistentVolumeClaims are each constructed with a the name specific to the Pod. This name is determined
// by getPersistentVolumeClaimName.
func getPersistentVolumeClaims(set *appsv1beta1.StatefulSet, pod *v1.Pod) map[string]v1.PersistentVolumeClaim {
	ordinal := getOrdinal(pod)
	templates := set.Spec.VolumeClaimTemplates
	claims := make(map[string]v1.PersistentVolumeClaim, len(templates))
	for i := range templates {
		claim := templates[i]
		claim.Name = getPersistentVolumeClaimName(set, &claim, ordinal)
		claim.Namespace = set.Namespace
		claim.Labels = set.Spec.Selector.MatchLabels
		claims[templates[i].Name] = claim
	}
	return claims
}

// updateStorage updates pod's Volumes to conform with the PersistentVolumeClaim of set's templates. If pod has
// conflicting local Volumes these are replaced with Volumes that conform to the set's templates.
func updateStorage(set *appsv1beta1.StatefulSet, pod *v1.Pod) {
	currentVolumes := pod.Spec.Volumes
	claims := getPersistentVolumeClaims(set, pod)
	newVolumes := make([]v1.Volume, 0, len(claims))
	for name, claim := range claims {
		newVolumes = append(newVolumes, v1.Volume{
			Name: name,
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: claim.Name,
					// TODO: Use source definition to set this value when we have one.
					ReadOnly: false,
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

func initIdentity(set *appsv1beta1.StatefulSet, pod *v1.Pod) {
	updateIdentity(set, pod)
	// Set these immutable fields only on initial Pod creation, not updates.
	pod.Spec.Hostname = pod.Name
	pod.Spec.Subdomain = set.Spec.ServiceName
}

// updateIdentity updates pod's name, hostname, and subdomain, and StatefulSetPodNameLabel to conform to set's name
// and headless service.
func updateIdentity(set *appsv1beta1.StatefulSet, pod *v1.Pod) {
	ordinal := getOrdinal(pod)
	pod.Name = getPodName(set, ordinal)
	pod.Namespace = set.Namespace
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels[apps.StatefulSetPodNameLabel] = pod.Name
	if utilfeature.DefaultFeatureGate.Enabled(features.PodIndexLabel) {
		pod.Labels[apps.PodIndexLabel] = strconv.Itoa(ordinal)
	}
}

// isRunningAndAvailable returns true if pod is in the PodRunning Phase,
// and it has a condition of PodReady for a minimum of minReadySeconds.
// return true if it's available
// return false with zero means it's not ready
// return false with a positive value means it's not available and should recheck with that time
func isRunningAndAvailable(pod *v1.Pod, minReadySeconds int32) (bool, time.Duration) {
	state := lifecycle.GetPodLifecycleState(pod)
	if state != "" && state != appspub.LifecycleStateNormal {
		// when state exists and is not normal, it is unavailable
		return false, 0
	}
	if pod.Status.Phase != v1.PodRunning || !podutil.IsPodReady(pod) {
		return false, 0
	}
	c := podutil.GetPodReadyCondition(pod.Status)
	minReadySecondsDuration := time.Duration(minReadySeconds) * time.Second
	if minReadySeconds == 0 {
		return true, 0
	}
	if c.LastTransitionTime.IsZero() {
		return false, minReadySecondsDuration
	}
	waitTime := c.LastTransitionTime.Time.Add(minReadySecondsDuration).Sub(time.Now())
	if waitTime > 0 {
		return false, waitTime
	}
	return true, 0
}

// isRunningAndReady returns true if pod is in the PodRunning Phase, if it has a condition of PodReady.
func isRunningAndReady(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodRunning && podutil.IsPodReady(pod)
}

// isCreated returns true if pod has been created and is maintained by the API server
func isCreated(pod *v1.Pod) bool {
	return pod.Status.Phase != ""
}

// isPending returns true if pod has a Phase of PodPending
func isPending(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodPending
}

// isFailed returns true if pod has a Phase of PodFailed
func isFailed(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodFailed
}

// isSucceeded returns true if pod has a Phase of PodSucceeded
func isSucceeded(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodSucceeded
}

// isTerminating returns true if pod's DeletionTimestamp has been set
func isTerminating(pod *v1.Pod) bool {
	return pod.DeletionTimestamp != nil
}

// isHealthy returns true if pod is running and ready and has not been terminated
func isHealthy(pod *v1.Pod) bool {
	state := lifecycle.GetPodLifecycleState(pod)
	if state != "" && state != appspub.LifecycleStateNormal {
		return false
	}
	return isRunningAndReady(pod) && !isTerminating(pod)
}

// allowsBurst is true if the alpha burst annotation is set.
func allowsBurst(set *appsv1beta1.StatefulSet) bool {
	return set.Spec.PodManagementPolicy == apps.ParallelPodManagement
}

// getMinReadySeconds returns the minReadySeconds set in the rollingUpdate, default is 0
func getMinReadySeconds(set *appsv1beta1.StatefulSet) int32 {
	if set.Spec.UpdateStrategy.RollingUpdate == nil ||
		set.Spec.UpdateStrategy.RollingUpdate.MinReadySeconds == nil {
		return 0
	}
	return *set.Spec.UpdateStrategy.RollingUpdate.MinReadySeconds
}

// setPodRevision sets the revision of Pod to revision by adding the StatefulSetRevisionLabel
func setPodRevision(pod *v1.Pod, revision string) {
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels[apps.StatefulSetRevisionLabel] = revision
}

// getPodRevision gets the revision of Pod by inspecting the StatefulSetRevisionLabel. If pod has no revision the empty
// string is returned.
func getPodRevision(pod *v1.Pod) string {
	if pod.Labels == nil {
		return ""
	}
	return pod.Labels[apps.StatefulSetRevisionLabel]
}

// newStatefulSetPod returns a new Pod conforming to the set's Spec with an identity generated from ordinal.
func newStatefulSetPod(set *appsv1beta1.StatefulSet, ordinal int) *v1.Pod {
	pod, _ := controller.GetPodFromTemplate(&set.Spec.Template, set, metav1.NewControllerRef(set, controllerKind))
	pod.Name = getPodName(set, ordinal)
	initIdentity(set, pod)
	updateStorage(set, pod)
	return pod
}

// newVersionedStatefulSetPod creates a new Pod for a StatefulSet. currentSet is the representation of the set at the
// current revision. updateSet is the representation of the set at the updateRevision. currentRevision is the name of
// the current revision. updateRevision is the name of the update revision. ordinal is the ordinal of the Pod. If the
// returned error is nil, the returned Pod is valid.
func newVersionedStatefulSetPod(currentSet, updateSet *appsv1beta1.StatefulSet, currentRevision, updateRevision string,
	ordinal int, replicas []*v1.Pod,
) *v1.Pod {
	if isCurrentRevisionNeeded(currentSet, updateRevision, ordinal, replicas) {
		pod := newStatefulSetPod(currentSet, ordinal)
		setPodRevision(pod, currentRevision)
		return pod
	}
	pod := newStatefulSetPod(updateSet, ordinal)
	setPodRevision(pod, updateRevision)
	return pod
}

// isCurrentRevisionNeeded calculate if the 'ordinal' Pod should be current revision.
func isCurrentRevisionNeeded(set *appsv1beta1.StatefulSet, updateRevision string, ordinal int, replicas []*v1.Pod) bool {
	if set.Spec.UpdateStrategy.Type != apps.RollingUpdateStatefulSetStrategyType {
		return false
	}
	if set.Spec.UpdateStrategy.RollingUpdate == nil {
		return ordinal < getStartOrdinal(set)+int(set.Status.CurrentReplicas)
	}
	if set.Spec.UpdateStrategy.RollingUpdate.UnorderedUpdate == nil {
		unreservedPodsNum := 0
		// assume all pods [0, idx) are created and only reserved pods are nil
		idx := ordinal - getStartOrdinal(set)
		for i := 0; i < idx; i++ {
			if replicas[i] != nil {
				unreservedPodsNum++
			}
		}
		// if all pods [0, idx] are current revision
		return unreservedPodsNum+1 <= int(*set.Spec.UpdateStrategy.RollingUpdate.Partition)
	}

	var noUpdatedReplicas int
	for _, pod := range replicas {
		if pod == nil || getOrdinal(pod) == ordinal {
			continue
		}
		if !revision.IsPodUpdate(pod, updateRevision) {
			noUpdatedReplicas++
		}
	}
	return noUpdatedReplicas < int(*set.Spec.UpdateStrategy.RollingUpdate.Partition)
}

// Match check if the given StatefulSet's template matches the template stored in the given history.
func Match(ss *appsv1beta1.StatefulSet, history *apps.ControllerRevision) (bool, error) {
	// Encoding the set for the patch may update its GVK metadata, which causes data races if this
	// set is in an informer cache.
	clone := ss.DeepCopy()
	patch, err := getPatch(clone)
	if err != nil {
		return false, err
	}
	return bytes.Equal(patch, history.Data.Raw), nil
}

// getPatch returns a strategic merge patch that can be applied to restore a StatefulSet to a
// previous version. If the returned error is nil the patch is valid. The current state that we save is just the
// PodSpecTemplate. We can modify this later to encompass more state (or less) and remain compatible with previously
// recorded patches.
func getPatch(set *appsv1beta1.StatefulSet) ([]byte, error) {
	str, err := runtime.Encode(patchCodec, set)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	err = json.Unmarshal(str, &raw)
	if err != nil {
		return nil, err
	}
	objCopy := make(map[string]interface{})
	specCopy := make(map[string]interface{})
	spec := raw["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	specCopy["template"] = template
	template["$patch"] = "replace"
	objCopy["spec"] = specCopy
	patch, err := json.Marshal(objCopy)
	return patch, err
}

// newRevision creates a new ControllerRevision containing a patch that reapplies the target state of set.
// The Revision of the returned ControllerRevision is set to revision. If the returned error is nil, the returned
// ControllerRevision is valid. StatefulSet revisions are stored as patches that re-apply the current state of set
// to a new StatefulSet using a strategic merge patch to replace the saved state of the new StatefulSet.
func newRevision(set *appsv1beta1.StatefulSet, revision int64, collisionCount *int32) (*apps.ControllerRevision, error) {
	patch, err := getPatch(set)
	if err != nil {
		return nil, err
	}
	cr, err := history.NewControllerRevision(set,
		controllerKind,
		set.Spec.Template.Labels,
		runtime.RawExtension{Raw: patch},
		revision,
		collisionCount)
	if err != nil {
		return nil, err
	}
	if cr.ObjectMeta.Annotations == nil {
		cr.ObjectMeta.Annotations = make(map[string]string)
	}
	for key, value := range set.Annotations {
		cr.ObjectMeta.Annotations[key] = value
	}
	return cr, nil
}

// ApplyRevision returns a new StatefulSet constructed by restoring the state in revision to set. If the returned error
// is nil, the returned StatefulSet is valid.
func ApplyRevision(set *appsv1beta1.StatefulSet, revision *apps.ControllerRevision) (*appsv1beta1.StatefulSet, error) {
	clone := set.DeepCopy()
	patched, err := strategicpatch.StrategicMergePatch([]byte(runtime.EncodeOrDie(patchCodec, clone)), revision.Data.Raw, clone)
	if err != nil {
		return nil, err
	}
	restoredSet := &appsv1beta1.StatefulSet{}
	err = json.Unmarshal(patched, restoredSet)
	if err != nil {
		return nil, err
	}
	return restoredSet, nil
}

// nextRevision finds the next valid revision number based on revisions. If the length of revisions
// is 0 this is 1. Otherwise, it is 1 greater than the largest revision's Revision. This method
// assumes that revisions has been sorted by Revision.
func nextRevision(revisions []*apps.ControllerRevision) int64 {
	count := len(revisions)
	if count <= 0 {
		return 1
	}
	return revisions[count-1].Revision + 1
}

// inconsistentStatus returns true if the ObservedGeneration of status is greater than set's
// Generation or if any of the status's fields do not match those of set's status.
func inconsistentStatus(set *appsv1beta1.StatefulSet, status *appsv1beta1.StatefulSetStatus) bool {
	if status.ObservedGeneration > set.Status.ObservedGeneration ||
		status.Replicas != set.Status.Replicas ||
		status.CurrentReplicas != set.Status.CurrentReplicas ||
		status.ReadyReplicas != set.Status.ReadyReplicas ||
		status.AvailableReplicas != set.Status.AvailableReplicas ||
		status.UpdatedReplicas != set.Status.UpdatedReplicas ||
		status.CurrentRevision != set.Status.CurrentRevision ||
		status.UpdateRevision != set.Status.UpdateRevision ||
		status.LabelSelector != set.Status.LabelSelector {
		return true
	}

	volumeClaimName2StatusIdx := map[string]int{}
	for i, v := range status.VolumeClaims {
		volumeClaimName2StatusIdx[v.VolumeClaimName] = i
	}
	for _, v := range set.Status.VolumeClaims {
		if idx, ok := volumeClaimName2StatusIdx[v.VolumeClaimName]; !ok {
			// raw template not exist in current status => inconsistent
			return true
		} else if status.VolumeClaims[idx].CompatibleReplicas != v.CompatibleReplicas ||
			status.VolumeClaims[idx].CompatibleReadyReplicas != v.CompatibleReadyReplicas {
			return true
		}
	}
	return false
}

// completeRollingUpdate completes a rolling update when all of set's replica Pods have been updated
// to the updateRevision. status's currentRevision is set to updateRevision and its' updateRevision
// is set to the empty string. status's currentReplicas is set to updateReplicas and its updateReplicas
// are set to 0.
func completeRollingUpdate(set *appsv1beta1.StatefulSet, status *appsv1beta1.StatefulSetStatus) {
	if set.Spec.UpdateStrategy.Type == apps.RollingUpdateStatefulSetStrategyType &&
		status.UpdatedReplicas == status.Replicas &&
		status.ReadyReplicas == status.Replicas {
		status.CurrentReplicas = status.UpdatedReplicas
		status.CurrentRevision = status.UpdateRevision
	}
}

// SortPodsAscendingOrdinal sorts the given Pods according to their ordinals.
func SortPodsAscendingOrdinal(pods []*v1.Pod) {
	sort.Sort(ascendingOrdinal(pods))
}

// ascendingOrdinal is a sort.Interface that Sorts a list of Pods based on the ordinals extracted
// from the Pod. Pod's that have not been constructed by StatefulSet's have an ordinal of -1, and are therefore pushed
// to the front of the list.
type ascendingOrdinal []*v1.Pod

func (ao ascendingOrdinal) Len() int {
	return len(ao)
}

func (ao ascendingOrdinal) Swap(i, j int) {
	ao[i], ao[j] = ao[j], ao[i]
}

func (ao ascendingOrdinal) Less(i, j int) bool {
	return getOrdinal(ao[i]) < getOrdinal(ao[j])
}

type descendingOrdinal []*v1.Pod

func (do descendingOrdinal) Len() int {
	return len(do)
}

func (do descendingOrdinal) Swap(i, j int) {
	do[i], do[j] = do[j], do[i]
}

func (do descendingOrdinal) Less(i, j int) bool {
	return getOrdinal(do[i]) > getOrdinal(do[j])
}

// NewStatefulsetCondition creates a new statefulset condition.
func NewStatefulsetCondition(conditionType apps.StatefulSetConditionType, conditionStatus v1.ConditionStatus, reason, message string) apps.StatefulSetCondition {
	return apps.StatefulSetCondition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// GetStatefulsetConditition returns the condition with the provided type.
func GetStatefulsetConditition(status appsv1beta1.StatefulSetStatus, condType apps.StatefulSetConditionType) *apps.StatefulSetCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// SetStatefulsetCondition updates the statefulset to include the provided condition. If the condition that
func SetStatefulsetCondition(status *appsv1beta1.StatefulSetStatus, condition apps.StatefulSetCondition) {
	currentCond := GetStatefulsetConditition(*status, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}
	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}

	newConditions := filterOutCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, condition)
}

func filterOutCondition(conditions []apps.StatefulSetCondition, condType apps.StatefulSetConditionType) []apps.StatefulSetCondition {
	var newCondititions []apps.StatefulSetCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newCondititions = append(newCondititions, c)
	}
	return newCondititions
}

func getStatefulSetKey(o metav1.Object) string {
	return o.GetNamespace() + "/" + o.GetName()
}

func decreaseAndCheckMaxUnavailable(maxUnavailable *int) bool {
	if maxUnavailable == nil {
		return false
	}
	val := *maxUnavailable - 1
	*maxUnavailable = val
	return val <= 0
}

// return parameters is startOrdinal(inclusive), endOrdinal(exclusive) and reserveOrdinals,
// and they are used to support reserveOrdinals scenarios.
// When configured as follows:
/*
	apiVersion: apps.kruise.io/v1beta1
	kind: StatefulSet
	spec:
	  # ...
	  replicas: 4
	  reserveOrdinals:
	  - 1
      - 3
      Spec.Ordinals.Start: 2
*/
// result is startOrdinal 2(inclusive), endOrdinal 7(exclusive), reserveOrdinals = {1, 3}
// replicas[endOrdinal - startOrdinal] stores [replica-2, nil(reserveOrdinal 3), replica-4, replica-5, replica-6]
// todo: maybe we should remove ineffective reserveOrdinals in webhook, reserveOrdinals = {3}
func getStatefulSetReplicasRange(set *appsv1beta1.StatefulSet) (int, int, sets.Set[int]) {
	reserveOrdinals := apiutil.GetReserveOrdinalIntSet(set.Spec.ReserveOrdinals)
	replicaMaxOrdinal := getStartOrdinal(set)
	for realReplicaCount := 0; realReplicaCount < int(*set.Spec.Replicas); replicaMaxOrdinal++ {
		if reserveOrdinals.Has(replicaMaxOrdinal) {
			continue
		}
		realReplicaCount++
	}
	return getStartOrdinal(set), replicaMaxOrdinal, reserveOrdinals
}
