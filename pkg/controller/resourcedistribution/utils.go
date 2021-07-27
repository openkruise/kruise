package resourcedistribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utils "github.com/openkruise/kruise/pkg/webhook/resourcedistribution/validating"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	GetConditionID                     = 0
	CreateConditionID                  = 1
	UpdateConditionID                  = 2
	DeleteConditionID                  = 3
	ConflictConditionID                = 4
	NumberOfConditionTypes             = 5
	ReconcileSuccessfully              = "ReconcileSuccessfully"
	ResourceDistributionListAnnotation = "kruise.io/resourcedistribution-injected-list"
)

// UnexpectedError is designed to capture the information about .status.conditions when error occurs
type UnexpectedError struct {
	err           error
	namespace     string
	conditionID   int
	conditionType appsv1alpha1.ResourceDistributionConditionType
}

// namespaceMatchedResourceDistribution check whether Namespace matches ResourceDistribution via labels
func namespaceMatchedResourceDistribution(namespace *corev1.Namespace, distributor *appsv1alpha1.ResourceDistribution) (bool, error) {
	selector, err := metav1.LabelSelectorAsSelector(&distributor.Spec.Targets.NamespaceLabelSelector)
	if err != nil {
		return false, err
	}
	if !selector.Empty() && selector.Matches(labels.Set(namespace.Labels)) {
		return true, nil
	}

	return false, nil
}

// hashResource hash resource yaml using SHA256
func hashResource(resourceYaml runtime.RawExtension) string {
	md5Hash := sha256.Sum256(resourceYaml.Raw)
	return hex.EncodeToString(md5Hash[:])
}

// prepareNamespaces return:
// (1) isInTargets: a map that contains all target namespace names
// (2) allNamespaces: all namespace names, including both those fetched from cluster and newly-created namespaces
// NOTE: if a target namespace doesn't exist in cluster, this function will create it
func prepareNamespaces(handlerClient client.Client, distributor *appsv1alpha1.ResourceDistribution) (inTargets map[string]struct{}, allNamespaces []string, err error) {
	// 1. parse all targets, fetch existed namespaces
	inTargets, existed, err := parseNamespaces(handlerClient, &distributor.Spec.Targets)
	if err != nil {
		klog.Errorf("ResourceDistribution get target namespaces error, err: %v, name: %s", err, distributor.Name)
		return nil, nil, err
	}
	klog.V(3).Infof("ResourceDistribution %s target namespaces %v\n\n", distributor.Name, inTargets)

	// 2. add existed namespaces
	for namespace := range existed {
		allNamespaces = append(allNamespaces, namespace)
	}

	// 3. prepare un-created namespaces and add them in allNamespaces
	for namespace := range inTargets {
		if _, ok := existed[namespace]; ok {
			continue
		}
		if err := createNamespace(handlerClient, namespace); err != nil {
			return nil, nil, err
		}
		allNamespaces = append(allNamespaces, namespace)
	}
	return
}

// setCondition set condition[].Type, .Reason, and .FailedNamespaces
func setCondition(condition *appsv1alpha1.ResourceDistributionCondition, conditionType appsv1alpha1.ResourceDistributionConditionType, namespace string, err error) {
	condition.Type = conditionType
	if err != nil {
		condition.Reason = err.Error()
		condition.FailedNamespaces = append(condition.FailedNamespaces, namespace)
	}
}

// makeNewStatus will return a complete new status to update distributor
func makeNewStatus(status *appsv1alpha1.ResourceDistributionStatus, distributor *appsv1alpha1.ResourceDistribution,
	newConditions []appsv1alpha1.ResourceDistributionCondition, succeeded int32) *appsv1alpha1.ResourceDistributionStatus {
	// set .Succeeded, .Failed, .ObservedGeneration
	status.Succeeded = succeeded
	status.Failed = status.Desired - succeeded
	status.ObservedGeneration = distributor.Generation

	// set .Condition
	oldConditions := distributor.Status.Conditions
	for i := 0; i < NumberOfConditionTypes; i++ {
		if len(newConditions[i].FailedNamespaces) == 0 {
			// if no error occurred
			newConditions[i].Reason = ReconcileSuccessfully
			newConditions[i].Status = appsv1alpha1.ResourceDistributionConditionTrue
		} else {
			newConditions[i].Status = appsv1alpha1.ResourceDistributionConditionFalse
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

// makeResourceObject set some necessary information for resource before updating and creating
func makeResourceObject(distributor *appsv1alpha1.ResourceDistribution, namespace string, resource runtime.Object, hashCode string) runtime.Object {
	// convert to unstructured
	resourceOperation := utils.ConvertToUnstructured(resource)
	// 1. set namespace
	resourceOperation.SetNamespace(namespace)
	// 2. set ownerReference for cascading deletion
	resourceOperation.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: distributor.APIVersion,
			Kind:       distributor.Kind,
			Name:       distributor.Name,
			UID:        distributor.UID,
		},
	})
	// 3. set resource annotations
	annotations := resourceOperation.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[utils.ResourceHashCodeAnnotation] = hashCode
	annotations[utils.ResourceDistributedTimestamp] = time.Now().String()
	annotations[utils.SourceResourceDistributionOfResource] = distributor.Name
	resourceOperation.SetAnnotations(annotations)

	return resource
}

// createNamespace try to create namespace
func createNamespace(handlerClient client.Client, name string) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		createErr := handlerClient.Create(context.TODO(), namespace, &client.CreateOptions{})
		if createErr == nil {
			return nil
		}
		if err := handlerClient.Get(context.TODO(), types.NamespacedName{Namespace: name, Name: name}, namespace); err != nil {
			klog.Errorf("error create namespace %s from client", name)
		}
		return createErr
	}); err != nil {
		return err
	}

	return nil
}

// parseNamespaces will return two sets: one contains target namespaces, another contains all existing namespaces
// including existing namespace, and the uncreated but list in target.IncludeNamespaces
// YOU SHOULD KNOW THE RULES BEFORE USING THIS FUNCTION:
// Priority: ExcludedNamespaces > AllNamespaces = IncludedNamespaces = NamespaceLabelSelector.
// ResourceDistributionTargets will first parse AllNamespaces, IncludedNamespaces, and NamespaceLabelSelector, then calculate their union,
// At last ExcludedNamespaces will act on the union to remove and exclude the designated namespaces from it.
// For example, if a namespace is in both ExcludedNamespaces and IncludedNamespaces, this namespace will be still excluded.
func parseNamespaces(handlerClient client.Client, targets *appsv1alpha1.ResourceDistributionTargets) (targetNamespaces, allNamespaces map[string]struct{}, err error) {
	allNamespaces = make(map[string]struct{})
	targetNamespaces = make(map[string]struct{})

	existingNamespaces := &corev1.NamespaceList{}
	if err = handlerClient.List(context.TODO(), existingNamespaces, &client.ListOptions{}); err != nil {
		return nil, nil, err
	}
	for _, namespace := range existingNamespaces.Items {
		allNamespaces[namespace.Name] = struct{}{}
	}

	// 1. select the namespaces via targets.NamespaceLabelSelector
	if !targets.AllNamespaces && (len(targets.NamespaceLabelSelector.MatchLabels) != 0 || len(targets.NamespaceLabelSelector.MatchExpressions) != 0) {
		selectors, err := util.GetFastLabelSelector(&targets.NamespaceLabelSelector)
		if err != nil {
			return nil, nil, err
		}
		namespaces := &corev1.NamespaceList{}
		if err := handlerClient.List(context.TODO(), namespaces, &client.ListOptions{LabelSelector: selectors}); err != nil {
			return nil, nil, err
		}
		for _, namespace := range namespaces.Items {
			targetNamespaces[namespace.Name] = struct{}{}
		}
	}

	// 2. select all namespaces via targets.AllNamespace
	if targets.AllNamespaces {
		for namespace := range allNamespaces {
			targetNamespaces[namespace] = struct{}{}
		}
	}

	// 3. select the namespaces via targets.IncludedNamespaces
	if len(targets.IncludedNamespaces.List) != 0 {
		for _, namespace := range targets.IncludedNamespaces.List {
			targetNamespaces[namespace.Name] = struct{}{}
		}
	}

	// 4. exclude the namespaces via target.ExcludedNamespaces
	if len(targets.ExcludedNamespaces.List) != 0 {
		for _, namespace := range targets.ExcludedNamespaces.List {
			delete(targetNamespaces, namespace.Name)
		}
	}

	// 5. exclude forbidden namespaces
	for _, forbiddenNamespace := range utils.ForbiddenNamespaces {
		delete(allNamespaces, forbiddenNamespace)
		delete(targetNamespaces, forbiddenNamespace)
	}

	return targetNamespaces, allNamespaces, nil
}
