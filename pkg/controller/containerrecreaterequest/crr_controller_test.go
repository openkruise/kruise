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

package containerrecreaterequest

import (
	"context"
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/clock"
	clocktesting "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	utilpodreadiness "github.com/openkruise/kruise/pkg/util/podreadiness"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	v1.AddToScheme(scheme)
	appsv1alpha1.AddToScheme(scheme)
}

type fakePodReadinessControl struct {
	containsReadinessGate bool
	addNotReadyError      error
	removeNotReadyError   error
	addedKeys             map[types.NamespacedName][]utilpodreadiness.Message
	removedKeys           map[types.NamespacedName][]utilpodreadiness.Message
}

func (f *fakePodReadinessControl) ContainsReadinessGate(pod *v1.Pod) bool {
	return f.containsReadinessGate
}

func (f *fakePodReadinessControl) AddNotReadyKey(pod *v1.Pod, msg utilpodreadiness.Message) error {
	if f.addNotReadyError != nil {
		return f.addNotReadyError
	}
	if f.addedKeys == nil {
		f.addedKeys = make(map[types.NamespacedName][]utilpodreadiness.Message)
	}
	nn := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}
	f.addedKeys[nn] = append(f.addedKeys[nn], msg)
	return nil
}

func (f *fakePodReadinessControl) RemoveNotReadyKey(pod *v1.Pod, msg utilpodreadiness.Message) error {
	if f.removeNotReadyError != nil {
		return f.removeNotReadyError
	}
	if f.removedKeys == nil {
		f.removedKeys = make(map[types.NamespacedName][]utilpodreadiness.Message)
	}
	nn := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}
	f.removedKeys[nn] = append(f.removedKeys[nn], msg)
	return nil
}

func TestReconcile(t *testing.T) {
	tests := []struct {
		name               string
		crr                *appsv1alpha1.ContainerRecreateRequest
		pods               []*v1.Pod
		existingObjs       []client.Object
		fakeClock          clock.Clock
		podReadinessGate   bool
		expectedPhase      appsv1alpha1.ContainerRecreateRequestPhase
		expectedRequeue    time.Duration
		expectedFinalizers []string
		expectedMsg        string
		creationTimeOffset time.Duration
	}{
		{
			name:               "Happy Path: New CRR created, Pod exists, Phase empty -> Recreating",
			creationTimeOffset: 0,
			crr: &appsv1alpha1.ContainerRecreateRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-crr",
					Namespace: "default",
					UID:       "crr-uid",
					Labels: map[string]string{
						appsv1alpha1.ContainerRecreateRequestPodUIDKey: "pod-uid",
					},
				},
				Spec: appsv1alpha1.ContainerRecreateRequestSpec{
					PodName: "test-pod",
					Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
						{Name: "main"},
					},
					Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "test-pod",
						Namespace:   "default",
						UID:         "pod-uid",
						Annotations: map[string]string{},
						Labels:      map[string]string{},
					},
					Status: v1.PodStatus{
						ContainerStatuses: []v1.ContainerStatus{
							{
								Name: "main",
								State: v1.ContainerState{
									Running: &v1.ContainerStateRunning{
										StartedAt: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
									},
								},
								ContainerID: "docker://123",
							},
						},
					},
				},
			},
			existingObjs:     []client.Object{},
			fakeClock:        clocktesting.NewFakeClock(time.Now()),
			podReadinessGate: true,
			expectedPhase:    "",
			expectedRequeue:  responseTimeout,
		},
		{
			name: "Status Sync: CRR in Recreating phase should sync container statuses",
			crr: &appsv1alpha1.ContainerRecreateRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-crr-sync",
					Namespace: "default",
					UID:       "crr-uid-sync",
					Labels: map[string]string{
						appsv1alpha1.ContainerRecreateRequestPodUIDKey: "pod-uid-sync",
					},
				},
				Spec: appsv1alpha1.ContainerRecreateRequestSpec{
					PodName: "test-pod-sync",
					Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
						{Name: "main"},
					},
					Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{},
				},
				Status: appsv1alpha1.ContainerRecreateRequestStatus{
					Phase: appsv1alpha1.ContainerRecreateRequestRecreating,
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-sync",
						Namespace: "default",
						UID:       "pod-uid-sync",
					},
					Status: v1.PodStatus{
						ContainerStatuses: []v1.ContainerStatus{
							{
								Name: "main",
								State: v1.ContainerState{
									Running: &v1.ContainerStateRunning{
										StartedAt: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
									},
								},
								ContainerID:  "docker://abc",
								RestartCount: 1,
							},
						},
					},
				},
			},
			existingObjs:    []client.Object{},
			fakeClock:       clocktesting.NewFakeClock(time.Now()),
			expectedPhase:   appsv1alpha1.ContainerRecreateRequestRecreating,
			expectedRequeue: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.crr.CreationTimestamp = metav1.NewTime(time.Now().Add(tt.creationTimeOffset))
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&appsv1alpha1.ContainerRecreateRequest{}).WithObjects(tt.crr).Build()
			for _, p := range tt.pods {
				if err := fakeClient.Create(context.TODO(), p); err != nil {
					t.Fatalf("Failed to create pod: %v", err)
				}
			}
			for _, o := range tt.existingObjs {
				if err := fakeClient.Create(context.TODO(), o); err != nil {
					t.Fatalf("Failed to create existing object: %v", err)
				}
			}

			r := &ReconcileContainerRecreateRequest{
				Client: fakeClient,
				clock:  tt.fakeClock,
				podReadinessControl: &fakePodReadinessControl{
					containsReadinessGate: tt.podReadinessGate,
				},
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: tt.crr.Namespace,
					Name:      tt.crr.Name,
				},
			}

			res, err := r.Reconcile(context.TODO(), req)
			if err != nil {
				t.Errorf("Reconcile() error = %v", err)
			}

			if tt.expectedRequeue > 0 {
				if res.RequeueAfter > tt.expectedRequeue || res.RequeueAfter < tt.expectedRequeue-5*time.Second {
					t.Errorf("Reconcile() RequeueAfter = %v, want ~%v", res.RequeueAfter, tt.expectedRequeue)
				}
			} else if res.RequeueAfter != 0 {
				t.Errorf("Reconcile() RequeueAfter = %v, want 0", res.RequeueAfter)
			}

			updatedCRR := &appsv1alpha1.ContainerRecreateRequest{}
			fakeClient.Get(context.TODO(), req.NamespacedName, updatedCRR)

			if tt.expectedPhase != "" && updatedCRR.Status.Phase != tt.expectedPhase {
				t.Errorf("Requests Phase = %v, want %v", updatedCRR.Status.Phase, tt.expectedPhase)
			}

			if tt.expectedMsg != "" && updatedCRR.Status.Message != tt.expectedMsg {
				t.Errorf("Requests Message = %v, want %v", updatedCRR.Status.Message, tt.expectedMsg)
			}

			if len(tt.expectedFinalizers) > 0 {
				if !reflect.DeepEqual(updatedCRR.Finalizers, tt.expectedFinalizers) {
					t.Errorf("Finalizers = %v, want %v", updatedCRR.Finalizers, tt.expectedFinalizers)
				}
			}
		})
	}
}
