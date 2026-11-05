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

package pubcontrol

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openkruise/kruise/apis/apps/pub"
	policyv1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
)

func TestCanResizeInplace(t *testing.T) {
	cases := []struct {
		name      string
		getOldPod func() *corev1.Pod
		getNewPod func() *corev1.Pod
		expect    bool
	}{
		{
			name: "qos changed",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				}
				return demo
			},
			expect: false,
		},
		{
			name: "resources changed",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				}
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("3"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				}
				return demo
			},
			expect: true,
		},
		{
			name: "storage resources changed 1",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				}
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:     resource.MustParse("2"),
						corev1.ResourceMemory:  resource.MustParse("4Gi"),
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				}
				return demo
			},
			expect: false,
		},
		{
			name: "storage resources changed 2",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:     resource.MustParse("1"),
						corev1.ResourceMemory:  resource.MustParse("2Gi"),
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				}
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				}
				return demo
			},
			expect: false,
		},
		{
			name: "resources changed but static pod",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				}
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				if demo.Annotations == nil {
					demo.Annotations = make(map[string]string)
				}
				demo.Annotations[types.ConfigSourceAnnotationKey] = types.FileSource
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("3"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				}
				return demo
			},
			expect: false,
		},
		{
			name: "resources changed but resizePolicy is restart",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].ResizePolicy = []corev1.ContainerResizePolicy{
					{
						ResourceName:  corev1.ResourceCPU,
						RestartPolicy: corev1.RestartContainer,
					},
				}
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				}
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].ResizePolicy = []corev1.ContainerResizePolicy{
					{
						ResourceName:  corev1.ResourceCPU,
						RestartPolicy: corev1.RestartContainer,
					},
				}
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				}
				return demo
			},
			expect: false,
		},
		{
			name: "resources changed mixed resizePolicy",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].ResizePolicy = []corev1.ContainerResizePolicy{
					{
						ResourceName:  corev1.ResourceCPU,
						RestartPolicy: corev1.RestartContainer,
					},
					{
						ResourceName:  corev1.ResourceMemory,
						RestartPolicy: corev1.NotRequired,
					},
				}
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				}
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].ResizePolicy = []corev1.ContainerResizePolicy{
					{
						ResourceName:  corev1.ResourceCPU,
						RestartPolicy: corev1.RestartContainer,
					},
					{
						ResourceName:  corev1.ResourceMemory,
						RestartPolicy: corev1.NotRequired,
					},
				}
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("5Gi"),
					},
				}
				return demo
			},
			expect: true,
		},
		{
			name: "resources changed but add unavailable labels",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				}
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Labels[fmt.Sprintf("%sdata", pub.PubUnavailablePodLabelPrefix)] = "true"
				demo.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("3"),
						corev1.ResourceMemory: resource.MustParse("3Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				}
				return demo
			},
			expect: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			control := commonControl{}
			is := control.CanResizeInplace(cs.getOldPod(), cs.getNewPod())
			if cs.expect != is {
				t.Fatalf("CanResizeInplace failed")
			}
		})
	}
}

func TestIsPodUnavailableChanged(t *testing.T) {
	cases := []struct {
		name      string
		getOldPod func() *corev1.Pod
		getNewPod func() *corev1.Pod
		getPub    func() *policyv1beta1.PodUnavailableBudget
		expect    bool
	}{
		{
			name: "only annotations change",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Annotations["add"] = "annotations"
				return demo
			},
			getPub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expect: false,
		},
		{
			name: "only annotations change with featureGate enabled",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Annotations["add"] = "annotations"
				return demo
			},
			getPub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expect: false,
		},
		{
			name: "add unavailable label",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Labels[fmt.Sprintf("%sdata", pub.PubUnavailablePodLabelPrefix)] = "true"
				return demo
			},
			getPub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expect: true,
		},
		{
			name: "image changed",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].Image = "nginx:v2"
				return demo
			},
			getPub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expect: true,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs.getPub()).
				WithStatusSubresource(&policyv1beta1.PodUnavailableBudget{}).Build()
			finder := &controllerfinder.ControllerFinder{Client: fakeClient}
			control := commonControl{
				Client:           fakeClient,
				controllerFinder: finder,
			}
			is := control.IsPodUnavailableChanged(cs.getOldPod(), cs.getNewPod())
			if cs.expect != is {
				t.Fatalf("IsPodUnavailableChanged failed")
			}
		})
	}
}

func TestIsResourceChanged(t *testing.T) {
	cases := []struct {
		name               string
		getOldResourceList func() corev1.ResourceList
		getNewResourceList func() corev1.ResourceList
		resourceName       corev1.ResourceName
		expect             bool
	}{
		{
			name: "resource not exist in old",
			getOldResourceList: func() corev1.ResourceList {
				return corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				}
			},
			getNewResourceList: func() corev1.ResourceList {
				return corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				}
			},
			resourceName: corev1.ResourceCPU,
			expect:       true,
		},
		{
			name: "resource not exist in new",
			getOldResourceList: func() corev1.ResourceList {
				return corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				}
			},
			getNewResourceList: func() corev1.ResourceList {
				return corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				}
			},
			resourceName: corev1.ResourceCPU,
			expect:       true,
		},
		{
			name: "resource not exist in new and old",
			getOldResourceList: func() corev1.ResourceList {
				return corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				}
			},
			getNewResourceList: func() corev1.ResourceList {
				return corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				}
			},
			resourceName: corev1.ResourceCPU,
			expect:       false,
		},
		{
			name: "resource changed",
			getOldResourceList: func() corev1.ResourceList {
				return corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				}
			},
			getNewResourceList: func() corev1.ResourceList {
				return corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				}
			},
			resourceName: corev1.ResourceCPU,
			expect:       true,
		},
		{
			name: "resource not changed",
			getOldResourceList: func() corev1.ResourceList {
				return corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				}
			},
			getNewResourceList: func() corev1.ResourceList {
				return corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				}
			},
			resourceName: corev1.ResourceMemory,
			expect:       false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			is := isResourceChanged(cs.getOldResourceList(), cs.getNewResourceList(), cs.resourceName)
			if cs.expect != is {
				t.Fatalf("IsResourceChanged failed")
			}
		})
	}
}

func TestIsPodReady(t *testing.T) {
	cases := []struct {
		name   string
		getPod func() *corev1.Pod
		expect bool
	}{
		{
			name: "pod ready",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				return demo
			},
			expect: true,
		},
		{
			name: "pod not ready",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Status.Conditions[0].Status = corev1.ConditionFalse
				return demo
			},
			expect: false,
		},
		{
			name: "pod contains unavailable label",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Labels[fmt.Sprintf("%sdata", pub.PubUnavailablePodLabelPrefix)] = "true"
				return demo
			},
			expect: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			control := commonControl{}
			is := control.IsPodReady(cs.getPod())
			if cs.expect != is {
				t.Fatalf("IsPodReady failed")
			}
		})
	}
}

// TestIsPodStateConsistent covers the digest fast-path of IsPodStateConsistent, including
// restartable init containers, and makes sure the fast-path can no longer skip the in-place
// update checks.
func TestIsPodStateConsistent(t *testing.T) {
	const (
		digestImage     = "busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"
		digestImageID   = "docker-pullable://busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"
		staleImageID    = "docker-pullable://busybox@sha256:00006defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"
		tagImage        = "busybox:1.36"
		tagImageID      = "docker-pullable://busybox:1.36"
		graceAnnotation = `{"revision":"r2","containerImages":{"main":"busybox:1.37"}}`
	)

	restartAlways := corev1.ContainerRestartPolicyAlways
	newPod := func() *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "pod-0"},
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{Name: "setup", Image: digestImage},
					{Name: "sidecar", Image: digestImage, RestartPolicy: &restartAlways},
				},
				Containers: []corev1.Container{
					{Name: "main", Image: digestImage},
				},
			},
			Status: corev1.PodStatus{
				InitContainerStatuses: []corev1.ContainerStatus{
					{Name: "setup", ImageID: digestImageID},
					{Name: "sidecar", ImageID: digestImageID},
				},
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "main", ImageID: digestImageID},
				},
			},
		}
	}

	cases := []struct {
		name     string
		mutate   func(pod *corev1.Pod)
		expected bool
	}{
		{
			name:     "all digest images consistent",
			mutate:   func(pod *corev1.Pod) {},
			expected: true,
		},
		{
			name: "all digest images must not skip the in-place update grace period check",
			mutate: func(pod *corev1.Pod) {
				pod.Annotations = map[string]string{pub.InPlaceUpdateGraceKey: graceAnnotation}
			},
			expected: false,
		},
		{
			name: "restartable init container with stale imageID is inconsistent",
			mutate: func(pod *corev1.Pod) {
				pod.Status.InitContainerStatuses[1].ImageID = staleImageID
			},
			expected: false,
		},
		{
			name: "regular init container with stale imageID is skipped",
			mutate: func(pod *corev1.Pod) {
				pod.Status.InitContainerStatuses[0].ImageID = staleImageID
			},
			expected: true,
		},
		{
			name: "tag images keep the previous behavior",
			mutate: func(pod *corev1.Pod) {
				pod.Spec.Containers[0].Image = tagImage
				pod.Spec.InitContainers[1].Image = tagImage
				pod.Status.ContainerStatuses[0].ImageID = tagImageID
				pod.Status.ContainerStatuses[0].Image = tagImage
				pod.Status.InitContainerStatuses[1].ImageID = tagImageID
				pod.Status.InitContainerStatuses[1].Image = tagImage
			},
			expected: true,
		},
		{
			name: "tag images with unchanged imageID in the update state are inconsistent",
			mutate: func(pod *corev1.Pod) {
				pod.Spec.Containers[0].Image = "busybox:1.37"
				pod.Status.ContainerStatuses[0].ImageID = tagImageID
				pod.Status.ContainerStatuses[0].Image = tagImage
				pod.Annotations = map[string]string{pub.InPlaceUpdateStateKey: `{"revision":"r2","lastContainerStatuses":{"main":{"imageID":"` + tagImageID + `"}}}`}
			},
			expected: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			control := commonControl{}
			pod := newPod()
			cs.mutate(pod)
			is := control.IsPodStateConsistent(pod)
			if cs.expected != is {
				t.Fatalf("IsPodStateConsistent expected %v, but got %v", cs.expected, is)
			}
		})
	}
}
