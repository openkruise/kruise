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

package validating

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

// GetTargetNamespaces will return **the intersection** of them when multiple options are enabled.
func GetTargetNamespaces(handlerClient client.Client, targets *appsv1alpha1.ResourceDistributionTargets) (selectedNamespaces []string, allErrs field.ErrorList) {
	threshold := 0
	selectedTimes := make(map[string]int)

	//select ns via targets.Namespaces option
	if len(targets.IncludedNamespaces) != 0 {
		threshold++
		for _, namespace := range targets.IncludedNamespaces {
			selectedTimes[namespace.Name] = 1
		}
	}

	// select ns via targets.NamespaceLabelSelector option
	if len(targets.NamespaceLabelSelector.MatchLabels) != 0 || len(targets.NamespaceLabelSelector.MatchExpressions) != 0 {
		selectors, err := metav1.LabelSelectorAsSelector(&targets.NamespaceLabelSelector)
		if err != nil {
			allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("query namespace label selector, err: %v", err)))
		} else {
			namespaces := &corev1.NamespaceList{}
			if err := handlerClient.List(context.TODO(), namespaces, &client.ListOptions{LabelSelector: selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("query namespaces via label selector failed, err: %v", err)))
			}

			threshold++
			for _, namespace := range namespaces.Items {
				selectedTimes[namespace.Name]++
			}
		}
	}

	// select all namespaces
	// when threshold == 0, enable this default targets option option
	if len(targets.ExcludedNamespaces) != 0 || threshold == 0 {
		namespaces := &corev1.NamespaceList{}
		if err := handlerClient.List(context.TODO(), namespaces, &client.ListOptions{}); err != nil {
			allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("query all namespaces failed, err: %v", err)))
		}

		// counting existing namespaces
		existedNamespaces := make(map[string]struct{})
		for _, namespace := range namespaces.Items {
			selectedTimes[namespace.Name]++
			existedNamespaces[namespace.Name] = struct{}{}
		}
		// counting uncreated namespaces
		for namespace := range selectedTimes {
			if _, ok := existedNamespaces[namespace]; !ok {
				selectedTimes[namespace]++
			}
		}

		threshold++
		for _, cond := range targets.ExcludedNamespaces {
			selectedTimes[cond.Name]--
		}
	}

	// get the intersection, i.e., filter the namespaces selected by all enabled options
	for namespace, times := range selectedTimes {
		if times == threshold && isNotInForbidden(namespace) {
			selectedNamespaces = append(selectedNamespaces, namespace)
		}
	}

	return
}

func isNotInForbidden(namespace string) bool {
	forbiddenNamespaces := []string{
		"kube-system",
		"kube-public",
	}
	for _, forbiddenNamespace := range forbiddenNamespaces {
		if namespace == forbiddenNamespace {
			return false
		}
	}
	return true
}
