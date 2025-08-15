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
	"testing"
	"time"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	testingclock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	utilpodreadiness "github.com/openkruise/kruise/pkg/util/podreadiness"
)

var s = runtime.NewScheme()

func init() {
	_ = scheme.AddToScheme(s)
	_ = appsv1alpha1.AddToScheme(s)
}

func TestReconcileContainerRecreateRequest(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name     string
		testFunc func(t *testing.T, g *gomega.GomegaWithT)
	}{
		{
			name: "NotFound",
			testFunc: func(t *testing.T, g *gomega.GomegaWithT) {
				fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

				r := &ReconcileContainerRecreateRequest{
					Client:              fakeClient,
					clock:               testingclock.NewFakeClock(time.Now()),
					podReadinessControl: &fakePodReadinessControl{},
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "test-crr",
					},
				}

				res, err := r.Reconcile(context.TODO(), req)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(res.Requeue).To(gomega.BeFalse())
				g.Expect(res.RequeueAfter).To(gomega.Equal(time.Duration(0)))
			},
		},
		{
			name: "CompletedCRR",
			testFunc: func(t *testing.T, g *gomega.GomegaWithT) {
				now := metav1.Now()
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-crr",
						Namespace: "default",
						Labels: map[string]string{
							appsv1alpha1.ContainerRecreateRequestActiveKey: "true",
						},
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: "test-pod",
					},
					Status: appsv1alpha1.ContainerRecreateRequestStatus{
						Phase:          appsv1alpha1.ContainerRecreateRequestCompleted,
						CompletionTime: &now,
					},
				}

				fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr).Build()

				r := &ReconcileContainerRecreateRequest{
					Client:              fakeClient,
					clock:               testingclock.NewFakeClock(now.Time),
					podReadinessControl: &fakePodReadinessControl{},
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "test-crr",
					},
				}

				res, err := r.Reconcile(context.TODO(), req)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(res.Requeue).To(gomega.BeFalse())

				updatedCRR := &appsv1alpha1.ContainerRecreateRequest{}
				err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-crr"}, updatedCRR)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(updatedCRR.Labels).NotTo(gomega.HaveKey(appsv1alpha1.ContainerRecreateRequestActiveKey))
			},
		},
		{
			name: "TTLExpired",
			testFunc: func(t *testing.T, g *gomega.GomegaWithT) {
				completionTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
				ttl := int32(3600)
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-crr",
						Namespace: "default",
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName:                 "test-pod",
						TTLSecondsAfterFinished: &ttl,
					},
					Status: appsv1alpha1.ContainerRecreateRequestStatus{
						Phase:          appsv1alpha1.ContainerRecreateRequestCompleted,
						CompletionTime: &completionTime,
					},
				}

				fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr).Build()

				r := &ReconcileContainerRecreateRequest{
					Client:              fakeClient,
					clock:               testingclock.NewFakeClock(time.Now()),
					podReadinessControl: &fakePodReadinessControl{},
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "test-crr",
					},
				}

				res, err := r.Reconcile(context.TODO(), req)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(res.Requeue).To(gomega.BeFalse())

				updatedCRR := &appsv1alpha1.ContainerRecreateRequest{}
				err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-crr"}, updatedCRR)
				g.Expect(err).To(gomega.HaveOccurred())
			},
		},
		{
			name: "PodGone",
			testFunc: func(t *testing.T, g *gomega.GomegaWithT) {
				ttl := int32(7200)
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-crr",
						Namespace: "default",
						Labels: map[string]string{
							appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
						},
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName:                 "test-pod",
						TTLSecondsAfterFinished: &ttl,
					},
					Status: appsv1alpha1.ContainerRecreateRequestStatus{
						Phase: appsv1alpha1.ContainerRecreateRequestPending,
					},
				}

				fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr).Build()

				r := &ReconcileContainerRecreateRequest{
					Client:              fakeClient,
					clock:               testingclock.NewFakeClock(time.Now()),
					podReadinessControl: &fakePodReadinessControl{},
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "test-crr",
					},
				}

				res, err := r.Reconcile(context.TODO(), req)
				if err != nil {
					g.Expect(err.Error()).To(gomega.ContainSubstring("not found"))
				}
				g.Expect(res.Requeue).To(gomega.BeFalse())
			},
		},
		{
			name: "ResponseTimeout",
			testFunc: func(t *testing.T, g *gomega.GomegaWithT) {
				creationTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-crr",
						Namespace:         "default",
						CreationTimestamp: creationTime,
						Labels: map[string]string{
							appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
						},
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: "test-pod",
					},
					Status: appsv1alpha1.ContainerRecreateRequestStatus{
						Phase: "",
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       "test-pod-uid",
					},
				}

				fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr, pod).Build()

				r := &ReconcileContainerRecreateRequest{
					Client:              fakeClient,
					clock:               testingclock.NewFakeClock(time.Now()),
					podReadinessControl: &fakePodReadinessControl{},
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "test-crr",
					},
				}

				res, err := r.Reconcile(context.TODO(), req)
				if err != nil {
					g.Expect(err.Error()).To(gomega.ContainSubstring("not found"))
				}
				g.Expect(res.Requeue).To(gomega.BeFalse())
			},
		},
		{
			name: "ActiveDeadlineExceeded",
			testFunc: func(t *testing.T, g *gomega.GomegaWithT) {
				creationTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
				activeDeadline := int64(3600)
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-crr",
						Namespace:         "default",
						CreationTimestamp: creationTime,
						Labels: map[string]string{
							appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
						},
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName:               "test-pod",
						ActiveDeadlineSeconds: &activeDeadline,
					},
					Status: appsv1alpha1.ContainerRecreateRequestStatus{
						Phase: appsv1alpha1.ContainerRecreateRequestRecreating,
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       "test-pod-uid",
					},
				}

				fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr, pod).Build()

				r := &ReconcileContainerRecreateRequest{
					Client:              fakeClient,
					clock:               testingclock.NewFakeClock(time.Now()),
					podReadinessControl: &fakePodReadinessControl{},
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "test-crr",
					},
				}

				res, err := r.Reconcile(context.TODO(), req)
				if err != nil {
					g.Expect(err.Error()).To(gomega.ContainSubstring("not found"))
				}
				g.Expect(res.Requeue).To(gomega.BeFalse())
			},
		},
		{
			name: "SyncContainerStatuses",
			testFunc: func(t *testing.T, g *gomega.GomegaWithT) {
				now := metav1.Now()
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-crr",
						Namespace:         "default",
						CreationTimestamp: now,
						Labels: map[string]string{
							appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
						},
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: "test-pod",
						Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
							{Name: "container1"},
							{Name: "container2"},
						},
						Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
							FailurePolicy: appsv1alpha1.ContainerRecreateRequestFailurePolicyFail,
						},
					},
					Status: appsv1alpha1.ContainerRecreateRequestStatus{
						Phase: appsv1alpha1.ContainerRecreateRequestRecreating,
					},
				}

				runningTime := metav1.NewTime(now.Add(time.Minute))
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       "test-pod-uid",
					},
					Status: v1.PodStatus{
						ContainerStatuses: []v1.ContainerStatus{
							{
								Name:         "container1",
								Ready:        true,
								RestartCount: 1,
								ContainerID:  "container1-id",
								State: v1.ContainerState{
									Running: &v1.ContainerStateRunning{
										StartedAt: runningTime,
									},
								},
							},
							{
								Name:         "container2",
								Ready:        false,
								RestartCount: 0,
								ContainerID:  "container2-id",
								State: v1.ContainerState{
									Running: &v1.ContainerStateRunning{
										StartedAt: runningTime,
									},
								},
							},
						},
					},
				}

				fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr, pod).Build()

				r := &ReconcileContainerRecreateRequest{
					Client:              fakeClient,
					clock:               testingclock.NewFakeClock(time.Now()),
					podReadinessControl: &fakePodReadinessControl{},
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "test-crr",
					},
				}

				res, err := r.Reconcile(context.TODO(), req)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(res.Requeue).To(gomega.BeFalse())

				updatedCRR := &appsv1alpha1.ContainerRecreateRequest{}
				err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-crr"}, updatedCRR)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(updatedCRR.Annotations).To(gomega.HaveKey(appsv1alpha1.ContainerRecreateRequestSyncContainerStatusesKey))

				syncStatuses := updatedCRR.Annotations[appsv1alpha1.ContainerRecreateRequestSyncContainerStatusesKey]
				g.Expect(syncStatuses).To(gomega.ContainSubstring("container1"))
				g.Expect(syncStatuses).To(gomega.ContainSubstring("container2"))
			},
		},
		{
			name: "AcquirePodNotReady",
			testFunc: func(t *testing.T, g *gomega.GomegaWithT) {
				now := metav1.Now()
				unreadyGrace := int64(30)
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-crr",
						Namespace:         "default",
						CreationTimestamp: now,
						Labels: map[string]string{
							appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
						},
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: "test-pod",
						Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
							UnreadyGracePeriodSeconds: &unreadyGrace,
						},
					},
					Status: appsv1alpha1.ContainerRecreateRequestStatus{
						Phase: appsv1alpha1.ContainerRecreateRequestRecreating,
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       "test-pod-uid",
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{
							{ConditionType: appspub.KruisePodReadyConditionType},
						},
					},
				}

				fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr, pod).Build()

				fakePodReadiness := &fakePodReadinessControl{
					containsReadinessGate: true,
				}

				r := &ReconcileContainerRecreateRequest{
					Client:              fakeClient,
					clock:               testingclock.NewFakeClock(time.Now()),
					podReadinessControl: fakePodReadiness,
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "test-crr",
					},
				}

				res, err := r.Reconcile(context.TODO(), req)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(res.Requeue).To(gomega.BeFalse())

				updatedCRR := &appsv1alpha1.ContainerRecreateRequest{}
				err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-crr"}, updatedCRR)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(updatedCRR.Finalizers).To(gomega.ContainElement(appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey))
				g.Expect(updatedCRR.Annotations).To(gomega.HaveKey(appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey))
				g.Expect(fakePodReadiness.addNotReadyKeyCalled).To(gomega.BeTrue())
			},
		},
		{
			name: "ReleasePodNotReady",
			testFunc: func(t *testing.T, g *gomega.GomegaWithT) {
				now := metav1.Now()
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-crr",
						Namespace:         "default",
						CreationTimestamp: now,
						Finalizers:        []string{appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey},
						Labels: map[string]string{
							appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
						},
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: "test-pod",
					},
					Status: appsv1alpha1.ContainerRecreateRequestStatus{
						Phase:          appsv1alpha1.ContainerRecreateRequestCompleted,
						CompletionTime: &now,
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       "test-pod-uid",
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{
							{ConditionType: appspub.KruisePodReadyConditionType},
						},
					},
				}

				fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr, pod).Build()

				fakePodReadiness := &fakePodReadinessControl{
					containsReadinessGate: true,
				}

				r := &ReconcileContainerRecreateRequest{
					Client:              fakeClient,
					clock:               testingclock.NewFakeClock(time.Now()),
					podReadinessControl: fakePodReadiness,
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "test-crr",
					},
				}

				res, err := r.Reconcile(context.TODO(), req)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(res.Requeue).To(gomega.BeFalse())

				updatedCRR := &appsv1alpha1.ContainerRecreateRequest{}
				err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-crr"}, updatedCRR)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(updatedCRR.Finalizers).NotTo(gomega.ContainElement(appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey))
				g.Expect(fakePodReadiness.removeNotReadyKeyCalled).To(gomega.BeTrue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t, g)
		})
	}
}

type fakePodReadinessControl struct {
	containsReadinessGate   bool
	addNotReadyKeyCalled    bool
	removeNotReadyKeyCalled bool
}

func (f *fakePodReadinessControl) ContainsReadinessGate(pod *v1.Pod) bool {
	return f.containsReadinessGate
}

func (f *fakePodReadinessControl) AddNotReadyKey(pod *v1.Pod, msg utilpodreadiness.Message) error {
	f.addNotReadyKeyCalled = true
	return nil
}

func (f *fakePodReadinessControl) RemoveNotReadyKey(pod *v1.Pod, msg utilpodreadiness.Message) error {
	f.removeNotReadyKeyCalled = true
	return nil
}

func (f *fakePodReadinessControl) RemoveNotReadyKeyWithoutError(pod *v1.Pod, msg utilpodreadiness.Message) {
}
