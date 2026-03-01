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

package validating

import (
	"context"
	"reflect"
	"testing"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/apis/policy"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(policyv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
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
					"app": "pub-controller",
				},
			},
			MaxUnavailable: &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "30%",
			},
		},
		Status: policyv1alpha1.PodUnavailableBudgetStatus{
			DisruptedPods: map[string]metav1.Time{
				"test-pod-9": metav1.Now(),
			},
			UnavailablePods: map[string]metav1.Time{
				"test-pod-8": metav1.Now(),
			},
			UnavailableAllowed: 0,
			CurrentAvailable:   7,
			DesiredAvailable:   7,
			TotalReplicas:      10,
		},
	}

	podDemo = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-0",
			Namespace: "default",
			Labels:    map[string]string{"app": "pub-controller"},
			Annotations: map[string]string{
				pubcontrol.PodRelatedPubAnnotation: pubDemo.Name,
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
)

func TestValidateUpdatePodForPub(t *testing.T) {
	cases := []struct {
		name            string
		oldPod          func() *corev1.Pod
		newPod          func() *corev1.Pod
		pub             func() *policyv1alpha1.PodUnavailableBudget
		subresource     string
		expectAllow     bool
		expectPubStatus func() *policyv1alpha1.PodUnavailableBudgetStatus
	}{
		{
			name: "valid update pod, allow",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Spec.Containers[0].Image = "nginx:1.18"
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Status.CurrentAvailable = 8
				pub.Status.UnavailableAllowed = 1
				return pub
			},
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				pubStatus.UnavailablePods["test-pod-0"] = metav1.Now()
				pubStatus.CurrentAvailable = 8
				pubStatus.UnavailableAllowed = 0
				return pubStatus
			},
		},
		{
			name: "valid update pod, reject",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Spec.Containers[0].Image = "nginx:1.18"
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectAllow: false,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, pod deletion, ignore",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Spec.Containers[0].Image = "nginx:1.18"
				pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, pod status subresource, ignore",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				podReadyCondition := podutil.GetPodReadyCondition(pod.Status)
				podReadyCondition.Status = corev1.ConditionFalse
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			subresource: "status",
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, pod not ready, ignore",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				podReadyCondition := podutil.GetPodReadyCondition(pod.Status)
				podReadyCondition.Status = corev1.ConditionFalse
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				podReadyCondition := podutil.GetPodReadyCondition(pod.Status)
				podReadyCondition.Status = corev1.ConditionFalse
				pod.Spec.Containers[0].Image = "nginx:1.18"
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, label and annotations, ignore",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Labels["test"] = "labelb"
				pod.Annotations["ab"] = "annob"
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, sidecar container consistent and ready, reject",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
					Name:  "sidecar-mesh",
					Image: "envoy:v1",
					Env: []corev1.EnvVar{
						{
							Name:  sidecarcontrol.SidecarEnvKey,
							Value: "true",
						},
					},
				})
				pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, corev1.ContainerStatus{
					Name:    "sidecar-mesh",
					Image:   "envoy:v1",
					ImageID: "envoy@sha256:f7108338109c3a0b94d2df4734fc480aac7caefc0e6fa4b11951216c2bec6dcf",
					Ready:   true,
				})
				pod.Annotations[sidecarcontrol.SidecarsetInplaceUpdateStateKey] = `{"test-sidecarset": {"revision":"new-revision","lastContainerStatuses":{"sidecar-mesh":{"imageID":"envoy@sha256:1ba0da74b20aad52b091877b0e0ece503c563f39e37aa6b0e46777c4d820a2ae"}}}}`
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
					Name:  "sidecar-mesh",
					Image: "envoy:v2",
					Env: []corev1.EnvVar{
						{
							Name:  sidecarcontrol.SidecarEnvKey,
							Value: "true",
						},
					},
				})
				pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, corev1.ContainerStatus{
					Name:    "sidecar-mesh",
					Image:   "envoy:v1",
					ImageID: "envoy@sha256:f7108338109c3a0b94d2df4734fc480aac7caefc0e6fa4b11951216c2bec6dcf",
					Ready:   true,
				})
				pod.Annotations[sidecarcontrol.SidecarsetInplaceUpdateStateKey] = `{"test-sidecarset": {"revision":"new-revision","lastContainerStatuses":{"sidecar-mesh":{"imageID":"envoy@sha256:1ba0da74b20aad52b091877b0e0ece503c563f39e37aa6b0e46777c4d820a2ae"}}}}`
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectAllow: false,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, main container consistent and ready, reject",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Labels[apps.ControllerRevisionHashLabelKey] = "new-revision"
				pod.Annotations[appspub.InPlaceUpdateStateKey] = `{"revision":"new-revision","lastContainerStatuses":{"nginx":{"imageID":"nginx@sha256:53f029ad8b1058e966d6714a30d2582943c6e936f4b3c3b344a302c4567f471d"}}}`
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Spec.Containers[0].Image = "nginx:v2"
				pod.Labels[apps.ControllerRevisionHashLabelKey] = "new-revision"
				pod.Annotations[appspub.InPlaceUpdateStateKey] = `{"revision":"new-revision","lastContainerStatuses":{"nginx":{"imageID":"nginx@sha256:53f029ad8b1058e966d6714a30d2582943c6e936f4b3c3b344a302c4567f471d"}}}`
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectAllow: false,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, no matched pub, allow",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				delete(pod.Annotations, pubcontrol.PodRelatedPubAnnotation)
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Labels["app"] = "no-pub"
				delete(pod.Annotations, pubcontrol.PodRelatedPubAnnotation)
				pod.Spec.Containers[0].Image = "nginx:1.18"
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, recorded in pub, allow",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Spec.Containers[0].Image = "nginx:1.18"
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Status.UnavailablePods["test-pod-0"] = metav1.Now()
				return pub
			},
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				pubStatus.UnavailablePods["test-pod-0"] = metav1.Now()
				return pubStatus
			},
		},
		{
			name: "valid update pod, container image digest and consistent, reject",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Status.ContainerStatuses[0].Image = "nginx@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"
				pod.Spec.Containers[0].Image = "nginx@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Status.ContainerStatuses[0].Image = "nginx@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"
				pod.Spec.Containers[0].Image = "sha256:04f32c9b5a29a3eb09cab973b72de4c3297bcf0b1834e05faa47d18bcc15f5f1"
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectAllow: false,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, pub feature-gate annotation, allow",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Spec.Containers[0].Image = "nginx:1.18"
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Annotations[policyv1alpha1.PubProtectOperationAnnotation] = "DELETE"
				return pub
			},
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "valid update pod, pub feature-gate annotation, reject",
			oldPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
			newPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Spec.Containers[0].Image = "nginx:1.18"
				return pod
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Annotations[policyv1alpha1.PubProtectOperationAnnotation] = "UPDATE"
				return pub
			},
			expectAllow: false,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			decoder := admission.NewDecoder(scheme)
			fClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs.pub()).
				WithStatusSubresource(&policyv1alpha1.PodUnavailableBudget{}).Build()
			podHandler := PodCreateHandler{
				Client:  fClient,
				Decoder: decoder,
			}
			finder := &controllerfinder.ControllerFinder{Client: fClient}
			pubcontrol.InitPubControl(fClient, finder, record.NewFakeRecorder(10))
			oldPodRaw := runtime.RawExtension{
				Raw: []byte(util.DumpJSON(cs.oldPod())),
			}
			podRaw := runtime.RawExtension{
				Raw: []byte(util.DumpJSON(cs.newPod())),
			}
			req := newAdmission(cs.newPod().Namespace, cs.newPod().Name, admissionv1.Update, podRaw, oldPodRaw, cs.subresource)
			req.Options = runtime.RawExtension{
				Raw: []byte(util.DumpJSON(metav1.UpdateOptions{})),
			}
			allow, _, err := podHandler.podUnavailableBudgetValidatingPod(context.TODO(), req)
			if err != nil {
				t.Errorf("Pub validate pod failed: %s", err.Error())
			}
			if allow != cs.expectAllow {
				t.Fatalf("expect allow(%v) but get(%v)", cs.expectAllow, allow)
			}
			newPub, err := getLatestPub(fClient, cs.pub())
			if err != nil {
				t.Errorf("get latest pub failed: %s", err.Error())
			}
			if !isPubStatusEqual(&newPub.Status, cs.expectPubStatus()) {
				t.Fatalf("expect pub status(%v) but get(%v)", cs.expectPubStatus(), newPub.Status)
			}
			_ = util.GlobalCache.Delete(newPub)
		})
	}
}

func TestValidateEvictPodForPub(t *testing.T) {
	cases := []struct {
		name            string
		eviction        func() *policy.Eviction
		newPod          func() *corev1.Pod
		pub             func() *policyv1alpha1.PodUnavailableBudget
		subresource     string
		expectAllow     bool
		expectPubStatus func() *policyv1alpha1.PodUnavailableBudgetStatus
	}{
		{
			name: "evict pod, reject",
			eviction: func() *policy.Eviction {
				return &policy.Eviction{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-0",
						Namespace: "default",
					},
					DeleteOptions: &metav1.DeleteOptions{},
				}
			},
			newPod: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				return podIn
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			subresource: "eviction",
			expectAllow: false,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "evict pod, allow",
			eviction: func() *policy.Eviction {
				return &policy.Eviction{
					ObjectMeta: metav1.ObjectMeta{
						Name:      podDemo.Name,
						Namespace: podDemo.Namespace,
					},
					DeleteOptions: &metav1.DeleteOptions{},
				}
			},
			newPod: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				return podIn
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Status.CurrentAvailable = 8
				pub.Status.UnavailableAllowed = 1
				return pub
			},
			subresource: "eviction",
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				pubStatus.DisruptedPods["test-pod-0"] = metav1.Now()
				pubStatus.CurrentAvailable = 8
				pubStatus.UnavailableAllowed = 0
				return pubStatus
			},
		},
		{
			name: "evict pod, dry run",
			eviction: func() *policy.Eviction {
				return &policy.Eviction{
					ObjectMeta: metav1.ObjectMeta{
						Name:      podDemo.Name,
						Namespace: podDemo.Namespace,
					},
					DeleteOptions: &metav1.DeleteOptions{
						DryRun: []string{"All"},
					},
				}
			},
			newPod: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				return podIn
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Status.CurrentAvailable = 8
				pub.Status.UnavailableAllowed = 1
				return pub
			},
			subresource: "eviction",
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				pubStatus.CurrentAvailable = 8
				pubStatus.UnavailableAllowed = 1
				return pubStatus
			},
		},
		{
			name: "create pod, ignore",
			newPod: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				return podIn
			},
			eviction: func() *policy.Eviction {
				return nil
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "evict pod, allow",
			eviction: func() *policy.Eviction {
				return &policy.Eviction{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-0",
						Namespace: "default",
					},
					DeleteOptions: &metav1.DeleteOptions{},
				}
			},
			newPod: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				return podIn
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Status = policyv1alpha1.PodUnavailableBudgetStatus{
					TotalReplicas:      0,
					DesiredAvailable:   0,
					CurrentAvailable:   10,
					UnavailableAllowed: 0,
				}
				return pub
			},
			subresource: "eviction",
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := &policyv1alpha1.PodUnavailableBudgetStatus{
					TotalReplicas:      0,
					DesiredAvailable:   0,
					CurrentAvailable:   10,
					UnavailableAllowed: 0,
				}
				return pubStatus
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			decoder := admission.NewDecoder(scheme)
			fClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs.pub(), cs.newPod()).
				WithStatusSubresource(&policyv1alpha1.PodUnavailableBudget{}).Build()
			podHandler := PodCreateHandler{
				Client:  fClient,
				Decoder: decoder,
			}
			finder := &controllerfinder.ControllerFinder{Client: fClient}
			pubcontrol.InitPubControl(fClient, finder, record.NewFakeRecorder(10))
			evictionRaw := runtime.RawExtension{
				Raw: []byte(util.DumpJSON(cs.eviction())),
			}
			req := newAdmission(cs.newPod().Namespace, cs.newPod().Name, admissionv1.Create, evictionRaw, runtime.RawExtension{}, cs.subresource)
			allow, _, err := podHandler.podUnavailableBudgetValidatingPod(context.TODO(), req)
			if err != nil {
				t.Errorf("Pub validate pod failed: %s", err.Error())
			}
			if allow != cs.expectAllow {
				t.Fatalf("expect allow(%v) but get(%v)", cs.expectAllow, allow)
			}
			newPub, err := getLatestPub(fClient, cs.pub())
			if err != nil {
				t.Errorf("get latest pub failed: %s", err.Error())
			}
			if !isPubStatusEqual(&newPub.Status, cs.expectPubStatus()) {
				t.Fatalf("expect pub status(%v) but get(%v)", cs.expectPubStatus(), newPub.Status)
			}
			_ = util.GlobalCache.Delete(newPub)
		})
	}
}

func TestValidateDeletePodForPub(t *testing.T) {
	cases := []struct {
		name            string
		deletion        func() *metav1.DeleteOptions
		newPod          func() *corev1.Pod
		pub             func() *policyv1alpha1.PodUnavailableBudget
		subresource     string
		expectAllow     bool
		expectPubStatus func() *policyv1alpha1.PodUnavailableBudgetStatus
	}{
		{
			name: "delete pod, subresource ignore",
			deletion: func() *metav1.DeleteOptions {
				return &metav1.DeleteOptions{}
			},
			newPod: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				return podIn
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			subresource: "status",
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "delete pod, reject",
			deletion: func() *metav1.DeleteOptions {
				return &metav1.DeleteOptions{}
			},
			newPod: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				return podIn
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			subresource: "",
			expectAllow: false,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "delete pod, pub feature-gate annotation, allow",
			deletion: func() *metav1.DeleteOptions {
				return &metav1.DeleteOptions{}
			},
			newPod: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				return podIn
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Annotations[policyv1alpha1.PubProtectOperationAnnotation] = "UPDATE"
				return pub
			},
			subresource: "",
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				return pubStatus
			},
		},
		{
			name: "delete pod, allow",
			newPod: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				return podIn
			},
			deletion: func() *metav1.DeleteOptions {
				return &metav1.DeleteOptions{}
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Status.CurrentAvailable = 8
				pub.Status.UnavailableAllowed = 1
				return pub
			},
			subresource: "",
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				pubStatus.DisruptedPods["test-pod-0"] = metav1.Now()
				pubStatus.CurrentAvailable = 8
				pubStatus.UnavailableAllowed = 0
				return pubStatus
			},
		},
		{
			name: "delete pod, dry run",
			newPod: func() *corev1.Pod {
				podIn := podDemo.DeepCopy()
				return podIn
			},
			deletion: func() *metav1.DeleteOptions {
				return &metav1.DeleteOptions{
					DryRun: []string{"All"},
				}
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Status.CurrentAvailable = 8
				pub.Status.UnavailableAllowed = 1
				return pub
			},
			subresource: "",
			expectAllow: true,
			expectPubStatus: func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pubStatus := pubDemo.Status.DeepCopy()
				pubStatus.CurrentAvailable = 8
				pubStatus.UnavailableAllowed = 1
				return pubStatus
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			decoder := admission.NewDecoder(scheme)
			fClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs.pub(), cs.newPod()).
				WithStatusSubresource(&policyv1alpha1.PodUnavailableBudget{}).Build()
			podHandler := PodCreateHandler{
				Client:  fClient,
				Decoder: decoder,
			}
			finder := &controllerfinder.ControllerFinder{Client: fClient}
			pubcontrol.InitPubControl(fClient, finder, record.NewFakeRecorder(10))
			deletionRaw := runtime.RawExtension{
				Raw: []byte(util.DumpJSON(cs.deletion())),
			}
			podRaw := runtime.RawExtension{
				Raw: []byte(util.DumpJSON(cs.newPod())),
			}
			req := newAdmission(cs.newPod().Namespace, cs.newPod().Name, admissionv1.Delete, runtime.RawExtension{}, podRaw, cs.subresource)
			req.AdmissionRequest.Options = deletionRaw
			allow, _, err := podHandler.podUnavailableBudgetValidatingPod(context.TODO(), req)
			if err != nil {
				t.Errorf("Pub validate pod failed: %s", err.Error())
			}
			if allow != cs.expectAllow {
				t.Fatalf("expect allow(%v) but get(%v)", cs.expectAllow, allow)
			}
			newPub, err := getLatestPub(fClient, cs.pub())
			if err != nil {
				t.Errorf("get latest pub failed: %s", err.Error())
			}
			if !isPubStatusEqual(&newPub.Status, cs.expectPubStatus()) {
				t.Fatalf("expect pub status(%v) but get(%v)", cs.expectPubStatus(), newPub.Status)
			}
			_ = util.GlobalCache.Delete(newPub)
		})
	}
}

func newAdmission(ns, name string, op admissionv1.Operation, object, oldObject runtime.RawExtension, subResource string) admission.Request {
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Resource:    metav1.GroupVersionResource{Group: corev1.SchemeGroupVersion.Group, Version: corev1.SchemeGroupVersion.Version, Resource: "pods"},
			Operation:   op,
			Object:      object,
			OldObject:   oldObject,
			SubResource: subResource,
			Namespace:   ns,
			Name:        name,
		},
	}
}

func isPubStatusEqual(expectStatus, nowStatus *policyv1alpha1.PodUnavailableBudgetStatus) bool {
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

func getLatestPub(client client.Client, pub *policyv1alpha1.PodUnavailableBudget) (*policyv1alpha1.PodUnavailableBudget, error) {
	newPub := &policyv1alpha1.PodUnavailableBudget{}
	key := types.NamespacedName{
		Namespace: pub.Namespace,
		Name:      pub.Name,
	}
	err := client.Get(context.TODO(), key, newPub)
	return newPub, err
}
