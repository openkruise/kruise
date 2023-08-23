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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

	uncompletedSidecarContainerStatus = corev1.ContainerStatus{
		Name: "sidecar",
		State: corev1.ContainerState{
			Terminated: nil,
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
		name   string
		getIn  func() *corev1.Pod
		getCRR func() *appsv1alpha1.ContainerRecreateRequest
	}{
		{
			name: "normal pod with sidecar, restartPolicy=Never, main containers have not been completed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Status.ContainerStatuses[0] = uncompletedMainContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return nil
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=Never, main containers failed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Status.ContainerStatuses[0] = failedMainContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return crrDemo.DeepCopy()
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=Never, main containers succeeded",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Status.ContainerStatuses[0] = succeededMainContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return crrDemo.DeepCopy()
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=OnFailure, main containers have not been completed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses[0] = uncompletedMainContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return nil
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=OnFailure, main containers failed",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses[0] = failedMainContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return nil
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=OnFailure, main containers succeeded",
			getIn: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				podIn.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
				podIn.Status.ContainerStatuses[0] = succeededMainContainerStatus
				return podIn
			},
			getCRR: func() *appsv1alpha1.ContainerRecreateRequest {
				return crrDemo.DeepCopy()
			},
		},
		{
			name: "normal pod with sidecar, restartPolicy=OnFailure, 2 succeeded main containers, 2 sidecars",
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

			realBy, _ := json.Marshal(realCRR)
			expectBy, _ := json.Marshal(expectCRR)

			if !(expectCRR == nil && errors.IsNotFound(err) || reflect.DeepEqual(realBy, expectBy)) {
				t.Fatal("Get unexpected CRR")
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
