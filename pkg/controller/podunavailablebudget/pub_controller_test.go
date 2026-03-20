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

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	utilpointer "k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
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
			Replicas: ptr.To[int32](10),
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
			Replicas: ptr.To[int32](10),
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
							Controller: ptr.To(true),
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
							Controller: ptr.To(true),
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
							Controller: ptr.To(true),
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
							Controller: ptr.To(true),
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
							Controller: ptr.To(true),
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
							Controller: ptr.To(true),
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
							Controller: ptr.To(true),
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
							Controller: ptr.To(true),
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
							Controller: ptr.To(true),
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
							Controller: ptr.To(true),
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
							Controller: ptr.To(true),
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
							Controller: ptr.To(true),
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
							Controller: ptr.To(true),
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
				object.Spec.Replicas = ptr.To[int32](100)
				return object
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = ptr.To[int32](100)
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
				object.Spec.Replicas = ptr.To[int32](100)
				return object
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = ptr.To[int32](100)
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
				object.Spec.Replicas = ptr.To[int32](100)
				return object
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = ptr.To[int32](100)
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
				object.Spec.Replicas = ptr.To[int32](100)
				return object
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = ptr.To[int32](100)
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
				object.Spec.Replicas = ptr.To[int32](100)
				return object
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = ptr.To[int32](100)
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
				object.Spec.Replicas = ptr.To[int32](100)
				return object
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = ptr.To[int32](100)
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
		// ============ PodGroupPolicy test cases ============
		// For podGroupPolicy scenarios, the workload (like LeaderWorkerSet) reports
		// replicas as the number of groups, not the total number of pods.
		{
			name: "podGroupPolicy: all groups available, maxUnavailable 30%",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				// 3 groups (g0, g1, g2), each with 2 pods, all ready
				var matchedPods []*corev1.Pod
				groups := []string{"g0", "g0", "g1", "g1", "g2", "g2"}
				for i, g := range groups {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("test-pod-%d", i)
					pod.Labels["group-index"] = g
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(3) // 3 groups
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj := replicaSetDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(3)
				return []*apps.ReplicaSet{obj}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.PodGroupPolicy = &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
					GroupLabelKey: "group-index",
					GroupSize:     ptr.To[int32](2),
				}
				pub.Spec.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "30%",
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
					// 3 groups total, expectedCount = 3 (group count after fix)
					// maxUnavailable = ceil(30% * 3) = 1
					// desiredAvailable = 3 - 1 = 2
					// currentAvailable = 3 (all 3 groups available)
					// unavailableAllowed = 3 - 2 = 1
					return policyv1alpha1.PodUnavailableBudgetStatus{
						UnavailableAllowed: 1,
						CurrentAvailable:   3,
						DesiredAvailable:   2,
						TotalReplicas:      3,
					}
				},
		},
		{
			name: "podGroupPolicy: all groups available, maxUnavailable 2 (int)",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				// 5 groups, each with 2 pods, all ready
				var matchedPods []*corev1.Pod
				for i := 0; i < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("test-pod-%d", i)
					pod.Labels["group-index"] = fmt.Sprintf("g%d", i/2)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(5) // 5 groups
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj := replicaSetDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(5)
				return []*apps.ReplicaSet{obj}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.PodGroupPolicy = &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
					GroupLabelKey: "group-index",
					GroupSize:     ptr.To[int32](2),
				}
				pub.Spec.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 2,
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				// 5 groups all available, expectedCount = 5 (group count after fix)
				// maxUnavailable = 2, desiredAvailable = 5-2 = 3
				// currentAvailable = 5, unavailableAllowed = 5-3 = 2
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 2,
					CurrentAvailable:   5,
					DesiredAvailable:   3,
					TotalReplicas:      5,
				}
			},
		},
		{
			name: "podGroupPolicy: one group has not-ready pod, maxUnavailable 2",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				// 5 groups, each with 2 pods. group g2 has 1 not-ready pod
				var matchedPods []*corev1.Pod
				for i := 0; i < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("test-pod-%d", i)
					pod.Labels["group-index"] = fmt.Sprintf("g%d", i/2)
					// pod-5 is in group g2, make it not-ready
					if i == 5 {
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
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(5) // 5 groups
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj := replicaSetDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(5)
				return []*apps.ReplicaSet{obj}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.PodGroupPolicy = &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
					GroupLabelKey: "group-index",
					GroupSize:     ptr.To[int32](2),
				}
				pub.Spec.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 2,
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				// 5 groups, expectedCount = 5 (group count after fix)
				// group g2 is unavailable (only 1 ready out of 2 needed)
				// currentAvailable = 4, maxUnavailable = 2
				// desiredAvailable = 5-2 = 3
				// unavailableAllowed = 4-3 = 1
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed:   1,
					CurrentAvailable:     4,
					DesiredAvailable:     3,
					TotalReplicas:        5,
					UnavailablePodGroups: map[string]metav1.Time{"g2": metav1.Now()},
				}
			},
		},
		{
			name: "podGroupPolicy: pod missing group label falls back to per-pod group",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				// 2 groups with label + 2 pods without label (treated as individual groups)
				var matchedPods []*corev1.Pod
				for i := 0; i < 4; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("test-pod-%d", i)
					if i < 2 {
						pod.Labels["group-index"] = "g0"
					}
					// pod 2 and 3 don't have group-index label
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(3) // 3 groups: g0, test-pod-2, test-pod-3
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj := replicaSetDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(3)
				return []*apps.ReplicaSet{obj}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.PodGroupPolicy = &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
					GroupLabelKey: "group-index",
					GroupSize:     ptr.To[int32](2),
				}
				pub.Spec.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 1,
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				// 3 groups: g0, test-pod-2, test-pod-3. expectedCount = 3 (group count after fix)
				// g0 available (2 ready >= groupSize 2), test-pod-2 unavailable (1 < 2), test-pod-3 unavailable (1 < 2)
				// currentAvailable = 1, maxUnavailable = 1
				// desiredAvailable = 3-1 = 2
				// unavailableAllowed = 1-2 = -1 -> 0
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 0,
					CurrentAvailable:   1,
					DesiredAvailable:   2,
					TotalReplicas:      3,
					UnavailablePodGroups: map[string]metav1.Time{
						"test-pod-2": metav1.Now(),
						"test-pod-3": metav1.Now(),
					},
				}
			},
		},
		{
			name: "podGroupPolicy: groupSize auto-derived (no explicit groupSize)",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				// 2 groups: g0 has 3 pods, g1 has 3 pods, all ready
				var matchedPods []*corev1.Pod
				for i := 0; i < 6; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("test-pod-%d", i)
					if i < 3 {
						pod.Labels["group-index"] = "g0"
					} else {
						pod.Labels["group-index"] = "g1"
					}
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(2) // 2 groups: g0, g1
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj := replicaSetDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(2)
				return []*apps.ReplicaSet{obj}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.PodGroupPolicy = &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
					GroupLabelKey: "group-index",
					// GroupSize is nil -> auto derived as max(2, max_group_size) = max(2,3) = 3
				}
				pub.Spec.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 1,
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				// GroupSize is nil -> default 2 (no auto-derive)
				// g0: 3 pods, 3 ready >= 2 -> available
				// g1: 3 pods, 3 ready >= 2 -> available
				// expectedCount = 2 (group count after fix), maxUnavailable = 1
				// desiredAvailable = 2-1 = 1, currentAvailable = 2
				// unavailableAllowed = 2-1 = 1
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 1,
					CurrentAvailable:   2,
					DesiredAvailable:   1,
					TotalReplicas:      2,
				}
			},
		},
		{
			name: "podGroupPolicy: groupSize explicitly set, all groups with enough ready pods",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				// 5 groups (g0..g4), each with 3 pods, all ready
				var matchedPods []*corev1.Pod
				for i := 0; i < 15; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("test-pod-%d", i)
					pod.Labels["group-index"] = fmt.Sprintf("g%d", i/3)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(5) // 5 groups
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj := replicaSetDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(5)
				return []*apps.ReplicaSet{obj}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.PodGroupPolicy = &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
					GroupLabelKey: "group-index",
					GroupSize:     ptr.To[int32](3),
				}
				pub.Spec.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 5,
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus{
				// 5 groups all available, expectedCount = 5 (group count after fix)
				// maxUnavailable = 5, desiredAvailable = max(0, 5-5) = 0
				// currentAvailable = 5, unavailableAllowed = 5-0 = 5
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 5,
					CurrentAvailable:   5,
					DesiredAvailable:   0,
					TotalReplicas:      5,
				}
			},
		},
		{
			name: "podGroupPolicy: multiple groups unavailable due to not-ready pods",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				// 4 groups (g0..g3), each with 2 pods
				// g0: both ready, g1: 1 not ready, g2: both not ready, g3: both ready
				var matchedPods []*corev1.Pod
				for i := 0; i < 8; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("test-pod-%d", i)
					pod.Labels["group-index"] = fmt.Sprintf("g%d", i/2)
					// g1: pod 3 not ready
					// g2: pod 4,5 not ready
					if i == 3 || i == 4 || i == 5 {
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
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(4) // 4 groups
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj := replicaSetDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(4)
				return []*apps.ReplicaSet{obj}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.PodGroupPolicy = &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
					GroupLabelKey: "group-index",
					GroupSize:     ptr.To[int32](2),
				}
				pub.Spec.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 2,
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				// 4 groups, expectedCount = 4 (group count after fix)
				// g0: available, g1: unavailable, g2: unavailable, g3: available
				// currentAvailable = 2, maxUnavailable = 2
				// desiredAvailable = 4-2 = 2
				// unavailableAllowed = 2-2 = 0
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 0,
					CurrentAvailable:   2,
					DesiredAvailable:   2,
					TotalReplicas:      4,
					UnavailablePodGroups: map[string]metav1.Time{
						"g1": metav1.Now(),
						"g2": metav1.Now(),
					},
				}
			},
		},
		{
			name: "podGroupPolicy with targetRef, all groups available",
			getPods: func(rs ...*apps.ReplicaSet) []*corev1.Pod {
				// 3 groups each with 2 pods, all ready
				var matchedPods []*corev1.Pod
				for i := 0; i < 6; i++ {
					pod := podDemo.DeepCopy()
					pod.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs[0].Name,
							UID:        rs[0].UID,
							Controller: ptr.To(true),
						},
					}
					pod.Name = fmt.Sprintf("test-pod-%d", i)
					pod.Labels["group-index"] = fmt.Sprintf("g%d", i/2)
					matchedPods = append(matchedPods, pod)
				}
				return matchedPods
			},
			getDeployment: func() *apps.Deployment {
				obj := deploymentDemo.DeepCopy()
				obj.Spec.Replicas = utilpointer.Int32(3) // 3 groups
				return obj
			},
			getReplicaSet: func() []*apps.ReplicaSet {
				obj := replicaSetDemo.DeepCopy()
				obj.Name = "nginx-rs-1"
				obj.Spec.Replicas = utilpointer.Int32(3)
				return []*apps.ReplicaSet{obj}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.TargetReference = &policyv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "nginx",
				}
				pub.Spec.PodGroupPolicy = &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
					GroupLabelKey: "group-index",
					GroupSize:     ptr.To[int32](2),
				}
				pub.Spec.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 1,
				}
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				// 3 groups all available, expectedCount = 3 (group count after fix)
				// maxUnavailable = 1, desiredAvailable = 3-1 = 2
				// currentAvailable = 3, unavailableAllowed = 3-2 = 1
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 1,
					CurrentAvailable:   3,
					DesiredAvailable:   2,
					TotalReplicas:      3,
				}
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
	for i := range expectStatus.UnavailablePodGroups {
		expectStatus.UnavailablePodGroups[i] = nTime
	}
	for i := range nowStatus.UnavailablePods {
		nowStatus.UnavailablePods[i] = nTime
	}
	for i := range nowStatus.DisruptedPods {
		nowStatus.DisruptedPods[i] = nTime
	}
	for i := range nowStatus.UnavailablePodGroups {
		nowStatus.UnavailablePodGroups[i] = nTime
	}

	return reflect.DeepEqual(expectStatus, nowStatus)
}

func TestGetGroupSize(t *testing.T) {
	cases := []struct {
		name       string
		pub        *policyv1alpha1.PodUnavailableBudget
		groupPods  map[string][]*corev1.Pod
		expectSize int32
	}{
		{
			name: "no PodGroupPolicy returns 0",
			pub: &policyv1alpha1.PodUnavailableBudget{
				Spec: policyv1alpha1.PodUnavailableBudgetSpec{},
			},
			groupPods:  map[string][]*corev1.Pod{"g0": {{}, {}}},
			expectSize: 0,
		},
		{
			name: "explicit GroupSize is used",
			pub: &policyv1alpha1.PodUnavailableBudget{
				Spec: policyv1alpha1.PodUnavailableBudgetSpec{
					PodGroupPolicy: &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
						GroupLabelKey: "key",
						GroupSize:     ptr.To[int32](5),
					},
				},
			},
			groupPods: map[string][]*corev1.Pod{
				"g0": {{}, {}, {}},
				"g1": {{}, {}},
			},
			expectSize: 5,
		},
		{
			name: "nil GroupSize defaults to 2",
			pub: &policyv1alpha1.PodUnavailableBudget{
				Spec: policyv1alpha1.PodUnavailableBudgetSpec{
					PodGroupPolicy: &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
						GroupLabelKey: "key",
					},
				},
			},
			groupPods: map[string][]*corev1.Pod{
				"g0": {{}, {}, {}, {}}, // 4 pods
				"g1": {{}, {}},         // 2 pods
			},
			expectSize: 2, // default 2 when GroupSize is nil
		},
		{
			name: "nil GroupSize defaults to 2 even with small groups",
			pub: &policyv1alpha1.PodUnavailableBudget{
				Spec: policyv1alpha1.PodUnavailableBudgetSpec{
					PodGroupPolicy: &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
						GroupLabelKey: "key",
					},
				},
			},
			groupPods: map[string][]*corev1.Pod{
				"g0": {{}}, // 1 pod
			},
			expectSize: 2, // max(2, 1) = 2
		},
		{
			name: "auto-derived GroupSize: empty groupPods returns 2",
			pub: &policyv1alpha1.PodUnavailableBudget{
				Spec: policyv1alpha1.PodUnavailableBudgetSpec{
					PodGroupPolicy: &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
						GroupLabelKey: "key",
					},
				},
			},
			groupPods:  map[string][]*corev1.Pod{},
			expectSize: 2,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			got := getGroupSize(cs.pub, cs.groupPods)
			if got != cs.expectSize {
				t.Fatalf("expected groupSize %d, got %d", cs.expectSize, got)
			}
		})
	}
}

func TestCountAvailablePodGroups(t *testing.T) {
	cases := []struct {
		name                 string
		groupPods            map[string][]*corev1.Pod
		unavailablePodGroups map[string]metav1.Time
		expectAvailable      int32
	}{
		{
			name: "all groups available",
			groupPods: map[string][]*corev1.Pod{
				"g0": {{}},
				"g1": {{}},
				"g2": {{}},
			},
			unavailablePodGroups: map[string]metav1.Time{},
			expectAvailable:      3,
		},
		{
			name: "one group unavailable",
			groupPods: map[string][]*corev1.Pod{
				"g0": {{}},
				"g1": {{}},
				"g2": {{}},
			},
			unavailablePodGroups: map[string]metav1.Time{
				"g1": metav1.Now(),
			},
			expectAvailable: 2,
		},
		{
			name: "all groups unavailable",
			groupPods: map[string][]*corev1.Pod{
				"g0": {{}},
				"g1": {{}},
			},
			unavailablePodGroups: map[string]metav1.Time{
				"g0": metav1.Now(),
				"g1": metav1.Now(),
			},
			expectAvailable: 0,
		},
		{
			name:                 "empty groups",
			groupPods:            map[string][]*corev1.Pod{},
			unavailablePodGroups: map[string]metav1.Time{},
			expectAvailable:      0,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			got := countAvailablePodGroups(cs.groupPods, cs.unavailablePodGroups)
			if got != cs.expectAvailable {
				t.Fatalf("expected %d available, got %d", cs.expectAvailable, got)
			}
		})
	}
}

func TestCountAvailableReplicas(t *testing.T) {
	// Initialize PubControl with a fake client so that countAvailablePods (non-group path) works.
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	pubcontrol.InitPubControl(fakeClient, nil, record.NewFakeRecorder(10))

	cases := []struct {
		name            string
		pub             *policyv1alpha1.PodUnavailableBudget
		groupPods       map[string][]*corev1.Pod
		disruptedPods   map[string]metav1.Time
		unavailPods     map[string]metav1.Time
		unavailGroups   map[string]metav1.Time
		expectAvailable int32
	}{
		{
			name: "without PodGroupPolicy, uses per-pod counting",
			pub: &policyv1alpha1.PodUnavailableBudget{
				Spec: policyv1alpha1.PodUnavailableBudgetSpec{},
			},
			groupPods: map[string][]*corev1.Pod{
				"pod-0": {podDemo.DeepCopy()},
				"pod-1": {podDemo.DeepCopy()},
				"pod-2": {podDemo.DeepCopy()},
			},
			disruptedPods:   map[string]metav1.Time{},
			unavailPods:     map[string]metav1.Time{},
			unavailGroups:   map[string]metav1.Time{},
			expectAvailable: 3,
		},
		{
			name: "with PodGroupPolicy, uses group counting",
			pub: &policyv1alpha1.PodUnavailableBudget{
				Spec: policyv1alpha1.PodUnavailableBudgetSpec{
					PodGroupPolicy: &policyv1alpha1.PodUnavailableBudgetPodGroupPolicy{
						GroupLabelKey: "group-index",
					},
				},
			},
			groupPods: map[string][]*corev1.Pod{
				"g0": {podDemo.DeepCopy(), podDemo.DeepCopy()},
				"g1": {podDemo.DeepCopy(), podDemo.DeepCopy()},
				"g2": {podDemo.DeepCopy(), podDemo.DeepCopy()},
			},
			disruptedPods: map[string]metav1.Time{},
			unavailPods:   map[string]metav1.Time{},
			unavailGroups: map[string]metav1.Time{
				"g1": metav1.Now(),
			},
			expectAvailable: 2,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			got := countAvailableReplicas(cs.pub, cs.groupPods, cs.disruptedPods, cs.unavailPods, cs.unavailGroups)
			if got != cs.expectAvailable {
				t.Fatalf("expected %d, got %d", cs.expectAvailable, got)
			}
		})
	}
}
