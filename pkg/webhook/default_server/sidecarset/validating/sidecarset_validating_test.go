package validating

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

func TestValidateSidecarSet(t *testing.T) {
	errorCases := map[string]appsv1alpha1.SidecarSet{
		"missing-selector": {
			ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
			Spec: appsv1alpha1.SidecarSetSpec{
				Containers: []appsv1alpha1.SidecarContainer{
					{
						Container: corev1.Container{
							Name:                     "test-sidecar",
							Image:                    "test-image",
							ImagePullPolicy:          corev1.PullIfNotPresent,
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
				},
			},
		},
		"wrong-containers": {
			ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
			Spec: appsv1alpha1.SidecarSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"a": "b"},
				},
				Containers: []appsv1alpha1.SidecarContainer{
					{
						Container: corev1.Container{
							Name:            "test-sidecar",
							Image:           "test-image",
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
				},
			},
		},
	}

	for name, sidecarSet := range errorCases {
		allErrs := validateSidecarSet(&sidecarSet)
		if len(allErrs) == 0 {
			t.Errorf("%v: expect more than 1, but got 0 errors", name)
		} else {
			fmt.Println(allErrs)
		}
	}
}
