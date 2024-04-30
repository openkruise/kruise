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
	"testing"
	"time"

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
