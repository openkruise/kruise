/*
Copyright 2024 The Kruise Authors.

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

package adapter

import (
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/apis/apps/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/scale/scheme/appsv1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPostUpdate(t *testing.T) {
	fakeClient, scheme := getClientAndScheme()
	testCases := []struct {
		name         string
		adapter      Adapter
		subsetGetter func() client.Object
	}{
		{
			name: "AdvancedStatefulSet",
			adapter: &AdvancedStatefulSetAdapter{
				Client: fakeClient,
				Scheme: scheme,
			},
			subsetGetter: func() client.Object {
				return nil
			},
		},
		{
			name: "CloneSet",
			adapter: &CloneSetAdapter{
				Client: fakeClient,
				Scheme: scheme,
			},
			subsetGetter: func() client.Object {
				return nil
			},
		},
		{
			name: "Deployment",
			adapter: &DeploymentAdapter{
				Client: fakeClient,
				Scheme: scheme,
			},
			subsetGetter: func() client.Object {
				return nil
			},
		},
		{
			name: "StatefulSet",
			adapter: &StatefulSetAdapter{
				Client: fakeClient,
				Scheme: scheme,
			},
			subsetGetter: func() client.Object {
				return &appsv1.StatefulSet{
					Spec: appsv1.StatefulSetSpec{
						UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
							Type: appsv1.OnDeleteStatefulSetStrategyType,
						},
					},
				}
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.adapter.PostUpdate(nil, testCase.subsetGetter(), "", 0); err != nil {
				t.Errorf("PostUpdate() error = %v", err)
			}
		})
	}
}

func TestApplySubsetTemplate(t *testing.T) {
	var scheme = runtime.NewScheme()
	var fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = appsv1beta1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	testCases := []struct {
		name    string
		adapter Adapter
	}{
		{
			name: "AdvancedStatefulSet",
			adapter: &AdvancedStatefulSetAdapter{
				Client: fakeClient,
				Scheme: scheme,
			},
		},
		{
			name: "CloneSet",
			adapter: &CloneSetAdapter{
				Client: fakeClient,
				Scheme: scheme,
			},
		},
		{
			name: "Deployment",
			adapter: &DeploymentAdapter{
				Client: fakeClient,
				Scheme: scheme,
			},
		},
		{
			name: "StatefulSet",
			adapter: &StatefulSetAdapter{
				Client: fakeClient,
				Scheme: scheme,
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			subset := testCase.adapter.NewResourceObject()
			ud := newUnitedDeploymentWithAdapter(testCase.adapter)
			revision := "abcd"
			subsetName := "subset-a"
			if err := testCase.adapter.ApplySubsetTemplate(ud, subsetName, revision, 2, 4, subset); err != nil {
				t.Fatalf("ApplySubsetTemplate() error = %v", err)
			}
			if subset.GetNamespace() != ud.Namespace {
				t.Errorf("compare namespace failed: ud %+v, subset %+v", subset.GetNamespace(), ud.Namespace)
			}
			compareMap(subset.GetLabels(), map[string]string{
				"custom-label-1":                            "custom-value-1",
				"selector-key":                              "selector-value",
				appsv1alpha1.SubSetNameLabelKey:             subsetName,
				appsv1alpha1.ControllerRevisionHashLabelKey: revision,
			}, t)
			compareMap(subset.GetAnnotations(), map[string]string{
				"annotation-key":                      "annotation-value",
				appsv1alpha1.AnnotationSubsetPatchKey: `{"metadata":{"annotations":{"patched-key":"patched-value"}}}`,
			}, t)
			compareMap(getPodAnnotationsFromSubset(subset), map[string]string{"patched-key": "patched-value"}, t)
		})
	}
}

func compareMap(actual, expect map[string]string, t *testing.T) {
	for k := range expect {
		ev := expect[k]
		av, ok := actual[k]
		if !ok {
			t.Errorf("missing key %s", k)
		}
		if ev != av {
			t.Errorf("diff in map key %s: expect %s, actual %s", k, ev, av)
		}
	}
}

func getClientAndScheme() (fakeClient client.Client, scheme *runtime.Scheme) {
	scheme = runtime.NewScheme()
	fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = appsv1beta1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	return
}

func getPodAnnotationsFromSubset(object client.Object) map[string]string {
	switch object.(type) {
	case *v1beta1.StatefulSet:
		return object.(*v1beta1.StatefulSet).Spec.Template.Annotations
	case *appsv1alpha1.CloneSet:
		return object.(*appsv1alpha1.CloneSet).Spec.Template.Annotations
	case *appsv1.Deployment:
		return object.(*appsv1.Deployment).Spec.Template.Annotations
	case *appsv1.StatefulSet:
		return object.(*appsv1.StatefulSet).Spec.Template.Annotations
	}
	return nil
}

func newUnitedDeploymentWithAdapter(adapter Adapter) *appsv1alpha1.UnitedDeployment {
	ud := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"selector-key": "selector-value",
			}},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"patched-key":"patched-value"}}}`),
						},
					},
				},
			},
		},
	}
	object := adapter.NewResourceObject()
	switch object.(type) {
	case *v1beta1.StatefulSet:
		ud.Spec.Template.AdvancedStatefulSetTemplate = &appsv1alpha1.AdvancedStatefulSetTemplateSpec{}
		ud.Spec.Template.AdvancedStatefulSetTemplate.Labels = map[string]string{"custom-label-1": "custom-value-1"}
		ud.Spec.Template.AdvancedStatefulSetTemplate.Annotations = map[string]string{"annotation-key": "annotation-value"}
	case *appsv1alpha1.CloneSet:
		ud.Spec.Template.CloneSetTemplate = &appsv1alpha1.CloneSetTemplateSpec{}
		ud.Spec.Template.CloneSetTemplate.Labels = map[string]string{"custom-label-1": "custom-value-1"}
		ud.Spec.Template.CloneSetTemplate.Annotations = map[string]string{"annotation-key": "annotation-value"}
	case *appsv1.Deployment:
		ud.Spec.Template.DeploymentTemplate = &appsv1alpha1.DeploymentTemplateSpec{}
		ud.Spec.Template.DeploymentTemplate.Labels = map[string]string{"custom-label-1": "custom-value-1"}
		ud.Spec.Template.DeploymentTemplate.Annotations = map[string]string{"annotation-key": "annotation-value"}
	case *appsv1.StatefulSet:
		ud.Spec.Template.StatefulSetTemplate = &appsv1alpha1.StatefulSetTemplateSpec{}
		ud.Spec.Template.StatefulSetTemplate.Labels = map[string]string{"custom-label-1": "custom-value-1"}
		ud.Spec.Template.StatefulSetTemplate.Annotations = map[string]string{"annotation-key": "annotation-value"}
	}
	return ud
}
