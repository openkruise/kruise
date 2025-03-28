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

package sidecarset

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/controller/history"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	testImageV1ImageID = "docker-pullable://test-image@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"
	testImageV2ImageID = "docker-pullable://test-image@sha256:f7988fb6c02e0ce69257d9bd9cf37ae20a60f1df7563c3a2a6abe24160306b8d"
)

func TestUpdateColdUpgradeSidecar(t *testing.T) {
	sidecarSetInput := sidecarSetDemo.DeepCopy()
	podInput := podDemo.DeepCopy()
	podInput.Spec.Containers[1].Env = []corev1.EnvVar{
		{
			Name:  "nginx-env",
			Value: "nginx-value",
		},
	}
	podInput.Spec.Containers[1].VolumeMounts = []corev1.VolumeMount{
		{
			MountPath: "/data/nginx",
		},
	}
	handlers := map[string]HandlePod{
		"pod test-pod-1 is upgrading": func(pods []*corev1.Pod) {
			cStatus := &pods[0].Status.ContainerStatuses[1]
			cStatus.Image = "test-image:v2"
			cStatus.ImageID = testImageV2ImageID
		},
		"pod test-pod-2 is upgrading": func(pods []*corev1.Pod) {
			cStatus := &pods[1].Status.ContainerStatuses[1]
			cStatus.Image = "test-image:v2"
			cStatus.ImageID = testImageV2ImageID
		},
	}
	testUpdateColdUpgradeSidecar(t, podInput, sidecarSetInput, handlers)
}

func testUpdateColdUpgradeSidecar(t *testing.T, podDemo *corev1.Pod, sidecarSetInput *appsv1alpha1.SidecarSet, handlers map[string]HandlePod) {
	podInput1 := podDemo.DeepCopy()
	podInput2 := podDemo.DeepCopy()
	podInput2.Name = "test-pod-2"
	cases := []struct {
		name          string
		getPods       func() []*corev1.Pod
		getSidecarset func() *appsv1alpha1.SidecarSet
		// pod.name -> infos []string{Image, Env, volumeMounts}
		expectedInfo map[*corev1.Pod][]string
		// MatchedPods, UpdatedPods, ReadyPods, AvailablePods, UnavailablePods
		expectedStatus []int32
	}{
		{
			name: "sidecarset update pod test-pod-1",
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					podInput1.DeepCopy(), podInput2.DeepCopy(),
				}
				return pods
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[*corev1.Pod][]string{
				podInput1: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
				podInput2: {"test-image:v1"},
			},
			expectedStatus: []int32{2, 0, 2, 0},
		},
		{
			name: "pod test-pod-1 is upgrading",
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					podInput1.DeepCopy(), podInput2.DeepCopy(),
				}
				return pods
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[*corev1.Pod][]string{
				podInput1: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
				podInput2: {"test-image:v1"},
			},
			expectedStatus: []int32{2, 1, 1, 0},
		},
		{
			name: "pod test-pod-1 upgrade complete, and start update pod test-pod-2",
			getPods: func() []*corev1.Pod {
				pod1 := podInput1.DeepCopy()
				pods := []*corev1.Pod{
					pod1, podInput2.DeepCopy(),
				}
				return pods
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[*corev1.Pod][]string{
				podInput1: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
				podInput2: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
			},
			expectedStatus: []int32{2, 1, 2, 1},
		},
		{
			name: "pod test-pod-2 is upgrading",
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					podInput1.DeepCopy(), podInput2.DeepCopy(),
				}
				return pods
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[*corev1.Pod][]string{
				podInput1: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
				podInput2: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
			},
			expectedStatus: []int32{2, 2, 1, 1},
		},
		{
			name: "pod test-pod-2 upgrade complete",
			getPods: func() []*corev1.Pod {
				pod2 := podInput2.DeepCopy()
				pods := []*corev1.Pod{
					podInput1.DeepCopy(), pod2,
				}
				return pods
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[*corev1.Pod][]string{
				podInput1: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
				podInput2: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
			},
			expectedStatus: []int32{2, 2, 2, 2},
		},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			pods := cs.getPods()
			sidecarset := cs.getSidecarset()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(sidecarset, pods[0], pods[1]).
				WithStatusSubresource(&appsv1alpha1.SidecarSet{}).Build()
			processor := NewSidecarSetProcessor(fakeClient, record.NewFakeRecorder(10))
			_, err := processor.UpdateSidecarSet(sidecarset)
			if err != nil {
				t.Errorf("processor update sidecarset failed: %s", err.Error())
			}

			for pod, infos := range cs.expectedInfo {
				podOutput, err := getLatestPod(fakeClient, pod)
				if err != nil {
					t.Errorf("get latest pod(%s) failed: %s", pod.Name, err.Error())
				}
				sidecarContainer := &podOutput.Spec.Containers[1]
				if infos[0] != sidecarContainer.Image {
					t.Fatalf("expect pod(%s) container(%s) image(%s), but get image(%s)", pod.Name, sidecarContainer.Name, infos[0], sidecarContainer.Image)
				}
				if len(infos) >= 2 && util.GetContainerEnvVar(sidecarContainer, infos[1]) == nil {
					t.Fatalf("expect pod(%s) container(%s) env(%s), but get nil", pod.Name, sidecarContainer.Name, infos[1])
				}
				if len(infos) >= 3 && util.GetContainerVolumeMount(sidecarContainer, infos[2]) == nil {
					t.Fatalf("expect pod(%s) container(%s) volumeMounts(%s), but get nil", pod.Name, sidecarContainer.Name, infos[2])
				}
				if len(infos) >= 4 && podOutput.Annotations[sidecarcontrol.SidecarSetListAnnotation] != infos[3] {
					t.Fatalf("expect pod(%s) annotations[%s]=%s, but get %s", pod.Name, sidecarcontrol.SidecarSetListAnnotation, infos[3], podOutput.Annotations[sidecarcontrol.SidecarSetListAnnotation])
				}
				if pod.Name == "test-pod-1" {
					podInput1 = podOutput
				} else {
					podInput2 = podOutput
				}
			}

			sidecarsetOutput, err := getLatestSidecarSet(fakeClient, sidecarset)
			if err != nil {
				t.Errorf("get latest sidecarset(%s) failed: %s", sidecarset.Name, err.Error())
			}
			sidecarSetInput = sidecarsetOutput
			for k, v := range cs.expectedStatus {
				var actualValue int32
				switch k {
				case 0:
					actualValue = sidecarsetOutput.Status.MatchedPods
				case 1:
					actualValue = sidecarsetOutput.Status.UpdatedPods
				case 2:
					actualValue = sidecarsetOutput.Status.ReadyPods
				case 3:
					actualValue = sidecarsetOutput.Status.UpdatedReadyPods
				default:
					//
				}

				if v != actualValue {
					t.Fatalf("except sidecarset status(%d:%d), but get value(%d)", k, v, actualValue)
				}
			}
			//handle potInput
			if handle, ok := handlers[cs.name]; ok {
				handle([]*corev1.Pod{podInput1, podInput2})
			}
		})
	}
}

func TestScopeNamespacePods(t *testing.T) {
	sidecarSet := sidecarSetDemo.DeepCopy()
	sidecarSet.Spec.Namespace = "test-ns"
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidecarSet).Build()
	for i := 0; i < 100; i++ {
		pod := podDemo.DeepCopy()
		pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
		if i >= 50 {
			pod.Namespace = "test-ns"
		}
		fakeClient.Create(context.TODO(), pod)
	}
	processor := NewSidecarSetProcessor(fakeClient, record.NewFakeRecorder(10))
	pods, err := processor.getMatchingPods(sidecarSet)
	if err != nil {
		t.Fatalf("getMatchingPods failed: %s", err.Error())
		return
	}

	if len(pods) != 50 {
		t.Fatalf("except matching pods count(%d), but get count(%d)", 50, len(pods))
	}
}

func TestCanUpgradePods(t *testing.T) {
	sidecarSet := factorySidecarSet()
	sidecarSet.Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] = "without-bbb"
	sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
		Type:   intstr.String,
		StrVal: "50%",
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidecarSet).
		WithStatusSubresource(&appsv1alpha1.SidecarSet{}).Build()
	pods := factoryPodsCommon(100, 0, sidecarSet)
	for i := range pods {
		pods[i].Annotations[sidecarcontrol.SidecarSetListAnnotation] = `test-sidecarset`
		if i < 50 {
			pods[i].Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] = `{"test-sidecarset":{"hash":"without-aaa"}}`
		} else {
			pods[i].Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] = `{"test-sidecarset":{"hash":"without-bbb"}}`
		}
		fakeClient.Create(context.TODO(), pods[i])
	}

	processor := NewSidecarSetProcessor(fakeClient, record.NewFakeRecorder(10))
	_, err := processor.UpdateSidecarSet(sidecarSet)
	if err != nil {
		t.Errorf("processor update sidecarset failed: %s", err.Error())
	}

	for i := range pods {
		pod := pods[i]
		podOutput, err := getLatestPod(fakeClient, pod)
		if err != nil {
			t.Errorf("get latest pod(%s) failed: %s", pod.Name, err.Error())
		}
		if i < 50 {
			if podOutput.Spec.Containers[1].Image != "test-image:v1" {
				t.Fatalf("except pod(%d) image(test-image:v1), but get image(%s)", i, podOutput.Spec.Containers[1].Image)
			}
		} else {
			if podOutput.Spec.Containers[1].Image != "test-image:v2" {
				t.Fatalf("except pod(%d) image(test-image:v2), but get image(%s)", i, podOutput.Spec.Containers[1].Image)
			}
		}
	}
}

func TestGetActiveRevisions(t *testing.T) {
	sidecarSet := factorySidecarSet()
	sidecarSet.SetUID("1223344")
	kubeSysNs := &corev1.Namespace{}
	//Note that webhookutil.GetNamespace() return "" here
	kubeSysNs.SetName(webhookutil.GetNamespace())
	kubeSysNs.SetNamespace(webhookutil.GetNamespace())
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidecarSet, kubeSysNs).Build()
	processor := NewSidecarSetProcessor(fakeClient, record.NewFakeRecorder(10))

	// case 1
	latestRevision, _, err := processor.registerLatestRevision(sidecarSet, nil)
	if err != nil || latestRevision == nil || latestRevision.Revision != int64(1) {
		t.Fatalf("in case of create: get active revision failed when the latest revision = 1, err: %v, actual latestRevision: %v",
			err, latestRevision.Revision)
	}

	// case 2
	newSidecar := sidecarSet.DeepCopy()
	newSidecar.Spec.InitContainers = []appsv1alpha1.SidecarContainer{
		{Container: corev1.Container{Name: "emptyInitC"}},
	}
	newSidecar.Spec.Volumes = []corev1.Volume{
		{Name: "emptyVolume"},
	}
	newSidecar.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
		{Name: "emptySecret"},
	}
	for i := 0; i < 5; i++ {
		latestRevision, _, err = processor.registerLatestRevision(newSidecar, nil)
		if err != nil || latestRevision == nil || latestRevision.Revision != int64(2) {
			t.Fatalf("in case of update: get active revision failed when the latest revision = 2, err: %v, actual latestRevision: %v",
				err, latestRevision.Revision)
		}
		revision := make(map[string]interface{})
		if err := json.Unmarshal(latestRevision.Data.Raw, &revision); err != nil {
			t.Fatalf("failed to decode revision, err: %v", err)
		}
		spec := revision["spec"].(map[string]interface{})
		_, ok1 := spec["volumes"]
		_, ok2 := spec["containers"]
		_, ok3 := spec["initContainers"]
		_, ok4 := spec["imagePullSecrets"]
		if !(ok1 && ok2 && ok3 && ok4) {
			t.Fatalf("failed to store revision, err: %v", err)
		}
	}

	// case 3
	for i := 0; i < 5; i++ {
		latestRevision, _, err = processor.registerLatestRevision(sidecarSet, nil)
		if err != nil || latestRevision == nil || latestRevision.Revision != int64(3) {
			t.Fatalf("in case of rollback: get active revision failed when the latest revision = 3, err: %v, actual latestRevision: %v",
				err, latestRevision.Revision)
		}
	}

	// case 4
	for i := 0; i < 100; i++ {
		sidecarSet.Spec.Containers[0].Image = fmt.Sprintf("%d", i)
		if _, _, err = processor.registerLatestRevision(sidecarSet, nil); err != nil {
			t.Fatalf("unexpected error, err: %v", err)
		}
	}
	revisionList := &apps.ControllerRevisionList{}
	processor.Client.List(context.TODO(), revisionList)
	if len(revisionList.Items) != int(*sidecarSet.Spec.RevisionHistoryLimit) {
		t.Fatalf("in case of maxStoredRevisions: get wrong number of revisions, expected %d, actual %d", *sidecarSet.Spec.RevisionHistoryLimit, len(revisionList.Items))
	}
}

func TestReplaceRevision(t *testing.T) {
	const TotalRevisions int = 10
	var revisions, pick []*apps.ControllerRevision
	// init revision slice
	for i := 1; i <= TotalRevisions; i++ {
		rv := &apps.ControllerRevision{}
		rv.Revision = int64(i)
		pick = append(pick, rv)
	}

	// check whether the list is ordered
	check := func(list []*apps.ControllerRevision) bool {
		if len(list) != TotalRevisions {
			return false
		}
		for i := 1; i < TotalRevisions; i++ {
			if list[i].Revision < list[i-1].Revision {
				return false
			}
		}
		return true
	}

	// reset revisions
	reset := func() {
		revisions = make([]*apps.ControllerRevision, 0)
		revisions = append(revisions, pick...)
	}

	newOne := &apps.ControllerRevision{}
	newOne.Revision = int64(TotalRevisions + 1)
	for i := 0; i < TotalRevisions; i++ {
		reset()
		replaceRevision(revisions, pick[i], newOne)
		if !check(revisions) {
			t.Fatalf("replaceRevision failed when replacing the %d-th item of %d", i, TotalRevisions)
		}
	}
}

func TestTruncateHistory(t *testing.T) {
	sidecarSet := factorySidecarSet()
	sidecarSet.SetName("sidecar")
	sidecarSet.SetUID("1223344")
	if sidecarSet.Spec.Selector.MatchLabels == nil {
		sidecarSet.Spec.Selector.MatchLabels = make(map[string]string)
	}
	kubeSysNs := &corev1.Namespace{}
	kubeSysNs.SetName(webhookutil.GetNamespace()) //Note that util.GetKruiseManagerNamespace() return "" here
	kubeSysNs.SetNamespace(webhookutil.GetNamespace())
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidecarSet, kubeSysNs).Build()
	processor := NewSidecarSetProcessor(fakeClient, record.NewFakeRecorder(10))

	getName := func(i int) string {
		return "sidecar-" + strconv.Itoa(i)
	}
	reset := func(usedCount int) ([]*corev1.Pod, []*apps.ControllerRevision) {
		pods := factoryPodsCommon(100, 0, sidecarSet)
		for i := range pods {
			sidecarSetHash := make(map[string]sidecarcontrol.SidecarSetUpgradeSpec)
			sidecarSetHash[sidecarSet.Name] = sidecarcontrol.SidecarSetUpgradeSpec{
				SidecarSetControllerRevision: getName(i%usedCount + 1),
			}
			by, _ := json.Marshal(&sidecarSetHash)
			pods[i].Annotations[sidecarcontrol.SidecarSetHashAnnotation] = string(by)
		}
		revisions := make([]*apps.ControllerRevision, 0)
		for i := 1; i <= 15; i++ {
			rv := &apps.ControllerRevision{}
			rv.SetName(getName(i))
			rv.SetNamespace(webhookutil.GetNamespace())
			rv.Revision = int64(i)
			revisions = append(revisions, rv)
		}
		return pods, revisions
	}

	stderr := func(num int) string {
		return fmt.Sprintf("failed to limit the number of stored revisions, limited: 10, actual: %d, name: sidecar", num)
	}

	// check successful cases
	for i := 1; i <= 9; i++ {
		pods, revisions := reset(i)
		err := processor.truncateHistory(revisions, sidecarSet, pods)
		if err != nil {
			t.Fatalf("expected revision len: %d, err: %v", 10, err)
		}
	}

	// check failed cases
	failedCases := []int{10, 11, 14, 15, 20}
	expectedResults := []int{11, 12, 15, 15, 15}
	for i := range failedCases {
		pods, revisions := reset(failedCases[i])
		err := processor.truncateHistory(revisions, sidecarSet, pods)
		if err == nil || err.Error() != stderr(expectedResults[i]) {
			t.Fatalf("expected revision len: %d, err: %v", expectedResults[i], err)
		}
	}

	// check revisions exactly
	pods, revisions := reset(8)
	for _, rv := range revisions {
		if err := processor.Client.Create(context.TODO(), rv); err != nil {
			t.Fatalf("failed to create revisions")
		}
	}
	if err := processor.truncateHistory(revisions, sidecarSet, pods); err != nil {
		t.Fatalf("failed to truncate revisions, err %v", err)
	}
	list := &apps.ControllerRevisionList{}
	if err := processor.Client.List(context.TODO(), list); err != nil {
		t.Fatalf("failed to list revisions, err %v", err)
	}
	if len(list.Items) != 10 {
		t.Fatalf("expected revision len: %d, actual: %d", 10, len(list.Items))
	}
	// sort history by revision field
	rvs := make([]*apps.ControllerRevision, 0)
	for i := range list.Items {
		rvs = append(rvs, &list.Items[i])
	}
	history.SortControllerRevisions(rvs)

	expected := make(map[int]int)
	for i := 0; i < 8; i++ {
		if rvs[i].Name != getName(i+1) {
			t.Fatalf("expected name %s, actual : %s", getName(expected[i]), rvs[i].Name)
		}
	}
	if rvs[8].Name != getName(14) {
		t.Fatalf("expected name %s, actual : %s", getName(14), rvs[8].Name)
	}
	if rvs[9].Name != getName(15) {
		t.Fatalf("expected name %s, actual : %s", getName(15), rvs[9].Name)
	}
}
