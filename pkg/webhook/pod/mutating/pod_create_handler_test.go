package mutating

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/openkruise/kruise/pkg/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/webhook/sidecarset/mutating"
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
			Annotations: map[string]string{
				mutating.SidecarSetHashAnnotation:             "c4k2dbb95d",
				mutating.SidecarSetHashWithoutImageAnnotation: "26c9ct5hfb",
			},
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
						Name:  "sidecar1",
						Image: "sidecar-image1",
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "volume1",
				},
			},
		},
	}

	sidecarSet2 := &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				mutating.SidecarSetHashAnnotation:             "gm967682cm",
				mutating.SidecarSetHashWithoutImageAnnotation: "h8c6gb5d2b",
			},
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
						Name:  "sidecar2",
						Image: "sidecar-image2",
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "volume2",
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
			Volumes: []corev1.Volume{
				{
					Name: "nginx-volume",
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
	if len(pod1.Spec.Volumes) != 3 {
		t.Errorf("expect 3 volumes, but got %v", len(pod1.Spec.Volumes))
	}
	if !isMarkedSidecar(pod1.Spec.Containers[1]) || !isMarkedSidecar(pod1.Spec.Containers[2]) {
		t.Errorf("expect env injected, but got nothing")
	}
	hashKey1 := mutating.SidecarSetHashAnnotation
	hashKey2 := mutating.SidecarSetHashWithoutImageAnnotation
	expectedAnnotation1 := `{"sidecarset1":"c4k2dbb95d","sidecarset2":"gm967682cm"}`
	expectedAnnotation2 := `{"sidecarset1":"26c9ct5hfb","sidecarset2":"h8c6gb5d2b"}`
	if pod1.Annotations[hashKey1] != expectedAnnotation1 {
		t.Errorf("expect annotation %v but got %v", expectedAnnotation1, pod1.Annotations[hashKey1])
	}
	if pod1.Annotations[hashKey2] != expectedAnnotation2 {
		t.Errorf("expect annotation %v but got %v", expectedAnnotation2, pod1.Annotations[hashKey2])
	}

	// nothing changed
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

func TestMergeVolumes(t *testing.T) {
	original := []corev1.Volume{
		{
			Name: "vol01",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "vol02",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{},
			},
		},
	}
	additional := []corev1.Volume{
		{
			Name: "vol02",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "vol03",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{},
			},
		},
		{
			Name: "vol03",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
	expected := []corev1.Volume{
		{
			Name: "vol01",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "vol02",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{},
			},
		},
		{
			Name: "vol03",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{},
			},
		},
	}

	got := mergeVolumes(original, additional)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected %v, got %v", util.DumpJSON(expected), util.DumpJSON(got))
	}
}
