/*
Copyright 2023 The Kruise Authors.

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

package sidecarterminator

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestEnqueueRequestForPodUpdate(t *testing.T) {
	oldPodDemo := podDemo.DeepCopy()
	oldPodDemo.SetResourceVersion("1")
	newPodDemo := podDemo.DeepCopy()
	newPodDemo.SetResourceVersion("2")

	cases := []struct {
		name      string
		getOldPod func() *corev1.Pod
		getNewPod func() *corev1.Pod
		expectLen int
	}{
		{
			name: "Pod not running",
			getOldPod: func() *corev1.Pod {
				oldPod := oldPodDemo.DeepCopy()
				return oldPod
			},
			getNewPod: func() *corev1.Pod {
				newPod := newPodDemo.DeepCopy()
				newPod.Status.Phase = corev1.PodPending
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod restartPolicy=Always",
			getOldPod: func() *corev1.Pod {
				oldPod := oldPodDemo.DeepCopy()
				return oldPod
			},
			getNewPod: func() *corev1.Pod {
				newPod := newPodDemo.DeepCopy()
				newPod.Spec.RestartPolicy = corev1.RestartPolicyAlways
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod, main container uncompleted -> completed, sidecar container uncompleted",
			getOldPod: func() *corev1.Pod {
				oldPod := oldPodDemo.DeepCopy()
				oldPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					uncompletedMainContainerStatus,
					uncompletedSidecarContainerStatus,
				}
				return oldPod
			},
			getNewPod: func() *corev1.Pod {
				newPod := newPodDemo.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					failedMainContainerStatus,
					uncompletedSidecarContainerStatus,
				}
				return newPod
			},
			expectLen: 1,
		},
		{
			name: "Pod, main container completed -> completed, sidecar container uncompleted",
			getOldPod: func() *corev1.Pod {
				oldPod := oldPodDemo.DeepCopy()
				oldPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					uncompletedSidecarContainerStatus,
				}
				return oldPod
			},
			getNewPod: func() *corev1.Pod {
				newPod := newPodDemo.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					uncompletedSidecarContainerStatus,
				}
				return newPod
			},
			expectLen: 1,
		},
		{
			name: "Pod, main container uncompleted -> completed, sidecar container completed",
			getOldPod: func() *corev1.Pod {
				oldPod := oldPodDemo.DeepCopy()
				oldPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					uncompletedMainContainerStatus,
					completedSidecarContainerStatus,
				}
				return oldPod
			},
			getNewPod: func() *corev1.Pod {
				newPod := newPodDemo.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					completedSidecarContainerStatus,
				}
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod, main container completed -> completed, sidecar container completed",
			getOldPod: func() *corev1.Pod {
				oldPod := oldPodDemo.DeepCopy()
				oldPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					completedSidecarContainerStatus,
				}
				return oldPod
			},
			getNewPod: func() *corev1.Pod {
				newPod := newPodDemo.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					completedSidecarContainerStatus,
				}
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod, main container completed -> completed, sidecar container completed and pod has reached succeeded phase",
			getOldPod: func() *corev1.Pod {
				oldPod := oldPodDemo.DeepCopy()
				oldPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					completedSidecarContainerStatus,
				}
				return oldPod
			},
			getNewPod: func() *corev1.Pod {
				newPod := newPodDemo.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					completedSidecarContainerStatus,
				}
				newPod.Status.Phase = corev1.PodSucceeded
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod, main container completed -> completed, sidecar container failed and pod has reached succeeded phase",
			getOldPod: func() *corev1.Pod {
				oldPod := oldPodDemo.DeepCopy()
				oldPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					completedSidecarContainerStatus,
				}
				return oldPod
			},
			getNewPod: func() *corev1.Pod {
				newPod := newPodDemo.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					failedSidecarContainerStatus,
				}
				newPod.Status.Phase = corev1.PodSucceeded
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod, main container completed -> uncompleted, sidecar container completed",
			getOldPod: func() *corev1.Pod {
				oldPod := oldPodDemo.DeepCopy()
				oldPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					uncompletedSidecarContainerStatus,
				}
				return oldPod
			},
			getNewPod: func() *corev1.Pod {
				newPod := newPodDemo.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					uncompletedMainContainerStatus,
					completedSidecarContainerStatus,
				}
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod, main container completed -> uncompleted, sidecar container uncompleted",
			getOldPod: func() *corev1.Pod {
				oldPod := oldPodDemo.DeepCopy()
				oldPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					uncompletedSidecarContainerStatus,
				}
				return oldPod
			},
			getNewPod: func() *corev1.Pod {
				newPod := newPodDemo.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					uncompletedMainContainerStatus,
					uncompletedSidecarContainerStatus,
				}
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod, main container uncompleted, sidecar container uncompleted -> completed",
			getOldPod: func() *corev1.Pod {
				oldPod := oldPodDemo.DeepCopy()
				oldPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					uncompletedMainContainerStatus,
					uncompletedSidecarContainerStatus,
				}
				return oldPod
			},
			getNewPod: func() *corev1.Pod {
				newPod := newPodDemo.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					uncompletedMainContainerStatus,
					completedSidecarContainerStatus,
				}
				return newPod
			},
			expectLen: 0,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			eventHandler := &enqueueRequestForPod{}
			evt := event.TypedUpdateEvent[*corev1.Pod]{
				ObjectOld: cs.getOldPod(),
				ObjectNew: cs.getNewPod(),
			}
			que := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			eventHandler.Update(context.TODO(), evt, que)
			if que.Len() != cs.expectLen {
				t.Fatalf("Get unexpected queue length, expected %v, actual %v", cs.expectLen, que.Len())
			}
		})
	}
}

func TestEnqueueRequestForPodCreate(t *testing.T) {
	demoPod := podDemo.DeepCopy()
	demoPod.SetResourceVersion("1")

	cases := []struct {
		name      string
		getPod    func() *corev1.Pod
		expectLen int
	}{
		{
			name: "Pod not running",
			getPod: func() *corev1.Pod {
				newPod := demoPod.DeepCopy()
				newPod.Status.Phase = corev1.PodPending
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod restartPolicy=Always",
			getPod: func() *corev1.Pod {
				newPod := demoPod.DeepCopy()
				newPod.Spec.RestartPolicy = corev1.RestartPolicyAlways
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod, main container completed, sidecar container uncompleted",
			getPod: func() *corev1.Pod {
				newPod := demoPod.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					failedMainContainerStatus,
					uncompletedSidecarContainerStatus,
				}
				return newPod
			},
			expectLen: 1,
		},
		{
			name: "Pod, main container completed, sidecar container completed and pod has reached succeeded phase",
			getPod: func() *corev1.Pod {
				newPod := demoPod.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					completedSidecarContainerStatus,
				}
				newPod.Status.Phase = corev1.PodSucceeded
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod, main container completed, sidecar container failed and pod has reached succeeded phase",
			getPod: func() *corev1.Pod {
				newPod := demoPod.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					succeededMainContainerStatus,
					failedSidecarContainerStatus,
				}
				newPod.Status.Phase = corev1.PodSucceeded
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod, main container uncompleted, sidecar container completed",
			getPod: func() *corev1.Pod {
				newPod := demoPod.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					uncompletedMainContainerStatus,
					completedSidecarContainerStatus,
				}
				return newPod
			},
			expectLen: 0,
		},
		{
			name: "Pod, main container uncompleted, sidecar container uncompleted",
			getPod: func() *corev1.Pod {
				newPod := demoPod.DeepCopy()
				newPod.Status.ContainerStatuses = []corev1.ContainerStatus{
					uncompletedMainContainerStatus,
					uncompletedSidecarContainerStatus,
				}
				return newPod
			},
			expectLen: 0,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			eventHandler := &enqueueRequestForPod{}
			evt := event.TypedCreateEvent[*corev1.Pod]{
				Object: cs.getPod(),
			}
			que := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			eventHandler.Create(context.TODO(), evt, que)
			if que.Len() != cs.expectLen {
				t.Fatalf("Get unexpected queue length, expected %v, actual %v", cs.expectLen, que.Len())
			}
		})
	}
}
