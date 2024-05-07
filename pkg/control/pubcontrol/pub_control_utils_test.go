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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openkruise/kruise/apis/apps/pub"
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
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
			UnavailablePods:    map[string]metav1.Time{},
			DisruptedPods:      map[string]metav1.Time{},
			UnavailableAllowed: 0,
			DesiredAvailable:   1,
		},
	}

	podDemo = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "nginx", "pub-controller": "true"},
			Annotations: map[string]string{
				PodRelatedPubAnnotation: pubDemo.Name,
			},
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

func TestPodUnavailableBudgetValidatePod(t *testing.T) {
	cases := []struct {
		name            string
		getPod          func() *corev1.Pod
		getPub          func() *policyv1alpha1.PodUnavailableBudget
		operation       policyv1alpha1.PubOperation
		expectAllow     bool
		expectPubStatus func() *policyv1alpha1.PodUnavailableBudgetStatus
	}{
		{
			name: "valid update pod, allow",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Status.UnavailableAllowed = 1
				return pub
			},
			operation:   policyv1alpha1.PubUpdateOperation,
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				pubStatus.UnavailablePods[podDemo.Name] = metav1.Now()
				pubStatus.UnavailableAllowed = 0
				return pubStatus
			},
		},
		{
			name: "valid update pod, reject",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			operation:   policyv1alpha1.PubUpdateOperation,
			expectAllow: false,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, pod deletion, ignore",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				return pod
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			operation:   policyv1alpha1.PubUpdateOperation,
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, pod not ready, ignore",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				podReadyCondition := podutil.GetPodReadyCondition(pod.Status)
				podReadyCondition.Status = corev1.ConditionFalse
				return pod
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			operation:   policyv1alpha1.PubUpdateOperation,
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, pod unavailable labels, ignore",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Labels[fmt.Sprintf("%sdata", appspub.PubUnavailablePodLabelPrefix)] = "true"
				return pod
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			operation:   policyv1alpha1.PubUpdateOperation,
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid delete pod, pod state is inconsistent(inplace update not completed yet), ignore",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[pub.InPlaceUpdateStateKey] = `{"nextContainerImages":{"main":"nginx:v2"}}`
				return pod
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			operation:   policyv1alpha1.PubDeleteOperation,
			expectAllow: true,
		},
		{
			name: "valid delete pod, pod declared no protect , ignore",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[policyv1alpha1.PodPubNoProtectionAnnotation] = "true"
				return pod
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			operation:   policyv1alpha1.PubDeleteOperation,
			expectAllow: true,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs.getPub()).
				WithStatusSubresource(&policyv1alpha1.PodUnavailableBudget{}).Build()
			finder := &controllerfinder.ControllerFinder{Client: fakeClient}
			InitPubControl(fakeClient, finder, record.NewFakeRecorder(10))
			allow, _, err := PodUnavailableBudgetValidatePod(cs.getPod(), cs.operation, "fake-user", false)
			if err != nil {
				t.Fatalf("PodUnavailableBudgetValidatePod failed: %s", err.Error())
			}
			if cs.expectAllow != allow {
				t.Fatalf("PodUnavailableBudgetValidatePod failed")
			}
		})
	}
}

func TestGetPodUnavailableBudgetForPod(t *testing.T) {
	cases := []struct {
		name          string
		getPod        func() *corev1.Pod
		getDeployment func() *apps.Deployment
		getReplicaSet func() *apps.ReplicaSet
		getPub        func() *policyv1alpha1.PodUnavailableBudget
		matchedPub    bool
	}{
		{
			name: "matched pub",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[PodRelatedPubAnnotation] = pubDemo.Name
				return pod
			},
			getDeployment: func() *apps.Deployment {
				dep := deploymentDemo.DeepCopy()
				return dep
			},
			getReplicaSet: func() *apps.ReplicaSet {
				rep := replicaSetDemo.DeepCopy()
				return rep
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.TargetReference = &policyv1alpha1.TargetReference{
					Name:       deploymentDemo.Name,
					Kind:       deploymentDemo.Kind,
					APIVersion: deploymentDemo.APIVersion,
				}
				return pub
			},
			matchedPub: true,
		},
		{
			name: "no matched pub targetRef deployment, for unequal ns",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[PodRelatedPubAnnotation] = pubDemo.Name
				return pod
			},
			getDeployment: func() *apps.Deployment {
				dep := deploymentDemo.DeepCopy()
				return dep
			},
			getReplicaSet: func() *apps.ReplicaSet {
				rep := replicaSetDemo.DeepCopy()
				return rep
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Namespace = "no-ns"
				pub.Spec.Selector = nil
				pub.Spec.TargetReference = &policyv1alpha1.TargetReference{
					Name:       deploymentDemo.Name,
					Kind:       deploymentDemo.Kind,
					APIVersion: deploymentDemo.APIVersion,
				}
				return pub
			},
			matchedPub: false,
		},
		{
			name: "no match, pub not found",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[PodRelatedPubAnnotation] = "o-pub"
				return pod
			},
			getDeployment: func() *apps.Deployment {
				dep := deploymentDemo.DeepCopy()
				return dep
			},
			getReplicaSet: func() *apps.ReplicaSet {
				rep := replicaSetDemo.DeepCopy()
				return rep
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"pub-controller": "false",
					},
				}
				return pub
			},
			matchedPub: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs.getDeployment(), cs.getReplicaSet(), cs.getPub()).Build()
			finder := &controllerfinder.ControllerFinder{Client: fakeClient}
			InitPubControl(fakeClient, finder, record.NewFakeRecorder(10))
			pod := cs.getPod()
			pub, err := PubControl.GetPubForPod(pod)
			if err != nil {
				t.Fatalf("GetPubForPod failed: %s", err.Error())
			}
			if cs.matchedPub && pub == nil {
				t.Fatalf("GetPubForPod failed")
			}
			if !cs.matchedPub && pub != nil {
				t.Fatalf("GetPubForPod failed")
			}
		})
	}
}
