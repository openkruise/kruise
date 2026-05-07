/*
Copyright 2025 The Kruise Authors.

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

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
)

func FuzzMatchFunctions(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		rd := &appsv1beta1.ResourceDistribution{}
		if err := cf.GenerateStruct(rd); err != nil {
			return
		}
		alphaRD := &appsv1alpha1.ResourceDistribution{}

		if err := fuzzutils.GenerateResourceDistributionResource(cf, alphaRD); err != nil {
			return
		}
		if err := fuzzutils.GenerateResourceDistributionTargets(cf, alphaRD); err != nil {
			return
		}
		rd.Spec.Resource = alphaRD.Spec.Resource
		rd.Spec.Targets = appsv1beta1.ResourceDistributionTargets{
			AllNamespaces:          alphaRD.Spec.Targets.AllNamespaces,
			NamespaceLabelSelector: alphaRD.Spec.Targets.NamespaceLabelSelector,
			IncludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
				List: convertNamespacesToV1beta1(alphaRD.Spec.Targets.IncludedNamespaces.List),
			},
			ExcludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
				List: convertNamespacesToV1beta1(alphaRD.Spec.Targets.ExcludedNamespaces.List),
			},
		}

		matched, err := cf.GetBool()
		if err != nil {
			return
		}

		namespace := &corev1.Namespace{}
		if matched {
			// If includedNamespaces is not empty, use the first one as namespace name
			if len(rd.Spec.Targets.IncludedNamespaces.List) > 0 {
				namespace.SetName(rd.Spec.Targets.IncludedNamespaces.List[0].Name)
			}
			namespace.SetLabels(rd.Spec.Targets.NamespaceLabelSelector.MatchLabels)
		} else {
			if err := cf.GenerateStruct(namespace); err != nil {
				return
			}
		}

		_, _ = matchViaIncludedNamespaces(namespace, rd)
		_, _ = matchViaLabelSelector(namespace, rd)
		_, _ = matchViaTargets(namespace, rd)
	})
}

func convertNamespacesToV1beta1(namespaces []appsv1alpha1.ResourceDistributionNamespace) []appsv1beta1.ResourceDistributionNamespace {
	values := make([]appsv1beta1.ResourceDistributionNamespace, 0, len(namespaces))
	for _, namespace := range namespaces {
		values = append(values, appsv1beta1.ResourceDistributionNamespace{Name: namespace.Name})
	}
	return values
}

func FuzzMergeMetadata(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		newResource := &unstructured.Unstructured{}
		if err := fuzzutils.GenerateUnstructured(cf, newResource); err != nil {
			return
		}
		oldResource := &unstructured.Unstructured{}
		if err := fuzzutils.GenerateUnstructured(cf, oldResource); err != nil {
			return
		}

		mergeMetadata(newResource, oldResource)
	})
}
