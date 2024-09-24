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

package podunavailablebudget

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	"github.com/openkruise/kruise/pkg/util/fieldindex"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(policyv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(apps.AddToScheme(scheme))
}

var (
	scheme *runtime.Scheme

	pubDemo = policyv1alpha1.PodUnavailableBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: policyv1alpha1.GroupVersion.String(),
			Kind:       "PodUnavailableBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "default",
			Name:        "pub-test",
			Annotations: map[string]string{},
		},
		Spec: policyv1alpha1.PodUnavailableBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"pub-controller": "true",
				},
			},
			MaxUnavailable: &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "30%",
			},
		},
		Status: policyv1alpha1.PodUnavailableBudgetStatus{
			UnavailablePods: map[string]metav1.Time{},
			DisruptedPods:   map[string]metav1.Time{},
		},
	}

	podDemo = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pod",
			Namespace:   "default",
			Labels:      map[string]string{"app": "nginx", "pub-controller": "true"},
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "nginx",
					UID:        types.UID("606132e0-85ef-460a-8cf5-cd8f915a8cc3"),
					Controller: utilpointer.BoolPtr(true),
				},
			},
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
					Status: corev1.ConditionTrue,
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:    "nginx",
					Image:   "nginx:v1",
					ImageID: "nginx@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d",
					Ready:   true,
				},
			},
		},
	}

	deploymentDemo = &apps.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
			UID:       types.UID("f6d5b184-d82f-461c-a432-fbd59e2f0379"),
		},
		Spec: apps.DeploymentSpec{
			Replicas: utilpointer.Int32Ptr(10),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
		},
	}

	replicaSetDemo = &apps.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ReplicaSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "nginx",
					UID:        types.UID("f6d5b184-d82f-461c-a432-fbd59e2f0379"),
					Controller: utilpointer.BoolPtr(true),
				},
			},
			UID: types.UID("606132e0-85ef-460a-8cf5-cd8f915a8cc3"),
			Labels: map[string]string{
				"app": "nginx",
			},
		},
		Spec: apps.ReplicaSetSpec{
			Replicas: utilpointer.Int32Ptr(10),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
		},
	}
)

func TestPubReconcile(t *testing.T) {
	cases := []struct {
		name            string
		getPods         func(rs ...*apps.ReplicaSet) []*corev1.Pod
		getDeployment   func() *apps.Deployment
		getReplicaSet   func() []*apps.ReplicaSet
		getPub          func() *policyv1alpha1.PodUnavailableBudget
		expectPubStatus func() policyv1alpha1.PodUnavailableBudgetStatus
	}{
		{
			name: "select matched deployment(replicas=0), selector and maxUnavailable 30%",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < 5; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[0].Name,
							UID:        rs[0].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				for i := 5; int32(i) < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[1].Name,
							UID:        rs[1].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(0)
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj1 := replicaSetDemo.DeepCopy()
				obj1.Name = "nginx-rs-1"
				obj2 := replicaSetDemo.DeepCopy()
				obj2.Name = "nginx-rs-2"
				obj2.UID = "a34b0453-3426-4685-a79c-752e7062a523"
				return []*apps.ReplicaSet{obj1, obj2}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 10,
					CurrentAvailable:   10,
					DesiredAvailable:   0,
					TotalReplicas:      0,
				}
			},
		},
		{
			name: "select matched pub.annotations[pub.kruise.io/protect-total-replicas]=15 and selector, selector and maxUnavailable 30%",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < 5; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[0].Name,
							UID:        rs[0].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				for i := 5; int32(i) < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[1].Name,
							UID:        rs[1].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(0)
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj1 := replicaSetDemo.DeepCopy()
				obj1.Name = "nginx-rs-1"
				obj2 := replicaSetDemo.DeepCopy()
				obj2.Name = "nginx-rs-2"
				obj2.UID = "a34b0453-3426-4685-a79c-752e7062a523"
				return []*apps.ReplicaSet{obj1, obj2}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Annotations[policyv1alpha1.PubProtectTotalReplicasAnnotation] = "15"
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 0,
					CurrentAvailable:   10,
					DesiredAvailable:   10,
					TotalReplicas:      15,
				}
			},
		},
		{
			name: "select matched deployment(replicas=10,maxSurge=30%,maxUnavailable=0), and pub(selector,maxUnavailable=30%)",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[0].Name,
							UID:        rs[0].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				for i := 10; int32(i) < 13; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[1].Name,
							UID:        rs[1].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					if i == 12 {
						pod.Status.Conditions = []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionFalse,
							},
						}
					}
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				return deploymentDemo.DeepCopy()
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj1 := replicaSetDemo.DeepCopy()
				obj1.Name = "nginx-rs-1"
				obj2 := replicaSetDemo.DeepCopy()
				obj2.Name = "nginx-rs-2"
				obj2.UID = "a34b0453-3426-4685-a79c-752e7062a523"
				return []*apps.ReplicaSet{obj1, obj2}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 5,
					CurrentAvailable:   12,
					DesiredAvailable:   7,
					TotalReplicas:      10,
				}
			},
		},
		{
			name: "select matched deployment(replicas=10,maxSurge=0,maxUnavailable=30%), and pub(selector,maxUnavailable=30%)",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[0].Name,
							UID:        rs[0].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					t := metav1.Now()
					if i >= 7 && i < 10 {
						pod.DeletionTimestamp = &t
						pod.Finalizers = []string{"finalizers.sigs.k8s.io/test"}
					}
					matchedPods = append(matchedPods, pod)
				}
				for i := 10; int32(i) < 13; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[1].Name,
							UID:        rs[1].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					if i == 12 {
						pod.Status.Conditions = []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionFalse,
							},
						}
					}
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				return deploymentDemo.DeepCopy()
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj1 := replicaSetDemo.DeepCopy()
				obj1.Name = "nginx-rs-1"
				obj2 := replicaSetDemo.DeepCopy()
				obj2.Name = "nginx-rs-2"
				obj2.UID = "a34b0453-3426-4685-a79c-752e7062a523"
				return []*apps.ReplicaSet{obj1, obj2}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 2,
					CurrentAvailable:   9,
					DesiredAvailable:   7,
					TotalReplicas:      10,
				}
			},
		},
		{
			name: "select matched deployment(Deletion), selector and maxUnavailable 30%",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < *deploymentDemo.Spec.Replicas; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				t := metav1.Now()
				obj.DeletionTimestamp = &t
				obj.Finalizers = []string{"finalizers.sigs.k8s.io/test"}
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				return []*apps.ReplicaSet{replicaSetDemo.DeepCopy()}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: *deploymentDemo.Spec.Replicas,
					CurrentAvailable:   *deploymentDemo.Spec.Replicas,
					DesiredAvailable:   0,
					TotalReplicas:      0,
				}
			},
		},
		{
			name: "select matched deployment(replicas=0,maxSurge=0,maxUnavailable=30%), and pub(targetRef,maxUnavailable=30%)",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[0].Name,
							UID:        rs[0].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(0)
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj1 := replicaSetDemo.DeepCopy()
				obj1.Name = "nginx-rs-1"
				return []*apps.ReplicaSet{obj1}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.TargetReference = &policyv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "nginx",
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 0,
					CurrentAvailable:   0,
					DesiredAvailable:   0,
					TotalReplicas:      0,
				}
			},
		},
		{
			name: "select matched deployment(replicas=1,maxSurge=0,maxUnavailable=30%), pub.kruise.io/protect-total-replicas=15 and pub(targetRef,maxUnavailable=30%)",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[0].Name,
							UID:        rs[0].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(1)
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj1 := replicaSetDemo.DeepCopy()
				obj1.Name = "nginx-rs-1"
				return []*apps.ReplicaSet{obj1}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.TargetReference = &policyv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "nginx",
				}
				pub.Annotations[policyv1alpha1.PubProtectTotalReplicasAnnotation] = "15"
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 0,
					CurrentAvailable:   10,
					DesiredAvailable:   10,
					TotalReplicas:      15,
				}
			},
		},
		{
			name: "select matched deployment(replicas=1,maxSurge=0,maxUnavailable=30%), and pub(targetRef,maxUnavailable=30%)",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[0].Name,
							UID:        rs[0].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(1)
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj1 := replicaSetDemo.DeepCopy()
				obj1.Name = "nginx-rs-1"
				return []*apps.ReplicaSet{obj1}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.TargetReference = &policyv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "nginx",
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 10,
					CurrentAvailable:   10,
					DesiredAvailable:   0,
					TotalReplicas:      1,
				}
			},
		},
		{
			name: "select matched deployment(replicas=10,maxSurge=0,maxUnavailable=30%), and pub(targetRef,maxUnavailable=30%)",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[0].Name,
							UID:        rs[0].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					if i >= 7 {
						t := metav1.Now()
						pod.DeletionTimestamp = &t
						pod.Finalizers = []string{"finalizers.sigs.k8s.io/test"}
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(10)
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj1 := replicaSetDemo.DeepCopy()
				obj1.Name = "nginx-rs-1"
				return []*apps.ReplicaSet{obj1}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.TargetReference = &policyv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "nginx",
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 0,
					CurrentAvailable:   7,
					DesiredAvailable:   7,
					TotalReplicas:      10,
				}
			},
		},
		{
			name: "select matched deployment(deletion), and pub(targetRef,maxUnavailable=30%)",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[0].Name,
							UID:        rs[0].UID,
							Controller: utilpointer.BoolPtr(true),
						},
					}
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				t := metav1.Now()
				obj.DeletionTimestamp = &t
				obj.Finalizers = []string{"finalizers.sigs.k8s.io/test"}
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj1 := replicaSetDemo.DeepCopy()
				obj1.Name = "nginx-rs-1"
				return []*apps.ReplicaSet{obj1}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.TargetReference = &policyv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "nginx",
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 0,
					CurrentAvailable:   0,
					DesiredAvailable:   0,
					TotalReplicas:      0,
				}
			},
		},
		{
			name: "select matched deployment, maxUnavailable 0%",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < *deploymentDemo.Spec.Replicas; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				return deploymentDemo.DeepCopy()
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				return []*apps.ReplicaSet{replicaSetDemo.DeepCopy()}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "0%",
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 0,
					CurrentAvailable:   *deploymentDemo.Spec.Replicas,
					DesiredAvailable:   *deploymentDemo.Spec.Replicas,
					TotalReplicas:      *deploymentDemo.Spec.Replicas,
				}
			},
		},
		{
			name: "select matched deployment, maxUnavailable 100%",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < *deploymentDemo.Spec.Replicas; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				return deploymentDemo.DeepCopy()
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				return []*apps.ReplicaSet{replicaSetDemo.DeepCopy()}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "100%",
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: *deploymentDemo.Spec.Replicas,
					CurrentAvailable:   *deploymentDemo.Spec.Replicas,
					DesiredAvailable:   0,
					TotalReplicas:      *deploymentDemo.Spec.Replicas,
				}
			},
		},
		{
			name: "select matched deployment, maxUnavailable 100, and expect scale 10",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; int32(i) < *deploymentDemo.Spec.Replicas; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				return deploymentDemo.DeepCopy()
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				return []*apps.ReplicaSet{replicaSetDemo.DeepCopy()}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: *deploymentDemo.Spec.Replicas,
					CurrentAvailable:   *deploymentDemo.Spec.Replicas,
					DesiredAvailable:   0,
					TotalReplicas:      *deploymentDemo.Spec.Replicas,
				}
			},
		},
		{
			name: "select matched deployment, minAvailable 100 int",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; i < 5; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				return deploymentDemo.DeepCopy()
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				return []*apps.ReplicaSet{replicaSetDemo.DeepCopy()}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.MaxUnavailable = nil
				pub.Spec.MinAvailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 0,
					CurrentAvailable:   5,
					DesiredAvailable:   100,
					TotalReplicas:      10,
				}
			},
		},
		{
			name: "select matched deployment, minAvailable 50%",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; i < 5; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				return deploymentDemo.DeepCopy()
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				return []*apps.ReplicaSet{replicaSetDemo.DeepCopy()}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.MaxUnavailable = nil
				pub.Spec.MinAvailable = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "50%",
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 0,
					CurrentAvailable:   5,
					DesiredAvailable:   5,
					TotalReplicas:      10,
				}
			},
		},
		{
			name: "select matched deployment, minAvailable 80%",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; i < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				return deploymentDemo.DeepCopy()
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				return []*apps.ReplicaSet{replicaSetDemo.DeepCopy()}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.MaxUnavailable = nil
				pub.Spec.MinAvailable = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "80%",
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 2,
					CurrentAvailable:   10,
					DesiredAvailable:   8,
					TotalReplicas:      10,
				}
			},
		},
		{
			name: "select matched deployment, 10 UnavailablePods and 10 DisruptionPods",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; i < 100; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				object := deploymentDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return object
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return []*apps.ReplicaSet{object}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				for i := 0; i < 10; i++ {
					pub.Status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				for i := 10; i < 20; i++ {
					pub.Status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				status := pubDemo.Status.DeepCopy()
				for i := 0; i < 10; i++ {
					status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				for i := 10; i < 20; i++ {
					status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				status.TotalReplicas = 100
				status.DesiredAvailable = 70
				status.CurrentAvailable = 80
				status.UnavailableAllowed = 10
				return *status
			},
		},
		{
			name: "select matched deployment, 10 UnavailablePods, 10 DisruptionPods and 5 not ready",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; i < 100; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					if i >= 20 && i < 25 {
						readyCondition := podutil.GetPodReadyCondition(pod.Status)
						readyCondition.Status = corev1.ConditionFalse
					}
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				object := deploymentDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return object
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return []*apps.ReplicaSet{object}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				for i := 0; i < 10; i++ {
					pub.Status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				for i := 10; i < 20; i++ {
					pub.Status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				status := pubDemo.Status.DeepCopy()
				for i := 0; i < 10; i++ {
					status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				for i := 10; i < 20; i++ {
					status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				status.TotalReplicas = 100
				status.DesiredAvailable = 70
				status.CurrentAvailable = 75
				status.UnavailableAllowed = 5
				return *status
			},
		},
		{
			name: "select matched deployment, 10 UnavailablePods, 10 DisruptionPods and 5 deletion",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; i < 100; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					if i >= 20 && i < 25 {
						pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
						pod.Finalizers = []string{"finalizers.sigs.k8s.io/test"}
					}
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				object := deploymentDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return object
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return []*apps.ReplicaSet{object}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				for i := 0; i < 10; i++ {
					pub.Status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				for i := 10; i < 20; i++ {
					pub.Status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				status := pubDemo.Status.DeepCopy()
				for i := 0; i < 10; i++ {
					status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				for i := 10; i < 20; i++ {
					status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				status.TotalReplicas = 100
				status.DesiredAvailable = 70
				status.CurrentAvailable = 75
				status.UnavailableAllowed = 5
				return *status
			},
		},
		{
			name: "select matched deployment, 10 UnavailablePods(5 ready), 10 DisruptionPods(5 deletion)",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; i < 100; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					if i >= 10 && i < 15 {
						pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
						pod.Finalizers = []string{"finalizers.sigs.k8s.io/test"}
					}
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				object := deploymentDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return object
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return []*apps.ReplicaSet{object}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				for i := 0; i < 10; i++ {
					if i >= 0 && i < 5 {
						pub.Status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Time{Time: time.Now().Add(-10 * time.Second)}
					} else {
						pub.Status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
					}
				}
				for i := 10; i < 20; i++ {
					pub.Status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				status := pubDemo.Status.DeepCopy()
				for i := 5; i < 10; i++ {
					status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				for i := 15; i < 20; i++ {
					status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				status.TotalReplicas = 100
				status.DesiredAvailable = 70
				status.CurrentAvailable = 85
				status.UnavailableAllowed = 15
				return *status
			},
		},
		{
			name: "select matched deployment, 10 UnavailablePods(5 ready), 10 DisruptionPods(5 delay) and 5 deletion",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; i < 100; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					if i >= 20 && i < 25 {
						pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
						pod.Finalizers = []string{"finalizers.sigs.k8s.io/test"}
					}
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				object := deploymentDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return object
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return []*apps.ReplicaSet{object}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				for i := 0; i < 10; i++ {
					if i >= 0 && i < 5 {
						pub.Status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Time{Time: time.Now().Add(-10 * time.Second)}
					} else {
						pub.Status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
					}
				}
				for i := 10; i < 20; i++ {
					if i >= 10 && i < 15 {
						pub.Status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Time{Time: time.Now().Add(-125 * time.Second)}
					} else {
						pub.Status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
					}
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				status := pubDemo.Status.DeepCopy()
				for i := 5; i < 10; i++ {
					status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				for i := 15; i < 20; i++ {
					status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				status.TotalReplicas = 100
				status.DesiredAvailable = 70
				status.CurrentAvailable = 85
				status.UnavailableAllowed = 15
				return *status
			},
		},
		{
			name: "test select matched deployment, 10 UnavailablePods(5 ready), 10 DisruptionPods(5 delay) and 5 deletion",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; i < 100; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = nil
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					if i >= 20 && i < 25 {
						pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
						pod.Finalizers = []string{"finalizers.sigs.k8s.io/test"}
					}
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				object := deploymentDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return object
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return []*apps.ReplicaSet{object}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()

				pub.Annotations[policyv1alpha1.PubProtectTotalReplicasAnnotation] = "50"
				for i := 0; i < 10; i++ {
					if i >= 0 && i < 5 {
						pub.Status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Time{Time: time.Now().Add(-10 * time.Second)}
					} else {
						pub.Status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
					}
				}
				for i := 10; i < 20; i++ {
					if i >= 10 && i < 15 {
						pub.Status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Time{Time: time.Now().Add(-125 * time.Second)}
					} else {
						pub.Status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
					}
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				status := pubDemo.Status.DeepCopy()
				for i := 5; i < 10; i++ {
					status.UnavailablePods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				for i := 15; i < 20; i++ {
					status.DisruptedPods[fmt.Sprintf("test-pod-%d", i)] = metav1.Now()
				}
				status.TotalReplicas = 50
				status.DesiredAvailable = 35
				status.CurrentAvailable = 85
				status.UnavailableAllowed = 50
				return *status
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			pub := cs.getPub()
			defer util.GlobalCache.Delete(pub)

			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs.getDeployment(), pub)
			builder.WithIndex(&corev1.Pod{}, fieldindex.IndexNameForOwnerRefUID, func(obj client.Object) []string {
				var owners []string
				for _, ref := range obj.GetOwnerReferences() {
					owners = append(owners, string(ref.UID))
				}
				return owners
			})
			builder.WithStatusSubresource(&policyv1alpha1.PodUnavailableBudget{})
			for _, pod := range cs.getPods(cs.getReplicaSet()...) {
				podIn := pod.DeepCopy()
				builder.WithObjects(podIn)
			}
			for _, obj := range cs.getReplicaSet() {
				builder.WithObjects(obj)
			}
			fakeClient := builder.Build()

			finder := &controllerfinder.ControllerFinder{Client: fakeClient}
			pubcontrol.InitPubControl(fakeClient, finder, record.NewFakeRecorder(10))
			controllerfinder.Finder = &controllerfinder.ControllerFinder{Client: fakeClient}
			reconciler := ReconcilePodUnavailableBudget{
				Client:           fakeClient,
				recorder:         record.NewFakeRecorder(10),
				controllerFinder: &controllerfinder.ControllerFinder{Client: fakeClient},
			}
			_, err := reconciler.syncPodUnavailableBudget(pub)
			if err != nil {
				t.Fatalf("sync PodUnavailableBudget failed: %s", err.Error())
			}
			newPub, err := getLatestPub(fakeClient, pub)
			if err != nil {
				t.Fatalf("getLatestPub failed: %s", err.Error())
			}
			if !isPubStatusEqual(cs.expectPubStatus(), newPub.Status) {
				t.Fatalf("expect pub status(%s) but get(%s)", util.DumpJSON(cs.expectPubStatus()), util.DumpJSON(newPub.Status))
			}
		})
	}
}

func TestDesiredAvailableForPub(t *testing.T) {
	cases := []struct {
		name             string
		getPub           func() *policyv1alpha1.PodUnavailableBudget
		totalReplicas    int32
		desiredAvailable int32
	}{
		{
			name: "DesiredAvailableForPub, maxUnavailable 10%, total 15",
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				demo := pubDemo.DeepCopy()
				demo.Spec.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "10%",
				}
				return demo
			},
			totalReplicas:    15,
			desiredAvailable: 13,
		},
	}

	rec := ReconcilePodUnavailableBudget{}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			expect, _ := rec.getDesiredAvailableForPub(cs.getPub(), cs.totalReplicas)
			if expect != cs.desiredAvailable {
				t.Fatalf("expect %d, but get %d", cs.desiredAvailable, expect)
			}
		})
	}
}

func getLatestPub(client client.Client, pub *policyv1alpha1.PodUnavailableBudget) (*policyv1alpha1.PodUnavailableBudget, error) {
	newPub := &policyv1alpha1.PodUnavailableBudget{}
	key := types.NamespacedName{
		Namespace: pub.Namespace,
		Name:      pub.Name,
	}
	err := client.Get(context.TODO(), key, newPub)
	return newPub, err
}

func isPubStatusEqual(expectStatus, nowStatus policyv1alpha1.PodUnavailableBudgetStatus) bool {
	nTime := metav1.Now()
	for i := range expectStatus.UnavailablePods {
		expectStatus.UnavailablePods[i] = nTime
	}
	for i := range expectStatus.DisruptedPods {
		expectStatus.DisruptedPods[i] = nTime
	}
	for i := range nowStatus.UnavailablePods {
		nowStatus.UnavailablePods[i] = nTime
	}
	for i := range nowStatus.DisruptedPods {
		nowStatus.DisruptedPods[i] = nTime
	}

	return reflect.DeepEqual(expectStatus, nowStatus)
}
