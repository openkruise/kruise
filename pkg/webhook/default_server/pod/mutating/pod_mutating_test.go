package mutating

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/pkg/apis"
	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

func TestMain(m *testing.M) {
	t := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crds")},
	}
	apis.AddToScheme(scheme.Scheme)

	code := m.Run()
	t.Stop()
	os.Exit(code)
}

func TestSidecarSetMutatePod(t *testing.T) {
	sidecarSet1 := &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "sidecarset1",
			Generation: 123,
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name: "sidecar1",
					},
				},
			},
		},
	}

	sidecarSet2 := &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "sidecarset2",
			Generation: 456,
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name: "sidecar2",
					},
				},
			},
		},
	}

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "nginx"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
				},
			},
		},
	}

	pod2 := pod1.DeepCopy()
	pod2.Labels = map[string]string{}

	client := fake.NewFakeClient(sidecarSet1, sidecarSet2)
	decoder, _ := admission.NewDecoder(scheme.Scheme)
	podHandler := &PodCreateHandler{Decoder: decoder, Client: client}

	expectedMutatedPod2 := pod2.DeepCopy()

	_ = podHandler.mutatingPodFn(context.TODO(), pod1)
	_ = podHandler.mutatingPodFn(context.TODO(), pod2)

	if len(pod1.Spec.Containers) != 3 {
		t.Errorf("expect 3 containers, but got %v", len(pod1.Spec.Containers))
	}
	if !isMarkedSidecar(pod1.Spec.Containers[1]) || !isMarkedSidecar(pod1.Spec.Containers[2]) {
		t.Errorf("expect env injected, but got nothing")
	}
	expectedAnnotation := `{"sidecarset1":123,"sidecarset2":456}`
	if pod1.Annotations[SidecarSetGenerationAnnotation] != expectedAnnotation {
		t.Errorf("expect annotation %v but got %v", pod1.Annotations[SidecarSetGenerationAnnotation], expectedAnnotation)
	}
	if !reflect.DeepEqual(pod2, expectedMutatedPod2) {
		t.Errorf("\nexpected mutated pod:\n%+v,\nbut got %+v\n", expectedMutatedPod2, pod2)
	}
}

func isMarkedSidecar(container corev1.Container) bool {
	for _, env := range container.Env {
		if env.Name == SidecarEnvKey && env.Value == "true" {
			return true
		}
	}
	return false
}
