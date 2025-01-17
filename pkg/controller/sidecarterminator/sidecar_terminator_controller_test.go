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
	"encoding/json"
	"reflect"
	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

const (
	ExitQuicklyImage = "exit-quickly:latest"
)

var (
	scheme                       *runtime.Scheme
	succeededMainContainerStatus = corev1.ContainerStatus{
		Name: "main",
		State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: int32(0),
			},
		},
	}

	failedMainContainerStatus = corev1.ContainerStatus{
		Name: "main",
		State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: int32(137),
			},
		},
	}

	uncompletedMainContainerStatus = corev1.ContainerStatus{
		Name: "main",
		State: corev1.ContainerState{
			Terminated: nil,
		},
	}

	completedSidecarContainerStatus = corev1.ContainerStatus{
		Name: "sidecar",
		State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: int32(0),
			},
		},
	}

	failedSidecarContainerStatus = corev1.ContainerStatus{
		Name: "sidecar",
		State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: int32(137),
			},
		},
	}
	uncompletedSidecarContainerStatus = corev1.ContainerStatus{
		Name: "sidecar",
		State: corev1.ContainerState{
			Terminated: nil,
		},
	}
	runningSidecarContainerStatus = corev1.ContainerStatus{
		Name: "sidecar",
		State: corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{
				StartedAt: metav1.Now(),
			},
		},
	}

	podDemo = &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-sidecar-terminator",
			UID:       "87076677",
			Name:      "job-generate-rand-str",
		},
		Spec: corev1.PodSpec{
			NodeName:      "normal-node",
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "main-container:latest",
				},
				{
					Name:  "sidecar",
					Image: "sidecar-container-images",
					Env: []corev1.EnvVar{
						{
							Name:  appsv1alpha1.KruiseTerminateSidecarEnv,
							Value: "true",
						},
						{
							Name:  appsv1alpha1.KruiseTerminateSidecarWithImageEnv,
							Value: ExitQuicklyImage,
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				succeededMainContainerStatus,
				uncompletedSidecarContainerStatus,
			},
		},
	}

	crrDemo = &appsv1alpha1.ContainerRecreateRequest{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ContainerRecreateRequest",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       podDemo.Namespace,
			Name:            getCRRName(podDemo),
			ResourceVersion: "1",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(podDemo, podDemo.GroupVersionKind()),
			},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName: podDemo.Name,
			Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
				{Name: "sidecar"},
			},
			Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
				ForceRecreate: true,
				FailurePolicy: appsv1alpha1.ContainerRecreateRequestFailurePolicyIgnore,
			},
		},
	}

	normalNode = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "normal-node",
		},
	}
	vkNode = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vk-node",
			Labels: map[string]string{
				DefaultVKLabelKey: DefaultVKLabelValue,
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{
					Effect: corev1.TaintEffectNoSchedule,
					Key:    DefaultVKTaintKey,
					Value:  "cloudProvider",
				},
			},
		},
	}
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
}

func sidecarContainerFactory(name string, strategy string) corev1.Container {
	return corev1.Container{
		Name:  name,
		Image: "sidecar-container-images",
		Env: []corev1.EnvVar{
			{
				Name:  appsv1alpha1.KruiseTerminateSidecarEnv,
				Value: strategy,
			},
			{
				Name:  appsv1alpha1.KruiseTerminateSidecarWithImageEnv,
				Value: ExitQuicklyImage,
			},
		},
	}
}

func TestKruiseDaemonStrategy(t *testing.T) {
	cases := []struct {
		name        string
		getIn       func() *corev1.Pod
		getCRR      func() *appsv1alpha1.ContainerRecreateRequest
		expectedPod func() *corev1.Pod
	}{
		{
			name: "normal pod with sidecar, restartPolicy=Never, main containers have not been completed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Status.ContainerStatuses[0] = uncompletedMainContainerStatus
				podIn.Status.ContainerStatuses[1] = runningSidecarContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return nil
			},
			expectedPod: func() *corev1.Pod {
				return podDemo.DeepCopy()
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=Never, main containers failed and sidecar running",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Status.ContainerStatuses[0] = failedMainContainerStatus
				podIn.Status.ContainerStatuses[1] = runningSidecarContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return crrDemo.DeepCopy()
			},
			expectedPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Status.Phase = corev1.PodFailed
				return pod
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=Never, main containers failed and sidecar running",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Status.ContainerStatuses[0] = failedMainContainerStatus
				podIn.Status.ContainerStatuses[1] = runningSidecarContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return crrDemo.DeepCopy()
			},
			expectedPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Status.Phase = corev1.PodFailed
				return pod
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=Never, main containers failed and sidecar failed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Status.ContainerStatuses[0] = failedMainContainerStatus
				podIn.Status.ContainerStatuses[1] = failedSidecarContainerStatus
				podIn.Status.Phase = corev1.PodFailed //todo
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return nil
			},
			expectedPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Status.Phase = corev1.PodFailed
				return pod
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=Never, main containers succeeded and sidecar running",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Status.ContainerStatuses[0] = succeededMainContainerStatus
				podIn.Status.ContainerStatuses[1] = runningSidecarContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return crrDemo.DeepCopy()
			},
			expectedPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Status.Phase = corev1.PodSucceeded
				return pod
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=OnFailure, main containers have not been completed and sidecar running",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses[0] = uncompletedMainContainerStatus
				podIn.Status.ContainerStatuses[1] = runningSidecarContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return nil
			},
			expectedPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=OnFailure, main containers failed and sidecar succeeded",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses[0] = failedMainContainerStatus
				podIn.Status.ContainerStatuses[1] = completedSidecarContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return nil
			},
			expectedPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=OnFailure, main containers succeeded and sidecar succeeded",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.Phase = corev1.PodSucceeded
				podIn.Status.ContainerStatuses[0] = succeededMainContainerStatus
				podIn.Status.ContainerStatuses[1] = completedSidecarContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return nil
			},
			expectedPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Status.Phase = corev1.PodSucceeded
				return pod
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=OnFailure, 2 succeeded main containers, 2 sidecars uncompleted",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.Containers = []corev1.Container{
					mainContainerFactory("main-1"),
					mainContainerFactory("main-2"),
					sidecarContainerFactory("sidecar-1", "true"),
					sidecarContainerFactory("sidecar-2", "true"),
				}
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses = []corev1.ContainerStatus{
					rename(succeededMainContainerStatus.DeepCopy(), "main-1"),
					rename(succeededMainContainerStatus.DeepCopy(), "main-2"),
					rename(uncompletedSidecarContainerStatus.DeepCopy(), "sidecar-1"),
					rename(uncompletedSidecarContainerStatus.DeepCopy(), "sidecar-2"),
				}
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				crr := crrDemo.DeepCopy()
				crr.Spec.Containers = []appsv1alpha1.ContainerRecreateRequestContainer{
					{Name: "sidecar-1"}, {Name: "sidecar-2"},
				}
				return crr
			},
			expectedPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Status.Phase = corev1.PodSucceeded
				return pod
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=OnFailure, 2 succeeded main containers, 2 sidecars running",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.Containers = []corev1.Container{
					mainContainerFactory("main-1"),
					mainContainerFactory("main-2"),
					sidecarContainerFactory("sidecar-1", "true"),
					sidecarContainerFactory("sidecar-2", "true"),
				}
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses = []corev1.ContainerStatus{
					rename(succeededMainContainerStatus.DeepCopy(), "main-1"),
					rename(succeededMainContainerStatus.DeepCopy(), "main-2"),
					rename(runningSidecarContainerStatus.DeepCopy(), "sidecar-1"),
					rename(runningSidecarContainerStatus.DeepCopy(), "sidecar-2"),
				}
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				crr := crrDemo.DeepCopy()
				crr.Spec.Containers = []appsv1alpha1.ContainerRecreateRequestContainer{
					{Name: "sidecar-1"}, {Name: "sidecar-2"},
				}
				return crr
			},
			expectedPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Status.Phase = corev1.PodSucceeded
				return pod
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=OnFailure, 2 succeeded main containers, 2 sidecars but 1 completed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.Containers = []corev1.Container{
					mainContainerFactory("main-1"),
					mainContainerFactory("main-2"),
					sidecarContainerFactory("sidecar-1", "true"),
					sidecarContainerFactory("sidecar-2", "true"),
				}
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses = []corev1.ContainerStatus{
					rename(succeededMainContainerStatus.DeepCopy(), "main-1"),
					rename(succeededMainContainerStatus.DeepCopy(), "main-2"),
					rename(completedSidecarContainerStatus.DeepCopy(), "sidecar-1"),
					rename(uncompletedSidecarContainerStatus.DeepCopy(), "sidecar-2"),
				}
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				crr := crrDemo.DeepCopy()
				crr.Spec.Containers = []appsv1alpha1.ContainerRecreateRequestContainer{
					{Name: "sidecar-2"},
				}
				return crr
			},
			expectedPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Status.Phase = corev1.PodSucceeded
				return pod
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=OnFailure, 2 main containers but 1 uncompleted, 2 sidecars but 1 completed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.Containers = []corev1.Container{
					mainContainerFactory("main-1"),
					mainContainerFactory("main-2"),
					sidecarContainerFactory("sidecar-1", "true"),
					sidecarContainerFactory("sidecar-2", "true"),
				}
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses = []corev1.ContainerStatus{
					rename(succeededMainContainerStatus.DeepCopy(), "main-1"),
					rename(uncompletedMainContainerStatus.DeepCopy(), "main-2"),
					rename(completedSidecarContainerStatus.DeepCopy(), "sidecar-1"),
					rename(uncompletedSidecarContainerStatus.DeepCopy(), "sidecar-2"),
				}
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return nil
			},
			expectedPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(normalNode, vkNode, cs.getIn()).Build()
			fakeRecord := record.NewFakeRecorder(100)
			r := ReconcileSidecarTerminator{
				Client:   fakeClient,
				recorder: fakeRecord,
			}

			_, err := r.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      podDemo.Name,
					Namespace: podDemo.Namespace,
				},
			})
			if err != nil {
				t.Fatalf("Failed to reconcile, error: %v", err)
			}

			realCRR := &appsv1alpha1.ContainerRecreateRequest{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{
				Name:      getCRRName(podDemo),
				Namespace: podDemo.Namespace,
			}, realCRR)
			expectCRR := cs.getCRR()

			realCRR.TypeMeta.APIVersion = appsv1alpha1.SchemeGroupVersion.String()
			realCRR.TypeMeta.Kind = "ContainerRecreateRequest"

			realBy, _ := json.Marshal(realCRR)
			expectBy, _ := json.Marshal(expectCRR)

			if !(expectCRR == nil && errors.IsNotFound(err) || reflect.DeepEqual(realBy, expectBy)) {
				t.Fatal("Get unexpected CRR")
			}

			pod := &corev1.Pod{}
			err = fakeClient.Get(context.TODO(), client.ObjectKey{Namespace: podDemo.Namespace, Name: podDemo.Name}, pod)
			if err != nil {
				t.Fatalf("Get pod error: %v", err)
			}
			expectPod := cs.expectedPod()
			if pod.Status.Phase != expectPod.Status.Phase {
				t.Fatalf("Get an expected pod phase : expectd=%s,got=%s", expectPod.Status.Phase, pod.Status.Phase)
			}

		})
	}
}

func TestInPlaceUpdateStrategy(t *testing.T) {
	cases := []struct {
		name           string
		getIn          func() *corev1.Pod
		expectedNumber int
	}{
		{
			name: "vk pod with sidecar, restartPolicy=Never, main containers have not been completed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.NodeName = vkNode.Name
				podIn.Status.ContainerStatuses[0] = uncompletedMainContainerStatus
				return podIn
			},
			expectedNumber: 0,
		},
		{
			name: "vk pod with sidecar, restartPolicy=Never, main containers failed, Strategy=InPlaceUpdateOnly",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Status.ContainerStatuses[0] = failedMainContainerStatus
				podIn.Spec.Containers[1].Env = []corev1.EnvVar{
					{Name: appsv1alpha1.KruiseTerminateSidecarWithImageEnv, Value: ExitQuicklyImage},
				}
				return podIn
			},
			expectedNumber: 1,
		},
		{
			name: "vk pod with sidecar, restartPolicy=Never, main containers succeeded",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.NodeName = vkNode.Name
				podIn.Status.ContainerStatuses[0] = succeededMainContainerStatus
				return podIn
			},
			expectedNumber: 1,
		},
		{
			name: "vk pod with sidecar, restartPolicy=OnFailure, main containers have not been completed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.NodeName = vkNode.Name
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses[0] = uncompletedMainContainerStatus
				return podIn
			},
			expectedNumber: 0,
		},
		{
			name: "vk pod with sidecar, restartPolicy=OnFailure, main containers failed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.NodeName = vkNode.Name
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses[0] = failedMainContainerStatus
				return podIn
			},
			expectedNumber: 0,
		},
		{
			name: "vk pod with sidecar, restartPolicy=OnFailure, main containers succeeded",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.NodeName = vkNode.Name
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses[0] = succeededMainContainerStatus
				return podIn
			},
			expectedNumber: 1,
		},
		{
			name: "vk pod with sidecar, restartPolicy=OnFailure, 2 succeeded main containers, 2 sidecars",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.NodeName = vkNode.Name
				podIn.Spec.Containers = []corev1.Container{
					mainContainerFactory("main-1"),
					mainContainerFactory("main-2"),
					sidecarContainerFactory("sidecar-1", "true"),
					sidecarContainerFactory("sidecar-2", "true"),
				}
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses = []corev1.ContainerStatus{
					rename(succeededMainContainerStatus.DeepCopy(), "main-1"),
					rename(succeededMainContainerStatus.DeepCopy(), "main-2"),
					rename(uncompletedSidecarContainerStatus.DeepCopy(), "sidecar-1"),
					rename(uncompletedSidecarContainerStatus.DeepCopy(), "sidecar-2"),
				}
				return podIn
			},
			expectedNumber: 2,
		},
		{
			name: "vk pod with sidecar, restartPolicy=OnFailure, 2 succeeded main containers, 2 sidecars but 1 completed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.NodeName = vkNode.Name
				podIn.Spec.Containers = []corev1.Container{
					mainContainerFactory("main-1"),
					mainContainerFactory("main-2"),
					sidecarContainerFactory("sidecar-1", "true"),
					sidecarContainerFactory("sidecar-2", "true"),
				}
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses = []corev1.ContainerStatus{
					rename(succeededMainContainerStatus.DeepCopy(), "main-1"),
					rename(succeededMainContainerStatus.DeepCopy(), "main-2"),
					rename(completedSidecarContainerStatus.DeepCopy(), "sidecar-1"),
					rename(uncompletedSidecarContainerStatus.DeepCopy(), "sidecar-2"),
				}
				return podIn
			},
			expectedNumber: 1,
		},
		{
			name: "vk pod with sidecar, restartPolicy=OnFailure, 2 main containers but 1 uncompleted, 2 sidecars but 1 completed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.Containers = []corev1.Container{
					mainContainerFactory("main-1"),
					mainContainerFactory("main-2"),
					sidecarContainerFactory("sidecar-1", "true"),
					sidecarContainerFactory("sidecar-2", "true"),
				}
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses = []corev1.ContainerStatus{
					rename(succeededMainContainerStatus.DeepCopy(), "main-1"),
					rename(uncompletedMainContainerStatus.DeepCopy(), "main-2"),
					rename(completedSidecarContainerStatus.DeepCopy(), "sidecar-1"),
					rename(uncompletedSidecarContainerStatus.DeepCopy(), "sidecar-2"),
				}
				return podIn
			},
			expectedNumber: 0,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(normalNode, vkNode, cs.getIn()).Build()
			fakeRecord := record.NewFakeRecorder(100)
			r := ReconcileSidecarTerminator{
				Client:   fakeClient,
				recorder: fakeRecord,
			}

			_, err := r.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      podDemo.Name,
					Namespace: podDemo.Namespace,
				},
			})
			if err != nil {
				t.Fatalf("Failed to reconcile, error: %v", err)
			}

			pod := &corev1.Pod{}
			if err = fakeClient.Get(context.TODO(), types.NamespacedName{
				Name:      podDemo.Name,
				Namespace: podDemo.Namespace,
			}, pod); err != nil {
				t.Fatalf("Failed to get pod, error %v", err)
			}

			actualNumber := 0
			for i := range pod.Spec.Containers {
				if pod.Spec.Containers[i].Image == ExitQuicklyImage {
					actualNumber++
				}
			}

			if cs.expectedNumber != actualNumber {
				t.Fatalf("get unexpected new sidecar image number, expected %v, actual %v", cs.expectedNumber, actualNumber)
			}

			crr := &appsv1alpha1.ContainerRecreateRequest{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{
				Name:      getCRRName(podDemo),
				Namespace: podDemo.Namespace,
			}, crr)
			if err == nil || !errors.IsNotFound(err) {
				t.Fatal("Get unexpected CRR")
			}
		})
	}
}

func mainContainerFactory(name string) corev1.Container {
	return corev1.Container{
		Name:  name,
		Image: "main-container:latest",
	}
}

func rename(status *corev1.ContainerStatus, name string) corev1.ContainerStatus {
	status.Name = name
	return *status
}
