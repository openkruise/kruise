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
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util/fieldindex"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestInjectHotUpgradeSidecar(t *testing.T) {
	sidecarSetIn := sidecarSet1.DeepCopy()
	sidecarSetIn.Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] = "without-c4k2dbb95d"
	sidecarSetIn.Spec.Containers[0].UpgradeStrategy.UpgradeType = appsv1alpha1.SidecarContainerHotUpgrade
	sidecarSetIn.Spec.Containers[0].UpgradeStrategy.HotUpgradeEmptyImage = "busy:hotupgrade-empty"
	testInjectHotUpgradeSidecar(t, sidecarSetIn)
}

func testInjectHotUpgradeSidecar(t *testing.T, sidecarSetIn *appsv1alpha1.SidecarSet) {
	podIn := pod1.DeepCopy()
	decoder := admission.NewDecoder(scheme.Scheme)
	client := fake.NewClientBuilder().WithObjects(sidecarSetIn).WithIndex(
		&appsv1alpha1.SidecarSet{}, fieldindex.IndexNameForSidecarSetNamespace, fieldindex.IndexSidecarSet,
	).Build()
	podOut := podIn.DeepCopy()
	podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
	req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
	_, err := podHandler.sidecarsetMutatingPod(context.Background(), req, podOut)
	if err != nil {
		t.Fatalf("inject sidecar into pod failed, err: %v", err)
	}
	if len(podOut.Spec.Containers) != 4 {
		t.Fatalf("expect 4 containers but got %v", len(podOut.Spec.Containers))
	}
	if podOut.Spec.Containers[0].Image != sidecarSetIn.Spec.Containers[0].Image {
		t.Fatalf("expect image %v but got %v", sidecarSetIn.Spec.Containers[0].Image, podOut.Spec.Containers[0].Image)
	}
	if podOut.Spec.Containers[1].Image != sidecarSetIn.Spec.Containers[0].UpgradeStrategy.HotUpgradeEmptyImage {
		t.Fatalf("expect image busy:hotupgrade-empty but got %v", podOut.Spec.Containers[1].Image)
	}
	if sidecarcontrol.GetPodSidecarSetRevision("sidecarset1", podOut) != sidecarcontrol.GetSidecarSetRevision(sidecarSetIn) {
		t.Fatalf("pod sidecarset revision(%s) error", sidecarcontrol.GetPodSidecarSetRevision("sidecarset1", podOut))
	}
	if sidecarcontrol.GetPodSidecarSetWithoutImageRevision("sidecarset1", podOut) != sidecarcontrol.GetSidecarSetWithoutImageRevision(sidecarSetIn) {
		t.Fatalf("pod sidecarset without image revision(%s) error", sidecarcontrol.GetPodSidecarSetWithoutImageRevision("sidecarset1", podOut))
	}
	if podOut.Annotations[sidecarcontrol.SidecarSetListAnnotation] != "sidecarset1" {
		t.Fatalf("pod annotations[%s]=%s error", sidecarcontrol.SidecarSetListAnnotation, podOut.Annotations[sidecarcontrol.SidecarSetListAnnotation])
	}
	if sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podOut)["dns-f"] != "dns-f-1" {
		t.Fatalf("pod annotations[%s]=%s error", sidecarcontrol.SidecarSetWorkingHotUpgradeContainer, podOut.Annotations[sidecarcontrol.SidecarSetWorkingHotUpgradeContainer])
	}
	if podOut.Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation("dns-f-1")] != "1" {
		t.Fatalf("pod annotations dns-f-1 version=%s", podOut.Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation("dns-f-1")])
	}
	if podOut.Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation("dns-f-2")] != "0" {
		t.Fatalf("pod annotations dns-f-2 version=%s", podOut.Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation("dns-f-2")])
	}
}
