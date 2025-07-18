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
	"k8s.io/kubernetes/pkg/kubelet/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openkruise/kruise/apis/apps/pub"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
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
		getPub    func() *policyv1alpha1.PodUnavailableBudget
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
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
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
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
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
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
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
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expect: true,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs.getPub()).
				WithStatusSubresource(&policyv1alpha1.PodUnavailableBudget{}).Build()
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
