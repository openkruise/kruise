package mutating

import (
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestMutatingSidecarSetFn(t *testing.T) {
	sidecarSet := &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "123",
			Name:            "sidecarset-test",
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns:1.0",
					},
				},
			},
		},
	}
	appsv1alpha1.SetDefaultsSidecarSet(sidecarSet)
	_ = setHashSidecarSet(sidecarSet)
	if sidecarSet.Spec.UpdateStrategy.Type != appsv1alpha1.NotUpdateSidecarSetStrategyType {
		t.Fatalf("update strategy not initialized")
	}
	if *sidecarSet.Spec.UpdateStrategy.Partition != intstr.FromInt(0) {
		t.Fatalf("partition not initialized")
	}
	if *sidecarSet.Spec.UpdateStrategy.MaxUnavailable != intstr.FromInt(1) {
		t.Fatalf("maxUnavailable not initialized")
	}
	for _, container := range sidecarSet.Spec.Containers {
		if container.PodInjectPolicy != appsv1alpha1.BeforeAppContainerType {
			t.Fatalf("container %v podInjectPolicy initialized incorrectly", container.Name)
		}
		if container.ShareVolumePolicy.Type != appsv1alpha1.ShareVolumePolicyDisabled {
			t.Fatalf("container %v shareVolumePolicy initialized incorrectly", container.Name)
		}
		if sidecarSet.Spec.Containers[0].UpgradeStrategy.UpgradeType != appsv1alpha1.SidecarContainerColdUpgrade {
			t.Fatalf("container %v upgradePolicy initialized incorrectly", container.Name)
		}

		if container.ImagePullPolicy != corev1.PullIfNotPresent {
			t.Fatalf("container %v imagePullPolicy initialized incorrectly", container.Name)
		}
		if container.TerminationMessagePath != "/dev/termination-log" {
			t.Fatalf("container %v terminationMessagePath initialized incorrectly", container.Name)
		}
		if container.TerminationMessagePolicy != corev1.TerminationMessageReadFile {
			t.Fatalf("container %v terminationMessagePolicy initialized incorrectly", container.Name)
		}
	}
	if sidecarSet.Annotations[sidecarcontrol.SidecarSetHashAnnotation] != "9w829wfc74c22465fv2z2dwf54x7c5wv6424f98dv7bcwx8444768wf6wfv4bdfc" {
		t.Fatalf("sidecarset %v hash initialized incorrectly, got %v", sidecarSet.Name, sidecarSet.Annotations[sidecarcontrol.SidecarSetHashAnnotation])
	}
}
