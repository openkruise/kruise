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

func TestNeedToUpdate(t *testing.T) {
	cases := []struct {
		name         string
		oldObj       func() *unstructured.Unstructured
		newObj       func() *unstructured.Unstructured
		expectUpdate bool
	}{
		{
			name: "unchanged resource without labels/annotations",
			oldObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.SetNamespace("default")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				return u
			},
			newObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				return u
			},
			expectUpdate: false,
		},
		{
			name: "data changed",
			oldObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.SetNamespace("default")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				return u
			},
			newObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.Object["data"] = map[string]interface{}{"key": "v2"}
				return u
			},
			expectUpdate: true,
		},
		{
			name: "label added to new resource",
			oldObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.SetNamespace("default")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				u.SetLabels(map[string]string{"env": "prod"})
				return u
			},
			newObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				u.SetLabels(map[string]string{"env": "prod", "tier": "backend"})
				return u
			},
			expectUpdate: true,
		},
		{
			name: "label modified on new resource",
			oldObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.SetNamespace("default")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				u.SetLabels(map[string]string{"env": "prod"})
				return u
			},
			newObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				u.SetLabels(map[string]string{"env": "staging"})
				return u
			},
			expectUpdate: true,
		},
		{
			name: "annotation added to new resource",
			oldObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.SetNamespace("default")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				return u
			},
			newObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				u.SetAnnotations(map[string]string{"note": "test"})
				return u
			},
			expectUpdate: true,
		},
		{
			name: "annotation modified on new resource",
			oldObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.SetNamespace("default")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				u.SetAnnotations(map[string]string{"note": "old"})
				return u
			},
			newObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				u.SetAnnotations(map[string]string{"note": "new"})
				return u
			},
			expectUpdate: true,
		},
		{
			name: "old resource has extra labels and annotations (retained without triggering update)",
			oldObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.SetNamespace("default")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				u.SetLabels(map[string]string{"env": "prod", "injected-by-namespace": "true"})
				u.SetAnnotations(map[string]string{"managed": "rd", "istio.io/inject": "false"})
				return u
			},
			newObj: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
				u.SetName("cm")
				u.Object["data"] = map[string]interface{}{"key": "v1"}
				u.SetLabels(map[string]string{"env": "prod"})
				u.SetAnnotations(map[string]string{"managed": "rd"})
				return u
			},
			expectUpdate: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := needToUpdate(tc.oldObj(), tc.newObj())
			if got != tc.expectUpdate {
				t.Fatalf("expected needToUpdate %v, but got %v", tc.expectUpdate, got)
			}
		})
	}
}
