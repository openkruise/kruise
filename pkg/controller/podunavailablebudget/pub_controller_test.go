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

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"

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
	_ = policyv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = apps.AddToScheme(scheme)
}

var (
	scheme *runtime.Scheme

	pubDemo = policyv1alpha1.PodUnavailableBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: policyv1alpha1.GroupVersion.String(),
			Kind:       "PodUnavailableBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "pub-test",
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
		getPods         func() []*corev1.Pod
		getDeployment   func() *apps.Deployment
		getReplicaSet   func() *apps.ReplicaSet
		getPub          func() *policyv1alpha1.PodUnavailableBudget
		expectPubStatus func() policyv1alpha1.PodUnavailableBudgetStatus
	}{
		{
			name: "selector no matched deployment",
			getPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Labels["pub-controller"] = "false"
				return []*corev1.Pod{pod}
			},
			getDeployment: func() *apps.Deployment {
				return deploymentDemo.DeepCopy()
			},
			getReplicaSet: func() *apps.ReplicaSet {
				return replicaSetDemo.DeepCopy()
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				return pubDemo.DeepCopy()
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{}
			},
		},
		{
			name: "selector no matched namespace deployment",
			getPods: func() []*corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Namespace = "other-ns"
				return []*corev1.Pod{pod}
			},
			getDeployment: func() *apps.Deployment {
				object := deploymentDemo.DeepCopy()
				object.Namespace = "other-ns"
				return object
			},
			getReplicaSet: func() *apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Namespace = "other-ns"
				return object
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				return pubDemo.DeepCopy()
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{}
			},
		},
		{
			name: "select matched deployment, TargetReference and maxUnavailable 30%",
			getPods: func() []*corev1.Pod {
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
			getReplicaSet: func() *apps.ReplicaSet {
				return replicaSetDemo.DeepCopy()
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
					UnavailableAllowed: 3,
					CurrentAvailable:   *deploymentDemo.Spec.Replicas,
					DesiredAvailable:   7,
					TotalReplicas:      *deploymentDemo.Spec.Replicas,
				}
			},
		},
		{
			name: "select matched deployment, selector and maxUnavailable 30%",
			getPods: func() []*corev1.Pod {
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
			getReplicaSet: func() *apps.ReplicaSet {
				return replicaSetDemo.DeepCopy()
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Name = "liheng"
				return pub
			},
			expectPubStatus: func() policyv1alpha1.PodUnavailableBudgetStatus {
				return policyv1alpha1.PodUnavailableBudgetStatus{
					UnavailableAllowed: 3,
					CurrentAvailable:   *deploymentDemo.Spec.Replicas,
					DesiredAvailable:   7,
					TotalReplicas:      *deploymentDemo.Spec.Replicas,
				}
			},
		},
		{
			name: "select matched deployment, maxUnavailable 0%",
			getPods: func() []*corev1.Pod {
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
			getReplicaSet: func() *apps.ReplicaSet {
				return replicaSetDemo.DeepCopy()
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
			getPods: func() []*corev1.Pod {
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
			getReplicaSet: func() *apps.ReplicaSet {
				return replicaSetDemo.DeepCopy()
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
			getPods: func() []*corev1.Pod {
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
			getReplicaSet: func() *apps.ReplicaSet {
				return replicaSetDemo.DeepCopy()
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
			getPods: func() []*corev1.Pod {
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
			getReplicaSet: func() *apps.ReplicaSet {
				return replicaSetDemo.DeepCopy()
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
					TotalReplicas:      5,
				}
			},
		},
		{
			name: "select matched deployment, minAvailable 50%",
			getPods: func() []*corev1.Pod {
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
			getReplicaSet: func() *apps.ReplicaSet {
				return replicaSetDemo.DeepCopy()
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
			getPods: func() []*corev1.Pod {
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
			getReplicaSet: func() *apps.ReplicaSet {
				return replicaSetDemo.DeepCopy()
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
			getPods: func() []*corev1.Pod {
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
			getReplicaSet: func() *apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return object
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
			getPods: func() []*corev1.Pod {
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
			getReplicaSet: func() *apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return object
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
			getPods: func() []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; i < 100; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					if i >= 20 && i < 25 {
						pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
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
			getReplicaSet: func() *apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return object
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
			getPods: func() []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; i < 100; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					if i >= 10 && i < 15 {
						pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
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
			getReplicaSet: func() *apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return object
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
			getPods: func() []*corev1.Pod {
				var matchedPods []*corev1.Pod
				for i := 0; i < 100; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
					if i >= 20 && i < 25 {
						pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
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
			getReplicaSet: func() *apps.ReplicaSet {
				object := replicaSetDemo.DeepCopy()
				object.Spec.Replicas = utilpointer.Int32Ptr(100)
				return object
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
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			pub := cs.getPub()
			fakeClient := fake.NewFakeClientWithScheme(scheme, cs.getDeployment(), cs.getReplicaSet(), pub)
			for _, pod := range cs.getPods() {
				podIn := pod.DeepCopy()
				err := fakeClient.Create(context.TODO(), podIn)
				if err != nil {
					t.Fatalf("create pod failed: %s", err.Error())
				}
			}
			reconciler := ReconcilePodUnavailableBudget{
				Client:           fakeClient,
				recorder:         record.NewFakeRecorder(10),
				controllerFinder: controllerfinder.NewControllerFinder(fakeClient),
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
				t.Fatalf("expect pub status(%v) but get(%v)", cs.expectPubStatus(), newPub.Status)
			}
			_ = util.GlobalCache.Delete(pub)
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
