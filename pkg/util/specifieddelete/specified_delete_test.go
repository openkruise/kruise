package specifieddelete

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestPatchPodSpecifiedDelete(t *testing.T) {
	tests := []struct {
		pod      *corev1.Pod
		keepPVC  bool
		expected *corev1.Pod
	}{
		{
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec: corev1.PodSpec{},
			},
			keepPVC: false,
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Labels: map[string]string{
						appsv1alpha1.SpecifiedDeleteKey: "true",
					},
				},
				Spec: corev1.PodSpec{},
			},
		},
		{
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec: corev1.PodSpec{},
			},
			keepPVC: true,
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Labels: map[string]string{
						appsv1alpha1.SpecifiedDeleteKey:    "true",
						appsv1alpha1.KeepPVCForDeletionKey: "true",
					},
				},
				Spec: corev1.PodSpec{},
			},
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			cli := fake.NewClientBuilder().WithObjects(test.pod).Build()
			_, err := PatchPodSpecifiedDelete(cli, test.pod, test.keepPVC)
			assert.NoError(t, err)

			// patch will write result object into the given test.pod
			if apiequality.Semantic.DeepEqual(test.expected, test.pod) {
				t.Fatalf("expected %v but got %v", test.expected, test.pod)
			}
		})
	}
}
