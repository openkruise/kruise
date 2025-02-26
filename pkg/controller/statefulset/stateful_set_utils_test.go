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

package statefulset

import (
	"fmt"
	"math/rand"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"testing"
	"time"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/controller/history"
	utilpointer "k8s.io/utils/pointer"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

// noopRecorder is an EventRecorder that does nothing. record.FakeRecorder has a fixed
// buffer size, which causes tests to hang if that buffer's exceeded.
type noopRecorder struct{}

func (r *noopRecorder) Event(object runtime.Object, eventtype, reason, message string) {}
func (r *noopRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
}
func (r *noopRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
}

// getClaimPodName gets the name of the Pod associated with the Claim, or an empty string if this doesn't look matching.
func getClaimPodName(set *appsv1beta1.StatefulSet, claim *corev1.PersistentVolumeClaim) string {
	podName := ""

	statefulClaimRegex := regexp.MustCompile(fmt.Sprintf(".*-(%s-[0-9]+)$", set.Name))
	matches := statefulClaimRegex.FindStringSubmatch(claim.Name)
	if len(matches) != 2 {
		return podName
	}
	return matches[1]
}

// overlappingStatefulSets sorts a list of StatefulSets by creation timestamp, using their names as a tie breaker.
// Generally used to tie break between StatefulSets that have overlapping selectors.
type overlappingStatefulSets []*appsv1beta1.StatefulSet

func (o overlappingStatefulSets) Len() int { return len(o) }

func (o overlappingStatefulSets) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o overlappingStatefulSets) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].CreationTimestamp.Before(&o[j].CreationTimestamp)
}

func TestGetParentNameAndOrdinal(t *testing.T) {
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 1)
	if parent, ordinal := getParentNameAndOrdinal(pod); parent != set.Name {
		t.Errorf("Extracted the wrong parent name expected %s found %s", set.Name, parent)
	} else if ordinal != 1 {
		t.Errorf("Extracted the wrong ordinal expected %d found %d", 1, ordinal)
	}
	pod.Name = "1-bar"
	if parent, ordinal := getParentNameAndOrdinal(pod); parent != "" {
		t.Error("Expected empty string for non-member Pod parent")
	} else if ordinal != -1 {
		t.Error("Expected -1 for non member Pod ordinal")
	}
}

func TestGetClaimPodName(t *testing.T) {
	set := appsv1beta1.StatefulSet{}
	set.Name = "my-set"
	claim := corev1.PersistentVolumeClaim{}
	claim.Name = "volume-my-set-2"
	if pod := getClaimPodName(&set, &claim); pod != "my-set-2" {
		t.Errorf("Expected my-set-2 found %s", pod)
	}
	claim.Name = "long-volume-my-set-20"
	if pod := getClaimPodName(&set, &claim); pod != "my-set-20" {
		t.Errorf("Expected my-set-20 found %s", pod)
	}
	claim.Name = "volume-2-my-set"
	if pod := getClaimPodName(&set, &claim); pod != "" {
		t.Errorf("Expected empty string found %s", pod)
	}
	claim.Name = "volume-pod-2"
	if pod := getClaimPodName(&set, &claim); pod != "" {
		t.Errorf("Expected empty string found %s", pod)
	}
}

func TestIsMemberOf(t *testing.T) {
	set := newStatefulSet(3)
	set2 := newStatefulSet(3)
	set2.Name = "foo2"
	pod := newStatefulSetPod(set, 1)
	if !isMemberOf(set, pod) {
		t.Error("isMemberOf returned false negative")
	}
	if isMemberOf(set2, pod) {
		t.Error("isMemberOf returned false positive")
	}
}

func TestIdentityMatches(t *testing.T) {
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 1)
	if !identityMatches(set, pod) {
		t.Error("Newly created Pod has a bad identity")
	}
	pod.Name = "foo"
	if identityMatches(set, pod) {
		t.Error("identity matches for a Pod with the wrong name")
	}
	pod = newStatefulSetPod(set, 1)
	pod.Namespace = ""
	if identityMatches(set, pod) {
		t.Error("identity matches for a Pod with the wrong namespace")
	}
	pod = newStatefulSetPod(set, 1)
	delete(pod.Labels, apps.StatefulSetPodNameLabel)
	if identityMatches(set, pod) {
		t.Error("identity matches for a Pod with the wrong statefulSetPodNameLabel")
	}
}

func TestStorageMatches(t *testing.T) {
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 1)
	if !storageMatches(set, pod) {
		t.Error("Newly created Pod has a invalid storage")
	}
	pod.Spec.Volumes = nil
	if storageMatches(set, pod) {
		t.Error("Pod with invalid Volumes has valid storage")
	}
	pod = newStatefulSetPod(set, 1)
	for i := range pod.Spec.Volumes {
		pod.Spec.Volumes[i].PersistentVolumeClaim = nil
	}
	if storageMatches(set, pod) {
		t.Error("Pod with invalid Volumes claim valid storage")
	}
	pod = newStatefulSetPod(set, 1)
	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].PersistentVolumeClaim != nil {
			pod.Spec.Volumes[i].PersistentVolumeClaim.ClaimName = "foo"
		}
	}
	if storageMatches(set, pod) {
		t.Error("Pod with invalid Volumes claim valid storage")
	}
	pod = newStatefulSetPod(set, 1)
	pod.Name = "bar"
	if storageMatches(set, pod) {
		t.Error("Pod with invalid ordinal has valid storage")
	}
}

func TestUpdateIdentity(t *testing.T) {
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 1)
	if !identityMatches(set, pod) {
		t.Error("Newly created Pod has a bad identity")
	}
	pod.Namespace = ""
	if identityMatches(set, pod) {
		t.Error("identity matches for a Pod with the wrong namespace")
	}
	updateIdentity(set, pod)
	if !identityMatches(set, pod) {
		t.Error("updateIdentity failed to update the Pods namespace")
	}
	delete(pod.Labels, apps.StatefulSetPodNameLabel)
	updateIdentity(set, pod)
	if !identityMatches(set, pod) {
		t.Error("updateIdentity failed to restore the statefulSetPodName label")
	}
}

func TestUpdateIdentityWithPodIndexLabel(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.PodIndexLabel, true)()

	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 1)
	updateIdentity(set, pod)

	podIndexFromLabel, exists := pod.Labels[apps.PodIndexLabel]
	if !exists {
		t.Errorf("Missing pod index label: %s", apps.PodIndexLabel)
	}
	podIndexFromName := strconv.Itoa(getOrdinal(pod))
	if podIndexFromLabel != podIndexFromName {
		t.Errorf("Pod index label value (%s) does not match pod index in pod name (%s)", podIndexFromLabel, podIndexFromName)
	}
}

func TestUpdateStorage(t *testing.T) {
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 1)
	if !storageMatches(set, pod) {
		t.Error("Newly created Pod has a invalid storage")
	}
	pod.Spec.Volumes = nil
	if storageMatches(set, pod) {
		t.Error("Pod with invalid Volumes has valid storage")
	}
	updateStorage(set, pod)
	if !storageMatches(set, pod) {
		t.Error("updateStorage failed to recreate volumes")
	}
	pod = newStatefulSetPod(set, 1)
	for i := range pod.Spec.Volumes {
		pod.Spec.Volumes[i].PersistentVolumeClaim = nil
	}
	if storageMatches(set, pod) {
		t.Error("Pod with invalid Volumes claim valid storage")
	}
	updateStorage(set, pod)
	if !storageMatches(set, pod) {
		t.Error("updateStorage failed to recreate volume claims")
	}
	pod = newStatefulSetPod(set, 1)
	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].PersistentVolumeClaim != nil {
			pod.Spec.Volumes[i].PersistentVolumeClaim.ClaimName = "foo"
		}
	}
	if storageMatches(set, pod) {
		t.Error("Pod with invalid Volumes claim valid storage")
	}
	updateStorage(set, pod)
	if !storageMatches(set, pod) {
		t.Error("updateStorage failed to recreate volume claim names")
	}
}

func TestGetPersistentVolumeClaimRetentionPolicy(t *testing.T) {
	retainPolicy := appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
		WhenScaled:  appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
		WhenDeleted: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
	}
	scaledownPolicy := appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
		WhenScaled:  appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
		WhenDeleted: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
	}

	set := appsv1beta1.StatefulSet{}
	set.Spec.PersistentVolumeClaimRetentionPolicy = &retainPolicy
	got := getPersistentVolumeClaimRetentionPolicy(&set)
	if got.WhenScaled != appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType || got.WhenDeleted != appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType {
		t.Errorf("Expected retain policy")
	}
	set.Spec.PersistentVolumeClaimRetentionPolicy = &scaledownPolicy
	got = getPersistentVolumeClaimRetentionPolicy(&set)
	if got.WhenScaled != appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType || got.WhenDeleted != appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType {
		t.Errorf("Expected scaledown policy")
	}
}

func TestClaimOwnerMatchesSetAndPod(t *testing.T) {
	testCases := []struct {
		name            string
		scaleDownPolicy appsv1beta1.PersistentVolumeClaimRetentionPolicyType
		setDeletePolicy appsv1beta1.PersistentVolumeClaimRetentionPolicyType
		needsPodRef     bool
		needsSetRef     bool
		replicas        int32
		ordinal         int
	}{
		{
			name:            "retain",
			scaleDownPolicy: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			setDeletePolicy: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			needsPodRef:     false,
			needsSetRef:     false,
		},
		{
			name:            "on SS delete",
			scaleDownPolicy: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			setDeletePolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			needsPodRef:     false,
			needsSetRef:     true,
		},
		{
			name:            "on scaledown only, condemned",
			scaleDownPolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			setDeletePolicy: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			needsPodRef:     true,
			needsSetRef:     false,
			replicas:        2,
			ordinal:         2,
		},
		{
			name:            "on scaledown only, remains",
			scaleDownPolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			setDeletePolicy: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			needsPodRef:     false,
			needsSetRef:     false,
			replicas:        2,
			ordinal:         1,
		},
		{
			name:            "on both, condemned",
			scaleDownPolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			setDeletePolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			needsPodRef:     true,
			needsSetRef:     false,
			replicas:        2,
			ordinal:         2,
		},
		{
			name:            "on both, remains",
			scaleDownPolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			setDeletePolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			needsPodRef:     false,
			needsSetRef:     true,
			replicas:        2,
			ordinal:         1,
		},
	}

	for _, tc := range testCases {
		for _, useOtherRefs := range []bool{false, true} {
			for _, setPodRef := range []bool{false, true} {
				for _, setSetRef := range []bool{false, true} {
					claim := corev1.PersistentVolumeClaim{}
					claim.Name = "target-claim"
					pod := corev1.Pod{}
					pod.Name = fmt.Sprintf("pod-%d", tc.ordinal)
					pod.GetObjectMeta().SetUID("pod-123")
					set := appsv1beta1.StatefulSet{}
					set.Name = "stateful-set"
					set.GetObjectMeta().SetUID("ss-456")
					set.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
						WhenScaled:  tc.scaleDownPolicy,
						WhenDeleted: tc.setDeletePolicy,
					}
					set.Spec.Replicas = &tc.replicas
					if setPodRef {
						setOwnerRef(&claim, &pod, &pod.TypeMeta)
					}
					if setSetRef {
						setOwnerRef(&claim, &set, &set.TypeMeta)
					}
					if useOtherRefs {
						randomObject1 := corev1.Pod{}
						randomObject1.Name = "rand1"
						randomObject1.GetObjectMeta().SetUID("rand1-abc")
						randomObject2 := corev1.Pod{}
						randomObject2.Name = "rand2"
						randomObject2.GetObjectMeta().SetUID("rand2-def")
						setOwnerRef(&claim, &randomObject1, &randomObject1.TypeMeta)
						setOwnerRef(&claim, &randomObject2, &randomObject2.TypeMeta)
					}
					shouldMatch := setPodRef == tc.needsPodRef && setSetRef == tc.needsSetRef
					if claimOwnerMatchesSetAndPod(&claim, &set, &pod) != shouldMatch {
						t.Errorf("Bad match for %s with pod=%v,set=%v,others=%v", tc.name, setPodRef, setSetRef, useOtherRefs)
					}
				}
			}
		}
	}
}

func TestUpdateClaimOwnerRefForSetAndPod(t *testing.T) {
	testCases := []struct {
		name            string
		scaleDownPolicy appsv1beta1.PersistentVolumeClaimRetentionPolicyType
		setDeletePolicy appsv1beta1.PersistentVolumeClaimRetentionPolicyType
		condemned       bool
		needsPodRef     bool
		needsSetRef     bool
	}{
		{
			name:            "retain",
			scaleDownPolicy: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			setDeletePolicy: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			condemned:       false,
			needsPodRef:     false,
			needsSetRef:     false,
		},
		{
			name:            "delete with set",
			scaleDownPolicy: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			setDeletePolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			condemned:       false,
			needsPodRef:     false,
			needsSetRef:     true,
		},
		{
			name:            "delete with scaledown, not condemned",
			scaleDownPolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			setDeletePolicy: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			condemned:       false,
			needsPodRef:     false,
			needsSetRef:     false,
		},
		{
			name:            "delete on scaledown, condemned",
			scaleDownPolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			setDeletePolicy: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			condemned:       true,
			needsPodRef:     true,
			needsSetRef:     false,
		},
		{
			name:            "delete on both, not condemned",
			scaleDownPolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			setDeletePolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			condemned:       false,
			needsPodRef:     false,
			needsSetRef:     true,
		},
		{
			name:            "delete on both, condemned",
			scaleDownPolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			setDeletePolicy: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			condemned:       true,
			needsPodRef:     true,
			needsSetRef:     false,
		},
	}
	for _, tc := range testCases {
		for _, hasPodRef := range []bool{true, false} {
			for _, hasSetRef := range []bool{true, false} {
				set := appsv1beta1.StatefulSet{}
				set.Name = "ss"
				numReplicas := int32(5)
				set.Spec.Replicas = &numReplicas
				set.SetUID("ss-123")
				set.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
					WhenScaled:  tc.scaleDownPolicy,
					WhenDeleted: tc.setDeletePolicy,
				}
				pod := corev1.Pod{}
				if tc.condemned {
					pod.Name = "pod-8"
				} else {
					pod.Name = "pod-1"
				}
				pod.SetUID("pod-456")
				claim := corev1.PersistentVolumeClaim{}
				if hasPodRef {
					setOwnerRef(&claim, &pod, &pod.TypeMeta)
				}
				if hasSetRef {
					setOwnerRef(&claim, &set, &set.TypeMeta)
				}
				needsUpdate := hasPodRef != tc.needsPodRef || hasSetRef != tc.needsSetRef
				shouldUpdate := updateClaimOwnerRefForSetAndPod(&claim, &set, &pod)
				if shouldUpdate != needsUpdate {
					t.Errorf("Bad update for %s hasPodRef=%v hasSetRef=%v", tc.name, hasPodRef, hasSetRef)
				}
				if hasOwnerRef(&claim, &pod) != tc.needsPodRef {
					t.Errorf("Bad pod ref for %s hasPodRef=%v hasSetRef=%v", tc.name, hasPodRef, hasSetRef)
				}
				if hasOwnerRef(&claim, &set) != tc.needsSetRef {
					t.Errorf("Bad set ref for %s hasPodRef=%v hasSetRef=%v", tc.name, hasPodRef, hasSetRef)
				}
			}
		}
	}
}

func TestHasOwnerRef(t *testing.T) {
	target := corev1.Pod{}
	target.SetOwnerReferences([]metav1.OwnerReference{
		{UID: "123"}, {UID: "456"}})
	ownerA := corev1.Pod{}
	ownerA.GetObjectMeta().SetUID("123")
	ownerB := corev1.Pod{}
	ownerB.GetObjectMeta().SetUID("789")
	if !hasOwnerRef(&target, &ownerA) {
		t.Error("Missing owner")
	}
	if hasOwnerRef(&target, &ownerB) {
		t.Error("Unexpected owner")
	}
}

func TestHasStaleOwnerRef(t *testing.T) {
	target := corev1.Pod{}
	target.SetOwnerReferences([]metav1.OwnerReference{
		{Name: "bob", UID: "123"}, {Name: "shirley", UID: "456"}})
	ownerA := corev1.Pod{}
	ownerA.SetUID("123")
	ownerA.Name = "bob"
	ownerB := corev1.Pod{}
	ownerB.Name = "shirley"
	ownerB.SetUID("789")
	ownerC := corev1.Pod{}
	ownerC.Name = "yvonne"
	ownerC.SetUID("345")
	if hasStaleOwnerRef(&target, &ownerA) {
		t.Error("ownerA should not be stale")
	}
	if !hasStaleOwnerRef(&target, &ownerB) {
		t.Error("ownerB should be stale")
	}
	if hasStaleOwnerRef(&target, &ownerC) {
		t.Error("ownerC should not be stale")
	}
}

func TestSetOwnerRef(t *testing.T) {
	target := corev1.Pod{}
	ownerA := corev1.Pod{}
	ownerA.Name = "A"
	ownerA.GetObjectMeta().SetUID("ABC")
	if setOwnerRef(&target, &ownerA, &ownerA.TypeMeta) != true {
		t.Errorf("Unexpected lack of update")
	}
	ownerRefs := target.GetObjectMeta().GetOwnerReferences()
	if len(ownerRefs) != 1 {
		t.Errorf("Unexpected owner ref count: %d", len(ownerRefs))
	}
	if ownerRefs[0].UID != "ABC" {
		t.Errorf("Unexpected owner UID %v", ownerRefs[0].UID)
	}
	if setOwnerRef(&target, &ownerA, &ownerA.TypeMeta) != false {
		t.Errorf("Unexpected update")
	}
	if len(target.GetObjectMeta().GetOwnerReferences()) != 1 {
		t.Error("Unexpected duplicate reference")
	}
	ownerB := corev1.Pod{}
	ownerB.Name = "B"
	ownerB.GetObjectMeta().SetUID("BCD")
	if setOwnerRef(&target, &ownerB, &ownerB.TypeMeta) != true {
		t.Error("Unexpected lack of second update")
	}
	ownerRefs = target.GetObjectMeta().GetOwnerReferences()
	if len(ownerRefs) != 2 {
		t.Errorf("Unexpected owner ref count: %d", len(ownerRefs))
	}
	if ownerRefs[0].UID != "ABC" || ownerRefs[1].UID != "BCD" {
		t.Errorf("Bad second ownerRefs: %v", ownerRefs)
	}
}

func TestRemoveOwnerRef(t *testing.T) {
	target := corev1.Pod{}
	ownerA := corev1.Pod{}
	ownerA.Name = "A"
	ownerA.GetObjectMeta().SetUID("ABC")
	if removeOwnerRef(&target, &ownerA) != false {
		t.Error("Unexpected update on empty remove")
	}
	setOwnerRef(&target, &ownerA, &ownerA.TypeMeta)
	if removeOwnerRef(&target, &ownerA) != true {
		t.Error("Unexpected lack of update")
	}
	if len(target.GetObjectMeta().GetOwnerReferences()) != 0 {
		t.Error("Unexpected owner reference remains")
	}

	ownerB := corev1.Pod{}
	ownerB.Name = "B"
	ownerB.GetObjectMeta().SetUID("BCD")

	setOwnerRef(&target, &ownerA, &ownerA.TypeMeta)
	if removeOwnerRef(&target, &ownerB) != false {
		t.Error("Unexpected update for mismatched owner")
	}
	if len(target.GetObjectMeta().GetOwnerReferences()) != 1 {
		t.Error("Missing ref after no-op remove")
	}
	setOwnerRef(&target, &ownerB, &ownerB.TypeMeta)
	if removeOwnerRef(&target, &ownerA) != true {
		t.Error("Missing update for second remove")
	}
	ownerRefs := target.GetObjectMeta().GetOwnerReferences()
	if len(ownerRefs) != 1 {
		t.Error("Extra ref after second remove")
	}
	if ownerRefs[0].UID != "BCD" {
		t.Error("Bad UID after second remove")
	}
}

func TestIsRunningAndReady(t *testing.T) {
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 1)
	if isRunningAndReady(pod) {
		t.Error("isRunningAndReady does not respect Pod phase")
	}
	pod.Status.Phase = corev1.PodRunning
	if isRunningAndReady(pod) {
		t.Error("isRunningAndReady does not respect Pod condition")
	}
	condition := corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionTrue}
	podutil.UpdatePodCondition(&pod.Status, &condition)
	if !isRunningAndReady(pod) {
		t.Error("Pod should be running and ready")
	}
}

func TestGetMinReadySeconds(t *testing.T) {
	set := newStatefulSet(3)
	if getMinReadySeconds(set) != 0 {
		t.Error("getMinReadySeconds should be zero")
	}
	set.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{}
	if getMinReadySeconds(set) != 0 {
		t.Error("getMinReadySeconds should be zero")
	}
	set.Spec.UpdateStrategy.RollingUpdate.MinReadySeconds = utilpointer.Int32Ptr(3)
	if getMinReadySeconds(set) != 3 {
		t.Error("getMinReadySeconds should be 3")
	}
	set.Spec.UpdateStrategy.RollingUpdate.MinReadySeconds = utilpointer.Int32Ptr(30)
	if getMinReadySeconds(set) != 30 {
		t.Error("getMinReadySeconds should be 3")
	}
}

func TestIsRunningAndAvailable(t *testing.T) {
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 1)
	if avail, wait := isRunningAndAvailable(pod, 0); avail || wait != 0 {
		t.Errorf("isRunningAndAvailable does not respect Pod phase, avail = %t, wait = %d", avail, wait)
	}
	pod.Status.Phase = corev1.PodPending
	if avail, wait := isRunningAndAvailable(pod, 0); avail || wait != 0 {
		t.Errorf("isRunningAndAvailable does not respect Pod phase, avail = %t, wait = %d", avail, wait)
	}
	pod.Status.Phase = corev1.PodRunning
	if avail, wait := isRunningAndAvailable(pod, 0); avail || wait != 0 {
		t.Errorf("isRunningAndAvailable does not respect Pod condition, avail = %t, wait = %d", avail, wait)
	}
	pod.Status.Conditions = []corev1.PodCondition{
		{
			Type:   corev1.PodReady,
			Status: corev1.ConditionTrue,
		},
	}
	if avail, wait := isRunningAndAvailable(pod, 0); !avail || wait != 0 {
		t.Errorf("isRunningAndAvailable does not respect 0 minReadySecond, avail = %t, wait = %d", avail, wait)
	}
	if avail, wait := isRunningAndAvailable(pod, 10); avail || wait != 10*time.Second {
		t.Errorf("isRunningAndAvailable does not respect non 0 minReadySecond, avail = %t, wait = %d", avail, wait)
	}
	pod.Status.Conditions[0].LastTransitionTime = metav1.NewTime(time.Now().Add(-5 * time.Second))
	if avail, wait := isRunningAndAvailable(pod, 10); avail || wait < 4*time.Second {
		t.Errorf("isRunningAndAvailable does not respect Pod condition last transaction, avail = %t, wait = %d",
			avail, wait)
	}
	if avail, wait := isRunningAndAvailable(pod, 3); !avail || wait != 0 {
		t.Errorf("isRunningAndAvailable does not respect Pod condition  last transaction, avail = %t, wait = %d",
			avail, wait)
	}
}

func TestAscendingOrdinal(t *testing.T) {
	set := newStatefulSet(10)
	pods := make([]*corev1.Pod, 10)
	perm := rand.Perm(10)
	for i, v := range perm {
		pods[i] = newStatefulSetPod(set, v)
	}
	sort.Sort(ascendingOrdinal(pods))
	if !sort.IsSorted(ascendingOrdinal(pods)) {
		t.Error("ascendingOrdinal fails to sort Pods")
	}
}

func TestOverlappingStatefulSets(t *testing.T) {
	sets := make([]*appsv1beta1.StatefulSet, 10)
	perm := rand.Perm(10)
	for i, v := range perm {
		sets[i] = newStatefulSet(10)
		sets[i].CreationTimestamp = metav1.NewTime(sets[i].CreationTimestamp.Add(time.Duration(v) * time.Second))
	}
	sort.Sort(overlappingStatefulSets(sets))
	if !sort.IsSorted(overlappingStatefulSets(sets)) {
		t.Error("ascendingOrdinal fails to sort Pods")
	}
	for i, v := range perm {
		sets[i] = newStatefulSet(10)
		sets[i].Name = strconv.FormatInt(int64(v), 10)
	}
	sort.Sort(overlappingStatefulSets(sets))
	if !sort.IsSorted(overlappingStatefulSets(sets)) {
		t.Error("ascendingOrdinal fails to sort Pods")
	}
}

func TestNewPodControllerRef(t *testing.T) {
	set := newStatefulSet(1)
	pod := newStatefulSetPod(set, 0)
	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef == nil {
		t.Fatalf("No ControllerRef found on new pod")
	}
	if got, want := controllerRef.APIVersion, appsv1beta1.SchemeGroupVersion.String(); got != want {
		t.Errorf("controllerRef.APIVersion = %q, want %q", got, want)
	}
	if got, want := controllerRef.Kind, "StatefulSet"; got != want {
		t.Errorf("controllerRef.Kind = %q, want %q", got, want)
	}
	if got, want := controllerRef.Name, set.Name; got != want {
		t.Errorf("controllerRef.Name = %q, want %q", got, want)
	}
	if got, want := controllerRef.UID, set.UID; got != want {
		t.Errorf("controllerRef.UID = %q, want %q", got, want)
	}
	if got, want := *controllerRef.Controller, true; got != want {
		t.Errorf("controllerRef.Controller = %v, want %v", got, want)
	}
}

func TestCreateApplyRevision(t *testing.T) {
	set := newStatefulSet(1)
	set.Status.CollisionCount = new(int32)
	revision, err := newRevision(set, 1, set.Status.CollisionCount)
	if err != nil {
		t.Fatal(err)
	}
	set.Spec.Template.Spec.Containers[0].Name = "foo"
	if set.Annotations == nil {
		set.Annotations = make(map[string]string)
	}
	key := "foo"
	expectedValue := "bar"
	set.Annotations[key] = expectedValue
	restoredSet, err := ApplyRevision(set, revision)
	if err != nil {
		t.Fatal(err)
	}
	restoredRevision, err := newRevision(restoredSet, 2, restoredSet.Status.CollisionCount)
	if err != nil {
		t.Fatal(err)
	}
	if !history.EqualRevision(revision, restoredRevision) {
		t.Errorf("wanted %v got %v", string(revision.Data.Raw), string(restoredRevision.Data.Raw))
	}
	value, ok := restoredRevision.Annotations[key]
	if !ok {
		t.Errorf("missing annotation %s", key)
	}
	if value != expectedValue {
		t.Errorf("for annotation %s wanted %s got %s", key, expectedValue, value)
	}
}

func TestRollingUpdateApplyRevision(t *testing.T) {
	set := newStatefulSet(1)
	set.Status.CollisionCount = new(int32)
	currentSet := set.DeepCopy()
	currentRevision, err := newRevision(set, 1, set.Status.CollisionCount)
	if err != nil {
		t.Fatal(err)
	}

	set.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{{Name: "foo", Value: "bar"}}
	updateSet := set.DeepCopy()
	updateRevision, err := newRevision(set, 2, set.Status.CollisionCount)
	if err != nil {
		t.Fatal(err)
	}

	restoredCurrentSet, err := ApplyRevision(set, currentRevision)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(currentSet.Spec.Template, restoredCurrentSet.Spec.Template) {
		t.Errorf("want %v got %v", currentSet.Spec.Template, restoredCurrentSet.Spec.Template)
	}

	restoredUpdateSet, err := ApplyRevision(set, updateRevision)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updateSet.Spec.Template, restoredUpdateSet.Spec.Template) {
		t.Errorf("want %v got %v", updateSet.Spec.Template, restoredUpdateSet.Spec.Template)
	}
}

func newPVC(name string) corev1.PersistentVolumeClaim {
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: *resource.NewQuantity(1, resource.BinarySI),
				},
			},
		},
	}
}

func newStatefulSetWithVolumes(replicas int, name string, petMounts []corev1.VolumeMount, podMounts []corev1.VolumeMount) *appsv1beta1.StatefulSet {
	mounts := append(petMounts, podMounts...)
	claims := []corev1.PersistentVolumeClaim{}
	for _, m := range petMounts {
		claims = append(claims, newPVC(m.Name))
	}

	vols := []corev1.Volume{}
	for _, m := range podMounts {
		vols = append(vols, corev1.Volume{
			Name: m.Name,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/tmp/%v", m.Name),
				},
			},
		})
	}

	template := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:         "nginx",
					Image:        "nginx",
					VolumeMounts: mounts,
				},
			},
			Volumes: vols,
		},
	}

	template.Labels = map[string]string{"foo": "bar"}

	return &appsv1beta1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps.kruise.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: corev1.NamespaceDefault,
			UID:       "test",
		},
		Spec: appsv1beta1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
			Replicas:             func() *int32 { i := int32(replicas); return &i }(),
			Template:             template,
			VolumeClaimTemplates: claims,
			ServiceName:          "governingsvc",
			UpdateStrategy:       appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			PersistentVolumeClaimRetentionPolicy: &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenScaled:  appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
				WhenDeleted: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			},
			RevisionHistoryLimit: func() *int32 {
				limit := int32(2)
				return &limit
			}(),
		},
	}
}

func newStatefulSet(replicas int) *appsv1beta1.StatefulSet {
	petMounts := []corev1.VolumeMount{
		{Name: "datadir", MountPath: "/tmp/zookeeper"},
	}
	podMounts := []corev1.VolumeMount{
		{Name: "home", MountPath: "/home"},
	}
	return newStatefulSetWithVolumes(replicas, "foo", petMounts, podMounts)
}

func TestGetStatefulSetReplicasRange(t *testing.T) {
	int32Ptr := func(i int32) *int32 {
		return &i
	}

	tests := []struct {
		name          string
		statefulSet   *appsv1beta1.StatefulSet
		expectedCount int
		expectedRes   sets.Int
	}{
		{
			name: "Ordinals start 0",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas: int32Ptr(4),
					ReserveOrdinals: []intstr.IntOrString{
						intstr.FromInt32(1),
						intstr.FromInt32(3),
					},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
				},
			},
			expectedCount: 6,
			expectedRes:   sets.NewInt(1, 3),
		},
		{
			name: "Ordinals start 2 with ReserveOrdinals 1&3",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas: int32Ptr(4),
					ReserveOrdinals: []intstr.IntOrString{
						intstr.FromInt32(1),
						intstr.FromInt32(3),
					},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 2,
					},
				},
			},
			expectedCount: 5,
			expectedRes:   sets.NewInt(1, 3),
		},
		{
			name: "Ordinals start 3 with ReserveOrdinals 1&3",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas: int32Ptr(4),
					ReserveOrdinals: []intstr.IntOrString{
						intstr.FromInt32(1),
						intstr.FromInt32(3),
					},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 3,
					},
				},
			},
			expectedCount: 5,
			expectedRes:   sets.NewInt(1, 3),
		},
		{
			name: "Ordinals start 4 with ReserveOrdinals 1&3",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas: int32Ptr(4),
					ReserveOrdinals: []intstr.IntOrString{
						intstr.FromInt32(1),
						intstr.FromInt32(3),
					},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 4,
					},
				},
			},
			expectedCount: 4,
			expectedRes:   sets.NewInt(1, 3),
		},
		// ... other test cases
	}
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.StatefulSetStartOrdinal, true)()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//count, res := getStatefulSetReplicasRange(tt.statefulSet)
			startOrdinal, endOrdinal, res := getStatefulSetReplicasRange(tt.statefulSet)
			count := endOrdinal - startOrdinal
			if count != tt.expectedCount || len(res) != len(tt.expectedRes) || res.HasAll(tt.expectedRes.Len()) {
				t.Errorf("getStatefulSetReplicasRange(%v) got (%v, %v), want (%v, %v)",
					tt.name, count, res, tt.expectedCount, tt.expectedRes)
			}
		})
	}
}

func newStorageClass(name string, canExpand bool) storagev1.StorageClass {
	return storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		AllowVolumeExpansion: &canExpand,
	}
}

func TestIsCurrentRevisionNeeded(t *testing.T) {
	currentRevisionHash := "1"
	updatedRevisionHash := "2"
	int32Ptr := func(i int32) *int32 {
		return &i
	}
	setName := "ss"

	newReplicas := func(startOrdinal, size int, revisionHash string) []*corev1.Pod {
		pods := make([]*corev1.Pod, size)
		for i := 0; i < size; i++ {
			pods[i] = &corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey: revisionHash,
					},
					Name: fmt.Sprintf("%s-%d", setName, i+startOrdinal),
				},
			}
		}
		return pods
	}

	utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.StatefulSetStartOrdinal, true)

	tests := []struct {
		name           string
		statefulSet    *appsv1beta1.StatefulSet
		updateRevision string
		ordinal        int
		replicas       []*corev1.Pod
		expectedRes    bool
	}{
		{
			// replicas 3, no partition
			// 0: current revision
			// 2: current revision
			// => 1: should be current revision
			name: "Ordinals start 0, no partition with update 0",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 2,
					UpdatedReplicas: 0,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        1,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(0, 1, currentRevisionHash)
				pods = append(pods, newReplicas(2, 1, currentRevisionHash)...)
				return pods
			}(),
			expectedRes: true,
		},
		{
			// replicas 3, no partition
			// 0: current revision
			// 2: updated revision
			// => 1: should be updated revision
			name: "Ordinals start 0, no partition with update 1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 1,
					UpdatedReplicas: 1,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        1,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(0, 1, currentRevisionHash)
				pods = append(pods, newReplicas(2, 1, updatedRevisionHash)...)
				return pods
			}(),
			expectedRes: false,
		},
		{
			// replicas 3, partition 1
			// 0: current revision
			// 2: current revision
			// => 1: should be updated revision
			name: "Ordinals start 0, partition 1 with update 0",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 2,
					UpdatedReplicas: 0,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        1,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(0, 1, currentRevisionHash)
				pods = append(pods, newReplicas(2, 1, currentRevisionHash)...)
				return pods
			}(),
			expectedRes: false,
		},
		{
			// replicas 3, partition 1
			// 0: current revision
			// 2: updated revision
			// => 1: should be updated revision
			name: "Ordinals start 0, partition 1 with update 1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 1,
					UpdatedReplicas: 1,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        1,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(0, 1, currentRevisionHash)
				pods = append(pods, newReplicas(2, 1, updatedRevisionHash)...)
				return pods
			}(),
			expectedRes: false,
		},
		{
			// replicas 3, partition 1
			// 1: current revision
			// 2: current revision
			// => 0: should be current revision
			name: "Ordinals start 0, partition 1, create current pod1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 2,
					UpdatedReplicas: 0,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        0,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(1, 2, currentRevisionHash)
				return pods
			}(),
			expectedRes: true,
		},
		{
			// replicas 3, partition 1
			// 1: updated revision
			// 2: updated revision
			// => 0: should be current revision
			name: "Ordinals start 0, partition 1, create current pod2",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 0,
					UpdatedReplicas: 2,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        0,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(1, 2, currentRevisionHash)
				return pods
			}(),
			expectedRes: true,
		},
		// with reservedIds
		{
			// reservedId 1, replicas 3, partition 2
			// 2: current revision
			// 3: current revision
			// => 0: should be current revision
			name: "ReservedId 1, partition 2, create current pod1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas: int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{
						intstr.FromInt32(1),
					},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(2),
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 2,
					UpdatedReplicas: 0,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        0,
			replicas: func() []*corev1.Pod {
				pods := []*corev1.Pod{nil, nil}
				pods = append(pods, newReplicas(2, 2, currentRevisionHash)...)
				return pods
			}(),
			expectedRes: true,
		},

		// with startOrdinal
		{
			// start ordinal 2, replicas 3, no partition
			// 2: current revision
			// 4: current revision
			// => 3: should be current revision
			name: "Ordinals start 2, no partition with update 0",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 2,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 2,
					UpdatedReplicas: 0,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        3,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(2, 1, currentRevisionHash)
				pods = append(pods, newReplicas(4, 1, currentRevisionHash)...)
				return pods
			}(),
			expectedRes: true,
		},
		{
			// start ordinal 2, replicas 3, no partition
			// 2: current revision
			// 4: updated revision
			// => 3: should be updated revision
			name: "Ordinals start 2, no partition with update 1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 2,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 1,
					UpdatedReplicas: 1,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        3,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(2, 1, currentRevisionHash)
				pods = append(pods, newReplicas(4, 1, updatedRevisionHash)...)
				return pods
			}(),
			expectedRes: false,
		},
		{
			// start ordinal 2, replicas 3, partition 1
			// 2: current revision
			// 4: current revision
			// => 3: should be updated revision
			name: "Ordinals start 2, partition 1 with update 0",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 2,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 2,
					UpdatedReplicas: 0,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        3,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(2, 1, currentRevisionHash)
				pods = append(pods, newReplicas(4, 1, currentRevisionHash)...)
				return pods
			}(),
			expectedRes: false,
		},
		{
			// start ordinal 2, replicas 3, partition 1
			// 2: current revision
			// 4: updated revision
			// => 3: should be updated revision
			name: "Ordinals start 2, partition 1 with update 1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 2,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 1,
					UpdatedReplicas: 1,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        3,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(2, 1, currentRevisionHash)
				pods = append(pods, newReplicas(4, 1, updatedRevisionHash)...)
				return pods
			}(),
			expectedRes: false,
		},
		{
			// start ordinal 2, replicas 3, partition 1
			// 3: current revision
			// 4: current revision
			// => 2: should be current revision
			name: "Ordinals start 2, partition 1, create current pod1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 2,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 2,
					UpdatedReplicas: 0,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        2,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(3, 2, currentRevisionHash)
				return pods
			}(),
			expectedRes: true,
		},
		{
			// start ordinal 2, replicas 3, partition 1
			// 3: updated revision
			// 4: updated revision
			// => 2: should be current revision
			name: "Ordinals start 0, partition 1, create current pod2",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 2,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 0,
					UpdatedReplicas: 2,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        2,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(1, 2, currentRevisionHash)
				return pods
			}(),
			expectedRes: true,
		},

		// with UnorderedUpdate
		{
			// replicas 3, partition 1 with UnorderedUpdate
			// 0: current revision
			// 2: updated revision
			// => 1: should be updated revision
			name: "UnorderedUpdate, partition 1 with update 1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
							UnorderedUpdate: &appsv1beta1.UnorderedUpdateStrategy{
								PriorityStrategy: &appspub.UpdatePriorityStrategy{
									// some priority strategy
								},
							},
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 1,
					UpdatedReplicas: 1,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        1,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(0, 1, currentRevisionHash)
				pods = append(pods, newReplicas(2, 1, updatedRevisionHash)...)
				return pods
			}(),
			expectedRes: false,
		},
		{
			// replicas 3, partition 1 with UnorderedUpdate
			// 0: updated revision
			// 2: updated revision
			// => 1: should be current revision
			name: "UnorderedUpdate, partition 1 with update 1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
							UnorderedUpdate: &appsv1beta1.UnorderedUpdateStrategy{
								PriorityStrategy: &appspub.UpdatePriorityStrategy{
									// some priority strategy
								},
							},
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 0,
					UpdatedReplicas: 2,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        1,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(0, 1, updatedRevisionHash)
				pods = append(pods, newReplicas(2, 1, updatedRevisionHash)...)
				return pods
			}(),
			expectedRes: true,
		},

		// with UnorderedUpdate and startOrdinal
		{
			// start ordinal 2, replicas 3, partition 1 with UnorderedUpdate
			// 2: current revision
			// 4: updated revision
			// => 3: should be updated revision
			name: "UnorderedUpdate, partition 1 with update 1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 2,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
							UnorderedUpdate: &appsv1beta1.UnorderedUpdateStrategy{
								PriorityStrategy: &appspub.UpdatePriorityStrategy{
									// some priority strategy
								},
							},
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 1,
					UpdatedReplicas: 1,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        3,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(2, 1, currentRevisionHash)
				pods = append(pods, newReplicas(4, 1, updatedRevisionHash)...)
				return pods
			}(),
			expectedRes: false,
		},
		{
			// start ordinal 2, replicas 3, partition 1 with UnorderedUpdate
			// 2: updated revision
			// 4: updated revision
			// => 3: should be current revision
			name: "UnorderedUpdate, partition 1 with update 1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:        int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 2,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
							UnorderedUpdate: &appsv1beta1.UnorderedUpdateStrategy{
								PriorityStrategy: &appspub.UpdatePriorityStrategy{
									// some priority strategy
								},
							},
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 0,
					UpdatedReplicas: 2,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        3,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(2, 1, updatedRevisionHash)
				pods = append(pods, newReplicas(4, 1, updatedRevisionHash)...)
				return pods
			}(),
			expectedRes: true,
		},

		// with UnorderedUpdate and reservedIds
		{
			// reservedIds 1, replicas 3, partition 1 with UnorderedUpdate
			// 0: current revision
			// 3: updated revision
			// => 2: should be updated revision
			name: "ReservedIds 1, UnorderedUpdate, partition 2 with update 1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas: int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{
						intstr.FromInt32(1),
					},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
							UnorderedUpdate: &appsv1beta1.UnorderedUpdateStrategy{
								PriorityStrategy: &appspub.UpdatePriorityStrategy{
									// some priority strategy
								},
							},
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 1,
					UpdatedReplicas: 1,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        2,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(0, 1, currentRevisionHash)
				pods = append(pods, nil)
				pods = append(pods, nil)
				pods = append(pods, newReplicas(3, 1, updatedRevisionHash)...)
				return pods
			}(),
			expectedRes: false,
		},
		{
			// ReservedIds 1, replicas 3, partition 1 with UnorderedUpdate
			// 0: updated revision
			// 3: updated revision
			// => 2: should be current revision
			name: "ReservedIds 1, UnorderedUpdate, partition 2 with update 1",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas: int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{
						intstr.FromInt32(1),
					},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(1),
							UnorderedUpdate: &appsv1beta1.UnorderedUpdateStrategy{
								PriorityStrategy: &appspub.UpdatePriorityStrategy{
									// some priority strategy
								},
							},
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 0,
					UpdatedReplicas: 2,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        2,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(0, 1, updatedRevisionHash)
				// reversed ids
				pods = append(pods, nil)
				// pod need to be created
				pods = append(pods, newReplicas(2, 1, currentRevisionHash)...)
				pods = append(pods, newReplicas(3, 1, updatedRevisionHash)...)
				return pods
			}(),
			expectedRes: true,
		},

		// with startOrdinal and reservedIds
		// fixes https://github.com/openkruise/kruise/issues/1813
		{
			// reservedId 1, replicas 3, partition 2
			// 3: updated revision
			// 0: current revision
			// => 2: should be current revision
			name: "Ordinals start 0, reservedId 1, partition 1, create pod2",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas: int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{
						intstr.FromInt32(1),
					},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 0,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(2),
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 1,
					UpdatedReplicas: 1,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        2,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(0, 1, currentRevisionHash)
				pods = append(pods, nil, nil)
				pods = append(pods, newReplicas(3, 1, updatedRevisionHash)...)
				return pods
			}(),
			expectedRes: true,
		},
		{
			// start ordinal 2, reservedId 3, replicas 3, partition 2
			// 5: updated revision
			// 2: current revision
			// => 4: should be current revision
			name: "Ordinals start 2, reservedId 1, partition 1, create pod2",
			statefulSet: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas: int32Ptr(3),
					ReserveOrdinals: []intstr.IntOrString{
						intstr.FromInt32(1),
					},
					Ordinals: &appsv1beta1.StatefulSetOrdinals{
						Start: 2,
					},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: int32Ptr(2),
						},
					},
				},
				Status: appsv1beta1.StatefulSetStatus{
					CurrentReplicas: 1,
					UpdatedReplicas: 1,
				},
			},
			updateRevision: updatedRevisionHash,
			ordinal:        4,
			replicas: func() []*corev1.Pod {
				pods := newReplicas(2, 1, currentRevisionHash)
				pods = append(pods, nil, nil)
				pods = append(pods, newReplicas(5, 1, updatedRevisionHash)...)
				return pods
			}(),
			expectedRes: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// for some cases RollingUpdateStatefulSetStrategy is nil
			// we will create current revision pods if the ordinal has not been updated
			current, updated := int32(0), int32(0)
			for _, pod := range tt.replicas {
				if pod == nil {
					continue
				}
				if pod.Labels[apps.ControllerRevisionHashLabelKey] == currentRevisionHash {
					current++
				} else if pod.Labels[apps.ControllerRevisionHashLabelKey] == updatedRevisionHash {
					updated++
				}
			}
			if tt.statefulSet != nil {
				tt.statefulSet.Status.CurrentReplicas = current
				tt.statefulSet.Status.UpdatedReplicas = updated
			}

			res := isCurrentRevisionNeeded(tt.statefulSet, tt.updateRevision, tt.ordinal, tt.replicas)
			if res != tt.expectedRes {
				t.Errorf("checkIsCurrentRevisionNeeded(%v) got (%v), want (%v)",
					tt.name, res, tt.expectedRes)
			}
		})
	}
}
