/*
Copyright 2026 The Kruise Authors.

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

package ephemeraljob

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func newTestPod(name, appLabel string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels:    map[string]string{"app": appLabel},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{Name: "main", Image: "nginx"}},
		},
		Status: v1.PodStatus{Phase: v1.PodRunning},
	}
}

func newFakeReconciler(t *testing.T, pods ...*v1.Pod) *ReconcileEphemeralJob {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)

	initObjs := make([]client.Object, 0, len(pods))
	for i := range pods {
		initObjs = append(initObjs, pods[i])
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjs...).Build()
	return &ReconcileEphemeralJob{Client: fakeClient, scheme: scheme}
}

// TestFilterPodsEmptySelector is a regression test for #2656: a non-nil selector
// with no matchLabels and no matchExpressions was converted into labels.Everything(),
// so filterPods listed every active pod in the namespace and syncTargetPods injected
// ephemeral containers into unrelated pods. An empty selector should match nothing,
// consistent with podMatchedEphemeralJob.
func TestFilterPodsEmptySelector(t *testing.T) {
	r := newFakeReconciler(t, newTestPod("unrelated-a", "some-other-app"), newTestPod("unrelated-b", "yet-another-app"))

	job := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "ejob"},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Selector: &metav1.LabelSelector{},
		},
	}

	pods, err := r.filterPods(job)
	if err != nil {
		t.Fatalf("filterPods returned error: %v", err)
	}
	if len(pods) != 0 {
		names := make([]string, 0, len(pods))
		for i := range pods {
			names = append(names, pods[i].Name)
		}
		t.Errorf("empty selector matched %d unrelated pods (%v), expected none", len(pods), names)
	}
}

// TestFilterInjectedPodsEmptySelector checks the cleanup path shares the same
// selector semantics: an empty selector should match nothing.
func TestFilterInjectedPodsEmptySelector(t *testing.T) {
	r := newFakeReconciler(t, newTestPod("unrelated-a", "some-other-app"))

	job := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "ejob"},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Selector: &metav1.LabelSelector{},
		},
	}

	pods, err := r.filterInjectedPods(job)
	if err != nil {
		t.Fatalf("filterInjectedPods returned error: %v", err)
	}
	if len(pods) != 0 {
		t.Errorf("empty selector matched %d pods, expected none", len(pods))
	}
}

// TestFilterPodsSelectorMatchesLabeledPods checks the populated-selector path
// still matches only pods with the requested labels.
func TestFilterPodsSelectorMatchesLabeledPods(t *testing.T) {
	r := newFakeReconciler(t, newTestPod("foo-pod", "foo"), newTestPod("bar-pod", "bar"))

	job := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "ejob"},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "foo"},
			},
		},
	}

	pods, err := r.filterPods(job)
	if err != nil {
		t.Fatalf("filterPods returned error: %v", err)
	}
	if len(pods) != 1 || pods[0].Name != "foo-pod" {
		names := make([]string, 0, len(pods))
		for i := range pods {
			names = append(names, pods[i].Name)
		}
		t.Errorf("expected only [foo-pod], got %d pods (%v)", len(pods), names)
	}
}
