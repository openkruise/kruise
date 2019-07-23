package sidecarset

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"

	"github.com/openkruise/kruise/pkg/webhook/default_server/sidecarset/mutating"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

var (
	scheme *runtime.Scheme

	sidecarSetDemo = &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				mutating.SidecarSetHashAnnotation:             "ccc",
				mutating.SidecarSetHashWithoutImageAnnotation: "bbb",
			},
			Name: "test-sidecarset",
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "test-sidecar",
						Image: "test-image:v2",
					},
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
		},
	}

	podDemo = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				mutating.SidecarSetHashAnnotation:             `{"test-sidecarset":"aaa"}`,
				mutating.SidecarSetHashWithoutImageAnnotation: `{"test-sidecarset":"bbb"}`,
			},
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
				{
					Name:  "test-sidecar",
					Image: "test-image:v1",
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
		},
	}
)

func init() {
	scheme = runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
}

func getLatestPod(client client.Client, pod *corev1.Pod) (*corev1.Pod, error) {
	newPod := &corev1.Pod{}
	podKey := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      pod.Name,
	}
	err := client.Get(context.TODO(), podKey, newPod)
	return newPod, err
}

func isSidecarImageUpdated(pod *corev1.Pod, containerName, containerImage string) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == containerName {
			return container.Image == containerImage
		}
	}
	return false
}

func TestUpdateWhenEverythingIsFine(t *testing.T) {
	sidecarSetInput := sidecarSetDemo.DeepCopy()
	podInput := podDemo.DeepCopy()
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: sidecarSetInput.Namespace,
			Name:      sidecarSetInput.Name,
		},
	}

	fakeClient := fake.NewFakeClientWithScheme(scheme, sidecarSetInput, podInput)
	reconciler := ReconcileSidecarSet{Client: fakeClient}
	if _, err := reconciler.Reconcile(request); err != nil {
		t.Errorf("reconcile failed, err: %v", err)
	}

	podOutput, err := getLatestPod(fakeClient, podInput)
	if err != nil {
		t.Errorf("get latest pod failed, err: %v", err)
	}
	if !isSidecarImageUpdated(podOutput, "test-sidecar", "test-image:v2") {
		t.Errorf("should update sidecar")
	}
}

func TestUpdateWhenSidecarSetPaused(t *testing.T) {
	sidecarSetInput := sidecarSetDemo.DeepCopy()
	sidecarSetInput.Spec.Paused = true
	podInput := podDemo.DeepCopy()
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: sidecarSetInput.Namespace,
			Name:      sidecarSetInput.Name,
		},
	}

	fakeClient := fake.NewFakeClientWithScheme(scheme, sidecarSetInput, podInput)
	reconciler := ReconcileSidecarSet{Client: fakeClient}
	if _, err := reconciler.Reconcile(request); err != nil {
		t.Errorf("reconcile failed, err: %v", err)
	}

	podOutput, err := getLatestPod(fakeClient, podInput)
	if err != nil {
		t.Errorf("get latest pod failed, err: %v", err)
	}
	if isSidecarImageUpdated(podOutput, "test-sidecar", "test-image:v2") {
		t.Errorf("shouldn't update sidecar because sidecarset is paused")
	}
}

func TestUpdateWhenOtherFieldsChanged(t *testing.T) {
	sidecarSetInput := sidecarSetDemo.DeepCopy()
	sidecarSetInput.Annotations[mutating.SidecarSetHashAnnotation] = "ccc"
	sidecarSetInput.Annotations[mutating.SidecarSetHashWithoutImageAnnotation] = "ddd"
	podInput := podDemo.DeepCopy()
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: sidecarSetInput.Namespace,
			Name:      sidecarSetInput.Name,
		},
	}

	fakeClient := fake.NewFakeClientWithScheme(scheme, sidecarSetInput, podInput)
	reconciler := ReconcileSidecarSet{Client: fakeClient}
	if _, err := reconciler.Reconcile(request); err != nil {
		t.Errorf("reconcile failed, err: %v", err)
	}

	podOutput, err := getLatestPod(fakeClient, podInput)
	if err != nil {
		t.Errorf("get latest pod failed, err: %v", err)
	}
	if isSidecarImageUpdated(podOutput, "test-sidecar", "test-image:v2") {
		t.Errorf("shouldn't update sidecar because other fields in sidecarset had changed")
	}
}

func TestUpdateWhenMaxUnavailableNotZero(t *testing.T) {
	sidecarSetInput := sidecarSetDemo.DeepCopy()
	updateCache.reset(sidecarSetInput)
	podInput := podDemo.DeepCopy()
	podInput.Status.Phase = corev1.PodPending
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: sidecarSetInput.Namespace,
			Name:      sidecarSetInput.Name,
		},
	}

	fakeClient := fake.NewFakeClientWithScheme(scheme, sidecarSetInput, podInput)
	reconciler := ReconcileSidecarSet{Client: fakeClient}
	if _, err := reconciler.Reconcile(request); err != nil {
		t.Errorf("reconcile failed, err: %v", err)
	}

	podOutput, err := getLatestPod(fakeClient, podInput)
	if err != nil {
		t.Errorf("get latest pod failed, err: %v", err)
	}
	if isSidecarImageUpdated(podOutput, "test-sidecar", "test-image:v2") {
		t.Errorf("shouldn't update sidecar because unavailable number is not zero")
	}
}
