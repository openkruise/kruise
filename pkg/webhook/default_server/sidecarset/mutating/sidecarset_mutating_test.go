package mutating

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

var (
	sidecarSetDemo = &appsv1alpha1.SidecarSet{
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "test-sidecar",
						Image: "test-image:v1",
					},
				},
			},
		},
	}
)

func TestSidecarSetDefault(t *testing.T) {
	sidecarSet := sidecarSetDemo.DeepCopy()

	expectedOutputSidecarSet := sidecarSet.DeepCopy()
	expectedOutputSidecarSet.Spec.Containers[0].TerminationMessagePath = corev1.TerminationMessagePathDefault
	expectedOutputSidecarSet.Spec.Containers[0].TerminationMessagePolicy = corev1.TerminationMessageReadFile
	expectedOutputSidecarSet.Spec.Containers[0].ImagePullPolicy = corev1.PullIfNotPresent

	setDefaultSidecarSet(sidecarSet)

	if !reflect.DeepEqual(expectedOutputSidecarSet, sidecarSet) {
		t.Errorf("\nexpect:\n%+v\nbut got:\n%+v", expectedOutputSidecarSet, sidecarSet)
	}
}

func TestSidecarSetHash(t *testing.T) {
	sidecarSet := sidecarSetDemo.DeepCopy()

	expectedOutputSidecarSet := sidecarSet.DeepCopy()
	if expectedOutputSidecarSet.Annotations == nil {
		expectedOutputSidecarSet.Annotations = make(map[string]string)
	}
	expectedOutputSidecarSet.Annotations[SidecarSetHashAnnotation] = "vd6xbxv9w5f8794x5v852cf9288x4v8d926zw2bbcc8545847xw4w7f4xdbx5z6b"
	expectedOutputSidecarSet.Annotations[SidecarSetHashWithoutImageAnnotation] = "cfd67dc8z844x4f7cd9f7b624x5ddxxd97wdwv45x48z49cx4942w5c8z84v2dzx"

	if err := setHashSidecarSet(sidecarSet); err != nil {
		t.Errorf("got error %v", err)
	}

	if !reflect.DeepEqual(expectedOutputSidecarSet, sidecarSet) {
		t.Errorf("\nexpect:\n%+v\nbut got:\n%+v", expectedOutputSidecarSet, sidecarSet)
	}
}
