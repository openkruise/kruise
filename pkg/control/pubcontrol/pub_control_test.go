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

// ==================== PodGroupPolicy tests ====================

func TestGroupPubPods(t *testing.T) {
	cases := []struct {
		name           string
		pub            *policyv1alpha1.PodUnavailableBudget
		pods           []*corev1.Pod
		expectGroups   int
		expectGroupMap map[string]int // groupName -> pod count
	}{
		{
			name: "no PodGroupPolicy, each pod is its own group",
			pub: &policyv1alpha1.PodUnavailableBudget{
				Spec: policyv1alpha1.PodUnavailableBudgetSpec{},
			},
			pods: []*corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{"group": "g0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Labels: map[string]string{"group": "g0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Labels: map[string]string{"group": "g1"}}},
			},
			expectGroups: 3,
			expectGroupMap: map[string]int{
				"pod-0": 1,
				"pod-1": 1,
				"pod-2": 1,
			},
		},
		{
			name: "PodGroupPolicy with empty GroupLabelKey, fallback to per-pod",
			pub: &policyv1alpha1.PodUnavailableBudget{
				Spec: policyv1alpha1.PodUnavailableBudgetSpec{
					PodGroupPolicy: &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
						GroupLabelKey: "",
					},
				},
			},
			pods: []*corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{"group": "g0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Labels: map[string]string{"group": "g0"}}},
			},
			expectGroups: 2,
			expectGroupMap: map[string]int{
				"pod-0": 1,
				"pod-1": 1,
			},
		},
		{
			name: "PodGroupPolicy groups pods by label key",
			pub: &policyv1alpha1.PodUnavailableBudget{
				Spec: policyv1alpha1.PodUnavailableBudgetSpec{
					PodGroupPolicy: &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
						GroupLabelKey: "group-index",
					},
				},
			},
			pods: []*corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{"group-index": "g0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Labels: map[string]string{"group-index": "g0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Labels: map[string]string{"group-index": "g1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-3", Labels: map[string]string{"group-index": "g1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-4", Labels: map[string]string{"group-index": "g2"}}},
			},
			expectGroups: 3,
			expectGroupMap: map[string]int{
				"g0": 2,
				"g1": 2,
				"g2": 1,
			},
		},
		{
			name: "pod missing group label falls back to per-pod semantics",
			pub: &policyv1alpha1.PodUnavailableBudget{
				Spec: policyv1alpha1.PodUnavailableBudgetSpec{
					PodGroupPolicy: &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
						GroupLabelKey: "group-index",
					},
				},
			},
			pods: []*corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{"group-index": "g0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Labels: map[string]string{"group-index": "g0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-no-label", Labels: map[string]string{}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-nil-label"}},
			},
			expectGroups: 3,
			expectGroupMap: map[string]int{
				"g0":            2,
				"pod-no-label":  1,
				"pod-nil-label": 1,
			},
		},
		{
			name: "empty pod list",
			pub: &policyv1alpha1.PodUnavailableBudget{
				Spec: policyv1alpha1.PodUnavailableBudgetSpec{
					PodGroupPolicy: &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
						GroupLabelKey: "group-index",
					},
				},
			},
			pods:           []*corev1.Pod{},
			expectGroups:   0,
			expectGroupMap: map[string]int{},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			result := groupPubPods(cs.pub, cs.pods)
			if len(result) != cs.expectGroups {
				t.Fatalf("expected %d groups, got %d", cs.expectGroups, len(result))
			}
			for gName, expectedCount := range cs.expectGroupMap {
				pods, ok := result[gName]
				if !ok {
					t.Fatalf("expected group %q not found in result", gName)
				}
				if len(pods) != expectedCount {
					t.Fatalf("expected group %q to have %d pods, got %d", gName, expectedCount, len(pods))
				}
			}
		})
	}
}

func TestIsPodGroupConsistentAndReady(t *testing.T) {
	makePod := func(name string, ready bool) *corev1.Pod {
		condStatus := corev1.ConditionTrue
		if !ready {
			condStatus = corev1.ConditionFalse
		}
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:v1",
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: condStatus,
					},
				},
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:    "nginx",
						Image:   "nginx:v1",
						ImageID: "nginx@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d",
						Ready:   ready,
					},
				},
			},
		}
	}

	cases := []struct {
		name   string
		pods   []*corev1.Pod
		size   int32
		expect bool
	}{
		{
			name:   "all pods ready, size matches",
			pods:   []*corev1.Pod{makePod("p0", true), makePod("p1", true), makePod("p2", true)},
			size:   3,
			expect: true,
		},
		{
			name:   "all pods ready, size less than pod count",
			pods:   []*corev1.Pod{makePod("p0", true), makePod("p1", true), makePod("p2", true)},
			size:   2,
			expect: true,
		},
		{
			name:   "fewer pods than size",
			pods:   []*corev1.Pod{makePod("p0", true)},
			size:   3,
			expect: false,
		},
		{
			name:   "enough pods but some not ready",
			pods:   []*corev1.Pod{makePod("p0", true), makePod("p1", false), makePod("p2", true)},
			size:   3,
			expect: false,
		},
		{
			name:   "enough pods and enough ready (2/3 ready, size=2)",
			pods:   []*corev1.Pod{makePod("p0", true), makePod("p1", false), makePod("p2", true)},
			size:   2,
			expect: true,
		},
		{
			name:   "empty pod list",
			pods:   []*corev1.Pod{},
			size:   1,
			expect: false,
		},
		{
			name:   "size 0, empty pods",
			pods:   []*corev1.Pod{},
			size:   0,
			expect: true,
		},
	}

	ctrl := &commonControl{}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			got := ctrl.IsPodGroupConsistentAndReady(cs.pods, cs.size)
			if got != cs.expect {
				t.Fatalf("expected %v, got %v", cs.expect, got)
			}
		})
	}
}

func TestIsPodGroupConsistentAndReadyViaInterface(t *testing.T) {
	// Also test through the PubControl global interface to make sure it's wired up
	makePod := func(name string, ready bool) *corev1.Pod {
		condStatus := corev1.ConditionTrue
		if !ready {
			condStatus = corev1.ConditionFalse
		}
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c", Image: "img:v1"}},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: condStatus},
				},
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c", Image: "img:v1", ImageID: "img@sha256:abc123", Ready: ready},
				},
			},
		}
	}

	ctrl := &commonControl{}
	pods := []*corev1.Pod{makePod("a", true), makePod("b", true)}
	if !ctrl.IsPodGroupConsistentAndReady(pods, 2) {
		t.Fatal("expected group to be consistent and ready")
	}
	if ctrl.IsPodGroupConsistentAndReady(pods, 3) {
		t.Fatal("expected group NOT to be consistent and ready when size > pod count")
	}
}
