/*
Copyright 2020 The Kruise Authors.

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

package daemonset

import (
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func newTestPodEventHandler(reader client.Reader, expectations kubecontroller.ControllerExpectationsInterface) *podEventHandler {
	return &podEventHandler{
		Reader:       reader,
		expectations: expectations,
	}
}

func TestEnqueueRequestForPodCreate(t *testing.T) {
	lTrue := true
	cases := []struct {
		name                          string
		dss                           []*appsv1alpha1.DaemonSet
		e                             event.TypedCreateEvent[*v1.Pod]
		alterExpectationCreationsKey  string
		alterExpectationCreationsAdds []string
		expectedQueueLen              int
	}{
		{
			name: "no ds",
			e:    event.TypedCreateEvent[*v1.Pod]{Object: &v1.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}}}},

			expectedQueueLen: 0,
		},
		{
			name: "no ds",
			e:    event.TypedCreateEvent[*v1.Pod]{Object: &v1.Pod{}},

			expectedQueueLen: 0,
		},
		{
			name: "multi ds",
			dss: []*appsv1alpha1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds01",
						Namespace: "default",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds02",
						Namespace: "default",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
			},
			e:                event.TypedCreateEvent[*v1.Pod]{Object: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "pod-abc", Labels: map[string]string{"key": "v1"}}}},
			expectedQueueLen: 2,
		},
		{
			name: "correct owner reference",
			dss: []*appsv1alpha1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
			},
			e: event.TypedCreateEvent[*v1.Pod]{
				Object: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "pod-xyz",
						Labels:    map[string]string{"key": "v1"},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "DaemonSet",
								Name:       "ds02",
								UID:        "002",
								Controller: &lTrue,
							},
						},
					},
				},
			},
			alterExpectationCreationsKey:  "default/ds02",
			alterExpectationCreationsAdds: []string{"pod-xyz"},
			expectedQueueLen:              1,
		},
	}
	logger := klog.FromContext(context.TODO())
	for _, testCase := range cases {
		fakeClient := fake.NewClientBuilder().Build()
		for _, ds := range testCase.dss {
			fakeClient.Create(context.TODO(), ds)
		}

		exp := kubecontroller.NewControllerExpectations()
		enqueueHandler := newTestPodEventHandler(fakeClient, exp)
		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test-queue")

		for i := 0; i < len(testCase.alterExpectationCreationsAdds); i++ {
			exp.ExpectCreations(logger, testCase.alterExpectationCreationsKey, 1)
		}

		if testCase.alterExpectationCreationsKey != "" {
			if ok := exp.SatisfiedExpectations(logger, testCase.alterExpectationCreationsKey); ok {
				t.Fatalf("%s before execute, should not be satisfied", testCase.name)
			}
		}

		enqueueHandler.Create(context.TODO(), testCase.e, q)
		if q.Len() != testCase.expectedQueueLen {
			t.Fatalf("%s failed, expected queue len %d, got queue len %d", testCase.name, testCase.expectedQueueLen, q.Len())
		}

		if testCase.alterExpectationCreationsKey != "" {
			if ok := exp.SatisfiedExpectations(logger, testCase.alterExpectationCreationsKey); !ok {
				t.Fatalf("%s after execute, should be satisfied", testCase.name)
			}
		}
	}
}

func TestEnqueueRequestForPodUpdate(t *testing.T) {
	lTrue := true
	cases := []struct {
		name             string
		dss              []*appsv1alpha1.DaemonSet
		e                event.TypedUpdateEvent[*v1.Pod]
		expectedQueueLen int
	}{
		{
			name:             "resourceVersion no changed",
			e:                event.TypedUpdateEvent[*v1.Pod]{ObjectNew: &v1.Pod{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "01"}}, ObjectOld: &v1.Pod{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "01"}}},
			expectedQueueLen: 0,
		},
		{
			name: "label changed and update 1",
			dss: []*appsv1alpha1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v2"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v2"},
							},
						},
					},
				},
			},
			e: event.TypedUpdateEvent[*v1.Pod]{
				ObjectOld: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "default",
						ResourceVersion: "01",
						Labels:          map[string]string{"key": "v1", "test": "true"},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "DaemonSet",
								Name:       "ds01",
								UID:        "001",
								Controller: &lTrue,
							},
						},
					},
				},
				ObjectNew: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						ResourceVersion:   "02",
						Labels:            map[string]string{"key": "v1", "test": "false"},
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "DaemonSet",
								Name:       "ds01",
								UID:        "001",
								Controller: &lTrue,
							},
						},
					},
				},
			},
			expectedQueueLen: 1,
		},
		{
			name: "label changed and update 2",
			dss: []*appsv1alpha1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v2"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v2"},
							},
						},
					},
				},
			},
			e: event.TypedUpdateEvent[*v1.Pod]{
				ObjectOld: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "default",
						ResourceVersion: "01",
						Labels:          map[string]string{"key": "v1", "test": "true"},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "DaemonSet",
								Name:       "ds01",
								UID:        "001",
								Controller: &lTrue,
							},
						},
					},
				},
				ObjectNew: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						ResourceVersion:   "02",
						Labels:            map[string]string{"key": "v2", "test": "true"},
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "DaemonSet",
								Name:       "ds02",
								UID:        "002",
								Controller: &lTrue,
							},
						},
					},
				},
			},
			expectedQueueLen: 2,
		},
		{
			name: "reference changed",
			dss: []*appsv1alpha1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v2"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v2"},
							},
						},
					},
				},
			},
			e: event.TypedUpdateEvent[*v1.Pod]{
				ObjectOld: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "default",
						ResourceVersion: "01",
						Labels:          map[string]string{"key": "v1", "test": "true"},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "DaemonSet",
								Name:       "ds01",
								UID:        "001",
								Controller: &lTrue,
							},
						},
					},
				},
				ObjectNew: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "default",
						ResourceVersion: "02",
						Labels:          map[string]string{"key": "v2", "test": "true"},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "DaemonSet",
								Name:       "ds02",
								UID:        "002",
								Controller: &lTrue,
							},
						},
					},
				},
			},
			expectedQueueLen: 2,
		},
		{
			name: "reference not changed",
			dss: []*appsv1alpha1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v2"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v2"},
							},
						},
					},
				},
			},
			e: event.TypedUpdateEvent[*v1.Pod]{
				ObjectOld: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "default",
						ResourceVersion: "01",
						Labels:          map[string]string{"key": "v1", "test": "true"},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "DaemonSet",
								Name:       "ds01",
								UID:        "001",
								Controller: &lTrue,
							},
						},
					},
				},
				ObjectNew: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "default",
						ResourceVersion: "02",
						Labels:          map[string]string{"key": "v1", "test": "true"},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "DaemonSet",
								Name:       "ds01",
								UID:        "001",
								Controller: &lTrue,
							},
						},
					},
				},
			},
			expectedQueueLen: 1,
		},
		{
			name: "orphan changed",
			dss: []*appsv1alpha1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v2"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v2"},
							},
						},
					},
				},
			},
			e: event.TypedUpdateEvent[*v1.Pod]{
				ObjectOld: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "default",
						ResourceVersion: "01",
						Labels:          map[string]string{"key": "v1", "test": "true"},
					},
				},
				ObjectNew: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "default",
						ResourceVersion: "02",
						Labels:          map[string]string{"key": "v1", "test": "true"},
					},
				},
			},
			expectedQueueLen: 0,
		},
		{
			name: "reference changed",
			dss: []*appsv1alpha1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v2"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v2"},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds03",
						Namespace: "default",
						UID:       "003",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v2"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v2"},
							},
						},
					},
				},
			},
			e: event.TypedUpdateEvent[*v1.Pod]{
				ObjectOld: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "default",
						ResourceVersion: "01",
						Labels:          map[string]string{"key": "v1", "test": "true"},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "DaemonSet",
								Name:       "ds01",
								UID:        "001",
								Controller: &lTrue,
							},
						},
					},
				},
				ObjectNew: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "default",
						ResourceVersion: "02",
						Labels:          map[string]string{"key": "v2", "test": "true"},
					},
				},
			},
			expectedQueueLen: 3,
		},
	}

	for _, testCase := range cases {
		fakeClient := fake.NewClientBuilder().Build()
		for _, ds := range testCase.dss {
			fakeClient.Create(context.TODO(), ds)
		}
		exp := kubecontroller.NewControllerExpectations()
		enqueueHandler := newTestPodEventHandler(fakeClient, exp)
		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test-queue")

		enqueueHandler.Update(context.TODO(), testCase.e, q)
		time.Sleep(time.Millisecond * 10)
		if q.Len() != testCase.expectedQueueLen {
			t.Fatalf("%s failed, expected queue len %d, got queue len %d", testCase.name, testCase.expectedQueueLen, q.Len())
		}
	}
}

func newTestNodeEventHandler(client client.Client) *nodeEventHandler {
	return &nodeEventHandler{
		reader: client,
	}
}

func TestEnqueueRequestForNodeCreate(t *testing.T) {
	cases := []struct {
		name             string
		dss              []*appsv1alpha1.DaemonSet
		e                event.TypedCreateEvent[*v1.Node]
		expectedQueueLen int
	}{
		{
			name: "add one unmatched node",
			dss: []*appsv1alpha1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds01",
						Namespace: "default",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds02",
						Namespace: "default",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
			},
			e: event.TypedCreateEvent[*v1.Node]{
				Object: &v1.Node{
					Spec: v1.NodeSpec{
						Taints: []v1.Taint{
							{
								Key:    "aaa",
								Value:  "bbb",
								Effect: v1.TaintEffectNoSchedule,
							},
						},
					},
				},
			},
			expectedQueueLen: 0,
		},
		{
			name: "add one matched node",
			dss: []*appsv1alpha1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds01",
						Namespace: "default",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds02",
						Namespace: "default",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
			},
			e: event.TypedCreateEvent[*v1.Node]{
				Object: &v1.Node{},
			},
			expectedQueueLen: 2,
		},
	}
	for _, testCase := range cases {
		fakeClient := fake.NewClientBuilder().Build()
		for _, ds := range testCase.dss {
			fakeClient.Create(context.TODO(), ds)
		}
		enqueueHandler := newTestNodeEventHandler(fakeClient)
		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test-queue")

		enqueueHandler.Create(context.TODO(), testCase.e, q)
		if q.Len() != testCase.expectedQueueLen {
			t.Fatalf("%s failed, expected queue len %d, got queue len %d", testCase.name, testCase.expectedQueueLen, q.Len())
		}
	}
}

func TestEnqueueRequestForNodeUpdate(t *testing.T) {
	cases := []struct {
		name             string
		dss              []*appsv1alpha1.DaemonSet
		e                event.TypedUpdateEvent[*v1.Node]
		expectedQueueLen int
	}{
		{
			name: "shouldIgnoreNodeUpdate",
			e: event.TypedUpdateEvent[*v1.Node]{
				ObjectOld: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name0",
					},
				},
				ObjectNew: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name1",
					},
				},
			},
			expectedQueueLen: 0,
		},
		{
			name: "from unmatched to matched",
			dss: []*appsv1alpha1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds01",
						Namespace: "default",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
			},
			e: event.TypedUpdateEvent[*v1.Node]{
				ObjectOld: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name0",
					},
					Spec: v1.NodeSpec{
						Taints: []v1.Taint{
							{
								Key:    "aaa",
								Value:  "bbb",
								Effect: v1.TaintEffectNoSchedule,
							},
						},
					},
				},
				ObjectNew: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name1",
					},
				},
			},
			expectedQueueLen: 1,
		},
		{
			name: "from matched to unmatched",
			dss: []*appsv1alpha1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds01",
						Namespace: "default",
					},
					Spec: appsv1alpha1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"key": "v1"},
						},
						Template: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"key": "v1"},
							},
						},
					},
				},
			},
			e: event.TypedUpdateEvent[*v1.Node]{
				ObjectOld: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name0",
					},
				},
				ObjectNew: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name1",
					},
					Spec: v1.NodeSpec{
						Taints: []v1.Taint{
							{
								Key:    "aaa",
								Value:  "bbb",
								Effect: v1.TaintEffectNoSchedule,
							},
						},
					},
				},
			},
			expectedQueueLen: 1,
		},
	}
	for _, testCase := range cases {
		fakeClient := fake.NewClientBuilder().Build()
		for _, ds := range testCase.dss {
			fakeClient.Create(context.TODO(), ds)
		}
		enqueueHandler := newTestNodeEventHandler(fakeClient)
		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test-queue")

		enqueueHandler.Update(context.TODO(), testCase.e, q)
		if q.Len() != testCase.expectedQueueLen {
			t.Fatalf("%s failed, expected queue len %d, got queue len %d", testCase.name, testCase.expectedQueueLen, q.Len())
		}
	}
}
