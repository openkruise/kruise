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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"sync"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utils "github.com/openkruise/kruise/pkg/webhook/resourcedistribution/validating"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/integer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	GetConditionID         = 0
	CreateConditionID      = 1
	UpdateConditionID      = 2
	DeleteConditionID      = 3
	ConflictConditionID    = 4
	NotExistConditionID    = 5
	NumberOfConditionTypes = 6
	OperationSucceeded     = "Succeeded"
)

// UnexpectedError is designed to store the information about .status.conditions when error occurs
type UnexpectedError struct {
	err         error
	namespace   string
	conditionID int
}

// isInList return true if namespaceName is in namespaceList, else return false
func isInList(namespaceName string, namespaceList []appsv1alpha1.ResourceDistributionNamespace) bool {
	for _, namespace := range namespaceList {
		if namespaceName == namespace.Name {
			return true
		}
	}
	return false
}

// matchViaIncludedNamespaces return true if namespace is in targets.IncludedNamespaces
func matchViaIncludedNamespaces(namespace *corev1.Namespace, distributor *appsv1alpha1.ResourceDistribution) (bool, error) {
	if isInList(namespace.Name, distributor.Spec.Targets.IncludedNamespaces.List) {
		return true, nil
	}
	return false, nil
}

// matchViaLabelSelector return true if namespace matches with target.NamespacesLabelSelectors
func matchViaLabelSelector(namespace *corev1.Namespace, distributor *appsv1alpha1.ResourceDistribution) (bool, error) {
	selector, err := util.ValidatedLabelSelectorAsSelector(&distributor.Spec.Targets.NamespaceLabelSelector)
	if err != nil {
		return false, err
	}
	if !selector.Empty() && selector.Matches(labels.Set(namespace.Labels)) {
		return true, nil
	}
	return false, nil
}

// matchViaTargets check whether Namespace matches ResourceDistribution via spec.targets
func matchViaTargets(namespace *corev1.Namespace, distributor *appsv1alpha1.ResourceDistribution) (bool, error) {
	targets := &distributor.Spec.Targets
	if isInList(namespace.Name, targets.ExcludedNamespaces.List) {
		return false, nil
	}
	if targets.AllNamespaces {
		return true, nil
	}
	if isInList(namespace.Name, targets.IncludedNamespaces.List) {
		return true, nil
	}
	return matchViaLabelSelector(namespace, distributor)
}

// addMatchedResourceDistributionToWorkQueue adds rds into q
func addMatchedResourceDistributionToWorkQueue(q workqueue.RateLimitingInterface, rds []*appsv1alpha1.ResourceDistribution) {
	for _, rd := range rds {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: rd.Name,
			},
		})
	}
}

// hashResource hash resource yaml as version using SHA256
func hashResource(resourceYaml runtime.RawExtension) string {
	md5Hash := sha256.Sum256(resourceYaml.Raw)
	return hex.EncodeToString(md5Hash[:])
}

// setCondition set condition[].Type, .Reason, and .FailedNamespaces
func setCondition(condition *appsv1alpha1.ResourceDistributionCondition, err error, namespaces ...string) {
	if condition == nil || err == nil {
		return
	}
	condition.Reason = err.Error()
	condition.FailedNamespaces = append(condition.FailedNamespaces, namespaces...)
}

// initConditionType set conditionTypes for .status.conditions
func initConditionType(conditions []appsv1alpha1.ResourceDistributionCondition) {
	if len(conditions) < NumberOfConditionTypes {
		return
	}
	conditions[GetConditionID].Type = appsv1alpha1.ResourceDistributionGetResourceFailed
	conditions[CreateConditionID].Type = appsv1alpha1.ResourceDistributionCreateResourceFailed
	conditions[UpdateConditionID].Type = appsv1alpha1.ResourceDistributionUpdateResourceFailed
	conditions[DeleteConditionID].Type = appsv1alpha1.ResourceDistributionDeleteResourceFailed
	conditions[ConflictConditionID].Type = appsv1alpha1.ResourceDistributionConflictOccurred
	conditions[NotExistConditionID].Type = appsv1alpha1.ResourceDistributionNamespaceNotExists
}

// calculateNewStatus returns a complete new status to update distributor.status
func calculateNewStatus(distributor *appsv1alpha1.ResourceDistribution, newConditions []appsv1alpha1.ResourceDistributionCondition, desired, succeeded int32) *appsv1alpha1.ResourceDistributionStatus {
	status := &appsv1alpha1.ResourceDistributionStatus{}
	if distributor == nil || len(newConditions) < NumberOfConditionTypes {
		return status
	}

	// set .Succeeded, .Failed, .ObservedGeneration
	status.Desired = desired
	status.Succeeded = succeeded
	status.Failed = desired - succeeded
	status.ObservedGeneration = distributor.Generation

	// set .Conditions
	oldConditions := distributor.Status.Conditions
	for i := 0; i < NumberOfConditionTypes; i++ {
		if len(newConditions[i].FailedNamespaces) == 0 {
			// if no error occurred
			newConditions[i].Reason = OperationSucceeded
			newConditions[i].Status = appsv1alpha1.ResourceDistributionConditionFalse
		} else {
			newConditions[i].Status = appsv1alpha1.ResourceDistributionConditionTrue
		}
		if len(oldConditions) == 0 || oldConditions[i].Status != newConditions[i].Status {
			// if .conditions.status changed
			newConditions[i].LastTransitionTime = metav1.Time{Time: time.Now()}
		} else {
			newConditions[i].LastTransitionTime = oldConditions[i].LastTransitionTime
		}
	}
	status.Conditions = newConditions
	return status
}

// mergeMetadata will merge labels/annotations/finalizers
func mergeMetadata(newResource, oldResource *unstructured.Unstructured) {
	if newResource.GetLabels() == nil {
		newResource.SetLabels(make(map[string]string))
	}

	if newResource.GetAnnotations() == nil {
		newResource.SetAnnotations(make(map[string]string))
	}

	for k, v := range oldResource.GetLabels() {
		newLabels := newResource.GetLabels()
		if _, ok := newLabels[k]; !ok {
			newLabels[k] = v
		}
		newResource.SetLabels(newLabels)
	}

	for k, v := range oldResource.GetAnnotations() {
		newAnnotations := newResource.GetAnnotations()
		if _, ok := newAnnotations[k]; !ok {
			newAnnotations[k] = v
		}
		newResource.SetAnnotations(newAnnotations)
	}

	newResource.SetFinalizers(sets.NewString(newResource.GetFinalizers()...).
		Union(sets.NewString(oldResource.GetFinalizers()...)).List())
}

// makeResourceObject set some necessary information for resource before updating and creating
func makeResourceObject(distributor *appsv1alpha1.ResourceDistribution, namespace string, resource runtime.Object, hashCode string, oldResource *unstructured.Unstructured) runtime.Object {
	// convert to unstructured
	newResource := utils.ConvertToUnstructured(resource.DeepCopyObject())
	if oldResource != nil {
		mergeMetadata(newResource, oldResource)
	}

	// 1. set namespace
	newResource.SetNamespace(namespace)

	// 2. set ownerReference for cascading deletion
	found := false
	owners := newResource.GetOwnerReferences()
	for i := range owners {
		if owners[i].UID == distributor.UID {
			found = true
			break
		}
	}
	if !found {
		newResource.SetOwnerReferences(append(owners, *metav1.NewControllerRef(distributor, distributor.GroupVersionKind())))
	}

	// 3. set resource annotations
	annotations := newResource.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[utils.ResourceHashCodeAnnotation] = hashCode
	annotations[utils.SourceResourceDistributionOfResource] = distributor.Name
	newResource.SetAnnotations(annotations)

	return newResource
}

func syncItSlowly(namespaces []string, initialBatchSize int, fn func(namespace string) *UnexpectedError) (int32, []*UnexpectedError) {
	successes := int32(0)
	remaining := len(namespaces)
	errList := make([]*UnexpectedError, 0)
	for batchSize := integer.IntMin(remaining, initialBatchSize); batchSize > 0; batchSize = integer.IntMin(2*batchSize, remaining) {
		errCh := make(chan *UnexpectedError, batchSize)
		var wg sync.WaitGroup
		wg.Add(batchSize)
		for i := 0; i < batchSize; i++ {
			namespace := namespaces[int(successes)+len(errList)+i]
			go func() {
				defer wg.Done()
				if err := fn(namespace); err != nil {
					errCh <- err
				}
			}()
		}
		wg.Wait()
		errCount := len(errCh)
		curSuccesses := batchSize - errCount
		successes += int32(curSuccesses)
		for i := 0; i < errCount; i++ {
			errList = append(errList, <-errCh)
		}
		remaining -= batchSize
	}
	return successes, errList
}

// listNamespacesForDistributor returns two slices: one contains all matched namespaces, another contains all unmatched.
// Firstly, Spec.Targets will parse .AllNamespaces, .IncludedNamespaces, and .NamespaceLabelSelector; Then calculate their
// union; At last ExcludedNamespaces will act on the union to remove the designated namespaces from it.
func listNamespacesForDistributor(handlerClient client.Client, targets *appsv1alpha1.ResourceDistributionTargets) ([]string, []string, error) {
	matchedSet := sets.NewString()
	unmatchedSet := sets.NewString()

	namespacesList := &corev1.NamespaceList{}
	if err := handlerClient.List(context.TODO(), namespacesList); err != nil {
		return nil, nil, err
	}

	for _, namespace := range namespacesList.Items {
		unmatchedSet.Insert(namespace.Name)
	}

	if targets.AllNamespaces {
		// 1. select all namespaces via targets.AllNamespace
		for _, namespace := range namespacesList.Items {
			matchedSet.Insert(namespace.Name)
		}
	} else {
		// 2. select the namespaces via targets.IncludedNamespaces
		for _, namespace := range targets.IncludedNamespaces.List {
			matchedSet.Insert(namespace.Name)
		}
	}

	if !targets.AllNamespaces && (len(targets.NamespaceLabelSelector.MatchLabels) != 0 || len(targets.NamespaceLabelSelector.MatchExpressions) != 0) {
		// 3. select the namespaces via targets.NamespaceLabelSelector
		selectors, err := util.ValidatedLabelSelectorAsSelector(&targets.NamespaceLabelSelector)
		if err != nil {
			return nil, nil, err
		}
		namespaces := &corev1.NamespaceList{}
		if err := handlerClient.List(context.TODO(), namespaces, &client.ListOptions{LabelSelector: selectors}); err != nil {
			return nil, nil, err
		}
		for _, namespace := range namespaces.Items {
			matchedSet.Insert(namespace.Name)
		}
	}

	// 4. exclude the namespaces via target.ExcludedNamespaces
	for _, namespace := range targets.ExcludedNamespaces.List {
		matchedSet.Delete(namespace.Name)
	}

	// 5. remove matched namespaces from unmatched namespace set
	unmatchedSet = unmatchedSet.Difference(matchedSet)

	return matchedSet.List(), unmatchedSet.List(), nil
}

func needToUpdate(old, new *unstructured.Unstructured) bool {
	oldObject := old.DeepCopy().Object
	newObject := new.DeepCopy().Object
	oldObject["metadata"] = nil
	newObject["metadata"] = nil
	oldObject["status"] = nil
	newObject["status"] = nil
	return !reflect.DeepEqual(oldObject, newObject)
}

func isControlledByDistributor(resource metav1.Object, distributor *appsv1alpha1.ResourceDistribution) bool {
	controller := metav1.GetControllerOf(resource)
	if controller != nil && distributor != nil &&
		distributor.APIVersion == controller.APIVersion &&
		distributor.Kind == controller.Kind &&
		distributor.Name == controller.Name {
		return true
	}
	return false
}
