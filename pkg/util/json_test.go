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

package util

import (
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "k8s.io/api/core/v1"
)

func TestDumpJson(t *testing.T) {
	object := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
					Env: []v1.EnvVar{
						{
							Name:  "nginx-env",
							Value: "value-1",
						},
					},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "nginx-volume",
							MountPath: "/data/nginx",
						},
					},
				},
				{
					Name:  "test-sidecar",
					Image: "test-image:v1",
					Env: []v1.EnvVar{
						{
							Name:  "IS_INJECTED",
							Value: "true",
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "nginx-volume",
				},
			},
		},
	}

	except := `{"metadata":{"creationTimestamp":null},"spec":{"volumes":[{"name":"nginx-volume"}],"containers":[{"name":"nginx","image":"nginx:1.15.1","env":[{"name":"nginx-env","value":"value-1"}],"resources":{},"volumeMounts":[{"name":"nginx-volume","mountPath":"/data/nginx"}]},{"name":"test-sidecar","image":"test-image:v1","env":[{"name":"IS_INJECTED","value":"true"}],"resources":{}}]},"status":{}}`
	if except != DumpJSON(object) {
		t.Errorf("expect %v but got %v", except, DumpJSON(object))
	}

}

type TimeTestCase struct {
	TestTime *metav1.Time `json:"testTime"`
}

func TestIsJSONEqual(t *testing.T) {
	now := metav1.Now()
	t1 := TimeTestCase{TestTime: &metav1.Time{Time: now.Time}}
	if t1.TestTime.Time.UnixMilli()%1000 < 3 {
		// avoid t1 1.001, and t2 0.999
		time.Sleep(time.Millisecond * 2)
		t1 = TimeTestCase{TestTime: &metav1.Time{Time: now.Time}}
	}
	t2 := TimeTestCase{TestTime: &metav1.Time{Time: now.Add(-time.Millisecond * 2)}}
	t3 := TimeTestCase{TestTime: &metav1.Time{Time: now.Add(-time.Second * 2)}}

	if reflect.DeepEqual(t1, t2) {
		t.Fatalf("expect t1 not equal to t2")
	}
	if reflect.DeepEqual(t1, t3) {
		t.Fatalf("expect t1 not equal to t2")
	}

	if !IsJSONObjectEqual(t1, t2) {
		t.Fatalf("expect t1 json equal to t2: %v, %v", t1.TestTime.Time, t2.TestTime.Time)
	}
	if IsJSONObjectEqual(t1, t3) {
		t.Fatalf("expect t1 not json equal to t3")
	}
}
