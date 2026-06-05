package sidecarcontrol

import (
	"testing"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInitContainerVolumeSharing(t *testing.T) {
	// Setup Pod with InitContainer having volumes
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name: "init-container",
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "init-vol",
							MountPath: "/init-mnt",
						},
					},
					VolumeDevices: []corev1.VolumeDevice{
						{
							Name:       "init-device",
							DevicePath: "/dev/init",
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name: "main-container",
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "main-vol",
							MountPath: "/main-mnt",
						},
					},
					VolumeDevices: []corev1.VolumeDevice{
						{
							Name:       "main-device",
							DevicePath: "/dev/main",
						},
					},
				},
			},
		},
	}

	// Setup SidecarSet with volume sharing enabled
	sidecarSet := &appsv1beta1.SidecarSet{
		Spec: appsv1beta1.SidecarSetSpec{
			Containers: []appsv1beta1.SidecarContainer{
				{
					ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
						Type: appsv1beta1.ShareVolumePolicyEnabled,
					},
					ShareVolumeDevicePolicy: &appsv1beta1.ShareVolumePolicy{
						Type: appsv1beta1.ShareVolumePolicyEnabled,
					},
				},
			},
		},
	}
	control := New(sidecarSet)
	sidecarContainer := &sidecarSet.Spec.Containers[0]

	// Test VolumeMounts
	injectedMounts, _ := GetInjectedVolumeMountsAndEnvs(control, sidecarContainer, pod)

	// Check if init-vol is present
	foundInitVol := false
	for _, m := range injectedMounts {
		if m.Name == "init-vol" {
			foundInitVol = true
			break
		}
	}

	if !foundInitVol {
		t.Errorf("Failed to find init container volume mount 'init-vol'.")
	}

	// Test VolumeDevices
	injectedDevices := GetInjectedVolumeDevices(sidecarContainer, pod)

	// Check if init-device is present
	foundInitDevice := false
	for _, d := range injectedDevices {
		if d.Name == "init-device" {
			foundInitDevice = true
			break
		}
	}

	if !foundInitDevice {
		t.Errorf("Failed to find init container volume device 'init-device'.")
	}
}
