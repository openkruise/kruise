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
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
)

func FuzzMatchFunctions(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		rd := &appsv1alpha1.ResourceDistribution{}
		if err := cf.GenerateStruct(rd); err != nil {
			return
		}

		if err := fuzzutils.GenerateResourceDistributionResource(cf, rd); err != nil {
			return
		}
		if err := fuzzutils.GenerateResourceDistributionTargets(cf, rd); err != nil {
			return
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
