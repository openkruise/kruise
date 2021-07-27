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

	v1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/kubernetes/pkg/api/legacyscheme"
	_ "k8s.io/kubernetes/pkg/apis/apps/install"
	_ "k8s.io/kubernetes/pkg/apis/core/install"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

// GetTargetNamespaces will return the intersection of them when multiple options are enabled.
func GetTargetNamespaces(handlerClient client.Client, targets *appsv1alpha1.ResourceDistributionTargets, fldPath *field.Path) (selectedNamespaces []string, allErrs field.ErrorList) {
	threshold := 0
	selectedTimes := make(map[string]int)

	//select ns via targets.Namespaces
	if len(targets.Namespaces) != 0 {
		threshold++
		for _, namespace := range targets.Namespaces {
			selectedTimes[namespace.Name] = 1
		}
	}

	// select ns via targets.NamespaceLabelSelector
	if len(targets.NamespaceLabelSelector.MatchLabels) != 0 || len(targets.NamespaceLabelSelector.MatchExpressions) != 0 {
		selectors, err := metav1.LabelSelectorAsSelector(&targets.NamespaceLabelSelector)
		if err != nil {
			allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("query namespace label selector, err: %v", err)))
		} else {
			namespaces := &corev1.NamespaceList{}
			if err := handlerClient.List(context.TODO(), namespaces, &client.ListOptions{LabelSelector: selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("query all namespaces failed, err: %v", err)))
			}

			threshold++
			for _, namespace := range namespaces.Items {
				selectedTimes[namespace.Name] += 1
			}
		}
	}

	// select ns via targets.WorkloadLabelSelector
	if len(targets.WorkloadLabelSelector.MatchLabels) != 0 || len(targets.WorkloadLabelSelector.MatchExpressions) != 0 {
		selectors, err := metav1.LabelSelectorAsSelector(&targets.WorkloadLabelSelector.LabelSelector)
		if err != nil {
			allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("query workload label selector, err: %v", err)))
		} else {
			namespaces, errs := SelectNamespaceByWorkloadLabels(handlerClient, &selectors, targets.WorkloadLabelSelector.APIVersion, targets.WorkloadLabelSelector.Kind, fldPath.Child("workloadType"))
			allErrs = append(allErrs, errs...)

			threshold++
			for _, namespace := range namespaces {
				selectedTimes[namespace] += 1
			}
		}
	}

	// select all namespaces
	// when threshold == 0, enable this default targets option
	if len(targets.All) != 0 || threshold == 0 {
		namespaces := &corev1.NamespaceList{}
		if err := handlerClient.List(context.TODO(), namespaces, &client.ListOptions{}); err != nil {
			allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("query all namespaces failed, err: %v", err)))
		}

		threshold++
		for _, namespace := range namespaces.Items {
			selectedTimes[namespace.Name] += 1
		}

		for _, cond := range targets.All {
			selectedTimes[cond.Exception] -= 1
		}
	}

	// get the intersection, i.e., filter the namespaces selected by all enabled options
	for namespace, times := range selectedTimes {
		if times == threshold {
			selectedNamespaces = append(selectedNamespaces, namespace)
		}
	}

	return
}

// SelectNamespaceByWorkloadLabels only supports:
// 1. k8s build-in workload types: [Pod, Deployment, ReplicaSet, StatefulSet, DaemonSet, Job, CornJob]
// 2. kruise workload types: [CloneSet, Advanced StatefulSet, UnitedDeployment, BroadcastJob, AdvancedCronJob, ImagePullJob]
func SelectNamespaceByWorkloadLabels(handlerClient client.Client, selectors *labels.Selector, apiVersion, kind string, fldPath *field.Path) (namespaces []string, allErrs field.ErrorList) {
	// recorder the namespaces
	recorder := make(map[string]struct{})

	switch kind {
	case "Pod":
		switch apiVersion {
		case "v1":
			workloads := &corev1.PodList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		default:
			allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload apiVersion in workloadLabelSelector"))
		}

	case "Deployment":
		switch apiVersion {
		case "apps/v1":
			workloads := &v1.DeploymentList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		default:
			allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload apiVersion in workloadLabelSelector"))
		}

	case "ReplicaSet":
		switch apiVersion {
		case "apps/v1":
			workloads := &v1.ReplicaSetList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		default:
			allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload apiVersion in workloadLabelSelector"))
		}

	case "StatefulSet":
		switch apiVersion {
		case "apps.kruise.io/v1alpha1":
			workloads := &appsv1alpha1.StatefulSetList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		case "apps/v1":
			workloads := &v1.StatefulSetList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		default:
			allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload apiVersion in workloadLabelSelector"))
		}
	case "DaemonSet":
		switch apiVersion {
		case "apps.kruise.io/v1alpha1":
			workloads := &appsv1alpha1.DaemonSetList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		case "apps/v1":
			workloads := &v1.DaemonSetList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		default:
			allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload apiVersion in workloadLabelSelector"))
		}
	case "CornJob":
		switch apiVersion {
		case "batch/v1beta1":
			workloads := &v1beta1.CronJobList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		default:
			allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload apiVersion in workloadLabelSelector"))
		}
	case "Job":
		switch apiVersion {
		case "batch/v1":
			workloads := &batchv1.JobList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		default:
			allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload apiVersion in workloadLabelSelector"))
		}
	case "AdvancedCronJob":
		switch apiVersion {
		case "apps.kruise.io/v1alpha1":
			workloads := &appsv1alpha1.AdvancedCronJobList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		default:
			allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload apiVersion in workloadLabelSelector"))
		}
	case "CloneSet":
		switch apiVersion {
		case "apps.kruise.io/v1alpha1":
			workloads := &appsv1alpha1.CloneSetList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		default:
			allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload apiVersion in workloadLabelSelector"))
		}
	case "UnitedDeployment":
		switch apiVersion {
		case "apps.kruise.io/v1alpha1":
			workloads := &appsv1alpha1.UnitedDeploymentList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		default:
			allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload apiVersion in workloadLabelSelector"))
		}
	case "BroadcastJob":
		switch apiVersion {
		case "apps.kruise.io/v1alpha1":
			workloads := &appsv1alpha1.BroadcastJobList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		default:
			allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload apiVersion in workloadLabelSelector"))
		}
	case "NodeImage":
		switch apiVersion {
		case "apps.kruise.io/v1alpha1":
			workloads := &appsv1alpha1.ImagePullJobList{}
			if err := handlerClient.List(context.TODO(), workloads, &client.ListOptions{LabelSelector: *selectors}); err != nil {
				allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to list workload, err: %v", err)))
			}
			for _, workload := range workloads.Items {
				recorder[workload.Namespace] = struct{}{}
			}
		default:
			allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload apiVersion in workloadLabelSelector"))
		}
	default:
		allErrs = append(allErrs, field.Invalid(fldPath, selectors, "unknown or unsupported workload kind in workloadLabelSelector"))
	}

	// process namespaces
	for namespace := range recorder {
		namespaces = append(namespaces, namespace)
	}

	return
}

func DecodeYamlAsSecretOrConfigMap(resourceYAML *runtime.RawExtension, fldPath *field.Path) (secret *corev1.Secret, configMap *corev1.ConfigMap, allErrs field.ErrorList) {
	//Decode Yaml
	if resourceYAML.Raw == nil {
		allErrs = append(allErrs, field.Invalid(fldPath, resourceYAML, "empty resource is not allowed"))
		return
	}
	resourceObject, groupVersionKind, err := legacyscheme.Codecs.UniversalDeserializer().Decode(resourceYAML.Raw, nil, nil)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, resourceYAML, fmt.Sprintf("failed to deserialize the resource, err %v", err)))
	}

	// convert to specific resource
	switch resourceObject.(type) {
	case *corev1.Secret:
		if resource, ok := resourceObject.(*corev1.Secret); !ok {
			allErrs = append(allErrs, field.InternalError(fldPath, fmt.Errorf("failed to convert resourceObject to Secret")))
		} else {
			secret = resource
		}
	case *corev1.ConfigMap:
		if resource, ok := resourceObject.(*corev1.ConfigMap); !ok {
			allErrs = append(allErrs, field.InternalError(fldPath, fmt.Errorf("failed to convert resourceObject to ConfigMap")))
		} else {
			configMap = resource
		}
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), resourceYAML, fmt.Sprintf("unknown or unsupported resource type %v", groupVersionKind)))
	}

	return
}
