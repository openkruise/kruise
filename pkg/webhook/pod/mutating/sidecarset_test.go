/*
Copyright 2020 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mutating

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openkruise/kruise/apis"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"

	admissionv1 "k8s.io/api/admission/v1"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	defaultNs = "default"
)

func TestMain(m *testing.M) {
	t := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crds")},
	}
	apis.AddToScheme(scheme.Scheme)

	code := m.Run()
	t.Stop()
	os.Exit(code)
}

var (
	sidecarSet1 = &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sidecarset1",
			Annotations: map[string]string{
				sidecarcontrol.SidecarSetHashAnnotation: "c4k2dbb95d",
			},
			Labels: map[string]string{},
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "suxing-test",
				},
			},
			InitContainers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "init-2",
						Image: "busybox:1.0.0",
					},
				},
				{
					Container: corev1.Container{
						Name:  "init-1",
						Image: "busybox:1.0.0",
					},
				},
			},
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns-f-image:1.0",
					},
					PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
					ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
						Type: appsv1alpha1.ShareVolumePolicyDisabled,
					},
				},
				{
					Container: corev1.Container{
						Name:  "log-agent",
						Image: "log-agent-image:1.0",
					},
					PodInjectPolicy: appsv1alpha1.AfterAppContainerType,
					ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
						Type: appsv1alpha1.ShareVolumePolicyDisabled,
					},
				},
			},
		},
	}

	sidecarsetWithTransferEnv = &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				sidecarcontrol.SidecarSetHashAnnotation: "c4k2dbb95d",
			},
			Name:   "sidecarset2",
			Labels: map[string]string{},
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "suxing-test",
				},
			},
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns-f-image:1.0",
						VolumeMounts: []corev1.VolumeMount{
							{Name: "volume-1"},
						},
					},
					PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
					ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
						Type: appsv1alpha1.ShareVolumePolicyDisabled,
					},
					TransferEnv: []appsv1alpha1.TransferEnvVar{
						{
							SourceContainerName: "nginx",
							EnvName:             "hello2",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{Name: "volume-1"},
				{Name: "volume-2"},
			},
		},
	}

	sidecarSet3 = &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				sidecarcontrol.SidecarSetHashAnnotation: "gm967682cm",
			},
			Name:   "sidecarset3",
			Labels: map[string]string{},
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "suxing-test",
				},
			},
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns-f-image:1.0",
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "volume-1",
								MountPath: "/a/b/c",
							},
							{
								Name:      "volume-2",
								MountPath: "/d/e/f",
							},
						},
					},
					PodInjectPolicy: appsv1alpha1.AfterAppContainerType,
					ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
						Type: appsv1alpha1.ShareVolumePolicyEnabled,
					},
				},
				{
					Container: corev1.Container{
						Name:  "log-agent",
						Image: "log-agent-image:1.0",
					},
					PodInjectPolicy: appsv1alpha1.AfterAppContainerType,
					ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
						Type: appsv1alpha1.ShareVolumePolicyDisabled,
					},
				},
			},
			Volumes: []corev1.Volume{
				{Name: "volume-1"},
				{Name: "volume-2"},
			},
		},
	}

	sidecarSetWithStaragent = &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				sidecarcontrol.SidecarSetHashAnnotation: "gm967682cm",
			},
			Name:   "sidecarset3",
			Labels: map[string]string{},
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "suxing-test",
				},
			},
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns-f-image:1.0",
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "volume-1",
								MountPath: "/a/b/c",
							},
							{
								Name:      "volume-2",
								MountPath: "/d/e/f",
							},
							{
								Name:      "volume-staragent",
								MountPath: "/staragent",
							},
						},
					},
					PodInjectPolicy: appsv1alpha1.AfterAppContainerType,
					ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
						Type: appsv1alpha1.ShareVolumePolicyEnabled,
					},
				},
				{
					Container: corev1.Container{
						Name:  "staragent",
						Image: "staragent-image:1.0",
					},
					PodInjectPolicy: appsv1alpha1.AfterAppContainerType,
					ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
						Type: appsv1alpha1.ShareVolumePolicyEnabled,
					},
				},
				{
					Container: corev1.Container{
						Name:  "log-agent",
						Image: "log-agent-image:1.0",
					},
					PodInjectPolicy: appsv1alpha1.AfterAppContainerType,
					ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
						Type: appsv1alpha1.ShareVolumePolicyDisabled,
					},
				},
			},
			Volumes: []corev1.Volume{
				{Name: "volume-1"},
				{Name: "volume-2"},
				{Name: "volume-staragent"},
			},
		},
	}

	pod1 = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: defaultNs,
			Labels:    map[string]string{"app": "suxing-test"},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:  "init-0",
					Image: "busybox:1.0.0",
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
					Env: []corev1.EnvVar{
						{
							Name:  "hello1",
							Value: "world1",
						},
						{
							Name:  "hello2",
							Value: "world2",
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "volume-a",
							MountPath: "/a/b",
						},
						{
							Name:      "volume-b",
							MountPath: "/e/f",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{Name: "volume-a"},
				{Name: "volume-b"},
			},
		},
	}

	podWithStaragent = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: defaultNs,
			Labels:    map[string]string{"app": "suxing-test"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
					Env: []corev1.EnvVar{
						{
							Name:  "hello1",
							Value: "world1",
						},
						{
							Name:  "hello2",
							Value: "world2",
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "volume-a",
							MountPath: "/a/b",
						},
						{
							Name:      "volume-b",
							MountPath: "/e/f",
						},
						{
							Name:      "volume-staragent",
							MountPath: "/staragent",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{Name: "volume-a"},
				{Name: "volume-b"},
				{Name: "volume-staragent"},
			},
		},
	}
)

func TestPodHasNoMatchedSidecarSet(t *testing.T) {
	sidecarSetIn := sidecarSet1.DeepCopy()
	testPodHasNoMatchedSidecarSet(t, sidecarSetIn)
}

func testPodHasNoMatchedSidecarSet(t *testing.T, sidecarSetIn *appsv1alpha1.SidecarSet) {
	podIn := pod1.DeepCopy()
	podIn.Labels["app"] = "doesnt-match"
	podOut := podIn.DeepCopy()
	decoder, _ := admission.NewDecoder(scheme.Scheme)
	client := fake.NewClientBuilder().WithObjects(sidecarSetIn).Build()
	podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
	req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
	_ = podHandler.sidecarsetMutatingPod(context.Background(), req, podOut)

	if len(podOut.Spec.Containers) != len(podIn.Spec.Containers) {
		t.Fatalf("expect %v containers but got %v", len(podIn.Spec.Containers), len(podOut.Spec.Containers))
	}
}

func TestMergeSidecarSecrets(t *testing.T) {
	sidecarSetIn := sidecarSet1.DeepCopy()
	testMergeSidecarSecrets(t, sidecarSetIn)
}

func testMergeSidecarSecrets(t *testing.T, sidecarSetIn *appsv1alpha1.SidecarSet) {
	sidecarImagePullSecrets := []corev1.LocalObjectReference{
		{Name: "s0"}, {Name: "s1"}, {Name: "s2"},
	}
	podImagePullSecrets := []corev1.LocalObjectReference{
		{Name: "s2"}, {Name: "s3"}, {Name: "s3"},
	}
	// 3 + 3 - 2 = 4
	sidecarSetIn.Spec.ImagePullSecrets = sidecarImagePullSecrets
	doMergeSidecarSecretsTest(t, sidecarSetIn, podImagePullSecrets, 2)
	// 3 + 3 - 0 = 6
	podImagePullSecrets = []corev1.LocalObjectReference{
		{Name: "s3"}, {Name: "s4"}, {Name: "s5"},
	}
	doMergeSidecarSecretsTest(t, sidecarSetIn, podImagePullSecrets, 0)
	// 3 + 0 - 0 = 3
	doMergeSidecarSecretsTest(t, sidecarSetIn, nil, 0)
	// 0 + 3 - 0 = 3
	sidecarSetIn.Spec.ImagePullSecrets = nil
	doMergeSidecarSecretsTest(t, sidecarSetIn, podImagePullSecrets, 0)
}

func doMergeSidecarSecretsTest(t *testing.T, sidecarSetIn *appsv1alpha1.SidecarSet, podImagePullSecrets []corev1.LocalObjectReference, repeat int) {
	podIn := pod1.DeepCopy()
	podIn.Spec.ImagePullSecrets = podImagePullSecrets
	podOut := podIn.DeepCopy()
	decoder, _ := admission.NewDecoder(scheme.Scheme)
	client := fake.NewClientBuilder().WithObjects(sidecarSetIn).Build()
	podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
	req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
	_ = podHandler.sidecarsetMutatingPod(context.Background(), req, podOut)

	if len(podOut.Spec.ImagePullSecrets) != len(podIn.Spec.ImagePullSecrets)+len(sidecarSetIn.Spec.ImagePullSecrets)-repeat {
		t.Fatalf("expect %v secrets but got %v", len(podIn.Spec.ImagePullSecrets)+len(sidecarSetIn.Spec.ImagePullSecrets)-repeat, len(podOut.Spec.ImagePullSecrets))
	}
}

func TestInjectionStrategyPaused(t *testing.T) {
	sidecarSetIn := sidecarSet1.DeepCopy()
	testInjectionStrategyPaused(t, sidecarSetIn)
}

func testInjectionStrategyPaused(t *testing.T, sidecarIn *appsv1alpha1.SidecarSet) {
	podIn := pod1.DeepCopy()
	podOut := podIn.DeepCopy()
	sidecarPaused := sidecarIn
	sidecarPaused.Spec.InjectionStrategy.Paused = true
	decoder, _ := admission.NewDecoder(scheme.Scheme)
	client := fake.NewClientBuilder().WithObjects(sidecarPaused).Build()
	podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
	req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
	_ = podHandler.sidecarsetMutatingPod(context.Background(), req, podOut)

	if len(podOut.Spec.Containers) != len(podIn.Spec.Containers) {
		t.Fatalf("expect %v containers but got %v", len(podIn.Spec.Containers), len(podOut.Spec.Containers))
	}
}

func TestInjectionStrategyRevision(t *testing.T) {
	spec := map[string]interface{}{
		"spec": map[string]interface{}{
			"initContainers": []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "init-2",
						Image: "busybox:1.0.0",
					},
				},
			},
			"containers": []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns-f-image:1.0",
					},
					PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
					ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
						Type: appsv1alpha1.ShareVolumePolicyDisabled,
					},
				},
			},
		},
	}

	raw, _ := json.Marshal(spec)
	revisionName := fmt.Sprintf("%s-12345", sidecarSet1.Name)
	sidecarSetIn := sidecarSet1.DeepCopy()
	sidecarSetIn.Spec.InjectionStrategy.Revision = revisionName
	historyInjection := []runtime.Object{
		sidecarSetIn,
		&apps.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: webhookutil.GetNamespace(),
				Name:      revisionName,
			},
			Data: runtime.RawExtension{
				Raw: raw,
			},
		},
	}
	testInjectionStrategyRevision(t, historyInjection)
}

func testInjectionStrategyRevision(t *testing.T, env []runtime.Object) {
	podIn := pod1.DeepCopy()
	podOut := podIn.DeepCopy()
	decoder, _ := admission.NewDecoder(scheme.Scheme)
	client := fake.NewFakeClient(env...)
	podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
	req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
	_ = podHandler.sidecarsetMutatingPod(context.Background(), req, podOut)

	if len(podIn.Spec.Containers)+len(podIn.Spec.InitContainers)+2 != len(podOut.Spec.Containers)+len(podOut.Spec.InitContainers) {
		t.Fatalf("expect %v containers but got %v", len(podIn.Spec.Containers)+2, len(podOut.Spec.Containers))
	}
}

func TestSidecarSetPodInjectPolicy(t *testing.T) {
	sidecarSetIn := sidecarSet1.DeepCopy()
	testSidecarSetPodInjectPolicy(t, sidecarSetIn)
}

func testSidecarSetPodInjectPolicy(t *testing.T, sidecarSetIn *appsv1alpha1.SidecarSet) {
	podIn := pod1.DeepCopy()
	decoder, _ := admission.NewDecoder(scheme.Scheme)
	client := fake.NewClientBuilder().WithObjects(sidecarSetIn).Build()
	podOut := podIn.DeepCopy()
	podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
	req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
	err := podHandler.sidecarsetMutatingPod(context.Background(), req, podOut)
	if err != nil {
		t.Fatalf("inject sidecar into pod failed, err: %v", err)
	}

	expectLen := len(podIn.Spec.Containers) + len(sidecarSetIn.Spec.Containers)
	if len(podOut.Spec.Containers) != expectLen {
		t.Fatalf("expect %v containers but got %v", expectLen, len(podOut.Spec.Containers))
	}

	for i, container := range podOut.Spec.Containers {
		switch i {
		case 0:
			if container.Name != "dns-f" {
				t.Fatalf("expect dns-f but got %v", container.Name)
			}
		case 1:
			if container.Name != "nginx" {
				t.Fatalf("expect nginx but got %v", container.Name)
			}
		case 2:
			if container.Name != "log-agent" {
				t.Fatalf("expect log-agent but got %v", container.Name)
			}
		}
	}

	expectInitLen := len(podIn.Spec.InitContainers) + len(sidecarSetIn.Spec.InitContainers)
	if len(podOut.Spec.InitContainers) != expectInitLen {
		t.Fatalf("expect %v initContainers but got %v", expectLen, len(podOut.Spec.InitContainers))
	}

	for i, container := range podOut.Spec.InitContainers {
		//injected container must be contain env "IS_INJECTED"
		if i > 0 {
			exist := false
			for _, env := range container.Env {
				if env.Name == sidecarcontrol.SidecarEnvKey {
					exist = true
					break
				}
			}
			if !exist {
				t.Fatalf("Injected initContainer %v don't contain env(%v)", container.Name, sidecarcontrol.SidecarEnvKey)
			}
		}

		switch i {
		case 0:
			if container.Name != "init-0" {
				t.Fatalf("expect dns-f but got %v", container.Name)
			}
		case 1:
			if container.Name != "init-1" {
				t.Fatalf("expect nginx but got %v", container.Name)
			}
		case 2:
			if container.Name != "init-2" {
				t.Fatalf("expect log-agent but got %v", container.Name)
			}
		}
	}
}

func TestSidecarVolumesAppend(t *testing.T) {
	sidecarSetIn := sidecarsetWithTransferEnv.DeepCopy()
	testSidecarVolumesAppend(t, sidecarSetIn)
}

func testSidecarVolumesAppend(t *testing.T, sidecarSetIn *appsv1alpha1.SidecarSet) {
	podIn := pod1.DeepCopy()

	decoder, _ := admission.NewDecoder(scheme.Scheme)
	client := fake.NewClientBuilder().WithObjects(sidecarSetIn).Build()
	podOut := podIn.DeepCopy()
	podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
	req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
	err := podHandler.sidecarsetMutatingPod(context.Background(), req, podOut)
	if err != nil {
		t.Fatalf("inject sidecar into pod failed, err: %v", err)
	}

	expectLen := len(podIn.Spec.Volumes) + 1
	if len(podOut.Spec.Volumes) != expectLen {
		t.Fatalf("expect %v volumes but got %v", expectLen, len(podOut.Spec.Volumes))
	}

	for i, volume := range podOut.Spec.Volumes {
		switch i {
		case 0:
			if volume.Name != "volume-a" {
				t.Fatalf("expect volume-a but got %v", volume.Name)
			}
		case 1:
			if volume.Name != "volume-b" {
				t.Fatalf("expect volume-b but got %v", volume.Name)
			}
		case 2:
			if volume.Name != "volume-1" {
				t.Fatalf("expect volume-1 but got %v", volume.Name)
			}
		}
	}
}

func TestPodSidecarSetHashCompatibility(t *testing.T) {
	podIn := pod1.DeepCopy()
	podIn.Annotations = map[string]string{}
	podIn.Annotations[sidecarcontrol.SidecarSetHashAnnotation] = `{"sidecarset-test":"bv6d2fbw97wz8xx5x4v4wddwbd5z744wcf7c786dd4dvxvd5w6w424df7vx47989"}`
	podIn.Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] = `{"sidecarset-test":"54x5977vf9zz4248w7v44456zf655b8bcffv7x74w88f6dwb994fw48b8f9b8959"}`
	_, _, _, _, annotations, err := buildSidecars(false, podIn, nil, nil)
	if err != nil {
		t.Fatalf("compatible pod sidecarSet Hash failed: %s", err.Error())
	}
	// format: sidecarset.name -> sidecarset hash
	sidecarSetHash := make(map[string]sidecarcontrol.SidecarSetUpgradeSpec)
	// format: sidecarset.name -> sidecarset hash(without image)
	sidecarSetHashWithoutImage := make(map[string]sidecarcontrol.SidecarSetUpgradeSpec)
	// parse sidecar hash in pod annotations
	if oldHashStr := annotations[sidecarcontrol.SidecarSetHashAnnotation]; len(oldHashStr) > 0 {
		if err := json.Unmarshal([]byte(oldHashStr), &sidecarSetHash); err != nil {
			t.Fatalf("compatible pod sidecarSet Hash failed: %s", err.Error())
		}
	}
	if oldHashStr := annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation]; len(oldHashStr) > 0 {
		if err := json.Unmarshal([]byte(oldHashStr), &sidecarSetHashWithoutImage); err != nil {
			t.Fatalf("compatible pod sidecarSet Hash failed: %s", err.Error())
		}
	}
}

func TestPodVolumeMountsAppend(t *testing.T) {
	sidecarSetIn := sidecarSetWithStaragent.DeepCopy()
	// /a/b/c, /d/e/f, /staragent
	sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
	testPodVolumeMountsAppend(t, sidecarSetIn)
}

func testPodVolumeMountsAppend(t *testing.T, sidecarSetIn *appsv1alpha1.SidecarSet) {
	// /a/b、/e/f
	podIn := podWithStaragent.DeepCopy()
	cases := []struct {
		name               string
		getPod             func() *corev1.Pod
		getSidecarSets     func() *appsv1alpha1.SidecarSet
		exceptVolumeMounts []string
		exceptEnvs         []string
	}{
		{
			name: "append normal volumeMounts",
			getPod: func() *corev1.Pod {
				return podIn.DeepCopy()
			},
			getSidecarSets: func() *appsv1alpha1.SidecarSet {
				return sidecarSetIn.DeepCopy()
			},
			exceptVolumeMounts: []string{"/a/b", "/e/f", "/a/b/c", "/d/e/f", "/staragent"},
		},
		{
			name: "append volumeMounts SubPathExpr, volumes with expanded subpath",
			getPod: func() *corev1.Pod {
				podOut := podIn.DeepCopy()
				podOut.Spec.Containers[0].VolumeMounts = append(podOut.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
					Name:        "volume-expansion",
					MountPath:   "/e/expansion",
					SubPathExpr: "foo/$(POD_NAME)/$(OD_NAME)/conf",
				})
				podOut.Spec.Containers[0].Env = append(podOut.Spec.Containers[0].Env, corev1.EnvVar{
					Name:  "POD_NAME",
					Value: "bar",
				})
				podOut.Spec.Containers[0].Env = append(podOut.Spec.Containers[0].Env, corev1.EnvVar{
					Name:  "OD_NAME",
					Value: "od_name",
				})
				return podOut
			},
			getSidecarSets: func() *appsv1alpha1.SidecarSet {
				return sidecarSetIn.DeepCopy()
			},
			exceptVolumeMounts: []string{"/a/b", "/e/f", "/a/b/c", "/d/e/f", "/staragent", "/e/expansion"},
			exceptEnvs:         []string{"POD_NAME", "OD_NAME"},
		},
		{
			name: "append volumeMounts SubPathExpr, subpath with no expansion",
			getPod: func() *corev1.Pod {
				podOut := podIn.DeepCopy()
				podOut.Spec.Containers[0].VolumeMounts = append(podOut.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
					Name:        "volume-expansion",
					MountPath:   "/e/expansion",
					SubPathExpr: "foo",
				})
				return podOut
			},
			getSidecarSets: func() *appsv1alpha1.SidecarSet {
				return sidecarSetIn.DeepCopy()
			},
			exceptVolumeMounts: []string{"/a/b", "/e/f", "/a/b/c", "/d/e/f", "/staragent", "/e/expansion"},
		},
		{
			name: "append volumeMounts SubPathExpr, volumes expanded with empty subpath",
			getPod: func() *corev1.Pod {
				podOut := podIn.DeepCopy()
				podOut.Spec.Containers[0].VolumeMounts = append(podOut.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
					Name:        "volume-expansion",
					MountPath:   "/e/expansion",
					SubPathExpr: "",
				})
				return podOut
			},
			getSidecarSets: func() *appsv1alpha1.SidecarSet {
				return sidecarSetIn.DeepCopy()
			},
			exceptVolumeMounts: []string{"/a/b", "/e/f", "/a/b/c", "/d/e/f", "/staragent", "/e/expansion"},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			podIn := cs.getPod()
			decoder, _ := admission.NewDecoder(scheme.Scheme)
			client := fake.NewClientBuilder().WithObjects(cs.getSidecarSets()).Build()
			podOut := podIn.DeepCopy()
			podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
			req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
			err := podHandler.sidecarsetMutatingPod(context.Background(), req, podOut)
			if err != nil {
				t.Fatalf("inject sidecar into pod failed, err: %v", err)
			}

			for _, mount := range cs.exceptVolumeMounts {
				if util.GetContainerVolumeMount(&podOut.Spec.Containers[1], mount) == nil {
					t.Fatalf("expect volume mounts %s but got nil", mount)
				}
			}

			for _, env := range cs.exceptEnvs {
				if util.GetContainerEnvVar(&podOut.Spec.Containers[1], env) == nil {
					t.Fatalf("expect env %s but got nil", env)
				}
			}
		})
	}
}

func TestSidecarSetTransferEnv(t *testing.T) {
	sidecarSetIn := sidecarsetWithTransferEnv.DeepCopy()
	testSidecarSetTransferEnv(t, sidecarSetIn)
}

func testSidecarSetTransferEnv(t *testing.T, sidecarSetIn *appsv1alpha1.SidecarSet) {
	podIn := pod1.DeepCopy()
	decoder, _ := admission.NewDecoder(scheme.Scheme)
	client := fake.NewClientBuilder().WithObjects(sidecarSetIn).Build()
	podOut := podIn.DeepCopy()
	podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
	req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
	err := podHandler.sidecarsetMutatingPod(context.Background(), req, podOut)
	if err != nil {
		t.Fatalf("inject sidecar into pod failed, err: %v", err)
	}

	if len(podOut.Spec.Containers[0].Env) != 2 {
		t.Fatalf("expect 2 envs but got %v", len(podOut.Spec.Containers[0].Env))
	}
	if podOut.Spec.Containers[0].Env[1].Value != "world2" {
		t.Fatalf("expect env with value 'world2' but got %v", podOut.Spec.Containers[0].Env[1].Value)
	}
}

func TestSidecarSetHashInject(t *testing.T) {
	sidecarSetIn1 := sidecarSet1.DeepCopy()
	testSidecarSetHashInject(t, sidecarSetIn1)
}

func testSidecarSetHashInject(t *testing.T, sidecarSetIn1 *appsv1alpha1.SidecarSet) {
	podIn := pod1.DeepCopy()
	sidecarSetIn1.Spec.Selector.MatchLabels["app"] = "doesnt-match"
	sidecarSetIn2 := sidecarsetWithTransferEnv.DeepCopy()
	sidecarSetIn3 := sidecarSet3.DeepCopy()

	decoder, _ := admission.NewDecoder(scheme.Scheme)
	client := fake.NewClientBuilder().WithObjects(sidecarSetIn1, sidecarSetIn2, sidecarSetIn3).Build()
	podOut := podIn.DeepCopy()
	podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
	req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
	err := podHandler.sidecarsetMutatingPod(context.Background(), req, podOut)
	if err != nil {
		t.Fatalf("inject sidecar into pod failed, err: %v", err)
	}

	hashKey := sidecarcontrol.SidecarSetHashAnnotation
	//expectedAnnotation := `{"sidecarset2":"c4k2dbb95d","sidecarset3":"gm967682cm"}`
	expectedRevision := map[string]string{
		"sidecarset2": "c4k2dbb95d",
		"sidecarset3": "gm967682cm",
	}
	for k, v := range expectedRevision {
		if sidecarcontrol.GetPodSidecarSetRevision(k, podOut) != v {
			t.Errorf("except sidecarset(%s:%s), but get in pod annotations(%s)", k, v, podOut.Annotations[hashKey])
		}
	}
}

func TestSidecarSetNameInject(t *testing.T) {
	sidecarSetIn1 := sidecarSet1.DeepCopy()
	sidecarSetIn3 := sidecarSet3.DeepCopy()
	testSidecarSetNameInject(t, sidecarSetIn1, sidecarSetIn3)
}

func testSidecarSetNameInject(t *testing.T, sidecarSetIn1, sidecarSetIn3 *appsv1alpha1.SidecarSet) {
	podIn := pod1.DeepCopy()
	decoder, _ := admission.NewDecoder(scheme.Scheme)
	client := fake.NewClientBuilder().WithObjects(sidecarSetIn1, sidecarSetIn3).Build()
	podOut := podIn.DeepCopy()
	podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
	req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
	err := podHandler.sidecarsetMutatingPod(context.Background(), req, podOut)
	if err != nil {
		t.Fatalf("inject sidecar into pod failed, err: %v", err)
	}
	sidecarSetListKey := sidecarcontrol.SidecarSetListAnnotation
	expectedAnnotation := "sidecarset1,sidecarset3"
	if podOut.Annotations[sidecarSetListKey] != expectedAnnotation {
		t.Errorf("expect annotation %v but got %v", expectedAnnotation, podOut.Annotations[sidecarSetListKey])
	}
}

func TestMergeSidecarContainers(t *testing.T) {
	podContainers := []corev1.Container{
		{
			Name: "sidecar-1",
		},
		{
			Name: "app-container",
		},
		{
			Name: "sidecar-2",
		},
	}

	sidecarContainers := []*appsv1alpha1.SidecarContainer{
		{
			Container: corev1.Container{
				Name: "sidecar-1",
			},
			PodInjectPolicy: appsv1alpha1.AfterAppContainerType,
		},
		{
			Container: corev1.Container{
				Name: "sidecar-2",
			},
			PodInjectPolicy: appsv1alpha1.AfterAppContainerType,
		},
		{
			Container: corev1.Container{
				Name: "new-sidecar-1",
			},
			PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
		},
	}

	cases := []struct {
		name               string
		getOrigins         func() []corev1.Container
		getInjected        func() []*appsv1alpha1.SidecarContainer
		expectContainerLen int
		expectedContainers []string
	}{
		{
			name: "origins not sidecar, and inject new sidecar",
			getOrigins: func() []corev1.Container {
				return podContainers[1:2]
			},
			getInjected: func() []*appsv1alpha1.SidecarContainer {
				return sidecarContainers
			},
			expectContainerLen: 4,
			expectedContainers: []string{"new-sidecar-1", "app-container", "sidecar-1", "sidecar-2"},
		},
		{
			name: "origins not sidecar, and inject new sidecar, only before app container",
			getOrigins: func() []corev1.Container {
				return podContainers[1:2]
			},
			getInjected: func() []*appsv1alpha1.SidecarContainer {
				return sidecarContainers[2:]
			},
			expectContainerLen: 2,
			expectedContainers: []string{"new-sidecar-1", "app-container"},
		},
		{
			name: "origins not sidecar, and inject new sidecar, only after app container",
			getOrigins: func() []corev1.Container {
				return podContainers[1:2]
			},
			getInjected: func() []*appsv1alpha1.SidecarContainer {
				return sidecarContainers[:2]
			},
			expectContainerLen: 3,
			expectedContainers: []string{"app-container", "sidecar-1", "sidecar-2"},
		},
		{
			name: "origin have sidecars, sidecar no new containers",
			getOrigins: func() []corev1.Container {
				return podContainers
			},
			getInjected: func() []*appsv1alpha1.SidecarContainer {
				return sidecarContainers[:2]
			},
			expectContainerLen: 3,
			expectedContainers: []string{"sidecar-1", "app-container", "sidecar-2"},
		},
		{
			name: "origin have sidecars, sidecar have new containers",
			getOrigins: func() []corev1.Container {
				return podContainers
			},
			getInjected: func() []*appsv1alpha1.SidecarContainer {
				return sidecarContainers
			},
			expectContainerLen: 4,
			expectedContainers: []string{"new-sidecar-1", "sidecar-1", "app-container", "sidecar-2"},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			origins := cs.getOrigins()
			injected := cs.getInjected()
			finals := mergeSidecarContainers(origins, injected)
			if len(finals) != cs.expectContainerLen {
				t.Fatalf("expect %d containers but got %v", cs.expectContainerLen, len(finals))
			}
			for index, cName := range cs.expectedContainers {
				if finals[index].Name != cName {
					t.Fatalf("expect index(%d) container(%s) but got %s", index, cName, finals[index].Name)
				}
			}
		})
	}
}

func newAdmission(op admissionv1.Operation, object, oldObject runtime.RawExtension, subResource string) admission.Request {
	return admission.Request{
		AdmissionRequest: newAdmissionRequest(op, object, oldObject, subResource),
	}
}

func newAdmissionRequest(op admissionv1.Operation, object, oldObject runtime.RawExtension, subResource string) admissionv1.AdmissionRequest {
	return admissionv1.AdmissionRequest{
		Resource:    metav1.GroupVersionResource{Group: corev1.SchemeGroupVersion.Group, Version: corev1.SchemeGroupVersion.Version, Resource: "pods"},
		Operation:   op,
		Object:      object,
		OldObject:   oldObject,
		SubResource: subResource,
	}
}
