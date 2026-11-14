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
	"k8s.io/apimachinery/pkg/api/resource"
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

func setRestartableInitResourceGates(t *testing.T, initGate, verticalGate bool) {
	t.Helper()
	setInitContainerGate(t, initGate)
	if err := utilfeature.DefaultMutableFeatureGate.Set(
		fmt.Sprintf("%s=%v", features.InPlaceWorkloadVerticalScaling, verticalGate)); err != nil {
		t.Fatalf("failed to set vertical scaling feature-gate: %v", err)
	}
}

func initContainerTestResources(cpu, memory string) v1.ResourceList {
	return v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse(cpu),
		v1.ResourceMemory: resource.MustParse(memory),
	}
}

func guaranteedInitTestContainer(name string, restartable bool, cpu, memory string) v1.Container {
	c := v1.Container{
		Name:  name,
		Image: name + ":latest",
		Resources: v1.ResourceRequirements{
			Limits:   initContainerTestResources(cpu, memory),
			Requests: initContainerTestResources(cpu, memory),
		},
	}
	if restartable {
		c.RestartPolicy = restartAlways()
	}
	return c
}

func resourceRevision(t *testing.T, name string, main, init v1.Container) *apps.ControllerRevision {
	t.Helper()
	raw, err := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"template": v1.PodTemplateSpec{Spec: v1.PodSpec{
				Containers:     []v1.Container{main},
				InitContainers: []v1.Container{init},
			}},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal revision: %v", err)
	}
	return revisionOf(name, string(raw))
}

func TestCalculateSpecForRestartableInitContainerResources(t *testing.T) {
	main := guaranteedInitTestContainer("main", false, "1", "1Gi")
	oldSidecar := guaranteedInitTestContainer("sidecar", true, "100m", "128Mi")
	newSidecar := *oldSidecar.DeepCopy()
	newSidecar.Resources = v1.ResourceRequirements{
		Limits:   initContainerTestResources("200m", "256Mi"),
		Requests: initContainerTestResources("200m", "256Mi"),
	}
	oldRegular := *oldSidecar.DeepCopy()
	oldRegular.RestartPolicy = nil
	newRegular := *newSidecar.DeepCopy()
	newRegular.RestartPolicy = nil
	qosChangedSidecar := *newSidecar.DeepCopy()
	qosChangedSidecar.Resources.Requests = initContainerTestResources("100m", "128Mi")

	cases := []struct {
		name               string
		initGate           bool
		verticalGate       bool
		unsupportedCluster bool
		oldInit            v1.Container
		newInit            v1.Container
		expectInPlace      bool
	}{
		{
			name:          "both gates enabled",
			initGate:      true,
			verticalGate:  true,
			oldInit:       oldSidecar,
			newInit:       newSidecar,
			expectInPlace: true,
		},
		{
			name:         "restartable init gate disabled",
			initGate:     false,
			verticalGate: true,
			oldInit:      oldSidecar,
			newInit:      newSidecar,
		},
		{
			name:         "vertical scaling gate disabled",
			initGate:     true,
			verticalGate: false,
			oldInit:      oldSidecar,
			newInit:      newSidecar,
		},
		{
			name:               "unsupported Kubernetes version falls back to recreate",
			initGate:           true,
			verticalGate:       true,
			unsupportedCluster: true,
			oldInit:            oldSidecar,
			newInit:            newSidecar,
		},
		{
			name:         "regular init container falls back to recreate",
			initGate:     true,
			verticalGate: true,
			oldInit:      oldRegular,
			newInit:      newRegular,
		},
		{
			name:         "QoS change falls back to recreate",
			initGate:     true,
			verticalGate: true,
			oldInit:      oldSidecar,
			newInit:      qosChangedSidecar,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRestartableInitResourceGates(t, tc.initGate, tc.verticalGate)
			defer setRestartableInitResourceGates(t, false, false)
			originalCapabilityCheck := shouldUpdateSidecarResourceByResize
			shouldUpdateSidecarResourceByResize = func() bool { return !tc.unsupportedCluster }
			defer func() { shouldUpdateSidecarResourceByResize = originalCapabilityCheck }()
			got := defaultCalculateInPlaceUpdateSpec(
				resourceRevision(t, "old", main, tc.oldInit),
				resourceRevision(t, "new", main, tc.newInit), nil)
			if !tc.expectInPlace {
				if got != nil {
					t.Fatalf("expected nil spec (recreate), got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected a non-nil in-place update spec")
			}
			resources := got.ContainerResources["sidecar"]
			if !resources.Limits.Cpu().Equal(resource.MustParse("200m")) ||
				!resources.Requests.Memory().Equal(resource.MustParse("256Mi")) {
				t.Fatalf("unexpected sidecar resources: %+v", resources)
			}
			if !got.VerticalUpdateOnly() {
				t.Fatalf("expected a resource-only update, got %+v", got)
			}
		})
	}
}

func TestPatchRestartableInitContainerResourcesToPod(t *testing.T) {
	setRestartableInitResourceGates(t, true, true)
	defer setRestartableInitResourceGates(t, false, false)

	main := guaranteedInitTestContainer("main", false, "1", "1Gi")
	sidecar := guaranteedInitTestContainer("sidecar", true, "100m", "128Mi")
	setup := guaranteedInitTestContainer("setup", false, "50m", "64Mi")
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Annotations: map[string]string{}},
		Spec: v1.PodSpec{
			Containers:     []v1.Container{main},
			InitContainers: []v1.Container{sidecar, setup},
		},
	}
	newResources := v1.ResourceRequirements{
		Limits:   initContainerTestResources("200m", "256Mi"),
		Requests: initContainerTestResources("200m", "256Mi"),
	}
	spec := &UpdateSpec{ContainerResources: map[string]v1.ResourceRequirements{
		"sidecar": newResources,
		"setup":   newResources,
	}}
	state := &appspub.InPlaceUpdateState{}

	got, _, err := defaultPatchUpdateSpecToPod(pod, spec, state)
	if err != nil {
		t.Fatalf("failed to patch Pod: %v", err)
	}
	if !got.Spec.InitContainers[0].Resources.Limits.Cpu().Equal(resource.MustParse("200m")) ||
		!got.Spec.InitContainers[0].Resources.Requests.Memory().Equal(resource.MustParse("256Mi")) {
		t.Fatalf("sidecar resources were not updated: %+v", got.Spec.InitContainers[0].Resources)
	}
	if !got.Spec.InitContainers[1].Resources.Limits.Cpu().Equal(resource.MustParse("50m")) {
		t.Fatalf("regular init container must not be updated: %+v", got.Spec.InitContainers[1].Resources)
	}
	if len(state.NextContainerResources) != 0 {
		t.Fatalf("sidecar resources must be updated in the current batch: %v", state.NextContainerResources)
	}
	var persistedState appspub.InPlaceUpdateState
	if err := json.Unmarshal([]byte(got.Annotations[appspub.InPlaceUpdateStateKey]), &persistedState); err != nil {
		t.Fatalf("failed to decode in-place update state: %v", err)
	}
	if _, ok := persistedState.CurrentInitContainerResources["sidecar"]; !ok {
		t.Fatalf("sidecar resource target was not persisted: %+v", persistedState.CurrentInitContainerResources)
	}
	if _, ok := persistedState.CurrentInitContainerResources["setup"]; ok {
		t.Fatalf("regular init container must not be persisted as a resize target: %+v", persistedState.CurrentInitContainerResources)
	}
	if len(state.ContainerBatchesRecord) != 1 {
		t.Fatalf("expected one batch record, got %+v", state.ContainerBatchesRecord)
	}
	for _, name := range state.ContainerBatchesRecord[0].Containers {
		if name == "setup" {
			t.Fatalf("regular init container entered an in-place update batch: %+v", state.ContainerBatchesRecord[0])
		}
	}
}

func TestGenerateResourcePatchForRestartableInitContainer(t *testing.T) {
	main := guaranteedInitTestContainer("main", false, "1", "1Gi")
	sidecar := guaranteedInitTestContainer("sidecar", true, "100m", "128Mi")
	setup := guaranteedInitTestContainer("setup", false, "50m", "64Mi")
	pod := &v1.Pod{Spec: v1.PodSpec{
		Containers:     []v1.Container{main},
		InitContainers: []v1.Container{sidecar, setup},
	}}
	newResources := v1.ResourceRequirements{
		Limits:   initContainerTestResources("200m", "256Mi"),
		Requests: initContainerTestResources("200m", "256Mi"),
	}
	patchBytes := verticalUpdateImpl.GenerateResourcePatch(pod, map[string]*v1.ResourceRequirements{
		"main":    &newResources,
		"sidecar": &newResources,
		"setup":   &newResources,
	})
	var patch struct {
		Spec struct {
			Containers     []v1.Container `json:"containers"`
			InitContainers []v1.Container `json:"initContainers"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(patchBytes, &patch); err != nil {
		t.Fatalf("failed to unmarshal resize patch %s: %v", patchBytes, err)
	}
	if len(patch.Spec.Containers) != 1 || patch.Spec.Containers[0].Name != "main" {
		t.Fatalf("unexpected regular container patch: %+v", patch.Spec.Containers)
	}
	if len(patch.Spec.InitContainers) != 1 || patch.Spec.InitContainers[0].Name != "sidecar" {
		t.Fatalf("unexpected init container patch: %+v", patch.Spec.InitContainers)
	}
	if !patch.Spec.InitContainers[0].Resources.Limits.Cpu().Equal(resource.MustParse("200m")) {
		t.Fatalf("unexpected sidecar patch resources: %+v", patch.Spec.InitContainers[0].Resources)
	}
}

func TestRestartableInitContainerResourceCompletion(t *testing.T) {
	main := guaranteedInitTestContainer("main", false, "1", "1Gi")
	sidecar := guaranteedInitTestContainer("sidecar", true, "200m", "256Mi")
	setup := guaranteedInitTestContainer("setup", false, "50m", "64Mi")
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers:     []v1.Container{main},
			InitContainers: []v1.Container{sidecar, setup},
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{{
				Name: "main", Resources: main.Resources.DeepCopy(),
			}},
			InitContainerStatuses: []v1.ContainerStatus{{
				Name: "sidecar", Resources: sidecar.Resources.DeepCopy(),
			}},
		},
	}
	completed, err := verticalUpdateImpl.IsUpdateCompleted(pod, map[string]v1.ResourceRequirements{"sidecar": sidecar.Resources})
	if err != nil || !completed {
		t.Fatalf("expected sidecar resize to be completed, completed=%v, err=%v", completed, err)
	}

	oldResources := guaranteedInitTestContainer("sidecar", true, "100m", "128Mi").Resources
	pod.Status.InitContainerStatuses[0].Resources = &oldResources
	completed, err = verticalUpdateImpl.IsUpdateCompleted(pod, map[string]v1.ResourceRequirements{"sidecar": sidecar.Resources})
	if err == nil || completed {
		t.Fatalf("expected incomplete sidecar resize, completed=%v, err=%v", completed, err)
	}

	pod.Status.InitContainerStatuses[0].Resources = nil
	completed, err = verticalUpdateImpl.IsUpdateCompleted(pod, map[string]v1.ResourceRequirements{"sidecar": sidecar.Resources})
	if err == nil || completed {
		t.Fatalf("targeted sidecar without resource status must stay incomplete, completed=%v, err=%v", completed, err)
	}
	completed, err = verticalUpdateImpl.IsUpdateCompleted(pod, nil)
	if err != nil || !completed {
		t.Fatalf("regular-only resize must ignore unrelated sidecar status, completed=%v, err=%v", completed, err)
	}
}

func TestRestartableInitContainerResourceResizePolicy(t *testing.T) {
	setRestartableInitResourceGates(t, true, true)
	defer setRestartableInitResourceGates(t, false, false)

	sidecar := guaranteedInitTestContainer("sidecar", true, "100m", "128Mi")
	sidecar.ResizePolicy = []v1.ContainerResizePolicy{{
		ResourceName: v1.ResourceCPU, RestartPolicy: v1.RestartContainer,
	}}
	pod := &v1.Pod{Spec: v1.PodSpec{
		InitContainers: []v1.Container{sidecar},
		ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
	}}
	spec := &UpdateSpec{ContainerResources: map[string]v1.ResourceRequirements{
		"sidecar": {Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("200m")}},
	}}
	if !defaultCheckPodNeedsBeUnready(pod, spec) {
		t.Fatal("expected RestartContainer resize policy to mark the Pod unready")
	}

	pod.Spec.InitContainers[0].ResizePolicy[0].RestartPolicy = v1.NotRequired
	if defaultCheckPodNeedsBeUnready(pod, spec) {
		t.Fatal("NotRequired resize policy should keep the Pod ready")
	}

	pod.Spec.InitContainers[0].RestartPolicy = nil
	pod.Spec.InitContainers[0].ResizePolicy[0].RestartPolicy = v1.RestartContainer
	if defaultCheckPodNeedsBeUnready(pod, spec) {
		t.Fatal("regular init containers must not enter sidecar resource resize handling")
	}
}
