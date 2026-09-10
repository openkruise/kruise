/*
Copyright 2025 The Kruise Authors.

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

package inplaceupdate

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/appscode/jsonpatch"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeletcontainer "k8s.io/kubernetes/pkg/kubelet/container"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

func restartAlways() *v1.ContainerRestartPolicy {
	p := v1.ContainerRestartPolicyAlways
	return &p
}

func setInitContainerGate(t *testing.T, enabled bool) {
	t.Helper()
	if err := utilfeature.DefaultMutableFeatureGate.Set(
		fmt.Sprintf("%s=%v", features.InPlaceUpdateRestartableInitContainer, enabled)); err != nil {
		t.Fatalf("failed to set feature-gate: %v", err)
	}
}

func revisionOf(name, raw string) *apps.ControllerRevision {
	return &apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       runtime.RawExtension{Raw: []byte(raw)},
	}
}

// TestCalculateSpecForRestartableInitContainer covers the gating rules of in-place updating
// the image of an init container.
func TestCalculateSpecForRestartableInitContainer(t *testing.T) {
	const (
		// a restartable init container (native sidecar), image changes foo1 -> foo2
		oldSidecar = `{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"main1"}],"initContainers":[{"name":"sidecar","image":"foo1","restartPolicy":"Always"}]}}}}`
		newSidecar = `{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"main1"}],"initContainers":[{"name":"sidecar","image":"foo2","restartPolicy":"Always"}]}}}}`
		// a regular init container (no restartPolicy), image changes foo1 -> foo2
		oldRegular = `{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"main1"}],"initContainers":[{"name":"setup","image":"foo1"}]}}}}`
		newRegular = `{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"main1"}],"initContainers":[{"name":"setup","image":"foo2"}]}}}}`
		// restartPolicy itself is being removed together with the image change
		newSidecarDemoted = `{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"main1"}],"initContainers":[{"name":"sidecar","image":"foo2"}]}}}}`
	)

	cases := []struct {
		name        string
		gateEnabled bool
		old, new    string
		expectNil   bool
		expectInit  map[string]string
	}{
		{
			name:        "gate disabled falls back to recreate",
			gateEnabled: false,
			old:         oldSidecar,
			new:         newSidecar,
			expectNil:   true,
		},
		{
			name:        "restartable init container can be in-place updated",
			gateEnabled: true,
			old:         oldSidecar,
			new:         newSidecar,
			expectInit:  map[string]string{"sidecar": "foo2"},
		},
		{
			name:        "regular init container falls back to recreate",
			gateEnabled: true,
			old:         oldRegular,
			new:         newRegular,
			expectNil:   true,
		},
		{
			name:        "demoting a sidecar falls back to recreate",
			gateEnabled: true,
			old:         oldSidecar,
			new:         newSidecarDemoted,
			expectNil:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setInitContainerGate(t, tc.gateEnabled)
			defer setInitContainerGate(t, false)

			got := defaultCalculateInPlaceUpdateSpec(
				revisionOf("old-revision", tc.old), revisionOf("new-revision", tc.new), nil)

			if tc.expectNil {
				if got != nil {
					t.Fatalf("expected nil spec (recreate), got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected a non-nil spec, got nil")
			}
			if len(got.InitContainerImages) != len(tc.expectInit) {
				t.Fatalf("expected initContainerImages %v, got %v", tc.expectInit, got.InitContainerImages)
			}
			for k, v := range tc.expectInit {
				if got.InitContainerImages[k] != v {
					t.Fatalf("expected initContainerImages %v, got %v", tc.expectInit, got.InitContainerImages)
				}
			}
		})
	}
}

// TestCalculateSpecKeepsInitContainerImagesNilByDefault makes sure the calculated spec is
// unchanged for the ordinary containers-only case, so that existing consumers and the
// serialized annotation are not affected when the feature is not used.
func TestCalculateSpecKeepsInitContainerImagesNilByDefault(t *testing.T) {
	setInitContainerGate(t, true)
	defer setInitContainerGate(t, false)

	old := `{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`
	new := `{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`

	got := defaultCalculateInPlaceUpdateSpec(revisionOf("old", old), revisionOf("new", new), nil)
	if got == nil {
		t.Fatalf("expected a non-nil spec")
	}
	if got.InitContainerImages != nil {
		t.Fatalf("expected InitContainerImages to stay nil, got %v", got.InitContainerImages)
	}
}

// TestPatchRestartableInitContainerToPod verifies that the new image is written into
// spec.initContainers and the old imageID is recorded from status.initContainerStatuses.
func TestPatchRestartableInitContainerToPod(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Annotations: map[string]string{}},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				{Name: "sidecar", Image: "foo1", RestartPolicy: restartAlways()},
			},
			Containers: []v1.Container{{Name: "c1", Image: "main1"}},
		},
		Status: v1.PodStatus{
			InitContainerStatuses: []v1.ContainerStatus{
				{Name: "sidecar", Image: "foo1", ImageID: "img-old"},
			},
			ContainerStatuses: []v1.ContainerStatus{
				{Name: "c1", Image: "main1", ImageID: "main-old"},
			},
		},
	}

	spec := &UpdateSpec{
		Revision:            "new-revision",
		InitContainerImages: map[string]string{"sidecar": "foo2"},
	}
	state := &appspub.InPlaceUpdateState{Revision: "new-revision"}

	got, _, err := defaultPatchUpdateSpecToPod(pod.DeepCopy(), spec, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Spec.InitContainers[0].Image != "foo2" {
		t.Fatalf("expected init container image to be patched to foo2, got %s", got.Spec.InitContainers[0].Image)
	}
	if got.Spec.Containers[0].Image != "main1" {
		t.Fatalf("main container image should not change, got %s", got.Spec.Containers[0].Image)
	}
	if cs, ok := state.LastContainerStatuses["sidecar"]; !ok || cs.ImageID != "img-old" {
		t.Fatalf("expected the old imageID of the sidecar to be recorded, got %v", state.LastContainerStatuses)
	}
}

// TestCheckCompletedForRestartableInitContainer covers the imageID based fallback, which is used
// when kruise-daemon is not deployed and therefore runtime-container-meta is absent.
func TestCheckCompletedForRestartableInitContainer(t *testing.T) {
	newPod := func(specImage, statusImage, statusImageID string) *v1.Pod {
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1"},
			Spec: v1.PodSpec{
				InitContainers: []v1.Container{
					{Name: "sidecar", Image: specImage, RestartPolicy: restartAlways()},
				},
				Containers: []v1.Container{{Name: "c1", Image: "main1"}},
			},
			Status: v1.PodStatus{
				InitContainerStatuses: []v1.ContainerStatus{
					{Name: "sidecar", Image: statusImage, ImageID: statusImageID},
				},
				ContainerStatuses: []v1.ContainerStatus{
					{Name: "c1", Image: "main1", ImageID: "main-old"},
				},
			},
		}
	}

	state := func() *appspub.InPlaceUpdateState {
		return &appspub.InPlaceUpdateState{
			Revision:     "new-revision",
			UpdateImages: true,
			LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{
				"sidecar": {ImageID: "img-old"},
			},
		}
	}

	// kubelet has not restarted the sidecar yet: imageID is still the old one.
	if err := defaultCheckContainersInPlaceUpdateCompleted(
		newPod("foo2:v2", "foo1:v1", "img-old"), state()); err == nil {
		t.Fatalf("expected not-completed while the sidecar imageID has not changed")
	}

	// kubelet has restarted the sidecar with the new image.
	if err := defaultCheckContainersInPlaceUpdateCompleted(
		newPod("foo2:v2", "foo2:v2", "img-new"), state()); err != nil {
		t.Fatalf("expected completed, got %v", err)
	}
}

// TestHashConsistentCoversRestartableInitContainer verifies that the runtime-container-meta fast
// path now covers the restartable init containers, so a sidecar whose image has not actually been
// restarted yet can no longer be mistaken for completed.
func TestHashConsistentCoversRestartableInitContainer(t *testing.T) {
	sidecarOld := v1.Container{Name: "sidecar", Image: "foo1", RestartPolicy: restartAlways()}
	sidecarNew := v1.Container{Name: "sidecar", Image: "foo2", RestartPolicy: restartAlways()}
	main := v1.Container{Name: "c1", Image: "main1"}

	// The meta reported by kruise-daemon still describes the OLD sidecar.
	metaSet := &appspub.RuntimeContainerMetaSet{
		Containers: []appspub.RuntimeContainerMeta{
			{
				Name:        "sidecar",
				ContainerID: "docker://sidecar-1",
				Hashes:      appspub.RuntimeContainerHashes{PlainHash: kubeletcontainer.HashContainer(&sidecarOld)},
			},
			{
				Name:        "c1",
				ContainerID: "docker://c1-1",
				Hashes:      appspub.RuntimeContainerHashes{PlainHash: kubeletcontainer.HashContainer(&main)},
			},
		},
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1"},
		Spec:       v1.PodSpec{InitContainers: []v1.Container{sidecarNew}, Containers: []v1.Container{main}},
		Status: v1.PodStatus{
			InitContainerStatuses: []v1.ContainerStatus{{Name: "sidecar", ContainerID: "docker://sidecar-1"}},
			ContainerStatuses:     []v1.ContainerStatus{{Name: "c1", ContainerID: "docker://c1-1"}},
		},
	}

	// spec says foo2 but the meta still hashes foo1 -> not consistent yet.
	if checkAllContainersHashConsistent(pod, metaSet, plainHash) {
		t.Fatalf("expected inconsistent while the sidecar has not been restarted")
	}

	// after kubelet restarted the sidecar, kruise-daemon reports the new hash and containerID.
	metaSet.Containers[0].ContainerID = "docker://sidecar-2"
	metaSet.Containers[0].Hashes.PlainHash = kubeletcontainer.HashContainer(&sidecarNew)
	pod.Status.InitContainerStatuses[0].ContainerID = "docker://sidecar-2"
	if !checkAllContainersHashConsistent(pod, metaSet, plainHash) {
		t.Fatalf("expected consistent after the sidecar has been restarted")
	}
}

// TestHashConsistentIgnoresRegularInitContainer makes sure a regular (non-restartable) init
// container is not required to appear in runtime-container-meta, for kruise-daemon does not
// report it and kubelet never restarts it on spec change.
func TestHashConsistentIgnoresRegularInitContainer(t *testing.T) {
	main := v1.Container{Name: "c1", Image: "main1"}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1"},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{{Name: "setup", Image: "bar1"}},
			Containers:     []v1.Container{main},
		},
		Status: v1.PodStatus{
			InitContainerStatuses: []v1.ContainerStatus{{Name: "setup", ContainerID: "docker://setup-1"}},
			ContainerStatuses:     []v1.ContainerStatus{{Name: "c1", ContainerID: "docker://c1-1"}},
		},
	}
	metaSet := &appspub.RuntimeContainerMetaSet{
		Containers: []appspub.RuntimeContainerMeta{
			{
				Name:        "c1",
				ContainerID: "docker://c1-1",
				Hashes:      appspub.RuntimeContainerHashes{PlainHash: kubeletcontainer.HashContainer(&main)},
			},
		},
	}

	if !checkAllContainersHashConsistent(pod, metaSet, plainHash) {
		t.Fatalf("expected consistent, the regular init container should be ignored")
	}
}

func TestDiffRestartableInitContainerImages(t *testing.T) {
	oldTemp := &v1.PodTemplateSpec{Spec: v1.PodSpec{
		InitContainers: []v1.Container{
			{Name: "sidecar", Image: "foo1", RestartPolicy: restartAlways()},
			{Name: "setup", Image: "bar1"},
		},
	}}
	newTemp := &v1.PodTemplateSpec{Spec: v1.PodSpec{
		InitContainers: []v1.Container{
			{Name: "sidecar", Image: "foo2", RestartPolicy: restartAlways()},
			{Name: "setup", Image: "bar2"},
		},
	}}

	setInitContainerGate(t, false)
	if got := DiffRestartableInitContainerImages(oldTemp, newTemp); got != nil {
		t.Fatalf("expected nil when the feature-gate is disabled, got %v", got)
	}

	setInitContainerGate(t, true)
	defer setInitContainerGate(t, false)
	got := DiffRestartableInitContainerImages(oldTemp, newTemp)
	if len(got) != 1 || got["sidecar"] != "foo2" {
		t.Fatalf("expected only the restartable init container to be diffed, got %v", got)
	}
}

// TestValidateInPlaceOnlyTemplateSpecPatches covers the InPlaceOnly whitelist used by the
// CloneSet / Advanced StatefulSet validating webhooks. Without accepting the images of the
// restartable init containers here, the workload controllers would never get the chance to
// in-place update them, for the update request is rejected by the admission webhook first.
func TestValidateInPlaceOnlyTemplateSpecPatches(t *testing.T) {
	templateOf := func(mainImage, initImage string, restartable bool, extra func(*v1.PodTemplateSpec)) *v1.PodTemplateSpec {
		temp := &v1.PodTemplateSpec{Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				{Name: "setup", Image: "setup1"},
				{Name: "sidecar", Image: initImage},
			},
			Containers: []v1.Container{{Name: "c1", Image: mainImage}},
		}}
		if restartable {
			temp.Spec.InitContainers[1].RestartPolicy = restartAlways()
		}
		if extra != nil {
			extra(temp)
		}
		return temp
	}

	cases := []struct {
		name        string
		gateEnabled bool
		oldTemp     *v1.PodTemplateSpec
		newTemp     *v1.PodTemplateSpec
		expectErr   bool
	}{
		{
			name:        "regular container image is always allowed",
			gateEnabled: false,
			oldTemp:     templateOf("main1", "foo1", true, nil),
			newTemp:     templateOf("main2", "foo1", true, nil),
			expectErr:   false,
		},
		{
			name:        "restartable init container image is allowed when the gate is enabled",
			gateEnabled: true,
			oldTemp:     templateOf("main1", "foo1", true, nil),
			newTemp:     templateOf("main1", "foo2", true, nil),
			expectErr:   false,
		},
		{
			name:        "restartable init container image is rejected when the gate is disabled",
			gateEnabled: false,
			oldTemp:     templateOf("main1", "foo1", true, nil),
			newTemp:     templateOf("main1", "foo2", true, nil),
			expectErr:   true,
		},
		{
			name:        "regular init container image is rejected",
			gateEnabled: true,
			oldTemp:     templateOf("main1", "foo1", false, nil),
			newTemp:     templateOf("main1", "foo2", false, nil),
			expectErr:   true,
		},
		{
			name:        "demoting a restartable init container is rejected",
			gateEnabled: true,
			oldTemp:     templateOf("main1", "foo1", true, nil),
			newTemp:     templateOf("main1", "foo2", false, nil),
			expectErr:   true,
		},
		{
			name:        "both a regular and a restartable init container image is rejected",
			gateEnabled: true,
			oldTemp:     templateOf("main1", "foo1", true, nil),
			newTemp: templateOf("main1", "foo2", true, func(temp *v1.PodTemplateSpec) {
				temp.Spec.InitContainers[0].Image = "setup2"
			}),
			expectErr: true,
		},
		{
			name:        "non-image field of an init container is rejected",
			gateEnabled: true,
			oldTemp:     templateOf("main1", "foo1", true, nil),
			newTemp: templateOf("main1", "foo1", true, func(temp *v1.PodTemplateSpec) {
				temp.Spec.InitContainers[1].Env = []v1.EnvVar{{Name: "k", Value: "v"}}
			}),
			expectErr: true,
		},
		{
			name:        "adding an init container is rejected",
			gateEnabled: true,
			oldTemp:     templateOf("main1", "foo1", true, nil),
			newTemp: templateOf("main1", "foo1", true, func(temp *v1.PodTemplateSpec) {
				temp.Spec.InitContainers = append(temp.Spec.InitContainers,
					v1.Container{Name: "extra", Image: "extra1", RestartPolicy: restartAlways()})
			}),
			expectErr: true,
		},
		{
			name:        "no change is allowed",
			gateEnabled: true,
			oldTemp:     templateOf("main1", "foo1", true, nil),
			newTemp:     templateOf("main1", "foo1", true, nil),
			expectErr:   false,
		},
	}

	defer setInitContainerGate(t, false)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setInitContainerGate(t, tc.gateEnabled)

			oldJSON, _ := json.Marshal(tc.oldTemp.Spec)
			newJSON, _ := json.Marshal(tc.newTemp.Spec)
			patches, err := jsonpatch.CreatePatch(oldJSON, newJSON)
			if err != nil {
				t.Fatalf("failed to create patches: %v", err)
			}

			err = ValidateInPlaceOnlyTemplateSpecPatches(patches, tc.oldTemp, tc.newTemp)
			if tc.expectErr && err == nil {
				t.Fatalf("expected an error, got nil (patches: %v)", patches)
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("expected no error, got %v (patches: %v)", err, patches)
			}
		})
	}
}
