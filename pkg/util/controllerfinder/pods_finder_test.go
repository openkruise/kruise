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

package controllerfinder

import (
	"testing"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	fakescale "k8s.io/client-go/scale/fake"
	clienttesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openkruise/kruise/pkg/util/fieldindex"
)

func TestGetPodsForRefCustomWorkloadScaleSelector(t *testing.T) {
	rolloutGVK := schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Rollout"}
	rolloutUID := types.UID("rollout-uid")

	newPod := func(name string, labels map[string]string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name, Labels: labels},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		}
	}

	cases := []struct {
		name          string
		scaleSelector string
		expectedPods  []string
	}{
		{
			// A freshly created custom workload has not reported status.selector
			// yet, so its scale subresource returns an empty selector. That must
			// mean "no pods yet", not "every pod in the namespace".
			name:          "empty scale selector matches no pods",
			scaleSelector: "",
			expectedPods:  nil,
		},
		{
			name:          "non-empty scale selector matches its pods",
			scaleSelector: "app=demo",
			expectedPods:  []string{"demo-1"},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithObjects(
					newPod("demo-1", map[string]string{"app": "demo"}),
					newPod("other-1", map[string]string{"app": "other"}),
					newPod("unlabeled-1", nil),
				).
				WithIndex(&corev1.Pod{}, fieldindex.IndexNameForOwnerRefUID, func(obj client.Object) []string {
					var owners []string
					for _, ref := range obj.GetOwnerReferences() {
						owners = append(owners, string(ref.UID))
					}
					return owners
				}).Build()

			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{rolloutGVK.GroupVersion()})
			mapper.Add(rolloutGVK, meta.RESTScopeNamespace)

			scaleClient := &fakescale.FakeScaleClient{}
			scaleClient.AddReactor("get", "rollouts", func(action clienttesting.Action) (bool, runtime.Object, error) {
				return true, &autoscalingv1.Scale{
					ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "demo", UID: rolloutUID},
					Spec:       autoscalingv1.ScaleSpec{Replicas: 2},
					Status:     autoscalingv1.ScaleStatus{Selector: cs.scaleSelector},
				}, nil
			})

			finder := &ControllerFinder{Client: fakeClient, mapper: mapper, scaleNamespacer: scaleClient}
			pods, replicas, err := finder.GetPodsForRef(rolloutGVK.GroupVersion().String(), rolloutGVK.Kind, "default", "demo", false)
			if err != nil {
				t.Fatalf("GetPodsForRef returned error: %v", err)
			}
			if replicas != 2 {
				t.Fatalf("expected 2 replicas, got %d", replicas)
			}
			var names []string
			for _, pod := range pods {
				names = append(names, pod.Name)
			}
			if len(names) != len(cs.expectedPods) {
				t.Fatalf("expected pods %v, got %v", cs.expectedPods, names)
			}
			for i := range names {
				if names[i] != cs.expectedPods[i] {
					t.Fatalf("expected pods %v, got %v", cs.expectedPods, names)
				}
			}
		})
	}
}
