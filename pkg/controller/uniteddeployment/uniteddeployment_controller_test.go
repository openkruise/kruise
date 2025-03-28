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
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
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
	mgr, err := manager.New(cfg, manager.Options{
		Controller: config.Controller{
			SkipNameValidation: &[]bool{true}[0],
		},
	})
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

func TestDefaultAdaptiveStrategy(t *testing.T) {
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
				subset.Status.UnschedulableStatus.UnavailablePods = 0
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
			processSubsetsForDefaultAdaptiveStrategy(subset.Name, subset, ud)
			cost := time.Now().Sub(start)
			if subset.Status.UnschedulableStatus.UnavailablePods != c.expectPendingPods {
				t.Logf("case %s failed: expect pending pods %d, but got %d", c.name, c.expectPendingPods, subset.Status.UnschedulableStatus.UnavailablePods)
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

func TestProcessSubsetForTemporaryAdaptiveStrategy(t *testing.T) {
	now := time.Now()
	subsetName := "subset-1"
	modifyPod := func(pod *corev1.Pod, readyTime time.Time, creationTime time.Time, pending bool, patch string) {
		pod.Status.Conditions = []corev1.PodCondition{}
		if pending {
			pod.Status.Phase = corev1.PodPending
			pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
				Type:   corev1.PodScheduled,
				Status: corev1.ConditionFalse,
				Reason: corev1.PodReasonUnschedulable,
			}, corev1.PodCondition{
				Type:               corev1.PodReady,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: metav1.NewTime(readyTime),
			})
		} else {
			pod.Status.Phase = corev1.PodRunning
			pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
				Type:   corev1.PodScheduled,
				Status: corev1.ConditionTrue,
			}, corev1.PodCondition{
				Type:               corev1.PodReady,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(readyTime),
			})
		}
		pod.CreationTimestamp = metav1.NewTime(creationTime)
		if patch != "" {
			pod.Labels[appsv1alpha1.ReservedPodLabelKey] = patch
		}
	}
	baseEnvFactory := func() (*Subset, *appsv1alpha1.UnitedDeployment, *corev1.Pod) {
		pod := &corev1.Pod{}
		pod.Labels = make(map[string]string)
		subset := &Subset{
			ObjectMeta: metav1.ObjectMeta{
				Name: subsetName,
			},
			Spec: SubsetSpec{
				SubsetPods: []*corev1.Pod{pod},
			},
		}
		return subset, &appsv1alpha1.UnitedDeployment{
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
						Adaptive: &appsv1alpha1.AdaptiveUnitedDeploymentStrategy{
							ReserveUnschedulablePods:  true,
							RescheduleCriticalSeconds: ptr.To[int32](10),
							UnschedulableLastSeconds:  ptr.To[int32](30),
						},
					},
				},
			},
		}, pod
	}

	cases := []struct {
		name              string
		envFactory        func() (*Subset, *appsv1alpha1.UnitedDeployment)
		requeueAfter      time.Duration
		expectStagingPods int32
		unschedulable     bool
		podsToPatch       int
	}{
		// RescheduleCriticalSeconds = 10s
		// UnschedulableLastSeconds = 30s
		{
			name: "Pod just created, pending",
			envFactory: func() (*Subset, *appsv1alpha1.UnitedDeployment) {
				subset, ud, pod := baseEnvFactory()
				// create -> 1s check
				modifyPod(pod, now, now.Add(-1*time.Second), true, "")
				return subset, ud
			},
			expectStagingPods: 0,
			unschedulable:     false,
			requeueAfter:      9 * time.Second,
			podsToPatch:       1, // should patch to add the reserved
		},
		{
			name: "Pod created, running within RescheduleCriticalSeconds",
			envFactory: func() (*Subset, *appsv1alpha1.UnitedDeployment) {
				subset, ud, pod := baseEnvFactory()
				// create -> 5s ready -> 3s check
				modifyPod(pod, now.Add(-5*time.Second), now.Add(-8*time.Second), false, "false")
				return subset, ud
			},
			expectStagingPods: 0,
			unschedulable:     false,
			requeueAfter:      0,
			podsToPatch:       0,
		},
		{
			name: "Pod created, pending until timeout",
			envFactory: func() (*Subset, *appsv1alpha1.UnitedDeployment) {
				subset, ud, pod := baseEnvFactory()
				// create -> 13s check
				modifyPod(pod, now.Add(-13*time.Second), now.Add(-13*time.Second), true, "false")
				return subset, ud
			},
			expectStagingPods: 1,
			unschedulable:     true,
			requeueAfter:      30 * time.Second,
			podsToPatch:       1,
		},
		{
			name: "Pod recovered, but not long enough",
			envFactory: func() (*Subset, *appsv1alpha1.UnitedDeployment) {
				subset, ud, pod := baseEnvFactory()
				// create -> 15s check as unavailable -> 10s running, check
				modifyPod(pod, now.Add(-10*time.Second), now.Add(-25*time.Second), false, "true")
				return subset, ud
			},
			expectStagingPods: 1,
			unschedulable:     true,
			requeueAfter:      20 * time.Second,
			podsToPatch:       0,
		},
		{
			name: "Pod recovered, long enough",
			envFactory: func() (*Subset, *appsv1alpha1.UnitedDeployment) {
				subset, ud, pod := baseEnvFactory()
				// create -> 15s check as unavailable -> 35s running
				modifyPod(pod, now.Add(-35*time.Second), now.Add(-50*time.Second), false, "true")
				return subset, ud
			},
			expectStagingPods: 0,
			unschedulable:     false,
			requeueAfter:      0,
			podsToPatch:       1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			subset, ud := c.envFactory()
			podsToPatch := processSubsetsForTemporaryAdaptiveStrategy(subset.Name, subset, ud, now)
			if len(podsToPatch) != c.podsToPatch {
				t.Logf("case %s failed: expect pods to patch %d, but got %d", c.name, c.podsToPatch, len(podsToPatch))
				t.Fail()
			}
			if subset.Status.UnschedulableStatus.UnavailablePods != c.expectStagingPods {
				t.Logf("case %s failed: expect unavailable pods %d, but got %d", c.name, c.expectStagingPods, subset.Status.UnschedulableStatus.UnavailablePods)
				t.Fail()
			}
			status := ud.Status.GetSubsetStatus(subsetName)
			if status == nil {
				t.Logf("case %s failed: SubsetStatus not found", c.name)
				t.Fail()
			}
			condition := status.GetCondition(appsv1alpha1.UnitedDeploymentSubsetSchedulable)
			if condition == nil {
				t.Logf("case %s failed: Condition not found", c.name)
				t.Fail()
			}
			if condition.Status != corev1.ConditionTrue && !c.unschedulable {
				t.Logf("case %s failed: expect unschedulable false, but got true", c.name)
				t.Fail()
			}
			if condition.Status != corev1.ConditionFalse && c.unschedulable {
				t.Logf("case %s failed: expect unschedulable true, but got false", c.name)
				t.Fail()
			}
			if after := durationStore.Pop(getUnitedDeploymentKey(ud)); after != c.requeueAfter {
				t.Logf("case %s failed: expect requeueAfter %v, but got %v", c.name, c.requeueAfter, after)
				t.Fail()
			}
		})
	}
}

func TestPatchStagingChangedPods(t *testing.T) {
	basicPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    make(map[string]string),
		},
	}
	tests := []struct {
		name          string
		reserved      bool
		expectedLabel string
	}{
		{
			name:          "reserved true",
			reserved:      true,
			expectedLabel: "true",
		},
		{
			name:          "reserved false",
			reserved:      false,
			expectedLabel: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := basicPod.DeepCopy()
			cli := fake.NewClientBuilder().WithObjects(pod).Build()
			r := ReconcileUnitedDeployment{
				Client: cli,
			}
			_ = r.patchReservedStatusChangedPods([]podToPatchReservedLabel{
				{pod, tt.reserved},
			})
			_ = cli.Get(context.Background(), client.ObjectKeyFromObject(pod), pod)
			if pod.Labels[appsv1alpha1.ReservedPodLabelKey] != tt.expectedLabel {
				t.Errorf("expected reserved %s, but got %s", tt.expectedLabel, pod.Labels[appsv1alpha1.ReservedPodLabelKey])
			}
		})
	}
}

// TestRescheduleTemporarily tests what happened after TestGetTemporaryAdaptiveNext
// The cases of two tests must be aligned.
func TestRescheduleTemporarily(t *testing.T) {
	tests := []struct {
		name        string
		replicas    int32   // total replicas of UnitedDeployment
		next        []int32 // allocated replicas calculated this time
		cur         []int32 // last allocated results, equals to last expect
		unavailable []int32 // current unavailable pod nums of each subset
		newPods     []int32 // newly created and pending pods caused by replica allocation
		expect      []int32 // final allocated result after reschedule
	}{
		{
			name:        "4 subsets, 2 each, start",
			replicas:    8,
			next:        []int32{2, 2, 2, 2},
			cur:         []int32{0, 0, 0, 0},
			unavailable: []int32{0, 0, 0, 0},
			expect:      []int32{2, 2, 2, 2},
		},
		{
			name:        "4 subsets, subset 1 and 2 unschedulable detected",
			replicas:    8,
			next:        []int32{2, 0, 0, 6},
			cur:         []int32{2, 2, 2, 2},
			unavailable: []int32{0, 2, 2, 0},
			expect:      []int32{2, 2, 2, 6},
		},
		{
			name:        "4 subsets, subset 1 and 2 starts each 1 pods",
			replicas:    8,
			next:        []int32{2, 1, 1, 4},
			cur:         []int32{2, 2, 2, 6},
			unavailable: []int32{0, 1, 1, 0},
			expect:      []int32{2, 2, 2, 4},
		},
		{
			name:        "4 subsets, subset 1 recovered",
			replicas:    8,
			next:        []int32{2, 2, 1, 3},
			cur:         []int32{2, 2, 2, 4},
			unavailable: []int32{0, 0, 1, 0},
			expect:      []int32{2, 2, 2, 3},
		},
		{
			name:        "4 subsets, all subset recovered",
			replicas:    8,
			next:        []int32{2, 2, 2, 2},
			cur:         []int32{2, 2, 2, 3},
			unavailable: []int32{0, 0, 0, 0},
			expect:      []int32{2, 2, 2, 2},
		},
		{
			name:        "4 subsets, part of subset 1 and 2 started, scaled to 16 replicas",
			replicas:    16,
			next:        []int32{4, 1, 1, 10},
			cur:         []int32{2, 2, 2, 6},
			unavailable: []int32{0, 1, 1, 0},
			expect:      []int32{4, 2, 2, 10},
		},
		{
			name:        "4 subsets, part of subset 1 and 2 started, scaled back to 8 replicas",
			replicas:    8,
			next:        []int32{2, 1, 1, 4},
			cur:         []int32{4, 2, 2, 10},
			unavailable: []int32{0, 1, 1, 0},
			expect:      []int32{2, 2, 2, 4},
		},
		{
			name:        "4 subsets, all of subset 1 and 2 started, already scaled to 16 replicas",
			replicas:    16,
			next:        []int32{4, 4, 4, 4},
			cur:         []int32{4, 2, 2, 10},
			unavailable: []int32{0, 0, 0, 0},
			expect:      []int32{4, 4, 4, 8}, // 4 temp running pods retains for 4 new pods not created
		},
		{
			name:        "4 subsets, all of subset 1 and 2 started, already scaled to 16 replicas, 4 new pods just created",
			replicas:    16,
			next:        []int32{4, 4, 4, 4},
			cur:         []int32{4, 4, 4, 8},
			unavailable: []int32{0, 0, 0, 0},
			newPods:     []int32{0, 2, 2, 0},
			expect:      []int32{4, 4, 4, 8}, // 4 temp running pods retains for 4 new pods not started
		},
		{
			name:        "4 subsets, all of subset 1 and 2 started, already scaled to 16 replicas, 4 new pods just created",
			replicas:    16,
			next:        []int32{4, 4, 4, 4},
			cur:         []int32{4, 4, 4, 8},
			unavailable: []int32{0, 0, 0, 0},
			newPods:     []int32{0, 0, 0, 0},
			expect:      []int32{4, 4, 4, 4}, // 4 temp running pods retains for 4 new pods not started
		},
		{
			name:        "3 infinity subsets, start",
			replicas:    2,
			next:        []int32{2, 0, 0},
			cur:         []int32{0, 0, 0},
			unavailable: []int32{0, 0, 0},
			expect:      []int32{2, 0, 0},
		},
		{
			name:        "3 infinity subsets, found subset-0 unschedulable",
			replicas:    2,
			next:        []int32{0, 2, 0},
			cur:         []int32{2, 0, 0},
			unavailable: []int32{2, 0, 0},
			expect:      []int32{2, 2, 0},
		},
		{
			name:        "3 infinity subsets, found subset-1 unschedulable",
			replicas:    2,
			next:        []int32{0, 0, 2},
			cur:         []int32{2, 2, 0},
			unavailable: []int32{2, 2, 0},
			expect:      []int32{2, 2, 2},
		},
		{
			name:        "3 infinity subsets, one of subset-1 started",
			replicas:    2,
			next:        []int32{0, 1, 1},
			cur:         []int32{2, 2, 2},
			unavailable: []int32{2, 1, 0},
			expect:      []int32{2, 2, 1},
		},
		{
			name:        "3 infinity subsets, subset-0 recovered",
			replicas:    2,
			next:        []int32{2, 0, 0},
			cur:         []int32{2, 2, 1},
			unavailable: []int32{0, 1, 0},
			expect:      []int32{2, 0, 0},
		},
		{
			name:        "3 infinity subsets, too many pods running",
			replicas:    2,
			next:        []int32{2, 0, 0},
			cur:         []int32{2, 2, 0},
			unavailable: []int32{0, 0, 0},
			expect:      []int32{2, 0, 0},
		},
		{
			name:        "3 infinity subsets, scaled from 2 to 4 and subset-1 recovered",
			replicas:    4,
			cur:         []int32{2, 2, 4},
			next:        []int32{4, 0, 0},
			unavailable: []int32{0, 2, 0},
			expect:      []int32{4, 2, 2},
		},
		{
			name:        "3 infinity subsets, scaled from 2 to 4 and subset-1 recovered, two new pods just created",
			replicas:    4,
			cur:         []int32{4, 2, 2},
			next:        []int32{4, 0, 0},
			unavailable: []int32{0, 2, 0},
			newPods:     []int32{2, 0, 0},
			expect:      []int32{4, 0, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextReplicas := make(map[string]int32)
			for i := 0; i < len(tt.next); i++ {
				nextReplicas[fmt.Sprintf("subset-%d", i)] = tt.next[i]
			}
			if tt.newPods == nil {
				tt.newPods = make([]int32, len(tt.next))
			}
			existingSubsets := make(map[string]*Subset)
			for i := 0; i < len(tt.cur); i++ {
				existingSubsets[fmt.Sprintf("subset-%d", i)] = &Subset{
					Status: SubsetStatus{
						UnschedulableStatus: SubsetUnschedulableStatus{
							UnavailablePods: tt.unavailable[i],
							Unschedulable:   tt.unavailable[i] != 0,
						},
						UpdatedReadyReplicas: tt.cur[i] - tt.unavailable[i] - tt.newPods[i],
						ReadyReplicas:        tt.cur[i] - tt.unavailable[i] - tt.newPods[i],
					},
					Spec: SubsetSpec{
						Replicas:   tt.cur[i],
						SubsetPods: generateSubsetPods(tt.cur[i], tt.unavailable[i], i),
					},
				}
			}
			subsets := make([]appsv1alpha1.Subset, len(tt.next))
			for i := range tt.next {
				subsets[i] = appsv1alpha1.Subset{
					Name: fmt.Sprintf("subset-%d", i),
				}
			}
			result := rescheduleTemporarily(&nextReplicas, &existingSubsets, tt.replicas, subsets)
			actual := make([]int32, len(tt.expect))
			for i := 0; i < len(tt.expect); i++ {
				actual[i] = (*result)[fmt.Sprintf("subset-%d", i)]
			}
			if !reflect.DeepEqual(actual, tt.expect) {
				t.Errorf("expected %v, but got %v", tt.expect, actual)
			}
		})
	}
}
