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

package validating

import (
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
)

var (
	fakeScheme = runtime.NewScheme()
	h          = &ResourceDistributionCreateUpdateHandler{
		Client:  fake.NewClientBuilder().WithScheme(fakeScheme).Build(),
		Decoder: admission.NewDecoder(fakeScheme),
	}
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = appsv1alpha1.AddToScheme(fakeScheme)
	_ = appsv1beta1.AddToScheme(fakeScheme)
}

func FuzzDeserializeResource(f *testing.F) {
	for _, data := range fuzzutils.StructuredResources {
		f.Add(data.Data) // Add seed corpus
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		resourceRawExtension := &runtime.RawExtension{}
		resourceRawExtension.Raw = data
		_, _ = DeserializeResource(resourceRawExtension, field.NewPath("spec", "resource"))
	})
}

func FuzzValidateResourceDistributionSpec(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		newObj := &appsv1alpha1.ResourceDistribution{}
		if err := cf.GenerateStruct(newObj); err != nil {
			return
		}
		if err := fuzzutils.GenerateResourceDistributionResource(cf, newObj); err != nil {
			return
		}
		if err := fuzzutils.GenerateResourceDistributionTargets(cf, newObj); err != nil {
			return
		}

		oldObj := &appsv1alpha1.ResourceDistribution{}
		if hasOld, err := cf.GetBool(); hasOld && err == nil {
			if err := cf.GenerateStruct(oldObj); err != nil {
				return
			}
			if err := fuzzutils.GenerateResourceDistributionResource(cf, oldObj); err != nil {
				return
			}
			if err := fuzzutils.GenerateResourceDistributionTargets(cf, oldObj); err != nil {
				return
			}
			// Make sure oldObj has the same GVK as newObj
			if sameGVK, err := cf.GetBool(); sameGVK && err == nil {
				oldObj.SetGroupVersionKind(newObj.GetObjectKind().GroupVersionKind())
			}
		}
		_ = h.validateResourceDistributionSpec(specResourceView{resource: &newObj.Spec.Resource, targets: targetsViewFromV1alpha1(&newObj.Spec.Targets)}, oldSpecResourceView(oldObj), field.NewPath("spec"), false)
	})
}

func FuzzValidateResourceDistributionTargets(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		rd := &appsv1alpha1.ResourceDistribution{}
		if err := cf.GenerateStruct(rd); err != nil {
			return
		}

		if err := fuzzutils.GenerateResourceDistributionTargets(cf, rd); err != nil {
			return
		}

		_ = validateResourceDistributionTargets(targetsViewFromV1alpha1(&rd.Spec.Targets), field.NewPath("targets"), false)
	})
}

func FuzzValidateResourceDistributionResource(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		newObj, err := fuzzutils.GenerateResourceObject(cf)
		if err != nil {
			return
		}

		var oldObj runtime.Object
		if hasOld, err := cf.GetBool(); hasOld && err == nil {
			oldObj, err = fuzzutils.GenerateResourceObject(cf)
			if err != nil {
				return
			}

			// Make sure oldObj has the same GVK as newObj
			if sameGVK, err := cf.GetBool(); sameGVK && err == nil {
				oldObj.GetObjectKind().SetGroupVersionKind(newObj.GetObjectKind().GroupVersionKind())
			}
		}

		_ = h.validateResourceDistributionSpecResource(newObj, oldObj, field.NewPath("resource"), false)
	})
}

func FuzzValidateResourceDistributionSpecV1beta1(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		newObj := &appsv1beta1.ResourceDistribution{}
		if err := cf.GenerateStruct(newObj); err != nil {
			return
		}
		alphaObj := &appsv1alpha1.ResourceDistribution{}
		if err := fuzzutils.GenerateResourceDistributionResource(cf, alphaObj); err != nil {
			return
		}
		if err := fuzzutils.GenerateResourceDistributionTargets(cf, alphaObj); err != nil {
			return
		}
		newObj.Spec.Resource = alphaObj.Spec.Resource
		newObj.Spec.Targets = appsv1beta1.ResourceDistributionTargets{
			AllNamespaces:          alphaObj.Spec.Targets.AllNamespaces,
			NamespaceLabelSelector: alphaObj.Spec.Targets.NamespaceLabelSelector,
			IncludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
				List: toBetaNamespaces(alphaObj.Spec.Targets.IncludedNamespaces.List),
			},
			ExcludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
				List: toBetaNamespaces(alphaObj.Spec.Targets.ExcludedNamespaces.List),
			},
		}

		_ = h.validateResourceDistributionV1beta1(newObj, nil)
	})
}

func toBetaNamespaces(namespaces []appsv1alpha1.ResourceDistributionNamespace) []appsv1beta1.ResourceDistributionNamespace {
	values := make([]appsv1beta1.ResourceDistributionNamespace, 0, len(namespaces))
	for _, namespace := range namespaces {
		values = append(values, appsv1beta1.ResourceDistributionNamespace{Name: namespace.Name})
	}
	return values
}
