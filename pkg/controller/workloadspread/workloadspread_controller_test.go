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
	"strconv"
	"testing"
	"time"

	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/configuration"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	utilpointer "k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
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
					Controller:         ptr.To(true),
					UID:                types.UID("a03eb001-27eb-4713-b634-7c46f6861758"),
					BlockOwnerDeletion: ptr.To(true),
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
			Replicas: ptr.To(int32(10)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
		},
		Status: appsv1alpha1.CloneSetStatus{
			ObservedGeneration: 10,
			UpdateRevision:     wsutil.VersionIgnored,
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

	getWhiteListDemoCopy = func() configuration.WSCustomWorkloadWhiteList {
		return configuration.WSCustomWorkloadWhiteList{
			Workloads: []configuration.CustomWorkload{
				{
					GroupVersionKind: schema.GroupVersionKind{
						Group:   "mock.kruise.io",
						Version: "v1",
						Kind:    "GameServerSet",
					},
					ReplicasPath: "spec.replicas",
				},
			},
		}
	}
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
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
				pods[0].Finalizers = []string{"finalizers.sigs.k8s.io/test"}
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
				pods[4].Finalizers = []string{"finalizers.sigs.k8s.io/test"}
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
				pods[3].Finalizers = []string{"finalizers.sigs.k8s.io/test"}
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
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
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
	getTwoPodsWithDifferentLabels := func() []*corev1.Pod {
		pod1 := podDemo.DeepCopy()
		pod1.Annotations = map[string]string{
			wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
		}
		pod1.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: "mock.kruise.io/v1",
				Kind:       "GameServerSet",
				Name:       "workload",
				UID:        "12345",
			},
		}
		pod1.Labels["selected"] = "true"
		pod2 := pod1.DeepCopy()
		pod2.Name = "another"
		pod2.Labels["selected"] = "false"
		pod2.Labels["app"] = "not-nginx" // preventing being selected by func getLatestPods
		return []*corev1.Pod{pod1, pod2}
	}

	getWorkloadSpreadWithPercentSubsetB := func() *appsv1alpha1.WorkloadSpread {
		workloadSpread := workloadSpreadDemo.DeepCopy()
		workloadSpread.Spec.Subsets = append(workloadSpread.Spec.Subsets, appsv1alpha1.WorkloadSpreadSubset{
			Name:        "subset-b",
			MaxReplicas: &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
		})
		workloadSpread.Spec.TargetReference = &appsv1alpha1.TargetReference{
			APIVersion: "mock.kruise.io/v1",
			Kind:       "GameServerSet",
			Name:       "workload",
		}
		workloadSpread.Spec.TargetFilter = &appsv1alpha1.TargetFilter{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"selected": "true",
				},
			},
		}
		return workloadSpread
	}
	expectWorkloadSpreadWithPercentSubsetB := func() *appsv1alpha1.WorkloadSpread {
		workloadSpread := workloadSpreadDemo.DeepCopy()
		workloadSpread.Spec.TargetReference = &appsv1alpha1.TargetReference{
			APIVersion: "mock.kruise.io/v1",
			Kind:       "GameServerSet",
			Name:       "workload",
		}
		workloadSpread.Status.SubsetStatuses = append(workloadSpread.Status.SubsetStatuses, appsv1alpha1.WorkloadSpreadSubsetStatus{})
		workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 4
		workloadSpread.Status.SubsetStatuses[0].Replicas = 1
		workloadSpread.Status.SubsetStatuses[0].CreatingPods = map[string]metav1.Time{}
		workloadSpread.Status.SubsetStatuses[0].DeletingPods = map[string]metav1.Time{}
		workloadSpread.Status.SubsetStatuses[1].Name = "subset-b"
		workloadSpread.Status.SubsetStatuses[1].MissingReplicas = 0
		workloadSpread.Status.SubsetStatuses[1].Replicas = 0
		workloadSpread.Status.SubsetStatuses[1].CreatingPods = map[string]metav1.Time{}
		workloadSpread.Status.SubsetStatuses[1].DeletingPods = map[string]metav1.Time{}
		return workloadSpread
	}
	cases := []struct {
		name                 string
		getPods              func() []*corev1.Pod
		getWorkloadSpread    func() *appsv1alpha1.WorkloadSpread
		getCloneSet          func() *appsv1alpha1.CloneSet
		getWorkloads         func() []client.Object
		getWhiteList         func() configuration.WSCustomWorkloadWhiteList
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
				cloneSet.Spec.Replicas = ptr.To(int32(5))
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
				cloneSet.Spec.Replicas = ptr.To(int32(5))
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
				pods[4].Finalizers = []string{"finalizers.sigs.k8s.io/test"}
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
			name: "multiVersion pods",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 10)
				for i := 0; i < 6; i++ {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				for i := 6; i < 10; i++ {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-b"}`,
					}
				}
				pods[0].Labels[apps.DefaultDeploymentUniqueLabelKey] = "oldVersion"
				pods[2].Labels[apps.DefaultDeploymentUniqueLabelKey] = "oldVersion"
				pods[4].Labels[apps.DefaultDeploymentUniqueLabelKey] = "oldVersion"
				pods[6].Labels[apps.DefaultDeploymentUniqueLabelKey] = "oldVersion"
				pods[8].Labels[apps.DefaultDeploymentUniqueLabelKey] = "oldVersion"
				pods[1].Labels[apps.DefaultDeploymentUniqueLabelKey] = "newVersion"
				pods[3].Labels[apps.DefaultDeploymentUniqueLabelKey] = "newVersion"
				pods[5].Labels[apps.DefaultDeploymentUniqueLabelKey] = "newVersion"
				pods[7].Labels[apps.DefaultDeploymentUniqueLabelKey] = "newVersion"
				pods[9].Labels[apps.DefaultDeploymentUniqueLabelKey] = "newVersion"
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets = []appsv1alpha1.WorkloadSpreadSubset{
					{
						Name:        "subset-a",
						MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
					},
					{
						Name: "subset-b",
					},
				}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				clone := cloneSetDemo.DeepCopy()
				clone.Status.UpdateRevision = "newVersion"
				return clone
			},
			expectPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 10)
				for i := 0; i < 6; i++ {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				for i := 6; i < 10; i++ {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-b"}`,
					}
				}
				pods[0].Annotations[PodDeletionCostAnnotation] = "100"
				pods[0].Labels[apps.DefaultDeploymentUniqueLabelKey] = "oldVersion"
				pods[2].Annotations[PodDeletionCostAnnotation] = "100"
				pods[2].Labels[apps.DefaultDeploymentUniqueLabelKey] = "oldVersion"
				pods[4].Annotations[PodDeletionCostAnnotation] = "100"
				pods[4].Labels[apps.DefaultDeploymentUniqueLabelKey] = "oldVersion"
				pods[6].Annotations[PodDeletionCostAnnotation] = "200"
				pods[6].Labels[apps.DefaultDeploymentUniqueLabelKey] = "oldVersion"
				pods[8].Annotations[PodDeletionCostAnnotation] = "200"
				pods[8].Labels[apps.DefaultDeploymentUniqueLabelKey] = "oldVersion"

				pods[1].Annotations[PodDeletionCostAnnotation] = "200"
				pods[1].Labels[apps.DefaultDeploymentUniqueLabelKey] = "newVersion"
				pods[3].Annotations[PodDeletionCostAnnotation] = "200"
				pods[3].Labels[apps.DefaultDeploymentUniqueLabelKey] = "newVersion"
				pods[5].Annotations[PodDeletionCostAnnotation] = "200"
				pods[5].Labels[apps.DefaultDeploymentUniqueLabelKey] = "newVersion"
				pods[7].Annotations[PodDeletionCostAnnotation] = "100"
				pods[7].Labels[apps.DefaultDeploymentUniqueLabelKey] = "newVersion"
				pods[9].Annotations[PodDeletionCostAnnotation] = "100"
				pods[9].Labels[apps.DefaultDeploymentUniqueLabelKey] = "newVersion"
				return pods
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses = make([]appsv1alpha1.WorkloadSpreadSubsetStatus, 2)
				workloadSpread.Status.SubsetStatuses[0].Name = "subset-a"
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 0
				workloadSpread.Status.SubsetStatuses[0].Replicas = 6
				workloadSpread.Status.SubsetStatuses[1].Name = "subset-b"
				workloadSpread.Status.SubsetStatuses[1].MissingReplicas = -1
				workloadSpread.Status.SubsetStatuses[1].Replicas = 4

				workloadSpread.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					"oldVersion": {
						{
							Name:            "subset-a",
							MissingReplicas: 0,
							Replicas:        3,
						},
						{
							Name:            "subset-b",
							MissingReplicas: -1,
							Replicas:        2,
						},
					},
					"newVersion": {
						{
							Name:            "subset-a",
							MissingReplicas: 0,
							Replicas:        3,
						},
						{
							Name:            "subset-b",
							MissingReplicas: -1,
							Replicas:        2,
						},
					},
				}
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
		{
			name: "custom workload with replica path in whitelist",
			getWorkloads: func() []client.Object {
				clone := cloneSetDemo.DeepCopy()
				clone.Name = "workload"
				clone.Kind = "GameServerSet"
				clone.APIVersion = "mock.kruise.io/v1"
				clone.UID = "12345"
				clone.Spec.Replicas = utilpointer.Int32(14)
				unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(clone)
				if err != nil {
					panic("err when convert to unstructured object")
				}
				return []client.Object{&unstructured.Unstructured{Object: unstructuredMap}}
			},
			getPods: getTwoPodsWithDifferentLabels,
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := getWorkloadSpreadWithPercentSubsetB()
				workloadSpread.Spec.TargetFilter = nil
				return workloadSpread
			},
			getWhiteList: getWhiteListDemoCopy,
			expectPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
				}
				return []*corev1.Pod{pod}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := expectWorkloadSpreadWithPercentSubsetB()
				workloadSpread.Status.SubsetStatuses[0].Replicas = 2
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 3
				workloadSpread.Status.SubsetStatuses[1].Replicas = 0
				workloadSpread.Status.SubsetStatuses[1].MissingReplicas = 7
				return workloadSpread
			},
		},
		{
			name: "custom workload with target filter",
			getWorkloads: func() []client.Object {
				clone := cloneSetDemo.DeepCopy()
				clone.Name = "workload"
				clone.Kind = "GameServerSet"
				clone.APIVersion = "mock.kruise.io/v1"
				clone.UID = "12345"
				clone.Spec.RevisionHistoryLimit = utilpointer.Int32(18) // as replicas
				unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(clone)
				if err != nil {
					panic("err when convert to unstructured object")
				}
				return []client.Object{&unstructured.Unstructured{Object: unstructuredMap}}
			},
			getPods: func() []*corev1.Pod {
				pod1 := podDemo.DeepCopy()
				pod1.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: "mock.kruise.io/v1",
						Kind:       "GameServerSet",
						Name:       "workload",
						UID:        "12345",
					},
				}
				pod1.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
				}
				pod2 := pod1.DeepCopy()
				pod1.Labels["selected"] = "true"
				pod2.Labels["selected"] = "false"
				return []*corev1.Pod{pod1}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := getWorkloadSpreadWithPercentSubsetB()
				workloadSpread.Spec.TargetFilter.ReplicasPathList = []string{"spec.revisionHistoryLimit"}
				return workloadSpread
			},
			getWhiteList: getWhiteListDemoCopy,
			expectPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
				}
				return []*corev1.Pod{pod}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := expectWorkloadSpreadWithPercentSubsetB()
				workloadSpread.Status.SubsetStatuses[1].MissingReplicas = 9
				return workloadSpread
			},
		},
		{
			name: "custom workload without replicas",
			getWorkloads: func() []client.Object {
				clone := cloneSetDemo.DeepCopy()
				clone.Name = "workload"
				clone.Kind = "GameServerSet"
				clone.APIVersion = "mock.kruise.io/v1"
				clone.UID = "12345"
				// with no any replicas
				unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(clone)
				if err != nil {
					panic("err when convert to unstructured object")
				}
				return []client.Object{&unstructured.Unstructured{Object: unstructuredMap}}
			},
			getPods:           getTwoPodsWithDifferentLabels,
			getWorkloadSpread: getWorkloadSpreadWithPercentSubsetB,
			getWhiteList: func() configuration.WSCustomWorkloadWhiteList {
				whiteList := getWhiteListDemoCopy()
				whiteList.Workloads[0].ReplicasPath = "" // not configured
				return whiteList
			},
			expectPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
				}
				return []*corev1.Pod{pod}
			},
			expectWorkloadSpread: expectWorkloadSpreadWithPercentSubsetB,
		},
	}
	if !wsutil.EnabledWorkloadSetForVersionedStatus.Has("cloneset") {
		wsutil.EnabledWorkloadSetForVersionedStatus.Insert("cloneset")
		defer wsutil.EnabledWorkloadSetForVersionedStatus.Delete("cloneset")
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			currentTime = time.Now()
			workloadSpread := cs.getWorkloadSpread()
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workloadSpread).
				WithIndex(&corev1.Pod{}, fieldindex.IndexNameForOwnerRefUID, func(obj client.Object) []string {
					var owners []string
					for _, ref := range obj.GetOwnerReferences() {
						owners = append(owners, string(ref.UID))
					}
					return owners
				}).WithStatusSubresource(&appsv1alpha1.WorkloadSpread{})
			if cs.getCloneSet != nil {
				builder.WithObjects(cs.getCloneSet())
			}
			if cs.getWorkloads != nil {
				builder.WithObjects(cs.getWorkloads()...)
			}
			if cs.getWhiteList != nil {
				whiteList := cs.getWhiteList()
				marshaled, _ := json.Marshal(whiteList)
				builder.WithObjects(&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configuration.KruiseConfigurationName,
						Namespace: util.GetKruiseNamespace(),
					},
					Data: map[string]string{
						configuration.WSWatchCustomWorkloadWhiteList: string(marshaled),
					},
				})
			}
			for _, pod := range cs.getPods() {
				podIn := pod.DeepCopy()
				builder.WithObjects(podIn)
			}
			fakeClient := builder.Build()

			reconciler := ReconcileWorkloadSpread{
				Client:           fakeClient,
				recorder:         record.NewFakeRecorder(10),
				controllerFinder: &controllerfinder.ControllerFinder{Client: fakeClient},
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
					t.Fatalf("set Pod %v deletion-cost annotation failed", latestPodList[i].Name)
				}
			}

			latestWorkloadSpread, err := getLatestWorkloadSpread(fakeClient, workloadSpread)
			if err != nil {
				t.Fatalf("getLatestWorkloadSpread failed: %s", err.Error())
			}

			latestStatus := latestWorkloadSpread.Status.SubsetStatuses
			by, _ := json.Marshal(latestStatus)
			fmt.Println(string(by))

			exceptStatus := cs.expectWorkloadSpread().Status.SubsetStatuses
			by, _ = json.Marshal(exceptStatus)
			fmt.Println(string(by))

			if !apiequality.Semantic.DeepEqual(latestStatus, exceptStatus) {
				t.Fatalf("workloadSpread status DeepEqual failed")
			}

			if len(latestWorkloadSpread.Status.VersionedSubsetStatuses) > 1 {
				latestVersionedStatus := latestWorkloadSpread.Status.VersionedSubsetStatuses
				by, _ := json.Marshal(latestVersionedStatus)
				fmt.Println(string(by))

				exceptVersionedStatus := cs.expectWorkloadSpread().Status.VersionedSubsetStatuses
				by, _ = json.Marshal(exceptVersionedStatus)
				fmt.Println(string(by))

				if !apiequality.Semantic.DeepEqual(latestVersionedStatus, exceptVersionedStatus) {
					t.Fatalf("workloadSpread versioned status DeepEqual failed")
				}
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

	r := ReconcileWorkloadSpread{}
	versionedPodMap, subsetsPods, err := r.groupVersionedPods(workloadSpread, pods, 5)
	if err != nil {
		t.Fatalf("error group pods")
	}
	status, _ := r.calculateWorkloadSpreadStatus(workloadSpread, versionedPodMap, subsetsPods, 5)
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
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs.getCloneSet(), workloadSpread).
				WithIndex(&corev1.Pod{}, fieldindex.IndexNameForOwnerRefUID, func(obj client.Object) []string {
					var owners []string
					for _, ref := range obj.GetOwnerReferences() {
						owners = append(owners, string(ref.UID))
					}
					return owners
				}).Build()
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
				controllerFinder: &controllerfinder.ControllerFinder{Client: fakeClient},
			}

			durationStore = requeueduration.DurationStore{}

			nsn := types.NamespacedName{Namespace: workloadSpread.Namespace, Name: workloadSpread.Name}
			start := time.Now()
			result, _ := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: nsn})
			cost := time.Now().Sub(start)
			t.Logf("time cost %f seconds", cost.Seconds())
			if cs.expectRequeueAfter-result.RequeueAfter > 1*time.Second+cost {
				t.Fatalf("requeue key failed")
			}
		})
	}
}

func TestManagerExistingPods(t *testing.T) {
	cases := []struct {
		name              string
		getPods           func() []*corev1.Pod
		getNodes          func() []*corev1.Node
		getCloneSet       func() *appsv1alpha1.CloneSet
		getWorkloadSpread func() *appsv1alpha1.WorkloadSpread
		getExpectedResult func() (map[string]int32, int, int, int)
	}{
		{
			name: "manage the pods that were created before workloadSpread",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 10)
				// pods that were created before the workloadSpread
				// subset-a
				for i := 0; i < 2; i++ {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Spec.NodeName = "node-a"
					pods[i].Spec.Tolerations = []corev1.Toleration{{
						Key:    "schedule-system",
						Value:  "unified",
						Effect: corev1.TaintEffectNoSchedule,
					}}
					pods[i].Spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key:      "type",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"worker"},
							}},
						}},
					}}}
				}
				// pods that were created before the workloadSpread and injected old workloadSpread
				// subset-a
				for i := 2; i < 4; i++ {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Spec.NodeName = "node-a"
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"unmatched-workloadSpread","Subset":"subset-a"}`,
					}
					pods[i].Spec.Tolerations = []corev1.Toleration{{
						Key:    "schedule-system",
						Value:  "unified",
						Effect: corev1.TaintEffectNoSchedule,
					}}
					pods[i].Spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key:      "type",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"worker"},
							}},
						}},
					}}}
				}
				// pods that were created after the workloadSpread
				// subset-a
				for i := 4; i < 6; i++ {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				// pods that were created after the workloadSpread
				// subset-b
				for i := 6; i < 8; i++ {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-b"}`,
					}
				}
				// pods that cannot match any subset, without other ws annotations
				for i := 8; i < 9; i++ {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Spec.NodeName = "node-c"
					pods[i].Spec.Tolerations = []corev1.Toleration{{
						Key:    "node-taint",
						Value:  "node-c",
						Effect: corev1.TaintEffectNoSchedule,
					}}
				}
				// pods that cannot match any subset, with other ws annotations
				for i := 9; i < 10; i++ {
					pods[i] = podDemo.DeepCopy()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Spec.NodeName = "node-a"
					pods[i].Spec.Tolerations = []corev1.Toleration{{
						Key:    "node-taint",
						Value:  "node-c",
						Effect: corev1.TaintEffectNoSchedule,
					}}
					pods[i].Annotations = map[string]string{
						PodDeletionCostAnnotation:                     "200",
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-c"}`,
					}
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets = []appsv1alpha1.WorkloadSpreadSubset{
					{
						Name: "subset-a",
						RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "name-is",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"node-a"},
						}}},
						Tolerations: []corev1.Toleration{{
							Key:    "node-taint",
							Value:  "node-a",
							Effect: corev1.TaintEffectNoSchedule,
						}},
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-a"}}}`),
						},
						MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 5},
					},
					{
						Name: "subset-b",
						RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "name-is",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"node-b"},
						}}},
					},
				}
				workloadSpread.Status.SubsetStatuses = append(workloadSpread.Status.SubsetStatuses, appsv1alpha1.WorkloadSpreadSubsetStatus{
					Name:            "subset-b",
					MissingReplicas: int32(-1),
					CreatingPods:    map[string]metav1.Time{},
					DeletingPods:    map[string]metav1.Time{},
				})
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				cloneSet := cloneSetDemo.DeepCopy()
				cloneSet.Spec.Replicas = ptr.To(int32(10))
				return cloneSet
			},
			getNodes: func() []*corev1.Node {
				return []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "node-a",
							Labels: map[string]string{"name-is": "node-a", "type": "worker"},
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:       "node-taint",
									Value:     "node-a",
									Effect:    corev1.TaintEffectNoSchedule,
									TimeAdded: &metav1.Time{Time: time.Now()},
								},
								{
									Key:       "schedule-system",
									Value:     "unified",
									Effect:    corev1.TaintEffectNoSchedule,
									TimeAdded: &metav1.Time{Time: time.Now()},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "node-c",
							Labels: map[string]string{"name-is": "node-c"},
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{{
								Key:       "node-taint",
								Value:     "node-c",
								Effect:    corev1.TaintEffectNoSchedule,
								TimeAdded: &metav1.Time{Time: time.Now()},
							}},
						},
					},
				}
			},
			getExpectedResult: func() (expectedSubsetReplicas map[string]int32, expectedFreePodsCount,
				expectedWithinMaxReplicasCount, expectedBeyondMaxReplicasCount int) {
				expectedSubsetReplicas = map[string]int32{
					"subset-a": 6,
					"subset-b": 2,
				}
				expectedFreePodsCount = 2
				expectedWithinMaxReplicasCount = 7
				expectedBeyondMaxReplicasCount = 1
				return
			},
		},
		{
			name: "manage the pods that were created before workloadSpread, use preferredNodeSelectorTerms to match subset",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 3)
				pods[0] = podDemo.DeepCopy()
				pods[0].Name = fmt.Sprintf("test-pod-%d", 0)
				pods[0].Spec.NodeName = "worker-1"

				pods[1] = podDemo.DeepCopy()
				pods[1].Name = fmt.Sprintf("test-pod-%d", 1)
				pods[1].Spec.NodeName = "worker-2"

				pods[2] = podDemo.DeepCopy()
				pods[2].Name = fmt.Sprintf("test-pod-%d", 2)
				pods[2].Spec.NodeName = "worker-3"
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets = []appsv1alpha1.WorkloadSpreadSubset{
					{ // prefer nothing
						Name: "subset-a",
						Tolerations: []corev1.Toleration{{
							Key:      "node-taint",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						}},
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-a"}}}`),
						},
						MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
					},
					{ // prefer worker-2
						Name: "subset-b",
						Tolerations: []corev1.Toleration{{
							Key:      "node-taint",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						}},
						PreferredNodeSelectorTerms: []corev1.PreferredSchedulingTerm{
							{
								Weight: 200,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "type",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"worker-2"},
									}},
								},
							},
							{
								Weight: 100,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "type",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"worker-3"},
									}},
								},
							},
						},
						MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
					},
					{ // prefer worker-3
						Name: "subset-c",
						Tolerations: []corev1.Toleration{{
							Key:      "node-taint",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						}},
						PreferredNodeSelectorTerms: []corev1.PreferredSchedulingTerm{
							{
								Weight: 100,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "type",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"worker-2"},
									}},
								},
							},
							{
								Weight: 200,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "type",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"worker-3"},
									}},
								},
							},
						},
					},
				}
				workloadSpread.Status.SubsetStatuses = []appsv1alpha1.WorkloadSpreadSubsetStatus{
					{
						Name:            "subset-a",
						MissingReplicas: int32(3),
						CreatingPods:    map[string]metav1.Time{},
						DeletingPods:    map[string]metav1.Time{},
					},
					{
						Name:            "subset-b",
						MissingReplicas: int32(3),
						CreatingPods:    map[string]metav1.Time{},
						DeletingPods:    map[string]metav1.Time{},
					},
					{
						Name:            "subset-c",
						MissingReplicas: int32(-1),
						CreatingPods:    map[string]metav1.Time{},
						DeletingPods:    map[string]metav1.Time{},
					},
				}
				return workloadSpread
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				cloneSet := cloneSetDemo.DeepCopy()
				cloneSet.Spec.Replicas = ptr.To(int32(3))
				return cloneSet
			},
			getNodes: func() []*corev1.Node {
				return []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "worker-1",
							Labels: map[string]string{"type": "worker-1"},
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:       "node-taint",
									Value:     "whatever",
									Effect:    corev1.TaintEffectNoSchedule,
									TimeAdded: &metav1.Time{Time: time.Now()},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "worker-2",
							Labels: map[string]string{"type": "worker-2"},
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:       "node-taint",
									Value:     "whatever",
									Effect:    corev1.TaintEffectNoSchedule,
									TimeAdded: &metav1.Time{Time: time.Now()},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "worker-3",
							Labels: map[string]string{"type": "worker-3"},
						},
						Spec: corev1.NodeSpec{
							Taints: []corev1.Taint{
								{
									Key:       "node-taint",
									Value:     "whatever",
									Effect:    corev1.TaintEffectNoSchedule,
									TimeAdded: &metav1.Time{Time: time.Now()},
								},
							},
						},
					},
				}
			},
			getExpectedResult: func() (expectedSubsetReplicas map[string]int32, expectedFreePodsCount,
				expectedWithinMaxReplicasCount, expectedBeyondMaxReplicasCount int) {
				expectedSubsetReplicas = map[string]int32{
					"subset-a": 1,
					"subset-b": 1,
					"subset-c": 1,
				}
				expectedFreePodsCount = 0
				expectedWithinMaxReplicasCount = 3
				expectedBeyondMaxReplicasCount = 0
				return
			},
		},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			currentTime = time.Now()
			workloadSpread := cs.getWorkloadSpread()
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs.getCloneSet(), workloadSpread)
			builder.WithIndex(&corev1.Pod{}, fieldindex.IndexNameForOwnerRefUID, func(obj client.Object) []string {
				var owners []string
				for _, ref := range obj.GetOwnerReferences() {
					owners = append(owners, string(ref.UID))
				}
				return owners
			})
			for _, pod := range cs.getPods() {
				podIn := pod.DeepCopy()
				builder.WithObjects(podIn)
			}
			for _, node := range cs.getNodes() {
				nodeIn := node.DeepCopy()
				builder.WithObjects(nodeIn)
			}
			builder.WithStatusSubresource(&appsv1alpha1.WorkloadSpread{})
			fakeClient := builder.Build()

			reconciler := ReconcileWorkloadSpread{
				Client:           fakeClient,
				recorder:         record.NewFakeRecorder(10),
				controllerFinder: &controllerfinder.ControllerFinder{Client: fakeClient},
			}

			durationStore = requeueduration.DurationStore{}

			nsn := types.NamespacedName{Namespace: workloadSpread.Namespace, Name: workloadSpread.Name}
			_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: nsn})
			if err != nil {
				t.Fatalf("unexpected error when reconciling, err: %s", err.Error())
			}
			workloadSpread, err = getLatestWorkloadSpread(fakeClient, workloadSpread)
			if err != nil {
				t.Fatalf("get workloadspread failed, err: %s", err.Error())
			}

			expectedSubsetReplicas, expectedFreePodsCount, expectedWithinMaxReplicasCount, expectedBeyondMaxReplicasCount := cs.getExpectedResult()

			for i := range workloadSpread.Status.SubsetStatuses {
				subset := &workloadSpread.Status.SubsetStatuses[i]
				if subset.Replicas != expectedSubsetReplicas[workloadSpread.Status.SubsetStatuses[i].Name] {
					t.Fatalf("expected %d pods in %s, but got %d\n",
						expectedSubsetReplicas[workloadSpread.Status.SubsetStatuses[i].Name], subset.Name, subset.Replicas)
				}
			}

			pods, err := getLatestPods(fakeClient, workloadSpread)
			if err != nil {
				t.Fatalf("get workloadspread failed, err: %s", err.Error())
			}
			freePodsCount := 0
			beyondMaxReplicasCount := 0
			withinMaxReplicasCount := 0
			for _, pod := range pods {
				cost, _ := strconv.Atoi(pod.Annotations[PodDeletionCostAnnotation])
				if cost > 0 {
					withinMaxReplicasCount++
				} else if cost >= -100*len(workloadSpread.Spec.Subsets) {
					beyondMaxReplicasCount++
				} else {
					freePodsCount++
				}
			}
			if freePodsCount != expectedFreePodsCount {
				t.Fatalf("unexpected %d pods marked as minimum deletion cost, but got %d", expectedFreePodsCount, freePodsCount)
			}
			if withinMaxReplicasCount != expectedWithinMaxReplicasCount {
				t.Fatalf("unexpected %d pods marked positive deletion cost, but got %d", expectedWithinMaxReplicasCount, withinMaxReplicasCount)
			}
			if beyondMaxReplicasCount != expectedBeyondMaxReplicasCount {
				t.Fatalf("unexpected %d pods marked negative deletion cost, but got %d", expectedBeyondMaxReplicasCount, beyondMaxReplicasCount)
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
