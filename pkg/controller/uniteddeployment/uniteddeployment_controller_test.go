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

package uniteddeployment

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "foo",
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": "foo",
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": "foo",
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &one,
		},
	}

	// Set up the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = utilclient.NewClientFromManager(mgr, "test-uniteddeployment-controller")

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	ctx, cancel := context.WithCancel(context.Background())
	mgrStopped := StartTestManager(ctx, mgr, g)

	defer func() {
		cancel()
		mgrStopped.Wait()
	}()

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
}

func TestUnschedulableStatusManagement(t *testing.T) {
	subsetName := "subset-1"
	baseEnvFactory := func() (*corev1.Pod, *Subset, *appsv1alpha1.UnitedDeployment) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionFalse,
						Reason: corev1.PodReasonUnschedulable,
					},
				},
			},
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.NewTime(time.Now().Add(-15 * time.Second)),
			},
		}
		subset := &Subset{
			ObjectMeta: metav1.ObjectMeta{
				Name: subsetName,
			},
			Status: SubsetStatus{
				ReadyReplicas: 0,
				Replicas:      1,
			},
			Spec: SubsetSpec{
				SubsetPods: []*corev1.Pod{pod},
			},
		}
		return pod, subset, &appsv1alpha1.UnitedDeployment{
			Status: appsv1alpha1.UnitedDeploymentStatus{
				SubsetStatuses: []appsv1alpha1.UnitedDeploymentSubsetStatus{
					{
						Name: subsetName,
						Conditions: []appsv1alpha1.UnitedDeploymentSubsetCondition{
							{
								Type:   appsv1alpha1.UnitedDeploymentSubsetSchedulable,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Topology: appsv1alpha1.Topology{
					ScheduleStrategy: appsv1alpha1.UnitedDeploymentScheduleStrategy{
						Type: appsv1alpha1.AdaptiveUnitedDeploymentScheduleStrategyType,
					},
				},
			},
		}
	}

	cases := []struct {
		name              string
		envFactory        func() (*Subset, *appsv1alpha1.UnitedDeployment)
		expectPendingPods int32
		requeueUpperLimit time.Duration
		requeueLowerLimit time.Duration
		unschedulable     bool
	}{
		{
			name: "Not timeouted yet",
			envFactory: func() (*Subset, *appsv1alpha1.UnitedDeployment) {
				_, subset, ud := baseEnvFactory()
				return subset, ud
			},
			expectPendingPods: 0,
			requeueUpperLimit: appsv1alpha1.DefaultRescheduleCriticalDuration - 15*time.Second + 100*time.Millisecond,
			requeueLowerLimit: appsv1alpha1.DefaultRescheduleCriticalDuration - 15*time.Second - 100*time.Millisecond,
			unschedulable:     false,
		},
		{
			name: "Timeouted",
			envFactory: func() (*Subset, *appsv1alpha1.UnitedDeployment) {
				pod, subset, ud := baseEnvFactory()
				pod.CreationTimestamp = metav1.NewTime(time.Now().Add(-31 * time.Second))
				return subset, ud
			},
			expectPendingPods: 1,
			requeueUpperLimit: appsv1alpha1.DefaultUnschedulableStatusLastDuration,
			requeueLowerLimit: appsv1alpha1.DefaultUnschedulableStatusLastDuration,
			unschedulable:     true,
		},
		{
			name: "During unschedulable status",
			envFactory: func() (*Subset, *appsv1alpha1.UnitedDeployment) {
				_, subset, ud := baseEnvFactory()
				ud.Status.SubsetStatuses = []appsv1alpha1.UnitedDeploymentSubsetStatus{
					{
						Name: subset.Name,
						Conditions: []appsv1alpha1.UnitedDeploymentSubsetCondition{
							{
								Type:               appsv1alpha1.UnitedDeploymentSubsetSchedulable,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: metav1.Time{Time: time.Now().Add(-time.Minute)},
							},
						},
					},
				}
				subset.Status.ReadyReplicas = 1
				subset.Status.UnschedulableStatus.PendingPods = 0
				return subset, ud
			},
			expectPendingPods: 0,
			requeueUpperLimit: appsv1alpha1.DefaultUnschedulableStatusLastDuration - time.Minute + time.Second,
			requeueLowerLimit: appsv1alpha1.DefaultUnschedulableStatusLastDuration - time.Minute - time.Second,
			unschedulable:     true,
		},
		{
			name: "After status reset",
			envFactory: func() (*Subset, *appsv1alpha1.UnitedDeployment) {
				pod, subset, ud := baseEnvFactory()
				ud.Status.SubsetStatuses = []appsv1alpha1.UnitedDeploymentSubsetStatus{
					{
						Name: subset.Name,
						Conditions: []appsv1alpha1.UnitedDeploymentSubsetCondition{
							{
								Type:               appsv1alpha1.UnitedDeploymentSubsetSchedulable,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: metav1.Time{Time: time.Now().Add(-time.Minute - appsv1alpha1.DefaultUnschedulableStatusLastDuration)},
							},
						},
					},
				}
				pod.Status.Conditions = []corev1.PodCondition{
					{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionTrue,
					},
				}
				return subset, ud
			},
			expectPendingPods: 0,
			requeueUpperLimit: 0,
			requeueLowerLimit: 0,
			unschedulable:     false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			subset, ud := c.envFactory()
			start := time.Now()
			manageUnschedulableStatusForExistingSubset(subset.Name, subset, ud)
			cost := time.Now().Sub(start)
			if subset.Status.UnschedulableStatus.PendingPods != c.expectPendingPods {
				t.Logf("case %s failed: expect pending pods %d, but got %d", c.name, c.expectPendingPods, subset.Status.UnschedulableStatus.PendingPods)
				t.Fail()
			}
			requeueAfter := durationStore.Pop(getUnitedDeploymentKey(ud))
			if c.requeueUpperLimit != c.requeueLowerLimit {
				// result is not a const, which means this case will be affected by low execution speed.
				requeueAfter += cost
			} else {
				cost = 0
			}
			t.Logf("got requeueAfter %f not in range [%f, %f] (cost fix %f)",
				requeueAfter.Seconds(), c.requeueLowerLimit.Seconds(), c.requeueUpperLimit.Seconds(), cost.Seconds())
			if requeueAfter > c.requeueUpperLimit || requeueAfter < c.requeueLowerLimit {
				t.Fail()
			}
			if subset.Status.UnschedulableStatus.Unschedulable != c.unschedulable {
				t.Logf("case %s failed: expect unschedulable %v, but got %v", c.name, c.unschedulable, subset.Status.UnschedulableStatus.Unschedulable)
				t.Fail()
			}
		})
	}
}
