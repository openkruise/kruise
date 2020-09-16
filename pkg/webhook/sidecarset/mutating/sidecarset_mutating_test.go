package mutating

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

var (
	sidecarSetDemo = &appsv1alpha1.SidecarSet{
		Spec: appsv1alpha1.SidecarSetSpec{
			InitContainers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "test-init-containers",
						Image: "test-init-image:latest",
					},
				},
			},
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
	expectedOutputSidecarSet.Spec.InitContainers[0].TerminationMessagePath = corev1.TerminationMessagePathDefault
	expectedOutputSidecarSet.Spec.InitContainers[0].TerminationMessagePolicy = corev1.TerminationMessageReadFile
	expectedOutputSidecarSet.Spec.InitContainers[0].ImagePullPolicy = corev1.PullAlways
	maxUnavailable := intstr.FromInt(1)
	expectedOutputSidecarSet.Spec.Strategy = appsv1alpha1.SidecarSetUpdateStrategy{
		RollingUpdate: &appsv1alpha1.RollingUpdateSidecarSet{
			MaxUnavailable: &maxUnavailable,
		},
	}

	appsv1alpha1.SetDefaultsSidecarSet(sidecarSet)

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
	expectedOutputSidecarSet.Annotations[SidecarSetHashAnnotation] = "8f92wdb9w96824dvw54566vx89wcxd6b75cd4ccxbv4zcvbd7fvfffw4v889dcz2"
	expectedOutputSidecarSet.Annotations[SidecarSetHashWithoutImageAnnotation] = "cfd67dc8z844x4f7cd9f7b624x5ddxxd97wdwv45x48z49cx4942w5c8z84v2dzx"

	if err := setHashSidecarSet(sidecarSet); err != nil {
		t.Errorf("got error %v", err)
	}

	if !reflect.DeepEqual(expectedOutputSidecarSet, sidecarSet) {
		t.Errorf("\nexpect:\n%+v\nbut got:\n%+v", expectedOutputSidecarSet, sidecarSet)
	}
}

func TestSidecarSetHashWithoutImage(t *testing.T) {
	hashDemo, err := SidecarSetHashWithoutImage(sidecarSetDemo)
	if err != nil {
		t.Errorf("SidecarSetHashWithoutImage got error %v", err)
	}
	// change the container image alone won't change the hashWithoutImage value
	sidecarSet := sidecarSetDemo.DeepCopy()
	sidecarSet.Spec.Containers[0].Image = "test-image:v2"
	hashNew, err := SidecarSetHashWithoutImage(sidecarSet)
	if err != nil {
		t.Errorf("SidecarSetHashWithoutImage got error %v", err)
	}
	if hashNew != hashDemo {
		t.Errorf("\nexpect:\n%+v\nbut got:\n%+v", hashDemo, hashNew)
	}
	// change any of the init container won't change the hashWithoutImage value
	sidecarSet.Spec.InitContainers[0].Image = "test-init-image:v1"
	sidecarSet.Spec.InitContainers[0].Command = []string{"sh", "-c", "ls -l"}
	hashNew, err = SidecarSetHashWithoutImage(sidecarSet)
	if err != nil {
		t.Errorf("SidecarSetHashWithoutImage got error %v", err)
	}
	if hashNew != hashDemo {
		t.Errorf("\nexpect:\n%+v\nbut got:\n%+v", hashDemo, hashNew)
	}
	// change the other part of container will change the hashWithoutImage value
	sidecarSet.Spec.Containers[0].WorkingDir = "/tmp"
	hashNew, err = SidecarSetHashWithoutImage(sidecarSet)
	if err != nil {
		t.Errorf("SidecarSetHashWithoutImage got error %v", err)
	}
	if hashNew == hashDemo {
		t.Errorf("\ndon't expect:\n%+v\nbut got:\n%+v", hashDemo, hashNew)
	}
}
