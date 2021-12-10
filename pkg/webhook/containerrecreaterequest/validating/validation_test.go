package validating

import (
	"context"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func init() {
	scheme = runtime.NewScheme()
	_ = policyv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)
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
	crrDemo = appsv1alpha1.ContainerRecreateRequest{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1alpha1.GroupVersion.String(),
			Kind:       "ContainerRecreateRequest",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "crr-test",
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName: "test-pod-0",
			Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
				{
					Name: "nginx",
				},
			},
			Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
				FailurePolicy:     "Fail",
				OrderedRecreate:   true,
				MinStartedSeconds: 10,
			},
		},
	}

	podDemo = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pod-0",
			Namespace:   "default",
			Labels:      map[string]string{"app": "pub-controller"},
			Annotations: map[string]string{},
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

func TestValidatingContainerRecreateRequest(t *testing.T) {
	cases := []struct {
		name            string
		subresource     string
		crr             func() *appsv1alpha1.ContainerRecreateRequest
		pub             func() *policyv1alpha1.PodUnavailableBudget
		pod             func() *corev1.Pod
		expectAllow     bool
		expectPubStatus func() *policyv1alpha1.PodUnavailableBudgetStatus
	}{
		{
			name: "valid create crr, allow",
			crr: func() *appsv1alpha1.ContainerRecreateRequest {
				crr := crrDemo.DeepCopy()
				return crr
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Status.CurrentAvailable = 8
				pub.Status.UnavailableAllowed = 1
				return pub
			},
			pod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
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

			name: "valid create crr, reject",
			crr: func() *appsv1alpha1.ContainerRecreateRequest {
				crr := crrDemo.DeepCopy()
				return crr
			},
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				return pub
			},
			pod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
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
			decoder, _ := admission.NewDecoder(scheme)
			fClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs.pub(), cs.crr(), cs.pod()).Build()
			crrHandler := ContainerRecreateRequestHandler{
				Client:  fClient,
				Decoder: decoder,
			}
			crrRaw := runtime.RawExtension{
				Raw: []byte(util.DumpJSON(cs.crr())),
			}
			req := newAdmission(cs.crr().Namespace, cs.crr().Name, admissionv1.Create, crrRaw, runtime.RawExtension{}, cs.subresource)
			req.Options = runtime.RawExtension{
				Raw: []byte(util.DumpJSON(metav1.UpdateOptions{})),
			}
			allow, _, err := crrHandler.podUnavailableBudgetValidating(context.TODO(), req)
			if err != nil {
				t.Errorf("Pub validate crr failed: %s", err.Error())
			}
			if allow != cs.expectAllow {
				t.Fatalf("expect allow(%v) but get(%v)", cs.expectAllow, allow)
			}
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
