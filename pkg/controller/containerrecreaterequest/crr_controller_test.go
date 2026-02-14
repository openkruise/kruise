package containerrecreaterequest

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/podadapter"
	utilpodreadiness "github.com/openkruise/kruise/pkg/util/podreadiness"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
}

func TestReconcile_ActiveDeadlineSeconds(t *testing.T) {
	// 1. Setup
	activeDeadlineSeconds := int64(10)
	// Create timestamp 15 seconds ago, so deadline (10s) is exceeded
	creationTime := metav1.NewTime(time.Now().Add(-15 * time.Second))

	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-crr",
			Namespace:         "default",
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-uid",
			},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName:               "test-pod",
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
			Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
				{Name: "c1"},
				{Name: "c2"},
				{Name: "c3"},
			},
		},
		Status: appsv1alpha1.ContainerRecreateRequestStatus{
			Phase: appsv1alpha1.ContainerRecreateRequestRecreating,
			ContainerRecreateStates: []appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
				{Name: "c1", Phase: appsv1alpha1.ContainerRecreateRequestRecreating},
				{Name: "c2", Phase: appsv1alpha1.ContainerRecreateRequestPending},
				{Name: "c3", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "c1"},
				{Name: "c2"},
				{Name: "c3"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crr, pod).WithStatusSubresource(&appsv1alpha1.ContainerRecreateRequest{}).Build()

	reconciler := &ReconcileContainerRecreateRequest{
		Client:              fakeClient,
		clock:               clock.RealClock{},
		podReadinessControl: utilpodreadiness.NewForAdapter(&podadapter.AdapterRuntimeClient{Client: fakeClient}),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-crr",
		},
	}

	// 2. Execution
	_, err := reconciler.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// 3. Assertion
	updatedCRR := &appsv1alpha1.ContainerRecreateRequest{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-crr", Namespace: "default"}, updatedCRR)
	if err != nil {
		t.Fatalf("Get CRR failed: %v", err)
	}

	if updatedCRR.Status.Phase != appsv1alpha1.ContainerRecreateRequestCompleted {
		t.Errorf("Expected Phase to be Completed, got %v", updatedCRR.Status.Phase)
	}

	// Check individual container states
	stateMap := make(map[string]appsv1alpha1.ContainerRecreateRequestContainerRecreateState)
	for _, s := range updatedCRR.Status.ContainerRecreateStates {
		stateMap[s.Name] = s
	}

	expectedMessage := "recreating has exceeded the activeDeadlineSeconds"

	if s, ok := stateMap["c1"]; !ok {
		t.Errorf("c1 state not found")
	} else {
		if s.Phase != appsv1alpha1.ContainerRecreateRequestFailed {
			t.Errorf("Expected c1 to be Failed, got %v", s.Phase)
		}
		if s.Message != expectedMessage {
			t.Errorf("Expected c1 message to be '%s', got '%s'", expectedMessage, s.Message)
		}
	}

	if s, ok := stateMap["c2"]; !ok {
		t.Errorf("c2 state not found")
	} else {
		if s.Phase != appsv1alpha1.ContainerRecreateRequestFailed {
			t.Errorf("Expected c2 to be Failed, got %v", s.Phase)
		}
		if s.Message != expectedMessage {
			t.Errorf("Expected c2 message to be '%s', got '%s'", expectedMessage, s.Message)
		}
	}

	if s, ok := stateMap["c3"]; !ok {
		t.Errorf("c3 state not found")
	} else {
		if s.Phase != appsv1alpha1.ContainerRecreateRequestSucceeded {
			t.Errorf("Expected c3 to be Succeeded, got %v", s.Phase)
		}
	}
}
