/*
Copyright 2022 The Kruise Authors.

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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/volumeclaimtemplate"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	testingclock "k8s.io/utils/clock/testing"
)

func TestDefaultPatchUpdateSpecToPod(t *testing.T) {
	now := time.Now()
	Clock = testingclock.NewFakeClock(now)
	givenPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{"label-k1": "foo", "label-k2": "foo"},
			Annotations: map[string]string{"annotation-k1": "foo"},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "c1",
					Image: "c1-img",
					Env: []v1.EnvVar{
						{Name: appspub.ContainerLaunchBarrierEnvName, ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: &v1.ConfigMapKeySelector{Key: "p_20"}}},
						{Name: "config", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['label-k1']"}}},
					},
				},
				{
					Name:  "c2",
					Image: "c2-img",
					Env: []v1.EnvVar{
						{Name: appspub.ContainerLaunchBarrierEnvName, ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: &v1.ConfigMapKeySelector{Key: "p_10"}}},
						{Name: "config", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['label-k2']"}}},
					},
				},
				{
					Name:  "c3",
					Image: "c3-img",
					Env: []v1.EnvVar{
						{Name: appspub.ContainerLaunchBarrierEnvName, ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: &v1.ConfigMapKeySelector{Key: "p_0"}}},
						{Name: "config", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['label-k2']"}}},
					},
				},
				{
					Name:  "c4",
					Image: "c4-img",
					Env: []v1.EnvVar{
						{Name: appspub.ContainerLaunchBarrierEnvName, ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: &v1.ConfigMapKeySelector{Key: "p_0"}}},
						{Name: "config", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.annotations['annotation-k1']"}}},
					},
				},
			},
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name:    "c1",
					ImageID: "containerd://c1-img",
				},
				{
					Name:    "c2",
					ImageID: "containerd://c2-img",
				},
				{
					Name:    "c3",
					ImageID: "containerd://c3-img",
				},
				{
					Name:    "c4",
					ImageID: "containerd://c4-img",
				},
			},
		},
	}

	cases := []struct {
		name          string
		spec          *UpdateSpec
		state         *appspub.InPlaceUpdateState
		expectedState *appspub.InPlaceUpdateState
		expectedPatch map[string]interface{}
	}{
		{
			name: "update a signal container image",
			spec: &UpdateSpec{
				ContainerImages: map[string]string{"c1": "c1-img-new"},
			},
			state: &appspub.InPlaceUpdateState{},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}},
			},
			expectedPatch: map[string]interface{}{
				//"metadata": map[string]interface{}{
				//	"annotations": map[string]interface{}{
				//		appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{
				//			LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}},
				//			ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}},
				//		}),
				//	},
				//},
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c1",
							"image": "c1-img-new",
						},
					},
				},
			},
		},
		{
			name: "update two container images without priority",
			spec: &UpdateSpec{
				MetaDataPatch:   []byte(`{"metadata":{"annotations":{"new-key": "bar"}}}`),
				ContainerImages: map[string]string{"c3": "c3-img-new", "c4": "c4-img-new"},
			},
			state: &appspub.InPlaceUpdateState{},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c3": {ImageID: "containerd://c3-img"}, "c4": {ImageID: "containerd://c4-img"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c3", "c4"}}},
			},
			expectedPatch: map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						"new-key": "bar",
					},
				},
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c3",
							"image": "c3-img-new",
						},
						{
							"name":  "c4",
							"image": "c4-img-new",
						},
					},
				},
			},
		},
		{
			name: "update two container image and env from metadata without priority",
			spec: &UpdateSpec{
				MetaDataPatch:        []byte(`{"metadata":{"annotations":{"new-key": "bar"}}}`),
				ContainerImages:      map[string]string{"c3": "c3-img-new"},
				ContainerRefMetadata: map[string]metav1.ObjectMeta{"c4": {Annotations: map[string]string{"annotation-k1": "bar"}}},
			},
			state: &appspub.InPlaceUpdateState{},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c3": {ImageID: "containerd://c3-img"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c3", "c4"}}},
			},
			expectedPatch: map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						"new-key":       "bar",
						"annotation-k1": "bar",
					},
				},
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c3",
							"image": "c3-img-new",
						},
					},
				},
			},
		},
		{
			name: "update two container images with different priorities, batch 1st",
			spec: &UpdateSpec{
				MetaDataPatch:   []byte(`{"metadata":{"annotations":{"new-key": "bar"}}}`),
				ContainerImages: map[string]string{"c1": "c1-img-new", "c2": "c2-img-new"},
			},
			state: &appspub.InPlaceUpdateState{},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}},
				NextContainerImages:    map[string]string{"c2": "c2-img-new"},
				PreCheckBeforeNext:     &appspub.InPlaceUpdatePreCheckBeforeNext{ContainersRequiredReady: []string{"c1"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}},
			},
			expectedPatch: map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						"new-key": "bar",
					},
				},
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c1",
							"image": "c1-img-new",
						},
					},
				},
			},
		},
		{
			name: "update two container images with different priorities, batch 2nd",
			spec: &UpdateSpec{
				ContainerImages: map[string]string{"c2": "c2-img-new"},
			},
			state: &appspub.InPlaceUpdateState{
				LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img-old"}},
				NextContainerImages:    map[string]string{"c2": "c2-img-new"},
				PreCheckBeforeNext:     &appspub.InPlaceUpdatePreCheckBeforeNext{ContainersRequiredReady: []string{"c1"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}},
			},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img-old"}, "c2": {ImageID: "containerd://c2-img"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}, {Timestamp: metav1.NewTime(now), Containers: []string{"c2"}}},
			},
			expectedPatch: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c2",
							"image": "c2-img-new",
						},
					},
				},
			},
		},
		{
			name: "update three container images with different priorities, batch 1st",
			spec: &UpdateSpec{
				ContainerImages: map[string]string{"c1": "c1-img-new", "c2": "c2-img-new", "c4": "c4-img-new"},
			},
			state: &appspub.InPlaceUpdateState{},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}},
				NextContainerImages:    map[string]string{"c2": "c2-img-new", "c4": "c4-img-new"},
				PreCheckBeforeNext:     &appspub.InPlaceUpdatePreCheckBeforeNext{ContainersRequiredReady: []string{"c1"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}},
			},
			expectedPatch: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c1",
							"image": "c1-img-new",
						},
					},
				},
			},
		},
		{
			name: "update three container images with different priorities, batch 2nd",
			spec: &UpdateSpec{
				ContainerImages: map[string]string{"c2": "c2-img-new", "c4": "c4-img-new"},
			},
			state: &appspub.InPlaceUpdateState{
				LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}},
				NextContainerImages:    map[string]string{"c2": "c2-img-new", "c4": "c4-img-new"},
				PreCheckBeforeNext:     &appspub.InPlaceUpdatePreCheckBeforeNext{ContainersRequiredReady: []string{"c1"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}},
			},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}, "c2": {ImageID: "containerd://c2-img"}},
				NextContainerImages:    map[string]string{"c4": "c4-img-new"},
				PreCheckBeforeNext:     &appspub.InPlaceUpdatePreCheckBeforeNext{ContainersRequiredReady: []string{"c2"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}, {Timestamp: metav1.NewTime(now), Containers: []string{"c2"}}},
			},
			expectedPatch: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c2",
							"image": "c2-img-new",
						},
					},
				},
			},
		},
		{
			name: "update three container images with different priorities, batch 3rd",
			spec: &UpdateSpec{
				ContainerImages: map[string]string{"c4": "c4-img-new"},
			},
			state: &appspub.InPlaceUpdateState{
				LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}, "c2": {ImageID: "containerd://c2-img"}},
				NextContainerImages:    map[string]string{"c4": "c4-img-new"},
				PreCheckBeforeNext:     &appspub.InPlaceUpdatePreCheckBeforeNext{ContainersRequiredReady: []string{"c2"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}, {Timestamp: metav1.NewTime(now), Containers: []string{"c2"}}},
			},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}, "c2": {ImageID: "containerd://c2-img"}, "c4": {ImageID: "containerd://c4-img"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{
					{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}},
					{Timestamp: metav1.NewTime(now), Containers: []string{"c2"}},
					{Timestamp: metav1.NewTime(now), Containers: []string{"c4"}},
				},
			},
			expectedPatch: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c4",
							"image": "c4-img-new",
						},
					},
				},
			},
		},
		{
			name: "update four container images and env from metadata with different priorities, batch 1st",
			spec: &UpdateSpec{
				ContainerImages: map[string]string{"c1": "c1-img-new", "c2": "c2-img-new", "c4": "c4-img-new"},
				ContainerRefMetadata: map[string]metav1.ObjectMeta{
					"c2": {Labels: map[string]string{"label-k2": "bar"}},
					"c3": {Labels: map[string]string{"label-k2": "bar"}},
				},
			},
			state: &appspub.InPlaceUpdateState{},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}},
				NextContainerImages:   map[string]string{"c2": "c2-img-new", "c4": "c4-img-new"},
				NextContainerRefMetadata: map[string]metav1.ObjectMeta{
					"c2": {Labels: map[string]string{"label-k2": "bar"}},
					"c3": {Labels: map[string]string{"label-k2": "bar"}},
				},
				PreCheckBeforeNext:     &appspub.InPlaceUpdatePreCheckBeforeNext{ContainersRequiredReady: []string{"c1"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}},
			},
			expectedPatch: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c1",
							"image": "c1-img-new",
						},
					},
				},
			},
		},
		{
			name: "update four container images and env from metadata with different priorities, batch 2nd",
			spec: &UpdateSpec{
				ContainerImages: map[string]string{"c2": "c2-img-new", "c4": "c4-img-new"},
				ContainerRefMetadata: map[string]metav1.ObjectMeta{
					"c2": {Labels: map[string]string{"label-k2": "bar"}},
					"c3": {Labels: map[string]string{"label-k2": "bar"}},
				},
			},
			state: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}},
				NextContainerImages:   map[string]string{"c2": "c2-img-new", "c4": "c4-img-new"},
				NextContainerRefMetadata: map[string]metav1.ObjectMeta{
					"c2": {Labels: map[string]string{"label-k2": "bar"}},
					"c3": {Labels: map[string]string{"label-k2": "bar"}},
				},
				PreCheckBeforeNext:     &appspub.InPlaceUpdatePreCheckBeforeNext{ContainersRequiredReady: []string{"c1"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}},
			},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}, "c2": {ImageID: "containerd://c2-img"}},
				NextContainerImages:   map[string]string{"c4": "c4-img-new"},
				PreCheckBeforeNext:    &appspub.InPlaceUpdatePreCheckBeforeNext{ContainersRequiredReady: []string{"c2", "c3"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{
					{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}},
					{Timestamp: metav1.NewTime(now), Containers: []string{"c2", "c3"}},
				},
			},
			expectedPatch: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"label-k2": "bar",
					},
				},
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c2",
							"image": "c2-img-new",
						},
					},
				},
			},
		},
		{
			name: "update four container images and env from metadata with different priorities, batch 3rd",
			spec: &UpdateSpec{
				ContainerImages: map[string]string{"c4": "c4-img-new"},
			},
			state: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}, "c2": {ImageID: "containerd://c2-img"}},
				NextContainerImages:   map[string]string{"c4": "c4-img-new"},
				PreCheckBeforeNext:    &appspub.InPlaceUpdatePreCheckBeforeNext{ContainersRequiredReady: []string{"c2", "c3"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{
					{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}},
					{Timestamp: metav1.NewTime(now), Containers: []string{"c2", "c3"}},
				},
			},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "containerd://c1-img"}, "c2": {ImageID: "containerd://c2-img"}, "c4": {ImageID: "containerd://c4-img"}},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{
					{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}},
					{Timestamp: metav1.NewTime(now), Containers: []string{"c2", "c3"}},
					{Timestamp: metav1.NewTime(now), Containers: []string{"c4"}},
				},
			},
			expectedPatch: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c4",
							"image": "c4-img-new",
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotPod, err := defaultPatchUpdateSpecToPod(givenPod.DeepCopy(), tc.spec, tc.state)
			if err != nil {
				t.Fatal(err)
			}

			if !apiequality.Semantic.DeepEqual(tc.state, tc.expectedState) {
				t.Fatalf("expected state \n%v\n but got \n%v", util.DumpJSON(tc.expectedState), util.DumpJSON(tc.state))
			}

			originPodJS, _ := json.Marshal(givenPod)
			patchJS, _ := json.Marshal(tc.expectedPatch)
			expectedPodJS, err := strategicpatch.StrategicMergePatch(originPodJS, patchJS, &v1.Pod{})
			if err != nil {
				t.Fatal(err)
			}
			expectedPod := &v1.Pod{}
			if err := json.Unmarshal(expectedPodJS, expectedPod); err != nil {
				t.Fatal(err)
			}
			expectedPod.Annotations[appspub.InPlaceUpdateStateKey] = util.DumpJSON(tc.state)
			if !apiequality.Semantic.DeepEqual(gotPod, expectedPod) {
				t.Fatalf("expected pod \n%v\n but got \n%v", util.DumpJSON(expectedPod), util.DumpJSON(gotPod))
			}
		})
	}
}

func Test_defaultCalculateInPlaceUpdateSpec_VCTHash(t *testing.T) {
	type args struct {
		oldRevision *apps.ControllerRevision
		newRevision *apps.ControllerRevision
		opts        *UpdateOptions
	}
	oldData := `{
  "spec": {
    "template": {
      "$patch": "replace",
      "metadata": {
        "creationTimestamp": null,
        "labels": {
          "app": "nginx"
        }
      },
      "spec": {
        "containers": [
          {
            "env": [
              {
                "name": "version",
                "value": "v1"
              }
            ],
            "image": "nginx:stable-alpine22",
            "imagePullPolicy": "Always",
            "name": "nginx",
            "resources": {},
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "volumeMounts": [
              {
                "mountPath": "/usr/share/nginx/html",
                "name": "www-data"
              }
            ]
          }
        ],
        "dnsPolicy": "ClusterFirst",
        "restartPolicy": "Always",
        "schedulerName": "default-scheduler",
        "securityContext": {},
        "terminationGracePeriodSeconds": 30
      }
    }
  }
}`
	newData := `{
  "spec": {
    "template": {
      "$patch": "replace",
      "metadata": {
        "creationTimestamp": null,
        "labels": {
          "app": "nginx"
        }
      },
      "spec": {
        "containers": [
          {
            "env": [
              {
                "name": "version",
                "value": "v1"
              }
            ],
            "image": "nginx:stable-alpine",
            "imagePullPolicy": "Always",
            "name": "nginx",
            "resources": {},
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "volumeMounts": [
              {
                "mountPath": "/usr/share/nginx/html",
                "name": "www-data"
              }
            ]
          }
        ],
        "dnsPolicy": "ClusterFirst",
        "restartPolicy": "Always",
        "schedulerName": "default-scheduler",
        "securityContext": {},
        "terminationGracePeriodSeconds": 30
      }
    }
  }
}`

	desiredWhenDisableFG := &UpdateSpec{
		Revision: "new-revision",
		ContainerImages: map[string]string{
			"nginx": "nginx:stable-alpine",
		},
	}
	ignoreVCTHashUpdateOpts := &UpdateOptions{IgnoreVolumeClaimTemplatesHashDiff: true}
	tests := []struct {
		name string
		args args
		want *UpdateSpec

		wantWhenDisable *UpdateSpec
	}{
		{
			name: "both revision annotation is nil=> ignore",
			args: args{
				oldRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "old-revision",
						Annotations: nil,
					},
					Data: runtime.RawExtension{
						Raw: []byte(oldData),
					},
				},
				newRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "new-revision",
						Annotations: map[string]string{},
					},
					Data: runtime.RawExtension{
						Raw: []byte(newData),
					},
				},
				opts: &UpdateOptions{},
			},
			want:            desiredWhenDisableFG,
			wantWhenDisable: desiredWhenDisableFG,
		},
		{
			name: "old revision annotation is nil=> ignore",
			args: args{
				oldRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "old-revision",
						Annotations: nil,
					},
					Data: runtime.RawExtension{
						Raw: []byte(oldData),
					},
				},
				newRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "new-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: "balala"},
					},
					Data: runtime.RawExtension{
						Raw: []byte(newData),
					},
				},
				opts: &UpdateOptions{},
			},
			want:            desiredWhenDisableFG,
			wantWhenDisable: desiredWhenDisableFG,
		},
		{
			name: "new revision annotation is nil => ignore",
			args: args{
				oldRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "old-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: "balala"},
					},
					Data: runtime.RawExtension{
						Raw: []byte(oldData),
					},
				},
				newRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "new-revision",
						Annotations: nil,
					},
					Data: runtime.RawExtension{
						Raw: []byte(newData),
					},
				},
				opts: &UpdateOptions{},
			},
			want:            desiredWhenDisableFG,
			wantWhenDisable: desiredWhenDisableFG,
		},
		{
			name: "revision annotation changes => recreate",
			args: args{
				oldRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "old-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: ""},
					},
					Data: runtime.RawExtension{
						Raw: []byte(oldData),
					},
				},
				newRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "new-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: "balala"},
					},
					Data: runtime.RawExtension{
						Raw: []byte(newData),
					},
				},
				opts: &UpdateOptions{},
			},
			want:            nil,
			wantWhenDisable: desiredWhenDisableFG,
		},
		{
			name: "the same revision annotation => in-place update",
			args: args{
				oldRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "old-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: "balala"},
					},
					Data: runtime.RawExtension{
						Raw: []byte(oldData),
					},
				},
				newRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "new-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: "balala"},
					},
					Data: runtime.RawExtension{
						Raw: []byte(newData),
					},
				},
				opts: &UpdateOptions{},
			},
			want:            desiredWhenDisableFG,
			wantWhenDisable: desiredWhenDisableFG,
		},
		{
			name: "both empty revision annotation => in-place update",
			args: args{
				oldRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "old-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: ""},
					},
					Data: runtime.RawExtension{
						Raw: []byte(oldData),
					},
				},
				newRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "new-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: ""},
					},
					Data: runtime.RawExtension{
						Raw: []byte(newData),
					},
				},
				opts: &UpdateOptions{},
			},
			want:            desiredWhenDisableFG,
			wantWhenDisable: desiredWhenDisableFG,
		},

		// IgnoreVolumeClaimTemplatesHashDiff is true
		{
			name: "IgnoreVolumeClaimTemplatesHashDiff&both revision annotation is nil=> ignore",
			args: args{
				oldRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "old-revision",
						Annotations: nil,
					},
					Data: runtime.RawExtension{
						Raw: []byte(oldData),
					},
				},
				newRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "new-revision",
						Annotations: map[string]string{},
					},
					Data: runtime.RawExtension{
						Raw: []byte(newData),
					},
				},
				opts: ignoreVCTHashUpdateOpts,
			},
			want:            desiredWhenDisableFG,
			wantWhenDisable: desiredWhenDisableFG,
		},
		{
			name: "IgnoreVolumeClaimTemplatesHashDiff&old revision annotation is nil=> ignore",
			args: args{
				oldRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "old-revision",
						Annotations: nil,
					},
					Data: runtime.RawExtension{
						Raw: []byte(oldData),
					},
				},
				newRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "new-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: "balala"},
					},
					Data: runtime.RawExtension{
						Raw: []byte(newData),
					},
				},
				opts: ignoreVCTHashUpdateOpts,
			},
			want:            desiredWhenDisableFG,
			wantWhenDisable: desiredWhenDisableFG,
		},
		{
			name: "IgnoreVolumeClaimTemplatesHashDiff&new revision annotation is nil => ignore",
			args: args{
				oldRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "old-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: "balala"},
					},
					Data: runtime.RawExtension{
						Raw: []byte(oldData),
					},
				},
				newRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "new-revision",
						Annotations: nil,
					},
					Data: runtime.RawExtension{
						Raw: []byte(newData),
					},
				},
				opts: ignoreVCTHashUpdateOpts,
			},
			want:            desiredWhenDisableFG,
			wantWhenDisable: desiredWhenDisableFG,
		},
		{
			name: "IgnoreVolumeClaimTemplatesHashDiff&revision annotation changes => in-place update",
			args: args{
				oldRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "old-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: ""},
					},
					Data: runtime.RawExtension{
						Raw: []byte(oldData),
					},
				},
				newRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "new-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: "balala"},
					},
					Data: runtime.RawExtension{
						Raw: []byte(newData),
					},
				},
				opts: ignoreVCTHashUpdateOpts,
			},
			want:            desiredWhenDisableFG,
			wantWhenDisable: desiredWhenDisableFG,
		},
		{
			name: "IgnoreVolumeClaimTemplatesHashDiff&the same revision annotation => in-place update",
			args: args{
				oldRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "old-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: "balala"},
					},
					Data: runtime.RawExtension{
						Raw: []byte(oldData),
					},
				},
				newRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "new-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: "balala"},
					},
					Data: runtime.RawExtension{
						Raw: []byte(newData),
					},
				},
				opts: ignoreVCTHashUpdateOpts,
			},
			want:            desiredWhenDisableFG,
			wantWhenDisable: desiredWhenDisableFG,
		},
		{
			name: "IgnoreVolumeClaimTemplatesHashDiff&both empty revision annotation => in-place update",
			args: args{
				oldRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "old-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: ""},
					},
					Data: runtime.RawExtension{
						Raw: []byte(oldData),
					},
				},
				newRevision: &apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "new-revision",
						Annotations: map[string]string{volumeclaimtemplate.HashAnnotation: ""},
					},
					Data: runtime.RawExtension{
						Raw: []byte(newData),
					},
				},
				opts: ignoreVCTHashUpdateOpts,
			},
			want:            desiredWhenDisableFG,
			wantWhenDisable: desiredWhenDisableFG,
		},
	}

	testWhenEnable := func(enable bool) {
		defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.RecreatePodWhenChangeVCTInCloneSetGate, enable)()
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%v-%v", tt.name, enable), func(t *testing.T) {
				got := defaultCalculateInPlaceUpdateSpec(tt.args.oldRevision, tt.args.newRevision, tt.args.opts)
				wanted := tt.wantWhenDisable
				if utilfeature.DefaultFeatureGate.Enabled(features.RecreatePodWhenChangeVCTInCloneSetGate) {
					wanted = tt.want
				}
				if got != nil && wanted != nil {
					if !reflect.DeepEqual(got.ContainerImages, wanted.ContainerImages) {
						t.Errorf("defaultCalculateInPlaceUpdateSpec() = %v, want %v", got, wanted)
					}
				} else if !(got == nil && wanted == nil) {
					t.Errorf("defaultCalculateInPlaceUpdateSpec() = %v, want %v", got, wanted)
				}
				// got == nil && tt.want == nil => pass
			})
		}
	}
	testWhenEnable(true)
	testWhenEnable(false)

}

func getFakeControllerRevisionData() string {
	oldData := `{
  "spec": {
    "template": {
      "$patch": "replace",
      "metadata": {
        "creationTimestamp": null,
        "labels": {
          "app": "nginx"
        }
      },
      "spec": {
        "containers": [
          {
            "env": [
              {
                "name": "version",
                "value": "v1"
              }
            ],
            "image": "nginx:stable-alpine22",
            "imagePullPolicy": "Always",
            "name": "nginx",
            "resources": {
				"limits": {
					"cpu": "2",
					"memory": "4Gi",
					"sigma/eni": "2"
				},
				"requests": {
					"cpu": "1",
					"memory": "2Gi",
					"sigma/eni": "2"
				}
			},
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "volumeMounts": [
              {
                "mountPath": "/usr/share/nginx/html",
                "name": "www-data"
              }
            ]
          },
          {
            "env": [
              {
                "name": "version",
                "value": "v1"
              }
            ],
            "image": "nginx:stable-alpine22",
            "imagePullPolicy": "Always",
            "name": "nginx2",
            "resources": {
				"limits": {
					"cpu": "2",
					"memory": "4Gi",
					"sigma/eni": "2"
				},
				"requests": {
					"cpu": "1",
					"memory": "2Gi",
					"sigma/eni": "2"
				}
			},
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "volumeMounts": [
              {
                "mountPath": "/usr/share/nginx/html",
                "name": "www-data"
              }
            ]
          }
        ],
        "dnsPolicy": "ClusterFirst",
        "restartPolicy": "Always",
        "schedulerName": "default-scheduler",
        "securityContext": {},
        "terminationGracePeriodSeconds": 30
      }
    }
  }
}`
	return oldData
}

func TestDefaultCalculateInPlaceUpdateSpec(t *testing.T) {
	baseRevision := &apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "old-revision",
			Annotations: map[string]string{},
		},
		Data: runtime.RawExtension{
			Raw: []byte(getFakeControllerRevisionData()),
		},
	}
	revisionGetter := func(imageChanged, resourceChanged, otherChanged bool, updateContainerNum int) *apps.ControllerRevision {
		base := getFakeControllerRevisionData()
		if imageChanged {
			base = strings.Replace(base, `"image": "nginx:stable-alpine22"`, `"image": "nginx:stable-alpine23"`, updateContainerNum)
		}
		if resourceChanged {
			base = strings.Replace(base, `"cpu": "1",`, `"cpu": "2",`, updateContainerNum)
		}
		if otherChanged {
			base = strings.Replace(base, `"imagePullPolicy": "Always",`, `"imagePullPolicy": "222",`, updateContainerNum)
		}
		return &apps.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "new-revision",
				Annotations: map[string]string{},
			},
			Data: runtime.RawExtension{
				Raw: []byte(base),
			},
		}
	}
	// Define your test cases
	tests := []struct {
		name           string
		oldRevision    *apps.ControllerRevision
		newRevision    *apps.ControllerRevision
		opts           *UpdateOptions
		expectedResult *UpdateSpec
		vpaEnabled     bool
	}{
		{
			vpaEnabled:  true,
			name:        "only change resource",
			oldRevision: baseRevision,
			newRevision: revisionGetter(false, true, false, 1),
			opts:        &UpdateOptions{},
			expectedResult: &UpdateSpec{
				Revision: "new-revision",
				ContainerResources: map[string]v1.ResourceRequirements{
					"nginx": {
						Requests: v1.ResourceList{
							"cpu": resource.MustParse("2"),
						},
					},
				},
				VerticalUpdateOnly: true,
			},
		},
		{
			vpaEnabled:  true,
			name:        "change image and resource",
			oldRevision: baseRevision,
			newRevision: revisionGetter(true, true, false, 1),
			opts:        &UpdateOptions{},
			expectedResult: &UpdateSpec{
				Revision: "new-revision",
				ContainerImages: map[string]string{
					"nginx": "nginx:stable-alpine23",
				},
				ContainerResources: map[string]v1.ResourceRequirements{
					"nginx": {
						Requests: v1.ResourceList{
							"cpu": resource.MustParse("2"),
						},
					},
				},
				VerticalUpdateOnly: false,
			},
		},
		{
			vpaEnabled:     true,
			name:           "change other and resource",
			oldRevision:    baseRevision,
			newRevision:    revisionGetter(false, true, true, 1),
			opts:           &UpdateOptions{},
			expectedResult: nil,
		},
		{
			vpaEnabled:     true,
			name:           "change all",
			oldRevision:    baseRevision,
			newRevision:    revisionGetter(true, true, true, 1),
			opts:           &UpdateOptions{},
			expectedResult: nil,
		},
		// Add more test cases as needed
		{
			vpaEnabled:  true,
			name:        "only change resource of two containers",
			oldRevision: baseRevision,
			newRevision: revisionGetter(false, true, false, 2),
			opts:        &UpdateOptions{},
			expectedResult: &UpdateSpec{
				Revision: "new-revision",
				ContainerResources: map[string]v1.ResourceRequirements{
					"nginx": {
						Requests: v1.ResourceList{
							"cpu": resource.MustParse("2"),
						},
					},
					"nginx2": {
						Requests: v1.ResourceList{
							"cpu": resource.MustParse("2"),
						},
					},
				},
				VerticalUpdateOnly: true,
			},
		},
		{
			vpaEnabled:  true,
			name:        "change image and resource of two containers",
			oldRevision: baseRevision,
			newRevision: revisionGetter(true, true, false, 2),
			opts:        &UpdateOptions{},
			expectedResult: &UpdateSpec{
				Revision: "new-revision",
				ContainerImages: map[string]string{
					"nginx":  "nginx:stable-alpine23",
					"nginx2": "nginx:stable-alpine23",
				},
				ContainerResources: map[string]v1.ResourceRequirements{
					"nginx": {
						Requests: v1.ResourceList{
							"cpu": resource.MustParse("2"),
						},
					},
					"nginx2": {
						Requests: v1.ResourceList{
							"cpu": resource.MustParse("2"),
						},
					},
				},
				VerticalUpdateOnly: false,
			},
		},
		{
			vpaEnabled:     true,
			name:           "change other and resource of two containers",
			oldRevision:    baseRevision,
			newRevision:    revisionGetter(false, true, true, 2),
			opts:           &UpdateOptions{},
			expectedResult: nil,
		},
		{
			vpaEnabled:     true,
			name:           "change all of two containers",
			oldRevision:    baseRevision,
			newRevision:    revisionGetter(true, true, true, 2),
			opts:           &UpdateOptions{},
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.InPlaceWorkloadVerticalScaling, tt.vpaEnabled)()
			result := defaultCalculateInPlaceUpdateSpec(tt.oldRevision, tt.newRevision, tt.opts)

			if !apiequality.Semantic.DeepEqual(tt.expectedResult, result) {
				t.Fatalf("expected updateSpec \n%v\n but got \n%v", util.DumpJSON(tt.expectedResult), util.DumpJSON(result))
			}
		})
	}
}

func getTestPodWithResource() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{"label-k1": "foo", "label-k2": "foo"},
			Annotations: map[string]string{"annotation-k1": "foo"},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "c1",
					Image: "c1-img",
					Env: []v1.EnvVar{
						{Name: appspub.ContainerLaunchBarrierEnvName, ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: &v1.ConfigMapKeySelector{Key: "p_20"}}},
						{Name: "config", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['label-k1']"}}},
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("1Gi"),
							v1.ResourceCPU:    resource.MustParse("1"),
						},
						Limits: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("2Gi"),
							v1.ResourceCPU:    resource.MustParse("2"),
						},
					},
					ResizePolicy: []v1.ContainerResizePolicy{
						{
							ResourceName:  v1.ResourceCPU,
							RestartPolicy: v1.NotRequired,
						},
					},
				},
				{
					Name:  "c2",
					Image: "c2-img",
					Env: []v1.EnvVar{
						{Name: appspub.ContainerLaunchBarrierEnvName, ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: &v1.ConfigMapKeySelector{Key: "p_10"}}},
						{Name: "config", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['label-k2']"}}},
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("1Gi"),
							v1.ResourceCPU:    resource.MustParse("1"),
						},
						Limits: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("2Gi"),
							v1.ResourceCPU:    resource.MustParse("2"),
						},
					},
					ResizePolicy: []v1.ContainerResizePolicy{
						{
							ResourceName:  v1.ResourceMemory,
							RestartPolicy: v1.RestartContainer,
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name:    "c1",
					ImageID: "containerd://c1-img",
					Resources: &v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("1Gi"),
							v1.ResourceCPU:    resource.MustParse("1"),
						},
						Limits: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("2Gi"),
							v1.ResourceCPU:    resource.MustParse("2"),
						},
					},
				},
				{
					Name:    "c2",
					ImageID: "containerd://c2-img",
					Resources: &v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("1Gi"),
							v1.ResourceCPU:    resource.MustParse("1"),
						},
						Limits: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("2Gi"),
							v1.ResourceCPU:    resource.MustParse("2"),
						},
					},
				},
			},
		},
	}
}

func TestDefaultPatchUpdateSpecToPod_Resource(t *testing.T) {
	// disableVPA cases already be tested in TestDefaultPatchUpdateSpecToPod
	now := time.Now()
	Clock = testingclock.NewFakeClock(now)
	pod := getTestPodWithResource()

	// Define the test cases
	tests := []struct {
		name          string
		spec          *UpdateSpec
		state         *appspub.InPlaceUpdateState
		expectedState *appspub.InPlaceUpdateState
		expectedPatch map[string]interface{}
		vpaEnabled    bool
	}{
		{
			name: "only change container 0 resource cpu",
			spec: &UpdateSpec{
				ContainerResources: map[string]v1.ResourceRequirements{
					"c1": {
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("2Gi"),
							v1.ResourceCPU:    resource.MustParse("2"),
						},
					},
				},
			},
			state: &appspub.InPlaceUpdateState{},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{
					"c1": {
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("1Gi"),
								v1.ResourceCPU:    resource.MustParse("1"),
							},
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2Gi"),
								v1.ResourceCPU:    resource.MustParse("2"),
							},
						},
					},
				},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}},
			},
			expectedPatch: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name": "c1",
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"memory": "2Gi",
									"cpu":    "2",
								},
							},
						},
					},
				},
			},
			vpaEnabled: true,
		},
		{
			name: "change container 0 resource cpu and image",
			spec: &UpdateSpec{
				ContainerImages: map[string]string{"c1": "c1-img-new"},
				ContainerResources: map[string]v1.ResourceRequirements{
					"c1": {
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("2Gi"),
							v1.ResourceCPU:    resource.MustParse("2"),
						},
					},
				},
			},
			state: &appspub.InPlaceUpdateState{},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{
					"c1": {
						ImageID: "containerd://c1-img",
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("1Gi"),
								v1.ResourceCPU:    resource.MustParse("1"),
							},
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2Gi"),
								v1.ResourceCPU:    resource.MustParse("2"),
							},
						},
					},
				},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}},
			},
			expectedPatch: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c1",
							"image": "c1-img-new",
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"memory": "2Gi",
									"cpu":    "2",
								},
							},
						},
					},
				},
			},
			vpaEnabled: true,
		},
		{
			name: "change two containers resource cpu and image step1",
			spec: &UpdateSpec{
				ContainerImages: map[string]string{"c1": "c1-img-new", "c2": "c1-img-new"},
				ContainerResources: map[string]v1.ResourceRequirements{
					"c1": {
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("2Gi"),
							v1.ResourceCPU:    resource.MustParse("2"),
						},
					},
					"c2": {
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("2Gi"),
							v1.ResourceCPU:    resource.MustParse("2"),
						},
					},
				},
			},
			state: &appspub.InPlaceUpdateState{},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{
					"c1": {
						ImageID: "containerd://c1-img",
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("1Gi"),
								v1.ResourceCPU:    resource.MustParse("1"),
							},
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2Gi"),
								v1.ResourceCPU:    resource.MustParse("2"),
							},
						},
					},
				},
				NextContainerResources: map[string]v1.ResourceRequirements{
					"c2": {
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("2Gi"),
							v1.ResourceCPU:    resource.MustParse("2"),
						},
					},
				},
				PreCheckBeforeNext: &appspub.InPlaceUpdatePreCheckBeforeNext{
					ContainersRequiredReady: []string{"c1"},
				},
				NextContainerImages:    map[string]string{"c2": "c1-img-new"},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}},
			},
			expectedPatch: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c1",
							"image": "c1-img-new",
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"memory": "2Gi",
									"cpu":    "2",
								},
							},
						},
					},
				},
			},
			vpaEnabled: true,
		},
		{
			name: "change two containers resource cpu and image step2",
			spec: &UpdateSpec{
				ContainerImages: map[string]string{"c2": "c1-img-new"},
				ContainerResources: map[string]v1.ResourceRequirements{
					"c2": {
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("2Gi"),
							v1.ResourceCPU:    resource.MustParse("2"),
						},
					},
				},
			},
			state: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{
					"c1": {
						ImageID: "containerd://c2-img",
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("1Gi"),
								v1.ResourceCPU:    resource.MustParse("1"),
							},
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2Gi"),
								v1.ResourceCPU:    resource.MustParse("2"),
							},
						},
					},
				},
				NextContainerResources: map[string]v1.ResourceRequirements{
					"c2": {
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("2Gi"),
							v1.ResourceCPU:    resource.MustParse("2"),
						},
					},
				},
				PreCheckBeforeNext: &appspub.InPlaceUpdatePreCheckBeforeNext{
					ContainersRequiredReady: []string{"c1"},
				},
				NextContainerImages:    map[string]string{"c2": "c1-img-new"},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}},
			},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{
					"c1": {
						ImageID: "containerd://c2-img",
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("1Gi"),
								v1.ResourceCPU:    resource.MustParse("1"),
							},
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2Gi"),
								v1.ResourceCPU:    resource.MustParse("2"),
							},
						},
					},
					"c2": {
						ImageID: "containerd://c2-img",
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("1Gi"),
								v1.ResourceCPU:    resource.MustParse("1"),
							},
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2Gi"),
								v1.ResourceCPU:    resource.MustParse("2"),
							},
						},
					},
				},
				ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: metav1.NewTime(now), Containers: []string{"c1"}}, {Timestamp: metav1.NewTime(now), Containers: []string{"c2"}}},
			},
			expectedPatch: map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "c2",
							"image": "c1-img-new",
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"memory": "2Gi",
									"cpu":    "2",
								},
							},
						},
					},
				},
			},
			vpaEnabled: true,
		},
	}

	// Initialize the vertical update operator
	verticalUpdateOperator = &VerticalUpdate{}

	// Run the test cases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.InPlaceWorkloadVerticalScaling, tc.vpaEnabled)()
			gotPod, err := defaultPatchUpdateSpecToPod(pod.DeepCopy(), tc.spec, tc.state)
			if err != nil {
				t.Fatal(err)
			}

			if !apiequality.Semantic.DeepEqual(tc.state, tc.expectedState) {
				t.Fatalf("expected state \n%v\n but got \n%v", util.DumpJSON(tc.expectedState), util.DumpJSON(tc.state))
			}

			originPodJS, _ := json.Marshal(pod)
			patchJS, _ := json.Marshal(tc.expectedPatch)
			expectedPodJS, err := strategicpatch.StrategicMergePatch(originPodJS, patchJS, &v1.Pod{})
			if err != nil {
				t.Fatal(err)
			}
			expectedPod := &v1.Pod{}
			if err := json.Unmarshal(expectedPodJS, expectedPod); err != nil {
				t.Fatal(err)
			}
			expectedPod.Annotations[appspub.InPlaceUpdateStateKey] = util.DumpJSON(tc.state)
			if !apiequality.Semantic.DeepEqual(gotPod, expectedPod) {
				t.Fatalf("expected pod \n%v\n but got \n%v", util.DumpJSON(expectedPod), util.DumpJSON(gotPod))
			}
		})
	}
}

func createFakePod(imageInject, resourceInject, stateInject bool, num, imageOKNum, resourceOKNumber int) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				apps.StatefulSetRevisionLabel: "new-revision",
			},
			Annotations: map[string]string{},
			Name:        "test-pod",
		},
	}

	for i := 0; i < num; i++ {
		name := fmt.Sprintf("c%d", i)
		pod.Spec.Containers = append(pod.Spec.Containers, v1.Container{
			Name: name,
		})
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, v1.ContainerStatus{
			Name: name,
		})
	}
	state := appspub.InPlaceUpdateState{Revision: "new-revision", LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{}}
	for i := 0; i < num; i++ {
		imageID := fmt.Sprintf("img0%d", i)
		lastStatus := appspub.InPlaceUpdateContainerStatus{}
		if imageInject {
			pod.Spec.Containers[i].Image = fmt.Sprintf("busybox:test%d", i)
			pod.Status.ContainerStatuses[i].ImageID = imageID

			lastImgId := "different-img01"
			img := lastImgId
			if i < imageOKNum {
				// ok => imgId != lastImageId
				img = fmt.Sprintf("img0%d", i)
			}
			pod.Status.ContainerStatuses[i].ImageID = img
			lastStatus.ImageID = lastImgId
		}
		if resourceInject {
			defaultCPU := resource.MustParse("200m")
			defaultMem := resource.MustParse("200Mi")
			lastCPU := resource.MustParse("100m")
			lastMem := resource.MustParse("100Mi")
			pod.Spec.Containers[i].Resources = v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU:    defaultCPU,
					v1.ResourceMemory: defaultMem,
				},
			}
			pod.Status.ContainerStatuses[i].Resources = &v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU:    defaultCPU,
					v1.ResourceMemory: defaultMem,
				},
			}
			if i >= resourceOKNumber {
				pod.Status.ContainerStatuses[i].Resources = &v1.ResourceRequirements{
					Requests: map[v1.ResourceName]resource.Quantity{
						v1.ResourceCPU:    lastCPU,
						v1.ResourceMemory: lastMem,
					},
				}
			}
			lastStatus.Resources = v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU:    lastCPU,
					v1.ResourceMemory: lastMem,
				},
			}
		}
		state.LastContainerStatuses[pod.Spec.Containers[i].Name] = lastStatus
	}

	if stateInject {
		if resourceInject {
			state.UpdateResources = true
		}
		v, _ := json.Marshal(state)
		pod.Annotations[appspub.InPlaceUpdateStateKey] = string(v)
	}
	return pod
}

func TestDefaultCheckInPlaceUpdateCompleted_Resource(t *testing.T) {
	// 构建测试用例
	tests := []struct {
		name        string
		pod         *v1.Pod
		expectError bool
		vpaEnabled  bool
	}{
		// normal case with feature gate disabled
		{
			name:        "empty pod",
			pod:         createFakePod(false, false, false, 1, 0, 0),
			expectError: false,
			vpaEnabled:  false,
		},
		{
			name:        "image ok",
			pod:         createFakePod(true, false, true, 1, 1, 0),
			expectError: false,
			vpaEnabled:  false,
		},
		{
			name:        "image not ok",
			pod:         createFakePod(true, false, true, 1, 0, 0),
			expectError: true,
			vpaEnabled:  false,
		},
		{
			name:        "all image ok",
			pod:         createFakePod(true, false, true, 2, 2, 0),
			expectError: false,
			vpaEnabled:  false,
		},
		{
			name:        "remain image not ok",
			pod:         createFakePod(true, false, true, 2, 1, 0),
			expectError: true,
			vpaEnabled:  false,
		},
		{
			name:        "all image ok with resource ok",
			pod:         createFakePod(true, true, true, 2, 2, 2),
			expectError: false,
			vpaEnabled:  false,
		},
		{
			name:        "all image ok with resource not ok",
			pod:         createFakePod(true, true, true, 2, 2, 1),
			expectError: false,
			vpaEnabled:  false,
		},
		{
			name:        "remain image not ok with resource not ok",
			pod:         createFakePod(true, true, true, 2, 1, 1),
			expectError: true,
			vpaEnabled:  false,
		},
		// normal case with feature gate enabled
		{
			name:        "empty pod",
			pod:         createFakePod(false, false, false, 1, 0, 0),
			expectError: false,
			vpaEnabled:  true,
		},
		{
			name:        "image ok",
			pod:         createFakePod(true, false, true, 1, 1, 0),
			expectError: false,
			vpaEnabled:  true,
		},
		{
			name:        "image not ok",
			pod:         createFakePod(true, false, true, 1, 0, 0),
			expectError: true,
			vpaEnabled:  true,
		},
		{
			name:        "all image ok",
			pod:         createFakePod(true, false, true, 2, 2, 0),
			expectError: false,
			vpaEnabled:  true,
		},
		{
			name:        "remain image not ok",
			pod:         createFakePod(true, false, true, 2, 1, 0),
			expectError: true,
			vpaEnabled:  true,
		},
		{
			name:        "all image ok with resource ok",
			pod:         createFakePod(true, true, true, 2, 2, 2),
			expectError: false,
			vpaEnabled:  true,
		},
		{
			name:        "all image ok with resource not ok",
			pod:         createFakePod(true, true, true, 2, 2, 1),
			expectError: true,
			vpaEnabled:  true,
		},
		{
			name:        "remain image not ok with resource not ok",
			pod:         createFakePod(true, true, true, 2, 1, 1),
			expectError: true,
			vpaEnabled:  true,
		},
		{
			name:        "only resource not ok",
			pod:         createFakePod(true, true, true, 3, 3, 1),
			expectError: true,
			vpaEnabled:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("case: %v vpa-enabled: %v", tt.name, tt.vpaEnabled)
			defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.InPlaceWorkloadVerticalScaling, tt.vpaEnabled)()
			err := DefaultCheckInPlaceUpdateCompleted(tt.pod)
			if tt.expectError {
				//t.Logf("get error: %v", err)
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultCheckPodNeedsBeUnready(t *testing.T) {
	// Setup test cases
	tests := []struct {
		name       string
		pod        *v1.Pod
		spec       *UpdateSpec
		expected   bool
		vpaEnabled bool
	}{
		{
			name: "contains ReadinessGates, vpa disabled",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{},
					ReadinessGates: []v1.PodReadinessGate{
						{ConditionType: appspub.InPlaceUpdateReady},
					},
				},
			},
			spec: &UpdateSpec{
				VerticalUpdateOnly: true,
				ContainerResources: map[string]v1.ResourceRequirements{},
			},
			expected:   true,
			vpaEnabled: false,
		},
		{
			name: "contains no ReadinessGates1",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{},
				},
			},
			spec: &UpdateSpec{
				VerticalUpdateOnly: true,
				ContainerResources: map[string]v1.ResourceRequirements{},
			},
			expected:   false,
			vpaEnabled: false,
		},
		{
			name: "contains ReadinessGates, vpa enabled and VerticalUpdateOnly",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{},
					ReadinessGates: []v1.PodReadinessGate{
						{ConditionType: appspub.InPlaceUpdateReady},
					},
				},
			},
			spec: &UpdateSpec{
				VerticalUpdateOnly: true,
				ContainerResources: map[string]v1.ResourceRequirements{},
			},
			expected:   false,
			vpaEnabled: true,
		},
		{
			name: "contains ReadinessGates, vpa enabled but not VerticalUpdateOnly",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{},
					ReadinessGates: []v1.PodReadinessGate{
						{ConditionType: appspub.InPlaceUpdateReady},
					},
				},
			},
			spec: &UpdateSpec{
				VerticalUpdateOnly: false,
				ContainerResources: map[string]v1.ResourceRequirements{},
			},
			expected:   true,
			vpaEnabled: true,
		},
		{
			name: "contains no ReadinessGates2",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{},
				},
			},
			spec: &UpdateSpec{
				VerticalUpdateOnly: true,
				ContainerResources: map[string]v1.ResourceRequirements{},
			},
			expected:   false,
			vpaEnabled: true,
		},
		{
			name: "contains no ReadinessGates3",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{},
				},
			},
			spec: &UpdateSpec{
				VerticalUpdateOnly: false,
				ContainerResources: map[string]v1.ResourceRequirements{},
			},
			expected:   false,
			vpaEnabled: true,
		},
		{
			name: "contains ReadinessGates, other restart container",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "11",
							ResizePolicy: []v1.ContainerResizePolicy{
								{ResourceName: v1.ResourceCPU, RestartPolicy: v1.RestartContainer},
							},
						},
					},
					ReadinessGates: []v1.PodReadinessGate{
						{ConditionType: appspub.InPlaceUpdateReady},
					},
				},
			},
			spec: &UpdateSpec{
				VerticalUpdateOnly: true,
				ContainerResources: map[string]v1.ResourceRequirements{},
			},
			expected:   false,
			vpaEnabled: true,
		},
		{
			name: "contains ReadinessGates, resize cpu and cpu restart policy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "11",
							ResizePolicy: []v1.ContainerResizePolicy{
								{ResourceName: v1.ResourceCPU, RestartPolicy: v1.RestartContainer},
							},
						},
					},
					ReadinessGates: []v1.PodReadinessGate{
						{ConditionType: appspub.InPlaceUpdateReady},
					},
				},
			},
			spec: &UpdateSpec{
				VerticalUpdateOnly: true,
				ContainerResources: map[string]v1.ResourceRequirements{
					"11": {
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("100m"),
						},
					},
				},
			},
			expected:   true,
			vpaEnabled: true,
		},
		{
			name: "contains ReadinessGates, resize mem and cpu restart policy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "11",
							ResizePolicy: []v1.ContainerResizePolicy{
								{ResourceName: v1.ResourceCPU, RestartPolicy: v1.RestartContainer},
							},
						},
					},
					ReadinessGates: []v1.PodReadinessGate{
						{ConditionType: appspub.InPlaceUpdateReady},
					},
				},
			},
			spec: &UpdateSpec{
				VerticalUpdateOnly: true,
				ContainerResources: map[string]v1.ResourceRequirements{
					"11": {
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
				},
			},
			expected:   false,
			vpaEnabled: true,
		},
		{
			name: "contains ReadinessGates, resize mem and cpu restart policy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "11",
							ResizePolicy: []v1.ContainerResizePolicy{
								{ResourceName: v1.ResourceCPU, RestartPolicy: v1.RestartContainer},
								{ResourceName: v1.ResourceMemory, RestartPolicy: v1.NotRequired},
							},
						},
					},
					ReadinessGates: []v1.PodReadinessGate{
						{ConditionType: appspub.InPlaceUpdateReady},
					},
				},
			},
			spec: &UpdateSpec{
				VerticalUpdateOnly: true,
				ContainerResources: map[string]v1.ResourceRequirements{
					"11": {
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
				},
			},
			expected:   false,
			vpaEnabled: true,
		},
		{
			name: "contains ReadinessGates, vpa disabled and resize mem and cpu restart policy",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "11",
							ResizePolicy: []v1.ContainerResizePolicy{
								{ResourceName: v1.ResourceCPU, RestartPolicy: v1.RestartContainer},
								{ResourceName: v1.ResourceMemory, RestartPolicy: v1.NotRequired},
							},
						},
					},
					ReadinessGates: []v1.PodReadinessGate{
						{ConditionType: appspub.InPlaceUpdateReady},
					},
				},
			},
			spec: &UpdateSpec{
				VerticalUpdateOnly: true,
				ContainerResources: map[string]v1.ResourceRequirements{
					"11": {
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
				},
			},
			expected:   true,
			vpaEnabled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.InPlaceWorkloadVerticalScaling, tc.vpaEnabled)()
			result := defaultCheckPodNeedsBeUnready(tc.pod, tc.spec)
			assert.Equal(t, tc.expected, result)
		})
	}
}
