package mutating

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

func TestMutateSidecarSet(t *testing.T) {
	sidecarSet := &appsv1alpha1.SidecarSet{
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name: "test-sidecar",
					},
				},
			},
		},
	}

	expectedOutputSidecarSet := sidecarSet.DeepCopy()
	expectedOutputSidecarSet.Spec.Containers[0].TerminationMessagePath = corev1.TerminationMessagePathDefault
	expectedOutputSidecarSet.Spec.Containers[0].TerminationMessagePolicy = corev1.TerminationMessageReadFile
	expectedOutputSidecarSet.Spec.Containers[0].ImagePullPolicy = corev1.PullIfNotPresent

	setDefaultSidecarSet(sidecarSet)

	if !reflect.DeepEqual(expectedOutputSidecarSet, sidecarSet) {
		t.Errorf("\nexpect:\n%+v\nbut got:\n%+v", expectedOutputSidecarSet, sidecarSet)
	}
}
