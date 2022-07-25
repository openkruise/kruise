package mutating

import (
	"testing"

	"github.com/openkruise/kruise/apis/apps/defaults"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
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
			PatchPodMetadata: []appsv1alpha1.SidecarSetPatchPodMetadata{
				{
					Annotations: map[string]string{
						"key1": "value1",
					},
				},
			},
			InjectionStrategy: appsv1alpha1.SidecarSetInjectionStrategy{
				Revision: &appsv1alpha1.SidecarSetInjectRevision{
					CustomVersion: pointer.String("1"),
				},
			},
		},
	}
	defaults.SetDefaultsSidecarSet(sidecarSet)
	_ = setHashSidecarSet(sidecarSet)
	if sidecarSet.Spec.UpdateStrategy.Type != appsv1alpha1.RollingUpdateSidecarSetStrategyType {
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
	if sidecarSet.Annotations[sidecarcontrol.SidecarSetHashAnnotation] != "6wbd76bd7984x24fb4f44fv9222cw9v9bcf85x766744wddd4zwx927zzz2zb684" {
		t.Fatalf("sidecarset %v hash initialized incorrectly, got %v", sidecarSet.Name, sidecarSet.Annotations[sidecarcontrol.SidecarSetHashAnnotation])
	}
	if sidecarSet.Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] != "82684vwf9d4cb4wz4vffx4ddfbb47ww4z4wwxdbwb8w2zbb7zvf4524cdd49bv94" {
		t.Fatalf("sidecarset %v hash-without-image initialized incorrectly, got %v", sidecarSet.Name, sidecarSet.Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation])
	}
	if sidecarSet.Spec.PatchPodMetadata[0].PatchPolicy != appsv1alpha1.SidecarSetRetainPatchPolicy {
		t.Fatalf("sidecarset %v patchPodMetadata incorrectly, got %v", sidecarSet.Name, sidecarSet.Spec.PatchPodMetadata)
	}
	if sidecarSet.Spec.InjectionStrategy.Revision.Policy != appsv1alpha1.AlwaysSidecarSetInjectRevisionPolicy {
		t.Fatalf("sidecarset %v InjectionStrategy inilize incorrectly, got %v", sidecarSet.Name, sidecarSet.Spec.InjectionStrategy.Revision.Policy)
	}
}
