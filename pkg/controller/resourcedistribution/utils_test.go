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

package resourcedistribution

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestMatchFunctions(t *testing.T) {
	distributor := buildResourceDistributionWithSecret()
	matchedNamespace, unmatchedNamespace := &corev1.Namespace{}, &corev1.Namespace{}
	matchedNamespace.SetName("ns-1")
	matchedNamespace.SetLabels(map[string]string{"group": "one"})
	unmatchedNamespace.SetName("ns-4")
	unmatchedNamespace.SetLabels(map[string]string{"group": "two"})
	// case 1
	if ok, _ := matchViaIncludedNamespaces(matchedNamespace, distributor); !ok {
		t.Fatalf("failed to matchViaIncludedNamespaces, expected: matched, autual: unmatched")
	}
	// case 2
	if ok, _ := matchViaIncludedNamespaces(unmatchedNamespace, distributor); ok {
		t.Fatalf("failed to matchViaIncludedNamespaces, expected: unmatched, autual: matched")
	}
	//case 3
	if ok, err := matchViaLabelSelector(matchedNamespace, distributor); !ok || err != nil {
		t.Fatalf("failed to matchViaIncludedNamespaces, expected: matched, autual: unmatched")
	}
	//case 4
	if ok, err := matchViaLabelSelector(unmatchedNamespace, distributor); ok || err != nil {
		t.Fatalf("failed to matchViaIncludedNamespaces, expected: unmatched, autual: matched, err %v", err)

	}
	//case 5
	if ok, err := matchViaTargets(matchedNamespace, distributor); !ok || err != nil {
		t.Fatalf("failed to matchViaTargets, expected: matched, autual: unmatched")
	}
	//case 6
	if ok, err := matchViaTargets(unmatchedNamespace, distributor); ok || err != nil {
		t.Fatalf("failed to matchViaTargets, expected: unmatched, autual: matched, err %v", err)

	}
}

func TestGetNamespaceForDistributor(t *testing.T) {
	distributor := buildResourceDistributionWithSecret()
	makeClientEnvironment(distributor)

	matched, unmatched, err := listNamespacesForDistributor(reconcileHandler.Client, &distributor.Spec.Targets)
	if err != nil {
		t.Fatalf("failed to test getNamespaceForDistributor function, err %v", err)
	}
	if len(matched) != 4 {
		t.Fatalf("the number of expected matched namespace is %d, but got %d", 3, len(matched))
	}
	if len(unmatched) != 1 {
		t.Fatalf("the number of expected unmatched namespace is %d, but got %d", 1, len(unmatched))
	}
}

func TestIsControlledByDistributorAcceptsAlphaOwnerRef(t *testing.T) {
	distributor := buildResourceDistributionWithSecret()
	distributor.SetUID("rd-uid")

	resource := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret-1",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: appsv1alpha1.GroupVersion.String(),
				Kind:       "ResourceDistribution",
				Name:       distributor.Name,
				UID:        distributor.UID,
				Controller: ptrTo(true),
			}},
		},
	}

	if !isControlledByDistributor(resource, distributor) {
		t.Fatalf("expected beta distributor to recognize alpha owner reference")
	}
}

func TestMakeResourceObjectRewritesOwnerRefToBeta(t *testing.T) {
	distributor := buildResourceDistributionWithSecret()
	distributor.SetUID("rd-uid")
	resource := &unstructured.Unstructured{}
	resource.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	resource.SetName("test-secret-1")

	oldResource := &unstructured.Unstructured{}
	oldResource.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	oldResource.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: appsv1alpha1.GroupVersion.String(),
		Kind:       "ResourceDistribution",
		Name:       distributor.Name,
		UID:        distributor.UID,
		Controller: ptrTo(true),
	}})

	newResource := makeResourceObject(distributor, "ns-1", resource, "hash", oldResource).(*unstructured.Unstructured)
	controllerRef := metav1.GetControllerOf(newResource)
	if controllerRef == nil {
		t.Fatalf("expected controller ref to be set")
	}
	if controllerRef.APIVersion != appsv1beta1.GroupVersion.String() {
		t.Fatalf("expected beta ownerRef apiVersion, got %s", controllerRef.APIVersion)
	}
}

func ptrTo[T any](value T) *T {
	return &value
}

func TestCalculateNewStatus_PartialAndReorderedConditions(t *testing.T) {
	pastTime := metav1.Time{Time: time.Now().Add(-10 * time.Minute)}

	// Test 1: Partially populated conditions (e.g. only 1 condition) should not panic and preserve transition time.
	distributor := &appsv1beta1.ResourceDistribution{
		Status: appsv1beta1.ResourceDistributionStatus{
			Conditions: []appsv1beta1.ResourceDistributionCondition{
				{
					Type:               appsv1beta1.ResourceDistributionConflictOccurred,
					Status:             appsv1beta1.ResourceDistributionConditionTrue,
					LastTransitionTime: pastTime,
				},
			},
		},
	}
	newConditions := make([]appsv1beta1.ResourceDistributionCondition, NumberOfConditionTypes)
	initConditionType(newConditions)
	newConditions[ConflictConditionID].FailedNamespaces = []string{"ns-conflict"}

	newStatus := calculateNewStatus(distributor, newConditions, 2, 1)
	if newStatus.Desired != 2 || newStatus.Succeeded != 1 || newStatus.Failed != 1 {
		t.Fatalf("unexpected status counts: desired=%d, succeeded=%d, failed=%d", newStatus.Desired, newStatus.Succeeded, newStatus.Failed)
	}

	conflictCond := getDistributionCondition(newStatus.Conditions, appsv1beta1.ResourceDistributionConflictOccurred)
	if conflictCond == nil || conflictCond.Status != appsv1beta1.ResourceDistributionConditionTrue {
		t.Fatalf("expected ConflictOccurred to be True, got %v", conflictCond)
	}
	if !conflictCond.LastTransitionTime.Equal(&pastTime) {
		t.Fatalf("expected ConflictOccurred LastTransitionTime to be preserved, got %v vs %v", conflictCond.LastTransitionTime, pastTime)
	}

	// Test 2: Reordered conditions in Status.Conditions should match by Type, preserving LastTransitionTime.
	reorderedConditions := []appsv1beta1.ResourceDistributionCondition{
		{
			Type:               appsv1beta1.ResourceDistributionNamespaceNotExists,
			Status:             appsv1beta1.ResourceDistributionConditionFalse,
			LastTransitionTime: pastTime,
		},
		{
			Type:               appsv1beta1.ResourceDistributionGetResourceFailed,
			Status:             appsv1beta1.ResourceDistributionConditionFalse,
			LastTransitionTime: pastTime,
		},
	}
	distributor.Status.Conditions = reorderedConditions
	newConditions2 := make([]appsv1beta1.ResourceDistributionCondition, NumberOfConditionTypes)
	initConditionType(newConditions2)

	newStatus2 := calculateNewStatus(distributor, newConditions2, 1, 1)
	getCond := getDistributionCondition(newStatus2.Conditions, appsv1beta1.ResourceDistributionGetResourceFailed)
	if getCond == nil || getCond.Status != appsv1beta1.ResourceDistributionConditionFalse {
		t.Fatalf("expected GetResourceFailed to be False, got %v", getCond)
	}
	if !getCond.LastTransitionTime.Equal(&pastTime) {
		t.Fatalf("expected GetResourceFailed LastTransitionTime to be preserved from reordered old conditions, got %v vs %v", getCond.LastTransitionTime, pastTime)
	}

	// Test 3: Status transition updates LastTransitionTime
	newConditions3 := make([]appsv1beta1.ResourceDistributionCondition, NumberOfConditionTypes)
	initConditionType(newConditions3)
	// GetResourceFailed now fails
	newConditions3[GetConditionID].FailedNamespaces = []string{"ns-err"}

	newStatus3 := calculateNewStatus(distributor, newConditions3, 1, 0)
	getCondUpdated := getDistributionCondition(newStatus3.Conditions, appsv1beta1.ResourceDistributionGetResourceFailed)
	if getCondUpdated == nil || getCondUpdated.Status != appsv1beta1.ResourceDistributionConditionTrue {
		t.Fatalf("expected GetResourceFailed to be True, got %v", getCondUpdated)
	}
	if getCondUpdated.LastTransitionTime.Equal(&pastTime) {
		t.Fatalf("expected GetResourceFailed LastTransitionTime to be updated on status change")
	}
}
