/*
Copyright 2019 The Kruise Authors.

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

package statefulset

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/appscode/jsonpatch"
	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestPodInPlaceUpdate(t *testing.T) {
	fakePod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fake-pod",
			Namespace: "default",
			Labels: map[string]string{
				apps.StatefulSetRevisionLabel: "old-revision",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "c1",
					Image: "img01",
				},
				{
					Name:  "c2",
					Image: "img10",
				},
			},
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name:    "c1",
					ImageID: "imgID1",
				},
				{
					Name:    "c2",
					ImageID: "imgID2",
				},
			},
		},
	}

	inPlaceUpdateSpec := InPlaceUpdateSpec{
		revision:        "new-revision",
		containerImages: map[string]string{"c1": "img02"},
		patches: []jsonpatch.JsonPatchOperation{{
			Operation: "replace",
			Path:      "/spec/containers/0/image",
			Value:     "img02",
		}},
	}
	gotPod, err := podInPlaceUpdate(fakePod, &inPlaceUpdateSpec)
	if err != nil {
		t.Fatal(err)
	}

	if gotPod.Labels[apps.StatefulSetRevisionLabel] != inPlaceUpdateSpec.revision {
		t.Fatalf("Expected pod revision updated, got %v", gotPod.Labels[apps.StatefulSetRevisionLabel])
	}

	if gotPod.Spec.Containers[0].Image != "img02" {
		t.Fatalf("Expected pod image updated, got %v", gotPod.Spec.Containers[0].Image)
	}

	stateStr := gotPod.Annotations[appsv1alpha1.StatefulSetInPlaceUpdateStateAnnotation]
	gotInPlaceUpdateState := appsv1alpha1.InPlaceUpdateState{}
	if err := json.Unmarshal([]byte(stateStr), &gotInPlaceUpdateState); err != nil {
		t.Fatalf("Failed unmarshal InPlaceUpdateState: %v", err)
	}

	if gotInPlaceUpdateState.Revision != inPlaceUpdateSpec.revision ||
		gotInPlaceUpdateState.LastContainerStatuses["c1"].ImageID != "imgID1" {
		t.Fatalf("Incorrect InPlaceUpdateState: %v", gotInPlaceUpdateState)
	}
}

func TestShouldDoInPlaceUpdate(t *testing.T) {
	tests := []struct {
		name                      string
		rollingUpdateStrategy     *appsv1alpha1.RollingUpdateStatefulSetStrategy
		updateRevision            *apps.ControllerRevision
		oldRevisionName           string
		revisions                 []*apps.ControllerRevision
		expectedInPlaceUpdate     bool
		expectedInPlaceUpdateSpec *InPlaceUpdateSpec
	}{
		{
			name: "use in-place-update",
			rollingUpdateStrategy: &appsv1alpha1.RollingUpdateStatefulSetStrategy{
				PodUpdatePolicy: appsv1alpha1.InPlaceIfPossiblePodUpdateStrategyType,
			},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "new-revision"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			oldRevisionName: "old-revision",
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old-revision"},
					Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			expectedInPlaceUpdate: true,
			expectedInPlaceUpdateSpec: &InPlaceUpdateSpec{
				revision:        "new-revision",
				containerImages: map[string]string{"c1": "foo2"},
				patches: []jsonpatch.JsonPatchOperation{{
					Operation: "replace",
					Path:      "/spec/containers/0/image",
					Value:     "foo2",
				}},
			},
		},
		{
			name:                  "podUpdatePolicy is not InPlaceIfPossible",
			rollingUpdateStrategy: &appsv1alpha1.RollingUpdateStatefulSetStrategy{},
			updateRevision: &apps.ControllerRevision{
				Data: runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			oldRevisionName: "old-revision",
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old-revision"},
					Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
		},
		{
			name: "old revision not found",
			rollingUpdateStrategy: &appsv1alpha1.RollingUpdateStatefulSetStrategy{
				PodUpdatePolicy: appsv1alpha1.InPlaceIfPossiblePodUpdateStrategyType,
			},
			updateRevision: &apps.ControllerRevision{
				Data: runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			oldRevisionName: "old-revision",
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old-revision1"},
					Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
		},
		{
			name: "modify other than image",
			rollingUpdateStrategy: &appsv1alpha1.RollingUpdateStatefulSetStrategy{
				PodUpdatePolicy: appsv1alpha1.InPlaceIfPossiblePodUpdateStrategyType,
			},
			updateRevision: &apps.ControllerRevision{
				Data: runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2","env":["name":"k", "value":"v"]}]}}}}`)},
			},
			oldRevisionName: "old-revision",
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old-revision"},
					Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"name":"c1","containers":[{"image":"foo1"}]}}}}`)},
				},
			},
		},
	}

	for _, testCase := range tests {
		set := &appsv1alpha1.StatefulSet{
			Spec: appsv1alpha1.StatefulSetSpec{
				UpdateStrategy: appsv1alpha1.StatefulSetUpdateStrategy{
					Type:          apps.RollingUpdateStatefulSetStrategyType,
					RollingUpdate: testCase.rollingUpdateStrategy,
				},
			},
		}
		allowed, inPlaceUpdateSpec := shouldDoInPlaceUpdate(set, testCase.updateRevision, testCase.oldRevisionName, testCase.revisions)
		if allowed != testCase.expectedInPlaceUpdate {
			t.Errorf("%s: expected allowed %v, got %v", testCase.name, testCase.expectedInPlaceUpdate, allowed)
		}
		if !reflect.DeepEqual(inPlaceUpdateSpec, testCase.expectedInPlaceUpdateSpec) {
			t.Errorf("%s: expected inPlaceUpdateSpec %v, got %v", testCase.name, testCase.expectedInPlaceUpdateSpec, inPlaceUpdateSpec)
		}
	}
}

func TestCheckInPlaceUpdateReady(t *testing.T) {
	succeedPods := []*v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "s1",
				Labels: map[string]string{
					apps.StatefulSetRevisionLabel: "new-revision",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "s2",
				Labels: map[string]string{
					apps.StatefulSetRevisionLabel: "new-revision",
				},
				Annotations: map[string]string{
					appsv1alpha1.StatefulSetInPlaceUpdateStateAnnotation: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
				},
			},
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					{
						Name:    "c1",
						ImageID: "img02",
					},
				},
			},
		},
	}
	failPods := []*v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "f1",
				Labels: map[string]string{
					apps.StatefulSetRevisionLabel: "new-revision",
				},
				Annotations: map[string]string{
					appsv1alpha1.StatefulSetInPlaceUpdateStateAnnotation: `{"revision":"old-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
				},
			},
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					{
						Name:    "c1",
						ImageID: "img02",
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "f2",
				Labels: map[string]string{
					apps.StatefulSetRevisionLabel: "new-revision",
				},
				Annotations: map[string]string{
					appsv1alpha1.StatefulSetInPlaceUpdateStateAnnotation: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
				},
			},
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					{
						Name:    "c1",
						ImageID: "img01",
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "f3",
				Labels: map[string]string{
					apps.StatefulSetRevisionLabel: "new-revision",
				},
				Annotations: map[string]string{
					appsv1alpha1.StatefulSetInPlaceUpdateStateAnnotation: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
				},
			},
			Status: v1.PodStatus{},
		},
	}

	for _, p := range succeedPods {
		if err := checkInPlaceUpdateCompleted(p); err != nil {
			t.Errorf("pod %s expected check success, got %v", p.Name, err)
		}
	}
	for _, p := range failPods {
		if err := checkInPlaceUpdateCompleted(p); err == nil {
			t.Errorf("pod %s expected check failure, got no error", p.Name)
		}
	}
}

func TestShouldInPlaceConditionBeTrue(t *testing.T) {
	cases := []struct {
		name     string
		pod      *v1.Pod
		expected bool
	}{
		{
			name:     "no readiness-gate",
			pod:      &v1.Pod{},
			expected: false,
		},
		{
			name: "not in-place updated yet",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						apps.StatefulSetRevisionLabel: "new-revision",
					},
					Annotations: map[string]string{
						appsv1alpha1.StatefulSetInPlaceUpdateStateAnnotation: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
					},
				},
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appsv1alpha1.StatefulSetInPlaceUpdateReady}},
				},
			},
			expected: false,
		},
		{
			name: "no existing condition",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appsv1alpha1.StatefulSetInPlaceUpdateReady}},
				},
			},
			expected: true,
		},
		{
			name: "existing condition status is False",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appsv1alpha1.StatefulSetInPlaceUpdateReady}},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionTrue,
						},
						{
							Type:   appsv1alpha1.StatefulSetInPlaceUpdateReady,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "existing condition status is True",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appsv1alpha1.StatefulSetInPlaceUpdateReady}},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionFalse,
						},
						{
							Type:   appsv1alpha1.StatefulSetInPlaceUpdateReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, testCase := range cases {
		got := shouldUpdateInPlaceReady(testCase.pod)
		if got != testCase.expected {
			t.Fatalf("%s expected %v, got %v", testCase.name, testCase.expected, got)
		}
	}
}
