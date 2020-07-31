package validating

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

var (
	maxUnavailable   = intstr.FromInt(1)
	wrongUnavailable = intstr.FromInt(-1)
)

func TestValidateSidecarSet(t *testing.T) {
	errorCases := map[string]appsv1alpha1.SidecarSet{
		"missing-selector": {
			ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
			Spec: appsv1alpha1.SidecarSetSpec{
				Strategy: appsv1alpha1.SidecarSetUpdateStrategy{
					RollingUpdate: &appsv1alpha1.RollingUpdateSidecarSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
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
				Strategy: appsv1alpha1.SidecarSetUpdateStrategy{
					RollingUpdate: &appsv1alpha1.RollingUpdateSidecarSet{
						MaxUnavailable: &maxUnavailable,
					},
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
		"wrong-rollingUpdate": {
			ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
			Spec: appsv1alpha1.SidecarSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"a": "b"},
				},
				Strategy: appsv1alpha1.SidecarSetUpdateStrategy{
					RollingUpdate: &appsv1alpha1.RollingUpdateSidecarSet{
						MaxUnavailable: &wrongUnavailable,
					},
				},
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
		"wrong-volumes": {
			ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
			Spec: appsv1alpha1.SidecarSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"a": "b"},
				},
				Strategy: appsv1alpha1.SidecarSetUpdateStrategy{
					RollingUpdate: &appsv1alpha1.RollingUpdateSidecarSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
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
				Volumes: []corev1.Volume{
					{
						Name: "test-volume",
					},
				},
			},
		},
	}

	for name, sidecarSet := range errorCases {
		allErrs := validateSidecarSet(&sidecarSet)
		if len(allErrs) != 1 {
			t.Errorf("%v: expect errors len 1, but got: %v", name, allErrs)
		} else {
			fmt.Println(allErrs)
		}
	}
}
