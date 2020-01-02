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

package inplaceupdate

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCalculateInPlaceUpdateSpec(t *testing.T) {
	cases := []struct {
		oldRevision  *apps.ControllerRevision
		newRevision  *apps.ControllerRevision
		expectedSpec *updateSpec
	}{
		{
			oldRevision: nil,
			newRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "new-revision"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			expectedSpec: nil,
		},
		{
			oldRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "old-revision"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
			},
			newRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "new-revision"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			expectedSpec: &updateSpec{
				revision:        "new-revision",
				containerImages: map[string]string{"c1": "foo2"},
			},
		},
		{
			oldRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "old-revision"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"name":"c1","containers":[{"image":"foo1"}]}}}}`)},
			},
			newRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "new-revision"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2","env":["name":"k", "value":"v"]}]}}}}`)},
			},
			expectedSpec: nil,
		},
	}

	for i, tc := range cases {
		res := calculateInPlaceUpdateSpec(tc.oldRevision, tc.newRevision)
		if !reflect.DeepEqual(res, tc.expectedSpec) {
			t.Fatalf("case #%d failed, expected %v, got %v", i, tc.expectedSpec, res)
		}
	}
}

func TestCheckInPlaceUpdateCompleted(t *testing.T) {
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
					appsv1alpha1.InPlaceUpdateStateKey: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
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
					appsv1alpha1.InPlaceUpdateStateKey: `{"revision":"old-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
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
					appsv1alpha1.InPlaceUpdateStateKey: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
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
					appsv1alpha1.InPlaceUpdateStateKey: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
				},
			},
			Status: v1.PodStatus{},
		},
	}

	for _, p := range succeedPods {
		if err := CheckInPlaceUpdateCompleted(p); err != nil {
			t.Errorf("pod %s expected check success, got %v", p.Name, err)
		}
	}
	for _, p := range failPods {
		if err := CheckInPlaceUpdateCompleted(p); err == nil {
			t.Errorf("pod %s expected check failure, got no error", p.Name)
		}
	}
}

func TestUpdateCondition(t *testing.T) {
	now := metav1.NewTime(time.Unix(time.Now().Add(-time.Hour).Unix(), 0))
	cases := []struct {
		name        string
		pod         *v1.Pod
		expectedPod *v1.Pod
	}{
		{
			name:        "no readiness-gate",
			pod:         &v1.Pod{},
			expectedPod: &v1.Pod{},
		},
		{
			name: "not in-place updated yet",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						apps.StatefulSetRevisionLabel: "new-revision",
					},
					Annotations: map[string]string{
						appsv1alpha1.InPlaceUpdateStateKey: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
					},
				},
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appsv1alpha1.InPlaceUpdateReady}},
				},
			},
			expectedPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						apps.StatefulSetRevisionLabel: "new-revision",
					},
					Annotations: map[string]string{
						appsv1alpha1.InPlaceUpdateStateKey: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
					},
				},
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appsv1alpha1.InPlaceUpdateReady}},
				},
			},
		},
		{
			name: "no existing condition",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appsv1alpha1.InPlaceUpdateReady}},
				},
			},
			expectedPod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appsv1alpha1.InPlaceUpdateReady}},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{{Type: appsv1alpha1.InPlaceUpdateReady, Status: v1.ConditionTrue, LastTransitionTime: now}},
				},
			},
		},
		{
			name: "existing condition status is False",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appsv1alpha1.InPlaceUpdateReady}},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionTrue,
						},
						{
							Type:   appsv1alpha1.InPlaceUpdateReady,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			expectedPod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appsv1alpha1.InPlaceUpdateReady}},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionTrue,
						},
						{
							Type:               appsv1alpha1.InPlaceUpdateReady,
							Status:             v1.ConditionTrue,
							LastTransitionTime: now,
						},
					},
				},
			},
		},
		{
			name: "existing condition status is True",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appsv1alpha1.InPlaceUpdateReady}},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionFalse,
						},
						{
							Type:   appsv1alpha1.InPlaceUpdateReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			expectedPod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appsv1alpha1.InPlaceUpdateReady}},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionFalse,
						},
						{
							Type:   appsv1alpha1.InPlaceUpdateReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
		},
	}

	for i, testCase := range cases {
		testCase.pod.Name = fmt.Sprintf("pod-%d", i)
		testCase.expectedPod.Name = fmt.Sprintf("pod-%d", i)

		cli := fake.NewFakeClient(testCase.pod)
		ctrl := NewForTest(cli, apps.ControllerRevisionHashLabelKey, func() metav1.Time { return now })
		if err := ctrl.UpdateCondition(testCase.pod); err != nil {
			t.Fatalf("failed to update condition: %v", err)
		}

		got := &v1.Pod{}
		if err := cli.Get(context.TODO(), types.NamespacedName{Name: testCase.pod.Name}, got); err != nil {
			t.Fatalf("failed to get pod: %v", err)
		}

		if !reflect.DeepEqual(testCase.expectedPod, got) {
			t.Fatalf("case %s failed, expected \n%v got \n%v", testCase.name, util.DumpJSON(testCase.expectedPod), util.DumpJSON(got))
		}
	}
}
