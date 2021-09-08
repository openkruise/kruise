/*
Copyright 2021 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOWorkloadSpread WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workloadspread

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	"github.com/openkruise/kruise/pkg/util/requeueduration"
	wsutil "github.com/openkruise/kruise/pkg/util/workloadspread"
)

const (
	PodDeletionCostDefault = "0"
)

var (
	PodDeletionCostAnnotation = "controller.kubernetes.io/pod-deletion-cost"
	scheme                    *runtime.Scheme
	currentTime               time.Time
	timeFormat                = "2006-01-02 15:04:05"
	m, _                      = time.ParseDuration("-1m")
	s, _                      = time.ParseDuration("-1s")

	podDemo = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-0",
			Namespace: "default",
			Labels: map[string]string{
				"app": "nginx",
			},
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "apps.kruise.io/v1alpha1",
					Kind:               "CloneSet",
					Name:               "cloneset-test",
					Controller:         utilpointer.BoolPtr(true),
					UID:                types.UID("a03eb001-27eb-4713-b634-7c46f6861758"),
					BlockOwnerDeletion: utilpointer.BoolPtr(true),
				},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
				},
			},
		},
	}

	cloneSetDemo = &appsv1alpha1.CloneSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CloneSet",
			APIVersion: "apps.kruise.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cloneset-test",
			Namespace:  "default",
			Generation: 10,
			UID:        types.UID("a03eb001-27eb-4713-b634-7c46f6861758"),
		},
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: utilpointer.Int32Ptr(10),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
		},
	}

	workloadSpreadDemo = &appsv1alpha1.WorkloadSpread{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps.kruise.io/v1alpha1",
			Kind:       "WorkloadSpread",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-workloadSpread",
			Namespace:  "default",
			Generation: 10,
		},
		Spec: appsv1alpha1.WorkloadSpreadSpec{
			TargetReference: &appsv1alpha1.TargetReference{
				APIVersion: "apps.kruise.io/v1alpha1",
				Kind:       "CloneSet",
				Name:       "cloneset-test",
			},
			Subsets: []appsv1alpha1.WorkloadSpreadSubset{
				{
					Name:        "subset-a",
					MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 5},
				},
			},
		},
		Status: appsv1alpha1.WorkloadSpreadStatus{
			ObservedGeneration: 10,
			SubsetStatuses: []appsv1alpha1.WorkloadSpreadSubsetStatus{
				{
					Name:            "subset-a",
					MissingReplicas: 5,
					CreatingPods:    map[string]metav1.Time{},
					DeletingPods:    map[string]metav1.Time{},
				},
			},
		},
	}

	subsetDemo = appsv1alpha1.WorkloadSpreadSubset{
		Name:        "subset-a",
		MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 5},
	}
)

func init() {
	scheme = runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)
}

func TestSubsetPodDeletionCost(t *testing.T) {
	cases := []struct {
		name              string
		subsetIndex       int
		getPods           func() []*corev1.Pod
		getWorkloadSpread func() *appsv1alpha1.WorkloadSpread
		expectPods        func() []*corev1.Pod
	}{
		{
			name: "pods number == maxReplicas, subsetsLen = 2, subsetIndex = 0, maxReplicas is 3, pods number is 3",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 3)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets = make([]appsv1alpha1.WorkloadSpreadSubset, 2)
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 3}
				workloadSpread.Spec.Subsets[1].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 3}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 3)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "200",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
		},
		{
			name:        "pods number == maxReplicas, subsetsLen = 2, subsetIndex = 1, maxReplicas is 3, pods number is 3",
			subsetIndex: 1,
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 3)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets = make([]appsv1alpha1.WorkloadSpreadSubset, 2)
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 3}
				workloadSpread.Spec.Subsets[1].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 3}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 3)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "100",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
		},
		{
			name: "pods number == maxReplicas, subsetsLen = 2, subsetIndex = 0, maxReplicas is 3, pods number is 4",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 4)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets = make([]appsv1alpha1.WorkloadSpreadSubset, 2)
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 3}
				workloadSpread.Spec.Subsets[1].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 3}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 4)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "200",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[0].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				return pods
			},
		},
		{
			name:        "pods number == maxReplicas, subsetsLen = 2, subsetIndex = 1, maxReplicas is 3, pods number is 4",
			subsetIndex: 1,
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 4)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets = make([]appsv1alpha1.WorkloadSpreadSubset, 2)
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 3}
				workloadSpread.Spec.Subsets[1].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 3}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 4)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "100",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[0].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-200",
				}
				return pods
			},
		},
		{
			name: "pods number == maxReplicas, maxReplicas is 3, pods number is 3",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 3)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 3}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 3)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "100",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
		},
		{
			name: "pods number < maxReplicas, maxReplicas is 3, pods number is 2",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 2)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 3}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 2)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "100",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
		},
		{
			name: "active pods number > maxReplicas, maxReplicas is 1, pods number is 3",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 3)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 1}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 3)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "-100",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[2].Annotations = map[string]string{
					PodDeletionCostAnnotation: "100",
				}
				return pods
			},
		},
		{
			name: "active pods number > maxReplicas, maxReplicas is 1, pods number is 4, pod-0 deletionTimestamp is not nil",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 4)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				deleteTime := metav1.Now()
				pods[0].DeletionTimestamp = &deleteTime
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 1}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 4)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "-100",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[0].Annotations = map[string]string{}
				pods[3].Annotations = map[string]string{
					PodDeletionCostAnnotation: "100",
				}
				return pods
			},
		},
		{
			name: "maxReplicas is nil, pods number is 3, original annotation == 1",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 3)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations[PodDeletionCostAnnotation] = PodDeletionCostDefault
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = nil
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 3)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations[PodDeletionCostAnnotation] = "100"
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				return pods
			},
		},
		{
			name: "maxReplicas is 2, active pods number is 5, (pod-0, pod-3, pod-4) are unhealthy",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[0].Spec.NodeName = ""
				pods[1].Status.Phase = corev1.PodRunning
				pods[2].Status.Phase = corev1.PodRunning
				pods[3].Status.Phase = corev1.PodPending
				pods[4].Status.Phase = corev1.PodPending
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 2}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "100",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[0].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				pods[3].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				pods[4].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				return pods
			},
		},
		{
			name: "maxReplicas is 2, active pods number is 5, (pod-2, pod-3, pod-4) are unhealthy",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[0].Status.Phase = corev1.PodRunning
				pods[1].Status.Phase = corev1.PodRunning

				pods[2].Spec.NodeName = ""
				pods[3].Status.Phase = corev1.PodPending
				pods[4].Status.Phase = corev1.PodUnknown
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 2}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "100",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[2].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				pods[3].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				pods[4].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				return pods
			},
		},
		{
			name: "maxReplicas is 2, active pods number is 5, (pod-2, pod-3) are unhealthy, pod-4 deletionTimestamp is not nil",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[0].Status.Phase = corev1.PodRunning
				pods[1].Status.Phase = corev1.PodRunning

				pods[2].Spec.NodeName = ""
				pods[3].Status.Phase = corev1.PodPending

				deleteTime := metav1.Now()
				pods[4].DeletionTimestamp = &deleteTime
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 2}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "100",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[2].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				pods[3].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				pods[4].Annotations = map[string]string{}
				return pods
			},
		},
		{
			name: "maxReplicas is 2, active pods number is 5, (pod-2, pod-4) are unhealthy, pod-3 deletionTimestamp is not nil",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[0].Status.Phase = corev1.PodRunning
				pods[1].Status.Phase = corev1.PodRunning

				pods[2].Spec.NodeName = ""
				pods[4].Status.Phase = corev1.PodPending

				deleteTime := metav1.Now()
				pods[3].DeletionTimestamp = &deleteTime
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 2}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "100",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[2].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				pods[4].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				pods[3].Annotations = map[string]string{}
				return pods
			},
		},
		{
			name: "maxReplicas is 2, active pods number is 4, (pod-2, pod-4) are unhealthy, pod-3 is failed",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[0].Status.Phase = corev1.PodRunning
				pods[1].Status.Phase = corev1.PodRunning

				pods[2].Spec.NodeName = ""
				pods[4].Status.Phase = corev1.PodPending
				pods[3].Status.Phase = corev1.PodFailed
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 2}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "100",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[2].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				pods[4].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				pods[3].Annotations = map[string]string{}
				return pods
			},
		},
		{
			name: "maxReplicas is 2, active pods number is 4, (pod-2, pod-4) are unhealthy, pod-3 is succeed",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[0].Status.Phase = corev1.PodRunning
				pods[1].Status.Phase = corev1.PodRunning

				pods[2].Spec.NodeName = ""
				pods[4].Status.Phase = corev1.PodPending
				pods[3].Status.Phase = corev1.PodSucceeded
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 2}
				return workloadSpread
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation: "100",
					}
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
				}
				pods[2].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				pods[4].Annotations = map[string]string{
					PodDeletionCostAnnotation: "-100",
				}
				pods[3].Annotations = map[string]string{}
				return pods
			},
		},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClientWithScheme(scheme)
			for _, pod := range cs.getPods() {
				podIn := pod.DeepCopy()
				err := fakeClient.Create(context.TODO(), podIn)
				if err != nil {
					t.Fatalf("create pod failed: %s", err.Error())
				}
			}

			workloadSpread := cs.getWorkloadSpread()

			r := ReconcileWorkloadSpread{
				Client:   fakeClient,
				recorder: record.NewFakeRecorder(10),
			}

			err := r.syncSubsetPodDeletionCost(workloadSpread, &workloadSpread.Spec.Subsets[0], cs.subsetIndex, cs.getPods(), 5)
			if err != nil {
				t.Fatalf("set pod deletion-cost annotation failed: %s", err.Error())
			}

			latestPods, _ := getLatestPods(fakeClient, workloadSpread)
			expectPods := cs.expectPods()

			if len(latestPods) != len(expectPods) {
				t.Fatalf("set Pod deletion-coset annotation failed")
			}
			for i := range expectPods {
				annotation1 := latestPods[i].Annotations
				annotation2 := expectPods[i].Annotations
				if !apiequality.Semantic.DeepEqual(annotation1, annotation2) {
					fmt.Println(expectPods[i].Name)
					fmt.Println(annotation1)
					fmt.Println(annotation2)
					t.Fatalf("set Pod deletion-coset annotation failed")
				}
			}
		})
	}
}

func TestWorkloadSpreadReconcile(t *testing.T) {
	cases := []struct {
		name                 string
		getPods              func() []*corev1.Pod
		getWorkloadSpread    func() *appsv1alpha1.WorkloadSpread
		getCloneSet          func() *appsv1alpha1.CloneSet
		expectPods           func() []*corev1.Pod
		expectWorkloadSpread func() *appsv1alpha1.WorkloadSpread
	}{
		{
			name: "one subset, create zero pod, maxReplicas = 5, missingReplicas = 5",
			getPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				return workloadSpreadDemo.DeepCopy()
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return nil
			},
			expectPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.ObservedGeneration = 10
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 5
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
		{
			name: "two subset, create zero pod, maxReplicas = 5, missingReplicas = 5",
			getPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				subset1 := subsetDemo.DeepCopy()
				subset2 := subsetDemo.DeepCopy()
				subset2.Name = "subset-b"
				workloadSpread.Spec.Subsets = []appsv1alpha1.WorkloadSpreadSubset{*subset1, *subset2}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.ObservedGeneration = 10
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(10)
				workloadSpread.Status.SubsetStatuses = make([]appsv1alpha1.WorkloadSpreadSubsetStatus, 2)
				workloadSpread.Status.SubsetStatuses[0].Name = "subset-a"
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 5
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[1].Name = "subset-b"
				workloadSpread.Status.SubsetStatuses[1].MissingReplicas = 5
				workloadSpread.Status.SubsetStatuses[1].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[1].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
		{
			name: "two subset, create two pod, maxReplicas = 5, missingReplicas = 4",
			getPods: func() []*corev1.Pod {
				pod1 := podDemo.DeepCopy()
				pod1.Name = "test-pod-0"
				pod1.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
				}
				pod2 := podDemo.DeepCopy()
				pod2.Name = "test-pod-1"
				pod2.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-b"}`,
				}
				return []*corev1.Pod{pod1, pod2}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				subset1 := subsetDemo.DeepCopy()
				subset2 := subsetDemo.DeepCopy()
				subset2.Name = "subset-b"
				workloadSpread.Spec.Subsets = []appsv1alpha1.WorkloadSpreadSubset{*subset1, *subset2}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				pod1 := podDemo.DeepCopy()
				pod1.Name = "test-pod-0"
				pod1.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					PodDeletionCostAnnotation:                     "200",
				}
				pod2 := podDemo.DeepCopy()
				pod2.Name = "test-pod-1"
				pod2.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-b"}`,
					PodDeletionCostAnnotation:                     "100",
				}
				return []*corev1.Pod{pod1, pod2}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.ObservedGeneration = 10
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(10)
				workloadSpread.Status.SubsetStatuses = make([]appsv1alpha1.WorkloadSpreadSubsetStatus, 2)
				workloadSpread.Status.SubsetStatuses[0].Name = "subset-a"
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 4
				workloadSpread.Status.SubsetStatuses[0].Replicas = 1
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[1].Name = "subset-b"
				workloadSpread.Status.SubsetStatuses[1].MissingReplicas = 4
				workloadSpread.Status.SubsetStatuses[1].Replicas = 1
				workloadSpread.Status.SubsetStatuses[1].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[1].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
		{
			name: "create one pod, maxReplicas = 5, missingReplicas = 4",
			getPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
				}
				return []*corev1.Pod{pod}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				return workloadSpreadDemo.DeepCopy()
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					PodDeletionCostAnnotation:                     "100",
				}
				return []*corev1.Pod{pod}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(10)
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 4
				workloadSpread.Status.SubsetStatuses[0].Replicas = 1
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
		{
			name: "create one pod, maxReplicas = 100%, missingReplicas = 4",
			getPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
				}
				return []*corev1.Pod{pod}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.String, StrVal: "100%"}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				cloneSet := cloneSetDemo.DeepCopy()
				cloneSet.Spec.Replicas = utilpointer.Int32Ptr(5)
				return cloneSet
			},
			expectPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					PodDeletionCostAnnotation:                     "100",
				}
				return []*corev1.Pod{pod}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(5)
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 4
				workloadSpread.Status.SubsetStatuses[0].Replicas = 1
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
		{
			name: "create one pod, maxReplicas = 80%, missingReplicas = 3",
			getPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
				}
				return []*corev1.Pod{pod}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.String, StrVal: "80%"}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				cloneSet := cloneSetDemo.DeepCopy()
				cloneSet.Spec.Replicas = utilpointer.Int32Ptr(5)
				return cloneSet
			},
			expectPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					PodDeletionCostAnnotation:                     "100",
				}
				return []*corev1.Pod{pod}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(5)
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 3
				workloadSpread.Status.SubsetStatuses[0].Replicas = 1
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
		{
			name: "create two pods, one failed but no timeout, maxReplicas = 5, missingReplicas = 3",
			getPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
				}
				return []*corev1.Pod{pod}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
					"test-pod-0": {Time: currentTime.Add(2 * s)},
					"test-pod-1": {Time: currentTime.Add(2 * s)},
				}
				workloadSpread.Status.SubsetStatuses[0].Name = "subset-a"
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					PodDeletionCostAnnotation:                     "100",
				}
				return []*corev1.Pod{pod}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				formatTime := currentTime.Format(timeFormat)
				currentTime, _ = time.ParseInLocation(timeFormat, formatTime, time.Local)

				workloadSpread := workloadSpreadDemo.DeepCopy()
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(10)
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 3
				workloadSpread.Status.SubsetStatuses[0].Replicas = 1
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
					"test-pod-1": {Time: currentTime.Add(2 * s)},
				}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
		{
			name: "create two pod, one failed and timeout, maxReplicas = 5, missingReplicas = 4",
			getPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
				}
				return []*corev1.Pod{pod}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
					"test-pod-0": {Time: currentTime.Add(1 * m)},
					"test-pod-1": {Time: currentTime.Add(1 * m)},
				}
				workloadSpread.Status.SubsetStatuses[0].Name = "subset-a"
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					PodDeletionCostAnnotation:                     "100",
				}
				return []*corev1.Pod{pod}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(10)
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 4
				workloadSpread.Status.SubsetStatuses[0].Replicas = 1
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].Name = "subset-a"
				return workloadSpread
			},
		},
		{
			name: "create two pods, two succeed, maxReplicas = 5, missingReplicas = 3",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 2)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
					"test-pod-0": {Time: currentTime.Add(3 * s)},
					"test-pod-1": {Time: currentTime.Add(3 * s)},
				}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 2)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
						PodDeletionCostAnnotation:                     "100",
					}
				}
				return pods
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(10)
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 3
				workloadSpread.Status.SubsetStatuses[0].Replicas = 2
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
		{
			name: "create five pods, two succeed, maxReplicas = 5, missingReplicas = 3",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				pods[2].Status.Phase = corev1.PodFailed
				pods[3].Status.Phase = corev1.PodFailed
				pods[4].Status.Phase = corev1.PodSucceeded
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
					"test-pod-0": {Time: currentTime.Add(3 * s)},
					"test-pod-1": {Time: currentTime.Add(3 * s)},
				}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				pods[0].Annotations[PodDeletionCostAnnotation] = "100"
				pods[1].Annotations[PodDeletionCostAnnotation] = "100"
				return pods
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(10)
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 3
				workloadSpread.Status.SubsetStatuses[0].Replicas = 2
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
		{
			name: "create five pods, all succeed, maxReplicas = 5, missingReplicas = 0",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
					"test-pod-3": {Time: currentTime.Add(5 * s)},
					"test-pod-4": {Time: currentTime.Add(5 * s)},
				}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
						PodDeletionCostAnnotation:                     "100",
					}
				}
				return pods
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(10)
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 0
				workloadSpread.Status.SubsetStatuses[0].Replicas = 5
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
		{
			name: "create five pods, all succeed and remove one, maxReplicas = 5, missingReplicas = 1",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				deleteTime := metav1.Time{Time: currentTime.Add(1 * s)}
				pods[4].DeletionTimestamp = &deleteTime
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
					"test-pod-3": {Time: currentTime.Add(5 * s)},
					"test-pod-4": {Time: currentTime.Add(5 * s)},
				}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{
					"test-pod-4": {Time: currentTime.Add(1 * s)},
				}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
						PodDeletionCostAnnotation:                     "100",
					}
				}
				pods[4].Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
				}
				return pods
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(10)
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 1
				workloadSpread.Status.SubsetStatuses[0].Replicas = 4
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
		{
			name: "create five pods, all succeed and remove one failed but no timeout, maxReplicas = 5, missingReplicas = 1",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
					"test-pod-3": {Time: currentTime.Add(5 * s)},
					"test-pod-4": {Time: currentTime.Add(5 * s)},
				}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{
					"test-pod-4": {Time: currentTime.Add(1 * s)},
				}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
						PodDeletionCostAnnotation:                     "100",
					}
				}
				return pods
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(10)
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 1
				workloadSpread.Status.SubsetStatuses[0].Replicas = 5
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}

				formatTime := currentTime.Format(timeFormat)
				currentTime, _ = time.ParseInLocation(timeFormat, formatTime, time.Local)
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{
					"test-pod-4": {Time: currentTime.Add(1 * s)},
				}
				return workloadSpread
			},
		},
		{
			name: "create five pods, all succeed, remove one failed and timeout, maxReplicas = 5, missingReplicas = 0",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
					"test-pod-3": {Time: currentTime.Add(6 * m)},
					"test-pod-4": {Time: currentTime.Add(6 * m)},
				}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{
					"test-pod-4": {Time: currentTime.Add(16 * m)},
				}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
						PodDeletionCostAnnotation:                     "100",
					}
				}
				return pods
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(10)
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 0
				workloadSpread.Status.SubsetStatuses[0].Replicas = 5
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
		{
			name: "create five pods, all succeed, maxReplicas = 3, missingReplicas = 0, pod-0, pod-1 deletion-cost = -100",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.Int, IntVal: 3}
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
					"test-pod-3": {Time: currentTime.Add(6 * m)},
					"test-pod-4": {Time: currentTime.Add(6 * m)},
				}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pods-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
						PodDeletionCostAnnotation:                     "100",
					}
				}
				pods[0].Annotations[PodDeletionCostAnnotation] = "-100"
				pods[1].Annotations[PodDeletionCostAnnotation] = "-100"
				return pods
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				//workloadSpread.Status.ObservedWorkloadReplicas = int32(10)
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 0
				workloadSpread.Status.SubsetStatuses[0].Replicas = 5
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
				return workloadSpread
			},
		},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			currentTime = time.Now()
			workloadSpread := cs.getWorkloadSpread()
			fakeClient := fake.NewFakeClientWithScheme(scheme, workloadSpread)
			if cs.getCloneSet() != nil {
				err := fakeClient.Create(context.TODO(), cs.getCloneSet())
				if err != nil {
					t.Fatalf("create pod failed: %s", err.Error())
				}
			}
			for _, pod := range cs.getPods() {
				podIn := pod.DeepCopy()
				err := fakeClient.Create(context.TODO(), podIn)
				if err != nil {
					t.Fatalf("create pod failed: %s", err.Error())
				}
			}

			reconciler := ReconcileWorkloadSpread{
				Client:           fakeClient,
				recorder:         record.NewFakeRecorder(10),
				controllerFinder: controllerfinder.NewControllerFinder(fakeClient),
			}

			err := reconciler.syncWorkloadSpread(workloadSpread)
			if err != nil {
				t.Fatalf("sync WorkloadSpread failed: %s", err.Error())
			}

			latestPodList, _ := getLatestPods(fakeClient, workloadSpread)
			expectPodList := cs.expectPods()

			for i := range latestPodList {
				annotation1 := latestPodList[i].Annotations
				annotation2 := expectPodList[i].Annotations
				if !reflect.DeepEqual(annotation1, annotation2) {
					fmt.Println(annotation1)
					fmt.Println(annotation2)
					t.Fatalf("set Pod deletion-coset annotation failed")
				}
			}

			latestWorkloadSpread, err := getLatestWorkloadSpread(fakeClient, workloadSpread)
			if err != nil {
				t.Fatalf("getLatestWorkloadSpread failed: %s", err.Error())
			}

			latestStatus := latestWorkloadSpread.Status
			by, _ := json.Marshal(latestStatus)
			fmt.Println(string(by))

			exceptStatus := cs.expectWorkloadSpread().Status
			by, _ = json.Marshal(exceptStatus)
			fmt.Println(string(by))

			if !apiequality.Semantic.DeepEqual(latestStatus, exceptStatus) {
				t.Fatalf("workloadSpread status DeepEqual failed")
			}
		})
	}
}

// This test checks that user changes subsets sequence of WorkloadSpreadSpec.
func TestUpdateSubsetSequence(t *testing.T) {
	pods := make([]*corev1.Pod, 2)
	pods[0] = podDemo.DeepCopy()
	pods[0].Name = "test-pod-0"
	pods[0].Annotations = map[string]string{
		wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
	}
	pods[1] = podDemo.DeepCopy()
	pods[1].Name = "test-pod-1"
	pods[1].Annotations = map[string]string{
		wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-b"}`,
	}

	workloadSpread := workloadSpreadDemo.DeepCopy()
	subset1 := subsetDemo.DeepCopy()
	subset2 := subsetDemo.DeepCopy()
	subset2.Name = "subset-b"
	workloadSpread.Spec.Subsets = []appsv1alpha1.WorkloadSpreadSubset{*subset2, *subset1}

	workloadSpread.Status.SubsetStatuses = make([]appsv1alpha1.WorkloadSpreadSubsetStatus, 2)
	workloadSpread.Status.SubsetStatuses[0].Name = "subset-a"
	workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 4
	workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
		"test-pod-0": {Time: currentTime.Add(2 * s)},
	}
	workloadSpread.Status.SubsetStatuses[1].Name = "subset-b"
	workloadSpread.Status.SubsetStatuses[1].MissingReplicas = 4
	workloadSpread.Status.SubsetStatuses[1].CreatingPods = map[string]metav1.Time{
		"test-pod-1": {Time: currentTime.Add(2 * s)},
	}

	subsetsPods := groupPod(workloadSpread, pods)

	r := ReconcileWorkloadSpread{}
	status, _ := r.calculateWorkloadSpreadStatus(workloadSpread, subsetsPods, 5)
	if status == nil {
		t.Fatalf("error get WorkloadSpread status")
	} else {
		if status.SubsetStatuses[0].Name != subset2.Name || status.SubsetStatuses[1].Name != subset1.Name {
			t.Fatalf("WorkloadSpread's status mismatch subsets of spec")
		}
		if status.SubsetStatuses[0].MissingReplicas != 4 ||
			!apiequality.Semantic.DeepEqual(status.SubsetStatuses[0].CreatingPods, map[string]metav1.Time{}) {
			t.Fatalf("failted to sync WorkloadSpread subset-0 status")
		}
		if status.SubsetStatuses[1].MissingReplicas != 4 ||
			!apiequality.Semantic.DeepEqual(status.SubsetStatuses[1].CreatingPods, map[string]metav1.Time{}) {
			t.Fatalf("failted to sync WorkloadSpread subset-1 status")
		}
	}
}

// This test checks that some creation or deletion failed but no timeout and we need requeue it to reconcile it again
// when timeout.
func TestDelayReconcile(t *testing.T) {
	cases := []struct {
		name                 string
		getPods              func() []*corev1.Pod
		getWorkloadSpread    func() *appsv1alpha1.WorkloadSpread
		getCloneSet          func() *appsv1alpha1.CloneSet
		expectWorkloadSpread func() *appsv1alpha1.WorkloadSpread
		expectRequeueAfter   time.Duration
	}{
		{
			name: "create three pods, two failed but no timeout",
			getPods: func() []*corev1.Pod {
				return []*corev1.Pod{podDemo.DeepCopy()}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
					"test-pod-2": {Time: currentTime.Add(3 * s)},
					"test-pod-3": {Time: currentTime.Add(1 * s)},
				}
				workloadSpread.Status.SubsetStatuses[0].Name = "subset-a"
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectRequeueAfter: 27 * time.Second,
		},
		{
			name: "create five pods, remove two, all failed but no timeout",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 5)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{
					"test-pod-3": {Time: currentTime.Add(3 * s)},
					"test-pod-4": {Time: currentTime.Add(1 * s)},
				}
				workloadSpread.Status.SubsetStatuses[0].Name = "subset-a"
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectRequeueAfter: 12 * time.Second,
		},
		{
			name: "create five pods, create one no timeout, remove one no timeout",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 4)
				for i := range pods {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{
					"test-pod-4": {Time: currentTime.Add(2 * s)},
				}
				workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{
					"test-pod-1": {Time: currentTime.Add(3 * s)},
				}
				workloadSpread.Status.SubsetStatuses[0].Name = "subset-a"
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectRequeueAfter: 12 * time.Second,
		},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			currentTime = time.Now()
			workloadSpread := cs.getWorkloadSpread()
			fakeClient := fake.NewFakeClientWithScheme(scheme, cs.getCloneSet(), workloadSpread)
			for _, pod := range cs.getPods() {
				podIn := pod.DeepCopy()
				err := fakeClient.Create(context.TODO(), podIn)
				if err != nil {
					t.Fatalf("create pod failed: %s", err.Error())
				}
			}

			reconciler := ReconcileWorkloadSpread{
				Client:           fakeClient,
				recorder:         record.NewFakeRecorder(10),
				controllerFinder: controllerfinder.NewControllerFinder(fakeClient),
			}

			durationStore = requeueduration.DurationStore{}

			nsn := types.NamespacedName{Namespace: workloadSpread.Namespace, Name: workloadSpread.Name}
			result, _ := reconciler.Reconcile(reconcile.Request{NamespacedName: nsn})
			if (cs.expectRequeueAfter - result.RequeueAfter) > 1*time.Second {
				t.Fatalf("requeue key failed")
			}
		})
	}
}

func getLatestWorkloadSpread(client client.Client, workloadSpread *appsv1alpha1.WorkloadSpread) (*appsv1alpha1.WorkloadSpread, error) {
	newWorkloadSpread := &appsv1alpha1.WorkloadSpread{}
	key := types.NamespacedName{
		Namespace: workloadSpread.Namespace,
		Name:      workloadSpread.Name,
	}
	err := client.Get(context.TODO(), key, newWorkloadSpread)
	return newWorkloadSpread, err
}

func getLatestPods(c client.Client, workloadSpread *appsv1alpha1.WorkloadSpread) ([]*corev1.Pod, error) {
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "nginx",
		}}
	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}
	podList := &corev1.PodList{}
	opts := &client.ListOptions{
		Namespace:     workloadSpread.Namespace,
		LabelSelector: labelSelector,
	}
	err = c.List(context.TODO(), podList, opts)
	matchedPods := make([]*corev1.Pod, len(podList.Items))
	for i := range podList.Items {
		matchedPods[i] = &podList.Items[i]
	}
	return matchedPods, err
}
