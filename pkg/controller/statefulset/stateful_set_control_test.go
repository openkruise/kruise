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
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/history"
	testingclock "k8s.io/utils/clock/testing"
	utilpointer "k8s.io/utils/pointer"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	kruisefake "github.com/openkruise/kruise/pkg/client/clientset/versioned/fake"
	kruiseinformers "github.com/openkruise/kruise/pkg/client/informers/externalversions"
	kruiseappsinformers "github.com/openkruise/kruise/pkg/client/informers/externalversions/apps/v1beta1"
	kruiseappslisters "github.com/openkruise/kruise/pkg/client/listers/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	"github.com/openkruise/kruise/pkg/util/revisionadapter"
)

type invariantFunc func(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error

func setupController(client clientset.Interface, kruiseClient kruiseclientset.Interface) (*fakeObjectManager, *fakeStatefulSetStatusUpdater, StatefulSetControlInterface, chan struct{}) {
	informerFactory := informers.NewSharedInformerFactory(client, controller.NoResyncPeriodFunc())
	kruiseInformerFactory := kruiseinformers.NewSharedInformerFactory(kruiseClient, controller.NoResyncPeriodFunc())
	om := newFakeObjectManager(informerFactory, kruiseInformerFactory)
	spc := NewStatefulPodControlFromManager(om, &noopRecorder{})
	ssu := newFakeStatefulSetStatusUpdater(kruiseInformerFactory.Apps().V1beta1().StatefulSets())
	recorder := &noopRecorder{}
	inplaceControl := inplaceupdate.NewForInformer(informerFactory.Core().V1().Pods(), revisionadapter.NewDefaultImpl())
	lifecycleControl := lifecycle.NewForInformer(informerFactory.Core().V1().Pods())
	ssc := NewDefaultStatefulSetControl(spc, inplaceControl, lifecycleControl, ssu, history.NewFakeHistory(informerFactory.Apps().V1().ControllerRevisions()), recorder)

	stop := make(chan struct{})
	informerFactory.Start(stop)
	kruiseInformerFactory.Start(stop)
	cache.WaitForCacheSync(
		stop,
		kruiseInformerFactory.Apps().V1beta1().StatefulSets().Informer().HasSynced,
		// informerFactory.Apps().V1().StatefulSets().Informer().HasSynced,
		informerFactory.Core().V1().Pods().Informer().HasSynced,
		informerFactory.Apps().V1().ControllerRevisions().Informer().HasSynced,
	)
	return om, ssu, ssc, stop
}

func burst(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
	set.Spec.PodManagementPolicy = apps.ParallelPodManagement
	return set
}

func runTestOverPVCRetentionPolicies(t *testing.T, testName string, testFn func(*testing.T, *appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy)) {
	subtestName := "StatefulSetAutoDeletePVCDisabled"
	if testName != "" {
		subtestName = fmt.Sprintf("%s/%s", testName, subtestName)
	}
	t.Run(subtestName, func(t *testing.T) {
		defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.StatefulSetAutoDeletePVC, true)()
		testFn(t, &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
			WhenScaled:  appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			WhenDeleted: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
		})
	})

	for _, policy := range []*appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
		{
			WhenScaled:  appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			WhenDeleted: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
		},
		{
			WhenScaled:  appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			WhenDeleted: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
		},
		{
			WhenScaled:  appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
			WhenDeleted: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
		},
		{
			WhenScaled:  appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			WhenDeleted: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
		},
	} {
		subtestName := pvcDeletePolicyString(policy) + "/StatefulSetAutoDeletePVCEnabled"
		if testName != "" {
			subtestName = fmt.Sprintf("%s/%s", testName, subtestName)
		}
		t.Run(subtestName, func(t *testing.T) {
			defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.StatefulSetAutoDeletePVC, true)()
			testFn(t, policy)
		})
	}
}

func pvcDeletePolicyString(policy *appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy) string {
	const retain = appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType
	const delete = appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType
	switch {
	case policy.WhenScaled == retain && policy.WhenDeleted == retain:
		return "Retain"
	case policy.WhenScaled == retain && policy.WhenDeleted == delete:
		return "SetDeleteOnly"
	case policy.WhenScaled == delete && policy.WhenDeleted == retain:
		return "ScaleDownOnly"
	case policy.WhenScaled == delete && policy.WhenDeleted == delete:
		return "Delete"
	}
	return "invalid"
}

func TestStatefulSetControl(t *testing.T) {
	t.SkipNow()
	simpleSetFn := func() *appsv1beta1.StatefulSet { return newStatefulSet(3) }
	largeSetFn := func() *appsv1beta1.StatefulSet { return newStatefulSet(5) }

	testCases := []struct {
		fn  func(*testing.T, *appsv1beta1.StatefulSet, invariantFunc)
		obj func() *appsv1beta1.StatefulSet
	}{
		{CreatesPods, simpleSetFn},
		{ScalesUp, simpleSetFn},
		{ScalesDown, simpleSetFn},
		{ReplacesPods, largeSetFn},
		{RecreatesFailedPod, simpleSetFn},
		{CreatePodFailure, simpleSetFn},
		{UpdatePodFailure, simpleSetFn},
		{UpdateSetStatusFailure, simpleSetFn},
		{PodRecreateDeleteFailure, simpleSetFn},
	}

	for _, testCase := range testCases {
		fnName := runtime.FuncForPC(reflect.ValueOf(testCase.fn).Pointer()).Name()
		if i := strings.LastIndex(fnName, "."); i != -1 {
			fnName = fnName[i+1:]
		}
		testObj := testCase.obj
		testFn := testCase.fn
		runTestOverPVCRetentionPolicies(
			t,
			fmt.Sprintf("%s/Monotonic", fnName),
			func(t *testing.T, policy *appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy) {
				set := testObj()
				set.Spec.PersistentVolumeClaimRetentionPolicy = policy
				testFn(t, set, assertMonotonicInvariants)
			},
		)
		runTestOverPVCRetentionPolicies(
			t,
			fmt.Sprintf("%s/Burst", fnName),
			func(t *testing.T, policy *appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy) {
				set := burst(testObj())
				set.Spec.PersistentVolumeClaimRetentionPolicy = policy
				testFn(t, set, assertBurstInvariants)
			},
		)
	}
}

func CreatesPods(t *testing.T, set *appsv1beta1.StatefulSet, invariants invariantFunc) {
	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	om, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)

	if err := scaleUpStatefulSetControl(set, ssc, om, invariants); err != nil {
		t.Errorf("Failed to turn up StatefulSet : %s", err)
	}
	var err error
	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if set.Status.Replicas != 3 {
		t.Error("Failed to scale statefulset to 3 replicas")
	}
	if set.Status.ReadyReplicas != 3 {
		t.Error("Failed to set ReadyReplicas correctly")
	}
	if set.Status.AvailableReplicas != 3 {
		t.Error("Failed to set availableReplicas correctly")
	}
	if set.Status.UpdatedReplicas != 3 {
		t.Error("Failed to set UpdatedReplicas correctly")
	}
	if set.Status.UpdatedAvailableReplicas != 3 {
		t.Error("Failed to set UpdatedAvailableReplicas correctly")
	}
}

func ScalesUp(t *testing.T, set *appsv1beta1.StatefulSet, invariants invariantFunc) {
	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	om, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)

	if err := scaleUpStatefulSetControl(set, ssc, om, invariants); err != nil {
		t.Errorf("Failed to turn up StatefulSet : %s", err)
	}
	*set.Spec.Replicas = 4
	if err := scaleUpStatefulSetControl(set, ssc, om, invariants); err != nil {
		t.Errorf("Failed to scale StatefulSet : %s", err)
	}
	var err error
	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if set.Status.Replicas != 4 {
		t.Error("Failed to scale statefulset to 4 replicas")
	}
	if set.Status.ReadyReplicas != 4 {
		t.Error("Failed to set readyReplicas correctly")
	}
	if set.Status.AvailableReplicas != 4 {
		t.Error("Failed to set availableReplicas correctly")
	}
	if set.Status.UpdatedReplicas != 4 {
		t.Error("Failed to set updatedReplicas correctly")
	}
	if set.Status.UpdatedAvailableReplicas != 4 {
		t.Error("Failed to set updatedAvailableReplicas correctly")
	}
}

func ScalesDown(t *testing.T, set *appsv1beta1.StatefulSet, invariants invariantFunc) {
	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	om, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)

	if err := scaleUpStatefulSetControl(set, ssc, om, invariants); err != nil {
		t.Errorf("Failed to turn up StatefulSet : %s", err)
	}
	var err error
	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	*set.Spec.Replicas = 0
	if err := scaleDownStatefulSetControl(set, ssc, om, invariants); err != nil {
		t.Errorf("Failed to scale StatefulSet : %s", err)
	}

	// Check updated set.
	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if set.Status.Replicas != 0 {
		t.Error("Failed to scale statefulset to 0 replicas")
	}
	if set.Status.ReadyReplicas != 0 {
		t.Error("Failed to set readyReplicas correctly")
	}
	if set.Status.AvailableReplicas != 0 {
		t.Error("Failed to set availableReplicas correctly")
	}
	if set.Status.UpdatedReplicas != 0 {
		t.Error("Failed to set updatedReplicas correctly")
	}
	if set.Status.UpdatedAvailableReplicas != 0 {
		t.Error("Failed to set updatedAvailableReplicas correctly")
	}
}

func ReplacesPods(t *testing.T, set *appsv1beta1.StatefulSet, invariants invariantFunc) {
	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	om, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)

	if err := scaleUpStatefulSetControl(set, ssc, om, invariants); err != nil {
		t.Errorf("Failed to turn up StatefulSet : %s", err)
	}
	var err error
	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if set.Status.Replicas != 5 {
		t.Error("Failed to scale statefulset to 5 replicas")
	}
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Error(err)
	}
	claims, err := om.claimsLister.PersistentVolumeClaims(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}

	pods, err := om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}
	for _, pod := range pods {
		podClaims := getPersistentVolumeClaims(set, pod)
		for _, claim := range claims {
			if _, found := podClaims[claim.Name]; found {
				if hasOwnerRef(claim, pod) {
					t.Errorf("Unexpected ownerRef on %s", claim.Name)
				}
			}
		}
	}

	sort.Sort(ascendingOrdinal(pods))
	om.podsIndexer.Delete(pods[0])
	om.podsIndexer.Delete(pods[2])
	om.podsIndexer.Delete(pods[4])
	for i := 0; i < 5; i += 2 {
		pods, err := om.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Error(err)
		}
		if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
			t.Errorf("Failed to update StatefulSet : %s", err)
		}
		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("Error getting updated StatefulSet: %v", err)
		}
		if pods, err = om.setPodRunning(set, i); err != nil {
			t.Error(err)
		}
		if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
			t.Errorf("Failed to update StatefulSet : %s", err)
		}
		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("Error getting updated StatefulSet: %v", err)
		}
		if _, err = om.setPodReady(set, i); err != nil {
			t.Error(err)
		}
	}
	pods, err = om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}
	if err := ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Errorf("Failed to update StatefulSet : %s", err)
	}
	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if e, a := int32(5), set.Status.Replicas; e != a {
		t.Errorf("Expected to scale to %d, got %d", e, a)
	}
}

func RecreatesFailedPod(t *testing.T, set *appsv1beta1.StatefulSet, invariants invariantFunc) {
	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset()
	om, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Error(err)
	}
	pods, err := om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}
	if err := ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Errorf("Error updating StatefulSet %s", err)
	}
	if err := invariants(set, om); err != nil {
		t.Error(err)
	}
	pods, err = om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}
	pods[0].Status.Phase = v1.PodFailed
	om.podsIndexer.Update(pods[0])
	if err := ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Errorf("Error updating StatefulSet %s", err)
	}
	if err := invariants(set, om); err != nil {
		t.Error(err)
	}
	pods, err = om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}
	if isCreated(pods[0]) {
		t.Error("StatefulSet did not recreate failed Pod")
	}
}

func CreatePodFailure(t *testing.T, set *appsv1beta1.StatefulSet, invariants invariantFunc) {
	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	om, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)
	om.SetCreateStatefulPodError(apierrors.NewInternalError(errors.New("API server failed")), 2)

	if err := scaleUpStatefulSetControl(set, ssc, om, invariants); err != nil && isOrHasInternalError(err) {
		t.Errorf("StatefulSetControl did not return InternalError found %s", err)
	}
	// Update so set.Status is set for the next scaleUpStatefulSetControl call.
	var err error
	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if err := scaleUpStatefulSetControl(set, ssc, om, invariants); err != nil {
		t.Errorf("Failed to turn up StatefulSet : %s", err)
	}
	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if set.Status.Replicas != 3 {
		t.Error("Failed to scale StatefulSet to 3 replicas")
	}
	if set.Status.ReadyReplicas != 3 {
		t.Error("Failed to set readyReplicas correctly")
	}
	if set.Status.AvailableReplicas != 3 {
		t.Error("Failed to set availableReplicas correctly")
	}
	if set.Status.UpdatedReplicas != 3 {
		t.Error("Failed to updatedReplicas correctly")
	}
	if set.Status.UpdatedAvailableReplicas != 4 {
		t.Error("Failed to set updatedAvailableReplicas correctly")
	}
}

func UpdatePodFailure(t *testing.T, set *appsv1beta1.StatefulSet, invariants invariantFunc) {
	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	om, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)
	om.SetUpdateStatefulPodError(apierrors.NewInternalError(errors.New("API server failed")), 0)

	// have to have 1 successful loop first
	if err := scaleUpStatefulSetControl(set, ssc, om, invariants); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	var err error
	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if set.Status.Replicas != 3 {
		t.Error("Failed to scale StatefulSet to 3 replicas")
	}
	if set.Status.ReadyReplicas != 3 {
		t.Error("Failed to set readyReplicas correctly")
	}
	if set.Status.AvailableReplicas != 3 {
		t.Error("Failed to set availableReplicas correctly")
	}
	if set.Status.UpdatedReplicas != 3 {
		t.Error("Failed to set updatedReplicas correctly")
	}
	if set.Status.UpdatedAvailableReplicas != 3 {
		t.Error("Failed to set updatedAvailableReplicas correctly")
	}

	// now mutate a pod's identity
	pods, err := om.podsLister.List(labels.Everything())
	if err != nil {
		t.Fatalf("Error listing pods: %v", err)
	}
	if len(pods) != 3 {
		t.Fatalf("Expected 3 pods, got %d", len(pods))
	}
	sort.Sort(ascendingOrdinal(pods))
	pods[0].Name = "goo-0"
	om.podsIndexer.Update(pods[0])

	// now it should fail
	if err := ssc.UpdateStatefulSet(context.TODO(), set, pods); !apierrors.IsInternalError(err) {
		t.Errorf("StatefulSetControl did not return InternalError found %s", err)
	}
}

func UpdateSetStatusFailure(t *testing.T, set *appsv1beta1.StatefulSet, invariants invariantFunc) {
	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	om, ssu, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)
	ssu.SetUpdateStatefulSetStatusError(apierrors.NewInternalError(errors.New("API server failed")), 2)

	if err := scaleUpStatefulSetControl(set, ssc, om, invariants); !apierrors.IsInternalError(err) {
		t.Errorf("StatefulSetControl did not return InternalError found %s", err)
	}
	// Update so set.Status is set for the next scaleUpStatefulSetControl call.
	var err error
	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if err := scaleUpStatefulSetControl(set, ssc, om, invariants); err != nil {
		t.Errorf("Failed to turn up StatefulSet : %s", err)
	}
	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if set.Status.Replicas != 3 {
		t.Error("Failed to scale StatefulSet to 3 replicas")
	}
	if set.Status.ReadyReplicas != 3 {
		t.Error("Failed to set readyReplicas to 3")
	}
	if set.Status.AvailableReplicas != 3 {
		t.Error("Failed to set availableReplicas correctly")
	}
	if set.Status.UpdatedReplicas != 3 {
		t.Error("Failed to set updatedReplicas to 3")
	}
	if set.Status.UpdatedAvailableReplicas != 4 {
		t.Error("Failed to set updatedAvailableReplicas correctly")
	}
}

func PodRecreateDeleteFailure(t *testing.T, set *appsv1beta1.StatefulSet, invariants invariantFunc) {
	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	om, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)

	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Error(err)
	}
	pods, err := om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}
	if err := ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Errorf("Error updating StatefulSet %s", err)
	}
	if err := invariants(set, om); err != nil {
		t.Error(err)
	}
	pods, err = om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}
	pods[0].Status.Phase = v1.PodFailed
	om.podsIndexer.Update(pods[0])
	om.SetDeleteStatefulPodError(apierrors.NewInternalError(errors.New("API server failed")), 0)
	if err := ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil && isOrHasInternalError(err) {
		t.Errorf("StatefulSet failed to %s", err)
	}
	if err := invariants(set, om); err != nil {
		t.Error(err)
	}
	if err := ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Errorf("Error updating StatefulSet %s", err)
	}
	if err := invariants(set, om); err != nil {
		t.Error(err)
	}
	pods, err = om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}
	if isCreated(pods[0]) {
		t.Error("StatefulSet did not recreate failed Pod")
	}
}

func TestStatefulSetControlScaleDownDeleteError(t *testing.T) {
	runTestOverPVCRetentionPolicies(
		t, "", func(t *testing.T, policy *appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy) {
			set := newStatefulSet(3)
			set.Spec.PersistentVolumeClaimRetentionPolicy = policy
			invariants := assertMonotonicInvariants
			client := fake.NewSimpleClientset()
			kruiseClient := kruisefake.NewSimpleClientset(set)
			om, _, ssc, stop := setupController(client, kruiseClient)
			defer close(stop)

			if err := scaleUpStatefulSetControl(set, ssc, om, invariants); err != nil {
				t.Errorf("Failed to turn up StatefulSet : %s", err)
			}
			var err error
			set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
			if err != nil {
				t.Fatalf("Error getting updated StatefulSet: %v", err)
			}
			*set.Spec.Replicas = 0
			om.SetDeleteStatefulPodError(apierrors.NewInternalError(errors.New("API server failed")), 2)
			if err := scaleDownStatefulSetControl(set, ssc, om, invariants); err != nil && isOrHasInternalError(err) {
				t.Errorf("StatefulSetControl failed to throw error on delete %s", err)
			}
			set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
			if err != nil {
				t.Fatalf("Error getting updated StatefulSet: %v", err)
			}
			if err := scaleDownStatefulSetControl(set, ssc, om, invariants); err != nil {
				t.Errorf("Failed to turn down StatefulSet %s", err)
			}
			set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
			if err != nil {
				t.Fatalf("Error getting updated StatefulSet: %v", err)
			}
			if set.Status.Replicas != 0 {
				t.Error("Failed to scale statefulset to 0 replicas")
			}
			if set.Status.ReadyReplicas != 0 {
				t.Error("Failed to set readyReplicas to 0")
			}
			if set.Status.UpdatedReplicas != 0 {
				t.Error("Failed to set updatedReplicas to 0")
			}
			if set.Status.UpdatedAvailableReplicas != 0 {
				t.Error("Failed to set updatedAvailableReplicas to 0")
			}
		})
}

func TestStatefulSetControl_getSetRevisions(t *testing.T) {
	type testcase struct {
		name            string
		existing        []*apps.ControllerRevision
		set             *appsv1beta1.StatefulSet
		expectedCount   int
		expectedCurrent *apps.ControllerRevision
		expectedUpdate  *apps.ControllerRevision
		err             bool
	}

	testFn := func(test *testcase, t *testing.T) {
		client := fake.NewSimpleClientset()
		kruiseClient := kruisefake.NewSimpleClientset()
		informerFactory := informers.NewSharedInformerFactory(client, controller.NoResyncPeriodFunc())
		kruiseInformerFactory := kruiseinformers.NewSharedInformerFactory(kruiseClient, controller.NoResyncPeriodFunc())
		om := newFakeObjectManager(informerFactory, kruiseInformerFactory)
		spc := NewStatefulPodControlFromManager(om, &noopRecorder{})
		ssu := newFakeStatefulSetStatusUpdater(kruiseInformerFactory.Apps().V1beta1().StatefulSets())
		recorder := record.NewFakeRecorder(10)
		inplaceControl := inplaceupdate.NewForInformer(informerFactory.Core().V1().Pods(), revisionadapter.NewDefaultImpl())
		lifecycleControl := lifecycle.NewForInformer(informerFactory.Core().V1().Pods())
		ssc := defaultStatefulSetControl{spc, ssu, history.NewFakeHistory(informerFactory.Apps().V1().ControllerRevisions()), recorder, inplaceControl, lifecycleControl}

		stop := make(chan struct{})
		defer close(stop)
		informerFactory.Start(stop)
		kruiseInformerFactory.Start(stop)
		cache.WaitForCacheSync(
			stop,
			kruiseInformerFactory.Apps().V1beta1().StatefulSets().Informer().HasSynced,
			// informerFactory.Apps().V1().StatefulSets().Informer().HasSynced,
			informerFactory.Core().V1().Pods().Informer().HasSynced,
			informerFactory.Apps().V1().ControllerRevisions().Informer().HasSynced,
		)
		test.set.Status.CollisionCount = new(int32)
		for i := range test.existing {
			ssc.controllerHistory.CreateControllerRevision(test.set, test.existing[i], test.set.Status.CollisionCount)
		}
		revisions, err := ssc.ListRevisions(test.set)
		if err != nil {
			t.Fatal(err)
		}
		current, update, _, err := ssc.getStatefulSetRevisions(test.set, revisions)
		if err != nil {
			t.Fatalf("error getting statefulset revisions:%v", err)
		}
		revisions, err = ssc.ListRevisions(test.set)
		if err != nil {
			t.Fatal(err)
		}
		if len(revisions) != test.expectedCount {
			t.Errorf("%s: want %d revisions got %d", test.name, test.expectedCount, len(revisions))
		}
		if test.err && err == nil {
			t.Errorf("%s: expected error", test.name)
		}
		if !test.err && !history.EqualRevision(current, test.expectedCurrent) {
			t.Errorf("%s: for current want %v got %v", test.name, test.expectedCurrent, current)
		}
		if !test.err && !history.EqualRevision(update, test.expectedUpdate) {
			t.Errorf("%s: for update want %v got %v", test.name, test.expectedUpdate, update)
		}
		if !test.err && test.expectedCurrent != nil && current != nil && test.expectedCurrent.Revision != current.Revision {
			t.Errorf("%s: for current revision want %d got %d", test.name, test.expectedCurrent.Revision, current.Revision)
		}
		if !test.err && test.expectedUpdate != nil && update != nil && test.expectedUpdate.Revision != update.Revision {
			t.Errorf("%s: for update revision want %d got %d", test.name, test.expectedUpdate.Revision, update.Revision)
		}
	}

	updateRevision := func(cr *apps.ControllerRevision, revision int64) *apps.ControllerRevision {
		clone := cr.DeepCopy()
		clone.Revision = revision
		return clone
	}

	runTestOverPVCRetentionPolicies(
		t, "", func(t *testing.T, policy *appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy) {
			set := newStatefulSet(3)
			set.Spec.PersistentVolumeClaimRetentionPolicy = policy
			set.Status.CollisionCount = new(int32)
			rev0 := newRevisionOrDie(set, 1)
			set1 := set.DeepCopy()
			set1.Spec.Template.Spec.Containers[0].Image = "foo"
			set1.Status.CurrentRevision = rev0.Name
			set1.Status.CollisionCount = new(int32)
			rev1 := newRevisionOrDie(set1, 2)
			set2 := set1.DeepCopy()
			set2.Spec.Template.Labels["new"] = "label"
			set2.Status.CurrentRevision = rev0.Name
			set2.Status.CollisionCount = new(int32)
			rev2 := newRevisionOrDie(set2, 3)
			tests := []testcase{
				{
					name:            "creates initial revision",
					existing:        nil,
					set:             set,
					expectedCount:   1,
					expectedCurrent: rev0,
					expectedUpdate:  rev0,
					err:             false,
				},
				{
					name:            "creates revision on update",
					existing:        []*apps.ControllerRevision{rev0},
					set:             set1,
					expectedCount:   2,
					expectedCurrent: rev0,
					expectedUpdate:  rev1,
					err:             false,
				},
				{
					name:            "must not recreate a new revision of same set",
					existing:        []*apps.ControllerRevision{rev0, rev1},
					set:             set1,
					expectedCount:   2,
					expectedCurrent: rev0,
					expectedUpdate:  rev1,
					err:             false,
				},
				{
					name:            "must rollback to a previous revision",
					existing:        []*apps.ControllerRevision{rev0, rev1, rev2},
					set:             set1,
					expectedCount:   3,
					expectedCurrent: rev0,
					expectedUpdate:  updateRevision(rev1, 4),
					err:             false,
				},
			}
			for i := range tests {
				testFn(&tests[i], t)
			}
		})
}

func TestStatefulSetControlRollingUpdate(t *testing.T) {
	type testcase struct {
		name       string
		invariants func(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error
		initial    func() *appsv1beta1.StatefulSet
		update     func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet
		validate   func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error
	}

	testFn := func(test *testcase, t *testing.T) {
		set := test.initial()
		client := fake.NewSimpleClientset()
		kruiseClient := kruisefake.NewSimpleClientset(set)
		om, _, ssc, stop := setupController(client, kruiseClient)
		defer close(stop)
		if err := scaleUpStatefulSetControl(set, ssc, om, test.invariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err := om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set = test.update(set)
		if err := updateStatefulSetControl(set, ssc, om, assertUpdateInvariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		pods, err := om.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err := test.validate(set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
	}

	tests := []testcase{
		{
			name:       "monotonic image update",
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "monotonic image update and scale up",
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 5
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "monotonic image update and scale down",
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(5)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 3
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update",
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(3))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update and scale up",
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(3))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 5
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update and scale down",
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(5))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 3
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
	}
	for i := range tests {
		testFn(&tests[i], t)
	}
}

func TestStatefulSetControlOnDeleteUpdate(t *testing.T) {
	type testcase struct {
		name            string
		invariants      func(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error
		initial         func() *appsv1beta1.StatefulSet
		update          func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet
		validateUpdate  func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error
		validateRestart func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error
	}

	originalImage := newStatefulSet(3).Spec.Template.Spec.Containers[0].Image

	testFn := func(t *testing.T, test *testcase, policy *appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy) {
		set := test.initial()
		set.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{Type: apps.OnDeleteStatefulSetStrategyType}
		client := fake.NewSimpleClientset()
		kruiseClient := kruisefake.NewSimpleClientset(set)
		om, _, ssc, stop := setupController(client, kruiseClient)
		defer close(stop)
		if err := scaleUpStatefulSetControl(set, ssc, om, test.invariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err := om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set = test.update(set)
		if err := updateStatefulSetControl(set, ssc, om, assertUpdateInvariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}

		// Pods may have been deleted in the update. Delete any claims with a pod ownerRef.
		selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		claims, err := om.claimsLister.PersistentVolumeClaims(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		for _, claim := range claims {
			for _, ref := range claim.GetOwnerReferences() {
				if strings.HasPrefix(ref.Name, "foo-") {
					om.claimsIndexer.Delete(claim)
					break
				}
			}
		}

		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		pods, err := om.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err := test.validateUpdate(set, pods); err != nil {
			for i := range pods {
				t.Log(pods[i].Name)
			}
			t.Fatalf("%s: %s", test.name, err)

		}
		claims, err = om.claimsLister.PersistentVolumeClaims(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		for _, claim := range claims {
			for _, ref := range claim.GetOwnerReferences() {
				if strings.HasPrefix(ref.Name, "foo-") {
					t.Fatalf("Unexpected pod reference on %s: %v", claim.Name, claim.GetOwnerReferences())
				}
			}
		}

		replicas := *set.Spec.Replicas
		*set.Spec.Replicas = 0
		if err := scaleDownStatefulSetControl(set, ssc, om, test.invariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		*set.Spec.Replicas = replicas

		claims, err = om.claimsLister.PersistentVolumeClaims(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		for _, claim := range claims {
			for _, ref := range claim.GetOwnerReferences() {
				if strings.HasPrefix(ref.Name, "foo-") {
					t.Fatalf("Unexpected pod reference on %s: %v", claim.Name, claim.GetOwnerReferences())
				}
			}
		}

		if err := scaleUpStatefulSetControl(set, ssc, om, test.invariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		pods, err = om.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err := test.validateRestart(set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
	}

	tests := []testcase{
		{
			name:       "monotonic image update",
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validateUpdate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
			validateRestart: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "monotonic image update and scale up",
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 5
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validateUpdate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if i < 3 && pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
					if i >= 3 && pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
			validateRestart: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "monotonic image update and scale down",
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(5)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 3
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validateUpdate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
			validateRestart: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update",
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(3))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validateUpdate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
			validateRestart: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update and scale up",
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(3))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 5
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validateUpdate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if i < 3 && pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
					if i >= 3 && pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
			validateRestart: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update and scale down",
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(5))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 3
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validateUpdate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
			validateRestart: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
	}
	runTestOverPVCRetentionPolicies(t, "", func(t *testing.T, policy *appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy) {
		for i := range tests {
			testFn(t, &tests[i], policy)
		}
	})
}

func TestStatefulSetControlRollingUpdateWithPaused(t *testing.T) {
	type testcase struct {
		name       string
		paused     bool
		invariants func(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error
		initial    func() *appsv1beta1.StatefulSet
		update     func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet
		validate   func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error
	}

	testFn := func(t *testing.T, test *testcase, policy *appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy) {
		set := test.initial()
		var partition int32
		set.Spec.PersistentVolumeClaimRetentionPolicy = policy
		set.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
			Type: apps.RollingUpdateStatefulSetStrategyType,
			RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
				return &appsv1beta1.RollingUpdateStatefulSetStrategy{
					Partition: &partition,
					Paused:    test.paused,
				}
			}(),
		}
		client := fake.NewSimpleClientset()
		kruiseClient := kruisefake.NewSimpleClientset(set)
		om, _, ssc, stop := setupController(client, kruiseClient)
		defer close(stop)
		if err := scaleUpStatefulSetControl(set, ssc, om, test.invariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err := om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set = test.update(set)
		if err := updateStatefulSetControl(set, ssc, om, assertUpdateInvariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		pods, err := om.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err := test.validate(set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
	}

	originalImage := newStatefulSet(3).Spec.Template.Spec.Containers[0].Image

	tests := []testcase{
		{
			name:       "monotonic image update",
			paused:     false,
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "monotonic image update with paused",
			paused:     true,
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "monotonic image update and scale up with paused",
			paused:     true,
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 5
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if i < 3 && pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
					if i >= 3 && pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update",
			paused:     false,
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(3))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update with paused",
			paused:     true,
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(3))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update and scale up with paused",
			paused:     true,
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(3))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 5
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if i < 3 && pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
					if i >= 3 && pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
	}
	runTestOverPVCRetentionPolicies(t, "", func(t *testing.T, policy *appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy) {
		for i := range tests {
			testFn(t, &tests[i], policy)
		}
	})
}

func TestScaleUpStatefulSetWithMinReadySeconds(t *testing.T) {
	type testcase struct {
		name            string
		minReadySeconds int32
		invariants      func(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error
		initial         func() *appsv1beta1.StatefulSet
		updatePod       func(om *fakeObjectManager, set *appsv1beta1.StatefulSet, pods []*v1.Pod) error
		validate        func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error
	}

	readyPods := func(partition, pauseSecond int) func(om *fakeObjectManager, set *appsv1beta1.StatefulSet,
		pods []*v1.Pod) error {
		return func(om *fakeObjectManager, set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
			sort.Sort(ascendingOrdinal(pods))
			for i := 0; i < partition; i++ {
				pod := pods[i].DeepCopy()
				pod.Status.Phase = v1.PodRunning
				condition := v1.PodCondition{Type: v1.PodReady, Status: v1.ConditionTrue}
				podutil.UpdatePodCondition(&pod.Status, &condition)
				fakeResourceVersion(pod)
				if err := om.podsIndexer.Update(pod); err != nil {
					return err
				}
			}
			time.Sleep(time.Duration(pauseSecond) * time.Second)
			return nil
		}
	}

	validateAllPodsReady := func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
		sort.Sort(ascendingOrdinal(pods))
		if len(pods) != 5 {
			return fmt.Errorf("we didn't get 5 pods exactly, num of pods = %d", len(pods))
		}
		for i := 0; i < 5; i++ {
			if !isRunningAndReady(pods[i]) {
				return fmt.Errorf("pod %s is not ready yet, status = %v", pods[i].Name, pods[i].Status)
			}
		}
		return nil
	}
	tests := []testcase{
		{
			name:            "monotonic scale up with 0 min ready seconds",
			minReadySeconds: 0,
			invariants:      assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			updatePod: readyPods(1, 0),
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				if len(pods) != 2 {
					return fmt.Errorf("we didn't get 2 pods exactly, num of pods = %d", len(pods))
				}
				if !isRunningAndReady(pods[0]) {
					return fmt.Errorf("pod %s is not ready yet, status = %v", pods[0].Name, pods[0].Status)
				}
				if isRunningAndReady(pods[1]) {
					return fmt.Errorf("pod %s is should not be ready yet, status = %v", pods[1].Name, pods[1].Status)
				}
				return nil
			},
		},
		{
			name:            "monotonic scaleup with 1 min ready seconds",
			minReadySeconds: 60,
			invariants:      assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			updatePod: readyPods(1, 0),
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				if len(pods) != 1 {
					return fmt.Errorf("we didn't get 1 pod exactly, num of pods = %d", len(pods))
				}
				if !isRunningAndReady(pods[0]) {
					return fmt.Errorf("pod %s is not ready yet, status = %v", pods[0].Name, pods[0].Status)
				}
				if avail, wait := isRunningAndAvailable(pods[0], 60); avail || wait == 0 {
					return fmt.Errorf("pod %s should not be ready yet, wait time = %s", pods[0].Name, wait)
				}
				return nil
			},
		},
		{
			name:            "monotonic scale up with 3 seconds ready seconds and sleep",
			minReadySeconds: 3,
			invariants:      assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			updatePod: readyPods(1, 5),
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				if len(pods) != 2 {
					return fmt.Errorf("we didn't get 2 pods exactly, num of pods = %d", len(pods))
				}
				if !isRunningAndReady(pods[0]) {
					return fmt.Errorf("pod %s is not ready yet, status = %v", pods[0].Name, pods[0].Status)
				}
				if isRunningAndReady(pods[1]) {
					return fmt.Errorf("pod %s is should not be ready yet, status = %v", pods[1].Name, pods[1].Status)
				}
				return nil
			},
		},
		{
			name:            "burst scale with 0 min ready seconds burst",
			minReadySeconds: 0,
			invariants:      assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(5))
			},
			updatePod: readyPods(5, 0),
			validate:  validateAllPodsReady,
		},
		{
			name:            "burst scale with 1 min ready seconds still burst",
			minReadySeconds: 60,
			invariants:      assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(5))
			},
			updatePod: readyPods(5, 0),
			validate:  validateAllPodsReady,
		},
	}

	testFn := func(test *testcase, t *testing.T) {
		// init according to test
		set := test.initial()
		// modify according to test
		set.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
			Type: apps.RollingUpdateStatefulSetStrategyType,
			RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
				return &appsv1beta1.RollingUpdateStatefulSetStrategy{
					Partition:       utilpointer.Int32Ptr(0),
					MinReadySeconds: &test.minReadySeconds,
				}
			}(),
		}
		// setup
		client := fake.NewSimpleClientset()
		kruiseClient := kruisefake.NewSimpleClientset(set)
		spc, _, ssc, stop := setupController(client, kruiseClient)
		defer close(stop)
		// reconcile once, start with no pod
		if err := ssc.UpdateStatefulSet(context.TODO(), set, []*v1.Pod{}); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		// update the pods
		selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		sort.Sort(ascendingOrdinal(pods))
		if err := test.updatePod(spc, set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		// reconcile once more
		pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		sort.Sort(ascendingOrdinal(pods))
		if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		// validate the result
		pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err := test.validate(set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
	}

	for i := range tests {
		testFn(&tests[i], t)
	}
}

func TestUpdateStatefulSetWithMinReadySeconds(t *testing.T) {
	type testcase struct {
		name            string
		minReadySeconds int32
		maxUnavailable  intstr.IntOrString
		partition       int
		updatePod       func(om *fakeObjectManager, set *appsv1beta1.StatefulSet, pods []*v1.Pod) error
		validate        func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error
	}
	const setSize = 5
	// originalImage := newStatefulSet(1).Spec.Template.Spec.Containers[0].Image
	newImage := "foo"

	readyPods := func(partition, pauseSecond int) func(om *fakeObjectManager, set *appsv1beta1.StatefulSet,
		pods []*v1.Pod) error {
		return func(om *fakeObjectManager, set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
			sort.Sort(ascendingOrdinal(pods))
			for i := setSize - 1; i >= partition; i-- {
				pod := pods[i].DeepCopy()
				pod.Status.Phase = v1.PodRunning
				condition := v1.PodCondition{Type: v1.PodReady, Status: v1.ConditionTrue}
				podutil.UpdatePodCondition(&pod.Status, &condition)
				// force reset pod ready time to now because we use InPlaceIfPossible strategy in these testcases and
				// the fakeObjectManager won't reset the pod's status when update pod in place.
				podutil.GetPodReadyCondition(pod.Status).LastTransitionTime = metav1.Now()
				fakeResourceVersion(pod)
				if err := om.podsIndexer.Update(pod); err != nil {
					return err
				}
			}
			time.Sleep(time.Duration(pauseSecond) * time.Second)
			return nil
		}
	}

	validatePodsUpdated := func(partition int) func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
		return func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
			sort.Sort(ascendingOrdinal(pods))
			i := setSize - 1
			for ; i >= partition; i-- {
				if !isRunningAndReady(pods[i]) {
					return fmt.Errorf("pod %s is not ready yet, status = %v", pods[i].Name, pods[i].Status)
				}
				if pods[i].Spec.Containers[0].Image != newImage {
					return fmt.Errorf("pod %s is not updated yet, pod revision = %s", pods[i].Name, getPodRevision(pods[i]))
				}
			}
			if i >= 0 {
				if pods[i].Spec.Containers[0].Image == newImage {
					return fmt.Errorf("pod %s should not be updated yet, pod revision = %s", pods[i].Name,
						getPodRevision(pods[i]))
				}
			}
			return nil
		}
	}

	tests := []testcase{
		{
			name:            "update with 0 min ready seconds",
			minReadySeconds: 0,
			maxUnavailable:  intstr.FromInt(1),
			partition:       4,
			updatePod:       readyPods(4, 0),
			validate:        validatePodsUpdated(3), // only 2 are upgraded
		},
		{
			name:            "update with 1 min ready seconds",
			minReadySeconds: 60,
			maxUnavailable:  intstr.FromInt(1),
			partition:       4,
			updatePod:       readyPods(4, 0),
			validate:        validatePodsUpdated(4), // only one is upgraded
		},
		{
			name:            "update with 1 min ready seconds and sleep",
			minReadySeconds: 5,
			maxUnavailable:  intstr.FromInt(1),
			partition:       4,
			updatePod:       readyPods(4, 10),
			validate:        validatePodsUpdated(3), // only 2 are upgraded
		},
		{
			name:            "update with 0 min ready seconds and 2 max unavailable",
			minReadySeconds: 0,
			maxUnavailable:  intstr.FromInt(2),
			partition:       4,
			updatePod:       readyPods(3, 0),
			validate:        validatePodsUpdated(1), // 4 are upgraded
		},
		{
			name:            "update with 1 min ready seconds and 2 max unavailable",
			minReadySeconds: 60,
			maxUnavailable:  intstr.FromInt(2),
			partition:       4,
			updatePod:       readyPods(3, 0),
			validate:        validatePodsUpdated(3), // only 2 are upgraded
		},
		{
			name:            "update with 1 min ready seconds and 2 max unavailable and sleep",
			minReadySeconds: 5,
			maxUnavailable:  intstr.FromInt(2),
			partition:       4,
			updatePod:       readyPods(3, 10),
			validate:        validatePodsUpdated(1), // 4 are upgraded
		},
	}

	testFn := func(test *testcase, t *testing.T) {
		// use burst mode to get around minReadyMin
		set := burst(newStatefulSet(setSize))
		// modify according to test
		set.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
			Type: apps.RollingUpdateStatefulSetStrategyType,
			RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
				return &appsv1beta1.RollingUpdateStatefulSetStrategy{
					Partition:       utilpointer.Int32Ptr(0),
					MaxUnavailable:  &test.maxUnavailable,
					PodUpdatePolicy: appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
					MinReadySeconds: &test.minReadySeconds,
				}
			}(),
		}
		// setup
		client := fake.NewSimpleClientset()
		kruiseClient := kruisefake.NewSimpleClientset(set)
		spc, _, ssc, stop := setupController(client, kruiseClient)
		defer close(stop)
		// scale the statefulset up to the target first
		if err := scaleUpStatefulSetControl(set, ssc, spc, assertBurstInvariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err := spc.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		// update the image
		set.Spec.Template.Spec.Containers[0].Image = "foo"
		// reconcile once, start with no pod updated
		if err := ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		// get the pods
		pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err := test.updatePod(spc, set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		// reconcile twice
		pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		// validate the result
		pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err := test.validate(set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
	}

	for i := range tests {
		testFn(&tests[i], t)
	}
}

func TestStatefulSetControlRollingUpdateWithPartition(t *testing.T) {
	type testcase struct {
		name       string
		partition  int32
		invariants func(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error
		initial    func() *appsv1beta1.StatefulSet
		update     func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet
		validate   func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error
	}

	testFn := func(test *testcase, t *testing.T) {
		set := test.initial()
		set.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
			Type: apps.RollingUpdateStatefulSetStrategyType,
			RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
				return &appsv1beta1.RollingUpdateStatefulSetStrategy{Partition: &test.partition}
			}(),
		}
		client := fake.NewSimpleClientset()
		kruiseClient := kruisefake.NewSimpleClientset(set)
		spc, _, ssc, stop := setupController(client, kruiseClient)
		defer close(stop)
		if err := scaleUpStatefulSetControl(set, ssc, spc, test.invariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err := spc.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set = test.update(set)
		if err := updateStatefulSetControl(set, ssc, spc, assertUpdateInvariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err = spc.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err := test.validate(set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
	}

	originalImage := newStatefulSet(3).Spec.Template.Spec.Containers[0].Image

	tests := []testcase{
		{
			name:       "monotonic image update",
			invariants: assertMonotonicInvariants,
			partition:  2,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if i < 2 && pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
					if i >= 2 && pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "monotonic image update and scale up",
			partition:  2,
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 5
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if i < 2 && pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
					if i >= 2 && pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update",
			partition:  2,
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(3))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if i < 2 && pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
					if i >= 2 && pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update and scale up",
			invariants: assertBurstInvariants,
			partition:  2,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(3))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 5
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if i < 2 && pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
					if i >= 2 && pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
	}
	for i := range tests {
		testFn(&tests[i], t)
	}
}

func TestStatefulSetControlRollingUpdateWithMaxUnavailable(t *testing.T) {
	set := burst(newStatefulSet(6))
	var partition int32 = 3
	var maxUnavailable = intstr.FromInt(2)
	set.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
		Type: apps.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
			return &appsv1beta1.RollingUpdateStatefulSetStrategy{
				Partition:      &partition,
				MaxUnavailable: &maxUnavailable,
			}
		}(),
	}

	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	spc, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)
	if err := scaleUpStatefulSetControl(set, ssc, spc, assertBurstInvariants); err != nil {
		t.Fatal(err)
	}
	set, err := spc.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatal(err)
	}

	// start to update
	set.Spec.Template.Spec.Containers[0].Image = "foo"

	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Fatal(err)
	}
	originalPods, err := spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(originalPods))

	// first update pods 4/5
	if err = ssc.UpdateStatefulSet(context.TODO(), set, originalPods); err != nil {
		t.Fatal(err)
	}
	pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if !reflect.DeepEqual(pods, originalPods[:4]) {
		t.Fatalf("Expected pods %v, got pods %v", originalPods[:3], pods)
	}

	// create new pods 4/5
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 6 {
		t.Fatalf("Expected create pods 4/5, got pods %v", pods)
	}

	// if pod 4 ready, start to update pod 3
	spc.setPodRunning(set, 4)
	spc.setPodRunning(set, 5)
	originalPods, _ = spc.setPodReady(set, 4)
	sort.Sort(ascendingOrdinal(originalPods))
	if err = ssc.UpdateStatefulSet(context.TODO(), set, originalPods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if !reflect.DeepEqual(pods, append(originalPods[:3], originalPods[4:]...)) {
		t.Fatalf("Expected pods %v, got pods %v", append(originalPods[:3], originalPods[4:]...), pods)
	}

	// create new pod 3
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 6 {
		t.Fatalf("Expected create pods 2/3, got pods %v", pods)
	}

	// pods 3/4/5 ready, should not update other pods
	spc.setPodRunning(set, 3)
	spc.setPodRunning(set, 5)
	spc.setPodReady(set, 5)
	originalPods, _ = spc.setPodReady(set, 3)
	sort.Sort(ascendingOrdinal(originalPods))
	if err = ssc.UpdateStatefulSet(context.TODO(), set, originalPods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if !reflect.DeepEqual(pods, originalPods) {
		t.Fatalf("Expected pods %v, got pods %v", originalPods, pods)
	}
}

func TestStatefulSetControlRollingUpdateBlockByMaxUnavailable(t *testing.T) {
	set := burst(newStatefulSet(6))
	var partition int32 = 3
	var maxUnavailable = intstr.FromInt(2)
	set.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
		Type: apps.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
			return &appsv1beta1.RollingUpdateStatefulSetStrategy{
				Partition:      &partition,
				MaxUnavailable: &maxUnavailable,
			}
		}(),
	}

	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	spc, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)
	if err := scaleUpStatefulSetControl(set, ssc, spc, assertBurstInvariants); err != nil {
		t.Fatal(err)
	}
	set, err := spc.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatal(err)
	}
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Fatal(err)
	}

	// set pod 0 to terminating
	originalPods, err := spc.setPodTerminated(set, 0)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(originalPods))

	// start to update
	set.Spec.Template.Spec.Containers[0].Image = "foo"

	// first update pod 5 only because pod 0 is terminating
	if err = ssc.UpdateStatefulSet(context.TODO(), set, originalPods); err != nil {
		t.Fatal(err)
	}
	pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if !reflect.DeepEqual(pods, originalPods[:5]) {
		t.Fatalf("Expected pods %v, got pods %v", originalPods[:3], pods)
	}

	// create new pods 5
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 6 {
		t.Fatalf("Expected create pods 5, got pods %v", pods)
	}

	// set pod 2 to terminating and pod 5 to ready
	spc.setPodTerminated(set, 2)
	spc.setPodRunning(set, 5)
	originalPods, _ = spc.setPodReady(set, 5)
	sort.Sort(ascendingOrdinal(originalPods))
	// should not update any pods because pod 0 and 2 are terminating
	if err = ssc.UpdateStatefulSet(context.TODO(), set, originalPods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if !reflect.DeepEqual(pods, originalPods) {
		t.Fatalf("Expected pods %v, got pods %v", originalPods, pods)
	}
}

func TestStatefulSetControlRollingUpdateWithSpecifiedDelete(t *testing.T) {
	set := burst(newStatefulSet(6))
	var partition int32 = 3
	var maxUnavailable = intstr.FromInt(3)
	set.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
		Type: apps.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
			return &appsv1beta1.RollingUpdateStatefulSetStrategy{
				Partition:       &partition,
				MaxUnavailable:  &maxUnavailable,
				PodUpdatePolicy: appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
			}
		}(),
	}

	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	spc, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)
	if err := scaleUpStatefulSetControl(set, ssc, spc, assertBurstInvariants); err != nil {
		t.Fatal(err)
	}
	set, err := spc.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatal(err)
	}
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Fatal(err)
	}

	// set pod 0 to specified delete
	originalPods, err := spc.setPodSpecifiedDelete(set, 0)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(originalPods))

	// start to update
	set.Spec.Template.Spec.Containers[0].Image = "foo"

	// first update pod 5 only because pod 0 is specified deleted
	if err = ssc.UpdateStatefulSet(context.TODO(), set, originalPods); err != nil {
		t.Fatal(err)
	}
	pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}

	// inplace update 5 and create 0
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 6 {
		t.Fatalf("Expected create pods 5, got pods %v", pods)
	}
	sort.Sort(ascendingOrdinal(pods))
	_, exist := pods[0].Labels[appsv1alpha1.SpecifiedDeleteKey]
	assert.True(t, !exist)
	// pod 0 is old image and pod 5/4 is new image
	assert.Equal(t, pods[5].Spec.Containers[0].Image, "foo")
	assert.Equal(t, pods[4].Spec.Containers[0].Image, "foo")
	assert.Equal(t, pods[0].Spec.Containers[0].Image, "nginx")

	// set pod 1/2/5 to specified deleted and pod 0/4/5 to ready
	spc.setPodSpecifiedDelete(set, 0)
	spc.setPodSpecifiedDelete(set, 1)
	spc.setPodSpecifiedDelete(set, 2)
	for i := 0; i < 6; i++ {
		spc.setPodRunning(set, i)
		spc.setPodReady(set, i)
	}
	originalPods, _ = spc.setPodSpecifiedDelete(set, 5)
	sort.Sort(ascendingOrdinal(originalPods))

	// create new pod for 1/2/5, do not update 3
	if err = ssc.UpdateStatefulSet(context.TODO(), set, originalPods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}

	// create new pods 5 and inplace update 3
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if len(pods) != 6 {
		t.Fatalf("Expected create pods 5, got pods %v", pods)
	}

	_, exist = pods[5].Labels[appsv1alpha1.SpecifiedDeleteKey]
	assert.True(t, !exist)
	_, exist = pods[2].Labels[appsv1alpha1.SpecifiedDeleteKey]
	assert.True(t, !exist)
	_, exist = pods[1].Labels[appsv1alpha1.SpecifiedDeleteKey]
	assert.True(t, !exist)
	// pod 0 still undeleted
	_, exist = pods[0].Labels[appsv1alpha1.SpecifiedDeleteKey]
	assert.True(t, exist)
	assert.Equal(t, pods[5].Spec.Containers[0].Image, "foo")
	assert.Equal(t, pods[3].Spec.Containers[0].Image, "nginx")
	assert.Equal(t, pods[2].Spec.Containers[0].Image, "nginx")
	assert.Equal(t, pods[1].Spec.Containers[0].Image, "nginx")

	// set pod 3 to specified deleted and all pod to ready => pod3 will be deleted and updated
	for i := 0; i < 6; i++ {
		spc.setPodRunning(set, i)
		spc.setPodReady(set, i)
	}
	originalPods, _ = spc.setPodSpecifiedDelete(set, 3)
	sort.Sort(ascendingOrdinal(originalPods))
	// create new pod for 3, do not inplace-update 3
	if err = ssc.UpdateStatefulSet(context.TODO(), set, originalPods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}

	// create new pods 5 and inplace update 3
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if len(pods) != 6 {
		t.Fatalf("Expected create pods 5, got pods %v", pods)
	}
	assert.Equal(t, pods[3].Spec.Containers[0].Image, "foo")
}

func TestStatefulSetControlInPlaceUpdate(t *testing.T) {
	set := burst(newStatefulSet(3))
	var partition int32 = 1
	set.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
		Type: apps.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
			return &appsv1beta1.RollingUpdateStatefulSetStrategy{
				Partition:       &partition,
				PodUpdatePolicy: appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
			}
		}(),
	}
	set.Spec.Template.Spec.ReadinessGates = append(set.Spec.Template.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})

	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	spc, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)
	if err := scaleUpStatefulSetControl(set, ssc, spc, assertBurstInvariants); err != nil {
		t.Fatal(err)
	}
	set, err := spc.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatal(err)
	}

	// ready to update
	set.Spec.Template.Spec.Containers[0].Image = "foo"

	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Fatal(err)
	}
	originalPods, err := spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(originalPods))
	// mock pod container statuses
	for _, p := range originalPods {
		p.Status.ContainerStatuses = append(p.Status.ContainerStatuses, v1.ContainerStatus{
			Name:    "nginx",
			ImageID: "imgID1",
		})
	}
	oldRevision := originalPods[2].Labels[apps.StatefulSetRevisionLabel]

	// in-place update pod 2
	if err = ssc.UpdateStatefulSet(context.TODO(), set, originalPods); err != nil {
		t.Fatal(err)
	}
	pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if len(pods) < 3 {
		t.Fatalf("Expected in-place update, actually got pods num: %v", len(pods))
	}

	if pods[2].Spec.Containers[0].Image != "foo" ||
		pods[2].Labels[apps.StatefulSetRevisionLabel] == oldRevision {
		t.Fatalf("Expected in-place update pod2, actually got %+v", pods[2])
	}
	condition := inplaceupdate.GetCondition(pods[2])
	if condition == nil || condition.Status != v1.ConditionFalse {
		t.Fatalf("Expected InPlaceUpdateReady condition False after in-place update, got %v", condition)
	}
	updateExpectations.ObserveUpdated(getStatefulSetKey(set), pods[2].Labels[apps.StatefulSetRevisionLabel], pods[2])
	if state := lifecycle.GetPodLifecycleState(pods[2]); state != appspub.LifecycleStateUpdating {
		t.Fatalf("Expected lifecycle to be Updating during in-place update: %v", state)
	}

	// should not update pod 1, because of pod2 status not changed
	if err = ssc.UpdateStatefulSet(context.TODO(), set, originalPods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if pods[1].Labels[apps.StatefulSetRevisionLabel] != oldRevision {
		t.Fatalf("Expected not to update pod1, actually got %+v", pods[1])
	}

	// update pod2 status, then update pod 1
	pods[2].Status.ContainerStatuses = []v1.ContainerStatus{{
		Name:    "nginx",
		ImageID: "imgID2",
	}}
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if state := lifecycle.GetPodLifecycleState(pods[2]); state != appspub.LifecycleStateNormal {
		t.Fatalf("Expected lifecycle to be Normal after in-place update: %v", state)
	}
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if len(pods) < 3 {
		t.Fatalf("Expected in-place update, actually got pods num: %v", len(pods))
	}

	if pods[1].Spec.Containers[0].Image != "foo" ||
		pods[1].Labels[apps.StatefulSetRevisionLabel] == oldRevision {
		t.Fatalf("Expected in-place update pod1, actually got %+v", pods[2])
	}
	condition = inplaceupdate.GetCondition(pods[1])
	if condition == nil || condition.Status != v1.ConditionFalse {
		t.Fatalf("Expected InPlaceUpdateReady condition False after in-place update, got %v", condition)
	}
	condition = inplaceupdate.GetCondition(pods[2])
	if condition == nil || condition.Status != v1.ConditionTrue {
		t.Fatalf("Expected InPlaceUpdateReady condition True after in-place update completed, got %v", condition)
	}
	updateExpectations.ObserveUpdated(getStatefulSetKey(set), pods[1].Labels[apps.StatefulSetRevisionLabel], pods[1])

	// should not update pod 0
	pods[1].Status.ContainerStatuses = []v1.ContainerStatus{{
		Name:    "nginx",
		ImageID: "imgID2",
	}}
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if len(pods) < 3 {
		t.Fatalf("Expected no update, actually got pods num: %v", len(pods))
	}
	if pods[0].Labels[apps.StatefulSetRevisionLabel] != oldRevision {
		t.Fatalf("Expected not to update pod0, actually got %+v", pods[1])
	}
	condition = inplaceupdate.GetCondition(pods[1])
	if condition == nil || condition.Status != v1.ConditionTrue {
		t.Fatalf("Expected InPlaceUpdateReady condition True after in-place update completed, got %v", condition)
	}
}

func TestStatefulSetControlLifecycleHook(t *testing.T) {
	set := burst(newStatefulSet(3))
	var partition int32 = 2
	set.Spec.Lifecycle = &appspub.Lifecycle{
		InPlaceUpdate: &appspub.LifecycleHook{
			LabelsHandler: map[string]string{
				"unready-block": "true",
			},
		},
	}
	set.Spec.Template.Labels["unready-block"] = "true"

	set.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
		Type: apps.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
			return &appsv1beta1.RollingUpdateStatefulSetStrategy{
				Partition:       &partition,
				PodUpdatePolicy: appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
			}
		}(),
	}
	set.Spec.Template.Spec.ReadinessGates = append(set.Spec.Template.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})

	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	spc, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)
	if err := scaleUpStatefulSetControl(set, ssc, spc, assertBurstInvariants); err != nil {
		t.Fatal(err)
	}
	set, err := spc.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatal(err)
	}

	// ready to update
	set.Spec.Template.Spec.Containers[0].Image = "foo"

	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Fatal(err)
	}
	originalPods, err := spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(originalPods))
	// mock pod container statuses
	for _, p := range originalPods {
		p.Status.ContainerStatuses = append(p.Status.ContainerStatuses, v1.ContainerStatus{
			Name:    "nginx",
			ImageID: "imgID1",
		})
	}
	oldRevision := originalPods[2].Labels[apps.StatefulSetRevisionLabel]

	// prepare in-place update pod 2
	if err = ssc.UpdateStatefulSet(context.TODO(), set, originalPods); err != nil {
		t.Fatal(err)
	}
	pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))

	if lifecycle.GetPodLifecycleState(pods[2]) != appspub.LifecycleStatePreparingUpdate {
		t.Fatalf("Expected pod2 in state %v, actually in state %v", appspub.LifecycleStatePreparingUpdate, lifecycle.GetPodLifecycleState(pods[2]))
	}

	// update pod2 label to be not hooked
	pods[2].Labels["unready-block"] = "false"

	// inplace update pod2
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if pods[2].Spec.Containers[0].Image != "foo" ||
		pods[2].Labels[apps.StatefulSetRevisionLabel] == oldRevision {
		t.Fatalf("Expected in-place update pod2, actually got %+v", pods[2])
	}
	condition := inplaceupdate.GetCondition(pods[2])
	if condition == nil || condition.Status != v1.ConditionFalse {
		t.Fatalf("Expected InPlaceUpdateReady condition False after in-place update, got %v", condition)
	}
	updateExpectations.ObserveUpdated(getStatefulSetKey(set), pods[2].Labels[apps.StatefulSetRevisionLabel], pods[2])

	// update pod2 status, make pod2 state to be Updated
	pods[2].Status.ContainerStatuses = []v1.ContainerStatus{{
		Name:    "nginx",
		ImageID: "imgID2",
	}}
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))

	if lifecycle.GetPodLifecycleState(pods[2]) != appspub.LifecycleStateUpdated {
		t.Fatalf("Expected pod2 in state %v, actually in state %v", appspub.LifecycleStateUpdated, lifecycle.GetPodLifecycleState(pods[2]))
	}

	// update pod2 to be hooked, pod2 state to Normal
	pods[2].Labels["unready-block"] = "true"
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))

	if lifecycle.GetPodLifecycleState(pods[2]) != appspub.LifecycleStateNormal {
		t.Fatalf("Expected pod2 in state %v, actually in state %v", appspub.LifecycleStateNormal, lifecycle.GetPodLifecycleState(pods[2]))
	}

	// should not prepare in-place update pod 1
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		t.Fatal(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if lifecycle.GetPodLifecycleState(pods[1]) == appspub.LifecycleStatePreparingUpdate {
		t.Fatalf("Expected pod1 in state %v, actually in state %v", appspub.LifecycleStatePreparingUpdate, lifecycle.GetPodLifecycleState(pods[1]))
	}
}

type manageCase struct {
	name           string
	set            *appsv1beta1.StatefulSet
	updateRevision *apps.ControllerRevision
	revisions      []*apps.ControllerRevision
	pods           []*v1.Pod
	expectedPods   []*v1.Pod
}

func TestUpdateWithLifecycleHook(t *testing.T) {
	testNs := "test"
	labelselector, _ := metav1.ParseToLabelSelector("test=asts")
	maxUnavailable := intstr.FromInt(1)
	now := metav1.NewTime(time.Unix(time.Now().Add(-time.Hour).Unix(), 0))

	cases := []manageCase{
		{
			name: "create: preparingNormal->Normal without hook",
			set: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: testNs},
				Spec: appsv1beta1.StatefulSetSpec{Replicas: utilpointer.Int32(1),
					Selector: labelselector,
				}},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
						}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sts-0",
						Namespace: testNs,
						UID:       "sts-0-uid",
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateNormal),
						}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
		},
		{
			name: "create: preparingNormal->preparingNormal, preNormal does not hook",
			set: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: testNs},
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:  utilpointer.Int32(1),
					Lifecycle: &appspub.Lifecycle{PreNormal: &appspub.LifecycleHook{LabelsHandler: map[string]string{"preNormalHooked": "true"}}},
					Selector:  labelselector,
				}},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
						}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sts-0",
						Namespace: testNs,
						UID:       "sts-0-uid",
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
						}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
		},
		{
			name: "create: preparingNormal->preparingNormal, preNormal does not all hooked",
			set: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: testNs},
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas: utilpointer.Int32(1),
					Lifecycle: &appspub.Lifecycle{
						PreNormal: &appspub.LifecycleHook{
							LabelsHandler:     map[string]string{"preNormalHooked": "true"},
							FinalizersHandler: []string{"slb"},
						},
					},
					Selector: labelselector,
				}},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
						}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sts-0",
						Namespace: testNs,
						UID:       "sts-0-uid",
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
						}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
		},
		{
			name: "create: preparingNormal->Normal, preNormal does hook",
			set: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: testNs},
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:  utilpointer.Int32(1),
					Lifecycle: &appspub.Lifecycle{PreNormal: &appspub.LifecycleHook{LabelsHandler: map[string]string{"preNormalHooked": "true"}}},
					Selector:  labelselector,
				}},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							"preNormalHooked":                    "true",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
						}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sts-0",
						Namespace: testNs,
						UID:       "sts-0-uid",
						Labels: map[string]string{
							"test":                               "asts",
							"preNormalHooked":                    "true",
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateNormal),
						}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
		},
		{
			name: "recreate update: Parallel-preparingNormal, pre-delete does not hook",
			set: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: testNs},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: apps.ParallelPodManagement,
					Replicas:            utilpointer.Int32(1),
					Lifecycle: &appspub.Lifecycle{
						PreNormal: &appspub.LifecycleHook{LabelsHandler: map[string]string{"preNormalHooked": "true"}},
						PreDelete: &appspub.LifecycleHook{LabelsHandler: map[string]string{"preNormalHooked": "true"}},
					},
					Selector: labelselector,
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							MaxUnavailable: &maxUnavailable,
						},
					},
				},
			},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							"test":                               "asts",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
						}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionTrue},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPods: []*v1.Pod{},
		},
		{
			name: "recreate update: Parallel-preparingNormal, pre-delete does hook",
			set: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: testNs},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: apps.ParallelPodManagement,
					Replicas:            utilpointer.Int32(1),
					Lifecycle: &appspub.Lifecycle{
						PreNormal: &appspub.LifecycleHook{LabelsHandler: map[string]string{"preNormalHooked": "true"}},
						PreDelete: &appspub.LifecycleHook{LabelsHandler: map[string]string{"preDeleteHooked": "true"}},
					},
					Selector: labelselector,
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							MaxUnavailable: &maxUnavailable,
						},
					},
				},
			},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"preDeleteHooked":                    "true",
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
						}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionTrue},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"preDeleteHooked":                    "true",
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingDelete),
						},
						UID: "sts-0-uid",
					},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionTrue},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
		},
		{
			name: "in-place update: preparingNormal->Updating, preNormal & InPlaceUpdate does not hook",
			set: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: testNs},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: apps.ParallelPodManagement,
					Replicas:            utilpointer.Int32(1),
					Lifecycle:           &appspub.Lifecycle{PreNormal: &appspub.LifecycleHook{FinalizersHandler: []string{"slb.com/online"}}},
					Selector:            labelselector,
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							MaxUnavailable:  &maxUnavailable,
							PodUpdatePolicy: appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
						},
					},
				},
			},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       apiruntime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       apiruntime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
						}},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"apps.kubernetes.io/pod-index":       "0",
							"statefulset.kubernetes.io/pod-name": "sts-0",
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateUpdating),
						},
						Annotations: map[string]string{
							appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{
								Revision:               "rev_new",
								UpdateTimestamp:        metav1.NewTime(now.Time),
								LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "image-id-xyz"}},
								UpdateImages:           true,
								ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: now, Containers: []string{"c1"}}},
							}),
						},
						UID: "sts-0-uid",
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo2"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning, Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionFalse, Reason: "StartInPlaceUpdate", LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
		},
		{
			name: "in-place update: preparingNormal->Normal, preNormal & InPlaceUpdate does hook",
			set: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: testNs},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: apps.ParallelPodManagement,
					Replicas:            utilpointer.Int32(1),
					Lifecycle: &appspub.Lifecycle{PreNormal: &appspub.LifecycleHook{FinalizersHandler: []string{"slb/online"}},
						InPlaceUpdate: &appspub.LifecycleHook{FinalizersHandler: []string{"slb/online"}}},
					Selector: labelselector,
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							MaxUnavailable:  &maxUnavailable,
							PodUpdatePolicy: appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
						},
					},
				},
			},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       apiruntime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       apiruntime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"apps.kubernetes.io/pod-index":       "0",
							"statefulset.kubernetes.io/pod-name": "sts-0",
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
						},
						Finalizers: []string{"slb/online"},
					},

					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue, LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"apps.kubernetes.io/pod-index":       "0",
							"statefulset.kubernetes.io/pod-name": "sts-0",
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateNormal),
						},
						Finalizers: []string{"slb/online"},
						UID:        "sts-0-uid",
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning, Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue, LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
		},
		{
			name: "in-place update: preparingUpdate->Updating, InPlaceUpdate does not hook",
			set: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: testNs},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: apps.ParallelPodManagement,
					Replicas:            utilpointer.Int32(1),
					Lifecycle:           &appspub.Lifecycle{InPlaceUpdate: &appspub.LifecycleHook{FinalizersHandler: []string{"slb/online"}}},
					Selector:            labelselector,
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							MaxUnavailable:  &maxUnavailable,
							PodUpdatePolicy: appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
						},
					},
				},
			},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       apiruntime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       apiruntime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingUpdate),
						}},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"apps.kubernetes.io/pod-index":       "0",
							"statefulset.kubernetes.io/pod-name": "sts-0",
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateUpdating),
						},
						Annotations: map[string]string{
							appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{
								Revision:               "rev_new",
								UpdateTimestamp:        metav1.NewTime(now.Time),
								LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "image-id-xyz"}},
								UpdateImages:           true,
								ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: now, Containers: []string{"c1"}}},
							}),
						},
						UID: "sts-0-uid",
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo2"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning, Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionFalse, Reason: "StartInPlaceUpdate", LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
		},
		{
			name: "in-place update: Updated->Normal, InPlaceUpdate does not hook",
			set: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: testNs},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: apps.ParallelPodManagement,
					Replicas:            utilpointer.Int32(1),
					Lifecycle:           &appspub.Lifecycle{InPlaceUpdate: &appspub.LifecycleHook{FinalizersHandler: []string{"slb/online"}}},
					Selector:            labelselector,
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							MaxUnavailable:  &maxUnavailable,
							PodUpdatePolicy: appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
						},
					},
				},
			},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       apiruntime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       apiruntime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateUpdated),
						},
						Finalizers: []string{"slb/online"},
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateNormal),
						},
						Finalizers: []string{"slb/online"},
						UID:        "sts-0-uid",
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning, Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
		},
		{
			name: "in-place update: Normal->PrepareUpdating, InPlaceUpdate does hook",
			set: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: testNs},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: apps.ParallelPodManagement,
					Replicas:            utilpointer.Int32(1),
					Lifecycle:           &appspub.Lifecycle{InPlaceUpdate: &appspub.LifecycleHook{FinalizersHandler: []string{"slb/online"}}},
					Selector:            labelselector,
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
							MaxUnavailable:  &maxUnavailable,
							PodUpdatePolicy: appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
						},
					},
				},
			},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       apiruntime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       apiruntime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateNormal),
						},
						Finalizers: []string{"slb/online"},
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sts-0",
						Namespace: testNs,
						Labels: map[string]string{
							"test":                               "asts",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingUpdate),
						},
						Finalizers: []string{"slb/online"},
						UID:        "sts-0-uid",
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning, Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
		},
	}
	inplaceupdate.Clock = testingclock.NewFakeClock(now.Time)
	for _, mc := range cases {
		t.Run(mc.name, func(t *testing.T) {
			updateExpectations.DeleteExpectations(getStatefulSetKey(mc.set))
			fakeClient := fake.NewSimpleClientset()
			kruiseClient := kruisefake.NewSimpleClientset(mc.set)
			om, _, ssc, stop := setupController(fakeClient, kruiseClient)
			stsController := ssc.(*defaultStatefulSetControl)
			defer close(stop)

			spc := stsController.podControl
			for _, po := range mc.pods {
				err := spc.objectMgr.CreatePod(context.TODO(), po)
				assert.Nil(t, err)
			}

			currentRevision := mc.updateRevision
			if len(mc.revisions) > 0 {
				currentRevision = mc.revisions[0]
			}
			if _, err := stsController.updateStatefulSet(context.TODO(), mc.set, currentRevision, mc.updateRevision, 0, mc.pods, mc.revisions); err != nil {
				t.Fatalf("Failed to test %s, manage error: %v", mc.name, err)
			}

			selector, err := metav1.LabelSelectorAsSelector(mc.set.Spec.Selector)
			if err != nil {
				t.Fatal(err)
			}
			pods, err := om.podsLister.Pods(mc.set.Namespace).List(selector)
			assert.Nil(t, err)
			if len(pods) != len(mc.expectedPods) {
				t.Fatalf("Failed to test %s, unexpected pods length, expected %v, got %v", mc.name, util.DumpJSON(mc.expectedPods), util.DumpJSON(pods))
			}
			for _, p := range mc.expectedPods {
				p.APIVersion = "v1"
				p.Kind = "Pod"

				var gotPod *v1.Pod
				if gotPod, err = om.podsLister.Pods(p.Namespace).Get(p.Name); err != nil {
					t.Fatalf("Failed to test %s, get pod %s error: %v", mc.name, p.Name, err)
				}
				gotPod.APIVersion = "v1"
				gotPod.Kind = "Pod"

				if v, ok := gotPod.Annotations[appspub.LifecycleTimestampKey]; ok {
					if p.Annotations == nil {
						p.Annotations = map[string]string{}
					}
					p.Annotations[appspub.LifecycleTimestampKey] = v
				}
				p.ResourceVersion = gotPod.ResourceVersion

				if !reflect.DeepEqual(gotPod, p) {
					t.Fatalf("Failed to test %s, unexpected pod %s, expected \n%v\n got \n%v", mc.name, p.Name, util.DumpJSON(p), util.DumpJSON(gotPod))
				}
			}
		})
	}
}

func TestStatefulSetHonorRevisionHistoryLimit(t *testing.T) {
	runTestOverPVCRetentionPolicies(t, "", func(t *testing.T, policy *appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy) {
		invariants := assertMonotonicInvariants
		set := newStatefulSet(3)
		set.Spec.PersistentVolumeClaimRetentionPolicy = policy
		client := fake.NewSimpleClientset()
		kruiseClient := kruisefake.NewSimpleClientset(set)
		om, ssu, ssc, stop := setupController(client, kruiseClient)
		defer close(stop)

		if err := scaleUpStatefulSetControl(set, ssc, om, invariants); err != nil {
			t.Errorf("Failed to turn up StatefulSet : %s", err)
		}
		var err error
		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("Error getting updated StatefulSet: %v", err)
		}

		for i := 0; i < int(*set.Spec.RevisionHistoryLimit)+5; i++ {
			set.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("foo-%d", i)
			ssu.SetUpdateStatefulSetStatusError(apierrors.NewInternalError(errors.New("API server failed")), 2)
			updateStatefulSetControl(set, ssc, om, assertUpdateInvariants)
			set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
			if err != nil {
				t.Fatalf("Error getting updated StatefulSet: %v", err)
			}
			revisions, err := ssc.ListRevisions(set)
			if err != nil {
				t.Fatalf("Error listing revisions: %v", err)
			}
			// the extra 2 revisions are `currentRevision` and `updateRevision`
			// They're considered as `live`, and truncateHistory only cleans up non-live revisions
			if len(revisions) > int(*set.Spec.RevisionHistoryLimit)+2 {
				t.Fatalf("%s: %d greater than limit %d", "", len(revisions), *set.Spec.RevisionHistoryLimit)
			}
		}
	})
}

func TestStatefulSetControlLimitsHistory(t *testing.T) {
	type testcase struct {
		name       string
		invariants func(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error
		initial    func() *appsv1beta1.StatefulSet
	}

	testFn := func(t *testing.T, test *testcase) {
		set := test.initial()
		client := fake.NewSimpleClientset()
		kruiseClient := kruisefake.NewSimpleClientset(set)
		om, _, ssc, stop := setupController(client, kruiseClient)
		defer close(stop)
		if err := scaleUpStatefulSetControl(set, ssc, om, test.invariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err := om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		for i := 0; i < 10; i++ {
			set.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("foo-%d", i)
			if err := updateStatefulSetControl(set, ssc, om, assertUpdateInvariants); err != nil {
				t.Fatalf("%s: %s", test.name, err)
			}
			selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
			if err != nil {
				t.Fatalf("%s: %s", test.name, err)
			}
			pods, err := om.podsLister.Pods(set.Namespace).List(selector)
			if err != nil {
				t.Fatalf("%s: %s", test.name, err)
			}
			set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
			if err != nil {
				t.Fatalf("%s: %s", test.name, err)
			}
			err = ssc.UpdateStatefulSet(context.TODO(), set, pods)
			if err != nil {
				t.Fatalf("%s: %s", test.name, err)
			}
			revisions, err := ssc.ListRevisions(set)
			if err != nil {
				t.Fatalf("%s: %s", test.name, err)
			}
			if len(revisions) > int(*set.Spec.RevisionHistoryLimit)+2 {
				t.Fatalf("%s: %d greater than limit %d", test.name, len(revisions), *set.Spec.RevisionHistoryLimit)
			}
		}
	}

	tests := []testcase{
		{
			name:       "monotonic update",
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
		},
		{
			name:       "burst update",
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(3))
			},
		},
	}
	for i := range tests {
		testFn(t, &tests[i])
	}
}

func TestStatefulSetControlRollback(t *testing.T) {
	type testcase struct {
		name             string
		invariants       func(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error
		initial          func() *appsv1beta1.StatefulSet
		update           func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet
		validateUpdate   func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error
		validateRollback func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error
	}

	originalImage := newStatefulSet(3).Spec.Template.Spec.Containers[0].Image

	testFn := func(test *testcase, t *testing.T) {
		set := test.initial()
		client := fake.NewSimpleClientset()
		kruiseClient := kruisefake.NewSimpleClientset(set)
		om, _, ssc, stop := setupController(client, kruiseClient)
		defer close(stop)
		if err := scaleUpStatefulSetControl(set, ssc, om, test.invariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err := om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set = test.update(set)
		if err := updateStatefulSetControl(set, ssc, om, assertUpdateInvariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		pods, err := om.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err := test.validateUpdate(set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		revisions, err := ssc.ListRevisions(set)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		history.SortControllerRevisions(revisions)
		set, err = ApplyRevision(set, revisions[0])
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err := updateStatefulSetControl(set, ssc, om, assertUpdateInvariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		pods, err = om.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if err := test.validateRollback(set, pods); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
	}

	tests := []testcase{
		{
			name:       "monotonic image update",
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validateUpdate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
			validateRollback: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "monotonic image update and scale up",
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(3)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 5
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validateUpdate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
			validateRollback: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "monotonic image update and scale down",
			invariants: assertMonotonicInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return newStatefulSet(5)
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 3
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validateUpdate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
			validateRollback: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update",
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(3))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validateUpdate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
			validateRollback: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update and scale up",
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(3))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 5
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validateUpdate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
			validateRollback: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
		{
			name:       "burst image update and scale down",
			invariants: assertBurstInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return burst(newStatefulSet(5))
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				*set.Spec.Replicas = 3
				set.Spec.Template.Spec.Containers[0].Image = "foo"
				return set
			},
			validateUpdate: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != "foo" {
						return fmt.Errorf("want pod %s image foo found %s", pods[i].Name, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
			validateRollback: func(set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
				sort.Sort(ascendingOrdinal(pods))
				for i := range pods {
					if pods[i].Spec.Containers[0].Image != originalImage {
						return fmt.Errorf("want pod %s image %s found %s", pods[i].Name, originalImage, pods[i].Spec.Containers[0].Image)
					}
				}
				return nil
			},
		},
	}
	for i := range tests {
		testFn(&tests[i], t)
	}
}

type requestTracker struct {
	l        sync.Mutex
	requests int
	err      error
	after    int
}

func (rt *requestTracker) errorReady() bool {
	return rt.err != nil && rt.requests >= rt.after
}

func (rt *requestTracker) inc() {
	rt.l.Lock()
	defer rt.l.Unlock()
	rt.requests++
}

func (rt *requestTracker) reset() {
	rt.l.Lock()
	defer rt.l.Unlock()
	rt.err = nil
	rt.after = 0
}

type fakeObjectManager struct {
	podsLister       corelisters.PodLister
	claimsLister     corelisters.PersistentVolumeClaimLister
	scLister         storagelisters.StorageClassLister
	setsLister       kruiseappslisters.StatefulSetLister
	podsIndexer      cache.Indexer
	claimsIndexer    cache.Indexer
	setsIndexer      cache.Indexer
	revisionsIndexer cache.Indexer
	createPodTracker requestTracker
	updatePodTracker requestTracker
	deletePodTracker requestTracker
}

func newFakeObjectManager(informerFactory informers.SharedInformerFactory, kruiseInformerFactory kruiseinformers.SharedInformerFactory) *fakeObjectManager {
	podInformer := informerFactory.Core().V1().Pods()
	claimInformer := informerFactory.Core().V1().PersistentVolumeClaims()
	revisionInformer := informerFactory.Apps().V1().ControllerRevisions()
	setInformer := kruiseInformerFactory.Apps().V1beta1().StatefulSets()
	scInformer := informerFactory.Storage().V1().StorageClasses()

	return &fakeObjectManager{
		podInformer.Lister(),
		claimInformer.Lister(),
		scInformer.Lister(),
		setInformer.Lister(),
		podInformer.Informer().GetIndexer(),
		claimInformer.Informer().GetIndexer(),
		setInformer.Informer().GetIndexer(),
		revisionInformer.Informer().GetIndexer(),
		requestTracker{sync.Mutex{}, 0, nil, 0},
		requestTracker{sync.Mutex{}, 0, nil, 0},
		requestTracker{sync.Mutex{}, 0, nil, 0}}
}

func (om *fakeObjectManager) CreatePod(ctx context.Context, pod *v1.Pod) error {
	defer om.createPodTracker.inc()
	if om.createPodTracker.errorReady() {
		defer om.createPodTracker.reset()
		return om.createPodTracker.err
	}
	pod.SetUID(types.UID(pod.Name + "-uid"))
	return om.podsIndexer.Update(pod)
}

func (om *fakeObjectManager) GetPod(namespace, podName string) (*v1.Pod, error) {
	return om.podsLister.Pods(namespace).Get(podName)
}

func (om *fakeObjectManager) UpdatePod(pod *v1.Pod) error {
	return om.podsIndexer.Update(pod)
}

func (om *fakeObjectManager) DeletePod(pod *v1.Pod) error {
	defer om.deletePodTracker.inc()
	if om.deletePodTracker.errorReady() {
		defer om.deletePodTracker.reset()
		return om.deletePodTracker.err
	}
	if key, err := controller.KeyFunc(pod); err != nil {
		return err
	} else if obj, found, err := om.podsIndexer.GetByKey(key); err != nil {
		return err
	} else if found {
		return om.podsIndexer.Delete(obj)
	}
	return nil // Not found, no error in deleting.
}

func (om *fakeObjectManager) CreateClaim(claim *v1.PersistentVolumeClaim) error {
	claimClone := claim.DeepCopy()
	om.claimsIndexer.Update(claimClone)
	return nil
}

func (om *fakeObjectManager) GetClaim(namespace, claimName string) (*v1.PersistentVolumeClaim, error) {
	return om.claimsLister.PersistentVolumeClaims(namespace).Get(claimName)
}

func (om *fakeObjectManager) UpdateClaim(claim *v1.PersistentVolumeClaim) error {
	// Validate ownerRefs.
	refs := claim.GetOwnerReferences()
	for _, ref := range refs {
		if ref.APIVersion == "" || ref.Kind == "" || ref.Name == "" {
			return fmt.Errorf("invalid ownerRefs: %s %v", claim.Name, refs)
		}
	}
	om.claimsIndexer.Update(claim)
	return nil
}

func (om *fakeObjectManager) GetStorageClass(scName string) (*storagev1.StorageClass, error) {
	return om.scLister.Get(scName)
}

func (om *fakeObjectManager) SetCreateStatefulPodError(err error, after int) {
	om.createPodTracker.err = err
	om.createPodTracker.after = after
}

func (om *fakeObjectManager) SetUpdateStatefulPodError(err error, after int) {
	om.updatePodTracker.err = err
	om.updatePodTracker.after = after
}

func (om *fakeObjectManager) SetDeleteStatefulPodError(err error, after int) {
	om.deletePodTracker.err = err
	om.deletePodTracker.after = after
}

func (om *fakeObjectManager) setPodPending(set *appsv1beta1.StatefulSet, ordinal int) ([]*v1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}
	pods, err := om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		return nil, err
	}
	if 0 > ordinal || ordinal >= len(pods) {
		return nil, fmt.Errorf("ordinal %d out of range [0,%d)", ordinal, len(pods))
	}
	sort.Sort(ascendingOrdinal(pods))
	pod := pods[ordinal].DeepCopy()
	pod.Status.Phase = v1.PodPending
	fakeResourceVersion(pod)
	om.podsIndexer.Update(pod)
	return om.podsLister.Pods(set.Namespace).List(selector)
}

func (om *fakeObjectManager) setPodRunning(set *appsv1beta1.StatefulSet, ordinal int) ([]*v1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}
	pods, err := om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		return nil, err
	}
	if 0 > ordinal || ordinal >= len(pods) {
		return nil, fmt.Errorf("ordinal %d out of range [0,%d)", ordinal, len(pods))
	}
	sort.Sort(ascendingOrdinal(pods))
	pod := pods[ordinal].DeepCopy()
	pod.Status.Phase = v1.PodRunning
	fakeResourceVersion(pod)
	om.podsIndexer.Update(pod)
	return om.podsLister.Pods(set.Namespace).List(selector)
}

func (om *fakeObjectManager) setPodReady(set *appsv1beta1.StatefulSet, ordinal int) ([]*v1.Pod, error) {
	return om.setPodReadyWithMinReadySeconds(set, ordinal, 0)
}

func (om *fakeObjectManager) setPodReadyWithMinReadySeconds(
	set *appsv1beta1.StatefulSet, ordinal int, minReadySeconds int32,
) ([]*v1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}
	pods, err := om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		return nil, err
	}
	if 0 > ordinal || ordinal >= len(pods) {
		return nil, fmt.Errorf("ordinal %d out of range [0,%d)", ordinal, len(pods))
	}
	sort.Sort(ascendingOrdinal(pods))
	pod := pods[ordinal].DeepCopy()
	condition := v1.PodCondition{Type: v1.PodReady, Status: v1.ConditionTrue}
	podutil.UpdatePodCondition(&pod.Status, &condition)
	if readyTime := podutil.GetPodReadyCondition(pod.Status).LastTransitionTime; minReadySeconds > 0 &&
		metav1.Now().Sub(readyTime.Time) < time.Duration(minReadySeconds)*time.Second {

		podutil.GetPodReadyCondition(pod.Status).LastTransitionTime = metav1.NewTime(
			time.Now().Add(-time.Second * time.Duration(minReadySeconds)),
		)
	}
	fakeResourceVersion(pod)
	om.podsIndexer.Update(pod)
	return om.podsLister.Pods(set.Namespace).List(selector)
}

func (om *fakeObjectManager) addTerminatingPod(set *appsv1beta1.StatefulSet, ordinal int) ([]*v1.Pod, error) {
	pod := newStatefulSetPod(set, ordinal)
	pod.SetUID(types.UID(pod.Name + "-uid")) // To match fakeObjectManager.CreatePod
	pod.Status.Phase = v1.PodRunning
	deleted := metav1.NewTime(time.Now())
	pod.DeletionTimestamp = &deleted
	condition := v1.PodCondition{Type: v1.PodReady, Status: v1.ConditionTrue}
	fakeResourceVersion(pod)
	podutil.UpdatePodCondition(&pod.Status, &condition)
	om.podsIndexer.Update(pod)
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}
	return om.podsLister.Pods(set.Namespace).List(selector)
}

func (om *fakeObjectManager) setPodTerminated(set *appsv1beta1.StatefulSet, ordinal int) ([]*v1.Pod, error) {
	pod := newStatefulSetPod(set, ordinal)
	deleted := metav1.NewTime(time.Now())
	pod.DeletionTimestamp = &deleted
	fakeResourceVersion(pod)
	om.podsIndexer.Update(pod)
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}
	return om.podsLister.Pods(set.Namespace).List(selector)
}

func (om *fakeObjectManager) setPodSpecifiedDelete(set *appsv1beta1.StatefulSet, ordinal int) ([]*v1.Pod, error) {
	pod := newStatefulSetPod(set, ordinal)
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels[appsv1alpha1.SpecifiedDeleteKey] = "true"
	fakeResourceVersion(pod)
	om.podsIndexer.Update(pod)
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}
	return om.podsLister.Pods(set.Namespace).List(selector)
}

var _ StatefulPodControlObjectManager = &fakeObjectManager{}

type fakeStatefulSetStatusUpdater struct {
	setsLister          kruiseappslisters.StatefulSetLister
	setsIndexer         cache.Indexer
	updateStatusTracker requestTracker
}

func newFakeStatefulSetStatusUpdater(setInformer kruiseappsinformers.StatefulSetInformer) *fakeStatefulSetStatusUpdater {
	return &fakeStatefulSetStatusUpdater{
		setInformer.Lister(),
		setInformer.Informer().GetIndexer(),
		requestTracker{sync.Mutex{}, 0, nil, 0},
	}
}

func (ssu *fakeStatefulSetStatusUpdater) UpdateStatefulSetStatus(ctx context.Context, set *appsv1beta1.StatefulSet, status *appsv1beta1.StatefulSetStatus) error {
	defer ssu.updateStatusTracker.inc()
	if ssu.updateStatusTracker.errorReady() {
		defer ssu.updateStatusTracker.reset()
		return ssu.updateStatusTracker.err
	}
	set.Status = *status
	ssu.setsIndexer.Update(set)
	return nil
}

func (ssu *fakeStatefulSetStatusUpdater) SetUpdateStatefulSetStatusError(err error, after int) {
	ssu.updateStatusTracker.err = err
	ssu.updateStatusTracker.after = after
}

var _ StatusUpdaterInterface = &fakeStatefulSetStatusUpdater{}

func assertMonotonicInvariants(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return err
	}
	pods, err := om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		return err
	}
	sort.Sort(ascendingOrdinal(pods))
	for ord := 0; ord < len(pods); ord++ {
		if ord > 0 && isRunningAndReady(pods[ord]) && !isRunningAndReady(pods[ord-1]) {
			return fmt.Errorf("Successor %s is Running and Ready while %s is not", pods[ord].Name, pods[ord-1].Name)
		}

		if getOrdinal(pods[ord]) != ord {
			return fmt.Errorf("pods %s deployed in the wrong order %d", pods[ord].Name, ord)
		}

		if !storageMatches(set, pods[ord]) {
			return fmt.Errorf("pods %s does not match the storage specification of StatefulSet %s ", pods[ord].Name, set.Name)
		}

		for _, claim := range getPersistentVolumeClaims(set, pods[ord]) {
			claim, _ := om.claimsLister.PersistentVolumeClaims(set.Namespace).Get(claim.Name)
			if err := checkClaimInvariants(set, pods[ord], claim, ord); err != nil {
				return err
			}
		}

		if !identityMatches(set, pods[ord]) {
			return fmt.Errorf("pods %s does not match the identity specification of StatefulSet %s ", pods[ord].Name, set.Name)
		}
	}
	return nil
}

func assertBurstInvariants(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return err
	}
	pods, err := om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		return err
	}
	sort.Sort(ascendingOrdinal(pods))
	for ord := 0; ord < len(pods); ord++ {
		if !storageMatches(set, pods[ord]) {
			return fmt.Errorf("pods %s does not match the storage specification of StatefulSet %s ", pods[ord].Name, set.Name)
		}

		for _, claim := range getPersistentVolumeClaims(set, pods[ord]) {
			claim, err := om.claimsLister.PersistentVolumeClaims(set.Namespace).Get(claim.Name)
			if err != nil {
				return err
			}
			if err := checkClaimInvariants(set, pods[ord], claim, ord); err != nil {
				return err
			}
		}

		if !identityMatches(set, pods[ord]) {
			return fmt.Errorf("pods %s does not match the identity specification of StatefulSet %s ",
				pods[ord].Name,
				set.Name)
		}
	}
	return nil
}

func assertUpdateInvariants(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return err
	}
	pods, err := om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		return err
	}
	sort.Sort(ascendingOrdinal(pods))
	for ord := 0; ord < len(pods); ord++ {

		if !storageMatches(set, pods[ord]) {
			return fmt.Errorf("pod %s does not match the storage specification of StatefulSet %s ", pods[ord].Name, set.Name)
		}

		for _, claim := range getPersistentVolumeClaims(set, pods[ord]) {
			claim, err := om.claimsLister.PersistentVolumeClaims(set.Namespace).Get(claim.Name)
			if err != nil {
				return err
			}
			if err := checkClaimInvariants(set, pods[ord], claim, ord); err != nil {
				return err
			}
		}

		if !identityMatches(set, pods[ord]) {
			return fmt.Errorf("pod %s does not match the identity specification of StatefulSet %s ", pods[ord].Name, set.Name)
		}
	}
	if set.Spec.UpdateStrategy.Type == apps.OnDeleteStatefulSetStrategyType {
		return nil
	}
	if set.Spec.UpdateStrategy.Type == apps.RollingUpdateStatefulSetStrategyType {
		for i := 0; i < int(set.Status.CurrentReplicas) && i < len(pods); i++ {
			if want, got := set.Status.CurrentRevision, getPodRevision(pods[i]); want != got {
				return fmt.Errorf("pod %s want current revision %s got %s", pods[i].Name, want, got)
			}
		}
		for i, j := len(pods)-1, 0; j < int(set.Status.UpdatedReplicas); i, j = i-1, j+1 {
			if want, got := set.Status.UpdateRevision, getPodRevision(pods[i]); want != got {
				return fmt.Errorf("pod %s want update revision %s got %s", pods[i].Name, want, got)
			}
		}
	}
	return nil
}

func checkClaimInvariants(set *appsv1beta1.StatefulSet, pod *v1.Pod, claim *v1.PersistentVolumeClaim, ordinal int) error {
	policy := appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
		WhenScaled:  appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
		WhenDeleted: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
	}
	if set.Spec.PersistentVolumeClaimRetentionPolicy != nil && utilfeature.DefaultFeatureGate.Enabled(features.StatefulSetAutoDeletePVC) {
		policy = *set.Spec.PersistentVolumeClaimRetentionPolicy
	}
	claimShouldBeRetained := policy.WhenScaled == appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType
	if claim == nil {
		if claimShouldBeRetained {
			return fmt.Errorf("claim for Pod %s was not created", pod.Name)
		}
		return nil // A non-retained claim has no invariants to satisfy.
	}

	if pod.Status.Phase != v1.PodRunning || !podutil.IsPodReady(pod) {
		// The pod has spun up yet, we do not expect the owner refs on the claim to have been set.
		return nil
	}

	const retain = appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType
	const delete = appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType
	switch {
	case policy.WhenScaled == retain && policy.WhenDeleted == retain:
		if hasOwnerRef(claim, set) {
			return fmt.Errorf("claim %s has unexpected owner ref on %s for StatefulSet retain", claim.Name, set.Name)
		}
		if hasOwnerRef(claim, pod) {
			return fmt.Errorf("claim %s has unexpected owner ref on pod %s for StatefulSet retain", claim.Name, pod.Name)
		}
	case policy.WhenScaled == retain && policy.WhenDeleted == delete:
		if !hasOwnerRef(claim, set) {
			return fmt.Errorf("claim %s does not have owner ref on %s for StatefulSet deletion", claim.Name, set.Name)
		}
		if hasOwnerRef(claim, pod) {
			return fmt.Errorf("claim %s has unexpected owner ref on pod %s for StatefulSet deletion", claim.Name, pod.Name)
		}
	case policy.WhenScaled == delete && policy.WhenDeleted == retain:
		if hasOwnerRef(claim, set) {
			return fmt.Errorf("claim %s has unexpected owner ref on %s for scaledown only", claim.Name, set.Name)
		}
		if ordinal >= int(*set.Spec.Replicas) && !hasOwnerRef(claim, pod) {
			return fmt.Errorf("claim %s does not have owner ref on condemned pod %s for scaledown delete", claim.Name, pod.Name)
		}
		if ordinal < int(*set.Spec.Replicas) && hasOwnerRef(claim, pod) {
			return fmt.Errorf("claim %s has unexpected owner ref on condemned pod %s for scaledown delete", claim.Name, pod.Name)
		}
	case policy.WhenScaled == delete && policy.WhenDeleted == delete:
		if ordinal >= int(*set.Spec.Replicas) {
			if !hasOwnerRef(claim, pod) || hasOwnerRef(claim, set) {
				return fmt.Errorf("condemned claim %s has bad owner refs: %v", claim.Name, claim.GetOwnerReferences())
			}
		} else {
			if hasOwnerRef(claim, pod) || !hasOwnerRef(claim, set) {
				return fmt.Errorf("live claim %s has bad owner refs: %v", claim.Name, claim.GetOwnerReferences())
			}
		}
	}
	return nil
}

func fakeResourceVersion(object interface{}) {
	obj, isObj := object.(metav1.Object)
	if !isObj {
		return
	}
	if version := obj.GetResourceVersion(); version == "" {
		obj.SetResourceVersion("1")
	} else if intValue, err := strconv.ParseInt(version, 10, 32); err == nil {
		obj.SetResourceVersion(strconv.FormatInt(intValue+1, 10))
	}
}

func scaleUpStatefulSetControl(set *appsv1beta1.StatefulSet,
	ssc StatefulSetControlInterface,
	om *fakeObjectManager,
	invariants invariantFunc) error {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return err
	}
	for set.Status.UpdatedAvailableReplicas < *set.Spec.Replicas {
		pods, err := om.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			return err
		}
		sort.Sort(ascendingOrdinal(pods))

		// ensure all pods are valid (have a phase)
		for ord, pod := range pods {
			if pod.Status.Phase == "" {
				if pods, err = om.setPodPending(set, ord); err != nil {
					return err
				}
				break
			}
		}

		// select one of the pods and move it forward in status
		if len(pods) > 0 {
			ord := int(rand.Int63n(int64(len(pods))))
			pod := pods[ord]
			switch pod.Status.Phase {
			case v1.PodPending:
				if pods, err = om.setPodRunning(set, ord); err != nil {
					return err
				}
			case v1.PodRunning:
				if pods, err = om.setPodReadyWithMinReadySeconds(set, ord, getMinReadySeconds(set)); err != nil {
					return err
				}
			default:
				continue
			}
		}

		// run the controller once and check invariants
		if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
			return err
		}
		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			return err
		}
		if err := invariants(set, om); err != nil {
			return err
		}
	}
	return invariants(set, om)
}

func scaleDownStatefulSetControl(set *appsv1beta1.StatefulSet, ssc StatefulSetControlInterface, om *fakeObjectManager, invariants invariantFunc) error {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return err
	}

	for set.Status.Replicas > *set.Spec.Replicas {
		pods, err := om.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			return err
		}
		sort.Sort(ascendingOrdinal(pods))
		if ordinal := len(pods) - 1; ordinal >= 0 {
			if err := ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
				return err
			}
			set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
			if err != nil {
				return err
			}
			if pods, err = om.addTerminatingPod(set, ordinal); err != nil {
				return err
			}
			if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
				return err
			}
			set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
			if err != nil {
				return err
			}
			pods, err = om.podsLister.Pods(set.Namespace).List(selector)
			if err != nil {
				return err
			}
			sort.Sort(ascendingOrdinal(pods))

			if len(pods) > 0 {
				om.podsIndexer.Delete(pods[len(pods)-1])
			}
		}
		if err := ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
			return err
		}
		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			return err
		}

		if err := invariants(set, om); err != nil {
			return err
		}
	}
	// If there are claims with ownerRefs on pods that have been deleted, delete them.
	pods, err := om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		return err
	}
	currentPods := map[string]bool{}
	for _, pod := range pods {
		currentPods[pod.Name] = true
	}
	claims, err := om.claimsLister.PersistentVolumeClaims(set.Namespace).List(selector)
	if err != nil {
		return err
	}
	for _, claim := range claims {
		claimPodName := getClaimPodName(set, claim)
		if claimPodName == "" {
			continue // Skip claims not related to a stateful set pod.
		}
		if _, found := currentPods[claimPodName]; found {
			continue // Skip claims which still have a current pod.
		}
		for _, refs := range claim.GetOwnerReferences() {
			if refs.Name == claimPodName {
				om.claimsIndexer.Delete(claim)
				break
			}
		}
	}

	return invariants(set, om)
}

func updateComplete(set *appsv1beta1.StatefulSet, pods []*v1.Pod) bool {
	sort.Sort(ascendingOrdinal(pods))
	if len(pods) != int(*set.Spec.Replicas) {
		return false
	}
	if set.Status.ReadyReplicas != *set.Spec.Replicas {
		return false
	}

	switch set.Spec.UpdateStrategy.Type {
	case apps.OnDeleteStatefulSetStrategyType:
		return true
	case apps.RollingUpdateStatefulSetStrategyType:
		if set.Spec.UpdateStrategy.RollingUpdate != nil && set.Spec.UpdateStrategy.RollingUpdate.Paused == true {
			return true
		}
		if set.Spec.UpdateStrategy.RollingUpdate == nil || *set.Spec.UpdateStrategy.RollingUpdate.Partition <= 0 {
			if set.Status.CurrentReplicas < *set.Spec.Replicas {
				return false
			}
			for i := range pods {
				if getPodRevision(pods[i]) != set.Status.CurrentRevision {
					return false
				}
			}
		} else {
			partition := int(*set.Spec.UpdateStrategy.RollingUpdate.Partition)
			if len(pods) < partition {
				return false
			}
			for i := partition; i < len(pods); i++ {
				if getPodRevision(pods[i]) != set.Status.UpdateRevision {
					return false
				}
			}
		}
	}
	return true
}

func updateStatefulSetControl(set *appsv1beta1.StatefulSet,
	ssc StatefulSetControlInterface,
	om *fakeObjectManager,
	invariants invariantFunc) error {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return err
	}
	pods, err := om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		return err
	}
	if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
		return err
	}

	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		return err
	}
	pods, err = om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		return err
	}
	for !updateComplete(set, pods) {
		pods, err = om.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			return err
		}
		sort.Sort(ascendingOrdinal(pods))
		initialized := false
		for ord, pod := range pods {
			if pod.Status.Phase == "" {
				if pods, err = om.setPodPending(set, ord); err != nil {
					return err
				}
				break
			}
		}
		if initialized {
			continue
		}

		if len(pods) > 0 {
			ord := int(rand.Int63n(int64(len(pods))))
			pod := pods[ord]
			switch pod.Status.Phase {
			case v1.PodPending:
				if pods, err = om.setPodRunning(set, ord); err != nil {
					return err
				}
			case v1.PodRunning:
				if pods, err = om.setPodReady(set, ord); err != nil {
					return err
				}
			default:
				continue
			}
		}

		if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
			return err
		}
		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			return err
		}
		if err := invariants(set, om); err != nil {
			return err
		}
		pods, err = om.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			return err
		}

	}
	return invariants(set, om)
}

func newRevisionOrDie(set *appsv1beta1.StatefulSet, revision int64) *apps.ControllerRevision {
	rev, err := newRevision(set, revision, set.Status.CollisionCount)
	if err != nil {
		panic(err)
	}
	return rev
}

func TestScaleUpWithMaxUnavailable(t *testing.T) {
	set := newStatefulSet(5)
	set.Spec.PodManagementPolicy = apps.ParallelPodManagement
	set.Spec.ScaleStrategy = &appsv1beta1.StatefulSetScaleStrategy{
		MaxUnavailable: func() *intstr.IntOrString { i := intstr.FromInt(2); return &i }(),
	}

	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	spc, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)

	selector, _ := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	var err error
	var pods []*v1.Pod
	reconcileTwice := func() {
		for i := 0; i < 2; i++ {
			set, err = spc.setsLister.StatefulSets(set.Namespace).Get(set.Name)
			if err != nil {
				t.Fatalf("Error getting updated StatefulSet: %v", err)
			}

			pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
			if err != nil {
				t.Fatalf("Failed to list pods: %v", err)
			}
			sort.Sort(ascendingOrdinal(pods))

			// ensure all pods are valid (have a phase)
			for ord, pod := range pods {
				if pod.Status.Phase == "" {
					if pods, err = spc.setPodPending(set, ord); err != nil {
						t.Fatalf("Failed to set phase for pod: %v", err)
					}
				}
			}

			if err = ssc.UpdateStatefulSet(context.TODO(), set, pods); err != nil {
				t.Fatalf("Failed to reconcile update statefulset: %v", err)
			}
		}
	}

	reconcileTwice()
	set, err = spc.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if set.Status.Replicas != 2 {
		t.Fatalf("Expect status replicas=2, got %v", set.Status.Replicas)
	}

	spc.setPodRunning(set, 1)
	spc.setPodReady(set, 1)
	reconcileTwice()
	set, err = spc.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if set.Status.Replicas != 3 {
		t.Fatalf("Expect status replicas=3, got %v", set.Status.Replicas)
	}
}

func isOrHasInternalError(err error) bool {
	var agg utilerrors.Aggregate
	ok := errors.As(err, &agg)
	return !ok && !apierrors.IsInternalError(err) || ok && len(agg.Errors()) > 0 && !apierrors.IsInternalError(agg.Errors()[0])
}

func emptyInvariants(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error {
	return nil
}

func TestStatefulSetControlWithStartOrdinal(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.StatefulSetStartOrdinal, true)()

	simpleSetFn := func(replicas, startOrdinal int, reservedIds ...int32) *appsv1beta1.StatefulSet {
		statefulSet := newStatefulSet(replicas)
		statefulSet.Spec.Ordinals = &appsv1beta1.StatefulSetOrdinals{Start: int32(startOrdinal)}
		for _, id := range reservedIds {
			statefulSet.Spec.ReserveOrdinals = append(statefulSet.Spec.ReserveOrdinals, intstr.FromInt32(id))
		}
		return statefulSet
	}

	testCases := []struct {
		fn          func(*testing.T, *appsv1beta1.StatefulSet, invariantFunc, []int)
		obj         func() *appsv1beta1.StatefulSet
		expectedIds []int
	}{
		{
			CreatesPodsWithStartOrdinal,
			func() *appsv1beta1.StatefulSet {
				return simpleSetFn(3, 2)
			},
			[]int{2, 3, 4},
		},
		{
			CreatesPodsWithStartOrdinal,
			func() *appsv1beta1.StatefulSet {
				return simpleSetFn(3, 2, 0, 4)
			},
			[]int{2, 3, 5},
		},
		{
			CreatesPodsWithStartOrdinal,
			func() *appsv1beta1.StatefulSet {
				return simpleSetFn(3, 2, 0, 2, 3, 4, 5)
			},
			[]int{6, 7, 8},
		},
		{
			CreatesPodsWithStartOrdinal,
			func() *appsv1beta1.StatefulSet {
				return simpleSetFn(4, 1)
			},
			[]int{1, 2, 3, 4},
		},
		{
			CreatesPodsWithStartOrdinal,
			func() *appsv1beta1.StatefulSet {
				return simpleSetFn(4, 1, 1, 3, 4)
			},
			[]int{2, 5, 6, 7},
		},
	}

	for _, testCase := range testCases {
		testObj := testCase.obj
		testFn := testCase.fn

		set := testObj()
		testFn(t, set, emptyInvariants, testCase.expectedIds)
	}
}

func CreatesPodsWithStartOrdinal(t *testing.T, set *appsv1beta1.StatefulSet, invariants invariantFunc, expectedIds []int) {
	client := fake.NewSimpleClientset()
	kruiseClient := kruisefake.NewSimpleClientset(set)
	om, _, ssc, stop := setupController(client, kruiseClient)
	defer close(stop)

	if err := scaleUpStatefulSetControl(set, ssc, om, invariants); err != nil {
		t.Errorf("Failed to turn up StatefulSet : %s", err)
	}
	var err error
	set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
	if err != nil {
		t.Fatalf("Error getting updated StatefulSet: %v", err)
	}
	if set.Status.Replicas != *set.Spec.Replicas {
		t.Errorf("Failed to scale statefulset to %d replicas", *set.Spec.Replicas)
	}
	if set.Status.ReadyReplicas != *set.Spec.Replicas {
		t.Errorf("Failed to set ReadyReplicas correctly, expected %d", *set.Spec.Replicas)
	}
	if set.Status.UpdatedReplicas != *set.Spec.Replicas {
		t.Errorf("Failed to set UpdatedReplicas correctly, expected %d", *set.Spec.Replicas)
	}
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Error(err)
	}
	pods, err := om.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}
	sort.Sort(ascendingOrdinal(pods))
	if len(expectedIds) != len(pods) {
		t.Errorf("Expected %d pods. Got %d", len(expectedIds), len(pods))
		return
	}
	for i, pod := range pods {
		expectedOrdinal := expectedIds[i]
		actualPodOrdinal := getOrdinal(pod)
		if actualPodOrdinal != expectedOrdinal {
			t.Errorf("Expected pod ordinal %d. Got %d", expectedOrdinal, actualPodOrdinal)
		}
	}
}

func newStatefulSetWithGivenSC(replicas int, vctNumber int, scs []*string) *appsv1beta1.StatefulSet {
	petMounts := []corev1.VolumeMount{}
	for i := 0; i < vctNumber; i++ {
		petMounts = append(petMounts, corev1.VolumeMount{
			Name: fmt.Sprintf("datadir-%d", i), MountPath: fmt.Sprintf("/tmp/vct-%d", i),
		})
	}
	podMounts := []corev1.VolumeMount{
		{Name: "home", MountPath: "/home"},
	}
	sts := newStatefulSetWithVolumes(replicas, "foo", petMounts, podMounts)
	if len(sts.Spec.VolumeClaimTemplates) != vctNumber || len(scs) == 0 {
		return sts
	}
	for i := 0; i < vctNumber; i++ {
		sts.Spec.VolumeClaimTemplates[i].Spec.StorageClassName = scs[i%len(scs)]
	}
	return sts
}

func TestStatefulSetVCTResize(t *testing.T) {
	//Q: Why this test can work without csi work?
	//A: All pvcs are not ready. => All pods are unavailable. =>  All pods/pvcs can update.
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.StatefulSetAutoResizePVCGate, true)()

	sc1 := newStorageClass("can_expand", true)
	sc2 := newStorageClass("cannot_expand", false)
	simpleSetFn := func(scs []*string) *appsv1beta1.StatefulSet {
		statefulSet := newStatefulSetWithGivenSC(5, len(scs), scs)
		statefulSet.Spec.VolumeClaimUpdateStrategy.Type = appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType
		return statefulSet
	}
	validationFn := func(set *appsv1beta1.StatefulSet, pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) error {
		for _, pvc := range pvcs {
			if *pvc.Spec.StorageClassName == sc1.Name {
				if !pvc.Spec.Resources.Requests.Storage().Equal(resource.MustParse("20")) {
					return errors.New("pvc size is not updated")
				}
			} else if *pvc.Spec.StorageClassName == sc2.Name {
				if !pvc.Spec.Resources.Requests.Storage().Equal(resource.MustParse("1")) {
					return errors.New("pvc size is not updated")
				}
			}
		}
		return nil
	}
	type testCase struct {
		name       string
		invariants func(set *appsv1beta1.StatefulSet, om *fakeObjectManager) error
		initial    func() *appsv1beta1.StatefulSet
		update     func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet
		canUpdate  bool
		validate   func(set *appsv1beta1.StatefulSet, pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) error
	}
	testFn := func(test *testCase, t *testing.T) {
		set := test.initial()
		client := fake.NewSimpleClientset(&sc1, &sc2)
		kruiseClient := kruisefake.NewSimpleClientset(set)
		om, _, ssc, stop := setupController(client, kruiseClient)
		defer close(stop)
		if err := scaleUpStatefulSetControl(set, ssc, om, test.invariants); err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set, err := om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		set = test.update(set)
		if err := updateStatefulSetControl(set, ssc, om, assertUpdateInvariants); (err == nil) != test.canUpdate {
			t.Fatalf("%s expected can update %v: %v", test.name, test.canUpdate, err)
		}
		selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		pods, err := om.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}

		pvcs, err := om.claimsLister.PersistentVolumeClaims(set.Namespace).List(selector)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}

		set, err = om.setsLister.StatefulSets(set.Namespace).Get(set.Name)
		if err != nil {
			t.Fatalf("%s: %s", test.name, err)
		}
		if test.canUpdate {
			if err := test.validate(set, pods, pvcs); err != nil {
				t.Fatalf("%s: %s", test.name, err)
			}
		}
	}

	testCases := []testCase{
		{
			name:       "expand_vct_with_sc_can_expand",
			invariants: emptyInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return simpleSetFn([]*string{&sc1.Name})
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "busybox"
				set.Spec.VolumeClaimTemplates[0].Spec.Resources = corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: *resource.NewQuantity(20, resource.BinarySI),
					},
				}
				return set
			},
			canUpdate: true,
			validate:  validationFn,
		},
		{
			name:       "expand_sts_with_vct_cannot_expand",
			invariants: emptyInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return simpleSetFn([]*string{&sc2.Name})
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "busybox"
				set.Spec.VolumeClaimTemplates[0].Spec.Resources = corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: *resource.NewQuantity(20, resource.BinarySI),
					},
				}
				return set
			},
			canUpdate: false,
			validate:  validationFn,
		},
		{
			name:       "expand_vct_with_2sc_can_expand",
			invariants: emptyInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return simpleSetFn([]*string{&sc1.Name, &sc1.Name})
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "busybox"
				set.Spec.VolumeClaimTemplates[0].Spec.Resources = corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: *resource.NewQuantity(20, resource.BinarySI),
					},
				}
				set.Spec.VolumeClaimTemplates[1].Spec.Resources = corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: *resource.NewQuantity(20, resource.BinarySI),
					},
				}
				return set
			},
			canUpdate: true,
			validate:  validationFn,
		},
		{
			name:       "expand_vct_with_sc_cannot_expand",
			invariants: emptyInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return simpleSetFn([]*string{&sc2.Name, &sc2.Name})
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "busybox"
				set.Spec.VolumeClaimTemplates[0].Spec.Resources = corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: *resource.NewQuantity(20, resource.BinarySI),
					},
				}
				return set
			},
			canUpdate: false,
			validate:  validationFn,
		},
		{
			name:       "expand_vct_with_2sc_mixed_expand",
			invariants: emptyInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return simpleSetFn([]*string{&sc1.Name, &sc2.Name})
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "busybox"
				set.Spec.VolumeClaimTemplates[0].Spec.Resources = corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: *resource.NewQuantity(20, resource.BinarySI),
					},
				}
				set.Spec.VolumeClaimTemplates[1].Spec.Resources = corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: *resource.NewQuantity(20, resource.BinarySI),
					},
				}
				return set
			},
			canUpdate: false,
			validate:  validationFn,
		},
		{
			name:       "expand_vct_with_2sc_mixed_expand2",
			invariants: emptyInvariants,
			initial: func() *appsv1beta1.StatefulSet {
				return simpleSetFn([]*string{&sc1.Name, &sc2.Name})
			},
			update: func(set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
				set.Spec.Template.Spec.Containers[0].Image = "busybox"
				set.Spec.VolumeClaimTemplates[0].Spec.Resources = corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: *resource.NewQuantity(20, resource.BinarySI),
					},
				}
				return set
			},
			canUpdate: true,
			validate:  validationFn,
		},
	}
	for _, c := range testCases {
		testFn(&c, t)
	}
}
