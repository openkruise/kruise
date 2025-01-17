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

package cloneset

import (
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util/expectations"
	"github.com/openkruise/kruise/pkg/util/feature"
)

func newTestPodEventHandler(reader client.Reader) *podEventHandler {
	return &podEventHandler{
		Reader: reader,
	}
}

func TestEnqueueRequestForPodCreate(t *testing.T) {
	lTrue := true
	cases := []struct {
		name                          string
		css                           []*appsv1alpha1.CloneSet
		e                             event.TypedCreateEvent[*v1.Pod]
		alterExpectationCreationsKey  string
		alterExpectationCreationsAdds []string
		expectedQueueLen              int
	}{
		{
			name: "no cs",
			e:    event.TypedCreateEvent[*v1.Pod]{Object: &v1.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}}}},

			expectedQueueLen: 0,
		},
		{
			name: "no cs",
			e:    event.TypedCreateEvent[*v1.Pod]{Object: &v1.Pod{}},

			expectedQueueLen: 0,
		},
		{
			name: "multi cs",
			css: []*appsv1alpha1.CloneSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cs01",
						Namespace: "default",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
						Name:      "cs02",
						Namespace: "default",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
			css: []*appsv1alpha1.CloneSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cs01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
						Name:      "cs02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
								Kind:       "CloneSet",
								Name:       "cs02",
								UID:        "002",
								Controller: &lTrue,
							},
						},
					},
				},
			},
			alterExpectationCreationsKey:  "default/cs02",
			alterExpectationCreationsAdds: []string{"pod-xyz"},
			expectedQueueLen:              1,
		},
	}

	for _, testCase := range cases {
		fakeClient := fake.NewClientBuilder().Build()
		for _, cs := range testCase.css {
			fakeClient.Create(context.TODO(), cs)
		}

		enqueueHandler := newTestPodEventHandler(fakeClient)
		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test-queue")
		modifySatisfied := false
		if testCase.alterExpectationCreationsKey != "" && len(testCase.alterExpectationCreationsAdds) > 0 {
			for _, n := range testCase.alterExpectationCreationsAdds {
				clonesetutils.ScaleExpectations.ExpectScale(testCase.alterExpectationCreationsKey, expectations.Create, n)
			}
			if ok, _, _ := clonesetutils.ScaleExpectations.SatisfiedExpectations(testCase.alterExpectationCreationsKey); ok {
				t.Fatalf("%s before execute, should not be satisfied", testCase.name)
			}
			modifySatisfied = true
		}

		enqueueHandler.Create(context.TODO(), testCase.e, q)
		if q.Len() != testCase.expectedQueueLen {
			t.Fatalf("%s failed, expected queue len %d, got queue len %d", testCase.name, testCase.expectedQueueLen, q.Len())
		}
		if modifySatisfied {
			if ok, _, _ := clonesetutils.ScaleExpectations.SatisfiedExpectations(testCase.alterExpectationCreationsKey); !ok {
				t.Fatalf("%s expected satisfied, but it is not", testCase.name)
			}
		}
	}
}

func TestEnqueueRequestForPodUpdate(t *testing.T) {
	lTrue := true
	cases := []struct {
		name             string
		css              []*appsv1alpha1.CloneSet
		e                event.TypedUpdateEvent[*v1.Pod]
		expectedQueueLen int
	}{
		{
			name:             "resourceVersion no changed",
			e:                event.TypedUpdateEvent[*v1.Pod]{ObjectNew: &v1.Pod{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "01"}}, ObjectOld: &v1.Pod{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "01"}}},
			expectedQueueLen: 0,
		},
		{
			name: "label changed and delete 1",
			css: []*appsv1alpha1.CloneSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cs01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
						Name:      "cs02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
								Kind:       "CloneSet",
								Name:       "cs01",
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
								Kind:       "CloneSet",
								Name:       "cs01",
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
			name: "label changed and delete 2",
			css: []*appsv1alpha1.CloneSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cs01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
						Name:      "cs02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
								Kind:       "CloneSet",
								Name:       "cs01",
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
								Kind:       "CloneSet",
								Name:       "cs02",
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
			css: []*appsv1alpha1.CloneSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cs01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
						Name:      "cs02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
								Kind:       "CloneSet",
								Name:       "cs01",
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
								Kind:       "CloneSet",
								Name:       "cs02",
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
			css: []*appsv1alpha1.CloneSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cs01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
						Name:      "cs02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
								Kind:       "CloneSet",
								Name:       "cs01",
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
								Kind:       "CloneSet",
								Name:       "cs01",
								UID:        "001",
								Controller: &lTrue,
							},
						},
					},
				},
			},
			expectedQueueLen: 0,
		},
		{
			name: "orphan changed",
			css: []*appsv1alpha1.CloneSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cs01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
						Name:      "cs02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
			css: []*appsv1alpha1.CloneSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cs01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
						Name:      "cs02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
						Name:      "cs03",
						Namespace: "default",
						UID:       "003",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
								Kind:       "CloneSet",
								Name:       "cs01",
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
		{
			name: "container status invalid change",
			css: []*appsv1alpha1.CloneSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cs01",
						Namespace: "default",
						UID:       "001",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
						Name:      "cs02",
						Namespace: "default",
						UID:       "002",
					},
					Spec: appsv1alpha1.CloneSetSpec{
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
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "CloneSet",
								Name:       "cs01",
								UID:        "001",
								Controller: &lTrue,
							},
						},
					},
					Status: v1.PodStatus{
						Phase: v1.PodPending,
					},
				},
				ObjectNew: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						ResourceVersion:   "02",
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "CloneSet",
								Name:       "cs01",
								UID:        "001",
								Controller: &lTrue,
							},
						},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
					},
				},
			},
			expectedQueueLen: 1,
		},
	}

	defer feature.SetFeatureGateDuringTest(t, feature.DefaultFeatureGate, features.CloneSetEventHandlerOptimization, true)()

	for _, testCase := range cases {
		fakeClient := fake.NewClientBuilder().Build()
		for _, cs := range testCase.css {
			fakeClient.Create(context.TODO(), cs)
		}
		enqueueHandler := newTestPodEventHandler(fakeClient)
		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test-queue")

		enqueueHandler.Update(context.TODO(), testCase.e, q)
		time.Sleep(time.Millisecond * 10)
		if q.Len() != testCase.expectedQueueLen {
			t.Fatalf("%s failed, expected queue len %d, got queue len %d", testCase.name, testCase.expectedQueueLen, q.Len())
		}
	}
}

func TestEnqueueRequestForPodDelete(t *testing.T) {
	// Nothing to do, DeleteEvent already tested in other testing cases before
}

func TestGetPodCloneSets(t *testing.T) {
	testCases := []struct {
		inRSs     []*appsv1alpha1.CloneSet
		pod       *v1.Pod
		outRSName string
	}{
		// pods without labels don't match any CloneSets
		{
			inRSs: []*appsv1alpha1.CloneSet{
				{ObjectMeta: metav1.ObjectMeta{Name: "basic"}}},
			pod:       &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo1", Namespace: metav1.NamespaceAll}},
			outRSName: "",
		},
		// Matching labels, not namespace
		{
			inRSs: []*appsv1alpha1.CloneSet{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "foo"},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
					},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo2", Namespace: "ns", Labels: map[string]string{"foo": "bar"}}},
			outRSName: "",
		},
		// Matching ns and labels returns the key to the CloneSet, not the CloneSet name
		{
			inRSs: []*appsv1alpha1.CloneSet{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "bar", Namespace: "ns"},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
					},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo3", Namespace: "ns", Labels: map[string]string{"foo": "bar"}}},
			outRSName: "bar",
		},
	}

	for _, c := range testCases {
		fakeClient := fake.NewClientBuilder().Build()
		enqueueHandler := newTestPodEventHandler(fakeClient)
		for _, r := range c.inRSs {
			_ = fakeClient.Create(context.TODO(), r)
		}
		if rss := enqueueHandler.getPodCloneSets(c.pod); rss != nil {
			if len(rss) != 1 {
				t.Errorf("len(rss) = %v, want %v", len(rss), 1)
				continue
			}
			rs := rss[0]
			if c.outRSName != rs.GetName() {
				t.Errorf("Got replica set %+v expected %+v", rs.GetName(), c.outRSName)
			}
		} else if c.outRSName != "" {
			t.Errorf("Expected a replica set %v pod %v, found none", c.outRSName, c.pod.Name)
		}
	}
}
