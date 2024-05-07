package sidecarset

import (
	"context"
	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type HandlePod func(pod []*corev1.Pod)

var (
	scheme *runtime.Scheme

	sidecarSetDemo = &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			//Generation: 123,
			Annotations: map[string]string{
				sidecarcontrol.SidecarSetHashAnnotation: "bbb",
			},
			Name:   "test-sidecarset",
			Labels: map[string]string{},
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
			UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
				// default type=RollingUpdate, Partition=0, MaxUnavailable=1
				//Type:           appsv1alpha1.RollingUpdateSidecarSetStrategyType,
				//Partition:      &partition,
				//MaxUnavailable: &maxUnavailable,
			},
			RevisionHistoryLimit: utilpointer.Int32Ptr(10),
		},
	}

	podDemo = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				sidecarcontrol.SidecarSetHashAnnotation: `{"test-sidecarset":{"hash":"aaa","sidecarList":["test-sidecar"]}}`,
				sidecarcontrol.SidecarSetListAnnotation: "test-sidecarset",
			},
			Name:      "test-pod-1",
			Namespace: "default",
			Labels:    map[string]string{"app": "nginx"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
					Env: []corev1.EnvVar{
						{
							Name:  "nginx-env",
							Value: "value-1",
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "nginx-volume",
							MountPath: "/data/nginx",
						},
					},
				},
				{
					Name:  "test-sidecar",
					Image: "test-image:v1",
					Env: []corev1.EnvVar{
						{
							Name:  "IS_INJECTED",
							Value: "true",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "nginx-volume",
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
					Image:   "nginx:1.15.1",
					ImageID: "docker-pullable://nginx@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d",
					Ready:   true,
				},
				{
					Name:    "test-sidecar",
					Image:   "test-image:v1",
					ImageID: "docker-pullable://test-image@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d",
					Ready:   true,
				},
			},
		},
	}
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(clientgoscheme.Scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
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

func getLatestSidecarSet(client client.Client, sidecarset *appsv1alpha1.SidecarSet) (*appsv1alpha1.SidecarSet, error) {
	newSidecarSet := &appsv1alpha1.SidecarSet{}
	Key := types.NamespacedName{
		Name: sidecarset.Name,
	}
	err := client.Get(context.TODO(), Key, newSidecarSet)
	return newSidecarSet, err
}

func isSidecarImageUpdated(pod *corev1.Pod, containerName, containerImage string) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == containerName {
			return container.Image == containerImage
		}
	}
	return false
}

func TestUpdateWhenUseNotUpdateStrategy(t *testing.T) {
	sidecarSetInput := sidecarSetDemo.DeepCopy()
	testUpdateWhenUseNotUpdateStrategy(t, sidecarSetInput)
}

func testUpdateWhenUseNotUpdateStrategy(t *testing.T, sidecarSetInput *appsv1alpha1.SidecarSet) {
	sidecarSetInput.Spec.UpdateStrategy.Type = appsv1alpha1.NotUpdateSidecarSetStrategyType
	podInput := podDemo.DeepCopy()
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: sidecarSetInput.Namespace,
			Name:      sidecarSetInput.Name,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(sidecarSetInput, podInput).
		WithStatusSubresource(&appsv1alpha1.SidecarSet{}).Build()
	reconciler := ReconcileSidecarSet{
		Client:    fakeClient,
		processor: NewSidecarSetProcessor(fakeClient, record.NewFakeRecorder(10)),
	}
	if _, err := reconciler.Reconcile(context.TODO(), request); err != nil {
		t.Errorf("reconcile failed, err: %v", err)
	}

	podOutput, err := getLatestPod(fakeClient, podInput)
	if err != nil {
		t.Errorf("get latest pod failed, err: %v", err)
	}
	if isSidecarImageUpdated(podOutput, "test-sidecar", "test-image:v2") {
		t.Errorf("shouldn't update sidecar because sidecarset use not update strategy")
	}
}

func TestUpdateWhenSidecarSetPaused(t *testing.T) {
	sidecarSetInput := sidecarSetDemo.DeepCopy()
	testUpdateWhenSidecarSetPaused(t, sidecarSetInput)
}

func testUpdateWhenSidecarSetPaused(t *testing.T, sidecarSetInput *appsv1alpha1.SidecarSet) {
	sidecarSetInput.Spec.UpdateStrategy.Paused = true
	podInput := podDemo.DeepCopy()
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: sidecarSetInput.Namespace,
			Name:      sidecarSetInput.Name,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(sidecarSetInput, podInput).
		WithStatusSubresource(&appsv1alpha1.SidecarSet{}).Build()
	reconciler := ReconcileSidecarSet{
		Client:    fakeClient,
		processor: NewSidecarSetProcessor(fakeClient, record.NewFakeRecorder(10)),
	}
	if _, err := reconciler.Reconcile(context.TODO(), request); err != nil {
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

func TestUpdateWhenMaxUnavailableNotZero(t *testing.T) {
	sidecarSetInput := sidecarSetDemo.DeepCopy()
	testUpdateWhenMaxUnavailableNotZero(t, sidecarSetInput)
}

func testUpdateWhenMaxUnavailableNotZero(t *testing.T, sidecarSetInput *appsv1alpha1.SidecarSet) {
	podInput := podDemo.DeepCopy()
	podInput.Status.Phase = corev1.PodPending
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: sidecarSetInput.Namespace,
			Name:      sidecarSetInput.Name,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(sidecarSetInput, podInput).
		WithStatusSubresource(&appsv1alpha1.SidecarSet{}).Build()
	reconciler := ReconcileSidecarSet{
		Client:    fakeClient,
		processor: NewSidecarSetProcessor(fakeClient, record.NewFakeRecorder(10)),
	}
	if _, err := reconciler.Reconcile(context.TODO(), request); err != nil {
		t.Errorf("reconcile failed, err: %v", err)
	}

	podOutput, err := getLatestPod(fakeClient, podInput)
	if err != nil {
		t.Errorf("get latest pod failed, err: %v", err)
	}
	if !isSidecarImageUpdated(podOutput, "test-sidecar", "test-image:v2") {
		t.Errorf("sidecarset upgrade image failed")
	}
}

func TestUpdateWhenPartitionFinished(t *testing.T) {
	sidecarSetInput := sidecarSetDemo.DeepCopy()
	testUpdateWhenPartitionFinished(t, sidecarSetInput)
}

func testUpdateWhenPartitionFinished(t *testing.T, sidecarSetInput *appsv1alpha1.SidecarSet) {
	newPartition := intstr.FromInt(1)
	sidecarSetInput.Spec.UpdateStrategy.Partition = &newPartition
	podInput := podDemo.DeepCopy()
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: sidecarSetInput.Namespace,
			Name:      sidecarSetInput.Name,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidecarSetInput, podInput).
		WithStatusSubresource(&appsv1alpha1.SidecarSet{}).Build()
	reconciler := ReconcileSidecarSet{
		Client:    fakeClient,
		processor: NewSidecarSetProcessor(fakeClient, record.NewFakeRecorder(10)),
	}
	if _, err := reconciler.Reconcile(context.TODO(), request); err != nil {
		t.Errorf("reconcile failed, err: %v", err)
	}

	podOutput, err := getLatestPod(fakeClient, podInput)
	if err != nil {
		t.Errorf("get latest pod failed, err: %v", err)
	}
	if isSidecarImageUpdated(podOutput, "test-sidecar", "test-image:v2") {
		t.Errorf("shouldn't update sidecar because partition is 1")
	}
}

func TestRemoveSidecarSet(t *testing.T) {
	sidecarSetInput := sidecarSetDemo.DeepCopy()
	testRemoveSidecarSet(t, sidecarSetInput)
}

func testRemoveSidecarSet(t *testing.T, sidecarSetInput *appsv1alpha1.SidecarSet) {
	podInput := podDemo.DeepCopy()
	hashKey := sidecarcontrol.SidecarSetHashAnnotation
	podInput.Annotations[hashKey] = `{"test-sidecarset":{"hash":"bbb"}}`
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: sidecarSetInput.Namespace,
			Name:      sidecarSetInput.Name,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidecarSetInput, podInput).
		WithStatusSubresource(&appsv1alpha1.SidecarSet{}).Build()
	reconciler := ReconcileSidecarSet{
		Client:    fakeClient,
		processor: NewSidecarSetProcessor(fakeClient, record.NewFakeRecorder(10)),
	}
	if _, err := reconciler.Reconcile(context.TODO(), request); err != nil {
		t.Errorf("reconcile failed, err: %v", err)
	}

	podOutput, err := getLatestPod(fakeClient, podInput)
	if err != nil {
		t.Errorf("get latest pod failed, err: %v", err)
	}
	if podOutput.Annotations[hashKey] == "" {
		t.Errorf("should remove sidecarset info")
	}
}
