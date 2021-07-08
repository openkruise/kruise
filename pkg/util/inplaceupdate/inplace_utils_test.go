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

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/revisionadapter"
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
		expectedSpec *UpdateSpec
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
			expectedSpec: &UpdateSpec{
				Revision:        "new-revision",
				ContainerImages: map[string]string{"c1": "foo2"},
			},
		},
		{
			oldRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "old-revision"},
				Data:       runtime.RawExtension{Raw: []byte(`{"metadata":{"labels":{"k":"v"}},"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
			},
			newRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "new-revision"},
				Data:       runtime.RawExtension{Raw: []byte(`{"metadata":{"labels":{"k":"v"}},"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			expectedSpec: &UpdateSpec{
				Revision:        "new-revision",
				ContainerImages: map[string]string{"c1": "foo2"},
			},
		},
		{
			oldRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "old-revision"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","metadata":{"labels":{"k":"v"}},"spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
			},
			newRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "new-revision"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","metadata":{"labels":{"k":"v","k1":"v1"}},"spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			expectedSpec: &UpdateSpec{
				Revision:        "new-revision",
				ContainerImages: map[string]string{"c1": "foo2"},
				MetaDataPatch:   []byte(`{"metadata":{"labels":{"k1":"v1"}}}`),
			},
		},
		{
			oldRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "old-revision"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","metadata":{"labels":{"k":"v","k2":"v2"},"finalizers":["fz1","fz2"]},"spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
			},
			newRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "new-revision"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","metadata":{"labels":{"k":"v","k1":"v1"},"finalizers":["fz2"]},"spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			expectedSpec: &UpdateSpec{
				Revision:        "new-revision",
				ContainerImages: map[string]string{"c1": "foo2"},
				MetaDataPatch:   []byte(`{"metadata":{"$deleteFromPrimitiveList/finalizers":["fz1"],"$setElementOrder/finalizers":["fz2"],"labels":{"k1":"v1","k2":null}}}`),
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
		res := defaultCalculateInPlaceUpdateSpec(tc.oldRevision, tc.newRevision, nil)
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
					appspub.InPlaceUpdateStateKey: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
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
		//{
		//	ObjectMeta: metav1.ObjectMeta{
		//		Name: "f1",
		//		Labels: map[string]string{
		//			apps.StatefulSetRevisionLabel: "new-revision",
		//		},
		//		Annotations: map[string]string{
		//			appspub.InPlaceUpdateStateKey: `{"revision":"old-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
		//		},
		//	},
		//	Status: v1.PodStatus{
		//		ContainerStatuses: []v1.ContainerStatus{
		//			{
		//				Name:    "c1",
		//				ImageID: "img02",
		//			},
		//		},
		//	},
		//},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "f2",
				Labels: map[string]string{
					apps.StatefulSetRevisionLabel: "new-revision",
				},
				Annotations: map[string]string{
					appspub.InPlaceUpdateStateKey: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
				},
			},
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					{
						Name:    "c1",
						ImageID: "img01",
						Image:   "image01",
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
					appspub.InPlaceUpdateStateKey: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
				},
			},
			Status: v1.PodStatus{},
		},
	}

	for _, p := range succeedPods {
		if err := DefaultCheckInPlaceUpdateCompleted(p); err != nil {
			t.Errorf("pod %s expected check success, got %v", p.Name, err)
		}
	}
	for _, p := range failPods {
		if err := DefaultCheckInPlaceUpdateCompleted(p); err == nil {
			t.Errorf("pod %s expected check failure, got no error", p.Name)
		}
	}
}

func TestRefresh(t *testing.T) {
	aHourAgo := metav1.NewTime(time.Unix(time.Now().Add(-time.Hour).Unix(), 0))
	tenSecondsAgo := metav1.NewTime(time.Now().Add(-time.Second * 10))

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
						appspub.InPlaceUpdateStateKey: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
					},
				},
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				},
			},
			expectedPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						apps.StatefulSetRevisionLabel: "new-revision",
					},
					Annotations: map[string]string{
						appspub.InPlaceUpdateStateKey: `{"revision":"new-revision","lastContainerStatuses":{"c1":{"imageID":"img01"}}}`,
					},
				},
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				},
			},
		},
		{
			name: "no existing condition",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				},
			},
			expectedPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1"},
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue, LastTransitionTime: aHourAgo}},
				},
			},
		},
		{
			name: "existing condition status is False",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionTrue,
						},
						{
							Type:   appspub.InPlaceUpdateReady,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			expectedPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1"},
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionTrue,
						},
						{
							Type:               appspub.InPlaceUpdateReady,
							Status:             v1.ConditionTrue,
							LastTransitionTime: aHourAgo,
						},
					},
				},
			},
		},
		{
			name: "existing condition status is True",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionFalse,
						},
						{
							Type:   appspub.InPlaceUpdateReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			expectedPod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionFalse,
						},
						{
							Type:   appspub.InPlaceUpdateReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
		},
		{
			name: "in-place update still in grace",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						apps.StatefulSetRevisionLabel: "new-revision",
					},
					Annotations: map[string]string{
						appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{Revision: "new-revision", UpdateTimestamp: tenSecondsAgo, LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "img01"}}}),
						appspub.InPlaceUpdateGraceKey: `{"revision":"new-revision","containerImages":{"main":"img-name02"},"graceSeconds":30}`,
					},
				},
				Spec: v1.PodSpec{
					Containers:     []v1.Container{{Name: "main", Image: "img-name01"}},
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				},
			},
			expectedPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						apps.StatefulSetRevisionLabel: "new-revision",
					},
					Annotations: map[string]string{
						appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{Revision: "new-revision", UpdateTimestamp: tenSecondsAgo, LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "img01"}}}),
						appspub.InPlaceUpdateGraceKey: `{"revision":"new-revision","containerImages":{"main":"img-name02"},"graceSeconds":30}`,
					},
				},
				Spec: v1.PodSpec{
					Containers:     []v1.Container{{Name: "main", Image: "img-name01"}},
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				},
			},
		},
		{
			name: "in-place update reach grace period",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						apps.StatefulSetRevisionLabel: "new-revision",
					},
					Annotations: map[string]string{
						appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{Revision: "new-revision", UpdateTimestamp: tenSecondsAgo, LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "img01"}}}),
						appspub.InPlaceUpdateGraceKey: `{"revision":"new-revision","containerImages":{"main":"img-name02"},"graceSeconds":5}`,
					},
				},
				Spec: v1.PodSpec{
					Containers:     []v1.Container{{Name: "main", Image: "img-name01"}},
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				},
			},
			expectedPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						apps.StatefulSetRevisionLabel: "new-revision",
					},
					Annotations: map[string]string{
						appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{Revision: "new-revision", UpdateTimestamp: tenSecondsAgo, LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "img01"}}}),
					},
					ResourceVersion: "1",
				},
				Spec: v1.PodSpec{
					Containers:     []v1.Container{{Name: "main", Image: "img-name02"}},
					ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				},
			},
		},
	}

	for i, testCase := range cases {
		testCase.pod.Name = fmt.Sprintf("pod-%d", i)
		testCase.expectedPod.Name = fmt.Sprintf("pod-%d", i)
		testCase.expectedPod.APIVersion = "v1"
		testCase.expectedPod.Kind = "Pod"

		cli := fake.NewFakeClient(testCase.pod)
		ctrl := NewForTest(cli, revisionadapter.NewDefaultImpl(), func() metav1.Time { return aHourAgo })
		if res := ctrl.Refresh(testCase.pod, nil); res.RefreshErr != nil {
			t.Fatalf("failed to update condition: %v", res.RefreshErr)
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
